package clusterurlmonitor_test

import (
	"time"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	customerrors "github.com/openshift/route-monitor-operator/pkg/util/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	"github.com/openshift/route-monitor-operator/controllers/clusterurlmonitor"
	constinit "github.com/openshift/route-monitor-operator/pkg/consts/test/init"
	"github.com/openshift/route-monitor-operator/pkg/util/reconcile"
	utilreconcile "github.com/openshift/route-monitor-operator/pkg/util/reconcile"
	clientmocks "github.com/openshift/route-monitor-operator/pkg/util/test/generated/mocks/client"
	controllermocks "github.com/openshift/route-monitor-operator/pkg/util/test/generated/mocks/controllers"
)

var _ = Describe("Clusterurlmonitor", func() {
	var (
		clusterUrlMonitor  v1alpha1.ClusterUrlMonitor
		reconciler         clusterurlmonitor.ClusterUrlMonitorReconciler
		mockClient         *clientmocks.MockClient
		mockCommon         *controllermocks.MockMonitorResourceHandler
		mockPrometheusRule *controllermocks.MockPrometheusRuleHandler
		mockServiceMonitor *controllermocks.MockServiceMonitorHandler

		mockCtrl *gomock.Controller

		prefix string
		port   string
		suffix string

		res utilreconcile.Result
		err error
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockClient = clientmocks.NewMockClient(mockCtrl)
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
			Log:            constinit.Logger,
			Client:         mockClient,
			Scheme:         constinit.Scheme,
			Common:         mockCommon,
			ServiceMonitor: mockServiceMonitor,
			Prom:           mockPrometheusRule,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Describe("EnsureServiceMonitorExists", func() {
		BeforeEach(func() {
			port = "1337"
			prefix = "prefix."
			suffix = "/suffix"
		})
		JustBeforeEach(func() {
			res, err = reconciler.EnsureServiceMonitorExists(clusterUrlMonitor)
		})
		When("the ServiceMonitor doesn't exist", func() {
			BeforeEach(func() {
				mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Times(1) // fetching domain
				mockServiceMonitor.EXPECT().TemplateAndUpdateServiceMonitorDeployment(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1)
				ns := types.NamespacedName{Name: clusterUrlMonitor.Name, Namespace: clusterUrlMonitor.Namespace}
				mockCommon.EXPECT().GetOSDClusterID().Times(1)
				mockCommon.EXPECT().SetResourceReference(&clusterUrlMonitor.Status.ServiceMonitorRef, ns).Times(1).Return(true, nil)
				mockCommon.EXPECT().UpdateMonitorResourceStatus(&clusterUrlMonitor).Times(1)
			})
			It("creates a ServiceMonitor and updates the ServiceRef", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(res).To(Equal(utilreconcile.StopOperation()))
			})
		})
	})

	Describe("EnsurePrometheusRuleResourceExists", func() {
		var (
			res utilreconcile.Result
			err error
		)
		JustBeforeEach(func() {
			res, err = reconciler.EnsurePrometheusRuleExists(clusterUrlMonitor)
		})
		When("the ClusterUrlMonitor has an invalid slo value", func() {
			BeforeEach(func() {
				mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Times(1)
				err := customerrors.InvalidSLO
				mockCommon.EXPECT().ParseMonitorSLOSpecs(gomock.Any(), clusterUrlMonitor.Spec.Slo).Times(1).Return("", err)
				mockCommon.EXPECT().SetErrorStatus(&clusterUrlMonitor.Status.ErrorStatus, err)
				// It deletes old pormetheus rule deployment if still there
				mockPrometheusRule.EXPECT().DeletePrometheusRuleDeployment(gomock.Any()).Times(1)
				mockCommon.EXPECT().SetResourceReference(&clusterUrlMonitor.Status.PrometheusRuleRef, types.NamespacedName{}).Times(1)
			})
			It("sets the error in the ClusterUrlMonitor and stops processing", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(res).NotTo(BeNil())
				Expect(res).To(Equal(utilreconcile.StopOperation()))
			})
		})
		When("the resource Exists but not the same as the generated template", func() {
			BeforeEach(func() {
				mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Times(1)
				mockCommon.EXPECT().ParseMonitorSLOSpecs(gomock.Any(), clusterUrlMonitor.Spec.Slo).Times(1).Return("99.5", nil)
				mockCommon.EXPECT().SetErrorStatus(&clusterUrlMonitor.Status.ErrorStatus, nil)
				mockPrometheusRule.EXPECT().UpdatePrometheusRuleDeployment(gomock.Any()).Times(1)
				mockCommon.EXPECT().SetResourceReference(&clusterUrlMonitor.Status.PrometheusRuleRef, gomock.Any()).Times(1).Return(false, nil)
			})
			It("doesn't update the clusterUrlMonitor reference and continues reconciling", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(res).NotTo(BeNil())
				Expect(res).To(Equal(utilreconcile.ContinueOperation()))
			})
		})
		When("the resource doesn't exists", func() {
			BeforeEach(func() {
				mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Times(1)
				mockCommon.EXPECT().ParseMonitorSLOSpecs(gomock.Any(), clusterUrlMonitor.Spec.Slo).Times(1).Return("99.5", nil)
				mockCommon.EXPECT().SetErrorStatus(&clusterUrlMonitor.Status.ErrorStatus, nil)
				mockPrometheusRule.EXPECT().UpdatePrometheusRuleDeployment(gomock.Any()).Times(1)
				ns := types.NamespacedName{Name: clusterUrlMonitor.Name, Namespace: clusterUrlMonitor.Namespace}
				mockCommon.EXPECT().SetResourceReference(&clusterUrlMonitor.Status.PrometheusRuleRef, ns).Times(1).Return(true, nil)
				mockCommon.EXPECT().UpdateMonitorResourceStatus(&clusterUrlMonitor).Times(1).Return(utilreconcile.StopOperation(), nil)
			})

			It("should create one and update the clusterURLMonitor", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(res).NotTo(BeNil())
				Expect(res).To(Equal(utilreconcile.StopOperation()))
			})
		})
	})

	Describe("EnsureDeletionProcessed", func() {
		var (
			res utilreconcile.Result
			err error
		)
		BeforeEach(func() {
			clusterUrlMonitor.Finalizers = []string{clusterurlmonitor.FinalizerKey}
		})
		JustBeforeEach(func() {
			res, err = reconciler.EnsureMonitorAndDependenciesAbsent(clusterUrlMonitor)
		})
		When("the ClusterUrlMonitor CR is not being deleted", func() {
			It("does nothing", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(res).To(Equal(reconcile.ContinueOperation()))
			})
		})

		When("the ClusterUrlMonitor CR is being deleted", func() {
			BeforeEach(func() {
				clusterUrlMonitor.DeletionTimestamp = &metav1.Time{Time: time.Unix(0, 0)}
			})
			When("the ServiceMonitor still exists", func() {
				BeforeEach(func() {
					mockPrometheusRule.EXPECT().DeletePrometheusRuleDeployment(clusterUrlMonitor.Status.PrometheusRuleRef).Times(1)
					mockServiceMonitor.EXPECT().DeleteServiceMonitorDeployment(clusterUrlMonitor.Status.ServiceMonitorRef, gomock.Any()).Times(1)
					gomock.InOrder(
						mockCommon.EXPECT().DeleteFinalizer(&clusterUrlMonitor, clusterurlmonitor.FinalizerKey).Times(1).Return(true),
						mockCommon.EXPECT().DeleteFinalizer(&clusterUrlMonitor, clusterurlmonitor.PrevFinalizerKey).Times(1),
					)
					mockCommon.EXPECT().UpdateMonitorResource(&clusterUrlMonitor).Return(reconcile.StopOperation(), nil)

				})
			})
		})
	})
})
