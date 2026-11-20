package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// Adjustments 是库存调整的 PostgreSQL 适配器。提交不写库存流水。
type Adjustments struct {
	db *bentopg.DB
}

func NewAdjustments(db *bentopg.DB) (*Adjustments, error) {
	if db == nil {
		return nil, fmt.Errorf("warehouse fulfillment postgres: db is nil")
	}
	return &Adjustments{db: db}, nil
}

var _ ports.AdjustmentStore = (*Adjustments)(nil)

func (store *Adjustments) Hold(ctx context.Context, tenant domain.TenantID, id domain.RequestKey) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("hold adjustment: %w", err)
	}
	_, err = executor.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1), hashtext($2 || chr(31) || $3))`,
		"warehouse_fulfillment.stock_adjustment", tenant.String(), id.String())
	if err != nil {
		return fmt.Errorf("hold adjustment: %w", err)
	}
	return nil
}

func (store *Adjustments) Load(ctx context.Context, tenant domain.TenantID, id domain.RequestKey) (ports.StockAdjustment, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return ports.StockAdjustment{}, false, err
	}
	var owner, warehouse, reason, operator, status string
	var submitted time.Time
	err = querier.QueryRow(ctx,
		`SELECT owner_account_id, warehouse_code, reason, operator_id, submitted_at, status
		   FROM warehouse_fulfillment.stock_adjustment WHERE tenant_id = $1 AND adjustment_id = $2`,
		tenant.String(), id.String()).Scan(&owner, &warehouse, &reason, &operator, &submitted, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.StockAdjustment{}, false, nil
	}
	if err != nil {
		return ports.StockAdjustment{}, false, fmt.Errorf("load adjustment: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT kind, location_code, goods_code, unit_name, stock_state, label_alias, batch_number, expiry_date::text,
		        inbound_batch_id, quantity, basis
		   FROM warehouse_fulfillment.stock_adjustment_line
		  WHERE tenant_id = $1 AND adjustment_id = $2
		  ORDER BY line_index`, tenant.String(), id.String())
	if err != nil {
		return ports.StockAdjustment{}, false, err
	}
	defer rows.Close()
	var lines []ports.AdjustmentLine
	for rows.Next() {
		var kind, location, goods, unit, state string
		var alias, batch, expiry, inbound, basis *string
		var quantity int64
		if err := rows.Scan(&kind, &location, &goods, &unit, &state, &alias, &batch, &expiry, &inbound, &quantity, &basis); err != nil {
			return ports.StockAdjustment{}, false, err
		}
		line := ports.AdjustmentLine{
			Kind: ports.AdjustmentKind(kind), Location: mustLocation(&location), Goods: mustGoods(&goods), Unit: mustUnit(&unit),
			State: mustState(&state), Alias: mustAlias(alias), Batch: mustBatch(batch), Expiry: mustExpiry(expiry), Quantity: quantity,
			Basis: valueOrEmpty(basis),
		}
		if inbound != nil {
			line.Inbound = mustInbound(inbound)
		}
		lines = append(lines, line)
	}
	if err := rows.Err(); err != nil {
		return ports.StockAdjustment{}, false, err
	}
	return ports.StockAdjustment{
		ID: id, Owner: mustOwnerID(owner), Warehouse: mustWarehouse(warehouse), Reason: reason,
		Operator: mustRegistrant(&operator), Submitted: submitted, Status: status, Lines: lines,
	}, true, nil
}

func (store *Adjustments) Save(ctx context.Context, tenant domain.TenantID, adjustment ports.StockAdjustment) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save adjustment: %w", err)
	}
	_, err = executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.stock_adjustment (
		    tenant_id, adjustment_id, owner_account_id, warehouse_code, reason, operator_id, submitted_at, status
		 ) VALUES ($1,$2,$3,$4,$5,$6,$7,'PENDING')`,
		tenant.String(), adjustment.ID.String(), adjustment.Owner.String(), adjustment.Warehouse.String(),
		adjustment.Reason, adjustment.Operator.String(), adjustment.Submitted)
	if err != nil {
		return fmt.Errorf("save adjustment: %w", err)
	}
	for index, line := range adjustment.Lines {
		if _, err := executor.Exec(ctx,
			`INSERT INTO warehouse_fulfillment.stock_adjustment_line (
			    tenant_id, adjustment_id, line_index, kind, location_code, goods_code, unit_name, stock_state,
			    label_alias, batch_number, expiry_date, inbound_batch_id, quantity, basis
			 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::date,$12,$13,$14)`,
			tenant.String(), adjustment.ID.String(), index, string(line.Kind), line.Location.String(), line.Goods.String(),
			line.Unit.String(), line.State.String(), optionalText(line.Alias.String(), line.Alias.String() != ""),
			optionalText(line.Batch.String(), line.Batch.Given()), optionalText(line.Expiry.String(), line.Expiry.Given()),
			nullableText(line.Inbound.String()), line.Quantity, nullableText(line.Basis),
		); err != nil {
			return fmt.Errorf("save adjustment: %w", err)
		}
	}
	return nil
}

func (store *Adjustments) MarkPosted(ctx context.Context, tenant domain.TenantID, id domain.RequestKey, operator domain.Registrant, at time.Time, note string, dutyRevision int) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("post adjustment: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.stock_adjustment
		    SET status = 'POSTED', approved_by = $3, approved_at = $4, approval_note = $5, duty_revision = $6
		  WHERE tenant_id = $1 AND adjustment_id = $2 AND status = 'PENDING'`,
		tenant.String(), id.String(), operator.String(), at, nullableText(note), dutyRevision)
	if err != nil {
		return fmt.Errorf("post adjustment: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("post adjustment: %s was not pending", id.String())
	}
	return nil
}

func (store *Adjustments) MarkRejected(ctx context.Context, tenant domain.TenantID, id domain.RequestKey, operator domain.Registrant, at time.Time, reason string) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("reject adjustment: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.stock_adjustment
		    SET status = 'REJECTED', rejected_by = $3, rejected_at = $4, reject_reason = $5
		  WHERE tenant_id = $1 AND adjustment_id = $2 AND status = 'PENDING'`,
		tenant.String(), id.String(), operator.String(), at, reason)
	if err != nil {
		return fmt.Errorf("reject adjustment: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("reject adjustment: %s was not pending", id.String())
	}
	return nil
}
