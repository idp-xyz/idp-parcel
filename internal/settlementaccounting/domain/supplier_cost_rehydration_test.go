package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

func rehydrationSpec(t *testing.T) domain.RehydrateSupplierExpectedCostSpec {
	t.Helper()
	return domain.RehydrateSupplierExpectedCostSpec{
		Version:            mustValue(t, domain.NewSupplierCostVersionID, "cost-1/v2"),
		Occurrence:         occurrence(t),
		FeeItem:            mustValue(t, domain.NewFeeItemReference, "LINEHAUL_BASE"),
		RuleVersion:        mustValue(t, domain.NewPurchaseRuleVersionReference, "purchase-rules/v2"),
		Agreement:          mustValue(t, domain.NewSupplierAgreementReference, "agreement-1/v3"),
		Evaluation:         mustValue(t, domain.NewBuyEvaluationReference, "evaluation-buy-2"),
		OriginalCurrency:   mustValue(t, domain.NewCurrencyCode, "USD"),
		OriginalMinor:      3900,
		SettlementCurrency: mustValue(t, domain.NewCurrencyCode, "USD"),
		SettlementMinor:    3900,
		PriorVersion:       mustValue(t, domain.NewSupplierCostVersionID, "cost-1/v1"),
		CorrectionReason:   mustValue(t, domain.NewCostCorrectionReason, "RULE_CORRECTED"),
	}
}

// Covers: ADR-0067 决定四在重建门上的一半——同币种两额必须相等对所有版本成立，
// 纠错行不再例外；等额的纠错行照常重建并带回回指与原因。
func TestRehydrationHoldsSameCurrencyEqualityForCorrections(t *testing.T) {
	unequal := rehydrationSpec(t)
	unequal.OriginalMinor = 4200
	if _, err := domain.RehydrateSupplierExpectedCost(unequal); !errors.Is(err, domain.ErrInvalidSupplierCost) {
		t.Fatalf("err = %v; 同币种两额不等的纠错行被重建门收下了", err)
	}

	cost, err := domain.RehydrateSupplierExpectedCost(rehydrationSpec(t))
	if err != nil {
		t.Fatalf("重建等额纠错行：%v", err)
	}
	prior, present := cost.PriorVersion()
	if !present || prior.String() != "cost-1/v1" {
		t.Fatalf("prior = %s present = %v", prior, present)
	}
}
