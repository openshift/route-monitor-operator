package routemonitor

import (
	"context"

	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	utilfinalizer "github.com/openshift/route-monitor-operator/pkg/util/finalizer"
	utilreconcile "github.com/openshift/route-monitor-operator/pkg/util/reconcile"

	routemonitorconst "github.com/openshift/route-monitor-operator/pkg/const"
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
		return r.ensureBlackBoxExporterResourcesAbsent(ctx, routeMonitor)
	}

	log.V(2).Info("Entering ensureServiceMonitorResourceAbsent")
	err = r.EnsureServiceMonitorResourceAbsent(ctx, routeMonitor)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}

	res, err := r.ensureFinalizerAbsent(ctx, routeMonitor)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}
	if res.ShouldStop() {
		return res, nil
	}

	// the result of EnsureRouteMonitorAbsent is discarded because
	_, err = r.ensureRouteMonitorAbsent(ctx, routeMonitor)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}
	return utilreconcile.StopReconcile()
}

func (r *RouteMonitorReconciler) ensureBlackBoxExporterResourcesAbsent(ctx context.Context, routeMonitor v1alpha1.RouteMonitor) (utilreconcile.Result, error) {
	if err := r.EnsureBlackBoxExporterServiceAbsent(ctx); err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}
	if err := r.EnsureBlackBoxExporterDeploymentAbsent(ctx); err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}
	return utilreconcile.RequeueReconcile()
}

func (r *RouteMonitorReconciler) ensureFinalizerAbsent(ctx context.Context, routeMonitor v1alpha1.RouteMonitor) (utilreconcile.Result, error) {
	if routeMonitor.HasFinalizer() {
		// if finalizer is still here and ServiceMonitor is deleted, then remove the finalizer
		utilfinalizer.Remove(&routeMonitor, routemonitorconst.FinalizerKey)
		if err := r.Update(ctx, &routeMonitor); err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
		// After any modification we need to requeue to prevent two threads working on the same code
		return utilreconcile.StopReconcile()
	}
	return utilreconcile.ContinueReconcile()
}

// ensureRouteMonitorAbsent assumes that the ServiceMonitor that is related was deleted
func (r *RouteMonitorReconciler) ensureRouteMonitorAbsent(ctx context.Context, routeMonitor v1alpha1.RouteMonitor) (utilreconcile.Result, error) {
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
