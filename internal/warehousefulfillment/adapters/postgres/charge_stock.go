package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

func (store *Charges) ListStorageDeltas(
	ctx context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	warehouse domain.WarehouseCode,
	at time.Time,
) ([]ports.StorageDelta, error) {
	executor, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list storage deltas: %w", err)
	}
	goods, err := executor.Query(ctx,
		`SELECT movement.posted_at, line.location_code, line.goods_code, line.unit_name, line.stock_state,
		        line.inbound_batch_id, line.quantity_delta, batch.inbound_at
		   FROM warehouse_fulfillment.stock_movement_line line
		   JOIN warehouse_fulfillment.stock_movement movement
		     ON movement.tenant_id = line.tenant_id AND movement.movement_id = line.movement_id
		   JOIN warehouse_fulfillment.inbound_batch batch
		     ON batch.tenant_id = line.tenant_id AND batch.inbound_batch_id = line.inbound_batch_id
		  WHERE line.tenant_id = $1 AND line.owner_account_id = $2 AND line.warehouse_code = $3
		    AND movement.posted_at <= $4`,
		tenant.String(), owner.String(), warehouse.String(), at)
	if err != nil {
		return nil, fmt.Errorf("list storage deltas: %w", err)
	}
	defer goods.Close()
	var deltas []ports.StorageDelta
	for goods.Next() {
		var delta ports.StorageDelta
		if err := goods.Scan(&delta.Posted, &delta.Location, &delta.Goods, &delta.Unit, &delta.State, &delta.Batch, &delta.Delta, &delta.InboundAt); err != nil {
			return nil, fmt.Errorf("list storage deltas: %w", err)
		}
		delta.Form = "GOODS"
		deltas = append(deltas, delta)
	}
	if err := goods.Err(); err != nil {
		return nil, fmt.Errorf("list storage deltas: %w", err)
	}
	cartons, err := executor.Query(ctx,
		`SELECT movement.posted_at, line.location_code, line.carton_number, line.inbound_batch_id,
		        line.quantity_delta, batch.inbound_at
		   FROM warehouse_fulfillment.carton_movement_line line
		   JOIN warehouse_fulfillment.carton_movement movement
		     ON movement.tenant_id = line.tenant_id AND movement.movement_id = line.movement_id
		   JOIN warehouse_fulfillment.carton_inbound_batch batch
		     ON batch.tenant_id = line.tenant_id AND batch.inbound_batch_id = line.inbound_batch_id
		  WHERE line.tenant_id = $1 AND line.owner_account_id = $2 AND line.warehouse_code = $3
		    AND movement.posted_at <= $4`,
		tenant.String(), owner.String(), warehouse.String(), at)
	if err != nil {
		return nil, fmt.Errorf("list storage deltas: %w", err)
	}
	defer cartons.Close()
	for cartons.Next() {
		var delta ports.StorageDelta
		if err := cartons.Scan(&delta.Posted, &delta.Location, &delta.Carton, &delta.Batch, &delta.Delta, &delta.InboundAt); err != nil {
			return nil, fmt.Errorf("list storage deltas: %w", err)
		}
		delta.Form = "CARTON"
		deltas = append(deltas, delta)
	}
	if err := cartons.Err(); err != nil {
		return nil, fmt.Errorf("list storage deltas: %w", err)
	}
	return deltas, nil
}

func (store *Charges) ListStorageFreezes(
	ctx context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	warehouse domain.WarehouseCode,
	at time.Time,
) ([]ports.StorageFreeze, error) {
	executor, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list storage freezes: %w", err)
	}
	rows, err := executor.Query(ctx,
		`SELECT target, COALESCE(location_code, ''), COALESCE(goods_code, ''), COALESCE(unit_name, ''),
		        COALESCE(stock_state, ''), COALESCE(inbound_batch_id, ''), COALESCE(carton_number, ''), quantity
		   FROM warehouse_fulfillment.stock_freeze
		  WHERE tenant_id = $1 AND owner_account_id = $2 AND warehouse_code = $3
		    AND frozen_at <= $4 AND (unfrozen_at IS NULL OR unfrozen_at > $4)`,
		tenant.String(), owner.String(), warehouse.String(), at)
	if err != nil {
		return nil, fmt.Errorf("list storage freezes: %w", err)
	}
	defer rows.Close()
	var freezes []ports.StorageFreeze
	for rows.Next() {
		var freeze ports.StorageFreeze
		if err := rows.Scan(&freeze.Target, &freeze.Location, &freeze.Goods, &freeze.Unit, &freeze.State, &freeze.Batch, &freeze.Carton, &freeze.Quantity); err != nil {
			return nil, fmt.Errorf("list storage freezes: %w", err)
		}
		freezes = append(freezes, freeze)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list storage freezes: %w", err)
	}
	return freezes, nil
}

func (store *Charges) PlaceAt(
	ctx context.Context,
	tenant domain.TenantID,
	warehouse domain.WarehouseCode,
	location string,
	at time.Time,
) (string, string, bool, error) {
	executor, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return "", "", false, fmt.Errorf("load storage place: %w", err)
	}
	var zone string
	var locationType *string
	err = executor.QueryRow(ctx,
		`SELECT zone_code, location_type_code
		   FROM warehouse_fulfillment.location_revision
		  WHERE tenant_id = $1 AND warehouse_code = $2 AND location_code = $3 AND registered_at <= $4
		  ORDER BY revision DESC LIMIT 1`,
		tenant.String(), warehouse.String(), location, at).Scan(&zone, &locationType)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, fmt.Errorf("load storage place: %w", err)
	}
	var usage string
	err = executor.QueryRow(ctx,
		`SELECT zone_usage FROM warehouse_fulfillment.zone_revision
		  WHERE tenant_id = $1 AND warehouse_code = $2 AND zone_code = $3 AND registered_at <= $4
		  ORDER BY revision DESC LIMIT 1`,
		tenant.String(), warehouse.String(), zone, at).Scan(&usage)
	if errors.Is(err, pgx.ErrNoRows) {
		usage = ""
		err = nil
	}
	if err != nil {
		return "", "", false, fmt.Errorf("load storage place: %w", err)
	}
	kind := ""
	if locationType != nil {
		kind = *locationType
	}
	return usage, kind, true, nil
}
