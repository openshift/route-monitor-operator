package blackboxexporter

const ( // All things related BlackBoxExporter
	BlackBoxExporterName         = "blackbox-exporter"
	BlackBoxExporterPortName     = "blackbox"
	BlackBoxExporterPortNumber   = 9115
	BlackBoxExporterFinalizerKey = "blackboxexporter.routemonitoroperator.monitoring.openshift.io/finalizer"
)

// generateBlackBoxLables creates a set of common labels to most resources
// this function is here in case we need more labels in the future
func GenerateBlackBoxExporterLables() map[string]string {
	return map[string]string{"app": BlackBoxExporterName}
}

type ShouldDeleteBlackBoxExporter bool

var (
	KeepBlackBoxExporter   ShouldDeleteBlackBoxExporter = false
	DeleteBlackBoxExporter ShouldDeleteBlackBoxExporter = true
)
