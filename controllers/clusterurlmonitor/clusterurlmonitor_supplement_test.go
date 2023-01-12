package clusterurlmonitor_test

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	"github.com/openshift/route-monitor-operator/controllers/clusterurlmonitor"
	constinit "github.com/openshift/route-monitor-operator/pkg/consts/test/init"
	clientmocks "github.com/openshift/route-monitor-operator/pkg/util/test/generated/mocks/client"
	controllermocks "github.com/openshift/route-monitor-operator/pkg/util/test/generated/mocks/controllers"
)

var _ = Describe("ClusterUrlMonitorSupplement", func() {
	var (
		clusterUrlMonitor    v1alpha1.ClusterUrlMonitor
		reconciler           clusterurlmonitor.ClusterUrlMonitorReconciler
		mockClient           *clientmocks.MockClient
		mockBlackBoxExporter *controllermocks.MockBlackBoxExporterHandler
		mockCommon           *controllermocks.MockMonitorResourceHandler
		mockPrometheusRule   *controllermocks.MockPrometheusRuleHandler
		mockServiceMonitor   *controllermocks.MockServiceMonitorHandler

		mockCtrl *gomock.Controller

		prefix string
		port   string
		suffix string

		err error
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockClient = clientmocks.NewMockClient(mockCtrl)
		mockBlackBoxExporter = controllermocks.NewMockBlackBoxExporterHandler(mockCtrl)
		mockServiceMonitor = controllermocks.NewMockServiceMonitorHandler(mockCtrl)
		mockPrometheusRule = controllermocks.NewMockPrometheusRuleHandler(mockCtrl)
		mockCommon = controllermocks.NewMockMonitorResourceHandler(mockCtrl)
		clusterUrlMonitor = v1alpha1.ClusterUrlMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "fake-clusterurlmonitor",
				Namespace: "fake-namespace",
			},
		}
	})

	JustBeforeEach(func() {
		clusterUrlMonitor.Spec.Prefix = prefix
		clusterUrlMonitor.Spec.Suffix = suffix
		clusterUrlMonitor.Spec.Port = port
		reconciler = clusterurlmonitor.ClusterUrlMonitorReconciler{
			Log:              constinit.Logger,
			Client:           mockClient,
			Scheme:           constinit.Scheme,
			BlackBoxExporter: mockBlackBoxExporter,
			Common:           mockCommon,
			ServiceMonitor:   mockServiceMonitor,
			Prom:             mockPrometheusRule,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Describe("GetClusterDomain()", func() {
		Context("HyperShift", func() {
			BeforeEach(func() {
				mockClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "cluster"}, &configv1.Infrastructure{}).DoAndReturn(
					func(arg0, arg1, arg2 interface{}, arg3 ...interface{}) error {
						return nil
					})
			})

			It("should return a cluster URL", func() {
				// can't figure out how to use gomock to modify arguments
				_, err = reconciler.GetClusterDomain()

				Expect(err).To(BeNil())
			})
		})
	})
})
