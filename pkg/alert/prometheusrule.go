package alert

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"time"

	prometheus "github.com/prometheus/common/model"

	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	"github.com/openshift/route-monitor-operator/pkg/servicemonitor"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"

	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PrometheusRule struct {
	Client client.Client
}

func NewPrometheusRule(c client.Client) *PrometheusRule {
	return &PrometheusRule{
		Client: c,
	}
}

// Creates or Updates PrometheusRule Deployment according to the template
func (u *PrometheusRule) UpdatePrometheusRuleDeployment(ctx context.Context, template monitoringv1.PrometheusRule) error {
	namespacedName := types.NamespacedName{Name: template.Name, Namespace: template.Namespace}
	deployedPrometheusRule := &monitoringv1.PrometheusRule{}
	if err := u.Client.Get(ctx, namespacedName, deployedPrometheusRule); err != nil {
		// No similar Prometheus Rule exists
		if !kerr.IsNotFound(err) {
			return err
		}
		return u.Client.Create(ctx, &template)
	}
	if !reflect.DeepEqual(template.Spec, deployedPrometheusRule.Spec) {
		// Update existing PrometheusRule for the case that the template changed
		deployedPrometheusRule.Spec = template.Spec
		return u.Client.Update(ctx, deployedPrometheusRule)
	}
	return nil
}

func (u *PrometheusRule) DeletePrometheusRuleDeployment(ctx context.Context, prometheusRuleRef v1alpha1.NamespacedName) error {
	if prometheusRuleRef.Name == "" || prometheusRuleRef.Namespace == "" {
		return nil
	}

	resource := &monitoringv1.PrometheusRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      prometheusRuleRef.Name,
			Namespace: prometheusRuleRef.Namespace,
		},
	}

	if err := u.Client.Delete(ctx, resource); err != nil {
		return client.IgnoreNotFound(err)
	}

	return nil
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
	windowDuration := time.Duration(window)
	mPeriod, _ := prometheus.ParseDuration(servicemonitor.MetricsScrapeInterval)
	mPeriodDuration := time.Duration(mPeriod)
	necessaryProbesInWindow := int(windowDuration.Minutes() / mPeriodDuration.Minutes() * 0.5)

	rule := "sum(count_over_time(probe_success{" + label + "}[" + windowSize + "]))" +
		" > " + strconv.Itoa(necessaryProbesInWindow)

	return rule
}

// render creates a monitoring rule for the defined multiwindow multi-burn rate alert
func (r *multiWindowMultiBurnAlertRule) render(url string, percent string, namespacedName types.NamespacedName) monitoringv1.Rule {
	labelSelector := fmt.Sprintf(`%s="%s"`, servicemonitor.UrlLabelName, url)

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
		Labels: r.renderLabels(url, namespacedName.Namespace),
		Annotations: map[string]string{
			"message": fmt.Sprintf("High error budget burn for %s (current value: {{ $value }})", url),
		},
		For: monitoringv1.Duration(r.duration),
	}
}

func (r *multiWindowMultiBurnAlertRule) renderLabels(url, namespace string) map[string]string {
	return map[string]string{
		servicemonitor.UrlLabelName: url,
		"namespace":                 namespace,
		"severity":                  r.severity,
		"long_window":               r.longWindow,
		"short_window":              r.shortWindow,
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
