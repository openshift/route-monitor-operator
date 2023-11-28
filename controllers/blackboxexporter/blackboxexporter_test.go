package blackboxexporter_test

import (
	"time"

	"github.com/golang/mock/gomock"
	fuzz "github.com/google/gofuzz"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	"github.com/openshift/route-monitor-operator/controllers/blackboxexporter"
	consterror "github.com/openshift/route-monitor-operator/pkg/consts/test/error"
	constinit "github.com/openshift/route-monitor-operator/pkg/consts/test/init"

	utilreconcile "github.com/openshift/route-monitor-operator/pkg/util/reconcile"
	clientmocks "github.com/openshift/route-monitor-operator/pkg/util/test/generated/mocks/client"
	"github.com/openshift/route-monitor-operator/pkg/util/test/helper"
)

var _ = Describe("BlackBoxExporter", func() {
	var (
		mockClient *clientmocks.MockClient
		mockCtrl   *gomock.Controller

		update helper.MockHelper
		delete helper.MockHelper
		get    helper.MockHelper
		create helper.MockHelper

		blackBoxExporterReconciler        blackboxexporter.BlackBoxExporterReconciler
		blackBoxExporter                  v1alpha1.BlackBoxExporter
		blackBoxExporterDeletionTimestamp *metav1.Time
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockClient = clientmocks.NewMockClient(mockCtrl)

		blackBoxExporterReconciler = blackboxexporter.BlackBoxExporterReconciler{
			Log:    constinit.Logger,
			Client: mockClient,
			Scheme: constinit.Scheme,
		}

		update = helper.MockHelper{}
		delete = helper.MockHelper{}
		get = helper.MockHelper{}
		create = helper.MockHelper{}

		blackBoxExporterDeletionTimestamp = &metav1.Time{}
		blackBoxExporter = v1alpha1.BlackBoxExporter{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "test-name",
				Namespace:         "test-namespace",
				DeletionTimestamp: blackBoxExporterDeletionTimestamp,
			},
			Status: v1alpha1.BlackBoxExporterStatus{},
			Spec: v1alpha1.BlackBoxExporterSpec{
				Image: "quay.io/prometheus/blackbox-exporter:master",
				NodeSelector: corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{{
						MatchExpressions: []corev1.NodeSelectorRequirement{{
							Key:      "node-role.kubernetes.io/infra",
							Operator: corev1.NodeSelectorOpExists,
						}},
					}},
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

	Describe("GetBlackBoxExporter", func() {
		f := fuzz.New()

		var (
			scheme                    *runtime.Scheme
			req                       ctrl.Request
			blackBoxExporterName      string
			blackBoxExporterNamespace string

			resBlackBoxExporter v1alpha1.BlackBoxExporter
			res                 utilreconcile.Result
			err                 error
		)

		f.Fuzz(&blackBoxExporterName)
		f.Fuzz(&blackBoxExporterNamespace)

		JustBeforeEach(func() {
			scheme = constinit.Scheme
			req = ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      "foo", // blackBoxExporterName,
					Namespace: "bar", // blackBoxExporterNamespace,
				},
			}

			resBlackBoxExporter, res, err = blackBoxExporterReconciler.GetBlackBoxExporter(req)
		})

		When("func Get fails unexpectedly", func() {
			BeforeEach(func() {
				get = helper.CustomErrorHappensOnce()
			})
			It("should stop requeue", func() {
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(consterror.CustomError))
			})
		})
		When("the BlackBoxExporter is not found", func() {
			BeforeEach(func() {
				blackBoxExporterReconciler.Client = fake.NewClientBuilder().WithScheme(scheme).Build()
			})
			It("should stop requeue", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(res).To(Equal(utilreconcile.StopOperation()))
				Expect(resBlackBoxExporter).To(BeZero())
			})
		})
	})
	Describe("Create Resources", func() {
		var (
			namespacedName types.NamespacedName
			err            error
		)
		When("the Deployment template is created", func() {
			BeforeEach(func() {
				namespacedName = types.NamespacedName{
					Name:      "foo",
					Namespace: "bar",
				}
				_, err = blackBoxExporterReconciler.DeploymentTemplate(namespacedName, blackBoxExporter)
			})
			It("doesn't fail", func() {
				Expect(err).NotTo(HaveOccurred())
			})
		})
		When("the Service template is created", func() {
			BeforeEach(func() {
				namespacedName = types.NamespacedName{
					Name:      "foo",
					Namespace: "bar",
				}
				_, err = blackBoxExporterReconciler.ServiceTemplate(namespacedName)
			})
			It("doesn't fail", func() {
				Expect(err).NotTo(HaveOccurred())
			})
		})
		When("the create image field is not set", func() {
			BeforeEach(func() {
				blackBoxExporter.Spec.Image = ""
				err = blackBoxExporterReconciler.CreateResources(blackBoxExporter)
			})
			It("will requeue with the error", func() {
				Expect(err).To(HaveOccurred())
			})
		})
	})
	Describe("Delete BlackBoxExporter", func() {
		var (
			err error
		)
		JustAfterEach(func() {
			err = blackBoxExporterReconciler.DeleteResources(blackBoxExporter)
		})
		When("the BlackBoxExporter has a deleted timestamp", func() {
			BeforeEach(func() {
				blackBoxExporter.ObjectMeta.DeletionTimestamp = &metav1.Time{
					Time: time.Now(),
				}
				get.CalledTimes = 2
				delete.CalledTimes = 2
				update.CalledTimes = 1 // Removes finalizer from BlackBoxExporter
			})
			It("deletes the BlackBoxExporter", func() {
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})
