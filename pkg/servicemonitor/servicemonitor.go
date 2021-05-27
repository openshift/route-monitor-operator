package servicemonitor

import (
	"context"
	"fmt"

	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	util "github.com/openshift/route-monitor-operator/pkg/reconcile"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ServiceMonitor struct {
	Rc util.ResourceComparerInterface
}

// Creates or Updates Service Monitor Deployment according to the template
func (u *ServiceMonitor) UpdateServiceMonitorDeployment(ctx context.Context, c client.Client, template monitoringv1.ServiceMonitor) error {
	fmt.Println("UpdateServiceMonitorDeployment")
	namespacedName := types.NamespacedName{Name: template.Name, Namespace: template.Namespace}
	deployedServiceMonitor := &monitoringv1.ServiceMonitor{}
	err := c.Get(ctx, namespacedName, deployedServiceMonitor)
	if err != nil {
		// No similar ServiceMonitor exists
		if !k8serrors.IsNotFound(err) {
			return err
		}
		return c.Create(ctx, &template)
	} else if !u.Rc.DeepEqual(deployedServiceMonitor.Spec, template.Spec) {
		// Update existing ServiceMonitor for the case that the template changed
		deployedServiceMonitor.Spec = template.Spec
		return c.Update(ctx, deployedServiceMonitor)
	}
	return nil
}

// Deletes the ServiceMonitor Deployment
func (u *ServiceMonitor) DeleteServiceMonitorDeployment(ctx context.Context, c client.Client, serviceMonitorRef v1alpha1.NamespacedName) error {
	fmt.Println("DeleteServiceMonitorDeployment")
	if serviceMonitorRef == (v1alpha1.NamespacedName{}) {
		return nil
	}
	namespacedName := types.NamespacedName{Name: serviceMonitorRef.Name, Namespace: serviceMonitorRef.Namespace}
	resource := &monitoringv1.ServiceMonitor{}
	// Does the resource already exist?
	err := c.Get(ctx, namespacedName, resource)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			// If this is an unknown error
			return err
		}
		// Resource doesn't exist, nothing to do
		return nil
	}
	return c.Delete(ctx, resource)
}

func (u *ServiceMonitor) GetServiceMonitor(ctx context.Context, c client.Client, namespacedName types.NamespacedName) (monitoringv1.ServiceMonitor, error) {
	fmt.Println("GetClusterID")
	serviceMonitor := monitoringv1.ServiceMonitor{}
	err := c.Get(ctx, namespacedName, &serviceMonitor)
	return serviceMonitor, err
}
