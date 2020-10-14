package deleter

import (
	"github.com/go-logr/logr"
	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	"github.com/openshift/route-monitor-operator/controllers/routemonitor"
	"github.com/openshift/route-monitor-operator/pkg/const/blackbox"

	"context"
	"errors"
	"reflect"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
)

// RouteMonitorDeleter hold additional actions that supplement the Reconcile
type RouteMonitorDeleter struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

func New(r routemonitor.RouteMonitorReconciler) *RouteMonitorDeleter {
	return &RouteMonitorDeleter{
		Client: r.Client,
		Log:    r.Log,
		Scheme: r.Scheme,
	}
}

func (r *RouteMonitorDeleter) EnsureBlackBoxExporterDeploymentAbsent(ctx context.Context) error {
	resource := &appsv1.Deployment{}

	// Does the resource already exist?
	err := r.Get(ctx, blackbox.BlackBoxNamespacedName, resource)
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

func (r *RouteMonitorDeleter) EnsureBlackBoxExporterServiceAbsent(ctx context.Context) error {
	resource := &corev1.Service{}

	// Does the resource already exist?
	err := r.Get(ctx, blackbox.BlackBoxNamespacedName, resource)
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

func (r *RouteMonitorDeleter) EnsureServiceMonitorResourceAbsent(ctx context.Context, routeMonitor v1alpha1.RouteMonitor) error {
	namespacedName := routeMonitor.TemplateForServiceMonitorName()
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

func (r *RouteMonitorDeleter) ShouldDeleteBlackBoxExporterResources(ctx context.Context, routeMonitor v1alpha1.RouteMonitor) (blackbox.ShouldDeleteBlackBoxExporter, error) {

	// if a delete has not been requested then there is at least one resource using the BlackBoxExporter
	if !routeMonitor.WasDeleteRequested() {
		return blackbox.KeepBlackBoxExporter, nil
	}

	routeMonitors := &v1alpha1.RouteMonitorList{}
	if err := r.List(ctx, routeMonitors); err != nil {
		return blackbox.KeepBlackBoxExporter, err
	}

	amountOfRouteMonitors := len(routeMonitors.Items)
	// as this always shows up, should be logged out less then other results
	r.Log.V(4).Info("Current RouteMonitors Count:", "amountOfRouteMonitors", amountOfRouteMonitors)

	if amountOfRouteMonitors == 0 {
		err := errors.New("Internal Fault: Cannot be in reconcile loop and have not RouteMonitors on cluster")
		// the response is set to true as this technically a case where we should delete, but as it's not logical that this will happen, returning error
		return blackbox.KeepBlackBoxExporter, err
	}

	for _, route := range routeMonitors.Items {

		if !route.WasDeleteRequested() && !reflect.DeepEqual(route, routeMonitor) {
			r.Log.V(3).Info("Found Second RouteMonitor: found another RouteMonitor which not deleting")

			return blackbox.KeepBlackBoxExporter, nil
		}
	}

	r.Log.V(3).Info("Deleting BlackBoxResources: decided to clean BlackBoxExporter resources")
	return blackbox.DeleteBlackBoxExporter, nil
}
