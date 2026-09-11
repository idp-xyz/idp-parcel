package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 本文件对真实 PostgreSQL 16 证结算输入版本「付款核对」一格的登记册（票 sa-cc/09，迁移 0020）：
// 引用四维 + 采用时刻往返、同一版第二次写答`已采用`且不覆盖先到者、换指纹是另一行、无事务拒写。

var verificationAdoptedAt = time.Date(2026, 9, 11, 8, 30, 0, 0, time.UTC)

func newAdoptionStore(t *testing.T) (*adapter.DutyPaymentVerificationAdoptions, bentoapp.Transactor) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := adapter.NewDutyPaymentVerificationAdoptions(db)
	if err != nil {
		t.Fatalf("构造采用登记册：%v", err)
	}
	return store, db.Transactor()
}

func verificationAdoptionRecord(t *testing.T, tenant, version string, adoptedAt time.Time) ports.DutyPaymentVerificationAdoptionRecord {
	t.Helper()
	reference, err := domain.NewDutyPaymentVerificationReference(
		saValue(t, domain.NewDeclarationScopeReference, "SYN-UNIT-01"),
		saValue(t, domain.NewTaxObligationReference, "duty-1"),
		saValue(t, domain.NewFundsFactReference, "bank-fact-1"),
		saValue(t, domain.NewDutyVerificationVersion, version),
	)
	if err != nil {
		t.Fatalf("核对引用：%v", err)
	}
	adoption, err := domain.AdoptDutyPaymentVerification(reference, adoptedAt)
	if err != nil {
		t.Fatalf("采用：%v", err)
	}
	return ports.DutyPaymentVerificationAdoptionRecord{
		Key:      ports.DutyPaymentVerificationAdoptionKey{TenantID: saTenant(t, tenant), Verification: reference},
		Adoption: adoption,
	}
}

func TestAVerificationAdoptionRoundTripsAndTheSecondWriteKeepsTheWinner(t *testing.T) {
	store, transactor := newAdoptionStore(t)
	ctx := t.Context()
	record := verificationAdoptionRecord(t, "tenant-a", "digest-1", verificationAdoptedAt)

	var saved ports.DutyPaymentVerificationAdoptionSaveOutcome
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		saved, err = store.Save(txCtx, record)
		return err
	})
	if saved != ports.DutyPaymentVerificationAdoptionSaved {
		t.Fatalf("save outcome = %d, want Saved", saved)
	}

	found, exists, err := store.FindByKey(ctx, record.Key)
	if err != nil || !exists {
		t.Fatalf("读回失败：err=%v exists=%v", err, exists)
	}
	if found.Adoption.Verification() != record.Adoption.Verification() {
		t.Fatalf("引用往返变形：%+v", found.Adoption.Verification())
	}
	if !found.Adoption.AdoptedAt().Equal(verificationAdoptedAt) {
		t.Fatalf("采用时刻往返 = %v, want %v", found.Adoption.AdoptedAt(), verificationAdoptedAt)
	}

	// 同一版第二次到达、带另一个采用时刻：答`已采用`，读回仍是先到者的时刻——键就是全部内容，没有可覆盖的。
	later := verificationAdoptionRecord(t, "tenant-a", "digest-1", verificationAdoptedAt.Add(time.Hour))
	var second ports.DutyPaymentVerificationAdoptionSaveOutcome
	var winner ports.DutyPaymentVerificationAdoptionRecord
	saWithin(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		if second, err = store.Save(txCtx, later); err != nil {
			return err
		}
		winner, _, err = store.FindByKey(txCtx, record.Key)
		return err
	})
	if second != ports.DutyPaymentVerificationAlreadyAdopted {
		t.Fatalf("第二次写入结果 = %d, want AlreadyAdopted", second)
	}
	if !winner.Adoption.AdoptedAt().Equal(verificationAdoptedAt) {
		t.Fatalf("先到者被覆盖：adopted_at = %v", winner.Adoption.AdoptedAt())
	}
}

// Covers: 同一范围的下一版核对（换指纹）各成一行；另一租户同引用互不可见。
func TestEachVerificationVersionAndTenantIsItsOwnAdoption(t *testing.T) {
	store, transactor := newAdoptionStore(t)
	ctx := t.Context()
	first := verificationAdoptionRecord(t, "tenant-a", "digest-1", verificationAdoptedAt)
	next := verificationAdoptionRecord(t, "tenant-a", "digest-2", verificationAdoptedAt.Add(time.Minute))
	otherTenant := verificationAdoptionRecord(t, "tenant-b", "digest-1", verificationAdoptedAt)

	for _, record := range []ports.DutyPaymentVerificationAdoptionRecord{first, next} {
		var saved ports.DutyPaymentVerificationAdoptionSaveOutcome
		saWithin(t, transactor, ctx, func(txCtx context.Context) error {
			var err error
			saved, err = store.Save(txCtx, record)
			return err
		})
		if saved != ports.DutyPaymentVerificationAdoptionSaved {
			t.Fatalf("%s：save outcome = %d, want Saved", record.Adoption.Verification().Version(), saved)
		}
	}

	if _, exists, err := store.FindByKey(ctx, next.Key); err != nil || !exists {
		t.Fatalf("次版读不回：err=%v exists=%v", err, exists)
	}
	if _, exists, err := store.FindByKey(ctx, otherTenant.Key); err != nil || exists {
		t.Fatalf("另一租户读到了本租户的采用：err=%v exists=%v", err, exists)
	}
}

func TestAVerificationAdoptionRefusesToWriteOutsideATransaction(t *testing.T) {
	store, _ := newAdoptionStore(t)
	record := verificationAdoptionRecord(t, "tenant-a", "digest-1", verificationAdoptedAt)
	if _, err := store.Save(t.Context(), record); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务 Save 采用：%v——写入一律经框架事务", err)
	}
}

func TestTheAdoptionStoreRefusesANilDB(t *testing.T) {
	if _, err := adapter.NewDutyPaymentVerificationAdoptions(nil); err == nil {
		t.Fatal("nil db 被收下了")
	}
}
