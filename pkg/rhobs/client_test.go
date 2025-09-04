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

		if r.URL.Path != "/test-tenant/metrics/probes" {
			t.Errorf("Expected path /test-tenant/metrics/probes, got %s", r.URL.Path)
		}

		var req ProbeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("Failed to decode request: %v", err)
		}

		if req.ClusterID != "test-cluster" {
			t.Errorf("Expected cluster_id test-cluster, got %s", req.ClusterID)
		}

		// Return a mock response
		resp := ProbeResponse{
			ID:        "probe-123",
			ClusterID: "test-cluster",
			Status:    "active",
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
		ClusterID:    "test-cluster",
		APIServerURL: "https://api.test-cluster.example.com/livez",
		Private:      false,
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
		expectedSelector := "cluster_id=test-cluster"
		if labelSelector != expectedSelector {
			t.Errorf("Expected label_selector %s, got %s", expectedSelector, labelSelector)
		}

		// Return a mock response
		resp := ProbesListResponse{
			Probes: []ProbeResponse{
				{
					ID:        "probe-123",
					ClusterID: "test-cluster",
					Status:    "active",
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
						ID:        "probe-123",
						ClusterID: "test-cluster",
						Status:    "active",
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

		if r.URL.Path != "/test-tenant/metrics/probes/test-cluster" {
			t.Errorf("Expected path /test-tenant/metrics/probes/test-cluster, got %s", r.URL.Path)
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
				ClusterID:    "test-cluster",
				APIServerURL: "https://api.test-cluster.example.com/livez",
				Private:      false,
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

func TestCreateProbe_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-tenant", testr.New(t))
	probeReq := ProbeRequest{
		ClusterID:    "test-cluster",
		APIServerURL: "https://api.test-cluster.example.com/livez",
		Private:      false,
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
					ID:        "probe-123",
					ClusterID: "different-cluster",
					Status:    "active",
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
						ID:        "probe-123",
						ClusterID: "test-cluster",
						Status:    "active",
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
		ClusterID:    "test-cluster",
		APIServerURL: "https://api.test-cluster.example.com/livez",
		Private:      false,
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
						ID:        "probe-123",
						ClusterID: "test-cluster",
						Status:    "failed",
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
						ID:        "probe-123",
						ClusterID: "test-cluster",
						Status:    "active",
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
		if labelSelector != "cluster_id=my-cluster-123" {
			t.Errorf("Expected label_selector 'cluster_id=my-cluster-123', got '%s'", labelSelector)
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

// APIError represents an API error for testing
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return e.Message
}
