package hostedcontrolplane

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	hypershiftv1beta1 "github.com/openshift/hypershift/api/v1beta1"
	"github.com/openshift/route-monitor-operator/api/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("hostedcontrolplane_controller", func() {
	//DescribeTable("summations", 
	//	func(a, b int, expectedSum int) {
	//		sum := a + b
	//		Expect(sum).To(Equal(expectedSum))
	//	}, 
	//Entry("test 1+1", 1, 1, 2),
	//Entry("test 2+2", 2, 2, 4),
	//Entry("test 2+1", 2, 1, 4), // fail
	//)

	// Test vars
	var (
		ctx context.Context
		defaultHCP hypershiftv1beta1.HostedControlPlane
	)

	// Redefine test variables between each run to avoid cross contamination
	BeforeEach(func() {
		ctx = context.TODO()

		defaultHCP = hypershiftv1beta1.HostedControlPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
				Namespace: "test",
				Finalizers: []string{hostedcontrolplaneFinalizer},
			},
			Spec: hypershiftv1beta1.HostedControlPlaneSpec{
			},
			Status: hypershiftv1beta1.HostedControlPlaneStatus{
				Ready: true,
			},
		}
	})

	type test struct {
		reconciler HostedControlPlaneReconciler
		req ctrl.Request
	}

	type result struct {
		//result ctrl.Result
		err bool
		hcp hypershiftv1beta1.HostedControlPlane
	}

	DescribeTable("reconciling HostedControlPlanes",
		// Test definition
		func(setup func() test, expect func() result) {
			// Setup
			test := setup()
			r    := test.reconciler
			req  := test.req

			// Run
			result, err := r.Reconcile(ctx, req)

			fmt.Printf("result: %v\n", result)
			fmt.Printf("err: %v\n", err)

			// Evaluate
			expected := expect()

			fmt.Printf("expected: %v\n", expected)

			//Expect(result).To(Equal(expected.result))
			//Expect(err).To(Equal(expected.err))


			if expected.err {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
			}

			currentHCP := hypershiftv1beta1.HostedControlPlane{}
			err = r.Get(ctx, req.NamespacedName, &currentHCP)
			Expect(err).ToNot(HaveOccurred())
			Expect(currentHCP).To(Equal(expected.hcp))
		},

		// Test entries
		Entry("when HCP does not exist",
			func() test {
				return test {
					reconciler: buildReconciler(),
					req: buildRequest(defaultHCP),
				}
			},
			func() result {
				return result{
					//result: ctrl.Result{},
					err: false,
					hcp: defaultHCP,
				}
			},
		),

	//	Entry("when deletion timestamp set",
	//		func() testInput {
	//			defaultHCP.SetDeletionTimestamp(&metav1.Time{Time: time.Now()})
	//			return testInput{hcp: hcp}
	//		},
	//		testResult{
	//			hcp = defaultHCP
	//		},
	//	),
	//	Entry("when finalizer unset",
	//		testInput{},
	//		testResult{},
	//	),
	//	Entry("when HCP unready",
	//		testInput{},
	//		testResult{},
	//	),
	//	Entry("when route does not exist",
	//		testInput{},
	//		testResult{},
	//	),
	//	Entry("when routemonitor does not exist",
	//		testInput{},
	//		testResult{},
	//	),
	)
})

// buildRequest creates a request object for testing from the given HCP object
func buildRequest(hcp hypershiftv1beta1.HostedControlPlane) ctrl.Request {
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: hcp.Name,
			Namespace: hcp.Namespace,
		},
	}
	return req
}

// buildReconciler creates a reconciler for testing the provided objects
func buildReconciler(objs ...client.Object) HostedControlPlaneReconciler {
	c := buildClient(objs...)
	r := HostedControlPlaneReconciler{
		Client: c,
		Scheme: c.Scheme(),
	}
	return r
}

// buildClient creates a fake client pre-populated with the provided objects
func buildClient(objs ...client.Object) client.WithWatch {
	var err error
	s := scheme.Scheme

	err = hypershiftv1beta1.AddToScheme(s)
	Expect(err).ToNot(HaveOccurred())

	err = v1alpha1.AddToScheme(s)
	Expect(err).ToNot(HaveOccurred())

	builder := fake.NewClientBuilder().WithScheme(s).WithObjects(objs...)
	Expect(builder).ToNot(BeNil())
	return builder.Build()
}
