package clusterurlmonitor

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	interfaces "github.com/openshift/route-monitor-operator/controllers"
	"github.com/openshift/route-monitor-operator/pkg/alert"
	"github.com/openshift/route-monitor-operator/pkg/blackboxexporter"
	blackboxexporterconsts "github.com/openshift/route-monitor-operator/pkg/consts/blackboxexporter"
	reconcileCommon "github.com/openshift/route-monitor-operator/pkg/reconcileCommon"
	"github.com/openshift/route-monitor-operator/pkg/servicemonitor"
	utilreconcile "github.com/openshift/route-monitor-operator/pkg/util/reconcile"
	"github.com/openshift/route-monitor-operator/pkg/util/templates"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ClusterUrlMonitorSupplement struct {
	ClusterUrlMonitor v1alpha1.ClusterUrlMonitor
	Client            client.Client
	Log               logr.Logger
	Ctx               context.Context
	BlackBoxExporter  interfaces.BlackBoxExporter
	ClusterID         string
	common            interfaces.MonitorReconcileCommon
	serviceMonitor    interfaces.ServiceMonitor
	prom              interfaces.PrometheusRule
}

func NewSupplement(ClusterUrlMonitor v1alpha1.ClusterUrlMonitor, Client client.Client, log logr.Logger, blackBoxExporter *blackboxexporter.BlackBoxExporterHandler, ClusterID string) *ClusterUrlMonitorSupplement {
	return &ClusterUrlMonitorSupplement{
		ClusterUrlMonitor: ClusterUrlMonitor,
		Client:            Client,
		Log:               log,
		Ctx:               context.Background(),
		BlackBoxExporter:  blackBoxExporter,
		ClusterID:         ClusterID,
		common:            &reconcileCommon.MonitorReconcileCommon{},
		serviceMonitor:    &servicemonitor.ServiceMonitor{},
		prom:              &alert.PrometheusRule{},
	}
}

// Takes care that right PrometheusRules for the defined ClusterURLMonitor are in place
func (s *ClusterUrlMonitorSupplement) EnsurePrometheusRuleExists() (utilreconcile.Result, error) {
	clusterDomain, err := s.common.GetClusterDomain(s.Ctx, s.Client)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}
	spec := s.ClusterUrlMonitor.Spec
	clusterUrl := spec.Prefix + clusterDomain + ":" + spec.Port + spec.Suffix
	shouldHave, err, parsedSlo := s.common.AreMonitorSettingsValid(clusterUrl, s.ClusterUrlMonitor.Spec.Slo)

	if s.common.SetErrorStatus(&s.ClusterUrlMonitor.Status.ErrorStatus, err) {
		return s.common.UpdateReconciledMonitor(s.Ctx, s.Client, &s.ClusterUrlMonitor)
	}

	if !shouldHave {
		err = s.serviceMonitor.DeleteServiceMonitorDeployment(s.Ctx, s.Client, s.ClusterUrlMonitor.Status.ServiceMonitorRef)
		if err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
		updated, _ := s.common.SetResourceReference(&s.ClusterUrlMonitor.Status.PrometheusRuleRef, types.NamespacedName{})
		if updated {
			return s.common.UpdateReconciledMonitor(s.Ctx, s.Client, &s.ClusterUrlMonitor)
		}
		return utilreconcile.StopReconcile()
	}

	namespacedName := types.NamespacedName{Namespace: s.ClusterUrlMonitor.Namespace, Name: s.ClusterUrlMonitor.Name}
	template := templates.TemplateForPrometheusRuleResource(clusterUrl, parsedSlo, namespacedName, s.ClusterUrlMonitor.Kind)
	err = s.prom.UpdatePrometheusRuleDeployment(s.Ctx, s.Client, template)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}

	// Update PrometheusRuleReference in ClusterUrlMonitor if necessary
	updated, err := s.common.SetResourceReference(&s.ClusterUrlMonitor.Status.PrometheusRuleRef, namespacedName)
	if updated {
		return s.common.UpdateReconciledMonitor(s.Ctx, s.Client, &s.ClusterUrlMonitor)
	}
	return utilreconcile.ContinueReconcile()
}

// Takes care that right ServiceMonitor for the defined ClusterURLMonitor are in place
func (s *ClusterUrlMonitorSupplement) EnsureServiceMonitorExists() (utilreconcile.Result, error) {
	clusterDomain, err := s.common.GetClusterDomain(s.Ctx, s.Client)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}

	namespacedName := types.NamespacedName{Name: s.ClusterUrlMonitor.Name, Namespace: s.ClusterUrlMonitor.Namespace}
	spec := s.ClusterUrlMonitor.Spec
	clusterUrl := spec.Prefix + clusterDomain + ":" + spec.Port + spec.Suffix
	serviceMonitorTemplate := templates.TemplateForServiceMonitorResource(clusterUrl, s.BlackBoxExporter.GetBlackBoxExporterNamespace(), namespacedName, s.ClusterID)
	err = s.serviceMonitor.UpdateServiceMonitorDeployment(s.Ctx, s.Client, serviceMonitorTemplate)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}

	//Update RouteMonitor serviceMonitorRef if required
	updated, err := s.common.SetResourceReference(&s.ClusterUrlMonitor.Status.ServiceMonitorRef, namespacedName)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}
	if updated {
		return s.common.UpdateReconciledMonitor(s.Ctx, s.Client, &s.ClusterUrlMonitor)
	}
	return utilreconcile.ContinueReconcile()
}

// Ensures that all dependencies related to a ClusterUrlMonitor are deleted
func (s *ClusterUrlMonitorSupplement) EnsureDeletionProcessed() (utilreconcile.Result, error) {
	if s.ClusterUrlMonitor.DeletionTimestamp == nil {
		return utilreconcile.ContinueReconcile()
	}

	namespacedName := types.NamespacedName{Name: s.ClusterUrlMonitor.Name, Namespace: s.ClusterUrlMonitor.Namespace}
	serviceMonitor, err := s.serviceMonitor.GetServiceMonitor(s.Ctx, s.Client, namespacedName)
	if err != nil && !k8serrors.IsNotFound(err) {
		return utilreconcile.RequeueReconcileWith(err)
	}

	if err == nil {
		err = s.Client.Delete(s.Ctx, &serviceMonitor)
		if err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
	}

	shouldDelete, err := s.BlackBoxExporter.ShouldDeleteBlackBoxExporterResources()
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}
	if shouldDelete == blackboxexporterconsts.DeleteBlackBoxExporter {
		err := s.BlackBoxExporter.EnsureBlackBoxExporterResourcesAbsent()
		if err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
	}

	err = s.prom.DeletePrometheusRuleDeployment(s.Ctx, s.Client, s.ClusterUrlMonitor.Status.PrometheusRuleRef)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}

	if s.common.DeleteFinalizer(&s.ClusterUrlMonitor, FinalizerKey) {
		return s.common.UpdateReconciledMonitor(s.Ctx, s.Client, &s.ClusterUrlMonitor)
	}

	return utilreconcile.ContinueReconcile()
}

// Processes a Request
func ProcessRequest(blackboxExporter *blackboxexporter.BlackBoxExporterHandler, sup *ClusterUrlMonitorSupplement) (ctrl.Result, error) {
	res, err := sup.EnsureDeletionProcessed()
	if err != nil {
		return utilreconcile.RequeueWith(err)
	}
	if res.ShouldStop() {
		return utilreconcile.Stop()
	}

	if sup.common.SetFinalizer(&sup.ClusterUrlMonitor, FinalizerKey) {
		result, err := sup.common.UpdateReconciledMonitor(sup.Ctx, sup.Client, &sup.ClusterUrlMonitor)
		return result.Convert(), err
	}

	err = blackboxExporter.EnsureBlackBoxExporterResourcesExist()
	if err != nil {
		return utilreconcile.RequeueWith(err)
	}

	res, err = sup.EnsureServiceMonitorExists()
	if err != nil {
		return utilreconcile.RequeueWith(err)
	}

	res, err = sup.EnsurePrometheusRuleExists()
	if err != nil {
		return utilreconcile.RequeueWith(err)
	}

	return utilreconcile.Stop()
}

