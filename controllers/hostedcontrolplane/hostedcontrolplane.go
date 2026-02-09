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

package hostedcontrolplane

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"

	routev1 "github.com/openshift/api/route/v1"
	hypershiftv1beta1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	"github.com/openshift/route-monitor-operator/config"
	"github.com/openshift/route-monitor-operator/pkg/rhobs"
	"github.com/openshift/route-monitor-operator/pkg/util/finalizer"
	utilreconcile "github.com/openshift/route-monitor-operator/pkg/util/reconcile"

	corev1 "k8s.io/api/core/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	// hostedcontrolplaneFinalizer defines the finalizer used by this controller's objects
	hostedcontrolplaneFinalizer = "hostedcontrolplane.routemonitoroperator.monitoring.openshift.io/finalizer"

	// watchResourceLabel is a label key indicating which objects this controller should reconcile against
	watchResourceLabel = "hostedcontrolplane.routemonitoroperator.monitoring.openshift.io/managed"

	//fetch dynatrace secret to get dynatrace api token and tenant url
	dynatraceSecretNamespace = "openshift-route-monitor-operator"
	dynatraceSecretName      = "dynatrace-token" // nolint:gosec // Not a hardcoded credential
	dynatraceApiKey          = "apiToken"
	dynatraceTenantKey       = "apiUrl"

	// Retry timeout configuration
	retryTimeoutMinutes = 5

	// VPC endpoint readiness retry timeout
	vpcEndpointRetryTimeout = retryTimeoutMinutes * time.Minute

	// RHOBS API retry timeout for non-200 responses
	rhobsAPIRetryTimeout = retryTimeoutMinutes * time.Minute

	// RHOBS probe deletion timeout - after this duration, fail open to allow cluster deletion
	// This prevents indefinite blocking while still allowing time for transient failures to resolve
	rhobsProbeDeletionTimeout = 15 * time.Minute

	// ConfigMap name for dynamic configuration (uses config.OperatorName + "-config")
	configMapName = config.OperatorName + "-config"
)

var logger logr.Logger = ctrl.Log.WithName("controllers").WithName("HostedControlPlane")

// HostedControlPlaneReconciler reconciles a HostedControlPlane object
type HostedControlPlaneReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	RHOBSConfig RHOBSConfig
}

// NewHostedControlPlaneReconciler creates a HostedControlPlaneReconciler
func NewHostedControlPlaneReconciler(mgr manager.Manager, rhobsConfig RHOBSConfig) *HostedControlPlaneReconciler {
	return &HostedControlPlaneReconciler{
		Client:      mgr.GetClient(),
		Scheme:      mgr.GetScheme(),
		RHOBSConfig: rhobsConfig,
	}
}

// getRHOBSConfig reads RHOBS configuration from the ConfigMap at reconcile time.
// If the ConfigMap doesn't exist or has empty values, it falls back to the command-line
// flags stored in r.RHOBSConfig.
func (r *HostedControlPlaneReconciler) getRHOBSConfig(ctx context.Context) RHOBSConfig {
	configMap := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      configMapName,
		Namespace: config.OperatorNamespace,
	}, configMap)

	if err != nil {
		// ConfigMap not found or error - return fallback config from command-line flags
		if !kerr.IsNotFound(err) {
			logger.V(2).Info("Failed to read ConfigMap, using fallback config", "error", err.Error())
		}
		return r.RHOBSConfig
	}

	// Merge ConfigMap values with fallback defaults
	cfg := r.RHOBSConfig
	if v := strings.TrimSpace(configMap.Data["probe-api-url"]); v != "" {
		cfg.ProbeAPIURL = v
	}
	if v := strings.TrimSpace(configMap.Data["probe-tenant"]); v != "" {
		cfg.Tenant = v
	}
	if v := strings.TrimSpace(configMap.Data["oidc-client-id"]); v != "" {
		cfg.OIDCClientID = v
	}
	if v := strings.TrimSpace(configMap.Data["oidc-client-secret"]); v != "" {
		cfg.OIDCClientSecret = v
	}
	if v := strings.TrimSpace(configMap.Data["oidc-issuer-url"]); v != "" {
		cfg.OIDCIssuerURL = v
	}
	if strings.TrimSpace(configMap.Data["only-public-clusters"]) == "true" {
		cfg.OnlyPublicClusters = true
	}

	return cfg
}

//+kubebuilder:rbac:groups=openshift.io,resources=hostedcontrolplanes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=openshift.io,resources=hostedcontrolplanes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=openshift.io,resources=hostedcontrolplanes/finalizers,verbs=update

// Reconcile responds to events against watched objects
func (r *HostedControlPlaneReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logger.WithName("Reconcile").WithValues("name", req.Name, "namespace", req.Namespace)
	log.Info("Reconciling HostedControlPlanes")
	defer log.Info("Finished reconciling HostedControlPlane")

	// Get dynamic config from ConfigMap (with fallback to command-line flags)
	rhobsConfig := r.getRHOBSConfig(ctx)

	// Fetch the HostedControlPlane instance
	hostedcontrolplane := &hypershiftv1beta1.HostedControlPlane{}
	err := r.Get(ctx, req.NamespacedName, hostedcontrolplane)
	if err != nil {
		if kerr.IsNotFound(err) {
			log.Info("HostedControlPlane not found, assumed deleted")
			return utilreconcile.Stop()
		}
		log.Error(err, "unable to fetch HostedControlPlane")
		return utilreconcile.RequeueWith(err)
	}

	//Create Dynatrace API client
	dynatraceApiClient, err := r.NewDynatraceApiClient(ctx)
	if err != nil {
		// If RHOBS is configured, Dynatrace client creation failure is non-fatal
		if rhobsConfig.ProbeAPIURL != "" {
			log.Info("Dynatrace client creation failed, continuing with RHOBS-only monitoring", "error", err.Error())
			dynatraceApiClient = nil
		} else {
			log.Error(err, "failed to create dynatrace client")
			return utilreconcile.RequeueWith(err)
		}
	}

	// If the HostedControlPlane is marked for deletion, clean up
	shouldDelete := finalizer.WasDeleteRequested(hostedcontrolplane)
	if shouldDelete {
		// Only attempt Dynatrace deletion if client was successfully created
		if dynatraceApiClient != nil {
			err = r.deleteDynatraceHttpMonitorResources(dynatraceApiClient, log, hostedcontrolplane)
			if err != nil {
				// If RHOBS is configured, Dynatrace failures are non-fatal - log warning and continue
				if rhobsConfig.ProbeAPIURL != "" {
					log.Info("Dynatrace HTTP Monitor deletion failed, continuing with RHOBS probe deletion", "error", err.Error())
				} else {
					log.Error(err, "failed to delete Dynatrace HTTP Monitor Resources")
					return utilreconcile.RequeueWith(err)
				}
			}
		}

		// Delete RHOBS probe if API URL is configured
		if rhobsConfig.ProbeAPIURL != "" {
			log.Info("Attempting to delete RHOBS probe", "cluster_id", hostedcontrolplane.Spec.ClusterID, "probe_api_url", rhobsConfig.ProbeAPIURL)

			// Calculate how long the cluster has been in deletion state
			deletionElapsed := time.Since(hostedcontrolplane.DeletionTimestamp.Time)
			log.V(2).Info("Probe deletion attempt", "cluster_id", hostedcontrolplane.Spec.ClusterID, "deletion_elapsed", deletionElapsed)

			err = r.deleteRHOBSProbe(ctx, log, hostedcontrolplane, rhobsConfig)
			if err != nil {
				// HYBRID APPROACH (SREP-2832 + SREP-2966):
				// - If within timeout window: retry (fail closed - prevents orphaned probes)
				// - If past timeout: fail open (prevents indefinitely blocking cluster deletion)

				if deletionElapsed < rhobsProbeDeletionTimeout {
					// Still within retry window - fail closed to prevent orphaned probes
					log.Error(err, "Failed to delete RHOBS probe, will retry",
						"cluster_id", hostedcontrolplane.Spec.ClusterID,
						"deletion_elapsed", deletionElapsed,
						"timeout", rhobsProbeDeletionTimeout,
						"behavior", "fail_closed")

					// Check if it's a non-200 error and requeue with appropriate timeout
					if rhobs.IsNon200Error(err) {
						return utilreconcile.RequeueAfter(rhobsAPIRetryTimeout), nil
					}
					return utilreconcile.RequeueWith(err)
				} else {
					// Past timeout window - fail open to allow cluster deletion
					log.Error(err, "Failed to delete RHOBS probe but deletion timeout exceeded, allowing cluster deletion to proceed",
						"cluster_id", hostedcontrolplane.Spec.ClusterID,
						"deletion_elapsed", deletionElapsed,
						"timeout", rhobsProbeDeletionTimeout,
						"behavior", "fail_open",
						"note", "Orphaned probe may require manual cleanup via synthetics-api or will be cleaned up when API is restored")
					rhobs.RecordProbeDeletionTimeout()
					// Continue with deletion (do not return error)
				}
			} else {
				log.Info("Successfully deleted RHOBS probe", "cluster_id", hostedcontrolplane.Spec.ClusterID)
			}
		} else {
			// SREP-2832: Log warning if RHOBS API URL is not configured during deletion
			// This indicates a configuration issue that could lead to orphaned probes
			log.Info("RHOBS ProbeAPIURL not configured - skipping RHOBS probe deletion. If RHOBS monitoring is enabled, this may result in orphaned resources.", "cluster_id", hostedcontrolplane.Spec.ClusterID)
		}

		err := r.finalizeHostedControlPlane(ctx, log, hostedcontrolplane)
		if err != nil {
			log.Error(err, "failed to finalize HostedControlPlane")
			return utilreconcile.RequeueWith(err)
		}
		finalizer.Remove(hostedcontrolplane, hostedcontrolplaneFinalizer)
		err = r.Update(ctx, hostedcontrolplane)
		if err != nil {
			return utilreconcile.RequeueWith(err)
		}
		return utilreconcile.Stop()
	}

	if !finalizer.Contains(hostedcontrolplane.Finalizers, hostedcontrolplaneFinalizer) {
		finalizer.Add(hostedcontrolplane, hostedcontrolplaneFinalizer)
		err := r.Update(ctx, hostedcontrolplane)
		if err != nil {
			return utilreconcile.RequeueWith(err)
		}
	}

	// Ensure cluster is ready to be monitored before deploying any probing objects
	hcpReady, err := r.hcpReady(ctx, hostedcontrolplane)
	if err != nil {
		log.Error(err, "HCP readiness check failed")
		return utilreconcile.RequeueWith(fmt.Errorf("HCP readiness check failed: %v", err))
	}
	if !hcpReady {
		log.Info("skipped deploying monitoring objects, HostedControlPlane not yet ready")
		return utilreconcile.RequeueAfter(healthcheckIntervalSeconds * time.Second), nil
	}

	vpcEndpointReady, err := r.isVpcEndpointReady(ctx, hostedcontrolplane)
	if err != nil {
		log.Error(err, "VPC Endpoint check failed")
		return utilreconcile.RequeueWith(err)
	}
	if !vpcEndpointReady {
		log.Info("VPC Endpoint is not ready, delaying HTTP Monitor deployment")
		return utilreconcile.RequeueAfter(vpcEndpointRetryTimeout), err
	}

	// Cluster ready - deploy kube-apiserver monitoring objects
	log.Info("Deploying internal monitoring objects")
	err = r.deployInternalMonitoringObjects(ctx, log, hostedcontrolplane)
	if err != nil {
		log.Error(err, "failed to deploy internal monitoring components")
		return utilreconcile.RequeueWith(err)
	}

	// Only attempt Dynatrace deployment if client was successfully created
	if dynatraceApiClient != nil {
		log.Info("Deploying HTTP Monitor Resources")
		err = r.deployDynatraceHttpMonitorResources(ctx, dynatraceApiClient, log, hostedcontrolplane)
		if err != nil {
			// If RHOBS is configured, Dynatrace failures are non-fatal - log warning and continue
			if rhobsConfig.ProbeAPIURL != "" {
				log.Info("Dynatrace HTTP Monitor deployment failed, continuing with RHOBS probe deployment", "error", err.Error())
			} else {
				log.Error(err, "failed to deploy Dynatrace HTTP Monitor Resources")
				return utilreconcile.RequeueWith(err)
			}
		}
	}

	// Deploy RHOBS probe if API URL is configured
	if rhobsConfig.ProbeAPIURL != "" {
		log.Info("Deploying RHOBS probe")
		err = r.ensureRHOBSProbe(ctx, log, hostedcontrolplane, rhobsConfig)
		if err != nil {
			log.Error(err, "failed to deploy RHOBS probe")
			// Check if it's a non-200 error and requeue
			if rhobs.IsNon200Error(err) {
				return utilreconcile.RequeueAfter(rhobsAPIRetryTimeout), nil
			}
			return utilreconcile.RequeueWith(err)
		}
	}

	return ctrl.Result{}, err
}

// deployInternalMonitoringObjects creates or updates the objects needed to monitor the kube-apiserver using cluster-internal routes
func (r *HostedControlPlaneReconciler) deployInternalMonitoringObjects(ctx context.Context, log logr.Logger, hostedcontrolplane *hypershiftv1beta1.HostedControlPlane) error {
	// Create or update route object
	expectedRoute := r.buildInternalMonitoringRoute(hostedcontrolplane)
	err := r.Create(ctx, &expectedRoute)
	if err != nil {
		if !kerr.IsAlreadyExists(err) {
			log.Error(err, "failed to create internalMonitoringRoute")
			return err
		}
		// Object already exists: update it
		actualRoute := routev1.Route{}
		err := r.Get(ctx, types.NamespacedName{Name: expectedRoute.Name, Namespace: expectedRoute.Namespace}, &actualRoute)
		if err != nil {
			log.Error(err, "failed to retrieve internalMonitoringRoute")
			return err
		}
		expectedRoute.ObjectMeta = buildMetadataForUpdate(expectedRoute.ObjectMeta, actualRoute.ObjectMeta)
		err = r.Update(ctx, &expectedRoute)
		if err != nil {
			log.Error(err, "failed to update internalMonitoringRoute")
			return err
		}
	}

	// Quick fix to discover the API server port from the service resource
	apiServerService := corev1.Service{}
	err = r.Get(ctx, types.NamespacedName{Name: "kube-apiserver", Namespace: hostedcontrolplane.Namespace}, &apiServerService)
	if err != nil {
		return fmt.Errorf("couldn't query API server service resource: %w", err)
	}
	apiServerPort := int64(6443)
	if len(apiServerService.Spec.Ports) > 0 {
		apiServerPort = int64(apiServerService.Spec.Ports[0].Port)
	}

	// Create or update RouteMonitor object
	expectedRouteMonitor := r.buildInternalMonitoringRouteMonitor(expectedRoute, hostedcontrolplane, apiServerPort)
	err = r.Create(ctx, &expectedRouteMonitor)
	if err != nil {
		if !kerr.IsAlreadyExists(err) {
			return err
		}
		// Object already exists: update it
		actualRouteMonitor := v1alpha1.RouteMonitor{}
		err = r.Get(ctx, types.NamespacedName{Name: expectedRouteMonitor.Name, Namespace: expectedRouteMonitor.Namespace}, &actualRouteMonitor)
		if err != nil {
			return err
		}
		expectedRouteMonitor.ObjectMeta = buildMetadataForUpdate(expectedRouteMonitor.ObjectMeta, actualRouteMonitor.ObjectMeta)
		err = r.Update(ctx, &expectedRouteMonitor)
		if err != nil {
			return err
		}
	}

	return nil
}

// buildMetadataForUpdate is a helper function to generate valid metadata for an Update request by combining the expected object's Metadata and the actual (on-cluster) object's Metadata
func buildMetadataForUpdate(expected, actual metav1.ObjectMeta) metav1.ObjectMeta {
	actual.Labels = expected.Labels
	actual.OwnerReferences = expected.OwnerReferences
	return actual
}

// buildInternalMonitoringRoute constructs the Route needed to monitor a HostedControlPlane's kube-apiserver via cluster-internal routes
func (r *HostedControlPlaneReconciler) buildInternalMonitoringRoute(hostedcontrolplane *hypershiftv1beta1.HostedControlPlane) routev1.Route {
	weight := int32(100)
	route := routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:            fmt.Sprintf("%s-kube-apiserver-monitoring", hostedcontrolplane.Name),
			Namespace:       hostedcontrolplane.Namespace,
			OwnerReferences: buildOwnerReferences(hostedcontrolplane),
			Labels: map[string]string{
				watchResourceLabel: "true",
			},
		},
		Spec: routev1.RouteSpec{
			Host: fmt.Sprintf("kube-apiserver.%s.svc.cluster.local", hostedcontrolplane.Namespace),
			TLS: &routev1.TLSConfig{
				InsecureEdgeTerminationPolicy: routev1.InsecureEdgeTerminationPolicyNone,
				Termination:                   routev1.TLSTerminationPassthrough,
			},
			To: routev1.RouteTargetReference{
				Kind:   "Service",
				Name:   "kube-apiserver",
				Weight: &weight,
			},
			WildcardPolicy: routev1.WildcardPolicyNone,
		},
	}

	return route
}

// buildInternalMonitoringRouteMonitor constructs the expected RouteMonitor needed to probe a HostedControlPlane's kube-apiserver using cluster-internal routes
func (r *HostedControlPlaneReconciler) buildInternalMonitoringRouteMonitor(route routev1.Route, hostedcontrolplane *hypershiftv1beta1.HostedControlPlane, apiServerPort int64) v1alpha1.RouteMonitor {
	routemonitor := v1alpha1.RouteMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:            route.Name,
			Namespace:       route.Namespace,
			OwnerReferences: buildOwnerReferences(hostedcontrolplane),
			Labels: map[string]string{
				watchResourceLabel: "true",
			},
		},
		Spec: v1alpha1.RouteMonitorSpec{
			Route: v1alpha1.RouteMonitorRouteSpec{
				Name:      route.Name,
				Namespace: route.Namespace,
				Port:      apiServerPort,
				Suffix:    "/livez",
			},
			SkipPrometheusRule: false,
			Slo: v1alpha1.SloSpec{
				TargetAvailabilityPercent: "99.5",
			},
			InsecureSkipTLSVerify: true,
			ServiceMonitorType:    v1alpha1.ServiceMonitorTypeRHOBS,
		},
	}
	return routemonitor
}

// buildOwnerReferences generates a set OwnerReferences indicating the HostedControlPlane is the owner+controller of the object. This is used
// to trigger reconciles against non-HCP objects (ie - the route & routemonitor generated by this controller)
func buildOwnerReferences(hostedcontrolplane *hypershiftv1beta1.HostedControlPlane) []metav1.OwnerReference {
	return []metav1.OwnerReference{*metav1.NewControllerRef(&hostedcontrolplane.ObjectMeta, hostedcontrolplane.GroupVersionKind())}
}

// finalizeHostedControlPlane cleans up HostedControlPlane-related objects managed by the HostedControlPlaneReconciler
func (r *HostedControlPlaneReconciler) finalizeHostedControlPlane(ctx context.Context, log logr.Logger, hostedcontrolplane *hypershiftv1beta1.HostedControlPlane) error {
	err := r.deleteInternalMonitoringObjects(ctx, log, hostedcontrolplane)
	if err != nil {
		return fmt.Errorf("failed to cleanup internal monitoring resources: %w", err)
	}
	return nil
}

// deleteInternalMonitoringObjects removes the internal monitoring objects for the provided HostedControlPlane
func (r *HostedControlPlaneReconciler) deleteInternalMonitoringObjects(ctx context.Context, log logr.Logger, hostedcontrolplane *hypershiftv1beta1.HostedControlPlane) error {
	// Delete Route
	expectedRoute := r.buildInternalMonitoringRoute(hostedcontrolplane)
	err := r.Delete(ctx, &expectedRoute)
	if err != nil {
		if !kerr.IsNotFound(err) {
			return err
		}
		log.Info(fmt.Sprintf("Skipped deleting Route %s/%s: already deleted", expectedRoute.Namespace, expectedRoute.Name))
	}

	// Delete routemonitor, port is not relevant for deletion
	expectedRouteMonitor := r.buildInternalMonitoringRouteMonitor(expectedRoute, hostedcontrolplane, 6443)
	err = r.Delete(ctx, &expectedRouteMonitor)
	if err != nil {
		if !kerr.IsNotFound(err) {
			return err
		}
		log.Info(fmt.Sprintf("Skipped deleting RouteMonitor %s/%s: already deleted", expectedRoute.Namespace, expectedRoute.Name))
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *HostedControlPlaneReconciler) SetupWithManager(mgr ctrl.Manager) error {
	selector := metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      watchResourceLabel,
				Operator: metav1.LabelSelectorOpExists,
			},
		},
	}
	selectorPredicate, err := predicate.LabelSelectorPredicate(selector)
	if err != nil {
		return fmt.Errorf("failed to build label selector predicate for routes: %w", err)
	}

	// Create handler that requeues all HCPs when ConfigMap changes
	configMapHandler := handler.EnqueueRequestsFromMapFunc(
		func(ctx context.Context, obj client.Object) []reconcile.Request {
			cm, ok := obj.(*corev1.ConfigMap)
			if !ok {
				return nil
			}
			// Only react to our config ConfigMap
			if cm.Name != configMapName || cm.Namespace != config.OperatorNamespace {
				return nil
			}

			// List all HostedControlPlanes and requeue them
			hcpList := &hypershiftv1beta1.HostedControlPlaneList{}
			if err := r.List(ctx, hcpList); err != nil {
				logger.Error(err, "failed to list HostedControlPlanes for ConfigMap change requeue")
				return nil
			}

			requests := make([]reconcile.Request, 0, len(hcpList.Items))
			for _, hcp := range hcpList.Items {
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      hcp.Name,
						Namespace: hcp.Namespace,
					},
				})
			}
			logger.Info("ConfigMap changed, requeuing HostedControlPlanes",
				"configmap", cm.Name, "count", len(requests))
			return requests
		},
	)

	// Predicate to only watch specific ConfigMap
	configMapPredicate := predicate.NewPredicateFuncs(func(obj client.Object) bool {
		return obj.GetName() == configMapName && obj.GetNamespace() == config.OperatorNamespace
	})

	// The following:
	// - Reconciles against all HostedControlPlane objects
	// - Additionally watches against route & routemonitor objects with the 'watchResourceLabel' present.
	//   When these objects are modified, the HCP specified in the objects' .metadata.OwnerReferences is
	//   reconciled
	// - Watches the operator ConfigMap to requeue all HCPs when configuration changes
	return ctrl.NewControllerManagedBy(mgr).
		For(&hypershiftv1beta1.HostedControlPlane{}).
		Watches(
			&routev1.Route{},
			handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &hypershiftv1beta1.HostedControlPlane{}, handler.OnlyControllerOwner()),
			builder.WithPredicates(selectorPredicate),
		).
		Watches(
			&v1alpha1.RouteMonitor{},
			handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &hypershiftv1beta1.HostedControlPlane{}, handler.OnlyControllerOwner()),
			builder.WithPredicates(selectorPredicate),
		).
		Watches(
			&corev1.ConfigMap{},
			configMapHandler,
			builder.WithPredicates(configMapPredicate),
		).
		Complete(r)
}
