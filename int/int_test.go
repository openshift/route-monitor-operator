package int_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	. "github.com/openshift/route-monitor-operator/int"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Integrationtests", func() {
	var (
		i *Integration
	)
	BeforeSuite(func() {
		var err error
		i, err = NewIntegration()
		Expect(err).NotTo(HaveOccurred())
	})
	AfterSuite(func() {
		i.Shutdown()
	})

	Context("ClusterUrlMonitor creation", func() {
		var (
			clusterUrlMonitorName      string
			clusterUrlMonitorNamespace string
			clusterUrlMonitor          v1alpha1.ClusterUrlMonitor
			expectedServiceMonitorName types.NamespacedName
		)
		BeforeEach(func() {
			clusterUrlMonitorName = "fake-url-monitor"
			clusterUrlMonitorNamespace = "default"

			err := i.RemoveClusterUrlMonitor(clusterUrlMonitorNamespace, clusterUrlMonitorName)
			Expect(err).NotTo(HaveOccurred())
			expectedServiceMonitorName = types.NamespacedName{Name: clusterUrlMonitorName, Namespace: clusterUrlMonitorNamespace}
			clusterUrlMonitor = v1alpha1.ClusterUrlMonitor{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: clusterUrlMonitorNamespace,
					Name:      clusterUrlMonitorName,
				},
				Spec: v1alpha1.ClusterUrlMonitorSpec{
					Prefix: "fake-prefix.",
					Port:   "1234",
					Suffix: "/fake-suffix",
				},
			}
		})
		AfterEach(func() {
			err := i.RemoveClusterUrlMonitor(clusterUrlMonitorNamespace, clusterUrlMonitorName)
			Expect(err).NotTo(HaveOccurred())
		})

		When("the ClusterUrlMonitor does not exist", func() {
			It("creates a ServiceMonitor within 20 seconds", func() {
				err := i.Client.Create(context.TODO(), &clusterUrlMonitor)
				Expect(err).NotTo(HaveOccurred())

				serviceMonitor, err := i.WaitForServiceMonitor(expectedServiceMonitorName, 20)
				Expect(err).NotTo(HaveOccurred())

				Expect(serviceMonitor.Name).To(Equal(expectedServiceMonitorName.Name))
				Expect(serviceMonitor.Namespace).To(Equal(expectedServiceMonitorName.Namespace))

				clusterConfig := configv1.Ingress{}
				err = i.Client.Get(context.TODO(), types.NamespacedName{Name: "cluster"}, &clusterConfig)
				Expect(err).NotTo(HaveOccurred())
				spec := clusterUrlMonitor.Spec
				expectedUrl := spec.Prefix + clusterConfig.Spec.Domain + ":" + spec.Port + spec.Suffix
				Expect(len(serviceMonitor.Spec.Endpoints)).To(Equal(1))
				Expect(len(serviceMonitor.Spec.Endpoints[0].Params["target"])).To(Equal(1))
				Expect(serviceMonitor.Spec.Endpoints[0].Params["target"][0]).To(Equal(expectedUrl))

				updatedClusterUrlMonitor := v1alpha1.ClusterUrlMonitor{}
				err = i.Client.Get(context.TODO(), types.NamespacedName{Namespace: clusterUrlMonitorNamespace, Name: clusterUrlMonitorName}, &updatedClusterUrlMonitor)
				Expect(err).NotTo(HaveOccurred())
				Expect(updatedClusterUrlMonitor.Status.ServiceMonitorRef.Name).To(Equal(serviceMonitor.Name))
				Expect(updatedClusterUrlMonitor.Status.ServiceMonitorRef.Namespace).To(Equal(serviceMonitor.Namespace))
			})
		})

		When("the ClusterUrlMonitor is deleted", func() {
			BeforeEach(func() {
				err := i.Client.Create(context.TODO(), &clusterUrlMonitor)
				Expect(err).NotTo(HaveOccurred())

				_, err = i.WaitForServiceMonitor(expectedServiceMonitorName, 20)
				Expect(err).NotTo(HaveOccurred())

				err = i.RemoveClusterUrlMonitor(clusterUrlMonitorNamespace, clusterUrlMonitorName)
				Expect(err).NotTo(HaveOccurred())
			})

			It("removes the ServiceMonitor as well within 20 seconds", func() {
				serviceMonitor := monitoringv1.ServiceMonitor{}
				err := i.Client.Get(context.TODO(), expectedServiceMonitorName, &serviceMonitor)
				Expect(err).To(HaveOccurred())
				Expect(errors.IsNotFound(err)).To(BeTrue())
			})
		})
	})

	Context("RouteMonitor creation", func() {
		var (
			routeMonitorName          string
			routeMonitorNamespace     string
			routeMonitor              v1alpha1.RouteMonitor
			expectedDependentResource types.NamespacedName
		)
		BeforeEach(func() {
			routeMonitorName = "fake-route-monitor"
			routeMonitorNamespace = "default"
			err := i.RemoveRouteMonitor(routeMonitorNamespace, routeMonitorName)
			expectedDependentResource = types.NamespacedName{Name: routeMonitorName, Namespace: routeMonitorNamespace}
			Expect(err).NotTo(HaveOccurred())
			routeMonitor = v1alpha1.RouteMonitor{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: routeMonitorNamespace,
					Name:      routeMonitorName,
				},
				Spec: v1alpha1.RouteMonitorSpec{
					Slo: v1alpha1.SloSpec{
						TargetAvailabilityPercentile: "0.9995",
					},
					Route: v1alpha1.RouteMonitorRouteSpec{
						Name:      "console",
						Namespace: "openshift-console",
					},
				},
			}
		})
		AfterEach(func() {
			err := i.RemoveRouteMonitor(routeMonitorNamespace, routeMonitorName)
			Expect(err).NotTo(HaveOccurred())
		})

		When("the RouteMonitor does not exist", func() {
			It("creates a ServiceMonitor within 20 seconds", func() {
				err := i.Client.Create(context.TODO(), &routeMonitor)
				Expect(err).NotTo(HaveOccurred())

				serviceMonitor, err := i.WaitForServiceMonitor(expectedDependentResource, 20)
				Expect(err).NotTo(HaveOccurred())
				Expect(serviceMonitor.Name).To(Equal(expectedDependentResource.Name))
				Expect(serviceMonitor.Namespace).To(Equal(expectedDependentResource.Namespace))

				prometheusRule, err := i.WaitForPrometheusRule(expectedDependentResource, 20)
				Expect(err).NotTo(HaveOccurred())
				Expect(prometheusRule.Name).To(Equal(expectedDependentResource.Name))
				Expect(prometheusRule.Namespace).To(Equal(expectedDependentResource.Namespace))

				updatedRouteMonitor := v1alpha1.RouteMonitor{}
				err = i.Client.Get(context.TODO(), types.NamespacedName{Namespace: routeMonitorNamespace, Name: routeMonitorName}, &updatedRouteMonitor)
				Expect(err).NotTo(HaveOccurred())

				Expect(updatedRouteMonitor.Status.PrometheusRuleRef.Name).To(Equal(prometheusRule.Name))
				Expect(updatedRouteMonitor.Status.PrometheusRuleRef.Namespace).To(Equal(prometheusRule.Namespace))

				Expect(updatedRouteMonitor.Status.ServiceMonitorRef.Name).To(Equal(serviceMonitor.Name))
				Expect(updatedRouteMonitor.Status.ServiceMonitorRef.Namespace).To(Equal(serviceMonitor.Namespace))
			})
		})

		When("the RouteMonitor is deleted", func() {
			BeforeEach(func() {
				err := i.Client.Create(context.TODO(), &routeMonitor)
				Expect(err).NotTo(HaveOccurred())

				_, err = i.WaitForServiceMonitor(expectedDependentResource, 20)
				Expect(err).NotTo(HaveOccurred())

				_, err = i.WaitForPrometheusRule(expectedDependentResource, 20)
				Expect(err).NotTo(HaveOccurred())

				err = i.RemoveRouteMonitor(routeMonitorNamespace, routeMonitorName)
				Expect(err).NotTo(HaveOccurred())
			})

			It("removes the Dependant resources as well", func() {
				serviceMonitor := monitoringv1.ServiceMonitor{}
				err := i.Client.Get(context.TODO(), expectedDependentResource, &serviceMonitor)
				Expect(err).To(HaveOccurred())
				Expect(errors.IsNotFound(err)).To(BeTrue())

				prometheusRule := monitoringv1.PrometheusRule{}
				err = i.Client.Get(context.TODO(), expectedDependentResource, &prometheusRule)
				Expect(err).To(HaveOccurred())
				Expect(errors.IsNotFound(err)).To(BeTrue())
			})
		})

		PWhen("the RouteMonitor Slo changes", func() {
			It("creates/deletes a PrometheusRule as necessary", func() {
				By("creating a RouteMonitor with no SLO")
				routeMonitor.Spec.Slo = v1alpha1.SloSpec{}
				err := i.Client.Create(context.TODO(), &routeMonitor)
				Expect(err).NotTo(HaveOccurred())

				_, err = i.WaitForServiceMonitor(expectedDependentResource, 20)
				Expect(err).NotTo(HaveOccurred())
				err = i.WaitForPrometheusRuleToClear(expectedDependentResource, 20)
				Expect(err).NotTo(HaveOccurred())

				By("adding a SLO")
				err = i.Client.Get(context.TODO(), expectedDependentResource, &routeMonitor)
				Expect(err).NotTo(HaveOccurred())
				routeMonitor.Spec.Slo.TargetAvailabilityPercentile = "0.995"
				err = i.Client.Update(context.TODO(), &routeMonitor)
				Expect(err).NotTo(HaveOccurred())

				_, err = i.WaitForPrometheusRule(expectedDependentResource, 20)
				Expect(err).NotTo(HaveOccurred())

				By("removing the SLO again")
				err = i.Client.Get(context.TODO(), expectedDependentResource, &routeMonitor)
				Expect(err).NotTo(HaveOccurred())
				routeMonitor.Spec.Slo = v1alpha1.SloSpec{}
				err = i.Client.Update(context.TODO(), &routeMonitor)
				Expect(err).NotTo(HaveOccurred())

				err = i.WaitForPrometheusRuleToClear(expectedDependentResource, 20)
				Expect(err).NotTo(HaveOccurred())

				err = i.RemoveRouteMonitor(routeMonitorNamespace, routeMonitorName)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})
