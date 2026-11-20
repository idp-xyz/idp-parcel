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

// WarehouseMeasurements 是仓测尺寸重量册的 PostgreSQL 适配器。只追加，不改不删。
type WarehouseMeasurements struct {
	db *bentopg.DB
}

func NewWarehouseMeasurements(db *bentopg.DB) (*WarehouseMeasurements, error) {
	if db == nil {
		return nil, fmt.Errorf("warehouse fulfillment postgres: db is nil")
	}
	return &WarehouseMeasurements{db: db}, nil
}

var (
	_ ports.WarehouseMeasurementStore = (*WarehouseMeasurements)(nil)
	_ ports.WarehouseMeasurementRead  = (*WarehouseMeasurements)(nil)
)

const warehouseMeasurementColumns = `
	tenant_id, fact_id, device_id, operator_id, occurred_at, device_sequence, received_at, warehouse_code,
	target_kind, owner_account_id, goods_code, packaging_level, carton_number,
	length_value::text, width_value::text, height_value::text, dimensions_unit,
	weight_value::text, weight_unit`

func (store *WarehouseMeasurements) HoldFact(
	ctx context.Context,
	tenant domain.TenantID,
	factID domain.MeasurementFactID,
) error {
	const operation = "hold warehouse measurement fact"
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(
		    hashtext('warehouse_fulfillment.warehouse_measurement.fact'),
		    hashtext($1 || chr(31) || $2))`,
		tenant.String(), factID.String()); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}

func (store *WarehouseMeasurements) LoadByFact(
	ctx context.Context,
	tenant domain.TenantID,
	factID domain.MeasurementFactID,
) (domain.WarehouseMeasurement, bool, error) {
	const operation = "load warehouse measurement"
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return domain.WarehouseMeasurement{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	row := querier.QueryRow(ctx,
		`SELECT `+warehouseMeasurementColumns+`
		   FROM warehouse_fulfillment.warehouse_measurement
		  WHERE tenant_id = $1 AND fact_id = $2`,
		tenant.String(), factID.String())
	loaded, err := scanWarehouseMeasurement(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.WarehouseMeasurement{}, false, nil
	}
	if err != nil {
		return domain.WarehouseMeasurement{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return loaded, true, nil
}

func (store *WarehouseMeasurements) Save(
	ctx context.Context,
	measurement domain.WarehouseMeasurement,
) (bool, error) {
	const operation = "save warehouse measurement"
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return false, fmt.Errorf("%s: %w", operation, err)
	}
	target := measurement.Target()
	var goodsCode, level, carton *string
	switch target.Kind() {
	case domain.MeasurementTargetGoodsLevel:
		g := target.Goods().String()
		l := target.Level().String()
		goodsCode, level = &g, &l
	case domain.MeasurementTargetCarton:
		c := target.Carton().String()
		carton = &c
	default:
		return false, fmt.Errorf("%s: unknown target kind %s", operation, target.Kind())
	}
	dims := measurement.Dimensions()
	weight := measurement.Weight()
	var inserted string
	err = executor.QueryRow(ctx,
		`INSERT INTO warehouse_fulfillment.warehouse_measurement (
		    tenant_id, fact_id, device_id, operator_id, occurred_at, device_sequence, received_at, warehouse_code,
		    target_kind, owner_account_id, goods_code, packaging_level, carton_number,
		    length_value, width_value, height_value, dimensions_unit, weight_value, weight_unit
		 ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
		 ON CONFLICT (tenant_id, fact_id) DO NOTHING
		 RETURNING fact_id`,
		measurement.Tenant().String(), measurement.FactID().String(),
		measurement.Device().String(), measurement.Operator().String(),
		measurement.OccurredAt(), measurement.DeviceSequence(), measurement.ReceivedAt(),
		measurement.Warehouse().String(),
		target.Kind().String(), target.Owner().String(), goodsCode, level, carton,
		dims.Length().String(), dims.Width().String(), dims.Height().String(), dims.Unit().String(),
		weight.Value().String(), weight.Unit().String(),
	).Scan(&inserted)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("%s: %w", operation, err)
	}
	return true, nil
}

func (store *WarehouseMeasurements) ListForGoods(
	ctx context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	goods domain.GoodsCode,
) ([]domain.WarehouseMeasurement, error) {
	const operation = "list warehouse measurements for goods"
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	rows, err := querier.Query(ctx,
		`SELECT `+warehouseMeasurementColumns+`
		   FROM warehouse_fulfillment.warehouse_measurement
		  WHERE tenant_id = $1 AND target_kind = 'GOODS_LEVEL'
		    AND owner_account_id = $2 AND goods_code = $3
		  ORDER BY occurred_at, device_sequence, received_at, fact_id`,
		tenant.String(), owner.String(), goods.String())
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	return collectWarehouseMeasurements(rows, operation)
}

func (store *WarehouseMeasurements) ListForGoodsBatch(
	ctx context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	goods []domain.GoodsCode,
) (map[domain.GoodsCode][]domain.WarehouseMeasurement, error) {
	const operation = "list warehouse measurements for goods batch"
	grouped := make(map[domain.GoodsCode][]domain.WarehouseMeasurement, len(goods))
	if len(goods) == 0 {
		return grouped, nil
	}
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	codes := make([]string, len(goods))
	for index, code := range goods {
		codes[index] = code.String()
	}
	rows, err := querier.Query(ctx,
		`SELECT `+warehouseMeasurementColumns+`
		   FROM warehouse_fulfillment.warehouse_measurement
		  WHERE tenant_id = $1 AND target_kind = 'GOODS_LEVEL'
		    AND owner_account_id = $2 AND goods_code = ANY($3)
		  ORDER BY goods_code, occurred_at, device_sequence, received_at, fact_id`,
		tenant.String(), owner.String(), codes)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	list, err := collectWarehouseMeasurements(rows, operation)
	if err != nil {
		return nil, err
	}
	for _, item := range list {
		code := item.Target().Goods()
		grouped[code] = append(grouped[code], item)
	}
	return grouped, nil
}

func (store *WarehouseMeasurements) ListForGoodsLevel(
	ctx context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	goods domain.GoodsCode,
	level domain.GoodsUnitName,
) ([]domain.WarehouseMeasurement, error) {
	const operation = "list warehouse measurements for goods level"
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	rows, err := querier.Query(ctx,
		`SELECT `+warehouseMeasurementColumns+`
		   FROM warehouse_fulfillment.warehouse_measurement
		  WHERE tenant_id = $1 AND target_kind = 'GOODS_LEVEL'
		    AND owner_account_id = $2 AND goods_code = $3 AND packaging_level = $4
		  ORDER BY occurred_at, device_sequence, received_at, fact_id`,
		tenant.String(), owner.String(), goods.String(), level.String())
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	return collectWarehouseMeasurements(rows, operation)
}

func (store *WarehouseMeasurements) ListForCarton(
	ctx context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	warehouse domain.WarehouseCode,
	carton domain.CartonNumber,
) ([]domain.WarehouseMeasurement, error) {
	const operation = "list warehouse measurements for carton"
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	rows, err := querier.Query(ctx,
		`SELECT `+warehouseMeasurementColumns+`
		   FROM warehouse_fulfillment.warehouse_measurement
		  WHERE tenant_id = $1 AND target_kind = 'CARTON'
		    AND owner_account_id = $2 AND warehouse_code = $3 AND carton_number = $4
		  ORDER BY occurred_at, device_sequence, received_at, fact_id`,
		tenant.String(), owner.String(), warehouse.String(), carton.String())
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	return collectWarehouseMeasurements(rows, operation)
}

func collectWarehouseMeasurements(rows pgx.Rows, operation string) ([]domain.WarehouseMeasurement, error) {
	defer rows.Close()
	var list []domain.WarehouseMeasurement
	for rows.Next() {
		item, err := scanWarehouseMeasurement(rows)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", operation, err)
		}
		list = append(list, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	return list, nil
}

type measurementScanner interface {
	Scan(dest ...any) error
}

func scanWarehouseMeasurement(row measurementScanner) (domain.WarehouseMeasurement, error) {
	var (
		tenantText, factText, deviceText, operatorText, warehouseText string
		occurredAt, receivedAt                                        time.Time
		sequence                                                      int64
		targetKindText, ownerText                                     string
		goodsText, levelText, cartonText                              *string
		lengthText, widthText, heightText, dimsUnitText               string
		weightText, weightUnitText                                    string
	)
	if err := row.Scan(
		&tenantText, &factText, &deviceText, &operatorText, &occurredAt, &sequence, &receivedAt, &warehouseText,
		&targetKindText, &ownerText, &goodsText, &levelText, &cartonText,
		&lengthText, &widthText, &heightText, &dimsUnitText,
		&weightText, &weightUnitText,
	); err != nil {
		return domain.WarehouseMeasurement{}, err
	}
	tenant, err := domain.NewTenantID(tenantText)
	if err != nil {
		return domain.WarehouseMeasurement{}, err
	}
	factID, err := domain.NewMeasurementFactID(factText)
	if err != nil {
		return domain.WarehouseMeasurement{}, err
	}
	device, err := domain.NewMeasurementDeviceID(deviceText)
	if err != nil {
		return domain.WarehouseMeasurement{}, err
	}
	operator, err := domain.NewMeasurementOperatorID(operatorText)
	if err != nil {
		return domain.WarehouseMeasurement{}, err
	}
	warehouse, err := domain.NewWarehouseCode(warehouseText)
	if err != nil {
		return domain.WarehouseMeasurement{}, err
	}
	owner, err := domain.NewOwnerAccountID(ownerText)
	if err != nil {
		return domain.WarehouseMeasurement{}, err
	}
	kind, err := domain.NewMeasurementTargetKind(targetKindText)
	if err != nil {
		return domain.WarehouseMeasurement{}, err
	}
	var target domain.MeasurementTarget
	switch kind {
	case domain.MeasurementTargetGoodsLevel:
		if goodsText == nil || levelText == nil {
			return domain.WarehouseMeasurement{}, errors.New("goods level measurement missing goods or level")
		}
		goods, err := domain.NewGoodsCode(*goodsText)
		if err != nil {
			return domain.WarehouseMeasurement{}, err
		}
		level, err := domain.NewGoodsUnitName(*levelText)
		if err != nil {
			return domain.WarehouseMeasurement{}, err
		}
		target, err = domain.NewGoodsLevelMeasurementTarget(owner, goods, level)
		if err != nil {
			return domain.WarehouseMeasurement{}, err
		}
	case domain.MeasurementTargetCarton:
		if cartonText == nil {
			return domain.WarehouseMeasurement{}, errors.New("carton measurement missing carton number")
		}
		carton, err := domain.NewCartonNumber(*cartonText)
		if err != nil {
			return domain.WarehouseMeasurement{}, err
		}
		target, err = domain.NewCartonMeasurementTarget(owner, carton)
		if err != nil {
			return domain.WarehouseMeasurement{}, err
		}
	default:
		return domain.WarehouseMeasurement{}, fmt.Errorf("unknown target kind %q", targetKindText)
	}
	length, err := domain.NewMeasure(lengthText)
	if err != nil {
		return domain.WarehouseMeasurement{}, err
	}
	width, err := domain.NewMeasure(widthText)
	if err != nil {
		return domain.WarehouseMeasurement{}, err
	}
	height, err := domain.NewMeasure(heightText)
	if err != nil {
		return domain.WarehouseMeasurement{}, err
	}
	dimsUnit, err := domain.NewUnitCode(dimsUnitText)
	if err != nil {
		return domain.WarehouseMeasurement{}, err
	}
	dimensions, err := domain.NewDeclaredDimensions(length, width, height, dimsUnit)
	if err != nil {
		return domain.WarehouseMeasurement{}, err
	}
	weightVal, err := domain.NewMeasure(weightText)
	if err != nil {
		return domain.WarehouseMeasurement{}, err
	}
	weightUnit, err := domain.NewUnitCode(weightUnitText)
	if err != nil {
		return domain.WarehouseMeasurement{}, err
	}
	weight, err := domain.NewDeclaredWeight(weightVal, weightUnit)
	if err != nil {
		return domain.WarehouseMeasurement{}, err
	}
	return domain.NewWarehouseMeasurement(domain.WarehouseMeasurementSpec{
		Tenant:         tenant,
		FactID:         factID,
		Device:         device,
		Operator:       operator,
		OccurredAt:     occurredAt,
		DeviceSequence: sequence,
		ReceivedAt:     receivedAt,
		Warehouse:      warehouse,
		Target:         target,
		Dimensions:     dimensions,
		Weight:         weight,
	})
}
