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
	prometheusop "github.com/prometheus-operator/prometheus-operator/pkg/client/versioned"
	corev1 "k8s.io/api/core/v1"
	kubev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("Route Monitor Operator", Ordered, func() {
	var (
		k8s      *openshift.Client
		err      error
		pod      *corev1.Pod
		svc      *corev1.Service
		appRoute *routev1.Route
		rmo      *routemonitorv1alpha1.RouteMonitor
	)
	const (
		operatorNamespace            = "openshift-route-monitor-operator"
		deploymentName               = "route-monitor-operator-controller-manager"
		operatorName                 = "route-monitor-operator"
		consoleName                  = "console"
		pollingDuration              = 10 * time.Minute
		routeMonitorName             = "routemonitor-e2e-test"
		defaultDesiredReplicas int32 = 1
	)
	BeforeAll(func(ctx context.Context) {
		log.SetLogger(GinkgoLogr)
		k8s, err = openshift.New(GinkgoLogr)
		Expect(err).ShouldNot(HaveOccurred(), "unable to setup k8s client")
		Expect(routemonitorv1alpha1.AddToScheme(k8s.GetScheme())).Should(Succeed(), "unable to register RouteMonitor scheme")
	})

	AfterAll(func(ctx context.Context) {
		By("Deleting test pod " + routeMonitorName)
		Expect(k8s.Delete(ctx, pod)).Should(Succeed(), "Failed to delete namespace")
		By("Deleting test service " + routeMonitorName)
		Expect(k8s.Delete(ctx, svc)).Should(Succeed(), "Failed to delete service")
		By("Deleting test route " + routeMonitorName)
		if appRoute != nil {
			Expect(k8s.Delete(ctx, appRoute)).Should(Succeed(), "Failed to delete route")
		} else {
			Expect(err).ShouldNot(HaveOccurred(), "appRoute is nil")
		}
	})

	It("is installed", func(ctx context.Context) {
		By("checking the deployment exists and is available")
		EventuallyDeployment(ctx, k8s, deploymentName, operatorNamespace).Should(BeAvailable())
	})

	It("can be upgraded", func(ctx context.Context) {
		By("forcing operator upgrade")
		err = k8s.UpgradeOperator(ctx, operatorName, operatorNamespace)
		Expect(err).NotTo(HaveOccurred(), "operator upgrade failed")
	})

	It("has all of the required resources", func(ctx context.Context) {
		promclient, err := prometheusop.NewForConfig(k8s.GetConfig())
		Expect(err).ShouldNot(HaveOccurred(), "failed to configure Prometheus-operator clientset")
		_, err = promclient.MonitoringV1().ServiceMonitors(operatorNamespace).Get(ctx, consoleName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred(), "Could not get console serviceMonitor")
		_, err = promclient.MonitoringV1().PrometheusRules(operatorNamespace).Get(ctx, consoleName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred(), "Could not get console prometheusRule")
	})

	It("Creates and deletes a RouteMonitor to see if it works accordingly", func(ctx context.Context) {
		By("Creating a pod, service and route to monitor with a ServiceMonitor and PrometheusRule")
		clientset, err := kubernetes.NewForConfig(k8s.GetConfig())
		Expect(err).ShouldNot(HaveOccurred(), "failed to configure Kubernetes clientset")

		By("Creating the test pod")
		pod = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      routeMonitorName,
				Namespace: operatorNamespace,
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "test",
						Image: "quay.io/jitesoft/nginx:mainline",
					},
				},
			},
		}
		_, err = clientset.CoreV1().Pods(operatorNamespace).Create(ctx, pod, metav1.CreateOptions{})
		Expect(err).ShouldNot(HaveOccurred(), "Could not create a pod")

		By("Checking the pod state")
		var phase kubev1.PodPhase
		for i := 0; i < 6; i++ {
			pod, err = clientset.CoreV1().Pods(pod.Namespace).Get(ctx, pod.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred(), "Couldn't get the testing pod")
			if pod != nil {
				phase = pod.Status.Phase
				// stop checking if Pod has reached state or failed
				if phase == kubev1.PodRunning || phase == kubev1.PodFailed {
					break
				}
			}
			GinkgoLogr.Info(fmt.Sprintf("Waiting for Pod '%s/%s' to be %s, currently %s...", pod.Namespace, pod.Name, kubev1.PodRunning, phase))
			time.Sleep(time.Second * 15)
		}
		Expect(phase).To(Equal(kubev1.PodRunning), fmt.Sprintf("pod %s in ns %s is not running, current pod state is %v", routeMonitorName, operatorNamespace, phase))

		By("Creating the service")
		svc = &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      routeMonitorName,
				Namespace: operatorNamespace,
				Labels:    map[string]string{routeMonitorName: routeMonitorName},
			},
			TypeMeta: metav1.TypeMeta{
				Kind:       "Service",
				APIVersion: "v1",
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
		// err = k8s.Create(ctx, svc)
		_, err = clientset.CoreV1().Services(operatorNamespace).Create(ctx, svc, metav1.CreateOptions{})
		Expect(err).ShouldNot(HaveOccurred(), "Unable to create service %s/%s", svc.Namespace, svc.Name)

		By("Checking the route")
		appRoute := &routev1.Route{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: operatorNamespace,
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

		By("Creating the route")
		err = k8s.Create(ctx, appRoute)
		Expect(err).ShouldNot(HaveOccurred(), "Unable to create route %s/%s", appRoute.Namespace, appRoute.Name)

		By("Creating a sample RouteMonitor to monitor the service")
		rmo = &routemonitorv1alpha1.RouteMonitor{
			TypeMeta: metav1.TypeMeta{
				Kind:       "RouteMonitor",
				APIVersion: "monitoring.openshift.io/v1alpha1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: operatorNamespace,
				Name:      routeMonitorName,
			},
			Spec: routemonitorv1alpha1.RouteMonitorSpec{
				Slo: routemonitorv1alpha1.SloSpec{
					TargetAvailabilityPercent: "99.95",
				},
				Route: routemonitorv1alpha1.RouteMonitorRouteSpec{
					Namespace: operatorNamespace,
					Name:      routeMonitorName,
				},
			},
		}

		err = k8s.Create(ctx, rmo)
		Expect(err).ShouldNot(HaveOccurred(), "Unable to create RouteMonitor %s/%s", rmo.Namespace, rmo.Name)

		promclient, err := prometheusop.NewForConfig(k8s.GetConfig())
		Expect(err).ShouldNot(HaveOccurred(), "failed to configure Prometheus-operator clientset")

		err = wait.PollUntilContextTimeout(ctx, 15*time.Second, pollingDuration, false, func(ctx context.Context) (bool, error) {
			_, err = promclient.MonitoringV1().ServiceMonitors(operatorNamespace).Get(ctx, routeMonitorName, metav1.GetOptions{})
			if k8serrors.IsNotFound(err) {
				return false, nil // Continue polling if resource is not found
			}
			if err != nil {
				return false, err // Return error to stop polling if other errors occur
			}

			_, err = promclient.MonitoringV1().PrometheusRules(operatorNamespace).Get(ctx, routeMonitorName, metav1.GetOptions{})
			if k8serrors.IsNotFound(err) {
				return false, nil // Continue polling if resource is not found
			}
			if err != nil {
				return false, err // Return error to stop polling if other errors occur
			}
			return true, nil // Stop polling if both resources are found
		})
		Expect(err).NotTo(HaveOccurred(), "dependant resources weren't created via RouteMonitor")

		By("Deleting the sample RouteMonitor")
		err = k8s.Delete(ctx, rmo)
		Expect(err).NotTo(HaveOccurred(), "failed to delete RouteMonitor")

		// Deleting a namespace can take a while. If desired, wait for the namespace to delete before returning.
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
		Expect(err).NotTo(HaveOccurred(), "Couldn't delete application route monitor")

		_, err = promclient.MonitoringV1().ServiceMonitors(operatorNamespace).Get(ctx, routeMonitorName, metav1.GetOptions{})
		Expect(k8serrors.IsNotFound(err)).To(BeTrue(), "sample serviceMonitor still exists, deletion of RouteMonitor didn't clean it up")
		_, err = promclient.MonitoringV1().PrometheusRules(operatorNamespace).Get(ctx, routeMonitorName, metav1.GetOptions{})
		Expect(k8serrors.IsNotFound(err)).To(BeTrue(), "sample prometheusRule still exists, deletion of RouteMonitor didn't clean it up")
	})
})
