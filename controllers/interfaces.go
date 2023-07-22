package controllers

import (
	"context"

	hypershiftv1beta1 "github.com/openshift/hypershift/api/v1beta1"
	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	"github.com/openshift/route-monitor-operator/pkg/consts/blackboxexporter"
	utilreconcile "github.com/openshift/route-monitor-operator/pkg/util/reconcile"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	rhobsv1 "github.com/rhobs/obo-prometheus-operator/pkg/apis/monitoring/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:generate mockgen -source $GOFILE -destination ../pkg/util/test/generated/mocks/$GOPACKAGE/interfaces.go -package $GOPACKAGE

// ResourceMonitorHandler interface describes common behavior for handling the Monitors
type MonitorResourceHandler interface {
	// SetErrorStatus updates the Error Status String within a monitor CR object
	// For the case the error is empty it flushes the string
	// It returns whether the status has been updated
	SetErrorStatus(errorStatus *string, err error) bool

	// ParseMonitorSLOSpecs extracts and validates the SLO targets and route endpoint
	// from the Spec. For the case they are valid, it returns the SLO in percent,
	// otherwise an error
	ParseMonitorSLOSpecs(routeURL string, sloSpec v1alpha1.SloSpec) (string, error)

	// SetResourceReference updates the ResourceRef in the Monitor Resources
	// It receives a pointer to the ref string within the monitor resource
	// In case the reference has been changed it returns true as a boolean
	SetResourceReference(reference *v1alpha1.NamespacedName, target types.NamespacedName) (bool, error)

	// UpdateMonitorResource updates the Spec of the ClusterURLMonitor & RouteMonitor CR
	// Should be called after object that triggered reconcile loop has been changed
	UpdateMonitorResource(cr client.Object) (utilreconcile.Result, error)

	// UpdateMonitorResourceStatus updates the State Field of the ClusterURLMonitor & RouteMonitor
	// Should be called after object that triggered reconcile loop has been changed
	UpdateMonitorResourceStatus(cr client.Object) (utilreconcile.Result, error)

	// SetFinalizer adds finalizerKey to an object
	SetFinalizer(o metav1.Object, finalizerKey string) bool

	// DeleteFinalizer removes Finalizer from object
	DeleteFinalizer(o metav1.Object, finalizerKey string) bool

	// GetClusterID fetches the Cluster ID
	GetOSDClusterID() (string, error)

	// GetHypershiftClusterID returns the Cluster ID based on the HostedControlPlane object in the provided namespace
	GetHypershiftClusterID(ns string) (string, error)

	// GetHCP fetches the HostedControlPlane for the hosted cluster the provided ClusterURLMonitor tracks
	GetHCP(ns string) (hypershiftv1beta1.HostedControlPlane, error)
}

type ServiceMonitorHandler interface {
	// UpdateServiceMonitorDeployment ensures that a ServiceMonitor deployment according
	// to the template exists. If none exists, it will create a new one.
	// If the template changed, it will update the existing deployment
	UpdateServiceMonitorDeployment(ctx context.Context, template monitoringv1.ServiceMonitor) error

	// TemplateAndUpdateServiceMonitorDeployment will generate a template and then
	// call UpdateServiceMonitorDeployment to ensure its current state matches the template.
	TemplateAndUpdateServiceMonitorDeployment(ctx context.Context, url, blackBoxExporterNamespace string, namespacedName types.NamespacedName, clusterID string, hcp bool) error

	// DeleteServiceMonitorDeployment deletes a ServiceMonitor refrenced by a namespaced name
	DeleteServiceMonitorDeployment(ctx context.Context, serviceMonitorRef v1alpha1.NamespacedName, hcp bool) error

	// HypershiftUpdateServiceMonitorDeployment is for HyperShift cluster to ensure that a ServiceMonitor deployment according
	// to the template exists. If none exists, it will create a new one. If the template changed, it will update the existing deployment
	HypershiftUpdateServiceMonitorDeployment(ctx context.Context, template rhobsv1.ServiceMonitor) error
}

type PrometheusRuleHandler interface {
	// UpdatePrometheusRuleDeployment ensures that a PrometheusRule deployment according
	// to the template exists. If none exists, it will create a new one.
	// If the template changed, it will update the existing deployment
	UpdatePrometheusRuleDeployment(ctx context.Context, template monitoringv1.PrometheusRule) error

	// DeletePrometheusRuleDeployment deletes a PrometheusRule refrenced by a namespaced name
	DeletePrometheusRuleDeployment(ctx context.Context, prometheusRuleRef v1alpha1.NamespacedName) error
}

type BlackBoxExporterHandler interface {
	EnsureBlackBoxExporterResourcesExist() error
	EnsureBlackBoxExporterResourcesAbsent() error
	ShouldDeleteBlackBoxExporterResources() (blackboxexporter.ShouldDeleteBlackBoxExporter, error)
	GetBlackBoxExporterNamespace() string
}
