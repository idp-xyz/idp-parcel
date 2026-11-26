package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// Tote 是一个登记过的周转箱。Retired 为真是已停用：不再绑新的出库请求，过去绑过的照旧查得到。
type Tote struct {
	Warehouse domain.WarehouseCode
	Code      domain.ToteCode
	Retired   bool
}

// ToteStore 读写周转箱名册。写入在没有环境事务时拒绝。
type ToteStore interface {
	LoadTote(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, code domain.ToteCode) (Tote, bool, error)
	// RegisterTote 登记一个箱号：没有就新登，停用的恢复在用。已经在用的不动，交回 false。
	RegisterTote(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, code domain.ToteCode, operator domain.Registrant, at time.Time) (bool, error)
	// RetireTote 停用一个在用的箱。箱上还绑着出库请求时整份不成立。
	RetireTote(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, code domain.ToteCode, operator domain.Registrant, at time.Time) error
	// ToteHolder 交回这个箱此刻绑着的出库请求；没绑着交回 false。
	ToteHolder(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, code domain.ToteCode) (domain.OutboundRequestID, bool, error)
	// BindTote 把一个箱绑到一张出库请求上。箱没登记、已停用、绑着别的请求时不绑，照实交回是哪一种；
	// 已经绑着这张请求的照旧。
	BindTote(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, code domain.ToteCode, request domain.OutboundRequestID, at time.Time) (ToteBinding, error)
	// ReleaseTotes 放开一张出库请求绑着的全部箱：它的货不在箱里了。
	ReleaseTotes(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, at time.Time) error
	// ReleaseTote 由人放开一个箱，记下是谁放的。箱没绑着交回 false。
	ReleaseTote(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, code domain.ToteCode, operator domain.Registrant, at time.Time) (bool, error)
}

// ToteBinding 是绑一个箱的结果。
type ToteBinding uint8

const (
	// ToteBindingInvalid 不是一种结果，只在出错时交回。
	ToteBindingInvalid ToteBinding = iota
	// ToteBound 是这一次绑上的。
	ToteBound
	// ToteAlreadyBound 是这个箱早就绑着这张请求。
	ToteAlreadyBound
	// ToteBoundElsewhere 是这个箱绑着别的出库请求。
	ToteBoundElsewhere
	// ToteUnregistered 是这个仓没有登记过这个箱号。
	ToteUnregistered
	// ToteOutOfService 是这个箱已停用。
	ToteOutOfService
)

// 周转箱在列表上的三种状态：空着、绑着一张出库请求、已停用。
const (
	ToteFree    = "FREE"
	ToteInUse   = "IN_USE"
	ToteRetired = "RETIRED"
)

// TotePages 是运营查阅周转箱名册的查询声明（ADR-0144）。筛选维覆盖仓与状态；q 覆盖箱号与绑着的出库请求。
// 按箱号排：找一个箱是按号找的，登记的先后没有用处。
var TotePages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "warehouse-fulfillment-totes",
	Sorts: []cataloguepage.SortDimension{
		{Name: "toteCode", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "toteCode"},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text, cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "warehouseCode"},
		{Name: "state", Vocabulary: []string{ToteFree, ToteInUse, ToteRetired}},
	},
})

// ToteRow 是周转箱列表的一行：这个箱此刻空着、绑着哪张出库请求，还是已停用。
type ToteRow struct {
	WarehouseCode string
	ToteCode      string
	State         string
	RequestID     string
	BoundAt       *time.Time
	RegisteredBy  string
	RegisteredAt  time.Time
	RetiredBy     string
	RetiredAt     *time.Time
}
