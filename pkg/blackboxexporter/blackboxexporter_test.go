package blackboxexporter

import (
	"context"
	"testing"
	"time"

	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	"github.com/stretchr/testify/assert"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	testBlackBoxImage     = "blackbox-exporter:latest"
	testBlackBoxNamespace = "test-namespace"
)

func TestEnsureBlackBoxExporterDeployment(t *testing.T) {
	tests := []struct {
		name string
		objs []client.Object
	}{
		{
			name: "Ensure creation and deletion are idempotent",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := runtime.NewScheme()
			if err := corev1.AddToScheme(s); err != nil {
				t.Fatal(err)
			}

			if err := appsv1.AddToScheme(s); err != nil {
				t.Fatal(err)
			}

			bbe := New(fake.NewClientBuilder().WithScheme(s).WithObjects(test.objs...).Build(), testBlackBoxImage, testBlackBoxNamespace)
			if err := bbe.EnsureBlackBoxExporterResourcesExist(context.TODO()); err != nil {
				t.Error(err)
			}

			dep := &appsv1.Deployment{}
			if err := bbe.Client.Get(context.TODO(), types.NamespacedName{Name: blackboxExporterName, Namespace: testBlackBoxNamespace}, dep); err != nil {
				t.Errorf("expected no err, got %v", err)
			}

			svc := &corev1.Service{}
			if err := bbe.Client.Get(context.TODO(), types.NamespacedName{Name: blackboxExporterName, Namespace: testBlackBoxNamespace}, svc); err != nil {
				t.Errorf("expected no err, got %v", err)
			}

			if err := bbe.EnsureBlackBoxExporterResourcesExist(context.TODO()); err != nil {
				t.Error(err)
			}

			if err := bbe.EnsureBlackBoxExporterResourcesAbsent(context.TODO()); err != nil {
				t.Errorf("failed to ensure blackbox exporter resources absent: %v", err)
			}

			if err := bbe.EnsureBlackBoxExporterResourcesAbsent(context.TODO()); err != nil {
				t.Errorf("failed to ensure blackbox exporter resources absent a second time: %v", err)
			}
		})
	}
}

func TestShouldDeleteBlackboxExporterResources(t *testing.T) {
	tests := []struct {
		name     string
		objs     []client.Object
		expected bool
	}{
		{
			name: "only one object with deletion timestamp",
			objs: []client.Object{
				&v1alpha1.ClusterUrlMonitor{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-clusterurlmonitor",
						Namespace: "test-namespace",
						DeletionTimestamp: &metav1.Time{
							Time: time.Now(),
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "many objects",
			objs: []client.Object{
				&v1alpha1.ClusterUrlMonitor{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-clusterurlmonitor",
						Namespace: "test-namespace",
						DeletionTimestamp: &metav1.Time{
							Time: time.Now(),
						},
					},
				},
				&v1alpha1.RouteMonitor{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-routemonitor",
						Namespace: "test-namespace",
					},
				},
			},
			expected: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := runtime.NewScheme()
			if err := v1alpha1.AddToScheme(s); err != nil {
				t.Fatal(err)
			}

			bbe := New(fake.NewClientBuilder().WithScheme(s).WithObjects(test.objs...).Build(), testBlackBoxImage, testBlackBoxNamespace)
			actual, err := bbe.ShouldDeleteBlackBoxExporterResources(context.TODO())
			if err != nil {
				t.Error(err)
			}

			assert.Equal(t, test.expected, actual)
		})
	}
}
