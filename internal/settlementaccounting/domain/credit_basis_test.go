package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

func creditPolicyReference(t *testing.T, raw string) domain.CreditPolicyReference {
	t.Helper()
	reference, err := domain.NewCreditPolicyReference(raw)
	if err != nil {
		t.Fatalf("new credit policy reference: %v", err)
	}
	return reference
}

// Covers: ADR-0127 决定四——授信依据两格封闭（金额 / 比例），恰一在场；零额度是合法的商业声明，
// 负值与无出处立不住。
func TestCreditBasisIsEitherAnAmountOrARatioWithAPolicyBehindIt(t *testing.T) {
	policy := creditPolicyReference(t, "credit-1/v1")

	amount, err := domain.NewCreditAmountBasis(policy, 0)
	if err != nil {
		t.Fatalf("零额度是合法声明，却被拒：%v", err)
	}
	if minor, ok := amount.AmountMinor(); !ok || minor != 0 {
		t.Fatalf("amount = (%d, %v), want (0, true)", minor, ok)
	}
	if _, ok := amount.RatioBasisPoints(); ok {
		t.Fatal("金额额度答出了比例在场")
	}
	if amount.Policy() != policy {
		t.Fatal("依据丢了政策出处")
	}

	ratio, err := domain.NewCreditRatioBasis(policy, 2500)
	if err != nil {
		t.Fatalf("new ratio basis: %v", err)
	}
	if bps, ok := ratio.RatioBasisPoints(); !ok || bps != 2500 {
		t.Fatalf("ratio = (%d, %v), want (2500, true)", bps, ok)
	}
	if _, ok := ratio.AmountMinor(); ok {
		t.Fatal("比例额度答出了金额在场")
	}

	if _, err := domain.NewCreditAmountBasis(policy, -1); !errors.Is(err, domain.ErrInvalidCreditBasis) {
		t.Fatalf("负金额：err = %v, want ErrInvalidCreditBasis", err)
	}
	if _, err := domain.NewCreditRatioBasis(policy, -1); !errors.Is(err, domain.ErrInvalidCreditBasis) {
		t.Fatalf("负比例：err = %v, want ErrInvalidCreditBasis", err)
	}
	if _, err := domain.NewCreditAmountBasis(domain.CreditPolicyReference{}, 100); !errors.Is(err, domain.ErrInvalidCreditBasis) {
		t.Fatalf("无出处：err = %v, want ErrInvalidCreditBasis——没有政策出处的额度就是本上下文自己发明的额度", err)
	}
}

// Covers: ADR-0127 决定四——信用状况换上商业侧授权额度：只换额度，已占用与逾期原样留着；原值不改。
func TestWithAuthorizedLimitReplacesOnlyTheLimit(t *testing.T) {
	scope := settlementScope(t, "legal-1", "account-1", "CNY")
	registered, err := domain.NewCreditStanding(scope, 1_000, 300, true)
	if err != nil {
		t.Fatalf("new credit standing: %v", err)
	}

	authorized, err := registered.WithAuthorizedLimit(10_000)
	if err != nil {
		t.Fatalf("with authorized limit: %v", err)
	}
	if authorized.LimitMinor() != 10_000 || authorized.Headroom() != 9_700 || !authorized.Overdue() {
		t.Fatalf("authorized = limit %d headroom %d overdue %v, want 10000 / 9700 / true",
			authorized.LimitMinor(), authorized.Headroom(), authorized.Overdue())
	}
	if authorized.Scope() != scope {
		t.Fatal("换额度换掉了作用域")
	}
	if registered.LimitMinor() != 1_000 || registered.Headroom() != 700 {
		t.Fatal("原状况快照被就地改写")
	}
	if _, err := registered.WithAuthorizedLimit(-1); !errors.Is(err, domain.ErrInvalidCreditStanding) {
		t.Fatalf("负额度：err = %v, want ErrInvalidCreditStanding", err)
	}
	if _, err := (domain.CreditStanding{}).WithAuthorizedLimit(1); !errors.Is(err, domain.ErrInvalidCreditStanding) {
		t.Fatalf("零值状况：err = %v, want ErrInvalidCreditStanding", err)
	}
}
