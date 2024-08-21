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
	dynatrace "github.com/openshift/route-monitor-operator/pkg/dynatrace"
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
			if err := r.Client.Create(ctx, secret); err != nil {
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

func TestGetDynatraceHTTPMonitorId(t *testing.T) {
	tests := []struct {
		name               string
		hostedControlPlane *hypershiftv1beta1.HostedControlPlane
		expectedMonitorId  string
		expectedFound      bool
	}{
		{
			name: "Key exists in labels",
			hostedControlPlane: &hypershiftv1beta1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						httpMonitorLabel: "sampleMonitorId",
					},
				},
			},
			expectedMonitorId: "sampleMonitorId",
			expectedFound:     true,
		},
		{
			name: "Key does not exist in labels",
			hostedControlPlane: &hypershiftv1beta1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{},
				},
			},
			expectedMonitorId: "",
			expectedFound:     false,
		},
		{
			name: "Multiple keys but target key absent",
			hostedControlPlane: &hypershiftv1beta1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"otherLabel": "someValue",
					},
				},
			},
			expectedMonitorId: "",
			expectedFound:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			monitorId, found := getDynatraceHttpMonitorId(tt.hostedControlPlane)

			if monitorId != tt.expectedMonitorId {
				t.Errorf("Expected Monitor Id for case '%s': %s, Got: %s", tt.name, tt.expectedMonitorId, monitorId)
			}
			if found != tt.expectedFound {
				t.Errorf("Expected found status for case '%s': %v, Got: %v", tt.name, tt.expectedFound, found)
			}
		})
	}
}

func TestHostedControlPlaneReconciler_UpdateHostedControlPlaneLabels(t *testing.T) {
	r := newTestReconciler(t)
	ctx := context.Background()

	tests := []struct {
		name          string
		initialLabels map[string]string
		key           string
		value         string
		expectError   bool
		expectedValue string
	}{
		{
			name: "Update existing label",
			initialLabels: map[string]string{
				"existingKey": "existingValue",
			},
			key:           "testKey",
			value:         "testValue",
			expectError:   false,
			expectedValue: "testValue",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Initialize the HostedControlPlane object
			hostedcontrolplane := &hypershiftv1beta1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hostedcontrolplane",
					Namespace: "default",
					Labels:    tt.initialLabels,
				},
			}

			err := r.Client.Create(ctx, hostedcontrolplane)
			if err != nil {
				t.Fatalf("Failed to create mock HostedControlPlane resource: %v", err)
			}

			// Call the method being tested
			err = r.UpdateHostedControlPlaneLabels(ctx, hostedcontrolplane, tt.key, tt.value)

			if (err != nil) != tt.expectError {
				t.Errorf("UpdateHostedControlPlaneLabels error = %v, expectError %v", err, tt.expectError)
			}

			// Verify that the labels are updated correctly
			updatedLabels := hostedcontrolplane.GetLabels()
			if updatedLabels[tt.key] != tt.expectedValue {
				t.Errorf("Expected value: %s, Actual value: %s", tt.expectedValue, updatedLabels[tt.key])
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
		expectedError        error
	}{
		{
			name:                 "Create Monitor Successfully",
			dynatraceMonitorId:   "",
			mockServerResponse:   `{"id":"new-monitor-id"}`,
			mockServerStatusCode: http.StatusOK,
			expectedError:        nil,
		},
		{
			name:                 "Error Creating Monitor",
			dynatraceMonitorId:   "",
			mockServerResponse:   `{"error":"creation error"}`,
			mockServerStatusCode: http.StatusInternalServerError,
			expectedError:        fmt.Errorf("error creating HTTP monitor: creation error"),
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
			r.deployDynatraceHttpMonitorResources(ctx, apiClient, log, hostedControlPlane)

		})
	}
}

func TestCheckHttpMonitorExists(t *testing.T) {
	tests := []struct {
		name               string
		mockExistsResponse string
		mockApiError       bool
		hostedControlPlane *hypershiftv1beta1.HostedControlPlane
		expectedExists     bool
		expectedError      bool
	}{
		{
			name: "Monitor exists",
			// Updated response to use the correct format with entityId
			mockExistsResponse: `{"monitors": [{"entityId": "sampleMonitorId"}]}`,
			mockApiError:       false,
			hostedControlPlane: &hypershiftv1beta1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						httpMonitorLabel: "sampleMonitorId",
					},
				},
			},
			expectedExists: true,
			expectedError:  false,
		},
		{
			name: "Monitor does not exist",
			// Updated response to indicate no monitors found
			mockExistsResponse: `{"monitors": []}`,
			mockApiError:       false,
			hostedControlPlane: &hypershiftv1beta1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						httpMonitorLabel: "sampleMonitorId",
					},
				},
			},
			expectedExists: false,
			expectedError:  false,
		},
		{
			name: "API error when checking monitor existence",
			// Simulating an error response
			mockExistsResponse: `{"error": "mock error"}`,
			mockApiError:       true,
			hostedControlPlane: &hypershiftv1beta1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						httpMonitorLabel: "sampleMonitorId",
					},
				},
			},
			expectedExists: false,
			expectedError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Prepare the mock response
			statusCode := http.StatusOK
			if tt.mockApiError {
				statusCode = http.StatusInternalServerError
			}

			// Set up the mock server
			mockServerURL := setupMockServer(createMockHandlerFunc(tt.mockExistsResponse, statusCode))
			apiClient := dynatrace.NewDynatraceApiClient(mockServerURL, "mockedToken")

			// Call the function to test
			exists, err := checkHttpMonitorExists(apiClient, tt.hostedControlPlane)

			// Validate the expected values
			if exists != tt.expectedExists {
				t.Errorf("Expected exists: %v, got: %v", tt.expectedExists, exists)
			}
			if (err != nil) != tt.expectedError {
				t.Errorf("Expected error: %v, got: %v", tt.expectedError, err)
			}
		})
	}
}

func TestAPIClient_DeleteDynatraceHTTPMonitorResources(t *testing.T) {
	tests := []struct {
		name         string
		mockResponse string
		mockStatus   int
		expectError  bool
	}{
		{
			name:         "HTTP Monitor Id not found",
			mockResponse: "",
			mockStatus:   http.StatusOK,
			expectError:  false,
		},
		{
			name:         "Successful deletion of HTTP Monitor",
			mockResponse: "", // Adjust as needed for a successful response
			mockStatus:   http.StatusOK,
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockServer := setupMockServer(createMockHandlerFunc(tt.mockResponse, tt.mockStatus))
			apiClient := dynatrace.NewDynatraceApiClient(mockServer, "mockedToken")

			log := log.Log
			hostedControlPlane := &hypershiftv1beta1.HostedControlPlane{}

			err := deleteDynatraceHttpMonitorResources(apiClient, log, hostedControlPlane)

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
