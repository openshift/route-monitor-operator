package controllers

import (
	"github.com/openshift/route-monitor-operator/pkg/consts/blackboxexporter"
	utilreconcile "github.com/openshift/route-monitor-operator/pkg/util/reconcile"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"

	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

//go:generate mockgen -source $GOFILE -destination ../pkg/util/test/generated/mocks/$GOPACKAGE/interfaces.go -package $GOPACKAGE

// ResourceMonitorHandler interface describes common behavior for handling the Monitors
type ResourceMonitorHandler interface {
	SetErrorStatus(errorStatus *string, err error) bool
	ParseSLOMonitorSpecs(routeURL string, sloSpec v1alpha1.SloSpec) (string, error)
	SetResourceReference(reference *v1alpha1.NamespacedName, target types.NamespacedName) (bool, error)

	UpdateReconciledMonitor(cr runtime.Object) (utilreconcile.Result, error)
	UpdateReconciledMonitorStatus(cr runtime.Object) (utilreconcile.Result, error)
	DeleteFinalizer(o v1.Object, finalizerKey string) bool
	SetFinalizer(o v1.Object, finalizerKey string) bool

	GetClusterID() string
}

type ServiceMonitorHandler interface {
	GetServiceMonitor(namespacedName types.NamespacedName) (monitoringv1.ServiceMonitor, error)
	UpdateServiceMonitorDeployment(template monitoringv1.ServiceMonitor) error
	DeleteServiceMonitorDeployment(serviceMonitorRef v1alpha1.NamespacedName) error
}

type PrometheusRuleHandler interface {
	UpdatePrometheusRuleDeployment(template monitoringv1.PrometheusRule) error
	DeletePrometheusRuleDeployment(prometheusRuleRef v1alpha1.NamespacedName) error
}

type BlackBoxExporterHandler interface {
	EnsureBlackBoxExporterResourcesExist() error
	EnsureBlackBoxExporterResourcesAbsent() error
	ShouldDeleteBlackBoxExporterResources() (blackboxexporter.ShouldDeleteBlackBoxExporter, error)
	GetBlackBoxExporterNamespace() string
}
