package v1alpha1

import "gopkg.in/inf.v0"

// NamespacedName contains the name of a object and its namespace
type NamespacedName struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// SloSpec defines what is the percentage
type SloSpec struct {
	// TargetAvailabilityPercent defines the percent number to be used
	TargetAvailabilityPercent string `json:"targetAvailabilityPercent"`
}

func (s SloSpec) IsValid() (bool, string) {
	if s.TargetAvailabilityPercent == "" {
		return false, ""
	}

	d, sucess := new(inf.Dec).SetString(s.TargetAvailabilityPercent)
	// value is not parsable
	if !sucess {
		return false, ""
	}

	// will be 90
	ninty := inf.NewDec(9, -1)
	// is lower than lower bound
	if d.Cmp(ninty) <= 0 {
		return false, ""
	}

	// will be 100
	hundred := inf.NewDec(1, -2)
	// is higher than upper bound
	if d.Cmp(hundred) >= 0 {
		return false, ""
	}

	// will be 1/100
	oneHundredth := inf.NewDec(1, 2)

	// if value / 100 is invalid
	res := d.Mul(d, oneHundredth).String()
	if res == "" || res == "<nil>" {
		return false, ""
	}
	return true, res
}
