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
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/go-logr/logr"

	hypershiftv1beta1 "github.com/openshift/hypershift/api/hypershift/v1beta1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	dynatrace "github.com/openshift/route-monitor-operator/pkg/dynatrace"
	"github.com/openshift/route-monitor-operator/pkg/rhobs"
)

func (r *HostedControlPlaneReconciler) NewDynatraceApiClient(ctx context.Context) (*dynatrace.DynatraceApiClient, error) {
	//Create Dynatrace API client
	apiToken, tenant, err := r.getDynatraceSecrets(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get secret for Dynatrace API client: %w", err)
	}
	baseURL := fmt.Sprintf("%s/v1", tenant)
	dynatraceApiClient := dynatrace.NewDynatraceApiClient(baseURL, apiToken)

	return dynatraceApiClient, nil
}

func (r *HostedControlPlaneReconciler) getDynatraceSecrets(ctx context.Context) (string, string, error) {
	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: dynatraceSecretName, Namespace: dynatraceSecretNamespace}, secret)
	if err != nil {
		return "", "", fmt.Errorf("error getting Kubernetes secret: %v", err)
	}

	apiTokenBytes, ok := secret.Data[dynatraceApiKey]
	if !ok {
		return "", "", fmt.Errorf("secret did not contain key %s", dynatraceApiKey)
	}
	if len(apiTokenBytes) == 0 {
		return "", "", fmt.Errorf("%s is empty", dynatraceApiKey)
	}
	apiToken := string(apiTokenBytes)

	dynatraceTenantBytes, ok := secret.Data[dynatraceTenantKey]
	if !ok {
		return "", "", fmt.Errorf("secret did not contain key %s", dynatraceTenantKey)
	}
	if len(dynatraceTenantBytes) == 0 {
		return "", "", fmt.Errorf("%s is empty", dynatraceTenantKey)
	}
	tenant := string(dynatraceTenantBytes)

	return apiToken, tenant, nil
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

func removeDyntraceMonitors(dynatraceClient *dynatrace.DynatraceApiClient, monitors []dynatrace.BasicHttpMonitor) error {
	var err error
	for _, monitor := range monitors {
		err = errors.Join(err, dynatraceClient.DeleteSingleMonitor(monitor.EntityId))
	}
	return err
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
	apiServerHostname, err := GetAPIServerHostname(hostedcontrolplane)
	if err != nil {
		return fmt.Errorf("failed to get APIServer hostname %v", err)
	}
	monitorName := strings.Replace(apiServerHostname, "api.", "", 1)
	monitorLocationType := hostedcontrolplane.Spec.Platform.AWS.EndpointAccess
	apiUrl := fmt.Sprintf("https://%s/livez", apiServerHostname)
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

	clusterID := hostedcontrolplane.Spec.ClusterID
	if clusterID == "" {
		return fmt.Errorf("hostedcontrolplane has empty .Spec.ClusterID field")
	}

	// Check for existing monitors
	monitors, err := dynatraceApiClient.ListDynatraceHttpMonitorsForCluster(clusterID)
	if err != nil {
		return fmt.Errorf("failed to retrieve existing HTTP monitors from Dynatrace for cluster %q: %w", clusterID, err)
	}

	if len(monitors) > 0 {
		// Cleanup excess HTTP Monitors, if any exist
		if len(monitors) > 1 {
			err = removeDyntraceMonitors(dynatraceApiClient, monitors[1:])
			if err != nil {
				// log any errors regarding extra-monitor cleanup, but do not block further action
				log.Error(err, "failed to cleanup excess Dynatrace monitors")
			}
		}

		existingMonitor, err := dynatraceApiClient.GetDynatraceHttpMonitor(monitors[0].EntityId)
		if err != nil {
			return fmt.Errorf("failed to retrieve existing monitor %q (ID=%q) from Dynatrace: %w", existingMonitor.Name, existingMonitor.EntityId, err)
		}

		if slices.Contains(existingMonitor.Locations, locationId) {
			// Existing monitor matches expected - no further action needed
			return nil
		}
		log.Info(fmt.Sprintf("monitor location needs to be updated, possibly due to API publishing strategy change in OCM. Deleting Dynatrace HTTP monitor %q in order to recreate in the correct synthetic location", existingMonitor.Name))
		log.V(2).Info("current location(s) is %v, should be %q", existingMonitor.Locations, locationId)

		err = dynatraceApiClient.DeleteSingleMonitor(existingMonitor.EntityId)
		if err != nil {
			return fmt.Errorf("failed to delete HTTP monitor %q (ID=%q) from Dynatrace: %w", existingMonitor.Name, existingMonitor.EntityId, err)
		}
	}

	monitorId, err := dynatraceApiClient.CreateDynatraceHttpMonitor(monitorName, apiUrl, clusterID, locationId, clusterRegion)
	if err != nil {
		return fmt.Errorf("error creating HTTP monitor %q: %w", monitorName, err)
	}
	log.Info("Successfully created HTTP monitor", "monitor ID", monitorId)

	return nil
}

func (r *HostedControlPlaneReconciler) deleteDynatraceHttpMonitorResources(dynatraceApiClient *dynatrace.DynatraceApiClient, log logr.Logger, hostedcontrolplane *hypershiftv1beta1.HostedControlPlane) error {
	clusterId := hostedcontrolplane.Spec.ClusterID

	err := dynatraceApiClient.DeleteDynatraceMonitorByCluserId(clusterId)
	if err != nil {
		return fmt.Errorf("error deleting HTTP monitor(s). Status Code: %v", err)
	}
	log.Info("Successfully deleted HTTP monitor(s)")
	return nil
}

// RHOBSConfig holds RHOBS API configuration
type RHOBSConfig struct {
	ProbeAPIURL        string
	Tenant             string
	OIDCClientID       string
	OIDCClientSecret   string
	OIDCIssuerURL      string
	OnlyPublicClusters bool
}

// DynatraceConfig holds Dynatrace feature flag configuration
type DynatraceConfig struct {
	Enabled bool // Feature flag
}

// ensureRHOBSProbe ensures that a RHOBS probe exists for the HostedControlPlane
func (r *HostedControlPlaneReconciler) ensureRHOBSProbe(ctx context.Context, log logr.Logger, hostedcontrolplane *hypershiftv1beta1.HostedControlPlane, cfg RHOBSConfig) error {
	clusterID := hostedcontrolplane.Spec.ClusterID
	if clusterID == "" {
		return fmt.Errorf("cluster ID is empty")
	}

	// Determine if cluster is private
	isPrivate := hostedcontrolplane.Spec.Platform.AWS != nil &&
		hostedcontrolplane.Spec.Platform.AWS.EndpointAccess == hypershiftv1beta1.Private

	// Skip private clusters if OnlyPublicClusters flag is set
	if cfg.OnlyPublicClusters && isPrivate {
		log.V(2).Info("Skipping probe creation for private cluster (only-public-clusters is enabled)", "cluster_id", clusterID)
		return nil
	}

	// Get monitoring URL (API server health endpoint in this case)
	monitoringURL, err := GetAPIServerHostname(hostedcontrolplane)
	if err != nil {
		return fmt.Errorf("failed to get API server hostname: %w", err)
	}
	monitoringURL = fmt.Sprintf("https://%s/livez", monitoringURL)

	// Create RHOBS client
	client := r.createRHOBSClient(log, cfg)

	existingProbe, err := client.GetProbe(ctx, clusterID)
	if err != nil {
		return fmt.Errorf("failed to check existing probe: %w", err)
	}

	if existingProbe != nil {
		// Handle failed probes by deleting and recreating them
		if existingProbe.Status == "failed" {
			log.Info("Found probe in failed state, recreating", "cluster_id", clusterID, "probe_id", existingProbe.ID)
			// Delete the failed probe first
			err := client.DeleteProbe(ctx, clusterID)
			if err != nil {
				return fmt.Errorf("failed to delete failed probe: %w", err)
			}
			// Continue to create new probe below

		} else {
			// Probe exists - validate that it's configured correctly according to the hostedcontrolplane object
			log.V(2).Info("RHOBS probe already exists", "cluster_id", clusterID, "probe_id", existingProbe.ID, "status", existingProbe.Status)
			if isPrivateProbe(existingProbe) == isPrivate {
				// Probe already configured correctly, return
				return nil
			}

			// Probe configuration incorrect or out-of-date: delete and recreate
			log.Info("RHOBS probe 'private' label does not match hostedcontrolplane configuration, possibly due to API publishing strategy change in OCM. Deleting RHOBS probe in order to recreate in the correct cell", "probe", existingProbe)
			err = client.DeleteProbe(ctx, clusterID)
			if err != nil {
				return fmt.Errorf("failed to delete RHOBS probe: %w", err)
			}
			// Continue to create new probe below
		}
	}

	// Create probe request using the convenience function
	// Note: Additional labels like management-cluster-id can be added in the future
	probeReq := rhobs.NewClusterProbeRequest(clusterID, monitoringURL, isPrivate)

	// Create the probe
	probe, err := client.CreateProbe(ctx, probeReq)
	if err != nil {
		return fmt.Errorf("failed to create RHOBS probe: %w", err)
	}

	log.Info("Successfully created RHOBS probe", "cluster_id", clusterID, "probe_id", probe.ID)
	return nil
}

// deleteRHOBSProbe deletes the RHOBS probe for the HostedControlPlane
//
// This function attempts to mark the probe for deletion (sets status to terminating).
// It returns an error if the deletion fails to enable retry logic in the caller.
func (r *HostedControlPlaneReconciler) deleteRHOBSProbe(ctx context.Context, log logr.Logger, hostedcontrolplane *hypershiftv1beta1.HostedControlPlane, cfg RHOBSConfig) error {
	clusterID := hostedcontrolplane.Spec.ClusterID
	if clusterID == "" {
		return fmt.Errorf("cluster ID is empty")
	}

	// Create RHOBS client
	client := r.createRHOBSClient(log, cfg)

	// Delete the probe (sets status to terminating)
	err := client.DeleteProbe(ctx, clusterID)
	if err != nil {
		return fmt.Errorf("failed to delete RHOBS probe for cluster %s: %w", clusterID, err)
	}

	log.V(2).Info("Successfully marked RHOBS probe for termination", "cluster_id", clusterID)
	return nil
}

// createRHOBSClient creates an RHOBS client with or without OIDC authentication based on configuration
func (r *HostedControlPlaneReconciler) createRHOBSClient(log logr.Logger, cfg RHOBSConfig) *rhobs.Client {
	if cfg.OIDCClientID != "" && cfg.OIDCClientSecret != "" && cfg.OIDCIssuerURL != "" {
		oidcConfig := rhobs.OIDCConfig{
			ClientID:     cfg.OIDCClientID,
			ClientSecret: cfg.OIDCClientSecret,
			IssuerURL:    cfg.OIDCIssuerURL,
		}
		log.V(2).Info("Creating RHOBS client with OIDC authentication")
		// Use configurable tenant name in URL path, OIDC client ID is used for authentication headers
		return rhobs.NewClientWithOIDC(cfg.ProbeAPIURL, cfg.Tenant, oidcConfig, log)
	}

	log.V(2).Info("Creating RHOBS client without authentication")
	return rhobs.NewClient(cfg.ProbeAPIURL, cfg.Tenant, log)
}

func isPrivateProbe(probe *rhobs.ProbeResponse) bool {
	return probe.Labels["private"] == "true"
}
