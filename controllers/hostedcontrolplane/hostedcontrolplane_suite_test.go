package hostedcontrolplane

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

//	hypershiftv1beta1 "github.com/openshift/hypershift/api/v1beta1"
//	"github.com/openshift/route-monitor-operator/api/v1alpha1"
//
//	"k8s.io/client-go/kubernetes/scheme"
//	"sigs.k8s.io/controller-runtime/pkg/client"
//	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestRoutemonitor(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "HostedControlPlane Suite")
}
//
//// buildClient creates a fake client object pre-populated with the provided objects
//func buildClient(objs ...client.Object) client.WithWatch {
//	var err error
//	s := scheme.Scheme
//
//	err = hypershiftv1beta1.AddToScheme(s)
//	Expect(err).ToNot(HaveOccurred())
//
//	err = v1alpha1.AddToScheme(s)
//	Expect(err).ToNot(HaveOccurred())
//
//	builder := fake.NewClientBuilder().WithScheme(s).WithObjects(objs...)
//	Expect(builder).ToNot(BeNil())
//	return builder.Build()
//}
