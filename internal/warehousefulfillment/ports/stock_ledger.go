package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// StockAppendOutcome 是一笔流水在落册时的结果。业务格不用 error 表达，事务因此保持可用，与商品登记册的落点同一理由。
type StockAppendOutcome uint8

const (
	StockAppendOutcomeInvalid StockAppendOutcome = iota
	// StockAppended 是新写入的一笔。
	StockAppended
	// StockAlreadyPosted 是同一个判重位置上已经有一笔。内容同不同由调用方再读了比。
	StockAlreadyPosted
	// StockInsufficientBalance 是落册时再判，取走之后某一格的余额会成负的。这是并发下的定论。
	StockInsufficientBalance
)

func (outcome StockAppendOutcome) String() string {
	switch outcome {
	case StockAppended:
		return "APPENDED"
	case StockAlreadyPosted:
		return "ALREADY_POSTED"
	case StockInsufficientBalance:
		return "INSUFFICIENT_BALANCE"
	default:
		return ""
	}
}

// StockLedger 是库存账的写入面。余额不另存，由流水行现算（CONTEXT「库存账」）。
//
// 读不动走 error，不折成 found=false 或 0，理由同 GoodsStockLookup：0 与「没有」都是可以成立的业务答案，故障不是。
// 记数在同一个环境事务里调用串行、读取与落册；落册在没有环境事务时拒绝。
type StockLedger interface {
	// SerializePosting 把（租户，货主，仓）上的记数串行起来，并让这一笔与同一货主的商品修订分出先后。
	// 须在环境事务里调用，没有环境事务时拒绝。记数先取商品锁的共享一侧，商品登记问有没有在库库存时取独占一侧
	// （UC-WF-002「一致性、幂等与并发」）。调用方须把它排在任何读之前：读到的商品修订与余额要撑到落册。
	SerializePosting(
		ctx context.Context,
		tenant domain.TenantID,
		owner domain.OwnerAccountID,
		warehouse domain.WarehouseCode,
	) error
	// LoadMovementBySource 按判重位置（租户，原因，来源事实引用）取在册的那一笔。
	LoadMovementBySource(
		ctx context.Context,
		tenant domain.TenantID,
		reason domain.StockMovementReason,
		source domain.SourceFactReference,
	) (domain.StockMovement, bool, error)
	LoadInboundBatch(
		ctx context.Context,
		tenant domain.TenantID,
		id domain.InboundBatchID,
	) (domain.InboundBatch, bool, error)
	// CellBalance 是一格此刻的余额；这一格还没有任何流水时答 0。格里的入库批次必有，没带是调用方的错，交 error，
	// 不把「没点名批次」答成 0。读不动同样走 error：0 是这一格确实没有货。
	CellBalance(
		ctx context.Context,
		tenant domain.TenantID,
		owner domain.OwnerAccountID,
		warehouse domain.WarehouseCode,
		cell domain.StockCell,
	) (int64, error)
	// AppendMovement 落册：流水与它形成的入库批次同一笔写入。落册里再判一次判重位置与取走之后余额不为负，
	// 这是并发下的定论；应用层先查后写挡不住两个记数同时取走同一格。
	AppendMovement(
		ctx context.Context,
		movement domain.StockMovement,
		formed []domain.InboundBatch,
	) (StockAppendOutcome, error)
}

// StockIdentities 签发流水与入库批次的标识。标识只在决定落册时取，校验没过不取。
type StockIdentities interface {
	NextStockMovementID() (domain.StockMovementID, error)
	NextInboundBatchID() (domain.InboundBatchID, error)
}

// StockGoodsLookup 是记数核商品要的那一点读法。真实现是商品登记册。
// LoadLatestGoods 取入账这一刻的当前修订（包装层级与是否跟踪批号、效期）；
// LoadGoodsAt 取某一时刻生效的那一笔修订（贴标别名按事实发生时刻判）。
// 读不动走 error，不折成「没登记」：折成没登记会把一次该重试的故障答成拒绝。
type StockGoodsLookup interface {
	LoadLatestGoods(
		ctx context.Context,
		tenant domain.TenantID,
		owner domain.OwnerAccountID,
		code domain.GoodsCode,
	) (domain.GoodsRevision, bool, error)
	LoadGoodsAt(
		ctx context.Context,
		tenant domain.TenantID,
		owner domain.OwnerAccountID,
		code domain.GoodsCode,
		at time.Time,
	) (domain.GoodsRevision, bool, error)
}

// StockLocationLookup 是记数核「记入行的库位在这个仓已登记」要的那一点读法。真实现是库位登记册的 LoadLatestLocation。
// 读不动走 error，不折成「没登记」，理由同 WarehouseLookup。
type StockLocationLookup interface {
	LoadLatestLocation(
		ctx context.Context,
		tenant domain.TenantID,
		warehouse domain.WarehouseCode,
		code domain.LocationCode,
	) (domain.LocationRevision, bool, error)
}

// StockBalanceRow 是一个（货主，仓）下余额为正的一格。货主与仓在查询上，不在行上。
// 贴标别名、批号、效期可以没有：没有是 nil，不是空串——给了的取值都不是空白，空串会和「没有」撞在一起。
type StockBalanceRow struct {
	LocationCode   string
	GoodsCode      string
	Unit           string
	State          string
	Alias          *string
	InboundBatchID string
	BatchNumber    *string
	Expiry         *string
	Quantity       int64
}

// StockOnHandRead 列出一个（货主，仓）下余额为正的各格，给本段之后的用例用，不接端点。
// 读不动走 error，不折成空切片：空切片是这个货主在这个仓没有在库的货。
type StockOnHandRead interface {
	ListOnHandCells(
		ctx context.Context,
		tenant domain.TenantID,
		owner domain.OwnerAccountID,
		warehouse domain.WarehouseCode,
	) ([]StockBalanceRow, error)
}
