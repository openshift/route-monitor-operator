// DO NOT REMOVE TAGS BELOW. IF ANY NEW TEST FILES ARE CREATED UNDER /osde2e, PLEASE ADD THESE TAGS TO THEM IN ORDER TO BE EXCLUDED FROM UNIT TESTS. //go:build osde2e
//go:build osde2e
// +build osde2e

package osde2etests

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openshift/osde2e-common/pkg/clients/openshift"
	"github.com/openshift/osde2e-common/pkg/clients/prometheus"
	. "github.com/openshift/osde2e-common/pkg/gomega/assertions"
	. "github.com/openshift/osde2e-common/pkg/gomega/matchers"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var routeMonitorOperatorTestName string = "[Suite: informing] [OSD] Route Monitor Operator (rmo)"

var _ = Describe(routeMonitorOperatorTestName, Ordered, func() {
	var (
		k8s               *openshift.Client
		prom              *prometheus.Client
		operatorNamespace = "openshift-route-monitor-operator"
		serviceName       = "route-monitor-operator-registry"
		deploymentName    = "route-monitor-operator-controller-manager"
		rolePrefix        = "route-monitor-operator"
		clusterRolePrefix = "route-monitor-operator"
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

		prom, err = prometheus.New(ctx, k8s)
		Expect(err).ShouldNot(HaveOccurred(), "unable to setup prometheus client")
	})

	It("is installed", func(ctx context.Context) {
		By("checking the namespace exists")
		err := k8s.Get(ctx, operatorNamespace, "", &corev1.Namespace{})
		Expect(err).ShouldNot(HaveOccurred(), "namespace %s not found", operatorNamespace)

		By("checking the role exists")
		var roles rbacv1.RoleList
		err = k8s.WithNamespace(operatorNamespace).List(ctx, &roles)
		Expect(err).ShouldNot(HaveOccurred(), "failed to list roles")
		Expect(&roles).Should(ContainItemWithPrefix(rolePrefix), "unable to find roles with prefix %s", rolePrefix)

		By("checking the rolebinding exists")
		var rolebindings rbacv1.RoleBindingList
		err = k8s.List(ctx, &rolebindings)
		Expect(err).ShouldNot(HaveOccurred(), "failed to list rolebindings")
		Expect(&rolebindings).Should(ContainItemWithPrefix(rolePrefix), "unable to find rolebindings with prefix %s", rolePrefix)

		By("checking the clusterrole exists")
		var clusterRoles rbacv1.ClusterRoleList
		err = k8s.List(ctx, &clusterRoles)
		Expect(err).ShouldNot(HaveOccurred(), "failed to list clusterroles")
		Expect(&clusterRoles).Should(ContainItemWithPrefix(clusterRolePrefix), "unable to find cluster role with prefix %s", clusterRolePrefix)

		By("checking the clusterrolebinding exists")
		var clusterRoleBindings rbacv1.ClusterRoleBindingList
		err = k8s.List(ctx, &clusterRoleBindings)
		Expect(err).ShouldNot(HaveOccurred(), "unable to list clusterrolebindings")
		Expect(&clusterRoleBindings).Should(ContainItemWithPrefix(clusterRolePrefix), "unable to find clusterrolebinding with prefix %s", clusterRolePrefix)

		By("checking the service exists")
		err = k8s.Get(ctx, serviceName, operatorNamespace, &corev1.Service{})
		Expect(err).ShouldNot(HaveOccurred(), "service %s/%s not found", operatorNamespace, serviceName)

		By("checking the deployment exists and is available")
		EventuallyDeployment(ctx, k8s, deploymentName, operatorNamespace).Should(BeAvailable())
	})

	It("can be upgraded", func(ctx context.Context) {
		By("forcing operator upgrade")
		err := k8s.UpgradeOperator(ctx, operatorName, operatorNamespace)
		Expect(err).NotTo(HaveOccurred(), "operator upgrade failed")
	})

	Context("rmo Route Monitor Operator regression for console", func() {
		It("has all of the required resources", func() {
			results, err := prom.InstantQuery(context.Background(), `up{job="route-monitor-operator"}`)
			Expect(err).NotTo(HaveOccurred(), "failed to query prometheus")
			Expect(results).NotTo(BeNil(), "No results for prometheus exporter")
			Expect(results.Len()).To(BeNumerically(">=", 0), "No metrics returned for the route-monitor-operator job")

			query := `count(up{namespace="` + consoleNamespace + `", service="` + consoleName + `"})`
			srvresult, err := prom.InstantQuery(context.Background(), query)
			Expect(err).NotTo(HaveOccurred(), "Could not get console serviceMonitor")
			Expect(srvresult).NotTo(BeNil(), "No results for ServiceMonitor query")
			Expect(srvresult.Len()).To(BeNumerically(">=", 0), "ServiceMonitor is not active")

			ruleQuery := `count(prometheus_rule_group_last_duration_seconds{namespace="` + consoleNamespace + `", rule_group="` + consoleName + `"})`
			ruleResult, err := prom.InstantQuery(context.Background(), ruleQuery)
			Expect(err).NotTo(HaveOccurred(), "Could not get console prometheusRule")
			Expect(ruleResult).NotTo(BeNil(), "No results for PrometheusRule query")
			Expect(ruleResult.Len()).To(BeNumerically(">=", 0), "PrometheusRule is not active")
		})
	})
	/*
			// TODO: implement testRouteMonitorCreationWorks
		 	Context("rmo Route Monitor Operator integration test", func(ctx context.Context) {
	*/
})
