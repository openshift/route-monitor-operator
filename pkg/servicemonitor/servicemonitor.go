package servicemonitor

import (
	"context"
	"fmt"

	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	"github.com/openshift/route-monitor-operator/pkg/consts/blackboxexporter"
	util "github.com/openshift/route-monitor-operator/pkg/reconcile"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ServiceMonitor struct {
	Client   client.Client
	Ctx      context.Context
	Comparer util.ResourceComparerInterface
}

func NewServiceMonitor(ctx context.Context, c client.Client) *ServiceMonitor {
	return &ServiceMonitor{
		Client:   c,
		Ctx:      ctx,
		Comparer: &util.ResourceComparer{},
	}
}

const (
	ServiceMonitorPeriod string = "30s"
	UrlLabelName         string = "probe_url"
)

// Creates or Updates Service Monitor Deployment according to the template
func (u *ServiceMonitor) UpdateServiceMonitorDeployment(template monitoringv1.ServiceMonitor) error {
	namespacedName := types.NamespacedName{Name: template.Name, Namespace: template.Namespace}
	deployedServiceMonitor := &monitoringv1.ServiceMonitor{}
	err := u.Client.Get(u.Ctx, namespacedName, deployedServiceMonitor)
	if err != nil {
		// No similar ServiceMonitor exists
		if !k8serrors.IsNotFound(err) {
			return err
		}
		return u.Client.Create(u.Ctx, &template)
	}
	if !u.Comparer.DeepEqual(deployedServiceMonitor.Spec, template.Spec) {
		// Update existing ServiceMonitor for the case that the template changed
		deployedServiceMonitor.Spec = template.Spec
		return u.Client.Update(u.Ctx, deployedServiceMonitor)
	}
	return nil
}

// Deletes the ServiceMonitor Deployment
func (u *ServiceMonitor) DeleteServiceMonitorDeployment(serviceMonitorRef v1alpha1.NamespacedName) error {
	if serviceMonitorRef == (v1alpha1.NamespacedName{}) {
		return nil
	}
	namespacedName := types.NamespacedName{Name: serviceMonitorRef.Name, Namespace: serviceMonitorRef.Namespace}
	resource := &monitoringv1.ServiceMonitor{}
	// Does the resource already exist?
	err := u.Client.Get(u.Ctx, namespacedName, resource)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			// If this is an unknown error
			return err
		}
		// Resource doesn't exist, nothing to do
		return nil
	}
	return u.Client.Delete(u.Ctx, resource)
}

func (u *ServiceMonitor) GetServiceMonitor(namespacedName types.NamespacedName) (monitoringv1.ServiceMonitor, error) {
	fmt.Println("GetClusterID")
	serviceMonitor := monitoringv1.ServiceMonitor{}
	err := u.Client.Get(u.Ctx, namespacedName, &serviceMonitor)
	return serviceMonitor, err
}

// TemplateForServiceMonitorResource returns a ServiceMonitor
func TemplateForServiceMonitorResource(url, blackBoxExporterNamespace string, namespacedName types.NamespacedName, clusterID string) monitoringv1.ServiceMonitor {

	routeURL := url

	routeMonitorLabels := blackboxexporter.GenerateBlackBoxExporterLables()

	labelSelector := metav1.LabelSelector{MatchLabels: routeMonitorLabels}

	// Currently we only support `http_2xx` as module
	// Still make it a variable so we can easily add functionality later
	modules := []string{"http_2xx"}

	params := map[string][]string{
		"module": modules,
		"target": {routeURL},
	}

	serviceMonitor := monitoringv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespacedName.Name,
			Namespace: namespacedName.Namespace,
		},
		Spec: monitoringv1.ServiceMonitorSpec{
			Endpoints: []monitoringv1.Endpoint{
				{
					Port: blackboxexporter.BlackBoxExporterPortName,
					// Probe every 30s
					Interval: ServiceMonitorPeriod,
					// Timeout has to be smaller than probe interval
					ScrapeTimeout: "15s",
					Path:          "/probe",
					Scheme:        "http",
					Params:        params,
					MetricRelabelConfigs: []*monitoringv1.RelabelConfig{
						{
							Replacement: routeURL,
							TargetLabel: UrlLabelName,
						},
						{
							Replacement: clusterID,
							TargetLabel: "_id",
						},
					},
				}},
			Selector: labelSelector,
			NamespaceSelector: monitoringv1.NamespaceSelector{
				MatchNames: []string{
					blackBoxExporterNamespace,
				},
			},
		},
	}
	return serviceMonitor
}
