package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件是取消边界要读、要记与要排队的口（PS CONTEXT 取消一条，ADR-0254）：提交边界上读面单渠道服务的
// 实际承运商首次有效收寄；取消已提交之后才到的边界事实与取消冲突时，把冲突留在册上；越过同一包裹取消边界
// 的几处判断按包裹排队。

// CancellationBoundaryLock 让同一（租户，包裹）上越过取消边界的几处判断一次只走一路：取消的提交、网络服务的
// 收寄采用、面单渠道服务的终局判断、网络服务履约结果（有效交付等）的终局采用，以及取消先后冲突的解开决定与它的续办
// （ADR-0295：决定改的是「这件包裹有没有有效取消」，那几处正是在锁里读它）。锁在当前事务里取、随事务结束放。
//
// 各处在自己的事务里读对方写的东西、写自己的：取消读当前有效终局与采用链，采用、终局判断与终局采用读已成立的
// 取消。读已提交下两笔并发、都在对方提交之前读，各自看到「对方不在册」，取消与责任起点（或非取消终局）就同时
// 成立；此后两边都已越过提交边界，没有哪封信会再比一次，ADR-0254 决定二的冲突那一格永远不被触发。取消、采用、
// 终局三张表不共一个唯一约束，挡不住这一格。锁要先于各自提交边界上的读、且与写入同一笔事务：后到的那一路才读得到
// 先到那一路已提交的写入，落进决定二的某一格。面单渠道服务的终局判断经终局采用落终局，同一事务里取两次是重入。
//
// 锁序：消费门认领 inbox 行（仅 dispatch）→ 本锁 → 本笔行写入（取消、采用、冲突册、决定留痕、终局各表）→ outbox 插入。本锁之前只有 inbox
// 认领与读已提交下的普通读；本锁之后不再取别的锁（不更新委托行，不取接受链的作用域锁与来源身份锁），一笔事务至多持一个本锁键，
// 所以各处之间、与持那几把锁的事务之间都不成环。往本锁之后加任何一把锁，先回头核这条全序。
type CancellationBoundaryLock interface {
	LockCancellationBoundary(ctx context.Context, tenant domain.TenantID, parcel domain.DeclaredParcelID) error
}

// CarrierPickupBoundaryState 是取消提交边界上读到的该包裹首次有效收寄链尾，封闭三格。链尾失效即
// 该对象当前无收寄（ADR-0135 决定六），与从未有过同答`没有`。
type CarrierPickupBoundaryState uint8

const (
	CarrierPickupBoundaryStateInvalid CarrierPickupBoundaryState = iota
	// CarrierPickupBoundaryAbsent：没有收寄，或链尾已失效。
	CarrierPickupBoundaryAbsent
	// CarrierPickupBoundaryFormed：已形成（首登或替代版本）——边界已越过。
	CarrierPickupBoundaryFormed
	// CarrierPickupBoundaryPending：判过了但待确认。它不构成收寄、不结束取消权，可也不能当作没有：
	// UC-PS-006 决定矩阵「收寄事实存在但有效性待确认 → 取消/处置未决」。
	CarrierPickupBoundaryPending
)

func (state CarrierPickupBoundaryState) String() string {
	switch state {
	case CarrierPickupBoundaryAbsent:
		return "ABSENT"
	case CarrierPickupBoundaryFormed:
		return "FORMED"
	case CarrierPickupBoundaryPending:
		return "PENDING"
	default:
		return ""
	}
}

// CarrierPickupBoundary 是读口的答复。Pickup 只在已形成时给出（事实、版本与业务发生时间）。
type CarrierPickupBoundary struct {
	State  CarrierPickupBoundaryState
	Pickup domain.CarrierFirstEffectivePickupSpec
}

// CarrierPickupBoundaryView 按（租户，包裹）读该包裹的实际承运商首次有效收寄链尾。事实由
// transport-fulfillment 拥有，适配器落在本上下文的 adapters/ 下（ADR-0025 消费方侧）；本上下文不判
// 「这条证据算不算收寄」，只读 TF 判过的结果。
type CarrierPickupBoundaryView interface {
	FindCurrentCarrierPickup(
		ctx context.Context,
		tenant domain.TenantID,
		parcel domain.DeclaredParcelID,
	) (CarrierPickupBoundary, error)
}

// CancellationOrderConflictRecord 是册上的一行：哪个租户、哪份冲突、何时发现。发现时间是本上下文
// 记下它的时刻，不参与比较——比较只用冲突里的两个业务时间。
//
// Replay 是被这份冲突挡住的来源的回放原料（收寄采用带来源规格）；面单渠道服务的收寄由 TF 的首次有效收寄链尾现读，不带。带上时与冲突
// 行同笔落地：冲突入了账却没有回放原料，认定取消不成立之后就没有东西可重驱。
type CancellationOrderConflictRecord struct {
	TenantID   domain.TenantID
	Conflict   domain.CancellationOrderConflict
	DetectedAt time.Time
	Replay     *CancellationBlockedReplay
}

// CancellationOrderConflictEntry 是读回的一行。读侧拆成平铺字段而不交回领域冲突：冲突只由判法
// 形成，读回的是已记下的数据，不再过一次判断。
type CancellationOrderConflictEntry struct {
	TenantID     domain.TenantID
	Parcel       domain.DeclaredParcelID
	Cancellation domain.ParcelCancellationID
	CancelledAt  time.Time
	Boundary     domain.CancellationBoundaryFact
	DetectedAt   time.Time
}

type CancellationOrderConflictSaveOutcome uint8

const (
	CancellationOrderConflictSaveOutcomeInvalid CancellationOrderConflictSaveOutcome = iota
	CancellationOrderConflictRecorded
	// CancellationOrderConflictAlreadyRecorded：同一份取消对同一代边界事实已有记录。同一来源重投会
	// 再判出同一份冲突，它不是第二份。
	CancellationOrderConflictAlreadyRecorded
)

// CancellationOrderConflictRegister 是取消先后冲突的只追加登记册（ADR-0254 决定二「消费门入账并
// 留下冲突」）。冲突重投推不动，消费门据它入账；册上那一行让冲突按包裹查得出、交人，解开它的
// 业务决定日后从这里取。写口要环境事务：冲突与消费入账同笔落地，入账了冲突却没记下就丢了它。
type CancellationOrderConflictRegister interface {
	Record(ctx context.Context, record CancellationOrderConflictRecord) (CancellationOrderConflictSaveOutcome, error)
	ListByParcel(
		ctx context.Context,
		tenant domain.TenantID,
		parcel domain.DeclaredParcelID,
	) ([]CancellationOrderConflictEntry, error)
}
