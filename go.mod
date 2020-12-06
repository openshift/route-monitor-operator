module github.com/openshift/route-monitor-operator

go 1.13

require (
	github.com/go-logr/logr v0.1.0
	github.com/golang/mock v1.4.4
	github.com/google/gofuzz v1.2.0
	github.com/onsi/ginkgo v1.12.1
	github.com/onsi/gomega v1.10.1
	github.com/openshift/api v0.0.0-20200116145750-0e2ff1e215dd
	github.com/prometheus-operator/prometheus-operator v0.41.1-0.20200806133437-e7d55e3fea24
	k8s.io/api v0.18.6
	k8s.io/apimachinery v0.18.6
	k8s.io/client-go v0.18.6
	sigs.k8s.io/controller-runtime v0.6.3
)
