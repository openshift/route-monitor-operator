/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package blackboxexporter

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/go-logr/logr"
	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	consts "github.com/openshift/route-monitor-operator/pkg/consts/blackboxexporter"
	"github.com/openshift/route-monitor-operator/pkg/util/errors"
	"github.com/openshift/route-monitor-operator/pkg/util/finalizer"
	utilreconcile "github.com/openshift/route-monitor-operator/pkg/util/reconcile"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
)

// BlackBoxExporterReconciler reconciles a BlackBoxExporter object
type BlackBoxExporterReconciler struct {
	Client           client.Client
	Ctx              context.Context
	Log              logr.Logger
	Scheme           *runtime.Scheme
	BlackBoxExporter v1alpha1.BlackBoxExporter
}

// NewReconciler returns a new BlackBoxExporterReconciler object
func NewReconciler(mgr manager.Manager) *BlackBoxExporterReconciler {
	log := ctrl.Log.WithName("controllers").WithName("BlackBoxExporter")
	client := mgr.GetClient()
	ctx := context.Background()
	return &BlackBoxExporterReconciler{
		Client:           client,
		Ctx:              ctx,
		Log:              log,
		Scheme:           mgr.GetScheme(),
		BlackBoxExporter: v1alpha1.BlackBoxExporter{},
	}
}

// +kubebuilder:rbac:groups=monitoring.openshift.io,resources=blackboxexporters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.openshift.io,resources=blackboxexporters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=monitoring.openshift.io,resources=blackboxexporters/finalizers,verbs=update
// +kubebuilder:rbac:groups=*,resources=services,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;delete
func (r *BlackBoxExporterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Ctx = ctx
	log := r.Log.WithName("Reconcile").WithValues("name", req.Name, "namespace", req.Namespace)

	log.V(2).Info("Entering GetBlackBoxExporter")
	blackBoxExporter, res, err := r.GetBlackBoxExporter(req)
	if err != nil {
		log.Error(err, "Failed to retrieve BlackBoxExporter. Requeueing...")
		return utilreconcile.RequeueWith(err)
	}
	if res.ShouldStop() {
		return utilreconcile.Stop()
	}
	r.BlackBoxExporter = blackBoxExporter

	// Examine DeletionTimestamp to determine if the object is under deletion
	if blackBoxExporter.ObjectMeta.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(&blackBoxExporter, consts.BlackBoxExporterFinalizerKey) {
			log.Info("Adding Finalizer")
			controllerutil.AddFinalizer(&blackBoxExporter, consts.BlackBoxExporterFinalizerKey)
			if err := r.Client.Update(r.Ctx, &blackBoxExporter); err != nil {
				log.Error(err, "Failed reconcile while adding Finalizer. Requeueing...")
			}
		}
	} else {
		err = r.DeleteResources(blackBoxExporter)
		if err != nil {
			log.Error(err, "Requeueing...")
			return utilreconcile.RequeueWith(err)
		}

		log.Info("Successfully deleted BlackBoxExporter")

		return utilreconcile.Stop()
	}

	err = r.CreateResources(blackBoxExporter)
	if err == errors.ImageFieldUndefined {
		return utilreconcile.Stop()
	}
	if err != nil {
		return utilreconcile.RequeueWith(err)
	}

	log.Info("All operations for BlackBoxExporter completed. Finished Reconcile.")
	return utilreconcile.Stop()
}

// SetupWithManager sets up the controller with the Manager.
func (r *BlackBoxExporterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.BlackBoxExporter{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}

// GetBlackBoxExporter returns the BlackBoxExporter object
func (r *BlackBoxExporterReconciler) GetBlackBoxExporter(req ctrl.Request) (v1alpha1.BlackBoxExporter, utilreconcile.Result, error) {
	blackBoxExporter := v1alpha1.BlackBoxExporter{}
	err := r.Client.Get(r.Ctx, req.NamespacedName, &blackBoxExporter)
	if err != nil {
		// If this is an unknown error
		if !k8serrors.IsNotFound(err) {
			res, err := utilreconcile.RequeueReconcileWith(err)
			return v1alpha1.BlackBoxExporter{}, res, err
		}
		r.Log.V(2).Info("StopRequeue", "As BlackBoxExporter is 'NotFound', stopping requeue", nil)
		return v1alpha1.BlackBoxExporter{}, utilreconcile.StopOperation(), nil
	}

	// if the resource is empty, we should terminate
	emptyBlackBoxExporter := v1alpha1.BlackBoxExporter{}
	if reflect.DeepEqual(blackBoxExporter, emptyBlackBoxExporter) {
		return v1alpha1.BlackBoxExporter{}, utilreconcile.StopOperation(), nil
	}

	return blackBoxExporter, utilreconcile.ContinueOperation(), nil
}

// CreateResources creates or updates the Deployment and Service objects
func (r *BlackBoxExporterReconciler) CreateResources(blackBoxExporter v1alpha1.BlackBoxExporter) error {
	namespacedName := types.NamespacedName{
		Name:      blackBoxExporter.Name,
		Namespace: blackBoxExporter.Namespace,
	}

	deploy, err := r.DeploymentTemplate(namespacedName, blackBoxExporter)
	if err != nil {
		return fmt.Errorf("Failed to reconcile BlackBoxExporter Deployment: %w", err)
	}
	_, err = ctrl.CreateOrUpdate(context.Background(), r.Client, &deploy, func() error {
		template, err := r.DeploymentTemplate(namespacedName, blackBoxExporter)
		if err != nil {
			return fmt.Errorf("Failed to update BlackBoxExporter Deployment: %w", err)
		}
		deploy.ObjectMeta.ResourceVersion = "" // Remove ResourceVersion to avoid 'apply' related errors
		deploy.Spec = template.Spec
		return controllerutil.SetControllerReference(&blackBoxExporter, &deploy, r.Scheme)
	})

	if err != nil {
		return fmt.Errorf("Failed to reconcile BlackBoxExporter Deployment: %w", err)
	}

	service, nil := r.ServiceTemplate(namespacedName)
	_, err = ctrl.CreateOrUpdate(context.Background(), r.Client, &service, func() error {
		template, err := r.ServiceTemplate(namespacedName)
		if err != nil {
			return fmt.Errorf("Failed to update BlackBoxExporter Service: %w", err)
		}
		service.ObjectMeta.ResourceVersion = "" // Remove ResourceVersion to avoid 'apply' related errors
		service.Spec = template.Spec
		return controllerutil.SetControllerReference(&blackBoxExporter, &deploy, r.Scheme)
	})

	if err != nil {
		return fmt.Errorf("Failed to reconcile BlackBoxExporter Service: %w", err)
	}

	return nil
}

// DeleteResources deletes the Deployment and Service objects
func (r *BlackBoxExporterReconciler) DeleteResources(blackBoxExporter v1alpha1.BlackBoxExporter) error {
	namespacedName := types.NamespacedName{
		Name:      blackBoxExporter.Name,
		Namespace: blackBoxExporter.Namespace,
	}
	deploy := &appsv1.Deployment{}
	err := r.Client.Get(r.Ctx, namespacedName, deploy)
	if err == nil {
		if err := r.Client.Delete(r.Ctx, deploy); err != nil {
			return fmt.Errorf("Failed to delete BlackBoxExporter Deployment: %w", err)
		}
	}

	service := &corev1.Service{}
	err = r.Client.Get(r.Ctx, namespacedName, service)
	if err == nil {
		if err := r.Client.Delete(r.Ctx, service); err != nil {
			return fmt.Errorf("Failed to delete BlackBoxExporter Deployment: %w", err)
		}
	}

	finalizer.Remove(&blackBoxExporter, consts.BlackBoxExporterFinalizerKey)
	if err := r.Client.Update(r.Ctx, &blackBoxExporter); err != nil {
		return fmt.Errorf("Failed reconcile while removing Finalizer: %w", err)
	}

	return nil
}

// DeploymentTemplate return a blackbox deployment
func (r *BlackBoxExporterReconciler) DeploymentTemplate(namespacedName types.NamespacedName, blackBoxExporter v1alpha1.BlackBoxExporter) (appsv1.Deployment, error) {
	if blackBoxExporter.Spec.Image == "" {
		return appsv1.Deployment{}, errors.ImageFieldUndefined
	}

	labels := consts.GenerateBlackBoxExporterLables()
	labelSelectors := metav1.LabelSelector{
		MatchLabels: labels,
	}

	// hardcode the replicasize for no
	// replicas := m.Spec.Size
	var replicas int32 = 1

	dep := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespacedName.Name,
			Namespace: namespacedName.Namespace,
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
						Image: blackBoxExporter.Spec.Image,
						Name:  consts.BlackBoxExporterName,
						Ports: []corev1.ContainerPort{{
							ContainerPort: consts.BlackBoxExporterPortNumber,
							Name:          consts.BlackBoxExporterPortName,
						}},
					}},
				},
			},
		},
	}

	if len(blackBoxExporter.Spec.NodeSelector.NodeSelectorTerms) != 0 {
		var schedulingTerms []corev1.PreferredSchedulingTerm
		for _, term := range blackBoxExporter.Spec.NodeSelector.NodeSelectorTerms {
			preferredTerm := corev1.PreferredSchedulingTerm{
				Weight:     1,
				Preference: term,
			}
			schedulingTerms = append(schedulingTerms, preferredTerm)
		}

		var tolerations []corev1.Toleration
		for _, term := range blackBoxExporter.Spec.NodeSelector.NodeSelectorTerms {
			key := term.MatchExpressions[0].Key
			if strings.Contains(key, "node-role.kubernetes.io") {
				tolerations = append(tolerations, corev1.Toleration{
					Operator: corev1.TolerationOpExists,
					Effect:   corev1.TaintEffectNoSchedule,
					Key:      key,
				})
			}
		}

		dep.Spec.Template.Spec.Affinity = &corev1.Affinity{
			NodeAffinity: &corev1.NodeAffinity{
				PreferredDuringSchedulingIgnoredDuringExecution: schedulingTerms,
			},
		}

		dep.Spec.Template.Spec.Tolerations = tolerations
	}

	return dep, nil
}

// ServiceTemplate returns a blackbox service
func (r *BlackBoxExporterReconciler) ServiceTemplate(namespacedName types.NamespacedName) (corev1.Service, error) {
	labels := consts.GenerateBlackBoxExporterLables()

	svc := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespacedName.Name,
			Namespace: namespacedName.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{{
				TargetPort: intstr.FromString(consts.BlackBoxExporterPortName),
				Port:       consts.BlackBoxExporterPortNumber,
				Name:       consts.BlackBoxExporterPortName,
			}},
		},
	}
	return svc, nil
}
