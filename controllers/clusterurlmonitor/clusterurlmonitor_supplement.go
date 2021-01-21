package clusterurlmonitor

import (
	"context"

	"github.com/go-logr/logr"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	"github.com/openshift/route-monitor-operator/pkg/blackboxexporter"
	"github.com/openshift/route-monitor-operator/pkg/consts/blackbox"
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

type BlackboxExporter interface {
	EnsureBlackBoxExporterResourcesExist() error
	EnsureBlackBoxExporterResourcesAbsent() error
	ShouldDeleteBlackBoxExporterResources() (blackbox.ShouldDeleteBlackBoxExporter, error)
}

type ClusterUrlMonitorSupplement struct {
	ClusterUrlMonitor v1alpha1.ClusterUrlMonitor
	Client            client.Client
	Log               logr.Logger
	Ctx               context.Context
	BlackboxExporter  BlackboxExporter
}

func NewSupplement(ClusterUrlMonitor v1alpha1.ClusterUrlMonitor, Client client.Client, log logr.Logger, BlackboxExporter *blackboxexporter.BlackboxExporter) *ClusterUrlMonitorSupplement {
	return &ClusterUrlMonitorSupplement{ClusterUrlMonitor, Client, log, context.Background(), BlackboxExporter}
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
	serviceMonitor := templates.TemplateForServiceMonitorResource(clusterUrl, namespacedName)
	err = s.Client.Create(s.Ctx, &serviceMonitor)
	if err != nil {
		return err
	}

	s.ClusterUrlMonitor.Status.ServiceMonitorRef.Namespace = namespacedName.Namespace
	s.ClusterUrlMonitor.Status.ServiceMonitorRef.Name = namespacedName.Name
	err = s.Client.Status().Update(s.Ctx, &s.ClusterUrlMonitor)
	return err
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

	shouldDelete, err := s.BlackboxExporter.ShouldDeleteBlackBoxExporterResources()
	if err != nil {
		return reconcile.RequeueReconcileWith(err)
	}
	if shouldDelete == blackbox.DeleteBlackBoxExporter {
		err := s.BlackboxExporter.EnsureBlackBoxExporterResourcesAbsent()
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

func ProcessRequest(blackboxExporter *blackboxexporter.BlackboxExporter, sup *ClusterUrlMonitorSupplement) (ctrl.Result, error) {
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

	return utilreconcile.Stop()
}
