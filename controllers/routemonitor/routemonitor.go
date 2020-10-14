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
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	monitoringv1alpha1 "github.com/openshift/route-monitor-operator/api/v1alpha1"
	utilreconcile "github.com/openshift/route-monitor-operator/pkg/util/reconcile"
)

// RouteMonitorReconciler reconciles a RouteMonitor object
type RouteMonitorReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
	RouteMonitorSupplement
	RouteMonitorAdder
	RouteMonitorDeleter
}

// +kubebuilder:rbac:groups=*,resources=services,verbs=get;list;watch;create
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups=monitoring.openshift.io,resources=routemonitors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.openshift.io,resources=routemonitors/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch

func (r *RouteMonitorReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithName("Reconcile")

	log.V(2).Info("Entering GetRouteMonitor")
	routeMonitor, res, err := r.GetRouteMonitor(ctx, req)
	if err != nil {
		return utilreconcile.RequeueWith(err)
	}

	if res.ShouldStop() {
		return utilreconcile.Stop()
	}

	// Handle deletion of RouteMonitor Resource
	shouldDelete := routeMonitor.WasDeleteRequested()
	log.V(2).Info("Tested WasDeleteRequested", "shouldDelete", shouldDelete)

	if shouldDelete {
		res, err := r.EnsureRouteMonitorAndDependenciesAbsent(ctx, routeMonitor)
		if err != nil {
			return utilreconcile.RequeueWith(err)
		}

		if res.ShouldStop() {
			return utilreconcile.Stop()
		}
		return utilreconcile.Stop()
	}

	log.V(2).Info("Entering CreateBlackBoxExporterResources")
	// Should happen once but cannot input in main.go
	err = r.EnsureBlackBoxExporterResourcesExists(ctx)
	if err != nil {
		return utilreconcile.RequeueWith(err)
	}

	log.V(2).Info("Entering GetRoute")
	route, err := r.GetRoute(ctx, routeMonitor)
	if err != nil {
		return utilreconcile.RequeueWith(err)
	}

	log.V(2).Info("Entering UpdateRouteURL")
	res, err = r.EnsureRouteURLExists(ctx, route, routeMonitor)
	if err != nil {
		return utilreconcile.RequeueWith(err)
	}
	if res.ShouldStop() {
		return utilreconcile.Stop()
	}

	log.V(2).Info("Entering CreateServiceMonitorResource")
	res, err = r.EnsureServiceMonitorResourceExists(ctx, routeMonitor)
	if err != nil {
		return utilreconcile.RequeueWith(err)
	}
	if res.ShouldStop() {
		return utilreconcile.Stop()
	}
	return utilreconcile.Stop()
}

func (r *RouteMonitorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&monitoringv1alpha1.RouteMonitor{}).
		Complete(r)
}
