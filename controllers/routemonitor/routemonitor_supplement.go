package routemonitor

import (
	"context"

	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	utilreconcile "github.com/openshift/route-monitor-operator/pkg/util/reconcile"
)

func (r *RouteMonitorReconciler) EnsureBlackBoxExporterResourcesExists(ctx context.Context) error {
	if err := r.EnsureBlackBoxExporterDeploymentExists(ctx); err != nil {
		return err
	}
	// Creating Service after because:
	//
	// A Service should not point to an empty target (Deployment)
	if err := r.EnsureBlackBoxExporterServiceExists(ctx); err != nil {
		return err
	}
	return nil
}

func (r *RouteMonitorReconciler) EnsureRouteMonitorAndDependenciesAbsent(ctx context.Context, routeMonitor v1alpha1.RouteMonitor) (utilreconcile.Result, error) {
	log := r.Log.WithName("Delete")

	shouldDeleteBlackBoxResources, err := r.ShouldDeleteBlackBoxExporterResources(ctx, routeMonitor)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}
	log.V(2).Info("Tested ShouldDeleteBlackBoxExporterResources", "shouldDeleteBlackBoxResources", shouldDeleteBlackBoxResources)

	if shouldDeleteBlackBoxResources {
		log.V(2).Info("Entering ensureBlackBoxExporterResourcesAbsent")
		res, err := r.ensureBlackBoxExporterResourcesAbsent(ctx, routeMonitor)
		if err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
		if res.ShouldStop() {
			return utilreconcile.StopReconcile()
		}
	}

	log.V(2).Info("Entering ensureServiceMonitorResourceAbsent")
	err = r.EnsureServiceMonitorResourceAbsent(ctx, routeMonitor)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}

	log.V(2).Info("Entering ensureRouteMonitorAbsent")
	res, err := r.ensureServiceMonitoRelatedResourcesrAbsent(ctx, routeMonitor)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}
	if res.ShouldStop() {
		return utilreconcile.StopReconcile()
	}

	log.V(2).Info("Entering ensureFinalizerAbsent")
	// only the last command can throw the result (as no matter what happens it will stop)
	_, err = r.EnsureFinalizerAbsent(ctx, routeMonitor)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}
	return utilreconcile.StopReconcile()
}

func (r *RouteMonitorReconciler) ensureBlackBoxExporterResourcesAbsent(ctx context.Context, routeMonitor v1alpha1.RouteMonitor) (utilreconcile.Result, error) {
	r.Log.V(2).Info("Entering EnsureBlackBoxExporterServiceAbsent")
	if err := r.EnsureBlackBoxExporterServiceAbsent(ctx); err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}
	r.Log.V(2).Info("Entering EnsureBlackBoxExporterDeploymentAbsent")
	if err := r.EnsureBlackBoxExporterDeploymentAbsent(ctx); err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}
	return utilreconcile.ContinueReconcile()
}

// ensureServiceMonitoRelatedResourcesrAbsent assumes that the ServiceMonitor that is related was deleted
func (r *RouteMonitorReconciler) ensureServiceMonitoRelatedResourcesrAbsent(ctx context.Context, routeMonitor v1alpha1.RouteMonitor) (utilreconcile.Result, error) {
	// if the monitor is not deleting no action is needed
	if !routeMonitor.WasDeleteRequested() {
		return utilreconcile.ContinueReconcile()
	}
	err := r.Delete(ctx, &routeMonitor)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}
	return utilreconcile.ContinueReconcile()
}
