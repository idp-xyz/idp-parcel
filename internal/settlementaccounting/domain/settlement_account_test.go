package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

func accountFixture(t *testing.T, direction domain.ChargeDirection, payer string) domain.SettlementAccount {
	t.Helper()
	legalEntity, err := domain.NewLegalEntityReference("LE-1")
	if err != nil {
		t.Fatal(err)
	}
	counterparty, err := domain.NewSettlementCounterpartyReference("CP-1")
	if err != nil {
		t.Fatal(err)
	}
	currency, err := domain.NewCurrencyCode("CNY")
	if err != nil {
		t.Fatal(err)
	}
	policy, err := domain.NewSettlementPolicyReference("POL-1")
	if err != nil {
		t.Fatal(err)
	}
	key, err := domain.NewSettlementAccountKey(legalEntity, counterparty, direction, currency, policy)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	var payerRef domain.SettlementCounterpartyReference
	distinct := payer != ""
	if distinct {
		payerRef, err = domain.NewSettlementCounterpartyReference(payer)
		if err != nil {
			t.Fatal(err)
		}
	}
	basis, err := domain.NewResponsibilityBasis("CONTRACT-1")
	if err != nil {
		t.Fatal(err)
	}
	account, err := domain.NewSettlementAccount(
		mustID(t, "ACCT-1"), key, payerRef, distinct, basis, mustStatement(t),
	)
	if err != nil {
		t.Fatalf("account: %v", err)
	}
	return account
}

func mustID(t *testing.T, value string) domain.SettlementAccountID {
	t.Helper()
	id, err := domain.NewSettlementAccountID(value)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func mustStatement(t *testing.T) domain.AccountStatementTerms {
	t.Helper()
	terms, err := domain.NewAccountStatementTerms("MONTHLY", "Asia/Shanghai", "18:00", "NET-30")
	if err != nil {
		t.Fatalf("statement: %v", err)
	}
	return terms
}

func mustRef[T any](t *testing.T, parse func(string) (T, error), value string) T {
	t.Helper()
	parsed, err := parse(value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func TestSettlementAccountRejectsAPayerThatRepeatsTheCounterparty(t *testing.T) {
	key, err := domain.NewSettlementAccountKey(
		mustRef(t, domain.NewLegalEntityReference, "LE-1"),
		mustRef(t, domain.NewSettlementCounterpartyReference, "CP-1"),
		domain.ChargeReceivable,
		mustRef(t, domain.NewCurrencyCode, "CNY"),
		mustRef(t, domain.NewSettlementPolicyReference, "POL-1"),
	)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	_, err = domain.NewSettlementAccount(
		mustID(t, "ACCT-1"),
		key,
		mustRef(t, domain.NewSettlementCounterpartyReference, "CP-1"),
		true,
		mustRef(t, domain.NewResponsibilityBasis, "CONTRACT-1"),
		mustStatement(t),
	)
	if err == nil {
		t.Fatal("付款责任方与结算相对方相同仍立了起来")
	}
}

func TestSettlementAccountReplayIgnoresNothingButTheWriteInstant(t *testing.T) {
	left := accountFixture(t, domain.ChargeReceivable, "PAYER-1")
	right := accountFixture(t, domain.ChargeReceivable, "PAYER-1")
	if !left.SameRegistration(right) {
		t.Fatal("同一份登记没有被认成重放")
	}
	other := accountFixture(t, domain.ChargePayable, "PAYER-1")
	if left.SameRegistration(other) {
		t.Fatal("收付方向不同仍被认成同一份登记")
	}
}

func TestChargeDirectionFromNameRejectsWordsOutsideThePair(t *testing.T) {
	if _, err := domain.ChargeDirectionFromName("DEBIT"); err == nil {
		t.Fatal("借贷词被收成了收付方向")
	}
}
