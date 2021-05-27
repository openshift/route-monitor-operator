package reconcileCommon

import (
	"context"
	"reflect"

	"github.com/openshift/route-monitor-operator/pkg/util/finalizer"
	"github.com/openshift/route-monitor-operator/pkg/util/reconcile"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	customerrors "github.com/openshift/route-monitor-operator/pkg/util/errors"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:generate mockgen -source $GOFILE -destination ../util/test/generated/mocks/reconcile/common.go -package $GOPACKAGE

type ResourceComparerInterface interface {
	DeepEqual(x, y interface{}) bool
}

type ResourceComparer struct{}

func (_ *ResourceComparer) DeepEqual(x, y interface{}) bool {
	return reflect.DeepEqual(x, y)
}

type MonitorReconcileCommon struct {
	Client   client.Client
	Ctx      context.Context
	Comparer ResourceComparerInterface
}

func NewMonitorReconcileCommon(ctx context.Context, c client.Client) *MonitorReconcileCommon {
	return &MonitorReconcileCommon{
		Client:   c,
		Ctx:      ctx,
		Comparer: &ResourceComparer{},
	}
}

// returns whether the errorStatus has changed
func (u *MonitorReconcileCommon) SetErrorStatus(errorStatus *string, err error) bool {
	switch {
	case u.areErrorAndErrorStatusFull(errorStatus, err):
		return false
	case u.needsErrorStatusToBeFlushed(errorStatus, err):
		*errorStatus = ""
		return true
	case u.needsErrorStatusToBeSet(errorStatus, err):
		*errorStatus = err.Error()
		return true
	}
	return false
}

// If an error has already been flagged and still occurs
func (u *MonitorReconcileCommon) areErrorAndErrorStatusFull(errorStatus *string, err error) bool {
	return *errorStatus != "" && err != nil
}

// If the error was flagged but stopped firing
func (u *MonitorReconcileCommon) needsErrorStatusToBeFlushed(errorStatus *string, err error) bool {
	return *errorStatus != "" && err == nil
}

// If the error was not flagged but has started firing
func (u *MonitorReconcileCommon) needsErrorStatusToBeSet(errorStatus *string, err error) bool {
	return *errorStatus == "" && err != nil
}

func (u *MonitorReconcileCommon) SetResourceReference(reference *v1alpha1.NamespacedName, targetNamespace types.NamespacedName) (bool, error) {
	desiredRef := v1alpha1.NamespacedName{Name: targetNamespace.Name, Namespace: targetNamespace.Namespace}
	if *reference == (v1alpha1.NamespacedName{}) {
		*reference = desiredRef
		return true, nil
	} else if desiredRef == (v1alpha1.NamespacedName{}) {
		*reference = desiredRef
		return true, nil
	} else if *reference != desiredRef {
		// TODO Check when this is really required
		return false, customerrors.InvalidReferenceUpdate
	} else {
		return false, nil
	}
}

// TODO rename ParseSLOMonitorSpecs (string, error)
// remove boolean
func (u *MonitorReconcileCommon) ParseSLOMonitorSpecs(routeURL string, sloSpec v1alpha1.SloSpec) (string, error) {
	if routeURL == "" {
		return "", customerrors.NoHost
	}
	if sloSpec == *new(v1alpha1.SloSpec) {
		return "", nil
	}
	isValid, parsedSlo := sloSpec.IsValid()
	if !isValid {
		return "", customerrors.InvalidSLO
	}
	return parsedSlo, nil
}

// Deletes Finalizer on object if defined
func (u *MonitorReconcileCommon) DeleteFinalizer(o v1.Object, finalizerKey string) bool {
	if finalizer.HasFinalizer(o, finalizerKey) {
		// if finalizer is still here and ServiceMonitor is deleted, then remove the finalizer	fmt.Println("DeleteServiceMonitorDeployment")
		finalizer.Remove(o, finalizerKey)
		return true
	}
	return false
}

// Attaches Finalizer to object
func (u *MonitorReconcileCommon) SetFinalizer(o v1.Object, finalizerKey string) bool {
	if !finalizer.HasFinalizer(o, finalizerKey) {
		// if finalizer is still here and ServiceMonitor is deleted, then remove the finalizer
		finalizer.Add(o, finalizerKey)
		return true
	}
	return false
}

// Updates the ClusterURLMonitor and RouteMonitor CR in reconcile loops
func (u *MonitorReconcileCommon) UpdateReconciledMonitor(cr runtime.Object) (reconcile.Result, error) {
	if err := u.Client.Update(u.Ctx, cr); err != nil {
		return reconcile.RequeueReconcileWith(err)
	}
	// After Updating watched CR we need to requeue, to prevent that two reconcile threads are running
	return reconcile.StopReconcile()
}

// Updates the ClusterURLMonitor and RouteMonitor CR Status in reconcile loops
func (u *MonitorReconcileCommon) UpdateReconciledMonitorStatus(cr runtime.Object) (reconcile.Result, error) {
	if err := u.Client.Status().Update(u.Ctx, cr); err != nil {
		return reconcile.RequeueReconcileWith(err)
	}
	// After Updating watched CR we need to requeue, to prevent that two reconcile threads are running
	return reconcile.StopReconcile()
}

func (u *MonitorReconcileCommon) GetClusterID() string {
	var version configv1.ClusterVersion
	err := u.Client.Get(u.Ctx, client.ObjectKey{Name: "version"}, &version)
	if err != nil {
		return ""
	}
	return string(version.Spec.ClusterID)
}

func (u *MonitorReconcileCommon) GetServiceMonitor(namespacedName types.NamespacedName) (monitoringv1.ServiceMonitor, error) {
	serviceMonitor := monitoringv1.ServiceMonitor{}
	err := u.Client.Get(u.Ctx, namespacedName, &serviceMonitor)
	return serviceMonitor, err
}
