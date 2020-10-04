package deleter_test

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"context"

	"github.com/openshift/route-monitor-operator/controllers/routemonitor/deleter"

	"sigs.k8s.io/controller-runtime/pkg/client"

	consterror "github.com/openshift/route-monitor-operator/pkg/const/test/error"
	constinit "github.com/openshift/route-monitor-operator/pkg/const/test/init"
	clientmocks "github.com/openshift/route-monitor-operator/pkg/util/tests/generated/mocks/client"

	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	//metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
				err := routeMonitorDeleter.DeleteBlackBoxExporterDeployment(ctx)
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
				err := routeMonitorDeleter.DeleteBlackBoxExporterDeployment(ctx)
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
				err := routeMonitorDeleter.DeleteBlackBoxExporterDeployment(ctx)
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
				err := routeMonitorDeleter.DeleteBlackBoxExporterDeployment(ctx)
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
				err := routeMonitorDeleter.DeleteBlackBoxExporterService(ctx)
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
				err := routeMonitorDeleter.DeleteBlackBoxExporterService(ctx)
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
				err := routeMonitorDeleter.DeleteBlackBoxExporterService(ctx)
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
				err := routeMonitorDeleter.DeleteBlackBoxExporterService(ctx)
				// Assert
				Expect(err).NotTo(HaveOccurred())
			})
		})

	})

	Describe("DeleteServiceMonitorResource", func() {
		var (
			// routeMonitor here is only used to query for the ServiceMonitor
			routeMonitor v1alpha1.RouteMonitor
		)
		BeforeEach(func() {
			getCalledTimes = 1
			routeMonitorDeleterClient = mockClient
			//routeMonitor = v1alpha1.RouteMonitor{
			//	ObjectMeta: metav1.ObjectMeta{
			//		Name:      "fake-name",
			//		Namespace: "fake-namespace",
			//	},
			//}

		})
		When("'Get' returns an unhandled error", func() {
			// Arrange
			BeforeEach(func() {
				getErrorResponse = consterror.CustomError
			})
			It("should bubble the error up", func() {
				// Act
				err := routeMonitorDeleter.DeleteServiceMonitorResource(ctx, routeMonitor)
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
				err := routeMonitorDeleter.DeleteServiceMonitorResource(ctx, routeMonitor)
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
				err := routeMonitorDeleter.DeleteServiceMonitorResource(ctx, routeMonitor)
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
				err := routeMonitorDeleter.DeleteServiceMonitorResource(ctx, routeMonitor)
				// Assert
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

})
