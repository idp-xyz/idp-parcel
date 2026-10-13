package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// Covers: ADR-0179 决定二、五——首版形态只有「逐件 SELL 试算合计」一种，词表外与缺席都在登记时拒，不默认成那唯一一种；一行采用
// 是（账户、形态、计价范围）的不可变元组，任一格不同即不是同一份登记。
func TestAPreAcceptanceEstimateAdoptionTakesOnlyTheBuiltInForm(t *testing.T) {
	form, err := domain.PreAcceptanceEstimateFormFromName("PER_PARCEL_SELL_TRIAL_SUM")
	if err != nil || form != domain.PerParcelSellTrialSum || form.String() != "PER_PARCEL_SELL_TRIAL_SUM" {
		t.Fatalf("form = %v err = %v, 想要内置那一种", form, err)
	}
	for _, name := range []string{"", "PER_PARCEL_MARKUP", "per_parcel_sell_trial_sum"} {
		if _, err := domain.PreAcceptanceEstimateFormFromName(name); !errors.Is(err, domain.ErrBlankValue) {
			t.Fatalf("form %q err = %v, 想要登记时拒", name, err)
		}
	}

	account := mustRef(t, domain.NewSettlementAccountID, "ACCT-1")
	scope := mustRef(t, domain.NewEstimatePricingScope, "SCOPE-SELL-1")
	adoption, err := domain.NewPreAcceptanceEstimateAdoption(account, domain.PerParcelSellTrialSum, scope)
	if err != nil {
		t.Fatalf("adoption: %v", err)
	}
	if adoption.Account() != account || adoption.Form() != domain.PerParcelSellTrialSum || adoption.PricingScope() != scope {
		t.Fatalf("adoption = %+v, 想要原样保存三格", adoption)
	}
	if _, err := domain.NewPreAcceptanceEstimateAdoption(account, domain.PreAcceptanceEstimateFormInvalid, scope); !errors.Is(err, domain.ErrBlankValue) {
		t.Fatalf("没给形态 err = %v, 想要拒", err)
	}
	if _, err := domain.NewPreAcceptanceEstimateAdoption(account, domain.PerParcelSellTrialSum, domain.EstimatePricingScope{}); !errors.Is(err, domain.ErrBlankValue) {
		t.Fatalf("没给计价范围 err = %v, 想要拒", err)
	}
	if _, err := domain.NewEstimatePricingScope(" "); err == nil {
		t.Fatal("空白计价范围被收下了")
	}

	same, _ := domain.NewPreAcceptanceEstimateAdoption(account, domain.PerParcelSellTrialSum, scope)
	otherScope, _ := domain.NewPreAcceptanceEstimateAdoption(account, domain.PerParcelSellTrialSum, mustRef(t, domain.NewEstimatePricingScope, "SCOPE-SELL-2"))
	otherAccount, _ := domain.NewPreAcceptanceEstimateAdoption(mustRef(t, domain.NewSettlementAccountID, "ACCT-2"), domain.PerParcelSellTrialSum, scope)
	if !adoption.SameRegistration(same) || adoption.SameRegistration(otherScope) || adoption.SameRegistration(otherAccount) {
		t.Fatal("同一份登记的判定不对：同内容要相同，换计价范围或换账户都要不同")
	}
}
