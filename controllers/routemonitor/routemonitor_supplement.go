package routemonitor

import (
	"context"

	// k8s packages
	k8serrors "k8s.io/apimachinery/pkg/api/errors"

	//api's used
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"

	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	"github.com/openshift/route-monitor-operator/pkg/consts/blackboxexporter"
	customerrors "github.com/openshift/route-monitor-operator/pkg/util/errors"
	utilreconcile "github.com/openshift/route-monitor-operator/pkg/util/reconcile"
	"github.com/openshift/route-monitor-operator/pkg/util/templates"
	"k8s.io/apimachinery/pkg/types"
)

//go:generate mockgen -source $GOFILE -destination ../../pkg/util/test/generated/mocks/$GOPACKAGE/blackboxexporter.go -package $GOPACKAGE BlackBoxExporter
type BlackBoxExporter interface {
	EnsureBlackBoxExporterResourcesExist() error
	EnsureBlackBoxExporterResourcesAbsent() error
	ShouldDeleteBlackBoxExporterResources() (blackboxexporter.ShouldDeleteBlackBoxExporter, error)
	GetBlackBoxExporterNamespace() string
}

func (r *RouteMonitorReconciler) EnsureRouteMonitorAndDependenciesAbsent(ctx context.Context, routeMonitor v1alpha1.RouteMonitor) (utilreconcile.Result, error) {
	log := r.Log.WithName("Delete")

	shouldDeleteBlackBoxResources, err := r.BlackBoxExporter.ShouldDeleteBlackBoxExporterResources()
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}
	log.V(2).Info("Response of ShouldDeleteBlackBoxExporterResources", "shouldDeleteBlackBoxResources", shouldDeleteBlackBoxResources)

	if shouldDeleteBlackBoxResources {
		log.V(2).Info("Entering ensureBlackBoxExporterResourcesAbsent")
		err := r.BlackBoxExporter.EnsureBlackBoxExporterResourcesAbsent()
		if err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
	}

	log.V(2).Info("Entering ensureServiceMonitorResourceAbsent")
	err = r.EnsureServiceMonitorResourceAbsent(ctx, routeMonitor)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}

	log.V(2).Info("Entering ensurePrometheusRuleResourceAbsent")
	err = r.EnsurePrometheusRuleResourceAbsent(ctx, routeMonitor)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}

	log.V(2).Info("Entering ensureFinalizerAbsent")
	// only the last command can throw the result (as no matter what happens it will stop)
	_, err = r.EnsureFinalizerAbsent(ctx, routeMonitor)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}
	return utilreconcile.StopReconcile()
}

func (r *RouteMonitorReconciler) EnsurePrometheusRuleResourceExists(ctx context.Context, routeMonitor v1alpha1.RouteMonitor) (utilreconcile.Result, error) {
	shouldHave, err, parsedSlo := shouldCreatePrometheusRule(routeMonitor)

	res, err := r.updateErrorStatus(ctx, routeMonitor, err)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}
	if res.ShouldStop() {
		return utilreconcile.StopReconcile()
	}

	if !shouldHave {
		err = r.EnsurePrometheusRuleResourceAbsent(ctx, routeMonitor)
		if err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
		return r.addPrometheusRuleRefToStatus(ctx, routeMonitor, types.NamespacedName{})
	}

	namespacedName := types.NamespacedName{Namespace: routeMonitor.Namespace, Name: routeMonitor.Name}

	resource := &monitoringv1.PrometheusRule{}
	populationFunc := func() monitoringv1.PrometheusRule {
		return templates.TemplateForPrometheusRuleResource(routeMonitor.Status.RouteURL, parsedSlo, namespacedName)
	}

	// Does the resource already exist?
	if err := r.Get(ctx, namespacedName, resource); err != nil {
		// If this is an unknown error
		if !k8serrors.IsNotFound(err) {
			// return unexpectedly
			return utilreconcile.RequeueReconcileWith(err)
		}
		// populate the resource with the template
		resource := populationFunc()
		// and create it
		err = r.Create(ctx, &resource)
		if err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
	}

	res, err = r.addPrometheusRuleRefToStatus(ctx, routeMonitor, namespacedName)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}
	if res.ShouldStop() {
		return utilreconcile.StopReconcile()
	}

	return utilreconcile.ContinueReconcile()
}

func (r *RouteMonitorReconciler) updateErrorStatus(ctx context.Context, routeMonitor v1alpha1.RouteMonitor, err error) (utilreconcile.Result, error) {
	// If an error has already been flagged and still occurs
	if routeMonitor.Status.ErrorStatus != "" && err != nil {
		// Skip as the resource should not be created
		return utilreconcile.ContinueReconcile()
	}

	// If the error was flagged but stopped firing
	if routeMonitor.Status.ErrorStatus != "" && err == nil {
		// Clear the error and restart
		routeMonitor.Status.ErrorStatus = ""
		err := r.Status().Update(ctx, &routeMonitor)
		if err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
		return utilreconcile.StopReconcile()
	}

	// If the error was not flagged but has started firing
	if routeMonitor.Status.ErrorStatus == "" && err != nil {
		// Raise the alert and restart
		routeMonitor.Status.ErrorStatus = err.Error()
		err := r.Status().Update(ctx, &routeMonitor)
		if err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
		return utilreconcile.StopReconcile()
	}

	// only case left is ErrorStatus == "" && err == nil
	// so there is no need to check if err != nil
	return utilreconcile.ContinueReconcile()
}

func (r *RouteMonitorReconciler) addPrometheusRuleRefToStatus(ctx context.Context, routeMonitor v1alpha1.RouteMonitor, namespacedName types.NamespacedName) (utilreconcile.Result, error) {
	desiredPrometheusRuleRef := v1alpha1.NamespacedName{
		Namespace: namespacedName.Namespace,
		Name:      namespacedName.Name,
	}
	if routeMonitor.Status.PrometheusRuleRef != desiredPrometheusRuleRef {
		// Update status with PrometheusRuleRef
		routeMonitor.Status.PrometheusRuleRef = desiredPrometheusRuleRef
		err := r.Status().Update(ctx, &routeMonitor)
		if err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
		return utilreconcile.StopReconcile()
	}
	return utilreconcile.ContinueReconcile()
}

func shouldCreatePrometheusRule(routeMonitor v1alpha1.RouteMonitor) (bool, error, string) {
	// Was the RouteURL populated by a previous step?
	if routeMonitor.Status.RouteURL == "" {
		return false, customerrors.NoHost, ""
	}

	// Is the SloSpec configured on this CR?
	if routeMonitor.Spec.Slo == *new(v1alpha1.SloSpec) {
		return false, nil, ""
	}
	isValid, parsedSlo := routeMonitor.Spec.Slo.IsValid()
	if !isValid {
		return false, customerrors.InvalidSLO, ""
	}
	return true, nil, parsedSlo
}
