package int

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/onsi/ginkgo"
	configv1 "github.com/openshift/api/config/v1"
	routev1 "github.com/openshift/api/route/v1"
	"github.com/openshift/route-monitor-operator/pkg/util/templates"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	monitoringopenshiftiov1alpha1 "github.com/openshift/route-monitor-operator/api/v1alpha1"
	monitoringv1alpha1 "github.com/openshift/route-monitor-operator/api/v1alpha1"
)

type Integration struct {
	Client     client.Client
	clientChan chan struct{}
	mgr        manager.Manager
}

func NewIntegration() (*Integration, error) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(monitoringv1alpha1.AddToScheme(scheme))
	utilruntime.Must(monitoringv1.AddToScheme(scheme))
	utilruntime.Must(routev1.AddToScheme(scheme))
	utilruntime.Must(configv1.AddToScheme(scheme))

	utilruntime.Must(monitoringopenshiftiov1alpha1.AddToScheme(scheme))
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
	})
	if err != nil {
		return &Integration{}, err
	}
	client := mgr.GetClient()
	i := Integration{client, make(chan struct{}), mgr}
	go func(x chan struct{}) {
		err := mgr.GetCache().Start(x)
		if err != nil {
			panic(err)
		}
	}(i.clientChan)

	// Wait for cache to start. Is there a better way?
	time.Sleep(2 * time.Second)
	return &i, nil
}

func (i *Integration) Shutdown() {
	close(i.clientChan)
}

func (i *Integration) RemoveClusterUrlMonitor(namespace, name string) error {
	namespacedName := types.NamespacedName{Namespace: namespace, Name: name}
	clusterUrlMonitor := v1alpha1.ClusterUrlMonitor{}

	err := i.Client.Get(context.TODO(), namespacedName, &clusterUrlMonitor)
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	err = i.Client.Delete(context.TODO(), &clusterUrlMonitor)
	if err != nil {
		return err
	}
	t := 0
	maxRetries := 20
	for ; t < maxRetries; t++ {
		err := i.Client.Get(context.TODO(), namespacedName, &clusterUrlMonitor)
		if errors.IsNotFound(err) {
			break
		}
		time.Sleep(1 * time.Second)
	}
	if t == maxRetries {
		ginkgo.Fail("ClusterUrlMonitor didn't appear after %d seconds", maxRetries)
	}
	return err
}

func (i *Integration) RemoveRouteMonitor(namespace, name string) error {
	namespacedName := types.NamespacedName{Namespace: namespace, Name: name}
	routeMonitor := v1alpha1.RouteMonitor{}

	err := i.Client.Get(context.TODO(), namespacedName, &routeMonitor)
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	err = i.Client.Delete(context.TODO(), &routeMonitor)
	if err != nil {
		return err
	}
	t := 0
	maxRetries := 20
	for ; t < maxRetries; t++ {
		err := i.Client.Get(context.TODO(), namespacedName, &routeMonitor)
		if errors.IsNotFound(err) {
			break
		}
		time.Sleep(1 * time.Second)
	}
	if t == maxRetries {
		ginkgo.Fail("RouteMonitor didn't appear after %d seconds", maxRetries)
	}
	return err
}

func (i *Integration) WaitForServiceMonitor(name types.NamespacedName, seconds int) (monitoringv1.ServiceMonitor, error) {
	serviceMonitor := monitoringv1.ServiceMonitor{}
	t := 0
	for ; t < seconds; t++ {
		err := i.Client.Get(context.TODO(), name, &serviceMonitor)
		if !errors.IsNotFound(err) {
			break
		}
		time.Sleep(1 * time.Second)
	}
	if t == seconds {
		return serviceMonitor, fmt.Errorf("ServiceMonitor didn't appear after %d seconds", seconds)
	}
	return serviceMonitor, nil
}

func (i *Integration) WaitForPrometheusRule(name types.NamespacedName, seconds int) (monitoringv1.PrometheusRule, error) {
	prometheusRule := monitoringv1.PrometheusRule{}
	t := 0
	for ; t < seconds; t++ {
		err := i.Client.Get(context.TODO(), name, &prometheusRule)
		if !errors.IsNotFound(err) {
			break
		}
		time.Sleep(1 * time.Second)
	}
	if t == seconds {
		return prometheusRule, fmt.Errorf("PrometheusRule didn't appear after %d seconds", seconds)
	}
	return prometheusRule, nil
}

func (i *Integration) RouteMonitorWaitForPrometheusRuleRef(name types.NamespacedName, seconds int) (v1alpha1.RouteMonitor, error) {
	routeMonitor := v1alpha1.RouteMonitor{}
	t := 0
	for ; t < seconds; t++ {
		err := i.Client.Get(context.TODO(), name, &routeMonitor)
		if routeMonitor.Status.PrometheusRuleRef.Name != "" || err != nil {
			return routeMonitor, err
		}
		time.Sleep(1 * time.Second)
	}
	if t == seconds {
		return routeMonitor, fmt.Errorf("PrometheusRuleRef didn't appear after %d seconds", seconds)
	}
	return routeMonitor, nil
}

func (i *Integration) ClusterUrlMonitorWaitForPrometheusRuleRef(name types.NamespacedName, seconds int) (v1alpha1.ClusterUrlMonitor, error) {
	clusterUrlMonitor := v1alpha1.ClusterUrlMonitor{}
	t := 0
	for ; t < seconds; t++ {
		err := i.Client.Get(context.TODO(), name, &clusterUrlMonitor)
		if clusterUrlMonitor.Status.PrometheusRuleRef.Name != "" || err != nil {
			return clusterUrlMonitor, err
		}
		time.Sleep(1 * time.Second)
	}
	if t == seconds {
		return clusterUrlMonitor, fmt.Errorf("PrometheusRuleRef didn't appear after %d seconds", seconds)
	}
	return clusterUrlMonitor, nil
}

func (i *Integration) WaitForPrometheusRuleToClear(name types.NamespacedName, seconds int) error {
	prometheusRule := monitoringv1.PrometheusRule{}
	t := 0
	for ; t < seconds; t++ {
		err := i.Client.Get(context.TODO(), name, &prometheusRule)
		if errors.IsNotFound(err) {
			break
		}
		time.Sleep(1 * time.Second)
	}
	if t == seconds {
		return fmt.Errorf("PrometheusRule didn't vanish after %d seconds", seconds)
	}
	return nil
}

func (i *Integration) RouteMonitorWaitForPrometheusRuleCorrectSLO(name types.NamespacedName, targetSlo string, seconds int, kind string) error {
	prometheusRule := monitoringv1.PrometheusRule{}
	err := i.Client.Get(context.TODO(), name, &prometheusRule)
	if errors.IsNotFound(err) {
		return fmt.Errorf("PrometheusRule wasn't found")
	}
	if err != nil {
		return err
	}

	routeMonitor := v1alpha1.RouteMonitor{}
	err = i.Client.Get(context.TODO(), name, &routeMonitor)
	if errors.IsNotFound(err) {
		return fmt.Errorf("RouteMonitor wasn't found")
	}
	if err != nil {
		return err
	}

	template := templates.TemplateForPrometheusRuleResource(routeMonitor.Status.RouteURL, targetSlo, name)
	t := 0
	for ; t < seconds; t++ {
		err := i.Client.Get(context.TODO(), name, &prometheusRule)
		if errors.IsNotFound(err) {
			return fmt.Errorf("PrometheusRule wasn't found")
		}
		if reflect.DeepEqual(template.Spec, prometheusRule.Spec) {
			break
		}
		time.Sleep(1 * time.Second)
	}
	if t == seconds {
		return fmt.Errorf("PrometheusRule wasn't updated with the correct SLO after %d seconds", seconds)
	}
	return nil
}

func (i *Integration) ClusterUrlMonitorWaitForPrometheusRuleCorrectSLO(name types.NamespacedName, targetSlo string, seconds int, expectedUrl string, kind string) error {
	prometheusRule := monitoringv1.PrometheusRule{}
	err := i.Client.Get(context.TODO(), name, &prometheusRule)
	if errors.IsNotFound(err) {
		return fmt.Errorf("PrometheusRule wasn't found")
	}
	if err != nil {
		return err
	}

	clusterUrlMonitor := v1alpha1.ClusterUrlMonitor{}
	err = i.Client.Get(context.TODO(), name, &clusterUrlMonitor)
	if errors.IsNotFound(err) {
		return fmt.Errorf("ClusterUrlMonitor wasn't found")
	}
	if err != nil {
		return err
	}

	template := templates.TemplateForPrometheusRuleResource(expectedUrl, targetSlo, name)
	t := 0
	for ; t < seconds; t++ {
		err := i.Client.Get(context.TODO(), name, &prometheusRule)
		if errors.IsNotFound(err) {
			return fmt.Errorf("PrometheusRule wasn't found")
		}
		if reflect.DeepEqual(template.Spec, prometheusRule.Spec) {
			break
		}
		time.Sleep(1 * time.Second)
	}
	if t == seconds {
		return fmt.Errorf("PrometheusRule wasn't updated with the correct SLO after %d seconds", seconds)
	}
	return nil
}
