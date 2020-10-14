package adder

import (
	"github.com/go-logr/logr"

	"context"

	// k8s packages
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	//api's used
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	//local packages
	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	"github.com/openshift/route-monitor-operator/controllers/routemonitor"
	routemonitorconst "github.com/openshift/route-monitor-operator/pkg/const"
	"github.com/openshift/route-monitor-operator/pkg/const/blackbox"
	customerrors "github.com/openshift/route-monitor-operator/pkg/util/errors"
	utilfinalizer "github.com/openshift/route-monitor-operator/pkg/util/finalizer"
	utilreconcile "github.com/openshift/route-monitor-operator/pkg/util/reconcile"
)

// RouteMonitorAdder hold additional actions that supplement the Reconcile
type RouteMonitorAdder struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

func New(r routemonitor.RouteMonitorReconciler) *RouteMonitorAdder {
	return &RouteMonitorAdder{
		Client: r.Client,
		Log:    r.Log,
		Scheme: r.Scheme,
	}
}

func (r *RouteMonitorAdder) EnsureBlackBoxExporterDeploymentExists(ctx context.Context) error {
	resource := appsv1.Deployment{}
	populationFunc := r.templateForBlackBoxExporterDeployment

	// Does the resource already exist?
	err := r.Get(ctx, blackbox.BlackBoxNamespacedName, &resource)
	if err != nil {
		// If this is an unknown error
		if !k8serrors.IsNotFound(err) {
			// return unexpectedly
			return err
		}
		// populate the resource with the template
		resource := populationFunc()
		// and create it
		err = r.Create(ctx, &resource)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *RouteMonitorAdder) EnsureBlackBoxExporterServiceExists(ctx context.Context) error {
	resource := corev1.Service{}
	populationFunc := r.templateForBlackBoxExporterService

	// Does the resource already exist?
	if err := r.Get(ctx, blackbox.BlackBoxNamespacedName, &resource); err != nil {
		// If this is an unknown error
		if !k8serrors.IsNotFound(err) {
			// return unexpectedly
			return err
		}
		// populate the resource with the template
		resource := populationFunc()
		// and create it
		if err = r.Create(ctx, &resource); err != nil {
			return err
		}
	}
	return nil
}

func (r *RouteMonitorAdder) EnsureServiceMonitorResourceExists(ctx context.Context, routeMonitor v1alpha1.RouteMonitor) (utilreconcile.Result, error) {
	// Was the RouteURL populated by a previous step?
	if routeMonitor.Status.RouteURL == "" {
		return utilreconcile.RequeueReconcileWith(customerrors.NoHost)
	}

	if !routeMonitor.HasFinalizer() {
		// If the routeMonitor doesn't have a finalizer, add it
		utilfinalizer.Add(&routeMonitor, routemonitorconst.FinalizerKey)
		if err := r.Update(ctx, &routeMonitor); err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
		return utilreconcile.StopReconcile()
	}

	namespacedName := routeMonitor.TemplateForServiceMonitorName()

	resource := &monitoringv1.ServiceMonitor{}
	populationFunc := func() monitoringv1.ServiceMonitor {
		return r.templateForServiceMonitorResource(routeMonitor)
	}

	// Does the resource already exist?
	if err := r.Get(ctx, namespacedName, resource); err != nil {
		// If this is an unknown error
		if !k8serrors.IsNotFound(err) {
			// return unexpectedly
			return utilreconcile.RequeueReconcileWith(err)
		}
		// populate the resource with the template
		resource := populationFunc()
		// and create it
		err = r.Create(ctx, &resource)
		if err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
	}

	return utilreconcile.ContinueReconcile()
}

// deploymentForBlackBoxExporter returns a blackbox deployment
func (*RouteMonitorAdder) templateForBlackBoxExporterDeployment() appsv1.Deployment {
	labels := blackbox.GenerateBlackBoxLables()
	labelSelectors := metav1.LabelSelector{
		MatchLabels: labels}
	// hardcode the replicasize for no
	//replicas := m.Spec.Size
	var replicas int32 = 1

	dep := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      blackbox.BlackBoxName,
			Namespace: blackbox.BlackBoxNamespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &labelSelectors,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Image: "prom/blackbox-exporter:master",
						Name:  "blackbox-exporter",
						Ports: []corev1.ContainerPort{{
							ContainerPort: blackbox.BlackBoxPortNumber,
							Name:          blackbox.BlackBoxPortName,
						}},
					}},
				},
			},
		},
	}
	return dep
}

// templateForBlackBoxExporterService returns a blackbox service
func (*RouteMonitorAdder) templateForBlackBoxExporterService() corev1.Service {
	labels := blackbox.GenerateBlackBoxLables()

	svc := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      blackbox.BlackBoxName,
			Namespace: blackbox.BlackBoxNamespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{{
				TargetPort: intstr.FromString(blackbox.BlackBoxPortName),
				Port:       blackbox.BlackBoxPortNumber,
				Name:       blackbox.BlackBoxPortName,
			}},
		},
	}
	return svc
}

// templateForServiceMonitorResource returns a ServiceMonitor
func (r *RouteMonitorAdder) templateForServiceMonitorResource(routeMonitor v1alpha1.RouteMonitor) monitoringv1.ServiceMonitor {

	routeURL := routeMonitor.Status.RouteURL
	serviceMonitorName := routeMonitor.TemplateForServiceMonitorName().Name

	routeMonitorLabels := blackbox.GenerateBlackBoxLables()

	labelSelector := metav1.LabelSelector{MatchLabels: routeMonitorLabels}

	// Currently we only support `http_2xx` as module
	// Still make it a variable so we can easily add functionality later
	modules := []string{"http_2xx"}

	params := map[string][]string{
		"module": modules,
		"target": {routeURL},
	}

	serviceMonitor := monitoringv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name: serviceMonitorName,
			// ServiceMonitors need to be in `openshift-monitoring` to be picked up by cluster-monitoring-operator
			Namespace: blackbox.BlackBoxNamespace,
		},
		Spec: monitoringv1.ServiceMonitorSpec{
			JobLabel: serviceMonitorName,
			Endpoints: []monitoringv1.Endpoint{
				{
					Port: blackbox.BlackBoxPortName,
					// Probe every 30s
					Interval: "30s",
					// Timeout has to be smaller than probe interval
					ScrapeTimeout: "15s",
					Path:          "/probe",
					Scheme:        "http",
					Params:        params,
					MetricRelabelConfigs: []*monitoringv1.RelabelConfig{
						{
							Replacement: routeURL,
							TargetLabel: "RouteMonitorUrl",
						},
					},
				}},
			Selector:          labelSelector,
			NamespaceSelector: monitoringv1.NamespaceSelector{},
		},
	}
	return serviceMonitor
}
