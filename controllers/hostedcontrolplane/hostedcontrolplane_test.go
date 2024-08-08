package hostedcontrolplane

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/go-logr/logr"
	routev1 "github.com/openshift/api/route/v1"
	hypershiftv1beta1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func Test_buildMetadataForUpdate(t *testing.T) {
	// Test-specific definitions
	var (
		hcp = hypershiftv1beta1.HostedControlPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
			},
		}

		dgps = int64(0)
		meta = metav1.ObjectMeta{
			Name:                       "actual",
			Namespace:                  "actualNS",
			ResourceVersion:            "0123456789",
			Generation:                 int64(1234),
			CreationTimestamp:          metav1.Now(),
			DeletionTimestamp:          &metav1.Time{Time: time.Now()},
			DeletionGracePeriodSeconds: &dgps,
		}
	)

	type args struct {
		expected metav1.ObjectMeta
		actual   metav1.ObjectMeta
	}

	// Testing
	tests := []struct {
		name string
		args args
		want metav1.ObjectMeta
	}{
		// Cases
		{
			name: "on-cluster object's labels differ from expected",
			args: args{
				actual: metav1.ObjectMeta{
					Name:   "actual",
					Labels: map[string]string{"incorrect-label": "true"},
				},
				expected: metav1.ObjectMeta{
					Name:   "expected",
					Labels: map[string]string{"correct-label": "true"},
				},
			},
			want: metav1.ObjectMeta{
				Name:   "actual",
				Labels: map[string]string{"correct-label": "true"},
			},
		},
		{
			name: "on-cluster object's OwnerReferences differ from expected",
			args: args{
				actual: metav1.ObjectMeta{
					Name:            "actual",
					OwnerReferences: []metav1.OwnerReference{},
				},
				expected: metav1.ObjectMeta{
					Name:            "expected",
					OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(&hcp, hcp.GroupVersionKind())},
				},
			},
			want: metav1.ObjectMeta{
				Name:            "actual",
				OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(&hcp, hcp.GroupVersionKind())},
			},
		},
		{
			name: "other fields in on-cluster object's metadata remains unchanged",
			args: args{
				actual: meta,
				expected: metav1.ObjectMeta{
					Name:            "expected",
					Namespace:       "otherNS",
					ResourceVersion: "9876543210",
					Generation:      int64(9876),
				},
			},
			want: meta,
		},
	}

	// Execution
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildMetadataForUpdate(tt.args.expected, tt.args.actual); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("buildMetadataForUpdate() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestHostedControlPlaneReconciler_buildInternalMonitoringRoute(t *testing.T) {
	// Test-specific definitions
	var (
		hcp = hypershiftv1beta1.HostedControlPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "test",
			},
		}
	)

	type args struct {
		hostedcontrolplane *hypershiftv1beta1.HostedControlPlane
	}

	// Testing
	tests := []struct {
		name string
		args args
		eval func(route routev1.Route) (passed bool, reason string)
	}{
		// Cases
		{
			name: "route is created with watch label",
			args: args{
				hostedcontrolplane: &hcp,
			},
			eval: func(route routev1.Route) (bool, string) {
				value, found := route.Labels[watchResourceLabel]
				if !found {
					return false, "watchResourceLabel key not found"
				}
				if value != "true" {
					return false, "watchResourceLabel value is not correct"
				}
				return true, ""
			},
		},
		{
			name: "route's .spec.host field is formulated properly",
			args: args{
				hostedcontrolplane: &hcp,
			},
			eval: func(route routev1.Route) (bool, string) {
				expectedHost := "kube-apiserver.test.svc.cluster.local"
				if route.Spec.Host != expectedHost {
					return false, fmt.Sprintf("host field was set incorrectly: expected '%s', got '%s'", expectedHost, route.Spec.Host)
				}
				return true, ""
			},
		},
		{
			name: "route's OwnerReference is set to the provided hostedcontrolplane",
			args: args{
				hostedcontrolplane: &hcp,
			},
			eval: func(route routev1.Route) (bool, string) {
				if len(route.OwnerReferences) != 1 {
					return false, fmt.Sprintf("incorrect number of ownerrefs: expected 1, got %d", len(route.OwnerReferences))
				}
				if reflect.DeepEqual(route.OwnerReferences[0], buildOwnerReferences(&hcp)) {
					return false, "ownerref for route is incorrect"
				}
				return true, ""
			},
		},
	}

	// Execution
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestReconciler(t)
			route := r.buildInternalMonitoringRoute(tt.args.hostedcontrolplane)
			passed, reason := tt.eval(route)
			if !passed {
				t.Errorf("HostedControlPlaneReconciler.buildInternalMonitoringRoute() resulting route = %#v, failed due to %s", route, reason)
			}
		})
	}
}

func TestHostedControlPlaneReconciler_buildInternalMonitoringRouteMonitor(t *testing.T) {
	// Test-specific definitions
	var (
		route = routev1.Route{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "test",
			},
		}

		hcp = hypershiftv1beta1.HostedControlPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "test",
			},
		}
	)
	type args struct {
		route              routev1.Route
		hostedcontrolplane *hypershiftv1beta1.HostedControlPlane
		apiServerPort      int64
	}

	// Testing
	tests := []struct {
		name string
		args args
		eval func(routemonitor v1alpha1.RouteMonitor) (passed bool, reason string)
	}{
		// Cases
		{
			name: "routemonitor is created with watch label",
			args: args{
				route:              route,
				hostedcontrolplane: &hcp,
				apiServerPort:      6443,
			},
			eval: func(routemonitor v1alpha1.RouteMonitor) (passed bool, reason string) {
				value, found := routemonitor.Labels[watchResourceLabel]
				if !found {
					return false, "watchResourceLabel key not found"
				}
				if value != "true" {
					return false, "watchResourceLabel value is not correct"
				}
				return true, ""
			},
		},
		{
			name: "provided apiserver port is reflected in routemonitor's .spec",
			args: args{
				route:              route,
				hostedcontrolplane: &hcp,
				apiServerPort:      9876,
			},
			eval: func(routemonitor v1alpha1.RouteMonitor) (passed bool, reason string) {
				if routemonitor.Spec.Route.Port != 9876 {
					return false, ".spec.route.port does not match the provided value '443'"
				}
				return true, ""
			},
		},
		{
			name: "routemonitor's ownerrefs are set correctly",
			args: args{
				route:              route,
				hostedcontrolplane: &hcp,
				apiServerPort:      6443,
			},
			eval: func(routemonitor v1alpha1.RouteMonitor) (passed bool, reason string) {
				if len(routemonitor.OwnerReferences) != 1 {
					return false, fmt.Sprintf("incorrect number of ownerrefs: expected 1, got %d", len(route.OwnerReferences))
				}
				if reflect.DeepEqual(routemonitor.OwnerReferences[0], buildOwnerReferences(&hcp)) {
					return false, "ownerref for routemonitor is incorrect"
				}
				return true, ""
			},
		},
	}

	// Execution
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestReconciler(t)
			routemonitor := r.buildInternalMonitoringRouteMonitor(tt.args.route, tt.args.hostedcontrolplane, tt.args.apiServerPort)
			passed, reason := tt.eval(routemonitor)
			if !passed {
				t.Errorf("HostedControlPlaneReconciler.buildInternalMonitoringRouteMonitor() resulting routemonitor = %#v, failed due to = %s", routemonitor, reason)
			}
		})
	}
}

func TestHostedControlPlaneReconciler_buildOwnerReferences(t *testing.T) {
	// Test-specific definitions
	var (
		hcp = hypershiftv1beta1.HostedControlPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "test",
			},
		}
	)
	type args struct {
		hostedcontrolplane *hypershiftv1beta1.HostedControlPlane
	}

	// Testing
	tests := []struct {
		name string
		args args
		eval func([]metav1.OwnerReference) (passed bool, reason string)
	}{
		// Cases
		{
			name: "exactly one ownerref is returned",
			args: args{
				hostedcontrolplane: &hcp,
			},
			eval: func(ownerrefs []metav1.OwnerReference) (passed bool, reason string) {
				if len(ownerrefs) != 1 {
					return false, fmt.Sprintf("incorrect number of ownerrefs returned: expected 1, got %d", len(ownerrefs))
				}
				return true, ""
			},
		},
		{
			name: "ownerref lists hcp as controller",
			args: args{
				hostedcontrolplane: &hcp,
			},
			eval: func(ownerrefs []metav1.OwnerReference) (passed bool, reason string) {
				ownerref := ownerrefs[0]
				if !*ownerref.Controller {
					return false, "ownerref doesn't set hcp as controller"
				}
				return true, ""
			},
		},
	}

	// Execution
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ownerrefs := buildOwnerReferences(tt.args.hostedcontrolplane)
			passed, reason := tt.eval(ownerrefs)
			if !passed {
				t.Errorf("HostedControlPlaneReconciler.buildOwnerReferences() = %#v, failed due to = %s", ownerrefs, reason)
			}
		})
	}
}

// Left as a placeholder for future testing.
// Currently, this function simply calls deleteInternalMonitoringObjects and wraps any error returned,
// so testing it doesn't actually provide any value at this point.
func TestHostedControlPlaneReconciler_finalizeHostedControlPlane(t *testing.T) {
	type args struct {
		ctx                context.Context
		log                logr.Logger
		hostedcontrolplane *hypershiftv1beta1.HostedControlPlane
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		// TODO: Add test cases if/when the function requires it
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestReconciler(t)
			if err := r.finalizeHostedControlPlane(tt.args.ctx, tt.args.log, tt.args.hostedcontrolplane); (err != nil) != tt.wantErr {
				t.Errorf("HostedControlPlaneReconciler.finalizeHostedControlPlane() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestHostedControlPlaneReconciler_deployInternalMonitoringObjects(t *testing.T) {
	// Test-specific definitions
	var (
		ctx = context.TODO()
		log = log.FromContext(ctx)

		hcp = hypershiftv1beta1.HostedControlPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "test",
			},
		}

		svc = corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kube-apiserver",
				Namespace: "test",
			},
		}

		route = routev1.Route{
			ObjectMeta: metav1.ObjectMeta{
				// Route name should align with the expected value for an HCP based on
				// what's configured in the buildInternalMonitoringRoute() function
				Name:      "test-kube-apiserver-monitoring",
				Namespace: "test",
			},
		}

		routemonitor = v1alpha1.RouteMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      route.Name,
				Namespace: route.Namespace,
			},
		}
	)

	type args struct {
		ctx                context.Context
		log                logr.Logger
		hostedcontrolplane *hypershiftv1beta1.HostedControlPlane
	}

	// Testing
	tests := []struct {
		name string
		args args
		objs []client.Object
		eval func(err error, r *HostedControlPlaneReconciler) (passed bool, reason string)
	}{
		// Cases
		// NOTE: we aren't testing the actual configuration of the route & routemonitor objects
		// in these tests, since those object definitions are generated by other functions.
		// Instead, we're focused on testing the *deployment* logic (ie - when something is created vs updated, etc)
		{
			name: "Error is returned when kube-apiserver service does not exist",
			args: args{
				ctx:                ctx,
				log:                log,
				hostedcontrolplane: &hcp,
			},
			objs: []client.Object{},
			eval: func(err error, _ *HostedControlPlaneReconciler) (passed bool, reason string) {
				if err == nil {
					return false, "expected error due to missing kube-apiserver service, got none"
				}
				return true, ""
			},
		},
		{
			name: "No error is returned and both route & routemonitor are created when neither exists",
			args: args{
				ctx:                ctx,
				log:                log,
				hostedcontrolplane: &hcp,
			},
			objs: []client.Object{
				&svc,
			},
			eval: func(err error, r *HostedControlPlaneReconciler) (passed bool, reason string) {
				// Evaluate returned error
				if err != nil {
					return false, fmt.Sprintf("unexpected error returned: %v", err)
				}

				// Check that route & routemonitor objects were created, as expected
				routes := routev1.RouteList{}
				err = r.List(context.TODO(), &routes)
				if err != nil {
					return false, fmt.Sprintf("failed to retrieve routes from test client: %v", err)
				}
				if len(routes.Items) != 1 {
					return false, fmt.Sprintf("unexpected number of routes found: expected 1, got %d. Routes: %#v", len(routes.Items), routes)
				}

				routemonitors := v1alpha1.RouteMonitorList{}
				err = r.List(context.TODO(), &routemonitors)
				if err != nil {
					return false, fmt.Sprintf("failed to retrieve routemonitors from test client: %v", err)
				}
				if len(routemonitors.Items) != 1 {
					return false, fmt.Sprintf("unexpected number of routemonitors found: expected 1, got %d. Routemonitors: %#v", len(routemonitors.Items), routemonitors)
				}

				return true, ""
			},
		},
		{
			name: "The route is updated when it already exists",
			args: args{
				ctx:                ctx,
				log:                log,
				hostedcontrolplane: &hcp,
			},
			objs: []client.Object{
				&svc,
				// Define a route w/ a bad label
				// In general, we shouldn't test object configuration here, since that's defined elsewhere as
				// noted above, but it's useful in this case to know that an unwanted label is expected to be
				// removed, that way we know if the route is actually being updated by the function as expected.
				&routev1.Route{
					ObjectMeta: metav1.ObjectMeta{
						Name:      route.Name,
						Namespace: route.Namespace,
						Labels:    map[string]string{"labelWeExpectToBeRemoved": "true"},
					},
				},
			},
			eval: func(err error, r *HostedControlPlaneReconciler) (passed bool, reason string) {
				// Evaluate returned error to ensure function did not unexpectedly fail
				if err != nil {
					return false, fmt.Sprintf("unexpected error returned: %v", err)
				}

				// Check that route was updated as expected
				result := routev1.Route{}
				err = r.Get(context.TODO(), types.NamespacedName{Name: route.Name, Namespace: route.Namespace}, &result)
				if err != nil {
					return false, fmt.Sprintf("failed to retrieve route from test client: %v", err)
				}
				_, found := result.Labels["labelWeExpectToBeRemoved"]
				if found {
					return false, fmt.Sprintf("route was not updated; expected labels to not contain key 'labelWeExpectToBeRemoved', route has the following labels: %#v", route.Labels)
				}
				return true, ""
			},
		},
		{
			name: "The routemonitor is updated when it already exists",
			args: args{
				ctx:                ctx,
				log:                log,
				hostedcontrolplane: &hcp,
			},
			objs: []client.Object{
				&svc,
				// Define a route w/ a bad label
				// In general, we shouldn't test object configuration here, since that's defined elsewhere as
				// noted above, but it's useful in this case to know that an unwanted label is expected to be
				// removed, that way we know if the route is actually being updated by the function as expected.
				&v1alpha1.RouteMonitor{
					ObjectMeta: metav1.ObjectMeta{
						Name:      routemonitor.Name,
						Namespace: routemonitor.Namespace,
						Labels:    map[string]string{"labelWeExpectToBeRemoved": "true"},
					},
				},
			},
			eval: func(err error, r *HostedControlPlaneReconciler) (passed bool, reason string) {
				// Evaluate returned error to ensure function did not unexpectedly fail
				if err != nil {
					return false, fmt.Sprintf("unexpected error returned: %v", err)
				}

				// Check that route was updated as expected
				result := v1alpha1.RouteMonitor{}
				err = r.Get(context.TODO(), types.NamespacedName{Name: routemonitor.Name, Namespace: routemonitor.Namespace}, &result)
				if err != nil {
					return false, fmt.Sprintf("failed to retrieve route from test client: %v", err)
				}
				_, found := result.Labels["labelWeExpectToBeRemoved"]
				if found {
					return false, fmt.Sprintf("route was not updated; expected labels to not contain key 'labelWeExpectToBeRemoved', route has the following labels: %#v", route.Labels)
				}
				return true, ""
			},
		},
	}

	// Execution
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestReconciler(t, tt.objs...)
			err := r.deployInternalMonitoringObjects(tt.args.ctx, tt.args.log, tt.args.hostedcontrolplane)
			passed, reason := tt.eval(err, r)
			if !passed {
				t.Errorf("HostedControlPlaneReconciler.deployInternalMonitoringObjects() did not pass due to = %s", reason)
			}

		})
	}
}

func TestHostedControlPlaneReconciler_deleteInternalMonitoringObjects(t *testing.T) {
	// Test-specific definitions
	var (
		ctx = context.TODO()
		log = log.FromContext(ctx)
		hcp = hypershiftv1beta1.HostedControlPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "test",
			},
		}

		route = routev1.Route{
			ObjectMeta: metav1.ObjectMeta{
				// Name must match the expected route created by the buildInternalMonitoringRoute() function
				Name:      "test-kube-apiserver-monitoring",
				Namespace: "test",
			},
		}
		routemonitor = v1alpha1.RouteMonitor{
			ObjectMeta: metav1.ObjectMeta{
				// Name must match the expected routemonitor created by the buildInternalMonitoringRouteMonitor() function
				Name:      "test-kube-apiserver-monitoring",
				Namespace: "test",
			},
		}
	)

	// Testing
	tests := []struct {
		// name defines the name of each subtest
		name string
		// objs defines the object present on-cluster when testing
		objs []client.Object
		// eval defines the logic used to determine if the test passed or failed, along with the reason for the failure, if applicable
		eval func(err error, r *HostedControlPlaneReconciler) (passed bool, reason string)
	}{
		// Cases
		// Test both route&routemonitor
		{
			name: "no error is returned when both route and routemonitor are present",
			objs: []client.Object{
				&route,
				&routemonitor,
			},
			eval: func(err error, r *HostedControlPlaneReconciler) (passed bool, reason string) {
				if err != nil {
					return false, fmt.Sprintf("unexpected error returned: %v", err)
				}
				return true, ""
			},
		},
		{
			name: "route and routemonitor are both deleted when both are present on cluster",
			objs: []client.Object{
				&route,
				&routemonitor,
			},
			eval: func(_ error, r *HostedControlPlaneReconciler) (passed bool, reason string) {
				err := r.Client.Get(ctx, types.NamespacedName{Name: route.Name, Namespace: route.Namespace}, &routev1.Route{})
				if !errors.IsNotFound(err) {
					return false, fmt.Sprintf("expected route to be deleted, instead got err: %v", err)
				}

				err = r.Client.Get(ctx, types.NamespacedName{Name: routemonitor.Name, Namespace: routemonitor.Namespace}, &v1alpha1.RouteMonitor{})
				if !errors.IsNotFound(err) {
					return false, fmt.Sprintf("expected routemonitor to be deleted, instead got err: %v", err)
				}
				return true, ""
			},
		},
		// Test when route is already gone
		{
			name: "no error is returned when route does not exist",
			objs: []client.Object{
				&routemonitor,
			},
			eval: func(err error, r *HostedControlPlaneReconciler) (passed bool, reason string) {
				if err != nil {
					return false, fmt.Sprintf("unexpected error returned: %v", err)
				}
				return true, ""
			},
		},
		{
			name: "routemonitor is still deleted when route is not present",
			objs: []client.Object{
				&routemonitor,
			},
			eval: func(_ error, r *HostedControlPlaneReconciler) (passed bool, reason string) {
				err := r.Client.Get(ctx, types.NamespacedName{Name: routemonitor.Name, Namespace: routemonitor.Namespace}, &v1alpha1.RouteMonitor{})
				if !errors.IsNotFound(err) {
					return false, fmt.Sprintf("expected routemonitor to have been deleted, instead got err: %v", err)
				}
				return true, ""
			},
		},
		// Test when routemonitor is already gone
		{
			name: "no error is returned when routemonitor does not exist",
			objs: []client.Object{
				&route,
			},
			eval: func(err error, r *HostedControlPlaneReconciler) (passed bool, reason string) {
				if err != nil {
					return false, fmt.Sprintf("unexpected error returned: %v", err)
				}
				return true, ""
			},
		},
		{
			name: "route is still deleted when routemonitor is not present",
			objs: []client.Object{
				&route,
			},
			eval: func(_ error, r *HostedControlPlaneReconciler) (passed bool, reason string) {
				err := r.Client.Get(ctx, types.NamespacedName{Name: route.Name, Namespace: route.Namespace}, &routev1.Route{})
				if !errors.IsNotFound(err) {
					return false, fmt.Sprintf("expected route to have been deleted, instead got err: %v", err)
				}
				return true, ""
			},
		},
	}

	// Execution
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestReconciler(t, tt.objs...)
			err := r.deleteInternalMonitoringObjects(ctx, log, &hcp)
			passed, reason := tt.eval(err, r)
			if !passed {
				t.Errorf("HostedControlPlaneReconciler.deleteInternalMonitoringObjects() error = %v, did not pass due to = %s", err, reason)
			}
		})
	}
}

// newTestReconciler creates a test client containing the following objects
func newTestReconciler(t *testing.T, objs ...client.Object) *HostedControlPlaneReconciler {
	var err error
	s := scheme.Scheme

	err = hypershiftv1beta1.AddToScheme(s)
	if err != nil {
		t.Errorf("failed to add hypershiftv1beta1 to scheme: %v", err)
	}

	err = v1alpha1.AddToScheme(s)
	if err != nil {
		t.Errorf("failed to add v1alpha1 to scheme: %v", err)
	}

	err = routev1.AddToScheme(s)
	if err != nil {
		t.Errorf("failed to add routev1 to scheme: %v", err)
	}

	client := fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build()

	r := &HostedControlPlaneReconciler{
		Client: client,
		Scheme: s,
	}
	return r
}

func setupMockServer(handlerFunc http.HandlerFunc) string {
	mockServer := httptest.NewServer(handlerFunc)
	return mockServer.URL
}
func createMockHandlerFunc(responseBody string, statusCode int) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
		w.Write([]byte(responseBody))
	})
}
func TestNewAPIClient(t *testing.T) {
	baseURL := "https://example.com/api"
	apiToken := "mockToken"
	apiClient := NewDynatraceAPIClient(baseURL, apiToken)

	if apiClient.baseURL != baseURL {
		t.Errorf("Expected baseURL to be %s, got %s", baseURL, apiClient.baseURL)
	}

	if apiClient.apiToken != apiToken {
		t.Errorf("Expected apiToken to be %s, got %s", apiToken, apiClient.apiToken)
	}

	if apiClient.httpClient == nil {
		t.Error("Expected httpClient to be initialized, got nil")
	}
}

func TestAPIClient_makeRequest(t *testing.T) {
	// Test case: Make a GET request
	// Create a custom handler function for the mock server
	handlerFunc := createMockHandlerFunc(`{"message": "Mocked response"}`, http.StatusOK)
	// Create the mock server using the setupMockServer function
	mockServerURL := setupMockServer(handlerFunc)
	// Create an instance of the APIClient
	APIClient := NewDynatraceAPIClient(mockServerURL, "mockedToken")
	response, err := APIClient.makeRequest("GET", "/test", "")
	if err != nil {
		t.Errorf("Error making GET request: %v", err)
	}
	defer response.Body.Close()
	// Add assertions based on the expected behavior of the mock server response
	if response.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, response.StatusCode)
	}

	// Test case: Make a POST request
	mockServerURL = setupMockServer(handlerFunc)
	APIClient = NewDynatraceAPIClient(mockServerURL, "mockedToken")
	response, err = APIClient.makeRequest("POST", "/test", `{"key": "value"}`)
	if err != nil {
		t.Errorf("Error making POST request: %v", err)
	}
	defer response.Body.Close()
	// Add assertions based on the expected behavior of the mock server response
	if response.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusCreated, response.StatusCode)
	}
}

func TestHostedControlPlaneReconciler_GetSecret(t *testing.T) {
	r := &HostedControlPlaneReconciler{
		Client: fake.NewFakeClient(),
	}
	ctx := context.Background()
	// Create a sample Secret object
	secretData := map[string][]byte{
		"apiToken": []byte("sampleApiToken123"),
		"apiUrl":   []byte("https://sampletenant.dynatrace.com"),
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "dynatrace-token", Namespace: "openshift-route-monitor-operator"},
		Data:       secretData,
	}
	if err := r.Client.Create(ctx, secret); err != nil {
		t.Fatalf("Failed to create test Secret: %v", err)
	}

	// Test the getDynatraceSecrets function
	apiToken, tenantUrl, err := r.getDynatraceSecrets(ctx)
	if err != nil {
		t.Fatalf("getDynatraceSecrets returned an error: %v", err)
	}

	expectedApiToken := "sampleApiToken123"
	expectedTenantUrl := "https://sampletenant.dynatrace.com"

	if apiToken != expectedApiToken {
		t.Errorf("Expected API Token: %s, Got: %s", expectedApiToken, apiToken)
	}

	if tenantUrl != expectedTenantUrl {
		t.Errorf("Expected Tenant URL: %s, Got: %s", expectedTenantUrl, tenantUrl)
	}
}

func TestAPIClient_GetDynatraceHTTPMonitorID(t *testing.T) {

	// Test case: Key exists in labels
	hostedControlPlane := &hypershiftv1beta1.HostedControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				httpMonitorLabel: "sampleMonitorID",
			},
		},
	}

	monitorID, err := getDynatraceHTTPMonitorID(hostedControlPlane)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	expectedMonitorID := "sampleMonitorID"
	if monitorID != expectedMonitorID {
		t.Errorf("Expected Monitor ID: %s, Got: %s", expectedMonitorID, monitorID)
	}

	// Test case: Key does not exist in labels
	hostedControlPlaneWithoutLabel := &hypershiftv1beta1.HostedControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{},
		},
	}

	monitorID, err = getDynatraceHTTPMonitorID(hostedControlPlaneWithoutLabel)
	if err == nil {
		t.Errorf("Expected error for key not found, but got nil %s", monitorID)
	}
}

func TestHostedControlPlaneReconciler_UpdateHostedControlPlaneLabels(t *testing.T) {
	r := newTestReconciler(t)

	ctx := context.Background()

	// Initialize the HostedControlPlane object with labels and metadata.name
	hostedcontrolplane := &hypershiftv1beta1.HostedControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-hostedcontrolplane",
			Namespace: "default",
			Labels: map[string]string{
				"existingKey": "existingValue",
			},
		},
	}

	key := "testKey"
	value := "testValue"

	err := r.Client.Create(ctx, hostedcontrolplane)
	if err != nil {
		t.Fatalf("Failed to create mock HostedControlPlane resource: %v", err)
	}

	// Call the method being tested
	err = r.UpdateHostedControlPlaneLabels(ctx, hostedcontrolplane, key, value)
	if err != nil {
		t.Errorf("UpdateHostedControlPlaneLabels failed: %v", err)
	}

	// Verify that the labels are updated correctly
	updatedLabels := hostedcontrolplane.GetLabels()
	if updatedLabels[key] != value {
		t.Errorf("Expected value: %s, Actual value: %s", value, updatedLabels[key])
	}
}

func TestHostedControlPlaneReconciler_GetAPIServerHostname(t *testing.T) {
	t.Run("APIServer Service Found", func(t *testing.T) {
		hostedcontrolplane := &hypershiftv1beta1.HostedControlPlane{
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
		}

		hostname, err := GetAPIServerHostname(hostedcontrolplane)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if hostname != "api.example.com" {
			t.Errorf("Expected hostname: api.example.com, got: %s", hostname)
		}
	})

	t.Run("APIServer Service Not Found", func(t *testing.T) {
		hostedcontrolplane := &hypershiftv1beta1.HostedControlPlane{
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
		}

		hostname, err := GetAPIServerHostname(hostedcontrolplane)
		if err == nil {
			t.Error("Expected error but got nil")
		}
		if hostname != "" {
			t.Errorf("Expected empty hostname, got: %s", hostname)
		}
	})
}

func TestAPIClient_GetDynatraceEquivalentClusterRegionId(t *testing.T) {
	tests := []struct {
		name               string
		region             string
		hostedControlPlane *hypershiftv1beta1.HostedControlPlane
		mockResponse       string
		mockStatusCode     int
		expectedRegionID   string
		expectedError      string
	}{
		{
			name:             "Valid region code and location found",
			region:           "us-west-2",
			mockResponse:     `{"locations": [{"name": "Oregon", "type": "PUBLIC", "cloudPlatform": "AMAZON_EC2", "entityId": "123"}]}`,
			mockStatusCode:   http.StatusOK,
			expectedRegionID: "123",
			expectedError:    "",
		},
		{
			name:             "Invalid region code (no matching location)",
			region:           "invalid-region-code",
			mockResponse:     `{"locations": []}`,
			mockStatusCode:   http.StatusBadRequest,
			expectedRegionID: "",
			expectedError:    "location not found for region: invalid-region-code",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mocking the HTTP server to return the desired response
			handlerFunc := createMockHandlerFunc(tt.mockResponse, tt.mockStatusCode)
			// Create the mock server using the setupMockServer function
			mockServer := setupMockServer(handlerFunc)
			// Create an instance of the APIClient using the reusable setup
			mockClient := NewDynatraceAPIClient(mockServer, "mockedToken")

			// Call the function being tested
			regionID, err := mockClient.getDynatraceEquivalentClusterRegionId(tt.region)

			// Check the returned values against the expected results
			if regionID != tt.expectedRegionID {
				t.Errorf("Got: %s, Expected: %s", regionID, tt.expectedRegionID)
			}

			if err != nil && err.Error() != tt.expectedError {
				t.Errorf("Got error: %v, Expected error: %s", err, tt.expectedError)
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

	// Mocked response from the server
	mockResponse := `{"entityId": "56789"}`

	// Mock the HTTP server to return the desired response
	mockServer := setupMockServer(createMockHandlerFunc(mockResponse, http.StatusOK))

	// Create an instance of the APIClient using the mock server
	mockClient := NewDynatraceAPIClient(mockServer, "mockedToken")

	t.Run("SuccessfulMonitorCreation", func(t *testing.T) {
		monitorID, err := mockClient.createDynatraceHTTPMonitor(mockMonitorName, mockApiUrl, mockClusterId, mockDynatraceEquivalentClusterRegionId)

		if err != nil {
			t.Errorf("Failed to create HTTP monitor: %v", err)
		}

		if monitorID != "56789" {
			t.Errorf("Incorrect monitor ID. Expected: 56789, Got: %s", monitorID)
		}
	})

	t.Run("ErrorResponseFromServer", func(t *testing.T) {
		// Mock the HTTP server to return an error response
		mockServerBadRequest := setupMockServer(createMockHandlerFunc("Bad request", http.StatusBadRequest))

		mockClientError := NewDynatraceAPIClient(mockServerBadRequest, "mockedToken")

		_, err := mockClientError.createDynatraceHTTPMonitor(mockMonitorName, mockApiUrl, mockClusterId, mockDynatraceEquivalentClusterRegionId)

		if err == nil {
			t.Errorf("Expected an error for failed monitor creation, but no error returned")
		}
	})
}

// Left as a placeholder for future testing.
// Currently, this function simply calls other methods and wraps any error returned,
func TestAPIClient_deployDynatraceHTTPMonitorResources(t *testing.T) {
	type fields struct {
		baseURL    string
		apiToken   string
		httpClient *http.Client
	}
	type args struct {
		ctx                context.Context
		log                logr.Logger
		hostedcontrolplane *hypershiftv1beta1.HostedControlPlane
		r                  *HostedControlPlaneReconciler
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			APIClient := &DynatraceAPIClient{
				baseURL:    tt.fields.baseURL,
				apiToken:   tt.fields.apiToken,
				httpClient: tt.fields.httpClient,
			}
			if err := APIClient.deployDynatraceHTTPMonitorResources(tt.args.ctx, tt.args.log, tt.args.hostedcontrolplane, tt.args.r); (err != nil) != tt.wantErr {
				t.Errorf("APIClient.deployDynatraceHTTPMonitorResources() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAPIClient_DeleteDynatraceHTTPMonitorResources(t *testing.T) {
	t.Run("HTTP Monitor ID not found", func(t *testing.T) {
		mockServer := setupMockServer(createMockHandlerFunc("", http.StatusOK))
		apiClient := NewDynatraceAPIClient(mockServer, "mockedToken")

		log := log.Log
		hostedControlPlane := &hypershiftv1beta1.HostedControlPlane{}

		err := apiClient.deleteDynatraceHTTPMonitorResources(log, hostedControlPlane)
		if err != nil {
			t.Errorf("Expected no error when HTTP monitor ID is not found, got: %v", err)
		}
	})

	t.Run("Successful deletion of HTTP Monitor", func(t *testing.T) {
		mockServer := setupMockServer(createMockHandlerFunc("", http.StatusOK))
		apiClient := NewDynatraceAPIClient(mockServer, "mockedToken")

		log := log.Log
		hostedControlPlane := &hypershiftv1beta1.HostedControlPlane{}

		err := apiClient.deleteDynatraceHTTPMonitorResources(log, hostedControlPlane)
		if err != nil {
			t.Errorf("Expected no error when deleting HTTP monitor, got: %v", err)
		}
	})
}

func TestAPIClient_DeleteDynatraceHTTPMonitor(t *testing.T) {
	t.Run("Successful DELETE request", func(t *testing.T) {
		mockServer := setupMockServer(createMockHandlerFunc("", http.StatusNoContent))
		apiClient := NewDynatraceAPIClient(mockServer, "mockedToken")

		err := apiClient.deleteDynatraceHTTPMonitor("123")
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
	})

	t.Run("Failed DELETE request", func(t *testing.T) {
		mockServer := setupMockServer(createMockHandlerFunc("", http.StatusInternalServerError))
		apiClient := NewDynatraceAPIClient(mockServer, "mockedToken")

		err := apiClient.deleteDynatraceHTTPMonitor("123")
		if err == nil {
			t.Error("Expected an error, but got nil")
		}
	})
}
