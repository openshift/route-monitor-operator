package clusterurlmonitor

import (
	"context"

	"github.com/go-logr/logr"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	"github.com/openshift/route-monitor-operator/pkg/blackboxexporter"
	"github.com/openshift/route-monitor-operator/pkg/consts/blackbox"
	customerrors "github.com/openshift/route-monitor-operator/pkg/util/errors"
	utilfinalizer "github.com/openshift/route-monitor-operator/pkg/util/finalizer"
	"github.com/openshift/route-monitor-operator/pkg/util/reconcile"
	utilreconcile "github.com/openshift/route-monitor-operator/pkg/util/reconcile"
	"github.com/openshift/route-monitor-operator/pkg/util/templates"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type BlackboxExporter interface {
	EnsureBlackBoxExporterResourcesExist() error
	EnsureBlackBoxExporterResourcesAbsent() error
	ShouldDeleteBlackBoxExporterResources() (blackbox.ShouldDeleteBlackBoxExporter, error)
}

type ClusterUrlMonitorSupplement struct {
	ClusterUrlMonitor v1alpha1.ClusterUrlMonitor
	Client            client.Client
	Log               logr.Logger
	Ctx               context.Context
	BlackboxExporter  BlackboxExporter
}

func NewSupplement(ClusterUrlMonitor v1alpha1.ClusterUrlMonitor, Client client.Client, log logr.Logger, BlackboxExporter *blackboxexporter.BlackboxExporter) *ClusterUrlMonitorSupplement {
	return &ClusterUrlMonitorSupplement{ClusterUrlMonitor, Client, log, context.Background(), BlackboxExporter}
}

func (s *ClusterUrlMonitorSupplement) EnsureFinalizer() (reconcile.Result, error) {
	if !utilfinalizer.Contains(s.ClusterUrlMonitor.GetFinalizers(), FinalizerKey) {
		utilfinalizer.Add(&s.ClusterUrlMonitor, FinalizerKey)
		err := s.Client.Update(s.Ctx, &s.ClusterUrlMonitor)
		return reconcile.RequeueReconcileWith(err)
	}
	return reconcile.ContinueReconcile()
}

func (s *ClusterUrlMonitorSupplement) EnsureServiceMonitorExists() (reconcile.Result, error) {

	namespacedName := types.NamespacedName{Name: s.ClusterUrlMonitor.Name, Namespace: s.ClusterUrlMonitor.Namespace}
	exists, err := s.doesServiceMonitorExist(namespacedName)
	if exists || err != nil {
		return reconcile.RequeueReconcileWith(err)
	}

	clusterDomain, err := s.getClusterDomain()
	if err != nil {
		return reconcile.RequeueReconcileWith(err)
	}

	spec := s.ClusterUrlMonitor.Spec
	clusterUrl := spec.Prefix + clusterDomain + ":" + spec.Port + spec.Suffix
	serviceMonitor := templates.TemplateForServiceMonitorResource(clusterUrl, namespacedName)

	// Does the resource already exist?
	if err := s.Client.Get(s.Ctx, namespacedName, &serviceMonitor); err != nil {
		// If this is an unknown error
		if !k8serrors.IsNotFound(err) {
			// return unexpectedly
			return utilreconcile.RequeueReconcileWith(err)
		}
		// and create it
		err = s.Client.Create(s.Ctx, &serviceMonitor)
		if err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
	}

	//Update status with serviceMonitorRef
	desiredRef := v1alpha1.NamespacedName{
		Name:      namespacedName.Name,
		Namespace: namespacedName.Namespace,
	}
	emptyRef := v1alpha1.NamespacedName{}

	currentRef := s.ClusterUrlMonitor.Status.ServiceMonitorRef
	if currentRef != emptyRef && desiredRef != currentRef {
		return utilreconcile.RequeueReconcileWith(customerrors.InvalidReferenceUpdate)
	}

	if currentRef == emptyRef && desiredRef != emptyRef {
		s.ClusterUrlMonitor.Status.ServiceMonitorRef = desiredRef

		err := s.Client.Status().Update(s.Ctx, &s.ClusterUrlMonitor)
		if err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
		return utilreconcile.StopReconcile()

	}
	return utilreconcile.ContinueReconcile()
}

func (s *ClusterUrlMonitorSupplement) EnsurePrometheusRuleResourceExists() (utilreconcile.Result, error) {
	shouldCreate, err, parsedSlo := s.shouldCreatePrometheusRule()
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	} else if !shouldCreate {
		return utilreconcile.ContinueReconcile()
	}

	clusterDomain, err := s.getClusterDomain()
	if err != nil {
		return reconcile.RequeueReconcileWith(err)
	}

	namespacedName := types.NamespacedName{Name: s.ClusterUrlMonitor.Name, Namespace: s.ClusterUrlMonitor.Namespace}

	spec := s.ClusterUrlMonitor.Spec
	clusterUrl := spec.Prefix + clusterDomain + ":" + spec.Port + spec.Suffix
	sourceCRName := "ClusterUrlMonitor"
	prometheusRuleResource := templates.TemplateForPrometheusRuleResource(clusterUrl, parsedSlo, namespacedName, sourceCRName)

	// Does the resource already exist?
	if err := s.Client.Get(s.Ctx, namespacedName, &prometheusRuleResource); err != nil {
		// If this is an unknown error
		if !k8serrors.IsNotFound(err) {
			// return unexpectedly
			return utilreconcile.RequeueReconcileWith(err)
		}
		// populate the resource with the template
		// and create it
		err = s.Client.Create(s.Ctx, &prometheusRuleResource)
		if err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
	}

	res, err := s.addPrometheusRuleRefToStatus(namespacedName)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}
	if res.ShouldStop() {
		return utilreconcile.StopReconcile()
	}

	return utilreconcile.ContinueReconcile()
}

func (s *ClusterUrlMonitorSupplement) shouldCreatePrometheusRule() (bool, error, string) {
	// Is the SloSpec configured on this CR?
	if s.ClusterUrlMonitor.Spec.Slo == *new(v1alpha1.SloSpec) {
		return false, nil, ""
	}
	isValid, parsedSlo := s.ClusterUrlMonitor.Spec.Slo.IsValid()
	if !isValid {
		return false, customerrors.InvalidSLO, ""
	}
	return true, nil, parsedSlo
}

func (s *ClusterUrlMonitorSupplement) addPrometheusRuleRefToStatus(namespacedName types.NamespacedName) (utilreconcile.Result, error) {
	desiredPrometheusRuleRef := v1alpha1.NamespacedName{
		Namespace: namespacedName.Namespace,
		Name:      namespacedName.Name,
	}
	if s.ClusterUrlMonitor.Status.PrometheusRuleRef != desiredPrometheusRuleRef {
		// Update status with PrometheusRuleRef
		s.ClusterUrlMonitor.Status.PrometheusRuleRef = desiredPrometheusRuleRef
		err := s.Client.Status().Update(s.Ctx, &s.ClusterUrlMonitor)
		return utilreconcile.RequeueReconcileWith(err)
	}
	return utilreconcile.ContinueReconcile()
}

func (s *ClusterUrlMonitorSupplement) getClusterDomain() (string, error) {
	clusterConfig := configv1.Ingress{}
	err := s.Client.Get(s.Ctx, types.NamespacedName{Name: "cluster"}, &clusterConfig)
	if err != nil {
		return "", err
	}

	return clusterConfig.Spec.Domain, nil
}

func (s *ClusterUrlMonitorSupplement) EnsureDeletionProcessed() (reconcile.Result, error) {
	if s.ClusterUrlMonitor.DeletionTimestamp == nil {
		return reconcile.ContinueReconcile()
	}

	namespacedName := types.NamespacedName{Name: s.ClusterUrlMonitor.Name, Namespace: s.ClusterUrlMonitor.Namespace}
	serviceMonitor, err := s.getServiceMonitor(namespacedName)
	if err != nil && !k8serrors.IsNotFound(err) {
		return reconcile.RequeueReconcileWith(err)
	}

	if err == nil {
		err = s.Client.Delete(s.Ctx, &serviceMonitor)
		if err != nil {
			return reconcile.RequeueReconcileWith(err)
		}
	}

	shouldDelete, err := s.BlackboxExporter.ShouldDeleteBlackBoxExporterResources()
	if err != nil {
		return reconcile.RequeueReconcileWith(err)
	}
	if shouldDelete == blackbox.DeleteBlackBoxExporter {
		err := s.BlackboxExporter.EnsureBlackBoxExporterResourcesAbsent()
		if err != nil {
			return reconcile.RequeueReconcileWith(err)
		}
	}

	if utilfinalizer.Contains(s.ClusterUrlMonitor.GetFinalizers(), FinalizerKey) {
		utilfinalizer.Remove(&s.ClusterUrlMonitor, FinalizerKey)
		err = s.Client.Update(s.Ctx, &s.ClusterUrlMonitor)
		return reconcile.RequeueReconcileWith(err)
	}
	return reconcile.ContinueReconcile()
}

func (s *ClusterUrlMonitorSupplement) EnsurePrometheusRuleResourceAbsent() error {
	// nothing to delete, stopping early
	if s.ClusterUrlMonitor.Status.PrometheusRuleRef == *new(v1alpha1.NamespacedName) {
		return nil
	}
	namespacedName := types.NamespacedName{Name: s.ClusterUrlMonitor.Status.PrometheusRuleRef.Name,
		Namespace: s.ClusterUrlMonitor.Status.PrometheusRuleRef.Namespace}
	resource := &monitoringv1.PrometheusRule{}
	// Does the resource already exist?
	err := s.Client.Get(s.Ctx, namespacedName, resource)
	if err != nil {
		// If this is an unknown error
		if !k8serrors.IsNotFound(err) {
			// return unexpectedly
			return err
		}
		// Resource doesn't exist, nothing to do
		return nil
	}
	err = s.Client.Delete(s.Ctx, resource)
	if err != nil {
		return err
	}
	return nil
}

func (s *ClusterUrlMonitorSupplement) getServiceMonitor(namespacedName types.NamespacedName) (monitoringv1.ServiceMonitor, error) {
	serviceMonitor := monitoringv1.ServiceMonitor{}
	err := s.Client.Get(s.Ctx, namespacedName, &serviceMonitor)
	return serviceMonitor, err
}

func (s *ClusterUrlMonitorSupplement) doesServiceMonitorExist(namespacedName types.NamespacedName) (bool, error) {
	_, err := s.getServiceMonitor(namespacedName)
	if k8serrors.IsNotFound(err) {
		return false, nil
	}
	return true, err
}

func ProcessRequest(blackboxExporter *blackboxexporter.BlackboxExporter, sup *ClusterUrlMonitorSupplement) (ctrl.Result, error) {
	res, err := sup.EnsureDeletionProcessed()
	if err != nil {
		return utilreconcile.RequeueWith(err)
	}
	if res.ShouldStop() {
		return utilreconcile.Stop()
	}

	err = sup.EnsurePrometheusRuleResourceAbsent()
	if err != nil {
		return utilreconcile.RequeueWith(err)
	}

	res, err = sup.EnsureFinalizer()
	if err != nil {
		return utilreconcile.RequeueWith(err)
	}
	if res.ShouldStop() {
		return utilreconcile.Stop()
	}

	err = blackboxExporter.EnsureBlackBoxExporterResourcesExist()
	if err != nil {
		return utilreconcile.RequeueWith(err)
	}

	res, err = sup.EnsurePrometheusRuleResourceExists()
	if err != nil {
		return utilreconcile.RequeueWith(err)
	}
	if res.ShouldStop() {
		return utilreconcile.Stop()
	}

	res, err = sup.EnsureServiceMonitorExists()
	if err != nil {
		return utilreconcile.RequeueWith(err)
	}
	if res.ShouldStop() {
		return utilreconcile.Stop()
	}

	return utilreconcile.Stop()
}
