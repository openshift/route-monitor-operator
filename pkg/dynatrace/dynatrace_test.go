package dynatrace

import (
	"net/http"
	"net/http/httptest"
	"testing"

	hypershiftv1beta1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
)

func setupMockServer(handlerFunc http.HandlerFunc) string {
	mockServer := httptest.NewServer(handlerFunc)
	return mockServer.URL
}
func createMockHandlerFunc(responseBody string, statusCode int) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
		w.Write([]byte(responseBody)) // nolint:errcheck
	})
}
func TestNewAPIClient(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		apiToken string
	}{
		{
			name:     "Valid API Client Initialization",
			baseURL:  "https://example.com/api",
			apiToken: "mockToken",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiClient := NewDynatraceApiClient(tt.baseURL, tt.apiToken)

			if apiClient.baseURL != tt.baseURL {
				t.Errorf("Expected baseURL to be %s, got %s", tt.baseURL, apiClient.baseURL)
			}

			if apiClient.apiToken != tt.apiToken {
				t.Errorf("Expected apiToken to be %s, got %s", tt.apiToken, apiClient.apiToken)
			}

			if apiClient.httpClient == nil {
				t.Error("Expected httpClient to be initialized, got nil")
			}
		})
	}
}

func TestAPIClient_makeRequest(t *testing.T) {
	// Define test cases in a slice of structs
	tests := []struct {
		name           string
		method         string
		body           string
		expectedStatus int
	}{
		{
			name:           "Make a GET request",
			method:         http.MethodGet,
			body:           "",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Make a POST request",
			method:         http.MethodPost,
			body:           `{"key": "value"}`,
			expectedStatus: http.StatusOK,
		},
	}

	// Iterate through the test cases
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a custom handler function for the mock server
			handlerFunc := createMockHandlerFunc(`{"message": "Mocked response"}`, http.StatusOK)
			// Create the mock server
			mockServerURL := setupMockServer(handlerFunc)
			// Create an instance of the APIClient
			apiClient := NewDynatraceApiClient(mockServerURL, "mockedToken")

			// Make the request
			response, err := apiClient.MakeRequest(tt.method, "/test", tt.body)
			if err != nil {
				t.Errorf("Error making %s request: %v", tt.method, err)
			}
			defer response.Body.Close()

			// Assert the response status code
			if response.StatusCode != tt.expectedStatus {
				t.Errorf("Expected status code %d, got %d", tt.expectedStatus, response.StatusCode)
			}
		})
	}
}

func TestAPIClient_CreateDynatraceHTTPMonitor(t *testing.T) {
	// Mocked response data for testing
	mockMonitorName := "TestMonitor"
	mockApiUrl := "https://example.com"
	mockClusterId := "12345"
	mockDynatraceEquivalentClusterRegionId := "us-east-1"
	mockClusterRegion := "us-east-1"

	// Create a list of test cases
	tests := []struct {
		name           string
		mockResponse   string
		mockStatusCode int
		expectedId     string
		expectError    bool
	}{
		{
			name:           "SuccessfulMonitorCreation",
			mockResponse:   `{"entityId": "56789"}`,
			mockStatusCode: http.StatusOK,
			expectedId:     "56789",
			expectError:    false,
		},
		{
			name:           "ErrorResponseFromServer",
			mockResponse:   "Bad request",
			mockStatusCode: http.StatusBadRequest,
			expectedId:     "",
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock the HTTP server to return the desired response
			mockServer := setupMockServer(createMockHandlerFunc(tt.mockResponse, tt.mockStatusCode))

			// Create an instance of the APIClient using the mock server
			mockClient := NewDynatraceApiClient(mockServer, "mockedToken")

			// Call the method under test
			monitorId, err := mockClient.CreateDynatraceHttpMonitor(mockMonitorName, mockApiUrl, mockClusterId, mockDynatraceEquivalentClusterRegionId, mockClusterRegion)

			// Check for errors or expected values based on the test case
			if (err != nil) != tt.expectError {
				t.Errorf("Unexpected error status. Expected error: %v, got: %v", tt.expectError, err)
			}

			if !tt.expectError && monitorId != tt.expectedId {
				t.Errorf("Incorrect monitor Id. Expected: %s, Got: %s", tt.expectedId, monitorId)
			}
		})
	}
}

func TestAPIClient_ExistsHttpMonitorInDynatrace(t *testing.T) {
	tests := []struct {
		name           string
		monitorId      string
		mockResponse   string
		mockStatusCode int
		expectExists   bool
		expectError    bool
	}{
		{
			name:           "Monitor exists",
			monitorId:      "monitor-1",
			mockResponse:   `{"monitors":[{"entityId":"monitor-1"}]}`,
			mockStatusCode: http.StatusOK,
			expectExists:   true,
			expectError:    false,
		},
		{
			name:           "Monitor does not exist",
			monitorId:      "monitor-2",
			mockResponse:   `{"monitors":[{"entityId":"monitor-1"}]}`,
			mockStatusCode: http.StatusOK,
			expectExists:   false,
			expectError:    false,
		},
		{
			name:           "HTTP error",
			monitorId:      "monitor-1",
			mockResponse:   "",
			mockStatusCode: http.StatusInternalServerError,
			expectExists:   false,
			expectError:    true,
		},
		{
			name:           "JSON parse error",
			monitorId:      "monitor-1",
			mockResponse:   "{invalid json", // Invalid JSON to simulate a parsing error
			mockStatusCode: http.StatusOK,
			expectExists:   false,
			expectError:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create the mock server using the new setup function
			mockServer := setupMockServer(createMockHandlerFunc(tt.mockResponse, tt.mockStatusCode))
			apiClient := NewDynatraceApiClient(mockServer, "mockedToken")

			// Call the function to test
			exists, err := apiClient.ExistsHttpMonitorInDynatrace(tt.monitorId)

			// Verify the results
			// Check for errors based on the expected outcome
			if exists != tt.expectExists {
				t.Errorf("Unexpected exists status. Expected: %v, got: %v", tt.expectExists, exists)
			}
			if (err != nil) != tt.expectError {
				t.Errorf("Unexpected error status. Expected error: %v, got: %v", tt.expectError, err)
			}
		})
	}
}

func TestAPIClient_DeleteDynatraceHTTPMonitor(t *testing.T) {
	// Create a list of test cases
	tests := []struct {
		name           string
		mockStatusCode int
		expectError    bool
	}{
		{
			name:           "Successful DELETE request",
			mockStatusCode: http.StatusNoContent,
			expectError:    false,
		},
		{
			name:           "Failed DELETE request",
			mockStatusCode: http.StatusInternalServerError,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock the HTTP server to return the desired response
			mockServer := setupMockServer(createMockHandlerFunc("", tt.mockStatusCode))
			apiClient := NewDynatraceApiClient(mockServer, "mockedToken")

			// Call the method under test
			err := apiClient.DeleteDynatraceHttpMonitor("123")

			// Check for errors based on the expected outcome
			if (err != nil) != tt.expectError {
				t.Errorf("Unexpected error status. Expected error: %v, got: %v", tt.expectError, err)
			}
		})
	}
}

func TestAPIClient_GetLocationEntityIdFromDynatrace(t *testing.T) {
	tests := []struct {
		name           string
		locationName   string
		locationType   hypershiftv1beta1.AWSEndpointAccessType
		mockResponse   string
		mockStatusCode int
		expectId       string
		expectError    bool
	}{
		{
			name:           "Public location found",
			locationName:   "N. Virginia",
			locationType:   hypershiftv1beta1.PublicAndPrivate,
			mockResponse:   `{"locations":[{"name":"N. Virginia","entityId":"exampleLocationId","type":"PUBLIC","cloudPlatform":"AMAZON_EC2","status":"ENABLED"}]}`,
			mockStatusCode: http.StatusOK,
			expectId:       "exampleLocationId",
			expectError:    false,
		},
		{
			name:           "Private location found",
			locationName:   "backplane",
			locationType:   hypershiftv1beta1.Private,
			mockResponse:   `{"locations":[{"name":"backplanei03xyz","entityId":"privateLocationId","type":"PRIVATE","status":"ENABLED"}]}`,
			mockStatusCode: http.StatusOK,
			expectId:       "privateLocationId",
			expectError:    false,
		},
		{
			name:           "Public location not found",
			locationName:   "Test",
			locationType:   hypershiftv1beta1.PublicAndPrivate,
			mockResponse:   `{"locations":[{"name":"Some Other Location","entityId":"someOtherId","type":"PUBLIC","cloudPlatform":"AMAZON_EC2","status":"ENABLED"}]}`,
			mockStatusCode: http.StatusOK,
			expectId:       "",
			expectError:    true,
		},
		{
			name:           "Private location not found",
			locationName:   "Test",
			locationType:   hypershiftv1beta1.Private,
			mockResponse:   `{"locations":[{"name":"Some Other Location","entityId":"someOtherId","type":"PRIVATE","status":"ENABLED"}]}`,
			mockStatusCode: http.StatusOK,
			expectId:       "",
			expectError:    true,
		},
		{
			name:           "HTTP error from API",
			locationName:   "N. Virginia",
			locationType:   hypershiftv1beta1.PublicAndPrivate,
			mockResponse:   "",
			mockStatusCode: http.StatusInternalServerError,
			expectId:       "",
			expectError:    true,
		},
		{
			name:           "JSON parse error",
			locationName:   "N. Virginia",
			locationType:   hypershiftv1beta1.PublicAndPrivate,
			mockResponse:   "{invalid json",
			mockStatusCode: http.StatusOK,
			expectId:       "",
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create the mock server
			mockServer := setupMockServer(createMockHandlerFunc(tt.mockResponse, tt.mockStatusCode))
			apiClient := NewDynatraceApiClient(mockServer, "mockedToken")

			// Call the function to test
			id, err := apiClient.GetLocationEntityIdFromDynatrace(tt.locationName, tt.locationType)

			// Verify the results
			if id != tt.expectId {
				t.Errorf("Unexpected ID. Expected: %v, got: %v", tt.expectId, id)
			}
			if (err != nil) != tt.expectError {
				t.Errorf("Unexpected error status. Expected error: %v, got: %v", tt.expectError, err)
			}
		})
	}
}
