package clusterurlmonitor

import (
	"reflect"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	"github.com/openshift/route-monitor-operator/pkg/alert"
	blackboxexporterconsts "github.com/openshift/route-monitor-operator/pkg/consts/blackboxexporter"
	utilreconcile "github.com/openshift/route-monitor-operator/pkg/util/reconcile"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

// Takes care that right PrometheusRules for the defined ClusterURLMonitor are in place
func (s *ClusterUrlMonitorReconciler) EnsurePrometheusRuleExists(clusterUrlMonitor v1alpha1.ClusterUrlMonitor) (utilreconcile.Result, error) {
	clusterDomain, err := s.GetClusterDomain()
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}

	spec := clusterUrlMonitor.Spec
	clusterUrl := spec.Prefix + clusterDomain + ":" + spec.Port + spec.Suffix
	parsedSlo, err := s.Common.ParseMonitorSLOSpecs(clusterUrl, clusterUrlMonitor.Spec.Slo)

	if s.Common.SetErrorStatus(&clusterUrlMonitor.Status.ErrorStatus, err) {
		return s.Common.UpdateMonitorResourceStatus(&clusterUrlMonitor)
	}
	if parsedSlo == "" {
		err = s.Prom.DeletePrometheusRuleDeployment(clusterUrlMonitor.Status.PrometheusRuleRef)
		if err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
		updated, _ := s.Common.SetResourceReference(&clusterUrlMonitor.Status.PrometheusRuleRef, types.NamespacedName{})
		if updated {
			return s.Common.UpdateMonitorResourceStatus(&clusterUrlMonitor)
		}
		return utilreconcile.StopReconcile()
	}

	namespacedName := types.NamespacedName{Namespace: clusterUrlMonitor.Namespace, Name: clusterUrlMonitor.Name}
	template := alert.TemplateForPrometheusRuleResource(clusterUrl, parsedSlo, namespacedName)
	err = s.Prom.UpdatePrometheusRuleDeployment(template)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}

	// Update PrometheusRuleReference in ClusterUrlMonitor if necessary
	updated, _ := s.Common.SetResourceReference(&clusterUrlMonitor.Status.PrometheusRuleRef, namespacedName)
	if updated {
		return s.Common.UpdateMonitorResourceStatus(&clusterUrlMonitor)
	}
	return utilreconcile.ContinueReconcile()
}

// Takes care that right ServiceMonitor for the defined ClusterURLMonitor are in place
func (s *ClusterUrlMonitorReconciler) EnsureServiceMonitorExists(clusterUrlMonitor v1alpha1.ClusterUrlMonitor) (utilreconcile.Result, error) {
	clusterDomain, err := s.GetClusterDomain()
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}

	namespacedName := types.NamespacedName{Name: clusterUrlMonitor.Name, Namespace: clusterUrlMonitor.Namespace}
	spec := clusterUrlMonitor.Spec
	clusterUrl := spec.Prefix + clusterDomain + ":" + spec.Port + spec.Suffix
	if err := s.ServiceMonitor.TemplateAndUpdateServiceMonitorDeployment(clusterUrl, s.BlackBoxExporter.GetBlackBoxExporterNamespace(), namespacedName, s.Common.GetClusterID()); err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}

	// Update RouteMonitor ServiceMonitorRef if required
	updated, err := s.Common.SetResourceReference(&clusterUrlMonitor.Status.ServiceMonitorRef, namespacedName)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}
	if updated {
		return s.Common.UpdateMonitorResourceStatus(&clusterUrlMonitor)
	}
	return utilreconcile.ContinueReconcile()
}

// Ensures that all dependencies related to a ClusterUrlMonitor are deleted
func (s *ClusterUrlMonitorReconciler) EnsureMonitorAndDependenciesAbsent(clusterUrlMonitor v1alpha1.ClusterUrlMonitor) (utilreconcile.Result, error) {
	if clusterUrlMonitor.DeletionTimestamp == nil {
		return utilreconcile.ContinueReconcile()
	}

	err := s.ServiceMonitor.DeleteServiceMonitorDeployment(clusterUrlMonitor.Status.ServiceMonitorRef)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}

	shouldDelete, err := s.BlackBoxExporter.ShouldDeleteBlackBoxExporterResources()
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}
	if shouldDelete == blackboxexporterconsts.DeleteBlackBoxExporter {
		err := s.BlackBoxExporter.EnsureBlackBoxExporterResourcesAbsent()
		if err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
	}

	err = s.Prom.DeletePrometheusRuleDeployment(clusterUrlMonitor.Status.PrometheusRuleRef)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}

	if s.Common.DeleteFinalizer(&clusterUrlMonitor, FinalizerKey) {
		// ignore the output as we want to remove the PrevFinalizerKey anyways
		s.Common.DeleteFinalizer(&clusterUrlMonitor, PrevFinalizerKey)
		return s.Common.UpdateMonitorResource(&clusterUrlMonitor)
	}

	return utilreconcile.ContinueReconcile()
}

func (s *ClusterUrlMonitorReconciler) EnsureFinalizerSet(clusterUrlMonitor v1alpha1.ClusterUrlMonitor) (utilreconcile.Result, error) {
	if s.Common.SetFinalizer(&clusterUrlMonitor, FinalizerKey) {
		// ignore the output as we want to remove the PrevFinalizerKey anyways
		s.Common.DeleteFinalizer(&clusterUrlMonitor, PrevFinalizerKey)
		return s.Common.UpdateMonitorResource(&clusterUrlMonitor)
	}
	return utilreconcile.ContinueReconcile()
}

// GetClusterUrlMonitor return the ClusterUrlMonitor that is tested
func (s *ClusterUrlMonitorReconciler) GetClusterUrlMonitor(req ctrl.Request) (v1alpha1.ClusterUrlMonitor, utilreconcile.Result, error) {
	ClusterUrlMonitor := v1alpha1.ClusterUrlMonitor{}
	err := s.Client.Get(s.Ctx, req.NamespacedName, &ClusterUrlMonitor)
	if err != nil {
		// If this is an unknown error
		if !k8serrors.IsNotFound(err) {
			res, err := utilreconcile.RequeueReconcileWith(err)
			return v1alpha1.ClusterUrlMonitor{}, res, err
		}
		s.Log.V(2).Info("StopRequeue", "As ClusterUrlMonitor is 'NotFound', stopping requeue", nil)

		return v1alpha1.ClusterUrlMonitor{}, utilreconcile.StopOperation(), nil
	}

	// if the resource is empty, we should terminate
	emptyClustUrlMonitor := v1alpha1.ClusterUrlMonitor{}
	if reflect.DeepEqual(ClusterUrlMonitor, emptyClustUrlMonitor) {
		return v1alpha1.ClusterUrlMonitor{}, utilreconcile.StopOperation(), nil
	}

	return ClusterUrlMonitor, utilreconcile.ContinueOperation(), nil
}

func (s *ClusterUrlMonitorReconciler) GetClusterDomain() (string, error) {
	clusterInfra := configv1.Infrastructure{}
	err := s.Client.Get(s.Ctx, types.NamespacedName{Name: "infrastructure"}, &clusterInfra)
	if err != nil {
		return "", err
	}
	return clusterInfra.Status.APIServerURL, nil
}
