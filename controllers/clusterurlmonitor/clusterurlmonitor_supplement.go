package clusterurlmonitor

import (
	"context"
	"reflect"

	"github.com/go-logr/logr"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	"github.com/openshift/route-monitor-operator/pkg/blackboxexporter"
	blackboxexporterconsts "github.com/openshift/route-monitor-operator/pkg/consts/blackboxexporter"
	customerrors "github.com/openshift/route-monitor-operator/pkg/util/errors"
	utilfinalizer "github.com/openshift/route-monitor-operator/pkg/util/finalizer"
	"github.com/openshift/route-monitor-operator/pkg/util/reconcile"
	utilreconcile "github.com/openshift/route-monitor-operator/pkg/util/reconcile"
	"github.com/openshift/route-monitor-operator/pkg/util/templates"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type BlackBoxExporter interface {
	EnsureBlackBoxExporterResourcesExist() error
	EnsureBlackBoxExporterResourcesAbsent() error
	ShouldDeleteBlackBoxExporterResources() (blackboxexporterconsts.ShouldDeleteBlackBoxExporter, error)
	GetBlackBoxExporterNamespace() string
}

type ResourceComparer interface {
	DeepEqual(x, y interface{}) bool
}

type ClusterUrlMonitorSupplement struct {
	ClusterUrlMonitor v1alpha1.ClusterUrlMonitor
	Client            client.Client
	Log               logr.Logger
	Ctx               context.Context
	BlackBoxExporter  BlackBoxExporter
	ResourceComparer  ResourceComparer
}

type ResourceComparerStruct struct{}

func (_ ResourceComparerStruct) DeepEqual(x, y interface{}) bool {
	return reflect.DeepEqual(x, y)
}

func DeepEqual(x, y interface{}) bool {
	return reflect.DeepEqual(x, y)
}

func NewSupplement(ClusterUrlMonitor v1alpha1.ClusterUrlMonitor, Client client.Client, log logr.Logger, blackBoxExporter *blackboxexporter.BlackBoxExporter, resourceComparer ResourceComparer) *ClusterUrlMonitorSupplement {
	return &ClusterUrlMonitorSupplement{ClusterUrlMonitor, Client, log, context.Background(), blackBoxExporter, resourceComparer}
}

func (s *ClusterUrlMonitorSupplement) EnsureFinalizer() (reconcile.Result, error) {
	if !utilfinalizer.Contains(s.ClusterUrlMonitor.GetFinalizers(), FinalizerKey) {
		utilfinalizer.Add(&s.ClusterUrlMonitor, FinalizerKey)
		err := s.Client.Update(s.Ctx, &s.ClusterUrlMonitor)
		return reconcile.RequeueReconcileWith(err)
	}
	return reconcile.ContinueReconcile()
}

func (s *ClusterUrlMonitorSupplement) EnsureServiceMonitorExists() error {

	namespacedName := types.NamespacedName{Name: s.ClusterUrlMonitor.Name, Namespace: s.ClusterUrlMonitor.Namespace}
	exists, err := s.doesServiceMonitorExist(namespacedName)
	if exists || err != nil {
		return err
	}

	clusterDomain, err := s.getClusterDomain()
	if err != nil {
		return err
	}

	spec := s.ClusterUrlMonitor.Spec
	clusterUrl := spec.Prefix + clusterDomain + ":" + spec.Port + spec.Suffix
	serviceMonitor := templates.TemplateForServiceMonitorResource(clusterUrl, s.BlackBoxExporter.GetBlackBoxExporterNamespace(), namespacedName)
	err = s.Client.Create(s.Ctx, &serviceMonitor)
	if err != nil {
		return err
	}

	s.ClusterUrlMonitor.Status.ServiceMonitorRef.Namespace = namespacedName.Namespace
	s.ClusterUrlMonitor.Status.ServiceMonitorRef.Name = namespacedName.Name
	err = s.Client.Status().Update(s.Ctx, &s.ClusterUrlMonitor)
	return err

}

func (s *ClusterUrlMonitorSupplement) EnsurePrometheusRuleResourceExists() (utilreconcile.Result, error) {
	shouldCreate, err, parsedSlo := s.shouldCreatePrometheusRule()

	res, err := s.updateErrorStatus(err)
	if err != nil || res.RequeueOrStop() {
		return res, err
	}

	if !shouldCreate {
		err = s.EnsurePrometheusRuleResourceAbsent()
		if err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
		return s.addPrometheusRuleRefToStatus(types.NamespacedName{})
	}

	clusterDomain, err := s.getClusterDomain()
	if err != nil {
		return reconcile.RequeueReconcileWith(err)
	}

	namespacedName := types.NamespacedName{Namespace: s.ClusterUrlMonitor.Namespace, Name: s.ClusterUrlMonitor.Name}
	spec := s.ClusterUrlMonitor.Spec
	clusterUrl := spec.Prefix + clusterDomain + ":" + spec.Port + spec.Suffix
	template := templates.TemplateForPrometheusRuleResource(clusterUrl, parsedSlo, namespacedName)

	err = s.createOrUpdatePrometheusRule(template)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}

	res, err = s.addPrometheusRuleRefToStatus(namespacedName)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}
	if res.ShouldStop() {
		return utilreconcile.StopReconcile()
	}

	return utilreconcile.ContinueReconcile()
}

func (s *ClusterUrlMonitorSupplement) createOrUpdatePrometheusRule(template monitoringv1.PrometheusRule) error {
	resource := monitoringv1.PrometheusRule{}
	namespacedName := types.NamespacedName{Name: s.ClusterUrlMonitor.Status.PrometheusRuleRef.Name,
		Namespace: s.ClusterUrlMonitor.Status.PrometheusRuleRef.Namespace}
	err := s.Client.Get(s.Ctx, namespacedName, &resource)

	if err != nil && !k8serrors.IsNotFound(err) {
		return err
	}

	if !k8serrors.IsNotFound(err) {
		if s.ResourceComparer.DeepEqual(template.Spec, resource.Spec) {
			return nil
		}
		// Update PrometheusRule if the specs are different
		resource.Spec = template.Spec
		return s.Client.Update(s.Ctx, &resource)
	}
	// Create the PrometheusRule if it does not exist
	return s.Client.Create(s.Ctx, &template)
}

func (s *ClusterUrlMonitorSupplement) shouldCreatePrometheusRule() (bool, error, string) {
	// Is the SloSpec configured on this CR?
	if s.ClusterUrlMonitor.Spec.Slo == *new(v1alpha1.SloSpec) {
		return false, nil, ""
	}
	isValid, parsedSlo := s.ClusterUrlMonitor.Spec.Slo.IsValid()
	if !isValid {
		return false, customerrors.InvalidSLO, ""
	}
	return true, nil, parsedSlo
}

func (s *ClusterUrlMonitorSupplement) addPrometheusRuleRefToStatus(namespacedName types.NamespacedName) (utilreconcile.Result, error) {
	desiredRef := v1alpha1.NamespacedName{
		Namespace: namespacedName.Namespace,
		Name:      namespacedName.Name,
	}
	if s.ClusterUrlMonitor.Status.PrometheusRuleRef != desiredRef {
		// Update status with PrometheusRuleRef
		s.ClusterUrlMonitor.Status.PrometheusRuleRef = desiredRef

		err := s.Client.Status().Update(s.Ctx, &s.ClusterUrlMonitor)
		if err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
		return utilreconcile.StopReconcile()

	}
	return utilreconcile.ContinueReconcile()
}

func (s *ClusterUrlMonitorSupplement) getClusterDomain() (string, error) {
	clusterConfig := configv1.Ingress{}
	err := s.Client.Get(s.Ctx, types.NamespacedName{Name: "cluster"}, &clusterConfig)
	if err != nil {
		return "", err
	}

	return clusterConfig.Spec.Domain, nil
}

func (s *ClusterUrlMonitorSupplement) EnsureDeletionProcessed() (reconcile.Result, error) {
	if s.ClusterUrlMonitor.DeletionTimestamp == nil {
		return reconcile.ContinueReconcile()
	}

	namespacedName := types.NamespacedName{Name: s.ClusterUrlMonitor.Name, Namespace: s.ClusterUrlMonitor.Namespace}
	serviceMonitor, err := s.getServiceMonitor(namespacedName)
	if err != nil && !k8serrors.IsNotFound(err) {
		return reconcile.RequeueReconcileWith(err)
	}

	if err == nil {
		err = s.Client.Delete(s.Ctx, &serviceMonitor)
		if err != nil {
			return reconcile.RequeueReconcileWith(err)
		}
	}

	shouldDelete, err := s.BlackBoxExporter.ShouldDeleteBlackBoxExporterResources()
	if err != nil {
		return reconcile.RequeueReconcileWith(err)
	}
	if shouldDelete == blackboxexporterconsts.DeleteBlackBoxExporter {
		err := s.BlackBoxExporter.EnsureBlackBoxExporterResourcesAbsent()
		if err != nil {
			return reconcile.RequeueReconcileWith(err)
		}
	}

	err = s.EnsurePrometheusRuleResourceAbsent()
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}

	if utilfinalizer.Contains(s.ClusterUrlMonitor.GetFinalizers(), FinalizerKey) {
		utilfinalizer.Remove(&s.ClusterUrlMonitor, FinalizerKey)
		err = s.Client.Update(s.Ctx, &s.ClusterUrlMonitor)
		return reconcile.RequeueReconcileWith(err)
	}
	return reconcile.ContinueReconcile()
}

func (s *ClusterUrlMonitorSupplement) EnsurePrometheusRuleResourceAbsent() error {
	// nothing to delete, stopping early
	if s.ClusterUrlMonitor.Status.PrometheusRuleRef == *new(v1alpha1.NamespacedName) {
		return nil
	}
	namespacedName := types.NamespacedName{Name: s.ClusterUrlMonitor.Status.PrometheusRuleRef.Name,
		Namespace: s.ClusterUrlMonitor.Status.PrometheusRuleRef.Namespace}
	resource := &monitoringv1.PrometheusRule{}
	// Does the resource already exist?
	err := s.Client.Get(s.Ctx, namespacedName, resource)
	if err != nil {
		// If this is an unknown error
		if !k8serrors.IsNotFound(err) {
			// return unexpectedly
			return err
		}
		// Resource doesn't exist, nothing to do
		return nil
	}
	err = s.Client.Delete(s.Ctx, resource)
	if err != nil {
		return err
	}
	return nil
}

func (s *ClusterUrlMonitorSupplement) getServiceMonitor(namespacedName types.NamespacedName) (monitoringv1.ServiceMonitor, error) {
	serviceMonitor := monitoringv1.ServiceMonitor{}
	err := s.Client.Get(s.Ctx, namespacedName, &serviceMonitor)
	return serviceMonitor, err
}

func (s *ClusterUrlMonitorSupplement) doesServiceMonitorExist(namespacedName types.NamespacedName) (bool, error) {
	_, err := s.getServiceMonitor(namespacedName)
	if k8serrors.IsNotFound(err) {
		return false, nil
	}
	return true, err
}

func (s *ClusterUrlMonitorSupplement) updateErrorStatus(err error) (utilreconcile.Result, error) {
	// If an error has already been flagged and still occurs
	if s.ClusterUrlMonitor.Status.ErrorStatus != "" && err != nil {
		// Skip as the resource should not be created
		return utilreconcile.ContinueReconcile()
	}

	// If the error was flagged but stopped firing
	if s.ClusterUrlMonitor.Status.ErrorStatus != "" && err == nil {
		// Clear the error and restart
		s.ClusterUrlMonitor.Status.ErrorStatus = ""
		err := s.Client.Status().Update(s.Ctx, &s.ClusterUrlMonitor)
		if err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
		return utilreconcile.StopReconcile()
	}

	// If the error was not flagged but has started firing
	if s.ClusterUrlMonitor.Status.ErrorStatus == "" && err != nil {
		// Raise the alert and restart
		s.ClusterUrlMonitor.Status.ErrorStatus = err.Error()
		err := s.Client.Status().Update(s.Ctx, &s.ClusterUrlMonitor)
		if err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
		return utilreconcile.StopReconcile()
	}

	// only case left is ErrorStatus == "" && err == nil
	// so there is no need to check if err != nil
	return utilreconcile.ContinueReconcile()
}

func ProcessRequest(blackboxExporter *blackboxexporter.BlackBoxExporter, sup *ClusterUrlMonitorSupplement) (ctrl.Result, error) {
	res, err := sup.EnsureDeletionProcessed()
	if err != nil {
		return utilreconcile.RequeueWith(err)
	}
	if res.ShouldStop() {
		return utilreconcile.Stop()
	}

	res, err = sup.EnsureFinalizer()
	if err != nil {
		return utilreconcile.RequeueWith(err)
	}
	if res.ShouldStop() {
		return utilreconcile.Stop()
	}

	err = blackboxExporter.EnsureBlackBoxExporterResourcesExist()
	if err != nil {
		return utilreconcile.RequeueWith(err)
	}

	err = sup.EnsureServiceMonitorExists()
	if err != nil {
		return utilreconcile.RequeueWith(err)
	}

	res, err = sup.EnsurePrometheusRuleResourceExists()
	if err != nil {
		return utilreconcile.RequeueWith(err)
	}

	return utilreconcile.Stop()
}
