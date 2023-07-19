package clusterurlmonitor_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	configv1 "github.com/openshift/api/config/v1"
	hypershiftv1beta1 "github.com/openshift/hypershift/api/v1beta1"
	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	"github.com/openshift/route-monitor-operator/controllers/clusterurlmonitor"
	constinit "github.com/openshift/route-monitor-operator/pkg/consts/test/init"
	reconcileCommon "github.com/openshift/route-monitor-operator/pkg/reconcile"
)

var _ = Describe("ClusterUrlMonitorSupplement", func() {
	var (
		clusterUrlMonitor v1alpha1.ClusterUrlMonitor
		reconciler        clusterurlmonitor.ClusterUrlMonitorReconciler

		testObjs []client.Object
	)

	BeforeEach(func() {
		clusterUrlMonitor = v1alpha1.ClusterUrlMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "fake-clusterurlmonitor",
				Namespace: "fake-namespace",
			},
		}
	})

	JustBeforeEach(func() {
		client := buildClient(testObjs...)
		ctx := context.TODO()
		reconciler = clusterurlmonitor.ClusterUrlMonitorReconciler{
			Log:    constinit.Logger,
			Client: client,
			Scheme: constinit.Scheme,
			Common: reconcileCommon.NewMonitorResourceCommon(ctx, client),
			Ctx:    ctx,
		}
	})

	AfterEach(func() {
		// Clear objects between tests to avoid cross-contamination
		testObjs = []client.Object{}
	})

	Describe("GetClusterDomain()", func() {
		Describe("Public clusters", func() {
			const (
				expectedDomain = "testdomain.devshift.org"
			)
			Context("HyperShift", func() {
				BeforeEach(func() {
					clusterUrlMonitor.Spec.DomainRef = v1alpha1.ClusterDomainRefHCP

					hcp := hypershiftv1beta1.HostedControlPlane{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-hcp",
							Namespace: "fake-namespace",
							Annotations: map[string]string{
								"hypershift.openshift.io/cluster": "test-ns/test-hc",
							},
						},
						Spec: hypershiftv1beta1.HostedControlPlaneSpec{
							Platform: hypershiftv1beta1.PlatformSpec{
								AWS: &hypershiftv1beta1.AWSPlatformSpec{
									EndpointAccess: hypershiftv1beta1.Public,
								},
							},
						},
						Status: hypershiftv1beta1.HostedControlPlaneStatus{
							Conditions: []metav1.Condition{{
								Type:   string(hypershiftv1beta1.InfrastructureReady),
								Status: metav1.ConditionTrue,
							}},
						},
					}
					svc := corev1.Service{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "private-router",
							Namespace: "fake-namespace",
						},
						Status: corev1.ServiceStatus{
							LoadBalancer: corev1.LoadBalancerStatus{
								Ingress: []corev1.LoadBalancerIngress{{
									Hostname: "testdomain.devshift.org",
								}},
							},
						},
					}
					testObjs = append(testObjs, &hcp)
					testObjs = append(testObjs, &svc)
				})

				It("should return a cluster URL", func() {
					domain, err := reconciler.GetClusterDomain(clusterUrlMonitor)
					Expect(err).ToNot(HaveOccurred())
					Expect(domain).To(Equal(expectedDomain))
				})
			})
			Context("OSD/ROSA", func() {
				var infra configv1.Infrastructure
				BeforeEach(func() {
					clusterUrlMonitor.Spec.DomainRef = v1alpha1.ClusterDomainRefInfra

					infra = configv1.Infrastructure{
						ObjectMeta: metav1.ObjectMeta{
							Name: "cluster",
						},
					}
					testObjs = append(testObjs, &infra)
				})

				It("should return a cluster URL", func() {
					// Objects cannot be created with a status predefined - it must
					// be added as an update after creating
					infra.Status.APIServerURL = fmt.Sprintf("https://api.%s:6443", expectedDomain)
					err := reconciler.Client.Status().Update(context.TODO(), &infra)
					Expect(err).ToNot(HaveOccurred())

					domain, err := reconciler.GetClusterDomain(clusterUrlMonitor)
					Expect(err).ToNot(HaveOccurred())
					Expect(domain).To(Equal(expectedDomain))
				})
			})
		})
	})
})

func buildClient(objs ...client.Object) client.Client {
	var err error
	err = hypershiftv1beta1.AddToScheme(constinit.Scheme)
	Expect(err).ToNot(HaveOccurred())
	err = configv1.AddToScheme(constinit.Scheme)
	Expect(err).ToNot(HaveOccurred())
	err = corev1.AddToScheme(constinit.Scheme)
	Expect(err).ToNot(HaveOccurred())

	builder := fake.NewClientBuilder().WithObjects(objs...).WithScheme(constinit.Scheme)
	return builder.Build()
}
