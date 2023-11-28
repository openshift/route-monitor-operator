package clusterurlmonitor

import (
	"fmt"
	"net/url"
	"reflect"
	"strings"

	configv1 "github.com/openshift/api/config/v1"
	hypershiftv1beta1 "github.com/openshift/hypershift/api/v1beta1"
	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	"github.com/openshift/route-monitor-operator/pkg/alert"
	utilreconcile "github.com/openshift/route-monitor-operator/pkg/util/reconcile"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	hcpClusterAnnotation = "hypershift.openshift.io/cluster"
)

// Takes care that right PrometheusRules for the defined ClusterURLMonitor are in place
func (s *ClusterUrlMonitorReconciler) EnsurePrometheusRuleExists(clusterUrlMonitor v1alpha1.ClusterUrlMonitor) (utilreconcile.Result, error) {
	// If .spec.skipPrometheusRule is true, ensure that the PrometheusRule does NOT exist
	if clusterUrlMonitor.Spec.SkipPrometheusRule {
		// Cleanup any existing PrometheusRules and update the status
		if err := s.Prom.DeletePrometheusRuleDeployment(clusterUrlMonitor.Status.PrometheusRuleRef); err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
		updated, _ := s.Common.SetResourceReference(&clusterUrlMonitor.Status.PrometheusRuleRef, types.NamespacedName{})
		if updated {
			return s.Common.UpdateMonitorResourceStatus(&clusterUrlMonitor)
		}

		return utilreconcile.ContinueReconcile()
	}

	// We shouldn't create prometheusrules for HCP clusterUrlMonitors, since alerting is implemented in the upstream RHOBS tenant
	if clusterUrlMonitor.Spec.DomainRef == v1alpha1.ClusterDomainRefHCP {
		return utilreconcile.ContinueReconcile()
	}

	clusterDomain, err := s.GetClusterDomain(clusterUrlMonitor)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}

	spec := clusterUrlMonitor.Spec
	clusterUrl := spec.Prefix + clusterDomain + ":" + spec.Port + spec.Suffix
	parsedSlo, err := s.Common.ParseMonitorSLOSpecs(clusterUrl, clusterUrlMonitor.Spec.Slo)

	if s.Common.SetErrorStatus(&clusterUrlMonitor.Status.ErrorStatus, err) {
		return s.Common.UpdateMonitorResourceStatus(&clusterUrlMonitor)
	}
	if parsedSlo == "" {
		err = s.Prom.DeletePrometheusRuleDeployment(clusterUrlMonitor.Status.PrometheusRuleRef)
		if err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
		updated, _ := s.Common.SetResourceReference(&clusterUrlMonitor.Status.PrometheusRuleRef, types.NamespacedName{})
		if updated {
			return s.Common.UpdateMonitorResourceStatus(&clusterUrlMonitor)
		}
		return utilreconcile.StopReconcile()
	}

	namespacedName := types.NamespacedName{Namespace: clusterUrlMonitor.Namespace, Name: clusterUrlMonitor.Name}
	template := alert.TemplateForPrometheusRuleResource(clusterUrl, parsedSlo, namespacedName)
	err = s.Prom.UpdatePrometheusRuleDeployment(template)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}

	// Update PrometheusRuleReference in ClusterUrlMonitor if necessary
	updated, _ := s.Common.SetResourceReference(&clusterUrlMonitor.Status.PrometheusRuleRef, namespacedName)
	if updated {
		return s.Common.UpdateMonitorResourceStatus(&clusterUrlMonitor)
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
func (s *ClusterUrlMonitorReconciler) EnsureServiceMonitorExists(clusterUrlMonitor v1alpha1.ClusterUrlMonitor) (utilreconcile.Result, error) {
	clusterDomain, err := s.GetClusterDomain(clusterUrlMonitor)
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
		id, err = s.Common.GetHypershiftClusterID(clusterUrlMonitor.Namespace)
		if err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
		hcp, err = s.Common.GetHCP(clusterUrlMonitor.Namespace)
		if err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
		err = isClusterVersionAvailable(hcp)
		if err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
	} else {
		id, err = s.Common.GetOSDClusterID()
		if err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
	}

	if err := s.ServiceMonitor.TemplateAndUpdateServiceMonitorDeployment(clusterUrl, namespacedName, id, isHCP); err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}

	// Update RouteMonitor ServiceMonitorRef if required
	updated, err := s.Common.SetResourceReference(&clusterUrlMonitor.Status.ServiceMonitorRef, namespacedName)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}
	if updated {
		return s.Common.UpdateMonitorResourceStatus(&clusterUrlMonitor)
	}
	return utilreconcile.ContinueReconcile()
}

// Ensures that all dependencies related to a ClusterUrlMonitor are deleted
func (s *ClusterUrlMonitorReconciler) EnsureMonitorAndDependenciesAbsent(clusterUrlMonitor v1alpha1.ClusterUrlMonitor) (utilreconcile.Result, error) {
	if clusterUrlMonitor.DeletionTimestamp == nil {
		return utilreconcile.ContinueReconcile()
	}

	isHCP := (clusterUrlMonitor.Spec.DomainRef == v1alpha1.ClusterDomainRefHCP)
	err := s.ServiceMonitor.DeleteServiceMonitorDeployment(clusterUrlMonitor.Status.ServiceMonitorRef, isHCP)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}

	err = s.Prom.DeletePrometheusRuleDeployment(clusterUrlMonitor.Status.PrometheusRuleRef)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}

	if s.Common.DeleteFinalizer(&clusterUrlMonitor, FinalizerKey) {
		// ignore the output as we want to remove the PrevFinalizerKey anyways
		s.Common.DeleteFinalizer(&clusterUrlMonitor, PrevFinalizerKey)
		return s.Common.UpdateMonitorResource(&clusterUrlMonitor)
	}

	return utilreconcile.ContinueReconcile()
}

func (s *ClusterUrlMonitorReconciler) EnsureFinalizerSet(clusterUrlMonitor v1alpha1.ClusterUrlMonitor) (utilreconcile.Result, error) {
	if s.Common.SetFinalizer(&clusterUrlMonitor, FinalizerKey) {
		// ignore the output as we want to remove the PrevFinalizerKey anyways
		s.Common.DeleteFinalizer(&clusterUrlMonitor, PrevFinalizerKey)
		return s.Common.UpdateMonitorResource(&clusterUrlMonitor)
	}
	return utilreconcile.ContinueReconcile()
}

// GetClusterUrlMonitor return the ClusterUrlMonitor that is tested
func (s *ClusterUrlMonitorReconciler) GetClusterUrlMonitor(req ctrl.Request) (v1alpha1.ClusterUrlMonitor, utilreconcile.Result, error) {
	ClusterUrlMonitor := v1alpha1.ClusterUrlMonitor{}
	err := s.Client.Get(s.Ctx, req.NamespacedName, &ClusterUrlMonitor)
	if err != nil {
		// If this is an unknown error
		if !k8serrors.IsNotFound(err) {
			res, err := utilreconcile.RequeueReconcileWith(err)
			return v1alpha1.ClusterUrlMonitor{}, res, err
		}
		s.Log.V(2).Info("StopRequeue", "As ClusterUrlMonitor is 'NotFound', stopping requeue", nil)

		return v1alpha1.ClusterUrlMonitor{}, utilreconcile.StopOperation(), nil
	}

	// if the resource is empty, we should terminate
	emptyClustUrlMonitor := v1alpha1.ClusterUrlMonitor{}
	if reflect.DeepEqual(ClusterUrlMonitor, emptyClustUrlMonitor) {
		return v1alpha1.ClusterUrlMonitor{}, utilreconcile.StopOperation(), nil
	}

	return ClusterUrlMonitor, utilreconcile.ContinueOperation(), nil
}

// GetClusterDomain returns the baseDomain for a cluster, using the correct method based on it's type
func (s *ClusterUrlMonitorReconciler) GetClusterDomain(monitor v1alpha1.ClusterUrlMonitor) (string, error) {
	if monitor.Spec.DomainRef == v1alpha1.ClusterDomainRefHCP {
		return s.getHypershiftClusterDomain(monitor)
	}
	return s.getInfraClusterDomain()
}

// getInfraClusterDomain returns a normal OSD/ROSA cluster's domain based on it's infrastructure object
func (s *ClusterUrlMonitorReconciler) getInfraClusterDomain() (string, error) {
	clusterInfra := configv1.Infrastructure{}
	err := s.Client.Get(s.Ctx, types.NamespacedName{Name: "cluster"}, &clusterInfra)
	if err != nil {
		return "", err
	}
	return removeSubdomain("api", clusterInfra.Status.APIServerURL)
}

// getHypershiftClusterDomain returns a hypershift hosted cluster's domain based on it's hostedCluster object
func (s *ClusterUrlMonitorReconciler) getHypershiftClusterDomain(monitor v1alpha1.ClusterUrlMonitor) (string, error) {
	clusterHCP, err := s.Common.GetHCP(monitor.Namespace)
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
	err = s.Client.Get(s.Ctx, hcReq, &hostedCluster)
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
