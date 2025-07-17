package util

import (
	"context"
	"fmt"
	"regexp"

	compare "github.com/hashicorp/go-version"
	configv1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetClusterVersion retrieves the current cluster version
func GetClusterVersion(kclient client.Client) (string, error) {
	cv := &configv1.ClusterVersion{}
	if err := kclient.Get(context.TODO(), client.ObjectKey{Name: "version"}, cv); err != nil {
		return "", err
	}
	return cv.Status.Desired.Version, nil
}

// IsClusterVersionHigherOrEqualThan check whether the given version is higher or equal to the current cluster version
// Returns false if there's an exception
func IsClusterVersionHigherOrEqualThan(kclient client.Client, givenVersionStr string) bool {
	currentVersionStr, err := GetClusterVersion(kclient)
	if err != nil {
		return false
	}

	// Handle the clusternames that have more than 4 chars(such as 4.10.0-rc.4)
	re := regexp.MustCompile("([0-9]+).([0-9]+)([0-9]?)")
	shortVersion := re.FindString(currentVersionStr)

	currentVersion, err := compare.NewVersion(shortVersion)
	if err != nil {
		return false
	}

	givenVersion, err := compare.NewVersion(givenVersionStr)
	if err != nil {
		return false
	}

	if currentVersion.GreaterThanOrEqual(givenVersion) {
		return true
	}

	return false
}

// ClusterHasPrivateNLB checks whether the default ingress is private and an aws NLB
// Returns false if there's an exception
func ClusterHasPrivateNLB(kclient client.Client) (bool, error) {
	i := &operatorv1.IngressController{}
	err := kclient.Get(context.TODO(), client.ObjectKey{
		Namespace: "openshift-ingress-operator",
		Name:      "default",
	}, i)
	if err != nil {
		return false, fmt.Errorf("failed to GET 'openshift-ingress-operator/default' ingresscontroller: %w", err)
	}

	// Ensure that the nested pointers in the IC's .Status are all defined
	if i.Status.EndpointPublishingStrategy == nil {
		return false, fmt.Errorf("failed to determine if cluster has private NLB: 'openshift-ingress-operator/default' has undefined .Status.EndpointPublishingStrategy")
	}
	if i.Status.EndpointPublishingStrategy.LoadBalancer == nil {
		return false, fmt.Errorf("failed to determine if cluster has private NLB: 'openshift-ingress-operator/default' has undefined .Status.EndpointPublishingStrategy.LoadBalancer")
	}
	if i.Status.EndpointPublishingStrategy.LoadBalancer.ProviderParameters == nil {
		return false, fmt.Errorf("failed to determine if cluster has private NLB: 'openshift-ingress-operator/default' has undefined .Status.EndpointPublishingStrategy.LoadBalancer.ProviderParameters")
	}
	if i.Status.EndpointPublishingStrategy.LoadBalancer.ProviderParameters.AWS == nil {
		return false, fmt.Errorf("failed to determine if cluster has private NLB: 'openshift-ingress-operator/default' has undefined .Status.EndpointPublishingStrategy.LoadBalancer.ProviderParameters.AWS is undefined")
	}

	if i.Status.EndpointPublishingStrategy.LoadBalancer.Scope == operatorv1.InternalLoadBalancer &&
		i.Status.EndpointPublishingStrategy.LoadBalancer.ProviderParameters.AWS.Type == operatorv1.AWSNetworkLoadBalancer {
		return true, nil
	}

	return false, nil
}
