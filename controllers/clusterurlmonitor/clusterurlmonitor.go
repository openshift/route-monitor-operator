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
	"sigs.k8s.io/controller-runtime/pkg/manager"

	monitoringv1alpha1 "github.com/openshift/route-monitor-operator/api/v1alpha1"
	"github.com/openshift/route-monitor-operator/controllers"
	"github.com/openshift/route-monitor-operator/pkg/alert"
	"github.com/openshift/route-monitor-operator/pkg/blackboxexporter"
	reconcileCommon "github.com/openshift/route-monitor-operator/pkg/reconcile"
	"github.com/openshift/route-monitor-operator/pkg/servicemonitor"
	utilreconcile "github.com/openshift/route-monitor-operator/pkg/util/reconcile"
)

// ClusterUrlMonitorReconciler reconciles a ClusterUrlMonitor object
type ClusterUrlMonitorReconciler struct {
	Client client.Client
	Ctx    context.Context
	Log    logr.Logger
	Scheme *runtime.Scheme

	BlackBoxExporter controllers.BlackBoxExporterHandler
	ServiceMonitor   controllers.ServiceMonitorHandler
	Prom             controllers.PrometheusRuleHandler
	Common           controllers.MonitorResourceHandler
	Hypershift       bool
}

func NewReconciler(mgr manager.Manager, blackboxExporterImage, blackboxExporterNamespace string, enablehypershift bool) *ClusterUrlMonitorReconciler {
	log := ctrl.Log.WithName("controllers").WithName("ClusterUrlMonitor")
	client := mgr.GetClient()
	ctx := context.Background()
	return &ClusterUrlMonitorReconciler{
		Client:           client,
		Ctx:              ctx,
		Log:              log,
		Hypershift:       enablehypershift,
		Scheme:           mgr.GetScheme(),
		BlackBoxExporter: blackboxexporter.New(client, log, ctx, blackboxExporterImage, blackboxExporterNamespace),
		ServiceMonitor:   servicemonitor.NewServiceMonitor(ctx, client, enablehypershift),
		Prom:             alert.NewPrometheusRule(ctx, client),
		Common:           reconcileCommon.NewMonitorResourceCommon(ctx, client),
	}
}

const (
	FinalizerKey string = "clusterurlmonitor.routemonitoroperator.monitoring.openshift.io/finalizer"
	// PrevFinalizerKey is here until migration to new key is done
	PrevFinalizerKey string = "clusterurlmonitor.monitoring.openshift.io/clusterurlmonitorcontroller"
)

// +kubebuilder:rbac:groups=monitoring.openshift.io,resources=clusterurlmonitors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.openshift.io,resources=clusterurlmonitors/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=config.openshift.io,resources=dnses,verbs=get;list;watch
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=prometheusrules,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups=config.openshift.io,resources=clusterversions,verbs=get;list;watch

func (r *ClusterUrlMonitorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Ctx = ctx
	log := r.Log.WithName("Reconcile").WithValues("name", req.Name, "namespace", req.Namespace)

	log.V(2).Info("Entering GetClusterUrlMonitor")
	clusterUrlMonitor, res, err := r.GetClusterUrlMonitor(req)
	if err != nil {
		log.Error(err, "Failed to retreive ClusterUrlMonitor. Requeueing...")
		return utilreconcile.RequeueWith(err)
	}
	if res.ShouldStop() {
		return utilreconcile.Stop()
	}

	res, err = r.EnsureMonitorAndDependenciesAbsent(clusterUrlMonitor)
	if err != nil {
		log.Error(err, "Failed to delete ClusterUrlMontior. Requeueing...")
		return utilreconcile.RequeueWith(err)
	}
	if res.ShouldStop() {
		log.Info("Successfully deleted ClusterUrlMonitor. Finished Reconcile")
		return utilreconcile.Stop()
	}

	log.V(2).Info("Entering EnsureFinalizerSet")
	res, err = r.EnsureFinalizerSet(clusterUrlMonitor)
	if err != nil {
		log.Error(err, "Failed to set ClusterUrlMonitor's Finalizer. Requeueing...")
		return utilreconcile.RequeueWith(err)
	}
	if res.ShouldStop() {
		log.Info("Successfully set ClusterUrlMonitor finalizers. Stopping...")
		return utilreconcile.Stop()
	}

	log.V(2).Info("Entering EnsureBlackBoxExporterResourcesExist")
	err = r.BlackBoxExporter.EnsureBlackBoxExporterResourcesExist()
	if err != nil {
		log.Error(err, "Failed to create BlackBoxExporter. Requeueing...")
		return utilreconcile.RequeueWith(err)
	}

	log.V(2).Info("Entering EnsureServiceMonitorExists")
	res, err = r.EnsureServiceMonitorExists(clusterUrlMonitor)
	if err != nil {
		log.Error(err, "Failed to set ServiceMonitor. Requeueing...")
		return utilreconcile.RequeueWith(err)
	}
	if res.ShouldStop() {
		log.Info("Successfully patched ClusterUrlMonitor with ServiceMonitorRef. Stopping...")
		return utilreconcile.Stop()
	}

	log.V(2).Info("Entering EnsurePrometheusRuleResourceExists")
	res, err = r.EnsurePrometheusRuleExists(clusterUrlMonitor)
	if err != nil {
		log.Error(err, "Failed to set PrometheusRule. Requeueing...")
		return utilreconcile.RequeueWith(err)
	}
	if res.ShouldStop() {
		log.Info("Successfully patched ClusterUrlMonitor with PrometheusRuleRef. Stopping...")
		return utilreconcile.Stop()
	}

	log.Info("All operations for ClusterUrlMonitor completed. Finished Reconcile.")
	return utilreconcile.Stop()
}

func (r *ClusterUrlMonitorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&monitoringv1alpha1.ClusterUrlMonitor{}).
		Complete(r)
}
