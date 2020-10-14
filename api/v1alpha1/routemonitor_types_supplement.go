package v1alpha1

import (
	"fmt"

	routemonitorconst "github.com/openshift/route-monitor-operator/pkg/const"
	"github.com/openshift/route-monitor-operator/pkg/const/blackbox"
	utilfinalizer "github.com/openshift/route-monitor-operator/pkg/util/finalizer"
	"k8s.io/apimachinery/pkg/types"
)

type RouteMonitorRouteSpec struct {
	// Name is the name of the Route
	Name string `json:"name,omitempty"`
	// Namespace is the namespace of the Route
	Namespace string `json:"namespace,omitempty"`
}

// TemplateForServiceMonitorName return the generated name from the RouteMonitor.
// The name is joined by the name and the namespace to create a unique ServiceMonitor for each RouteMonitor
func (r *RouteMonitor) TemplateForServiceMonitorName() types.NamespacedName {
	serviceMonitorName := fmt.Sprintf("%s-%s", r.Name, r.Namespace)
	return types.NamespacedName{Name: serviceMonitorName, Namespace: blackbox.BlackBoxNamespace}
}

// WasDeleteRequested verifies if the resource was requested for deletion
func (r RouteMonitor) WasDeleteRequested() bool {
	return r.DeletionTimestamp != nil
}

// HasFinalizer verifies if a finalizer is placed on the resource
func (r RouteMonitor) HasFinalizer() bool {
	return utilfinalizer.Contains(r.ObjectMeta.Finalizers, routemonitorconst.FinalizerKey)
}
