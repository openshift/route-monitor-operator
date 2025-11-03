//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	// Test configuration
	testNamespace = "route-monitor-operator-e2e-test"
	testClusterID = "e2e-test-cluster-12345"

	// Timeouts
	setupTimeout        = 30 * time.Second
	cleanupTimeout      = 10 * time.Second
	probeTimeout        = 30 * time.Second
	verificationTimeout = 30 * time.Second

	// RHOBS API configuration
	rhobsTenant = "e2e-test"
)

var (
	mockK8sClient     *MockKubernetesClient
	mockRHOBSClient   *MockRHOBSClient
	mockRMOController *MockRMOController
	testLogger        = &TestLogger{}
)

// TestLogger is a simple logger for tests
type TestLogger struct{}

func (l *TestLogger) Info(msg string, keysAndValues ...interface{}) {
	GinkgoWriter.Printf("[INFO] %s %v\n", msg, keysAndValues)
}

func (l *TestLogger) Error(err error, msg string, keysAndValues ...interface{}) {
	GinkgoWriter.Printf("[ERROR] %s: %v %v\n", msg, err, keysAndValues)
}

func TestFullE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Full E2E Test Suite")
}

var _ = BeforeSuite(func() {
	By("Setting up mock environment")

	// Initialize mock clients
	mockK8sClient = NewMockKubernetesClient()
	mockRHOBSClient = NewMockRHOBSClient()
	mockRMOController = NewMockRMOController(mockK8sClient, mockRHOBSClient)

	// Create test namespace
	err := mockK8sClient.CreateNamespace(testNamespace)
	Expect(err).NotTo(HaveOccurred(), "Failed to create test namespace")

	testLogger.Info("Mock environment setup complete",
		"namespace", testNamespace,
		"clusterID", testClusterID)
})

var _ = AfterSuite(func() {
	By("Cleaning up mock environment")

	// Clean up test namespace
	err := mockK8sClient.DeleteNamespace(testNamespace)
	Expect(err).NotTo(HaveOccurred(), "Failed to delete test namespace")

	testLogger.Info("Mock environment cleanup complete")
})

var _ = Describe("Full End-to-End Test for Route Monitor Operator", Ordered, func() {
	var (
		hostedControlPlane *MockHostedControlPlane
		probeID            string
	)

	BeforeEach(func() {
		By("Creating test HostedControlPlane")
		hostedControlPlane = CreateTestHostedControlPlane("e2e-test-hcp", testNamespace, testClusterID)

		// Simulate creating the HostedControlPlane in the mock cluster
		err := mockK8sClient.Create(context.TODO(), hostedControlPlane)
		Expect(err).NotTo(HaveOccurred(), "Failed to create HostedControlPlane")

		By("Waiting for HostedControlPlane to be processed")
		Eventually(func() bool {
			return verifyHostedControlPlaneReconciliation()
		}, setupTimeout, 1*time.Second).Should(BeTrue(), "HostedControlPlane should be reconciled")
	})

	AfterEach(func() {
		By("Cleaning up test resources")

		// Delete RHOBS probe if it was created
		if probeID != "" {
			err := mockRHOBSClient.DeleteProbe(context.TODO(), testClusterID)
			Expect(err).NotTo(HaveOccurred(), "Failed to delete RHOBS probe")
		}

		// Delete HostedControlPlane
		err := mockRMOController.DeleteHostedControlPlane(context.TODO(), hostedControlPlane)
		Expect(err).NotTo(HaveOccurred(), "Failed to delete HostedControlPlane")

		// Wait for HostedControlPlane to be deleted
		Eventually(func() bool {
			// Check if the probe was deleted
			_, err := mockRHOBSClient.GetProbe(context.TODO(), testClusterID)
			return err != nil
		}, cleanupTimeout, 1*time.Second).Should(BeTrue(), "HostedControlPlane should be deleted")
	})

	Context("HostedControlPlane to RHOBS Probe Workflow", func() {
		It("should create a synthetic probe in RHOBS API", func() {
			By("Waiting for RHOBS probe to be created")
			Eventually(func() bool {
				probe, err := mockRHOBSClient.GetProbe(context.TODO(), testClusterID)
				if err != nil {
					testLogger.Error(err, "Failed to get RHOBS probe")
					return false
				}
				if probe == nil {
					return false
				}
				probeID = probe.ID // Store probe ID for later cleanup
				return true
			}, probeTimeout, 1*time.Second).Should(BeTrue(), "RHOBS probe should be created")

			By("Verifying probe configuration")
			probe, err := mockRHOBSClient.GetProbe(context.TODO(), testClusterID)
			Expect(err).NotTo(HaveOccurred(), "Failed to get RHOBS probe after creation")
			Expect(probe).NotTo(BeNil(), "Probe should not be nil")
			Expect(probe.Labels["cluster-id"]).To(Equal(testClusterID), "Probe should have correct cluster-id label")
			Expect(probe.Labels["private"]).To(Equal("false"), "Probe should be marked as public")
			Expect(probe.Status).To(Equal("active"), "Probe should be active")
		})

		It("should verify synthetics agent picks up and executes the probe", func() {
			By("Waiting for probe to be created first")
			Eventually(func() bool {
				probe, err := mockRHOBSClient.GetProbe(context.TODO(), testClusterID)
				return err == nil && probe != nil
			}, probeTimeout, 1*time.Second).Should(BeTrue(), "RHOBS probe should be created")

			By("Waiting for synthetics agent to execute probe")
			Eventually(func() bool {
				return VerifySyntheticsAgentExecution("http://localhost:8081", testClusterID)
			}, verificationTimeout, 2*time.Second).Should(BeTrue(), "Synthetics agent should execute probe")
		})

		It("should verify probe results are reported back to API", func() {
			By("Waiting for probe to be created and executed")
			Eventually(func() bool {
				probe, err := mockRHOBSClient.GetProbe(context.TODO(), testClusterID)
				return err == nil && probe != nil
			}, probeTimeout, 1*time.Second).Should(BeTrue(), "RHOBS probe should be created")

			By("Waiting for probe execution results")
			Eventually(func() bool {
				return VerifyProbeResults(mockRHOBSClient, testClusterID)
			}, verificationTimeout, 2*time.Second).Should(BeTrue(), "Probe results should be reported")
		})
	})

	Context("Route Monitor Operator Logs", func() {
		It("should show successful reconciliation logs", func() {
			By("Checking RMO logs for reconciliation success")
			Eventually(func() bool {
				return VerifyRMOReconciliationLogs()
			}, setupTimeout, 1*time.Second).Should(BeTrue(), "RMO should log successful reconciliation")
		})
	})
})

// verifyHostedControlPlaneReconciliation verifies that the HostedControlPlane was reconciled
func verifyHostedControlPlaneReconciliation() bool {
	// Simulate the RMO controller reconciling the HostedControlPlane
	hostedControlPlane := CreateTestHostedControlPlane("e2e-test-hcp", testNamespace, testClusterID)

	err := mockRMOController.ReconcileHostedControlPlane(context.TODO(), hostedControlPlane)
	if err != nil {
		testLogger.Error(err, "Failed to reconcile HostedControlPlane")
		return false
	}

	return true
}

// VerifyRMOReconciliationLogs verifies that the RMO logged successful reconciliation
func VerifyRMOReconciliationLogs() bool {
	logs := mockRMOController.GetReconcileLog()

	// Look for successful reconciliation log
	for _, log := range logs {
		if contains(log, "Successfully reconciled HostedControlPlane") {
			return true
		}
	}

	return false
}
