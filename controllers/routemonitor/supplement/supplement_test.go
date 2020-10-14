package supplement_test

import (
	"github.com/golang/mock/gomock"
	fuzz "github.com/google/gofuzz"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/openshift/route-monitor-operator/controllers/routemonitor"
	"github.com/openshift/route-monitor-operator/controllers/routemonitor/supplement"

	consts "github.com/openshift/route-monitor-operator/pkg/const"
	customerrors "github.com/openshift/route-monitor-operator/pkg/util/errors"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	routev1 "github.com/openshift/api/route/v1"
	"github.com/openshift/route-monitor-operator/api/v1alpha1"

	routemonitorconst "github.com/openshift/route-monitor-operator/pkg/const"
	consterror "github.com/openshift/route-monitor-operator/pkg/const/test/error"
	constinit "github.com/openshift/route-monitor-operator/pkg/const/test/init"
	utilreconcile "github.com/openshift/route-monitor-operator/pkg/util/reconcile"
	clientmocks "github.com/openshift/route-monitor-operator/pkg/util/tests/generated/mocks/client"
)

var _ = Describe("Routemonitor", func() {
	// Add new Fuzzer
	f := fuzz.New()

	var (
		routeMonitor                  v1alpha1.RouteMonitor
		routeMonitorName              string
		routeMonitorNamespace         string
		routeMonitorRouteSpec         v1alpha1.RouteMonitorRouteSpec
		routeMonitorFinalizers        []string
		routeMonitorDeletionTimestamp *metav1.Time
		routeMonitorStatus            v1alpha1.RouteMonitorStatus

		routeMonitorSupplement       supplement.RouteMonitorSupplement
		routeMonitorSupplementClient client.Client

		req                  ctrl.Request
		route                routev1.Route
		expectedRouteMonitor v1alpha1.RouteMonitor

		mockClient       *clientmocks.MockClient
		mockStatusWriter *clientmocks.MockStatusWriter
		mockCtrl         *gomock.Controller

		// createCalledTimes is a toggle for the 'mockClient.EXPECT.Create()'. If set to 0 then ignored
		createCalledTimes   int
		createErrorResponse error
		// getCalledTimes is a toggle for the 'mockClient.EXPECT.Get()'. If set to 0 then ignored
		getCalledTimes   int
		getErrorResponse error
		// listCalledTimes is a toggle for the 'mockClient.EXPECT.List()'. If set to 0 then ignored
		listCalledTimes   int
		listErrorResponse error
		// updateCalledTimes is a toggle for the 'mockClient.EXPECT.Update()'. If set to 0 then ignored
		updateCalledTimes   int
		updateErrorResponse error
		// delete is a toggle for the 'mockClient.EXPECT.Delete()'. If set to 0 then ignored
		deleteCalledTimes   int
		deleteErrorResponse error
	)
	const (
		routeMonitorRouteURLDefault = "fake-route-url"
	)

	var ( // Practically const vars
		ctx    = constinit.Context
		scheme = constinit.Scheme
	)

	// Start Fuzz testing for values
	f.Fuzz(&routeMonitorName)
	f.Fuzz(&routeMonitorNamespace)

	BeforeEach(func() {
		routeMonitorName = "fake-name"
		routeMonitorNamespace = "fake-namespace"
		routeMonitorRouteSpec = v1alpha1.RouteMonitorRouteSpec{}
		routeMonitorSupplementClient = fake.NewFakeClientWithScheme(scheme)
		routeMonitorFinalizers = routemonitorconst.FinalizerList
		routeMonitorDeletionTimestamp = nil
		routeMonitorStatus = v1alpha1.RouteMonitorStatus{}
		mockCtrl = gomock.NewController(GinkgoT())
		mockClient = clientmocks.NewMockClient(mockCtrl)
		mockStatusWriter = clientmocks.NewMockStatusWriter(mockCtrl)

		createCalledTimes = 0
		createErrorResponse = nil
		getCalledTimes = 0
		getErrorResponse = nil
		listCalledTimes = 0
		listErrorResponse = nil
		updateCalledTimes = 0
		updateErrorResponse = nil
		deleteCalledTimes = 0
		deleteErrorResponse = nil
	})

	JustBeforeEach(func() {
		routeMonitor = v1alpha1.RouteMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:              routeMonitorName,
				Namespace:         routeMonitorNamespace,
				Finalizers:        routeMonitorFinalizers,
				DeletionTimestamp: routeMonitorDeletionTimestamp,
			},
			Spec: v1alpha1.RouteMonitorSpec{
				Route: routeMonitorRouteSpec,
			},
			Status: routeMonitorStatus,
		}
		expectedRouteMonitor = routeMonitor

		routeMonitorSupplement = supplement.RouteMonitorSupplement{
			Log:    constinit.Logger,
			Client: routeMonitorSupplementClient,
			Scheme: scheme,
		}
		req = ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      routeMonitorName,
				Namespace: routeMonitorNamespace,
			},
		}

		if createCalledTimes != 0 {
			mockClient.EXPECT().Create(gomock.Any(), gomock.Any()).
				Return(createErrorResponse).
				Times(createCalledTimes)
		}

		if updateCalledTimes != 0 {
			mockClient.EXPECT().Update(gomock.Any(), gomock.Any()).
				Return(updateErrorResponse).
				Times(updateCalledTimes)
		}

		mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(getErrorResponse).
			Times(getCalledTimes)

		mockClient.EXPECT().List(gomock.Any(), gomock.Any()).
			Return(listErrorResponse).
			Times(listCalledTimes)

		mockClient.EXPECT().Delete(gomock.Any(), gomock.Any()).
			Return(deleteErrorResponse).
			Times(deleteCalledTimes)

	})

	AfterEach(func() {
		mockCtrl.Finish()
	})
	Describe("GetRouteMonitor", func() {
		When("func Get fails unexpectedly", func() {
			// Arrange
			BeforeEach(func() {
				getCalledTimes = 1
				getErrorResponse = consterror.CustomError
				routeMonitorSupplementClient = mockClient
			})
			It("should stop requeue", func() {
				// Act
				_, _, err := routeMonitorSupplement.GetRouteMonitor(ctx, req)
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(consterror.CustomError))

			})
		})
		When("the RouteMonitor is not found", func() {
			// Arrange
			BeforeEach(func() {
				routeMonitorSupplementClient = fake.NewFakeClientWithScheme(scheme)
			})
			It("should stop requeue", func() {
				// Act
				resRouteMonitor, res, err := routeMonitorSupplement.GetRouteMonitor(ctx, req)
				// Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(res).To(Equal(utilreconcile.StopOperation()))
				Expect(resRouteMonitor).To(BeZero())
			})
		})
		When("the RouteMonitor is found", func() {
			// Arrange
			BeforeEach(func() {
				routeMonitorSupplementClient = fake.NewFakeClientWithScheme(scheme, &routeMonitor)
			})

			It("should return the object", func() {
				// Act
				resRouteMonitor, _, err := routeMonitorSupplement.GetRouteMonitor(ctx, req)
				// Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(resRouteMonitor).NotTo(BeNil())
			})
		})
	})

	Describe("GetRoute", func() {
		BeforeEach(func() {
			routeMonitorRouteSpec = v1alpha1.RouteMonitorRouteSpec{
				Name:      routeMonitorName,
				Namespace: routeMonitorNamespace,
			}
		})
		When("the Route is not found", func() {
			// Arrange
			BeforeEach(func() {
				route = routev1.Route{
					ObjectMeta: metav1.ObjectMeta{
						Name:      routeMonitorName + "-but-different",
						Namespace: routeMonitorNamespace,
					},
				}
				routeMonitorSupplementClient = fake.NewFakeClientWithScheme(scheme, &route)
			})
			It("should return a Not Found error", func() {
				// Act
				res, err := routeMonitorSupplement.GetRoute(ctx, routeMonitor)
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(k8serrors.IsNotFound(err)).To(BeTrue())
				Expect(res).To(BeZero())

			})
		})
		When("the Route is found", func() {
			// Arrange
			BeforeEach(func() {
				// Arrange
				route = routev1.Route{
					ObjectMeta: metav1.ObjectMeta{
						Name:      routeMonitorName,
						Namespace: routeMonitorNamespace,
					},
				}
				routeMonitorSupplementClient = fake.NewFakeClientWithScheme(scheme, &route)
			})
			It("should return the route", func() {
				// Act
				resRoute, err := routeMonitorSupplement.GetRoute(ctx, routeMonitor)
				// Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(resRoute).ShouldNot(BeNil())
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
				// Arrange
				BeforeEach(func() {
					routeMonitorRouteSpec.Namespace = ""
				})
				It("should return a custom error", func() {
					// Act
					resRoute, err := routeMonitorSupplement.GetRoute(ctx, routeMonitor)
					// Assert
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(HavePrefix("Invalid CR:"))
					Expect(resRoute).To(BeZero())
				})
			})
			When("the RouteMonitor doesnt have Spec.Route.Name", func() {
				// Arrange
				BeforeEach(func() {
					routeMonitorRouteSpec.Name = ""
				})
				It("should return a custom error", func() {
					// Act
					resRoute, err := routeMonitorSupplement.GetRoute(ctx, routeMonitor)
					// Assert
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(HavePrefix("Invalid CR:"))
					Expect(resRoute).To(BeZero())
				})
			})
		})
	})
	Describe("EnsureRouteURLExists", func() {
		var ingresses []string
		JustBeforeEach(func() {
			route = routev1.Route{
				Status: routev1.RouteStatus{
					Ingress: ConvertToIngressHosts(ingresses),
				},
			}
		})
		When("the Route has no Ingresses", func() {
			// Arrange
			It("should return No Ingress error", func() {
				// Act
				res, err := routeMonitorSupplement.EnsureRouteURLExists(ctx, route, routeMonitor)
				// Assert
				Expect(res).To(BeZero())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(HavePrefix("No Ingress:"))
			})
		})
		When("the Route has no Host", func() {
			// Arrange
			BeforeEach(func() {
				ingresses = []string{""}
			})
			It("should return No Host error", func() {
				// Act
				res, err := routeMonitorSupplement.EnsureRouteURLExists(ctx, route, routeMonitor)
				// Assert
				Expect(res).To(BeZero())
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(customerrors.NoHost))
			})
		})

		When("func Update fails unexpectedly", func() {
			// Arrange
			var (
				firstRouteURL = "freddy"
			)
			BeforeEach(func() {
				ingresses = []string{
					firstRouteURL,
					"eddie",
				}

				mockClient.EXPECT().Status().Return(mockStatusWriter).Times(1)
				routeMonitorSupplementClient = mockClient
			})
			JustBeforeEach(func() {
				expectedRouteMonitor.Status.RouteURL = firstRouteURL
				mockStatusWriter.EXPECT().Update(gomock.Any(), gomock.Any()).
					Times(1).
					Return(consterror.CustomError)

			})
			It("should bubble up the error", func() {
				// Act
				_, err := routeMonitorSupplement.EnsureRouteURLExists(ctx, route, routeMonitor)
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(consterror.CustomError))
			})
		})
		When("the Route has too many Ingress", func() {
			// Arrange
			var (
				firstRouteURL = "freddy"
			)
			BeforeEach(func() {
				ingresses = []string{
					firstRouteURL,
					"eddie",
				}

				mockClient.EXPECT().Status().Return(mockStatusWriter).Times(1)
				routeMonitorSupplementClient = mockClient
			})
			JustBeforeEach(func() {
				expectedRouteMonitor.Status.RouteURL = firstRouteURL
				mockStatusWriter.EXPECT().Update(gomock.Any(), gomock.Eq(&expectedRouteMonitor)).Times(1).Return(nil)

			})
			It("should update the first ingress", func() {
				// Act
				res, err := routeMonitorSupplement.EnsureRouteURLExists(ctx, route, routeMonitor)
				// Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(res).NotTo(BeNil())
				Expect(res).To(Equal(utilreconcile.StopOperation()))
			})
		})
		When("the RouteURL is not like the extracted Route", func() {
			// Arrange
			var (
				firstRouteURL = "freddy"
			)
			BeforeEach(func() {
				ingresses = []string{
					firstRouteURL,
				}

				mockClient.EXPECT().Status().Return(mockStatusWriter).Times(1)
				routeMonitorSupplementClient = mockClient
			})
			JustBeforeEach(func() {
				routeMonitor.Status.RouteURL = firstRouteURL + "but-different"
				expectedRouteMonitor.Status.RouteURL = firstRouteURL
				mockStatusWriter.EXPECT().Update(gomock.Any(), gomock.Eq(&expectedRouteMonitor)).Times(1).Return(nil)

			})
			It("should update with the Route information", func() {
				// Act
				res, err := routeMonitorSupplement.EnsureRouteURLExists(ctx, route, routeMonitor)
				// Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(res).NotTo(BeNil())
				Expect(res).To(Equal(utilreconcile.StopOperation()))
			})
		})

		When("the Route has the same RouteURL as the extracted one", func() {
			// Arrange
			BeforeEach(func() {
				ingresses = []string{
					routeMonitorRouteURLDefault,
				}

				routeMonitorStatus = v1alpha1.RouteMonitorStatus{
					RouteURL: routeMonitorRouteURLDefault,
				}
				routeMonitorSupplementClient = mockClient
			})
			JustBeforeEach(func() {
				expectedRouteMonitor.Status.RouteURL = routeMonitorRouteURLDefault
			})
			It("should skip this operation", func() {
				// Act
				res, err := routeMonitorSupplement.EnsureRouteURLExists(ctx, route, routeMonitor)
				// Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(res).To(Equal(utilreconcile.ContinueOperation()))
			})
		})
	})
	Describe("EnsureFinalizerAbsent", func() {
		BeforeEach(func() {
			routeMonitorSupplementClient = mockClient
		})
		When("RouteMonitor has no finalizer", func() {
			BeforeEach(func() {
				routeMonitorFinalizers = []string{}
			})
			It("should return continue", func() {
				res, err := routeMonitorSupplement.EnsureFinalizerAbsent(ctx, routeMonitor)
				// Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(res).To(Equal(utilreconcile.ContinueOperation()))
			})
		})
		When("RouteMonitor has finalizer but Update fails unexpectidly", func() {
			BeforeEach(func() {
				updateCalledTimes = 1
				updateErrorResponse = consterror.CustomError
			})
			It("should bubble the error up", func() {
				// Act
				_, err := routeMonitorSupplement.EnsureFinalizerAbsent(ctx, routeMonitor)
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(consterror.CustomError))
			})
		})
		When("RouteMonitor has multiple finalizer and Update succeeds", func() {
			const secondFinalizer = "a"
			BeforeEach(func() {
				routeMonitorFinalizers = []string{consts.FinalizerKey, secondFinalizer}
			})
			JustBeforeEach(func() {
				secondRouteMonitor := routeMonitor
				secondRouteMonitor.Finalizers = []string{secondFinalizer}
				mockClient.EXPECT().
					Update(
						gomock.Any(),
						// this checks that the finalizer was deleted
						gomock.Eq(&secondRouteMonitor),
					).
					Times(1).
					Return(nil)
			})
			It("stop proccesing and remove the neccesary finalizer", func() {
				res, err := routeMonitorSupplement.EnsureFinalizerAbsent(ctx, routeMonitor)
				// Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(res).To(Equal(utilreconcile.StopOperation()))
			})
		})
	})

	Describe("New", func() {
		When("func New is called", func() {
			It("should return a new Deleter object", func() {
				// Arrange
				r := routemonitor.RouteMonitorReconciler{
					Client: routeMonitorSupplementClient,
					Log:    constinit.Logger,
					Scheme: constinit.Scheme,
				}
				// Act
				res := supplement.New(r)
				// Assert
				Expect(res).To(Equal(&supplement.RouteMonitorSupplement{
					Client: routeMonitorSupplementClient,
					Log:    constinit.Logger,
					Scheme: constinit.Scheme,
				}))
			})
		})
	})
})

func ConvertToIngressHosts(in []string) []routev1.RouteIngress {
	res := make([]routev1.RouteIngress, len(in))
	for i, s := range in {
		res[i] = routev1.RouteIngress{
			Host: s,
		}
	}
	return res
}
