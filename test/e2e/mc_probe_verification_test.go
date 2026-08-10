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

var _ = Describe("MC Probe Verification", Ordered, func() {
	var (
		k8s             *openshift.Client
		clusterID       string
		rhobsAPIURL     string
		oidcCredentials *OIDCCredentials
	)

	const (
		rmoNamespace = "openshift-route-monitor-operator"
		probeTimeout = 10 * time.Minute
	)

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

		By("getting OIDC credentials from ConfigMap")
		configMap := &corev1.ConfigMap{}
		err = k8s.Get(ctx, "route-monitor-operator-config", rmoNamespace, configMap)
		Expect(err).ShouldNot(HaveOccurred(), "RMO config ConfigMap not found")

		oidcCredentials = &OIDCCredentials{
			ClientID:     configMap.Data["oidc-client-id"],
			ClientSecret: configMap.Data["oidc-client-secret"],
			IssuerURL:    configMap.Data["oidc-issuer-url"],
			ProbeAPIURL:  configMap.Data["probe-api-url"],
		}
		Expect(oidcCredentials.ProbeAPIURL).ShouldNot(BeEmpty(), "probe-api-url not configured")
		Expect(oidcCredentials.ClientID).ShouldNot(BeEmpty(), "oidc-client-id not configured")
		Expect(oidcCredentials.ClientSecret).ShouldNot(BeEmpty(), "oidc-client-secret not configured")
		Expect(oidcCredentials.IssuerURL).ShouldNot(BeEmpty(), "oidc-issuer-url not configured")

		rhobsAPIURL = oidcCredentials.ProbeAPIURL

		GinkgoLogr.Info("MC Probe Verification initialized",
			"clusterID", clusterID)
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

	It("verifies RMO created an RHOBS probe for the real HCP", func(ctx context.Context) {
		By(fmt.Sprintf("waiting for RHOBS probe for cluster %s (up to %s)", clusterID, probeTimeout))

		var probe map[string]interface{}
		Eventually(func(g Gomega) {
			probes, err := listRHOBSProbes(rhobsAPIURL, fmt.Sprintf("cluster-id=%s", clusterID), oidcCredentials)
			if err != nil {
				GinkgoLogr.Info("RHOBS API query failed, will retry")
			}
			g.Expect(err).ToNot(HaveOccurred(), "failed to query RHOBS API for cluster %s", clusterID)
			g.Expect(probes).ToNot(BeEmpty(), "no probe found for cluster %s", clusterID)
			probe = probes[0]
		}, probeTimeout, 15*time.Second).Should(Succeed(),
			"RMO did not create an RHOBS probe for cluster %s", clusterID)

		By("validating probe configuration")
		probeID, ok := probe["id"].(string)
		Expect(ok && probeID != "").To(BeTrue(), "probe should have an ID")

		probeLabels, _ := probe["labels"].(map[string]interface{})
		Expect(probeLabels).To(HaveKey("cluster-id"))
		Expect(probeLabels["cluster-id"]).To(Equal(clusterID))

		GinkgoLogr.Info("RHOBS probe verified for real HCP",
			"probeID", probeID,
			"clusterID", clusterID,
			"labels", probeLabels)
	})
})
