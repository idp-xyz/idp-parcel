package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// OutboundNumberIssuer 在调用方的受理事务里取出库请求的单据号。
type OutboundNumberIssuer interface {
	IssueOutboundNumber(ctx context.Context, tenant domain.TenantID, issuerZone domain.BusinessTimeZone) (PreAdviceNumberIssue, error)
}

// OutboundIdentities 签发出库请求标识。只在决定落册时取。
type OutboundIdentities interface {
	NextOutboundRequestID() (domain.OutboundRequestID, error)
}

// StoredShortagePolicy 是某个货主当前的缺货处置。没有这一行就是没选定。
type StoredShortagePolicy struct {
	Revision     int
	Kind         domain.ShortagePolicyKind
	Operator     string
	RegisteredAt time.Time
}

// StoredReservation 是一份还没分配的预占，数量按基本单位。
type StoredReservation struct {
	Goods         domain.GoodsCode
	State         domain.StockState
	Alias         domain.GoodsAliasCode
	Batch         domain.BatchNumber
	Expiry        domain.ExpiryDate
	Quantity      int64
	FromUnlabeled int64
}

// OutboundStore 读写出库请求、预占与缺货处置。写入在没有环境事务时拒绝。
type OutboundStore interface {
	LoadByKey(ctx context.Context, tenant domain.TenantID, origin domain.OutboundOrigin, key domain.RequestKey) (domain.OutboundRequest, bool, error)
	Save(ctx context.Context, request domain.OutboundRequest) error
	ListReservations(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode) ([]StoredReservation, error)
	UnassignedReservation(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode, goods domain.GoodsCode, state domain.StockState, alias domain.GoodsAliasCode, batch domain.BatchNumber, expiry domain.ExpiryDate) (int64, error)
	LoadPolicy(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID) (StoredShortagePolicy, bool, error)
	SavePolicy(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, policy StoredShortagePolicy) error
}
