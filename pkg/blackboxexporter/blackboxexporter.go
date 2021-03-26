package blackboxexporter

import (
	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	"github.com/openshift/route-monitor-operator/pkg/util/finalizer"

	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	blackboxconst "github.com/openshift/route-monitor-operator/pkg/consts/blackboxexporter"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type BlackBoxExporter struct {
	Client         client.Client
	Log            logr.Logger
	Ctx            context.Context
	Image          string
	NamespacedName types.NamespacedName
}

func New(client client.Client, log logr.Logger, ctx context.Context, blackBoxImage string, blackBoxExporterNamespace string) *BlackBoxExporter {
	blackboxNamespacedName := types.NamespacedName{Name: blackboxconst.BlackBoxExporterName, Namespace: blackBoxExporterNamespace}
	return &BlackBoxExporter{client, log, ctx, blackBoxImage, blackboxNamespacedName}
}

func (b *BlackBoxExporter) GetBlackBoxExporterNamespace() string {
	return b.NamespacedName.Namespace
}

func (b *BlackBoxExporter) ShouldDeleteBlackBoxExporterResources() (blackboxconst.ShouldDeleteBlackBoxExporter, error) {
	objectsDependingOnExporter := []v1.Object{}

	routeMonitors := &v1alpha1.RouteMonitorList{}
	if err := b.Client.List(b.Ctx, routeMonitors); err != nil {
		return blackboxconst.KeepBlackBoxExporter, err
	}
	for _, routeMonitor := range routeMonitors.Items {
		objectsDependingOnExporter = append(objectsDependingOnExporter, &routeMonitor)
	}

	clusterUrlMonitors := &v1alpha1.ClusterUrlMonitorList{}
	if err := b.Client.List(b.Ctx, clusterUrlMonitors); err != nil {
		return blackboxconst.KeepBlackBoxExporter, err
	}
	for _, clusterUrlMonitor := range clusterUrlMonitors.Items {
		objectsDependingOnExporter = append(objectsDependingOnExporter, &clusterUrlMonitor)
	}
	b.Log.V(4).Info("Number of objects depending on BlackBoxExporter:", "amountOfObjects", len(objectsDependingOnExporter))

	if len(objectsDependingOnExporter) == 1 && finalizer.WasDeleteRequested(objectsDependingOnExporter[0]) {
		b.Log.V(3).Info("Deleting BlackBoxResources: decided to clean BlackBoxExporter resources")
		return blackboxconst.DeleteBlackBoxExporter, nil
	}
	return blackboxconst.KeepBlackBoxExporter, nil
}

func (b *BlackBoxExporter) EnsureBlackBoxExporterDeploymentExists() error {
	resource := appsv1.Deployment{}

	// Does the resource already exist?
	err := b.Client.Get(b.Ctx, b.NamespacedName, &resource)
	if err != nil {
		// If this is an unknown error
		if !k8serrors.IsNotFound(err) {
			// return unexpectedly
			return err
		}
		// populate the resource with the template
		blackBoxLabels, err := blackboxconst.GetBlackBoxLabels(b.Client)
		if err != nil {
			return err
		}
		resource := templateForBlackBoxExporterDeployment(b.Image, b.NamespacedName, blackBoxLabels)
		// and create it
		err = b.Client.Create(b.Ctx, &resource)
		if err != nil {
			return err
		}
	}
	return nil
}

func (b *BlackBoxExporter) EnsureBlackBoxExporterServiceExists() error {
	resource := corev1.Service{}

	// Does the resource already exist?
	if err := b.Client.Get(b.Ctx, b.NamespacedName, &resource); err != nil {
		// If this is an unknown error
		if !k8serrors.IsNotFound(err) {
			// return unexpectedly
			return err
		}
		// populate the resource with the template
		blackBoxLabels, err := blackboxconst.GetBlackBoxLabels(b.Client)
		if err != nil {
			return err
		}
		resource := templateForBlackBoxExporterService(b.NamespacedName, blackBoxLabels)
		// and create it
		if err = b.Client.Create(b.Ctx, &resource); err != nil {
			return err
		}
	}
	return nil
}

// deploymentForBlackBoxExporter returns a blackbox deployment
func templateForBlackBoxExporterDeployment(blackBoxImage string, blackBoxNamespacedName types.NamespacedName, blackBoxLabels map[string]string) appsv1.Deployment {
	labelSelectors := metav1.LabelSelector{
		MatchLabels: blackBoxLabels}
	// hardcode the replicasize for no
	//replicas := m.Spec.Size
	var replicas int32 = 1

	dep := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      blackBoxNamespacedName.Name,
			Namespace: blackBoxNamespacedName.Namespace,
			Labels:    blackBoxLabels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &labelSelectors,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: blackBoxLabels,
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
							ContainerPort: blackboxconst.BlackBoxExporterPortNumber,
							Name:          blackboxconst.BlackBoxExporterPortName,
						}},
					}},
				},
			},
		},
	}
	return dep
}

// templateForBlackBoxExporterService returns a blackbox service
func templateForBlackBoxExporterService(blackboxNamespacedName types.NamespacedName, blackBoxLabels map[string]string) corev1.Service {

	svc := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      blackboxNamespacedName.Name,
			Namespace: blackboxNamespacedName.Namespace,
			Labels:    blackBoxLabels,
		},
		Spec: corev1.ServiceSpec{
			Selector: blackBoxLabels,
			Ports: []corev1.ServicePort{{
				TargetPort: intstr.FromString(blackboxconst.BlackBoxExporterPortName),
				Port:       blackboxconst.BlackBoxExporterPortNumber,
				Name:       blackboxconst.BlackBoxExporterPortName,
			}},
		},
	}
	return svc
}

func (b *BlackBoxExporter) EnsureBlackBoxExporterDeploymentAbsent() error {
	resource := &appsv1.Deployment{}

	// Does the resource already exist?
	err := b.Client.Get(b.Ctx, b.NamespacedName, resource)
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

func (b *BlackBoxExporter) EnsureBlackBoxExporterServiceAbsent() error {
	resource := &corev1.Service{}

	// Does the resource already exist?
	err := b.Client.Get(b.Ctx, b.NamespacedName, resource)
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

func (b *BlackBoxExporter) EnsureBlackBoxExporterResourcesAbsent() error {
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

func (b *BlackBoxExporter) EnsureBlackBoxExporterResourcesExist() error {
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
