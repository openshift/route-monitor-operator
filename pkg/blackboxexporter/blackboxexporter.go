package blackboxexporter

import (
	"context"
	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	"github.com/openshift/route-monitor-operator/pkg/consts/blackboxexporter"
	"github.com/openshift/route-monitor-operator/pkg/util/finalizer"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type BlackBoxExporter struct {
	Client         client.Client
	Image          string
	NamespacedName types.NamespacedName
}

func New(client client.Client, blackBoxImage string, blackBoxExporterNamespace string) *BlackBoxExporter {
	blackboxNamespacedName := types.NamespacedName{Name: blackboxexporter.BlackBoxExporterName, Namespace: blackBoxExporterNamespace}
	return &BlackBoxExporter{client, blackBoxImage, blackboxNamespacedName}
}

func (b *BlackBoxExporter) GetBlackBoxExporterNamespace() string {
	return b.NamespacedName.Namespace
}

func (b *BlackBoxExporter) ShouldDeleteBlackBoxExporterResources(ctx context.Context) (blackboxexporter.ShouldDeleteBlackBoxExporter, error) {
	objectsDependingOnExporter := []v1.Object{}

	routeMonitors := &v1alpha1.RouteMonitorList{}
	if err := b.Client.List(ctx, routeMonitors); err != nil {
		return blackboxexporter.KeepBlackBoxExporter, err
	}
	for i := range routeMonitors.Items {
		objectsDependingOnExporter = append(objectsDependingOnExporter, &routeMonitors.Items[i])
	}

	clusterUrlMonitors := &v1alpha1.ClusterUrlMonitorList{}
	if err := b.Client.List(ctx, clusterUrlMonitors); err != nil {
		return blackboxexporter.KeepBlackBoxExporter, err
	}
	for i := range clusterUrlMonitors.Items {
		objectsDependingOnExporter = append(objectsDependingOnExporter, &clusterUrlMonitors.Items[i])
	}
	//b.Log.V(4).Info("Number of objects depending on BlackBoxExporter:", "amountOfObjects", len(objectsDependingOnExporter))

	if len(objectsDependingOnExporter) == 1 && finalizer.WasDeleteRequested(objectsDependingOnExporter[0]) {
		//b.Log.V(3).Info("Deleting BlackBoxResources: decided to clean BlackBoxExporter resources")
		return blackboxexporter.DeleteBlackBoxExporter, nil
	}
	return blackboxexporter.KeepBlackBoxExporter, nil
}

func (b *BlackBoxExporter) EnsureBlackBoxExporterDeploymentExists(ctx context.Context) error {
	resource := appsv1.Deployment{}
	populationFunc := func() appsv1.Deployment {
		return templateForBlackBoxExporterDeployment(b.Image, b.NamespacedName)
	}

	// Does the resource already exist?
	err := b.Client.Get(ctx, b.NamespacedName, &resource)
	if err != nil {
		// If this is an unknown error
		if !k8serrors.IsNotFound(err) {
			// return unexpectedly
			return err
		}
		// populate the resource with the template
		resource := populationFunc()
		// and create it
		err = b.Client.Create(ctx, &resource)
		if err != nil {
			return err
		}
	}
	return nil
}

func (b *BlackBoxExporter) EnsureBlackBoxExporterServiceExists(ctx context.Context) error {
	resource := corev1.Service{}
	populationFunc := func() corev1.Service { return templateForBlackBoxExporterService(b.NamespacedName) }

	// Does the resource already exist?
	if err := b.Client.Get(ctx, b.NamespacedName, &resource); err != nil {
		// If this is an unknown error
		if !k8serrors.IsNotFound(err) {
			// return unexpectedly
			return err
		}
		// populate the resource with the template
		resource := populationFunc()
		// and create it
		if err = b.Client.Create(ctx, &resource); err != nil {
			return err
		}
	}
	return nil
}

// deploymentForBlackBoxExporter returns a blackbox deployment
func templateForBlackBoxExporterDeployment(blackBoxImage string, blackBoxNamespacedName types.NamespacedName) appsv1.Deployment {
	labels := blackboxexporter.GenerateBlackBoxExporterLables()
	labelSelectors := metav1.LabelSelector{
		MatchLabels: labels}
	// hardcode the replicasize for no
	// replicas := m.Spec.Size
	var replicas int32 = 1

	dep := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      blackBoxNamespacedName.Name,
			Namespace: blackBoxNamespacedName.Namespace,
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
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{{
								Preference: corev1.NodeSelectorTerm{
									MatchExpressions: []corev1.NodeSelectorRequirement{{
										Key:      "node-role.kubernetes.io/infra",
										Operator: corev1.NodeSelectorOpExists,
									}},
								},
								Weight: 1,
							}},
						},
					},
					Tolerations: []corev1.Toleration{{
						Operator: corev1.TolerationOpExists,
						Effect:   corev1.TaintEffectNoSchedule,
						Key:      "node-role.kubernetes.io/infra",
					}},
					Containers: []corev1.Container{{
						Image: blackBoxImage,
						Name:  "blackbox-exporter",
						Ports: []corev1.ContainerPort{{
							ContainerPort: blackboxexporter.BlackBoxExporterPortNumber,
							Name:          blackboxexporter.BlackBoxExporterPortName,
						}},
					}},
				},
			},
		},
	}
	return dep
}

// templateForBlackBoxExporterService returns a blackbox service
func templateForBlackBoxExporterService(blackboxNamespacedName types.NamespacedName) corev1.Service {
	labels := blackboxexporter.GenerateBlackBoxExporterLables()

	svc := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      blackboxNamespacedName.Name,
			Namespace: blackboxNamespacedName.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{{
				TargetPort: intstr.FromString(blackboxexporter.BlackBoxExporterPortName),
				Port:       blackboxexporter.BlackBoxExporterPortNumber,
				Name:       blackboxexporter.BlackBoxExporterPortName,
			}},
		},
	}
	return svc
}

func (b *BlackBoxExporter) EnsureBlackBoxExporterDeploymentAbsent(ctx context.Context) error {
	resource := &appsv1.Deployment{}

	// Does the resource already exist?
	err := b.Client.Get(ctx, b.NamespacedName, resource)
	if err != nil {
		// If this is an unknown error
		if !k8serrors.IsNotFound(err) {
			// return unexpectedly
			return err
		}
		// Resource doesn't exist, nothing to do
		return nil
	}
	err = b.Client.Delete(ctx, resource)
	if err != nil {
		return err
	}
	return nil
}

func (b *BlackBoxExporter) EnsureBlackBoxExporterServiceAbsent(ctx context.Context) error {
	resource := &corev1.Service{}

	// Does the resource already exist?
	err := b.Client.Get(ctx, b.NamespacedName, resource)
	if err != nil {
		// If this is an unknown error
		if !k8serrors.IsNotFound(err) {
			// return unexpectedly
			return err
		}
		// Resource doesn't exist, nothing to do
		return nil
	}
	err = b.Client.Delete(ctx, resource)
	if err != nil {
		return err
	}
	return nil
}

func (b *BlackBoxExporter) EnsureBlackBoxExporterResourcesAbsent(ctx context.Context) error {
	//b.Log.V(2).Info("Entering EnsureBlackBoxExporterServiceAbsent")
	if err := b.EnsureBlackBoxExporterServiceAbsent(ctx); err != nil {
		return err
	}
	//b.Log.V(2).Info("Entering EnsureBlackBoxExporterDeploymentAbsent")
	if err := b.EnsureBlackBoxExporterDeploymentAbsent(ctx); err != nil {
		return err
	}
	return nil
}

func (b *BlackBoxExporter) EnsureBlackBoxExporterResourcesExist(ctx context.Context) error {
	if err := b.EnsureBlackBoxExporterDeploymentExists(ctx); err != nil {
		return err
	}
	// Creating Service after because:
	//
	// A Service should not point to an empty target (Deployment)
	if err := b.EnsureBlackBoxExporterServiceExists(ctx); err != nil {
		return err
	}
	return nil
}
