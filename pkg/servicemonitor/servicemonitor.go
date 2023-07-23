package servicemonitor

import (
	"context"
	"reflect"

	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	"github.com/openshift/route-monitor-operator/pkg/blackboxexporter"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	rhobsv1 "github.com/rhobs/obo-prometheus-operator/pkg/apis/monitoring/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ServiceMonitor struct {
	Client client.Client
}

func NewServiceMonitor(c client.Client) *ServiceMonitor {
	return &ServiceMonitor{
		Client: c,
	}
}

const (
	MetricsScrapeInterval = "30s"
	UrlLabelName          = "probe_url"
)

// TemplateAndUpdateServiceMonitorDeployment will generate a template and then
// call UpdateServiceMonitorDeployment to ensure its current state matches the template.
func (u *ServiceMonitor) TemplateAndUpdateServiceMonitorDeployment(ctx context.Context, routeURL, blackBoxExporterNamespace string, namespacedName types.NamespacedName, clusterID string, isHCPMonitor bool) error {
	params := map[string][]string{
		// Currently we only support `http_2xx` as module
		"module": {"http_2xx"},
		"target": {routeURL},
	}

	if isHCPMonitor {
		s := u.HyperShiftTemplateForServiceMonitorResource(routeURL, blackBoxExporterNamespace, params, namespacedName, clusterID)
		return u.HypershiftUpdateServiceMonitorDeployment(ctx, s)
	}
	s := u.TemplateForServiceMonitorResource(routeURL, blackBoxExporterNamespace, params, namespacedName, clusterID)
	return u.UpdateServiceMonitorDeployment(ctx, s)
}

// UpdateServiceMonitorDeployment ensures that a ServiceMonitor deployment according
// to the template exists. If none exists, it will create a new one.
// If the template changed, it will update the existing deployment
func (u *ServiceMonitor) UpdateServiceMonitorDeployment(ctx context.Context, template monitoringv1.ServiceMonitor) error {
	namespacedName := types.NamespacedName{Name: template.Name, Namespace: template.Namespace}
	deployedServiceMonitor := &monitoringv1.ServiceMonitor{}
	if err := u.Client.Get(ctx, namespacedName, deployedServiceMonitor); err != nil {
		// No similar ServiceMonitor exists
		if !kerr.IsNotFound(err) {
			return err
		}
		return u.Client.Create(ctx, &template)
	}
	if !reflect.DeepEqual(deployedServiceMonitor.Spec, template.Spec) {
		// Update existing ServiceMonitor for the case that the template changed
		deployedServiceMonitor.Spec = template.Spec
		return u.Client.Update(ctx, deployedServiceMonitor)
	}
	return nil
}

// HypershiftUpdateServiceMonitorDeployment is for HyperShift cluster to ensure that a ServiceMonitor deployment according
// to the template exists. If none exists, it will create a new one. If the template changed, it will update the existing deployment
func (u *ServiceMonitor) HypershiftUpdateServiceMonitorDeployment(ctx context.Context, template rhobsv1.ServiceMonitor) error {
	namespacedName := types.NamespacedName{Name: template.Name, Namespace: template.Namespace}
	deployedServiceMonitor := &rhobsv1.ServiceMonitor{}
	if err := u.Client.Get(ctx, namespacedName, deployedServiceMonitor); err != nil {
		// No similar ServiceMonitor exists
		if !kerr.IsNotFound(err) {
			return err
		}
		return u.Client.Create(ctx, &template)
	}
	if !reflect.DeepEqual(deployedServiceMonitor.Spec, template.Spec) {
		// Update existing ServiceMonitor for the case that the template changed
		deployedServiceMonitor.Spec = template.Spec
		return u.Client.Update(ctx, deployedServiceMonitor)
	}
	return nil
}

// DeleteServiceMonitorDeployment deletes the corresponding ServiceMonitor
// servicemonitor.monitoring.rhobs for HCP or
// servicemonitor.monitoring.coreos.com otherwise
func (u *ServiceMonitor) DeleteServiceMonitorDeployment(ctx context.Context, serviceMonitorRef v1alpha1.NamespacedName, isHCP bool) error {
	if serviceMonitorRef.Name == "" || serviceMonitorRef.Namespace == "" {
		return nil
	}

	if isHCP {
		resource := &rhobsv1.ServiceMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceMonitorRef.Name,
				Namespace: serviceMonitorRef.Namespace,
			},
		}
		if err := u.Client.Delete(ctx, resource); err != nil {
			return client.IgnoreNotFound(err)
		}

		return nil
	}
	resource := &monitoringv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceMonitorRef.Name,
			Namespace: serviceMonitorRef.Namespace,
		},
	}
	if err := u.Client.Delete(ctx, resource); err != nil {
		return client.IgnoreNotFound(err)
	}

	return nil
}

// TemplateForServiceMonitorResource returns a ServiceMonitor
func (u *ServiceMonitor) TemplateForServiceMonitorResource(routeURL, blackBoxExporterNamespace string, params map[string][]string, namespacedName types.NamespacedName, clusterID string) monitoringv1.ServiceMonitor {
	return monitoringv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespacedName.Name,
			Namespace: namespacedName.Namespace,
		},
		Spec: monitoringv1.ServiceMonitorSpec{
			Endpoints: []monitoringv1.Endpoint{
				{
					Port: blackboxexporter.ContainerPortName,
					// Probe every 30s
					Interval: MetricsScrapeInterval,
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
			Selector: metav1.LabelSelector{
				MatchLabels: blackboxexporter.Labels(),
			},
			NamespaceSelector: monitoringv1.NamespaceSelector{
				MatchNames: []string{
					blackBoxExporterNamespace,
				},
			},
		},
	}
}

// HyperShiftTemplateForServiceMonitorResource returns a ServiceMonitor for Hypershift
func (u *ServiceMonitor) HyperShiftTemplateForServiceMonitorResource(routeURL, blackBoxExporterNamespace string, params map[string][]string, namespacedName types.NamespacedName, clusterID string) rhobsv1.ServiceMonitor {
	return rhobsv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespacedName.Name,
			Namespace: namespacedName.Namespace,
		},
		Spec: rhobsv1.ServiceMonitorSpec{
			Endpoints: []rhobsv1.Endpoint{
				{
					Port: blackboxexporter.ContainerPortName,
					// Probe every 30s
					Interval: MetricsScrapeInterval,
					// Timeout has to be smaller than probe interval
					ScrapeTimeout: "15s",
					Path:          "/probe",
					Scheme:        "http",
					Params:        params,
					MetricRelabelConfigs: []*rhobsv1.RelabelConfig{
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
			Selector: metav1.LabelSelector{
				MatchLabels: blackboxexporter.Labels(),
			},
			NamespaceSelector: rhobsv1.NamespaceSelector{
				MatchNames: []string{
					blackBoxExporterNamespace,
				},
			},
		},
	}
}
