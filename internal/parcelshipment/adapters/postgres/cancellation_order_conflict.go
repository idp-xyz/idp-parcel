package postgres

import (
	"context"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// CancellationOrderConflicts 实现 ports.CancellationOrderConflictRegister：取消先后冲突的只追加册
// （ADR-0254 决定二）。主键是（租户，包裹，边界种类，边界那一代，取消），同一份冲突重投撞主键答已有记录
// （ADR-0031 的写入代数）。
type CancellationOrderConflicts struct {
	db *bentopg.DB
}

func NewCancellationOrderConflicts(db *bentopg.DB) (*CancellationOrderConflicts, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel shipment postgres: db is nil")
	}
	return &CancellationOrderConflicts{db: db}, nil
}

var _ ports.CancellationOrderConflictRegister = (*CancellationOrderConflicts)(nil)

// Record 记下一份冲突。要环境事务：冲突与消费入账同笔落地。
func (register *CancellationOrderConflicts) Record(
	ctx context.Context,
	record ports.CancellationOrderConflictRecord,
) (ports.CancellationOrderConflictSaveOutcome, error) {
	executor, err := register.db.RequireExecutor(ctx)
	if err != nil {
		return ports.CancellationOrderConflictSaveOutcomeInvalid, fmt.Errorf("record cancellation order conflict: %w", err)
	}
	conflict := record.Conflict
	boundary := conflict.Boundary()
	tag, err := executor.Exec(ctx,
		`INSERT INTO parcel_shipment.cancellation_order_conflict
			(tenant_id, parcel_id, boundary_kind, boundary_ref, cancellation_id,
			 cancelled_at, boundary_at, detected_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT DO NOTHING`,
		record.TenantID.String(),
		conflict.Parcel().String(),
		boundary.Kind.String(),
		boundary.Reference.String(),
		conflict.Cancellation().String(),
		conflict.CancelledAt().UTC(),
		boundary.OccurredAt.UTC(),
		record.DetectedAt.UTC(),
	)
	if err != nil {
		return ports.CancellationOrderConflictSaveOutcomeInvalid, fmt.Errorf("record cancellation order conflict: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.CancellationOrderConflictAlreadyRecorded, nil
	}
	return ports.CancellationOrderConflictRecorded, nil
}

// ListByParcel 按包裹交回册上的冲突，按发现时间、边界那一代排序。否定结果只回空，不区分「没有」与
// 「属于另一个租户」。
func (register *CancellationOrderConflicts) ListByParcel(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.DeclaredParcelID,
) ([]ports.CancellationOrderConflictEntry, error) {
	querier, err := register.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list cancellation order conflicts: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT boundary_kind, boundary_ref, cancellation_id, cancelled_at, boundary_at, detected_at
		   FROM parcel_shipment.cancellation_order_conflict
		  WHERE tenant_id = $1 AND parcel_id = $2
		  ORDER BY detected_at, boundary_ref, cancellation_id`,
		tenant.String(), parcel.String())
	if err != nil {
		return nil, fmt.Errorf("list cancellation order conflicts: %w", err)
	}
	defer rows.Close()

	var entries []ports.CancellationOrderConflictEntry
	for rows.Next() {
		var (
			kind, reference, cancellationID     string
			cancelledAt, boundaryAt, detectedAt time.Time
		)
		if err := rows.Scan(&kind, &reference, &cancellationID, &cancelledAt, &boundaryAt, &detectedAt); err != nil {
			return nil, fmt.Errorf("list cancellation order conflicts: %w", err)
		}
		entry, err := conflictEntry(tenant, parcel, kind, reference, cancellationID, cancelledAt, boundaryAt, detectedAt)
		if err != nil {
			return nil, fmt.Errorf("list cancellation order conflicts: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list cancellation order conflicts: %w", err)
	}
	return entries, nil
}

func conflictEntry(
	tenant domain.TenantID,
	parcel domain.DeclaredParcelID,
	kindRaw, referenceRaw, cancellationRaw string,
	cancelledAt, boundaryAt, detectedAt time.Time,
) (ports.CancellationOrderConflictEntry, error) {
	kind, err := cancellationBoundaryKindFrom(kindRaw)
	if err != nil {
		return ports.CancellationOrderConflictEntry{}, err
	}
	reference, err := domain.NewCancellationBoundaryReference(referenceRaw)
	if err != nil {
		return ports.CancellationOrderConflictEntry{}, err
	}
	cancellation, err := domain.NewParcelCancellationID(cancellationRaw)
	if err != nil {
		return ports.CancellationOrderConflictEntry{}, err
	}
	return ports.CancellationOrderConflictEntry{
		TenantID:     tenant,
		Parcel:       parcel,
		Cancellation: cancellation,
		CancelledAt:  cancelledAt.UTC(),
		Boundary:     domain.CancellationBoundaryFact{Kind: kind, Reference: reference, OccurredAt: boundaryAt.UTC()},
		DetectedAt:   detectedAt.UTC(),
	}, nil
}

// cancellationBoundaryKindFrom 把库面的种类词认回封闭集合；词不在集合内即拒，不猜。
func cancellationBoundaryKindFrom(raw string) (domain.CancellationBoundaryKind, error) {
	for _, kind := range []domain.CancellationBoundaryKind{
		domain.NetworkIntakeBoundary,
		domain.CarrierFirstEffectivePickupBoundary,
	} {
		if kind.String() == raw {
			return kind, nil
		}
	}
	return domain.CancellationBoundaryKindInvalid, fmt.Errorf("unknown cancellation boundary kind %q", raw)
}
