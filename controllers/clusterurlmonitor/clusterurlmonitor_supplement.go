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

type ClusterUrlMonitorSupplement struct {
	ClusterUrlMonitor v1alpha1.ClusterUrlMonitor
	Client            client.Client
	Log               logr.Logger
	Ctx               context.Context
	BlackBoxExporter  BlackBoxExporter
}

func NewSupplement(ClusterUrlMonitor v1alpha1.ClusterUrlMonitor, Client client.Client, log logr.Logger, blackBoxExporter *blackboxexporter.BlackBoxExporter) *ClusterUrlMonitorSupplement {
	return &ClusterUrlMonitorSupplement{ClusterUrlMonitor, Client, log, context.Background(), blackBoxExporter}
}

func (s *ClusterUrlMonitorSupplement) EnsureFinalizer() (reconcile.Result, error) {
	if !utilfinalizer.Contains(s.ClusterUrlMonitor.GetFinalizers(), FinalizerKey) {
		utilfinalizer.Add(&s.ClusterUrlMonitor, FinalizerKey)
		err := s.Client.Update(s.Ctx, &s.ClusterUrlMonitor)
		return reconcile.RequeueReconcileWith(err)
	}
	return reconcile.ContinueReconcile()
}

func (s *ClusterUrlMonitorSupplement) EnsureServiceMonitorExists() (reconcile.Result, error) {

	namespacedName := types.NamespacedName{Name: s.ClusterUrlMonitor.Name, Namespace: s.ClusterUrlMonitor.Namespace}
	exists, err := s.doesServiceMonitorExist(namespacedName)
	if exists || err != nil {
		return reconcile.RequeueReconcileWith(err)
	}

	clusterDomain, err := s.getClusterDomain()
	if err != nil {
		return reconcile.RequeueReconcileWith(err)
	}

	spec := s.ClusterUrlMonitor.Spec
	clusterUrl := spec.Prefix + clusterDomain + ":" + spec.Port + spec.Suffix
	serviceMonitor := templates.TemplateForServiceMonitorResource(clusterUrl, s.BlackBoxExporter.GetBlackBoxExporterNamespace(), namespacedName)

	// Does the resource already exist?
	if err := s.Client.Get(s.Ctx, namespacedName, &serviceMonitor); err != nil {
		// If this is an unknown error
		if !k8serrors.IsNotFound(err) {
			// return unexpectedly
			return utilreconcile.RequeueReconcileWith(err)
		}
		// and create it
		err = s.Client.Create(s.Ctx, &serviceMonitor)
		if err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
	}

	//Update status with serviceMonitorRef
	desiredRef := v1alpha1.NamespacedName{
		Name:      namespacedName.Name,
		Namespace: namespacedName.Namespace,
	}
	emptyRef := v1alpha1.NamespacedName{}

	currentRef := s.ClusterUrlMonitor.Status.ServiceMonitorRef
	if currentRef != emptyRef && desiredRef != currentRef {
		return utilreconcile.RequeueReconcileWith(customerrors.InvalidReferenceUpdate)
	}

	if currentRef == emptyRef && desiredRef != emptyRef {
		s.ClusterUrlMonitor.Status.ServiceMonitorRef = desiredRef

		err := s.Client.Status().Update(s.Ctx, &s.ClusterUrlMonitor)
		if err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
		return utilreconcile.StopReconcile()

	}
	return utilreconcile.ContinueReconcile()
}

func (s *ClusterUrlMonitorSupplement) EnsurePrometheusRuleResourceExists() (utilreconcile.Result, error) {
	shouldCreate, err, parsedSlo := s.shouldCreatePrometheusRule()
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	} else if !shouldCreate {
		return utilreconcile.ContinueReconcile()
	}

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

	namespacedName := types.NamespacedName{Name: s.ClusterUrlMonitor.Name, Namespace: s.ClusterUrlMonitor.Namespace}
	spec := s.ClusterUrlMonitor.Spec
	clusterUrl := spec.Prefix + clusterDomain + ":" + spec.Port + spec.Suffix
	template := templates.TemplateForPrometheusRuleResource(clusterUrl, parsedSlo, namespacedName, s.ClusterUrlMonitor.Kind+"Url")

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
	resource := &monitoringv1.PrometheusRule{}
	err := s.Client.Get(s.Ctx, types.NamespacedName{Namespace: template.Namespace, Name: template.Name}, resource)
	if err != nil && !k8serrors.IsNotFound(err) {
		return err
	}

	if !k8serrors.IsNotFound(err) {
		if reflect.DeepEqual(template.Spec, resource.Spec) {
			return nil
		}
		// Update PrometheusRule if the specs are different
		resource.Spec = template.Spec
		return s.Client.Update(s.Ctx, resource)
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
	desiredPrometheusRuleRef := v1alpha1.NamespacedName{
		Namespace: namespacedName.Namespace,
		Name:      namespacedName.Name,
	}
	if s.ClusterUrlMonitor.Status.PrometheusRuleRef != desiredPrometheusRuleRef {
		// Update status with PrometheusRuleRef
		s.ClusterUrlMonitor.Status.PrometheusRuleRef = desiredPrometheusRuleRef
		err := s.Client.Status().Update(s.Ctx, &s.ClusterUrlMonitor)
		return utilreconcile.RequeueReconcileWith(err)
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

	err = sup.EnsurePrometheusRuleResourceAbsent()
	if err != nil {
		return utilreconcile.RequeueWith(err)
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

	res, err = sup.EnsurePrometheusRuleResourceExists()
	if err != nil {
		return utilreconcile.RequeueWith(err)
	}
	if res.ShouldStop() {
		return utilreconcile.Stop()
	}

	res, err = sup.EnsureServiceMonitorExists()
	if err != nil {
		return utilreconcile.RequeueWith(err)
	}
	if res.ShouldStop() {
		return utilreconcile.Stop()
	}

	return utilreconcile.Stop()
}
