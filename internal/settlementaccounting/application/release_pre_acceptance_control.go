package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// ReleaseOutcome 是释放请求的应用处理结果。与施加侧同一条纪律：没有一个取值是接受判决，
// 释放哪一笔由原业务关联认领，是否据此推进委托由 parcel-shipment 决定。
//
// `无可释放`是业务答案而不是错误：这个关联下从未占用过资金（冻结从未形成，或只形成过
// `业务限制`而限制不入账本），重试一万次也不会长出一笔冻结来；把它报成故障，补偿续办会
// 对着一笔不存在的占用无休止重试。
type ReleaseOutcome uint8

const (
	ReleaseOutcomeInvalid ReleaseOutcome = iota
	ControlReleased
	NothingToRelease
	ReleaseRequestNotAccepted
	ReleaseNotFormed
)

func (outcome ReleaseOutcome) String() string {
	switch outcome {
	case ControlReleased:
		return "CONTROL_RELEASED"
	case NothingToRelease:
		return "NOTHING_TO_RELEASE"
	case ReleaseRequestNotAccepted:
		return "RELEASE_REQUEST_NOT_ACCEPTED"
	case ReleaseNotFormed:
		return "RELEASE_NOT_FORMED"
	default:
		return ""
	}
}

// ReleasePreAcceptanceControlCommand 按原控制请求身份认领要释放的冻结。它不带 FreezeID
// ——那是本账本签发的内部编号，不随控制结果离开本上下文重建；也不带金额——释放多少由
// 原冻结记录回答，让调用方报一个数就是让它有机会报错一个数。
type ReleasePreAcceptanceControlCommand struct {
	TenantID  domain.TenantID
	RequestID domain.ControlRequestID
	Scope     domain.SettlementScope
}

func (command ReleasePreAcceptanceControlCommand) minimumIdentityEstablished() bool {
	return command.TenantID.String() != "" &&
		command.RequestID.String() != "" &&
		command.Scope.LegalEntity().String() != "" &&
		command.Scope.Account().String() != "" &&
		command.Scope.Currency().String() != ""
}

type ReleasePreAcceptanceControlResult struct {
	outcome      ReleaseOutcome
	freeze       domain.FundsFreeze
	hasFreeze    bool
	reason       NotFormedReason
	continuation ContinuationReference
}

func (result ReleasePreAcceptanceControlResult) Outcome() ReleaseOutcome {
	return result.outcome
}

// Freeze 只在`已释放`时给出，携带原金额、原冻结时间与释放时间——审计要的正是这三样。
// `无可释放`一律不带：交回一个零值冻结，对账会把它当成一笔被放掉的占用。
func (result ReleasePreAcceptanceControlResult) Freeze() (domain.FundsFreeze, bool) {
	return result.freeze, result.hasFreeze
}

func (result ReleasePreAcceptanceControlResult) NotFormedReason() NotFormedReason {
	return result.reason
}

func (result ReleasePreAcceptanceControlResult) ContinuationReference() ContinuationReference {
	return result.continuation
}

type ReleasePreAcceptanceControlHandler struct {
	ledger ports.FreezeLedgerRepository
	clock  ports.Clock
}

func NewReleasePreAcceptanceControlHandler(
	ledger ports.FreezeLedgerRepository,
	clock ports.Clock,
) *ReleasePreAcceptanceControlHandler {
	return &ReleasePreAcceptanceControlHandler{ledger: ledger, clock: clock}
}

// Handle 按原关联释放一笔接受前资金冻结。重复释放返回与首次相同的答案（含原释放时间），
// 幂等由领域账本承担，本编排不换答案。
func (handler *ReleasePreAcceptanceControlHandler) Handle(
	ctx context.Context,
	command ReleasePreAcceptanceControlCommand,
) (ReleasePreAcceptanceControlResult, error) {
	// 先判身份再读账本：一次已经发出的读取收不回来，它本身就回答了这个租户、这个作用域
	// 存不存在。
	if !command.minimumIdentityEstablished() {
		return ReleasePreAcceptanceControlResult{outcome: ReleaseRequestNotAccepted}, nil
	}

	ledger, err := handler.ledger.LoadForScope(ctx, command.TenantID, command.Scope)
	if err != nil || ledger == nil {
		return handler.notFormed(command, FreezeLedgerUnavailable), nil
	}

	freeze, found := ledger.FindByRequest(command.RequestID)
	if !found {
		return ReleasePreAcceptanceControlResult{outcome: NothingToRelease}, nil
	}

	released, err := ledger.Release(freeze.FreezeID(), handler.clock.Now())
	if err != nil {
		// 刚找到的冻结释放不了，只剩编程错误或时钟异常（释放时刻早于冻结时刻）。两者都
		// 不是调用方能据以行动的业务答案，上抛。
		return ReleasePreAcceptanceControlResult{}, fmt.Errorf("release freeze: %w", err)
	}

	if err := handler.ledger.Save(ctx, command.TenantID, command.Scope, ledger); err != nil {
		// 释放没落库就不算释放。交回`已释放`，对账会按一笔其实还占着的资金收口。
		return handler.notFormed(command, FreezeLedgerUnavailable), nil
	}

	return ReleasePreAcceptanceControlResult{
		outcome:   ControlReleased,
		freeze:    released,
		hasFreeze: true,
	}, nil
}

func (handler *ReleasePreAcceptanceControlHandler) notFormed(
	command ReleasePreAcceptanceControlCommand,
	reason NotFormedReason,
) ReleasePreAcceptanceControlResult {
	return ReleasePreAcceptanceControlResult{
		outcome:      ReleaseNotFormed,
		reason:       reason,
		continuation: releaseContinuationFor(command, reason),
	}
}

// releaseContinuationFor 由请求身份、作用域与原因共同派生，与施加侧的派生纪律一致：同一
// 请求因同一原因停滞时拿到的引用始终相同。
func releaseContinuationFor(command ReleasePreAcceptanceControlCommand, reason NotFormedReason) ContinuationReference {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		reason.String(),
		command.TenantID.String(),
		command.RequestID.String(),
		command.Scope.LegalEntity().String(),
		command.Scope.Account().String(),
		command.Scope.Currency().String(),
	}, "\x00")))
	return ContinuationReference{value: "CONT-" + hex.EncodeToString(digest[:8])}
}
