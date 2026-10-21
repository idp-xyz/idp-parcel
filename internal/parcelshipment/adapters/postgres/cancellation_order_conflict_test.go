package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证取消先后冲突册（ADR-0254 决定二）：经判法形成的冲突整行往返、同一份冲突重投答已有
// 记录、来源换一代另起一行、租户隔离、无事务拒；库内 CHECK 钉住冲突的定义——边界事实晚于取消的那一行进不来。

var conflictDetectedAt = time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)

func judgedConflict(t *testing.T, parcel, reference string, boundaryAt time.Time) domain.CancellationOrderConflict {
	t.Helper()
	cancellation, err := domain.DecideParcelCancellation(domain.ParcelCancellationSpec{
		ID:          mustBuild(t, domain.NewParcelCancellationID, "PCXL-1"),
		Parcel:      mustBuild(t, domain.NewDeclaredParcelID, parcel),
		Requester:   mustBuild(t, domain.NewCancellationRequesterReference, "customer-a"),
		Authority:   mustBuild(t, domain.NewCancellationAuthorityReference, "CANCEL-RULE/1"),
		Reason:      mustBuild(t, domain.NewCancellationReasonReference, "CHANGED_MIND"),
		RequestedAt: cancelRequestedAt,
	}, domain.CurrentIntakeFact{})
	if err != nil {
		t.Fatalf("decide cancellation: %v", err)
	}
	conflict, conflicted, err := domain.JudgeCancellationOrder(cancellation, domain.CancellationBoundaryFact{
		Kind:       domain.CarrierFirstEffectivePickupBoundary,
		Reference:  mustBuild(t, domain.NewCancellationBoundaryReference, reference),
		OccurredAt: boundaryAt,
	})
	if err != nil || !conflicted {
		t.Fatalf("judge order: conflicted = %v err = %v", conflicted, err)
	}
	return conflict
}

func TestACancellationOrderConflictIsRecordedOnceAndListedByParcel(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	register, err := adapter.NewCancellationOrderConflicts(db)
	if err != nil {
		t.Fatalf("构造冲突册：%v", err)
	}
	ctx := t.Context()
	record := func(tenant string, conflict domain.CancellationOrderConflict) ports.CancellationOrderConflictSaveOutcome {
		t.Helper()
		var outcome ports.CancellationOrderConflictSaveOutcome
		mustWithinTransaction(t, db.Transactor(), ctx, func(txCtx context.Context) error {
			var err error
			outcome, err = register.Record(txCtx, ports.CancellationOrderConflictRecord{
				TenantID: psTenant(t, tenant), Conflict: conflict, DetectedAt: conflictDetectedAt,
			})
			return err
		})
		return outcome
	}

	first := judgedConflict(t, "parcel-1", "CFEP-1@CFEV-1", cancelRequestedAt.Add(-time.Hour))
	if got := record("tenant-1", first); got != ports.CancellationOrderConflictRecorded {
		t.Fatalf("首记 = %d, want 已记下", got)
	}
	if got := record("tenant-1", first); got != ports.CancellationOrderConflictAlreadyRecorded {
		t.Fatalf("重投 = %d, want 已有记录——同一份冲突不是第二份", got)
	}
	corrected := judgedConflict(t, "parcel-1", "CFEP-1@CFEV-2", cancelRequestedAt)
	if got := record("tenant-1", corrected); got != ports.CancellationOrderConflictRecorded {
		t.Fatalf("来源换一代 = %d, want 另起一行", got)
	}

	entries, err := register.ListByParcel(ctx, psTenant(t, "tenant-1"), mustBuild(t, domain.NewDeclaredParcelID, "parcel-1"))
	if err != nil {
		t.Fatalf("按包裹列：%v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("行数 = %d, want 2", len(entries))
	}
	entry := entries[0]
	if entry.Cancellation.String() != "PCXL-1" || !entry.CancelledAt.Equal(cancelRequestedAt) ||
		entry.Boundary.Kind != domain.CarrierFirstEffectivePickupBoundary ||
		entry.Boundary.Reference.String() != "CFEP-1@CFEV-1" ||
		!entry.Boundary.OccurredAt.Equal(cancelRequestedAt.Add(-time.Hour)) ||
		!entry.DetectedAt.Equal(conflictDetectedAt) {
		t.Fatalf("往返变形：%+v", entry)
	}
	if foreign, err := register.ListByParcel(ctx, psTenant(t, "tenant-2"), mustBuild(t, domain.NewDeclaredParcelID, "parcel-1")); err != nil || len(foreign) != 0 {
		t.Fatalf("另一租户读到了 %d 行（err=%v）", len(foreign), err)
	}

	if _, err := register.Record(ctx, ports.CancellationOrderConflictRecord{
		TenantID: psTenant(t, "tenant-1"), Conflict: first, DetectedAt: conflictDetectedAt,
	}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务记冲突应返回 ErrTransactionRequired，实得：%v", err)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO parcel_shipment.cancellation_order_conflict
			(tenant_id, parcel_id, boundary_kind, boundary_ref, cancellation_id,
			 cancelled_at, boundary_at, detected_at)
		 VALUES ('tenant-1', 'parcel-x', 'NETWORK_INTAKE', 'NODE_INTAKE/v1', 'PCXL-9',
		         '2026-10-04T08:00:00Z', '2026-10-04T08:00:01Z', now())`); err == nil {
		t.Fatal("一行「边界事实晚于取消」进了冲突册——那是取消在先，不是冲突")
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO parcel_shipment.cancellation_order_conflict
			(tenant_id, parcel_id, boundary_kind, boundary_ref, cancellation_id,
			 cancelled_at, boundary_at, detected_at)
		 VALUES ('tenant-1', 'parcel-x', 'SCAN', 'SCAN/v1', 'PCXL-9',
		         '2026-10-04T08:00:00Z', '2026-10-04T07:00:00Z', now())`); err == nil {
		t.Fatal("封闭集合外的边界种类进了冲突册")
	}
}
