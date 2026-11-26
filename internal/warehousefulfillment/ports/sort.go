package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// SortPut 是分拣时的一次投放：合并拣货的波次里，把这张出库请求的这种货放进它的周转箱。数量按基本单位。
type SortPut struct {
	FactID     string
	Wave       domain.WaveID
	Request    domain.OutboundRequestID
	Goods      domain.GoodsCode
	Quantity   int64
	Warehouse  domain.WarehouseCode
	Tote       domain.ToteCode
	RecordedAt time.Time
}

// SortStore 读写分拣的投放。写入在没有环境事务时拒绝。
type SortStore interface {
	// WaveSorts 回答这个波次的货要不要分拣：放出时选了合并拣货的才要。没有这个波次 found 为假。
	WaveSorts(ctx context.Context, tenant domain.TenantID, id domain.WaveID) (sorts bool, found bool, err error)
	LoadSortPut(ctx context.Context, tenant domain.TenantID, factID string) (SortPut, bool, error)
	SaveSortPut(ctx context.Context, tenant domain.TenantID, put SortPut) error
	// SortedOf 是这张请求的这种货在这个波次里已经投放了几件。
	SortedOf(ctx context.Context, tenant domain.TenantID, wave domain.WaveID, request domain.OutboundRequestID, goods domain.GoodsCode) (int64, error)
	// UnsortedOf 是这张请求在合并拣货的各个波次里拣出来（拣满或少拣的任务）而还没投放的件数。
	// 它的任务都不在合并拣货的波次里时是 0。
	UnsortedOf(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID) (int64, error)
}

// SortPlanLine 是分拣台上一张请求的一种货：拣出来几件、已经投放几件、还有几件等着拣。
type SortPlanLine struct {
	Goods  string
	Unit   string
	Picked int64
	Sorted int64
	// Open 是这种货还待执行的任务要拣的件数：拣货的人还没拿回来。
	Open int64
}

// SortPlanRequest 是分拣台上的一张出库请求，和它此刻绑着的周转箱。
type SortPlanRequest struct {
	Request        string
	DocumentNumber string
	Owner          string
	Status         string
	Totes          []string
	Lines          []SortPlanLine
}

// SortPlan 是一个合并拣货的波次此刻的分拣情况。只列还留在波次里、没有取消的出库请求：
// 撤出的请求没有拣过货，取消的请求拣出来的货走回库，都不往箱里分。
type SortPlan struct {
	Wave      string
	Warehouse string
	Status    string
	// Sorts 为假是这个波次不用分拣（逐单拣，或单品单件波次）；这时 Requests 为空。
	Sorts    bool
	Requests []SortPlanRequest
}

// SortPlans 读一个波次的分拣情况。
type SortPlans interface {
	LoadSortPlan(ctx context.Context, tenant domain.TenantID, wave string) (SortPlan, bool, error)
}
