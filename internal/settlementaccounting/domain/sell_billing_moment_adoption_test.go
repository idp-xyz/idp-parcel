package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// Covers: ADR-0190 决定三——一户采用一种计费时点与一个计价范围，是（账户、时点、计价范围）的不可变元组；
// 缺格与词表外的时点在登记时拒，不默认成任何一种；任一格不同即不是同一份登记。
func TestASellBillingMomentAdoptionIsAnImmutableTupleOfBuiltInMoment(t *testing.T) {
	account := mustRef(t, domain.NewSettlementAccountID, "SYN-ACCT-1")
	scope := mustRef(t, domain.NewEstimatePricingScope, "SYN-SCOPE-SELL-1")

	adoption, err := domain.NewSellBillingMomentAdoption(account, domain.BillingMomentNetworkIntakeAdopted, scope)
	if err != nil {
		t.Fatalf("adoption: %v", err)
	}
	if adoption.Account() != account || adoption.Moment() != domain.BillingMomentNetworkIntakeAdopted || adoption.PricingScope() != scope {
		t.Fatalf("adoption = %+v，想要原样保存三格", adoption)
	}

	for name, build := range map[string]func() (domain.SellBillingMomentAdoption, error){
		"缺账户": func() (domain.SellBillingMomentAdoption, error) {
			return domain.NewSellBillingMomentAdoption(domain.SettlementAccountID{}, domain.BillingMomentNetworkIntakeAdopted, scope)
		},
		"缺计费时点": func() (domain.SellBillingMomentAdoption, error) {
			return domain.NewSellBillingMomentAdoption(account, domain.ParcelBillingMomentInvalid, scope)
		},
		"词表外的计费时点": func() (domain.SellBillingMomentAdoption, error) {
			return domain.NewSellBillingMomentAdoption(account, domain.ParcelBillingMoment(99), scope)
		},
		"缺计价范围": func() (domain.SellBillingMomentAdoption, error) {
			return domain.NewSellBillingMomentAdoption(account, domain.BillingMomentNetworkIntakeAdopted, domain.EstimatePricingScope{})
		},
	} {
		if _, err := build(); !errors.Is(err, domain.ErrBlankValue) {
			t.Fatalf("%s err = %v，想要登记时拒", name, err)
		}
	}

	same, _ := domain.NewSellBillingMomentAdoption(account, domain.BillingMomentNetworkIntakeAdopted, scope)
	otherMoment, _ := domain.NewSellBillingMomentAdoption(account, domain.BillingMomentEffectiveDeliveryFinalized, scope)
	otherScope, _ := domain.NewSellBillingMomentAdoption(account, domain.BillingMomentNetworkIntakeAdopted,
		mustRef(t, domain.NewEstimatePricingScope, "SYN-SCOPE-SELL-2"))
	otherAccount, _ := domain.NewSellBillingMomentAdoption(mustRef(t, domain.NewSettlementAccountID, "SYN-ACCT-2"),
		domain.BillingMomentNetworkIntakeAdopted, scope)
	if !adoption.SameRegistration(same) {
		t.Fatal("同内容的两份不算同一份登记")
	}
	if adoption.SameRegistration(otherMoment) || adoption.SameRegistration(otherScope) || adoption.SameRegistration(otherAccount) {
		t.Fatal("换时点、换计价范围或换账户仍算同一份登记")
	}
}
