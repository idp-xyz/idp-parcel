package registrationjson

import (
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

type periodicFeeDocument struct {
	TenantID          string            `json:"tenantId"`
	RuleRef           string            `json:"ruleRef"`
	Form              string            `json:"form"`
	MinimumMinor      *int64            `json:"minimumMinor,omitempty"`
	CommittedQuantity *int64            `json:"committedQuantity,omitempty"`
	RateMinor         *int64            `json:"rateMinor,omitempty"`
	Tiers             []periodicFeeTier `json:"tiers,omitempty"`
}

type periodicFeeTier struct {
	UpperMinor      *int64 `json:"upperMinor,omitempty"`
	RateBasisPoints int64  `json:"rateBasisPoints"`
}

// PeriodicFeeFromJSON 译装一种周期费用形态。不拿实绩去算补差或返利。
func PeriodicFeeFromJSON(raw []byte) (domain.TenantID, domain.PeriodicFeeRegistration, error) {
	var document periodicFeeDocument
	if err := decodeStrict(raw, &document); err != nil {
		return domain.TenantID{}, domain.PeriodicFeeRegistration{}, fmt.Errorf("周期费用登记输入不是本入口的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return domain.TenantID{}, domain.PeriodicFeeRegistration{}, fmt.Errorf("tenantId：%w", err)
	}
	rule, err := domain.NewPeriodicFeeRuleReference(document.RuleRef)
	if err != nil {
		return domain.TenantID{}, domain.PeriodicFeeRegistration{}, fmt.Errorf("ruleRef：%w", err)
	}
	form, err := domain.PeriodicFeeFormFromName(document.Form)
	if err != nil {
		return domain.TenantID{}, domain.PeriodicFeeRegistration{}, fmt.Errorf("form：%w", err)
	}
	terms, err := periodicTerms(form, document)
	if err != nil {
		return domain.TenantID{}, domain.PeriodicFeeRegistration{}, err
	}
	registration, err := domain.NewPeriodicFeeRegistration(rule, terms)
	if err != nil {
		return domain.TenantID{}, domain.PeriodicFeeRegistration{}, err
	}
	return tenant, registration, nil
}

func periodicTerms(form domain.PeriodicFeeForm, document periodicFeeDocument) (domain.PeriodicFeeTerms, error) {
	switch form {
	case domain.MinimumSpend:
		if document.MinimumMinor == nil || document.CommittedQuantity != nil || document.RateMinor != nil || len(document.Tiers) != 0 {
			return domain.PeriodicFeeTerms{}, fmt.Errorf("%w: minimum spend shape", domain.ErrInvalidPeriodicFee)
		}
		return domain.NewMinimumSpendTerms(*document.MinimumMinor)
	case domain.VolumeFloor:
		if document.CommittedQuantity == nil || document.RateMinor == nil || document.MinimumMinor != nil || len(document.Tiers) != 0 {
			return domain.PeriodicFeeTerms{}, fmt.Errorf("%w: volume floor shape", domain.ErrInvalidPeriodicFee)
		}
		return domain.NewVolumeFloorTerms(*document.CommittedQuantity, *document.RateMinor)
	case domain.TieredRebate:
		if document.MinimumMinor != nil || document.CommittedQuantity != nil || document.RateMinor != nil {
			return domain.PeriodicFeeTerms{}, fmt.Errorf("%w: rebate shape", domain.ErrInvalidPeriodicFee)
		}
		tiers := make([]domain.RebateTier, 0, len(document.Tiers))
		for _, tier := range document.Tiers {
			var parsed domain.RebateTier
			var err error
			if tier.UpperMinor == nil {
				parsed, err = domain.NewOpenRebateTier(tier.RateBasisPoints)
			} else {
				parsed, err = domain.NewRebateTier(*tier.UpperMinor, tier.RateBasisPoints)
			}
			if err != nil {
				return domain.PeriodicFeeTerms{}, err
			}
			tiers = append(tiers, parsed)
		}
		return domain.NewTieredRebateTerms(tiers)
	case domain.PeriodicFeeNotApplicable:
		if document.MinimumMinor != nil || document.CommittedQuantity != nil || document.RateMinor != nil || len(document.Tiers) != 0 {
			return domain.PeriodicFeeTerms{}, fmt.Errorf("%w: not applicable shape", domain.ErrInvalidPeriodicFee)
		}
		return domain.NewPeriodicFeeNotApplicable(), nil
	default:
		return domain.PeriodicFeeTerms{}, fmt.Errorf("%w: form", domain.ErrInvalidPeriodicFee)
	}
}
