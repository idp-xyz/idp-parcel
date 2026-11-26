package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// AllocationStrategy 是租户采用的那一版参考分配策略。没有这一行就是没采用。
type AllocationStrategy struct {
	Revision     int
	Operator     string
	RegisteredAt time.Time
}

// PrefetchRecord 是放波次预取交出的估算委托与估算测量。打包称重与它相同时直接沿用，不同时按资料修订更正。
type PrefetchRecord struct {
	ShipmentID       string
	DeclaredParcelID string
	Weight           string
	WeightUnit       string
	Length           string
	Width            string
	Height           string
	Dimension        string
}

// PickTask 是一条已经落到库位的拣货任务。数量按基本单位。
type PickTask struct {
	ID       domain.PickTaskID
	WaveID   domain.WaveID
	Request  domain.OutboundRequestID
	Location domain.LocationCode
	Staging  domain.LocationCode
	Goods    domain.GoodsCode
	Unit     domain.GoodsUnitName
	State    domain.StockState
	Alias    domain.GoodsAliasCode
	Inbound  domain.InboundBatchID
	Quantity int64
	Picked   int64
	Status   string
	Fact     string
}

// WaveMember 是波次放出时带的一张出库请求。撤出之后成员行留着，Withdrawn 为真。
type WaveMember struct {
	Request   domain.OutboundRequestID
	Withdrawn bool
}

// SingleUnitCandidateLimit 是一次配单至多看的等复核的单品单件请求数。等得最久的先看；
// 排在这之后的要等前面的复核走了才轮得到，不会被跳过去。
const SingleUnitCandidateLimit = 500

// SingleUnitCandidate 是一张已拣完、等复核的单品单件出库请求，和它拣的那一件是哪位货主的什么货。
type SingleUnitCandidate struct {
	Request        domain.OutboundRequestID
	DocumentNumber string
	Owner          domain.OwnerAccountID
	Goods          domain.GoodsCode
}

// ShortPickDisposal 是一张请求拣不齐之后照货主的部分发货规则办的那一次。一张请求只办一次。
// Rule 是当时照的规则（少发或拆单发），Source 是它出自哪一层；规则之后改了，这一次照的仍是记下的。
// Remainder 只在拆单发时有，是没拣到的那部分拆成的请求。
type ShortPickDisposal struct {
	Request    domain.OutboundRequestID
	Rule       string
	Source     string
	Wanted     int64
	Shipped    int64
	Remainder  string
	Operator   string
	RecordedAt time.Time
}

// ShortSpot 是一个库位上的一种货：这位货主在这里有少拣留下的拣货差异还没了结。
type ShortSpot struct {
	Location domain.LocationCode
	Goods    domain.GoodsCode
}

// PickDifferenceSpot 点名一位货主在一个库位上的一种货。
type PickDifferenceSpot struct {
	Owner    domain.OwnerAccountID
	Location domain.LocationCode
	Goods    domain.GoodsCode
}

// PickDifferenceClosure 圈出一份盘点要了结的拣货差异。Owner 为空是不限货主，Goods 为空是不限商品，与盘点范围的口径相同。
// Keep 里点名的不了结：那里盘出了差额，账还没调，等那份差额批准或驳回再了结。
// Before 之后才记下的差异不了结：这份盘点开始之后才少拣的，清点未必看到了它。
type PickDifferenceClosure struct {
	Warehouse domain.WarehouseCode
	Locations []domain.LocationCode
	Owner     domain.OwnerAccountID
	Goods     []domain.GoodsCode
	Keep      []PickDifferenceSpot
	Before    time.Time
	Count     string
	At        time.Time
}

// PickDifferences 是盘点这一侧看到的拣货差异：哪些库位挂着待处理的差异，盘过之后了结它们。
type PickDifferences interface {
	ShortSpots(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode) ([]ShortSpot, error)
	ClosePickDifferences(ctx context.Context, tenant domain.TenantID, closure PickDifferenceClosure) error
}

// WaveStore 读写波次、分配策略与拣货任务。写入在没有环境事务时拒绝。
type WaveStore interface {
	ToteStore
	LoadStrategy(ctx context.Context, tenant domain.TenantID) (AllocationStrategy, bool, error)
	SaveStrategy(ctx context.Context, tenant domain.TenantID, strategy AllocationStrategy) error
	LoadWave(ctx context.Context, tenant domain.TenantID, id domain.WaveID) ([]domain.OutboundRequestID, bool, error)
	// SaveWave 的 batch 是放出时选的合并拣货：拣货的人按库位、商品把几张请求的货合在一起拿，拿回来再分拣。
	SaveWave(ctx context.Context, tenant domain.TenantID, id domain.WaveID, warehouse domain.WarehouseCode, operator domain.Registrant, prefetch, batch bool, at time.Time, requests []domain.OutboundRequestID, tasks []PickTask) error
	LoadWaveMembers(ctx context.Context, tenant domain.TenantID, id domain.WaveID) ([]WaveMember, bool, error)
	// WavePicksMerged 回答这个波次的货是不是合在一起拿的：单品单件波次，或放出时选了合并拣货的波次。
	// 这种波次拣货时不按出库请求分箱。没有这个波次答否。
	WavePicksMerged(ctx context.Context, tenant domain.TenantID, id domain.WaveID) (bool, error)
	// SingleUnitAwaitingCheck 列出这个仓里已拣完、等复核的单品单件出库请求，等得最久的在前：
	// 先按波次放出的先后，再按请求受理的先后。一次至多交回 SingleUnitCandidateLimit 张。
	SingleUnitAwaitingCheck(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode) ([]SingleUnitCandidate, error)
	// WithdrawFromWave 记下撤出、作废这张请求在这个波次里待执行的任务，并把请求从已放波次改回已预占。
	WithdrawFromWave(ctx context.Context, tenant domain.TenantID, id domain.WaveID, request domain.OutboundRequestID, operator domain.Registrant, at time.Time) error
	LoadTask(ctx context.Context, tenant domain.TenantID, id domain.PickTaskID) (PickTask, bool, error)
	SavePick(ctx context.Context, tenant domain.TenantID, task PickTask, requestStatus domain.OutboundStatus, differenceID, factID string, short int64, at time.Time) error
	// ShortSpots 列出这位货主在这个仓里还挂着待处理拣货差异的库位与商品。分配拣货任务时避开它们。
	ShortSpots(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode) ([]ShortSpot, error)
	// InsertOpenTask 往已放出的波次里添一条待执行任务：少拣之后换格补拣，或库内移库把一条任务拆成两格。
	InsertOpenTask(ctx context.Context, tenant domain.TenantID, task PickTask) error
	TasksOf(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID) ([]PickTask, error)
	LoadShortPickDisposal(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID) (ShortPickDisposal, bool, error)
	// SaveShortPickDisposal 记下这一次处置，放开这张请求没拣到的预占，并把请求从已预占改成已拣货。
	SaveShortPickDisposal(ctx context.Context, tenant domain.TenantID, disposal ShortPickDisposal) error
	SaveCheck(ctx context.Context, tenant domain.TenantID, fact domain.PickFactID, request domain.OutboundRequestID, observed int64, matched bool, at time.Time, next domain.OutboundStatus) error
	LoadCheck(ctx context.Context, tenant domain.TenantID, fact domain.PickFactID) (observed int64, matched bool, found bool, err error)
	SavePack(ctx context.Context, tenant domain.TenantID, fact domain.PickFactID, request domain.OutboundRequestID, parcels []PackParcel, at time.Time) error
	LoadPack(ctx context.Context, tenant domain.TenantID, fact domain.PickFactID) (bool, error)
	ParcelsOf(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID) ([]PackParcel, error)
	MarkShipment(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, sequence int, shipmentRequestID, declaredParcelID string) error
	LoadConfirmation(ctx context.Context, tenant domain.TenantID, factID string) (OutboundConfirmation, bool, error)
	SaveConfirmation(ctx context.Context, tenant domain.TenantID, confirmation OutboundConfirmation) error
	ParcelConfirmed(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, sequence int) (bool, error)
	LoadOwnerLabel(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, sequence int) (string, bool, error)
	SaveOwnerLabel(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, sequence int, externalNumber string) error
	OwnerLabelsOf(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID) (map[int]string, error)
	LoadCorrection(ctx context.Context, tenant domain.TenantID, factID string) (ConfirmationCorrection, bool, error)
	SaveCorrection(ctx context.Context, tenant domain.TenantID, correction ConfirmationCorrection) error
	PostReplenishment(ctx context.Context, tenant domain.TenantID, factID string, approver domain.Registrant, dutyRevision int, at time.Time) error
	LoadCutoff(ctx context.Context, tenant domain.TenantID, key string) (requestID string, sequence int, found bool, err error)
	SaveCutoff(ctx context.Context, tenant domain.TenantID, key, requestID string, sequence int, basis, outcome string, at time.Time) error
	ReleaseReservations(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID) error
	CancelOpenTasks(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID) error
	SaveReturnTask(ctx context.Context, tenant domain.TenantID, task ReturnTask) error
	SetRequestStatus(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, status domain.OutboundStatus) error
	SavePrefetch(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, record PrefetchRecord) error
	LoadPrefetch(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID) (PrefetchRecord, bool, error)
	MarkParcelCutoff(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, sequence int) (domain.OutboundStatus, error)
	SaveLatePick(ctx context.Context, tenant domain.TenantID, task PickTask, factID string) error
	LoadReturnTask(ctx context.Context, tenant domain.TenantID, id string) (ReturnTask, bool, error)
	CompleteReturn(ctx context.Context, tenant domain.TenantID, id, factID string) error
	OpenReturns(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID) (int, error)
}

// ReturnTask 是截单后把已拣出的货移回可分配库区的任务。货还在待出库时它只是任务。
type ReturnTask struct {
	ID       string
	Request  domain.OutboundRequestID
	Sequence int
	Location domain.LocationCode
	Goods    domain.GoodsCode
	Unit     domain.GoodsUnitName
	State    domain.StockState
	Inbound  domain.InboundBatchID
	Quantity int64
	Status   string
	Fact     string
}

// ConfirmationCorrection 是对一版出库确认的一次更正。一版只有这一次。
type ConfirmationCorrection struct {
	FactID       string
	Kind         string
	OccurredAt   time.Time
	Operator     string
	Status       string
	DutyRevision int
	Approver     string
}

// OutboundConfirmation 是一次已经记下的出库确认或自提交付。
type OutboundConfirmation struct {
	FactID           string
	Request          domain.OutboundRequestID
	Sequence         int
	Kind             string
	Offline          bool
	Missing          string
	ExternalNumber   string
	PickupParty      string
	PickupCredential string
	OccurredAt       time.Time
	State            string
}

// PackParcel 是一个包裹的装箱记录。重量和尺寸按货主交来的字面留下。
type PackParcel struct {
	Sequence          int
	Packaging         string
	Weight            string
	WeightUnit        string
	Length            string
	Width             string
	Height            string
	Dimension         string
	Goods             domain.GoodsCode
	Unit              domain.GoodsUnitName
	Quantity          int64
	ShipmentRequestID string
	DeclaredParcelID  string
	CommissionState   string
	CorrectionState   string
}
