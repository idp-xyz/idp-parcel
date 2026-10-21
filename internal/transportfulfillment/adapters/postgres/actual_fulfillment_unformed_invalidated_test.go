package postgres_test

import (
	"context"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// Covers: AT-TF-312 的未形成册库面（S，ADR-0232 决定五，迁移 TF 0049）——册上那一条了结为运营决定失效：记下失效版本、读回同一格，
// 查阅页不列；册上没有的那一件了结什么也不写；之后又答未形成，回到列上、失效版本那两格清空。
func TestTheUnformedRegisterSettlesAnInvalidatedOccurrenceAsInvalidatedByDecision(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	register, err := adapter.NewActualFulfillmentUnformeds(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	at := time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
	key := unformedKey(t, "tenant-1", "SEG-A", "parcel-1")
	absent := unformedKey(t, "tenant-1", "SEG-A", "parcel-absent")
	invalidated := ports.ChargeOccurrenceKey{
		TenantID:   key.TenantID,
		Occurrence: deliveryValue(t, domain.NewChargeOccurrenceReference, "TCO-0123456789abcdef0123456789abcdef"),
		Validity:   deliveryValue(t, domain.NewOccurrenceValidityVersion, "V2"),
	}
	mustWithinDeliveryTransaction(t, db.Transactor(), ctx, func(txCtx context.Context) error {
		if err := register.RecordUnformed(txCtx, unformedEntry(t, key, at.Add(-time.Hour), ports.UnformedNotConfigured), at); err != nil {
			return err
		}
		if err := register.SettleInvalidatedByDecision(txCtx, key, invalidated, at.Add(time.Hour)); err != nil {
			return err
		}
		return register.SettleInvalidatedByDecision(txCtx, absent, invalidated, at.Add(time.Hour))
	})
	row, found, err := register.FindByKey(ctx, key)
	if err != nil || !found || row.State != ports.ActualFulfillmentUnformedInvalidatedByDecision || row.Occurrence != invalidated ||
		!row.SettledAt.Equal(at.Add(time.Hour)) {
		t.Fatalf("了结为运营决定失效 = %+v %v %v", row, found, err)
	}
	if _, found, _ := register.FindByKey(ctx, absent); found {
		t.Fatal("册上没有的那一件被了结出了一行")
	}
	if listed, err := register.ListActualFulfillmentUnformed(ctx, key.TenantID, 50); err != nil || len(listed) != 0 {
		t.Fatalf("查阅页列了 %+v %v", listed, err)
	}

	mustWithinDeliveryTransaction(t, db.Transactor(), ctx, func(txCtx context.Context) error {
		return register.RecordUnformed(txCtx, unformedEntry(t, key, at.Add(-time.Hour), ports.UnformedNotConfigured), at.Add(2*time.Hour))
	})
	if row, _, _ := register.FindByKey(ctx, key); row.State != ports.ActualFulfillmentUnformed || row.Occurrence != (ports.ChargeOccurrenceKey{}) {
		t.Fatalf("再答未形成 = %+v", row)
	}
}
