package adder_test

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"context"

	// tested package
	"github.com/openshift/route-monitor-operator/controllers/routemonitor/adder"

	"sigs.k8s.io/controller-runtime/pkg/client"
	//nolint:staticcheck // This will not be migrated until we migrate operator-sdk to a newer version (I think)
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	routemonitorconst "github.com/openshift/route-monitor-operator/pkg/consts"
	consterror "github.com/openshift/route-monitor-operator/pkg/consts/test/error"
	constinit "github.com/openshift/route-monitor-operator/pkg/consts/test/init"
	customerrors "github.com/openshift/route-monitor-operator/pkg/util/errors"
	utilreconcile "github.com/openshift/route-monitor-operator/pkg/util/reconcile"
	"github.com/openshift/route-monitor-operator/pkg/util/templates"
	clientmocks "github.com/openshift/route-monitor-operator/pkg/util/test/generated/mocks/client"
	utilmocks "github.com/openshift/route-monitor-operator/pkg/util/test/generated/mocks/reconcile"
	"github.com/openshift/route-monitor-operator/pkg/util/test/helper"
	testhelper "github.com/openshift/route-monitor-operator/pkg/util/test/helper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openshift/route-monitor-operator/controllers/routemonitor"
)

var _ = Describe("Adder", func() {
	var (
		scheme = constinit.Scheme
	)
	var (
		mockClient           *clientmocks.MockClient
		mockCtrl             *gomock.Controller
		mockResourceComparer *utilmocks.MockResourceComparer
		deepEqualCalledTimes int
		deepEqualResponse    bool

		routeMonitorAdder       adder.RouteMonitorAdder
		routeMonitorAdderClient client.Client

		ctx context.Context

		routeMonitor           v1alpha1.RouteMonitor
		routeMonitorName       string
		routeMonitorNamespace  string
		routeMonitorStatus     v1alpha1.RouteMonitorStatus
		routeMonitorFinalizers []string

		get    testhelper.MockHelper
		delete testhelper.MockHelper
		create testhelper.MockHelper
		update testhelper.MockHelper

		mockStatusWriter *clientmocks.MockStatusWriter
	)
	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockClient = clientmocks.NewMockClient(mockCtrl)
		mockStatusWriter = clientmocks.NewMockStatusWriter(mockCtrl)
		mockResourceComparer = utilmocks.NewMockResourceComparer(mockCtrl)

		routeMonitorAdderClient = mockClient
		deepEqualCalledTimes = 0
		deepEqualResponse = true

		ctx = constinit.Context

		get = testhelper.MockHelper{}
		delete = testhelper.MockHelper{}
		create = testhelper.MockHelper{}
		update = testhelper.MockHelper{}

		routeMonitorAdderClient = mockClient
		routeMonitorStatus = v1alpha1.RouteMonitorStatus{
			RouteURL: "fake-route-url",
		}
		routeMonitorName = "fake-name"
		routeMonitorNamespace = "fake-namespace"
		routeMonitorFinalizers = routemonitorconst.FinalizerList

	})
	JustBeforeEach(func() {
		routeMonitorAdder = adder.RouteMonitorAdder{
			Log:              constinit.Logger,
			Client:           routeMonitorAdderClient,
			Scheme:           constinit.Scheme,
			ClusterID:        "test-cluster",
			ResourceComparer: mockResourceComparer,
		}

		mockClient.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(update.ErrorResponse).
			Times(update.CalledTimes)

		mockClient.EXPECT().Delete(gomock.Any(), gomock.Any()).
			Return(delete.ErrorResponse).
			Times(delete.CalledTimes)

		mockClient.EXPECT().Create(gomock.Any(), gomock.Any()).
			Return(create.ErrorResponse).
			Times(create.CalledTimes)

		mockResourceComparer.EXPECT().DeepEqual(gomock.Any(), gomock.Any()).
			Return(deepEqualResponse).
			Times(deepEqualCalledTimes)

		routeMonitor = v1alpha1.RouteMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Finalizers: routeMonitorFinalizers,
				Name:       routeMonitorName,
				Namespace:  routeMonitorNamespace,
			},
			Status: routeMonitorStatus,
		}
	})
	AfterEach(func() {
		mockCtrl.Finish()
	})
	Describe("EnsureFinalizerSet", func() {
		When("Updating the finalizers using the 'Update' func failed unexpectedly", func() {
			// Arrange
			BeforeEach(func() {
				routeMonitorAdderClient = mockClient
				update = helper.CustomErrorHappensOnce()
				routeMonitorFinalizers = nil
				routeMonitorStatus = v1alpha1.RouteMonitorStatus{
					RouteURL: "fake-route-url",
				}
			})
			It("should bubble up the error", func() {
				// Act
				_, err := routeMonitorAdder.EnsureFinalizerSet(ctx, routeMonitor)
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(consterror.CustomError))
			})
		})
		When("the RouteMonitor has no Finalizer", func() {
			// Arrange
			BeforeEach(func() {
				routeMonitorAdderClient = mockClient
				update.CalledTimes = 1
				routeMonitorFinalizers = nil
				routeMonitorStatus = v1alpha1.RouteMonitorStatus{
					RouteURL: "fake-route-url",
				}
			})
			It("Should update the RouteMonitor with the finalizer", func() {
				// Act
				resp, err := routeMonitorAdder.EnsureFinalizerSet(ctx, routeMonitor)
				// Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(resp).NotTo(BeNil())
				Expect(resp).To(Equal(utilreconcile.StopOperation()))
			})
		})
	})
	Describe("EnsureServiceMonitorResourceExists", func() {
		When("the RouteMonitor has no Host", func() {
			// Arrange
			BeforeEach(func() {
				routeMonitorAdderClient = fake.NewFakeClientWithScheme(scheme)
				routeMonitorStatus = v1alpha1.RouteMonitorStatus{}
			})
			It("should return No Host error", func() {
				// Act
				_, err := routeMonitorAdder.EnsureServiceMonitorResourceExists(ctx, routeMonitor)
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(customerrors.NoHost))
			})
		})
		Describe("Testing ServiceMonitor creation", func() {
			BeforeEach(func() {
				// Arrange
				get = helper.NotFoundErrorHappensOnce()
				create.CalledTimes = 1
				get.CalledTimes = 1
				routeMonitorStatus = v1alpha1.RouteMonitorStatus{
					RouteURL: "fake-route-url",
				}
				routeMonitorFinalizers = routemonitorconst.FinalizerList
			})
			When("the resource Get fails unexpectedly", func() {
				// Arrange
				BeforeEach(func() {
					get.ErrorResponse = consterror.CustomError
					mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).
						Return(get.ErrorResponse).
						Times(get.CalledTimes)
					create.CalledTimes = 0
				})

				It("should return the error and not call `Create`", func() {
					//Act
					_, err := routeMonitorAdder.EnsureServiceMonitorResourceExists(ctx, routeMonitor)
					//Assert
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(consterror.CustomError))
				})
			})
			When("the resource Create fails unexpectedly", func() {
				BeforeEach(func() {
					// Arrange
					mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).
						Return(get.ErrorResponse).
						Times(get.CalledTimes)
					create.ErrorResponse = consterror.CustomError
				})
				It("should call `Get` Successfully and call `Create` but return the error", func() {
					//Act
					_, err := routeMonitorAdder.EnsureServiceMonitorResourceExists(ctx, routeMonitor)
					//Assert
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(consterror.CustomError))
				})
			})
			When("the resource Exists", func() {
				BeforeEach(func() {
					// Arrange
					get.ErrorResponse = nil
					create.CalledTimes = 0
					deepEqualResponse = true
					deepEqualCalledTimes = 1
				})
				JustBeforeEach(func() {
					namespacedName := types.NamespacedName{Name: routeMonitor.Name, Namespace: routeMonitor.Namespace}
					serviceMonintor := templates.TemplateForServiceMonitorResource(routeMonitor.Status.RouteURL, routeMonitorAdder.BlackBoxExporterNamespace, namespacedName)
					mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, serviceMonintor).
						Return(get.ErrorResponse).
						Times(get.CalledTimes)
					routeMonitor.Status.ServiceMonitorRef = v1alpha1.NamespacedName{
						Name:      routeMonitor.Name,
						Namespace: routeMonitor.Namespace,
					}
				})
				It("should pass all checks and continue", func() {
					//Act
					_, err := routeMonitorAdder.EnsureServiceMonitorResourceExists(ctx, routeMonitor)
					//Assert
					Expect(err).NotTo(HaveOccurred())
				})
			})
			When("ServiceMonitorRef exists and is equal to the RouteMonitor name", func() {
				JustBeforeEach(func() {
					namespacedName := types.NamespacedName{Name: routeMonitor.Name, Namespace: routeMonitor.Namespace}
					serviceMonintor := templates.TemplateForServiceMonitorResource(routeMonitor.Status.RouteURL, routeMonitorAdder.BlackBoxExporterNamespace, namespacedName)
					mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, serviceMonintor).
						Return(get.ErrorResponse).
						Times(get.CalledTimes)

					routeMonitor.Status.ServiceMonitorRef = v1alpha1.NamespacedName{
						Name:      routeMonitor.Name,
						Namespace: routeMonitor.Namespace,
					}
					deepEqualCalledTimes = 1
					deepEqualResponse = true
				})
				// Arrange
				It("Work correctly and continue processing", func() {
					//Act
					res, err := routeMonitorAdder.EnsureServiceMonitorResourceExists(ctx, routeMonitor)
					//Assert
					Expect(err).NotTo(HaveOccurred())
					Expect(res).NotTo(BeNil())
					Expect(res).To(Equal(utilreconcile.ContinueOperation()))
				})
			})
<<<<<<< HEAD

			When("ServiceMonitorRef exists and is equal to the RouteMonitor name", func() {
				JustBeforeEach(func() {
					namespacedName := types.NamespacedName{Name: routeMonitor.Name, Namespace: routeMonitor.Namespace}
					serviceMonintor := templates.TemplateForServiceMonitorResource(routeMonitor.Status.RouteURL, routeMonitorAdder.BlackBoxExporterNamespace, namespacedName)
					mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, serviceMonintor).
						Return(get.ErrorResponse).
						Times(get.CalledTimes)

					routeMonitor.Status.ServiceMonitorRef = v1alpha1.NamespacedName{
						Name:      routeMonitor.Name,
						Namespace: routeMonitor.Namespace,
					}
				})
				// Arrange
				It("Work correctly and continue processing", func() {
					//Act
					res, err := routeMonitorAdder.EnsureServiceMonitorResourceExists(ctx, routeMonitor)
					//Assert
					Expect(err).NotTo(HaveOccurred())
					Expect(res).NotTo(BeNil())
					Expect(res).To(Equal(utilreconcile.ContinueOperation()))
=======
			Describe("Testing ServiceMonitor update", func() {
				When("the ServiceMonitor template specs were updated", func() {
					// Arrange
					BeforeEach(func() {
						get.ErrorResponse = nil
						get.CalledTimes = 1
						create.CalledTimes = 0
						update.CalledTimes = 1
						deepEqualCalledTimes = 1
						deepEqualResponse = false
					})
					JustBeforeEach(func() {
						routeMonitor.Status.ServiceMonitorRef = v1alpha1.NamespacedName{
							Name:      routeMonitor.Name,
							Namespace: routeMonitor.Namespace,
						}
					})
					It("should update the deployed ServiceMonitor", func() {
						//Act
						res, err := routeMonitorAdder.EnsureServiceMonitorResourceExists(ctx, routeMonitor)
						//Assert
						Expect(err).NotTo(HaveOccurred())
						Expect(res).NotTo(BeNil())
						Expect(res).To(Equal(utilreconcile.ContinueOperation()))
					})
>>>>>>> 4295119 (added ClusterID to templates)
				})
			})
		})

		Context("Updating the ServiceMonitorRef", func() {
			When("the ref of servicemonitor is not equal to the expected output", func() {
				BeforeEach(func() {
					// Arrange
					routeMonitorStatus = v1alpha1.RouteMonitorStatus{
						RouteURL: "fake-route-url",
					}
					routeMonitorFinalizers = routemonitorconst.FinalizerList
					get.CalledTimes = 1
					deepEqualResponse = true
					deepEqualCalledTimes = 1
				})

				JustBeforeEach(func() {
					namespacedName := types.NamespacedName{Name: routeMonitor.Name, Namespace: routeMonitor.Namespace}
					serviceMonintor := templates.TemplateForServiceMonitorResource(routeMonitor.Status.RouteURL, routeMonitorAdder.BlackBoxExporterNamespace, namespacedName)
					mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, serviceMonintor).
						Return(get.ErrorResponse).
						Times(get.CalledTimes)

					routeMonitor.Status.ServiceMonitorRef = v1alpha1.NamespacedName{
						Name:      routeMonitor.Name + "-but-different",
						Namespace: routeMonitor.Namespace,
					}
				})

				It("Should throw an 'this is currently unsupported' error", func() {
					//Act
					_, err := routeMonitorAdder.EnsureServiceMonitorResourceExists(ctx, routeMonitor)
					//Assert
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(customerrors.InvalidReferenceUpdate))
				})
			})
			When("the ref in ServiceMonitorRef is empty", func() {
				BeforeEach(func() {
					// Arrange
					routeMonitorStatus = v1alpha1.RouteMonitorStatus{
						RouteURL: "fake-route-url",
					}
					routeMonitorFinalizers = routemonitorconst.FinalizerList
					get.CalledTimes = 1
					deepEqualResponse = true
					deepEqualCalledTimes = 1
					mockClient.EXPECT().Status().Return(mockStatusWriter).Times(1)
				})

				JustBeforeEach(func() {
					namespacedName := types.NamespacedName{Name: routeMonitor.Name, Namespace: routeMonitor.Namespace}
					serviceMonintor := templates.TemplateForServiceMonitorResource(routeMonitor.Status.RouteURL, routeMonitorAdder.BlackBoxExporterNamespace, namespacedName)
					mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, serviceMonintor).
						Return(get.ErrorResponse).
						Times(get.CalledTimes)

					mockStatusWriter.EXPECT().Update(gomock.Any(), gomock.Any()).
						Times(1).
						Return(nil)
					routeMonitor.Status.ServiceMonitorRef = v1alpha1.NamespacedName{}
				})

				It("Update the service monitor ref and stop processing", func() {
					//Act
					res, err := routeMonitorAdder.EnsureServiceMonitorResourceExists(ctx, routeMonitor)
					//Assert
					Expect(err).NotTo(HaveOccurred())
					Expect(res).NotTo(BeNil())
					Expect(res).To(Equal(utilreconcile.StopOperation()))
				})
			})
			When("the ref in ServiceMonitorRef is equal to the expected resource", func() {
				BeforeEach(func() {
					// Arrange
					routeMonitorStatus = v1alpha1.RouteMonitorStatus{
						RouteURL: "fake-route-url",
					}
					routeMonitorFinalizers = routemonitorconst.FinalizerList
					get.CalledTimes = 1
					deepEqualResponse = true
					deepEqualCalledTimes = 1
				})

				JustBeforeEach(func() {
					namespacedName := types.NamespacedName{Name: routeMonitor.Name, Namespace: routeMonitor.Namespace}
					serviceMonintor := templates.TemplateForServiceMonitorResource(routeMonitor.Status.RouteURL, routeMonitorAdder.BlackBoxExporterNamespace, namespacedName)
					mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, serviceMonintor).
						Return(get.ErrorResponse).
						Times(get.CalledTimes)

					routeMonitor.Status.ServiceMonitorRef = v1alpha1.NamespacedName{
						Name:      routeMonitor.Name,
						Namespace: routeMonitor.Namespace,
					}
				})

				It("should skip any operation and continue processing", func() {
					//Act
					res, err := routeMonitorAdder.EnsureServiceMonitorResourceExists(ctx, routeMonitor)
					//Assert
					Expect(err).NotTo(HaveOccurred())
					Expect(res).NotTo(BeNil())
					Expect(res).To(Equal(utilreconcile.ContinueOperation()))
				})
			})
		})
	})

	Describe("New", func() {
		When("func New is called", func() {
			It("should return a new Adder object", func() {
				// Arrange
				r := routemonitor.RouteMonitorReconciler{
					Client: routeMonitorAdderClient,
					Log:    constinit.Logger,
					Scheme: constinit.Scheme,
				}
				// Act
				res := adder.New(r, "")
				// Assert
				Expect(res).To(Equal(&adder.RouteMonitorAdder{
					Client: routeMonitorAdderClient,
					Log:    constinit.Logger,
					Scheme: constinit.Scheme,
				}))
			})
		})
	})
})
