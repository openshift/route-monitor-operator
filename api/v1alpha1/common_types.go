package v1alpha1

import "gopkg.in/inf.v0"

// NamespacedName contains the name of a object and its namespace
type NamespacedName struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// SloSpec defines what is the percentage
type SloSpec struct {
	// Value defines the whole number being used
	Value   string  `json:"value"`
	SloType SloType `json:"type"`
}

// +kubebuilder:validation:Enum=percent;percentile
type SloType string

const (
	Percent    SloType = "percent"
	Percentile SloType = "percentile"
)

func (s SloSpec) NormalizeValue() (string, bool) {
	switch s.SloType {
	case Percent:
		d, sucess := new(inf.Dec).SetString(s.Value)
		if !sucess {
			return "", false
		}
		percentile := d.Mul(inf.NewDec(1, -2), d)
		return percentile.String(), true
	case Percentile:
		return s.Value, true
	}
	return "", false
}

func (s SloSpec) IsValid() bool {
	switch s.SloType {
	case Percent:
		d, sucess := new(inf.Dec).SetString(s.Value)
		if !sucess {
			return false
		}

		// Is SLO a negative number
		if d.Sign() <= 0 {
			return false
		}

		// Is SLO higher that 100% availability
		diff := d.Sub(d, inf.NewDec(100, 0))
		if diff.Sign() >= 0 {
			return false
		}
		return true
	case Percentile:
		return false
	}
	return true
}
