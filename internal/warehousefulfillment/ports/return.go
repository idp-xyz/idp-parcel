package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// ReturnLine 是退货预报上预计的一种商品。预报本身不是库存。
type ReturnLine struct {
	Goods    domain.GoodsCode
	Unit     domain.GoodsUnitName
	Quantity int64
}

// ReturnAdvice 是一份已记下的退货预报。
type ReturnAdvice struct {
	ID        domain.ReturnAdviceID
	Owner     domain.OwnerAccountID
	Warehouse domain.WarehouseCode
	Origin    domain.ReturnOrigin
	Key       string
	Tracking  string
	Basis     string
	Operator  string
	Number    string
	Absence   string
	Lines     []ReturnLine
	Received  time.Time
}

// ReturnReceipt 是一次退货到仓。已清点的有货主；认不出的只引用待认领到货。
type ReturnReceipt struct {
	Fact      domain.ReturnFactID
	Warehouse domain.WarehouseCode
	Location  domain.LocationCode
	Tracking  string
	Owner     string
	Advice    string
	Arrival   string
	Goods     domain.GoodsCode
	Unit      domain.GoodsUnitName
	State     domain.StockState
	Quantity  int64
	Outcome   string
	Occurred  time.Time
}

// ReturnDisposition 是一件退货的去向。它不代替库内流水。
type ReturnDisposition struct {
	Decision    domain.ReturnDecisionID
	Fact        domain.ReturnFactID
	Destination domain.ReturnDestination
	Basis       string
	Operator    string
	Origin      domain.ReturnOrigin
	Decided     time.Time
}

// ReturnStore 记下退货预报、到仓与处置。写入在没有环境事务时拒绝。
type ReturnStore interface {
	LoadAdviceByKey(ctx context.Context, tenant domain.TenantID, origin domain.ReturnOrigin, key string) (ReturnAdvice, bool, error)
	SaveAdvice(ctx context.Context, tenant domain.TenantID, advice ReturnAdvice) error
	FindAdviceByTracking(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, tracking string) (owner domain.OwnerAccountID, advice domain.ReturnAdviceID, found bool, err error)
	FindOwnerByExternalNumber(ctx context.Context, tenant domain.TenantID, tracking string) (domain.OwnerAccountID, bool, error)
	LoadReceipt(ctx context.Context, tenant domain.TenantID, fact domain.ReturnFactID) (ReturnReceipt, bool, error)
	SaveReceipt(ctx context.Context, tenant domain.TenantID, receipt ReturnReceipt) error
	LoadDisposition(ctx context.Context, tenant domain.TenantID, decision domain.ReturnDecisionID) (ReturnDisposition, bool, error)
	SaveDisposition(ctx context.Context, tenant domain.TenantID, disposition ReturnDisposition) error
}

// ReturnNumberIssuer 在记下退货预报的同一事务里取单据号。
type ReturnNumberIssuer interface {
	IssueReturnNumber(ctx context.Context, tenant domain.TenantID, issuerZone domain.BusinessTimeZone) (PreAdviceNumberIssue, error)
}

// ReturnIdentities 签发退货预报标识。
type ReturnIdentities interface {
	NextReturnAdviceID() (domain.ReturnAdviceID, error)
}
