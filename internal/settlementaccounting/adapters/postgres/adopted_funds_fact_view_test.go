package postgres_test

import (
	"context"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

func newAdoptedFundsFactViewFixture(t *testing.T) (*adapter.ExternalFundsFacts, *adapter.AdoptedFundsFactView, *bentopg.DB) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	facts, err := adapter.NewExternalFundsFacts(db)
	if err != nil {
		t.Fatalf("构造资金事实库：%v", err)
	}
	view, err := adapter.NewAdoptedFundsFactView(db)
	if err != nil {
		t.Fatalf("构造只读视图：%v", err)
	}
	return facts, view, db
}

func saveFundsFact(t *testing.T, db *bentopg.DB, facts *adapter.ExternalFundsFacts, record ports.FundsFactRecord) {
	t.Helper()
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		saved, err := facts.Save(txCtx, record)
		if err != nil {
			return err
		}
		if saved != ports.FundsFactSaved {
			t.Fatalf("saved = %d, want FundsFactSaved", saved)
		}
		return nil
	}); err != nil {
		t.Fatalf("写资金事实：%v", err)
	}
}

// Covers: sa-cc/03 裁决——SA 按（租户 + 资金事实引用 + 采用版本）的只读口：取信封所指的那一版；
// 库里存的是另一版或还没有这一条都答 found=false，不拿 latest 顶替（票 lc/24 的教训）。
// 顺带钉 0018：来源提供的付款人随采用落库、读回同值；来源未提供读回显式缺席，不是空串。
func TestTheAdoptedFundsFactViewAnswersOnlyTheVersionTheEnvelopePointsAt(t *testing.T) {
	facts, view, db := newAdoptedFundsFactViewFixture(t)
	ctx := t.Context()
	tenant := saTenant(t, "tenant-a")

	withPayer := adoptedFactRecord(t, "tenant-a", "bank-fact-1", domain.FundsReceiptConfirmed, 8000)
	payerFact, err := domain.AdoptExternalFundsFact(domain.ExternalFundsFactSpec{
		Fact:        withPayer.Fact.Fact(),
		Source:      withPayer.Fact.Source(),
		Payer:       saValue(t, domain.NewFundsPayerReference, "payer-customer-7"),
		Kind:        withPayer.Fact.Kind(),
		Currency:    saValue(t, domain.NewCurrencyCode, "USD"),
		AmountMinor: 8000,
		Version:     withPayer.Fact.Version(),
		OccurredAt:  withPayer.Fact.OccurredAt(),
	})
	if err != nil {
		t.Fatalf("构造带付款人的事实：%v", err)
	}
	withPayer.Fact = payerFact
	saveFundsFact(t, db, facts, withPayer)

	loaded, found, err := view.LoadAdoptedFundsFact(ctx, tenant, withPayer.Key.Fact, withPayer.Fact.Version())
	if err != nil || !found {
		t.Fatalf("按信封所指版本读回：found = %v err = %v", found, err)
	}
	if loaded != payerFact {
		t.Fatalf("读回 = %#v, want 与采用时同值（含付款人）", loaded)
	}
	if payer, provided := loaded.Payer(); !provided || payer.String() != "payer-customer-7" {
		t.Fatalf("Payer() = (%q, %v), want (payer-customer-7, true)", payer, provided)
	}

	if _, found, err := view.LoadAdoptedFundsFact(ctx, tenant, withPayer.Key.Fact,
		saValue(t, domain.NewFundsFactVersion, "bank-fact/v2")); err != nil || found {
		t.Fatalf("库里存的是 v1、问 v2：found = %v err = %v，want false——不拿当前版顶替信封所指的版本", found, err)
	}
	if _, found, err := view.LoadAdoptedFundsFact(ctx, tenant,
		saValue(t, domain.NewFundsFactReference, "bank-fact-never"), withPayer.Fact.Version()); err != nil || found {
		t.Fatalf("未采用的事实：found = %v err = %v，want false", found, err)
	}
	if _, found, err := view.LoadAdoptedFundsFact(ctx, saTenant(t, "tenant-b"),
		withPayer.Key.Fact, withPayer.Fact.Version()); err != nil || found {
		t.Fatalf("另一租户同引用：found = %v err = %v，want false", found, err)
	}

	withoutPayer := adoptedFactRecord(t, "tenant-a", "bank-fact-2", domain.FundsReceiptConfirmed, 5000)
	saveFundsFact(t, db, facts, withoutPayer)
	loaded, found, err = view.LoadAdoptedFundsFact(ctx, tenant, withoutPayer.Key.Fact, withoutPayer.Fact.Version())
	if err != nil || !found {
		t.Fatalf("读回未带付款人的事实：found = %v err = %v", found, err)
	}
	if payer, provided := loaded.Payer(); provided || payer.String() != "" {
		t.Fatalf("来源未提供付款人，读回却是 (%q, %v)", payer, provided)
	}
	if loaded != withoutPayer.Fact {
		t.Fatalf("读回 = %#v, want 与采用时同值", loaded)
	}
}

func TestTheAdoptedFundsFactViewRefusesANilDB(t *testing.T) {
	if _, err := adapter.NewAdoptedFundsFactView(nil); err == nil {
		t.Fatal("nil db 被收下了")
	}
}
