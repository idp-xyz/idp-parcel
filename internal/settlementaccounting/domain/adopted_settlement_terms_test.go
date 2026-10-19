package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// Covers: ADR-0190 决定三那条链的中段——商业解析采用的结算政策四维缺一不成，方向由提问方给，拼出的账户键四维原样带过去。
func TestAdoptedSettlementTermsBuildTheAccountKeyForTheAskedDirection(t *testing.T) {
	legalEntity := mustRef(t, domain.NewLegalEntityReference, "SYN-LE-1")
	counterparty := mustRef(t, domain.NewSettlementCounterpartyReference, "SYN-CP-1")
	currency := mustRef(t, domain.NewCurrencyCode, "XTS")
	policy := mustRef(t, domain.NewSettlementPolicyReference, "SYN-SETTLE-1")

	terms, err := domain.NewAdoptedSettlementTerms(legalEntity, counterparty, currency, policy)
	if err != nil {
		t.Fatalf("terms: %v", err)
	}
	key, err := terms.AccountKey(domain.ChargeReceivable)
	if err != nil {
		t.Fatalf("account key: %v", err)
	}
	if key.LegalEntity() != legalEntity || key.Counterparty() != counterparty || key.Currency() != currency ||
		key.Policy() != policy || key.Direction() != domain.ChargeReceivable {
		t.Fatalf("账户键 = %+v，想要四维原样、方向取提问方给的应收", key)
	}
	if _, err := terms.AccountKey(domain.ChargeDirection(0)); !errors.Is(err, domain.ErrBlankValue) {
		t.Fatalf("没给方向 err = %v，想要拒", err)
	}

	for name, build := range map[string]func() (domain.AdoptedSettlementTerms, error){
		"缺法人": func() (domain.AdoptedSettlementTerms, error) {
			return domain.NewAdoptedSettlementTerms(domain.LegalEntityReference{}, counterparty, currency, policy)
		},
		"缺相对方": func() (domain.AdoptedSettlementTerms, error) {
			return domain.NewAdoptedSettlementTerms(legalEntity, domain.SettlementCounterpartyReference{}, currency, policy)
		},
		"缺币种": func() (domain.AdoptedSettlementTerms, error) {
			return domain.NewAdoptedSettlementTerms(legalEntity, counterparty, domain.CurrencyCode{}, policy)
		},
		"缺政策": func() (domain.AdoptedSettlementTerms, error) {
			return domain.NewAdoptedSettlementTerms(legalEntity, counterparty, currency, domain.SettlementPolicyReference{})
		},
	} {
		if _, err := build(); !errors.Is(err, domain.ErrBlankValue) {
			t.Fatalf("%s err = %v，想要拒", name, err)
		}
	}
}
