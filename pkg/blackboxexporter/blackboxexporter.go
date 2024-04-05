package blackboxexporter

import (
	"reflect"

	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	"github.com/openshift/route-monitor-operator/pkg/consts/blackboxexporter"
	"github.com/openshift/route-monitor-operator/pkg/util"
	"github.com/openshift/route-monitor-operator/pkg/util/finalizer"

	"context"

	"github.com/go-logr/logr"
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
	Log            logr.Logger
	Ctx            context.Context
	Image          string
	NamespacedName types.NamespacedName
}

func New(client client.Client, log logr.Logger, ctx context.Context, blackBoxImage string, blackBoxExporterNamespace string) *BlackBoxExporter {
	blackboxNamespacedName := types.NamespacedName{Name: blackboxexporter.BlackBoxExporterName, Namespace: blackBoxExporterNamespace}
	return &BlackBoxExporter{client, log, ctx, blackBoxImage, blackboxNamespacedName}
}

func (b *BlackBoxExporter) GetBlackBoxExporterNamespace() string {
	return b.NamespacedName.Namespace
}

func (b *BlackBoxExporter) ShouldDeleteBlackBoxExporterResources() (blackboxexporter.ShouldDeleteBlackBoxExporter, error) {
	objectsDependingOnExporter := []v1.Object{}

	routeMonitors := &v1alpha1.RouteMonitorList{}
	if err := b.Client.List(b.Ctx, routeMonitors); err != nil {
		return blackboxexporter.KeepBlackBoxExporter, err
	}
	for i := range routeMonitors.Items {
		objectsDependingOnExporter = append(objectsDependingOnExporter, &routeMonitors.Items[i])
	}

	clusterUrlMonitors := &v1alpha1.ClusterUrlMonitorList{}
	if err := b.Client.List(b.Ctx, clusterUrlMonitors); err != nil {
		return blackboxexporter.KeepBlackBoxExporter, err
	}
	for i := range clusterUrlMonitors.Items {
		objectsDependingOnExporter = append(objectsDependingOnExporter, &clusterUrlMonitors.Items[i])
	}
	b.Log.V(4).Info("Number of objects depending on BlackBoxExporter:", "amountOfObjects", len(objectsDependingOnExporter))

	if len(objectsDependingOnExporter) == 1 && finalizer.WasDeleteRequested(objectsDependingOnExporter[0]) {
		b.Log.V(3).Info("Deleting BlackBoxResources: decided to clean BlackBoxExporter resources")
		return blackboxexporter.DeleteBlackBoxExporter, nil
	}
	return blackboxexporter.KeepBlackBoxExporter, nil
}

func (b *BlackBoxExporter) EnsureBlackBoxExporterDeploymentExists() error {
	resource := appsv1.Deployment{}
	template := b.templateForBlackBoxExporterDeployment(b.Image, b.NamespacedName)

	// Does the resource already exist?
	err := b.Client.Get(b.Ctx, b.NamespacedName, &resource)
	if err != nil {
		// If this is an unknown error
		if !k8serrors.IsNotFound(err) {
			// return unexpectedly
			return err
		}
		// and create it
		err = b.Client.Create(b.Ctx, &template)
		if err != nil {
			return err
		}

		return nil
	}

	// Update the deployment if it's different than the template
	if !reflect.DeepEqual(resource.Spec, template.Spec) {
		resource.ObjectMeta.ResourceVersion = ""
		resource.Spec = template.Spec
		err = b.Client.Update(b.Ctx, &resource)
		if err != nil {
			return err
		}
	}

	return nil
}

func (b *BlackBoxExporter) EnsureBlackBoxExporterServiceExists() error {
	resource := corev1.Service{}
	populationFunc := func() corev1.Service { return templateForBlackBoxExporterService(b.NamespacedName) }

	// Does the resource already exist?
	if err := b.Client.Get(b.Ctx, b.NamespacedName, &resource); err != nil {
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

func (b *BlackBoxExporter) EnsureBlackBoxExporterConfigMapExists() error {
	resource := corev1.ConfigMap{}
	populationFunc := func() corev1.ConfigMap { return templateForBlackBoxExporterConfigMap(b.NamespacedName) }

	// Does the resource already exist?
	if err := b.Client.Get(b.Ctx, b.NamespacedName, &resource); err != nil {
		// If this is an unknown error
		if !k8serrors.IsNotFound(err) {
			// return unexpectedly
			return err
		}
		// populate the resource with the template
		resource := populationFunc()
		// and create it
		return b.Client.Create(b.Ctx, &resource)
	}
	return nil
}

// deploymentForBlackBoxExporter returns a blackbox deployment
func (b *BlackBoxExporter) templateForBlackBoxExporterDeployment(blackBoxImage string, blackBoxNamespacedName types.NamespacedName) appsv1.Deployment {
	nodeLabel := "node-role.kubernetes.io/infra"
	if util.IsClusterVersionHigherOrEqualThan(b.Client, "4.13") && util.ClusterHasPrivateNLB(b.Client) {
		nodeLabel = "node-role.kubernetes.io/master"
	}

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
										Key:      nodeLabel,
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
						Key:      nodeLabel,
					}},
					Containers: []corev1.Container{{
						Image: blackBoxImage,
						Name:  "blackbox-exporter",
						Args: []string{
							"--config.file=/config/blackbox.yaml",
						},
						Ports: []corev1.ContainerPort{{
							ContainerPort: blackboxexporter.BlackBoxExporterPortNumber,
							Name:          blackboxexporter.BlackBoxExporterPortName,
						}},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      "blackbox-config",
								ReadOnly:  true,
								MountPath: "/config",
							},
						},
					}},
					Volumes: []corev1.Volume{
						{
							Name: "blackbox-config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: blackBoxNamespacedName.Name,
									},
								},
							},
						},
					},
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

func templateForBlackBoxExporterConfigMap(blackboxNamespacedName types.NamespacedName) corev1.ConfigMap {
	labels := blackboxexporter.GenerateBlackBoxExporterLables()

	cfg := `modules:
  http_2xx:
    prober: http
    timeout: 15s
  insecure_http_2xx:
    prober: http
    timeout: 15s
    http:
      tls_config:
        insecure_skip_verify: true`

	cm := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      blackboxNamespacedName.Name,
			Namespace: blackboxNamespacedName.Namespace,
			Labels:    labels,
		},
		Data: map[string]string{
			"blackbox.yaml": cfg,
		},
	}
	return cm
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

func (b *BlackBoxExporter) EnsureBlackBoxExporterConfigMapAbsent() error {
	resource := &corev1.ConfigMap{}

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
	b.Log.V(2).Info("Entering EnsureBlackBoxExporterConfigMapAbsent")
	if err := b.EnsureBlackBoxExporterConfigMapAbsent(); err != nil {
		return err
	}
	return nil
}

func (b *BlackBoxExporter) EnsureBlackBoxExporterResourcesExist() error {
	if err := b.EnsureBlackBoxExporterConfigMapExists(); err != nil {
		return err
	}
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
