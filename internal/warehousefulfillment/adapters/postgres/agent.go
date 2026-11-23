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

// Agents 保全代理仓的声明、模拟源采用与报来的结果。
type Agents struct {
	db *bentopg.DB
}

func NewAgents(db *bentopg.DB) (*Agents, error) {
	if db == nil {
		return nil, errors.New("warehouse fulfillment postgres: agents need a db")
	}
	return &Agents{db: db}, nil
}

var _ ports.AgentStore = (*Agents)(nil)

func (store *Agents) LoadConnector(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode) (bool, error) {
	executor, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return false, fmt.Errorf("load agent connector: %w", err)
	}
	var found int
	err = executor.QueryRow(ctx,
		`SELECT 1 FROM warehouse_fulfillment.agent_connector WHERE tenant_id = $1 AND warehouse_code = $2`,
		tenant.String(), warehouse.String()).Scan(&found)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load agent connector: %w", err)
	}
	return true, nil
}

func (store *Agents) SaveConnector(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, at time.Time) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save agent connector: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.agent_connector (tenant_id, warehouse_code, registered_at) VALUES ($1,$2,$3)`,
		tenant.String(), warehouse.String(), at); err != nil {
		return fmt.Errorf("save agent connector: %w", err)
	}
	return nil
}

func (store *Agents) LoadStatement(ctx context.Context, tenant domain.TenantID, source string, statedAt time.Time) (ports.AgentStatement, bool, error) {
	executor, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return ports.AgentStatement{}, false, fmt.Errorf("load agent statement: %w", err)
	}
	statement, err := scanStatement(executor.QueryRow(ctx,
		`SELECT source_id, stated_at, valid_until, warehouse_code, owner_account_id, goods_code, unit_name, quantity
		   FROM warehouse_fulfillment.agent_statement
		  WHERE tenant_id = $1 AND source_id = $2 AND stated_at = $3`,
		tenant.String(), source, statedAt))
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.AgentStatement{}, false, nil
	}
	if err != nil {
		return ports.AgentStatement{}, false, fmt.Errorf("load agent statement: %w", err)
	}
	return statement, true, nil
}

func (store *Agents) SaveStatement(ctx context.Context, tenant domain.TenantID, statement ports.AgentStatement) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save agent statement: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.agent_statement (
		    tenant_id, source_id, stated_at, valid_until, warehouse_code, owner_account_id, goods_code, unit_name, quantity)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		tenant.String(), statement.Source, statement.StatedAt, statement.ValidUntil, statement.Warehouse.String(),
		statement.Owner.String(), statement.Goods.String(), statement.Unit.String(), statement.Quantity); err != nil {
		return fmt.Errorf("save agent statement: %w", err)
	}
	return nil
}

func (store *Agents) LatestStatement(ctx context.Context, tenant domain.TenantID, statement ports.AgentStatement) (ports.AgentStatement, bool, error) {
	executor, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return ports.AgentStatement{}, false, fmt.Errorf("load latest agent statement: %w", err)
	}
	loaded, err := scanStatement(executor.QueryRow(ctx,
		`SELECT source_id, stated_at, valid_until, warehouse_code, owner_account_id, goods_code, unit_name, quantity
		   FROM warehouse_fulfillment.agent_statement
		  WHERE tenant_id = $1 AND source_id = $2 AND warehouse_code = $3 AND owner_account_id = $4 AND goods_code = $5
		  ORDER BY stated_at DESC
		  LIMIT 1`,
		tenant.String(), statement.Source, statement.Warehouse.String(), statement.Owner.String(), statement.Goods.String()))
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.AgentStatement{}, false, nil
	}
	if err != nil {
		return ports.AgentStatement{}, false, fmt.Errorf("load latest agent statement: %w", err)
	}
	return loaded, true, nil
}

func (store *Agents) HasLaterStatement(ctx context.Context, tenant domain.TenantID, statement ports.AgentStatement) (bool, error) {
	executor, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return false, fmt.Errorf("load later agent statement: %w", err)
	}
	var found int
	err = executor.QueryRow(ctx,
		`SELECT 1 FROM warehouse_fulfillment.agent_statement
		  WHERE tenant_id = $1 AND source_id = $2 AND warehouse_code = $3 AND owner_account_id = $4 AND goods_code = $5
		    AND stated_at > $6
		  LIMIT 1`,
		tenant.String(), statement.Source, statement.Warehouse.String(), statement.Owner.String(), statement.Goods.String(), statement.StatedAt).Scan(&found)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load later agent statement: %w", err)
	}
	return true, nil
}

func (store *Agents) LoadResult(ctx context.Context, tenant domain.TenantID, id string) (ports.AgentResult, bool, error) {
	executor, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return ports.AgentResult{}, false, fmt.Errorf("load agent result: %w", err)
	}
	var result ports.AgentResult
	var request, warehouse string
	var weight, weightUnit, length, width, height, dimension *string
	err = executor.QueryRow(ctx,
		`SELECT result_id, request_id, warehouse_code, kind, weight_value, weight_unit, length_value, width_value, height_value, dimension_unit, reported_at
		   FROM warehouse_fulfillment.agent_result
		  WHERE tenant_id = $1 AND result_id = $2`,
		tenant.String(), id).Scan(&result.ID, &request, &warehouse, &result.Kind, &weight, &weightUnit, &length, &width, &height, &dimension, &result.Reported)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.AgentResult{}, false, nil
	}
	if err != nil {
		return ports.AgentResult{}, false, fmt.Errorf("load agent result: %w", err)
	}
	result.Request, err = domain.NewOutboundRequestID(request)
	if err != nil {
		return ports.AgentResult{}, false, err
	}
	result.Warehouse, err = domain.NewWarehouseCode(warehouse)
	if err != nil {
		return ports.AgentResult{}, false, err
	}
	result.Weight = valueOrEmpty(weight)
	result.WeightUnit = valueOrEmpty(weightUnit)
	result.Length = valueOrEmpty(length)
	result.Width = valueOrEmpty(width)
	result.Height = valueOrEmpty(height)
	result.Dimension = valueOrEmpty(dimension)
	return result, true, nil
}

func (store *Agents) SaveResult(ctx context.Context, tenant domain.TenantID, result ports.AgentResult) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save agent result: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.agent_result (
		    tenant_id, result_id, request_id, warehouse_code, kind, weight_value, weight_unit,
		    length_value, width_value, height_value, dimension_unit, reported_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		tenant.String(), result.ID, result.Request.String(), result.Warehouse.String(), result.Kind,
		nullableText(result.Weight), nullableText(result.WeightUnit), nullableText(result.Length), nullableText(result.Width),
		nullableText(result.Height), nullableText(result.Dimension), result.Reported); err != nil {
		return fmt.Errorf("save agent result: %w", err)
	}
	return nil
}

func (store *Agents) SaveReportedParcel(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, result ports.AgentResult, goods domain.GoodsCode, unit domain.GoodsUnitName, quantity int64) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save agent parcel: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.outbound_parcel (
		    tenant_id, request_id, parcel_sequence, packaging_material, weight_value, weight_unit,
		    length_value, width_value, height_value, dimension_unit)
		 VALUES ($1,$2,1,'AGENT',$3,$4,$5,$6,$7,$8)`,
		tenant.String(), request.String(), result.Weight, result.WeightUnit, result.Length, result.Width, result.Height, result.Dimension); err != nil {
		return fmt.Errorf("save agent parcel: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.outbound_parcel_line (
		    tenant_id, request_id, parcel_sequence, line_index, goods_code, unit_name, quantity)
		 VALUES ($1,$2,1,0,$3,$4,$5)`,
		tenant.String(), request.String(), goods.String(), unit.String(), quantity); err != nil {
		return fmt.Errorf("save agent parcel line: %w", err)
	}
	return nil
}

func (store *Agents) MarkReportedShipment(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, shipmentID, parcelID string) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("mark agent shipment: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.outbound_parcel
		    SET shipment_request_id = $3, declared_parcel_id = NULLIF($4, '')
		  WHERE tenant_id = $1 AND request_id = $2 AND parcel_sequence = 1 AND shipment_request_id IS NULL`,
		tenant.String(), request.String(), shipmentID, parcelID); err != nil {
		return fmt.Errorf("mark agent shipment: %w", err)
	}
	return nil
}

func (store *Agents) FirstLine(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID) (domain.GoodsCode, domain.GoodsUnitName, int64, bool, error) {
	executor, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return domain.GoodsCode{}, domain.GoodsUnitName{}, 0, false, fmt.Errorf("load agent line: %w", err)
	}
	var goods, unit string
	var quantity int64
	err = executor.QueryRow(ctx,
		`SELECT goods_code, unit_name, quantity
		   FROM warehouse_fulfillment.outbound_request_line
		  WHERE tenant_id = $1 AND request_id = $2
		  ORDER BY line_index
		  LIMIT 1`,
		tenant.String(), request.String()).Scan(&goods, &unit, &quantity)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.GoodsCode{}, domain.GoodsUnitName{}, 0, false, nil
	}
	if err != nil {
		return domain.GoodsCode{}, domain.GoodsUnitName{}, 0, false, fmt.Errorf("load agent line: %w", err)
	}
	goodsCode, err := domain.NewGoodsCode(goods)
	if err != nil {
		return domain.GoodsCode{}, domain.GoodsUnitName{}, 0, false, err
	}
	unitName, err := domain.NewGoodsUnitName(unit)
	if err != nil {
		return domain.GoodsCode{}, domain.GoodsUnitName{}, 0, false, err
	}
	return goodsCode, unitName, quantity, true, nil
}

type statementRow interface {
	Scan(dest ...any) error
}

func scanStatement(row statementRow) (ports.AgentStatement, error) {
	var statement ports.AgentStatement
	var warehouse, owner, goods, unit string
	err := row.Scan(&statement.Source, &statement.StatedAt, &statement.ValidUntil, &warehouse, &owner, &goods, &unit, &statement.Quantity)
	if err != nil {
		return ports.AgentStatement{}, err
	}
	statement.Warehouse, err = domain.NewWarehouseCode(warehouse)
	if err != nil {
		return ports.AgentStatement{}, err
	}
	statement.Owner, err = domain.NewOwnerAccountID(owner)
	if err != nil {
		return ports.AgentStatement{}, err
	}
	statement.Goods, err = domain.NewGoodsCode(goods)
	if err != nil {
		return ports.AgentStatement{}, err
	}
	statement.Unit, err = domain.NewGoodsUnitName(unit)
	if err != nil {
		return ports.AgentStatement{}, err
	}
	return statement, nil
}
