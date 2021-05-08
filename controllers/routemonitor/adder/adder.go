package adder

import (
	"context"

	"github.com/go-logr/logr"

	// k8s packages

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	//api's used

	//local packages
	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	"github.com/openshift/route-monitor-operator/controllers/routemonitor"
	"github.com/openshift/route-monitor-operator/pkg/consts"
	routemonitorconst "github.com/openshift/route-monitor-operator/pkg/consts"
	"github.com/openshift/route-monitor-operator/pkg/util"
	customerrors "github.com/openshift/route-monitor-operator/pkg/util/errors"
	"github.com/openshift/route-monitor-operator/pkg/util/finalizer"
	utilfinalizer "github.com/openshift/route-monitor-operator/pkg/util/finalizer"
	utilreconcile "github.com/openshift/route-monitor-operator/pkg/util/reconcile"
	"github.com/openshift/route-monitor-operator/pkg/util/templates"
)

// RouteMonitorAdder hold additional actions that supplement the Reconcile
type RouteMonitorAdder struct {
	client.Client
	Log                       logr.Logger
	Scheme                    *runtime.Scheme
	BlackBoxExporterNamespace string
	ClusterID                 string
	util.UtilCollection 
}

func New(r routemonitor.RouteMonitorReconciler, blackBoxExporterNamespace string) *RouteMonitorAdder {
	return &RouteMonitorAdder{
		Client:                    r.Client,
		Log:                       r.Log,
		Scheme:                    r.Scheme,
		BlackBoxExporterNamespace: blackBoxExporterNamespace,
	}
}

func (r *RouteMonitorAdder) EnsureServiceMonitorResourceExists(ctx context.Context, routeMonitor v1alpha1.RouteMonitor) (utilreconcile.Result, error) {
	// Was the RouteURL populated by a previous step?
	if routeMonitor.Status.RouteURL == "" {
		return utilreconcile.RequeueReconcileWith(customerrors.NoHost)
	}
	if r.ClusterID == "" {
		r.ClusterID = util.GetClusterID(r.Client)
	}

	namespacedName := types.NamespacedName{Name: routeMonitor.Name, Namespace: routeMonitor.Namespace}
	serviceMonitorTemplate := templates.TemplateForServiceMonitorResource(routeMonitor.Status.RouteURL, r.BlackBoxExporterNamespace, namespacedName, r.ClusterID)

	err := r.UpdateServiceMonitorDeplyoment(r.Client, serviceMonitorTemplate)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}

	//Update RouteMonitor serviceMonitorRef
	desiredRef := v1alpha1.NamespacedName{
		Name:      routeMonitor.Name,
		Namespace: routeMonitor.Namespace,
	}

	if routeMonitor.Status.ServiceMonitorRef == (v1alpha1.NamespacedName{}) {
		routeMonitor.Status.ServiceMonitorRef = desiredRef
		err := r.Status().Update(ctx, &routeMonitor)
		if err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
		return utilreconcile.StopReconcile()
	} else if routeMonitor.Status.ServiceMonitorRef != desiredRef {
		return utilreconcile.RequeueReconcileWith(customerrors.InvalidReferenceUpdate)
	}

	return utilreconcile.ContinueReconcile()
}

func (r *RouteMonitorAdder) EnsureFinalizerSet(ctx context.Context, routeMonitor v1alpha1.RouteMonitor) (utilreconcile.Result, error) {
	if !finalizer.HasFinalizer(&routeMonitor, consts.FinalizerKey) {
		// If the routeMonitor doesn't have a finalizer, add it
		utilfinalizer.Add(&routeMonitor, routemonitorconst.FinalizerKey)
		if err := r.Update(ctx, &routeMonitor); err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
		return utilreconcile.StopReconcile()
	}
	return utilreconcile.ContinueReconcile()
}
