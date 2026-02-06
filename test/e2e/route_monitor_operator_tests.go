// DO NOT REMOVE TAGS BELOW. IF ANY NEW TEST FILES ARE CREATED UNDER /osde2e, PLEASE ADD THESE TAGS TO THEM IN ORDER TO BE EXCLUDED FROM UNIT TESTS. //go:build osde2e
//go:build osde2e
// +build osde2e

package osde2etests

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	routev1 "github.com/openshift/api/route/v1"
	awsvpceapi "github.com/openshift/aws-vpce-operator/api/v1alpha2"
	hypershiftv1beta1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	"github.com/openshift/osde2e-common/pkg/clients/openshift"
	. "github.com/openshift/osde2e-common/pkg/gomega/assertions"
	. "github.com/openshift/osde2e-common/pkg/gomega/matchers"
	"github.com/openshift/osde2e/pkg/common/providers"
	routemonitorv1alpha1 "github.com/openshift/route-monitor-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	kubev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Local minimal ClusterDeployment type for testing
// We define this locally to avoid pulling in the full hive dependency
type ClusterDeployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ClusterDeploymentSpec `json:"spec,omitempty"`
}

// DeepCopyObject implements runtime.Object interface
func (cd *ClusterDeployment) DeepCopyObject() runtime.Object {
	if cd == nil {
		return nil
	}
	out := new(ClusterDeployment)
	cd.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies all properties of this object into another object of the same type
func (cd *ClusterDeployment) DeepCopyInto(out *ClusterDeployment) {
	*out = *cd
	out.TypeMeta = cd.TypeMeta
	cd.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = cd.Spec
}

type ClusterDeploymentSpec struct {
	ClusterName string `json:"clusterName"`
	BaseDomain  string `json:"baseDomain"`
}

const (
	// Embedded CRD definitions - no external dependencies required
	clusterDeploymentCRD = `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: clusterdeployments.hive.openshift.io
spec:
  group: hive.openshift.io
  names:
    kind: ClusterDeployment
    listKind: ClusterDeploymentList
    plural: clusterdeployments
    shortNames:
    - cd
    singular: clusterdeployment
  scope: Namespaced
  versions:
  - name: v1
    served: true
    storage: true
    schema:
      openAPIV3Schema:
        type: object
        x-kubernetes-preserve-unknown-fields: true
    subresources:
      status: {}
`

	hostedControlPlaneCRD = `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: hostedcontrolplanes.hypershift.openshift.io
spec:
  group: hypershift.openshift.io
  names:
    kind: HostedControlPlane
    listKind: HostedControlPlaneList
    plural: hostedcontrolplanes
    shortNames:
    - hcp
    singular: hostedcontrolplane
  scope: Namespaced
  versions:
  - name: v1beta1
    served: true
    storage: true
    schema:
      openAPIV3Schema:
        type: object
        x-kubernetes-preserve-unknown-fields: true
    subresources:
      status: {}
`

	vpcEndpointCRD = `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: vpcendpoints.avo.openshift.io
spec:
  group: avo.openshift.io
  names:
    kind: VpcEndpoint
    listKind: VpcEndpointList
    plural: vpcendpoints
    singular: vpcendpoint
  scope: Namespaced
  versions:
  - name: v1alpha2
    served: true
    storage: true
    schema:
      openAPIV3Schema:
        type: object
        x-kubernetes-preserve-unknown-fields: true
    subresources:
      status: {}
`
)

var _ = Describe("Route Monitor Operator", Ordered, func() {
	var (
		k8s                   *openshift.Client
		serviceMonitorsClient dynamic.ResourceInterface
		prometheusRulesClient dynamic.ResourceInterface
	)
	const (
		namespace        = "openshift-route-monitor-operator"
		deploymentName   = "route-monitor-operator-controller-manager"
		operatorName     = "route-monitor-operator"
		consoleName      = "console"
		pollingDuration  = 2 * time.Minute
		routeMonitorName = "routemonitor-e2e-test"
	)
	BeforeAll(func(ctx context.Context) {
		log.SetLogger(GinkgoLogr)
		var err error
		k8s, err = openshift.New(GinkgoLogr)
		Expect(err).ShouldNot(HaveOccurred(), "unable to setup k8s client")
		dynamicClient, err := dynamic.NewForConfig(k8s.GetConfig())
		Expect(err).ShouldNot(HaveOccurred(), "failed creating the dynamic client: %w", err)
		Expect(routemonitorv1alpha1.AddToScheme(k8s.GetScheme())).Should(Succeed(), "unable to register RouteMonitor scheme")
		serviceMonitorsClient = dynamicClient.Resource(schema.GroupVersionResource{
			Group:    "monitoring.openshift.io",
			Version:  "v1alpha1",
			Resource: "routemonitors",
		}).Namespace(namespace)
		prometheusRulesClient = dynamicClient.Resource(schema.GroupVersionResource{
			Group:    "monitoring.coreos.com",
			Version:  "v1",
			Resource: "prometheusrules",
		}).Namespace(namespace)
	})

	It("is installed", func(ctx context.Context) {
		By("checking the namespace exists")
		err := k8s.Get(ctx, namespace, "", &corev1.Namespace{})
		Expect(err).ShouldNot(HaveOccurred(), "namespace %s not found", namespace)

		By("checking the deployment exists and is available")
		EventuallyDeployment(ctx, k8s, deploymentName, namespace).Should(BeAvailable())
	})

	PIt("can be upgraded", func(ctx context.Context) {
		By("forcing operator upgrade")
		err := k8s.UpgradeOperator(ctx, operatorName, namespace)
		Expect(err).ShouldNot(HaveOccurred(), "operator upgrade failed")
	})

	It("has all of the required resources", func(ctx context.Context) {
		_, err := serviceMonitorsClient.Get(ctx, consoleName, metav1.GetOptions{})
		Expect(err).ShouldNot(HaveOccurred(), "Unable to get console serviceMonitor")

		_, err = prometheusRulesClient.Get(ctx, consoleName, metav1.GetOptions{})
		Expect(err).ShouldNot(HaveOccurred(), "Unable to get console prometheusRule")
	})

	It("required dependent resources are created", func(ctx context.Context) {
		By("Creating a pod, service and route to monitor with a ServiceMonitor and PrometheusRule")
		By("Creating the test pod")
		var allowPrivilegeEscalation *bool = new(bool)
		*allowPrivilegeEscalation = false
		var runAsNonRoot *bool = new(bool)
		*runAsNonRoot = true
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      routeMonitorName,
				Namespace: namespace,
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "test",
						Image: "quay.io/jitesoft/nginx:mainline",
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: allowPrivilegeEscalation,
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{"ALL"},
							},
							RunAsNonRoot: runAsNonRoot,
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
				},
			},
		}
		err := k8s.Create(ctx, pod)
		Expect(err).ShouldNot(HaveOccurred(), "Unable to create a pod")

		By("Checking the pod state")
		var phase kubev1.PodPhase
		err = wait.PollUntilContextTimeout(ctx, 15*time.Second, pollingDuration, false, func(ctx context.Context) (bool, error) {
			err := k8s.Get(ctx, pod.Name, pod.Namespace, pod)
			Expect(err).ShouldNot(HaveOccurred(), "Unable to get the testing pod")
			if pod != nil {
				phase = pod.Status.Phase
				if phase == kubev1.PodRunning || phase == kubev1.PodFailed {
					return true, nil
				}
			}
			GinkgoLogr.Info(fmt.Sprintf("Waiting for Pod '%s/%s' to be %s, currently %s...", pod.Namespace, pod.Name, kubev1.PodRunning, phase))
			return phase == kubev1.PodRunning, nil
		})
		Expect(err).ShouldNot(HaveOccurred(), fmt.Sprintf("Pod %s in ns %s is not running, current pod state is %v", routeMonitorName, namespace, phase))

		By("Creating the service")
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      routeMonitorName,
				Namespace: namespace,
				Labels:    map[string]string{routeMonitorName: routeMonitorName},
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{
						Port:       8080,
						Protocol:   corev1.ProtocolTCP,
						TargetPort: intstr.FromInt(80),
						Name:       "web",
					},
				},
				Selector: map[string]string{routeMonitorName: routeMonitorName},
			},
		}

		err = k8s.Create(ctx, svc)
		Expect(err).ShouldNot(HaveOccurred(), "Unable to create service %s/%s", svc.Namespace, svc.Name)

		By("Creating the route")
		appRoute := &routev1.Route{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      routeMonitorName,
			},
			Spec: routev1.RouteSpec{
				To: routev1.RouteTargetReference{
					Name: routeMonitorName,
				},
				TLS: &routev1.TLSConfig{Termination: "edge"},
			},
			Status: routev1.RouteStatus{},
		}

		err = k8s.Create(ctx, appRoute)
		Expect(err).ShouldNot(HaveOccurred(), "Unable to create route %s/%s", appRoute.Namespace, appRoute.Name)

		By("Creating a sample RouteMonitor to monitor the service")
		rmo := &routemonitorv1alpha1.RouteMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      routeMonitorName,
			},
			Spec: routemonitorv1alpha1.RouteMonitorSpec{
				Slo: routemonitorv1alpha1.SloSpec{
					TargetAvailabilityPercent: "99.95",
				},
				Route: routemonitorv1alpha1.RouteMonitorRouteSpec{
					Namespace: namespace,
					Name:      routeMonitorName,
				},
			},
		}

		err = k8s.Create(ctx, rmo)
		Expect(err).ShouldNot(HaveOccurred(), "Unable to create RouteMonitor %s/%s", rmo.Namespace, rmo.Name)

		By("Checking the dependent resources are created")
		err = wait.PollUntilContextTimeout(ctx, 15*time.Second, pollingDuration, false, func(ctx context.Context) (bool, error) {
			_, err = serviceMonitorsClient.Get(ctx, routeMonitorName, metav1.GetOptions{})
			if k8serrors.IsNotFound(err) {
				return false, err // Continue polling if resource is not found
			}
			if err != nil {
				return false, err // Return error to stop polling if other errors occur
			}

			_, err = prometheusRulesClient.Get(ctx, routeMonitorName, metav1.GetOptions{})
			if k8serrors.IsNotFound(err) {
				return false, err
			}
			if err != nil {
				return false, err
			}
			return true, nil
		})
		Expect(err).ShouldNot(HaveOccurred(), "Dependant resources weren't created via RouteMonitor")

		By("Deleting the sample RouteMonitor")
		err = k8s.Delete(ctx, rmo)
		Expect(err).ShouldNot(HaveOccurred(), "Failed to delete RouteMonitor")

		// wait for the routemonitor to delete before returning.
		err = wait.PollUntilContextTimeout(ctx, 2*time.Second, pollingDuration, false, func(ctx context.Context) (bool, error) {
			err = k8s.Get(ctx, rmo.Name, rmo.Namespace, rmo)
			if k8serrors.IsNotFound(err) {
				return true, nil // RouteMonitor is deleted
			}
			if err != nil {
				return false, err // Return error to stop retrying
			}
			return false, nil // RouteMonitor still exists, continue polling
		})
		Expect(err).ShouldNot(HaveOccurred(), "Couldn't delete application route monitor")

		_, err = serviceMonitorsClient.Get(ctx, routeMonitorName, metav1.GetOptions{})
		Expect(k8serrors.IsNotFound(err)).To(BeTrue(), "Sample serviceMonitor still exists, deletion of RouteMonitor didn't clean it up")
		_, err = prometheusRulesClient.Get(ctx, routeMonitorName, metav1.GetOptions{})
		Expect(k8serrors.IsNotFound(err)).To(BeTrue(), "Sample prometheusRule still exists, deletion of RouteMonitor didn't clean it up")

		DeferCleanup(func(ctx context.Context) {
			By("Cleaning up setup")
			By("Deleting test pod " + routeMonitorName)
			Expect(k8s.Delete(ctx, pod)).Should(Succeed(), "Failed to delete pod")
			By("Deleting test service " + routeMonitorName)
			Expect(k8s.Delete(ctx, svc)).Should(Succeed(), "Failed to delete service")
			By("Deleting test route " + routeMonitorName)
			Expect(k8s.Delete(ctx, appRoute)).Should(Succeed(), "Failed to delete route")
		})
	})
})
var _ = Describe("RHOBS Synthetic Monitoring", Ordered, func() {
	var (
		k8s                    *openshift.Client
		rhobsAPIURL            string
		rhobsTenant            string
		testNamespace          string
		pollingDuration        time.Duration
		probeActivationTimeout time.Duration
		oidcCredentials        *OIDCCredentials
	)

	const (
		rmoNamespace = "openshift-route-monitor-operator"
	)

	BeforeAll(func(ctx context.Context) {
		log.SetLogger(GinkgoLogr)
		var err error
		k8s, err = openshift.New(GinkgoLogr)
		Expect(err).ShouldNot(HaveOccurred(), "unable to setup k8s client")

		// Install required CRDs if they don't exist
		By("ensuring required CRDs are installed")
		err = ensureCRDsInstalled(ctx, k8s)
		Expect(err).ShouldNot(HaveOccurred(), "failed to ensure CRDs are installed")

		// Restart RMO to pick up CRDs (in case RMO was deployed before test ran)
		By("restarting RMO to ensure HostedControlPlane controller is running")
		err = restartRMODeployment(ctx, k8s)
		if err != nil {
			GinkgoLogr.Info("Warning: could not restart RMO deployment", "error", err)
		}

		// Register HyperShift and AWS VPCE schemes
		Expect(hypershiftv1beta1.AddToScheme(k8s.GetScheme())).Should(Succeed(), "unable to register HyperShift scheme")
		Expect(awsvpceapi.AddToScheme(k8s.GetScheme())).Should(Succeed(), "unable to register AWS VPCE scheme")

		// Determine environment from osde2e provider
		var environment string
		provider, err := providers.ClusterProvider()
		if err != nil {
			GinkgoLogr.Error(err, "Failed to get cluster provider, falling back to stage")
			environment = "stage"
		} else {
			environment = provider.Environment() // Returns "int", "stage", or "prod"
		}

		// Fetch OIDC credentials from environment variables
		By("fetching OIDC credentials")

		// Log which credential source is available for debugging
		if os.Getenv("EXTERNAL_SECRET_OIDC_CLIENT_ID") != "" {
			GinkgoLogr.Info("EXTERNAL_SECRET_ credentials detected")
		} else {
			GinkgoLogr.Info("No EXTERNAL_SECRET_ credentials found, test will fail")
		}

		// Get OIDC credentials from ConfigMap first, fall back to env vars if needed
		// This checks ConfigMap first, then environment variables, then creates/updates ConfigMap
		By("getting OIDC credentials from ConfigMap or environment variables")
		creds, err := getOrCreateOIDCCredentials(ctx, k8s, environment)
		Expect(err).ShouldNot(HaveOccurred(), "failed to fetch credentials")
		oidcCredentials = creds // Store for use in listRHOBSProbes

		// Set RHOBS API URL (from credentials or environment)
		if creds.ProbeAPIURL != "" {
			rhobsAPIURL = creds.ProbeAPIURL
		} else {
			rhobsAPIURL = getRHOBSAPIURL(environment)
		}

		rhobsTenant = getEnvOrDefault("RHOBS_TENANT", "hcp")
		testNamespace = getEnvOrDefault("HCP_TEST_NAMESPACE", "clusters")
		pollingDuration = 3 * time.Minute
		probeActivationTimeout = 5 * time.Minute

		// Label the test cluster as a management cluster
		By("labeling test cluster as management-cluster")
		err = labelTestClusterAsManagementCluster(ctx, k8s)
		if err != nil {
			GinkgoLogr.Info("Warning: could not label test cluster as management cluster", "error", err)
		}

		GinkgoLogr.Info("RHOBS Synthetic Monitoring test suite initialized",
			"environment", environment,
			"probeAPIURL", rhobsAPIURL,
			"tenant", rhobsTenant,
			"testNamespace", testNamespace,
			"oidcConfigured", creds.ClientID != "")
	})

	// Phase 1 Test 1: Verify RHOBS monitoring configuration
	It("has RHOBS monitoring configured", func(ctx context.Context) {
		By("checking RMO deployment exists")
		deployment := &appsv1.Deployment{}
		err := k8s.Get(ctx, "route-monitor-operator-controller-manager", rmoNamespace, deployment)
		Expect(err).ShouldNot(HaveOccurred(), "RMO deployment not found")

		By("verifying RMO deployment is ready")
		Expect(deployment.Status.ReadyReplicas).Should(BeNumerically(">", 0), "RMO deployment has no ready replicas")

		By("verifying RMO config ConfigMap has OIDC credentials")
		configMap := &corev1.ConfigMap{}
		err = k8s.Get(ctx, "route-monitor-operator-config", rmoNamespace, configMap)
		Expect(err).ShouldNot(HaveOccurred(), "RMO config ConfigMap not found")
		Expect(configMap.Data).Should(HaveKey("oidc-client-id"), "ConfigMap missing oidc-client-id")
		Expect(configMap.Data).Should(HaveKey("oidc-client-secret"), "ConfigMap missing oidc-client-secret")
		Expect(configMap.Data).Should(HaveKey("oidc-issuer-url"), "ConfigMap missing oidc-issuer-url")
		Expect(configMap.Data).Should(HaveKey("probe-api-url"), "ConfigMap missing probe-api-url")
		Expect(configMap.Data["oidc-client-id"]).ShouldNot(BeEmpty(), "oidc-client-id should not be empty")
		Expect(configMap.Data["oidc-client-secret"]).ShouldNot(BeEmpty(), "oidc-client-secret should not be empty")

		GinkgoLogr.Info("RMO deployment and configuration verified",
			"namespace", rmoNamespace,
			"readyReplicas", deployment.Status.ReadyReplicas,
			"configuredOIDC", true,
			"probeAPIURL", configMap.Data["probe-api-url"])
	})

	// Phase 1 Test 2: Create probe for public HostedControlPlane
	It("creates probe for public HostedControlPlane", func(ctx context.Context) {
		clusterID := fmt.Sprintf("test-osde2e-public-%d", time.Now().Unix())
		hcpName := fmt.Sprintf("public-hcp-%d", time.Now().UnixNano()%100000)
		// Use MC-style namespace pattern: {prefix}-{cluster-id}-{cluster-name}
		namespace := fmt.Sprintf("%s-%s-%s", testNamespace, clusterID, hcpName)

		By(fmt.Sprintf("creating MC-style namespace: %s", namespace))
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
		err := k8s.Create(ctx, ns)
		Expect(err).ShouldNot(HaveOccurred(), "failed to create namespace")

By(fmt.Sprintf("creating public HostedControlPlane with cluster ID: %s", clusterID))
		hcp := createMCStyleHCP(clusterID, hcpName, namespace, hypershiftv1beta1.Public)

		err = k8s.Create(ctx, hcp)
		Expect(err).ShouldNot(HaveOccurred(), "failed to create HostedControlPlane")

		By("setting HostedControlPlane status to Available")
		err = setHostedControlPlaneAvailable(ctx, k8s, hcp)
		Expect(err).ShouldNot(HaveOccurred(), "failed to update HostedControlPlane status")
		GinkgoLogr.Info("HCP status set to Available", "clusterID", clusterID)

		By("waiting for RMO to reconcile and create probe (up to 3 minutes)")
		var probe map[string]interface{}
		err = wait.PollUntilContextTimeout(ctx, 10*time.Second, pollingDuration, false, func(ctx context.Context) (bool, error) {
			probes, err := listRHOBSProbes(rhobsAPIURL, fmt.Sprintf("cluster-id=%s", clusterID), oidcCredentials)
			if err != nil {
				GinkgoLogr.Info("Error querying RHOBS API (will retry)", "error", err)
				return false, nil // Continue polling on error
			}

			if len(probes) > 0 {
				probe = probes[0]
				return true, nil
			}

			GinkgoLogr.Info("Waiting for probe to be created...", "clusterID", clusterID)
			return false, nil
		})
		Expect(err).ShouldNot(HaveOccurred(), "probe was not created within timeout")
		Expect(probe).ShouldNot(BeNil(), "probe should exist")

		By("validating probe configuration")
		probeID, ok := probe["id"].(string)
		Expect(ok).Should(BeTrue(), "probe ID should be a string")
		staticURL, ok := probe["static_url"].(string)
		Expect(ok).Should(BeTrue(), "probe static_url should be a string")
		labels, ok := probe["labels"].(map[string]interface{})
		Expect(ok).Should(BeTrue(), "probe labels should be a map")

		GinkgoLogr.Info("Probe created successfully",
			"probeID", probeID,
			"staticURL", staticURL,
			"labels", labels)

		// Extract hostname from HCP spec
		hostname := hcp.Spec.Services[0].ServicePublishingStrategy.Route.Hostname
		Expect(staticURL).Should(ContainSubstring(hostname), "probe URL should contain HCP hostname")
		Expect(labels["cluster-id"]).Should(Equal(clusterID), "probe should have correct cluster-id label")
		Expect(labels["private"]).Should(Equal("false"), "probe should be marked as public")

		// Optional: Wait for probe to become active
		By("optionally waiting for probe to become active")
		err = wait.PollUntilContextTimeout(ctx, 15*time.Second, probeActivationTimeout, false, func(ctx context.Context) (bool, error) {
			p, err := getRHOBSProbe(rhobsAPIURL, probeID)
			if err != nil {
				return false, nil
			}
			status, ok := p["status"].(string)
			if ok && status == "active" {
				GinkgoLogr.Info("Probe activated successfully", "probeID", probeID)
				return true, nil
			}
			GinkgoLogr.Info("Waiting for probe activation...", "currentStatus", status)
			return false, nil
		})
		if err != nil {
			GinkgoLogr.Info("Warning: probe did not reach active status within timeout (may be expected)", "error", err)
		}

		// Cleanup
		DeferCleanup(func(ctx context.Context) {
			By("cleaning up test HostedControlPlane")
			err := k8s.Delete(ctx, hcp)
			if err != nil {
				GinkgoLogr.Info("Warning: failed to delete HCP", "error", err)
			}

By("verifying probe is deleted from RHOBS API")
			err = wait.PollUntilContextTimeout(ctx, 5*time.Second, 2*time.Minute, false, func(ctx context.Context) (bool, error) {
				probes, err := listRHOBSProbes(rhobsAPIURL, fmt.Sprintf("cluster-id=%s", clusterID), oidcCredentials)
				if err != nil {
					return false, nil
				}
				if len(probes) == 0 {
					GinkgoLogr.Info("Probe successfully deleted", "clusterID", clusterID)
					return true, nil
				}
				GinkgoLogr.Info("Waiting for probe deletion...", "remainingProbes", len(probes))
				return false, nil
			})
			if err != nil {
				GinkgoLogr.Info("Warning: probe may not have been cleaned up", "clusterID", clusterID)
			}

			By("cleaning up test namespace")
			err = k8s.Delete(ctx, ns)
			if err != nil {
				GinkgoLogr.Info("Warning: failed to delete namespace", "error", err)
			}
		})
	})

	// Phase 1 Test 3: Create probe for private HostedControlPlane
	It("creates probe for private HostedControlPlane", func(ctx context.Context) {
		clusterID := fmt.Sprintf("test-osde2e-private-%d", time.Now().Unix())
		hcpName := fmt.Sprintf("private-hcp-%d", time.Now().UnixNano()%100000)
		// Use MC-style namespace pattern
		namespace := fmt.Sprintf("%s-%s-%s", testNamespace, clusterID, hcpName)

		By(fmt.Sprintf("creating MC-style namespace: %s", namespace))
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
		err := k8s.Create(ctx, ns)
		Expect(err).ShouldNot(HaveOccurred(), "failed to create namespace")

		By(fmt.Sprintf("creating private HostedControlPlane with cluster ID: %s", clusterID))
		hcp := createMCStyleHCP(clusterID, hcpName, namespace, hypershiftv1beta1.Private)

		err = k8s.Create(ctx, hcp)
		Expect(err).ShouldNot(HaveOccurred(), "failed to create HostedControlPlane")

		By("setting HostedControlPlane status to Available")
		err = setHostedControlPlaneAvailable(ctx, k8s, hcp)
		Expect(err).ShouldNot(HaveOccurred(), "failed to update HostedControlPlane status")
		GinkgoLogr.Info("Private HCP status set to Available", "clusterID", clusterID)

		By("waiting for RMO to reconcile and create probe")
		var probe map[string]interface{}
		err = wait.PollUntilContextTimeout(ctx, 10*time.Second, pollingDuration, false, func(ctx context.Context) (bool, error) {
			probes, err := listRHOBSProbes(rhobsAPIURL, fmt.Sprintf("cluster-id=%s", clusterID), oidcCredentials)
			if err != nil {
				GinkgoLogr.Info("Error querying RHOBS API (will retry)", "error", err)
				return false, nil
			}

			if len(probes) > 0 {
				probe = probes[0]
				return true, nil
			}

			GinkgoLogr.Info("Waiting for probe to be created...", "clusterID", clusterID)
			return false, nil
		})
		Expect(err).ShouldNot(HaveOccurred(), "probe was not created within timeout")
		Expect(probe).ShouldNot(BeNil(), "probe should exist")

		By("validating probe configuration for private cluster")
		probeID, ok := probe["id"].(string)
		Expect(ok).Should(BeTrue(), "probe ID should be a string")
		labels, ok := probe["labels"].(map[string]interface{})
		Expect(ok).Should(BeTrue(), "probe labels should be a map")

		GinkgoLogr.Info("Probe created successfully for private cluster",
			"probeID", probeID,
			"labels", labels)

		Expect(labels["cluster-id"]).Should(Equal(clusterID), "probe should have correct cluster-id label")
		Expect(labels["private"]).Should(Equal("true"), "probe should be marked as private")

		// Cleanup
		DeferCleanup(func(ctx context.Context) {
			By("cleaning up test resources")
			err := k8s.Delete(ctx, hcp)
			if err != nil {
				GinkgoLogr.Info("Warning: failed to delete HCP", "error", err)
			}

			By("verifying probe is deleted from RHOBS API")
			err = wait.PollUntilContextTimeout(ctx, 5*time.Second, 2*time.Minute, false, func(ctx context.Context) (bool, error) {
				probes, err := listRHOBSProbes(rhobsAPIURL, fmt.Sprintf("cluster-id=%s", clusterID), oidcCredentials)
				if err != nil {
					return false, nil
				}
				if len(probes) == 0 {
					GinkgoLogr.Info("Probe successfully deleted", "clusterID", clusterID)
					return true, nil
				}
				return false, nil
			})
			if err != nil {
				GinkgoLogr.Info("Warning: probe may not have been cleaned up", "clusterID", clusterID)
			}

			By("cleaning up test namespace")
			err = k8s.Delete(ctx, ns)
			if err != nil {
				GinkgoLogr.Info("Warning: failed to delete namespace", "error", err)
			}
		})
	})

	// Phase 1 Test 4: Probe deletion on HCP deletion (SREP-2832)
	It("deletes probe when HostedControlPlane is deleted", func(ctx context.Context) {
		clusterID := fmt.Sprintf("test-osde2e-deletion-%d", time.Now().Unix())
		hcpName := fmt.Sprintf("deletion-hcp-%d", time.Now().UnixNano()%100000)
		// Use MC-style namespace pattern
		namespace := fmt.Sprintf("%s-%s-%s", testNamespace, clusterID, hcpName)

		By(fmt.Sprintf("creating MC-style namespace: %s", namespace))
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
		err := k8s.Create(ctx, ns)
		Expect(err).ShouldNot(HaveOccurred(), "failed to create namespace")

By(fmt.Sprintf("creating HostedControlPlane for deletion test with cluster ID: %s", clusterID))
		hcp := createMCStyleHCP(clusterID, hcpName, namespace, hypershiftv1beta1.Public)

		err = k8s.Create(ctx, hcp)
		Expect(err).ShouldNot(HaveOccurred(), "failed to create HostedControlPlane")

		By("setting HostedControlPlane status to Available")
		err = setHostedControlPlaneAvailable(ctx, k8s, hcp)
		Expect(err).ShouldNot(HaveOccurred(), "failed to update HostedControlPlane status")
		GinkgoLogr.Info("HCP status set to Available for deletion test", "clusterID", clusterID)

		By("waiting for probe to be created")
		var probeID string
		err = wait.PollUntilContextTimeout(ctx, 10*time.Second, pollingDuration, false, func(ctx context.Context) (bool, error) {
			probes, err := listRHOBSProbes(rhobsAPIURL, fmt.Sprintf("cluster-id=%s", clusterID), oidcCredentials)
			if err != nil {
				return false, nil
			}
			if len(probes) > 0 {
				if id, ok := probes[0]["id"].(string); ok {
					probeID = id
					GinkgoLogr.Info("Probe created", "probeID", probeID)
					return true, nil
				}
			}
			return false, nil
		})
		Expect(err).ShouldNot(HaveOccurred(), "probe was not created within timeout")
		Expect(probeID).ShouldNot(BeEmpty(), "probe ID should be set")

		By("deleting HostedControlPlane CR")
		err = k8s.Delete(ctx, hcp)
		Expect(err).ShouldNot(HaveOccurred(), "failed to delete HostedControlPlane")

		By("waiting for probe to be deleted from RHOBS API (validates SREP-2832 fix)")
		err = wait.PollUntilContextTimeout(ctx, 5*time.Second, 2*time.Minute, false, func(ctx context.Context) (bool, error) {
			probes, err := listRHOBSProbes(rhobsAPIURL, fmt.Sprintf("cluster-id=%s", clusterID), oidcCredentials)
			if err != nil {
				// API error - continue polling
				return false, nil
			}

			if len(probes) == 0 {
				GinkgoLogr.Info("Probe successfully deleted - no orphaned probes (SREP-2832)", "clusterID", clusterID)
				return true, nil
			}

			// Check if probe is in terminating/deleted state
			for _, p := range probes {
				if status, ok := p["status"].(string); ok {
					if status == "terminating" || status == "deleted" {
						GinkgoLogr.Info("Probe in terminating state", "status", status)
						return true, nil
					}
				}
			}

			GinkgoLogr.Info("Waiting for probe deletion...", "remainingProbes", len(probes))
			return false, nil
		})
		Expect(err).ShouldNot(HaveOccurred(), "probe was not deleted - ORPHANED PROBE DETECTED (SREP-2832)")

		By("verifying HostedControlPlane is fully deleted (no stuck finalizers)")
		err = wait.PollUntilContextTimeout(ctx, 2*time.Second, 1*time.Minute, false, func(ctx context.Context) (bool, error) {
			hcpCheck := &hypershiftv1beta1.HostedControlPlane{}
			err := k8s.Get(ctx, hcpName, namespace, hcpCheck)
			if err != nil && isNotFoundError(err) {
				GinkgoLogr.Info("HostedControlPlane fully deleted", "name", hcpName)
				return true, nil
			}
			if err == nil {
				GinkgoLogr.Info("Waiting for HCP deletion...", "finalizers", hcpCheck.Finalizers)
			}
			return false, nil
		})
		Expect(err).ShouldNot(HaveOccurred(), "HostedControlPlane was not deleted - finalizer may be stuck (SREP-2966)")

		// Cleanup namespace after test
		By("cleaning up test namespace")
		err = k8s.Delete(ctx, ns)
		if err != nil {
			GinkgoLogr.Info("Warning: failed to delete namespace", "error", err)
		}
	})
})

// Helper functions for RHOBS API interaction

// getOIDCAccessToken gets an access token from the OIDC provider using client credentials flow
func getOIDCAccessToken(creds *OIDCCredentials) (string, error) {
	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", creds.ClientID)
	data.Set("client_secret", creds.ClientSecret)

	req, err := http.NewRequest("POST", creds.IssuerURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to get access token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("OIDC token endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode token response: %w", err)
	}

	return tokenResp.AccessToken, nil
}

func listRHOBSProbes(baseURL, labelSelector string, creds *OIDCCredentials) ([]map[string]interface{}, error) {
	// Get OIDC access token
	accessToken, err := getOIDCAccessToken(creds)
	if err != nil {
		return nil, fmt.Errorf("failed to get OIDC access token: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}

	reqURL := baseURL
	if labelSelector != "" {
		reqURL += "?label_selector=" + labelSelector
	}

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to query RHOBS API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("RHOBS API returned status %d: %s", resp.StatusCode, string(body))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Try to decode as array
	var probes []map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &probes); err != nil {
		// Try wrapped response format
		var wrapper map[string]interface{}
		if err2 := json.Unmarshal(bodyBytes, &wrapper); err2 == nil {
			if probesData, ok := wrapper["probes"].([]interface{}); ok {
				probes = make([]map[string]interface{}, len(probesData))
				for i, p := range probesData {
					if pm, ok := p.(map[string]interface{}); ok {
						probes[i] = pm
					}
				}
				return probes, nil
			}
		}
		return nil, fmt.Errorf("failed to decode RHOBS API response: %w", err)
	}

	return probes, nil
}

func getRHOBSProbe(baseURL, probeID string) (map[string]interface{}, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.Get(baseURL + "/" + probeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get probe: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("RHOBS API returned status %d: %s", resp.StatusCode, string(body))
	}

	var probe map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&probe); err != nil {
		return nil, fmt.Errorf("failed to decode probe response: %w", err)
	}

	return probe, nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getRHOBSAPIURL(environment string) string {
	switch environment {
	case "int": // osde2e uses "int" for integration
		return "https://rhobs.us-west-2.api.integration.openshift.com/api/metrics/v1/hcp/probes"
	case "stage":
		return "https://rhobs.us-east-1-0.api.stage.openshift.com/api/metrics/v1/hcp/probes"
	case "prod":
		// Production RHOBS not yet available - return empty string
		return ""
	default:
		// Default to stage
		return "https://rhobs.us-east-1-0.api.stage.openshift.com/api/metrics/v1/hcp/probes"
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

func isAlreadyExistsError(err error) bool {
	return k8serrors.IsAlreadyExists(err)
}

func isNotFoundError(err error) bool {
	return k8serrors.IsNotFound(err)
}

func setHostedControlPlaneAvailable(ctx context.Context, k8s *openshift.Client, hcp *hypershiftv1beta1.HostedControlPlane) error {
	// Update the status to simulate HyperShift controller behavior
	// RMO only processes HCPs with Available=true status
	// Use retry loop with optimistic concurrency control

	// Retry up to 5 times if we get "object has been modified" errors
	maxRetries := 5
	for attempt := 0; attempt < maxRetries; attempt++ {
		// Fetch the latest version
		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(hypershiftv1beta1.GroupVersion.WithKind("HostedControlPlane"))
		err := k8s.Get(ctx, hcp.Name, hcp.Namespace, u)
		if err != nil {
			return fmt.Errorf("failed to fetch HCP: %w", err)
		}

		// Update the status field
		now := metav1.Now()
		conditions := []interface{}{
			map[string]interface{}{
				"type":               string(hypershiftv1beta1.HostedControlPlaneAvailable),
				"status":             string(metav1.ConditionTrue),
				"reason":             "AsExpected",
				"message":            "HostedControlPlane is available",
				"lastTransitionTime": now.UTC().Format(time.RFC3339),
			},
		}

		status := map[string]interface{}{
			"conditions": conditions,
		}
		if err := unstructured.SetNestedField(u.Object, status, "status"); err != nil {
			return fmt.Errorf("failed to set status field: %w", err)
		}

		// Try to update
		err = k8s.Update(ctx, u)
		if err == nil {
			// Success!
			return nil
		}

		// Check if it's a conflict error
		if strings.Contains(err.Error(), "object has been modified") ||
		   strings.Contains(err.Error(), "the object has been modified") {
			// Retry after a short delay
			GinkgoLogr.V(1).Info("HCP was modified, retrying status update", "attempt", attempt+1, "maxRetries", maxRetries)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// Some other error, return it
		return fmt.Errorf("failed to update HCP status: %w", err)
	}

	return fmt.Errorf("failed to update HCP status after %d retries", maxRetries)
}

func createMCStyleHCP(clusterID, name, namespace string, endpointAccess hypershiftv1beta1.AWSEndpointAccessType) *hypershiftv1beta1.HostedControlPlane {
	// Create HCP matching Management Cluster patterns observed in staging
	hostname := fmt.Sprintf("api.%s.test.devshift.org", name)

	return &hypershiftv1beta1.HostedControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				// MC standard labels
				"api.openshift.com/id":          clusterID,
				"api.openshift.com/name":        name,
				"api.openshift.com/environment": "staging",
				"test-type":                     "osde2e-rhobs-synthetics",
			},
			Annotations: map[string]string{
				// MC standard annotation pointing to HostedCluster
				"hypershift.openshift.io/cluster": fmt.Sprintf("%s/%s", namespace, name),
				// Skip all infrastructure checks for osde2e test HCPs (health check, VPC endpoint, internal monitoring)
				"routemonitor.openshift.io/osde2e-testing": "true",
			},
		},
		Spec: hypershiftv1beta1.HostedControlPlaneSpec{
			ClusterID: clusterID,
			Platform: hypershiftv1beta1.PlatformSpec{
				Type: hypershiftv1beta1.AWSPlatform,
				AWS: &hypershiftv1beta1.AWSPlatformSpec{
					Region:         "us-west-2",
					EndpointAccess: endpointAccess,
				},
			},
			Services: []hypershiftv1beta1.ServicePublishingStrategyMapping{
				{
					Service: hypershiftv1beta1.APIServer,
					ServicePublishingStrategy: hypershiftv1beta1.ServicePublishingStrategy{
						Type: hypershiftv1beta1.Route,
						Route: &hypershiftv1beta1.RoutePublishingStrategy{
							Hostname: hostname,
						},
					},
				},
			},
		},
	}
}

// restartRMODeployment restarts the RMO deployment to pick up newly installed CRDs
func restartRMODeployment(ctx context.Context, k8s *openshift.Client) error {
	const rmoNamespace = "openshift-route-monitor-operator"

	// List all deployments in the RMO namespace to find the controller-manager
	deploymentList := &appsv1.DeploymentList{}
	err := k8s.List(ctx, deploymentList)
	if err != nil {
		return fmt.Errorf("failed to list deployments: %w", err)
	}

	var rmoDeployment *appsv1.Deployment
	for i := range deploymentList.Items {
		if deploymentList.Items[i].Namespace == rmoNamespace &&
			strings.Contains(deploymentList.Items[i].Name, "route-monitor-operator-controller-manager") {
			rmoDeployment = &deploymentList.Items[i]
			break
		}
	}

	if rmoDeployment == nil {
		return fmt.Errorf("RMO deployment not found in namespace %s", rmoNamespace)
	}

	// List all pods to find RMO pods
	podList := &corev1.PodList{}
	err = k8s.List(ctx, podList)
	if err != nil {
		return fmt.Errorf("failed to list pods: %w", err)
	}

	// Delete RMO pods matching the deployment's label selector
	for i := range podList.Items {
		pod := &podList.Items[i]
		if pod.Namespace != rmoNamespace {
			continue
		}

		// Check if pod matches deployment's selector
		labelSelector := labels.SelectorFromSet(rmoDeployment.Spec.Selector.MatchLabels)
		if labelSelector.Matches(labels.Set(pod.Labels)) {
			GinkgoLogr.Info("Deleting RMO pod to pick up CRDs", "pod", pod.Name)
			if err := k8s.Delete(ctx, pod); err != nil {
				return fmt.Errorf("failed to delete pod %s: %w", pod.Name, err)
			}
		}
	}

	// Wait for new pod to be ready
	GinkgoLogr.Info("Waiting for RMO deployment to be ready after restart")
	err = wait.PollUntilContextTimeout(ctx, 2*time.Second, 60*time.Second, false, func(ctx context.Context) (bool, error) {
		deployment := &appsv1.Deployment{}
		err := k8s.Get(ctx, rmoDeployment.Name, rmoNamespace, deployment)
		if err != nil {
			return false, err
		}
		return deployment.Status.ReadyReplicas > 0, nil
	})
	if err != nil {
		return fmt.Errorf("RMO deployment did not become ready: %w", err)
	}

	GinkgoLogr.Info("RMO deployment restarted successfully")
	return nil
}

// ensureCRDsInstalled checks if required CRDs exist and installs them if missing
func ensureCRDsInstalled(ctx context.Context, k8s *openshift.Client) error {
	crdsToInstall := []struct {
		name     string
		yamlDef  string
	}{
		{
			name:    "hostedcontrolplanes.hypershift.openshift.io",
			yamlDef: hostedControlPlaneCRD,
		},
		{
			name:    "vpcendpoints.avo.openshift.io",
			yamlDef: vpcEndpointCRD,
		},
		{
			name:    "clusterdeployments.hive.openshift.io",
			yamlDef: clusterDeploymentCRD,
		},
	}

	var missingCRDs []string

	for _, crd := range crdsToInstall {
		// Check if CRD already exists
		exists, err := crdExists(ctx, k8s, crd.name)
		if err != nil {
			return fmt.Errorf("failed to check if CRD %s exists: %w", crd.name, err)
		}

		if exists {
			GinkgoLogr.Info("CRD already exists, skipping installation", "crd", crd.name)
			continue
		}

		// CRD doesn't exist, try to install it from embedded YAML
		GinkgoLogr.Info("Installing CRD from embedded definition", "crd", crd.name)
		if err := installCRDFromYAML(ctx, k8s, crd.yamlDef); err != nil {
			// Check if CRD already exists (race condition or detection failure)
			if strings.Contains(err.Error(), "AlreadyExists") || strings.Contains(err.Error(), "already exists") {
				GinkgoLogr.Info("CRD already exists (detected during create), skipping", "crd", crd.name)
				continue
			}
			// Check if this is a permission error
			if strings.Contains(err.Error(), "Forbidden") || strings.Contains(err.Error(), "forbidden") {
				GinkgoLogr.Error(err, "Permission denied - cannot install CRD", "crd", crd.name)
				missingCRDs = append(missingCRDs, crd.name)
				continue
			}
			// Other error - fail immediately
			return fmt.Errorf("failed to install CRD %s: %w", crd.name, err)
		}
		GinkgoLogr.Info("Successfully installed CRD", "crd", crd.name)
	}

	// If we couldn't install CRDs due to permissions, provide helpful error
	if len(missingCRDs) > 0 {
		return fmt.Errorf(`
Missing required CRDs that could not be installed due to insufficient permissions:
  %s

You need cluster-admin privileges to install CRDs. Please ask a cluster administrator to run:

  kubectl apply -f - <<'EOF'
%s%s
EOF

Alternatively, if these CRDs are already installed in a different namespace or with a different name,
you may need to adjust the test configuration.
`, strings.Join(missingCRDs, "\n  "), hostedControlPlaneCRD, vpcEndpointCRD)
	}

	return nil
}

// crdExists checks if a CRD with the given name exists
func crdExists(ctx context.Context, k8s *openshift.Client, crdName string) (bool, error) {
	// Use unstructured client to check for CRD existence
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apiextensions.k8s.io",
		Version: "v1",
		Kind:    "CustomResourceDefinition",
	})

	err := k8s.Get(ctx, crdName, "", u)
	if err != nil {
		// Check if it's a "not found" error
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "NotFound") {
			return false, nil
		}
		// Some other error occurred
		return false, err
	}
	// No error means CRD exists
	return true, nil
}

// installCRDFromYAML applies a CRD from embedded YAML definition
func installCRDFromYAML(ctx context.Context, k8s *openshift.Client, crdYAML string) error {
	// Parse YAML into unstructured object
	obj := &unstructured.Unstructured{}
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(crdYAML), 4096)
	if err := decoder.Decode(obj); err != nil {
		return fmt.Errorf("failed to decode YAML: %w", err)
	}

	// Use the k8s client to create the CRD
	if err := k8s.Create(ctx, obj); err != nil {
		return fmt.Errorf("failed to create CRD: %w", err)
	}

	GinkgoLogr.V(1).Info("CRD installed successfully", "name", obj.GetName())
	return nil
}

// OIDCCredentials holds OIDC credentials for RMO
type OIDCCredentials struct {
	ClientID     string
	ClientSecret string
	IssuerURL    string
	ProbeAPIURL  string
}

// getOrCreateOIDCCredentials fetches credentials from ConfigMap first, then environment variables
// If ConfigMap doesn't exist or has incomplete credentials, falls back to environment variables
// and creates/updates the ConfigMap with those credentials
func getOrCreateOIDCCredentials(ctx context.Context, k8s *openshift.Client, environment string) (*OIDCCredentials, error) {
	// First, try to get credentials from existing ConfigMap
	creds, err := getOIDCCredentialsFromConfigMap(ctx, k8s)
	if err == nil && creds != nil {
		GinkgoLogr.Info("Using OIDC credentials from existing ConfigMap")
		return creds, nil
	}
	if err != nil && !k8serrors.IsNotFound(err) {
		GinkgoLogr.Info("Warning: failed to check ConfigMap, falling back to environment variables", "error", err)
	}

	// ConfigMap doesn't exist or is incomplete, fall back to environment variables
	GinkgoLogr.Info("ConfigMap not found or incomplete, fetching credentials from environment variables")
	creds, err = getOIDCCredentials(ctx, environment)
	if err != nil {
		return nil, err
	}

	// Create/update ConfigMap with credentials from environment variables
	GinkgoLogr.Info("Creating/updating RMO config ConfigMap with credentials from environment variables")
	if err := createRMOConfigMap(ctx, k8s, creds); err != nil {
		return nil, fmt.Errorf("failed to create/update ConfigMap: %w", err)
	}

	return creds, nil
}

// getOIDCCredentialsFromConfigMap fetches RMO credentials from the ConfigMap
// Returns nil if ConfigMap doesn't exist or has incomplete credentials
func getOIDCCredentialsFromConfigMap(ctx context.Context, k8s *openshift.Client) (*OIDCCredentials, error) {
	existingCM := &corev1.ConfigMap{}
	err := k8s.Get(ctx, "route-monitor-operator-config", "openshift-route-monitor-operator", existingCM)
	if err != nil {
		return nil, err
	}

	// Check if all required fields are present
	clientID := existingCM.Data["oidc-client-id"]
	clientSecret := existingCM.Data["oidc-client-secret"]
	issuerURL := existingCM.Data["oidc-issuer-url"]
	probeAPIURL := existingCM.Data["probe-api-url"]

	if clientID == "" || clientSecret == "" || issuerURL == "" {
		GinkgoLogr.Info("ConfigMap exists but has incomplete credentials", "has_client_id", clientID != "", "has_client_secret", clientSecret != "", "has_issuer_url", issuerURL != "")
		return nil, fmt.Errorf("incomplete credentials in ConfigMap")
	}

	return &OIDCCredentials{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		IssuerURL:    issuerURL,
		ProbeAPIURL:  probeAPIURL,
	}, nil
}

// getOIDCCredentials fetches RMO credentials from environment variables
// Uses EXTERNAL_SECRET_ env vars (same for both local testing and CI/CD via SDCICD-1739)
func getOIDCCredentials(ctx context.Context, environment string) (*OIDCCredentials, error) {
	// Check EXTERNAL_SECRET_ environment variables (used for both local and osde2e/CI)
	// In CI/CD, these are auto-loaded by osde2e from app-interface (see SDCICD-1739)
	// For local testing, export these same variables
	if clientID := os.Getenv("EXTERNAL_SECRET_OIDC_CLIENT_ID"); clientID != "" {
		GinkgoLogr.Info("Using OIDC credentials from EXTERNAL_SECRET_ environment variables")
		return &OIDCCredentials{
			ClientID:     clientID,
			ClientSecret: os.Getenv("EXTERNAL_SECRET_OIDC_CLIENT_SECRET"),
			IssuerURL:    os.Getenv("EXTERNAL_SECRET_OIDC_ISSUER_URL"),
			ProbeAPIURL:  os.Getenv("PROBE_API_URL"),
		}, nil
	}

	// No credentials found
	return nil, fmt.Errorf(`no OIDC credentials found in environment variables

Please set the following environment variables:
  export EXTERNAL_SECRET_OIDC_CLIENT_ID="your-client-id"
  export EXTERNAL_SECRET_OIDC_CLIENT_SECRET="your-client-secret"
  export EXTERNAL_SECRET_OIDC_ISSUER_URL="your-issuer-url"

Optional (will auto-detect based on environment if not set):
  export PROBE_API_URL="https://rhobs.us-east-1-0.api.stage.openshift.com/api/metrics/v1/hcp/probes"

Note: In osde2e/CI, the EXTERNAL_SECRET_* variables are automatically loaded from app-interface (SDCICD-1739).`)
}

// createRMOConfigMap creates the RMO config ConfigMap with OIDC credentials
func createRMOConfigMap(ctx context.Context, k8s *openshift.Client, creds *OIDCCredentials) error {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "route-monitor-operator-config",
			Namespace: "openshift-route-monitor-operator",
		},
		Data: map[string]string{
			"probe-api-url":      creds.ProbeAPIURL,
			"oidc-client-id":     creds.ClientID,
			"oidc-client-secret": creds.ClientSecret,
			"oidc-issuer-url":    creds.IssuerURL,
		},
	}

	// Check if ConfigMap already exists
	existingCM := &corev1.ConfigMap{}
	err := k8s.Get(ctx, "route-monitor-operator-config", "openshift-route-monitor-operator", existingCM)
	if err == nil {
		// ConfigMap exists, update it
		GinkgoLogr.Info("ConfigMap already exists, updating", "name", "route-monitor-operator-config")
		configMap.ResourceVersion = existingCM.ResourceVersion
		if err := k8s.Update(ctx, configMap); err != nil {
			return fmt.Errorf("failed to update ConfigMap: %w", err)
		}
		return nil
	} else if !k8serrors.IsNotFound(err) {
		return fmt.Errorf("failed to check for existing ConfigMap: %w", err)
	}

	// ConfigMap doesn't exist, create it
	GinkgoLogr.Info("Creating RMO config ConfigMap with OIDC credentials", "namespace", "openshift-route-monitor-operator")
	if err := k8s.Create(ctx, configMap); err != nil {
		return fmt.Errorf("failed to create ConfigMap: %w", err)
	}

	return nil
}

// labelTestClusterAsManagementCluster labels the cluster the test is running on as a management cluster
func labelTestClusterAsManagementCluster(ctx context.Context, k8s *openshift.Client) error {
	GinkgoLogr.Info("Looking for ClusterDeployment to label as management-cluster")

	// List all ClusterDeployments to find the one for this cluster
	cdList := &unstructured.UnstructuredList{}
	cdList.SetAPIVersion("hive.openshift.io/v1")
	cdList.SetKind("ClusterDeploymentList")

	err := k8s.List(ctx, cdList)
	if err != nil {
		return fmt.Errorf("failed to list ClusterDeployments: %w", err)
	}

	if len(cdList.Items) == 0 {
		return fmt.Errorf("no ClusterDeployments found in cluster")
	}

	// Label all ClusterDeployments we find (typically there should only be one for the test cluster)
	for idx := range cdList.Items {
		cd := &cdList.Items[idx]

		// Add the management-cluster label
		labels := cd.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		labels["ext-hypershift.openshift.io/cluster-type"] = "management-cluster"
		cd.SetLabels(labels)

		// Update the ClusterDeployment
		err = k8s.Update(ctx, cd)
		if err != nil {
			GinkgoLogr.Info("Warning: failed to update ClusterDeployment",
				"name", cd.GetName(),
				"namespace", cd.GetNamespace(),
				"error", err)
			continue
		}

		GinkgoLogr.Info("Successfully labeled ClusterDeployment as management-cluster",
			"name", cd.GetName(),
			"namespace", cd.GetNamespace())
	}

	return nil
}
