package templates

import (
	"github.com/openshift/route-monitor-operator/pkg/consts/blackbox"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"gopkg.in/inf.v0"
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
func TemplateForPrometheusRuleResource(url, name string, percent inf.Dec) monitoringv1.PrometheusRule {

	/*
	   groups:
	   - name: SLOs-http_requests_total
	     rules:
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
	     - alert: ErrorBudgetBurn
	       annotations:
	         message: 'High error budget burn for targeturl=getmeright (current value: {{ $value }})'
	       expr: |
	         sum(http_requests_total:burnrate30m{targeturl="getmeright"}) > (6.00 * (1-0.95000))
	         and
	         sum(http_requests_total:burnrate6h{targeturl="getmeright"}) > (6.00 * (1-0.95000))
	       for: 15m
	       labels:
	         severity: critical
	         targeturl: getmeright
	     - alert: ErrorBudgetBurn
	       annotations:
	         message: 'High error budget burn for targeturl=getmeright (current value: {{ $value }})'
	       expr: |
	         sum(http_requests_total:burnrate2h{targeturl="getmeright"}) > (3.00 * (1-0.95000))
	         and
	         sum(http_requests_total:burnrate1d{targeturl="getmeright"}) > (3.00 * (1-0.95000))
	       for: 1h
	       labels:
	         severity: warning
	         targeturl: getmeright
	     - alert: ErrorBudgetBurn
	       annotations:
	         message: 'High error budget burn for targeturl=getmeright (current value: {{ $value }})'
	       expr: |
	         sum(http_requests_total:burnrate6h{targeturl="getmeright"}) > (1.00 * (1-0.95000))
	         and
	         sum(http_requests_total:burnrate3d{targeturl="getmeright"}) > (1.00 * (1-0.95000))
	       for: 3h
	       labels:
	         severity: warning
	         targeturl: getmeright
	     - expr: |
	         sum(rate(http_requests_total{targeturl="getmeright",code=~"5.."}[1d]))
	         /
	         sum(rate(http_requests_total{targeturl="getmeright"}[1d]))
	       labels:
	         targeturl: getmeright
	       record: http_requests_total:burnrate1d
	     - expr: |
	         sum(rate(http_requests_total{targeturl="getmeright",code=~"5.."}[1h]))
	         /
	         sum(rate(http_requests_total{targeturl="getmeright"}[1h]))
	       labels:
	         targeturl: getmeright
	       record: http_requests_total:burnrate1h
	     - expr: |
	         sum(rate(http_requests_total{targeturl="getmeright",code=~"5.."}[2h]))
	         /
	         sum(rate(http_requests_total{targeturl="getmeright"}[2h]))
	       labels:
	         targeturl: getmeright
	       record: http_requests_total:burnrate2h
	     - expr: |
	         sum(rate(http_requests_total{targeturl="getmeright",code=~"5.."}[30m]))
	         /
	         sum(rate(http_requests_total{targeturl="getmeright"}[30m]))
	       labels:
	         targeturl: getmeright
	       record: http_requests_total:burnrate30m
	     - expr: |
	         sum(rate(http_requests_total{targeturl="getmeright",code=~"5.."}[3d]))
	         /
	         sum(rate(http_requests_total{targeturl="getmeright"}[3d]))
	       labels:
	         targeturl: getmeright
	       record: http_requests_total:burnrate3d
	     - expr: |
	         sum(rate(http_requests_total{targeturl="getmeright",code=~"5.."}[5m]))
	         /
	         sum(rate(http_requests_total{targeturl="getmeright"}[5m]))
	       labels:
	         targeturl: getmeright
	       record: http_requests_total:burnrate5m
	     - expr: |
	         sum(rate(http_requests_total{targeturl="getmeright",code=~"5.."}[6h]))
	         /
	         sum(rate(http_requests_total{targeturl="getmeright"}[6h]))
	       labels:
	         targeturl: getmeright
	       record: http_requests_total:burnrate6h
	*/

	/*- alert: ErrorBudgetBurn
	      annotations:
	         message: 'High error budget burn for targeturl=getmeright (current value: {{ $value }})'
		 /
	*/
	alertTemplate := ` 
	         sum(http_requests_total:burnrate%[3]s{RouteMonitorUrl="%[1]s"}) > (14.40 * (1-%[2]s))
	         and
	         sum(http_requests_total:burnrate%[4]s{RouteMonitorUrl="%[1]s"}) > (14.40 * (1-%[2]s))
		 `

	routeURL := url
	q := []monitoringv1.Rule{}
	for _, thunderStruct := range []struct {
		duration  string
		timeShort string
		timeLong  string
	}{
		{
			duration:  "2m",
			timeLong:  "1h",
			timeShort: "5m",
		},
		{
			duration:  "15m",
			timeLong:  "6h",
			timeShort: "30m",
		},
		{
			duration:  "1h",
			timeLong:  "1d",
			timeShort: "2h",
		},
		{
			duration:  "3h",
			timeLong:  "3d",
			timeShort: "6h",
		},
		{
			duration:  "2m",
			timeLong:  "1h",
			timeShort: "5m",
		},
	} {

		q = append(q, monitoringv1.Rule{
			Alert:  "ErrorBudgetBurn",
			Expr:   intstr.FromString(fmt.Sprintf(alertTemplate, routeURL, percent, thunderStruct.timeShort, thunderStruct.timeLong)),
			Labels: sampleTemplateLabelsCrit(routeURL),
			Annotations: map[string]string{
				"message": fmt.Sprintf("High error budget burn for RouteMonitorUrl=%s (current value: {{ $value }})", routeURL),
			},
			For: thunderStruct.duration,
		})
	}

	recordingRuleTemplate := `
        	sum(rate(http_requests_total{RouteMonitorUrl="%[1]s",code=~"5.."}[%[2]s]))
        	/
        	sum(rate(http_requests_total{RouteMonitorUrl="%[1]s"}[%[2]s])) `

	for _, time := range []string{"5m", "30m", "1h", "2h", "6h", "1d", "3d"} {
		q = append(q, monitoringv1.Rule{
			Expr:   intstr.FromString(fmt.Sprintf(recordingRuleTemplate, time)),
			Labels: sampleTemplateLabels(routeURL),
			Record: fmt.Sprintf("http_requests_total:burnrate%s", time),
		})

	}
	resource := monitoringv1.PrometheusRule{
		Spec: monitoringv1.PrometheusRuleSpec{
			Groups: []monitoringv1.RuleGroup{
				{
					Name:  "SLOs-http_requests_total",
					Rules: q,
				},
			},
		},
	}
	return resource
}
func sampleTemplateLabelsCrit(url string) map[string]string {
	return map[string]string{
		"severity":        "critical",
		"RouteMonitorUrl": url,
	}
}
func sampleTemplateLabels(url string) map[string]string {
	return map[string]string{
		"RouteMonitorUrl": url,
	}
}
