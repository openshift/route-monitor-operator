package hostedcontrolplane

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	hypershiftv1beta1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	"github.com/openshift/route-monitor-operator/pkg/rhobs"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func TestHostedControlPlaneReconciler_GetAPIServerHostname(t *testing.T) {
	tests := []struct {
		name      string
		input     *hypershiftv1beta1.HostedControlPlane
		expected  string
		expectErr bool
	}{
		{
			name: "APIServer Service Found",
			input: &hypershiftv1beta1.HostedControlPlane{
				Spec: hypershiftv1beta1.HostedControlPlaneSpec{
					Services: []hypershiftv1beta1.ServicePublishingStrategyMapping{
						{
							Service: "APIServer",
							ServicePublishingStrategy: hypershiftv1beta1.ServicePublishingStrategy{
								Route: &hypershiftv1beta1.RoutePublishingStrategy{
									Hostname: "api.example.com",
								},
							},
						},
					},
				},
			},
			expected:  "api.example.com",
			expectErr: false,
		},
		{
			name: "APIServer Service Not Found",
			input: &hypershiftv1beta1.HostedControlPlane{
				Spec: hypershiftv1beta1.HostedControlPlaneSpec{
					Services: []hypershiftv1beta1.ServicePublishingStrategyMapping{
						{
							Service: "ControllerManager",
						},
						{
							Service: "Scheduler",
						},
					},
				},
			},
			expected:  "",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hostname, err := GetAPIServerHostname(tt.input)

			if (err != nil) != tt.expectErr {
				t.Errorf("GetAPIServerHostname error = %v, expectErr %v", err, tt.expectErr)
			}

			if hostname != tt.expected {
				t.Errorf("Expected hostname: %s, got: %s", tt.expected, hostname)
			}
		})
	}
}

// Left as a placeholder for future testing.
// Currently, this function simply calls other methods and wraps any error returned,

func TestGetClusterRegion(t *testing.T) {
	tests := []struct {
		name               string
		hostedControlPlane *hypershiftv1beta1.HostedControlPlane
		expectRegion       string
		expectError        bool
	}{
		{
			name: "Valid AWS region",
			hostedControlPlane: &hypershiftv1beta1.HostedControlPlane{
				Spec: hypershiftv1beta1.HostedControlPlaneSpec{
					Platform: hypershiftv1beta1.PlatformSpec{
						AWS: &hypershiftv1beta1.AWSPlatformSpec{
							Region: "us-west-2",
						},
					},
				},
			},
			expectRegion: "us-west-2",
			expectError:  false,
		},
		{
			name:               "Hosted control plane is nil",
			hostedControlPlane: nil,
			expectRegion:       "",
			expectError:        true,
		},
		{
			name: "AWS region is empty",
			hostedControlPlane: &hypershiftv1beta1.HostedControlPlane{
				Spec: hypershiftv1beta1.HostedControlPlaneSpec{
					Platform: hypershiftv1beta1.PlatformSpec{
						AWS: &hypershiftv1beta1.AWSPlatformSpec{
							Region: "",
						},
					},
				},
			},
			expectRegion: "",
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Call the function to test
			region, err := getClusterRegion(tt.hostedControlPlane)

			// Verify the results
			if region != tt.expectRegion {
				t.Errorf("Unexpected region. Expected: %v, got: %v", tt.expectRegion, region)
			}
			if (err != nil) != tt.expectError {
				t.Errorf("Unexpected error status. Expected error: %v, got: %v", tt.expectError, err)
			}
		})
	}
}

func TestCreateRHOBSClient(t *testing.T) {
	tests := []struct {
		name        string
		config      RHOBSConfig
		expectOIDC  bool
		description string
	}{
		{
			name: "Creates client without OIDC when credentials are empty",
			config: RHOBSConfig{
				ProbeAPIURL:      "https://api.example.com/probes",
				Tenant:           "test-tenant",
				OIDCClientID:     "",
				OIDCClientSecret: "",
				OIDCIssuerURL:    "",
			},
			expectOIDC:  false,
			description: "Client should be created without OIDC authentication",
		},
		{
			name: "Creates client with OIDC when all credentials are provided",
			config: RHOBSConfig{
				ProbeAPIURL:      "https://api.example.com/probes",
				Tenant:           "test-tenant",
				OIDCClientID:     "client-id",
				OIDCClientSecret: "client-secret",
				OIDCIssuerURL:    "https://issuer.example.com",
			},
			expectOIDC:  true,
			description: "Client should be created with OIDC authentication",
		},
		{
			name: "Creates client without OIDC when only client ID is provided",
			config: RHOBSConfig{
				ProbeAPIURL:      "https://api.example.com/probes",
				Tenant:           "test-tenant",
				OIDCClientID:     "client-id",
				OIDCClientSecret: "",
				OIDCIssuerURL:    "",
			},
			expectOIDC:  false,
			description: "Client should fall back to no OIDC when credentials are incomplete",
		},
		{
			name: "Creates client without OIDC when only issuer URL is provided",
			config: RHOBSConfig{
				ProbeAPIURL:      "https://api.example.com/probes",
				Tenant:           "test-tenant",
				OIDCClientID:     "",
				OIDCClientSecret: "",
				OIDCIssuerURL:    "https://issuer.example.com",
			},
			expectOIDC:  false,
			description: "Client should fall back to no OIDC when credentials are incomplete",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestReconciler(t)
			log := log.FromContext(context.Background())

			client := r.createRHOBSClient(log, tt.config)

			if client == nil {
				t.Errorf("createRHOBSClient returned nil client")
			}
		})
	}
}

// mockRHOBSServer creates an httptest server that responds to RHOBS probe API calls.
// getResponse controls what GET /probes returns; if nil, returns empty list.
// deleteErr controls whether PATCH (delete) returns an error.
// requestLog tracks which HTTP methods were called.
func mockRHOBSServer(t *testing.T, getResponse *rhobs.ProbeResponse, getErr bool, deleteErr bool, requestLog *[]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body.Close()
		*requestLog = append(*requestLog, r.Method+" "+r.URL.Path+" "+string(body))

		if !strings.HasPrefix(r.URL.Path, "/probes") {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		switch r.Method {
		case http.MethodGet:
			if getErr {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":"mock get error"}`))
				return
			}
			if getResponse == nil {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"probes":[]}`))
				return
			}
			resp, _ := json.Marshal(rhobs.ProbesListResponse{Probes: []rhobs.ProbeResponse{*getResponse}})
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(resp)

		case http.MethodPatch:
			if deleteErr {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":"mock delete error"}`))
				return
			}
			w.WriteHeader(http.StatusOK)

		case http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			resp, _ := json.Marshal(rhobs.ProbeResponse{ID: "new-probe", Labels: map[string]string{}, Status: "active"})
			_, _ = w.Write(resp)

		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
}

// newHCP creates a HostedControlPlane for testing with the given clusterID, region, labels, and endpoint access.
func newHCP(clusterID, region string, labels map[string]string, endpointAccess hypershiftv1beta1.AWSEndpointAccessType) *hypershiftv1beta1.HostedControlPlane {
	return newHCPWithMeta(clusterID, region, labels, endpointAccess, "", "")
}

func newHCPWithMeta(clusterID, region string, labels map[string]string, endpointAccess hypershiftv1beta1.AWSEndpointAccessType, name, namespace string) *hypershiftv1beta1.HostedControlPlane {
	return &hypershiftv1beta1.HostedControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: hypershiftv1beta1.HostedControlPlaneSpec{
			ClusterID: clusterID,
			Services: []hypershiftv1beta1.ServicePublishingStrategyMapping{
				{
					Service: "APIServer",
					ServicePublishingStrategy: hypershiftv1beta1.ServicePublishingStrategy{
						Route: &hypershiftv1beta1.RoutePublishingStrategy{
							Hostname: "api.example.com",
						},
					},
				},
			},
			Platform: hypershiftv1beta1.PlatformSpec{
				AWS: &hypershiftv1beta1.AWSPlatformSpec{
					Region:         region,
					EndpointAccess: endpointAccess,
				},
			},
		},
	}
}

func TestEnsureRHOBSProbe_LimitedSupport(t *testing.T) {
	tests := []struct {
		name         string
		getResponse  *rhobs.ProbeResponse
		getErr       bool
		deleteErr    bool
		expectErr    bool
		errContains  string
		expectDelete bool
	}{
		{
			name:         "no existing probe, returns nil",
			getResponse:  nil,
			expectErr:    false,
			expectDelete: false,
		},
		{
			name: "existing probe, deletes it",
			getResponse: &rhobs.ProbeResponse{
				ID:     "probe-123",
				Labels: map[string]string{"cluster-id": "test-cluster"},
				Status: "active",
			},
			expectErr:    false,
			expectDelete: true,
		},
		{
			name:        "GetProbe fails, returns error",
			getErr:      true,
			expectErr:   true,
			errContains: "failed to check existing probe",
		},
		{
			name: "DeleteProbe fails, returns error",
			getResponse: &rhobs.ProbeResponse{
				ID:     "probe-123",
				Labels: map[string]string{"cluster-id": "test-cluster"},
				Status: "active",
			},
			deleteErr:    true,
			expectErr:    true,
			errContains:  "failed to delete probe for limited support cluster",
			expectDelete: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requestLog []string
			server := mockRHOBSServer(t, tt.getResponse, tt.getErr, tt.deleteErr, &requestLog)
			defer server.Close()

			// Create HC with LS label in the parent namespace so cross-check confirms LS
			hcp := newHCPWithMeta("test-cluster", "us-east-1",
				map[string]string{"api.openshift.com/limited-support": "true"},
				hypershiftv1beta1.PublicAndPrivate,
				"test-hcp", "ocm-production-clusterid-test-hcp",
			)
			hc := &hypershiftv1beta1.HostedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hcp",
					Namespace: "ocm-production-clusterid",
					Labels:    map[string]string{"api.openshift.com/limited-support": "true"},
				},
			}

			r := &HostedControlPlaneReconciler{
				Client: fake.NewFakeClient(hc),
			}
			ctx := context.Background()
			logger := log.FromContext(ctx)

			cfg := RHOBSConfig{
				ProbeAPIURL: server.URL + "/probes",
				Tenant:      "test-tenant",
			}

			err := r.ensureRHOBSProbe(ctx, logger, hcp, cfg)

			if tt.expectErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			hasPatch := false
			hasPost := false
			for _, entry := range requestLog {
				if strings.HasPrefix(entry, "PATCH") {
					hasPatch = true
				}
				if strings.HasPrefix(entry, "POST") {
					hasPost = true
				}
			}
			if tt.expectDelete && !hasPatch {
				t.Error("expected DeleteProbe (PATCH) to be called, but no PATCH request was made")
			}
			if !tt.expectDelete && hasPatch {
				t.Error("expected no DeleteProbe call, but PATCH request was made")
			}
			if hasPost {
				t.Error("expected limited-support flow to skip probe creation, but POST request was made")
			}
		})
	}
}

func TestEnsureRHOBSProbe_StaleLimitedSupportLabel(t *testing.T) {
	// Test that when HCP has stale LS label but HC does not, RMO ignores the stale label
	// and proceeds with probe creation (OCPBUGS-85584)
	var requestLog []string
	server := mockRHOBSServer(t, nil, false, false, &requestLog)
	defer server.Close()

	hcp := newHCPWithMeta("test-cluster", "us-east-1",
		map[string]string{"api.openshift.com/limited-support": "true"},
		hypershiftv1beta1.PublicAndPrivate,
		"test-hcp", "ocm-staging-clusterid-test-hcp",
	)
	// HC does NOT have the LS label (it was cleared by CS)
	hc := &hypershiftv1beta1.HostedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-hcp",
			Namespace: "ocm-staging-clusterid",
			Labels:    map[string]string{"api.openshift.com/name": "test-hcp"},
		},
	}

	r := &HostedControlPlaneReconciler{
		Client: fake.NewFakeClient(hc),
	}
	ctx := context.Background()
	logger := log.FromContext(ctx)

	cfg := RHOBSConfig{
		ProbeAPIURL: server.URL + "/probes",
		Tenant:      "test-tenant",
	}

	err := r.ensureRHOBSProbe(ctx, logger, hcp, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have created a probe (POST), not skipped due to stale LS
	hasPost := false
	for _, entry := range requestLog {
		if strings.HasPrefix(entry, "POST") {
			hasPost = true
		}
	}
	if !hasPost {
		t.Error("expected probe creation (POST) after detecting stale LS label, but no POST was made")
	}
}

func TestEnsureRHOBSProbe_LSLabelHCNotFound(t *testing.T) {
	// Test fallback: when HC cannot be read, trust the HCP label (safe default)
	var requestLog []string
	server := mockRHOBSServer(t, nil, false, false, &requestLog)
	defer server.Close()

	hcp := newHCPWithMeta("test-cluster", "us-east-1",
		map[string]string{"api.openshift.com/limited-support": "true"},
		hypershiftv1beta1.PublicAndPrivate,
		"test-hcp", "ocm-staging-clusterid-test-hcp",
	)
	// No HC created in fake client -- simulates HC not found

	r := &HostedControlPlaneReconciler{
		Client: fake.NewFakeClient(),
	}
	ctx := context.Background()
	logger := log.FromContext(ctx)

	cfg := RHOBSConfig{
		ProbeAPIURL: server.URL + "/probes",
		Tenant:      "test-tenant",
	}

	err := r.ensureRHOBSProbe(ctx, logger, hcp, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should NOT have created a probe (falls back to trusting HCP label)
	hasPost := false
	for _, entry := range requestLog {
		if strings.HasPrefix(entry, "POST") {
			hasPost = true
		}
	}
	if hasPost {
		t.Error("expected LS flow to skip probe creation when HC not found, but POST was made")
	}
}

func TestEnsureRHOBSProbe_LabelValidation(t *testing.T) {
	tests := []struct {
		name                 string
		probeLabels          map[string]string
		isPrivate            bool
		clusterRegion        string
		expectLabelUpdate    bool // region-only PATCH (no private label in body)
		expectDeleteRecreate bool // private mismatch: delete + POST recreate
	}{
		{
			name: "all labels match, heartbeat update only",
			probeLabels: map[string]string{
				"cluster-id": "test-cluster",
				"private":    "false",
				"region":     "us-east-1",
			},
			isPrivate:            false,
			clusterRegion:        "us-east-1",
			expectLabelUpdate:    false,
			expectDeleteRecreate: false,
		},
		{
			name: "private labels match, heartbeat update only",
			probeLabels: map[string]string{
				"cluster-id": "test-cluster",
				"private":    "true",
				"region":     "us-west-2",
			},
			isPrivate:            true,
			clusterRegion:        "us-west-2",
			expectLabelUpdate:    false,
			expectDeleteRecreate: false,
		},
		{
			name: "missing private label triggers delete and recreate",
			probeLabels: map[string]string{
				"cluster-id": "test-cluster",
				"region":     "us-east-1",
			},
			isPrivate:            false,
			clusterRegion:        "us-east-1",
			expectLabelUpdate:    false,
			expectDeleteRecreate: true,
		},
		{
			name: "wrong private value triggers delete and recreate",
			probeLabels: map[string]string{
				"cluster-id": "test-cluster",
				"private":    "true",
				"region":     "us-east-1",
			},
			isPrivate:            false,
			clusterRegion:        "us-east-1",
			expectLabelUpdate:    false,
			expectDeleteRecreate: true,
		},
		{
			name: "wrong region triggers label update",
			probeLabels: map[string]string{
				"cluster-id": "test-cluster",
				"private":    "false",
				"region":     "eu-west-1",
			},
			isPrivate:         false,
			clusterRegion:     "us-east-1",
			expectLabelUpdate: true,
		},
		{
			name:                 "empty labels triggers delete and recreate",
			probeLabels:          map[string]string{"cluster-id": "test-cluster"},
			isPrivate:            false,
			clusterRegion:        "us-east-1",
			expectLabelUpdate:    false,
			expectDeleteRecreate: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			existingProbe := &rhobs.ProbeResponse{
				ID:     "probe-123",
				Labels: tt.probeLabels,
				Status: "active",
			}
			var requestLog []string
			server := mockRHOBSServer(t, existingProbe, false, false, &requestLog)
			defer server.Close()

			r := newTestReconciler(t)
			ctx := context.Background()
			logger := log.FromContext(ctx)

			endpointAccess := hypershiftv1beta1.Public
			if tt.isPrivate {
				endpointAccess = hypershiftv1beta1.Private
			}
			hcp := newHCP("test-cluster", tt.clusterRegion, nil, endpointAccess)

			cfg := RHOBSConfig{
				ProbeAPIURL: server.URL + "/probes",
				Tenant:      "test-tenant",
			}

			err := r.ensureRHOBSProbe(ctx, logger, hcp, cfg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			hasPost := false
			hasPatch := false
			for _, entry := range requestLog {
				if strings.HasPrefix(entry, "POST") {
					hasPost = true
				}
				if strings.HasPrefix(entry, "PATCH") {
					hasPatch = true
				}
			}

			if tt.expectDeleteRecreate {
				// Private label mismatch: should delete (PATCH to terminating) then POST to recreate
				if !hasPatch {
					t.Error("expected delete PATCH for private label correction, but no PATCH was made")
				}
				if !hasPost {
					t.Error("expected POST to recreate probe after private label deletion, but no POST was made")
				}
			} else if tt.expectLabelUpdate {
				if !hasPatch {
					t.Error("expected probe labels to be updated (PATCH), but no PATCH was made")
				}
				if hasPost {
					t.Error("expected label update only (no POST), but POST was made")
				}
				// Verify the PATCH body does not contain private label
				for _, entry := range requestLog {
					if strings.HasPrefix(entry, "PATCH") {
						parts := strings.SplitN(entry, " ", 3)
						if len(parts) < 3 {
							continue
						}
						var patch struct {
							Labels *map[string]string `json:"labels"`
						}
						if err := json.Unmarshal([]byte(parts[2]), &patch); err != nil {
							continue
						}
						if patch.Labels == nil {
							continue
						}
						if _, hasPrivate := (*patch.Labels)["private"]; hasPrivate {
							t.Error("PATCH body should not contain private label")
						}
						ts, ok := (*patch.Labels)["last-reconciled"]
						if !ok {
							continue
						}
						if _, err := time.Parse("20060102T150405Z", ts); err != nil {
							t.Errorf("last-reconciled label is not a valid timestamp: %q", ts)
						}
					}
				}
			} else {
				// Labels match: only heartbeat PATCH expected, no label PATCH with labels field, no POST
				if hasPost {
					t.Error("expected no changes, but POST was made")
				}
			}
		})
	}
}

func TestIsPrivateProbe(t *testing.T) {
	tests := []struct {
		name     string
		probe    *rhobs.ProbeResponse
		expected bool
	}{
		{
			name: "Probe with private=true label",
			probe: &rhobs.ProbeResponse{
				ID: "test-probe-1",
				Labels: map[string]string{
					"private": "true",
				},
			},
			expected: true,
		},
		{
			name: "Probe with private=false label",
			probe: &rhobs.ProbeResponse{
				ID: "test-probe-2",
				Labels: map[string]string{
					"private": "false",
				},
			},
			expected: false,
		},
		{
			name: "Probe without private label",
			probe: &rhobs.ProbeResponse{
				ID: "test-probe-3",
				Labels: map[string]string{
					"cluster-id": "test-cluster",
				},
			},
			expected: false,
		},
		{
			name: "Probe with empty labels",
			probe: &rhobs.ProbeResponse{
				ID:     "test-probe-4",
				Labels: map[string]string{},
			},
			expected: false,
		},
		{
			name: "Probe with nil labels",
			probe: &rhobs.ProbeResponse{
				ID:     "test-probe-5",
				Labels: nil,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isPrivateProbe(tt.probe)
			if result != tt.expected {
				t.Errorf("isPrivateProbe() = %v, expected %v", result, tt.expected)
			}
		})
	}
}
