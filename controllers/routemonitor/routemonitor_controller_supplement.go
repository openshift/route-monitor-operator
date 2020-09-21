package routemonitor

import (
	"context"
	"errors"
	"fmt"

	routev1 "github.com/openshift/api/route/v1"
	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"

	customerrors "github.com/openshift/route-monitor-operator/pkg/util/errors"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
)

// GetRouteMonitor return the RouteMonitor that is tested
func (r *RouteMonitorReconciler) GetRouteMonitor(ctx context.Context, req ctrl.Request) (*v1alpha1.RouteMonitor, error) {
	res := &v1alpha1.RouteMonitor{}
	err := r.Client.Get(ctx, req.NamespacedName, res)
	if err != nil {
		return nil, err
	}
	return res, nil
}

// GetRoute returns the Route from the RouteMonitor spec
func (r *RouteMonitorReconciler) GetRoute(ctx context.Context, routeMonitor *v1alpha1.RouteMonitor) (*routev1.Route, error) {
	res := &routev1.Route{}
	nsName := types.NamespacedName{Name: routeMonitor.Spec.Route.Name, Namespace: routeMonitor.Spec.Route.Namespace}
	if nsName.Name == "" || nsName.Namespace == "" {
		err := errors.New("Invalid CR: Cannot retrieve route if one of the fields is empty")
		return nil, err
	}

	err := r.Client.Get(ctx, nsName, res)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (r *RouteMonitorReconciler) UpdateRouteURL(ctx context.Context, route *routev1.Route, routeMonitor *v1alpha1.RouteMonitor) (*ctrl.Result, error) {
	amountOfIngress := len(route.Status.Ingress)
	if amountOfIngress == 0 {
		err := errors.New("No Ingress: cannot extract route url from the Route resource")
		return nil, err
	}
	extractedRouteURL := route.Status.Ingress[0].Host
	if amountOfIngress > 1 {
		r.Log.V(1).Info(fmt.Sprintf("Too many Ingress: assuming first ingress is the correct, chosen ingress '%s'", extractedRouteURL))
	}

	if extractedRouteURL == "" {
		return nil, customerrors.NoHost
	}

	currentRouteURL := routeMonitor.Status.RouteURL
	if currentRouteURL == extractedRouteURL {
		r.Log.V(1).Info("Same RouteURL: currentRouteURL and extractedRouteURL are equal, update not required")
		return nil, nil
	}

	if currentRouteURL != "" && extractedRouteURL != currentRouteURL {
		r.Log.V(1).Info("RouteURL mismatch: currentRouteURL and extractedRouteURL are not equal, taking extractedRouteURL as source of truth")
	}

	routeMonitor.Status.RouteURL = extractedRouteURL
	err := r.Client.Status().Update(ctx, routeMonitor)
	if err != nil {
		return nil, err
	}
	// After each command that modifies the resource we are watching a new reconcile loop starts
	// in order for there not to be conflicts, we requeue the command and make this code more idempotent
	return &ctrl.Result{Requeue: true}, nil
}

func (r *RouteMonitorReconciler) CreateServiceMonitor(ctx context.Context, routeMonitor *v1alpha1.RouteMonitor) error {
	if routeMonitor.Status.RouteURL == "" {
		return customerrors.NoHost
	}
	// serviceMonitorName is joined by the name and the namespace to create a unique ServiceMonitor for each RouteMonitor
	serviceMonitorName := fmt.Sprintf("%s-%s", routeMonitor.Name, routeMonitor.Namespace)
	namespacedName := types.NamespacedName{Name: serviceMonitorName, Namespace: blackBoxNamespace}
	// PRE

	serviceMonitor := &monitoringv1.ServiceMonitor{}
	populationFunc := func() *monitoringv1.ServiceMonitor {
		return r.templateForServiceMonitorResource(routeMonitor, serviceMonitorName)
	}
	// Does the resource already exist?
	err := r.Get(ctx, namespacedName, serviceMonitor)
	if err != nil {
		// If this is an unknown error
		if !k8serrors.IsNotFound(err) {
			// return unexpectedly
			return err
		}
		// populate the resource with the template
		resource := populationFunc()
		// and create it
		err = r.Create(ctx, resource)
		if err != nil {
			return err
		}
	}
	return nil
}

const (
	blackBoxNamespace  = "openshift-monitoring"
	blackBoxName       = "blackbox-exporter"
	blackBoxPortName   = "blackbox"
	blackBoxPortNumber = 9115
)

var ( // cannot be a const but doesn't ever change
	blackBoxNamespacedName = types.NamespacedName{Name: blackBoxName, Namespace: blackBoxNamespace}
)

func (r *RouteMonitorReconciler) CreateBlackBoxExporterDeployment(ctx context.Context) error {
	resource := &appsv1.Deployment{}
	populationDeploymentFunc := func() *appsv1.Deployment {
		return r.templateForBlackBoxExporterDeployment()
	}
	// Does the resource already exist?
	err := r.Get(ctx, blackBoxNamespacedName, resource)
	if err != nil {
		// If this is an unknown error
		if !k8serrors.IsNotFound(err) {
			// return unexpectedly
			return err
		}
		// populate the resource with the template
		resource := populationDeploymentFunc()
		// and create it
		err = r.Create(ctx, resource)
		if err != nil {
			return err
		}
	}
	return nil
}
func (r *RouteMonitorReconciler) CreateBlackBoxExporterService(ctx context.Context) error {
	service := &corev1.Service{}
	populationServiceFunc := func() *corev1.Service {
		return r.templateForBlackBoxExporterService()
	}
	// Does the resource already exist?
	if err := r.Get(ctx, blackBoxNamespacedName, service); err != nil {
		// If this is an unknown error
		if !k8serrors.IsNotFound(err) {
			// return unexpectedly
			return err
		}
		// populate the resource with the template
		resource := populationServiceFunc()
		// and create it
		if err = r.Create(ctx, resource); err != nil {
			return err
		}
	}
	return nil
}
func (r *RouteMonitorReconciler) CreateBlackBoxExporterResources(ctx context.Context) error {
	if err := r.CreateBlackBoxExporterDeployment(ctx); err != nil {
		return err
	}
	if err := r.CreateBlackBoxExporterService(ctx); err != nil {
		return err
	}
	return nil
}

// deploymentForBlackBoxExporter returns a blackbox deployment
func (r *RouteMonitorReconciler) templateForBlackBoxExporterDeployment() *appsv1.Deployment {
	labels := commonTemplateLables()
	// hardcode the replicasize for no
	//replicas := m.Spec.Size
	var replicas int32 = 1

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      blackBoxName,
			Namespace: blackBoxNamespace,
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
							ContainerPort: blackBoxPortNumber,
							Name:          blackBoxPortName,
						}},
					}},
				},
			},
		},
	}
	return dep
}

// templateForBlackBoxExporterService returns a blackbox service
func (r *RouteMonitorReconciler) templateForBlackBoxExporterService() *corev1.Service {
	labels := commonTemplateLables()

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      blackBoxName,
			Namespace: blackBoxNamespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{{
				TargetPort: intstr.FromString(blackBoxPortName),
				Port:       blackBoxPortNumber,
				Name:       blackBoxPortName,
			}},
		},
	}
	return svc
}

// templateForServiceMonitorResource returns a ServiceMonitor
func (r *RouteMonitorReconciler) templateForServiceMonitorResource(routeMonitor *v1alpha1.RouteMonitor, serviceMonitorName string) *monitoringv1.ServiceMonitor {

	routeURL := routeMonitor.Status.RouteURL

	routeMonitorLabels := commonTemplateLables()
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

	serviceMonitor := &monitoringv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name: serviceMonitorName,
			// ServiceMonitors need to be in `openshift-monitoring` to be picked up by cluster-monitoring-operator
			Namespace: blackBoxNamespace,
		},
		Spec: monitoringv1.ServiceMonitorSpec{
			JobLabel: serviceMonitorName,
			Endpoints: []monitoringv1.Endpoint{
				{
					Port: blackBoxPortName,
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

// commonTemplateLables creates a set of common labels to most resources
// this function is here in case we need more labels in the future
func commonTemplateLables() map[string]string {
	return map[string]string{"app": blackBoxName}
}
