package controllers

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	//"k8s.io/apimachinery/pkg/util/intstr"
	//	ctrl "sigs.k8s.io/controller-runtime"

	monitoringv1alpha1 "github.com/openshift/route-monitor-operator/api/v1alpha1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	blackBoxNamespace = "openshift-monitoring"
	blackBoxName      = "blackbox-exporter"
)

// deploymentForBlackBoxExporter returns a blackbox deployment
func (r *RouteMonitorReconciler) deploymentForBlackboxExporter() *appsv1.Deployment {
	ls := labelsForRouteMonitor()
	// hardcode the replicasize for no
	//replicas := m.Spec.Size
	var replicas int32 = 1

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      blackBoxName,
			Namespace: blackBoxNamespace,
			Labels:    ls,
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

// serviceForBlackBoxExporter returns a blackbox service
func (r *RouteMonitorReconciler) serviceForBlackboxExporter() *corev1.Service {
	ls := labelsForRouteMonitor()

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      blackBoxName,
			Namespace: blackBoxNamespace,
			Labels:    ls,
		},
		Spec: corev1.ServiceSpec{
			Selector: ls,
			Ports: []corev1.ServicePort{{
				TargetPort: intstr.FromString("blackbox"),
				Port:       9115,
				Name:       "blackbox",
			}},
		},
	}
	// Set BlackboxExporter instance as the owner and controller
	// ctrl.SetControllerReference(m, dep, r.Scheme)
	return svc
}

// deploymentForServiceMonitor returns a ServiceMonitor
func (r *RouteMonitorReconciler) deploymentForServiceMonitor(m *monitoringv1alpha1.RouteMonitor) *monitoringv1.ServiceMonitor {
	var ls = metav1.LabelSelector{}

	routeMonitorLabels := labelsForRouteMonitor()
	err := metav1.Convert_Map_string_To_string_To_v1_LabelSelector(&routeMonitorLabels, &ls, nil)
	if err != nil {
		r.Log.Error(err, "Failed to convert LabelSelector to it's components")
	}
	// Currently we only support `http_2xx` as module
	// Still make it a variable so we can easily add functionality later
	modules := []string{"http_2xx"}

	params := map[string][]string{
		"module": modules,
		"target": {m.Status.RouteURL},
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
					HonorLabels: true,
				},
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
							Replacement: m.Status.RouteURL,
							TargetLabel: "RouteMonitorUrl",
						},
					},
				}},
			Selector:          ls,
			NamespaceSelector: monitoringv1.NamespaceSelector{},
		},
	}
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
func labelsForRouteMonitor() map[string]string {
	return map[string]string{"app": blackBoxName}
}
