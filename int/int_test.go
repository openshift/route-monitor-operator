package int_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	configv1 "github.com/openshift/api/config/v1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	//nolint:typecheck // this import is used for general prettines, and currently kept
	. "github.com/openshift/route-monitor-operator/int"
	customerrors "github.com/openshift/route-monitor-operator/pkg/util/errors"
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
					Slo: v1alpha1.SloSpec{
						TargetAvailabilityPercent: "99.95",
					},
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

				prometheusRule, err := i.WaitForPrometheusRule(expectedServiceMonitorName, 20)
				Expect(err).NotTo(HaveOccurred())
				Expect(prometheusRule.Name).To(Equal(expectedServiceMonitorName.Name))
				Expect(prometheusRule.Namespace).To(Equal(expectedServiceMonitorName.Namespace))

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

				Expect(updatedClusterUrlMonitor.Status.PrometheusRuleRef.Name).To(Equal(prometheusRule.Name))
				Expect(updatedClusterUrlMonitor.Status.PrometheusRuleRef.Namespace).To(Equal(prometheusRule.Namespace))

				Expect(updatedClusterUrlMonitor.Status.ServiceMonitorRef.Name).To(Equal(serviceMonitor.Name))
				Expect(updatedClusterUrlMonitor.Status.ServiceMonitorRef.Namespace).To(Equal(serviceMonitor.Namespace))
			})
		})

		When("the ClusterUrlMonitor spec has an incorrect value", func() {
			It("doesn't create a prometheusrule with error but does create a servicemonitor", func() {
				clusterUrlMonitor.Spec.Slo.TargetAvailabilityPercent = "100"
				err := i.Client.Create(context.TODO(), &clusterUrlMonitor)
				Expect(err).NotTo(HaveOccurred())

				serviceMonitor, err := i.WaitForServiceMonitor(expectedServiceMonitorName, 20)
				Expect(err).NotTo(HaveOccurred())
				Expect(serviceMonitor.Name).To(Equal(expectedServiceMonitorName.Name))
				Expect(serviceMonitor.Namespace).To(Equal(expectedServiceMonitorName.Namespace))

				err = i.WaitForPrometheusRuleToClear(expectedServiceMonitorName, 20)
				Expect(err).NotTo(HaveOccurred())

				updatedClusterUrlMonitor := v1alpha1.ClusterUrlMonitor{}
				err = i.Client.Get(context.TODO(), types.NamespacedName{Namespace: clusterUrlMonitorNamespace, Name: clusterUrlMonitorName}, &updatedClusterUrlMonitor)
				Expect(err).NotTo(HaveOccurred())

				Expect(updatedClusterUrlMonitor.Status.ErrorStatus).To(Equal(customerrors.InvalidSLO.Error()))
			})
		})

		When("A ClusterUrlMonitor with SLO is created, but rolled back later", func() {
			BeforeEach(func() {
				err := i.Client.Create(context.TODO(), &clusterUrlMonitor)
				Expect(err).NotTo(HaveOccurred())

				_, err = i.WaitForServiceMonitor(expectedServiceMonitorName, 20)
				Expect(err).NotTo(HaveOccurred())

				_, err = i.WaitForPrometheusRule(expectedServiceMonitorName, 20)
				Expect(err).NotTo(HaveOccurred())
			})

			It("removes the corresponding PrometheusRule", func() {
				latestClusterUrlMonitor, err := i.ClusterUrlMonitorWaitForPrometheusRuleRef(expectedServiceMonitorName, 20)
				Expect(err).NotTo(HaveOccurred())
				latestClusterUrlMonitor.Spec.Slo.TargetAvailabilityPercent = ""
				err = i.Client.Update(context.TODO(), &latestClusterUrlMonitor)
				Expect(err).NotTo(HaveOccurred())
				err = i.WaitForPrometheusRuleToClear(expectedServiceMonitorName, 20)
				Expect(err).NotTo(HaveOccurred())

				updatedClusterUrlMonitor := v1alpha1.ClusterUrlMonitor{}
				err = i.Client.Get(context.TODO(), types.NamespacedName{Namespace: clusterUrlMonitorNamespace, Name: clusterUrlMonitorName}, &updatedClusterUrlMonitor)
				Expect(err).NotTo(HaveOccurred())
				Expect(updatedClusterUrlMonitor.Status.PrometheusRuleRef.Name).To(Equal(""))
				Expect(updatedClusterUrlMonitor.Status.PrometheusRuleRef.Namespace).To(Equal(""))

			})
		})

		When("A ClusterUrlMonitor with SLO is created, but the SLO is changed later", func() {
			BeforeEach(func() {
				err := i.Client.Create(context.TODO(), &clusterUrlMonitor)
				Expect(err).NotTo(HaveOccurred())

				_, err = i.WaitForServiceMonitor(expectedServiceMonitorName, 20)
				Expect(err).NotTo(HaveOccurred())

				_, err = i.WaitForPrometheusRule(expectedServiceMonitorName, 20)
				Expect(err).NotTo(HaveOccurred())
			})

			It("changes the corresponding PrometheusRule", func() {
				latestClusterUrlMonitor, err := i.ClusterUrlMonitorWaitForPrometheusRuleRef(expectedServiceMonitorName, 20)
				Expect(err).NotTo(HaveOccurred())
				latestClusterUrlMonitor.Spec.Slo.TargetAvailabilityPercent = "99.5"
				err = i.Client.Update(context.TODO(), &latestClusterUrlMonitor)
				Expect(err).NotTo(HaveOccurred())
				_, parsedSlo := latestClusterUrlMonitor.Spec.Slo.IsValid()

				clusterConfig := configv1.Ingress{}
				err = i.Client.Get(context.TODO(), types.NamespacedName{Name: "cluster"}, &clusterConfig)
				Expect(err).NotTo(HaveOccurred())
				spec := clusterUrlMonitor.Spec
				expectedUrl := spec.Prefix + clusterConfig.Spec.Domain + ":" + spec.Port + spec.Suffix
				err = i.ClusterUrlMonitorWaitForPrometheusRuleCorrectSLO(expectedServiceMonitorName, parsedSlo, 20, expectedUrl, "ClusterUrlMonitor")
				Expect(err).NotTo(HaveOccurred())
			})
		})

		When("the ClusterUrlMonitor Slo changes", func() {
			It("creates/deletes a PrometheusRule as necessary", func() {
				By("creating a ClusterUrlMonitor with no SLO")
				clusterUrlMonitor.Spec.Slo = v1alpha1.SloSpec{}
				err := i.Client.Create(context.TODO(), &clusterUrlMonitor)
				Expect(err).NotTo(HaveOccurred())

				_, err = i.WaitForServiceMonitor(expectedServiceMonitorName, 20)
				Expect(err).NotTo(HaveOccurred())
				err = i.WaitForPrometheusRuleToClear(expectedServiceMonitorName, 20)
				Expect(err).NotTo(HaveOccurred())

				By("adding a SLO")
				err = i.Client.Get(context.TODO(), expectedServiceMonitorName, &clusterUrlMonitor)
				Expect(err).NotTo(HaveOccurred())
				clusterUrlMonitor.Spec.Slo.TargetAvailabilityPercent = "99.5"
				err = i.Client.Update(context.TODO(), &clusterUrlMonitor)
				Expect(err).NotTo(HaveOccurred())

				_, err = i.WaitForPrometheusRule(expectedServiceMonitorName, 20)
				Expect(err).NotTo(HaveOccurred())

				By("removing the SLO again")
				err = i.Client.Get(context.TODO(), expectedServiceMonitorName, &clusterUrlMonitor)
				Expect(err).NotTo(HaveOccurred())
				clusterUrlMonitor.Spec.Slo = v1alpha1.SloSpec{}
				err = i.Client.Update(context.TODO(), &clusterUrlMonitor)
				Expect(err).NotTo(HaveOccurred())

				err = i.WaitForPrometheusRuleToClear(expectedServiceMonitorName, 20)
				Expect(err).NotTo(HaveOccurred())

				err = i.RemoveClusterUrlMonitor(clusterUrlMonitorNamespace, clusterUrlMonitorName)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		When("the ClusterUrlMonitor is deleted", func() {
			BeforeEach(func() {
				err := i.Client.Create(context.TODO(), &clusterUrlMonitor)
				Expect(err).NotTo(HaveOccurred())

				_, err = i.WaitForServiceMonitor(expectedServiceMonitorName, 20)
				Expect(err).NotTo(HaveOccurred())

				_, err = i.WaitForPrometheusRule(expectedServiceMonitorName, 20)
				Expect(err).NotTo(HaveOccurred())

				err = i.RemoveClusterUrlMonitor(clusterUrlMonitorNamespace, clusterUrlMonitorName)
				Expect(err).NotTo(HaveOccurred())
			})

			It("removes the ServiceMonitor as well within 20 seconds", func() {
				serviceMonitor := monitoringv1.ServiceMonitor{}
				err := i.Client.Get(context.TODO(), expectedServiceMonitorName, &serviceMonitor)
				Expect(err).To(HaveOccurred())
				Expect(errors.IsNotFound(err)).To(BeTrue())

				prometheusRule := monitoringv1.PrometheusRule{}
				err = i.Client.Get(context.TODO(), expectedServiceMonitorName, &prometheusRule)
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
						TargetAvailabilityPercent: "99.95",
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

		When("the RouteMonitor spec has an incorrect value", func() {
			It("doesn't create a prometheusrule with error but does create a servicemonitor", func() {
				// This is a very available target XP
				routeMonitor.Spec.Slo.TargetAvailabilityPercent = "100"
				err := i.Client.Create(context.TODO(), &routeMonitor)
				Expect(err).NotTo(HaveOccurred())

				serviceMonitor, err := i.WaitForServiceMonitor(expectedDependentResource, 20)
				Expect(err).NotTo(HaveOccurred())
				Expect(serviceMonitor.Name).To(Equal(expectedDependentResource.Name))
				Expect(serviceMonitor.Namespace).To(Equal(expectedDependentResource.Namespace))

				err = i.WaitForPrometheusRuleToClear(expectedDependentResource, 20)
				Expect(err).NotTo(HaveOccurred())

				updatedRouteMonitor := v1alpha1.RouteMonitor{}
				err = i.Client.Get(context.TODO(), types.NamespacedName{Namespace: routeMonitorNamespace, Name: routeMonitorName}, &updatedRouteMonitor)
				Expect(err).NotTo(HaveOccurred())

				Expect(updatedRouteMonitor.Status.ErrorStatus).To(Equal(customerrors.InvalidSLO.Error()))
			})
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
		When("A RouteMonitor with SLO is created, but rolled back later", func() {
			BeforeEach(func() {
				err := i.Client.Create(context.TODO(), &routeMonitor)
				Expect(err).NotTo(HaveOccurred())

				_, err = i.WaitForServiceMonitor(expectedDependentResource, 20)
				Expect(err).NotTo(HaveOccurred())

				_, err = i.WaitForPrometheusRule(expectedDependentResource, 20)
				Expect(err).NotTo(HaveOccurred())
			})

			It("removes the corresponding PrometheusRule", func() {
				latestRouteMonitor, err := i.RouteMonitorWaitForPrometheusRuleRef(expectedDependentResource, 20)
				Expect(err).NotTo(HaveOccurred())
				latestRouteMonitor.Spec.Slo.TargetAvailabilityPercent = ""
				err = i.Client.Update(context.TODO(), &latestRouteMonitor)
				Expect(err).NotTo(HaveOccurred())
				err = i.WaitForPrometheusRuleToClear(expectedDependentResource, 20)
				Expect(err).NotTo(HaveOccurred())

				updatedRouteMonitor := v1alpha1.RouteMonitor{}
				err = i.Client.Get(context.TODO(), types.NamespacedName{Namespace: routeMonitorNamespace, Name: routeMonitorName}, &updatedRouteMonitor)
				Expect(err).NotTo(HaveOccurred())
				Expect(updatedRouteMonitor.Status.PrometheusRuleRef.Name).To(Equal(""))
				Expect(updatedRouteMonitor.Status.PrometheusRuleRef.Namespace).To(Equal(""))

			})
		})

		When("A RouteMonitor with SLO is created, but the SLO is changed later", func() {
			BeforeEach(func() {
				err := i.Client.Create(context.TODO(), &routeMonitor)
				Expect(err).NotTo(HaveOccurred())

				_, err = i.WaitForServiceMonitor(expectedDependentResource, 20)
				Expect(err).NotTo(HaveOccurred())

				_, err = i.WaitForPrometheusRule(expectedDependentResource, 20)
				Expect(err).NotTo(HaveOccurred())
			})

			It("changes the corresponding PrometheusRule", func() {
				latestRouteMonitor, err := i.RouteMonitorWaitForPrometheusRuleRef(expectedDependentResource, 20)
				Expect(err).NotTo(HaveOccurred())
				latestRouteMonitor.Spec.Slo.TargetAvailabilityPercent = "99.5"
				err = i.Client.Update(context.TODO(), &latestRouteMonitor)
				Expect(err).NotTo(HaveOccurred())
				_, parsedSlo := latestRouteMonitor.Spec.Slo.IsValid()
				err = i.RouteMonitorWaitForPrometheusRuleCorrectSLO(expectedDependentResource, parsedSlo, 20, routeMonitor.Kind)
				err = i.WaitForPrometheusRuleCorrectSLO(expectedDependentResource, parsedSlo, 20)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		When("the RouteMonitor Slo changes", func() {
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
				routeMonitor.Spec.Slo.TargetAvailabilityPercent = "99.5"
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
