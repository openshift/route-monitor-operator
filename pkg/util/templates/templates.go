package templates

import (
	"fmt"

	"github.com/openshift/route-monitor-operator/pkg/consts/blackbox"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// TemplateForServiceMonitorName return the generated name from the RouteMonitor.
// The name is joined by the name and the namespace to create a unique ServiceMonitor for each RouteMonitor
func TemplateForServiceMonitorName(namespace string, name string) types.NamespacedName {
	serviceMonitorName := fmt.Sprintf("%s-%s", name, namespace)
	return types.NamespacedName{Name: serviceMonitorName, Namespace: blackbox.BlackBoxNamespace}
}

// TemplateForServiceMonitorResource returns a ServiceMonitor
func TemplateForServiceMonitorResource(url, name string) monitoringv1.ServiceMonitor {

	routeURL := url
	serviceMonitorName := name

	routeMonitorLabels := blackbox.GenerateBlackBoxLables()

	labelSelector := metav1.LabelSelector{MatchLabels: routeMonitorLabels}

	// Currently we only support `http_2xx` as module
	// Still make it a variable so we can easily add functionality later
	modules := []string{"http_2xx"}

	params := map[string][]string{
		"module": modules,
		"target": {routeURL},
	}

	serviceMonitor := monitoringv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name: serviceMonitorName,
			// ServiceMonitors need to be in `openshift-monitoring` to be picked up by cluster-monitoring-operator
			Namespace: blackbox.BlackBoxNamespace,
		},
		Spec: monitoringv1.ServiceMonitorSpec{
			JobLabel: serviceMonitorName,
			Endpoints: []monitoringv1.Endpoint{
				{
					Port: blackbox.BlackBoxPortName,
					// Probe every 30s
					Interval: "30s",
					// Timeout has to be smaller than probe interval
					ScrapeTimeout: "15s",
					Path:          "/probe",
					Scheme:        "http",
					Params:        params,
					MetricRelabelConfigs: []*monitoringv1.RelabelConfig{
						{
							Replacement: routeURL,
							TargetLabel: "RouteMonitorUrl",
						},
					},
				}},
			Selector:          labelSelector,
			NamespaceSelector: monitoringv1.NamespaceSelector{},
		},
	}
	return serviceMonitor
}
