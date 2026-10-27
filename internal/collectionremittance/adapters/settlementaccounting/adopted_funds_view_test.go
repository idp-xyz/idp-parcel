package settlementaccounting_test

import (
	"context"
	"errors"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/collectionremittance/adapters/settlementaccounting"
	crdomain "go.idp.xyz/idp-parcel/internal/collectionremittance/domain"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

type viewStub struct {
	loaded saports.CollectedPrincipalFundsFact
	found  bool
	err    error
	asked  []string
}

func (stub *viewStub) LockCollectedPrincipalFact(
	_ context.Context, tenant sadomain.TenantID, fact sadomain.FundsFactReference,
) (saports.CollectedPrincipalFundsFact, bool, error) {
	stub.asked = append(stub.asked, tenant.String()+"/"+fact.String())
	return stub.loaded, stub.found, stub.err
}

func adoptedHead(t *testing.T, currency string, amountMinor int64, version string) sadomain.ExternalFundsFact {
	t.Helper()
	fact, err := sadomain.AdoptExternalFundsFact(sadomain.ExternalFundsFactSpec{
		Fact:        must(t, sadomain.NewFundsFactReference, "BANK-RECEIPT-1"),
		Source:      must(t, sadomain.NewFundsSourceRegistrationReference, "SYN-SOURCE-1"),
		Kind:        sadomain.FundsReceiptConfirmed,
		Currency:    must(t, sadomain.NewCurrencyCode, currency),
		AmountMinor: amountMinor,
		Version:     must(t, sadomain.NewFundsFactVersion, version),
		OccurredAt:  time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("采用：%v", err)
	}
	return fact
}

func must[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("构造 %q：%v", raw, err)
	}
	return value
}

// Covers: ADR-0330 的消费侧翻译——结算答「已采用且已认作代收本金」才译成可用的事实（版本与带币种的金额）；没有这条事实、
// 或有但没被认作，译成同一个 found=false（恢复动作相同：先去结算采用并认作）；读不动是 error，不折成 found=false。
func TestTheAdapterTranslatesOnlyASetAsideFactAndNeverFoldsAFailureIntoAbsence(t *testing.T) {
	tenant := must(t, crdomain.NewTenantID, "SYN-T1")
	reference := must(t, crdomain.NewAdoptedFundsReference, "BANK-RECEIPT-1")

	stub := &viewStub{found: true, loaded: saports.CollectedPrincipalFundsFact{
		Head: adoptedHead(t, "CNY", 100_000, "BANK-RECEIPT-1/v2"), SetAside: true,
	}}
	view, err := adapter.NewAdoptedFundsView(stub)
	if err != nil {
		t.Fatal(err)
	}
	funds, found, err := view.LockAdoptedFunds(t.Context(), tenant, reference)
	if err != nil || !found {
		t.Fatalf("已认作：found=%v err=%v", found, err)
	}
	if funds.Version().String() != "BANK-RECEIPT-1/v2" ||
		funds.Amount().Currency().String() != "CNY" || funds.Amount().AmountMinor() != 100_000 {
		t.Fatalf("译出 = %s %s %d", funds.Version(), funds.Amount().Currency(), funds.Amount().AmountMinor())
	}
	if len(stub.asked) != 1 || stub.asked[0] != "SYN-T1/BANK-RECEIPT-1" {
		t.Fatalf("问提供方的键 = %v", stub.asked)
	}

	stub.loaded.SetAside = false
	if _, found, err := view.LockAdoptedFunds(t.Context(), tenant, reference); err != nil || found {
		t.Fatalf("采用了但没认作代收本金：found=%v err=%v，要 found=false", found, err)
	}
	stub.found = false
	if _, found, err := view.LockAdoptedFunds(t.Context(), tenant, reference); err != nil || found {
		t.Fatalf("没采用：found=%v err=%v，要 found=false", found, err)
	}
	stub.err = errors.New("settlement read model unavailable")
	if _, found, err := view.LockAdoptedFunds(t.Context(), tenant, reference); err == nil || found {
		t.Fatalf("读不动：found=%v err=%v，要 error", found, err)
	}
}

func TestTheAdapterRefusesANilView(t *testing.T) {
	if _, err := adapter.NewAdoptedFundsView(nil); err == nil {
		t.Fatal("nil 读口被收下了")
	}
}
