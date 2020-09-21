module github.com/openshift/route-monitor-operator

go 1.13

require (
	github.com/go-logr/logr v0.1.0
	github.com/golang/mock v1.4.3
	github.com/onsi/ginkgo v1.11.0
	github.com/onsi/gomega v1.8.1
	github.com/openshift/api v3.9.0+incompatible
	github.com/prometheus-operator/prometheus-operator v0.41.1-0.20200806133437-e7d55e3fea24
	k8s.io/api v0.18.3
	k8s.io/apimachinery v0.18.3
	k8s.io/client-go v0.18.3
	sigs.k8s.io/controller-runtime v0.6.0
)
