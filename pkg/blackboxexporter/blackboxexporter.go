package blackboxexporter

import (
	"context"

	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	"github.com/openshift/route-monitor-operator/pkg/util/finalizer"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	blackboxExporterName = "blackbox-exporter"
	containerPort        = 9115
	// ContainerPortName is the named port the blackbox exporter will be exposed on
	ContainerPortName = "blackbox"
)

type BlackBoxExporter struct {
	Client         client.Client
	Image          string
	NamespacedName types.NamespacedName
}

func New(client client.Client, blackBoxImage string, blackBoxExporterNamespace string) *BlackBoxExporter {
	blackboxNamespacedName := types.NamespacedName{Name: blackboxExporterName, Namespace: blackBoxExporterNamespace}
	return &BlackBoxExporter{client, blackBoxImage, blackboxNamespacedName}
}

func (b *BlackBoxExporter) GetBlackBoxExporterNamespace() string {
	return b.NamespacedName.Namespace
}

func (b *BlackBoxExporter) ShouldDeleteBlackBoxExporterResources(ctx context.Context) (bool, error) {
	objectsDependingOnExporter := []metav1.Object{}

	routeMonitors := &v1alpha1.RouteMonitorList{}
	if err := b.Client.List(ctx, routeMonitors); err != nil {
		return false, err
	}
	for i := range routeMonitors.Items {
		objectsDependingOnExporter = append(objectsDependingOnExporter, &routeMonitors.Items[i])
	}

	clusterUrlMonitors := &v1alpha1.ClusterUrlMonitorList{}
	if err := b.Client.List(ctx, clusterUrlMonitors); err != nil {
		return false, err
	}
	for i := range clusterUrlMonitors.Items {
		objectsDependingOnExporter = append(objectsDependingOnExporter, &clusterUrlMonitors.Items[i])
	}

	if len(objectsDependingOnExporter) == 1 && finalizer.WasDeleteRequested(objectsDependingOnExporter[0]) {
		return true, nil
	}
	return false, nil
}

func (b *BlackBoxExporter) EnsureBlackBoxExporterDeploymentExists(ctx context.Context) error {
	resource := appsv1.Deployment{}
	// Does the resource already exist?
	err := b.Client.Get(ctx, b.NamespacedName, &resource)
	if err != nil {
		// If this is an unknown error
		if !kerr.IsNotFound(err) {
			// return unexpectedly
			return err
		}
		// populate the resource with the template
		resource := templateForBlackBoxExporterDeployment(b.Image, b.NamespacedName)
		// and create it
		return b.Client.Create(ctx, &resource)
	}
	return nil
}

func (b *BlackBoxExporter) EnsureBlackBoxExporterServiceExists(ctx context.Context) error {
	resource := corev1.Service{}
	// Does the resource already exist?
	if err := b.Client.Get(ctx, b.NamespacedName, &resource); err != nil {
		// If this is an unknown error
		if !kerr.IsNotFound(err) {
			// return unexpectedly
			return err
		}
		// populate the resource with the template
		resource := templateForBlackBoxExporterService(b.NamespacedName)
		// and create it
		return b.Client.Create(ctx, &resource)
	}
	return nil
}

// deploymentForBlackBoxExporter returns a blackbox deployment
func templateForBlackBoxExporterDeployment(blackBoxImage string, blackBoxNamespacedName types.NamespacedName) appsv1.Deployment {
	labels := Labels()
	// hardcode the replicasize for now
	var replicas int32 = 1

	dep := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      blackBoxNamespacedName.Name,
			Namespace: blackBoxNamespacedName.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
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
							ContainerPort: containerPort,
							Name:          ContainerPortName,
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
	labels := Labels()

	svc := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      blackboxNamespacedName.Name,
			Namespace: blackboxNamespacedName.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{{
				TargetPort: intstr.FromString(ContainerPortName),
				Port:       containerPort,
				Name:       ContainerPortName,
			}},
		},
	}
	return svc
}

func (b *BlackBoxExporter) EnsureBlackBoxExporterDeploymentAbsent(ctx context.Context) error {
	resource := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.NamespacedName.Name,
			Namespace: b.NamespacedName.Namespace,
		},
	}

	err := b.Client.Delete(ctx, resource)
	return client.IgnoreNotFound(err)
}

func (b *BlackBoxExporter) EnsureBlackBoxExporterServiceAbsent(ctx context.Context) error {
	resource := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.NamespacedName.Name,
			Namespace: b.NamespacedName.Namespace,
		},
	}

	err := b.Client.Delete(ctx, resource)
	return client.IgnoreNotFound(err)
}

func (b *BlackBoxExporter) EnsureBlackBoxExporterResourcesAbsent(ctx context.Context) error {
	if err := b.EnsureBlackBoxExporterServiceAbsent(ctx); err != nil {
		return err
	}
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

// Labels creates a set of common Labels to blackboxexporter resources
func Labels() map[string]string {
	return map[string]string{"app": blackboxExporterName}
}
