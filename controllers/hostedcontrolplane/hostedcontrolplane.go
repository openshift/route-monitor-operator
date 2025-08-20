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
	"github.com/openshift/route-monitor-operator/pkg/util/finalizer"
	utilreconcile "github.com/openshift/route-monitor-operator/pkg/util/reconcile"

	corev1 "k8s.io/api/core/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	avov1alpha2 "github.com/openshift/aws-vpce-operator/api/v1alpha2"

	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	dynatrace "github.com/openshift/route-monitor-operator/pkg/dynatrace"
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

	// vpcEndpointRetryTimeout can be used with RequeueAfter()
	vpcEndpointRetryTimeout = 5 * time.Minute
)

var logger logr.Logger = ctrl.Log.WithName("controllers").WithName("HostedControlPlane")

// HostedControlPlaneReconciler reconciles a HostedControlPlane object
type HostedControlPlaneReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// NewHostedControlPlaneReconciler creates a HostedControlPlaneReconciler
func NewHostedControlPlaneReconciler(mgr manager.Manager) *HostedControlPlaneReconciler {
	return &HostedControlPlaneReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}
}

//+kubebuilder:rbac:groups=openshift.io,resources=hostedcontrolplanes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=openshift.io,resources=hostedcontrolplanes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=openshift.io,resources=hostedcontrolplanes/finalizers,verbs=update

// Reconcile responds to events against watched objects
func (r *HostedControlPlaneReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logger.WithName("Reconcile").WithValues("name", req.Name, "namespace", req.Namespace)
	log.Info("Reconciling HostedControlPlanes")
	defer log.Info("Finished reconciling HostedControlPlane")

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
	valueDynatraceApiToken, valueDynatraceTenant, err := r.getDynatraceSecrets(ctx)
	if err != nil {
		log.Error(err, "failed to get secret for Dynatrace API client")
		return utilreconcile.RequeueWith(err)
	}
	baseURL := fmt.Sprintf("%s/v1", valueDynatraceTenant)
	dynatraceApiClient := dynatrace.NewDynatraceApiClient(baseURL, valueDynatraceApiToken)

	// If the HostedControlPlane is marked for deletion, clean up
	shouldDelete := finalizer.WasDeleteRequested(hostedcontrolplane)
	if shouldDelete {
		err = deleteDynatraceHttpMonitorResources(dynatraceApiClient, log, hostedcontrolplane)
		if err != nil {
			log.Error(err, "failed to delete Dynatrace HTTP Monitor Resources")
			return utilreconcile.RequeueWith(err)
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

	err = r.hcpReady(ctx, hostedcontrolplane)
	if err != nil {
		log.Info(fmt.Sprintf("skipped deploying monitoring objects, HostedControlPlane not ready: %v", err))
		return utilreconcile.RequeueAfter(healthcheckIntervalSeconds * time.Second), nil
	}

	log.Info("Deploying internal monitoring objects")
	err = r.deployInternalMonitoringObjects(ctx, log, hostedcontrolplane)
	if err != nil {
		log.Error(err, "failed to deploy internal monitoring components")
		return utilreconcile.RequeueWith(err)
	}

	isVpcEndpointReady, err := r.isVpcEndpointReady(ctx, hostedcontrolplane)
	if err != nil {
		log.Error(err, "VPC Endpoint check failed")
		return utilreconcile.RequeueWith(err)
	}
	if !isVpcEndpointReady {
		log.Info("VPC Endpoint is not ready, delaying HTTP Monitor deployment")
		return utilreconcile.RequeueAfter(vpcEndpointRetryTimeout), err
	}

	log.Info("Deploying HTTP Monitor Resources")
	err = r.deployDynatraceHttpMonitorResources(ctx, dynatraceApiClient, log, hostedcontrolplane)
	if err != nil {
		log.Error(err, "failed to deploy Dynatrace HTTP Monitor Resources")
		return utilreconcile.RequeueWith(err)
	}

	return ctrl.Result{}, err
}

// isVpcEndpointReady checks if the VPC Endpoint associated with the HostedControlPlane is ready.
func (r *HostedControlPlaneReconciler) isVpcEndpointReady(ctx context.Context, hostedcontrolplane *hypershiftv1beta1.HostedControlPlane) (bool, error) {
	// Create an instance of the VpcEndpoint
	vpcEndpoint := &avov1alpha2.VpcEndpoint{}

	// Construct the name and namespace of the VpcEndpoint
	vpcEndpointName := "private-hcp"
	vpcEndpointNamespace := hostedcontrolplane.Namespace

	// Fetch the VpcEndpoint resource
	err := r.Get(ctx, client.ObjectKey{Name: vpcEndpointName, Namespace: vpcEndpointNamespace}, vpcEndpoint)
	if err != nil {
		return false, err
	}

	// Check readiness using the Status field
	// Cases can be found here: https://github.com/openshift/aws-vpce-operator/blob/main/controllers/vpcendpoint/validation.go#L148
	switch vpcEndpoint.Status.Status {
	case "available":
		// VPC Endpoint is ready
		return true, nil
	case "pendingAcceptance", "pending", "deleting":
		// These states mean the VPC Endpoint is transitioning, so we return false (without an error)
		return false, nil
	case "rejected", "failed", "deleted":
		// Bad states, return an error
		return false, fmt.Errorf("VPC Endpoint %s/%s is in a bad state: %s", vpcEndpointNamespace, vpcEndpointName, vpcEndpoint.Status.Status)
	default:
		// Unknown state, return an error
		return false, fmt.Errorf("VPC Endpoint %s/%s is in an unknown state: %s", vpcEndpointNamespace, vpcEndpointName, vpcEndpoint.Status.Status)
	}
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

	// The following:
	// - Reconciles against all HostedControlPlane objects
	// - Additionally watches against route & routemonitor objects with the 'watchResourceLabel' present.
	//   When these objects are modified, the HCP specified in the objects' .metadata.OwnerReferences is
	//   reconciled
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
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1, // Prevent race conditions in monitor creation
		}).
		Complete(r)
}

// ------------------------------synthetic-monitoring--------------------------

func (r *HostedControlPlaneReconciler) getDynatraceSecrets(ctx context.Context) (string, string, error) {

	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: dynatraceSecretName, Namespace: dynatraceSecretNamespace}, secret)
	if err != nil {
		return "", "", fmt.Errorf("error getting Kubernetes secret: %v", err)
	}

	valueBytesDynatraceApiToken, ok := secret.Data[dynatraceApiKey]
	if !ok {
		return "", "", fmt.Errorf("secret did not contain key %s", dynatraceApiKey)
	}
	if len(valueBytesDynatraceApiToken) == 0 {
		return "", "", fmt.Errorf("%s is empty", dynatraceApiKey)
	}
	valueDynatraceApiToken := string(valueBytesDynatraceApiToken)

	valueBytesDynatraceTenant, ok := secret.Data[dynatraceTenantKey]
	if !ok {
		return "", "", fmt.Errorf("secret did not contain key %s", dynatraceTenantKey)
	}
	if len(valueBytesDynatraceTenant) == 0 {
		return "", "", fmt.Errorf("%s is empty", dynatraceTenantKey)
	}
	valueDynatraceTenant := string(valueBytesDynatraceTenant)

	return valueDynatraceApiToken, valueDynatraceTenant, nil
}

func getDynatraceEquivalentClusterRegionName(clusterRegion string) (string, error) {
	// Adapted from spreadsheet in https://issues.redhat.com/browse/SDE-3754
	// Coming soon regions - il-central-1, ca-west-1
	awsRegionToDynatraceRegionMapping := map[string]string{
		"us-east-1":      "N. Virginia",
		"us-east-2":      "N. Virginia",
		"us-west-1":      "Oregon",
		"us-west-2":      "Oregon",
		"af-south-1":     "São Paulo",
		"ap-southeast-1": "Singapore",
		"ap-southeast-2": "Sydney",
		"ap-southeast-3": "Singapore",
		"ap-southeast-4": "Sydney",
		"ap-northeast-1": "Singapore",
		"ap-northeast-2": "Sydney",
		"ap-northeast-3": "Singapore",
		"ap-south-1":     "Mumbai",
		"ap-south-2":     "Mumbai",
		"ap-east-1":      "Singapore",
		"ca-central-1":   "Montreal",
		"eu-west-1":      "Dublin",
		"eu-west-2":      "London",
		"eu-west-3":      "Frankfurt",
		"eu-central-1":   "Frankfurt",
		"eu-central-2":   "Frankfurt",
		"eu-south-1":     "Frankfurt",
		"eu-south-2":     "Frankfurt",
		"eu-north-1":     "London",
		"me-south-1":     "Mumbai",
		"me-central-1":   "Mumbai",
		"sa-east-1":      "São Paulo",
	}

	// Look up the equivalent dynatrace location name based on the aws region in map
	//e.g. "us-east-2" in aws has equivalent "N. Virginia" in Dynatrace Locations
	dynatraceLocationName, ok := awsRegionToDynatraceRegionMapping[clusterRegion]
	if !ok {
		return "", fmt.Errorf("location not found for region: %s", clusterRegion)
	}
	return dynatraceLocationName, nil
}

func GetAPIServerHostname(hostedcontrolplane *hypershiftv1beta1.HostedControlPlane) (string, error) {
	for _, service := range hostedcontrolplane.Spec.Services {
		if service.Service == "APIServer" {
			return service.Route.Hostname, nil
		}
	}
	return "", fmt.Errorf("APIServer service not found in the hostedcontrolplane")
}

func ensureHttpMonitor(dynatraceApiClient *dynatrace.DynatraceApiClient, hostedcontrolplane *hypershiftv1beta1.HostedControlPlane) (bool, error) {
	clusterId := hostedcontrolplane.Spec.ClusterID

	existsHttpMonitorResponse, err := dynatraceApiClient.GetDynatraceHttpMonitors(clusterId)
	if err != nil {
		return false, fmt.Errorf("failed calling ExistsHttpMonitorInDynatrace [clusterId:%s]: %v", clusterId, err)
	}
	countMonitors := len(existsHttpMonitorResponse.Monitors)
	switch {
	case countMonitors == 1:
		return true, nil

	case countMonitors == 0:
		return false, nil

	case countMonitors > 1:
		// Keep the first monitor, delete the rest
		monitorsToDelete := existsHttpMonitorResponse.Monitors[1:]
		for _, monitor := range monitorsToDelete {
			if err := dynatraceApiClient.DeleteSingleMonitor(monitor.EntityId); err != nil {
				return false, fmt.Errorf("failed to delete excess monitor %s for cluster id %s: %w", monitor.EntityId, clusterId, err)
			}
		}
		return true, nil
	}

	return false, nil
}

func getClusterRegion(hostedcontrolplane *hypershiftv1beta1.HostedControlPlane) (string, error) {
	if hostedcontrolplane == nil {
		return "", fmt.Errorf("hostedcontrolplane is nil %v", hostedcontrolplane)
	}

	clusterRegion := hostedcontrolplane.Spec.Platform.AWS.Region
	if clusterRegion == "" {
		return "", fmt.Errorf("aws region is not set in hcp %v", hostedcontrolplane)
	}

	return clusterRegion, nil
}

func determineDynatraceClusterRegionName(clusterRegion string, monitorLocationType hypershiftv1beta1.AWSEndpointAccessType) (string, error) {
	//public
	switch monitorLocationType {
	case hypershiftv1beta1.PublicAndPrivate:
		return getDynatraceEquivalentClusterRegionName(clusterRegion)
	case hypershiftv1beta1.Private:
		// cspell:ignore backplanei03xyz
		/*
			For "Private" HCPs, we have one backplane location deployed per dynatrace tenant. E.g. "name": "backplanei03xyz"
			"backplane" is returned from this function and passed to GetLocationEntityIdFromDynatrace function and this location is
			searched for in dynatrace - if strings.Contains(loc.Name, locationName) && loc.Type == "PRIVATE" && loc.Status == "ENABLED".
			Ref: https://issues.redhat.com/browse/OSD-25167
		*/
		return "backplane", nil
	default:
		return "", fmt.Errorf("monitorLocationType '%s' not supported", monitorLocationType)
	}
}

func (r *HostedControlPlaneReconciler) deployDynatraceHttpMonitorResources(ctx context.Context, dynatraceApiClient *dynatrace.DynatraceApiClient, log logr.Logger, hostedcontrolplane *hypershiftv1beta1.HostedControlPlane) error {
	//if http monitor does not exist, and hcp is not marked for deletion, and hcp is ready, then create http monitor
	//get apiserver
	apiServerHostname, err := GetAPIServerHostname(hostedcontrolplane)
	if err != nil {
		return fmt.Errorf("failed to get APIServer hostname %v", err)
	}
	monitorName := strings.Replace(apiServerHostname, "api.", "", 1)
	// apiServerHostname := hostedcontrolplane.Spec.Services[1].ServicePublishingStrategy.Route.Hostname
	monitorLocationType := hostedcontrolplane.Spec.Platform.AWS.EndpointAccess

	//in hcp, spec.services.service["APIServer"].servicePublishingStrategy.route.hostname is api.test-rs1.dgcj.i3.devshift.org // cspell:ignore dgcj, devshift
	// apiUrl := "https://api.hb-testing.j1b6.i3.devshift.org/livez"

	apiUrl := fmt.Sprintf("https://%s/livez", apiServerHostname)

	// Ensure the HTTP monitor has been created, and there is only a single instance of the monitor
	present, err := ensureHttpMonitor(dynatraceApiClient, hostedcontrolplane)
	if err != nil {
		return fmt.Errorf("failed to validate the http monitor: %v", err)
	}
	if present {
		log.Info(fmt.Sprintf("HTTP monitor found. Skipping any actions for monitor %s", monitorName))
		return nil
	}

	clusterId := hostedcontrolplane.Spec.ClusterID
	/* determine cluster region, find cluster region equivalent name in dynatrace, fetch locationId/entityId
	of the cluster region equivalent name in dynatrace, create http monitor and then update hcp labels.
	*/
	clusterRegion, err := getClusterRegion(hostedcontrolplane)
	if err != nil {
		return fmt.Errorf("error calling getClusterRegion: %v", err)
	}
	dynatraceClusterRegionName, err := determineDynatraceClusterRegionName(clusterRegion, monitorLocationType)
	if err != nil {
		return fmt.Errorf("error calling determineDynatraceClusterRegionId: %v", err)
	}

	locationId, err := dynatraceApiClient.GetLocationEntityIdFromDynatrace(dynatraceClusterRegionName, monitorLocationType)
	if err != nil {
		return fmt.Errorf("error calling GetLocationEntityIdFromDynatrace: %v", err)
	}

	monitorId, err := dynatraceApiClient.CreateDynatraceHttpMonitor(monitorName, apiUrl, clusterId, locationId, clusterRegion)
	if err != nil {
		return fmt.Errorf("error creating HTTP monitor: %v", err)
	}

	log.Info("Created HTTP monitor ", monitorId, clusterId)

	return nil
}

func deleteDynatraceHttpMonitorResources(dynatraceApiClient *dynatrace.DynatraceApiClient, log logr.Logger, hostedcontrolplane *hypershiftv1beta1.HostedControlPlane) error {
	clusterId := hostedcontrolplane.Spec.ClusterID

	err := dynatraceApiClient.DeleteDynatraceMonitorByCluserId(clusterId)
	if err != nil {
		return fmt.Errorf("error deleting HTTP monitor(s). Status Code: %v", err)
	}
	log.Info("Successfully deleted HTTP monitor(s)")
	return nil
}
