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
		`SELECT task_id, wave_id, request_id, location_code, goods_code, unit_name, stock_state, inbound_batch_id, quantity, picked, status, fact_id, staging_location
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
		`SELECT task_id, wave_id, request_id, location_code, goods_code, unit_name, stock_state, inbound_batch_id, quantity, picked, status, fact_id, staging_location
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
		    SET picked = $3, status = $4, fact_id = $5, staging_location = NULLIF($6, '')
		  WHERE tenant_id = $1 AND task_id = $2 AND status = 'OPEN'`,
		tenant.String(), task.ID.String(), task.Picked, task.Status, factID, task.Staging.String())
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
		        parcel.shipment_request_id, parcel.declared_parcel_id, parcel.commission_state, parcel.correction_state,
		        line.goods_code, line.unit_name, line.quantity
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
		var shipment, declared, commission, correction *string
		var goods, unit string
		if err := rows.Scan(&parcel.Sequence, &parcel.Packaging, &parcel.Weight, &parcel.WeightUnit,
			&parcel.Length, &parcel.Width, &parcel.Height, &parcel.Dimension, &shipment, &declared, &commission, &correction, &goods, &unit, &parcel.Quantity); err != nil {
			return nil, fmt.Errorf("list parcels: %w", err)
		}
		if shipment != nil {
			parcel.ShipmentRequestID = *shipment
		}
		if declared != nil {
			parcel.DeclaredParcelID = *declared
		}
		if commission != nil {
			parcel.CommissionState = *commission
		}
		if correction != nil {
			parcel.CorrectionState = *correction
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

func (store *Waves) MarkShipment(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, sequence int, shipmentRequestID, declaredParcelID string) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("mark shipment: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.outbound_parcel
		    SET shipment_request_id = $4, declared_parcel_id = NULLIF($5, '')
		  WHERE tenant_id = $1 AND request_id = $2 AND parcel_sequence = $3 AND shipment_request_id IS NULL`,
		tenant.String(), request.String(), sequence, shipmentRequestID, declaredParcelID)
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
		    AND shipment_request_id IS NOT NULL
		    AND (commission_state IS NULL OR commission_state = 'AWAITING_SUPPLEMENT')`,
		tenant.String(), request.String(), sequence, state)
	if err != nil {
		return fmt.Errorf("set commission state: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("set commission state: parcel %d of %s was not awaiting a decision", sequence, request.String())
	}
	return nil
}

func (store *Waves) LoadBasis(ctx context.Context, tenant domain.TenantID, sourceKey string) (string, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return "", false, err
	}
	var version string
	err = querier.QueryRow(ctx,
		`SELECT submission_version_id FROM warehouse_fulfillment.supplement_posting
		  WHERE tenant_id = $1 AND source_request_key = $2
		  ORDER BY posted_at DESC LIMIT 1`,
		tenant.String(), sourceKey).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("load supplement basis: %w", err)
	}
	return version, true, nil
}

func (store *Waves) ClearAwaiting(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, sequence int) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("clear awaiting supplement: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.outbound_parcel SET commission_state = NULL
		  WHERE tenant_id = $1 AND request_id = $2 AND parcel_sequence = $3 AND commission_state = 'AWAITING_SUPPLEMENT'`,
		tenant.String(), request.String(), sequence)
	if err != nil {
		return fmt.Errorf("clear awaiting supplement: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("clear awaiting supplement: parcel %d of %s was not awaiting", sequence, request.String())
	}
	return nil
}

func (store *Waves) SetCorrection(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, sequence int, state string) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("set correction: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.outbound_parcel SET correction_state = $4
		  WHERE tenant_id = $1 AND request_id = $2 AND parcel_sequence = $3`,
		tenant.String(), request.String(), sequence, state); err != nil {
		return fmt.Errorf("set correction: %w", err)
	}
	return nil
}

func (store *Waves) ParcelByDeclared(ctx context.Context, tenant domain.TenantID, declaredParcelID string) (domain.OutboundRequestID, int, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return domain.OutboundRequestID{}, 0, false, err
	}
	var request string
	var sequence int
	err = querier.QueryRow(ctx,
		`SELECT request_id, parcel_sequence FROM warehouse_fulfillment.outbound_parcel
		  WHERE tenant_id = $1 AND declared_parcel_id = $2`,
		tenant.String(), declaredParcelID).Scan(&request, &sequence)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.OutboundRequestID{}, 0, false, nil
	}
	if err != nil {
		return domain.OutboundRequestID{}, 0, false, fmt.Errorf("find declared parcel: %w", err)
	}
	id, err := domain.NewOutboundRequestID(request)
	if err != nil {
		return domain.OutboundRequestID{}, 0, false, err
	}
	return id, sequence, true, nil
}

func (store *Waves) LoadSourceData(ctx context.Context, tenant domain.TenantID, version string) (string, string, string, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return "", "", "", false, err
	}
	var source, key, adoption string
	err = querier.QueryRow(ctx,
		`SELECT source, source_request_key, adoption FROM warehouse_fulfillment.source_data_posting
		  WHERE tenant_id = $1 AND version_id = $2`,
		tenant.String(), version).Scan(&source, &key, &adoption)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", "", false, nil
	}
	if err != nil {
		return "", "", "", false, fmt.Errorf("load source data: %w", err)
	}
	return source, key, adoption, true, nil
}

func (store *Waves) SaveSourceData(ctx context.Context, tenant domain.TenantID, version, source, key, adoption string, at time.Time) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save source data: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.source_data_posting (
		    tenant_id, version_id, source, source_request_key, adoption, posted_at)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		tenant.String(), version, source, key, adoption, at); err != nil {
		return fmt.Errorf("save source data: %w", err)
	}
	return nil
}

func (store *Waves) LoadCancellation(ctx context.Context, tenant domain.TenantID, requestKey, parcelID string) (bool, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return false, false, err
	}
	var matched bool
	err = querier.QueryRow(ctx,
		`SELECT matched FROM warehouse_fulfillment.cancellation_posting
		  WHERE tenant_id = $1 AND request_key = $2 AND parcel_id = $3`,
		tenant.String(), requestKey, parcelID).Scan(&matched)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("load cancellation: %w", err)
	}
	return matched, true, nil
}

func (store *Waves) SaveCancellation(ctx context.Context, tenant domain.TenantID, requestKey, parcelID string, matched bool, at time.Time) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save cancellation: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.cancellation_posting (
		    tenant_id, request_key, parcel_id, matched, posted_at)
		 VALUES ($1,$2,$3,$4,$5)`,
		tenant.String(), requestKey, parcelID, matched, at); err != nil {
		return fmt.Errorf("save cancellation: %w", err)
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
	var fact, staging *string
	var quantity, picked int64
	if err := row.Scan(&task, &wave, &request, &location, &goods, &unit, &state, &inbound, &quantity, &picked, &status, &fact, &staging); err != nil {
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
	var parsedStaging domain.LocationCode
	if staging != nil && *staging != "" {
		parsedStaging, err = domain.NewLocationCode(*staging)
		if err != nil {
			return ports.PickTask{}, err
		}
	}
	return ports.PickTask{
		ID: parsedTask, WaveID: parsedWave, Request: parsedRequest, Location: parsedLocation, Staging: parsedStaging, Goods: parsedGoods,
		Unit: parsedUnit, State: parsedState, Inbound: parsedInbound, Quantity: quantity, Picked: picked, Status: status,
		Fact: valueOrEmpty(fact),
	}, nil
}

func (store *Waves) LoadConfirmation(ctx context.Context, tenant domain.TenantID, factID string) (ports.OutboundConfirmation, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return ports.OutboundConfirmation{}, false, err
	}
	var request, kind, missing string
	var sequence int
	var offline bool
	var external, party, credential *string
	var occurred time.Time
	err = querier.QueryRow(ctx,
		`SELECT request_id, parcel_sequence, kind, offline, missing, external_number, pickup_party, pickup_credential, occurred_at
		   FROM warehouse_fulfillment.outbound_confirmation
		  WHERE tenant_id = $1 AND fact_id = $2`,
		tenant.String(), factID).Scan(&request, &sequence, &kind, &offline, &missing, &external, &party, &credential, &occurred)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.OutboundConfirmation{}, false, nil
	}
	if err != nil {
		return ports.OutboundConfirmation{}, false, fmt.Errorf("load outbound confirmation: %w", err)
	}
	parsed, err := domain.NewOutboundRequestID(request)
	if err != nil {
		return ports.OutboundConfirmation{}, false, err
	}
	return ports.OutboundConfirmation{
		FactID: factID, Request: parsed, Sequence: sequence, Kind: kind, Offline: offline, Missing: missing,
		ExternalNumber: valueOrEmpty(external), PickupParty: valueOrEmpty(party), PickupCredential: valueOrEmpty(credential),
		OccurredAt: occurred,
	}, true, nil
}

func (store *Waves) SaveConfirmation(ctx context.Context, tenant domain.TenantID, confirmation ports.OutboundConfirmation) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save outbound confirmation: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.outbound_confirmation (
		    tenant_id, fact_id, request_id, parcel_sequence, kind, offline, missing,
		    external_number, pickup_party, pickup_credential, occurred_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7, NULLIF($8,''), NULLIF($9,''), NULLIF($10,''), $11)`,
		tenant.String(), confirmation.FactID, confirmation.Request.String(), confirmation.Sequence, confirmation.Kind,
		confirmation.Offline, confirmation.Missing, confirmation.ExternalNumber, confirmation.PickupParty,
		confirmation.PickupCredential, confirmation.OccurredAt); err != nil {
		return fmt.Errorf("save outbound confirmation: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.outbound_parcel SET confirmation_state = $4
		  WHERE tenant_id = $1 AND request_id = $2 AND parcel_sequence = $3 AND confirmation_state IS NULL`,
		tenant.String(), confirmation.Request.String(), confirmation.Sequence, confirmation.State)
	if err != nil {
		return fmt.Errorf("save outbound confirmation: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("save outbound confirmation: parcel %d of %s was already confirmed", confirmation.Sequence, confirmation.Request.String())
	}
	if _, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.outbound_request SET status = 'SHIPPED'
		  WHERE tenant_id = $1 AND request_id = $2 AND status = 'PACKED'
		    AND NOT EXISTS (
		        SELECT 1 FROM warehouse_fulfillment.outbound_parcel
		         WHERE tenant_id = $1 AND request_id = $2 AND confirmation_state IS NULL)`,
		tenant.String(), confirmation.Request.String()); err != nil {
		return fmt.Errorf("save outbound confirmation: %w", err)
	}
	return nil
}

func (store *Waves) ParcelConfirmed(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, sequence int) (bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return false, err
	}
	var state *string
	err = querier.QueryRow(ctx,
		`SELECT confirmation_state FROM warehouse_fulfillment.outbound_parcel
		  WHERE tenant_id = $1 AND request_id = $2 AND parcel_sequence = $3`,
		tenant.String(), request.String(), sequence).Scan(&state)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load parcel confirmation: %w", err)
	}
	return state != nil && *state != "", nil
}

func (store *Waves) LoadOwnerLabel(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, sequence int) (string, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return "", false, err
	}
	var number string
	err = querier.QueryRow(ctx,
		`SELECT external_number FROM warehouse_fulfillment.outbound_owner_label
		  WHERE tenant_id = $1 AND request_id = $2 AND parcel_sequence = $3`,
		tenant.String(), request.String(), sequence).Scan(&number)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("load owner label: %w", err)
	}
	return number, true, nil
}

func (store *Waves) SaveOwnerLabel(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, sequence int, externalNumber string) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save owner label: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.outbound_owner_label (tenant_id, request_id, parcel_sequence, external_number)
		 VALUES ($1,$2,$3,$4)`,
		tenant.String(), request.String(), sequence, externalNumber); err != nil {
		return fmt.Errorf("save owner label: %w", err)
	}
	return nil
}

func (store *Waves) OwnerLabelsOf(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID) (map[int]string, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := querier.Query(ctx,
		`SELECT parcel_sequence, external_number FROM warehouse_fulfillment.outbound_owner_label
		  WHERE tenant_id = $1 AND request_id = $2`,
		tenant.String(), request.String())
	if err != nil {
		return nil, fmt.Errorf("list owner labels: %w", err)
	}
	defer rows.Close()
	labels := map[int]string{}
	for rows.Next() {
		var sequence int
		var number string
		if err := rows.Scan(&sequence, &number); err != nil {
			return nil, err
		}
		labels[sequence] = number
	}
	return labels, rows.Err()
}

func (store *Waves) LoadCorrection(ctx context.Context, tenant domain.TenantID, factID string) (ports.ConfirmationCorrection, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ConfirmationCorrection{}, false, err
	}
	var kind, operator, status string
	var occurred *time.Time
	err = querier.QueryRow(ctx,
		`SELECT kind, occurred_at, operator_id, status FROM warehouse_fulfillment.outbound_confirmation_correction
		  WHERE tenant_id = $1 AND fact_id = $2`,
		tenant.String(), factID).Scan(&kind, &occurred, &operator, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.ConfirmationCorrection{}, false, nil
	}
	if err != nil {
		return ports.ConfirmationCorrection{}, false, fmt.Errorf("load confirmation correction: %w", err)
	}
	correction := ports.ConfirmationCorrection{FactID: factID, Kind: kind, Operator: operator, Status: status}
	if occurred != nil {
		correction.OccurredAt = *occurred
	}
	return correction, true, nil
}

func (store *Waves) SaveCorrection(ctx context.Context, tenant domain.TenantID, correction ports.ConfirmationCorrection) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save confirmation correction: %w", err)
	}
	var occurred *time.Time
	if !correction.OccurredAt.IsZero() {
		at := correction.OccurredAt.UTC()
		occurred = &at
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.outbound_confirmation_correction (
		    tenant_id, fact_id, kind, occurred_at, operator_id, status)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		tenant.String(), correction.FactID, correction.Kind, occurred, correction.Operator, correction.Status); err != nil {
		return fmt.Errorf("save confirmation correction: %w", err)
	}
	if correction.Kind == "TIME" {
		if _, err := executor.Exec(ctx,
			`UPDATE warehouse_fulfillment.outbound_confirmation SET occurred_at = $3
			  WHERE tenant_id = $1 AND fact_id = $2`,
			tenant.String(), correction.FactID, correction.OccurredAt.UTC()); err != nil {
			return fmt.Errorf("save confirmation correction: %w", err)
		}
	}
	return nil
}

func (store *Waves) PostReplenishment(ctx context.Context, tenant domain.TenantID, factID string, approver domain.Registrant, dutyRevision int, at time.Time) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("post replenishment: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.outbound_confirmation_correction
		    SET status = 'POSTED', approver_id = $3, duty_revision = $4, approved_at = $5
		  WHERE tenant_id = $1 AND fact_id = $2 AND status = 'PENDING'`,
		tenant.String(), factID, approver.String(), dutyRevision, at.UTC())
	if err != nil {
		return fmt.Errorf("post replenishment: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("post replenishment: %s was not pending", factID)
	}
	var request string
	var sequence int
	if err := executor.QueryRow(ctx,
		`SELECT request_id, parcel_sequence FROM warehouse_fulfillment.outbound_confirmation
		  WHERE tenant_id = $1 AND fact_id = $2`,
		tenant.String(), factID).Scan(&request, &sequence); err != nil {
		return fmt.Errorf("post replenishment: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.outbound_parcel SET confirmation_state = NULL
		  WHERE tenant_id = $1 AND request_id = $2 AND parcel_sequence = $3`,
		tenant.String(), request, sequence); err != nil {
		return fmt.Errorf("post replenishment: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.outbound_request SET status = 'PACKED'
		  WHERE tenant_id = $1 AND request_id = $2 AND status = 'SHIPPED'`,
		tenant.String(), request); err != nil {
		return fmt.Errorf("post replenishment: %w", err)
	}
	return nil
}

func (store *Waves) LoadCutoff(ctx context.Context, tenant domain.TenantID, key string) (string, int, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return "", 0, false, err
	}
	var request string
	var sequence *int
	err = querier.QueryRow(ctx,
		`SELECT request_id, parcel_sequence FROM warehouse_fulfillment.outbound_cutoff
		  WHERE tenant_id = $1 AND cutoff_key = $2`,
		tenant.String(), key).Scan(&request, &sequence)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", 0, false, nil
	}
	if err != nil {
		return "", 0, false, fmt.Errorf("load cutoff: %w", err)
	}
	if sequence == nil {
		return request, 0, true, nil
	}
	return request, *sequence, true, nil
}

func (store *Waves) SaveCutoff(ctx context.Context, tenant domain.TenantID, key, requestID string, sequence int, basis, outcome string, at time.Time) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save cutoff: %w", err)
	}
	var parcel *int
	if sequence > 0 {
		parcel = &sequence
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.outbound_cutoff (
		    tenant_id, cutoff_key, request_id, parcel_sequence, basis, outcome, posted_at)
		 VALUES ($1,$2,$3,$4, NULLIF($5,''), $6, $7)`,
		tenant.String(), key, requestID, parcel, basis, outcome, at.UTC()); err != nil {
		return fmt.Errorf("save cutoff: %w", err)
	}
	return nil
}

func (store *Waves) ReleaseReservations(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("release reservations: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.outbound_reservation
		    SET quantity = 0, from_unlabeled = 0
		  WHERE tenant_id = $1 AND request_id = $2`,
		tenant.String(), request.String()); err != nil {
		return fmt.Errorf("release reservations: %w", err)
	}
	return nil
}

func (store *Waves) CancelOpenTasks(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("cancel open tasks: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.pick_task SET status = 'CANCELLED'
		  WHERE tenant_id = $1 AND request_id = $2 AND status = 'OPEN'`,
		tenant.String(), request.String()); err != nil {
		return fmt.Errorf("cancel open tasks: %w", err)
	}
	return nil
}

func (store *Waves) SaveReturnTask(ctx context.Context, tenant domain.TenantID, task ports.ReturnTask) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save return task: %w", err)
	}
	var sequence *int
	if task.Sequence > 0 {
		value := task.Sequence
		sequence = &value
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.return_task (
		    tenant_id, return_id, request_id, parcel_sequence, location_code, goods_code, unit_name,
		    stock_state, inbound_batch_id, quantity, status)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'OPEN')`,
		tenant.String(), task.ID, task.Request.String(), sequence, task.Location.String(), task.Goods.String(),
		task.Unit.String(), task.State.String(), task.Inbound.String(), task.Quantity); err != nil {
		return fmt.Errorf("save return task: %w", err)
	}
	return nil
}

func (store *Waves) SetRequestStatus(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, status domain.OutboundStatus) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("set request status: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.outbound_request SET status = $3
		  WHERE tenant_id = $1 AND request_id = $2`,
		tenant.String(), request.String(), status.String()); err != nil {
		return fmt.Errorf("set request status: %w", err)
	}
	return nil
}

func (store *Waves) MarkParcelCutoff(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, sequence int) (domain.OutboundStatus, error) {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return domain.OutboundStatusInvalid, fmt.Errorf("mark parcel cutoff: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.outbound_parcel SET cutoff_state = 'CANCELLED'
		  WHERE tenant_id = $1 AND request_id = $2 AND parcel_sequence = $3
		    AND cutoff_state IS NULL AND confirmation_state IS NULL`,
		tenant.String(), request.String(), sequence)
	if err != nil {
		return domain.OutboundStatusInvalid, fmt.Errorf("mark parcel cutoff: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return domain.OutboundStatusInvalid, fmt.Errorf("mark parcel cutoff: parcel %d of %s was not open", sequence, request.String())
	}
	var remaining int
	if err := executor.QueryRow(ctx,
		`SELECT count(*) FROM warehouse_fulfillment.outbound_parcel
		  WHERE tenant_id = $1 AND request_id = $2 AND cutoff_state IS NULL AND confirmation_state IS NULL`,
		tenant.String(), request.String()).Scan(&remaining); err != nil {
		return domain.OutboundStatusInvalid, fmt.Errorf("mark parcel cutoff: %w", err)
	}
	next := domain.OutboundPartiallyCancelled
	if remaining == 0 {
		next = domain.OutboundCancelled
	}
	if err := store.SetRequestStatus(ctx, tenant, request, next); err != nil {
		return domain.OutboundStatusInvalid, err
	}
	return next, nil
}

func (store *Waves) SaveLatePick(ctx context.Context, tenant domain.TenantID, task ports.PickTask, factID string) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save late pick: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.pick_task
		    SET picked = $3, status = 'LATE', fact_id = $4, staging_location = NULLIF($5, '')
		  WHERE tenant_id = $1 AND task_id = $2 AND status = 'CANCELLED'`,
		tenant.String(), task.ID.String(), task.Picked, factID, task.Staging.String())
	if err != nil {
		return fmt.Errorf("save late pick: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("save late pick: %s was not cancelled", task.ID.String())
	}
	return nil
}

func (store *Waves) LoadReturnTask(ctx context.Context, tenant domain.TenantID, id string) (ports.ReturnTask, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ReturnTask{}, false, err
	}
	var request, location, goods, unit, state, inbound, status string
	var sequence *int
	var fact *string
	var quantity int64
	err = querier.QueryRow(ctx,
		`SELECT request_id, parcel_sequence, location_code, goods_code, unit_name, stock_state,
		        inbound_batch_id, quantity, status, fact_id
		   FROM warehouse_fulfillment.return_task
		  WHERE tenant_id = $1 AND return_id = $2`,
		tenant.String(), id).Scan(&request, &sequence, &location, &goods, &unit, &state, &inbound, &quantity, &status, &fact)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.ReturnTask{}, false, nil
	}
	if err != nil {
		return ports.ReturnTask{}, false, fmt.Errorf("load return task: %w", err)
	}
	parsedRequest, err := domain.NewOutboundRequestID(request)
	if err != nil {
		return ports.ReturnTask{}, false, err
	}
	parsedLocation, err := domain.NewLocationCode(location)
	if err != nil {
		return ports.ReturnTask{}, false, err
	}
	parsedGoods, err := domain.NewGoodsCode(goods)
	if err != nil {
		return ports.ReturnTask{}, false, err
	}
	parsedUnit, err := domain.NewGoodsUnitName(unit)
	if err != nil {
		return ports.ReturnTask{}, false, err
	}
	parsedState, err := domain.NewStockState(state)
	if err != nil {
		return ports.ReturnTask{}, false, err
	}
	parsedInbound, err := domain.NewInboundBatchID(inbound)
	if err != nil {
		return ports.ReturnTask{}, false, err
	}
	task := ports.ReturnTask{
		ID: id, Request: parsedRequest, Location: parsedLocation, Goods: parsedGoods, Unit: parsedUnit,
		State: parsedState, Inbound: parsedInbound, Quantity: quantity, Status: status, Fact: valueOrEmpty(fact),
	}
	if sequence != nil {
		task.Sequence = *sequence
	}
	return task, true, nil
}

func (store *Waves) CompleteReturn(ctx context.Context, tenant domain.TenantID, id, factID string) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("complete return: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.return_task SET status = 'DONE', fact_id = $3
		  WHERE tenant_id = $1 AND return_id = $2 AND status = 'OPEN'`,
		tenant.String(), id, factID)
	if err != nil {
		return fmt.Errorf("complete return: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("complete return: %s was not open", id)
	}
	return nil
}

func (store *Waves) OpenReturns(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID) (int, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return 0, err
	}
	var count int
	err = querier.QueryRow(ctx,
		`SELECT count(*) FROM warehouse_fulfillment.return_task
		  WHERE tenant_id = $1 AND request_id = $2 AND status = 'OPEN'`,
		tenant.String(), request.String()).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count open returns: %w", err)
	}
	return count, nil
}
