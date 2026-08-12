// DO NOT REMOVE TAGS BELOW. IF ANY NEW TEST FILES ARE CREATED UNDER /osde2e, PLEASE ADD THESE TAGS TO THEM IN ORDER TO BE EXCLUDED FROM UNIT TESTS.
//go:build osde2e
// +build osde2e

package osde2etests

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	hypershiftv1beta1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	"github.com/openshift/osde2e-common/pkg/clients/openshift"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	rmoFinalizer      = "hostedcontrolplane.routemonitoroperator.monitoring.openshift.io/finalizer"
	reconcileTimeout  = 5 * time.Minute
	reconcileInterval = 10 * time.Second
)

var _ = Describe("MC Probe Verification", Ordered, func() {
	var (
		k8s          *openshift.Client
		clusterID    string
		hcpName      string
		hcpNamespace string
	)

	const rmoNamespace = "openshift-route-monitor-operator"

	BeforeAll(func(ctx context.Context) {
		log.SetLogger(GinkgoLogr)

		sharedDir := os.Getenv("SHARED_DIR")
		if sharedDir == "" {
			Skip("SHARED_DIR not set, not running in CI")
		}

		mcKubeconfig := filepath.Join(sharedDir, "hs-mc.kubeconfig")
		if _, err := os.Stat(mcKubeconfig); err != nil {
			Skip("hs-mc.kubeconfig not found in SHARED_DIR, not an MC e2e run")
		}

		clusterIDBytes, err := os.ReadFile(filepath.Join(sharedDir, "cluster-id"))
		if err != nil {
			Skip("cluster-id not found in SHARED_DIR")
		}
		clusterID = strings.TrimSpace(string(clusterIDBytes))
		if clusterID == "" {
			Skip("cluster-id is empty")
		}
		GinkgoLogr.Info("Testing against real HCP", "clusterID", clusterID)

		k8s, err = openshift.New(GinkgoLogr)
		Expect(err).ShouldNot(HaveOccurred(), "unable to setup k8s client")

		Expect(hypershiftv1beta1.AddToScheme(k8s.GetScheme())).Should(Succeed())

		By("verifying RMO is running on the MC")
		configMap := &corev1.ConfigMap{}
		err = k8s.Get(ctx, "route-monitor-operator-config", rmoNamespace, configMap)
		Expect(err).ShouldNot(HaveOccurred(), "RMO config ConfigMap not found")
		GinkgoLogr.Info("RMO config found on MC")
	})

	It("finds the real HCP on the management cluster", func(ctx context.Context) {
		hcpList := &hypershiftv1beta1.HostedControlPlaneList{}
		err := k8s.List(ctx, hcpList)
		Expect(err).ShouldNot(HaveOccurred(), "failed to list HCPs")

		var found bool
		for _, hcp := range hcpList.Items {
			labelID := hcp.Labels["api.openshift.com/id"]
			if labelID == clusterID || hcp.Spec.ClusterID == clusterID {
				found = true
				hcpName = hcp.Name
				hcpNamespace = hcp.Namespace
				logFields := []interface{}{
					"name", hcp.Name,
					"namespace", hcp.Namespace,
					"specClusterID", hcp.Spec.ClusterID,
					"labelID", labelID,
					"platform", hcp.Spec.Platform.Type,
				}
				if hcp.Spec.Platform.AWS != nil {
					logFields = append(logFields, "endpointAccess", hcp.Spec.Platform.AWS.EndpointAccess)
				}
				GinkgoLogr.Info("Found HCP", logFields...)
				break
			}
		}
		Expect(found).To(BeTrue(), "HCP with cluster ID %s not found on MC", clusterID)
	})

	It("verifies RMO added its finalizer to the HCP", func(ctx context.Context) {
		if hcpName == "" {
			Skip("HCP not found in previous test")
		}

		By(fmt.Sprintf("waiting for RMO finalizer on HCP %s/%s", hcpNamespace, hcpName))
		Eventually(func(g Gomega) {
			hcp := &hypershiftv1beta1.HostedControlPlane{}
			err := k8s.Get(ctx, hcpName, hcpNamespace, hcp)
			g.Expect(err).ToNot(HaveOccurred(), "failed to get HCP")
			g.Expect(hcp.Finalizers).To(ContainElement(rmoFinalizer),
				"RMO finalizer not found on HCP, controller may not be watching")
		}, reconcileTimeout, reconcileInterval).Should(Succeed(),
			"RMO did not add finalizer to HCP %s/%s", hcpNamespace, hcpName)

		GinkgoLogr.Info("RMO finalizer verified on HCP", "name", hcpName, "namespace", hcpNamespace)
	})

	It("verifies RMO created a health check ConfigMap for the HCP", func(ctx context.Context) {
		if hcpName == "" {
			Skip("HCP not found in previous test")
		}

		healthcheckCMName := fmt.Sprintf("%s-kube-apiserver-rmo-healthcheck", hcpName)
		By(fmt.Sprintf("waiting for health check ConfigMap %s/%s", hcpNamespace, healthcheckCMName))

		Eventually(func(g Gomega) {
			cm := &corev1.ConfigMap{}
			err := k8s.Get(ctx, healthcheckCMName, hcpNamespace, cm)
			g.Expect(err).ToNot(HaveOccurred(), "health check ConfigMap not created yet")
		}, reconcileTimeout, reconcileInterval).Should(Succeed(),
			"RMO did not create health check ConfigMap for HCP %s", hcpName)

		GinkgoLogr.Info("RMO health check ConfigMap verified",
			"name", healthcheckCMName, "namespace", hcpNamespace)
	})
})
