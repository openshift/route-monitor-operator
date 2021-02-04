package blackbox

const ( // All things related BlackBoxExporter
	BlackBoxName       = "blackbox-exporter"
	BlackBoxPortName   = "blackbox"
	BlackBoxPortNumber = 9115
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
