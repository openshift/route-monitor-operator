package rhobs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-logr/logr/testr"
)

func TestCreateProbe(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST method, got %s", r.Method)
		}

		if r.URL.Path != "/api/metrics/v1/test-tenant/probes" {
			t.Errorf("Expected path /api/metrics/v1/test-tenant/probes, got %s", r.URL.Path)
		}

		var req ProbeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("Failed to decode request: %v", err)
		}

		if req.Labels["cluster-id"] != "test-cluster" {
			t.Errorf("Expected cluster-id test-cluster, got %s", req.Labels["cluster-id"])
		}

		// Check tenant header
		if r.Header.Get("X-Tenant") != "test-tenant" {
			t.Errorf("Expected X-Tenant header test-tenant, got %s", r.Header.Get("X-Tenant"))
		}

		// Check username header (should be empty for non-OIDC client)
		if r.Header.Get("X-Username") != "" {
			t.Errorf("Expected empty X-Username header for non-OIDC client, got %s", r.Header.Get("X-Username"))
		}

		// Return a mock response
		resp := ProbeResponse{
			ID:     "probe-123",
			Labels: map[string]string{"cluster-id": "test-cluster"},
			Status: "active",
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create client
	client := NewClient(server.URL, "test-tenant", testr.New(t))

	// Test create probe
	probeReq := ProbeRequest{
		StaticURL: "https://api.test-cluster.example.com/livez",
		Labels: map[string]string{
			"cluster-id": "test-cluster",
			"private":    "false",
		},
	}

	probe, err := client.CreateProbe(context.Background(), probeReq)
	if err != nil {
		t.Fatalf("CreateProbe failed: %v", err)
	}

	if probe.ID != "probe-123" {
		t.Errorf("Expected probe ID probe-123, got %s", probe.ID)
	}
}

func TestGetProbe(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("Expected GET method, got %s", r.Method)
		}

		labelSelector := r.URL.Query().Get("label_selector")
		expectedSelector := "cluster-id=test-cluster"
		if labelSelector != expectedSelector {
			t.Errorf("Expected label_selector %s, got %s", expectedSelector, labelSelector)
		}

		// Check tenant header
		if r.Header.Get("X-Tenant") != "test-tenant" {
			t.Errorf("Expected X-Tenant header test-tenant, got %s", r.Header.Get("X-Tenant"))
		}

		// Check username header (should be empty for non-OIDC client)
		if r.Header.Get("X-Username") != "" {
			t.Errorf("Expected empty X-Username header for non-OIDC client, got %s", r.Header.Get("X-Username"))
		}

		// Return a mock response
		resp := ProbesListResponse{
			Probes: []ProbeResponse{
				{
					ID:     "probe-123",
					Labels: map[string]string{"cluster-id": "test-cluster"},
					Status: "active",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create client
	client := NewClient(server.URL, "test-tenant", testr.New(t))

	// Test get probe
	probe, err := client.GetProbe(context.Background(), "test-cluster")
	if err != nil {
		t.Fatalf("GetProbe failed: %v", err)
	}

	if probe == nil {
		t.Fatal("Expected probe to be found, got nil")
	}

	if probe.ID != "probe-123" {
		t.Errorf("Expected probe ID probe-123, got %s", probe.ID)
	}
}

func TestDeleteProbe(t *testing.T) {
	// First call to GetProbe to check existing probe
	getCallCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			getCallCount++
			// Return existing probe on first call
			resp := ProbesListResponse{
				Probes: []ProbeResponse{
					{
						ID:     "probe-123",
						Labels: map[string]string{"cluster-id": "test-cluster"},
						Status: "active",
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		if r.Method != "PATCH" {
			t.Errorf("Expected PATCH method, got %s", r.Method)
		}

		if r.URL.Path != "/api/metrics/v1/test-tenant/probes/probe-123" {
			t.Errorf("Expected path /api/metrics/v1/test-tenant/probes/probe-123, got %s", r.URL.Path)
		}

		// Verify PATCH payload
		var req ProbePatchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("Failed to decode PATCH request: %v", err)
		}

		if req.Status != "terminating" {
			t.Errorf("Expected status 'terminating', got %s", req.Status)
		}

		// Check tenant header
		if r.Header.Get("X-Tenant") != "test-tenant" {
			t.Errorf("Expected X-Tenant header test-tenant, got %s", r.Header.Get("X-Tenant"))
		}

		// Check username header (should be empty for non-OIDC client)
		if r.Header.Get("X-Username") != "" {
			t.Errorf("Expected empty X-Username header for non-OIDC client, got %s", r.Header.Get("X-Username"))
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create client
	client := NewClient(server.URL, "test-tenant", testr.New(t))

	// Test delete probe
	err := client.DeleteProbe(context.Background(), "test-cluster")
	if err != nil {
		t.Fatalf("DeleteProbe failed: %v", err)
	}

	if getCallCount != 1 {
		t.Errorf("Expected 1 GET call, got %d", getCallCount)
	}
}

func TestCreateProbe_Errors(t *testing.T) {
	tests := []struct {
		name          string
		statusCode    int
		responseBody  string
		expectedError string
	}{
		{
			name:          "server error",
			statusCode:    http.StatusInternalServerError,
			responseBody:  "Internal Server Error",
			expectedError: "API request failed with status 500",
		},
		{
			name:          "bad request",
			statusCode:    http.StatusBadRequest,
			responseBody:  "Bad Request",
			expectedError: "API request failed with status 400",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			client := NewClient(server.URL, "test-tenant", testr.New(t))
			probeReq := ProbeRequest{
				StaticURL: "https://api.test-cluster.example.com/livez",
				Labels: map[string]string{
					"cluster-id": "test-cluster",
					"private":    "false",
				},
			}

			_, err := client.CreateProbe(context.Background(), probeReq)
			if err == nil {
				t.Fatal("Expected error, got nil")
			}

			if !strings.Contains(err.Error(), tt.expectedError) {
				t.Errorf("Expected error to contain %q, got %q", tt.expectedError, err.Error())
			}
		})
	}
}

func TestCreateProbe_Conflict(t *testing.T) {
	// Test that 409 conflict is treated as success
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":{"message":"a probe for static_url \"https://api.test-cluster.example.com/livez\" already exists"}}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-tenant", testr.New(t))
	probeReq := ProbeRequest{
		StaticURL: "https://api.test-cluster.example.com/livez",
		Labels: map[string]string{
			"cluster-id": "test-cluster",
			"private":    "false",
		},
	}

	probe, err := client.CreateProbe(context.Background(), probeReq)
	if err != nil {
		t.Fatalf("CreateProbe should succeed on 409 conflict, got error: %v", err)
	}

	if probe == nil {
		t.Fatal("Expected probe response, got nil")
	}

	if probe.ID != "existing" {
		t.Errorf("Expected probe ID 'existing', got %s", probe.ID)
	}

	if probe.Status != "active" {
		t.Errorf("Expected probe status 'active', got %s", probe.Status)
	}

	if probe.Labels["cluster-id"] != "test-cluster" {
		t.Errorf("Expected cluster-id 'test-cluster', got %s", probe.Labels["cluster-id"])
	}
}

func TestCreateProbe_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-tenant", testr.New(t))
	probeReq := ProbeRequest{
		StaticURL: "https://api.test-cluster.example.com/livez",
		Labels: map[string]string{
			"cluster-id": "test-cluster",
			"private":    "false",
		},
	}

	_, err := client.CreateProbe(context.Background(), probeReq)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	if !strings.Contains(err.Error(), "failed to unmarshal response") {
		t.Errorf("Expected unmarshal error, got %q", err.Error())
	}
}

func TestGetProbe_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-tenant", testr.New(t))

	probe, err := client.GetProbe(context.Background(), "non-existent-cluster")
	if err != nil {
		t.Fatalf("GetProbe failed: %v", err)
	}

	if probe != nil {
		t.Error("Expected nil probe for 404 response")
	}
}

func TestGetProbe_EmptyList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ProbesListResponse{
			Probes: []ProbeResponse{},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-tenant", testr.New(t))

	probe, err := client.GetProbe(context.Background(), "test-cluster")
	if err != nil {
		t.Fatalf("GetProbe failed: %v", err)
	}

	if probe != nil {
		t.Error("Expected nil probe for empty list")
	}
}

func TestGetProbe_NoMatchingCluster(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ProbesListResponse{
			Probes: []ProbeResponse{
				{
					ID:     "probe-123",
					Labels: map[string]string{"cluster-id": "different-cluster"},
					Status: "active",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-tenant", testr.New(t))

	probe, err := client.GetProbe(context.Background(), "test-cluster")
	if err != nil {
		t.Fatalf("GetProbe failed: %v", err)
	}

	if probe != nil {
		t.Error("Expected nil probe when no matching cluster_id")
	}
}

func TestGetProbe_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-tenant", testr.New(t))

	_, err := client.GetProbe(context.Background(), "test-cluster")
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	if !strings.Contains(err.Error(), "API request failed with status 500") {
		t.Errorf("Expected status error, got %q", err.Error())
	}
}

func TestGetProbe_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-tenant", testr.New(t))

	_, err := client.GetProbe(context.Background(), "test-cluster")
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	if !strings.Contains(err.Error(), "failed to unmarshal response") {
		t.Errorf("Expected unmarshal error, got %q", err.Error())
	}
}

func TestDeleteProbe_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			// Return empty list (probe not found)
			resp := ProbesListResponse{
				Probes: []ProbeResponse{},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-tenant", testr.New(t))

	err := client.DeleteProbe(context.Background(), "non-existent-cluster")
	if err != nil {
		t.Fatalf("DeleteProbe should succeed when probe doesn't exist, got error: %v", err)
	}
}

func TestDeleteProbe_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			// Return existing probe
			resp := ProbesListResponse{
				Probes: []ProbeResponse{
					{
						ID:     "probe-123",
						Labels: map[string]string{"cluster-id": "test-cluster"},
						Status: "active",
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-tenant", testr.New(t))

	err := client.DeleteProbe(context.Background(), "test-cluster")
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	if !strings.Contains(err.Error(), "API request failed with status 500") {
		t.Errorf("Expected status error, got %q", err.Error())
	}
}

func TestIsNon200Error(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "non-API error",
			err:      http.ErrUseLastResponse,
			expected: false,
		},
		{
			name:     "API error with status code",
			err:      fmt.Errorf("API request failed with status 400: Bad Request"),
			expected: true,
		},
		{
			name:     "other error with API text",
			err:      fmt.Errorf("some other API request failed with status in message"),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsNon200Error(tt.err)
			if result != tt.expected {
				t.Errorf("IsNon200Error() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestCreateProbe_NetworkError(t *testing.T) {
	// Use invalid URL to trigger HTTP client error
	client := NewClient("http://invalid-host-12345.invalid", "test-tenant", testr.New(t))
	probeReq := ProbeRequest{
		StaticURL: "https://api.test-cluster.example.com/livez",
		Labels: map[string]string{
			"cluster-id": "test-cluster",
			"private":    "false",
		},
	}

	_, err := client.CreateProbe(context.Background(), probeReq)
	if err == nil {
		t.Fatal("Expected network error, got nil")
	}

	if !strings.Contains(err.Error(), "failed to send HTTP request") {
		t.Errorf("Expected network error, got %q", err.Error())
	}
}

func TestGetProbe_NetworkError(t *testing.T) {
	// Use invalid URL to trigger HTTP client error
	client := NewClient("http://invalid-host-12345.invalid", "test-tenant", testr.New(t))

	_, err := client.GetProbe(context.Background(), "test-cluster")
	if err == nil {
		t.Fatal("Expected network error, got nil")
	}

	if !strings.Contains(err.Error(), "failed to send HTTP request") {
		t.Errorf("Expected network error, got %q", err.Error())
	}
}

func TestDeleteProbe_NetworkError(t *testing.T) {
	// Use invalid URL to trigger HTTP client error
	client := NewClient("http://invalid-host-12345.invalid", "test-tenant", testr.New(t))

	err := client.DeleteProbe(context.Background(), "test-cluster")
	if err == nil {
		t.Fatal("Expected network error, got nil")
	}

	if !strings.Contains(err.Error(), "failed to check existing probe") {
		t.Errorf("Expected check error, got %q", err.Error())
	}
}

func TestDeleteProbe_FailedProbeHandling(t *testing.T) {
	// Test that failed probes are handled appropriately
	getCallCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			getCallCount++
			// Return failed probe on first call
			resp := ProbesListResponse{
				Probes: []ProbeResponse{
					{
						ID:     "probe-123",
						Labels: map[string]string{"cluster-id": "test-cluster"},
						Status: "failed",
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		if r.Method != "PATCH" {
			t.Errorf("Expected PATCH method, got %s", r.Method)
		}

		// Verify PATCH payload
		var req ProbePatchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("Failed to decode PATCH request: %v", err)
		}

		if req.Status != "terminating" {
			t.Errorf("Expected status 'terminating', got %s", req.Status)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create client
	client := NewClient(server.URL, "test-tenant", testr.New(t))

	// Test delete probe with failed status
	err := client.DeleteProbe(context.Background(), "test-cluster")
	if err != nil {
		t.Fatalf("DeleteProbe failed: %v", err)
	}

	if getCallCount != 1 {
		t.Errorf("Expected 1 GET call, got %d", getCallCount)
	}
}

func TestDeleteProbe_GetProbeError(t *testing.T) {
	// Test error handling when GetProbe fails
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("Get error"))
			return
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-tenant", testr.New(t))

	err := client.DeleteProbe(context.Background(), "test-cluster")
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	if !strings.Contains(err.Error(), "failed to check existing probe") {
		t.Errorf("Expected check error, got %q", err.Error())
	}
}

func TestDeleteProbe_PatchError(t *testing.T) {
	// Test error handling when PATCH fails
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			// Return existing probe
			resp := ProbesListResponse{
				Probes: []ProbeResponse{
					{
						ID:     "probe-123",
						Labels: map[string]string{"cluster-id": "test-cluster"},
						Status: "active",
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		if r.Method == "PATCH" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("PATCH error"))
			return
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-tenant", testr.New(t))

	err := client.DeleteProbe(context.Background(), "test-cluster")
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	if !strings.Contains(err.Error(), "API request failed with status 400") {
		t.Errorf("Expected PATCH error, got %q", err.Error())
	}
}

func TestGetProbe_LabelSelectorFormat(t *testing.T) {
	// Test that label_selector parameter is properly formatted
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("Expected GET method, got %s", r.Method)
		}

		labelSelector := r.URL.Query().Get("label_selector")
		if labelSelector != "cluster-id=my-cluster-123" {
			t.Errorf("Expected label_selector 'cluster-id=my-cluster-123', got '%s'", labelSelector)
		}

		// Return empty list
		resp := ProbesListResponse{
			Probes: []ProbeResponse{},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-tenant", testr.New(t))

	// Test with cluster ID that contains hyphens and numbers
	probe, err := client.GetProbe(context.Background(), "my-cluster-123")
	if err != nil {
		t.Fatalf("GetProbe failed: %v", err)
	}

	if probe != nil {
		t.Error("Expected nil probe for empty list")
	}
}

func TestNewClientWithOIDC(t *testing.T) {
	oidcConfig := OIDCConfig{
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		IssuerURL:    "https://auth.example.com",
	}

	client := NewClientWithOIDC("https://api.example.com", "test-tenant", oidcConfig, testr.New(t))

	if client.oidcConfig == nil {
		t.Fatal("Expected OIDC config to be set")
	}

	if client.oidcConfig.ClientID != "test-client" {
		t.Errorf("Expected client ID 'test-client', got %s", client.oidcConfig.ClientID)
	}

	if client.oidcConfig.ClientSecret != "test-secret" {
		t.Errorf("Expected client secret 'test-secret', got %s", client.oidcConfig.ClientSecret)
	}

	if client.oidcConfig.IssuerURL != "https://auth.example.com" {
		t.Errorf("Expected issuer URL 'https://auth.example.com', got %s", client.oidcConfig.IssuerURL)
	}
}

func TestOIDCTokenFlow(t *testing.T) {
	// Mock OIDC token server
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			t.Errorf("Expected token path, got %s", r.URL.Path)
		}

		if r.Method != "POST" {
			t.Errorf("Expected POST method, got %s", r.Method)
		}

		if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Errorf("Expected application/x-www-form-urlencoded content type, got %s", r.Header.Get("Content-Type"))
		}

		// Parse form data
		if err := r.ParseForm(); err != nil {
			t.Fatalf("Failed to parse form: %v", err)
		}

		if r.Form.Get("grant_type") != "client_credentials" {
			t.Errorf("Expected grant_type 'client_credentials', got %s", r.Form.Get("grant_type"))
		}

		if r.Form.Get("client_id") != "test-client" {
			t.Errorf("Expected client_id 'test-client', got %s", r.Form.Get("client_id"))
		}

		if r.Form.Get("client_secret") != "test-secret" {
			t.Errorf("Expected client_secret 'test-secret', got %s", r.Form.Get("client_secret"))
		}

		// Return mock token response
		tokenResp := tokenResponse{
			AccessToken: "mock-access-token",
			TokenType:   "Bearer",
			ExpiresIn:   3600,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tokenResp)
	}))
	defer tokenServer.Close()

	// Mock API server that expects Bearer token
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer mock-access-token" {
			t.Errorf("Expected Authorization 'Bearer mock-access-token', got %s", auth)
		}

		// Check tenant header
		tenant := r.Header.Get("X-Tenant")
		if tenant != "test-tenant" {
			t.Errorf("Expected X-Tenant 'test-tenant', got %s", tenant)
		}

		// Check username header (should be client ID for OIDC client)
		username := r.Header.Get("X-Username")
		if username != "test-client" {
			t.Errorf("Expected X-Username 'test-client', got %s", username)
		}

		// Return mock probe response
		resp := ProbeResponse{
			ID:     "probe-123",
			Labels: map[string]string{"cluster-id": "test-cluster"},
			Status: "active",
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer apiServer.Close()

	// Create OIDC client
	oidcConfig := OIDCConfig{
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		IssuerURL:    tokenServer.URL,
	}

	client := NewClientWithOIDC(apiServer.URL, "test-tenant", oidcConfig, testr.New(t))

	// Test creating a probe with OIDC authentication
	probeReq := ProbeRequest{
		StaticURL: "https://api.test-cluster.example.com/livez",
		Labels: map[string]string{
			"cluster-id": "test-cluster",
			"private":    "false",
		},
	}

	probe, err := client.CreateProbe(context.Background(), probeReq)
	if err != nil {
		t.Fatalf("CreateProbe with OIDC failed: %v", err)
	}

	if probe.ID != "probe-123" {
		t.Errorf("Expected probe ID probe-123, got %s", probe.ID)
	}
}

func TestOIDCTokenError(t *testing.T) {
	// Mock OIDC token server that returns error
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid_client"))
	}))
	defer tokenServer.Close()

	// Create OIDC client
	oidcConfig := OIDCConfig{
		ClientID:     "invalid-client",
		ClientSecret: "invalid-secret",
		IssuerURL:    tokenServer.URL,
	}

	client := NewClientWithOIDC("https://api.example.com", "test-tenant", oidcConfig, testr.New(t))

	// Test creating a probe should fail due to OIDC error
	probeReq := ProbeRequest{
		StaticURL: "https://api.test-cluster.example.com/livez",
		Labels: map[string]string{
			"cluster-id": "test-cluster",
			"private":    "false",
		},
	}

	_, err := client.CreateProbe(context.Background(), probeReq)
	if err == nil {
		t.Fatal("Expected error due to OIDC token failure, got nil")
	}

	if !strings.Contains(err.Error(), "failed to add auth headers") {
		t.Errorf("Expected auth headers error, got %q", err.Error())
	}
}

func TestClientWithoutOIDC(t *testing.T) {
	// Test that regular client (without OIDC) doesn't add auth headers
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "" {
			t.Errorf("Expected no Authorization header, got %s", auth)
		}

		// Return mock probe response
		resp := ProbeResponse{
			ID:     "probe-123",
			Labels: map[string]string{"cluster-id": "test-cluster"},
			Status: "active",
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create regular client (no OIDC)
	client := NewClient(server.URL, "test-tenant", testr.New(t))

	probeReq := ProbeRequest{
		StaticURL: "https://api.test-cluster.example.com/livez",
		Labels: map[string]string{
			"cluster-id": "test-cluster",
			"private":    "false",
		},
	}

	_, err := client.CreateProbe(context.Background(), probeReq)
	if err != nil {
		t.Fatalf("CreateProbe without OIDC failed: %v", err)
	}
}

func TestOIDCTokenCaching(t *testing.T) {
	tokenRequestCount := 0
	// Mock OIDC token server
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenRequestCount++

		// Return mock token response
		tokenResp := tokenResponse{
			AccessToken: fmt.Sprintf("mock-access-token-%d", tokenRequestCount),
			TokenType:   "Bearer",
			ExpiresIn:   3600, // 1 hour
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tokenResp)
	}))
	defer tokenServer.Close()

	// Mock API server
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Just return success for any request
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}))
	defer apiServer.Close()

	// Create OIDC client
	oidcConfig := OIDCConfig{
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		IssuerURL:    tokenServer.URL,
	}

	client := NewClientWithOIDC(apiServer.URL, "test-tenant", oidcConfig, testr.New(t))

	// Make multiple requests - should reuse token
	for i := 0; i < 3; i++ {
		probeReq := ProbeRequest{
			StaticURL: "https://api.test-cluster.example.com/livez",
			Labels: map[string]string{
				"cluster-id": fmt.Sprintf("test-cluster-%d", i),
				"private":    "false",
			},
		}

		_, err := client.CreateProbe(context.Background(), probeReq)
		if err != nil {
			t.Fatalf("CreateProbe %d failed: %v", i, err)
		}
	}

	// Should only have made one token request due to caching
	if tokenRequestCount != 1 {
		t.Errorf("Expected 1 token request due to caching, got %d", tokenRequestCount)
	}
}

func TestOIDCTokenURL(t *testing.T) {
	tests := []struct {
		name         string
		issuerURL    string
		expectedPath string
		description  string
	}{
		{
			name:         "issuer URL without token path",
			issuerURL:    "https://auth.example.com",
			expectedPath: "/token",
			description:  "Should append /token to issuer URL",
		},
		{
			name:         "issuer URL with trailing slash",
			issuerURL:    "https://auth.example.com/",
			expectedPath: "/token",
			description:  "Should append /token to issuer URL with trailing slash",
		},
		{
			name:         "direct token endpoint URL",
			issuerURL:    "https://sso.redhat.com/auth/realms/redhat-external/protocol/openid-connect/token",
			expectedPath: "/auth/realms/redhat-external/protocol/openid-connect/token",
			description:  "Should use direct token endpoint URL as-is",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock token server that tracks the request path
			var requestPath string
			tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestPath = r.URL.Path

				// Return mock token response
				tokenResp := tokenResponse{
					AccessToken: "test-token",
					TokenType:   "Bearer",
					ExpiresIn:   3600,
				}

				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(tokenResp)
			}))
			defer tokenServer.Close()

			// For direct token endpoint test, use the full URL; for others, use server URL
			var issuerURL string
			if tt.name == "direct token endpoint URL" {
				issuerURL = tokenServer.URL + "/auth/realms/redhat-external/protocol/openid-connect/token"
			} else {
				// Replace the example domain with the test server URL
				issuerURL = strings.Replace(tt.issuerURL, "https://auth.example.com", tokenServer.URL, 1)
			}

			// Create OIDC client
			oidcConfig := OIDCConfig{
				ClientID:     "test-client",
				ClientSecret: "test-secret",
				IssuerURL:    issuerURL,
			}

			client := NewClientWithOIDC("https://api.example.com", "test-tenant", oidcConfig, testr.New(t))

			// Request token
			_, err := client.GetAccessToken(context.Background())
			if err != nil {
				t.Fatalf("GetAccessToken failed: %v", err)
			}

			// Verify the request path
			if requestPath != tt.expectedPath {
				t.Errorf("Expected request path %s, got %s. %s", tt.expectedPath, requestPath, tt.description)
			}
		})
	}
}

func TestFullURLSupport(t *testing.T) {
	tests := []struct {
		name        string
		baseURL     string
		expectedURL string
	}{
		{
			name:        "full URL with /probes endpoint",
			baseURL:     "https://rhobs.us-west-2.api.integration.openshift.com/api/metrics/v1/hcp/probes",
			expectedURL: "https://rhobs.us-west-2.api.integration.openshift.com/api/metrics/v1/hcp/probes",
		},
		{
			name:        "base URL without path",
			baseURL:     "https://rhobs.us-west-2.api.integration.openshift.com",
			expectedURL: "https://rhobs.us-west-2.api.integration.openshift.com/api/metrics/v1/test-tenant/probes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test server that captures the request URL
			var actualURL string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				actualURL = fmt.Sprintf("https://%s%s", r.Host, r.URL.Path)

				// Return a mock response
				resp := ProbeResponse{
					ID:     "probe-123",
					Labels: map[string]string{"cluster-id": "test-cluster"},
					Status: "active",
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(resp)
			}))
			defer server.Close()

			// Replace the example domain with test server for the full URL test
			baseURL := tt.baseURL
			if strings.Contains(baseURL, "rhobs.us-west-2.api.integration.openshift.com") {
				baseURL = strings.Replace(baseURL, "https://rhobs.us-west-2.api.integration.openshift.com", server.URL, 1)
			} else {
				baseURL = server.URL
			}

			client := NewClient(baseURL, "test-tenant", testr.New(t))

			// Test CreateProbe
			req := ProbeRequest{
				StaticURL: "https://api.test-cluster.example.com/livez",
				Labels: map[string]string{
					"cluster-id": "test-cluster",
					"private":    "false",
				},
			}

			_, err := client.CreateProbe(context.Background(), req)
			if err != nil {
				t.Fatalf("CreateProbe failed: %v", err)
			}

			// Verify the URL used
			expectedPath := strings.TrimPrefix(tt.expectedURL, "https://rhobs.us-west-2.api.integration.openshift.com")
			if !strings.HasSuffix(actualURL, expectedPath) {
				t.Errorf("Expected URL to end with %s, got %s", expectedPath, actualURL)
			}
		})
	}
}

func TestFullURLSupportForSpecificProbe(t *testing.T) {
	// Test that the buildProbeURL method works correctly with full URLs
	tests := []struct {
		name        string
		baseURL     string
		clusterID   string
		expectedURL string
	}{
		{
			name:        "full URL with /probes endpoint for specific probe",
			baseURL:     "https://rhobs.us-west-2.api.integration.openshift.com/api/metrics/v1/hcp/probes",
			clusterID:   "test-cluster-123",
			expectedURL: "https://rhobs.us-west-2.api.integration.openshift.com/api/metrics/v1/hcp/probes/probe-123",
		},
		{
			name:        "base URL without path for specific probe",
			baseURL:     "https://rhobs.us-west-2.api.integration.openshift.com",
			clusterID:   "test-cluster-123",
			expectedURL: "https://rhobs.us-west-2.api.integration.openshift.com/api/metrics/v1/test-tenant/probes/probe-123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test server that captures the request URL
			var actualURL string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				actualURL = fmt.Sprintf("https://%s%s", r.Host, r.URL.Path)

				if r.Method == "GET" {
					// Mock GetProbe response
					listResp := ProbesListResponse{
						Probes: []ProbeResponse{{
							ID:     "probe-123",
							Labels: map[string]string{"cluster-id": tt.clusterID},
							Status: "active",
						}},
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(listResp)
				} else {
					// Mock PATCH response
					w.WriteHeader(http.StatusOK)
				}
			}))
			defer server.Close()

			// Replace the example domain with test server for the full URL test
			baseURL := tt.baseURL
			if strings.Contains(baseURL, "rhobs.us-west-2.api.integration.openshift.com") {
				baseURL = strings.Replace(baseURL, "https://rhobs.us-west-2.api.integration.openshift.com", server.URL, 1)
			} else {
				baseURL = server.URL
			}

			client := NewClient(baseURL, "test-tenant", testr.New(t))

			// Test DeleteProbe which uses buildProbeURL
			err := client.DeleteProbe(context.Background(), tt.clusterID)
			if err != nil {
				t.Fatalf("DeleteProbe failed: %v", err)
			}

			// Verify the URL used
			expectedPath := strings.TrimPrefix(tt.expectedURL, "https://rhobs.us-west-2.api.integration.openshift.com")
			if !strings.HasSuffix(actualURL, expectedPath) {
				t.Errorf("Expected URL to end with %s, got %s", expectedPath, actualURL)
			}
		})
	}
}

// APIError represents an API error for testing
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return e.Message
}

func TestNewProbeRequest(t *testing.T) {
	staticURL := "https://example.com/health"
	labels := map[string]string{
		"service": "my-service",
		"env":     "production",
	}

	req := NewProbeRequest(staticURL, labels)

	if req.StaticURL != staticURL {
		t.Errorf("Expected StaticURL %s, got %s", staticURL, req.StaticURL)
	}

	if len(req.Labels) != len(labels) {
		t.Errorf("Expected %d labels, got %d", len(labels), len(req.Labels))
	}

	for key, value := range labels {
		if req.Labels[key] != value {
			t.Errorf("Expected label %s=%s, got %s", key, value, req.Labels[key])
		}
	}
}

func TestNewClusterProbeRequest(t *testing.T) {
	clusterID := "test-cluster-123"
	monitoringURL := "https://api.test-cluster.example.com/livez"
	isPrivate := true

	req := NewClusterProbeRequest(clusterID, monitoringURL, isPrivate)

	if req.StaticURL != monitoringURL {
		t.Errorf("Expected StaticURL %s, got %s", monitoringURL, req.StaticURL)
	}

	if req.Labels["cluster-id"] != clusterID {
		t.Errorf("Expected cluster-id %s, got %s", clusterID, req.Labels["cluster-id"])
	}

	if req.Labels["private"] != "true" {
		t.Errorf("Expected private 'true', got %s", req.Labels["private"])
	}

	// Test with private=false
	req2 := NewClusterProbeRequest(clusterID, monitoringURL, false)
	if req2.Labels["private"] != "false" {
		t.Errorf("Expected private 'false', got %s", req2.Labels["private"])
	}
}
