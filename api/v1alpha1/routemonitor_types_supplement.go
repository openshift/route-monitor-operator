package v1alpha1

import (
	utilfinalizer "github.com/openshift/route-monitor-operator/pkg/util/finalizer"
)

const (
	FinalizerKey string = "finalizer.routemonitor.openshift.io"
)

type RouteMonitorRouteSpec struct {
	// Name is the name of the Route
	Name string `json:"name,omitempty"`
	// Namespace is the namespace of the Route
	Namespace string `json:"namespace,omitempty"`
}

// WasDeleteRequested verifies if the resource was requested for deletion
func (r RouteMonitor) WasDeleteRequested() bool {
	return r.DeletionTimestamp != nil
}

func (r RouteMonitor) HasFinalizer() bool {
	return utilfinalizer.Contains(r.ObjectMeta.Finalizers, FinalizerKey)
}
