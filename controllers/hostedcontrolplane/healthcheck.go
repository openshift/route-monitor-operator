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
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	avov1alpha2 "github.com/openshift/aws-vpce-operator/api/v1alpha2"
	hypershiftv1beta1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	// healthcheckAnnotation is the key for the annotation which stores the historical probing data for an HCP cluster
	healthcheckAnnotation = "routemonitor.managed.openshift.io/successful-healthchecks"

	// consecutiveSuccessfulHealthchecks defines the number of healthchecks in a row that must succeed before
	// an HCP is considered healthy and it is fully reconciled
	consecutiveSuccessfulHealthchecks = 5

	// healthcheckIntervalSeconds defines the wait period between requeues when an HCP cluster in the process of being healthchecked
	healthcheckIntervalSeconds = 30

	// hcpHealthCheckSkipAge defines the minimum age an HCP cluster needs to be before it will no longer be healthchecked
	hcpHealthCheckSkipAge = 3 * time.Hour
)

// hcpReady attempts to determine the readiness of an HCP cluster. It returns a boolean indicating readiness, as well as string indicating
// the reason. An error indicates an issue was encountered while performing the healthcheck operation, and does not necessarily mean there is
// an issue with the HCP cluster itself.
//
// A new HCP is considered ready if its kube-apiserver's /livez endpoint can be polled successfully several times in a row; any cluster older than 3h is automatically considered ready. Polling history is
// stored in the annotation of a configmap object within the HCP's namespace.
//
// If the configmap's annotation indicates a cluster has already been polled successfully in the past, then this function returns true. If
// the polling history indicates that additional healthchecks are needed to determine if the cluster is ready, then the /livez endpoint will
// be probed again, and the updated probing history will be consulted once again to determine if the cluster is ready to be reconciled.
//
// If healthchecking should be restarted for a cluster for some reason, the annotation can be removed from the healthcheck configmap in the
// HCP namespace, and this process will be restarted. Additionally, should healthchecking need to be skipped for any reason, the annotation
// "routemonitor.managed.openshift.io/successful-healthchecks" can be added-to/edited-on the configmap with a large number (ie - 999) to bypass
// this functionality
func (r *HostedControlPlaneReconciler) hcpReady(ctx context.Context, hostedcontrolplane *hypershiftv1beta1.HostedControlPlane, cfg RHOBSConfig) (bool, error) {
	// Skip health check for test environments (e.g., osde2e tests without real kube-apiserver)
	if cfg.SkipInfrastructureHealthCheck {
		return true, nil
	}

	if olderThan(hostedcontrolplane, hcpHealthCheckSkipAge) {
		return true, nil
	}

	healthcheckConfigMap, err := r.getHealthCheckConfigMap(ctx, hostedcontrolplane)
	if err != nil {
		if !kerr.IsNotFound(err) {
			// if error is not related to the configmap not existing, return
			return false, fmt.Errorf("failed to retrieve healthcheck configmap: %w", err)
		}

		// healthcheck configmap does not exist - create it
		healthcheckConfigMap, err = r.createHealthcheckConfigMap(ctx, hostedcontrolplane)
		if err != nil {
			return false, fmt.Errorf("failed to create new healthcheck configmap: %w", err)
		}
	}

	successes := healthcheckConfigMapSuccesses(healthcheckConfigMap)
	if successes >= consecutiveSuccessfulHealthchecks {
		return true, nil
	}

	err = healthcheckHostedControlPlane(hostedcontrolplane)
	if err != nil {
		_, resetErr := r.resetHealthCheckSuccesses(ctx, healthcheckConfigMap)
		if resetErr != nil {
			err = errors.Join(err, resetErr)
			return false, fmt.Errorf("failed to update configmap healthcheck count following healthchecking failure. Errors: %w", err)
		}
		return false, nil
	}

	healthcheckConfigMap, err = r.addHealthCheckSuccess(ctx, healthcheckConfigMap)
	if err != nil {
		return false, fmt.Errorf("failed to increment healthcheck success count: %w", err)
	}

	successes = healthcheckConfigMapSuccesses(healthcheckConfigMap)
	if successes >= consecutiveSuccessfulHealthchecks {
		return true, nil
	}

	return false, nil
}

// getHealthCheckConfigMap retrieves the healthcheck configmap for the provided HCP from the cluster
func (r *HostedControlPlaneReconciler) getHealthCheckConfigMap(ctx context.Context, hostedcontrolplane *hypershiftv1beta1.HostedControlPlane) (corev1.ConfigMap, error) {
	configmap := buildHealthCheckConfigMap(hostedcontrolplane)
	err := r.Get(ctx, types.NamespacedName{Name: configmap.Name, Namespace: configmap.Namespace}, &configmap)
	return configmap, err
}

// createHealthcheckConfigMap creates a new configmap to track the healthchecking history of the provided HCP, and returns the resulting object along with any error encountered
func (r *HostedControlPlaneReconciler) createHealthcheckConfigMap(ctx context.Context, hostedcontrolplane *hypershiftv1beta1.HostedControlPlane) (corev1.ConfigMap, error) {
	configmap := buildHealthCheckConfigMap(hostedcontrolplane)
	err := r.Create(ctx, &configmap)
	return configmap, err
}

// buildHealthCheckConfigMap creates an empty configmap used to track the healthcheck history for a hostedcontrolplane cluster
func buildHealthCheckConfigMap(hostedcontrolplane *hypershiftv1beta1.HostedControlPlane) corev1.ConfigMap {
	configmap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            fmt.Sprintf("%s-kube-apiserver-rmo-healthcheck", hostedcontrolplane.Name),
			Namespace:       hostedcontrolplane.Namespace,
			OwnerReferences: buildOwnerReferences(hostedcontrolplane),
		},
	}
	return configmap
}

// healthcheckConfigMapSuccesses returns the number of recorded successful healthchecks the configmap has tallied under it's healthcheckAnnotation annotation.
//
// If the proper annotation cannot be found, or does not have an integer as its key, the success count is assumed to be 0
func healthcheckConfigMapSuccesses(configmap corev1.ConfigMap) int {
	value, found := configmap.Annotations[healthcheckAnnotation]
	if !found {
		// if the annotation hasn't been added yet, then healthcheck success count is 0
		return 0
	}

	successes, err := strconv.Atoi(value)
	if err != nil {
		// if there's an invalid value on the annotation, just assume healthcheck success count is 0
		return 0
	}
	return successes
}

// resetHealthCheckSuccesses sets the value of the healthcheck success counter to 0 on the configmap on-cluster, and returns an updated copy of the configmap
func (r *HostedControlPlaneReconciler) resetHealthCheckSuccesses(ctx context.Context, configmap corev1.ConfigMap) (corev1.ConfigMap, error) {
	delete(configmap.Annotations, healthcheckAnnotation)
	err := r.Update(ctx, &configmap)
	return configmap, err
}

// addHealthCheckSuccess increments the healthcheck success counter by 1, and updates the configmap on-cluster
func (r *HostedControlPlaneReconciler) addHealthCheckSuccess(ctx context.Context, configmap corev1.ConfigMap) (corev1.ConfigMap, error) {
	successes := healthcheckConfigMapSuccesses(configmap)
	successes++

	if configmap.Annotations == nil {
		configmap.Annotations = map[string]string{}
	}
	configmap.Annotations[healthcheckAnnotation] = fmt.Sprintf("%d", successes)
	err := r.Update(ctx, &configmap)
	return configmap, err
}

// healthcheckHostedControlPlane performs a healthcheck against the provided HCP by checking the response from its kube-apiserver's
// /livez endpoint
func healthcheckHostedControlPlane(hostedcontrolplane *hypershiftv1beta1.HostedControlPlane) error {
	controlplaneEndpoint := hostedcontrolplane.Status.ControlPlaneEndpoint.Host
	if controlplaneEndpoint == "" {
		return fmt.Errorf("missing .Status.ControlPlaneEndpoint.Host")
	}

	var url string
	var secure bool
	if hostedcontrolplane.Spec.Platform.AWS != nil &&
		hostedcontrolplane.Spec.Platform.AWS.EndpointAccess == hypershiftv1beta1.Private {
		url = fmt.Sprintf("https://kube-apiserver.%s.svc.cluster.local:6443/livez", hostedcontrolplane.Namespace)
		secure = false
	} else {
		url = fmt.Sprintf("https://%s/livez", controlplaneEndpoint)
		secure = true
	}

	return endpointOK(url, secure)
}

// endpointOK checks the readiness of the given url, and returns an error if the GET fails, or a non-200
// response is received
func endpointOK(endpoint string, secure bool) error {
	// Create HTTP client with appropriate TLS configuration
	client := &http.Client{}
	if !secure {
		// Skip certificate verification when secure is false
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // Required for internal cluster communication
		}
	}

	resp, err := client.Get(endpoint)
	if err != nil {
		return fmt.Errorf("failed to GET endpoint: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("non 200 HTTP status returned: %s", resp.Status)
	}
	return nil
}

// checkClusterOver1Hour determines if the HCP cluster is over one hour old
func olderThan(obj metav1.Object, age time.Duration) bool {
	now := time.Now()
	minCreationTime := now.Add((-1 * age))
	return obj.GetCreationTimestamp().Time.Before(minCreationTime)
}

// isVpcEndpointReady checks if the VPC Endpoint associated with the HostedControlPlane is ready.
func (r *HostedControlPlaneReconciler) isVpcEndpointReady(ctx context.Context, hostedcontrolplane *hypershiftv1beta1.HostedControlPlane, cfg RHOBSConfig) (bool, error) {
	// Skip VPC endpoint check for test environments (e.g., osde2e tests without real VPC infrastructure)
	if cfg.SkipInfrastructureHealthCheck {
		return true, nil
	}

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
