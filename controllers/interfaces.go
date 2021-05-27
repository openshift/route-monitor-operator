package controllers

import (
	"context"

	routev1 "github.com/openshift/api/route/v1"
	"github.com/openshift/route-monitor-operator/pkg/consts/blackboxexporter"
	utilreconcile "github.com/openshift/route-monitor-operator/pkg/util/reconcile"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:generate mockgen -source $GOFILE -destination ../pkg/util/test/generated/mocks/$GOPACKAGE/interfaces.go -package $GOPACKAGE
type MonitorReconciler interface {
	EnsurePrometheusRuleExists() (utilreconcile.Result, error)
	EnsureServiceMonitorExists() (utilreconcile.Result, error)
	EnsureRouteMonitorAndDependenciesAbsent() (utilreconcile.Result, error)
}

type RouteMonitorSupplement interface {
	MonitorReconciler
	EnsureRouteURLExists(route routev1.Route, routeMonitor v1alpha1.RouteMonitor) (utilreconcile.Result, error)
	GetRouteMonitor(req ctrl.Request) (routeMonitor v1alpha1.RouteMonitor, res utilreconcile.Result, err error)
	GetRoute(routeMonitor v1alpha1.RouteMonitor) (routev1.Route, error)
}

type ClusterURLMonitorSupplement interface {
	MonitorReconciler
	GetClusterUrlMonitor(req ctrl.Request, ctx context.Context) (v1alpha1.ClusterUrlMonitor, utilreconcile.Result, error)
	ProcessRequest() (ctrl.Result, error)
}

// MonitorReconcileCommon interface describes common behavior for handling the Monitors
type MonitorReconcileCommon interface {
	SetResourceReference(reference *v1alpha1.NamespacedName, targetNamespace types.NamespacedName) (bool, error)
	SetErrorStatus(errorStatus *string, err error) bool
	UpdateReconciledMonitor(ctx context.Context, c client.Client, cr runtime.Object) (utilreconcile.Result, error)
	DeleteFinalizer(o v1.Object, finalizerKey string) bool
	SetFinalizer(o v1.Object, finalizerKey string) bool
	AreMonitorSettingsValid(routeURL string, sloSpec v1alpha1.SloSpec) (bool, error, string)
	GetClusterID(c client.Client) string
	GetClusterDomain(ctx context.Context, c client.Client) (string, error)
}

type ServiceMonitor interface {
	GetServiceMonitor(ctx context.Context, c client.Client, namespacedName types.NamespacedName) (monitoringv1.ServiceMonitor, error)
	UpdateServiceMonitorDeployment(ctx context.Context, c client.Client, template monitoringv1.ServiceMonitor) error
	DeleteServiceMonitorDeployment(ctx context.Context, c client.Client, serviceMonitorRef v1alpha1.NamespacedName) error
}

type PrometheusRule interface {
	UpdatePrometheusRuleDeployment(ctx context.Context, c client.Client, template monitoringv1.PrometheusRule) error
	DeletePrometheusRuleDeployment(ctx context.Context, c client.Client, prometheusRuleRef v1alpha1.NamespacedName) error
}


//go:generate mockgen -source $GOFILE -destination ../../pkg/util/test/generated/mocks/$GOPACKAGE/blackboxexporter.go -package $GOPACKAGE BlackBoxExporter
type BlackBoxExporter interface {
	EnsureBlackBoxExporterResourcesExist() error
	EnsureBlackBoxExporterResourcesAbsent() error
	ShouldDeleteBlackBoxExporterResources() (blackboxexporter.ShouldDeleteBlackBoxExporter, error)
	GetBlackBoxExporterNamespace() string
}
