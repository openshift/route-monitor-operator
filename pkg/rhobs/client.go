package rhobs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
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
	probesEndpointPath = "/api/metrics/v1/%s/probes"
	probeEndpointPath  = "/api/metrics/v1/%s/probes/%s"

	// HTTP headers
	contentTypeJSON = "application/json"
	tenantHeader    = "X-Tenant"
	usernameHeader  = "X-Username"

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

// OIDCConfig holds OIDC authentication configuration
type OIDCConfig struct {
	ClientID     string
	ClientSecret string
	IssuerURL    string
}

// tokenResponse represents an OIDC token response
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// Client handles communication with the RHOBS synthetics API
type Client struct {
	baseURL    string
	httpClient *http.Client
	tenant     string
	oidcConfig *OIDCConfig
	logger     logr.Logger

	// Token management
	tokenMutex  sync.RWMutex
	accessToken string
	tokenExpiry time.Time
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

// NewClientWithOIDC creates a new RHOBS API client with OIDC authentication
func NewClientWithOIDC(baseURL, tenant string, oidcConfig OIDCConfig, logger logr.Logger) *Client {
	return &Client{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: defaultHTTPTimeout,
		},
		tenant:     tenant,
		oidcConfig: &oidcConfig,
		logger:     logger,
	}
}

// CreateProbe creates a new probe in RHOBS
func (c *Client) CreateProbe(ctx context.Context, req ProbeRequest) (*ProbeResponse, error) {
	url := c.buildProbesURL()

	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal probe request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, httpMethodPost, url, bytes.NewBuffer(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	httpReq.Header.Set("Content-Type", contentTypeJSON)

	// Add RHOBS-specific headers (tenant and username)
	c.addRHOBSHeaders(httpReq)

	// Add authentication headers if OIDC is configured
	if err := c.addAuthHeaders(ctx, httpReq); err != nil {
		return nil, fmt.Errorf("failed to add auth headers: %w", err)
	}

	username := ""
	if c.oidcConfig != nil {
		username = c.oidcConfig.ClientID
	}
	c.logger.V(debugLogLevel).Info("Creating RHOBS probe", "method", "POST", "url", url, "cluster_id", req.ClusterID, "tenant", c.tenant, "username", username)
	c.logger.Info("Sending RHOBS API request", "method", "POST", "url", url, "operation", "create-probe")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send HTTP request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	c.logger.Info("Received RHOBS API response", "method", "POST", "url", url, "status_code", resp.StatusCode, "operation", "create-probe")

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
	url := c.buildProbesURL()

	httpReq, err := http.NewRequestWithContext(ctx, httpMethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Add label_selector query parameter for cluster_id
	q := httpReq.URL.Query()
	q.Add(labelSelectorParam, fmt.Sprintf("cluster_id=%s", clusterID))
	httpReq.URL.RawQuery = q.Encode()

	// Add RHOBS-specific headers (tenant and username)
	c.addRHOBSHeaders(httpReq)

	// Add authentication headers if OIDC is configured
	if err := c.addAuthHeaders(ctx, httpReq); err != nil {
		return nil, fmt.Errorf("failed to add auth headers: %w", err)
	}

	username := ""
	if c.oidcConfig != nil {
		username = c.oidcConfig.ClientID
	}
	c.logger.V(debugLogLevel).Info("Getting RHOBS probe", "method", "GET", "url", httpReq.URL.String(), "cluster_id", clusterID, "tenant", c.tenant, "username", username)
	c.logger.Info("Sending RHOBS API request", "method", "GET", "url", httpReq.URL.String(), "operation", "get-probe")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send HTTP request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	c.logger.Info("Received RHOBS API response", "method", "GET", "url", httpReq.URL.String(), "status_code", resp.StatusCode, "operation", "get-probe")

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

	url := c.buildProbeURL(clusterID)

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

	// Add RHOBS-specific headers (tenant and username)
	c.addRHOBSHeaders(httpReq)

	// Add authentication headers if OIDC is configured
	if err := c.addAuthHeaders(ctx, httpReq); err != nil {
		return fmt.Errorf("failed to add auth headers: %w", err)
	}

	username := ""
	if c.oidcConfig != nil {
		username = c.oidcConfig.ClientID
	}
	c.logger.V(debugLogLevel).Info("Terminating RHOBS probe", "method", "PATCH", "url", url, "cluster_id", clusterID, "tenant", c.tenant, "username", username)
	c.logger.Info("Sending RHOBS API request", "method", "PATCH", "url", url, "operation", "delete-probe")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send HTTP request: %w", err)
	}
	defer resp.Body.Close()

	c.logger.Info("Received RHOBS API response", "method", "PATCH", "url", url, "status_code", resp.StatusCode, "operation", "delete-probe")

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

// GetAccessToken retrieves a valid access token, refreshing if necessary
func (c *Client) GetAccessToken(ctx context.Context) (string, error) {
	if c.oidcConfig == nil {
		return "", nil // No OIDC config, no token needed
	}

	c.tokenMutex.RLock()
	if c.accessToken != "" && time.Now().Before(c.tokenExpiry.Add(-30*time.Second)) {
		token := c.accessToken
		c.tokenMutex.RUnlock()
		return token, nil
	}
	c.tokenMutex.RUnlock()

	// Need to refresh token
	return c.refreshAccessToken(ctx)
}

// refreshAccessToken obtains a new access token using client credentials flow
func (c *Client) refreshAccessToken(ctx context.Context) (string, error) {
	c.tokenMutex.Lock()
	defer c.tokenMutex.Unlock()

	// Double-check that we still need to refresh
	if c.accessToken != "" && time.Now().Before(c.tokenExpiry.Add(-30*time.Second)) {
		return c.accessToken, nil
	}

	// Handle both direct token endpoint URLs and issuer URLs that need /token appended
	tokenURL := c.oidcConfig.IssuerURL
	if !strings.HasSuffix(tokenURL, "/token") {
		tokenURL = strings.TrimSuffix(tokenURL, "/") + "/token"
	}

	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", c.oidcConfig.ClientID)
	data.Set("client_secret", c.oidcConfig.ClientSecret)
	data.Set("scope", "profile")

	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	c.logger.V(debugLogLevel).Info("Requesting OIDC access token", "issuer_url", c.oidcConfig.IssuerURL, "token_url", tokenURL)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to request access token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp tokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("failed to parse token response: %w", err)
	}

	c.accessToken = tokenResp.AccessToken
	c.tokenExpiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	c.logger.V(debugLogLevel).Info("Successfully obtained OIDC access token",
		"expires_in", tokenResp.ExpiresIn)

	return c.accessToken, nil
}

// addAuthHeaders adds authentication headers to the request if OIDC is configured
func (c *Client) addAuthHeaders(ctx context.Context, req *http.Request) error {
	if c.oidcConfig == nil {
		return nil // No OIDC config, no auth needed
	}

	token, err := c.GetAccessToken(ctx)
	if err != nil {
		return fmt.Errorf("failed to get access token: %w", err)
	}

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
		c.logger.V(debugLogLevel).Info("Using Bearer token authentication", "client_id", c.oidcConfig.ClientID)
	}

	return nil
}

// addRHOBSHeaders adds RHOBS-specific headers to the request
func (c *Client) addRHOBSHeaders(req *http.Request) {
	// Set tenant header
	req.Header.Set(tenantHeader, c.tenant)

	// Set username header if OIDC is configured, use client ID as username
	if c.oidcConfig != nil {
		req.Header.Set(usernameHeader, c.oidcConfig.ClientID)
	}
}

// buildProbesURL constructs the URL for the probes endpoint
func (c *Client) buildProbesURL() string {
	// Check if baseURL already contains the probes path
	if strings.Contains(c.baseURL, "/probes") {
		return c.baseURL
	}
	// Otherwise, build the URL with tenant path
	return fmt.Sprintf("%s"+probesEndpointPath, c.baseURL, c.tenant)
}

// buildProbeURL constructs the URL for a specific probe endpoint
func (c *Client) buildProbeURL(clusterID string) string {
	// Check if baseURL already contains the probes path
	if strings.Contains(c.baseURL, "/probes") {
		// If baseURL ends with /probes, append the cluster ID
		if strings.HasSuffix(c.baseURL, "/probes") {
			return fmt.Sprintf("%s/%s", c.baseURL, clusterID)
		}
		// If baseURL contains /probes but doesn't end with it, use as-is and append cluster ID
		return fmt.Sprintf("%s/%s", c.baseURL, clusterID)
	}
	// Otherwise, build the URL with tenant path and cluster ID
	return fmt.Sprintf("%s"+probeEndpointPath, c.baseURL, c.tenant, clusterID)
}
