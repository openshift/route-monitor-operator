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

const (
	FinalizerKey string = "clusterurlmonitor.monitoring.openshift.io/clusterurlmonitorcontroller"
)

// ClusterUrlMonitorReconciler reconciles a ClusterUrlMonitor object
type ClusterUrlMonitorReconciler struct {
	Client    client.Client
	Ctx       context.Context
	Log       logr.Logger
	Scheme    *runtime.Scheme
	ClusterID string

	BlackBoxExporter controllers.BlackBoxExporterHandler
	ServiceMonitor   controllers.ServiceMonitorHandler
	Prom             controllers.PrometheusRuleHandler
	Common           controllers.ResourceMonitorHandler
}

func NewReconciler(mgr manager.Manager, blackboxExporterImage, blackboxExporterNamespace string) *ClusterUrlMonitorReconciler {
	log := ctrl.Log.WithName("controllers").WithName("ClusterUrlMonitor")
	client := mgr.GetClient()
	ctx := context.Background()
	return &ClusterUrlMonitorReconciler{
		Client:           client,
		Ctx:              ctx,
		Log:              log,
		Scheme:           mgr.GetScheme(),
		BlackBoxExporter: blackboxexporter.New(client, log, ctx, blackboxExporterImage, blackboxExporterNamespace),
		ServiceMonitor:   servicemonitor.NewServiceMonitor(ctx, client),
		Prom:             alert.NewPrometheusRule(ctx, client),
		Common:           reconcileCommon.NewMonitorReconcileCommon(ctx, client),
	}
}

// +kubebuilder:rbac:groups=monitoring.openshift.io,resources=clusterurlmonitors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.openshift.io,resources=clusterurlmonitors/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=config.openshift.io,resources=dnses,verbs=get;list;watch
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=prometheusrules,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups=config.openshift.io,resources=clusterversions,verbs=get;list;watch

func (r *ClusterUrlMonitorReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {

	clusterUrlMonitor, res, err := r.GetClusterUrlMonitor(req)
	if err != nil {
		return utilreconcile.RequeueWith(err)
	}
	if res.ShouldStop() {
		return utilreconcile.Stop()
	}

	if r.ClusterID == "" {
		r.ClusterID = r.Common.GetClusterID()
	}

	res, err = r.EnsureMonitorAndDependenciesAbsent(clusterUrlMonitor)
	if err != nil {
		return utilreconcile.RequeueWith(err)
	}
	if res.ShouldStop() {
		return utilreconcile.Stop()
	}

	if r.Common.SetFinalizer(&clusterUrlMonitor, FinalizerKey) {
		_, err := r.Common.UpdateReconciledMonitor(&clusterUrlMonitor)
		if err != nil {
			return utilreconcile.RequeueWith(err)
		}
		return utilreconcile.Stop()
	}

	err = r.BlackBoxExporter.EnsureBlackBoxExporterResourcesExist()
	if err != nil {
		return utilreconcile.RequeueWith(err)
	}

	res, err = r.EnsureServiceMonitorExists(clusterUrlMonitor)
	if err != nil {
		return utilreconcile.RequeueWith(err)
	}
	if res.ShouldStop() {
		return utilreconcile.Stop()
	}

	res, err = r.EnsurePrometheusRuleExists(clusterUrlMonitor)
	if err != nil {
		return utilreconcile.RequeueWith(err)
	}

	return utilreconcile.Stop()
}

func (r *ClusterUrlMonitorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&monitoringv1alpha1.ClusterUrlMonitor{}).
		Complete(r)
}
