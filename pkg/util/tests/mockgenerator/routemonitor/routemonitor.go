package routemonitor

//go:generate mockgen -destination ../../generated/mocks/routemonitor/routemonitor.go -package $GOPACKAGE github.com/openshift/route-monitor-operator/controllers/routemonitor  RouteMonitorInterface
