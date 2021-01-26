package deleter_test

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"context"

	// tested package
	"github.com/openshift/route-monitor-operator/controllers/routemonitor/deleter"

	"sigs.k8s.io/controller-runtime/pkg/client"

	consterror "github.com/openshift/route-monitor-operator/pkg/consts/test/error"
	constinit "github.com/openshift/route-monitor-operator/pkg/consts/test/init"
	clientmocks "github.com/openshift/route-monitor-operator/pkg/util/test/generated/mocks/client"
	"github.com/openshift/route-monitor-operator/pkg/util/test/helper"

	"github.com/openshift/route-monitor-operator/api/v1alpha1"

	"github.com/openshift/route-monitor-operator/controllers/routemonitor"
)

var _ = Describe("Deleter", func() {
	var (
		mockClient *clientmocks.MockClient
		mockCtrl   *gomock.Controller

		routeMonitorDeleter       deleter.RouteMonitorDeleter
		routeMonitorDeleterClient client.Client

		ctx context.Context

		get    helper.MockHelper
		delete helper.MockHelper
		list   helper.MockHelper
	)
	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockClient = clientmocks.NewMockClient(mockCtrl)

		routeMonitorDeleterClient = mockClient

		ctx = constinit.Context

		get = helper.MockHelper{}
		delete = helper.MockHelper{}
		list = helper.MockHelper{}
	})
	JustBeforeEach(func() {
		routeMonitorDeleter = deleter.RouteMonitorDeleter{
			Log:    constinit.Logger,
			Client: routeMonitorDeleterClient,
			Scheme: constinit.Scheme,
		}

		mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(get.ErrorResponse).
			Times(get.CalledTimes)

		mockClient.EXPECT().Delete(gomock.Any(), gomock.Any()).
			Return(delete.ErrorResponse).
			Times(delete.CalledTimes)

		mockClient.EXPECT().List(gomock.Any(), gomock.Any()).
			Return(list.ErrorResponse).
			Times(list.CalledTimes)
	})
	AfterEach(func() {
		mockCtrl.Finish()
	})

	Describe("DeletePrometheusRuleResource", func() {
		BeforeEach(func() {
			get.CalledTimes = 1
			routeMonitorDeleterClient = mockClient

		})
		When("'Get' returns an unhandled error", func() {
			// Arrange
			BeforeEach(func() {
				get.ErrorResponse = consterror.CustomError
			})
			It("should bubble the error up", func() {
				// Act
				err := routeMonitorDeleter.EnsurePrometheusRuleResourceAbsent(ctx, v1alpha1.RouteMonitor{})
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(consterror.CustomError))
			})
		})
		When("'Get' returns an 'Not found' error", func() {
			// Arrange
			BeforeEach(func() {
				get.ErrorResponse = consterror.NotFoundErr
			})
			It("should succeed as there is nothing to delete", func() {
				// Act
				err := routeMonitorDeleter.EnsurePrometheusRuleResourceAbsent(ctx, v1alpha1.RouteMonitor{})
				// Assert
				Expect(err).NotTo(HaveOccurred())
			})
		})
		When("'Delete' returns an unhandled error", func() {
			// Arrange
			BeforeEach(func() {
				delete = helper.CustomErrorHappensOnce()
				routeMonitorDeleterClient = mockClient
			})
			It("should bubble the error up", func() {
				// Act
				err := routeMonitorDeleter.EnsurePrometheusRuleResourceAbsent(ctx, v1alpha1.RouteMonitor{})
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(consterror.CustomError))
			})
		})
		When("'Delete' passes", func() {
			// Arrange
			BeforeEach(func() {
				delete.CalledTimes = 1
			})
			It("should succeed as the object was deleted", func() {
				// Act
				err := routeMonitorDeleter.EnsurePrometheusRuleResourceAbsent(ctx, v1alpha1.RouteMonitor{})
				// Assert
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
	Describe("DeleteServiceMonitorResource", func() {
		BeforeEach(func() {
			get.CalledTimes = 1
			routeMonitorDeleterClient = mockClient

		})
		When("'Get' returns an unhandled error", func() {
			// Arrange
			BeforeEach(func() {
				get.ErrorResponse = consterror.CustomError
			})
			It("should bubble the error up", func() {
				// Act
				err := routeMonitorDeleter.EnsureServiceMonitorResourceAbsent(ctx, v1alpha1.RouteMonitor{})
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(consterror.CustomError))
			})
		})
		When("'Get' returns an 'Not found' error", func() {
			// Arrange
			BeforeEach(func() {
				get.ErrorResponse = consterror.NotFoundErr
			})
			It("should succeed as there is nothing to delete", func() {
				// Act
				err := routeMonitorDeleter.EnsureServiceMonitorResourceAbsent(ctx, v1alpha1.RouteMonitor{})
				// Assert
				Expect(err).NotTo(HaveOccurred())
			})
		})
		When("'Delete' returns an unhandled error", func() {
			// Arrange
			BeforeEach(func() {
				delete = helper.CustomErrorHappensOnce()
				routeMonitorDeleterClient = mockClient
			})
			It("should bubble the error up", func() {
				// Act
				err := routeMonitorDeleter.EnsureServiceMonitorResourceAbsent(ctx, v1alpha1.RouteMonitor{})
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(consterror.CustomError))
			})
		})
		When("'Delete' passes", func() {
			// Arrange
			BeforeEach(func() {
				delete.CalledTimes = 1
			})
			It("should succeed as the object was deleted", func() {
				// Act
				err := routeMonitorDeleter.EnsureServiceMonitorResourceAbsent(ctx, v1alpha1.RouteMonitor{})
				// Assert
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
	Describe("New", func() {
		When("func New is called", func() {
			It("should return a new Deleter object", func() {
				// Arrange
				r := routemonitor.RouteMonitorReconciler{
					Client: routeMonitorDeleterClient,
					Log:    constinit.Logger,
					Scheme: constinit.Scheme,
				}
				// Act
				res := deleter.New(r)
				// Assert
				Expect(res).To(Equal(&deleter.RouteMonitorDeleter{
					Client: routeMonitorDeleterClient,
					Log:    constinit.Logger,
					Scheme: constinit.Scheme,
				}))
			})
		})
	})

})
