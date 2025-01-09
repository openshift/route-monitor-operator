// DO NOT REMOVE TAGS BELOW. IF ANY NEW TEST FILES ARE CREATED UNDER /osde2e, PLEASE ADD THESE TAGS TO THEM IN ORDER TO BE EXCLUDED FROM UNIT TESTS. //go:build osde2e
//go:build osde2e
// +build osde2e

package osde2etests

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	routev1 "github.com/openshift/api/route/v1"
	"github.com/openshift/osde2e-common/pkg/clients/openshift"
	. "github.com/openshift/osde2e-common/pkg/gomega/assertions"
	. "github.com/openshift/osde2e-common/pkg/gomega/matchers"
	routemonitorv1alpha1 "github.com/openshift/route-monitor-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	kubev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("Route Monitor Operator", Ordered, func() {
	var (
		k8s                   *openshift.Client
		serviceMonitorsClient dynamic.ResourceInterface
		prometheusRulesClient dynamic.ResourceInterface
	)
	const (
		namespace        = "openshift-route-monitor-operator"
		deploymentName   = "route-monitor-operator-controller-manager"
		operatorName     = "route-monitor-operator"
		consoleName      = "console"
		pollingDuration  = 10 * time.Minute
		routeMonitorName = "routemonitor-e2e-test"
	)
	BeforeAll(func(ctx context.Context) {
		log.SetLogger(GinkgoLogr)
		var err error
		k8s, err = openshift.New(GinkgoLogr)
		Expect(err).ShouldNot(HaveOccurred(), "unable to setup k8s client")
		dynamicClient, err := dynamic.NewForConfig(k8s.GetConfig())
		Expect(err).ShouldNot(HaveOccurred(), "failed creating the dynamic client: %w", err)
		Expect(routemonitorv1alpha1.AddToScheme(k8s.GetScheme())).Should(Succeed(), "unable to register RouteMonitor scheme")
		serviceMonitorsClient = dynamicClient.Resource(schema.GroupVersionResource{
			Group:    "monitoring.openshift.io",
			Version:  "v1alpha1",
			Resource: "routemonitors",
		}).Namespace(namespace)
		prometheusRulesClient = dynamicClient.Resource(schema.GroupVersionResource{
			Group:    "monitoring.coreos.com",
			Version:  "v1",
			Resource: "prometheusrules",
		}).Namespace(namespace)
	})

	It("is installed", func(ctx context.Context) {
		By("checking the namespace exists")
		err := k8s.Get(ctx, namespace, "", &corev1.Namespace{})
		Expect(err).ShouldNot(HaveOccurred(), "namespace %s not found", namespace)

		By("checking the deployment exists and is available")
		EventuallyDeployment(ctx, k8s, deploymentName, namespace).Should(BeAvailable())
	})

	It("can be upgraded", func(ctx context.Context) {
		By("forcing operator upgrade")
		err := k8s.UpgradeOperator(ctx, operatorName, namespace)
		Expect(err).ShouldNot(HaveOccurred(), "operator upgrade failed")
	})

	It("has all of the required resources", func(ctx context.Context) {
		_, err := serviceMonitorsClient.Get(ctx, consoleName, metav1.GetOptions{})
		Expect(err).ShouldNot(HaveOccurred(), "Unable to get console serviceMonitor")

		_, err = prometheusRulesClient.Get(ctx, consoleName, metav1.GetOptions{})
		Expect(err).ShouldNot(HaveOccurred(), "Unable to get console prometheusRule")
	})

	It("required dependent resources are created", func(ctx context.Context) {
		By("Creating a pod, service and route to monitor with a ServiceMonitor and PrometheusRule")
		By("Creating the test pod")
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      routeMonitorName,
				Namespace: namespace,
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "test",
						Image: "quay.io/jitesoft/nginx:mainline",
						SecurityContext: &corev1.SecurityContext{
                    					AllowPrivilegeEscalation: pointer.BoolPtr(false),
                    					Capabilities: &corev1.Capabilities{
                        					Drop: []corev1.Capability{"ALL"},
                   				 	},
                    					RunAsNonRoot: pointer.BoolPtr(true),
                    					SeccompProfile: &corev1.SeccompProfile{
                        					Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					},
				},
			},
		}
		err := k8s.Create(ctx, pod)
		Expect(err).ShouldNot(HaveOccurred(), "Unable to create a pod")

		By("Checking the pod state")
		var phase kubev1.PodPhase
		err = wait.PollUntilContextTimeout(ctx, 15*time.Second, 2*time.Minute, false, func(ctx context.Context) (bool, error) {
			err := k8s.Get(ctx, pod.Name, pod.Namespace, pod)
			Expect(err).ShouldNot(HaveOccurred(), "Unable to get the testing pod")
			if pod != nil {
				phase = pod.Status.Phase
				if phase == kubev1.PodRunning || phase == kubev1.PodFailed {
					return true, nil
				}
			}
			GinkgoLogr.Info(fmt.Sprintf("Waiting for Pod '%s/%s' to be %s, currently %s...", pod.Namespace, pod.Name, kubev1.PodRunning, phase))
			return phase == kubev1.PodRunning, nil
		})
		Expect(err).ShouldNot(HaveOccurred(), fmt.Sprintf("Pod %s in ns %s is not running, current pod state is %v", routeMonitorName, namespace, phase))

		By("Creating the service")
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      routeMonitorName,
				Namespace: namespace,
				Labels:    map[string]string{routeMonitorName: routeMonitorName},
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{
						Port:       8080,
						Protocol:   corev1.ProtocolTCP,
						TargetPort: intstr.FromInt(80),
						Name:       "web",
					},
				},
				Selector: map[string]string{routeMonitorName: routeMonitorName},
			},
		}

		err = k8s.Create(ctx, svc)
		Expect(err).ShouldNot(HaveOccurred(), "Unable to create service %s/%s", svc.Namespace, svc.Name)

		By("Creating the route")
		appRoute := &routev1.Route{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      routeMonitorName,
			},
			Spec: routev1.RouteSpec{
				To: routev1.RouteTargetReference{
					Name: routeMonitorName,
				},
				TLS: &routev1.TLSConfig{Termination: "edge"},
			},
			Status: routev1.RouteStatus{},
		}

		err = k8s.Create(ctx, appRoute)
		Expect(err).ShouldNot(HaveOccurred(), "Unable to create route %s/%s", appRoute.Namespace, appRoute.Name)

		By("Creating a sample RouteMonitor to monitor the service")
		rmo := &routemonitorv1alpha1.RouteMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      routeMonitorName,
			},
			Spec: routemonitorv1alpha1.RouteMonitorSpec{
				Slo: routemonitorv1alpha1.SloSpec{
					TargetAvailabilityPercent: "99.95",
				},
				Route: routemonitorv1alpha1.RouteMonitorRouteSpec{
					Namespace: namespace,
					Name:      routeMonitorName,
				},
			},
		}

		err = k8s.Create(ctx, rmo)
		Expect(err).ShouldNot(HaveOccurred(), "Unable to create RouteMonitor %s/%s", rmo.Namespace, rmo.Name)

		By("Checking the dependent resources are created")
		err = wait.PollUntilContextTimeout(ctx, 15*time.Second, pollingDuration, false, func(ctx context.Context) (bool, error) {
			_, err = serviceMonitorsClient.Get(ctx, routeMonitorName, metav1.GetOptions{})
			if k8serrors.IsNotFound(err) {
				return false, err // Continue polling if resource is not found
			}
			if err != nil {
				return false, err // Return error to stop polling if other errors occur
			}

			_, err = prometheusRulesClient.Get(ctx, routeMonitorName, metav1.GetOptions{})
			if k8serrors.IsNotFound(err) {
				return false, err
			}
			if err != nil {
				return false, err
			}
			return true, nil
		})
		Expect(err).ShouldNot(HaveOccurred(), "Dependant resources weren't created via RouteMonitor")

		By("Deleting the sample RouteMonitor")
		err = k8s.Delete(ctx, rmo)
		Expect(err).ShouldNot(HaveOccurred(), "Failed to delete RouteMonitor")

		// wait for the routemonitor to delete before returning.
		err = wait.PollUntilContextTimeout(ctx, 2*time.Second, 1*time.Minute, false, func(ctx context.Context) (bool, error) {
			err = k8s.Get(ctx, rmo.Name, rmo.Namespace, rmo)
			if k8serrors.IsNotFound(err) {
				return true, nil // RouteMonitor is deleted
			}
			if err != nil {
				return false, err // Return error to stop retrying
			}
			return false, nil // RouteMonitor still exists, continue polling
		})
		Expect(err).ShouldNot(HaveOccurred(), "Couldn't delete application route monitor")

		_, err = serviceMonitorsClient.Get(ctx, routeMonitorName, metav1.GetOptions{})
		Expect(k8serrors.IsNotFound(err)).To(BeTrue(), "Sample serviceMonitor still exists, deletion of RouteMonitor didn't clean it up")
		_, err = prometheusRulesClient.Get(ctx, routeMonitorName, metav1.GetOptions{})
		Expect(k8serrors.IsNotFound(err)).To(BeTrue(), "Sample prometheusRule still exists, deletion of RouteMonitor didn't clean it up")

		DeferCleanup(func(ctx context.Context) {
			By("Cleaning up setup")
			By("Deleting test pod " + routeMonitorName)
			Expect(k8s.Delete(ctx, pod)).Should(Succeed(), "Failed to delete pod")
			By("Deleting test service " + routeMonitorName)
			Expect(k8s.Delete(ctx, svc)).Should(Succeed(), "Failed to delete service")
			By("Deleting test route " + routeMonitorName)
			Expect(k8s.Delete(ctx, appRoute)).Should(Succeed(), "Failed to delete route")
		})
	})
})
