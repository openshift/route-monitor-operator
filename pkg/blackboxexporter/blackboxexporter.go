package blackboxexporter

import (
	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	"github.com/openshift/route-monitor-operator/pkg/consts/blackbox"
	"github.com/openshift/route-monitor-operator/pkg/util/finalizer"

	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type BlackboxExporter struct {
	Client        client.Client
	Log           logr.Logger
	Ctx           context.Context
	BlackBoxImage string
}

func New(client client.Client, log logr.Logger, ctx context.Context, blackBoxImage string) *BlackboxExporter {
	return &BlackboxExporter{client, log, ctx, blackBoxImage}
}

func (b *BlackboxExporter) ShouldDeleteBlackBoxExporterResources() (blackbox.ShouldDeleteBlackBoxExporter, error) {
	objectsDependingOnExporter := []v1.Object{}

	routeMonitors := &v1alpha1.RouteMonitorList{}
	if err := b.Client.List(b.Ctx, routeMonitors); err != nil {
		return blackbox.KeepBlackBoxExporter, err
	}
	for _, routeMonitor := range routeMonitors.Items {
		objectsDependingOnExporter = append(objectsDependingOnExporter, &routeMonitor)
	}

	clusterUrlMonitors := &v1alpha1.ClusterUrlMonitorList{}
	if err := b.Client.List(b.Ctx, clusterUrlMonitors); err != nil {
		return blackbox.KeepBlackBoxExporter, err
	}
	for _, clusterUrlMonitor := range clusterUrlMonitors.Items {
		objectsDependingOnExporter = append(objectsDependingOnExporter, &clusterUrlMonitor)
	}
	b.Log.V(4).Info("Number of objects depending on BlackBoxExporter:", "amountOfObjects", len(objectsDependingOnExporter))

	if len(objectsDependingOnExporter) == 1 && finalizer.WasDeleteRequested(objectsDependingOnExporter[0]) {
		b.Log.V(3).Info("Deleting BlackBoxResources: decided to clean BlackBoxExporter resources")
		return blackbox.DeleteBlackBoxExporter, nil
	}
	return blackbox.KeepBlackBoxExporter, nil
}

func (b *BlackboxExporter) EnsureBlackBoxExporterDeploymentExists() error {
	resource := appsv1.Deployment{}
	populationFunc := func() appsv1.Deployment { return templateForBlackBoxExporterDeployment(b.BlackBoxImage) }

	// Does the resource already exist?
	err := b.Client.Get(b.Ctx, blackbox.BlackBoxNamespacedName, &resource)
	if err != nil {
		// If this is an unknown error
		if !k8serrors.IsNotFound(err) {
			// return unexpectedly
			return err
		}
		// populate the resource with the template
		resource := populationFunc()
		// and create it
		err = b.Client.Create(b.Ctx, &resource)
		if err != nil {
			return err
		}
	}
	return nil
}

func (b *BlackboxExporter) EnsureBlackBoxExporterServiceExists() error {
	resource := corev1.Service{}
	populationFunc := templateForBlackBoxExporterService

	// Does the resource already exist?
	if err := b.Client.Get(b.Ctx, blackbox.BlackBoxNamespacedName, &resource); err != nil {
		// If this is an unknown error
		if !k8serrors.IsNotFound(err) {
			// return unexpectedly
			return err
		}
		// populate the resource with the template
		resource := populationFunc()
		// and create it
		if err = b.Client.Create(b.Ctx, &resource); err != nil {
			return err
		}
	}
	return nil
}

// deploymentForBlackBoxExporter returns a blackbox deployment
func templateForBlackBoxExporterDeployment(blackBoxImage string) appsv1.Deployment {
	labels := blackbox.GenerateBlackBoxLables()
	labelSelectors := metav1.LabelSelector{
		MatchLabels: labels}
	// hardcode the replicasize for no
	//replicas := m.Spec.Size
	var replicas int32 = 1

	dep := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      blackbox.BlackBoxName,
			Namespace: blackbox.BlackBoxNamespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &labelSelectors,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Image: blackBoxImage,
						Name:  "blackbox-exporter",
						Ports: []corev1.ContainerPort{{
							ContainerPort: blackbox.BlackBoxPortNumber,
							Name:          blackbox.BlackBoxPortName,
						}},
					}},
				},
			},
		},
	}
	return dep
}

// templateForBlackBoxExporterService returns a blackbox service
func templateForBlackBoxExporterService() corev1.Service {
	labels := blackbox.GenerateBlackBoxLables()

	svc := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      blackbox.BlackBoxName,
			Namespace: blackbox.BlackBoxNamespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{{
				TargetPort: intstr.FromString(blackbox.BlackBoxPortName),
				Port:       blackbox.BlackBoxPortNumber,
				Name:       blackbox.BlackBoxPortName,
			}},
		},
	}
	return svc
}

func (b *BlackboxExporter) EnsureBlackBoxExporterDeploymentAbsent() error {
	resource := &appsv1.Deployment{}

	// Does the resource already exist?
	err := b.Client.Get(b.Ctx, blackbox.BlackBoxNamespacedName, resource)
	if err != nil {
		// If this is an unknown error
		if !k8serrors.IsNotFound(err) {
			// return unexpectedly
			return err
		}
		// Resource doesn't exist, nothing to do
		return nil
	}
	err = b.Client.Delete(b.Ctx, resource)
	if err != nil {
		return err
	}
	return nil
}

func (b *BlackboxExporter) EnsureBlackBoxExporterServiceAbsent() error {
	resource := &corev1.Service{}

	// Does the resource already exist?
	err := b.Client.Get(b.Ctx, blackbox.BlackBoxNamespacedName, resource)
	if err != nil {
		// If this is an unknown error
		if !k8serrors.IsNotFound(err) {
			// return unexpectedly
			return err
		}
		// Resource doesn't exist, nothing to do
		return nil
	}
	err = b.Client.Delete(b.Ctx, resource)
	if err != nil {
		return err
	}
	return nil
}

func (b *BlackboxExporter) EnsureBlackBoxExporterResourcesAbsent() error {
	b.Log.V(2).Info("Entering EnsureBlackBoxExporterServiceAbsent")
	if err := b.EnsureBlackBoxExporterServiceAbsent(); err != nil {
		return err
	}
	b.Log.V(2).Info("Entering EnsureBlackBoxExporterDeploymentAbsent")
	if err := b.EnsureBlackBoxExporterDeploymentAbsent(); err != nil {
		return err
	}
	return nil
}

func (b *BlackboxExporter) EnsureBlackBoxExporterResourcesExist() error {
	if err := b.EnsureBlackBoxExporterDeploymentExists(); err != nil {
		return err
	}
	// Creating Service after because:
	//
	// A Service should not point to an empty target (Deployment)
	if err := b.EnsureBlackBoxExporterServiceExists(); err != nil {
		return err
	}
	return nil
}
