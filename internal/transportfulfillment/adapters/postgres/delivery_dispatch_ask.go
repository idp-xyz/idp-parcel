package postgres

import (
	"context"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// DeliveryDispatchAsks 实现问询册（ADR-0303 决定五与复核记录；迁移 0054）：只增不改，一次按下一行，不去重。写口要环境事务，
// 读口走 ReadExecutor；所有语句显式带租户条件。
type DeliveryDispatchAsks struct {
	db *bentopg.DB
}

var _ ports.DeliveryDispatchAskLedger = (*DeliveryDispatchAsks)(nil)

func NewDeliveryDispatchAsks(db *bentopg.DB) (*DeliveryDispatchAsks, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	return &DeliveryDispatchAsks{db: db}, nil
}

// RecordAsk 追加一行。补拍的段与对象、按册重问的问法与件数上限互斥，库上 CHECK 守一遍形状；这里只按种类落各自那几列，另一种的
// 列落 NULL。
func (ledger *DeliveryDispatchAsks) RecordAsk(ctx context.Context, record ports.DeliveryDispatchAskRecord) error {
	executor, err := ledger.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("record delivery dispatch ask: %w", err)
	}
	var segment, object, scope *string
	var limit *int
	switch record.Kind {
	case ports.DeliveryDispatchAskTrigger:
		segmentText, objectText := record.Segment.String(), record.Object.String()
		segment, object = &segmentText, &objectText
	case ports.DeliveryDispatchAskReask:
		scope, limit = &record.Scope, &record.Limit
	default:
		return fmt.Errorf("record delivery dispatch ask: unknown kind %d", record.Kind)
	}
	if !record.AskedBy.Present() {
		return fmt.Errorf("record delivery dispatch ask: an ask without an operator is not recorded")
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO transport_fulfillment.delivery_dispatch_ask
		     (tenant_id, kind, asked_by, asked_at, segment_ref, object_ref, scope, batch_limit, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())`,
		record.TenantID.String(), record.Kind.String(), record.AskedBy.String(), record.AskedAt.UTC(),
		segment, object, scope, limit,
	); err != nil {
		return fmt.Errorf("record delivery dispatch ask: %w", err)
	}
	return nil
}

// ListAsks 由近及远列一个租户的按下记录；读回逐列走构造门，坏行在这里响。
func (ledger *DeliveryDispatchAsks) ListAsks(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.DeliveryDispatchAskRecord, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("list delivery dispatch asks: limit must be positive, got %d", limit)
	}
	querier, err := ledger.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list delivery dispatch asks: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT kind, asked_by, asked_at, segment_ref, object_ref, scope, batch_limit
		   FROM transport_fulfillment.delivery_dispatch_ask
		  WHERE tenant_id = $1
		  ORDER BY asked_at DESC, ask_id DESC
		  LIMIT $2`,
		tenant.String(), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list delivery dispatch asks: %w", err)
	}
	defer rows.Close()
	list := make([]ports.DeliveryDispatchAskRecord, 0, limit)
	for rows.Next() {
		var kindText, askedBy string
		var askedAt time.Time
		var segment, object, scope *string
		var batchLimit *int
		if err := rows.Scan(&kindText, &askedBy, &askedAt, &segment, &object, &scope, &batchLimit); err != nil {
			return nil, fmt.Errorf("list delivery dispatch asks: %w", err)
		}
		record := ports.DeliveryDispatchAskRecord{TenantID: tenant, AskedAt: askedAt.UTC()}
		if record.AskedBy, err = domain.NewRegistrantReference(askedBy); err != nil {
			return nil, fmt.Errorf("list delivery dispatch asks: %w", err)
		}
		switch kindText {
		case ports.DeliveryDispatchAskTrigger.String():
			record.Kind = ports.DeliveryDispatchAskTrigger
			if segment == nil || object == nil {
				return nil, fmt.Errorf("list delivery dispatch asks: a trigger ask without its segment and object")
			}
			if record.Segment, err = domain.NewFulfillmentSegmentReference(*segment); err != nil {
				return nil, fmt.Errorf("list delivery dispatch asks: %w", err)
			}
			if record.Object, err = domain.NewCarriedObjectReference(*object); err != nil {
				return nil, fmt.Errorf("list delivery dispatch asks: %w", err)
			}
		case ports.DeliveryDispatchAskReask.String():
			record.Kind = ports.DeliveryDispatchAskReask
			if scope == nil || batchLimit == nil {
				return nil, fmt.Errorf("list delivery dispatch asks: a reask without its scope and limit")
			}
			record.Scope, record.Limit = *scope, *batchLimit
		default:
			return nil, fmt.Errorf("list delivery dispatch asks: unknown kind %q", kindText)
		}
		list = append(list, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list delivery dispatch asks: %w", err)
	}
	return list, nil
}
