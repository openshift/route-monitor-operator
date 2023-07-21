package clusterurlmonitor

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	configv1 "github.com/openshift/api/config/v1"
	hypershiftv1beta1 "github.com/openshift/hypershift/api/v1beta1"
	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	"github.com/openshift/route-monitor-operator/pkg/alert"
	blackboxexporterconsts "github.com/openshift/route-monitor-operator/pkg/consts/blackboxexporter"
	utilreconcile "github.com/openshift/route-monitor-operator/pkg/util/reconcile"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	hcpClusterAnnotation = "hypershift.openshift.io/cluster"
)

// Takes care that right PrometheusRules for the defined ClusterURLMonitor are in place
func (s *ClusterUrlMonitorReconciler) EnsurePrometheusRuleExists(ctx context.Context, clusterUrlMonitor v1alpha1.ClusterUrlMonitor) (utilreconcile.Result, error) {
	// We shouldn't create prometheusrules for HCP clusterUrlMonitors, since alerting is implemented in the upstream RHOBS tenant
	if clusterUrlMonitor.Spec.DomainRef == v1alpha1.ClusterDomainRefHCP {
		return utilreconcile.ContinueReconcile()
	}

	clusterDomain, err := s.GetClusterDomain(ctx, clusterUrlMonitor)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}

	spec := clusterUrlMonitor.Spec
	clusterUrl := spec.Prefix + clusterDomain + ":" + spec.Port + spec.Suffix
	parsedSlo, err := s.Common.ParseMonitorSLOSpecs(clusterUrl, clusterUrlMonitor.Spec.Slo)

	if s.Common.SetErrorStatus(&clusterUrlMonitor.Status.ErrorStatus, err) {
		return s.Common.UpdateMonitorResourceStatus(ctx, &clusterUrlMonitor)
	}
	if parsedSlo == "" {
		err = s.Prom.DeletePrometheusRuleDeployment(ctx, clusterUrlMonitor.Status.PrometheusRuleRef)
		if err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
		updated, _ := s.Common.SetResourceReference(&clusterUrlMonitor.Status.PrometheusRuleRef, types.NamespacedName{})
		if updated {
			return s.Common.UpdateMonitorResourceStatus(ctx, &clusterUrlMonitor)
		}
		return utilreconcile.StopReconcile()
	}

	namespacedName := types.NamespacedName{Namespace: clusterUrlMonitor.Namespace, Name: clusterUrlMonitor.Name}
	template := alert.TemplateForPrometheusRuleResource(clusterUrl, parsedSlo, namespacedName)
	err = s.Prom.UpdatePrometheusRuleDeployment(ctx, template)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}

	// Update PrometheusRuleReference in ClusterUrlMonitor if necessary
	updated, _ := s.Common.SetResourceReference(&clusterUrlMonitor.Status.PrometheusRuleRef, namespacedName)
	if updated {
		return s.Common.UpdateMonitorResourceStatus(ctx, &clusterUrlMonitor)
	}
	return utilreconcile.ContinueReconcile()
}
func isClusterVersionAvailable(hcp hypershiftv1beta1.HostedControlPlane) error {
	condition := meta.FindStatusCondition(hcp.Status.Conditions, string(hypershiftv1beta1.ClusterVersionAvailable))
	if condition == nil || condition.Status != metav1.ConditionTrue {
		return fmt.Errorf("The cluster API is not yet available")
	}
	return nil
}

// Takes care that right ServiceMonitor for the defined ClusterURLMonitor are in place
func (s *ClusterUrlMonitorReconciler) EnsureServiceMonitorExists(ctx context.Context, clusterUrlMonitor v1alpha1.ClusterUrlMonitor) (utilreconcile.Result, error) {
	clusterDomain, err := s.GetClusterDomain(ctx, clusterUrlMonitor)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}

	namespacedName := types.NamespacedName{Name: clusterUrlMonitor.Name, Namespace: clusterUrlMonitor.Namespace}
	spec := clusterUrlMonitor.Spec
	clusterUrl := spec.Prefix + clusterDomain + ":" + spec.Port + spec.Suffix
	isHCP := (clusterUrlMonitor.Spec.DomainRef == v1alpha1.ClusterDomainRefHCP)
	var id string
	if isHCP {
		var hcp hypershiftv1beta1.HostedControlPlane
		id, err = s.Common.GetHypershiftClusterID(ctx, clusterUrlMonitor.Namespace)
		if err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
		hcp, err = s.Common.GetHCP(ctx, clusterUrlMonitor.Namespace)
		if err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
		err = isClusterVersionAvailable(hcp)
		if err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
	} else {
		id, err = s.Common.GetOSDClusterID(ctx)
		if err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
	}

	if err := s.ServiceMonitor.TemplateAndUpdateServiceMonitorDeployment(ctx, clusterUrl, s.BlackBoxExporter.GetBlackBoxExporterNamespace(), namespacedName, id, isHCP); err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}

	// Update RouteMonitor ServiceMonitorRef if required
	updated, err := s.Common.SetResourceReference(&clusterUrlMonitor.Status.ServiceMonitorRef, namespacedName)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}
	if updated {
		return s.Common.UpdateMonitorResourceStatus(ctx, &clusterUrlMonitor)
	}
	return utilreconcile.ContinueReconcile()
}

// Ensures that all dependencies related to a ClusterUrlMonitor are deleted
func (s *ClusterUrlMonitorReconciler) EnsureMonitorAndDependenciesAbsent(ctx context.Context, clusterUrlMonitor v1alpha1.ClusterUrlMonitor) (utilreconcile.Result, error) {
	shouldDelete, err := s.BlackBoxExporter.ShouldDeleteBlackBoxExporterResources(ctx)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}
	if shouldDelete == blackboxexporterconsts.DeleteBlackBoxExporter {
		err := s.BlackBoxExporter.EnsureBlackBoxExporterResourcesAbsent(ctx)
		if err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
	}

	err = s.Prom.DeletePrometheusRuleDeployment(ctx, clusterUrlMonitor.Status.PrometheusRuleRef)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}

	if s.Common.DeleteFinalizer(&clusterUrlMonitor, FinalizerKey) {
		return s.Common.UpdateMonitorResource(ctx, &clusterUrlMonitor)
	}

	return utilreconcile.ContinueReconcile()
}

// GetClusterDomain returns the baseDomain for a cluster, using the correct method based on it's type
func (s *ClusterUrlMonitorReconciler) GetClusterDomain(ctx context.Context, monitor v1alpha1.ClusterUrlMonitor) (string, error) {
	if monitor.Spec.DomainRef == v1alpha1.ClusterDomainRefHCP {
		return s.getHypershiftClusterDomain(ctx, monitor)
	}
	return s.getInfraClusterDomain(ctx)
}

// getInfraClusterDomain returns a normal OSD/ROSA cluster's domain based on it's infrastructure object
func (s *ClusterUrlMonitorReconciler) getInfraClusterDomain(ctx context.Context) (string, error) {
	clusterInfra := configv1.Infrastructure{}
	err := s.Client.Get(ctx, types.NamespacedName{Name: "cluster"}, &clusterInfra)
	if err != nil {
		return "", err
	}
	return removeSubdomain("api", clusterInfra.Status.APIServerURL)
}

// getHypershiftClusterDomain returns a hypershift hosted cluster's domain based on it's hostedCluster object
func (s *ClusterUrlMonitorReconciler) getHypershiftClusterDomain(ctx context.Context, monitor v1alpha1.ClusterUrlMonitor) (string, error) {
	clusterHCP, err := s.Common.GetHCP(ctx, monitor.Namespace)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve HostedControlPlane for hosted cluster: %w", err)
	}

	clusterAnnotation := clusterHCP.Annotations[hcpClusterAnnotation]
	annotationTokens := strings.Split(clusterAnnotation, "/")
	if len(annotationTokens) != 2 {
		return "", fmt.Errorf("invalid annotation for HostedControlPlane '%s': expected <namespace>/<hostedcluster name>, got %s", clusterHCP.Name, clusterAnnotation)
	}

	// Retrieve hostedCluster using HCP annotation
	hostedCluster := hypershiftv1beta1.HostedCluster{}
	hcReq := types.NamespacedName{
		Namespace: annotationTokens[0],
		Name:      annotationTokens[1],
	}
	err = s.Client.Get(ctx, hcReq, &hostedCluster)
	if err != nil {
		return "", err
	}

	return removeSubdomain("rosa", hostedCluster.Spec.DNS.BaseDomain)
}

func removeSubdomain(subdomain, clusterURL string) (string, error) {
	// url.Parse requires a 'http://' or 'https://' prefix in order
	// to function properly
	if !strings.HasPrefix(clusterURL, "https://") && !strings.HasPrefix(clusterURL, "http://") {
		clusterURL = fmt.Sprintf("https://%s", clusterURL)
	}

	u, err := url.Parse(clusterURL)
	if err != nil {
		return "", err
	}

	// the hostname format is api.basename so cutting at the first '.' will give
	// us the base name
	before, baseName, _ := strings.Cut(u.Hostname(), ".")
	if before != subdomain {
		baseName = strings.Join([]string{before, baseName}, ".")
	}
	return baseName, nil
}
