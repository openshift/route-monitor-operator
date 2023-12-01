package blackboxexporter_test

import (
	"context"
	"time"

	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	"github.com/openshift/route-monitor-operator/pkg/consts/blackboxexporter"
	consterror "github.com/openshift/route-monitor-operator/pkg/consts/test/error"
	constinit "github.com/openshift/route-monitor-operator/pkg/consts/test/init"
	clientmocks "github.com/openshift/route-monitor-operator/pkg/util/test/generated/mocks/client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/openshift/route-monitor-operator/pkg/blackboxexporter"
	"github.com/openshift/route-monitor-operator/pkg/util/test/helper"
)

var _ = Describe("Blackboxexporter", func() {
	var (
		mockClient *clientmocks.MockClient
		mockCtrl   *gomock.Controller

		blackboxExporter BlackBoxExporter

		ctx context.Context

		get    helper.MockHelper
		delete helper.MockHelper
		create helper.MockHelper
		list   helper.MockHelper
	)
	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockClient = clientmocks.NewMockClient(mockCtrl)

		ctx = constinit.Context

		get = helper.MockHelper{}
		delete = helper.MockHelper{}
		create = helper.MockHelper{}
		list = helper.MockHelper{}
	})
	JustBeforeEach(func() {
		blackboxExporter = BlackBoxExporter{
			Log:    constinit.Logger,
			Client: mockClient,
			Ctx:    ctx,
		}

		mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(get.ErrorResponse).
			Times(get.CalledTimes)

		mockClient.EXPECT().Delete(gomock.Any(), gomock.Any()).
			Return(delete.ErrorResponse).
			Times(delete.CalledTimes)

		mockClient.EXPECT().Create(gomock.Any(), gomock.Any()).
			Return(create.ErrorResponse).
			Times(create.CalledTimes)
	})
	AfterEach(func() {
		mockCtrl.Finish()
	})
	Describe("DeleteBlackBoxExporterService", func() {
		BeforeEach(func() {
			get.CalledTimes = 1
		})

		When("'Get' return an error", func() {
			// Arrange
			BeforeEach(func() {
				get.ErrorResponse = consterror.CustomError
			})
			It("should bubble the error up", func() {
				// Act
				err := blackboxExporter.EnsureBlackBoxExporterServiceAbsent()
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(consterror.CustomError))
			})
		})

		When("'Get' return an 'NotFound' error", func() {
			// Arrange
			BeforeEach(func() {
				get.ErrorResponse = consterror.NotFoundErr
			})
			It("should do nothing", func() {
				// Act
				err := blackboxExporter.EnsureBlackBoxExporterServiceAbsent()
				// Assert
				Expect(err).NotTo(HaveOccurred())
			})
		})

		When("'Delete' return an an  error", func() {
			// Arrange
			BeforeEach(func() {
				delete = helper.CustomErrorHappensOnce()
			})
			It("should do nothing", func() {
				// Act
				err := blackboxExporter.EnsureBlackBoxExporterServiceAbsent()
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(consterror.CustomError))
			})
		})

		When("'Delete' succeeds", func() {
			// Arrange
			BeforeEach(func() {
				delete.CalledTimes = 1
			})
			It("should succeed", func() {
				// Act
				err := blackboxExporter.EnsureBlackBoxExporterServiceAbsent()
				// Assert
				Expect(err).NotTo(HaveOccurred())
			})
		})

	})
	Describe("DeleteBlackBoxExporterDeployment", func() {
		BeforeEach(func() {
			get.CalledTimes = 1
		})

		When("'Get' return an error", func() {
			// Arrange
			BeforeEach(func() {
				get.ErrorResponse = consterror.CustomError
			})
			It("should bubble the error up", func() {
				// Act
				err := blackboxExporter.EnsureBlackBoxExporterDeploymentAbsent()
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(consterror.CustomError))
			})
		})

		When("'Get' return an 'NotFound' error", func() {
			// Arrange
			BeforeEach(func() {
				get.ErrorResponse = consterror.NotFoundErr
			})
			It("should do nothing", func() {
				// Act
				err := blackboxExporter.EnsureBlackBoxExporterDeploymentAbsent()
				// Assert
				Expect(err).NotTo(HaveOccurred())
			})
		})

		When("'Delete' return an an  error", func() {
			// Arrange
			BeforeEach(func() {
				delete = helper.CustomErrorHappensOnce()
			})
			It("should do nothing", func() {
				// Act
				err := blackboxExporter.EnsureBlackBoxExporterDeploymentAbsent()
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(consterror.CustomError))
			})
		})

		When("'Delete' succeeds", func() {
			// Arrange
			BeforeEach(func() {
				delete.CalledTimes = 1
			})
			It("should succeed", func() {
				// Act
				err := blackboxExporter.EnsureBlackBoxExporterDeploymentAbsent()
				// Assert
				Expect(err).NotTo(HaveOccurred())
			})
		})

	})
	Describe("CreateBlackBoxExporterDeployment", func() {
		BeforeEach(func() {
			// Arrange
			get.CalledTimes = 2
		})

		When("the resource(deployment) is Not Found", func() {
			// Arrange
			BeforeEach(func() {
				get.ErrorResponse = consterror.NotFoundErr
				create.CalledTimes = 1
			})
			It("should call `Get` successfully and `Create` the resource(deployment)", func() {
				// Act
				err := blackboxExporter.EnsureBlackBoxExporterDeploymentExists()
				// Assert
				Expect(err).NotTo(HaveOccurred())
			})
		})
		When("the resource(deployment) Get fails unexpectedly", func() {
			// Arrange
			BeforeEach(func() {
				get.ErrorResponse = consterror.CustomError
			})
			It("should return the error and not call `Create`", func() {
				// Act
				err := blackboxExporter.EnsureBlackBoxExporterDeploymentExists()
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(consterror.CustomError))
			})
		})
		When("the resource(deployment) Create fails unexpectedly", func() {
			// Arrange
			BeforeEach(func() {
				get.ErrorResponse = consterror.NotFoundErr
				create = helper.CustomErrorHappensOnce()
			})
			It("should call `Get` Successfully and call `Create` but return the error", func() {
				// Act
				err := blackboxExporter.EnsureBlackBoxExporterDeploymentExists()
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(consterror.CustomError))
			})
		})
	})
	Describe("CreateBlackBoxExporterService", func() {

		When("the resource(service) Exists", func() {
			// Arrange
			BeforeEach(func() {
				get.CalledTimes = 1
			})
			It("should call `Get` and not call `Create`", func() {
				// Act
				err := blackboxExporter.EnsureBlackBoxExporterServiceExists()
				// Assert
				Expect(err).NotTo(HaveOccurred())

			})
		})
		When("the resource(service) is Not Found", func() {
			// Arrange
			BeforeEach(func() {
				get = helper.NotFoundErrorHappensOnce()
				create.CalledTimes = 1
			})
			It("should call `Get` successfully and `Create` the resource(service)", func() {
				// Act
				err := blackboxExporter.EnsureBlackBoxExporterServiceExists()
				// Assert
				Expect(err).NotTo(HaveOccurred())
			})
		})
		When("the resource(service) Get fails unexpectedly", func() {
			// Arrange
			BeforeEach(func() {
				get = helper.CustomErrorHappensOnce()
			})
			It("should return the error and not call `Create`", func() {
				// Act
				err := blackboxExporter.EnsureBlackBoxExporterServiceExists()
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(consterror.CustomError))
			})
		})
		When("the resource(service) Create fails unexpectedly", func() {
			// Arrange
			BeforeEach(func() {
				get = helper.NotFoundErrorHappensOnce()
				create = helper.CustomErrorHappensOnce()
			})
			It("should call `Get` Successfully and call `Create` but return the error", func() {
				// Act
				err := blackboxExporter.EnsureBlackBoxExporterServiceExists()
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(consterror.CustomError))
			})
		})
	})
	Describe("ShouldDeleteBlackBoxExporterResources", func() {
		var (
			routeMonitor       v1alpha1.RouteMonitor
			routeMonitors      v1alpha1.RouteMonitorList
			clusterUrlMonitors v1alpha1.ClusterUrlMonitorList
		)
		JustBeforeEach(func() {
			gomock.InOrder(
				mockClient.EXPECT().List(gomock.Any(), gomock.Any()).Return(list.ErrorResponse).SetArg(1, routeMonitors).Times(1),
				mockClient.EXPECT().List(gomock.Any(), gomock.Any()).Return(list.ErrorResponse).SetArg(1, clusterUrlMonitors).AnyTimes(),
			)
		})
		BeforeEach(func() {
			list.CalledTimes = 2
		})

		JustBeforeEach(func() {
			routeMonitor.DeletionTimestamp = &metav1.Time{Time: time.Unix(0, 0)}
		})
		When("the `List` command fails", func() {
			// Arrange
			BeforeEach(func() {
				list = helper.CustomErrorHappensOnce()
			})
			It("should fail with the List error", func() {
				// Act
				_, err := blackboxExporter.ShouldDeleteBlackBoxExporterResources()
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(consterror.CustomError))
			})
		})

		When("there are many RouteMonitors", func() {
			var (
				routeMonitorSecond v1alpha1.RouteMonitor
			)

			BeforeEach(func() {
				routeMonitor.ObjectMeta = metav1.ObjectMeta{
					Name:      "fake-name",
					Namespace: "fake-namespace",
				}
				routeMonitorSecond.ObjectMeta = metav1.ObjectMeta{
					Name:      routeMonitor.Name + "-but-different",
					Namespace: routeMonitor.Namespace,
				}
				routeMonitors.Items = []v1alpha1.RouteMonitor{
					routeMonitor,
					routeMonitorSecond,
				}
			})
			It("should return 'false' for too many RouteMonitors", func() {
				// Act
				res, err := blackboxExporter.ShouldDeleteBlackBoxExporterResources()
				// Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(res).To(Equal(blackboxexporter.KeepBlackBoxExporter))

			})
		})

		When("there is just one RouteMonitor, and it's being deleted", func() {
			BeforeEach(func() {
				clusterUrlMonitors.Items = []v1alpha1.ClusterUrlMonitor{}
				routeMonitors.Items = []v1alpha1.RouteMonitor{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:              "fake-route-monitor",
							Namespace:         "fake-route-monitor-namespace",
							DeletionTimestamp: &metav1.Time{Time: time.Unix(0, 0)},
						},
					},
				}
			})
			// Arrange
			It("should return 'true'", func() {
				// Act
				res, err := blackboxExporter.ShouldDeleteBlackBoxExporterResources()
				// Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(res).To(Equal(blackboxexporter.DeleteBlackBoxExporter))
			})

		})
	})

})
