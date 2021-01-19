package v1alpha1

// NamespacedName contains the name of a object and its namespace
type NamespacedName struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// SlaSpec defines what is the percentage
type SlaSpec struct {
	// Percentile is the fraction of the time that resource needs to be availble at
	Percentile int `json:"percentile"`
	// Precision is the precision the percentile will be (default to the length of the percentile + 1)
	Precision int `json:"precision,omitempty"`
}
