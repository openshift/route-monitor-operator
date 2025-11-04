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

func (tw *testWriter) GetLogs() []string {
	tw.logMutex.Lock()
	defer tw.logMutex.Unlock()

	logs := make([]string, len(tw.logs))
	copy(logs, tw.logs)
	return logs
}

// Probe represents a probe configuration
type Probe struct {
	ID        string            `json:"id,omitempty"`
	StaticURL string            `json:"static_url"`
	Labels    map[string]string `json:"labels"`
	Status    string            `json:"status,omitempty"`
}

// ProbeListResponse represents the response from listing probes
type ProbeListResponse struct {
	Probes []Probe `json:"probes"`
}

// createProbeViaAPI creates a probe by calling the API directly (simulates RMO behavior)
func createProbeViaAPI(baseURL, clusterID, probeURL string, isPrivate bool) (string, error) {
	createReq := map[string]interface{}{
		"static_url": probeURL,
		"labels": map[string]string{
			"cluster-id":    clusterID,
			"private":       fmt.Sprintf("%t", isPrivate),
			"source":        "route-monitor-operator",
			"resource_type": "hostedcontrolplane",
			"probe_type":    "blackbox",
		},
	}

	reqBody, err := json.Marshal(createReq)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("POST", baseURL+"/probes", bytes.NewBuffer(reqBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var probe Probe
	if err := json.NewDecoder(resp.Body).Decode(&probe); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	return probe.ID, nil
}

// getProbeByID fetches a single probe by ID from the API
func getProbeByID(baseURL, probeID string) (*Probe, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(baseURL + "/probes/" + probeID)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("probe not found")
	}

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

// listProbes fetches all probes matching the label selector from the API
func listProbes(baseURL, labelSelector string) ([]Probe, error) {
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

	var response ProbeListResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return response.Probes, nil
}

// deleteProbeViaAPI deletes a probe by calling the API directly
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

// setupRMODependencies creates the Kubernetes resources that RMO expects to exist
func setupRMODependencies(t *testing.T, k8sClient client.Client, ctx context.Context, dynatraceURL string) {
	// Create clusters namespace for HostedControlPlane resources
	clustersNs := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "clusters",
		},
	}
	if err := k8sClient.Create(ctx, clustersNs); err != nil && !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("Failed to create clusters namespace: %v", err)
	}

	// Create kube-apiserver service
	apiServerService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kube-apiserver",
			Namespace: "clusters",
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{Port: 6443}},
		},
	}
	if err := k8sClient.Create(ctx, apiServerService); err != nil && !strings.Contains(err.Error(), "already exists") {
		t.Logf("Warning: Failed to create kube-apiserver service: %v", err)
	}

	// Create VpcEndpoint resource
	// RMO expects VpcEndpoint named "private-" + suffix from HCP name
	vpcEndpoint := &awsvpceapi.VpcEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "private-hcp",
			Namespace: "clusters",
			Labels: map[string]string{
				"hypershift.openshift.io/cluster": "test-hcp",
			},
		},
		Status: awsvpceapi.VpcEndpointStatus{
			Status: "available",
		},
	}
	if err := k8sClient.Create(ctx, vpcEndpoint); err != nil && !strings.Contains(err.Error(), "already exists") {
		t.Logf("Warning: Failed to create VpcEndpoint: %v", err)
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
		}
	}))
}

// waitForProbe waits for a probe to be created in the API
func waitForProbe(baseURL, clusterID string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		probes, err := listProbes(baseURL, fmt.Sprintf("cluster-id=%s", clusterID))
		if err == nil && len(probes) > 0 {
			return probes[0].ID, nil
		}
		time.Sleep(2 * time.Second)
	}
	return "", fmt.Errorf("probe not found within timeout")
}

// startMockDynatraceServer starts a mock Dynatrace API server for testing
func startMockDynatraceServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/v1/synthetic/monitors/":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"monitors":[]}`))

		case r.Method == "POST" && r.URL.Path == "/v1/synthetic/monitors":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"entityId":"SYNTHETIC_TEST-1234567890","name":"mock-monitor"}`))

		case r.Method == "DELETE":
			w.WriteHeader(http.StatusNoContent)

		case r.Method == "GET" && r.URL.Path == "/v1/synthetic/locations":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"locations":[
				{"name":"N. Virginia","entityId":"SYNTHETIC_LOCATION-PUBLIC-123","type":"PUBLIC","status":"ENABLED"},
				{"name":"backplanei03xyz","entityId":"SYNTHETIC_LOCATION-PRIVATE-456","type":"PRIVATE","status":"ENABLED"},
				{"name":"Oregon","entityId":"SYNTHETIC_LOCATION-PUBLIC-789","type":"PUBLIC","status":"ENABLED"}
			]}`))

		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"success":true}`))
		}
	}))
}
