package deleter_test

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"context"
	"time"

	//"reflect"
	"github.com/openshift/route-monitor-operator/controllers/routemonitor/deleter"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openshift/route-monitor-operator/pkg/const/blackbox"
	consterror "github.com/openshift/route-monitor-operator/pkg/const/test/error"
	constinit "github.com/openshift/route-monitor-operator/pkg/const/test/init"
	clientmocks "github.com/openshift/route-monitor-operator/pkg/util/test/generated/mocks/client"

	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/route-monitor-operator/controllers/routemonitor"
)

var _ = Describe("Deleter", func() {
	var (
		mockClient *clientmocks.MockClient
		mockCtrl   *gomock.Controller

		routeMonitorDeleter       deleter.RouteMonitorDeleter
		routeMonitorDeleterClient client.Client

		ctx context.Context

		getCalledTimes      int
		getErrorResponse    error
		deleteCalledTimes   int
		deleteErrorResponse error
		listErrorResponse   error
		listCalledTimes     int

		// routeMonitor here is only used to query for the ServiceMonitor
		routeMonitor v1alpha1.RouteMonitor
	)
	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockClient = clientmocks.NewMockClient(mockCtrl)

		routeMonitorDeleterClient = mockClient

		ctx = constinit.Context

		getCalledTimes = 0
		getErrorResponse = nil
		deleteCalledTimes = 0
		deleteErrorResponse = nil
		listCalledTimes = 0
		listErrorResponse = nil
	})
	JustBeforeEach(func() {
		routeMonitorDeleter = deleter.RouteMonitorDeleter{
			Log:    constinit.Logger,
			Client: routeMonitorDeleterClient,
			Scheme: constinit.Scheme,
		}

		mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(getErrorResponse).
			Times(getCalledTimes)

		mockClient.EXPECT().Delete(gomock.Any(), gomock.Any()).
			Return(deleteErrorResponse).
			Times(deleteCalledTimes)

		mockClient.EXPECT().List(gomock.Any(), gomock.Any()).
			Return(listErrorResponse).
			Times(listCalledTimes)
	})
	AfterEach(func() {
		mockCtrl.Finish()
	})
	Describe("DeleteBlackBoxExporterDeployment", func() {
		BeforeEach(func() {
			getCalledTimes = 1
			routeMonitorDeleterClient = mockClient
		})

		When("'Get' return an error", func() {
			// Arrange
			BeforeEach(func() {
				getErrorResponse = consterror.CustomError
			})
			It("should bubble the error up", func() {
				// Act
				err := routeMonitorDeleter.EnsureBlackBoxExporterDeploymentAbsent(ctx)
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(consterror.CustomError))
			})
		})

		When("'Get' return an 'NotFound' error", func() {
			// Arrange
			BeforeEach(func() {
				getErrorResponse = consterror.NotFoundErr
			})
			It("should do nothing", func() {
				// Act
				err := routeMonitorDeleter.EnsureBlackBoxExporterDeploymentAbsent(ctx)
				// Assert
				Expect(err).NotTo(HaveOccurred())
			})
		})

		When("'Delete' return an an  error", func() {
			// Arrange
			BeforeEach(func() {
				deleteCalledTimes = 1
				deleteErrorResponse = consterror.CustomError
			})
			It("should do nothing", func() {
				// Act
				err := routeMonitorDeleter.EnsureBlackBoxExporterDeploymentAbsent(ctx)
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(consterror.CustomError))
			})
		})

		When("'Delete' succeeds", func() {
			// Arrange
			BeforeEach(func() {
				deleteCalledTimes = 1
			})
			It("should succeed", func() {
				// Act
				err := routeMonitorDeleter.EnsureBlackBoxExporterDeploymentAbsent(ctx)
				// Assert
				Expect(err).NotTo(HaveOccurred())
			})
		})

	})
	Describe("DeleteBlackBoxExporterService", func() {
		BeforeEach(func() {
			getCalledTimes = 1
			routeMonitorDeleterClient = mockClient
		})

		When("'Get' return an error", func() {
			// Arrange
			BeforeEach(func() {
				getErrorResponse = consterror.CustomError
			})
			It("should bubble the error up", func() {
				// Act
				err := routeMonitorDeleter.EnsureBlackBoxExporterServiceAbsent(ctx)
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(consterror.CustomError))
			})
		})

		When("'Get' return an 'NotFound' error", func() {
			// Arrange
			BeforeEach(func() {
				getErrorResponse = consterror.NotFoundErr
			})
			It("should do nothing", func() {
				// Act
				err := routeMonitorDeleter.EnsureBlackBoxExporterServiceAbsent(ctx)
				// Assert
				Expect(err).NotTo(HaveOccurred())
			})
		})

		When("'Delete' return an an  error", func() {
			// Arrange
			BeforeEach(func() {
				deleteCalledTimes = 1
				deleteErrorResponse = consterror.CustomError
			})
			It("should do nothing", func() {
				// Act
				err := routeMonitorDeleter.EnsureBlackBoxExporterServiceAbsent(ctx)
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(consterror.CustomError))
			})
		})

		When("'Delete' succeeds", func() {
			// Arrange
			BeforeEach(func() {
				deleteCalledTimes = 1
			})
			It("should succeed", func() {
				// Act
				err := routeMonitorDeleter.EnsureBlackBoxExporterServiceAbsent(ctx)
				// Assert
				Expect(err).NotTo(HaveOccurred())
			})
		})

	})

	Describe("DeleteServiceMonitorResource", func() {
		BeforeEach(func() {
			getCalledTimes = 1
			routeMonitorDeleterClient = mockClient

		})
		When("'Get' returns an unhandled error", func() {
			// Arrange
			BeforeEach(func() {
				getErrorResponse = consterror.CustomError
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
				getErrorResponse = consterror.NotFoundErr
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
				deleteCalledTimes = 1
				deleteErrorResponse = consterror.CustomError
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
				deleteCalledTimes = 1
			})
			It("should succeed as the object was deleted", func() {
				// Act
				err := routeMonitorDeleter.EnsureServiceMonitorResourceAbsent(ctx, v1alpha1.RouteMonitor{})
				// Assert
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
	Describe("ShouldDeleteBlackBoxExporterResources", func() {
		var scheme = constinit.Scheme

		JustBeforeEach(func() {
			routeMonitor.DeletionTimestamp = &metav1.Time{Time: time.Unix(0, 0)}
		})
		When("a delete was not requested (user did nothing)", func() {
			JustBeforeEach(func() {
				routeMonitor.DeletionTimestamp = nil
			})
			// Arrange
			It("should stop early and return 'false'", func() {
				// Act
				res, err := routeMonitorDeleter.ShouldDeleteBlackBoxExporterResources(ctx, routeMonitor)
				// Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(res).To(Equal(blackbox.KeepBlackBoxExporter))

			})
		})
		When("the `List` command fails", func() {
			// Arrange
			BeforeEach(func() {
				listCalledTimes = 1
				listErrorResponse = consterror.CustomError
				routeMonitorDeleterClient = mockClient
			})
			It("should fail with the List error", func() {
				// Act
				_, err := routeMonitorDeleter.ShouldDeleteBlackBoxExporterResources(ctx, routeMonitor)
				//Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(consterror.CustomError))
			})
		})
		When("there are no RouteMonitors", func() {
			BeforeEach(func() {
				listCalledTimes = 1
			})
			It("should technically return  'true' but return InternalFault error", func() {
				// Act
				res, err := routeMonitorDeleter.ShouldDeleteBlackBoxExporterResources(ctx, routeMonitor)
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(HavePrefix("Internal Fault:"))

				// is true because it should delete even if there are no items.
				// but this is in unusual situation because:
				// - a delete was requested
				// - there are no items of that resource on the cluster
				Expect(res).To(Equal(blackbox.KeepBlackBoxExporter))

			})
		})

		When("there are many RouteMonitors", func() {
			var routeMonitorSecond = v1alpha1.RouteMonitor{}

			BeforeEach(func() {
				routeMonitor.ObjectMeta = metav1.ObjectMeta{
					Name:      "fake-name",
					Namespace: "fake-namespace",
				}
				routeMonitorSecond.ObjectMeta = metav1.ObjectMeta{
					Name:      routeMonitor.Name + "-but-different",
					Namespace: routeMonitor.Namespace,
				}

				routeMonitorDeleterClient = fake.NewFakeClientWithScheme(scheme, &routeMonitor, &routeMonitorSecond)
			})
			It("should return 'false' for too many RouteMonitors", func() {
				// Act
				res, err := routeMonitorDeleter.ShouldDeleteBlackBoxExporterResources(ctx, routeMonitor)
				// Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(res).To(Equal(blackbox.KeepBlackBoxExporter))

			})
		})

		When("there is just one RouteMonitor", func() {
			// Arrange
			BeforeEach(func() {
				routeMonitorDeleterClient = fake.NewFakeClientWithScheme(scheme, &routeMonitor)
			})
			It("should return 'true'", func() {
				// Act
				res, err := routeMonitorDeleter.ShouldDeleteBlackBoxExporterResources(ctx, routeMonitor)
				// Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(res).To(Equal(blackbox.DeleteBlackBoxExporter))
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

})
