package domain

import "time"

// RehydrateExternalFundsFactSpec 是资金事实行在库里的样子。Adopt 造不出带更正回指的
// 版本——更正是转换门，读回不重放 CorrectAmount。
type RehydrateExternalFundsFactSpec struct {
	Fact        FundsFactReference
	Source      FundsSourceRegistrationReference
	Payer       FundsPayerReference
	Kind        FundsFactKind
	Currency    CurrencyCode
	AmountMinor int64
	Version     FundsFactVersion
	OccurredAt  time.Time
	Corrects    FundsFactVersion
	CorrectedAt time.Time
}

func RehydrateExternalFundsFact(spec RehydrateExternalFundsFactSpec) (ExternalFundsFact, error) {
	fact, err := AdoptExternalFundsFact(ExternalFundsFactSpec{
		Fact:        spec.Fact,
		Source:      spec.Source,
		Payer:       spec.Payer,
		Kind:        spec.Kind,
		Currency:    spec.Currency,
		AmountMinor: spec.AmountMinor,
		Version:     spec.Version,
		OccurredAt:  spec.OccurredAt,
	})
	if err != nil {
		return ExternalFundsFact{}, err
	}
	corrected := spec.Corrects.valid()
	if corrected != !spec.CorrectedAt.IsZero() {
		return ExternalFundsFact{}, ErrInvalidFundsFact
	}
	if !corrected {
		return fact, nil
	}
	if spec.Corrects == spec.Version || spec.CorrectedAt.Before(spec.OccurredAt) {
		return ExternalFundsFact{}, ErrInvalidFundsFact
	}
	fact.corrects = spec.Corrects
	fact.correctedAt = spec.CorrectedAt.UTC()
	return fact, nil
}

// RehydrateFundsMappingSpec 是映射行在库里的样子。MapFundsToTarget 要一份资金事实
// 才能拒付款失败，而行里只有映射本身——失败事实写不进来是写入时已经判过的。
type RehydrateFundsMappingSpec struct {
	Mapping    MappingReference
	Fact       FundsFactReference
	TargetKind SettlementTargetKind
	Target     SettlementTargetReference
	Basis      MappingBasisReference
	MappedAt   time.Time
}

func RehydrateFundsMapping(spec RehydrateFundsMappingSpec) (FundsMapping, error) {
	if !spec.Mapping.valid() ||
		!spec.Fact.valid() ||
		!spec.TargetKind.valid() ||
		!spec.Target.valid() ||
		!spec.Basis.valid() ||
		spec.MappedAt.IsZero() {
		return FundsMapping{}, ErrInvalidFundsMapping
	}
	return FundsMapping{
		mapping:    spec.Mapping,
		fact:       spec.Fact,
		targetKind: spec.TargetKind,
		target:     spec.Target,
		basis:      spec.Basis,
		mappedAt:   spec.MappedAt.UTC(),
	}, nil
}

// RehydrateSettlementApplicationSpec 是核销行在库里的样子。ApplySettlement 要映射
// 背书才能限分配，而行上只有分配本身——映射一致性是写入时已经判过的。
type RehydrateSettlementApplicationSpec struct {
	Application   ApplicationReference
	Fact          FundsFactReference
	Currency      CurrencyCode
	FactMinor     int64
	Allocations   []SettlementAllocation
	AppliedMinor  int64
	Basis         ApplicationBasisReference
	AppliedAt     time.Time
	ReversalBasis ApplicationBasisReference
	ReversedAt    time.Time
}

func RehydrateSettlementApplication(spec RehydrateSettlementApplicationSpec) (SettlementApplication, error) {
	if !spec.Application.valid() ||
		!spec.Fact.valid() ||
		!spec.Currency.valid() ||
		spec.FactMinor <= 0 ||
		!spec.Basis.valid() ||
		spec.AppliedAt.IsZero() ||
		len(spec.Allocations) == 0 {
		return SettlementApplication{}, ErrInvalidApplication
	}
	net := int64(0)
	for _, allocation := range spec.Allocations {
		if !allocation.Mapping.valid() || !allocation.TargetKind.valid() ||
			!allocation.Target.valid() || !allocation.Direction.valid() || allocation.AmountMinor <= 0 {
			return SettlementApplication{}, ErrInvalidApplication
		}
		if allocation.Direction == AllocationDebit {
			net += allocation.AmountMinor
		} else {
			net -= allocation.AmountMinor
		}
	}
	if net <= 0 || net > spec.FactMinor || net != spec.AppliedMinor {
		return SettlementApplication{}, ErrApplicationImbalance
	}
	reversed := !spec.ReversedAt.IsZero()
	if reversed != spec.ReversalBasis.valid() {
		return SettlementApplication{}, ErrInvalidApplication
	}
	if reversed && spec.ReversedAt.Before(spec.AppliedAt) {
		return SettlementApplication{}, ErrInvalidApplication
	}
	application := SettlementApplication{
		application:  spec.Application,
		fact:         spec.Fact,
		currency:     spec.Currency,
		factMinor:    spec.FactMinor,
		allocations:  append([]SettlementAllocation(nil), spec.Allocations...),
		appliedMinor: spec.AppliedMinor,
		basis:        spec.Basis,
		appliedAt:    spec.AppliedAt.UTC(),
	}
	if reversed {
		application.reversalBasis = spec.ReversalBasis
		application.reversedAt = spec.ReversedAt.UTC()
	}
	return application, nil
}
