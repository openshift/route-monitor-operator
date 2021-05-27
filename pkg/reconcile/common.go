package reconcileCommon

import (
	"context"
	"fmt"
	"reflect"

	"github.com/openshift/route-monitor-operator/pkg/util/finalizer"
	"github.com/openshift/route-monitor-operator/pkg/util/reconcile"
	utilreconcile "github.com/openshift/route-monitor-operator/pkg/util/reconcile"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	customerrors "github.com/openshift/route-monitor-operator/pkg/util/errors"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ResourceComparerInterface interface {
	DeepEqual(x, y interface{}) bool
}

type ResourceComparer struct{}

func (_ *ResourceComparer) DeepEqual(x, y interface{}) bool {
	fmt.Println("DeepEqual")
	return reflect.DeepEqual(x, y)
}

type MonitorReconcileCommon struct {
	Comparer ResourceComparerInterface
}

// returns whether the errorStatus has changed
func (u *MonitorReconcileCommon) SetErrorStatus(errorStatus *string, err error) bool {
	fmt.Println("SetErrorStatus")
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
	fmt.Println("SetResourceReference")
	desiredRef := v1alpha1.NamespacedName{Name: targetNamespace.Name, Namespace: targetNamespace.Namespace}
	if *reference == (v1alpha1.NamespacedName{}) {
		*reference = desiredRef
		return true, nil
	} else if *reference != desiredRef {
		// TODO Check when this is really required
		return false, customerrors.InvalidReferenceUpdate
	} else {
		return false, nil
	}
}

func (u *MonitorReconcileCommon) AreMonitorSettingsValid(routeURL string, sloSpec v1alpha1.SloSpec) (bool, error, string) {
	fmt.Println("AreMonitorSettingsValid")
	if routeURL == "" {
		return false, customerrors.NoHost, ""
	}
	if sloSpec == *new(v1alpha1.SloSpec) {
		return false, nil, ""
	}
	isValid, parsedSlo := sloSpec.IsValid()
	if !isValid {
		return false, customerrors.InvalidSLO, ""
	}
	return true, nil, parsedSlo
}

// Deletes Finalizer on object if defined
func (u *MonitorReconcileCommon) DeleteFinalizer(o v1.Object, finalizerKey string) bool {
	fmt.Println("DeleteFinalizer")
	if finalizer.HasFinalizer(o, finalizerKey) {
		// if finalizer is still here and ServiceMonitor is deleted, then remove the finalizer
		finalizer.Remove(o, finalizerKey)
		return true
	}
	return false
}

// Attaches Finalizer to object
func (u *MonitorReconcileCommon) SetFinalizer(o v1.Object, finalizerKey string) bool {
	fmt.Println("SetFinalizer")
	if !finalizer.HasFinalizer(o, finalizerKey) {
		// if finalizer is still here and ServiceMonitor is deleted, then remove the finalizer
		finalizer.Add(o, finalizerKey)
		return true
	}
	return false
}

// Updates the ClusterURLMonitor and RouteMonitor CR in reconcile loops
func (u *MonitorReconcileCommon) UpdateReconciledMonitor(ctx context.Context, c client.Client, cr runtime.Object) (reconcile.Result, error) {
	fmt.Println("UpdateReconciledCR")
	if err := c.Update(ctx, cr); err != nil {
		return utilreconcile.RequeueReconcileWith(err)
	}
	// After Updating watched CR we need to requeue, to prevent that two reconcile threads are running
	return utilreconcile.StopReconcile()
}

func (u *MonitorReconcileCommon) GetClusterID(c client.Client) string {
	fmt.Println("GetClusterID")
	var version configv1.ClusterVersion
	err := c.Get(context.TODO(), client.ObjectKey{Name: "version"}, &version)
	if err != nil {
		return ""
	}
	return string(version.Spec.ClusterID)
}

func (u *MonitorReconcileCommon) GetServiceMonitor(ctx context.Context, c client.Client, namespacedName types.NamespacedName) (monitoringv1.ServiceMonitor, error) {
	fmt.Println("GetClusterID")
	serviceMonitor := monitoringv1.ServiceMonitor{}
	err := c.Get(ctx, namespacedName, &serviceMonitor)
	return serviceMonitor, err
}

func (u *MonitorReconcileCommon) GetClusterDomain(ctx context.Context, c client.Client) (string, error) {
	clusterConfig := configv1.Ingress{}
	err := c.Get(ctx, types.NamespacedName{Name: "cluster"}, &clusterConfig)
	if err != nil {
		return "", err
	}
	return clusterConfig.Spec.Domain, nil
}
