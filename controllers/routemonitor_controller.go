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
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// +kubebuilder:rbac:groups=monitoring.openshift.io,resources=routemonitors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.openshift.io,resources=routemonitors/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch

func (r *RouteMonitorReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("routeMonitor", req.NamespacedName)
	namespacedName := types.NamespacedName{Name: "blackbox-exporter", Namespace: "openshift-monitoring"}

	// Fetch all routeMonitors
	// It doesn't matter how many there are, we only ever deploy one deploymentForBlackboxExporter
	routeMonitor := &monitoringv1alpha1.RouteMonitor{}
	err := r.Get(ctx, req.NamespacedName, routeMonitor)
	if err != nil {
		log.Error(err, "Cannot get RouteMonitor", "RouteMonitor.Name", req.Name, "RouteMonitor.Namespace", req.Namespace)
	}

	// Get route to extract the URL from it, should not be required
	actualRouteInfo := types.NamespacedName{Name: routeMonitor.Spec.Route, Namespace: routeMonitor.Spec.Namespace}
	foundRoute := &routev1.Route{}
	err = r.Get(ctx, actualRouteInfo, foundRoute)
	if err != nil {
		// Is it not found?
		if errors.IsNotFound(err) {
			log.Info("Route not found", "Route.Name", actualRouteInfo.Name, "Route.Namespace", actualRouteInfo.Namespace)
		}
		log.Error(err, "Cannot get Route", "Route.Name", actualRouteInfo.Name, "Route.Namespace", actualRouteInfo.Namespace)
	}

	// Check if there's a blackbox_exporter configmap
	// If not create one
	foundBlackboxConfigMap := &corev1.ConfigMap{}
	// To make sure we only create one, we hardcode the name instead of using routeMonitor names
	err = r.Get(ctx, namespacedName, foundBlackboxConfigMap)
	if err != nil {
		// Is it not found?
		if errors.IsNotFound(err) {
			// Create the configmap
			boxConf := r.configmapForBlackboxExporter(namespacedName)
			err = r.Create(ctx, boxConf)
			if err != nil {
				log.Error(err, "Cannot create new ConfigMap", "ConfigMap.Namespace", boxConf.Namespace, "ConfigMap.Name", boxConf.Name)
				return ctrl.Result{}, err
			}
			// Seems to be all good, should requeue for next step
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
		log.Error(err, "Cannot get ConfigMap", "ConfigMap.Name", namespacedName.Name, "ConfigMap.Namespace", namespacedName.Namespace)
	}

	// Check if there's a blackbox_exporter deployment
	// If not create one
	foundBlackboxDeployment := &appsv1.Deployment{}
	// To make sure we only create one, we hardcode the name instead of using routeMonitor names
	err = r.Get(ctx, namespacedName, foundBlackboxDeployment)
	if err != nil {
		// Is it not found?
		if errors.IsNotFound(err) {
			// Create the deployment
			boxDep := r.deploymentForBlackboxExporter(namespacedName)
			err = r.Create(ctx, boxDep)
			if err != nil {
				log.Error(err, "Cannot create new Deployment", "Deployment.Namespace", boxDep.Namespace, "Deployment.Name", boxDep.Name)
				return ctrl.Result{}, err
			}
			// Seems to be all good, should requeue for next step
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
		log.Error(err, "Cannot get BlackBox Deployment", "Deployment.Name", namespacedName.Name, "Deployment.Namespace", namespacedName.Namespace)
	}

	log.Info("finished blackboxdeploy, starting servicemonitor")
	// Check if a servicemonitor for each probe exists
	// If not create them
	foundServiceMonitor := &monitoringv1.ServiceMonitor{}
	// We need the serviceMonitor to exist in `openshift-monitoring` otherwise Cluster Monitoring Operator will not pick it up
	err = r.Get(ctx, types.NamespacedName{Name: req.NamespacedName.Name, Namespace: "openshift-monitoring"}, foundServiceMonitor)
	if err != nil {
		// Is it not found?
		if errors.IsNotFound(err) {
			// Create the ServiceMonitor
			log.Info("servicemonitor not found, creating")
			serviceMonitorDep := r.deploymentForServiceMonitor(routeMonitor)
			log.Info("Creating a new ServiceMonitor", "Deployment.Namespace", serviceMonitorDep.Namespace, "Deployment.Name", serviceMonitorDep.Name)
			err = r.Create(ctx, serviceMonitorDep)
			if err != nil {
				log.Error(err, "Cannot create ServiceMonitor", "ServiceMonitor.Namespace", serviceMonitorDep.Namespace, "ServiceMonitor.Name", serviceMonitorDep.Name)
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
		log.Error(err, "Cannot get ServiceMonitor", "ServiceMonitor.Name", req.Name, "ServiceMonitor.Namespace", "openshift-monitoring")
	}
	return ctrl.Result{}, nil
}

// deploymentForBlackBoxExporter returns a blackbox deployment
func (r *RouteMonitorReconciler) deploymentForBlackboxExporter(namespacedName types.NamespacedName) *appsv1.Deployment {
	ls := labelsForRouteMonitor(namespacedName.Name)
	// hardcode the replicasize for no
	//replicas := m.Spec.Size
	var replicas int32 = 1

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespacedName.Name,
			Namespace: namespacedName.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Image: "prom/blackbox-exporter:master",
						Name:  "blackbox-exporter",
						Ports: []corev1.ContainerPort{{
							ContainerPort: 9115,
							Name:          "blackbox",
						}},
					}},
				},
			},
		},
	}
	// Set BlackboxExporter instance as the owner and controller
	// ctrl.SetControllerReference(m, dep, r.Scheme)
	return dep
}

const blackboxConfig = `modules:
  http_2xx:
    prober: http
`

func (r *RouteMonitorReconciler) configmapForBlackboxExporter(namespacedName types.NamespacedName) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespacedName.Name,
			Namespace: namespacedName.Namespace,
		},
		Data: map[string]string{"blackbox.yml": blackboxConfig},
	}
}

// deploymentForServiceMonitor returns a ServiceMonitor
func (r *RouteMonitorReconciler) deploymentForServiceMonitor(m *monitoringv1alpha1.RouteMonitor) *monitoringv1.ServiceMonitor {
	var ls = metav1.LabelSelector{}

	routeMonitorLabels := labelsForRouteMonitor(m.ObjectMeta.Name)
	r.Log.Info("after")
	err := metav1.Convert_Map_string_To_string_To_v1_LabelSelector(&routeMonitorLabels, &ls, nil)
	if err != nil {
		r.Log.Error(err, "Failed to convert LabelSelector to it's components")
	}
	// Currently we only support `http_2xx` as module
	// Still make it a variable so we can easily add functionality later
	modules := []string{"http_2xx"}

	params := map[string][]string{
		"Module": modules,
		"Target": {m.Spec.URL},
	}

	dep := &monitoringv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name: m.Name,
			// ServiceMonitors need to be in `openshift-monitoring` to be picked up by cluster-monitoring-operator
			Namespace: "openshift-monitoring",
		},
		Spec: monitoringv1.ServiceMonitorSpec{
			JobLabel: m.Name,
			Endpoints: []monitoringv1.Endpoint{
				{
					// TODO use variable from blackbox exporter deployment
					Port: "blackbox",
					// Probe every 30s
					Interval: "30s",
					// Timeout has to be smaller than probe interval
					ScrapeTimeout: "15s",
					Path:          "/probe",
					Scheme:        "http",
					// TargetPort:    intstr.FromInt(9115),
					Params: params,
					MetricRelabelConfigs: []*monitoringv1.RelabelConfig{
						//&monitoringv1.RelabelConfig{
						{
							Replacement:  m.Spec.URL,
							SourceLabels: []string{},
							TargetLabel:  "RouteMonitorUrl",
						},
					},
				}},
			Selector:          ls,
			NamespaceSelector: monitoringv1.NamespaceSelector{},
		},
	}
	ctrl.SetControllerReference(m, dep, r.Scheme)
	return dep
}

// getPodNames returns the pod names of the array of pods passed in
func getPodNames(pods []corev1.Pod) []string {
	var podNames []string
	for _, pod := range pods {
		podNames = append(podNames, pod.Name)
	}
	return podNames
}

// Make this a function in case of hardcoding, in case we need more labels in the future
func labelsForRouteMonitor(name string) map[string]string {
	return map[string]string{"app": "blackbox-exporter"}
}

func (r *RouteMonitorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&monitoringv1alpha1.RouteMonitor{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.ConfigMap{}).
		Complete(r)
}
