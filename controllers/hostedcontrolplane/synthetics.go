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

	hypershiftv1beta1 "github.com/openshift/hypershift/api/hypershift/v1beta1"

	"k8s.io/apimachinery/pkg/types"

	"github.com/openshift/route-monitor-operator/pkg/rhobs"
)

func GetAPIServerHostname(hostedcontrolplane *hypershiftv1beta1.HostedControlPlane) (string, error) {
	for _, service := range hostedcontrolplane.Spec.Services {
		if service.Service == "APIServer" {
			if service.Route != nil && service.Route.Hostname != "" {
				return service.Route.Hostname, nil
			}
			return "", fmt.Errorf("APIServer Route hostname is empty (service type: %s)", service.Type)
		}
	}
	return "", fmt.Errorf("APIServer service not found in the hostedcontrolplane")
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

// RHOBSConfig holds RHOBS API configuration
type RHOBSConfig struct {
	ProbeAPIURL                   string
	Tenant                        string
	OIDCClientID                  string
	OIDCClientSecret              string
	OIDCIssuerURL                 string
	OnlyPublicClusters            bool
	SkipInfrastructureHealthCheck bool
	ReconcileInterval             time.Duration
}

// ensureRHOBSProbe ensures that a RHOBS probe exists for the HostedControlPlane
func (r *HostedControlPlaneReconciler) ensureRHOBSProbe(ctx context.Context, log logr.Logger, hostedcontrolplane *hypershiftv1beta1.HostedControlPlane, cfg RHOBSConfig) error {
	clusterID := hostedcontrolplane.Spec.ClusterID
	if clusterID == "" {
		return fmt.Errorf("cluster ID is empty")
	}

	// Determine if cluster is private. Only Private clusters have API URLs
	// that resolve exclusively to VPCE endpoints, making them unreachable
	// from RHOBS cell networks. PublicAndPrivate clusters have both public
	// and PrivateLink access, so their APIs ARE externally reachable and
	// can be probed by the cell-side synthetics-agent.
	isPrivate := hostedcontrolplane.Spec.Platform.AWS != nil &&
		hostedcontrolplane.Spec.Platform.AWS.EndpointAccess == hypershiftv1beta1.Private

	// Check if cluster is in limited support -- delete probe if it exists and skip creation.
	// Cross-check the HostedCluster CR label as the source of truth, since the HCP label
	// can become stale when LS is removed (OCPBUGS-85584: reconcileHostedControlPlane
	// only does additive label sync, never removes deleted labels).
	isLimitedSupport := false
	if hostedcontrolplane.Labels["api.openshift.com/limited-support"] == "true" {
		hcNamespace := strings.TrimSuffix(hostedcontrolplane.Namespace, "-"+hostedcontrolplane.Name)
		hc := &hypershiftv1beta1.HostedCluster{}
		err := r.Get(ctx, types.NamespacedName{Name: hostedcontrolplane.Name, Namespace: hcNamespace}, hc)
		if err != nil {
			log.Info("Could not read HostedCluster to verify LS status, trusting HCP label", "cluster_id", clusterID, "error", err.Error())
			isLimitedSupport = true
		} else if hc.Labels["api.openshift.com/limited-support"] == "true" {
			isLimitedSupport = true
		} else {
			log.Info("HCP has stale limited-support label (HC label cleared), ignoring", "cluster_id", clusterID)
		}
	}
	if isLimitedSupport {
		client := r.createRHOBSClient(log, cfg)
		existingProbe, err := client.GetProbe(ctx, clusterID)
		if err != nil {
			return fmt.Errorf("failed to check existing probe: %w", err)
		}
		if existingProbe != nil {
			log.Info("Cluster is in limited support, deleting probe", "cluster_id", clusterID, "probe_id", existingProbe.ID)
			if err := client.DeleteProbe(ctx, clusterID); err != nil {
				return fmt.Errorf("failed to delete probe for limited support cluster: %w", err)
			}
		}
		return nil
	}

	// Skip private clusters if OnlyPublicClusters flag is set
	if cfg.OnlyPublicClusters && isPrivate {
		log.V(2).Info("Skipping probe creation for private cluster (only-public-clusters is enabled)", "cluster_id", clusterID)
		return nil
	}

	// Get monitoring URL (API server health endpoint in this case)
	monitoringURL, err := GetAPIServerHostname(hostedcontrolplane)
	if err != nil {
		log.Info("Failed to get API server hostname for probe", "cluster_id", clusterID, "error", err.Error())
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

			// Always update heartbeat first so the probe doesn't get GC'd
			// even if the subsequent label update fails
			heartbeatLabels := map[string]string{"last-reconciled": time.Now().UTC().Format("20060102T150405Z")}
			heartbeatErr := client.UpdateProbeLabels(ctx, existingProbe.ID, heartbeatLabels)
			if heartbeatErr != nil {
				log.Info("Failed to update probe heartbeat", "cluster_id", clusterID, "probe_id", existingProbe.ID, "error", heartbeatErr)
			}

			// Check if labels match expected values
			clusterRegion, err := getClusterRegion(hostedcontrolplane)
			if err != nil {
				return fmt.Errorf("failed to get cluster region: %w", err)
			}
			_, hasPrivateLabel := existingProbe.Labels["private"]
			labelsMatch := hasPrivateLabel &&
				isPrivateProbe(existingProbe) == isPrivate &&
				existingProbe.Labels["region"] == clusterRegion
			if labelsMatch {
				// Requeue if heartbeat failed so it gets retried
				if heartbeatErr != nil {
					return fmt.Errorf("probe labels match but heartbeat update failed: %w", heartbeatErr)
				}
				return nil
			}

			// Private label mismatch requires delete+recreate (API treats private as system-managed)
			if !hasPrivateLabel || isPrivateProbe(existingProbe) != isPrivate {
				log.Info("Private label mismatch, deleting probe for recreation", "cluster_id", clusterID, "probe_id", existingProbe.ID, "expected_private", isPrivate, "actual_private", isPrivateProbe(existingProbe))
				if err := client.DeleteProbe(ctx, clusterID); err != nil {
					return fmt.Errorf("failed to delete probe for private label correction: %w", err)
				}
				// Fall through to recreate probe below
			} else {
				// Region-only mismatch: PATCH without private label
				log.Info("RHOBS probe region label mismatch, updating", "cluster_id", clusterID, "probe_id", existingProbe.ID, "expected_region", clusterRegion, "actual_region", existingProbe.Labels["region"])
				updatedLabels := map[string]string{
					"cluster-id":      clusterID,
					"region":          clusterRegion,
					"last-reconciled": time.Now().UTC().Format("20060102T150405Z"),
				}
				err = client.UpdateProbeLabels(ctx, existingProbe.ID, updatedLabels)
				if err != nil {
					return fmt.Errorf("failed to update RHOBS probe labels: %w", err)
				}
				return nil
			}
		}
	}

	// Get cluster region for probe assignment
	clusterRegion, err := getClusterRegion(hostedcontrolplane)
	if err != nil {
		return fmt.Errorf("failed to get cluster region: %w", err)
	}

	// Create probe request with region label for regional filtering
	probeReq := rhobs.NewClusterProbeRequest(clusterID, monitoringURL, clusterRegion, isPrivate)

	log.Info("Creating new RHOBS probe", "cluster_id", clusterID, "monitoring_url", monitoringURL, "private", isPrivate, "region", clusterRegion)

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
