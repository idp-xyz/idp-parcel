package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// AgentStatement 是代理仓报来的一份库存声明。它不进库存账。
type AgentStatement struct {
	Source     string
	StatedAt   time.Time
	ValidUntil time.Time
	Warehouse  domain.WarehouseCode
	Owner      domain.OwnerAccountID
	Goods      domain.GoodsCode
	Unit       domain.GoodsUnitName
	Quantity   int64
}

// AgentResult 是代理仓对一份已转出请求报来的结果，原样留下。
type AgentResult struct {
	ID         string
	Request    domain.OutboundRequestID
	Warehouse  domain.WarehouseCode
	Kind       string
	Weight     string
	WeightUnit string
	Length     string
	Width      string
	Height     string
	Dimension  string
	Reported   time.Time
}

// AgentStore 保全声明、连接器采用与代理仓结果。写入在没有环境事务时拒绝。
type AgentStore interface {
	LoadConnector(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode) (bool, error)
	SaveConnector(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, at time.Time) error
	LoadStatement(ctx context.Context, tenant domain.TenantID, source string, statedAt time.Time) (AgentStatement, bool, error)
	SaveStatement(ctx context.Context, tenant domain.TenantID, statement AgentStatement) error
	LatestStatement(ctx context.Context, tenant domain.TenantID, statement AgentStatement) (AgentStatement, bool, error)
	HasLaterStatement(ctx context.Context, tenant domain.TenantID, statement AgentStatement) (bool, error)
	LoadResult(ctx context.Context, tenant domain.TenantID, id string) (AgentResult, bool, error)
	SaveResult(ctx context.Context, tenant domain.TenantID, result AgentResult) error
	SaveReportedParcel(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, result AgentResult, goods domain.GoodsCode, unit domain.GoodsUnitName, quantity int64) error
	MarkReportedShipment(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, shipmentID, parcelID string) error
	FirstLine(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID) (domain.GoodsCode, domain.GoodsUnitName, int64, bool, error)
}
