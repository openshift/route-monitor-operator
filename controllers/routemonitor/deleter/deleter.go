package deleter

import (
	"github.com/go-logr/logr"
	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	"github.com/openshift/route-monitor-operator/controllers/routemonitor"

	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"

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

func (r *RouteMonitorDeleter) EnsureServiceMonitorResourceAbsent(ctx context.Context, routeMonitor v1alpha1.RouteMonitor) error {
	namespacedName := types.NamespacedName{Name: routeMonitor.Status.ServiceMonitorRef.Name,
		Namespace: routeMonitor.Status.ServiceMonitorRef.Namespace}
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
