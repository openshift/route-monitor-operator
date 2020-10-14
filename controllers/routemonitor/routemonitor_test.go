package routemonitor_test

import (
	"time"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"context"

	//tested package
	"github.com/openshift/route-monitor-operator/controllers/routemonitor"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	routemonitorconst "github.com/openshift/route-monitor-operator/pkg/const"
	"github.com/openshift/route-monitor-operator/pkg/const/blackbox"
	consterror "github.com/openshift/route-monitor-operator/pkg/const/test/error"
	constinit "github.com/openshift/route-monitor-operator/pkg/const/test/init"
	utilreconcile "github.com/openshift/route-monitor-operator/pkg/util/reconcile"
	clientmocks "github.com/openshift/route-monitor-operator/pkg/util/tests/generated/mocks/client"
	routemonitormocks "github.com/openshift/route-monitor-operator/pkg/util/tests/generated/mocks/routemonitor"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Routemonitor", func() {

	var (
		mockClient *clientmocks.MockClient
		mockCtrl   *gomock.Controller

		routeMonitorReconciler       routemonitor.RouteMonitorReconciler
		routeMonitorReconcilerClient client.Client
		mockSupplement               *routemonitormocks.MockRouteMonitorSupplement
		mockDeleter                  *routemonitormocks.MockRouteMonitorDeleter
		mockAdder                    *routemonitormocks.MockRouteMonitorAdder
		ctx                          context.Context

		updateCalledTimes   int
		updateErrorResponse error

		deleteCalledTimes   int
		deleteErrorResponse error

		getCalledTimes   int
		getErrorResponse error

		createCalledTimes   int
		createErrorResponse error

		ensureServiceMonitorResourceAbsentCalledTimes   int
		ensureServiceMonitorResourceAbsentErrorResponse error

		shouldDeleteBlackBoxExporterResourcesCalledTimes   int
		shouldDeleteBlackBoxExporterResourcesResponse      blackbox.ShouldDeleteBlackBoxExporter
		shouldDeleteBlackBoxExporterResourcesErrorResponse error

		ensureBlackBoxExporterServiceAbsentCalledTimes   int
		ensureBlackBoxExporterServiceAbsentErrorResponse error

		ensureBlackBoxExporterDeploymentAbsentCalledTimes   int
		ensureBlackBoxExporterDeploymentAbsentErrorResponse error

		ensureBlackBoxExporterDeploymentExistsCalledTimes   int
		ensureBlackBoxExporterDeploymentExistsErrorResponse error

		ensureBlackBoxExporterServiceExistsCalledTimes   int
		ensureBlackBoxExporterServiceExistsErrorResponse error

		ensureFinalizerAbsentCalledTimes   int
		ensureFinalizerAbsentErrorResponse error
		ensureFinalizerAbsentResponse      utilreconcile.Result

		routeMonitor                  v1alpha1.RouteMonitor
		routeMonitorFinalizers        []string
		routeMonitorDeletionTimestamp *metav1.Time
		routeMonitorStatus            v1alpha1.RouteMonitorStatus
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockClient = clientmocks.NewMockClient(mockCtrl)
		mockDeleter = routemonitormocks.NewMockRouteMonitorDeleter(mockCtrl)
		mockAdder = routemonitormocks.NewMockRouteMonitorAdder(mockCtrl)

		mockSupplement = routemonitormocks.NewMockRouteMonitorSupplement(mockCtrl)
		routeMonitorFinalizers = routemonitorconst.FinalizerList

		routeMonitorReconcilerClient = mockClient

		ctx = constinit.Context

		updateCalledTimes = 0
		updateErrorResponse = nil
		deleteCalledTimes = 0
		deleteErrorResponse = nil
		getCalledTimes = 0
		getErrorResponse = nil
		createCalledTimes = 0
		createErrorResponse = nil

		ensureServiceMonitorResourceAbsentCalledTimes = 0
		ensureServiceMonitorResourceAbsentErrorResponse = nil
		shouldDeleteBlackBoxExporterResourcesCalledTimes = 0
		// KeepBlackBoxExporter is the false value for this bool type
		shouldDeleteBlackBoxExporterResourcesResponse = blackbox.KeepBlackBoxExporter
		shouldDeleteBlackBoxExporterResourcesErrorResponse = nil
		ensureBlackBoxExporterServiceAbsentCalledTimes = 0
		ensureBlackBoxExporterServiceAbsentErrorResponse = nil
		ensureBlackBoxExporterDeploymentAbsentCalledTimes = 0
		ensureBlackBoxExporterDeploymentAbsentErrorResponse = nil

		ensureBlackBoxExporterDeploymentExistsCalledTimes = 0
		ensureBlackBoxExporterDeploymentExistsErrorResponse = nil
		ensureBlackBoxExporterServiceExistsCalledTimes = 0
		ensureBlackBoxExporterServiceExistsErrorResponse = nil

		ensureFinalizerAbsentCalledTimes = 0
		ensureFinalizerAbsentErrorResponse = nil
		ensureFinalizerAbsentResponse = utilreconcile.Result{}

	})
	JustBeforeEach(func() {
		mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(getErrorResponse).
			Times(getCalledTimes)

		mockClient.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(updateErrorResponse).
			Times(updateCalledTimes)

		mockClient.EXPECT().Delete(gomock.Any(), gomock.Any()).
			Return(deleteErrorResponse).
			Times(deleteCalledTimes)

		if createCalledTimes != 0 {
			mockClient.EXPECT().Create(gomock.Any(), gomock.Any()).
				Return(createErrorResponse).
				Times(createCalledTimes)
		}

		mockDeleter.EXPECT().EnsureServiceMonitorResourceAbsent(gomock.Any(), gomock.Any()).
			Return(ensureServiceMonitorResourceAbsentErrorResponse).
			Times(ensureServiceMonitorResourceAbsentCalledTimes)

		mockDeleter.EXPECT().ShouldDeleteBlackBoxExporterResources(gomock.Any(), gomock.Any()).
			Times(shouldDeleteBlackBoxExporterResourcesCalledTimes).
			Return(shouldDeleteBlackBoxExporterResourcesResponse, shouldDeleteBlackBoxExporterResourcesErrorResponse)

		gomock.InOrder(
			mockDeleter.EXPECT().EnsureBlackBoxExporterServiceAbsent(gomock.Any()).
				Times(ensureBlackBoxExporterServiceAbsentCalledTimes).
				Return(ensureBlackBoxExporterServiceAbsentErrorResponse),
			mockDeleter.EXPECT().EnsureBlackBoxExporterDeploymentAbsent(gomock.Any()).
				Times(ensureBlackBoxExporterDeploymentAbsentCalledTimes).
				Return(ensureBlackBoxExporterDeploymentAbsentErrorResponse),
		)

		gomock.InOrder(
			mockAdder.EXPECT().EnsureBlackBoxExporterDeploymentExists(gomock.Any()).
				Times(ensureBlackBoxExporterDeploymentExistsCalledTimes).
				Return(ensureBlackBoxExporterDeploymentExistsErrorResponse),
			mockAdder.EXPECT().EnsureBlackBoxExporterServiceExists(gomock.Any()).
				Times(ensureBlackBoxExporterServiceExistsCalledTimes).
				Return(ensureBlackBoxExporterServiceExistsErrorResponse),
		)

		mockSupplement.EXPECT().EnsureFinalizerAbsent(gomock.Any(), gomock.Any()).
			Times(ensureFinalizerAbsentCalledTimes).
			Return(ensureFinalizerAbsentResponse, ensureFinalizerAbsentErrorResponse)

		routeMonitorReconciler = routemonitor.RouteMonitorReconciler{
			Log:                    constinit.Logger,
			Client:                 routeMonitorReconcilerClient,
			Scheme:                 constinit.Scheme,
			RouteMonitorSupplement: mockSupplement,
			RouteMonitorDeleter:    mockDeleter,
			RouteMonitorAdder:      mockAdder,
		}

		routeMonitor = v1alpha1.RouteMonitor{
			ObjectMeta: metav1.ObjectMeta{
				DeletionTimestamp: routeMonitorDeletionTimestamp,
				Finalizers:        routeMonitorFinalizers,
			},
			Status: routeMonitorStatus,
		}
	})
	AfterEach(func() {
		mockCtrl.Finish()
	})

	Describe("EnsureRouteMonitorAndDependenciesAbsent", func() {
		BeforeEach(func() {
			// Arrange
			routeMonitorReconcilerClient = mockClient
			shouldDeleteBlackBoxExporterResourcesCalledTimes = 1
		})
		When("func ShouldDeleteBlackBoxExporterResources fails unexpectedly", func() {
			BeforeEach(func() {
				// Arrange
				shouldDeleteBlackBoxExporterResourcesErrorResponse = consterror.CustomError
			})
			It("should bubble up the error", func() {
				// Act
				_, err := routeMonitorReconciler.EnsureRouteMonitorAndDependenciesAbsent(ctx, routeMonitor)
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(consterror.CustomError))
			})
		})
		Describe("ShouldDeleteBlackBoxExporterResources instructs to delete", func() {
			BeforeEach(func() {
				// Arrange
				shouldDeleteBlackBoxExporterResourcesResponse = blackbox.DeleteBlackBoxExporter
			})

			When("func EnsureBlackBoxExporterServiceAbsent fails unexpectedly", func() {
				BeforeEach(func() {
					// Arrange
					ensureBlackBoxExporterServiceAbsentCalledTimes = 1
					ensureBlackBoxExporterServiceAbsentErrorResponse = consterror.CustomError
				})
				It("should bubble up the error", func() {
					// Act
					_, err := routeMonitorReconciler.EnsureRouteMonitorAndDependenciesAbsent(ctx, routeMonitor)
					// Assert
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(consterror.CustomError))
				})
			})
			When("func EnsureBlackBoxExporterDeploymentAbsent fails unexpectedly", func() {
				BeforeEach(func() {
					ensureBlackBoxExporterServiceAbsentCalledTimes = 1
					ensureBlackBoxExporterDeploymentAbsentCalledTimes = 1
					ensureBlackBoxExporterDeploymentAbsentErrorResponse = consterror.CustomError
				})
				It("should bubble up the error", func() {
					// Act
					_, err := routeMonitorReconciler.EnsureRouteMonitorAndDependenciesAbsent(ctx, routeMonitor)
					// Assert
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(consterror.CustomError))
				})
			})
			When("func EnsureServiceMonitorResourceAbsent fails unexpectedly", func() {
				BeforeEach(func() {
					ensureBlackBoxExporterServiceAbsentCalledTimes = 1
					ensureBlackBoxExporterDeploymentAbsentCalledTimes = 1
					ensureServiceMonitorResourceAbsentCalledTimes = 1
					ensureServiceMonitorResourceAbsentErrorResponse = consterror.CustomError
				})
				It("should bubble up the error", func() {
					// Act
					_, err := routeMonitorReconciler.EnsureRouteMonitorAndDependenciesAbsent(ctx, routeMonitor)
					// Assert
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(consterror.CustomError))
				})
			})
			When("func EnsureFinalizerAbsent fails unexpectedly", func() {
				BeforeEach(func() {
					ensureBlackBoxExporterServiceAbsentCalledTimes = 1
					ensureBlackBoxExporterDeploymentAbsentCalledTimes = 1
					ensureServiceMonitorResourceAbsentCalledTimes = 1
					ensureFinalizerAbsentCalledTimes = 1
					ensureFinalizerAbsentErrorResponse = consterror.CustomError
				})
				It("should bubble up the error", func() {
					// Act
					_, err := routeMonitorReconciler.EnsureRouteMonitorAndDependenciesAbsent(ctx, routeMonitor)
					// Assert
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(consterror.CustomError))
				})
			})
			When("all deletions happened successfully", func() {
				BeforeEach(func() {
					ensureBlackBoxExporterServiceAbsentCalledTimes = 1
					ensureBlackBoxExporterDeploymentAbsentCalledTimes = 1
					ensureServiceMonitorResourceAbsentCalledTimes = 1
					ensureFinalizerAbsentCalledTimes = 1
				})
				It("should reconcile", func() {
					// Act
					res, err := routeMonitorReconciler.EnsureRouteMonitorAndDependenciesAbsent(ctx, routeMonitor)
					// Assert
					Expect(err).NotTo(HaveOccurred())
					Expect(res).To(Equal(utilreconcile.StopOperation()))
				})
			})
		})
		When("ShouldDeleteBlackBoxExporterResources instructs to keep the BlackBoxExporter", func() {
			BeforeEach(func() {
				// Arrange
				shouldDeleteBlackBoxExporterResourcesResponse = blackbox.KeepBlackBoxExporter
				ensureServiceMonitorResourceAbsentCalledTimes = 1
			})
			When("func EnsureServiceMonitorResourceAbsent fails unexpectedly", func() {
				BeforeEach(func() {
					// Arrange
					ensureServiceMonitorResourceAbsentErrorResponse = consterror.CustomError
				})
				It("should bubble up the error", func() {
					// Act
					_, err := routeMonitorReconciler.EnsureRouteMonitorAndDependenciesAbsent(ctx, routeMonitor)
					// Assert
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(consterror.CustomError))
				})
			})

			When("the resource has a finalizer but 'Update' failed", func() {
				// Arrange
				BeforeEach(func() {
					ensureFinalizerAbsentCalledTimes = 1
					ensureFinalizerAbsentErrorResponse = consterror.CustomError

				})
				It("Should bubble up the failure", func() {
					// Act
					_, err := routeMonitorReconciler.EnsureRouteMonitorAndDependenciesAbsent(ctx, routeMonitor)
					// Assert
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(consterror.CustomError))
				})
			})
			When("the resource has a finalizer but 'Update' succeeds", func() {
				// Arrange
				BeforeEach(func() {
					ensureFinalizerAbsentCalledTimes = 1
				})
				It("Should succeed and call for a requeue", func() {
					// Act
					res, err := routeMonitorReconciler.EnsureRouteMonitorAndDependenciesAbsent(ctx, routeMonitor)
					// Assert
					Expect(err).NotTo(HaveOccurred())
					Expect(res).NotTo(BeNil())
					Expect(res).To(Equal(utilreconcile.StopOperation()))
				})
			})
			When("resorce has no finalizer", func() {
				BeforeEach(func() {
					routeMonitorFinalizers = []string{}
					routeMonitorDeletionTimestamp = &metav1.Time{Time: time.Unix(0, 0)}
					deleteCalledTimes = 1
				})
				When("no deletion was requested", func() {
					BeforeEach(func() {
						routeMonitorDeletionTimestamp = nil
						deleteCalledTimes = 0
						ensureFinalizerAbsentCalledTimes = 1
					})
					It("should skip next steps and stop processing", func() {
						// Act
						res, err := routeMonitorReconciler.EnsureRouteMonitorAndDependenciesAbsent(ctx, routeMonitor)
						// Assert
						Expect(err).NotTo(HaveOccurred())
						Expect(res).To(Equal(utilreconcile.StopOperation()))
					})
				})
				When("func 'Delete' fails unexpectedly", func() {
					// Arrange
					BeforeEach(func() {
						deleteErrorResponse = consterror.CustomError
					})
					It("Should bubble up the failure", func() {
						// Act
						_, err := routeMonitorReconciler.EnsureRouteMonitorAndDependenciesAbsent(ctx, routeMonitor)
						// Assert
						Expect(err).To(HaveOccurred())
						Expect(err).To(MatchError(consterror.CustomError))
					})
				})
				When("when the 'Delete' succeeds", func() {
					BeforeEach(func() {
						ensureFinalizerAbsentCalledTimes = 1
					})
					// Arrange
					It("should succeed and stop processing", func() {
						// Act
						res, err := routeMonitorReconciler.EnsureRouteMonitorAndDependenciesAbsent(ctx, routeMonitor)
						// Assert
						Expect(err).NotTo(HaveOccurred())
						Expect(res).To(Equal(utilreconcile.StopOperation()))

					})
				})
			})
		})
	})
	//
	Describe("EnsureBlackBoxExporterResourcesExists", func() {
		BeforeEach(func() {
			// Arrange
			ensureBlackBoxExporterDeploymentExistsCalledTimes = 1
		})
		When("func EnsureBlackBoxExporterDeploymentExists fails unexpectedly", func() {
			BeforeEach(func() {
				// Arrange
				ensureBlackBoxExporterDeploymentExistsErrorResponse = consterror.CustomError
			})
			It("should bubble up the error", func() {
				// Act
				err := routeMonitorReconciler.EnsureBlackBoxExporterResourcesExists(ctx)
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(consterror.CustomError))
			})
		})
		When("func EnsureBlackBoxExporterServiceExists fails unexpectedly", func() {
			BeforeEach(func() {
				// Arrange
				ensureBlackBoxExporterServiceExistsErrorResponse = consterror.CustomError

				ensureBlackBoxExporterServiceExistsCalledTimes = 1
			})
			It("should bubble up the error", func() {
				// Act
				err := routeMonitorReconciler.EnsureBlackBoxExporterResourcesExists(ctx)
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(consterror.CustomError))
			})
		})
		When("func EnsureBlackBoxExporterServiceExists fails unexpectedly", func() {
			BeforeEach(func() {
				// Arrange
				ensureBlackBoxExporterServiceExistsCalledTimes = 1
			})
			It("should succeed with no error", func() {
				// Act
				err := routeMonitorReconciler.EnsureBlackBoxExporterResourcesExists(ctx)
				// Assert
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

})
