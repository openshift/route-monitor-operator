package routemonitor

import (
	"context"

	routev1 "github.com/openshift/api/route/v1"
	v1alpha1 "github.com/openshift/route-monitor-operator/api/v1alpha1"
	utilreconcile "github.com/openshift/route-monitor-operator/pkg/util/reconcile"

	ctrl "sigs.k8s.io/controller-runtime"
)

type RouteMonitorInterface interface {
	GetRouteMonitor(ctx context.Context, req ctrl.Request) (routeMonitor *v1alpha1.RouteMonitor, res utilreconcile.ReconcileOperation, err error)
	GetRoute(ctx context.Context, routeMonitor *v1alpha1.RouteMonitor) (*routev1.Route, error)
	UpdateRouteURL(ctx context.Context, route *routev1.Route, routeMonitor *v1alpha1.RouteMonitor) (utilreconcile.ReconcileOperation, error)
	CreateBlackBoxExporterResources(ctx context.Context) error
	CreateServiceMonitorResource(ctx context.Context, routeMonitor *v1alpha1.RouteMonitor) (utilreconcile.ReconcileOperation, error)
	DeleteRouteMonitorAndDependencies(ctx context.Context, routeMonitor *v1alpha1.RouteMonitor) (utilreconcile.ReconcileOperation, error)
	PerformBlackBoxExporterDeletion(ctx context.Context, routeMonitor *v1alpha1.RouteMonitor) error
}
