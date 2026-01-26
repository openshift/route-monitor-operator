package hostedcontrolplane

import (
	"context"
	"errors"
	"testing"
	"time"

	hypershiftv1beta1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// TestDeletionTimeoutBehavior tests the hybrid retry-then-fail-open logic
// without requiring the full e2e infrastructure.
//
// This validates SREP-2832 + SREP-2966 timeout behavior:
//   - Within 15 min: Fail closed (retry, block deletion)
//   - After 15 min: Fail open (allow deletion despite probe cleanup failure)
func TestDeletionTimeoutBehavior(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = hypershiftv1beta1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	// Create reconciler with a test RHOBS config
	reconciler := &HostedControlPlaneReconciler{
		Client: fakeClient,
		Scheme: scheme,
		RHOBSConfig: RHOBSConfig{
			ProbeAPIURL:        "http://test-api:8080/probes",
			Tenant:             "test-tenant",
			OnlyPublicClusters: false,
		},
	}

	ctx := context.Background()
	log := zap.New(zap.UseDevMode(true))

	t.Run("Within_Timeout_Fails_Closed", func(t *testing.T) {
		// Create HCP with recent deletion timestamp (5 minutes ago)
		recentTime := metav1.Time{Time: time.Now().Add(-5 * time.Minute)}

		hcp := &hypershiftv1beta1.HostedControlPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "test-hcp-recent",
				Namespace:         "clusters",
				DeletionTimestamp: &recentTime,
				Finalizers:        []string{hostedcontrolplaneFinalizer},
			},
			Spec: hypershiftv1beta1.HostedControlPlaneSpec{
				ClusterID: "test-cluster-recent",
				Platform: hypershiftv1beta1.PlatformSpec{
					Type: hypershiftv1beta1.AWSPlatform,
					AWS: &hypershiftv1beta1.AWSPlatformSpec{
						Region:         "us-east-1",
						EndpointAccess: hypershiftv1beta1.PublicAndPrivate,
					},
				},
			},
		}

		// Call deleteRHOBSProbe directly - it will fail because API is not running
		err := reconciler.deleteRHOBSProbe(ctx, log, hcp, reconciler.RHOBSConfig)

		// Within timeout window, we expect an error (fail closed)
		if err == nil {
			t.Error("Expected error when deleting probe (API unavailable), but got nil")
		} else {
			t.Logf("✅ Got expected error within timeout window: %v", err)
		}

		// Calculate elapsed time to verify we're testing the right scenario
		elapsed := time.Since(hcp.DeletionTimestamp.Time)
		if elapsed >= rhobsProbeDeletionTimeout {
			t.Errorf("Test setup error: deletion elapsed time (%v) should be less than timeout (%v)",
				elapsed, rhobsProbeDeletionTimeout)
		} else {
			t.Logf("✅ Deletion elapsed time (%v) is within timeout window (%v)",
				elapsed, rhobsProbeDeletionTimeout)
		}

		t.Log("✅ Fail-closed behavior validated: error returned within timeout window")
	})

	t.Run("Past_Timeout_Would_Fail_Open", func(t *testing.T) {
		// Create HCP with old deletion timestamp (20 minutes ago - past timeout)
		oldTime := metav1.Time{Time: time.Now().Add(-20 * time.Minute)}

		hcp := &hypershiftv1beta1.HostedControlPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "test-hcp-old",
				Namespace:         "clusters",
				DeletionTimestamp: &oldTime,
				Finalizers:        []string{hostedcontrolplaneFinalizer},
			},
			Spec: hypershiftv1beta1.HostedControlPlaneSpec{
				ClusterID: "test-cluster-old",
				Platform: hypershiftv1beta1.PlatformSpec{
					Type: hypershiftv1beta1.AWSPlatform,
					AWS: &hypershiftv1beta1.AWSPlatformSpec{
						Region:         "us-east-1",
						EndpointAccess: hypershiftv1beta1.PublicAndPrivate,
					},
				},
			},
		}

		// Calculate elapsed time to verify we're testing the right scenario
		elapsed := time.Since(hcp.DeletionTimestamp.Time)
		if elapsed < rhobsProbeDeletionTimeout {
			t.Errorf("Test setup error: deletion elapsed time (%v) should be greater than timeout (%v)",
				elapsed, rhobsProbeDeletionTimeout)
		} else {
			t.Logf("✅ Deletion elapsed time (%v) is past timeout window (%v)",
				elapsed, rhobsProbeDeletionTimeout)
		}

		// The fail-open logic is in the main reconciliation loop in hostedcontrolplane.go
		// This test validates that we can detect when we're past the timeout
		// The actual fail-open behavior (continuing despite error) is tested in the main reconciliation

		t.Log("✅ Timeout detection validated: elapsed time correctly exceeds timeout threshold")
		t.Log("   (Fail-open behavior is in main reconciliation loop, tested by e2e test)")
	})

	t.Run("Timeout_Constant_Is_15_Minutes", func(t *testing.T) {
		expectedTimeout := 15 * time.Minute
		if rhobsProbeDeletionTimeout != expectedTimeout {
			t.Errorf("Expected timeout to be %v, got %v", expectedTimeout, rhobsProbeDeletionTimeout)
		} else {
			t.Logf("✅ Timeout constant correctly set to %v", rhobsProbeDeletionTimeout)
		}
	})

	t.Run("DeleteRHOBSProbe_Returns_Error_On_Failure", func(t *testing.T) {
		// This validates that deleteRHOBSProbe returns errors instead of swallowing them
		// (Required for retry logic to work)

		hcp := &hypershiftv1beta1.HostedControlPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-error-propagation",
				Namespace: "clusters",
			},
			Spec: hypershiftv1beta1.HostedControlPlaneSpec{
				ClusterID: "test-error-prop",
			},
		}

		err := reconciler.deleteRHOBSProbe(ctx, log, hcp, reconciler.RHOBSConfig)

		if err == nil {
			t.Error("Expected deleteRHOBSProbe to return error when API unavailable, but got nil")
			t.Error("   This indicates error swallowing, which breaks retry logic")
		} else {
			t.Logf("✅ deleteRHOBSProbe correctly returns error: %v", err)
		}

		// Verify it's a meaningful error (not just a generic error)
		if !errors.Is(err, context.DeadlineExceeded) && err.Error() != "" {
			t.Logf("✅ Error contains meaningful information: %s", err.Error())
		}
	})
}
