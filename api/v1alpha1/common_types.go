package v1alpha1

import "gopkg.in/inf.v0"

// NamespacedName contains the name of a object and its namespace
type NamespacedName struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// SloSpec defines what is the percentage
type SloSpec struct {
	// TargetAvailabilityPercentile defines the percentile number to be used
	TargetAvailabilityPercentile string `json:"targetAvailabilityPercentile"`
}

func (s SloSpec) IsValid() bool {
	if s.TargetAvailabilityPercentile == "" {
		return false
	}

	d, sucess := new(inf.Dec).SetString(s.TargetAvailabilityPercentile)
	// value is not parsable
	if !sucess {
		return false
	}

	// will be 0.9
	ninty := inf.NewDec(9, 1)
	// is higher than lower bound
	nintyPercentDiff := new(inf.Dec).Sub(d, ninty)
	if nintyPercentDiff.Sign() <= 0 {
		return false
	}

	// will be 1.0
	hundred := inf.NewDec(1, 0)
	// is higher than upper bound
	hundredPercentDiff := new(inf.Dec).Sub(d, hundred)
	if hundredPercentDiff.Sign() >= 0 {
		return false
	}

	return true
}
