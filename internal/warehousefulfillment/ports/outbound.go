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
// 一份修订说两件事，各说各的：Kind 是受理时可用量不够怎么办，PartialShipment 是拣不齐时怎么发；
// 哪一件没说，那一件就是没选定。
type StoredShortagePolicy struct {
	Revision        int
	Kind            domain.ShortagePolicyKind
	PartialShipment domain.PartialShipment
	Operator        string
	RegisteredAt    time.Time
}

// PartialShipmentConsent 是受理之后货主对一张请求另外表示的部分发货意思，连同代录的人与依据。
type PartialShipmentConsent struct {
	Request    domain.OutboundRequestID
	Basis      string
	Rule       domain.PartialShipment
	Operator   string
	RecordedAt time.Time
}

// StoredReservation 是一份还没分配的预占，数量按基本单位。
type StoredReservation struct {
	Warehouse     domain.WarehouseCode
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
	LoadByID(ctx context.Context, tenant domain.TenantID, id domain.OutboundRequestID) (domain.OutboundRequest, bool, error)
	ListChildren(ctx context.Context, tenant domain.TenantID, parent domain.OutboundRequestID) ([]domain.OutboundRequest, error)
	Save(ctx context.Context, request domain.OutboundRequest) error
	ReleaseForSplit(ctx context.Context, tenant domain.TenantID, id domain.OutboundRequestID) (bool, error)
	ListReservations(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode) ([]StoredReservation, error)
	UnassignedReservation(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode, goods domain.GoodsCode, state domain.StockState, alias domain.GoodsAliasCode, batch domain.BatchNumber, expiry domain.ExpiryDate) (int64, error)
	LoadPolicy(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID) (StoredShortagePolicy, bool, error)
	SavePolicy(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, policy StoredShortagePolicy) error
	LoadConsent(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, basis string) (PartialShipmentConsent, bool, error)
	// LatestConsent 是这张请求最晚记下的那一句。
	LatestConsent(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID) (PartialShipmentConsent, bool, error)
	SaveConsent(ctx context.Context, tenant domain.TenantID, consent PartialShipmentConsent) error
}
