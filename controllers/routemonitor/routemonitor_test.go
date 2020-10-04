package routemonitor_test

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"context"
	"time"

	"github.com/openshift/route-monitor-operator/controllers/routemonitor"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	routemonitorconst "github.com/openshift/route-monitor-operator/pkg/const"
	consterror "github.com/openshift/route-monitor-operator/pkg/const/test/error"
	constinit "github.com/openshift/route-monitor-operator/pkg/const/test/init"
	utilreconcile "github.com/openshift/route-monitor-operator/pkg/util/reconcile"
	clientmocks "github.com/openshift/route-monitor-operator/pkg/util/tests/generated/mocks/client"
	routemonitormocks "github.com/openshift/route-monitor-operator/pkg/util/tests/generated/mocks/routemonitor"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Routemonitor", func() {

	var (
		scheme = constinit.Scheme
	)
	var (
		mockClient *clientmocks.MockClient
		mockCtrl   *gomock.Controller

		routeMonitorReconciler       routemonitor.RouteMonitorReconciler
		routeMonitorReconcilerClient client.Client
		mockActionDoer               *routemonitormocks.MockRouteMonitorActionDoer
		mockDeleter                  *routemonitormocks.MockRouteMonitorDeleter

		ctx context.Context

		updateCalledTimes   int
		updateErrorResponse error
		deleteCalledTimes   int
		deleteErrorResponse error

		routeMonitor                  v1alpha1.RouteMonitor
		routeMonitorFinalizers        []string
		routeMonitorDeletionTimestamp *metav1.Time

		deleteServiceMonitorShouldPass bool
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockClient = clientmocks.NewMockClient(mockCtrl)
		mockDeleter = routemonitormocks.NewMockRouteMonitorDeleter(mockCtrl)
		mockActionDoer = routemonitormocks.NewMockRouteMonitorActionDoer(mockCtrl)
		routeMonitorFinalizers = routemonitorconst.FinalizerList

		routeMonitorReconcilerClient = mockClient

		ctx = constinit.Context

		updateCalledTimes = 0
		updateErrorResponse = nil
		deleteCalledTimes = 0
		deleteErrorResponse = nil

		deleteServiceMonitorShouldPass = true
	})
	JustBeforeEach(func() {
		mockClient.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(updateErrorResponse).
			Times(updateCalledTimes)

		mockClient.EXPECT().Delete(gomock.Any(), gomock.Any()).
			Return(deleteErrorResponse).
			Times(deleteCalledTimes)

		routeMonitorReconciler = routemonitor.RouteMonitorReconciler{
			Log:                    constinit.Logger,
			Client:                 routeMonitorReconcilerClient,
			Scheme:                 constinit.Scheme,
			RouteMonitorDeleter:    mockDeleter,
			RouteMonitorActionDoer: mockActionDoer,
		}

		routeMonitor = v1alpha1.RouteMonitor{
			ObjectMeta: metav1.ObjectMeta{
				DeletionTimestamp: routeMonitorDeletionTimestamp,
				Finalizers:        routeMonitorFinalizers,
			},
		}

		if deleteServiceMonitorShouldPass {
			mockDeleter.EXPECT().DeleteServiceMonitorResource(gomock.Any(), gomock.Any()).Return(nil).Times(1)
		}

	})
	AfterEach(func() {
		mockCtrl.Finish()
	})

	Describe("DeleteRouteMonitorAndDependencies", func() {
		// Arrange
		BeforeEach(func() {
			routeMonitorReconcilerClient = fake.NewFakeClientWithScheme(scheme, &routeMonitor)
		})
		When("DeleteServiceMonitorResource fails unexpectidly", func() {
			BeforeEach(func() {
				deleteServiceMonitorShouldPass = false
			})
			JustBeforeEach(func() {
				mockDeleter.EXPECT().DeleteServiceMonitorResource(gomock.Any(), gomock.Any()).Return(consterror.CustomError)
			})
			It("should bubble up the error and return it", func() {
				// Act
				_, err := routeMonitorReconciler.DeleteRouteMonitorAndDependencies(ctx, routeMonitor)
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(consterror.CustomError))
			})
		})
		When("the resource has a finalizer but 'Update' failed", func() {
			// Arrange
			BeforeEach(func() {
				updateCalledTimes = 1
				updateErrorResponse = consterror.CustomError
				routeMonitorReconcilerClient = mockClient
			})
			It("Should bubble up the failure", func() {
				// Act
				_, err := routeMonitorReconciler.DeleteRouteMonitorAndDependencies(ctx, routeMonitor)
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(consterror.CustomError))
			})
		})
		When("the resource has a finalizer but 'Update' succeeds", func() {
			// Arrange
			BeforeEach(func() {
				updateCalledTimes = 1
				routeMonitorReconcilerClient = mockClient
			})
			It("Should succeed and call for a requeue", func() {
				// Act
				res, err := routeMonitorReconciler.DeleteRouteMonitorAndDependencies(ctx, routeMonitor)
				// Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(res).NotTo(BeNil())
				Expect(res).To(Equal(utilreconcile.StopOperation()))
			})
		})
		When("the resource doesnt have a finalizer but not deletion was requested", func() {
			BeforeEach(func() {
				routeMonitorFinalizers = []string{}
				deleteServiceMonitorShouldPass = true
				routeMonitorReconcilerClient = mockClient
			})
			It("should complete successfully", func() {
				// Act
				res, err := routeMonitorReconciler.DeleteRouteMonitorAndDependencies(ctx, routeMonitor)
				// Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(res).To(Equal(utilreconcile.ContinueOperation()))
			})
		})

		When("the resource has a finalizer but 'Delete' failed", func() {
			// Arrange
			BeforeEach(func() {
				routeMonitorFinalizers = []string{}
				routeMonitorDeletionTimestamp = &metav1.Time{Time: time.Unix(0, 0)}
				deleteCalledTimes = 1
				deleteErrorResponse = consterror.CustomError
				routeMonitorReconcilerClient = mockClient
			})
			It("Should bubble up the failure", func() {
				// Act
				_, err := routeMonitorReconciler.DeleteRouteMonitorAndDependencies(ctx, routeMonitor)
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(consterror.CustomError))
			})
		})
		When("the resource has a finalizer but 'Delete' succeeds", func() {
			// Arrange
			BeforeEach(func() {
				routeMonitorFinalizers = []string{}
				routeMonitorDeletionTimestamp = &metav1.Time{Time: time.Unix(0, 0)}
				deleteCalledTimes = 1
				routeMonitorReconcilerClient = mockClient
			})
			It("should pass successfully", func() {
				// Act
				_, err := routeMonitorReconciler.DeleteRouteMonitorAndDependencies(ctx, routeMonitor)
				// Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(err).To(BeNil())
			})
		})
	})
})
