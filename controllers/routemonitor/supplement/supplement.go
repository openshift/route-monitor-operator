package supplement

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	routev1 "github.com/openshift/api/route/v1"
	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	"github.com/openshift/route-monitor-operator/controllers/routemonitor"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"

	routemonitorconst "github.com/openshift/route-monitor-operator/pkg/const"
	"github.com/openshift/route-monitor-operator/pkg/const/blackbox"
	customerrors "github.com/openshift/route-monitor-operator/pkg/util/errors"
	utilfinalizer "github.com/openshift/route-monitor-operator/pkg/util/finalizer"
	utilreconcile "github.com/openshift/route-monitor-operator/pkg/util/reconcile"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
)

// RouteMonitorSupplement hold additional actions that supplement the Reconcile
type RouteMonitorSupplement struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

func New(r routemonitor.RouteMonitorReconciler) *RouteMonitorSupplement {
	return &RouteMonitorSupplement{
		Client: r.Client,
		Log:    r.Log,
		Scheme: r.Scheme,
	}
}

// GetRouteMonitor return the RouteMonitor that is tested
func (r *RouteMonitorSupplement) GetRouteMonitor(ctx context.Context, req ctrl.Request) (routeMonitor v1alpha1.RouteMonitor, res utilreconcile.Result, err error) {
	err = r.Get(ctx, req.NamespacedName, &routeMonitor)
	if err != nil {
		// If this is an unknown error
		if !k8serrors.IsNotFound(err) {
			return
		}
		r.Log.V(2).Info("StopRequeue", "As RouteMonitor is 'NotFound', stopping requeue", nil)

		res = utilreconcile.StopOperation()
		err = nil
		return
	}
	res = utilreconcile.ContinueOperation()
	return
}

// GetRoute returns the Route from the RouteMonitor spec
func (r *RouteMonitorSupplement) GetRoute(ctx context.Context, routeMonitor v1alpha1.RouteMonitor) (routev1.Route, error) {
	res := routev1.Route{}
	nsName := types.NamespacedName{
		Name:      routeMonitor.Spec.Route.Name,
		Namespace: routeMonitor.Spec.Route.Namespace,
	}
	if nsName.Name == "" || nsName.Namespace == "" {
		err := errors.New("Invalid CR: Cannot retrieve route if one of the fields is empty")
		return res, err
	}

	err := r.Get(ctx, nsName, &res)
	if err != nil {
		return res, err
	}
	return res, nil
}

func (r *RouteMonitorSupplement) UpdateRouteURL(ctx context.Context, route routev1.Route, routeMonitor v1alpha1.RouteMonitor) (utilreconcile.Result, error) {
	amountOfIngress := len(route.Status.Ingress)
	if amountOfIngress == 0 {
		err := errors.New("No Ingress: cannot extract route url from the Route resource")
		return utilreconcile.RequeueReconcileWith(err)
	}
	extractedRouteURL := route.Status.Ingress[0].Host
	if amountOfIngress > 1 {
		r.Log.V(1).Info(fmt.Sprintf("Too many Ingress: assuming first ingress is the correct, chosen ingress '%s'", extractedRouteURL))
	}

	if extractedRouteURL == "" {
		return utilreconcile.RequeueReconcileWith(customerrors.NoHost)
	}

	currentRouteURL := routeMonitor.Status.RouteURL
	if currentRouteURL == extractedRouteURL {
		r.Log.V(3).Info("Same RouteURL: currentRouteURL and extractedRouteURL are equal, update not required")
		return utilreconcile.ContinueReconcile()
	}

	if currentRouteURL != "" && extractedRouteURL != currentRouteURL {
		r.Log.V(3).Info("RouteURL mismatch: currentRouteURL and extractedRouteURL are not equal, taking extractedRouteURL as source of truth")
	}

	routeMonitor.Status.RouteURL = extractedRouteURL
	err := r.Status().Update(ctx, &routeMonitor)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}
	return utilreconcile.StopReconcile()
}

func (r *RouteMonitorSupplement) CreateBlackBoxExporterResources(ctx context.Context) error {
	if err := r.CreateBlackBoxExporterDeployment(ctx); err != nil {
		return err
	}
	// Creating Service after because:
	//
	// A Service should not point to an empty target (Deployment)
	if err := r.CreateBlackBoxExporterService(ctx); err != nil {
		return err
	}
	return nil
}

func (r *RouteMonitorSupplement) CreateBlackBoxExporterDeployment(ctx context.Context) error {
	resource := appsv1.Deployment{}
	populationDeploymentFunc := func() appsv1.Deployment {
		return r.templateForBlackBoxExporterDeployment()
	}
	// Does the resource already exist?
	err := r.Get(ctx, blackbox.BlackBoxNamespacedName, &resource)
	if err != nil {
		// If this is an unknown error
		if !k8serrors.IsNotFound(err) {
			// return unexpectedly
			return err
		}
		// populate the resource with the template
		resource := populationDeploymentFunc()
		// and create it
		err = r.Create(ctx, &resource)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *RouteMonitorSupplement) CreateBlackBoxExporterService(ctx context.Context) error {
	resource := corev1.Service{}
	populationServiceFunc := func() corev1.Service {
		return r.templateForBlackBoxExporterService()
	}
	// Does the resource already exist?
	if err := r.Get(ctx, blackbox.BlackBoxNamespacedName, &resource); err != nil {
		// If this is an unknown error
		if !k8serrors.IsNotFound(err) {
			// return unexpectedly
			return err
		}
		// populate the resource with the template
		resource := populationServiceFunc()
		// and create it
		if err = r.Create(ctx, &resource); err != nil {
			return err
		}
	}
	return nil
}

func (r *RouteMonitorSupplement) CreateServiceMonitorResource(ctx context.Context, routeMonitor v1alpha1.RouteMonitor) (utilreconcile.Result, error) {
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
		return r.templateForServiceMonitorResource(routeMonitor, namespacedName.Name)
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

func (r *RouteMonitorSupplement) ShouldDeleteBlackBoxExporterResources(ctx context.Context, routeMonitor v1alpha1.RouteMonitor) (bool, error) {
	// if a delete has not been requested then there is at least one resource using the BlackBoxExporter
	if !routeMonitor.WasDeleteRequested() {
		return false, nil
	}

	routeMonitors := &v1alpha1.RouteMonitorList{}
	if err := r.List(ctx, routeMonitors); err != nil {
		return false, err
	}

	amountOfRouteMonitors := len(routeMonitors.Items)
	if amountOfRouteMonitors == 0 {
		err := errors.New("Internal Fault: Cannot be in reconcile loop and have not RouteMonitors on cluster")
		// the response is set to true as this technically a case where we should delete, but as it's not logical that this will happen, returning error
		return true, err
	}

	// If more than one resource exists, and the deletion was requsted there are stil at least one resource using the BlackBoxExporter
	if amountOfRouteMonitors > 1 {
		return false, nil
	}

	return true, nil
}

// deploymentForBlackBoxExporter returns a blackbox deployment
func (r *RouteMonitorSupplement) templateForBlackBoxExporterDeployment() appsv1.Deployment {
	labels := blackbox.GenerateBlackBoxLables()
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
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
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
func (r *RouteMonitorSupplement) templateForBlackBoxExporterService() corev1.Service {
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
func (r *RouteMonitorSupplement) templateForServiceMonitorResource(routeMonitor v1alpha1.RouteMonitor, serviceMonitorName string) monitoringv1.ServiceMonitor {

	routeURL := routeMonitor.Status.RouteURL

	routeMonitorLabels := blackbox.GenerateBlackBoxLables()
	var labelSelector = metav1.LabelSelector{}
	err := metav1.Convert_Map_string_To_string_To_v1_LabelSelector(&routeMonitorLabels, &labelSelector, nil)
	if err != nil {
		r.Log.Error(err, "Failed to convert LabelSelector to it's components")
	}
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
