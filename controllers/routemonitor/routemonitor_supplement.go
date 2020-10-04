package routemonitor

import (
	"context"

	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	utilfinalizer "github.com/openshift/route-monitor-operator/pkg/util/finalizer"
	utilreconcile "github.com/openshift/route-monitor-operator/pkg/util/reconcile"

	routemonitorconst "github.com/openshift/route-monitor-operator/pkg/const"
)

func (r *RouteMonitorReconciler) DeleteBlackBoxExporterResources(ctx context.Context) error {
	// Deleting Service first because:
	//
	// a Service should not point to an empty target (Deployment)
	if err := r.DeleteBlackBoxExporterService(ctx); err != nil {
		return err
	}
	if err := r.DeleteBlackBoxExporterDeployment(ctx); err != nil {
		return err
	}
	return nil
}

func (r *RouteMonitorReconciler) DeleteRouteMonitorAndDependencies(ctx context.Context, routeMonitor v1alpha1.RouteMonitor) (utilreconcile.Result, error) {

	err := r.DeleteServiceMonitorResource(ctx, routeMonitor)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}

	if routeMonitor.HasFinalizer() {
		// if finalizer is still here and ServiceMonitor is deleted, then remove the finalizer
		utilfinalizer.Remove(&routeMonitor, routemonitorconst.FinalizerKey)
		if err := r.Update(ctx, &routeMonitor); err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
		// After any modification we need to requeue to prevent two threads working on the same code
		return utilreconcile.StopReconcile()
	}

	// if the monitor is not deleting no action is needed
	if !routeMonitor.WasDeleteRequested() {
		return utilreconcile.ContinueReconcile()
	}
	err = r.Delete(ctx, &routeMonitor)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}
	return utilreconcile.ContinueReconcile()
}

func (r *RouteMonitorReconciler) PerformRouteMonitorDeletion(ctx context.Context, routeMonitor v1alpha1.RouteMonitor) (utilreconcile.Result, error) {
	log := r.Log.WithName("Delete")
	shouldDeleteBlackBoxResources, err := r.ShouldDeleteBlackBoxExporterResources(ctx, routeMonitor)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}
	log.V(2).Info("Tested ShouldDeleteBlackBoxExporterResources", "shouldDeleteBlackBoxResources", shouldDeleteBlackBoxResources)

	// if this is the last resource then delete the blackbox-exporter resources and then delete the RouteMonitor
	if shouldDeleteBlackBoxResources {
		log.V(2).Info("Entering DeleteBlackBoxExporterResources")
		err := r.DeleteBlackBoxExporterResources(ctx)
		if err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
	}

	log.V(2).Info("Entering DeleteRouteMonitorAndDependencies")
	deleteRouteMonitorResult, err := r.DeleteRouteMonitorAndDependencies(ctx, routeMonitor)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}
	if !deleteRouteMonitorResult.Continue {
		return deleteRouteMonitorResult, nil
	}

	return utilreconcile.StopReconcile()
}
