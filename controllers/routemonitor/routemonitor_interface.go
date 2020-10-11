package routemonitor

import (
	"context"

	"github.com/openshift/route-monitor-operator/pkg/const/blackbox"
	utilreconcile "github.com/openshift/route-monitor-operator/pkg/util/reconcile"
	ctrl "sigs.k8s.io/controller-runtime"

	routev1 "github.com/openshift/api/route/v1"
	"github.com/openshift/route-monitor-operator/api/v1alpha1"
)

//go:generate mockgen -source $GOFILE -destination ../../pkg/util/tests/generated/mocks/$GOPACKAGE/routemonitor.go -package $GOPACKAGE RouteMonitorActionDoer,RouteMonitorDeleter,RouteMonitorAdder

type RouteMonitorSupplement interface {
	GetRouteMonitor(ctx context.Context, req ctrl.Request) (routeMonitor v1alpha1.RouteMonitor, res utilreconcile.Result, err error)
	GetRoute(ctx context.Context, routeMonitor v1alpha1.RouteMonitor) (routev1.Route, error)
	EnsureRouteURLExists(ctx context.Context, route routev1.Route, routeMonitor v1alpha1.RouteMonitor) (utilreconcile.Result, error)
}

type RouteMonitorDeleter interface {
	ShouldDeleteBlackBoxExporterResources(ctx context.Context, routeMonitor v1alpha1.RouteMonitor) (blackbox.ShouldDeleteBlackBoxExporter, error)
	EnsureBlackBoxExporterDeploymentAbsent(ctx context.Context) error
	EnsureBlackBoxExporterServiceAbsent(ctx context.Context) error
	EnsureServiceMonitorResourceAbsent(ctx context.Context, routeMonitor v1alpha1.RouteMonitor) error
}

type RouteMonitorAdder interface {
	EnsureBlackBoxExporterDeploymentExists(ctx context.Context) error
	EnsureBlackBoxExporterServiceExists(ctx context.Context) error
	EnsureServiceMonitorResourceExists(ctx context.Context, routeMonitor v1alpha1.RouteMonitor) (utilreconcile.Result, error)
}
