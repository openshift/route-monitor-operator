package v1alpha1

// NamespacedName contains the name of a object and its namespace
type NamespacedName struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}
