package hostedcontrolplane

import (
	"context"
	"reflect"
	"testing"

	"github.com/go-logr/logr"
	routev1 "github.com/openshift/api/route/v1"
	"github.com/openshift/hypershift/api/v1beta1"
	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func TestNewHostedControlPlaneReconciler(t *testing.T) {
	type args struct {
		mgr                 manager.Manager
		monitoringNamespace string
	}
	tests := []struct {
		name string
		args args
		want *HostedControlPlaneReconciler
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewHostedControlPlaneReconciler(tt.args.mgr, tt.args.monitoringNamespace); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewHostedControlPlaneReconciler() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHostedControlPlaneReconciler_Reconcile(t *testing.T) {
	type fields struct {
		Client              client.Client
		Scheme              *runtime.Scheme
		monitoringNamespace string
	}
	type args struct {
		ctx context.Context
		req ctrl.Request
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    ctrl.Result
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &HostedControlPlaneReconciler{
				Client:              tt.fields.Client,
				Scheme:              tt.fields.Scheme,
				monitoringNamespace: tt.fields.monitoringNamespace,
			}
			got, err := r.Reconcile(tt.args.ctx, tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("HostedControlPlaneReconciler.Reconcile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("HostedControlPlaneReconciler.Reconcile() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHostedControlPlaneReconciler_deployInternalMonitoringObjects(t *testing.T) {
	type fields struct {
		Client              client.Client
		Scheme              *runtime.Scheme
		monitoringNamespace string
	}
	type args struct {
		ctx                context.Context
		log                logr.Logger
		hostedcontrolplane *v1beta1.HostedControlPlane
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
			r := &HostedControlPlaneReconciler{
				Client:              tt.fields.Client,
				Scheme:              tt.fields.Scheme,
				monitoringNamespace: tt.fields.monitoringNamespace,
			}
			if err := r.deployInternalMonitoringObjects(tt.args.ctx, tt.args.log, tt.args.hostedcontrolplane); (err != nil) != tt.wantErr {
				t.Errorf("HostedControlPlaneReconciler.deployInternalMonitoringObjects() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_buildMetadataForUpdate(t *testing.T) {
	type args struct {
		expected metav1.ObjectMeta
		actual   metav1.ObjectMeta
	}
	tests := []struct {
		name string
		args args
		want metav1.ObjectMeta
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildMetadataForUpdate(tt.args.expected, tt.args.actual); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("buildMetadataForUpdate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHostedControlPlaneReconciler_buildInternalMonitoringRoute(t *testing.T) {
	type fields struct {
		Client              client.Client
		Scheme              *runtime.Scheme
		monitoringNamespace string
	}
	type args struct {
		hostedcontrolplane *v1beta1.HostedControlPlane
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   routev1.Route
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &HostedControlPlaneReconciler{
				Client:              tt.fields.Client,
				Scheme:              tt.fields.Scheme,
				monitoringNamespace: tt.fields.monitoringNamespace,
			}
			if got := r.buildInternalMonitoringRoute(tt.args.hostedcontrolplane); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("HostedControlPlaneReconciler.buildInternalMonitoringRoute() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHostedControlPlaneReconciler_buildInternalMonitoringRouteMonitor(t *testing.T) {
	type fields struct {
		Client              client.Client
		Scheme              *runtime.Scheme
		monitoringNamespace string
	}
	type args struct {
		route              routev1.Route
		hostedcontrolplane *v1beta1.HostedControlPlane
		apiServerPort      int64
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   v1alpha1.RouteMonitor
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &HostedControlPlaneReconciler{
				Client:              tt.fields.Client,
				Scheme:              tt.fields.Scheme,
				monitoringNamespace: tt.fields.monitoringNamespace,
			}
			if got := r.buildInternalMonitoringRouteMonitor(tt.args.route, tt.args.hostedcontrolplane, tt.args.apiServerPort); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("HostedControlPlaneReconciler.buildInternalMonitoringRouteMonitor() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHostedControlPlaneReconciler_buildOwnerReferences(t *testing.T) {
	type fields struct {
		Client              client.Client
		Scheme              *runtime.Scheme
		monitoringNamespace string
	}
	type args struct {
		hostedcontrolplane *v1beta1.HostedControlPlane
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   []metav1.OwnerReference
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &HostedControlPlaneReconciler{
				Client:              tt.fields.Client,
				Scheme:              tt.fields.Scheme,
				monitoringNamespace: tt.fields.monitoringNamespace,
			}
			if got := r.buildOwnerReferences(tt.args.hostedcontrolplane); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("HostedControlPlaneReconciler.buildOwnerReferences() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHostedControlPlaneReconciler_finalizeHostedControlPlane(t *testing.T) {
	type fields struct {
		Client              client.Client
		Scheme              *runtime.Scheme
		monitoringNamespace string
	}
	type args struct {
		ctx                context.Context
		log                logr.Logger
		hostedcontrolplane *v1beta1.HostedControlPlane
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
			r := &HostedControlPlaneReconciler{
				Client:              tt.fields.Client,
				Scheme:              tt.fields.Scheme,
				monitoringNamespace: tt.fields.monitoringNamespace,
			}
			if err := r.finalizeHostedControlPlane(tt.args.ctx, tt.args.log, tt.args.hostedcontrolplane); (err != nil) != tt.wantErr {
				t.Errorf("HostedControlPlaneReconciler.finalizeHostedControlPlane() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestHostedControlPlaneReconciler_deleteInternalMonitoringObjects(t *testing.T) {
	type fields struct {
		Client              client.Client
		Scheme              *runtime.Scheme
		monitoringNamespace string
	}
	type args struct {
		ctx                context.Context
		log                logr.Logger
		hostedcontrolplane *v1beta1.HostedControlPlane
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
			r := &HostedControlPlaneReconciler{
				Client:              tt.fields.Client,
				Scheme:              tt.fields.Scheme,
				monitoringNamespace: tt.fields.monitoringNamespace,
			}
			if err := r.deleteInternalMonitoringObjects(tt.args.ctx, tt.args.log, tt.args.hostedcontrolplane); (err != nil) != tt.wantErr {
				t.Errorf("HostedControlPlaneReconciler.deleteInternalMonitoringObjects() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestHostedControlPlaneReconciler_SetupWithManager(t *testing.T) {
	type fields struct {
		Client              client.Client
		Scheme              *runtime.Scheme
		monitoringNamespace string
	}
	type args struct {
		mgr ctrl.Manager
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
			r := &HostedControlPlaneReconciler{
				Client:              tt.fields.Client,
				Scheme:              tt.fields.Scheme,
				monitoringNamespace: tt.fields.monitoringNamespace,
			}
			if err := r.SetupWithManager(tt.args.mgr); (err != nil) != tt.wantErr {
				t.Errorf("HostedControlPlaneReconciler.SetupWithManager() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
