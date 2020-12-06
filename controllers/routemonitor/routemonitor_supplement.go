package routemonitor

import (
	"context"

	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	"github.com/openshift/route-monitor-operator/pkg/consts/blackbox"
	"github.com/openshift/route-monitor-operator/pkg/util/finalizer"
	utilreconcile "github.com/openshift/route-monitor-operator/pkg/util/reconcile"
)

//go:generate mockgen -source $GOFILE -destination ../../pkg/util/test/generated/mocks/$GOPACKAGE/blackboxexporter.go -package $GOPACKAGE BlackboxExporter
type BlackboxExporter interface {
	EnsureBlackBoxExporterResourcesExist() error
	EnsureBlackBoxExporterResourcesAbsent() error
	ShouldDeleteBlackBoxExporterResources() (blackbox.ShouldDeleteBlackBoxExporter, error)
}

func (r *RouteMonitorReconciler) EnsureRouteMonitorAndDependenciesAbsent(ctx context.Context, routeMonitor v1alpha1.RouteMonitor) (utilreconcile.Result, error) {
	log := r.Log.WithName("Delete")

	shouldDeleteBlackBoxResources, err := r.BlackboxExporter.ShouldDeleteBlackBoxExporterResources()
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}
	log.V(2).Info("Response of ShouldDeleteBlackBoxExporterResources", "shouldDeleteBlackBoxResources", shouldDeleteBlackBoxResources)

	if shouldDeleteBlackBoxResources {
		log.V(2).Info("Entering ensureBlackBoxExporterResourcesAbsent")
		err := r.BlackboxExporter.EnsureBlackBoxExporterResourcesAbsent()
		if err != nil {
			return utilreconcile.RequeueReconcileWith(err)
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

// ensureServiceMonitoRelatedResourcesrAbsent assumes that the ServiceMonitor that is related was deleted
func (r *RouteMonitorReconciler) ensureServiceMonitoRelatedResourcesrAbsent(ctx context.Context, routeMonitor v1alpha1.RouteMonitor) (utilreconcile.Result, error) {
	// if the monitor is not deleting no action is needed
	if !finalizer.WasDeleteRequested(&routeMonitor) {
		return utilreconcile.ContinueReconcile()
	}
	err := r.Delete(ctx, &routeMonitor)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}
	return utilreconcile.ContinueReconcile()
}
