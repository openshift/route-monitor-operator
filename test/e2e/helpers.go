//go:build e2e
// +build e2e

package e2e

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
	"testing"
	"time"

	awsvpceapi "github.com/openshift/aws-vpce-operator/api/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// helpers.go - Helper functions and mock servers for E2E tests
//
// This file provides:
//   - API client helpers (createProbe, getProbe, deleteProbe, updateProbeStatus)
//   - Mock servers (Dynatrace, probe target endpoints)
//   - Test utilities (log capture and validation)
//   - Kubernetes resource setup (namespaces, secrets, VpcEndpoints)
//
// The updateProbeStatus function is particularly important - it mocks the
// agent's behavior of updating probe status after processing, since the agent
// cannot fully deploy resources without a real Kubernetes cluster.

// testWriter forwards log output to t.Log and captures logs for validation
type testWriter struct {
	t        *testing.T
	logs     []string
	logMutex sync.Mutex
}

func (tw *testWriter) Write(p []byte) (n int, err error) {
	logLine := string(p)
	
	// Clean up log formatting: replace tabs with single spaces and remove trailing newlines
	cleanedLog := strings.ReplaceAll(logLine, "\t", " ")
	cleanedLog = strings.TrimRight(cleanedLog, "\n")
	
	tw.t.Log(cleanedLog)

	tw.logMutex.Lock()
	tw.logs = append(tw.logs, logLine)
	tw.logMutex.Unlock()

	return len(p), nil
}

func (tw *testWriter) ContainsLog(substring string) bool {
	tw.logMutex.Lock()
	defer tw.logMutex.Unlock()

	for _, log := range tw.logs {
		if strings.Contains(log, substring) {
			return true
		}
	}
	return false
}

// createProbeViaAPI creates a probe directly through the API
func createProbeViaAPI(baseURL, clusterID, probeURL string, private bool) (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	probeData := map[string]interface{}{
		"static_url": probeURL,
		"labels": map[string]string{
			"cluster-id":   clusterID,
			"private":      fmt.Sprintf("%t", private),
			"app":          "rhobs-synthetics-probe",
			"source":       "route-monitor-operator",
			"resource_type": "hostedcontrolplane",
			"probe_type":   "blackbox",
		},
		"provider": "aws",
		"region":   "us-east-1",
	}

	jsonData, err := json.Marshal(probeData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal probe data: %w", err)
	}

	req, err := http.NewRequest("POST", baseURL+"/probes", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	probeID, ok := result["id"].(string)
	if !ok {
		return "", fmt.Errorf("probe ID not found in response")
	}

	return probeID, nil
}

// listProbes gets all probes with optional label selector
func listProbes(baseURL, labelSelector string) ([]map[string]interface{}, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	url := baseURL + "/probes"
	if labelSelector != "" {
		url += "?label_selector=" + labelSelector
	}

	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	// Read the response body first to see what we got
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Try to decode as array of objects
	var probes []map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &probes); err != nil {
		// If it fails, maybe it's wrapped in an object - try to extract
		var wrapper map[string]interface{}
		if err2 := json.Unmarshal(bodyBytes, &wrapper); err2 == nil {
			// Check if there's a "probes" or "data" field
			if probesData, ok := wrapper["probes"].([]interface{}); ok {
				// Convert to []map[string]interface{}
				probes = make([]map[string]interface{}, len(probesData))
				for i, p := range probesData {
					if pm, ok := p.(map[string]interface{}); ok {
						probes[i] = pm
					}
				}
				return probes, nil
			}
			if dataField, ok := wrapper["data"].([]interface{}); ok {
				// Convert to []map[string]interface{}
				probes = make([]map[string]interface{}, len(dataField))
				for i, p := range dataField {
					if pm, ok := p.(map[string]interface{}); ok {
						probes[i] = pm
					}
				}
				return probes, nil
			}
		}
		return nil, fmt.Errorf("failed to decode response (got %d bytes): %w\nBody: %s", len(bodyBytes), err, string(bodyBytes[:min(200, len(bodyBytes))]))
	}

	return probes, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

type Probe struct {
	ID        string            `json:"id"`
	StaticURL string            `json:"static_url"`
	Status    string            `json:"status"`
	Labels    map[string]string `json:"labels"`
}

// getProbeByID gets a specific probe by ID
func getProbeByID(baseURL, probeID string) (*Probe, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.Get(baseURL + "/probes/" + probeID)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var probe Probe
	if err := json.NewDecoder(resp.Body).Decode(&probe); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &probe, nil
}

// deleteProbeViaAPI deletes a probe via the API
func deleteProbeViaAPI(baseURL, probeID string) error {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("DELETE", baseURL+"/probes/"+probeID, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// waitForProbeStatus polls the API waiting for a probe to reach the expected status
func waitForProbeStatus(baseURL, probeID, expectedStatus string, timeout time.Duration) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	timeoutChan := time.After(timeout)

	for {
		select {
		case <-timeoutChan:
			probe, _ := getProbeByID(baseURL, probeID)
			currentStatus := "unknown"
			if probe != nil {
				currentStatus = probe.Status
			}
			return fmt.Errorf("timeout waiting for probe status '%s' (current: %s)", expectedStatus, currentStatus)
		case <-ticker.C:
			probe, err := getProbeByID(baseURL, probeID)
			if err == nil && probe.Status == expectedStatus {
				return nil
			}
		}
	}
}

// waitForProbeDeletion polls the API waiting for a probe to be deleted (404 or terminating/deleted status)
func waitForProbeDeletion(baseURL, probeID string, timeout time.Duration) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	timeoutChan := time.After(timeout)

	for {
		select {
		case <-timeoutChan:
			probe, _ := getProbeByID(baseURL, probeID)
			if probe != nil {
				return fmt.Errorf("timeout waiting for probe deletion (still exists with status: %s)", probe.Status)
			}
			return fmt.Errorf("timeout waiting for probe deletion")
		case <-ticker.C:
			probe, err := getProbeByID(baseURL, probeID)
			// Probe is deleted if we get an error (404) or if status is terminating/deleted
			if err != nil {
				return nil // 404 or other error means deleted
			}
			if probe.Status == "terminating" || probe.Status == "deleted" {
				return nil
			}
		}
	}
}

// updateProbeStatus updates a probe's status via PATCH request
//
// This function mocks the agent's behavior for local testing.
//
// In a real environment with Kubernetes:
//   1. Agent fetches probe from API (status: "pending")
//   2. Agent deploys Prometheus + blackbox-exporter resources to K8s
//   3. Agent updates probe status to "active" via API
//
// In this local test without Kubernetes:
//   1. Agent fetches probe from API (status: "pending") ✅ Works
//   2. Agent cannot deploy K8s resources ❌ No cluster
//   3. Test calls this function to simulate step 3 ✅ Mock
func updateProbeStatus(baseURL, probeID, status string) error {
	client := &http.Client{Timeout: 10 * time.Second}

	patchData := map[string]interface{}{
		"status": status,
	}

	jsonData, err := json.Marshal(patchData)
	if err != nil {
		return fmt.Errorf("failed to marshal patch data: %w", err)
	}

	req, err := http.NewRequest(http.MethodPatch, baseURL+"/probes/"+probeID, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create patch request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send patch request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("patch probe returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// setupRMODependencies creates the Kubernetes resources that RMO expects to exist
func setupRMODependencies(t *testing.T, k8sClient client.Client, ctx context.Context, dynatraceURL string) {
	// Create clusters namespace for HostedControlPlane resources
	clustersNs := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "clusters",
		},
	}
	if err := k8sClient.Create(ctx, clustersNs); err != nil && !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("Failed to create namespace: %v", err)
	}

	// Create VpcEndpoint resource (required for Private HCPs)
	vpcEndpoint := &awsvpceapi.VpcEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "private-hcp",
			Namespace: "clusters",
		},
		Spec: awsvpceapi.VpcEndpointSpec{},
		Status: awsvpceapi.VpcEndpointStatus{
			Status: "Ready",
		},
	}
	if err := k8sClient.Create(ctx, vpcEndpoint); err != nil && !strings.Contains(err.Error(), "already exists") {
		t.Logf("Warning: Failed to create VpcEndpoint: %v", err)
	}
	// Update status (status is a subresource)
	vpcEndpoint.Status.Status = "Ready"
	if err := k8sClient.Status().Update(ctx, vpcEndpoint); err != nil {
		t.Logf("Warning: Failed to update VpcEndpoint status: %v", err)
	}

	// Create openshift-route-monitor-operator namespace
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "openshift-route-monitor-operator",
		},
	}
	if err := k8sClient.Create(ctx, ns); err != nil && !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("Failed to create namespace: %v", err)
	}

	// Create Dynatrace secret
	dynatraceSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dynatrace-token",
			Namespace: "openshift-route-monitor-operator",
		},
		Data: map[string][]byte{
			"apiToken": []byte("mock-token"),
			"apiUrl":   []byte(dynatraceURL),
		},
	}
	if err := k8sClient.Create(ctx, dynatraceSecret); err != nil && !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("Failed to create Dynatrace secret: %v", err)
	}
}

// startMockProbeTargetServer starts a mock server that simulates a healthy cluster API endpoint
func startMockProbeTargetServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/livez" || r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
		}
	}))
}

// startMockDynatraceServer starts a mock Dynatrace server for testing
func startMockDynatraceServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock all Dynatrace API endpoints
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		// Return appropriate mock responses based on the path
		if strings.Contains(r.URL.Path, "/synthetic/monitors") {
			// Mock monitor creation response
			_, _ = w.Write([]byte(`{"entityId":"SYNTHETIC_TEST-1234567890"}`))
		} else {
			// Generic success response
			_, _ = w.Write([]byte(`{"success":true}`))
		}
	}))
}
