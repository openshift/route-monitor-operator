package int_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	. "github.com/openshift/route-monitor-operator/int"
	"github.com/openshift/route-monitor-operator/pkg/util/templates"
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
			err := i.RemoveClusterUrlMonitor(clusterUrlMonitorNamespace, clusterUrlMonitorName)
			clusterUrlMonitorName = "fake-url-monitor"
			clusterUrlMonitorNamespace = "default"
			expectedServiceMonitorName = templates.TemplateForServiceMonitorName(clusterUrlMonitorNamespace, clusterUrlMonitorName)
			Expect(err).NotTo(HaveOccurred())
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
				expectedUrl := clusterUrlMonitor.Spec.Prefix + clusterConfig.Spec.Domain + ":" + clusterUrlMonitor.Spec.Port + clusterUrlMonitor.Spec.Suffix
				Expect(len(serviceMonitor.Spec.Endpoints)).To(Equal(1))
				Expect(len(serviceMonitor.Spec.Endpoints[0].Params["target"])).To(Equal(1))
				Expect(serviceMonitor.Spec.Endpoints[0].Params["target"][0]).To(Equal(expectedUrl))
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
			routeMonitorName           string
			routeMonitorNamespace      string
			routeMonitor               v1alpha1.RouteMonitor
			expectedServiceMonitorName types.NamespacedName
		)
		BeforeEach(func() {
			err := i.RemoveRouteMonitor(routeMonitorNamespace, routeMonitorName)
			routeMonitorName = "fake-route-monitor"
			routeMonitorNamespace = "default"
			expectedServiceMonitorName = templates.TemplateForServiceMonitorName(routeMonitorNamespace, routeMonitorName)
			Expect(err).NotTo(HaveOccurred())
			routeMonitor = v1alpha1.RouteMonitor{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: routeMonitorNamespace,
					Name:      routeMonitorName,
				},
				Spec: v1alpha1.RouteMonitorSpec{
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
			It("creates a RouteMonitor within 20 seconds", func() {
				err := i.Client.Create(context.TODO(), &routeMonitor)
				Expect(err).NotTo(HaveOccurred())

				serviceMonitor, err := i.WaitForServiceMonitor(expectedServiceMonitorName, 20)
				Expect(err).NotTo(HaveOccurred())

				Expect(serviceMonitor.Name).To(Equal(expectedServiceMonitorName.Name))
				Expect(serviceMonitor.Namespace).To(Equal(expectedServiceMonitorName.Namespace))
			})
		})

		When("the ClusterUrlMonitor is deleted", func() {
			BeforeEach(func() {
				err := i.Client.Create(context.TODO(), &routeMonitor)
				Expect(err).NotTo(HaveOccurred())

				_, err = i.WaitForServiceMonitor(expectedServiceMonitorName, 20)
				Expect(err).NotTo(HaveOccurred())

				err = i.RemoveRouteMonitor(routeMonitorNamespace, routeMonitorName)
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
})
