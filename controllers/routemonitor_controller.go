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

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	monitoringv1alpha1 "github.com/RiRa12621/openshift-route-monitor-operator/api/v1alpha1"
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
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch

func (r *RouteMonitorReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("routeMonitor", req.NamespacedName)

	// Fetch all routeMonitors
	// It doesn't matter how many there are, we only ever deploy one deploymentForBlackboxExporter
	routeMonitor := &monitoringv1alpha1.RouteMonitor{}
	err := r.Get(ctx, req.NamespacedName, routeMonitor)
	if err != nil {
		log.Error(err, "Failed to get routeMonitor")
	}
	// Check if there's a blackbox_exporter deployment
	// If not create one
	foundBlackboxDeployment := &appsv1.Deployment{}
	// To make sure we only create one, we hardcode the name instead of using routeMonitor names
	err = r.Get(ctx, types.NamespacedName{Name: "blackbox-exporter", Namespace: "openshift-monitoring"}, foundBlackboxDeployment)
	if err != nil {
		// Is it not found?
		if errors.IsNotFound(err) {
			// Create the deployment
			boxDep := r.deploymentForBlackboxExporter(routeMonitor)
			log.Info("Creating a new Deployment", "Deployment.Namespace", boxDep.Namespace, "Deployment.Name", boxDep.Name)
			err = r.Create(ctx, boxDep)
			if err != nil {
				log.Error(err, "Failed to create new Deployment", "Deployment.Namespace", boxDep.Namespace, "Deployment.Name", boxDep.Name)
				return ctrl.Result{}, err
			}
			// Seems to be all good, if not return an err at least
			return ctrl.Result{}, err
		}
	}

	// Check if a servicemonitor for each probe exists
	// If not create them
	foundServiceMonitor := &monitoringv1.ServiceMonitor{}
	// We need the serviceMonitor to exist in `openshift-monitoring` otherwise Cluster Monitoring Operator will not pick it up
	err = r.Get(ctx, types.NamespacedName{Name: routeMonitor.Name, Namespace: "openshift-monitoring"}, foundServiceMonitor)
	if err != nil {
		// Is it not found?
		if errors.IsNotFound(err) {
			// Create the ServiceMonitor
			serviceMonitorDep := r.deploymentForServiceMonitor(routeMonitor)
			log.Info("Creating a new ServiceMonitor", "Deployment.Namespace", serviceMonitorDep.Namespace, "Deployment.Name", serviceMonitorDep.Name)
			err = r.Create(ctx, serviceMonitorDep)
			if err != nil {
				log.Error(err, "Failed to create new ServiceMonitor", "Deployment.Namespace", serviceMonitorDep.Namespace, "Deployment.Name", serviceMonitorDep.Name)
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

// deploymentForBlackBoxExporter returns a blackbox deployment
func (r *RouteMonitorReconciler) deploymentForBlackboxExporter(m *monitoringv1alpha1.RouteMonitor) *appsv1.Deployment {
	ls := labelsForRouteMonitor(m.Name)
	// hardcode the replicasize for no
	//replicas := m.Spec.Size
	var replicas int32 = 1

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.Name,
			Namespace: m.Namespace,
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
						Image:   "prom/blackbox-exporter:master",
						Name:    "blackbox-exporter",
						Command: []string{"--config.file=/config/blackbox.yml"},
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
	ctrl.SetControllerReference(m, dep, r.Scheme)
	return dep
}

// deploymentForServiceMonitor returns a ServiceMonitor
func (r *RouteMonitorReconciler) deploymentForServiceMonitor(m *monitoringv1alpha1.RouteMonitor) *monitoringv1.ServiceMonitor {
	ls := labelsForRouteMonitor(m.Name)
	// Currently we only support `http_2xx` as module
	// Still make it a variable so we can easily add functionality later
	modules := []string{"http_2xx"}

	params := monitoringv1.Params{
		Module: []string{modules},
		Target: []string{m.Url},
	}

	endpoint := monitoringv1.Endpoint{
		// TODO use variable from blackbox exporter deployment
		Port: "blackbox",
		// Probe every 30s
		Interval: "30s",
		// Timeout has to be smaller than probe interval
		ScrapeTimeout: "15s",
		Path:          "/probe",
		Scheme:        "http",
		TargetPort:    "9115",
		Params:        []monitoringv1Params{params},
		MetricRelabelings: monitoringv1.MetricRelabelConfigs{
			Replacement:  m.url,
			SourceLabels: "[]",
			TargetLabel:  "RouteMonitorUrl",
		},
	}

	dep := &monitoringv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name: m.Name,
			// ServiceMonitors need to be in `openshift-monitoring` to be picked up by cluster-monitoring-operator
			Namespace: "openshift-monitoring",
		},
		TypesMeta: metav1.TypeMeta{
			Kind:       monitoringv1.ServiceMonitorsKind,
			APIVersion: monitoringv1.SchemeGroupVersion.String(),
		},
		Spec: monitoringv1.ServiceMonitorSpec{
			JobLabel:  m.Name,
			Endpoints: []monitoringv1.Endpoint{endpoint},
			Selector:  ls,
			NameSpaceSelector: monitoringv1.NamespaceSelector{
				MatchNames: []string{m.Namespace},
			},
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
	return map[string]string{"app": "RouteMonitoringOperator"}
}

func (r *RouteMonitorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&monitoringv1alpha1.RouteMonitor{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}
