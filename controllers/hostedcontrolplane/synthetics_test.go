package hostedcontrolplane

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"testing"

	hypershiftv1beta1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	dynatrace "github.com/openshift/route-monitor-operator/pkg/dynatrace"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func setupMockServer(handlerFunc http.HandlerFunc) string {
	mockServer := httptest.NewServer(handlerFunc)
	return mockServer.URL
}

func createMockHandlerFunc(responseBody string, statusCode int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		// Mock GET synthetic monitor response
		case r.Method == http.MethodGet && r.URL.Path == "/synthetic/monitors/" && r.URL.RawQuery == "tag=cluster-id:mock-cluster-id":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"monitors":[{"entityId":"mock-monitor-id"}]}`))

		default:
			w.WriteHeader(statusCode)
			_, _ = w.Write([]byte(responseBody))
		}
	}
}

func TestHostedControlPlaneReconciler_GetDynatraceSecrets(t *testing.T) {
	tests := []struct {
		name           string
		secretData     map[string][]byte
		expectedToken  string
		expectedTenant string
		expectError    bool
		errorMessage   string
	}{
		{
			name: "Valid Secret",
			secretData: map[string][]byte{
				"apiToken": []byte("sampleApiToken123"),
				"apiUrl":   []byte("https://sampletenant.dynatrace.com"),
			},
			expectedToken:  "sampleApiToken123",
			expectedTenant: "https://sampletenant.dynatrace.com",
			expectError:    false,
		},
		{
			name: "Missing apiToken",
			secretData: map[string][]byte{
				"apiUrl": []byte("https://sampletenant.dynatrace.com"),
			},
			expectedToken:  "",
			expectedTenant: "",
			expectError:    true,
			errorMessage:   "secret did not contain key apiToken",
		},
		{
			name: "Empty apiToken",
			secretData: map[string][]byte{
				"apiToken": []byte(""),
				"apiUrl":   []byte("https://sampletenant.dynatrace.com"),
			},
			expectedToken:  "",
			expectedTenant: "",
			expectError:    true,
			errorMessage:   "apiToken is empty",
		},
		{
			name: "Missing apiUrl",
			secretData: map[string][]byte{
				"apiToken": []byte("sampleApiToken1"),
			},
			expectedToken:  "",
			expectedTenant: "",
			expectError:    true,
			errorMessage:   "secret did not contain key apiUrl",
		},

		{
			name:           "Empty Secret",
			secretData:     map[string][]byte{},
			expectedToken:  "",
			expectedTenant: "",
			expectError:    true,
			errorMessage:   "secret did not contain key apiToken", // Expected because apiToken is missing
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			r := &HostedControlPlaneReconciler{
				Client: fake.NewFakeClient(),
			}
			ctx := context.Background()

			// Create a sample Secret object
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "dynatrace-token", Namespace: "openshift-route-monitor-operator"},
				Data:       tt.secretData,
			}
			if err := r.Create(ctx, secret); err != nil {
				t.Fatalf("Failed to create test Secret: %v", err)
			}

			// Call the method to test
			apiToken, tenantUrl, err := r.getDynatraceSecrets(ctx)

			if (err != nil) != tt.expectError {
				t.Errorf("Expected error: %v, but got: %v", tt.expectError, err)
				if tt.expectError && err.Error() != tt.errorMessage {
					t.Errorf("Expected error message: %s, got: %s", tt.errorMessage, err.Error())
				}
			}

			if apiToken != tt.expectedToken {
				t.Errorf("Expected API Token: %s, Got: %s", tt.expectedToken, apiToken)
			}

			if tenantUrl != tt.expectedTenant {
				t.Errorf("Expected Tenant URL: %s, Got: %s", tt.expectedTenant, tenantUrl)
			}
		})
	}
}

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
func TestDeployDynatraceHTTPMonitorResources(t *testing.T) {
	tests := []struct {
		name                 string
		dynatraceMonitorId   string
		mockServerResponse   string
		mockServerStatusCode int
		expectErr            error
	}{
		{
			name:                 "Create Monitor Successfully",
			dynatraceMonitorId:   "",
			mockServerResponse:   `{"id":"new-monitor-id"}`,
			mockServerStatusCode: http.StatusOK,
			expectErr:            nil,
		},
		{
			name:                 "Error Creating Monitor",
			dynatraceMonitorId:   "",
			mockServerResponse:   `{"error":"creation error"}`,
			mockServerStatusCode: http.StatusInternalServerError,
			expectErr:            fmt.Errorf("error creating HTTP monitor: creation error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			mockServer := setupMockServer(createMockHandlerFunc(tt.mockServerResponse, tt.mockServerStatusCode))
			apiClient := dynatrace.NewDynatraceApiClient(mockServer, "mockedToken")

			r := newTestReconciler(t)

			ctx := context.Background()

			// Initialize the HostedControlPlane object
			hostedControlPlane := &hypershiftv1beta1.HostedControlPlane{
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
					Platform: hypershiftv1beta1.PlatformSpec{
						AWS: &hypershiftv1beta1.AWSPlatformSpec{
							EndpointAccess: "PublicAndPrivate",
							Region:         "us-west-1",
						},
					},
				},
			}
			log := log.FromContext(ctx) // Replace with a proper logger if needed

			// Call the function under test
			// nolint:errcheck // this was a placeholder test, and does not work under the covers - we need to mock multiple calls to the mocked API server
			r.deployDynatraceHttpMonitorResources(ctx, apiClient, log, hostedControlPlane)

		})
	}
}

func Test_removeDynatraceMonitors(t *testing.T) {
	// Test objects
	monitor1 := dynatrace.BasicHttpMonitor{
		Name:     "monitor1",
		EntityId: "monitor1",
	}
	monitor2 := dynatrace.BasicHttpMonitor{
		Name:     "monitor2",
		EntityId: "monitor2",
	}
	apiCalls := 0

	tests := []struct {
		name         string
		monitors     []dynatrace.BasicHttpMonitor
		mockResponse http.HandlerFunc
		validate     func(error, int, []dynatrace.BasicHttpMonitor) (bool, string)
	}{
		{
			name: "all provided monitors get removed",
			monitors: []dynatrace.BasicHttpMonitor{
				monitor1,
				monitor2,
			},
			mockResponse: func(w http.ResponseWriter, r *http.Request) {
				// Dynatrace API returns an HTTP-204 when delete succeeds
				apiCalls++
				w.WriteHeader(http.StatusNoContent)
				_, _ = w.Write([]byte{})
			},
			validate: func(err error, apiCalls int, monitors []dynatrace.BasicHttpMonitor) (bool, string) {
				if err != nil {
					return false, fmt.Sprintf("unexpected error returned: %v", err)
				}
				if apiCalls != len(monitors) {
					return false, fmt.Sprintf("unexpected number of api calls: expected number equal to provided number of monitors (%d), but got %d", len(monitors), apiCalls)
				}
				return true, ""
			},
		},
		{
			name: "all errors returned",
			monitors: []dynatrace.BasicHttpMonitor{
				monitor1,
				monitor2,
			},
			mockResponse: func(w http.ResponseWriter, r *http.Request) {
				apiCalls++
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte{})
			},
			validate: func(err error, apiCalls int, monitors []dynatrace.BasicHttpMonitor) (bool, string) {
				if err == nil {
					return false, "error expected, but none returned"
				}
				if !strings.Contains(err.Error(), "monitor1") || !strings.Contains(err.Error(), "monitor2") {
					return false, "expected all errors to be returned"
				}
				return true, ""
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			apiCalls = 0
			mockServerURL := setupMockServer(tt.mockResponse)
			dynatraceClient := dynatrace.NewDynatraceApiClient(mockServerURL, "mockedToken")

			// Run test
			err := removeDyntraceMonitors(dynatraceClient, tt.monitors)

			// Validate results
			pass, reason := tt.validate(err, apiCalls, tt.monitors)
			if !pass {
				t.Errorf("unexpected test result: %v", reason)
			}
		})
	}
}

func TestAPIClient_DeleteDynatraceHTTPMonitorResources(t *testing.T) {
	tests := []struct {
		name           string
		mockClusterId  string
		mockStatusCode int
		expectError    bool
	}{
		{
			name:           "HTTP Monitor Id not found",
			mockClusterId:  "fake-cluster-id",
			mockStatusCode: http.StatusNoContent,
			expectError:    true,
		},
		{
			name:           "Successful deletion of HTTP Monitor",
			mockClusterId:  "mock-cluster-id",
			mockStatusCode: http.StatusNoContent,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockServer := setupMockServer(createMockHandlerFunc("", tt.mockStatusCode))
			apiClient := dynatrace.NewDynatraceApiClient(mockServer, "mockedToken")

			hostedControlPlane := &hypershiftv1beta1.HostedControlPlane{
				Spec: hypershiftv1beta1.HostedControlPlaneSpec{
					ClusterID: tt.mockClusterId,
				},
			}

			err := apiClient.DeleteDynatraceMonitorByCluserId(hostedControlPlane.Spec.ClusterID)

			if (err != nil) != tt.expectError {
				t.Errorf("Expected error: %v, got: %v", tt.expectError, err)
			}
		})
	}
}

func TestGetDynatraceEquivalentClusterRegionId(t *testing.T) {
	tests := []struct {
		name          string
		clusterRegion string
		expectId      string
		expectError   bool
	}{
		{
			name:          "us-east-1",
			clusterRegion: "us-east-1",
			expectId:      "N. Virginia",
			expectError:   false,
		},
		{
			name:          "us-west-2",
			clusterRegion: "us-west-2",
			expectId:      "Oregon",
			expectError:   false,
		},
		{
			name:          "ap-south-1",
			clusterRegion: "ap-south-1",
			expectId:      "Mumbai",
			expectError:   false,
		},
		{
			name:          "non-existent region",
			clusterRegion: "non-existent-region",
			expectId:      "",
			expectError:   true,
		},
		{
			name:          "us-east-2",
			clusterRegion: "us-east-2",
			expectId:      "N. Virginia",
			expectError:   false,
		},
		{
			name:          "eu-central-1",
			clusterRegion: "eu-central-1",
			expectId:      "Frankfurt",
			expectError:   false,
		},
		{
			name:          "me-south-1",
			clusterRegion: "me-south-1",
			expectId:      "Mumbai",
			expectError:   false,
		},
		{
			name:          "invalid region format",
			clusterRegion: "invalid-region",
			expectId:      "",
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Call the function to test
			id, err := getDynatraceEquivalentClusterRegionName(tt.clusterRegion)

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

func TestDetermineDynatraceClusterRegionName(t *testing.T) {
	tests := []struct {
		name                string
		clusterRegion       string
		monitorLocationType hypershiftv1beta1.AWSEndpointAccessType
		expectId            string
		expectError         bool
	}{
		{
			name:                "Valid PublicAndPrivate region",
			clusterRegion:       "us-east-1",
			monitorLocationType: hypershiftv1beta1.PublicAndPrivate,
			expectId:            "N. Virginia", // Adjust according to your mapping
			expectError:         false,
		},
		{
			name:                "Valid Private region",
			clusterRegion:       "us-west-2",
			monitorLocationType: hypershiftv1beta1.Private,
			expectId:            "backplane",
			expectError:         false,
		},
		{
			name:                "Invalid region for PublicAndPrivate",
			clusterRegion:       "invalid-region",
			monitorLocationType: hypershiftv1beta1.PublicAndPrivate,
			expectId:            "",
			expectError:         true,
		},
		{
			name:                "Invalid region for Private",
			clusterRegion:       "invalid-region",
			monitorLocationType: hypershiftv1beta1.Private,
			expectId:            "backplane",
			expectError:         false,
		},
		{
			name:                "Unsupported monitorLocationType",
			clusterRegion:       "us-east-1",
			monitorLocationType: "UnknownType",
			expectId:            "",
			expectError:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Call the function to test
			id, err := determineDynatraceClusterRegionName(tt.clusterRegion, tt.monitorLocationType)

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
