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

package routemonitor

import (
	"context"

	"github.com/go-logr/logr"
	monitoringv1alpha1 "github.com/openshift/route-monitor-operator/api/v1alpha1"
	"github.com/openshift/route-monitor-operator/controllers"
	"github.com/openshift/route-monitor-operator/pkg/alert"
	"github.com/openshift/route-monitor-operator/pkg/blackboxexporter"
	reconcileCommon "github.com/openshift/route-monitor-operator/pkg/reconcile"
	"github.com/openshift/route-monitor-operator/pkg/servicemonitor"
	"github.com/openshift/route-monitor-operator/pkg/util/finalizer"
	utilreconcile "github.com/openshift/route-monitor-operator/pkg/util/reconcile"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// RouteMonitorReconciler reconciles a RouteMonitor object
type RouteMonitorReconciler struct {
	Client           client.Client
	Ctx              context.Context
	Log              logr.Logger
	Scheme           *runtime.Scheme
	BlackBoxExporter controllers.BlackBoxExporterHandler
	ServiceMonitor   controllers.ServiceMonitorHandler
	Prom             controllers.PrometheusRuleHandler
	Common           controllers.MonitorResourceHandler
}

func NewReconciler(mgr manager.Manager, blackboxExporterImage, blackboxExporterNamespace string, enablehypershift bool) *RouteMonitorReconciler {
	log := ctrl.Log.WithName("controllers").WithName("RouteMonitor")
	client := mgr.GetClient()
	ctx := context.Background()
	return &RouteMonitorReconciler{
		Client:           client,
		Ctx:              ctx,
		Log:              log,
		Scheme:           mgr.GetScheme(),
		BlackBoxExporter: blackboxexporter.New(client, log, ctx, blackboxExporterImage, blackboxExporterNamespace),
		ServiceMonitor:   servicemonitor.NewServiceMonitor(client),
		Prom:             alert.NewPrometheusRule(client),
		Common:           reconcileCommon.NewMonitorResourceCommon(ctx, client),
	}
}

// +kubebuilder:rbac:groups=*,resources=services,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;delete;update
// +kubebuilder:rbac:groups=monitoring.rhobs,resources=servicemonitors,verbs=get;list;watch;create;delete;update
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=prometheusrules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.openshift.io,resources=routemonitors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.openshift.io,resources=routemonitors/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch
// +kubebuilder:rbac:groups=config.openshift.io,resources=clusterversions,verbs=get;list;watch
// +kubebuilder:rbac:groups=config.openshift.io,resources=infrastructures,verbs=get;list;watch

func (r *RouteMonitorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Ctx = ctx
	log := r.Log.WithName("Reconcile").WithValues("name", req.Name, "namespace", req.Namespace)

	log.V(2).Info("Entering GetRouteMonitor")
	routeMonitor, res, err := r.GetRouteMonitor(req)
	if err != nil {
		log.Error(err, "Failed to retreive RouteMonitor. Requeueing...")
		return utilreconcile.RequeueWith(err)
	}
	if res.ShouldStop() {
		return utilreconcile.Stop()
	}

	// Handle deletion of RouteMonitor Resource
	shouldDelete := finalizer.WasDeleteRequested(&routeMonitor)
	log.V(2).Info("Response of WasDeleteRequested", "shouldDelete", shouldDelete)

	if shouldDelete {
		_, err := r.EnsureMonitorAndDependenciesAbsent(ctx, routeMonitor)
		if err != nil {
			log.Error(err, "Failed to delete RouteMonitor. Requeueing...")
			return utilreconcile.RequeueWith(err)
		}
		log.Info("Successfully deleted RouteMonitor. Finished reconcile.")
		return utilreconcile.Stop()
	}

	log.V(2).Info("Entering EnsureFinalizerSet")
	res, err = r.EnsureFinalizerSet(routeMonitor)
	if err != nil {
		log.Error(err, "Failed to set RouteMonitor's finalizer. Requeueing...")
		return utilreconcile.RequeueWith(err)
	}
	if res.ShouldStop() {
		log.Info("Successfully set RouteMonitor finalizers. Stopping...")
		return utilreconcile.Stop()
	}

	log.V(2).Info("Entering EnsureBlackBoxExporterResourcesExist")
	// Should happen once but cannot input in main.go
	err = r.BlackBoxExporter.EnsureBlackBoxExporterResourcesExist()
	if err != nil {
		log.Error(err, "Failed to create BlackBoxExporter. Requeueing...")
		return utilreconcile.RequeueWith(err)
	}

	log.V(2).Info("Entering GetRoute")
	route, err := r.GetRoute(routeMonitor)
	if err != nil {
		log.Error(err, "Failed to get Route. Requeueing...")
		return utilreconcile.RequeueWith(err)
	}

	log.V(2).Info("Entering EnsureRouteURLExists")
	res, err = r.EnsureRouteURLExists(route, routeMonitor)
	if err != nil {
		log.Error(err, "Failed to get RouteURL for RouteMonitor. Requeueing...")
		return utilreconcile.RequeueWith(err)
	}
	if res.ShouldStop() {
		log.Info("Successfully patched RouteMonitor with RouteURL. Stopping...")
		return utilreconcile.Stop()
	}

	log.V(2).Info("Entering EnsureServiceMonitorExists")
	res, err = r.EnsureServiceMonitorExists(ctx, routeMonitor)
	if err != nil {
		log.Error(err, "Failed to set ServiceMonitor. Requeueing...")
		return utilreconcile.RequeueWith(err)
	}
	if res.ShouldStop() {
		log.Info("Successfully patched RouteMonitor with ServiceMonitorRef. Stopping...")
		return utilreconcile.Stop()
	}

	log.V(2).Info("Entering EnsurePrometheusRuleResourceExists")
	// result is silenced as it's the end of the function, if this moves add it back
	res, err = r.EnsurePrometheusRuleExists(ctx, routeMonitor)
	if err != nil {
		log.Error(err, "Failed to set PrometheusRule. Requeueing...")
		return utilreconcile.RequeueWith(err)
	}
	if res.ShouldStop() {
		log.Info("Successfully patched RouteMonitor with PrometheusRuleRef. Stopping...")
		return utilreconcile.Stop()
	}

	log.Info("All operations for RouteMonitor completed. Finished Reconcile.")
	return utilreconcile.Stop()
}

func (r *RouteMonitorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&monitoringv1alpha1.RouteMonitor{}).
		Complete(r)
}
