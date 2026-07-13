package routemonitor_test

import (
	"github.com/go-logr/logr"
	fuzz "github.com/google/gofuzz"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	"time"

	// tested package
	"github.com/openshift/route-monitor-operator/controllers/routemonitor"

	routev1 "github.com/openshift/api/route/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	routemonitorconst "github.com/openshift/route-monitor-operator/pkg/consts"
	"github.com/openshift/route-monitor-operator/pkg/consts/blackboxexporter"
	consterror "github.com/openshift/route-monitor-operator/pkg/consts/test/error"
	constinit "github.com/openshift/route-monitor-operator/pkg/consts/test/init"
	customerrors "github.com/openshift/route-monitor-operator/pkg/util/errors"
	utilreconcile "github.com/openshift/route-monitor-operator/pkg/util/reconcile"
	clientmocks "github.com/openshift/route-monitor-operator/pkg/util/test/generated/mocks/client"
	controllermocks "github.com/openshift/route-monitor-operator/pkg/util/test/generated/mocks/controllers"
	"github.com/openshift/route-monitor-operator/pkg/util/test/helper"
)

var _ = Describe("Routemonitor", func() {

	var (
		mockClient           *clientmocks.MockClient
		mockCtrl             *gomock.Controller
		mockBlackboxExporter *controllermocks.MockBlackBoxExporterHandler
		mockUtils            *controllermocks.MockMonitorResourceHandler
		mockPrometheusRule   *controllermocks.MockPrometheusRuleHandler
		mockServiceMonitor   *controllermocks.MockServiceMonitorHandler

		update helper.MockHelper
		delete helper.MockHelper
		get    helper.MockHelper
		create helper.MockHelper

		routeMonitorReconciler        routemonitor.RouteMonitorReconciler
		routeMonitor                  v1alpha1.RouteMonitor
		routeMonitorFinalizers        []string
		routeMonitorDeletionTimestamp *metav1.Time
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockClient = clientmocks.NewMockClient(mockCtrl)
		mockBlackboxExporter = controllermocks.NewMockBlackBoxExporterHandler(mockCtrl)
		mockUtils = controllermocks.NewMockMonitorResourceHandler(mockCtrl)
		mockServiceMonitor = controllermocks.NewMockServiceMonitorHandler(mockCtrl)
		mockPrometheusRule = controllermocks.NewMockPrometheusRuleHandler(mockCtrl)

		routeMonitorReconciler = routemonitor.RouteMonitorReconciler{
			Log:              logr.Discard(),
			Client:           mockClient,
			Scheme:           constinit.Scheme,
			BlackBoxExporter: mockBlackboxExporter,
			Common:           mockUtils,
			ServiceMonitor:   mockServiceMonitor,
			Prom:             mockPrometheusRule,
		}

		update = helper.MockHelper{}
		delete = helper.MockHelper{}
		get = helper.MockHelper{}
		create = helper.MockHelper{}

		routeMonitorFinalizers = routemonitorconst.FinalizerList
		routeMonitorDeletionTimestamp = &metav1.Time{}
		routeMonitor = v1alpha1.RouteMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "scott-pilgrim",
				Namespace:         "the-world",
				DeletionTimestamp: routeMonitorDeletionTimestamp,
				Finalizers:        routeMonitorFinalizers,
			},
			Status: v1alpha1.RouteMonitorStatus{
				RouteURL: "fake-route-url",
			},
			Spec: v1alpha1.RouteMonitorSpec{
				Slo: v1alpha1.SloSpec{
					TargetAvailabilityPercent: "99.5",
				},
			},
		}
	})

	JustBeforeEach(func() {
		mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(get.ErrorResponse).
			Times(get.CalledTimes)

		mockClient.EXPECT().Delete(gomock.Any(), gomock.Any()).
			Return(delete.ErrorResponse).
			Times(delete.CalledTimes)

		mockClient.EXPECT().Create(gomock.Any(), gomock.Any()).
			Return(create.ErrorResponse).
			Times(create.CalledTimes)

		mockClient.EXPECT().Update(gomock.Any(), gomock.Any()).
			Return(update.ErrorResponse).
			Times(update.CalledTimes)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	//--------------------------------------------------------------------------------------
	// 		EnsureMonitorAndDependenciesAbsent
	//--------------------------------------------------------------------------------------
	Describe("EnsureMonitorAndDependenciesAbsent", func() {
		var (
			shouldDeleteBlackBoxExporterResources         helper.MockHelper
			ensureBlackBoxExporterResourcesAbsent         helper.MockHelper
			ensureBlackBoxExporterResourcesExist          helper.MockHelper
			deleteServiceMonitorDeployment                helper.MockHelper
			deletePrometheusRuleDeployment                helper.MockHelper
			deleteFinalizer                               helper.MockHelper
			shouldDeleteBlackBoxExporterResourcesResponse blackboxexporter.ShouldDeleteBlackBoxExporter

			res utilreconcile.Result
			err error
		)
		BeforeEach(func() {
			shouldDeleteBlackBoxExporterResources = helper.MockHelper{}
			ensureBlackBoxExporterResourcesAbsent = helper.MockHelper{}
			ensureBlackBoxExporterResourcesExist = helper.MockHelper{}
			deleteServiceMonitorDeployment = helper.MockHelper{}
			deletePrometheusRuleDeployment = helper.MockHelper{}
			deleteFinalizer = helper.MockHelper{}
			shouldDeleteBlackBoxExporterResourcesResponse = blackboxexporter.KeepBlackBoxExporter
		})
		JustBeforeEach(func() {

			shouldDeleteBlackBoxExporterResources.CalledTimes = 1

			mockBlackboxExporter.EXPECT().EnsureBlackBoxExporterResourcesAbsent().
				Times(ensureBlackBoxExporterResourcesAbsent.CalledTimes).
				Return(ensureBlackBoxExporterResourcesAbsent.ErrorResponse)

			mockBlackboxExporter.EXPECT().ShouldDeleteBlackBoxExporterResources().
				Times(shouldDeleteBlackBoxExporterResources.CalledTimes).
				Return(shouldDeleteBlackBoxExporterResourcesResponse, shouldDeleteBlackBoxExporterResources.ErrorResponse)

			mockBlackboxExporter.EXPECT().EnsureBlackBoxExporterResourcesExist().
				Times(ensureBlackBoxExporterResourcesExist.CalledTimes).
				Return(ensureBlackBoxExporterResourcesExist.ErrorResponse)

			mockServiceMonitor.EXPECT().DeleteServiceMonitorDeployment(gomock.Any(), gomock.Any()).
				Times(deleteServiceMonitorDeployment.CalledTimes).
				Return(deleteServiceMonitorDeployment.ErrorResponse)

			mockPrometheusRule.EXPECT().DeletePrometheusRuleDeployment(gomock.Any()).
				Times(deletePrometheusRuleDeployment.CalledTimes).
				Return(deletePrometheusRuleDeployment.ErrorResponse)

			// act
			res, err = routeMonitorReconciler.EnsureMonitorAndDependenciesAbsent(routeMonitor)
		})
		When("func ShouldDeleteBlackBoxExporterResources fails unexpectedly", func() {
			BeforeEach(func() {
				shouldDeleteBlackBoxExporterResources.ErrorResponse = consterror.ErrCustomError
			})
			It("should bubble up the error", func() {
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(consterror.ErrCustomError))
			})
		})
		Describe("ShouldDeleteBlackBoxExporterResources instructs to delete", func() {
			BeforeEach(func() {
				shouldDeleteBlackBoxExporterResourcesResponse = blackboxexporter.DeleteBlackBoxExporter
				ensureBlackBoxExporterResourcesAbsent.CalledTimes = 1
			})
			When("func EnsureBlackBoxExporterServiceAbsent fails unexpectedly", func() {
				BeforeEach(func() {
					ensureBlackBoxExporterResourcesAbsent.ErrorResponse = consterror.ErrCustomError
				})
				It("should bubble up the error", func() {
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(consterror.ErrCustomError))
				})
			})
			When("func EnsureBlackBoxExporterDeploymentAbsent fails unexpectedly", func() {
				BeforeEach(func() {
					ensureBlackBoxExporterResourcesAbsent = helper.CustomErrorHappensOnce()
				})
				It("should bubble up the error", func() {
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(consterror.ErrCustomError))
				})
			})
			When("func deleteServiceMonitorDeployment fails unexpectedly", func() {
				BeforeEach(func() {
					ensureBlackBoxExporterResourcesAbsent.CalledTimes = 1
					deleteServiceMonitorDeployment = helper.CustomErrorHappensOnce()
				})
				It("should bubble up the error", func() {
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(consterror.ErrCustomError))
				})
			})
			When("func DeletePrometheusRuleDeployment fails unexpectedly", func() {
				BeforeEach(func() {
					ensureBlackBoxExporterResourcesAbsent.CalledTimes = 1
					deleteServiceMonitorDeployment.CalledTimes = 1
					deletePrometheusRuleDeployment = helper.CustomErrorHappensOnce()
				})
				It("should bubble up the error", func() {
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(consterror.ErrCustomError))
				})
			})
			When("func EnsureFinalizerAbsent fails unexpectedly", func() {
				BeforeEach(func() {
					ensureBlackBoxExporterResourcesAbsent.CalledTimes = 1
					deleteServiceMonitorDeployment.CalledTimes = 1
					deletePrometheusRuleDeployment.CalledTimes = 1
					mockUtils.EXPECT().DeleteFinalizer(gomock.Any(), gomock.Any()).Return(true).Times(2)
					mockUtils.EXPECT().UpdateMonitorResource(gomock.Any()).Return(utilreconcile.RequeueOperation(), consterror.ErrCustomError)
				})
				It("should bubble up the error", func() {
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(consterror.ErrCustomError))
				})
			})
			When("all deletions happened successfully", func() {
				BeforeEach(func() {
					ensureBlackBoxExporterResourcesAbsent.CalledTimes = 1
					deleteServiceMonitorDeployment.CalledTimes = 1
					deletePrometheusRuleDeployment.CalledTimes = 1
					deleteFinalizer.CalledTimes = 1
					mockUtils.EXPECT().DeleteFinalizer(gomock.Any(), gomock.Any()).Return(true).Times(2)
					mockUtils.EXPECT().UpdateMonitorResource(gomock.Any())
				})
				It("should reconcile", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(res).To(Equal(utilreconcile.StopOperation()))
				})
			})
		})
		When("ShouldDeleteBlackBoxExporterResources instructs to keep the BlackBoxExporter", func() {
			BeforeEach(func() {
				shouldDeleteBlackBoxExporterResources.CalledTimes = 1
				shouldDeleteBlackBoxExporterResourcesResponse = blackboxexporter.KeepBlackBoxExporter
				deleteServiceMonitorDeployment.CalledTimes = 1
				deletePrometheusRuleDeployment.CalledTimes = 1
			})
			When("func EnsureServiceMonitorResourceAbsent fails unexpectedly", func() {
				BeforeEach(func() {
					deleteServiceMonitorDeployment.ErrorResponse = consterror.ErrCustomError
					deletePrometheusRuleDeployment.CalledTimes = 0
				})
				It("should bubble up the error", func() {
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(consterror.ErrCustomError))
				})
			})
			When("func EnsurePrometheusRuleResourceAbsent fails unexpectedly", func() {
				BeforeEach(func() {
					deletePrometheusRuleDeployment.ErrorResponse = consterror.ErrCustomError
				})
				It("should bubble up the error", func() {
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(consterror.ErrCustomError))
				})
			})
			When("the resource has a finalizer but 'Update' failed", func() {
				BeforeEach(func() {
					mockUtils.EXPECT().DeleteFinalizer(gomock.Any(), gomock.Any()).Return(true).Times(2)
					mockUtils.EXPECT().UpdateMonitorResource(gomock.Any()).Return(utilreconcile.RequeueOperation(), consterror.ErrCustomError)
				})
				It("Should bubble up the failure", func() {
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(consterror.ErrCustomError))
				})
			})
			When("the resource has a finalizer but 'Update' succeeds", func() {
				BeforeEach(func() {
					mockUtils.EXPECT().DeleteFinalizer(gomock.Any(), gomock.Any()).Return(true).Times(2)
					mockUtils.EXPECT().UpdateMonitorResource(gomock.Any()).Return(utilreconcile.StopOperation(), nil)
				})
				It("Should succeed and call for a requeue", func() {
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
					mockUtils.EXPECT().DeleteFinalizer(gomock.Any(), gomock.Any()).Return(false)
				})
				When("no deletion was requested", func() {
					BeforeEach(func() {
						routeMonitorDeletionTimestamp = nil
						delete.CalledTimes = 0
						deleteFinalizer.CalledTimes = 1
					})
					It("should skip next steps and stop processing", func() {
						Expect(err).NotTo(HaveOccurred())
						Expect(res).To(Equal(utilreconcile.StopOperation()))
					})
				})
			})
		})
	})

	//--------------------------------------------------------------------------------------
	// 		GetRouteMonitor
	//--------------------------------------------------------------------------------------
	Describe("GetRouteMonitor", func() {
		f := fuzz.New()

		var (
			scheme                *runtime.Scheme
			req                   ctrl.Request
			routeMonitorName      string
			routeMonitorNamespace string

			resRouteMonitor v1alpha1.RouteMonitor
			res             utilreconcile.Result
			err             error
		)

		f.Fuzz(&routeMonitorName)
		f.Fuzz(&routeMonitorNamespace)

		BeforeEach(func() {
		})

		JustBeforeEach(func() {
			scheme = constinit.Scheme
			req = ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      "bla", // routeMonitorName,
					Namespace: "bda", // routeMonitorNamespace,
				},
			}

			// Act
			resRouteMonitor, res, err = routeMonitorReconciler.GetRouteMonitor(req)
		})
		When("func Get fails unexpectedly", func() {
			BeforeEach(func() {
				get = helper.CustomErrorHappensOnce()
			})
			It("should stop requeue", func() {
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(consterror.ErrCustomError))
			})
		})
		When("the RouteMonitor is not found", func() {
			BeforeEach(func() {
				routeMonitorReconciler.Client = fake.NewClientBuilder().WithScheme(scheme).Build()

			})
			It("should stop requeue", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(res).To(Equal(utilreconcile.StopOperation()))
				Expect(resRouteMonitor).To(BeZero())
			})
		})
		When("the RouteMonitor is found", func() {
			BeforeEach(func() {
				routeMonitorReconciler.Client = fake.NewClientBuilder().WithScheme(scheme).WithObjects(&routeMonitor).
					Build()
			})
			It("should return the object", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(resRouteMonitor).NotTo(BeNil())
			})
		})
	})
	//--------------------------------------------------------------------------------------
	// 		GetRoute
	//--------------------------------------------------------------------------------------
	Describe("GetRoute", func() {

		f := fuzz.New()

		var (
			scheme                *runtime.Scheme
			routeMonitorName      string
			routeMonitorNamespace string

			route routev1.Route
			res   routev1.Route
			err   error
		)

		// Start Fuzz testing for values
		f.Fuzz(&routeMonitorName)
		f.Fuzz(&routeMonitorNamespace)

		BeforeEach(func() {
			routeMonitor.Spec.Route = v1alpha1.RouteMonitorRouteSpec{
				Name:      "fake",
				Namespace: "fake-namespace",
			}
			scheme = constinit.Scheme
		})
		JustBeforeEach(func() {

		})
		When("the Route is not found", func() {
			// Arrange
			BeforeEach(func() {
				get.CalledTimes = 1
				get.ErrorResponse = consterror.NotFoundErr
			})
			It("should return a Not Found error", func() {
				// Act
				res, err = routeMonitorReconciler.GetRoute(routeMonitor)
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(k8serrors.IsNotFound(err)).To(BeTrue())
				Expect(res).To(BeZero())

			})
		})
		When("the Route is found", func() {
			BeforeEach(func() {
				route = routev1.Route{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "fake",
						Namespace: "fake-namespace",
					},
				}

				routeMonitorReconciler.Client = fake.NewClientBuilder().WithScheme(scheme).WithObjects(&route).Build()
			})
			It("should return the route", func() {
				// Act
				res, err = routeMonitorReconciler.GetRoute(routeMonitor)
				// Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(res).ShouldNot(BeNil())
			})
		})

		Describe("Missing a RouteMonitor Field", func() {
			route = routev1.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      routeMonitorName,
					Namespace: routeMonitorNamespace,
				},
			}
			When("the RouteMonitor doesnt have Spec.Route.Namespace", func() {
				BeforeEach(func() {
					routeMonitor.Spec.Route.Namespace = ""
				})
				It("should return a custom error", func() {
					// Act
					res, err = routeMonitorReconciler.GetRoute(routeMonitor)
					// Assert
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(HavePrefix("invalid CR:"))
					Expect(res).To(BeZero())
				})
			})
			When("the RouteMonitor doesnt have Spec.Route.Name", func() {
				BeforeEach(func() {
					routeMonitor.Spec.Route.Name = ""
				})
				It("should return a custom error", func() {
					// Act
					res, err = routeMonitorReconciler.GetRoute(routeMonitor)
					// Assert
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(HavePrefix("invalid CR:"))
					Expect(res).To(BeZero())
				})
			})
		})
	})
	//--------------------------------------------------------------------------------------
	// 		EnsureRouteURLExists
	//--------------------------------------------------------------------------------------
	Describe("EnsureRouteURLExists", func() {

		f := fuzz.New()

		var (
			route                 routev1.Route
			expectedRouteMonitor  v1alpha1.RouteMonitor
			routeMonitorName      string
			routeMonitorNamespace string

			res       utilreconcile.Result
			err       error
			ingresses []string
		)

		// Start Fuzz testing for values
		f.Fuzz(&routeMonitorName)
		f.Fuzz(&routeMonitorNamespace)

		BeforeEach(func() {
			routeMonitor.Spec.Route = v1alpha1.RouteMonitorRouteSpec{
				Name:      routeMonitorName,
				Namespace: routeMonitorNamespace,
			}
			expectedRouteMonitor = routeMonitor
		})

		JustBeforeEach(func() {
			route = routev1.Route{
				Status: routev1.RouteStatus{
					Ingress: ConvertToIngressHosts(ingresses),
				},
			}

			// act
			res, err = routeMonitorReconciler.EnsureRouteURLExists(route, routeMonitor)
		})

		When("the Route has no Ingresses", func() {
			// Arrange
			It("should return No Ingress error", func() {
				Expect(res).To(Equal(utilreconcile.RequeueOperation()))
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(HavePrefix("no Ingress:"))
			})
		})
		When("the Route has no Host", func() {
			BeforeEach(func() {
				ingresses = []string{""}
			})
			It("should return No Host error", func() {
				Expect(res).To(Equal(utilreconcile.RequeueOperation()))
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(customerrors.ErrNoHost))
			})
		})

		When("func Update fails unexpectedly", func() {
			var (
				firstRouteURL = "freddy"
			)
			BeforeEach(func() {
				ingresses = []string{
					firstRouteURL,
					"eddie",
				}
				mockUtils.EXPECT().UpdateMonitorResourceStatus(gomock.Any()).Times(1).Return(utilreconcile.RequeueOperation(), consterror.ErrCustomError)
				routeMonitorReconciler.Client = mockClient
			})
			JustBeforeEach(func() {
				expectedRouteMonitor.Status.RouteURL = firstRouteURL
			})
			It("should bubble up the error", func() {
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(consterror.ErrCustomError))
			})
		})
		When("the Route has too many Ingress", func() {
			var (
				firstRouteURL = "freddy"
			)
			BeforeEach(func() {
				ingresses = []string{
					firstRouteURL,
					"eddie",
				}
				mockUtils.EXPECT().UpdateMonitorResourceStatus(gomock.Any()).Times(1).Return(utilreconcile.StopOperation(), nil)
			})
			JustBeforeEach(func() {
				expectedRouteMonitor.Status.RouteURL = firstRouteURL
			})
			It("should update the first ingress", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(res).NotTo(BeNil())
				Expect(res).To(Equal(utilreconcile.StopOperation()))
			})
		})
		When("the RouteURL is not like the extracted Route", func() {
			var (
				firstRouteURL = "freddy"
			)
			BeforeEach(func() {
				ingresses = []string{
					firstRouteURL,
				}

				mockUtils.EXPECT().UpdateMonitorResourceStatus(gomock.Any()).Times(1).Return(utilreconcile.StopOperation(), nil)
			})
			JustBeforeEach(func() {
				routeMonitor.Status.RouteURL = firstRouteURL + "but-different"
				expectedRouteMonitor.Status.RouteURL = firstRouteURL

			})
			It("should update with the Route information", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(res).NotTo(BeNil())
				Expect(res).To(Equal(utilreconcile.StopOperation()))
			})
		})

		When("the Route has the same RouteURL as the extracted one", func() {
			BeforeEach(func() {
				ingresses = []string{
					"fake-route-url",
				}

				routeMonitor.Status = v1alpha1.RouteMonitorStatus{
					RouteURL: "fake-route-url",
				}
				routeMonitorReconciler.Client = mockClient
			})
			JustBeforeEach(func() {
				expectedRouteMonitor.Status.RouteURL = "fake-route-url"
			})
			It("should skip this operation", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(res).To(Equal(utilreconcile.ContinueOperation()))
			})
		})
	})
	//--------------------------------------------------------------------------------------
	// 		EnsurePrometheusRuleResourceExists
	//--------------------------------------------------------------------------------------
	Describe("EnsurePrometheusRuleResourceExists", func() {
		var (
			resp utilreconcile.Result
			err  error
		)
		JustBeforeEach(func() {
			resp, err = routeMonitorReconciler.EnsurePrometheusRuleExists(routeMonitor)
		})
		Describe("The RouteMonitor settings are INVALID", func() {
			BeforeEach(func() {
				mockUtils.EXPECT().ParseMonitorSLOSpecs(routeMonitor.Status.RouteURL, routeMonitor.Spec.Slo).Return("", customerrors.ErrNoHost).Times(1)
			})
			Describe("It sets the Error state in the RouteMonitor the first time", func() {
				BeforeEach(func() {
					mockUtils.EXPECT().SetErrorStatus(gomock.Any(), customerrors.ErrNoHost).Return(true)
				})
				When("updating the RouteMonitor with the new error State works", func() {
					BeforeEach(func() {
						mockUtils.EXPECT().UpdateMonitorResourceStatus(gomock.Any()).Return(utilreconcile.StopOperation(), nil)
					})
					It("stops reconcileing", func() {
						Expect(err).NotTo(HaveOccurred())
						Expect(resp).To(Equal(utilreconcile.StopOperation()))
					})
				})
				When("updating the RouteMonitor new error State failes", func() {
					BeforeEach(func() {
						mockUtils.EXPECT().UpdateMonitorResourceStatus(gomock.Any()).Return(utilreconcile.RequeueOperation(), consterror.ErrCustomError)
					})
					It("requeues with the particular error", func() {
						Expect(err).To(Equal(consterror.ErrCustomError))
						Expect(resp).To(Equal(utilreconcile.RequeueOperation()))
					})
				})
			})

			Describe("It deletes existing PrometheusRules", func() {
				BeforeEach(func() {
					routeMonitor.Status.PrometheusRuleRef = v1alpha1.NamespacedName{Name: "test", Namespace: "test2"}
					mockUtils.EXPECT().SetErrorStatus(gomock.Any(), customerrors.ErrNoHost).Return(false)
				})
				When("the PrometheusRule deletion fails", func() {
					BeforeEach(func() {
						mockPrometheusRule.EXPECT().DeletePrometheusRuleDeployment(routeMonitor.Status.PrometheusRuleRef).Times(1).Return(consterror.ErrCustomError)
					})
					It("should reconcile with the particular error", func() {
						Expect(err).To(Equal(consterror.ErrCustomError))
						Expect(resp).To(Equal(utilreconcile.RequeueOperation()))
					})
				})
				When("the PrometheusRule deletion was successful", func() {
					BeforeEach(func() {
						mockPrometheusRule.EXPECT().DeletePrometheusRuleDeployment(routeMonitor.Status.PrometheusRuleRef).Times(1)
					})
					When("updating PrometheusRuleRef in the RouteMonitor fails", func() {
						BeforeEach(func() {
							mockUtils.EXPECT().SetResourceReference(gomock.Any(), gomock.Any()).Return(true, nil)
							mockUtils.EXPECT().UpdateMonitorResourceStatus(gomock.Any()).Return(utilreconcile.RequeueOperation(), consterror.ErrCustomError)
						})
						It("should reconcile with the particular error", func() {
							Expect(err).To(Equal(consterror.ErrCustomError))
							Expect(resp).To(Equal(utilreconcile.RequeueOperation()))
						})
					})
					When("updating PrometheusRuleRef in the RouteMonitor was successful", func() {
						BeforeEach(func() {
							mockUtils.EXPECT().SetResourceReference(gomock.Any(), gomock.Any()).Return(true, nil)
							mockUtils.EXPECT().UpdateMonitorResourceStatus(gomock.Any()).Return(utilreconcile.StopOperation(), nil)
						})
						It("stops reconciling", func() {
							Expect(err).NotTo(HaveOccurred())
							Expect(resp).To(Equal(utilreconcile.StopOperation()))
						})
					})
				})
			})
		})
		Describe("The RouteMonitor settings are VALID", func() {
			BeforeEach(func() {
				mockUtils.EXPECT().ParseMonitorSLOSpecs(routeMonitor.Status.RouteURL, routeMonitor.Spec.Slo).Return("99.5", nil).Times(1)
				mockUtils.EXPECT().SetErrorStatus(gomock.Any(), nil).Return(false)
			})
			When("the update the PrometheusRule failed", func() {
				BeforeEach(func() {
					mockPrometheusRule.EXPECT().UpdatePrometheusRuleDeployment(gomock.Any()).Return(consterror.ErrCustomError)
				})
				It("requeues with the error", func() {
					Expect(err).To(Equal(consterror.ErrCustomError))
					Expect(resp).To(Equal(utilreconcile.RequeueOperation()))
				})
			})
			When("the update of the PrometheusRule succeeded", func() {
				BeforeEach(func() {
					mockPrometheusRule.EXPECT().UpdatePrometheusRuleDeployment(gomock.Any())
				})
				When("a new PrometheusRule was created", func() {
					BeforeEach(func() {
						mockUtils.EXPECT().SetResourceReference(gomock.Any(), gomock.Any()).Return(true, nil)
					})
					When("the ServiceMonitor is updated successfully", func() {
						BeforeEach(func() {
							mockUtils.EXPECT().UpdateMonitorResourceStatus(gomock.Any()).Return(utilreconcile.StopOperation(), nil)
						})
						It("stops reconciling", func() {
							Expect(err).NotTo(HaveOccurred())
							Expect(resp).To(Equal(utilreconcile.StopOperation()))
						})
					})
					When("updating the reference fails", func() {
						BeforeEach(func() {
							mockUtils.EXPECT().UpdateMonitorResourceStatus(gomock.Any()).Return(utilreconcile.RequeueOperation(), consterror.ErrCustomError)
						})
						It("requeus with error", func() {
							Expect(err).To(Equal(consterror.ErrCustomError))
							Expect(resp).To(Equal(utilreconcile.RequeueOperation()))
						})
					})
				})
			})
		})
	})
	//--------------------------------------------------------------------------------------
	// 		EnsureServiceMonitorResourceExists
	//--------------------------------------------------------------------------------------
	Describe("EnsureServiceMonitorResourceExists", func() {
		var (
			resp utilreconcile.Result
			err  error
		)
		JustBeforeEach(func() {
			resp, err = routeMonitorReconciler.EnsureServiceMonitorExists(routeMonitor)
		})
		When("The RouteUrl is not set", func() {
			BeforeEach(func() {
				routeMonitor.Status.RouteURL = ""
			})
			It("will requeue with the NoHost error ", func() {
				Expect(err).To(Equal(customerrors.ErrNoHost))
				Expect(resp).To(Equal(utilreconcile.RequeueOperation()))
			})
		})
		Describe("It updates the ServiceMonitor targeting the blackbox Exporter Namespace", func() {
			When("the update of the ServiceMonitor fails", func() {
				BeforeEach(func() {
					mockServiceMonitor.EXPECT().TemplateAndUpdateServiceMonitorDeployment(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(consterror.ErrCustomError)
					mockBlackboxExporter.EXPECT().GetBlackBoxExporterNamespace().Return("bla")
					mockUtils.EXPECT().GetOSDClusterID().Return("test-cluster-id", nil)
				})
				It("will requeue with the error", func() {
					Expect(err).To(Equal(consterror.ErrCustomError))
					Expect(resp).To(Equal(utilreconcile.RequeueOperation()))
				})
			})
			When("the update of the ServiceMonitor is successful", func() {
				BeforeEach(func() {
					mockServiceMonitor.EXPECT().TemplateAndUpdateServiceMonitorDeployment(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1)
					mockBlackboxExporter.EXPECT().GetBlackBoxExporterNamespace().Return("bla")
					mockUtils.EXPECT().GetOSDClusterID().Return("test-cluster-id", nil)
				})
				When("the update of the ServiceMonitorRef fails", func() {
					BeforeEach(func() {
						mockUtils.EXPECT().SetResourceReference(gomock.Any(), gomock.Any()).Return(false, consterror.ErrCustomError)
					})
					It("will requeue with the error", func() {
						Expect(err).To(Equal(consterror.ErrCustomError))
						Expect(resp).To(Equal(utilreconcile.RequeueOperation()))
					})
				})
				When("the update of the ServiceMonitorRef is successful", func() {
					BeforeEach(func() {
						mockUtils.EXPECT().SetResourceReference(gomock.Any(), gomock.Any()).Return(true, nil)
					})
					When("it updates the RouteMonitor", func() {
						BeforeEach(func() {
							mockUtils.EXPECT().UpdateMonitorResourceStatus(gomock.Any()).Return(utilreconcile.StopOperation(), nil)
						})
						It("stops reconciiling", func() {
							Expect(err).NotTo(HaveOccurred())
							Expect(resp).To(Equal(utilreconcile.StopOperation()))
						})
					})
				})
			})
		})
	})
})

//--------------------------------------------------------------------------------------
// 		Helper Functions
//--------------------------------------------------------------------------------------

func ConvertToIngressHosts(in []string) []routev1.RouteIngress {
	res := make([]routev1.RouteIngress, len(in))
	for i, s := range in {
		res[i] = routev1.RouteIngress{
			Host: s,
		}
	}
	return res
}
