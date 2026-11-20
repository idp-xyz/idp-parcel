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

// Waves 是分配策略、波次与拣货任务的 PostgreSQL 适配器。
type Waves struct {
	db *bentopg.DB
}

func NewWaves(db *bentopg.DB) (*Waves, error) {
	if db == nil {
		return nil, fmt.Errorf("warehouse fulfillment postgres: db is nil")
	}
	return &Waves{db: db}, nil
}

var _ ports.WaveStore = (*Waves)(nil)

func (store *Waves) LoadStrategy(ctx context.Context, tenant domain.TenantID) (ports.AllocationStrategy, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return ports.AllocationStrategy{}, false, err
	}
	var revision int
	var operator string
	var at time.Time
	err = querier.QueryRow(ctx,
		`SELECT revision, operator_id, registered_at
		   FROM warehouse_fulfillment.allocation_strategy
		  WHERE tenant_id = $1
		  ORDER BY revision DESC
		  LIMIT 1`, tenant.String()).Scan(&revision, &operator, &at)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.AllocationStrategy{}, false, nil
	}
	if err != nil {
		return ports.AllocationStrategy{}, false, fmt.Errorf("load allocation strategy: %w", err)
	}
	return ports.AllocationStrategy{Revision: revision, Operator: operator, RegisteredAt: at}, true, nil
}

func (store *Waves) SaveStrategy(ctx context.Context, tenant domain.TenantID, strategy ports.AllocationStrategy) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save allocation strategy: %w", err)
	}
	_, err = executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.allocation_strategy (tenant_id, revision, operator_id, registered_at)
		 VALUES ($1,$2,$3,$4)`,
		tenant.String(), strategy.Revision, strategy.Operator, strategy.RegisteredAt)
	if err != nil {
		return fmt.Errorf("save allocation strategy: %w", err)
	}
	return nil
}

func (store *Waves) LoadWave(ctx context.Context, tenant domain.TenantID, id domain.WaveID) ([]domain.OutboundRequestID, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return nil, false, err
	}
	var exists bool
	if err := querier.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM warehouse_fulfillment.wave WHERE tenant_id = $1 AND wave_id = $2)`,
		tenant.String(), id.String()).Scan(&exists); err != nil {
		return nil, false, err
	}
	if !exists {
		return nil, false, nil
	}
	rows, err := querier.Query(ctx,
		`SELECT request_id FROM warehouse_fulfillment.wave_request
		  WHERE tenant_id = $1 AND wave_id = $2
		  ORDER BY request_id`, tenant.String(), id.String())
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var requests []domain.OutboundRequestID
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, false, err
		}
		parsed, err := domain.NewOutboundRequestID(raw)
		if err != nil {
			return nil, false, err
		}
		requests = append(requests, parsed)
	}
	return requests, true, rows.Err()
}

func (store *Waves) SaveWave(ctx context.Context, tenant domain.TenantID, id domain.WaveID, warehouse domain.WarehouseCode, operator domain.Registrant, prefetch bool, at time.Time, requests []domain.OutboundRequestID, tasks []ports.PickTask) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save wave: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.wave (tenant_id, wave_id, warehouse_code, operator_id, prefetch, released_at)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		tenant.String(), id.String(), warehouse.String(), operator.String(), prefetch, at); err != nil {
		return fmt.Errorf("save wave: %w", err)
	}
	seen := map[string]struct{}{}
	for _, request := range requests {
		if _, ok := seen[request.String()]; ok {
			continue
		}
		seen[request.String()] = struct{}{}
		if _, err := executor.Exec(ctx,
			`INSERT INTO warehouse_fulfillment.wave_request (tenant_id, wave_id, request_id) VALUES ($1,$2,$3)`,
			tenant.String(), id.String(), request.String()); err != nil {
			return fmt.Errorf("save wave request: %w", err)
		}
		tag, err := executor.Exec(ctx,
			`UPDATE warehouse_fulfillment.outbound_request
			    SET status = 'ALLOCATED'
			  WHERE tenant_id = $1 AND request_id = $2 AND status = 'RESERVED'`,
			tenant.String(), request.String())
		if err != nil {
			return fmt.Errorf("allocate outbound request: %w", err)
		}
		if tag.RowsAffected() != 1 {
			return fmt.Errorf("allocate outbound request: %s was not reserved", request.String())
		}
	}
	for _, task := range tasks {
		if _, err := executor.Exec(ctx,
			`INSERT INTO warehouse_fulfillment.pick_task (
			    tenant_id, task_id, wave_id, request_id, location_code, goods_code, unit_name, stock_state,
			    inbound_batch_id, quantity, picked, status, fact_id
			 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,0,'OPEN',NULL)`,
			tenant.String(), task.ID.String(), task.WaveID.String(), task.Request.String(), task.Location.String(),
			task.Goods.String(), task.Unit.String(), task.State.String(), task.Inbound.String(), task.Quantity); err != nil {
			return fmt.Errorf("save pick task: %w", err)
		}
	}
	return nil
}

func (store *Waves) LoadTask(ctx context.Context, tenant domain.TenantID, id domain.PickTaskID) (ports.PickTask, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return ports.PickTask{}, false, err
	}
	task, err := scanPickTask(querier.QueryRow(ctx,
		`SELECT task_id, wave_id, request_id, location_code, goods_code, unit_name, stock_state, inbound_batch_id, quantity, picked, status, fact_id
		   FROM warehouse_fulfillment.pick_task
		  WHERE tenant_id = $1 AND task_id = $2`, tenant.String(), id.String()))
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.PickTask{}, false, nil
	}
	if err != nil {
		return ports.PickTask{}, false, fmt.Errorf("load pick task: %w", err)
	}
	return task, true, nil
}

func (store *Waves) TasksOf(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID) ([]ports.PickTask, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := querier.Query(ctx,
		`SELECT task_id, wave_id, request_id, location_code, goods_code, unit_name, stock_state, inbound_batch_id, quantity, picked, status, fact_id
		   FROM warehouse_fulfillment.pick_task
		  WHERE tenant_id = $1 AND request_id = $2
		  ORDER BY task_id`, tenant.String(), request.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []ports.PickTask
	for rows.Next() {
		task, err := scanPickTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (store *Waves) SavePick(ctx context.Context, tenant domain.TenantID, task ports.PickTask, requestStatus domain.OutboundStatus, differenceID, factID string, short int64) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save pick: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.pick_task
		    SET picked = $3, status = $4, fact_id = $5
		  WHERE tenant_id = $1 AND task_id = $2 AND status = 'OPEN'`,
		tenant.String(), task.ID.String(), task.Picked, task.Status, factID)
	if err != nil {
		return fmt.Errorf("save pick: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("save pick: %s was not open", task.ID.String())
	}
	if err := reduceReservation(ctx, executor, tenant, task.Request, task.Picked); err != nil {
		return err
	}
	if _, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.outbound_request
		    SET status = $3
		  WHERE tenant_id = $1 AND request_id = $2`,
		tenant.String(), task.Request.String(), requestStatus.String()); err != nil {
		return fmt.Errorf("save pick: %w", err)
	}
	if short > 0 {
		if _, err := executor.Exec(ctx,
			`INSERT INTO warehouse_fulfillment.pick_difference (tenant_id, difference_id, task_id, fact_id, quantity, status)
			 VALUES ($1,$2,$3,$4,$5,'PENDING')`,
			tenant.String(), differenceID, task.ID.String(), factID, short); err != nil {
			return fmt.Errorf("save pick difference: %w", err)
		}
	}
	return nil
}

func (store *Waves) SaveCheck(ctx context.Context, tenant domain.TenantID, fact domain.PickFactID, request domain.OutboundRequestID, observed int64, matched bool, at time.Time, next domain.OutboundStatus) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save outbound check: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.outbound_check (tenant_id, fact_id, request_id, observed, matched, checked_at)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		tenant.String(), fact.String(), request.String(), observed, matched, at); err != nil {
		return fmt.Errorf("save outbound check: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.outbound_request SET status = $3
		  WHERE tenant_id = $1 AND request_id = $2 AND status = 'PICKED'`,
		tenant.String(), request.String(), next.String()); err != nil {
		return fmt.Errorf("save outbound check: %w", err)
	}
	return nil
}

func (store *Waves) LoadCheck(ctx context.Context, tenant domain.TenantID, fact domain.PickFactID) (int64, bool, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return 0, false, false, err
	}
	var observed int64
	var matched bool
	err = querier.QueryRow(ctx,
		`SELECT observed, matched FROM warehouse_fulfillment.outbound_check WHERE tenant_id = $1 AND fact_id = $2`,
		tenant.String(), fact.String()).Scan(&observed, &matched)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, false, nil
	}
	if err != nil {
		return 0, false, false, fmt.Errorf("load outbound check: %w", err)
	}
	return observed, matched, true, nil
}

func (store *Waves) SavePack(ctx context.Context, tenant domain.TenantID, fact domain.PickFactID, request domain.OutboundRequestID, parcels []ports.PackParcel, at time.Time) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save pack: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.outbound_pack (tenant_id, fact_id, request_id, packed_at) VALUES ($1,$2,$3,$4)`,
		tenant.String(), fact.String(), request.String(), at); err != nil {
		return fmt.Errorf("save pack: %w", err)
	}
	for _, parcel := range parcels {
		if _, err := executor.Exec(ctx,
			`INSERT INTO warehouse_fulfillment.outbound_parcel (
			    tenant_id, request_id, parcel_sequence, packaging_material, weight_value, weight_unit,
			    length_value, width_value, height_value, dimension_unit
			 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			tenant.String(), request.String(), parcel.Sequence, parcel.Packaging, parcel.Weight, parcel.WeightUnit,
			parcel.Length, parcel.Width, parcel.Height, parcel.Dimension); err != nil {
			return fmt.Errorf("save parcel: %w", err)
		}
		if _, err := executor.Exec(ctx,
			`INSERT INTO warehouse_fulfillment.outbound_parcel_line (
			    tenant_id, request_id, parcel_sequence, line_index, goods_code, unit_name, quantity
			 ) VALUES ($1,$2,$3,0,$4,$5,$6)`,
			tenant.String(), request.String(), parcel.Sequence, parcel.Goods.String(), parcel.Unit.String(), parcel.Quantity); err != nil {
			return fmt.Errorf("save parcel line: %w", err)
		}
	}
	tag, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.outbound_request SET status = 'PACKED'
		  WHERE tenant_id = $1 AND request_id = $2 AND status = 'CHECKED'`,
		tenant.String(), request.String())
	if err != nil {
		return fmt.Errorf("save pack: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("save pack: %s was not checked", request.String())
	}
	return nil
}

func (store *Waves) ParcelsOf(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID) ([]ports.PackParcel, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := querier.Query(ctx,
		`SELECT parcel.parcel_sequence, parcel.packaging_material, parcel.weight_value, parcel.weight_unit,
		        parcel.length_value, parcel.width_value, parcel.height_value, parcel.dimension_unit,
		        parcel.shipment_request_id, parcel.commission_state, line.goods_code, line.unit_name, line.quantity
		   FROM warehouse_fulfillment.outbound_parcel AS parcel
		   JOIN warehouse_fulfillment.outbound_parcel_line AS line
		     ON line.tenant_id = parcel.tenant_id AND line.request_id = parcel.request_id
		    AND line.parcel_sequence = parcel.parcel_sequence AND line.line_index = 0
		  WHERE parcel.tenant_id = $1 AND parcel.request_id = $2
		  ORDER BY parcel.parcel_sequence`,
		tenant.String(), request.String())
	if err != nil {
		return nil, fmt.Errorf("list parcels: %w", err)
	}
	defer rows.Close()
	var parcels []ports.PackParcel
	for rows.Next() {
		var parcel ports.PackParcel
		var shipment, commission *string
		var goods, unit string
		if err := rows.Scan(&parcel.Sequence, &parcel.Packaging, &parcel.Weight, &parcel.WeightUnit,
			&parcel.Length, &parcel.Width, &parcel.Height, &parcel.Dimension, &shipment, &commission, &goods, &unit, &parcel.Quantity); err != nil {
			return nil, fmt.Errorf("list parcels: %w", err)
		}
		if shipment != nil {
			parcel.ShipmentRequestID = *shipment
		}
		if commission != nil {
			parcel.CommissionState = *commission
		}
		parcel.Goods, err = domain.NewGoodsCode(goods)
		if err != nil {
			return nil, err
		}
		parcel.Unit, err = domain.NewGoodsUnitName(unit)
		if err != nil {
			return nil, err
		}
		parcels = append(parcels, parcel)
	}
	return parcels, rows.Err()
}

func (store *Waves) MarkShipment(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, sequence int, shipmentRequestID string) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("mark shipment: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.outbound_parcel SET shipment_request_id = $4
		  WHERE tenant_id = $1 AND request_id = $2 AND parcel_sequence = $3 AND shipment_request_id IS NULL`,
		tenant.String(), request.String(), sequence, shipmentRequestID)
	if err != nil {
		return fmt.Errorf("mark shipment: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("mark shipment: parcel %d of %s was already submitted", sequence, request.String())
	}
	return nil
}

func (store *Waves) LoadAcceptance(ctx context.Context, tenant domain.TenantID, decisionID string) (string, string, string, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return "", "", "", false, err
	}
	var source, key, state string
	err = querier.QueryRow(ctx,
		`SELECT source, source_request_key, state FROM warehouse_fulfillment.acceptance_posting
		  WHERE tenant_id = $1 AND decision_id = $2`,
		tenant.String(), decisionID).Scan(&source, &key, &state)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", "", false, nil
	}
	if err != nil {
		return "", "", "", false, fmt.Errorf("load acceptance: %w", err)
	}
	return source, key, state, true, nil
}

func (store *Waves) SaveAcceptance(ctx context.Context, tenant domain.TenantID, decisionID, source, key, state string, at time.Time) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save acceptance: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.acceptance_posting (
		    tenant_id, decision_id, source, source_request_key, state, posted_at)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		tenant.String(), decisionID, source, key, state, at); err != nil {
		return fmt.Errorf("save acceptance: %w", err)
	}
	return nil
}

func (store *Waves) SetCommissionState(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, sequence int, state string) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("set commission state: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.outbound_parcel SET commission_state = $4
		  WHERE tenant_id = $1 AND request_id = $2 AND parcel_sequence = $3
		    AND shipment_request_id IS NOT NULL AND commission_state IS NULL`,
		tenant.String(), request.String(), sequence, state)
	if err != nil {
		return fmt.Errorf("set commission state: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("set commission state: parcel %d of %s was not awaiting a decision", sequence, request.String())
	}
	return nil
}

func (store *Waves) LoadSupplement(ctx context.Context, tenant domain.TenantID, version string) (string, string, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return "", "", false, err
	}
	var source, key string
	err = querier.QueryRow(ctx,
		`SELECT source, source_request_key FROM warehouse_fulfillment.supplement_posting
		  WHERE tenant_id = $1 AND submission_version_id = $2`,
		tenant.String(), version).Scan(&source, &key)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, fmt.Errorf("load supplement: %w", err)
	}
	return source, key, true, nil
}

func (store *Waves) SaveSupplement(ctx context.Context, tenant domain.TenantID, version, source, key string, at time.Time) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save supplement: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.supplement_posting (
		    tenant_id, submission_version_id, source, source_request_key, posted_at)
		 VALUES ($1,$2,$3,$4,$5)`,
		tenant.String(), version, source, key, at); err != nil {
		return fmt.Errorf("save supplement: %w", err)
	}
	return nil
}

func (store *Waves) LoadPack(ctx context.Context, tenant domain.TenantID, fact domain.PickFactID) (bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return false, err
	}
	var found bool
	err = querier.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM warehouse_fulfillment.outbound_pack WHERE tenant_id = $1 AND fact_id = $2)`,
		tenant.String(), fact.String()).Scan(&found)
	return found, err
}

func reduceReservation(ctx context.Context, executor bentopg.Executor, tenant domain.TenantID, request domain.OutboundRequestID, picked int64) error {
	rows, err := executor.Query(ctx,
		`SELECT line_index, quantity
		   FROM warehouse_fulfillment.outbound_reservation
		  WHERE tenant_id = $1 AND request_id = $2
		  ORDER BY line_index`, tenant.String(), request.String())
	if err != nil {
		return fmt.Errorf("save pick: %w", err)
	}
	defer rows.Close()
	type row struct {
		index, quantity int64
	}
	var listed []row
	for rows.Next() {
		var item row
		if err := rows.Scan(&item.index, &item.quantity); err != nil {
			rows.Close()
			return err
		}
		listed = append(listed, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	left := picked
	for _, item := range listed {
		take := item.quantity
		if take > left {
			take = left
		}
		if _, err := executor.Exec(ctx,
			`UPDATE warehouse_fulfillment.outbound_reservation
			    SET quantity = $4, from_unlabeled = LEAST(from_unlabeled, $4)
			  WHERE tenant_id = $1 AND request_id = $2 AND line_index = $3`,
			tenant.String(), request.String(), item.index, item.quantity-take); err != nil {
			return fmt.Errorf("save pick: %w", err)
		}
		left -= take
	}
	return nil
}

type pickRowScanner interface {
	Scan(dest ...any) error
}

func scanPickTask(row pickRowScanner) (ports.PickTask, error) {
	var task, wave, request, location, goods, unit, state, inbound, status string
	var fact *string
	var quantity, picked int64
	if err := row.Scan(&task, &wave, &request, &location, &goods, &unit, &state, &inbound, &quantity, &picked, &status, &fact); err != nil {
		return ports.PickTask{}, err
	}
	parsedTask, err := domain.NewPickTaskID(task)
	if err != nil {
		return ports.PickTask{}, err
	}
	parsedWave, err := domain.NewWaveID(wave)
	if err != nil {
		return ports.PickTask{}, err
	}
	parsedRequest, err := domain.NewOutboundRequestID(request)
	if err != nil {
		return ports.PickTask{}, err
	}
	parsedLocation, err := domain.NewLocationCode(location)
	if err != nil {
		return ports.PickTask{}, err
	}
	parsedGoods, err := domain.NewGoodsCode(goods)
	if err != nil {
		return ports.PickTask{}, err
	}
	parsedUnit, err := domain.NewGoodsUnitName(unit)
	if err != nil {
		return ports.PickTask{}, err
	}
	parsedState, err := domain.NewStockState(state)
	if err != nil {
		return ports.PickTask{}, err
	}
	parsedInbound, err := domain.NewInboundBatchID(inbound)
	if err != nil {
		return ports.PickTask{}, err
	}
	return ports.PickTask{
		ID: parsedTask, WaveID: parsedWave, Request: parsedRequest, Location: parsedLocation, Goods: parsedGoods,
		Unit: parsedUnit, State: parsedState, Inbound: parsedInbound, Quantity: quantity, Picked: picked, Status: status,
		Fact: valueOrEmpty(fact),
	}, nil
}
