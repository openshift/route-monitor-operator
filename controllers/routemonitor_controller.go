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

package controllers

import (
	"context"
	goerrors "errors"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	//"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	routev1 "github.com/openshift/api/route/v1"
	monitoringv1alpha1 "github.com/openshift/route-monitor-operator/api/v1alpha1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
)

// RouteMonitorReconciler reconciles a RouteMonitor object
type RouteMonitorReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=*,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=monitoring.openshift.io,resources=routemonitors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.openshift.io,resources=routemonitors/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch

func (r *RouteMonitorReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("routeMonitor", req.NamespacedName)

	// Fetch all routeMonitors
	// It doesn't matter how many there are, we only ever deploy one deploymentForBlackboxExporter
	routeMonitor := &monitoringv1alpha1.RouteMonitor{}
	err := r.Get(ctx, req.NamespacedName, routeMonitor)
	if err != nil {
		log.Error(err, "Cannot get RouteMonitor", "Name", req.Name, "Namespace", req.Namespace)
	}

	// Check if there's a blackbox_exporter deployment
	// If not create one
	foundBlackboxDeployment := &appsv1.Deployment{}
	blackBoxNamespacedName := types.NamespacedName{Name: blackBoxName, Namespace: blackBoxNamespace}
	// To make sure we only create one, we hardcode the name instead of using routeMonitor names
	err = r.Get(ctx, blackBoxNamespacedName, foundBlackboxDeployment)
	if err != nil {
		// Is it not found?
		if errors.IsNotFound(err) {
			// Create the deployment
			boxDep := r.deploymentForBlackboxExporter()
			err = r.Create(ctx, boxDep)
			if err != nil {
				log.Error(err, "Cannot create new Deployment", "Namespace", boxDep.Namespace, "Name", boxDep.Name)
				return ctrl.Result{}, err
			}
			// Seems to be all good, should requeue for next step
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
		log.Error(err, "Cannot get BlackBox Deployment", "Name", blackBoxNamespacedName.Name, "Namespace", blackBoxNamespacedName.Namespace)
	}
	log.V(1).Info("Blackbox Exporter Deployment Exists")
	// Check if there's a blackbox_exporter Service
	// If not create one
	foundBlackboxService := &corev1.Service{}
	// To make sure we only create one, we hardcode the name instead of using routeMonitor names
	err = r.Get(ctx, blackBoxNamespacedName, foundBlackboxService)
	if err != nil {
		// Is it not found?
		if errors.IsNotFound(err) {
			// Create the deployment
			boxSvc := r.serviceForBlackboxExporter()
			err = r.Create(ctx, boxSvc)
			if err != nil {
				log.Error(err, "Cannot create new Service", "Namespace", boxSvc.Namespace, "Name", boxSvc.Name)
				return ctrl.Result{}, err
			}
			// Seems to be all good, should requeue for next step
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
		log.Error(err, "Cannot get BlackBox Service", "Name", blackBoxNamespacedName.Name, "Namespace", blackBoxNamespacedName.Namespace)
	}
	log.V(1).Info("Blackbox Exporter Service Exists")

	// Get route to extract the URL from it, should not be required
	actualRouteInfo := types.NamespacedName{Name: routeMonitor.Spec.Route.Name, Namespace: routeMonitor.Spec.Route.Namespace}
	foundRoute := &routev1.Route{}
	err = r.Get(ctx, actualRouteInfo, foundRoute)
	if err != nil {
		// Is it not found?
		if errors.IsNotFound(err) {
			log.Info("Route not found", "Name", actualRouteInfo.Name, "Namespace", actualRouteInfo.Namespace)
		}
		log.Error(err, "Cannot get Route", "Name", actualRouteInfo.Name, "Namespace", actualRouteInfo.Namespace)
	}
	log.V(1).Info("Route Exists")
	if len(foundRoute.Status.Ingress) == 0 {
		err = goerrors.New("There are no Ingresses, cannot extract Route host")
		log.Error(err, "Cannot Extract host")
	}
	foundRouteURL := foundRoute.Status.Ingress[0].Host
	if len(foundRoute.Status.Ingress) > 1 {
		log.Info("Too many ingresses", "Assuming first one is correct one",
			"IngressAmount", len(foundRoute.Status.Ingress),
			"FirstIngressHost", foundRouteURL)
	}
	routeMonitorURL := routeMonitor.Status.RouteURL
	if foundRouteURL != routeMonitorURL {
		if routeMonitorURL != "" {
			log.Info("RouteURL mismatch", "Mismatch between foundRouteURL and RouteMonitorURL, taking foundRouteURL as source of truth",
				"foundRouteURL", foundRouteURL,
				"RouteMonitorURL", routeMonitorURL)
		}
		routeMonitor.Status.RouteURL = foundRouteURL
		log.V(1).Info("Updating RouteMonitorURL", "foundRouteURL", foundRouteURL)
		err = r.Status().Update(context.TODO(), routeMonitor)
		if err != nil {
			log.Error(err, "Cannot Update RouteMonitor", "foundRouteURL", foundRouteURL)
		}
		return ctrl.Result{RequeueAfter: 5 * time.Second}, err
	}
	log.V(1).Info("RouteMonitorURL Exists")

	// Check if a servicemonitor for each probe exists
	// If not create them
	foundServiceMonitor := &monitoringv1.ServiceMonitor{}
	// We need the serviceMonitor to exist in `openshift-monitoring` otherwise Cluster Monitoring Operator will not pick it up
	err = r.Get(ctx, req.NamespacedName, foundServiceMonitor)
	if err != nil {
		// Is it not found?
		if errors.IsNotFound(err) {
			// Create the ServiceMonitor
			log.Info("servicemonitor not found, creating")
			serviceMonitorDep := r.deploymentForServiceMonitor(routeMonitor)
			log.Info("Creating a new ServiceMonitor", "Name", serviceMonitorDep.Name, "Namespace", serviceMonitorDep.Namespace)
			err = r.Create(ctx, serviceMonitorDep)
			if err != nil {
				log.Error(err, "Cannot create ServiceMonitor", "Name", serviceMonitorDep.Name, "Namespace", serviceMonitorDep.Namespace)
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
		log.Error(err, "Cannot get ServiceMonitor", "Name", req.Name, "Namespace", req.Namespace)
	}
	log.V(1).Info("ServiceMonitor Exists")

	return ctrl.Result{}, nil
}

func (r *RouteMonitorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&monitoringv1alpha1.RouteMonitor{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
