package routemonitor

import (
	"errors"
	"fmt"
	"reflect"

	routev1 "github.com/openshift/api/route/v1"
	"github.com/openshift/route-monitor-operator/api/v1alpha1"

	"github.com/openshift/route-monitor-operator/pkg/consts"
	customerrors "github.com/openshift/route-monitor-operator/pkg/util/errors"
	utilreconcile "github.com/openshift/route-monitor-operator/pkg/util/reconcile"
	"github.com/openshift/route-monitor-operator/pkg/util/templates"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

// Ensures that all PrometheusRules CR are created according to the RouteMonitor
func (r *RouteMonitorReconciler) EnsurePrometheusRuleExists(routeMonitor v1alpha1.RouteMonitor) (utilreconcile.Result, error) {
	fmt.Println("EnsurePrometheusRuleExists")
	shouldHave, err, parsedSlo := r.common.AreMonitorSettingsValid(routeMonitor.Status.RouteURL, routeMonitor.Spec.Slo)
	if r.common.SetErrorStatus(&routeMonitor.Status.ErrorStatus, err) {
		return r.common.UpdateReconciledMonitor(r.Ctx, r.Client, &routeMonitor)
	}
	if !shouldHave {
		// Delete existing PrometheusRules if required
		err = r.prom.DeletePrometheusRuleDeployment(r.Ctx, r.Client, routeMonitor.Status.PrometheusRuleRef)
		if err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
		updated, _ := r.common.SetResourceReference(&routeMonitor.Status.ServiceMonitorRef, types.NamespacedName{})
		if updated {
			return r.common.UpdateReconciledMonitor(r.Ctx, r.Client, &routeMonitor)
		}
		return utilreconcile.StopReconcile()
	}

	// Update PrometheusRule from templates
	namespacedName := types.NamespacedName{Namespace: routeMonitor.Namespace, Name: routeMonitor.Name}
	template := templates.TemplateForPrometheusRuleResource(routeMonitor.Status.RouteURL, parsedSlo, namespacedName)

	err = r.UpdatePrometheusRuleDeployment(ctx, r.Client, template)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}

	// Update PrometheusRuleReference in RouteMonitor if necessary
	updated, _ := r.common.SetResourceReference(&routeMonitor.Status.PrometheusRuleRef, namespacedName)
	if updated {
		return r.common.UpdateReconciledMonitor(r.Ctx, r.Client, &routeMonitor)
	}
	return utilreconcile.ContinueReconcile()
}

// Ensures that a ServiceMonitor is created from the RouteMonitor CR
func (r *RouteMonitorReconciler) EnsureServiceMonitorExists(routeMonitor v1alpha1.RouteMonitor) (utilreconcile.Result, error) {
	fmt.Println("EnsureServiceMonitorExists")
	if r.ClusterID == "" {
		r.ClusterID = r.common.GetClusterID(r.Client)
	}
	// Was the RouteURL populated by a previous step?
	if routeMonitor.Status.RouteURL == "" {
		return utilreconcile.RequeueReconcileWith(customerrors.NoHost)
	}

	// update ServiceMonitor if requiredctrl
	namespacedName := types.NamespacedName{Name: routeMonitor.Name, Namespace: routeMonitor.Namespace}
	serviceMonitorTemplate := templates.TemplateForServiceMonitorResource(routeMonitor.Status.RouteURL, r.BlackBoxExporter.GetBlackBoxExporterNamespace(), namespacedName, r.ClusterID)
	err := r.serviceMonitor.UpdateServiceMonitorDeployment(r.Ctx, r.Client, serviceMonitorTemplate)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}

	// update ServiceMonitorRef if required
	updated, err := r.common.SetResourceReference(&routeMonitor.Status.ServiceMonitorRef, namespacedName)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}
	if updated {
		return r.common.UpdateReconciledMonitor(r.Ctx, r.Client, &routeMonitor)
	}
	return utilreconcile.ContinueReconcile()
}

// GetRouteMonitor return the RouteMonitor that is tested
func (r *RouteMonitorReconciler) GetRouteMonitor(req ctrl.Request) (v1alpha1.RouteMonitor, utilreconcile.Result, error) {
	fmt.Println("GetRouteMonitor")
	routeMonitor := v1alpha1.RouteMonitor{}
	err := r.Client.Get(r.Ctx, req.NamespacedName, &routeMonitor)
	if err != nil {
		// If this is an unknown error
		if !k8serrors.IsNotFound(err) {
			res, err := utilreconcile.RequeueReconcileWith(err)
			return v1alpha1.RouteMonitor{}, res, err
		}
		r.Log.V(2).Info("StopRequeue", "As RouteMonitor is 'NotFound', stopping requeue", nil)
		return v1alpha1.RouteMonitor{}, utilreconcile.StopOperation(), nil
	}

	// if the resource is empty, we should terminate
	emptyRouteMonitor := v1alpha1.RouteMonitor{}
	if reflect.DeepEqual(routeMonitor, emptyRouteMonitor) {
		return v1alpha1.RouteMonitor{}, utilreconcile.StopOperation(), nil
	}

	return routeMonitor, utilreconcile.ContinueOperation(), nil
}

// GetRoute returns the Route from the RouteMonitor spec
func (r *RouteMonitorReconciler) GetRoute(routeMonitor v1alpha1.RouteMonitor) (routev1.Route, error) {
	fmt.Println("GetRoute")
	res := routev1.Route{}
	nsName := types.NamespacedName{
		Name:      routeMonitor.Spec.Route.Name,
		Namespace: routeMonitor.Spec.Route.Namespace,
	}
	if nsName.Name == "" || nsName.Namespace == "" {
		err := errors.New("Invalid CR: Cannot retrieve route if one of the fields is empty")
		return res, err
	}

	err := r.Client.Get(r.Ctx, nsName, &res)
	if err != nil {
		return res, err
	}
	return res, nil
}

// EnsureRouteURLExists verifies that the .spec.RouteURL has the Route URL inside
func (r *RouteMonitorReconciler) EnsureRouteURLExists(route routev1.Route, routeMonitor v1alpha1.RouteMonitor) (utilreconcile.Result, error) {
	fmt.Println("EnsureRouteURLExists")
	amountOfIngress := len(route.Status.Ingress)
	if amountOfIngress == 0 {
		err := errors.New("No Ingress: cannot extract route url from the Route resource")
		return utilreconcile.RequeueReconcileWith(err)
	}
	extractedRouteURL := route.Status.Ingress[0].Host
	if amountOfIngress > 1 {
		r.Log.V(1).Info(fmt.Sprintf("Too many Ingress: assuming first ingress is the correct, chosen ingress '%s'", extractedRouteURL))
	}

	if extractedRouteURL == "" {
		return utilreconcile.RequeueReconcileWith(customerrors.NoHost)
	}

	currentRouteURL := routeMonitor.Status.RouteURL
	if currentRouteURL == extractedRouteURL {
		r.Log.V(3).Info("Same RouteURL: currentRouteURL and extractedRouteURL are equal, update not required")
		return utilreconcile.ContinueReconcile()
	}

	if currentRouteURL != "" && extractedRouteURL != currentRouteURL {
		r.Log.V(3).Info("RouteURL mismatch: currentRouteURL and extractedRouteURL are not equal, taking extractedRouteURL as source of truth")
	}

	routeMonitor.Status.RouteURL = extractedRouteURL
	return r.common.UpdateReconciledMonitor(r.Ctx, r.Client, &routeMonitor)
}

// Ensures that all dependencies related to a RouteMonitor are deleted
func (r *RouteMonitorReconciler) EnsureRouteMonitorAndDependenciesAbsent(routeMonitor v1alpha1.RouteMonitor) (utilreconcile.Result, error) {
	fmt.Println("EnsureRouteMonitorAndDependenciesAbsent")
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
	err = r.serviceMonitor.DeleteServiceMonitorDeployment(r.Ctx, r.Client, routeMonitor.Status.ServiceMonitorRef)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}

	log.V(2).Info("Entering ensurePrometheusRuleResourceAbsent")
	err = r.prom.DeletePrometheusRuleDeployment(r.Ctx, r.Client, routeMonitor.Status.PrometheusRuleRef)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}

	log.V(2).Info("Entering ensureFinalizerAbsent")
	if r.common.DeleteFinalizer(&routeMonitor, consts.FinalizerKey) {
		return r.common.UpdateReconciledMonitor(r.Ctx, r.Client, &routeMonitor)
	}
	return utilreconcile.StopReconcile()
}
