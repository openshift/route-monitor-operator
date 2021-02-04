package templates

import (
	"fmt"
	"strings"

	"github.com/openshift/route-monitor-operator/pkg/consts/blackbox"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// TemplateForServiceMonitorResource returns a ServiceMonitor
func TemplateForServiceMonitorResource(url string, namespacedName types.NamespacedName) monitoringv1.ServiceMonitor {

	routeURL := url

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
			Name:      namespacedName.Name,
			Namespace: namespacedName.Namespace,
		},
		Spec: monitoringv1.ServiceMonitorSpec{
			JobLabel: namespacedName.Name,
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
		       sum(http_requests_total:burnrate5m{targeturl="getmeright"}) > (14.40 * (1-0.95000))
		       and
		       sum(http_requests_total:burnrate1h{targeturl="getmeright"}) > (14.40 * (1-0.95000))
		     for: 2m
		     labels:
		       severity: critical
		       targeturl: getmeright
		*/

		alertTemplate := strings.Join([]string{
			`sum(http_requests_total:burnrate%[3]s{%[1]s}) > (14.40 * (1-%[2]s))`,
			`and`,
			`sum(http_requests_total:burnrate%[4]s{%[1]s}) > (14.40 * (1-%[2]s))`}, "\n")
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

	// Create all the recording rules
	// Sample resource
	/*
	   - expr: |
	       sum(rate(http_requests_total{targeturl="getmeright",code=~"5.."}[6h]))
	       /
	       sum(rate(http_requests_total{targeturl="getmeright"}[6h]))
	     labels:
	       targeturl: getmeright
	     record: http_requests_total:burnrate6h
	*/
	recordingRuleTemplate := strings.Join([]string{
		`sum(rate(http_requests_total{%[1]s,code=~"5.."}[%[2]s]))`,
		`/`,
		`sum(rate(http_requests_total{%[1]s}[%[2]s]))`}, "\n")
	for _, timePeriod := range []string{"5m", "30m", "1h", "2h", "6h", "1d", "3d"} {
		rules = append(rules, monitoringv1.Rule{
			Expr:   intstr.FromString(fmt.Sprintf(recordingRuleTemplate, routeURLLabel, timePeriod)),
			Labels: sampleTemplateLabels(routeURL),
			Record: fmt.Sprintf("http_requests_total:burnrate%s", timePeriod),
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
