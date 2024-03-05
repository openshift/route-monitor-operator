package alert

import (
	"context"
	"fmt"
	"strconv"
	"time"

	prometheus "github.com/prometheus/common/model"

	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	util "github.com/openshift/route-monitor-operator/pkg/reconcile"
	"github.com/openshift/route-monitor-operator/pkg/servicemonitor"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	rhobsv1 "github.com/rhobs/obo-prometheus-operator/pkg/apis/monitoring/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PrometheusRule struct {
	Client   client.Client
	Ctx      context.Context
	Comparer util.ResourceComparerInterface
}

func NewPrometheusRule(ctx context.Context, c client.Client) *PrometheusRule {
	return &PrometheusRule{
		Client:   c,
		Ctx:      ctx,
		Comparer: &util.ResourceComparer{},
	}
}

// Creates or Updates PrometheusRule Deployment according to the template
func (u *PrometheusRule) UpdateCoreOSPrometheusRuleDeployment(template monitoringv1.PrometheusRule) error {
	namespacedName := types.NamespacedName{Name: template.Name, Namespace: template.Namespace}
	deployedPrometheusRule := &monitoringv1.PrometheusRule{}
	err := u.Client.Get(u.Ctx, namespacedName, deployedPrometheusRule)
	if err != nil {
		// No similar Prometheus Rule exists
		if !k8serrors.IsNotFound(err) {
			return err
		}
		return u.Client.Create(u.Ctx, &template)
	}
	if !u.Comparer.DeepEqual(template.Spec, deployedPrometheusRule.Spec) {
		// Update existing PrometheuesRule for the case that the template changed
		deployedPrometheusRule.Spec = template.Spec
		return u.Client.Update(u.Ctx, deployedPrometheusRule)
	}
	return nil
}

// Creates or Updates PrometheusRule Deployment according to the template
func (u *PrometheusRule) UpdateRHOBSPrometheusRuleDeployment(template rhobsv1.PrometheusRule) error {
	namespacedName := types.NamespacedName{Name: template.Name, Namespace: template.Namespace}
	deployedPrometheusRule := &rhobsv1.PrometheusRule{}
	err := u.Client.Get(u.Ctx, namespacedName, deployedPrometheusRule)
	if err != nil {
		// No similar Prometheus Rule exists
		if !k8serrors.IsNotFound(err) {
			return err
		}
		return u.Client.Create(u.Ctx, &template)
	}
	if !u.Comparer.DeepEqual(template.Spec, deployedPrometheusRule.Spec) {
		// Update existing PrometheuesRule for the case that the template changed
		deployedPrometheusRule.Spec = template.Spec
		return u.Client.Update(u.Ctx, deployedPrometheusRule)
	}
	return nil
}

func (u *PrometheusRule) DeletePrometheusRuleDeployment(prometheusRuleRef v1alpha1.NamespacedName) error {
	// nothing to delete, stopping early
	if prometheusRuleRef == (v1alpha1.NamespacedName{}) {
		return nil
	}
	namespacedName := types.NamespacedName{Name: prometheusRuleRef.Name, Namespace: prometheusRuleRef.Namespace}
	resource := &monitoringv1.PrometheusRule{}
	// Does the resource already exist?
	err := u.Client.Get(u.Ctx, namespacedName, resource)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			// If this is an unknown error
			return err
		}
		// Resource doesn't exist, nothing to do
		return nil
	}
	return u.Client.Delete(u.Ctx, resource)
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
	mPeriod, _ := prometheus.ParseDuration(servicemonitor.ServiceMonitorPeriod)
	mPeriod_duration := time.Duration(mPeriod)
	necessaryProbesInWindow := int(window_duration.Minutes() / mPeriod_duration.Minutes() * 0.5)

	rule := "sum(count_over_time(probe_success{" + label + "}[" + windowSize + "]))" +
		" > " + strconv.Itoa(necessaryProbesInWindow)

	return rule
}

// renderCoreOS creates a monitoring.coreos.com rule for the defined multiwindow multi-burn rate alert
func (r *multiWindowMultiBurnAlertRule) renderCoreOS(url string, percent string, namespacedName types.NamespacedName) monitoringv1.Rule {
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

// renderRHOBS creates a monitoring.rhobs rule for the defined multiwindow multi-burn rate alert
func (r *multiWindowMultiBurnAlertRule) renderRHOBS(url string, percent string, namespacedName types.NamespacedName) rhobsv1.Rule {
	labelSelector := fmt.Sprintf(`%s="%s"`, servicemonitor.UrlLabelName, url)

	alertString := "" +
		alertThreshold(r.shortWindow, percent, labelSelector, r.burnRate) +
		" and " +
		sufficientProbes(r.shortWindow, labelSelector) +
		"\nand\n" +
		alertThreshold(r.longWindow, percent, labelSelector, r.burnRate) +
		" and " +
		sufficientProbes(r.longWindow, labelSelector)

	return rhobsv1.Rule{
		Alert:  namespacedName.Name + "-ErrorBudgetBurn",
		Expr:   intstr.FromString(alertString),
		Labels: r.renderLabels(url, namespacedName.Namespace),
		Annotations: map[string]string{
			"message": fmt.Sprintf("High error budget burn for %s (current value: {{ $value }})", url),
		},
		For: r.duration,
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

// TemplateForCoreOSPrometheusRuleResource returns a PrometheusRule
func TemplateForCoreOSPrometheusRuleResource(url, percent string, namespacedName types.NamespacedName) monitoringv1.PrometheusRule {

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
		rules = append(rules, alertrule.renderCoreOS(url, percent, namespacedName))
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

// TemplateForRHOBSPrometheusRuleResource returns a PrometheusRule
func TemplateForRHOBSPrometheusRuleResource(url, percent string, namespacedName types.NamespacedName) rhobsv1.PrometheusRule {

	rules := []rhobsv1.Rule{}
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
		rules = append(rules, alertrule.renderRHOBS(url, percent, namespacedName))
	}

	resource := rhobsv1.PrometheusRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespacedName.Name,
			Namespace: namespacedName.Namespace,
		},
		Spec: rhobsv1.PrometheusRuleSpec{
			Groups: []rhobsv1.RuleGroup{
				{
					Name:  "SLOs-probe",
					Rules: rules,
				},
			},
		},
	}
	return resource
}
