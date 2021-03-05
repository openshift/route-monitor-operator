package templates

import (
	"fmt"
	"strings"

	"github.com/openshift/route-monitor-operator/pkg/consts/blackboxexporter"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// TemplateForServiceMonitorResource returns a ServiceMonitor
func TemplateForServiceMonitorResource(url, blackBoxExporterNamespace string, namespacedName types.NamespacedName) monitoringv1.ServiceMonitor {

	routeURL := url

	routeMonitorLabels := blackboxexporter.GenerateBlackBoxExporterLables()

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
			Name:      namespacedName.Name,
			Namespace: namespacedName.Namespace,
		},
		Spec: monitoringv1.ServiceMonitorSpec{
			Endpoints: []monitoringv1.Endpoint{
				{
					Port: blackboxexporter.BlackBoxExporterPortName,
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
			Selector: labelSelector,
			NamespaceSelector: monitoringv1.NamespaceSelector{
				MatchNames: []string{
					blackBoxExporterNamespace,
				},
			},
		},
	}
	return serviceMonitor
}

type multiWindowMultiBurnAlertRule struct {
	duration    string
	severity    string
	longWindow  string
	shortWindow string
}

//render creates a monitoring rule for the defined multiwindow multi-burn rate alert
// Sample result as yaml
/*
	 - alert: ErrorBudgetBurn
		 annotations:
			 message: 'High error budget burn for targeturl=getmeright (current value: {{ $value }})'
		 expr: |
			 1-rate(probe_success{targeturl="getmeright"}[5m]) > (14.40 * (1-0.95000))
			 and
			 1-rate(probe_success{targeturl="getmeright"}[1h]) > (14.40 * (1-0.95000))
		 for: 2m
		 labels:
			 severity: critical
			 targeturl: getmeright
*/
func (r *multiWindowMultiBurnAlertRule) render(url, percent, label, alertName string) monitoringv1.Rule {
	alertTemplate := strings.Join([]string{
		`1-rate(probe_success{%[1]s}[%[3]s]) > (14.40 * (1-%[2]s))`,
		`and`,
		`1-rate(probe_success{%[1]s}[%[4]s]) > (14.40 * (1-%[2]s))`}, "\n")
	return monitoringv1.Rule{
		Alert: alertName,
		Expr: intstr.FromString(fmt.Sprintf(alertTemplate,
			label,
			percent,
			r.shortWindow,
			r.longWindow)),
		Labels: sampleTemplateLabelsWithSev(url, r.severity),
		Annotations: map[string]string{
			"message": fmt.Sprintf("High error budget burn for %s (current value: {{ $value }})", label),
		},
		For: r.duration,
	}
}

// TemplateForPrometheusRuleResource returns a PrometheusRule
func TemplateForPrometheusRuleResource(url, percent string, namespacedName types.NamespacedName) monitoringv1.PrometheusRule {

	routeURLLabel := fmt.Sprintf(`RouteMonitorUrl="%s"`, url)
	rules := []monitoringv1.Rule{}
	alertRules := []multiWindowMultiBurnAlertRule{
		{
			duration:    "2m",
			severity:    "critical",
			longWindow:  "1h",
			shortWindow: "5m",
		},
		{
			duration:    "15m",
			severity:    "critical",
			longWindow:  "6h",
			shortWindow: "30m",
		},
		{
			duration:    "1h",
			severity:    "warning",
			longWindow:  "1d",
			shortWindow: "2h",
		},
		{
			duration:    "3h",
			severity:    "warning",
			longWindow:  "3d",
			shortWindow: "6h",
		},
	}

	for _, alertrule := range alertRules { // Create all the alerts
		rules = append(rules, alertrule.render(url, percent, routeURLLabel, namespacedName.Name+"-ErrorBudgetBurn"))
	}

	resource := monitoringv1.PrometheusRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespacedName.Name,
			Namespace: namespacedName.Namespace,
		},
		Spec: monitoringv1.PrometheusRuleSpec{
			Groups: []monitoringv1.RuleGroup{
				{
					Name:  "SLOs-probe",
					Rules: rules,
				},
			},
		},
	}
	return resource
}
func sampleTemplateLabelsWithSev(url, severity string) map[string]string {
	return map[string]string{
		"severity":        severity,
		"RouteMonitorUrl": url,
	}
}
func sampleTemplateLabels(url string) map[string]string {
	return map[string]string{
		"RouteMonitorUrl": url,
	}
}
