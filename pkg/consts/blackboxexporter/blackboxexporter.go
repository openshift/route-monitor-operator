package blackboxexporter

import (
	"context"

	configv1 "github.com/openshift/api/config/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const ( // All things related BlackBoxExporter
	BlackBoxExporterName       = "blackbox-exporter"
	BlackBoxExporterPortName   = "blackbox"
	BlackBoxExporterPortNumber = 9115
)

var blackBoxLabels = map[string]string{
	"app": BlackBoxExporterName,
}

func GetBlackBoxLabels(c client.Client) (map[string]string, error) {
	if blackBoxLabels["clusterid"] == "" {
		err := populateBlackBoxExporterLabels(c)
		if err != nil {
			return blackBoxLabels, err
		}
	}
	return blackBoxLabels, nil
}

// PopulateBlackBoxExporterLabels creates a set of common labels to most resources
// This function is called in the main to populate information not known at startup
// this function is here in case we need more labels in the future.
func populateBlackBoxExporterLabels(c client.Client) error {
	//Get clusterID for the labels map
	var version configv1.ClusterVersion
	err := c.Get(context.TODO(), client.ObjectKey{Name: "version"}, &version)
	if err != nil {
		return err
	}

	blackBoxLabels["clusterid"] = string(version.Spec.ClusterID)
	return nil
}

type ShouldDeleteBlackBoxExporter bool

var (
	KeepBlackBoxExporter   ShouldDeleteBlackBoxExporter = false
	DeleteBlackBoxExporter ShouldDeleteBlackBoxExporter = true
)
