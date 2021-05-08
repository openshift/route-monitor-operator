package util

import (
	"context"
	"fmt"
	"reflect"

	configv1 "github.com/openshift/api/config/v1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Util interface {
	DeepEqual(x, y interface{}) 
	UpdateServiceMonitorDeplyoment(c client.Client, template monitoringv1.ServiceMonitor)
	UpdatePrometheusRuleDeployment(ctx context.Context, c client.Client, template monitoringv1.PrometheusRule)
}

type UtilCollection struct{}

func (_ UtilCollection) DeepEqual(x, y interface{}) bool {
	return reflect.DeepEqual(x, y)
}

// Creates or Updates Service Monitor Deployment according to the template
func (u UtilCollection) UpdateServiceMonitorDeplyoment(c client.Client, template monitoringv1.ServiceMonitor) error {
	namespacedName := types.NamespacedName{Name: template.Name, Namespace: template.Namespace}
	deployedServiceMonitor := &monitoringv1.ServiceMonitor{}
	err := c.Get(context.TODO(), namespacedName, deployedServiceMonitor)
	if err != nil {
		// No similar ServiceMonitor exists
		if !k8serrors.IsNotFound(err) {
			return err
		}
		return c.Create(context.TODO(), &template)
	} else if !u.DeepEqual(deployedServiceMonitor.Spec, template.Spec) {
		// Update existing ServiceMonitor for the case that the template changed
		deployedServiceMonitor.Spec = template.Spec
		return c.Update(context.TODO(), deployedServiceMonitor)
	}
	return nil
}

// Creates or Updates PrometheusRule Deployment according to the template
func (u UtilCollection) UpdatePrometheusRuleDeployment(ctx context.Context, c client.Client, template monitoringv1.PrometheusRule) error {
	namespacedName := types.NamespacedName{Name: template.Name, Namespace: template.Namespace}
	deployedPrometheusRule := &monitoringv1.PrometheusRule{}
	err := c.Get(ctx, namespacedName, deployedPrometheusRule)
	if err != nil {
		// No similar Prometheus Rule exists
		if !k8serrors.IsNotFound(err) {
			return err
		}
		return c.Create(ctx, &template)
	} else if !u.DeepEqual(template.Spec, deployedPrometheusRule.Spec){
		// Update existing PrometheuesRule for the case that the template changed
		deployedPrometheusRule.Spec = template.Spec
		return c.Update(ctx, deployedPrometheusRule)
	} 
	return nil
}

func GetClusterID(c client.Client) string {
	var version configv1.ClusterVersion
	fmt.Println("Called GetClusterID")
	err := c.Get(context.TODO(), client.ObjectKey{Name: "version"}, &version)
	if err != nil {
		return ""
	}
	return string(version.Spec.ClusterID)
}

func GetServiceMonitor(c client.Client, namespacedName types.NamespacedName) (monitoringv1.ServiceMonitor, error) {
	serviceMonitor := monitoringv1.ServiceMonitor{}
	err := c.Get(context.TODO(), namespacedName, &serviceMonitor)
	return serviceMonitor, err
}

func GetClusterDomain(c client.Client, ctx context.Context) (string, error) {
	clusterConfig := configv1.Ingress{}
	err := c.Get(ctx, types.NamespacedName{Name: "cluster"}, &clusterConfig)
	if err != nil {
		return "", err
	}

	return clusterConfig.Spec.Domain, nil
}
