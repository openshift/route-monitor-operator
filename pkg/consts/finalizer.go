package consts

const (
	FinalizerKey string = "routemonitor.routemonitoroperator.monitoring.openshift.io/finalizer"
	// PrevFinalizerKey is here until migration to new key is done
	PrevFinalizerKey string = "finalizer.routemonitor.openshift.io"
)

var (
	FinalizerList = []string{FinalizerKey}
)
