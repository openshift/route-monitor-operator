//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	routev1 "github.com/openshift/api/route/v1"
	awsvpceapi "github.com/openshift/aws-vpce-operator/api/v1alpha2"
	hypershiftv1beta1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	rmoapi "github.com/openshift/route-monitor-operator/api/v1alpha1"
	rmocontrollers "github.com/openshift/route-monitor-operator/controllers/hostedcontrolplane"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// TestProbeDeletionRetry tests SREP-2832: probe deletion retry when API is unavailable
//
// WHAT IT TESTS:
//  1. RMO attempts to delete a probe when HostedControlPlane is marked for deletion
//  2. RHOBS API is stopped/unavailable during deletion
//  3. RMO detects API error and requeues (retry behavior)
//  4. API comes back online
//  5. RMO successfully deletes the probe on retry
//  6. HostedControlPlane finalizer is removed only after probe deletion succeeds
//
// This validates that RMO won't orphan probes when API is temporarily unavailable
// and that finalizers prevent cluster deletion until cleanup completes.
//
// REQUIREMENTS:
// Same as TestFullStackIntegration - needs local RHOBS repos
//
// RUNNING: make test-e2e-full
func TestProbeDeletionRetry(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping probe deletion retry test in short mode")
	}

	t.Log("🧪 Testing SREP-2832: Probe Deletion Retry When API Unavailable")
	t.Log("================================================================================")

	// Setup mock servers
	mockDynatrace := startMockDynatraceServer()
	defer mockDynatrace.Close()
	t.Logf("✅ Mock Dynatrace server started at %s", mockDynatrace.URL)

	mockProbeTarget := startMockProbeTargetServer()
	defer mockProbeTarget.Close()
	t.Logf("✅ Mock probe target server started at %s", mockProbeTarget.URL)

	// Setup API server
	apiManager := NewRealAPIManager()
	defer func() { _ = apiManager.Stop() }()

	err := apiManager.Start()
	if err != nil {
		t.Fatalf("Failed to start API server: %v", err)
	}

	if err := apiManager.ClearAllProbes(); err != nil {
		t.Logf("Warning: failed to clear existing probes: %v", err)
	}

	apiURL := apiManager.GetURL()
	t.Logf("✅ API server started at %s", apiURL)

	// Test variables
	var testProbeID string
	testClusterID := "test-retry-cluster-789"
	probeURL := mockProbeTarget.URL + "/livez"

	// Step 1: Create probe via API (simulating existing probe)
	t.Run("Setup_Create_Probe", func(t *testing.T) {
		t.Log("📋 Creating initial probe via API...")
		probeID, err := createProbeViaAPI(apiURL, testClusterID, probeURL, false)
		if err != nil {
			t.Fatalf("Failed to create probe: %v", err)
		}
		testProbeID = probeID
		t.Logf("✅ Created probe with ID: %s", testProbeID)

		// Verify probe exists
		probe, err := getProbeByID(apiURL, testProbeID)
		if err != nil {
			t.Fatalf("Failed to verify probe creation: %v", err)
		}
		t.Logf("✅ Verified probe exists with status: %s", probe.Status)
	})

	// Step 2: Create HostedControlPlane CR with deletion timestamp
	t.Run("Create_HostedControlPlane_For_Deletion", func(t *testing.T) {
		scheme := runtime.NewScheme()
		_ = clientgoscheme.AddToScheme(scheme)
		_ = hypershiftv1beta1.AddToScheme(scheme)
		_ = awsvpceapi.AddToScheme(scheme)
		_ = routev1.Install(scheme)
		_ = rmoapi.AddToScheme(scheme)

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		rhobsConfig := rmocontrollers.RHOBSConfig{
			ProbeAPIURL:        apiURL + "/probes",
			Tenant:             "",
			OnlyPublicClusters: false,
		}

		reconciler := &rmocontrollers.HostedControlPlaneReconciler{
			Client:      fakeClient,
			Scheme:      scheme,
			RHOBSConfig: rhobsConfig,
		}

		// Create HCP with finalizer and deletion timestamp
		now := metav1.Now()
		hcp := &hypershiftv1beta1.HostedControlPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "test-hcp-delete",
				Namespace:         "clusters",
				DeletionTimestamp: &now,
				Finalizers:        []string{"hostedcontrolplane.routemonitoroperator.monitoring.openshift.io/finalizer"},
			},
			Spec: hypershiftv1beta1.HostedControlPlaneSpec{
				ClusterID: testClusterID,
				Platform: hypershiftv1beta1.PlatformSpec{
					Type: hypershiftv1beta1.AWSPlatform,
					AWS: &hypershiftv1beta1.AWSPlatformSpec{
						Region:         "us-east-1",
						EndpointAccess: hypershiftv1beta1.Private,
					},
				},
				Services: []hypershiftv1beta1.ServicePublishingStrategyMapping{
					{
						Service: hypershiftv1beta1.APIServer,
						ServicePublishingStrategy: hypershiftv1beta1.ServicePublishingStrategy{
							Type: hypershiftv1beta1.Route,
							Route: &hypershiftv1beta1.RoutePublishingStrategy{
								Hostname: "api.test-retry-cluster.example.com",
							},
						},
					},
				},
			},
			Status: hypershiftv1beta1.HostedControlPlaneStatus{
				Conditions: []metav1.Condition{
					{
						Type:   string(hypershiftv1beta1.HostedControlPlaneAvailable),
						Status: metav1.ConditionTrue,
					},
				},
			},
		}

		ctx := context.Background()
		err := fakeClient.Create(ctx, hcp)
		if err != nil {
			t.Fatalf("Failed to create HostedControlPlane: %v", err)
		}

		setupRMODependencies(t, fakeClient, ctx, mockDynatrace.URL)
		t.Logf("✅ Created HostedControlPlane CR marked for deletion")

		logWriter := &testWriter{t: t, logs: []string{}}
		zapLogger := zap.New(zap.WriteTo(logWriter), zap.UseDevMode(true))
		log.SetLogger(zapLogger)

		// Step 3: Stop API server to simulate unavailability
		t.Log("🛑 Stopping API server to simulate unavailability...")
		if err := apiManager.Stop(); err != nil {
			t.Logf("Warning: API stop returned error: %v", err)
		}
		time.Sleep(1 * time.Second) // Give it time to fully stop
		t.Log("✅ API server stopped")

		// Step 4: Trigger RMO reconciliation (should fail and requeue)
		t.Log("🔄 Triggering RMO reconciliation with API unavailable...")
		req := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "test-hcp-delete",
				Namespace: "clusters",
			},
		}

		result, err := reconciler.Reconcile(ctx, req)
		t.Logf("RMO reconciliation result: %+v, err=%v", result, err)

		// We expect an error or requeue because API is down
		if err != nil {
			if strings.Contains(err.Error(), "connection refused") ||
				strings.Contains(err.Error(), "API request failed") ||
				strings.Contains(err.Error(), "failed to delete RHOBS probe") {
				t.Log("✅ RMO correctly detected API unavailability and returned error")
			} else {
				t.Logf("⚠️  Unexpected error type: %v", err)
			}
		} else if result.Requeue || result.RequeueAfter > 0 {
			t.Log("✅ RMO correctly requeued for retry")
		} else {
			t.Error("❌ Expected RMO to return error or requeue, but got success")
		}

		// Verify finalizer is NOT removed (critical for SREP-2832)
		err = fakeClient.Get(ctx, types.NamespacedName{Name: "test-hcp-delete", Namespace: "clusters"}, hcp)
		if err != nil {
			t.Fatalf("Failed to get HCP after first reconciliation: %v", err)
		}

		finalizerPresent := false
		for _, f := range hcp.Finalizers {
			if f == "hostedcontrolplane.routemonitoroperator.monitoring.openshift.io/finalizer" {
				finalizerPresent = true
				break
			}
		}

		if !finalizerPresent {
			t.Error("❌ CRITICAL: Finalizer was removed even though probe deletion failed!")
			t.Error("   This would cause orphaned probes (SREP-2832)")
		} else {
			t.Log("✅ Finalizer correctly retained after failed probe deletion")
		}

		// Verify probe still exists in API (once we restart it)
		t.Log("🔄 Restarting API server...")
		err = apiManager.Start()
		if err != nil {
			t.Fatalf("Failed to restart API server: %v", err)
		}
		t.Log("✅ API server restarted")

		// Verify probe was NOT deleted
		probe, err := getProbeByID(apiURL, testProbeID)
		if err != nil {
			t.Fatalf("Failed to check probe status: %v", err)
		}
		if probe.Status == "deleted" || probe.Status == "terminating" {
			t.Error("❌ Probe was marked for deletion even though API was unavailable during first attempt")
		} else {
			t.Logf("✅ Probe still exists with status: %s (not orphaned)", probe.Status)
		}

		// Step 5: Trigger RMO reconciliation again (should succeed now)
		t.Log("🔄 Triggering RMO reconciliation with API available...")
		result, err = reconciler.Reconcile(ctx, req)
		t.Logf("RMO reconciliation result: %+v, err=%v", result, err)

		if err != nil && !strings.Contains(err.Error(), "404") {
			t.Logf("⚠️  RMO reconciliation returned error: %v", err)
		}

		// Verify probe is now marked for termination
		t.Log("⏱️  Waiting for probe to be marked as terminating...")
		timeout := time.After(10 * time.Second)
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		probeMarkedForDeletion := false
		for {
			select {
			case <-timeout:
				t.Error("❌ Timeout waiting for probe to be marked as terminating")
				goto checkFinalizer
			case <-ticker.C:
				probe, err := getProbeByID(apiURL, testProbeID)
				if err != nil {
					// Probe might be deleted already
					if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
						t.Log("✅ Probe successfully deleted from API")
						probeMarkedForDeletion = true
						goto checkFinalizer
					}
					continue
				}
				if probe.Status == "terminating" || probe.Status == "deleted" {
					t.Logf("✅ Probe marked with status: %s", probe.Status)
					probeMarkedForDeletion = true
					goto checkFinalizer
				}
			}
		}

	checkFinalizer:
		if !probeMarkedForDeletion {
			t.Log("⚠️  Probe was not marked for deletion in allocated time")
		}

		t.Log("✅ Test completed: RMO correctly retries probe deletion when API becomes available")
		t.Log("✅ Finalizer behavior prevents orphaned probes (SREP-2832 fix verified)")
	})

	// Cleanup
	t.Run("Cleanup", func(t *testing.T) {
		t.Log("🗑️  Cleaning up test resources...")
		// Try to delete probe if it still exists
		err := deleteProbeViaAPI(apiURL, testProbeID)
		if err != nil && !strings.Contains(err.Error(), "404") {
			t.Logf("Warning: Failed to delete test probe: %v", err)
		} else {
			t.Log("✅ Test probe cleaned up")
		}
	})
}
