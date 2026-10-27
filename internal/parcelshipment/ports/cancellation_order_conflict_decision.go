package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件是取消先后冲突的解开决定面要读、要记与要交出的口（ADR-0295）：冲突册（CancellationOrderConflictRegister）只记下冲突，解开它的是人形成的
// 一份明确决定；决定只追加，不改冲突行，也不改写已提交的取消。

// CancellationOrderConflictKey 是册上一份冲突的身份：与册的主键同形（租户，包裹，边界种类，边界那一代，取消）。决定的主键就是它，
// 一份冲突至多被决定一次。
type CancellationOrderConflictKey struct {
	TenantID     domain.TenantID
	Parcel       domain.DeclaredParcelID
	BoundaryKind domain.CancellationBoundaryKind
	BoundaryRef  domain.CancellationBoundaryReference
	Cancellation domain.ParcelCancellationID
}

// KeyOfConflictEntry 取册上一行的身份。
func KeyOfConflictEntry(entry CancellationOrderConflictEntry) CancellationOrderConflictKey {
	return CancellationOrderConflictKey{
		TenantID:     entry.TenantID,
		Parcel:       entry.Parcel,
		BoundaryKind: entry.Boundary.Kind,
		BoundaryRef:  entry.Boundary.Reference,
		Cancellation: entry.Cancellation,
	}
}

// KeyOfConflict 取刚判出的一份冲突的身份：收寄采用与面单终局判断凭它问这份冲突有没有被决定过。
func KeyOfConflict(tenant domain.TenantID, conflict domain.CancellationOrderConflict) CancellationOrderConflictKey {
	return CancellationOrderConflictKey{
		TenantID:     tenant,
		Parcel:       conflict.Parcel(),
		BoundaryKind: conflict.Boundary().Kind,
		BoundaryRef:  conflict.Boundary().Reference,
		Cancellation: conflict.Cancellation(),
	}
}

// CancellationOrderConflictDecision 是对册上一份冲突的一次决定：只追加。
type CancellationOrderConflictDecision struct {
	Key       CancellationOrderConflictKey
	Kind      domain.CancellationOrderConflictDecisionKind
	Decider   domain.CancellationOrderConflictDeciderReference
	Basis     domain.CancellationOrderConflictDecisionBasis
	DecidedAt time.Time
}

// CancellationOrderConflictDecisionSaveOutcome 是追加一次决定的封闭结果。
//
//   - `已记下`：这一次赢了，决定落库；调用方同笔交续办信封（认定取消不成立时）。
//   - `已决定`：这份冲突先被别人决定了，读回先到的那一份；调用方不交第二封信封。
//   - `册上无此份`：册上没有这份冲突（或属于另一个租户），什么也没写。与「不可见」同答，不泄露别租户有没有。
type CancellationOrderConflictDecisionSaveOutcome uint8

const (
	CancellationOrderConflictDecisionSaveOutcomeInvalid CancellationOrderConflictDecisionSaveOutcome = iota
	CancellationOrderConflictDecisionRecorded
	CancellationOrderConflictAlreadyDecided
	CancellationOrderConflictNotRegistered
)

// CancellationOrderConflictDecisionView 读一份冲突有没有被决定过。收寄采用与面单终局判断在冲突那一格凭它认出「这一代边界事实已被交来源方更正」。
// 读不回是错误，调用方不把它当成「没决定过」。
type CancellationOrderConflictDecisionView interface {
	FindConflictDecision(
		ctx context.Context,
		key CancellationOrderConflictKey,
	) (CancellationOrderConflictDecision, bool, error)
}

// CancellationOrderConflictDecisions 是冲突的决定留痕（只追加）。写口要环境事务：决定留痕、作废与续办信封同笔落地，留痕落了信封没入队，
// 被认定不成立的取消就没有人去重驱被它挡住的来源。
//
// 并发：两人同时决定同一份冲突，主键相撞，只有一笔成立；败者读回赢家的那一份，答`已决定`，不交第二封信封。
type CancellationOrderConflictDecisions interface {
	CancellationOrderConflictDecisionView
	DecideCancellationOrderConflict(
		ctx context.Context,
		decision CancellationOrderConflictDecision,
	) (CancellationOrderConflictDecisionSaveOutcome, CancellationOrderConflictDecision, error)
}

// UnresolvedCancellationOrderConflicts 是一个租户册上还没有被决定的冲突：最早发现的在前，至多 limit 行；Count 是未解开的全部行数，
// 大于 len(Conflicts) 即清单被页大小截断。
type UnresolvedCancellationOrderConflicts struct {
	Conflicts []CancellationOrderConflictEntry
	Count     int
}

// CancellationOrderConflictCatalogue 是冲突册的运营查阅读口：按租户列未解开的行加计数。租户条件进每条语句，否定结果不区分「没有」与
// 「属于另一个租户」。
type CancellationOrderConflictCatalogue interface {
	ListUnresolvedCancellationOrderConflicts(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) (UnresolvedCancellationOrderConflicts, error)
}

// CancellationRefusedFinalVoider 作废终局采用账上依据一份取消的不采用记录。网络服务履约结果（有效交付）在取消在册时到达，终局采用答
// `来源不采用`、依据 `PARCEL_CANCELLATION/<取消>`；那条记录按（包裹，结果种类，来源版本）永久保全，同一版本重投只会读回它。取消被认定不成立
// 之后，依据它的记录不再成立，必须作废，被它挡住的来源才能重新进入终局规则判断。
//
// 只作废依据恰是这份取消的不采用行；已形成的终局、依据别的东西的不采用行一概不碰。返回作废的行数。写口要环境事务，与决定留痕同笔。
type CancellationRefusedFinalVoider interface {
	VoidCancellationRefusedFinals(
		ctx context.Context,
		tenant domain.TenantID,
		parcel domain.DeclaredParcelID,
		cancellation domain.ParcelCancellationID,
	) (int, error)
}

// CancellationNotUpheldHandoffIntent 把一次「认定取消依据不成立」的决定交给续办：携带决定本身（信封 ID 与载荷都由冲突的身份派生）。
type CancellationNotUpheldHandoffIntent struct {
	Decision CancellationOrderConflictDecision
}

// CancellationNotUpheldHandoff 把「取消被认定不成立」写入 Outbox。信封 ID 由冲突的身份认领，一份冲突至多被决定一次，所以至多一封。它必须与
// 决定留痕同一事务交出（ADR-0094 决定四：续办触发与格同笔落地），任一步失败全部回滚。
type CancellationNotUpheldHandoff interface {
	HandOffCancellationNotUpheld(ctx context.Context, intent CancellationNotUpheldHandoffIntent) error
}

// CancellationBlockedReplay 是一份被取消挡住的来源回放所需的原料：二居其一。Intake 是收寄采用判出冲突时的来源规格（网络服务），Final
// 是终局采用因取消不采用时的责任结果规格。回放时目标委托取回放那一刻的当前已接受委托，不存当时的——被挡住期间委托可以换版。
type CancellationBlockedReplay struct {
	Cancellation domain.ParcelCancellationID
	Intake       *domain.IntakeSourceSpec
	Final        *domain.ResponsibilityOutcomeSpec
}

// CancellationBlockedReplays 按（租户，包裹，取消）读回被这份取消挡住的来源回放原料，最早记下的在前。续办凭它把来源重新交给原来的编排。
type CancellationBlockedReplays interface {
	ListCancellationBlockedReplays(
		ctx context.Context,
		tenant domain.TenantID,
		parcel domain.DeclaredParcelID,
		cancellation domain.ParcelCancellationID,
	) ([]CancellationBlockedReplay, error)
}
