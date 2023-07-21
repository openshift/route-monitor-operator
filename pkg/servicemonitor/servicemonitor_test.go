package servicemonitor

import (
	"context"
	"testing"

	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	rhobsv1 "github.com/rhobs/obo-prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestTemplateAndUpdateServiceMonitorDeployment(t *testing.T) {
	const (
		routeURL                  = "example.com"
		blackBoxExporterNamespace = "blackbox-namespace"
		clusterID                 = "test-id"
		serviceMonitorName        = "test-name"
		serviceMonitorNamespace   = "test-namespace"
	)

	var namespacedName = types.NamespacedName{
		Namespace: serviceMonitorNamespace,
		Name:      serviceMonitorName,
	}

	tests := []struct {
		name  string
		objs  []client.Object
		isHCP bool
	}{
		{
			name:  "Creating for Classic OSD",
			isHCP: false,
		},
		{
			name: "Updating for Classic OSD",
			objs: []client.Object{
				&monitoringv1.ServiceMonitor{
					ObjectMeta: metav1.ObjectMeta{
						Name:      serviceMonitorName,
						Namespace: serviceMonitorNamespace,
					},
					Spec: monitoringv1.ServiceMonitorSpec{
						NamespaceSelector: monitoringv1.NamespaceSelector{
							MatchNames: []string{blackBoxExporterNamespace},
						},
					},
				},
			},
			isHCP: false,
		},
		{
			name:  "Creating for HCP",
			isHCP: true,
		},
		{
			name: "Updating for HCP",
			objs: []client.Object{
				&rhobsv1.ServiceMonitor{
					ObjectMeta: metav1.ObjectMeta{
						Name:      serviceMonitorName,
						Namespace: serviceMonitorNamespace,
					},
					Spec: rhobsv1.ServiceMonitorSpec{
						NamespaceSelector: rhobsv1.NamespaceSelector{
							MatchNames: []string{blackBoxExporterNamespace},
						},
					},
				},
			},
			isHCP: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := runtime.NewScheme()
			if err := rhobsv1.AddToScheme(s); err != nil {
				t.Fatal(err)
			}

			if err := monitoringv1.AddToScheme(s); err != nil {
				t.Fatal(err)
			}

			serviceMonitor := NewServiceMonitor(fake.NewClientBuilder().WithScheme(s).WithObjects(test.objs...).Build())
			if err := serviceMonitor.TemplateAndUpdateServiceMonitorDeployment(context.TODO(), routeURL, blackBoxExporterNamespace, namespacedName, clusterID, test.isHCP); err != nil {
				t.Error(err)
			}

			params := map[string][]string{
				"module": {"http_2xx"},
				"target": {routeURL},
			}
			if test.isHCP {
				template := serviceMonitor.HyperShiftTemplateForServiceMonitorResource(routeURL, blackBoxExporterNamespace, params, namespacedName, clusterID)
				sm := new(rhobsv1.ServiceMonitor)
				if err := serviceMonitor.Client.Get(context.TODO(), namespacedName, sm); err != nil {
					t.Errorf("expected no err, got %v", err)
				}
				assert.Equal(t, template.Spec, sm.Spec)
			} else {
				template := serviceMonitor.TemplateForServiceMonitorResource(routeURL, blackBoxExporterNamespace, params, namespacedName, clusterID)
				sm := new(monitoringv1.ServiceMonitor)
				if err := serviceMonitor.Client.Get(context.TODO(), namespacedName, sm); err != nil {
					t.Errorf("expected no err, got %v", err)
				}
				assert.Equal(t, template.Spec, sm.Spec)
			}
		})
	}
}

func TestDeleteServiceMonitorDeployment(t *testing.T) {
	var (
		objs = []client.Object{
			&monitoringv1.ServiceMonitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-name",
					Namespace: "test-namespace",
				},
			},
			&rhobsv1.ServiceMonitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-name",
					Namespace: "test-namespace",
				},
			},
		}
		serviceMonitorRef = v1alpha1.NamespacedName{
			Name:      "test-name",
			Namespace: "test-namespace",
		}
	)

	tests := []struct {
		name  string
		isHCP bool
	}{
		{
			name:  "Delete for classic OSD",
			isHCP: false,
		},
		{
			name:  "Delete for HCP",
			isHCP: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := runtime.NewScheme()
			if err := rhobsv1.AddToScheme(s); err != nil {
				t.Fatal(err)
			}

			if err := monitoringv1.AddToScheme(s); err != nil {
				t.Fatal(err)
			}

			serviceMonitor := NewServiceMonitor(fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build())
			if err := serviceMonitor.DeleteServiceMonitorDeployment(context.TODO(), serviceMonitorRef, test.isHCP); err != nil {
				t.Error(err)
			}

			// Ensure the "other" servicemonitor is still present
			// for HyperShift we don't expect to delete monitoringv1, otherwise we don't expect to delete rhobsv1
			if test.isHCP {
				sm := new(monitoringv1.ServiceMonitor)
				if err := serviceMonitor.Client.Get(context.TODO(), types.NamespacedName{Namespace: serviceMonitorRef.Namespace, Name: serviceMonitorRef.Name}, sm); err != nil {
					t.Errorf("expected no err, got %v", err)
				}
				assert.NotNil(t, sm)
			} else {
				sm := new(rhobsv1.ServiceMonitor)
				if err := serviceMonitor.Client.Get(context.TODO(), types.NamespacedName{Namespace: serviceMonitorRef.Namespace, Name: serviceMonitorRef.Name}, sm); err != nil {
					t.Errorf("expected no err, got %v", err)
				}
				assert.NotNil(t, sm)
			}
		})
	}
}
