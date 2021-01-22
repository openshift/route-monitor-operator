package v1alpha1

// NamespacedName contains the name of a object and its namespace
type NamespacedName struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// RawSloSpec defines the number form of the percent, and is derrived from the 'inf' package
type RawSloSpec struct {
	// Value defines the whole number being used
	// +kubebuilder:validation:Minimum=10
	Value int `json:"value"`
}

// SloSpec defines what is the percentage
type SloSpec struct {
	// Raw defines the raw value (value and shiftDirection) needed to express smaller percents
	Raw RawSloSpec `json:"raw"`
}
