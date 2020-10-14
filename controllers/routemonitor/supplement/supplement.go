package supplement

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	routev1 "github.com/openshift/api/route/v1"
	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	"github.com/openshift/route-monitor-operator/controllers/routemonitor"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	customerrors "github.com/openshift/route-monitor-operator/pkg/util/errors"
	utilfinalizer "github.com/openshift/route-monitor-operator/pkg/util/finalizer"
	utilreconcile "github.com/openshift/route-monitor-operator/pkg/util/reconcile"

	routemonitorconst "github.com/openshift/route-monitor-operator/pkg/const"

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
func (r *RouteMonitorSupplement) GetRouteMonitor(ctx context.Context, req ctrl.Request) (v1alpha1.RouteMonitor, utilreconcile.Result, error) {
	routeMonitor := v1alpha1.RouteMonitor{}
	err := r.Get(ctx, req.NamespacedName, &routeMonitor)
	if err != nil {
		// If this is an unknown error
		if !k8serrors.IsNotFound(err) {
			res, err := utilreconcile.RequeueReconcileWith(err)
			return v1alpha1.RouteMonitor{}, res, err
		}
		r.Log.V(2).Info("StopRequeue", "As RouteMonitor is 'NotFound', stopping requeue", nil)

		return v1alpha1.RouteMonitor{}, utilreconcile.StopOperation(), nil
	}

	// if the resource is empty, we should terminate
	emptyRouteMonitor := v1alpha1.RouteMonitor{}
	if reflect.DeepEqual(routeMonitor, emptyRouteMonitor) {
		return v1alpha1.RouteMonitor{}, utilreconcile.StopOperation(), nil
	}

	return routeMonitor, utilreconcile.ContinueOperation(), nil
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

// EnsureRouteURLExists verifies that the .spec.RouteURL has the Route URL inside
func (r *RouteMonitorSupplement) EnsureRouteURLExists(ctx context.Context, route routev1.Route, routeMonitor v1alpha1.RouteMonitor) (utilreconcile.Result, error) {
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

func (r *RouteMonitorSupplement) EnsureFinalizerAbsent(ctx context.Context, routeMonitor v1alpha1.RouteMonitor) (utilreconcile.Result, error) {
	if routeMonitor.HasFinalizer() {
		// if finalizer is still here and ServiceMonitor is deleted, then remove the finalizer
		utilfinalizer.Remove(&routeMonitor, routemonitorconst.FinalizerKey)
		if err := r.Update(ctx, &routeMonitor); err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
		// After any modification we need to requeue to prevent two threads working on the same code
		return utilreconcile.StopReconcile()
	}
	return utilreconcile.ContinueReconcile()
}
