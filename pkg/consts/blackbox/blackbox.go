package blackbox

import "k8s.io/apimachinery/pkg/types"

const ( // All things related BlackBoxExporter
	BlackBoxNamespace  = "openshift-monitoring"
	BlackBoxName       = "blackbox-exporter"
	BlackBoxPortName   = "blackbox"
	BlackBoxPortNumber = 9115
)

var ( // cannot be a const but doesn't ever change
	BlackBoxNamespacedName = types.NamespacedName{Name: BlackBoxName, Namespace: BlackBoxNamespace}
)

// generateBlackBoxLables creates a set of common labels to most resources
// this function is here in case we need more labels in the future
func GenerateBlackBoxLables() map[string]string {
	return map[string]string{"app": BlackBoxName}
}

type ShouldDeleteBlackBoxExporter bool

var (
	KeepBlackBoxExporter   ShouldDeleteBlackBoxExporter = false
	DeleteBlackBoxExporter ShouldDeleteBlackBoxExporter = true
)
