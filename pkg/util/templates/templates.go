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

// TemplateForPrometheusRuleResource returns a PrometheusRule
func TemplateForPrometheusRuleResource(url, percent string, namespacedName types.NamespacedName) monitoringv1.PrometheusRule {

	routeURL := url
	routeURLLabel := fmt.Sprintf(`RouteMonitorUrl="%s"`, routeURL)
	rules := []monitoringv1.Rule{}

	for _, alertStruct := range []struct {
		duration  string
		severity  string
		timeShort string
		timeLong  string
	}{
		{
			duration:  "2m",
			severity:  "critical",
			timeLong:  "1h",
			timeShort: "5m",
		},
		{
			duration:  "15m",
			severity:  "critical",
			timeLong:  "6h",
			timeShort: "30m",
		},
		{
			duration:  "1h",
			severity:  "warning",
			timeLong:  "1d",
			timeShort: "2h",
		},
		{
			duration:  "3h",
			severity:  "warning",
			timeLong:  "3d",
			timeShort: "6h",
		},
	} {
		// Create all the alerts
		// Sample resource
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

		alertTemplate := strings.Join([]string{
			`1-rate(probe_success{%[1]s}[%[3]s]) > (14.40 * (1-%[2]s))`,
			`and`,
			`1-rate(probe_success{%[1]s}[%[4]s]) > (14.40 * (1-%[2]s))`}, "\n")
		rules = append(rules, monitoringv1.Rule{
			Alert: "ErrorBudgetBurn",
			Expr: intstr.FromString(fmt.Sprintf(alertTemplate,
				routeURLLabel,
				percent,
				alertStruct.timeShort,
				alertStruct.timeLong)),
			Labels: sampleTemplateLabelsWithSev(routeURL, alertStruct.severity),
			Annotations: map[string]string{
				"message": fmt.Sprintf("High error budget burn for %s (current value: {{ $value }})", routeURLLabel),
			},
			For: alertStruct.duration,
		})
	}

	resource := monitoringv1.PrometheusRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespacedName.Name,
			Namespace: namespacedName.Namespace,
		},
		Spec: monitoringv1.PrometheusRuleSpec{
			Groups: []monitoringv1.RuleGroup{
				{
					Name:  "SLOs-http_requests_total",
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
