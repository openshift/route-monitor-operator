package routemonitor

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	routev1 "github.com/openshift/api/route/v1"
	"github.com/openshift/route-monitor-operator/api/v1alpha1"

	"github.com/openshift/route-monitor-operator/pkg/alert"
	"github.com/openshift/route-monitor-operator/pkg/consts"
	customerrors "github.com/openshift/route-monitor-operator/pkg/util/errors"
	utilreconcile "github.com/openshift/route-monitor-operator/pkg/util/reconcile"

	hypershiftv1beta1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Ensures that all PrometheusRules CR are created according to the RouteMonitor
func (r *RouteMonitorReconciler) EnsurePrometheusRuleExists(routeMonitor v1alpha1.RouteMonitor) (utilreconcile.Result, error) {
	// If .spec.skipPrometheusRule is true, ensure that the PrometheusRule does NOT exist
	if routeMonitor.Spec.SkipPrometheusRule {
		// Cleanup any existing PrometheusRules and update the status
		if err := r.Prom.DeletePrometheusRuleDeployment(routeMonitor.Status.PrometheusRuleRef); err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
		updated, _ := r.Common.SetResourceReference(&routeMonitor.Status.PrometheusRuleRef, types.NamespacedName{})
		if updated {
			return r.Common.UpdateMonitorResourceStatus(&routeMonitor)
		}

		return utilreconcile.ContinueReconcile()
	}

	parsedSlo, err := r.Common.ParseMonitorSLOSpecs(routeMonitor.Status.RouteURL, routeMonitor.Spec.Slo)
	if r.Common.SetErrorStatus(&routeMonitor.Status.ErrorStatus, err) {
		return r.Common.UpdateMonitorResourceStatus(&routeMonitor)
	}
	if parsedSlo == "" {
		// Delete existing PrometheusRules if required
		err = r.Prom.DeletePrometheusRuleDeployment(routeMonitor.Status.PrometheusRuleRef)
		if err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
		updated, _ := r.Common.SetResourceReference(&routeMonitor.Status.PrometheusRuleRef, types.NamespacedName{})
		if updated {
			return r.Common.UpdateMonitorResourceStatus(&routeMonitor)
		}
		return utilreconcile.StopReconcile()
	}

	// Update PrometheusRule from templates
	namespacedName := types.NamespacedName{Namespace: routeMonitor.Namespace, Name: routeMonitor.Name}
	template := alert.TemplateForPrometheusRuleResource(routeMonitor.Status.RouteURL, parsedSlo, namespacedName)
	err = r.Prom.UpdatePrometheusRuleDeployment(template)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}

	// Update PrometheusRuleReference in RouteMonitor if necessary
	updated, _ := r.Common.SetResourceReference(&routeMonitor.Status.PrometheusRuleRef, namespacedName)
	if updated {
		return r.Common.UpdateMonitorResourceStatus(&routeMonitor)
	}
	return utilreconcile.ContinueReconcile()
}

// Ensures that a ServiceMonitor is created from the RouteMonitor CR
func (r *RouteMonitorReconciler) EnsureServiceMonitorExists(routeMonitor v1alpha1.RouteMonitor) (utilreconcile.Result, error) {
	// Was the RouteURL populated by a previous step?
	if routeMonitor.Status.RouteURL == "" {
		return utilreconcile.RequeueReconcileWith(customerrors.NoHost)
	}

	var id string
	var err error
	useRHOBS := (routeMonitor.Spec.ServiceMonitorType == v1alpha1.ServiceMonitorTypeRHOBS)

	if useRHOBS {
		hcp, err := r.getHostedControlPlane(routeMonitor.Namespace)
		if err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
		id = hcp.Spec.ClusterID
	} else {
		id, err = r.Common.GetOSDClusterID()
		if err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
	}

	// update ServiceMonitor if requiredctrl
	namespacedName := types.NamespacedName{Name: routeMonitor.Name, Namespace: routeMonitor.Namespace}
	owner := metav1.NewControllerRef(&routeMonitor.ObjectMeta, routeMonitor.GroupVersionKind())
	if err := r.ServiceMonitor.TemplateAndUpdateServiceMonitorDeployment(routeMonitor.Status.RouteURL, r.BlackBoxExporter.GetBlackBoxExporterNamespace(), namespacedName, id, useRHOBS, routeMonitor.Spec.InsecureSkipTLSVerify, owner); err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}
	// update ServiceMonitorRef if required
	updated, err := r.Common.SetResourceReference(&routeMonitor.Status.ServiceMonitorRef, namespacedName)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}
	if updated {
		return r.Common.UpdateMonitorResourceStatus(&routeMonitor)
	}
	return utilreconcile.ContinueReconcile()
}

// Ensures that all dependencies related to a RouteMonitor are deleted
func (r *RouteMonitorReconciler) EnsureMonitorAndDependenciesAbsent(routeMonitor v1alpha1.RouteMonitor) (utilreconcile.Result, error) {
	log := r.Log.WithName("Delete")

	shouldDeleteBlackBoxResources, err := r.BlackBoxExporter.ShouldDeleteBlackBoxExporterResources()
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}
	log.V(2).Info("Response of ShouldDeleteBlackBoxExporterResources", "shouldDeleteBlackBoxResources", shouldDeleteBlackBoxResources)

	if shouldDeleteBlackBoxResources {
		log.V(2).Info("Entering ensureBlackBoxExporterResourcesAbsent")
		err := r.BlackBoxExporter.EnsureBlackBoxExporterResourcesAbsent()
		if err != nil {
			return utilreconcile.RequeueReconcileWith(err)
		}
	}

	log.V(2).Info("Entering ensureServiceMonitorResourceAbsent")
	isHCP := false
	if err = r.ServiceMonitor.DeleteServiceMonitorDeployment(routeMonitor.Status.ServiceMonitorRef, isHCP); err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}

	log.V(2).Info("Entering ensurePrometheusRuleResourceAbsent")
	err = r.Prom.DeletePrometheusRuleDeployment(routeMonitor.Status.PrometheusRuleRef)
	if err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}

	log.V(2).Info("Entering ensureFinalizerAbsent")
	if r.Common.DeleteFinalizer(&routeMonitor, consts.FinalizerKey) {
		// ignore the output as we want to remove the PrevFinalizerKey anyways
		r.Common.DeleteFinalizer(&routeMonitor, consts.PrevFinalizerKey)
		return r.Common.UpdateMonitorResource(&routeMonitor)
	}
	return utilreconcile.StopReconcile()
}

func (s *RouteMonitorReconciler) EnsureFinalizerSet(routeMonitor v1alpha1.RouteMonitor) (utilreconcile.Result, error) {
	if s.Common.SetFinalizer(&routeMonitor, consts.FinalizerKey) {
		// ignore the output as we want to remove the PrevFinalizerKey anyways
		s.Common.DeleteFinalizer(&routeMonitor, consts.PrevFinalizerKey)
		return s.Common.UpdateMonitorResource(&routeMonitor)
	}
	return utilreconcile.ContinueReconcile()
}

// GetRouteMonitor return the RouteMonitor that is tested
func (r *RouteMonitorReconciler) GetRouteMonitor(req ctrl.Request) (v1alpha1.RouteMonitor, utilreconcile.Result, error) {
	routeMonitor := v1alpha1.RouteMonitor{}
	err := r.Client.Get(r.Ctx, req.NamespacedName, &routeMonitor)
	if err != nil {
		// If this is an unknown error
		if !k8serrors.IsNotFound(err) {
			res, err := utilreconcile.RequeueReconcileWith(err)
			return v1alpha1.RouteMonitor{}, res, err
		}
		r.Log.V(2).Info("StopRequeue", "As RouteMonitor is 'NotFound', stopping requeue", nil)
		return v1alpha1.RouteMonitor{}, utilreconcile.StopOperation(), nil
	}

	// if the resource is empty, we should terminate
	emptyRouteMonitor := v1alpha1.RouteMonitor{}
	if reflect.DeepEqual(routeMonitor, emptyRouteMonitor) {
		return v1alpha1.RouteMonitor{}, utilreconcile.StopOperation(), nil
	}

	return routeMonitor, utilreconcile.ContinueOperation(), nil
}

// GetRoute returns the Route from the RouteMonitor spec
func (r *RouteMonitorReconciler) GetRoute(routeMonitor v1alpha1.RouteMonitor) (routev1.Route, error) {
	res := routev1.Route{}
	nsName := types.NamespacedName{
		Name:      routeMonitor.Spec.Route.Name,
		Namespace: routeMonitor.Spec.Route.Namespace,
	}
	if nsName.Name == "" || nsName.Namespace == "" {
		err := errors.New("Invalid CR: Cannot retrieve route if one of the fields is empty")
		return res, err
	}

	err := r.Client.Get(r.Ctx, nsName, &res)
	return res, err
}

// EnsureRouteURLExists verifies that the .spec.RouteURL has the Route URL inside
func (r *RouteMonitorReconciler) EnsureRouteURLExists(route routev1.Route, routeMonitor v1alpha1.RouteMonitor) (utilreconcile.Result, error) {
	amountOfIngress := len(route.Status.Ingress)
	if amountOfIngress == 0 {
		err := errors.New("No Ingress: cannot extract route url from the Route resource")
		return utilreconcile.RequeueReconcileWith(err)
	}
	extractedRouteURL := route.Status.Ingress[0].Host
	if amountOfIngress > 1 {
		r.Log.V(1).Info(fmt.Sprintf("Too many Ingress: assuming first ingress is the correct, chosen ingress '%s'", extractedRouteURL))
	}

	if extractedRouteURL == "" {
		return utilreconcile.RequeueReconcileWith(customerrors.NoHost)
	}

	if routeMonitor.Spec.Route.Port != 0 {
		extractedRouteURL = fmt.Sprintf("%s:%d", extractedRouteURL, routeMonitor.Spec.Route.Port)
	}
	if routeMonitor.Spec.Route.Suffix != "" {
		extractedRouteURL = fmt.Sprintf("%s%s", extractedRouteURL, routeMonitor.Spec.Route.Suffix)
	}

	currentRouteURL := routeMonitor.Status.RouteURL
	if route.Spec.TLS != nil {
		r.Log.V(3).Info("TLS detected: adding https to extractedRouteURL as the url ")
		extractedRouteURL = fmt.Sprintf("https://%s", extractedRouteURL)
	}

	if currentRouteURL == extractedRouteURL {
		r.Log.V(3).Info("Same RouteURL: currentRouteURL and extractedRouteURL are equal, update not required")
		return utilreconcile.ContinueReconcile()
	}

	var err error
	if currentRouteURL != "" && extractedRouteURL != currentRouteURL {
		extractedRouteURL, err = r.checkRedirect(extractedRouteURL, routeMonitor)
		if err != nil {
			r.Log.V(3).Error(err, "RouteURL mismatch: failed to check redirects")
			return utilreconcile.RequeueReconcileWith(err)
		} else {
			r.Log.V(3).Info("RouteURL mismatch: currentRouteURL and extractedRouteURL are not equal, taking extractedRouteURL as source of truth")
		}
	}

	routeMonitor.Status.RouteURL = extractedRouteURL
	return r.Common.UpdateMonitorResource(&routeMonitor)
}

// getHostedControlPlane retrieves the HostedControlPlane object from the provided namespace. It's expected that only a single HCP object is present in the namespace,
// if multiple are found, an error is returned instead.
func (r *RouteMonitorReconciler) getHostedControlPlane(namespace string) (hypershiftv1beta1.HostedControlPlane, error) {
	hcpList := hypershiftv1beta1.HostedControlPlaneList{}
	err := r.Client.List(context.TODO(), &hcpList, client.InNamespace(namespace))
	if err != nil {
		return hypershiftv1beta1.HostedControlPlane{}, fmt.Errorf("failed to retrieve HostedControlPlanes in namespace '%s': %w", namespace, err)
	}
	if len(hcpList.Items) != 1 {
		return hypershiftv1beta1.HostedControlPlane{}, fmt.Errorf("unexpected number of HostedControlPlane objects in namespace '%s': expected 1, got %d", namespace, len(hcpList.Items))
	}
	return hcpList.Items[0], nil
}

// checkRedirect makes a HEAD request to the provided URL and handles any redirection responses.
// If a redirection status code (300-399) is received, it updates the routeMonitor object's InsecureSkipTLSVerify field to true
// and sets the extractedRouteURL to the redirection location.
func (r *RouteMonitorReconciler) checkRedirect(extractedRouteURL string, routeMonitor v1alpha1.RouteMonitor) (string, error) {
	client := r.HTTPClient
	resp, err := client.Head(extractedRouteURL)
	if err != nil {
		r.Log.V(3).Error(err, "Failed to make HEAD request to the URL")
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		r.Log.V(3).Info("RouteURL mismatch: redirection status code received, taking defaultRouteURL as source of truth")
		extractedRouteURL = resp.Header.Get("Location")
		routeMonitor.Spec.InsecureSkipTLSVerify = true
	}

	return extractedRouteURL, nil
}
