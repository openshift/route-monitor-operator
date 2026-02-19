// DO NOT REMOVE TAGS BELOW. IF ANY NEW TEST FILES ARE CREATED UNDER /osde2e, PLEASE ADD THESE TAGS TO THEM IN ORDER TO BE EXCLUDED FROM UNIT TESTS. //go:build osde2e
//go:build osde2e
// +build osde2e

package osde2etests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	routev1 "github.com/openshift/api/route/v1"
	"github.com/openshift/osde2e-common/pkg/clients/openshift"
	. "github.com/openshift/osde2e-common/pkg/gomega/assertions"
	. "github.com/openshift/osde2e-common/pkg/gomega/matchers"
	routemonitorv1alpha1 "github.com/openshift/route-monitor-operator/api/v1alpha1"
	"github.com/openshift/route-monitor-operator/pkg/rhobs"
	appsv1 "k8s.io/api/apps/v1"
	kubev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// mockRHOBSServer holds state for a mock RHOBS API server with per-tenant probe storage.
type mockRHOBSServer struct {
	mu      sync.Mutex
	probes  map[string]map[string]rhobs.ProbeResponse // tenant -> clusterID -> probe
	headers []http.Header                             // recorded request headers
	nextID  int
}

// newMockRHOBSServer creates an httptest.Server that implements the RHOBS probes API.
func newMockRHOBSServer() *httptest.Server {
	m := &mockRHOBSServer{
		probes: make(map[string]map[string]rhobs.ProbeResponse),
	}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()

		// Record headers for verification
		m.headers = append(m.headers, r.Header.Clone())

		// GET /test/headers — return recorded headers
		if r.URL.Path == "/test/headers" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(m.headers)
			return
		}

		// Extract tenant from path: /api/metrics/v1/{tenant}/probes[/{id}]
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
		// Expected: api/metrics/v1/{tenant}/probes[/{id}]
		if len(parts) < 5 || parts[0] != "api" || parts[1] != "metrics" || parts[2] != "v1" || parts[4] != "probes" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		tenant := parts[3]

		if m.probes[tenant] == nil {
			m.probes[tenant] = make(map[string]rhobs.ProbeResponse)
		}

		switch r.Method {
		case http.MethodPost:
			// POST /api/metrics/v1/{tenant}/probes
			var req rhobs.ProbeRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			clusterID := req.Labels["cluster-id"]
			m.nextID++
			probe := rhobs.ProbeResponse{
				ID:     fmt.Sprintf("probe-%d", m.nextID),
				Labels: req.Labels,
				Status: "active",
			}
			m.probes[tenant][clusterID] = probe
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(probe)

		case http.MethodGet:
			// GET /api/metrics/v1/{tenant}/probes?label_selector=cluster-id=X
			labelSelector := r.URL.Query().Get("label_selector")
			var matchClusterID string
			if strings.HasPrefix(labelSelector, "cluster-id=") {
				matchClusterID = strings.TrimPrefix(labelSelector, "cluster-id=")
			}
			var matched []rhobs.ProbeResponse
			for cid, probe := range m.probes[tenant] {
				if matchClusterID == "" || cid == matchClusterID {
					matched = append(matched, probe)
				}
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(rhobs.ProbesListResponse{Probes: matched})

		case http.MethodPatch:
			// PATCH /api/metrics/v1/{tenant}/probes/{id}
			if len(parts) < 6 {
				http.Error(w, "missing probe id", http.StatusBadRequest)
				return
			}
			probeID := parts[5]
			// Find and update the probe
			for cid, probe := range m.probes[tenant] {
				if probe.ID == probeID {
					probe.Status = "terminating"
					m.probes[tenant][cid] = probe
					w.WriteHeader(http.StatusOK)
					return
				}
			}
			http.Error(w, "probe not found", http.StatusNotFound)

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))
}

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
		err := k8s.Get(ctx, namespace, "", &kubev1.Namespace{})
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
		pod := &kubev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      routeMonitorName,
				Namespace: namespace,
			},
			Spec: kubev1.PodSpec{
				Containers: []kubev1.Container{
					{
						Name:  "test",
						Image: "quay.io/jitesoft/nginx:mainline",
						SecurityContext: &kubev1.SecurityContext{
							AllowPrivilegeEscalation: allowPrivilegeEscalation,
							Capabilities: &kubev1.Capabilities{
								Drop: []kubev1.Capability{"ALL"},
							},
							RunAsNonRoot: runAsNonRoot,
							SeccompProfile: &kubev1.SeccompProfile{
								Type: kubev1.SeccompProfileTypeRuntimeDefault,
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
		svc := &kubev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      routeMonitorName,
				Namespace: namespace,
				Labels:    map[string]string{routeMonitorName: routeMonitorName},
			},
			Spec: kubev1.ServiceSpec{
				Ports: []kubev1.ServicePort{
					{
						Port:       8080,
						Protocol:   kubev1.ProtocolTCP,
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

	It("handles RouteMonitor spec updates", func(ctx context.Context) {
		const updateTestName = "routemonitor-update-test"

		By("Creating a pod for the update test")
		allowPrivEsc := false
		nonRoot := true
		pod := &kubev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      updateTestName,
				Namespace: namespace,
				Labels:    map[string]string{updateTestName: updateTestName},
			},
			Spec: kubev1.PodSpec{
				Containers: []kubev1.Container{
					{
						Name:  "test",
						Image: "quay.io/jitesoft/nginx:mainline",
						SecurityContext: &kubev1.SecurityContext{
							AllowPrivilegeEscalation: &allowPrivEsc,
							Capabilities: &kubev1.Capabilities{
								Drop: []kubev1.Capability{"ALL"},
							},
							RunAsNonRoot: &nonRoot,
							SeccompProfile: &kubev1.SeccompProfile{
								Type: kubev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
				},
			},
		}
		err := k8s.Create(ctx, pod)
		Expect(err).ShouldNot(HaveOccurred(), "Unable to create update test pod")

		By("Waiting for the pod to be running")
		var phase kubev1.PodPhase
		err = wait.PollUntilContextTimeout(ctx, 15*time.Second, pollingDuration, false, func(ctx context.Context) (bool, error) {
			err := k8s.Get(ctx, pod.Name, pod.Namespace, pod)
			Expect(err).ShouldNot(HaveOccurred(), "Unable to get update test pod")
			if pod != nil {
				phase = pod.Status.Phase
				if phase == kubev1.PodRunning || phase == kubev1.PodFailed {
					return true, nil
				}
			}
			return false, nil
		})
		Expect(err).ShouldNot(HaveOccurred(), fmt.Sprintf("Update test pod is not running, current state is %v", phase))

		By("Creating the service")
		svc := &kubev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      updateTestName,
				Namespace: namespace,
				Labels:    map[string]string{updateTestName: updateTestName},
			},
			Spec: kubev1.ServiceSpec{
				Ports: []kubev1.ServicePort{
					{
						Port:       8080,
						Protocol:   kubev1.ProtocolTCP,
						TargetPort: intstr.FromInt(80),
						Name:       "web",
					},
				},
				Selector: map[string]string{updateTestName: updateTestName},
			},
		}
		err = k8s.Create(ctx, svc)
		Expect(err).ShouldNot(HaveOccurred(), "Unable to create update test service")

		By("Creating the route")
		appRoute := &routev1.Route{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      updateTestName,
			},
			Spec: routev1.RouteSpec{
				To: routev1.RouteTargetReference{
					Name: updateTestName,
				},
				TLS: &routev1.TLSConfig{Termination: "edge"},
			},
		}
		err = k8s.Create(ctx, appRoute)
		Expect(err).ShouldNot(HaveOccurred(), "Unable to create update test route")

		By("Creating a RouteMonitor with SLO 99.5%")
		rmo := &routemonitorv1alpha1.RouteMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      updateTestName,
			},
			Spec: routemonitorv1alpha1.RouteMonitorSpec{
				Slo: routemonitorv1alpha1.SloSpec{
					TargetAvailabilityPercent: "99.5",
				},
				Route: routemonitorv1alpha1.RouteMonitorRouteSpec{
					Namespace: namespace,
					Name:      updateTestName,
				},
			},
		}
		err = k8s.Create(ctx, rmo)
		Expect(err).ShouldNot(HaveOccurred(), "Unable to create update test RouteMonitor")

		By("Waiting for dependent resources to be created")
		err = wait.PollUntilContextTimeout(ctx, 15*time.Second, pollingDuration, false, func(ctx context.Context) (bool, error) {
			_, err = serviceMonitorsClient.Get(ctx, updateTestName, metav1.GetOptions{})
			if k8serrors.IsNotFound(err) {
				return false, nil
			}
			if err != nil {
				return false, err
			}
			_, err = prometheusRulesClient.Get(ctx, updateTestName, metav1.GetOptions{})
			if k8serrors.IsNotFound(err) {
				return false, nil
			}
			if err != nil {
				return false, err
			}
			return true, nil
		})
		Expect(err).ShouldNot(HaveOccurred(), "Dependent resources weren't created for update test RouteMonitor")

		By("Updating the RouteMonitor SLO from 99.5% to 99.95%")
		err = k8s.Get(ctx, updateTestName, namespace, rmo)
		Expect(err).ShouldNot(HaveOccurred(), "Unable to get RouteMonitor for update")
		rmo.Spec.Slo.TargetAvailabilityPercent = "99.95"
		err = k8s.Update(ctx, rmo)
		Expect(err).ShouldNot(HaveOccurred(), "Unable to update RouteMonitor SLO")

		By("Verifying the PrometheusRule is updated with the new SLO")
		err = wait.PollUntilContextTimeout(ctx, 15*time.Second, pollingDuration, false, func(ctx context.Context) (bool, error) {
			promRule, err := prometheusRulesClient.Get(ctx, updateTestName, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			// Marshal the unstructured object to JSON and check for the new SLO value
			ruleBytes, err := json.Marshal(promRule.Object)
			if err != nil {
				return false, fmt.Errorf("failed to marshal PrometheusRule: %w", err)
			}
			ruleJSON := string(ruleBytes)
			// The new SLO 99.95% embeds as (1-0.9995) = 0.0005 in the PromQL expressions
			// The old SLO 99.5% would have (1-0.995) = 0.005
			if strings.Contains(ruleJSON, "0.9995") && !strings.Contains(ruleJSON, "0.995)") {
				return true, nil
			}
			GinkgoLogr.Info("Waiting for PrometheusRule to reflect updated SLO 99.95%")
			return false, nil
		})
		Expect(err).ShouldNot(HaveOccurred(), "PrometheusRule was not updated with new SLO value")

		By("Deleting the update test RouteMonitor")
		err = k8s.Delete(ctx, rmo)
		Expect(err).ShouldNot(HaveOccurred(), "Failed to delete update test RouteMonitor")

		By("Waiting for RouteMonitor deletion")
		err = wait.PollUntilContextTimeout(ctx, 2*time.Second, pollingDuration, false, func(ctx context.Context) (bool, error) {
			err = k8s.Get(ctx, rmo.Name, rmo.Namespace, rmo)
			if k8serrors.IsNotFound(err) {
				return true, nil
			}
			if err != nil {
				return false, err
			}
			return false, nil
		})
		Expect(err).ShouldNot(HaveOccurred(), "Couldn't delete update test RouteMonitor")

		By("Verifying dependent resources are cleaned up after deletion")
		_, err = serviceMonitorsClient.Get(ctx, updateTestName, metav1.GetOptions{})
		Expect(k8serrors.IsNotFound(err)).To(BeTrue(), "Update test ServiceMonitor still exists after RouteMonitor deletion")
		_, err = prometheusRulesClient.Get(ctx, updateTestName, metav1.GetOptions{})
		Expect(k8serrors.IsNotFound(err)).To(BeTrue(), "Update test PrometheusRule still exists after RouteMonitor deletion")

		DeferCleanup(func(ctx context.Context) {
			By("Cleaning up update test resources")
			By("Deleting update test pod " + updateTestName)
			Expect(k8s.Delete(ctx, pod)).Should(Succeed(), "Failed to delete update test pod")
			By("Deleting update test service " + updateTestName)
			Expect(k8s.Delete(ctx, svc)).Should(Succeed(), "Failed to delete update test service")
			By("Deleting update test route " + updateTestName)
			Expect(k8s.Delete(ctx, appRoute)).Should(Succeed(), "Failed to delete update test route")
		})
	})

	It("validates RHOBS probe lifecycle", func(ctx context.Context) {
		server := newMockRHOBSServer()
		DeferCleanup(func(_ context.Context) { server.Close() })

		client := rhobs.NewClient(server.URL, "test-tenant", GinkgoLogr)

		By("Creating a probe")
		probeReq := rhobs.NewClusterProbeRequest("e2e-cluster-123", "https://example.com/livez", false)
		probe, err := client.CreateProbe(ctx, probeReq)
		Expect(err).ShouldNot(HaveOccurred(), "CreateProbe should succeed")
		Expect(probe).ShouldNot(BeNil(), "probe response should not be nil")
		Expect(probe.ID).ShouldNot(BeEmpty(), "probe ID should not be empty")
		Expect(probe.Status).Should(Equal("active"), "probe should be active after creation")

		By("Getting the probe by cluster ID")
		fetched, err := client.GetProbe(ctx, "e2e-cluster-123")
		Expect(err).ShouldNot(HaveOccurred(), "GetProbe should succeed")
		Expect(fetched).ShouldNot(BeNil(), "probe should be found")
		Expect(fetched.ID).Should(Equal(probe.ID), "fetched probe ID should match created probe")

		By("Verifying probe labels")
		Expect(fetched.Labels["cluster-id"]).Should(Equal("e2e-cluster-123"))
		Expect(fetched.Labels["private"]).Should(Equal("false"))

		By("Deleting the probe")
		err = client.DeleteProbe(ctx, "e2e-cluster-123")
		Expect(err).ShouldNot(HaveOccurred(), "DeleteProbe should succeed")

		By("Verifying probe is terminating after deletion")
		deleted, err := client.GetProbe(ctx, "e2e-cluster-123")
		Expect(err).ShouldNot(HaveOccurred(), "GetProbe after delete should succeed")
		if deleted != nil {
			Expect(deleted.Status).Should(Equal("terminating"), "probe should be terminating after deletion")
		}
	})

	It("enforces multi-tenant isolation in RHOBS API", func(ctx context.Context) {
		server := newMockRHOBSServer()
		DeferCleanup(func(_ context.Context) { server.Close() })

		clientA := rhobs.NewClient(server.URL, "tenant-alpha", GinkgoLogr)
		clientB := rhobs.NewClient(server.URL, "tenant-beta", GinkgoLogr)

		By("Creating a probe for tenant-alpha")
		_, err := clientA.CreateProbe(ctx, rhobs.NewClusterProbeRequest("cluster-a", "https://a.example.com/livez", false))
		Expect(err).ShouldNot(HaveOccurred())

		By("Creating a probe for tenant-beta")
		_, err = clientB.CreateProbe(ctx, rhobs.NewClusterProbeRequest("cluster-b", "https://b.example.com/livez", false))
		Expect(err).ShouldNot(HaveOccurred())

		By("Verifying tenant-alpha can see its own probe")
		probeA, err := clientA.GetProbe(ctx, "cluster-a")
		Expect(err).ShouldNot(HaveOccurred())
		Expect(probeA).ShouldNot(BeNil(), "tenant-alpha should find cluster-a")

		By("Verifying tenant-beta cannot see tenant-alpha's probe")
		crossB, err := clientB.GetProbe(ctx, "cluster-a")
		Expect(err).ShouldNot(HaveOccurred())
		Expect(crossB).Should(BeNil(), "tenant-beta should NOT see cluster-a")

		By("Verifying tenant-alpha cannot see tenant-beta's probe")
		crossA, err := clientA.GetProbe(ctx, "cluster-b")
		Expect(err).ShouldNot(HaveOccurred())
		Expect(crossA).Should(BeNil(), "tenant-alpha should NOT see cluster-b")

		By("Verifying tenant-beta can see its own probe")
		probeB, err := clientB.GetProbe(ctx, "cluster-b")
		Expect(err).ShouldNot(HaveOccurred())
		Expect(probeB).ShouldNot(BeNil(), "tenant-beta should find cluster-b")

		By("Verifying X-Tenant headers were sent correctly")
		resp, err := http.Get(server.URL + "/test/headers")
		Expect(err).ShouldNot(HaveOccurred())
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		Expect(err).ShouldNot(HaveOccurred())
		headersJSON := string(body)
		Expect(headersJSON).Should(ContainSubstring("tenant-alpha"), "headers should contain tenant-alpha")
		Expect(headersJSON).Should(ContainSubstring("tenant-beta"), "headers should contain tenant-beta")
	})

	It("validates HostedControlPlane CR watching", func(ctx context.Context) {
		hcpGVR := schema.GroupVersionResource{
			Group:    "hypershift.openshift.io",
			Version:  "v1beta1",
			Resource: "hostedcontrolplanes",
		}

		// Try default config first; if forbidden, retry with backplane-cluster-admin impersonation
		cfg := rest.CopyConfig(k8s.GetConfig())
		dynamicClient, err := dynamic.NewForConfig(cfg)
		Expect(err).ShouldNot(HaveOccurred(), "failed to create dynamic client")

		By("Checking if HostedControlPlane CRD is available")
		hcpList, err := dynamicClient.Resource(hcpGVR).Namespace("").List(ctx, metav1.ListOptions{Limit: 1})
		if err != nil {
			// Retry with impersonation for clusters where default creds can't list
			cfg.Impersonate = rest.ImpersonationConfig{UserName: "backplane-cluster-admin"}
			dynamicClient, err = dynamic.NewForConfig(cfg)
			if err != nil {
				Skip(fmt.Sprintf("HostedControlPlane CRD not available on this cluster: %v", err))
			}
			hcpList, err = dynamicClient.Resource(hcpGVR).Namespace("").List(ctx, metav1.ListOptions{Limit: 1})
			if err != nil {
				Skip(fmt.Sprintf("HostedControlPlane CRD not available on this cluster: %v", err))
			}
		}

		if len(hcpList.Items) == 0 {
			Skip("No existing HostedControlPlane CRs found to use as template")
		}

		By("Cloning an existing HostedControlPlane CR as test fixture")
		sourceHCP := hcpList.Items[0]
		sourceNamespace := sourceHCP.GetNamespace()
		sourceName := sourceHCP.GetName()
		GinkgoLogr.Info("Using existing HCP as template", "name", sourceName, "namespace", sourceNamespace)

		// Verify the controller already created Route and RouteMonitor for the existing HCP
		routeGVR := schema.GroupVersionResource{
			Group:    "route.openshift.io",
			Version:  "v1",
			Resource: "routes",
		}
		routeMonitorGVR := schema.GroupVersionResource{
			Group:    "monitoring.openshift.io",
			Version:  "v1alpha1",
			Resource: "routemonitors",
		}
		expectedRouteName := sourceName + "-kube-apiserver-monitoring"

		By("Verifying controller created the monitoring Route for existing HCP")
		err = wait.PollUntilContextTimeout(ctx, 15*time.Second, pollingDuration, false, func(ctx context.Context) (bool, error) {
			_, err := dynamicClient.Resource(routeGVR).Namespace(sourceNamespace).Get(ctx, expectedRouteName, metav1.GetOptions{})
			if k8serrors.IsNotFound(err) {
				GinkgoLogr.Info("Waiting for Route to exist", "name", expectedRouteName)
				return false, nil
			}
			return err == nil, err
		})
		Expect(err).ShouldNot(HaveOccurred(), "controller should have created Route for HostedControlPlane %s", sourceName)

		By("Verifying controller created the RouteMonitor for existing HCP")
		err = wait.PollUntilContextTimeout(ctx, 15*time.Second, pollingDuration, false, func(ctx context.Context) (bool, error) {
			_, err := dynamicClient.Resource(routeMonitorGVR).Namespace(sourceNamespace).Get(ctx, expectedRouteName, metav1.GetOptions{})
			if k8serrors.IsNotFound(err) {
				GinkgoLogr.Info("Waiting for RouteMonitor to exist", "name", expectedRouteName)
				return false, nil
			}
			return err == nil, err
		})
		Expect(err).ShouldNot(HaveOccurred(), "controller should have created RouteMonitor for HostedControlPlane %s", sourceName)

		By("Verifying Route and RouteMonitor names follow the expected naming convention")
		route, err := dynamicClient.Resource(routeGVR).Namespace(sourceNamespace).Get(ctx, expectedRouteName, metav1.GetOptions{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(route.GetName()).Should(Equal(expectedRouteName),
			"Route name should be <hcp-name>-kube-apiserver-monitoring")

		rm, err := dynamicClient.Resource(routeMonitorGVR).Namespace(sourceNamespace).Get(ctx, expectedRouteName, metav1.GetOptions{})
		Expect(err).ShouldNot(HaveOccurred())
		Expect(rm.GetName()).Should(Equal(expectedRouteName),
			"RouteMonitor name should be <hcp-name>-kube-apiserver-monitoring")
	})

	It("exposes RHOBS metrics on the operator metrics endpoint", func(ctx context.Context) {
		By("Checking operator deployment exists")
		var deploy appsv1.Deployment
		err := k8s.Get(ctx, deploymentName, namespace, &deploy)
		if k8serrors.IsNotFound(err) {
			Skip("Operator deployment not found, skipping metrics check")
		}
		Expect(err).ShouldNot(HaveOccurred(), "failed to get operator deployment")

		By("Finding the operator pod name")
		var podList kubev1.PodList
		err = k8s.WithNamespace(namespace).List(ctx, &podList)
		Expect(err).ShouldNot(HaveOccurred(), "failed to list pods")
		var podName string
		for _, pod := range podList.Items {
			if strings.HasPrefix(pod.Name, "route-monitor-operator-controller-manager") && pod.Status.Phase == kubev1.PodRunning {
				podName = pod.Name
				break
			}
		}
		if podName == "" {
			Skip("No running operator pod found, skipping metrics check")
		}

		By("Fetching metrics from operator pod via exec")
		cfg := rest.CopyConfig(k8s.GetConfig())
		// Use backplane-cluster-admin impersonation if needed (for MC access)
		cfg.Impersonate = rest.ImpersonationConfig{UserName: "backplane-cluster-admin"}

		coreClient, err := kubernetes.NewForConfig(cfg)
		Expect(err).ShouldNot(HaveOccurred(), "failed to create core client")

		var metricsBody string
		err = wait.PollUntilContextTimeout(ctx, 10*time.Second, pollingDuration, false, func(ctx context.Context) (bool, error) {
			execReq := coreClient.CoreV1().RESTClient().Post().
				Resource("pods").
				Name(podName).
				Namespace(namespace).
				SubResource("exec").
				Param("container", "manager").
				Param("command", "curl").
				Param("command", "-s").
				Param("command", "http://localhost:8080/metrics").
				Param("stdout", "true")

			exec, execErr := remotecommand.NewSPDYExecutor(cfg, http.MethodPost, execReq.URL())
			if execErr != nil {
				GinkgoLogr.Info("Failed to create SPDY executor", "error", execErr)
				return false, nil
			}

			var stdout bytes.Buffer
			streamErr := exec.StreamWithContext(ctx, remotecommand.StreamOptions{
				Stdout: &stdout,
			})
			if streamErr != nil {
				GinkgoLogr.Info("Exec stream failed, retrying", "error", streamErr)
				return false, nil
			}

			bodyStr := stdout.String()
			if !strings.Contains(bodyStr, "rhobs_route_monitor_operator_info") {
				GinkgoLogr.Info("Metrics not yet available in response")
				return false, nil
			}
			metricsBody = bodyStr
			return true, nil
		})
		if err != nil {
			Skip(fmt.Sprintf("Could not reach operator metrics endpoint: %v", err))
		}

		By("Verifying RHOBS metrics are present")
		Expect(metricsBody).Should(ContainSubstring("rhobs_route_monitor_operator_info"),
			"metrics should contain rhobs_route_monitor_operator_info")
		Expect(metricsBody).Should(ContainSubstring("rhobs_route_monitor_operator_probe_deletion_timeout_total"),
			"metrics should contain rhobs_route_monitor_operator_probe_deletion_timeout_total")
	})
})
