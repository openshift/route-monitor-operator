/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package clusterurlmonitor

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	"github.com/openshift/route-monitor-operator/controllers"
	"github.com/openshift/route-monitor-operator/pkg/alert"
	"github.com/openshift/route-monitor-operator/pkg/blackboxexporter"
	reconcileCommon "github.com/openshift/route-monitor-operator/pkg/reconcile"
	"github.com/openshift/route-monitor-operator/pkg/servicemonitor"
)

const FinalizerKey string = "clusterurlmonitor.routemonitoroperator.monitoring.openshift.io/finalizer"

// ClusterUrlMonitorReconciler reconciles a ClusterUrlMonitor object
type ClusterUrlMonitorReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger

	BlackBoxExporter controllers.BlackBoxExporterHandler
	ServiceMonitor   controllers.ServiceMonitorHandler
	Prom             controllers.PrometheusRuleHandler
	Common           controllers.MonitorResourceHandler
}

func NewReconciler(mgr manager.Manager, blackboxExporterImage, blackboxExporterNamespace string) *ClusterUrlMonitorReconciler {
	client := mgr.GetClient()
	return &ClusterUrlMonitorReconciler{
		Client:           client,
		Scheme:           mgr.GetScheme(),
		BlackBoxExporter: blackboxexporter.New(client, blackboxExporterImage, blackboxExporterNamespace),
		ServiceMonitor:   servicemonitor.NewServiceMonitor(client),
		Prom:             alert.NewPrometheusRule(client),
		Common:           reconcileCommon.NewMonitorResourceCommon(client),
	}
}

// +kubebuilder:rbac:groups=monitoring.openshift.io,resources=clusterurlmonitors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.openshift.io,resources=clusterurlmonitors/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=config.openshift.io,resources=dnses,verbs=get;list;watch
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=prometheusrules,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups=config.openshift.io,resources=clusterversions,verbs=get;list;watch
// +kubebuilder:rbac:groups=config.openshift.io,resources=infrastructures,verbs=get;list;watch
// +kubebuilder:rbac:groups=hypershift.openshift.io,resources=hostedcontrolplanes,verbs=get;list;watch
// +kubebuilder:rbac:groups=hypershift.openshift.io,resources=hostedclusters,verbs=get;list;watch

func (r *ClusterUrlMonitorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Log = ctrllog.FromContext(ctx).WithName("controller").WithName("ClusterUrlMonitor")

	clusterUrlMonitor := new(v1alpha1.ClusterUrlMonitor)
	if err := r.Get(ctx, req.NamespacedName, clusterUrlMonitor); err != nil {
		// Ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification).
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if clusterUrlMonitor.ObjectMeta.DeletionTimestamp.IsZero() {
		// The ClusterUrlMonitor is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object.
		if !controllerutil.ContainsFinalizer(clusterUrlMonitor, FinalizerKey) {
			controllerutil.AddFinalizer(clusterUrlMonitor, FinalizerKey)
			if err := r.Update(ctx, clusterUrlMonitor); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// The ClusterUrlMonitor is being deleted
		isHCP := clusterUrlMonitor.Spec.DomainRef == v1alpha1.ClusterDomainRefHCP
		if err := r.ServiceMonitor.DeleteServiceMonitorDeployment(ctx, clusterUrlMonitor.Status.ServiceMonitorRef, isHCP); err != nil {
			return ctrl.Result{}, err
		}

		shouldDelete, err := r.BlackBoxExporter.ShouldDeleteBlackBoxExporterResources(ctx)
		if err != nil {
			return ctrl.Result{}, err
		}
		if shouldDelete {
			if err := r.BlackBoxExporter.EnsureBlackBoxExporterResourcesAbsent(ctx); err != nil {
				return ctrl.Result{}, err
			}
		}

		if err := r.Prom.DeletePrometheusRuleDeployment(ctx, clusterUrlMonitor.Status.PrometheusRuleRef); err != nil {
			return ctrl.Result{}, err
		}

		// Finished cleaning up dependent resources, remove finalizer
		controllerutil.RemoveFinalizer(clusterUrlMonitor, FinalizerKey)
		if err := r.Update(ctx, clusterUrlMonitor); err != nil {
			return ctrl.Result{}, err
		}

		// Stop reconciliation as the item has been deleted
		return ctrl.Result{}, nil
	}

	if err := r.BlackBoxExporter.EnsureBlackBoxExporterResourcesExist(ctx); err != nil {
		r.Log.Error(err, "failed to create BlackBoxExporter")
		return ctrl.Result{}, err
	}

	res, err := r.EnsureServiceMonitorExists(ctx, *clusterUrlMonitor)
	if err != nil {
		r.Log.Error(err, "Failed to set ServiceMonitor. Requeueing...")
		return ctrl.Result{}, err
	}
	if res.ShouldStop() {
		r.Log.Info("Successfully patched ClusterUrlMonitor with ServiceMonitorRef. Stopping...")
		return ctrl.Result{}, nil
	}

	res, err = r.EnsurePrometheusRuleExists(ctx, *clusterUrlMonitor)
	if err != nil {
		r.Log.Error(err, "failed to set PrometheusRule")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *ClusterUrlMonitorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.ClusterUrlMonitor{}).
		Complete(r)
}
