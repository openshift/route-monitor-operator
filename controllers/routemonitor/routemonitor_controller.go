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
)

// RouteMonitorReconciler reconciles a RouteMonitor object
type RouteMonitorReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=*,resources=services,verbs=get;list;watch;create;
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;
// +kubebuilder:rbac:groups=monitoring.openshift.io,resources=routemonitors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.openshift.io,resources=routemonitors/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch

func (r *RouteMonitorReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()

	routeMonitor, err := r.GetRouteMonitor(ctx, req)
	if err != nil {
		return ctrl.Result{Requeue: true}, err
	}

	// Handle deletion of RouteMonitor Resource
	shouldDelete := r.WasDeleteRequested(routeMonitor)

	if shouldDelete {
		shouldDeleteBlackBoxResources, err := r.ShouldDeleteBlackBoxExporterResources(ctx, routeMonitor)
		if err != nil {
			return ctrl.Result{Requeue: true}, err
		}

		// if this is the last resource then delete the blackbox-exporter resources and then delete the RouteMonitor
		if shouldDeleteBlackBoxResources {
			err := r.DeleteBlackBoxExporterResources(ctx)
			if err != nil {
				return ctrl.Result{Requeue: true}, err
			}
		}

		res, err := r.DeleteRouteMonitorAndDependencies(ctx, routeMonitor)
		if err != nil {
			return ctrl.Result{Requeue: true}, err
		}
		if res != nil {
			return *res, nil
		}

		// Break Reconcile early and do not requeue as this was a deletion request
		return ctrl.Result{Requeue: false}, nil
	}

	// Should happen once but cannot input in main.go
	err = r.CreateBlackBoxExporterResources(ctx)
	if err != nil {
		return ctrl.Result{Requeue: true}, err
	}

	route, err := r.GetRoute(ctx, routeMonitor)
	if err != nil {
		return ctrl.Result{Requeue: true}, err
	}

	res, err := r.UpdateRouteURL(ctx, route, routeMonitor)
	if err != nil {
		return ctrl.Result{Requeue: true}, err
	}
	if res != nil {
		return *res, nil
	}

	res, err = r.CreateServiceMonitorResource(ctx, routeMonitor)
	if err != nil {
		return ctrl.Result{Requeue: true}, err
	}
	if res != nil {
		return *res, nil
	}

	return ctrl.Result{Requeue: false}, nil
}

func (r *RouteMonitorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&monitoringv1alpha1.RouteMonitor{}).
		Complete(r)
}
