package alert

import (
	"context"
	"testing"

	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestTemplateAndUpdatePrometheusRuleDeployment(t *testing.T) {
	const (
		url                     = "example.com"
		percent                 = "99"
		prometheusRuleName      = "test-name"
		prometheusRuleNamespace = "test-namespace"
	)

	var namespacedName = types.NamespacedName{
		Namespace: prometheusRuleNamespace,
		Name:      prometheusRuleName,
	}

	tests := []struct {
		name string
		objs []client.Object
	}{
		{
			name: "Creating for Classic OSD",
		},
		{
			name: "Updating for Classic OSD",
			objs: []client.Object{
				&monitoringv1.PrometheusRule{
					ObjectMeta: metav1.ObjectMeta{
						Name:      prometheusRuleName,
						Namespace: prometheusRuleNamespace,
					},
					Spec: monitoringv1.PrometheusRuleSpec{},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := runtime.NewScheme()
			if err := monitoringv1.AddToScheme(s); err != nil {
				t.Fatal(err)
			}

			prometheusRule := NewPrometheusRule(fake.NewClientBuilder().WithScheme(s).WithObjects(test.objs...).Build())
			template := TemplateForPrometheusRuleResource(url, percent, namespacedName)
			if err := prometheusRule.UpdatePrometheusRuleDeployment(context.TODO(), template); err != nil {
				t.Error(err)
			}

			pr := new(monitoringv1.PrometheusRule)
			if err := prometheusRule.Client.Get(context.TODO(), namespacedName, pr); err != nil {
				t.Errorf("expected no err, got %v", err)
			}
			assert.Equal(t, template.Spec, pr.Spec)
		})
	}
}

func TestDeletePrometheusRuleDeployment(t *testing.T) {
	var (
		objs = []client.Object{
			&monitoringv1.PrometheusRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-name",
					Namespace: "test-namespace",
				},
			},
		}
	)

	tests := []struct {
		name              string
		prometheusRuleRef v1alpha1.NamespacedName
	}{
		{
			name: "Delete",
			prometheusRuleRef: v1alpha1.NamespacedName{
				Name:      "test-name",
				Namespace: "test-namespace",
			},
		},
		{
			name:              "Empty ref",
			prometheusRuleRef: v1alpha1.NamespacedName{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := runtime.NewScheme()
			if err := monitoringv1.AddToScheme(s); err != nil {
				t.Fatal(err)
			}

			prometheusRule := NewPrometheusRule(fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build())
			if err := prometheusRule.DeletePrometheusRuleDeployment(context.TODO(), test.prometheusRuleRef); err != nil {
				t.Error(err)
			}

			if err := prometheusRule.DeletePrometheusRuleDeployment(context.TODO(), test.prometheusRuleRef); err != nil {
				t.Errorf("expected no err when deleting a second time, got %v", err)
			}
		})
	}
}
