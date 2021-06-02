package templates

import (
	"fmt"
	"strconv"
	"time"

	prometheus "github.com/prometheus/common/model"

	"github.com/openshift/route-monitor-operator/pkg/consts/blackboxexporter"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	serviceMonitorPeriod string = "30s"
	UrlLabelName         string = "RouteMonitorUrl"
)

// TemplateForServiceMonitorResource returns a ServiceMonitor
func TemplateForServiceMonitorResource(url, blackBoxExporterNamespace string, namespacedName types.NamespacedName) monitoringv1.ServiceMonitor {

	routeMonitorLabels := blackboxexporter.GenerateBlackBoxExporterLables()

	labelSelector := metav1.LabelSelector{MatchLabels: routeMonitorLabels}

	// Currently we only support `http_2xx` as module
	// Still make it a variable so we can easily add functionality later
	modules := []string{"http_2xx"}

	params := map[string][]string{
		"module": modules,
		"target": {url},
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
					Interval: serviceMonitorPeriod,
					// Timeout has to be smaller than probe interval
					ScrapeTimeout: "15s",
					Path:          "/probe",
					Scheme:        "http",
					Params:        params,
					MetricRelabelConfigs: []*monitoringv1.RelabelConfig{
						{
							Replacement: url,
							TargetLabel: UrlLabelName,
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
	burnRate    string
}

func alertThreshold(windowSize, percent, label, burnRate string) string {

	rule := "1-(sum(sum_over_time(probe_success{" + label + "}[" + windowSize + "]))" +
		"/ sum(count_over_time(probe_success{" + label + "}[" + windowSize + "])))" +
		"> (" + burnRate + "*(1-" + percent + "))"

	return rule
}

func sufficientProbes(windowSize, label string) string {
	window, _ := prometheus.ParseDuration(windowSize)
	window_duration := time.Duration(window)
	mPeriod, _ := prometheus.ParseDuration(serviceMonitorPeriod)
	mPeriod_duration := time.Duration(mPeriod)
	necessaryProbesInWindow := int(window_duration.Minutes() / mPeriod_duration.Minutes() * 0.5)

	rule := "sum(count_over_time(probe_success{" + label + "}[" + windowSize + "]))" +
		" > " + strconv.Itoa(necessaryProbesInWindow)

	return rule
}

//render creates a monitoring rule for the defined multiwindow multi-burn rate alert
func (r *multiWindowMultiBurnAlertRule) render(url string, percent string, namespacedName types.NamespacedName) monitoringv1.Rule {
	labelSelector := fmt.Sprintf(`%s="%s"`, UrlLabelName, url)

	alertString := "" +
		alertThreshold(r.shortWindow, percent, labelSelector, r.burnRate) +
		" and " +
		sufficientProbes(r.shortWindow, labelSelector) +
		"\nand\n" +
		alertThreshold(r.longWindow, percent, labelSelector, r.burnRate) +
		" and " +
		sufficientProbes(r.longWindow, labelSelector)

	return monitoringv1.Rule{
		Alert:  namespacedName.Name + "-ErrorBudgetBurn",
		Expr:   intstr.FromString(alertString),
		Labels: sampleTemplateLabels(url, r.severity, namespacedName.Namespace),
		Annotations: map[string]string{
			"message": fmt.Sprintf("High error budget burn for %s (current value: {{ $value }})", url),
		},
		For: r.duration,
	}
}

// TemplateForPrometheusRuleResource returns a PrometheusRule
func TemplateForPrometheusRuleResource(url, percent string, namespacedName types.NamespacedName) monitoringv1.PrometheusRule {

	rules := []monitoringv1.Rule{}
	alertRules := []multiWindowMultiBurnAlertRule{
		{
			duration:    "2m",
			severity:    "critical",
			longWindow:  "1h",
			shortWindow: "5m",
			burnRate:    "14.40",
		},
		{
			duration:    "15m",
			severity:    "critical",
			longWindow:  "6h",
			shortWindow: "30m",
			burnRate:    "6",
		},
		{
			duration:    "1h",
			severity:    "warning",
			longWindow:  "1d",
			shortWindow: "2h",
			burnRate:    "3",
		},
		{
			duration:    "3h",
			severity:    "warning",
			longWindow:  "3d",
			shortWindow: "6h",
			burnRate:    "1",
		},
	}

	for _, alertrule := range alertRules { // Create all the alerts
		rules = append(rules, alertrule.render(url, percent, namespacedName))
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
func sampleTemplateLabels(url, severity, namespace string) map[string]string {
	return map[string]string{
		UrlLabelName: url,
		"namespace":  namespace,
		"severity":   severity,
	}
}
