package alert

import (
	"context"
	"fmt"

	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	util "github.com/openshift/route-monitor-operator/pkg/reconcile"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PrometheusRule struct {
	Rc util.ResourceComparerInterface
}

// Creates or Updates PrometheusRule Deployment according to the template
func (u *PrometheusRule) UpdatePrometheusRuleDeployment(ctx context.Context, c client.Client, template monitoringv1.PrometheusRule) error {
	fmt.Println("UpdatePrometheusRuleDeployment")
	namespacedName := types.NamespacedName{Name: template.Name, Namespace: template.Namespace}
	deployedPrometheusRule := &monitoringv1.PrometheusRule{}
	err := c.Get(ctx, namespacedName, deployedPrometheusRule)
	if err != nil {
		// No similar Prometheus Rule exists
		if !k8serrors.IsNotFound(err) {
			return err
		}
		return c.Create(ctx, &template)
	} else if !u.Rc.DeepEqual(template.Spec, deployedPrometheusRule.Spec) {
		// Update existing PrometheuesRule for the case that the template changed
		deployedPrometheusRule.Spec = template.Spec
		return c.Update(ctx, deployedPrometheusRule)
	}
	return nil
}

func (r *PrometheusRule) DeletePrometheusRuleDeployment(ctx context.Context, c client.Client, prometheusRuleRef v1alpha1.NamespacedName) error {
	fmt.Println("DeletePrometheusRuleDeployment")
	// nothing to delete, stopping early
	if prometheusRuleRef == *new(v1alpha1.NamespacedName) {
		return nil
	}
	namespacedName := types.NamespacedName{Name: prometheusRuleRef.Name, Namespace: prometheusRuleRef.Namespace}
	resource := &monitoringv1.PrometheusRule{}
	// Does the resource already exist?
	err := c.Get(ctx, namespacedName, resource)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			// If this is an unknown error
			return err
		}
		// Resource doesn't exist, nothing to do
		return nil
	}
	return c.Delete(ctx, resource)
}
