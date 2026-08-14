package domain

import "time"

// RehydrateCostAllocationSpec 是分摊行在库里的样子。FormCostAllocation 造不出带
// 重分摊回指的版本——重分摊是转换门，读回不重放 Reallocate。
type RehydrateCostAllocationSpec struct {
	ID          AllocationID
	Source      AllocationSourceReference
	SourceMinor int64
	Currency    CurrencyCode
	Rule        AllocationRuleVersionReference
	Portions    []AllocationPortion
	Version     AllocationVersion
	AllocatedAt time.Time
	Corrects    AllocationVersion
}

func RehydrateCostAllocation(spec RehydrateCostAllocationSpec) (CostAllocation, error) {
	allocation, err := FormCostAllocation(CostAllocationSpec{
		ID:          spec.ID,
		Source:      spec.Source,
		SourceMinor: spec.SourceMinor,
		Currency:    spec.Currency,
		Rule:        spec.Rule,
		Portions:    spec.Portions,
		Version:     spec.Version,
		AllocatedAt: spec.AllocatedAt,
	})
	if err != nil {
		return CostAllocation{}, err
	}
	if spec.Corrects.valid() {
		if spec.Corrects == spec.Version {
			return CostAllocation{}, ErrInvalidCostAllocation
		}
		allocation.corrects = spec.Corrects
	}
	return allocation, nil
}

// RehydrateOperatingResultSpec 是经营结果行在库里的样子。Derive 造不出带重派生回指
// 的版本——重派生是转换门，读回不重放 Rederive。
type RehydrateOperatingResultSpec struct {
	Scope      OperatingScopeReference
	Period     BillingPeriodReference
	Basis      OperatingBasis
	Currency   CurrencyCode
	Components []ResultComponent
	Version    OperatingResultVersion
	AsOf       time.Time
	Corrects   OperatingResultVersion
}

func RehydrateOperatingResult(spec RehydrateOperatingResultSpec) (OperatingResult, error) {
	result, err := DeriveOperatingResult(
		spec.Scope, spec.Period, spec.Basis, spec.Currency,
		spec.Components, spec.Version, spec.AsOf,
	)
	if err != nil {
		return OperatingResult{}, err
	}
	if spec.Corrects.valid() {
		if spec.Corrects == spec.Version {
			return OperatingResult{}, ErrInvalidOperatingResult
		}
		result.corrects = spec.Corrects
	}
	return result, nil
}
