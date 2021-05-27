module github.com/openshift/route-monitor-operator

go 1.16

require (
	github.com/go-logr/logr v0.4.0
	github.com/golang/mock v1.6.0
	github.com/google/gofuzz v1.2.0
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.13.0
	github.com/openshift/api v0.0.0-20200917102736-0a191b5b9bb0
	github.com/operator-framework/operator-sdk v1.2.0 // indirect
	github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring v0.48.1
	github.com/prometheus/common v0.26.0
	gopkg.in/inf.v0 v0.9.1
	k8s.io/api v0.21.2
	k8s.io/apimachinery v0.21.2
	k8s.io/client-go v0.21.2
	sigs.k8s.io/controller-runtime v0.9.2
)

replace github.com/googleapis/gnostic => github.com/googleapis/gnostic v0.4.0
