package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// ControlReleaseRefusalKey 是释放未受理册上一笔占用的身份：与册的主键同形（租户，委托，提交版本，控制结果）。处理留痕的主键就是它，
// 一笔占用至多被处理一次。
type ControlReleaseRefusalKey struct {
	TenantID          domain.TenantID
	ShipmentRequestID domain.ShipmentRequestID
	SubmissionVersion domain.SubmissionVersionID
	ControlResultID   domain.FinancialControlResultID
}

// KeyOfRefusal 取册上一行的身份。
func KeyOfRefusal(refusal ControlReleaseRefusal) ControlReleaseRefusalKey {
	return ControlReleaseRefusalKey{
		TenantID:          refusal.TenantID,
		ShipmentRequestID: refusal.ShipmentRequestID,
		SubmissionVersion: refusal.SubmissionVersion,
		ControlResultID:   refusal.ControlResultID,
	}
}

// ControlReleaseRefusalHandlingKind 是处理一笔被拒收的释放的两种做法（封闭集）。
//
//   - `已在结算侧释放`：处理人核对后，经按原关联的释放编排再请求一次释放，提供方答已收口才算处理。
//   - `标记已处理`：资金已在本产品之外处置（例如结算侧已手工放掉），处理人只留痕：谁、凭什么、何时。本上下文不替他判这是不是真的。
type ControlReleaseRefusalHandlingKind uint8

const (
	ControlReleaseRefusalHandlingKindInvalid ControlReleaseRefusalHandlingKind = iota
	ControlReleaseRefusalReleasedAtSettlement
	ControlReleaseRefusalMarkedHandled
)

func (kind ControlReleaseRefusalHandlingKind) String() string {
	switch kind {
	case ControlReleaseRefusalReleasedAtSettlement:
		return "RELEASED_AT_SETTLEMENT"
	case ControlReleaseRefusalMarkedHandled:
		return "MARKED_HANDLED"
	default:
		return ""
	}
}

// ControlReleaseRefusalHandling 是对册上一笔占用的一次处理留痕：只追加，不改册上原行。
type ControlReleaseRefusalHandling struct {
	Key       ControlReleaseRefusalKey
	Kind      ControlReleaseRefusalHandlingKind
	Handler   domain.ControlReleaseRefusalHandlerReference
	Basis     domain.ControlReleaseRefusalHandlingBasis
	HandledAt time.Time
}

// ControlReleaseRefusalHandlingSaveOutcome 是追加一次处理留痕的封闭结果。
//
//   - `已记下`：这一次赢了，处理留痕落库；调用方同笔交续办信封。
//   - `已处理`：这一笔先被别人处理了，读回先到的那一份；调用方不交第二封信封。
//   - `册上无此笔`：册上没有这一行（或属于另一个租户），什么也没写。与「不可见」同答，不泄露别租户有没有。
type ControlReleaseRefusalHandlingSaveOutcome uint8

const (
	ControlReleaseRefusalHandlingSaveOutcomeInvalid ControlReleaseRefusalHandlingSaveOutcome = iota
	ControlReleaseRefusalHandlingRecorded
	ControlReleaseRefusalAlreadyHandled
	ControlReleaseRefusalNotRegistered
)

// ControlReleaseRefusalHandlingView 读一笔占用有没有被处理过。接受链开头放被取代各版遇确定未受理时，凭它认出「这一笔人已处理」
// 而不再上抛。读不回是错误，调用方不把它当成「没处理过」。
type ControlReleaseRefusalHandlingView interface {
	FindControlReleaseRefusalHandling(
		ctx context.Context,
		key ControlReleaseRefusalKey,
	) (ControlReleaseRefusalHandling, bool, error)
}

// ControlReleaseRefusalHandlings 是释放未受理册的处理留痕（只追加）。写口要环境事务：处理留痕与续办信封同笔落地，
// 留痕落了信封没入队，被拒收的那一笔处理完了、接受链却不会再来。
//
// 并发：两人同时处理同一笔，主键相撞，只有一笔成立；败者读回赢家的那一份，答`已处理`，不交第二封信封。
type ControlReleaseRefusalHandlings interface {
	ControlReleaseRefusalHandlingView
	HandleControlReleaseRefusal(
		ctx context.Context,
		handling ControlReleaseRefusalHandling,
	) (ControlReleaseRefusalHandlingSaveOutcome, ControlReleaseRefusalHandling, error)
}

// UnhandledControlReleaseRefusals 是一个租户册上还没有被处理的行：最早登记的在前，至多 limit 行；Count 是未处理的全部行数，
// 大于 len(Refusals) 即清单被页大小截断。
type UnhandledControlReleaseRefusals struct {
	Refusals []ControlReleaseRefusal
	Count    int
}

// ControlReleaseRefusalCatalogue 是释放未受理册的运营查阅读口：按租户列未处理的行加计数。租户条件进每条语句，否定结果不区分
// 「没有」与「属于另一个租户」。
type ControlReleaseRefusalCatalogue interface {
	ListUnhandledControlReleaseRefusals(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) (UnhandledControlReleaseRefusals, error)
}

// ControlReleaseRefusalHandledHandoffIntent 把一次处理交给接受链续办：携带处理之后读回的委托（信封要的当前提交版本与成员清单
// 都在它的当前版本上）与这一次的处理留痕（信封 ID 由它派生）。
type ControlReleaseRefusalHandledHandoffIntent struct {
	Identity domain.SourceIdentity
	Request  domain.ShipmentRequest
	Handling ControlReleaseRefusalHandling
}

// ControlReleaseRefusalHandledHandoff 把「释放拒收已处理」写入 Outbox（`OutboxControlReleaseRefusalHandledHandoff`）。信封 ID 由
// 那一笔占用的身份认领，一笔占用至多被处理一次，所以至多一封。它必须与处理留痕同一事务交出（ADR-0094 决定四：续办触发与格同笔
// 落地），任一步失败全部回滚——留痕落了而信封没入队，被拒收的那一笔处理完了，接受链那一路的新版本却一直判不了。
type ControlReleaseRefusalHandledHandoff interface {
	HandOffControlReleaseRefusalHandled(ctx context.Context, intent ControlReleaseRefusalHandledHandoffIntent) error
}
