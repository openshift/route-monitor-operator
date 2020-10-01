package routemonitor

import (
	"context"

	utilreconcile "github.com/openshift/route-monitor-operator/pkg/util/reconcile"
	ctrl "sigs.k8s.io/controller-runtime"

	routev1 "github.com/openshift/api/route/v1"
	"github.com/openshift/route-monitor-operator/api/v1alpha1"
)

type RouteMonitorActionDoer interface {
	GetRouteMonitor(ctx context.Context, req ctrl.Request) (routeMonitor v1alpha1.RouteMonitor, res utilreconcile.Result, err error)
	GetRoute(ctx context.Context, routeMonitor v1alpha1.RouteMonitor) (routev1.Route, error)
	UpdateRouteURL(ctx context.Context, route routev1.Route, routeMonitor v1alpha1.RouteMonitor) (utilreconcile.Result, error)
	CreateBlackBoxExporterResources(ctx context.Context) error
	CreateBlackBoxExporterDeployment(ctx context.Context) error
	CreateBlackBoxExporterService(ctx context.Context) error
	CreateServiceMonitorResource(ctx context.Context, routeMonitor v1alpha1.RouteMonitor) (utilreconcile.Result, error)
	PerformRouteMonitorDeletion(ctx context.Context, routeMonitor v1alpha1.RouteMonitor) (utilreconcile.Result, error)
	ShouldDeleteBlackBoxExporterResources(ctx context.Context, routeMonitor v1alpha1.RouteMonitor) (bool, error)
	DeleteBlackBoxExporterResources(ctx context.Context) error
	DeleteBlackBoxExporterDeployment(ctx context.Context) error
	DeleteBlackBoxExporterService(ctx context.Context) error
	DeleteRouteMonitorAndDependencies(ctx context.Context, routeMonitor v1alpha1.RouteMonitor) (utilreconcile.Result, error)
	DeleteServiceMonitorResource(ctx context.Context, routeMonitor v1alpha1.RouteMonitor) error
	WasDeleteRequested(routeMonitor v1alpha1.RouteMonitor) bool
	HasFinalizer(routeMonitor v1alpha1.RouteMonitor) bool
}
