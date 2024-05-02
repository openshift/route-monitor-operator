// DO NOT REMOVE TAGS BELOW. IF ANY NEW TEST FILES ARE CREATED UNDER /osde2e, PLEASE ADD THESE TAGS TO THEM IN ORDER TO BE EXCLUDED FROM UNIT TESTS. //go:build osde2e
//go:build osde2e
// +build osde2e

package osde2etests

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openshift/osde2e-common/pkg/clients/openshift"
	. "github.com/openshift/osde2e-common/pkg/gomega/assertions"
	. "github.com/openshift/osde2e-common/pkg/gomega/matchers"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	prometheusop "github.com/prometheus-operator/prometheus-operator/pkg/client/versioned"
)

var _ = Describe("Route Monitor Operator", Ordered, func() {
	var (
		k8s               *openshift.Client
		operatorNamespace = "openshift-route-monitor-operator"
		deploymentName    = "route-monitor-operator-controller-manager"
		operatorName      = "route-monitor-operator"
		consoleNamespace  = operatorNamespace
		consoleName       = "console"
	)
	const (
		defaultDesiredReplicas int32 = 1
	)
	BeforeAll(func(ctx context.Context) {
		log.SetLogger(GinkgoLogr)
		var err error
		k8s, err = openshift.New(GinkgoLogr)
		Expect(err).ShouldNot(HaveOccurred(), "unable to setup k8s client")
	})

	It("is installed", func(ctx context.Context) {
		By("checking the deployment exists and is available")
		EventuallyDeployment(ctx, k8s, deploymentName, operatorNamespace).Should(BeAvailable())
	})

	It("can be upgraded", func(ctx context.Context) {
		By("forcing operator upgrade")
		err := k8s.UpgradeOperator(ctx, operatorName, operatorNamespace)
		Expect(err).NotTo(HaveOccurred(), "operator upgrade failed")
	})

	Context("rmo Route Monitor Operator regression for console", func() {
		It("has all of the required resources", func(ctx context.Context) {
			promclient, err := prometheusop.NewForConfig(k8s.GetConfig())
			Expect(err).ShouldNot(HaveOccurred(), "failed to configure Prometheus-operator clientset")

			_, err = promclient.MonitoringV1().ServiceMonitors(consoleNamespace).Get(ctx, consoleName, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred(), "Could not get console serviceMonitor")
			_, err = promclient.MonitoringV1().PrometheusRules(consoleNamespace).Get(ctx, consoleName, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred(), "Could not get console prometheusRule")
		})
	})

	// TODO: implement testRouteMonitorCreationWorks
	/* Context("rmo Route Monitor Operator integration test", func(ctx context.Context) {
		pollingDuration := 10 * time.Minute
		It("Creates and deletes a RouteMonitor to see if it works accordingly", func(ctx context.Context) {
			routeMonitorNamespace := "route-monitor-operator"
			const routeMonitorName = "routemonitor-e2e-test"

			By("Creating a pod, service, and route to monitor with a ServiceMonitor and PrometheusRule")
			// Create Pod
			pod := createSamplePod(routeMonitorName, routeMonitorNamespace)
			err := k8s.Create(ctx, &pod)
			Expect(err).NotTo(HaveOccurred(), "Couldn't create a testing pod")

			// Wait for Pod to be running
			err = waitForPodRunning(ctx, routeMonitorNamespace, routeMonitorName)
			Expect(err).NotTo(HaveOccurred(), "Pod is not running")

			// Create Service
			svc := createSampleService(routeMonitorName, routeMonitorNamespace)
			err = k8s.Create(ctx, &svc)
			Expect(err).NotTo(HaveOccurred(), "Couldn't create a testing service")

			// Create Route
			appRoute := createSampleRoute(routeMonitorName, routeMonitorNamespace)
			err = k8s.Create(ctx, &appRoute)
			Expect(err).NotTo(HaveOccurred(), "Couldn't create application route")

			Eventually(func() bool {
				_, err := k8s.CoreV1().Services(routeMonitorNamespace).Get(ctx, routeMonitorName, metav1.GetOptions{})
				if err != nil {
					return false
				}
				return true
			}, pollingDuration, time.Second).Should(BeTrue(), "Failed to verify that resources were created")

			By("Deleting the sample RouteMonitor")
			err := k8s.CoreV1().Services(routeMonitorNamespace).Delete(ctx, routeMonitorName, metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred(), "Couldn't delete the service")

			Eventually(func() bool {
				_, err := k8s.CoreV1().Services(routeMonitorNamespace).Get(ctx, routeMonitorName, metav1.GetOptions{})
				return err != nil // Expect an error since the resource should not exist
			}, pollingDuration, time.Second).Should(BeTrue(), "Service still exists after deletion")
		})
	}) */
})
