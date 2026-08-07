package domain

import (
	"fmt"
	"strings"
	"time"
)

type PricingInputSnapshot struct {
	tenantID         TenantID
	scope            PricingScopeID
	packageID        PackageID
	zone             string
	actualWeight     Weight
	volumetricWeight *Weight
	businessAt       time.Time
	factReferences   []VersionedFactReference
}

func NewPricingInputSnapshot(
	tenantID TenantID,
	scope PricingScopeID,
	packageID PackageID,
	zone string,
	actualWeight Weight,
	volumetricWeight *Weight,
	businessAt time.Time,
	factReferences ...VersionedFactReference,
) (PricingInputSnapshot, error) {
	if !tenantID.valid() || !scope.valid() || !packageID.valid() || strings.TrimSpace(zone) == "" || strings.TrimSpace(zone) != zone || !actualWeight.valid() || !validBusinessTime(businessAt) {
		return PricingInputSnapshot{}, ErrPricingInputInvalid
	}
	if volumetricWeight != nil {
		if !volumetricWeight.valid() {
			return PricingInputSnapshot{}, ErrPricingInputInvalid
		}
		if volumetricWeight.unit != actualWeight.unit {
			return PricingInputSnapshot{}, ErrWeightUnitMismatch
		}
	}
	copyOfReferences := append([]VersionedFactReference(nil), factReferences...)
	for _, reference := range copyOfReferences {
		if !reference.valid() {
			return PricingInputSnapshot{}, ErrPricingInputInvalid
		}
	}
	var volumetricCopy *Weight
	if volumetricWeight != nil {
		copy := *volumetricWeight
		volumetricCopy = &copy
	}
	return PricingInputSnapshot{
		tenantID:         tenantID,
		scope:            scope,
		packageID:        packageID,
		zone:             zone,
		actualWeight:     actualWeight,
		volumetricWeight: volumetricCopy,
		businessAt:       businessAt,
		factReferences:   copyOfReferences,
	}, nil
}

func (input PricingInputSnapshot) TenantID() TenantID    { return input.tenantID }
func (input PricingInputSnapshot) Scope() PricingScopeID { return input.scope }
func (input PricingInputSnapshot) PackageID() PackageID  { return input.packageID }
func (input PricingInputSnapshot) Zone() string          { return input.zone }
func (input PricingInputSnapshot) ActualWeight() Weight  { return input.actualWeight }
func (input PricingInputSnapshot) BusinessAt() time.Time { return input.businessAt }
func (input PricingInputSnapshot) FactReferences() []VersionedFactReference {
	return append([]VersionedFactReference(nil), input.factReferences...)
}

func (input PricingInputSnapshot) VolumetricWeight() (Weight, bool) {
	if input.volumetricWeight == nil {
		return Weight{}, false
	}
	return *input.volumetricWeight, true
}

func (input PricingInputSnapshot) valid() bool {
	if !input.tenantID.valid() || !input.scope.valid() || !input.packageID.valid() || strings.TrimSpace(input.zone) == "" || strings.TrimSpace(input.zone) != input.zone || !input.actualWeight.valid() || !validBusinessTime(input.businessAt) {
		return false
	}
	if input.volumetricWeight != nil && (!input.volumetricWeight.valid() || input.volumetricWeight.unit != input.actualWeight.unit) {
		return false
	}
	for _, reference := range input.factReferences {
		if !reference.valid() {
			return false
		}
	}
	return true
}

type BillableWeightResult struct {
	method       BillableWeightMethod
	actual       Weight
	volumetric   *Weight
	raw          Weight
	rounded      Weight
	roundingMode RoundingMode
	increment    Weight
	explanation  string
}

func CalculateBillableWeight(input PricingInputSnapshot, policy BillableWeightPolicy, expectedUnit WeightUnit) (BillableWeightResult, error) {
	if !input.valid() || !policy.valid() {
		return BillableWeightResult{}, ErrPricingInputInvalid
	}
	if input.actualWeight.unit != expectedUnit {
		return BillableWeightResult{}, ErrWeightUnitMismatch
	}
	if policy.rounding.increment.unit != expectedUnit {
		return BillableWeightResult{}, ErrWeightUnitMismatch
	}
	var raw Weight
	volumetric, hasVolumetric := input.VolumetricWeight()
	switch policy.method {
	case BillableWeightActualOnly:
		raw = input.actualWeight
	case BillableWeightMax:
		if !hasVolumetric {
			return BillableWeightResult{}, ErrMissingVolumetricWeight
		}
		comparison := input.actualWeight.value.Cmp(volumetric.value)
		if comparison >= 0 {
			raw = input.actualWeight
		} else {
			raw = volumetric
		}
	default:
		return BillableWeightResult{}, ErrPricingInputInvalid
	}
	roundedValue, err := raw.value.RoundToIncrement(policy.rounding.increment.value, policy.rounding.mode)
	if err != nil {
		return BillableWeightResult{}, err
	}
	rounded, err := NewWeight(roundedValue, raw.unit)
	if err != nil {
		return BillableWeightResult{}, err
	}
	explanation := fmt.Sprintf("billable weight uses %s; raw=%s %s; rounding=%s increment=%s %s; rounded=%s %s", policy.method, raw.value.String(), raw.unit, policy.rounding.mode, policy.rounding.increment.value.String(), policy.rounding.increment.unit, rounded.value.String(), rounded.unit)
	return BillableWeightResult{
		method:       policy.method,
		actual:       input.actualWeight,
		volumetric:   optionalWeightCopy(volumetric, hasVolumetric),
		raw:          raw,
		rounded:      rounded,
		roundingMode: policy.rounding.mode,
		increment:    policy.rounding.increment,
		explanation:  explanation,
	}, nil
}

func (result BillableWeightResult) Method() BillableWeightMethod { return result.method }
func (result BillableWeightResult) ActualWeight() Weight         { return result.actual }
func (result BillableWeightResult) RawWeight() Weight            { return result.raw }
func (result BillableWeightResult) RoundedWeight() Weight        { return result.rounded }
func (result BillableWeightResult) RoundingMode() RoundingMode   { return result.roundingMode }
func (result BillableWeightResult) RoundingIncrement() Weight    { return result.increment }
func (result BillableWeightResult) Explanation() string          { return result.explanation }

func (result BillableWeightResult) VolumetricWeight() (Weight, bool) {
	if result.volumetric == nil {
		return Weight{}, false
	}
	return *result.volumetric, true
}

func (result BillableWeightResult) valid() bool {
	if !result.method.valid() || !result.actual.valid() || !result.raw.valid() || !result.rounded.valid() || !result.roundingMode.valid() || !result.increment.valid() || result.increment.value.Sign() <= 0 || result.increment.unit != result.actual.unit || strings.TrimSpace(result.explanation) == "" {
		return false
	}
	if result.volumetric != nil && (!result.volumetric.valid() || result.volumetric.unit != result.actual.unit) {
		return false
	}
	return true
}

func optionalWeightCopy(value Weight, present bool) *Weight {
	if !present {
		return nil
	}
	copy := value
	return &copy
}
