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
	utilfinalizer "github.com/openshift/route-monitor-operator/pkg/util/finalizer"
	utilreconcile "github.com/openshift/route-monitor-operator/pkg/util/reconcile"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
)

const (
	FinalizerKey string = "finalizer.routemonitor.openshift.io"
)

const ( // All things related BlackBoxExporter
	blackBoxNamespace  = "openshift-monitoring"
	blackBoxName       = "blackbox-exporter"
	blackBoxPortName   = "blackbox"
	blackBoxPortNumber = 9115
)

var ( // cannot be a const but doesn't ever change
	blackBoxNamespacedName = types.NamespacedName{Name: blackBoxName, Namespace: blackBoxNamespace}
)

// generateBlackBoxLables creates a set of common labels to most resources
// this function is here in case we need more labels in the future
func generateBlackBoxLables() map[string]string {
	return map[string]string{"app": blackBoxName}
}

// GetRouteMonitor return the RouteMonitor that is tested
func (r *RouteMonitorReconciler) GetRouteMonitor(ctx context.Context, req ctrl.Request) (routeMonitor v1alpha1.RouteMonitor, res utilreconcile.Result, err error) {
	err = r.Client.Get(ctx, req.NamespacedName, &routeMonitor)
	if err != nil {
		// If this is an unknown error
		if !k8serrors.IsNotFound(err) {
			return
		}
		r.Log.V(2).Info("StopRequeue", "As RouteMonitor is 'NotFound', stopping requeue", nil)

		res = utilreconcile.StopOperation()
		return
	}
	res = utilreconcile.ContinueOperation()
	return
}

// GetRoute returns the Route from the RouteMonitor spec
func (r *RouteMonitorReconciler) GetRoute(ctx context.Context, routeMonitor v1alpha1.RouteMonitor) (routev1.Route, error) {
	res := routev1.Route{}
	nsName := types.NamespacedName{
		Name:      routeMonitor.Spec.Route.Name,
		Namespace: routeMonitor.Spec.Route.Namespace,
	}
	if nsName.Name == "" || nsName.Namespace == "" {
		err := errors.New("Invalid CR: Cannot retrieve route if one of the fields is empty")
		return res, err
	}

	err := r.Client.Get(ctx, nsName, &res)
	if err != nil {
		return res, err
	}
	return res, nil
}

func (r *RouteMonitorReconciler) UpdateRouteURL(ctx context.Context, route routev1.Route, routeMonitor v1alpha1.RouteMonitor) (utilreconcile.Result, error) {
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
	err := r.Client.Status().Update(ctx, &routeMonitor)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}
	return utilreconcile.StopReconcile()
}

func (r *RouteMonitorReconciler) CreateBlackBoxExporterResources(ctx context.Context) error {
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

func (r *RouteMonitorReconciler) CreateBlackBoxExporterDeployment(ctx context.Context) error {
	resource := appsv1.Deployment{}
	populationDeploymentFunc := func() appsv1.Deployment {
		return r.templateForBlackBoxExporterDeployment()
	}
	// Does the resource already exist?
	err := r.Get(ctx, blackBoxNamespacedName, &resource)
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

func (r *RouteMonitorReconciler) CreateBlackBoxExporterService(ctx context.Context) error {
	resource := corev1.Service{}
	populationServiceFunc := func() corev1.Service {
		return r.templateForBlackBoxExporterService()
	}
	// Does the resource already exist?
	if err := r.Get(ctx, blackBoxNamespacedName, &resource); err != nil {
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

func (r *RouteMonitorReconciler) CreateServiceMonitorResource(ctx context.Context, routeMonitor v1alpha1.RouteMonitor) (utilreconcile.Result, error) {
	// Was the RouteURL populated by a previous step?
	if routeMonitor.Status.RouteURL == "" {
		return utilreconcile.RequeueReconcileWith(customerrors.NoHost)
	}

	if !r.HasFinalizer(routeMonitor) {
		// If the routeMonitor doesn't have a finalizer, add it
		utilfinalizer.Add(&routeMonitor, FinalizerKey)
		if err := r.Update(ctx, &routeMonitor); err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
		return utilreconcile.StopReconcile()
	}

	namespacedName := r.templateForServiceMonitorName(routeMonitor)

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

func (r *RouteMonitorReconciler) PerformRouteMonitorDeletion(ctx context.Context, routeMonitor v1alpha1.RouteMonitor) (utilreconcile.Result, error) {
	log := r.Log.WithName("Delete")
	shouldDeleteBlackBoxResources, err := r.ShouldDeleteBlackBoxExporterResources(ctx, routeMonitor)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}
	log.V(2).Info("Tested ShouldDeleteBlackBoxExporterResources", "shouldDeleteBlackBoxResources", shouldDeleteBlackBoxResources)

	// if this is the last resource then delete the blackbox-exporter resources and then delete the RouteMonitor
	if shouldDeleteBlackBoxResources {
		log.V(2).Info("Entering DeleteBlackBoxExporterResources")
		err := r.DeleteBlackBoxExporterResources(ctx)
		if err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
	}

	log.V(2).Info("Entering DeleteRouteMonitorAndDependencies")
	deleteRouteMonitorResult, err := r.DeleteRouteMonitorAndDependencies(ctx, routeMonitor)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}
	if !deleteRouteMonitorResult.Continue {
		return deleteRouteMonitorResult, nil
	}

	return utilreconcile.StopReconcile()
}

func (r *RouteMonitorReconciler) ShouldDeleteBlackBoxExporterResources(ctx context.Context, routeMonitor v1alpha1.RouteMonitor) (bool, error) {
	// if a delete has not been requested then there is at least one resource using the BlackBoxExporter
	if !r.WasDeleteRequested(routeMonitor) {
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

func (r *RouteMonitorReconciler) DeleteBlackBoxExporterResources(ctx context.Context) error {
	// Deleting Service first because:
	//
	// a Service should not point to an empty target (Deployment)
	if err := r.DeleteBlackBoxExporterService(ctx); err != nil {
		return err
	}
	if err := r.DeleteBlackBoxExporterDeployment(ctx); err != nil {
		return err
	}
	return nil
}

func (r *RouteMonitorReconciler) DeleteBlackBoxExporterDeployment(ctx context.Context) error {
	resource := &appsv1.Deployment{}

	// Does the resource already exist?
	err := r.Get(ctx, blackBoxNamespacedName, resource)
	if err != nil {
		// If this is an unknown error
		if !k8serrors.IsNotFound(err) {
			// return unexpectedly
			return err
		}
		// Resource doesn't exist, nothing to do
		return nil
	}
	err = r.Delete(ctx, resource)
	if err != nil {
		return err
	}
	return nil
}

func (r *RouteMonitorReconciler) DeleteBlackBoxExporterService(ctx context.Context) error {
	resource := &corev1.Service{}

	// Does the resource already exist?
	err := r.Get(ctx, blackBoxNamespacedName, resource)
	if err != nil {
		// If this is an unknown error
		if !k8serrors.IsNotFound(err) {
			// return unexpectedly
			return err
		}
		// Resource doesn't exist, nothing to do
		return nil
	}
	err = r.Delete(ctx, resource)
	if err != nil {
		return err
	}
	return nil
}

// DeleteServiceMonitorResourceCommand is purely for testing purposes
var DeleteServiceMonitorResourceCommand func(context.Context, v1alpha1.RouteMonitor) error

func (r *RouteMonitorReconciler) DeleteRouteMonitorAndDependencies(ctx context.Context, routeMonitor v1alpha1.RouteMonitor) (utilreconcile.Result, error) {
	if DeleteServiceMonitorResourceCommand == nil {
		DeleteServiceMonitorResourceCommand = r.DeleteServiceMonitorResource
	}

	err := DeleteServiceMonitorResourceCommand(ctx, routeMonitor)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}

	if r.HasFinalizer(routeMonitor) {
		// if finalizer is still here and ServiceMonitor is deleted, then remove the finalizer
		utilfinalizer.Remove(&routeMonitor, FinalizerKey)
		if err := r.Update(ctx, &routeMonitor); err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
		// After any modification we need to requeue to prevent two threads working on the same code
		return utilreconcile.StopReconcile()
	}

	// if the monitor is not deleting no action is needed
	if !r.WasDeleteRequested(routeMonitor) {
		return utilreconcile.ContinueReconcile()
	}
	err = r.Delete(ctx, &routeMonitor)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}
	return utilreconcile.ContinueReconcile()
}

func (r *RouteMonitorReconciler) DeleteServiceMonitorResource(ctx context.Context, routeMonitor v1alpha1.RouteMonitor) error {
	namespacedName := r.templateForServiceMonitorName(routeMonitor)
	resource := &monitoringv1.ServiceMonitor{}
	// Does the resource already exist?
	err := r.Get(ctx, namespacedName, resource)
	if err != nil {
		// If this is an unknown error
		if !k8serrors.IsNotFound(err) {
			// return unexpectedly
			return err
		}
		// Resource doesn't exist, nothing to do
		return nil
	}
	err = r.Delete(ctx, resource)
	if err != nil {
		return err
	}
	return nil
}

// Util functions

// WasDeleteRequested verifies if the resource was requested for deletion
func (r *RouteMonitorReconciler) WasDeleteRequested(routeMonitor v1alpha1.RouteMonitor) bool {
	return routeMonitor.DeletionTimestamp != nil
}

func (r *RouteMonitorReconciler) HasFinalizer(routeMonitor v1alpha1.RouteMonitor) bool {
	return utilfinalizer.Contains(routeMonitor.ObjectMeta.Finalizers, FinalizerKey)
}

// deploymentForBlackBoxExporter returns a blackbox deployment
func (r *RouteMonitorReconciler) templateForBlackBoxExporterDeployment() appsv1.Deployment {
	labels := generateBlackBoxLables()
	// hardcode the replicasize for no
	//replicas := m.Spec.Size
	var replicas int32 = 1

	dep := appsv1.Deployment{
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
func (r *RouteMonitorReconciler) templateForBlackBoxExporterService() corev1.Service {
	labels := generateBlackBoxLables()

	svc := corev1.Service{
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
func (r *RouteMonitorReconciler) templateForServiceMonitorResource(routeMonitor v1alpha1.RouteMonitor, serviceMonitorName string) monitoringv1.ServiceMonitor {

	routeURL := routeMonitor.Status.RouteURL

	routeMonitorLabels := generateBlackBoxLables()
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

// templateForServiceMonitorName return the generated name from the RouteMonitor.
// The name is joined by the name and the namespace to create a unique ServiceMonitor for each RouteMonitor
func (r *RouteMonitorReconciler) templateForServiceMonitorName(routeMonitor v1alpha1.RouteMonitor) types.NamespacedName {
	serviceMonitorName := fmt.Sprintf("%s-%s", routeMonitor.Name, routeMonitor.Namespace)
	return types.NamespacedName{Name: serviceMonitorName, Namespace: blackBoxNamespace}
}
