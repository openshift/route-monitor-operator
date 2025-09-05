package rhobs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-logr/logr"
)

const (
	// HTTP client configuration
	defaultHTTPTimeout = 30 * time.Second

	// HTTP methods
	httpMethodPost  = "POST"
	httpMethodGet   = "GET"
	httpMethodPatch = "PATCH"

	// API endpoint paths
	probesEndpointPath = "/metrics/probes"
	probeEndpointPath  = "/metrics/probes/%s"

	// HTTP headers
	contentTypeJSON = "application/json"

	// Query parameters
	labelSelectorParam = "label_selector"

	// Log levels
	debugLogLevel = 2

	// Error message prefixes
	apiErrorPrefix = "API request failed with status"
)

// ProbeRequest represents the payload for creating/updating a probe
type ProbeRequest struct {
	ClusterID           string `json:"cluster_id"`
	APIServerURL        string `json:"apiserver_url"`
	ManagementClusterID string `json:"management_cluster_id,omitempty"`
	Private             bool   `json:"private"`
}

// ProbeResponse represents the response from the RHOBS API
type ProbeResponse struct {
	ID        string `json:"id"`
	ClusterID string `json:"cluster_id"`
	Status    string `json:"status"`
}

// ProbesListResponse represents the response from GET probes endpoint
type ProbesListResponse struct {
	Probes []ProbeResponse `json:"probes"`
}

// Client handles communication with the RHOBS synthetics API
type Client struct {
	baseURL    string
	httpClient *http.Client
	tenant     string
	logger     logr.Logger
}

// NewClient creates a new RHOBS API client
func NewClient(baseURL, tenant string, logger logr.Logger) *Client {
	return &Client{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: defaultHTTPTimeout,
		},
		tenant: tenant,
		logger: logger,
	}
}

// CreateProbe creates a new probe in RHOBS
func (c *Client) CreateProbe(ctx context.Context, req ProbeRequest) (*ProbeResponse, error) {
	url := fmt.Sprintf("%s/%s%s", c.baseURL, c.tenant, probesEndpointPath)

	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal probe request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, httpMethodPost, url, bytes.NewBuffer(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	httpReq.Header.Set("Content-Type", contentTypeJSON)

	c.logger.V(debugLogLevel).Info("Creating RHOBS probe", "url", url, "cluster_id", req.ClusterID)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send HTTP request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("%s %d: %s", apiErrorPrefix, resp.StatusCode, string(body))
	}

	var probeResp ProbeResponse
	if err := json.Unmarshal(body, &probeResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &probeResp, nil
}

// GetProbe retrieves a probe by cluster ID
func (c *Client) GetProbe(ctx context.Context, clusterID string) (*ProbeResponse, error) {
	url := fmt.Sprintf("%s/%s%s", c.baseURL, c.tenant, probesEndpointPath)

	httpReq, err := http.NewRequestWithContext(ctx, httpMethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Add label_selector query parameter for cluster_id
	q := httpReq.URL.Query()
	q.Add(labelSelectorParam, fmt.Sprintf("cluster_id=%s", clusterID))
	httpReq.URL.RawQuery = q.Encode()

	c.logger.V(debugLogLevel).Info("Getting RHOBS probe", "url", httpReq.URL.String(), "cluster_id", clusterID)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send HTTP request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // Probe doesn't exist
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s %d: %s", apiErrorPrefix, resp.StatusCode, string(body))
	}

	var listResp ProbesListResponse
	if err := json.Unmarshal(body, &listResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Find the probe with matching cluster_id
	for _, probe := range listResp.Probes {
		if probe.ClusterID == clusterID {
			return &probe, nil
		}
	}

	return nil, nil // Probe not found
}

// ProbePatchRequest represents the payload for updating a probe status
type ProbePatchRequest struct {
	Status string `json:"status"`
}

// DeleteProbe marks a probe for termination by cluster ID using PATCH method
func (c *Client) DeleteProbe(ctx context.Context, clusterID string) error {
	// First check if probe exists and get its current state
	existingProbe, err := c.GetProbe(ctx, clusterID)
	if err != nil {
		return fmt.Errorf("failed to check existing probe: %w", err)
	}

	if existingProbe == nil {
		// Probe doesn't exist, consider this success
		c.logger.V(debugLogLevel).Info("Probe not found, nothing to delete", "cluster_id", clusterID)
		return nil
	}

	// Handle failed probes by recreating them in terminating state
	if existingProbe.Status == "failed" {
		c.logger.Info("Probe is in failed state, will recreate in terminating state", "cluster_id", clusterID, "probe_id", existingProbe.ID)
		// Note: Actual probe deletion will be handled by agents
	}

	url := fmt.Sprintf("%s/%s"+probeEndpointPath, c.baseURL, c.tenant, clusterID)

	// Create patch request to set status to terminating
	patchReq := ProbePatchRequest{
		Status: "terminating",
	}

	payload, err := json.Marshal(patchReq)
	if err != nil {
		return fmt.Errorf("failed to marshal patch request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, httpMethodPatch, url, bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	httpReq.Header.Set("Content-Type", contentTypeJSON)

	c.logger.V(debugLogLevel).Info("Terminating RHOBS probe", "url", url, "cluster_id", clusterID)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// Probe already doesn't exist, consider this success
		return nil
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s %d: %s", apiErrorPrefix, resp.StatusCode, string(body))
	}

	return nil
}

// IsNon200Error checks if an error represents a non-200 HTTP status
func IsNon200Error(err error) bool {
	return err != nil && strings.Contains(err.Error(), apiErrorPrefix)
}
