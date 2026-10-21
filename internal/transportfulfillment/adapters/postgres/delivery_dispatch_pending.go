package postgres

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// DeliveryDispatchPendings 实现派送待形成册（ADR-0230 决定一、二；迁移 0045）：执行器写册的三口与按册重问读的队列口。
//
// 写口一律要环境事务：册上那一行与执行器那一拍的其余写入同笔，否则一拍回滚了、册上那一条却留下来，就成了一笔没人答过的
// 待形成。读口走 ReadExecutor。所有语句显式带租户条件。
type DeliveryDispatchPendings struct {
	db *bentopg.DB
}

var (
	_ ports.DeliveryDispatchPendingRegister = (*DeliveryDispatchPendings)(nil)
	_ ports.DeliveryDispatchPendingQueue    = (*DeliveryDispatchPendings)(nil)
)

func NewDeliveryDispatchPendings(db *bentopg.DB) (*DeliveryDispatchPendings, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	return &DeliveryDispatchPendings{db: db}, nil
}

// RecordMissing 立一条，或把已有那一条改回待形成。首次答复时刻只在立的那一刻写；入场依据、缺件与最近一次答复时刻取这一次——
// 册随执行器最近一次答复，不取较晚的业务时间（补拍报的时刻可以早于先前那一拍）。
func (register *DeliveryDispatchPendings) RecordMissing(
	ctx context.Context,
	key ports.DeliveryDispatchPendingKey,
	entryBasis domain.ParticipationBasisReference,
	missing []string,
	answeredAt time.Time,
) error {
	executor, err := register.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("record delivery dispatch pending: %w", err)
	}
	if len(missing) == 0 {
		return fmt.Errorf("record delivery dispatch pending: no missing requirement named")
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO transport_fulfillment.delivery_dispatch_pending
		     (tenant_id, segment_ref, object_ref, entry_basis, missing, first_answered_at, last_answered_at, state)
		 VALUES ($1, $2, $3, $4, $5, $6, $6, 'PENDING')
		 ON CONFLICT (tenant_id, segment_ref, object_ref) DO UPDATE
		    SET entry_basis      = EXCLUDED.entry_basis,
		        missing          = EXCLUDED.missing,
		        last_answered_at = EXCLUDED.last_answered_at,
		        state            = 'PENDING',
		        task_ref         = NULL,
		        lapsed_by        = NULL,
		        settled_at       = NULL`,
		key.TenantID.String(), key.Segment.String(), key.Object.String(),
		entryBasis.String(), slices.Clone(missing), answeredAt.UTC(),
	); err != nil {
		return fmt.Errorf("record delivery dispatch pending: %w", err)
	}
	return nil
}

// SettleFormed 把一条改成已形成。已形成的不改指别的任务：认出来的总是同一项（执行器沿替代链认前版任务），先落下的那一项为准。
func (register *DeliveryDispatchPendings) SettleFormed(
	ctx context.Context,
	key ports.DeliveryDispatchPendingKey,
	task domain.DispatchTaskReference,
	at time.Time,
) error {
	executor, err := register.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("settle delivery dispatch pending: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`UPDATE transport_fulfillment.delivery_dispatch_pending
		    SET state = 'FORMED', task_ref = $4, lapsed_by = NULL, settled_at = $5
		  WHERE tenant_id = $1 AND segment_ref = $2 AND object_ref = $3 AND state <> 'FORMED'`,
		key.TenantID.String(), key.Segment.String(), key.Object.String(), task.String(), at.UTC(),
	); err != nil {
		return fmt.Errorf("settle delivery dispatch pending: %w", err)
	}
	return nil
}

// SettleLapsed 只把待形成的那一条改成已作罢。
func (register *DeliveryDispatchPendings) SettleLapsed(
	ctx context.Context,
	key ports.DeliveryDispatchPendingKey,
	lapsedBy string,
	at time.Time,
) error {
	executor, err := register.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("lapse delivery dispatch pending: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`UPDATE transport_fulfillment.delivery_dispatch_pending
		    SET state = 'LAPSED', lapsed_by = $4, settled_at = $5
		  WHERE tenant_id = $1 AND segment_ref = $2 AND object_ref = $3 AND state = 'PENDING'`,
		key.TenantID.String(), key.Segment.String(), key.Object.String(), lapsedBy, at.UTC(),
	); err != nil {
		return fmt.Errorf("lapse delivery dispatch pending: %w", err)
	}
	return nil
}

// FindByKey 按键读回一条。否定结果只回 false。
func (register *DeliveryDispatchPendings) FindByKey(
	ctx context.Context,
	key ports.DeliveryDispatchPendingKey,
) (ports.DeliveryDispatchPendingRecord, bool, error) {
	querier, err := register.db.ReadExecutor(ctx)
	if err != nil {
		return ports.DeliveryDispatchPendingRecord{}, false, fmt.Errorf("find delivery dispatch pending: %w", err)
	}
	row := querier.QueryRow(ctx,
		`SELECT `+deliveryDispatchPendingColumns+`
		   FROM transport_fulfillment.delivery_dispatch_pending
		  WHERE tenant_id = $1 AND segment_ref = $2 AND object_ref = $3`,
		key.TenantID.String(), key.Segment.String(), key.Object.String(),
	)
	record, err := scanDeliveryDispatchPending(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.DeliveryDispatchPendingRecord{}, false, nil
	}
	if err != nil {
		return ports.DeliveryDispatchPendingRecord{}, false, fmt.Errorf("find delivery dispatch pending: %w", err)
	}
	return record, true, nil
}

// ListPending 列一个租户待形成的那几件：最近一次答复最早的在前，同刻按（段，对象）稳定排序。answeredBy 非零时只列最近一次
// 答复不晚于它的。
func (register *DeliveryDispatchPendings) ListPending(
	ctx context.Context,
	tenant domain.TenantID,
	answeredBy time.Time,
	limit int,
) ([]ports.DeliveryDispatchPendingRecord, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("list delivery dispatch pending: limit must be positive, got %d", limit)
	}
	querier, err := register.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list delivery dispatch pending: %w", err)
	}
	var due *time.Time
	if !answeredBy.IsZero() {
		at := answeredBy.UTC()
		due = &at
	}
	rows, err := querier.Query(ctx,
		`SELECT `+deliveryDispatchPendingColumns+`
		   FROM transport_fulfillment.delivery_dispatch_pending
		  WHERE tenant_id = $1 AND state = 'PENDING'
		    AND ($2::timestamptz IS NULL OR last_answered_at <= $2::timestamptz)
		  ORDER BY last_answered_at, segment_ref, object_ref
		  LIMIT $3`,
		tenant.String(), due, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list delivery dispatch pending: %w", err)
	}
	defer rows.Close()
	list := make([]ports.DeliveryDispatchPendingRecord, 0, limit)
	for rows.Next() {
		record, err := scanDeliveryDispatchPending(rows)
		if err != nil {
			return nil, fmt.Errorf("list delivery dispatch pending: %w", err)
		}
		list = append(list, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list delivery dispatch pending: %w", err)
	}
	return list, nil
}

const deliveryDispatchPendingColumns = `tenant_id, segment_ref, object_ref, entry_basis, missing, first_answered_at, last_answered_at,
		        state, task_ref, lapsed_by, settled_at`

// scanDeliveryDispatchPending 逐列走各自的构造门装回：按册重问拿这几件去问执行器，坏行要在这里响，不流进那一拍。
func scanDeliveryDispatchPending(row pgx.Row) (ports.DeliveryDispatchPendingRecord, error) {
	var tenantText, segmentText, objectText, basisText, stateText string
	var missing []string
	var firstAt, lastAt time.Time
	var task, lapsedBy *string
	var settledAt *time.Time
	if err := row.Scan(&tenantText, &segmentText, &objectText, &basisText, &missing, &firstAt, &lastAt,
		&stateText, &task, &lapsedBy, &settledAt); err != nil {
		return ports.DeliveryDispatchPendingRecord{}, err
	}
	var record ports.DeliveryDispatchPendingRecord
	var err error
	if record.Key.TenantID, err = domain.NewTenantID(tenantText); err != nil {
		return ports.DeliveryDispatchPendingRecord{}, err
	}
	if record.Key.Segment, err = domain.NewFulfillmentSegmentReference(segmentText); err != nil {
		return ports.DeliveryDispatchPendingRecord{}, err
	}
	if record.Key.Object, err = domain.NewCarriedObjectReference(objectText); err != nil {
		return ports.DeliveryDispatchPendingRecord{}, err
	}
	if record.EntryBasis, err = domain.NewParticipationBasisReference(basisText); err != nil {
		return ports.DeliveryDispatchPendingRecord{}, err
	}
	if record.State, err = deliveryDispatchPendingStateFrom(stateText); err != nil {
		return ports.DeliveryDispatchPendingRecord{}, err
	}
	record.Missing = missing
	record.FirstAnsweredAt, record.LastAnsweredAt = firstAt.UTC(), lastAt.UTC()
	if task != nil {
		record.Task = *task
	}
	if lapsedBy != nil {
		record.LapsedBy = *lapsedBy
	}
	if settledAt != nil {
		record.SettledAt = settledAt.UTC()
	}
	return record, nil
}

func deliveryDispatchPendingStateFrom(raw string) (ports.DeliveryDispatchPendingState, error) {
	for _, state := range []ports.DeliveryDispatchPendingState{
		ports.DeliveryDispatchPending, ports.DeliveryDispatchPendingFormed, ports.DeliveryDispatchPendingLapsed,
	} {
		if state.String() == raw {
			return state, nil
		}
	}
	return ports.DeliveryDispatchPendingStateInvalid, fmt.Errorf("unknown delivery dispatch pending state %q", raw)
}
