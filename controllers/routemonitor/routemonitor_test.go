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
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	routemonitorconst "github.com/openshift/route-monitor-operator/pkg/consts"
	"github.com/openshift/route-monitor-operator/pkg/consts/blackboxexporter"
	consterror "github.com/openshift/route-monitor-operator/pkg/consts/test/error"
	constinit "github.com/openshift/route-monitor-operator/pkg/consts/test/init"
	customerrors "github.com/openshift/route-monitor-operator/pkg/util/errors"
	utilreconcile "github.com/openshift/route-monitor-operator/pkg/util/reconcile"
	clientmocks "github.com/openshift/route-monitor-operator/pkg/util/test/generated/mocks/client"
	routemonitormocks "github.com/openshift/route-monitor-operator/pkg/util/test/generated/mocks/routemonitor"
	"github.com/openshift/route-monitor-operator/pkg/util/test/helper"

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
		mockBlackboxExporter         *routemonitormocks.MockBlackBoxExporter
		ctx                          context.Context

		update                                helper.MockHelper
		delete                                helper.MockHelper
		get                                   helper.MockHelper
		create                                helper.MockHelper
		ensureServiceMonitorResourceAbsent    helper.MockHelper
		ensurePrometheusRuleResourceAbsent    helper.MockHelper
		shouldDeleteBlackBoxExporterResources helper.MockHelper
		ensureBlackBoxExporterResourcesAbsent helper.MockHelper
		ensureBlackBoxExporterResourcesExist  helper.MockHelper
		ensureFinalizerAbsent                 helper.MockHelper

		shouldDeleteBlackBoxExporterResourcesResponse blackboxexporter.ShouldDeleteBlackBoxExporter
		ensureFinalizerAbsentResponse                 utilreconcile.Result

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
		mockBlackboxExporter = routemonitormocks.NewMockBlackBoxExporter(mockCtrl)
		routeMonitorFinalizers = routemonitorconst.FinalizerList

		routeMonitorReconcilerClient = mockClient

		ctx = constinit.Context

		update = helper.MockHelper{}
		delete = helper.MockHelper{}
		get = helper.MockHelper{}
		create = helper.MockHelper{}
		ensureServiceMonitorResourceAbsent = helper.MockHelper{}
		ensurePrometheusRuleResourceAbsent = helper.MockHelper{}
		shouldDeleteBlackBoxExporterResources = helper.MockHelper{}
		ensureBlackBoxExporterResourcesAbsent = helper.MockHelper{}
		ensureBlackBoxExporterResourcesExist = helper.MockHelper{}
		ensureFinalizerAbsent = helper.MockHelper{}
		shouldDeleteBlackBoxExporterResourcesResponse = blackboxexporter.KeepBlackBoxExporter

		ensureFinalizerAbsentResponse = utilreconcile.Result{}

	})
	JustBeforeEach(func() {
		mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(get.ErrorResponse).
			Times(get.CalledTimes)

		mockClient.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(update.ErrorResponse).
			Times(update.CalledTimes)

		mockClient.EXPECT().Delete(gomock.Any(), gomock.Any()).
			Return(delete.ErrorResponse).
			Times(delete.CalledTimes)

		mockClient.EXPECT().Create(gomock.Any(), gomock.Any()).
			Return(create.ErrorResponse).
			Times(create.CalledTimes)

		mockDeleter.EXPECT().EnsureServiceMonitorResourceAbsent(gomock.Any(), gomock.Any()).
			Return(ensureServiceMonitorResourceAbsent.ErrorResponse).
			Times(ensureServiceMonitorResourceAbsent.CalledTimes)

		mockDeleter.EXPECT().EnsurePrometheusRuleResourceAbsent(gomock.Any(), gomock.Any()).
			Return(ensurePrometheusRuleResourceAbsent.ErrorResponse).
			Times(ensurePrometheusRuleResourceAbsent.CalledTimes)

		mockBlackboxExporter.EXPECT().EnsureBlackBoxExporterResourcesAbsent().
			Times(ensureBlackBoxExporterResourcesAbsent.CalledTimes).
			Return(ensureBlackBoxExporterResourcesAbsent.ErrorResponse)

		mockBlackboxExporter.EXPECT().ShouldDeleteBlackBoxExporterResources().
			Times(shouldDeleteBlackBoxExporterResources.CalledTimes).
			Return(shouldDeleteBlackBoxExporterResourcesResponse, shouldDeleteBlackBoxExporterResources.ErrorResponse)

		mockBlackboxExporter.EXPECT().EnsureBlackBoxExporterResourcesExist().
			Times(ensureBlackBoxExporterResourcesExist.CalledTimes).
			Return(ensureBlackBoxExporterResourcesExist.ErrorResponse)

		mockSupplement.EXPECT().EnsureFinalizerAbsent(gomock.Any(), gomock.Any()).
			Times(ensureFinalizerAbsent.CalledTimes).
			Return(ensureFinalizerAbsentResponse, ensureFinalizerAbsent.ErrorResponse)

		routeMonitorReconciler = routemonitor.RouteMonitorReconciler{
			Log:                    constinit.Logger,
			Client:                 routeMonitorReconcilerClient,
			Scheme:                 constinit.Scheme,
			RouteMonitorSupplement: mockSupplement,
			RouteMonitorDeleter:    mockDeleter,
			RouteMonitorAdder:      mockAdder,
			BlackBoxExporter:       mockBlackboxExporter,
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

	Describe("EnsurePrometheusRuleResourceExists", func() {
		var (
			routeMonitorSlo  v1alpha1.SloSpec
			mockStatusWriter *clientmocks.MockStatusWriter
		)
		BeforeEach(func() {
			routeMonitorSlo = v1alpha1.SloSpec{}
			mockStatusWriter = clientmocks.NewMockStatusWriter(mockCtrl)

			routeMonitor.Spec = v1alpha1.RouteMonitorSpec{
				Slo: routeMonitorSlo,
			}
		})
		JustBeforeEach(func() {
			routeMonitor.Spec = v1alpha1.RouteMonitorSpec{
				Slo: routeMonitorSlo,
			}
		})
		When("the RouteMonitor has no Host", func() {
			// Arrange
			BeforeEach(func() {
				scheme := constinit.Scheme
				routeMonitorReconcilerClient = fake.NewFakeClientWithScheme(scheme)
				routeMonitorStatus = v1alpha1.RouteMonitorStatus{}
			})
			It("should return No Host error", func() {
				// Act
				_, err := routeMonitorReconciler.EnsurePrometheusRuleResourceExists(ctx, routeMonitor)
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(customerrors.NoHost))
			})
		})
		When("the RouteMonitor has no slo spec", func() {
			// Arrange
			BeforeEach(func() {
				routeMonitorReconcilerClient = mockClient
				routeMonitorStatus = v1alpha1.RouteMonitorStatus{
					RouteURL: "fake-route-url",
				}
				ensurePrometheusRuleResourceAbsent.CalledTimes = 1
			})
			It("should skip processing and continue", func() {
				// Act
				res, err := routeMonitorReconciler.EnsurePrometheusRuleResourceExists(ctx, routeMonitor)
				// Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(res).NotTo(BeNil())
				Expect(res).To(Equal(utilreconcile.ContinueOperation()))
			})
		})
		When("the RouteMonitor has a slo spec but percent is too low", func() {
			// Arrange
			BeforeEach(func() {
				routeMonitorReconcilerClient = mockClient
				routeMonitorStatus = v1alpha1.RouteMonitorStatus{
					RouteURL: "fake-route-url",
				}
				routeMonitorSlo = v1alpha1.SloSpec{
					TargetAvailabilityPercent: "-0/10",
				}
			})
			It("should Throw an error", func() {
				// Act
				_, err := routeMonitorReconciler.EnsurePrometheusRuleResourceExists(ctx, routeMonitor)
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(customerrors.InvalidSLO))
			})
		})
		When("the RouteMonitor has a slo spec but percent is too high", func() {
			// Arrange
			BeforeEach(func() {
				routeMonitorReconcilerClient = mockClient
				routeMonitorStatus = v1alpha1.RouteMonitorStatus{
					RouteURL: "fake-route-url",
				}
				routeMonitorSlo = v1alpha1.SloSpec{
					TargetAvailabilityPercent: "101",
				}
			})
			It("should Throw an error", func() {
				// Act
				_, err := routeMonitorReconciler.EnsurePrometheusRuleResourceExists(ctx, routeMonitor)
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(customerrors.InvalidSLO))
			})
		})
		When("the RouteMonitor has an empty slo value", func() {
			// Arrange
			BeforeEach(func() {
				routeMonitorReconcilerClient = mockClient
				routeMonitorStatus = v1alpha1.RouteMonitorStatus{
					RouteURL: "fake-route-url",
				}
				routeMonitorSlo = v1alpha1.SloSpec{}
				ensurePrometheusRuleResourceAbsent.CalledTimes = 1
			})
			It("should do nothing", func() {
				// Act
				res, err := routeMonitorReconciler.EnsurePrometheusRuleResourceExists(ctx, routeMonitor)
				// Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(res).NotTo(BeNil())
				Expect(res).To(Equal(utilreconcile.ContinueOperation()))
			})
		})
		When("the RouteMonitor has invalid slo type", func() {
			// Arrange
			BeforeEach(func() {
				routeMonitorReconcilerClient = mockClient
				routeMonitorStatus = v1alpha1.RouteMonitorStatus{
					RouteURL: "fake-route-url",
				}
				routeMonitorSlo = v1alpha1.SloSpec{
					TargetAvailabilityPercent: "fake-slo-type",
				}
			})
			It("should Throw an error", func() {
				// Act
				_, err := routeMonitorReconciler.EnsurePrometheusRuleResourceExists(ctx, routeMonitor)
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(customerrors.InvalidSLO))
			})
		})
		When("the resource Exists", func() {
			BeforeEach(func() {
				routeMonitorReconcilerClient = mockClient
				routeMonitorStatus = v1alpha1.RouteMonitorStatus{
					RouteURL: "fake-route-url",
				}
				routeMonitorSlo = v1alpha1.SloSpec{
					TargetAvailabilityPercent: "99.95",
				}
				routeMonitorFinalizers = routemonitorconst.FinalizerList
				mockClient.EXPECT().Status().Return(mockStatusWriter).Times(1)
				get.CalledTimes = 1
			})

			JustBeforeEach(func() {
				mockStatusWriter.EXPECT().Update(gomock.Any(), gomock.Any()).
					Times(1).
					Return(nil)
				routeMonitor.Name = "rmo-name"
				routeMonitor.Namespace = "rmo-namespace"
			})

			It("should call `Get` and `Update` and not call `Create` and stop reconciling", func() {
				//Act
				resp, err := routeMonitorReconciler.EnsurePrometheusRuleResourceExists(ctx, routeMonitor)
				//Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(resp).NotTo(BeNil())
				Expect(resp).To(Equal(utilreconcile.StopOperation()))
			})
		})
		When("the EnsurePrometheusRuleResourceExists should pass all checks", func() {
			BeforeEach(func() {
				routeMonitorReconcilerClient = mockClient
				routeMonitorStatus = v1alpha1.RouteMonitorStatus{
					RouteURL: "fake-route-url",
				}
				routeMonitorSlo = v1alpha1.SloSpec{
					TargetAvailabilityPercent: "99.95",
				}
				routeMonitorFinalizers = routemonitorconst.FinalizerList
				get.CalledTimes = 1
			})

			JustBeforeEach(func() {
				routeMonitor.Status.PrometheusRuleRef = v1alpha1.NamespacedName{
					Name:      routeMonitor.Name,
					Namespace: routeMonitor.Namespace,
				}
			})

			It("should continue reconciling", func() {
				//Act
				resp, err := routeMonitorReconciler.EnsurePrometheusRuleResourceExists(ctx, routeMonitor)
				//Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(resp).NotTo(BeNil())
				Expect(resp).To(Equal(utilreconcile.ContinueOperation()))
			})
		})
	})

	Describe("EnsureRouteMonitorAndDependenciesAbsent", func() {
		BeforeEach(func() {
			// Arrange
			routeMonitorReconcilerClient = mockClient
			shouldDeleteBlackBoxExporterResources.CalledTimes = 1
		})
		When("func ShouldDeleteBlackBoxExporterResources fails unexpectedly", func() {
			BeforeEach(func() {
				// Arrange
				shouldDeleteBlackBoxExporterResources.ErrorResponse = consterror.CustomError
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
				shouldDeleteBlackBoxExporterResourcesResponse = blackboxexporter.DeleteBlackBoxExporter
				ensureBlackBoxExporterResourcesAbsent.CalledTimes = 1

			})

			When("func EnsureBlackBoxExporterServiceAbsent fails unexpectedly", func() {
				BeforeEach(func() {
					// Arrange
					ensureBlackBoxExporterResourcesAbsent.ErrorResponse = consterror.CustomError
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
					ensureBlackBoxExporterResourcesAbsent = helper.CustomErrorHappensOnce()
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
					ensureBlackBoxExporterResourcesAbsent.CalledTimes = 1
					ensureServiceMonitorResourceAbsent = helper.CustomErrorHappensOnce()
				})
				It("should bubble up the error", func() {
					// Act
					_, err := routeMonitorReconciler.EnsureRouteMonitorAndDependenciesAbsent(ctx, routeMonitor)
					// Assert
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(consterror.CustomError))
				})
			})
			When("func EnsurePrometheusRuleResourceAbsent fails unexpectedly", func() {
				BeforeEach(func() {
					ensureBlackBoxExporterResourcesAbsent.CalledTimes = 1
					ensureServiceMonitorResourceAbsent.CalledTimes = 1
					ensurePrometheusRuleResourceAbsent = helper.CustomErrorHappensOnce()
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
					ensureBlackBoxExporterResourcesAbsent.CalledTimes = 1
					ensureServiceMonitorResourceAbsent.CalledTimes = 1
					ensurePrometheusRuleResourceAbsent.CalledTimes = 1
					ensureFinalizerAbsent = helper.CustomErrorHappensOnce()
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
					ensureBlackBoxExporterResourcesAbsent.CalledTimes = 1
					ensureServiceMonitorResourceAbsent.CalledTimes = 1
					ensurePrometheusRuleResourceAbsent.CalledTimes = 1
					ensureFinalizerAbsent.CalledTimes = 1
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
				shouldDeleteBlackBoxExporterResourcesResponse = blackboxexporter.KeepBlackBoxExporter
				ensureServiceMonitorResourceAbsent.CalledTimes = 1
				ensurePrometheusRuleResourceAbsent.CalledTimes = 1
			})
			When("func EnsureServiceMonitorResourceAbsent fails unexpectedly", func() {
				BeforeEach(func() {
					// Arrange
					ensureServiceMonitorResourceAbsent.ErrorResponse = consterror.CustomError
					ensurePrometheusRuleResourceAbsent.CalledTimes = 0
				})
				It("should bubble up the error", func() {
					// Act
					_, err := routeMonitorReconciler.EnsureRouteMonitorAndDependenciesAbsent(ctx, routeMonitor)
					// Assert
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(consterror.CustomError))
				})
			})

			When("func EnsurePrometheusRuleResourceAbsent fails unexpectedly", func() {
				BeforeEach(func() {
					// Arrange
					ensurePrometheusRuleResourceAbsent.ErrorResponse = consterror.CustomError
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
					ensureFinalizerAbsent = helper.CustomErrorHappensOnce()
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
					ensureFinalizerAbsent.CalledTimes = 1
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
					delete.CalledTimes = 1
				})
				When("no deletion was requested", func() {
					BeforeEach(func() {
						routeMonitorDeletionTimestamp = nil
						delete.CalledTimes = 0
						ensureFinalizerAbsent.CalledTimes = 1
					})
					It("should skip next steps and stop processing", func() {
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
})
