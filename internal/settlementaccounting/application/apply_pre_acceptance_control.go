// Package application 在 settlement-accounting 领域内核与本上下文自有端口之上编排用例。
// 它不含持久化、事务或事件机制，那些仍阻断在 Bento 闸门之后（ADR-0017）。
package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// ControlOutcome 是本用例接受前财务控制分支的应用处理结果。没有一个取值是接受判决：
// CONTEXT 把这条写死——本上下文形成的是估价、冻结、信用暴露或业务限制，是否据此阻断
// 委托由 `parcel-shipment` 决定，本上下文不得自行增设接单门槛。
//
// `已执行`与`业务限制`共用 `ControlApplied`：余额不足同样是一次已经执行过的控制，它的
// 结论装在 `FundsFreeze` 的状态里。分成两个应用结果会让「控制执行过没有」这个问题得靠
// 两处判断回答。
type ControlOutcome uint8

const (
	ControlOutcomeInvalid ControlOutcome = iota
	ControlApplied
	ControlNotApplicable
	ControlRequestConflict
	ControlRequestNotAccepted
	ControlNotFormed
)

func (outcome ControlOutcome) String() string {
	switch outcome {
	case ControlApplied:
		return "CONTROL_APPLIED"
	case ControlNotApplicable:
		return "CONTROL_NOT_APPLICABLE"
	case ControlRequestConflict:
		return "CONTROL_REQUEST_CONFLICT"
	case ControlRequestNotAccepted:
		return "CONTROL_REQUEST_NOT_ACCEPTED"
	case ControlNotFormed:
		return "CONTROL_NOT_FORMED"
	default:
		return ""
	}
}

// NotFormedReason 指名本次为何没有形成控制结果。它是封闭集合而非自由文本，这样待判断才能
// 按依赖阶段分类统计；取值与产生它的那条路径同时出现。
type NotFormedReason uint8

const (
	NotFormedReasonNone NotFormedReason = iota
	ControlPolicyUnavailable
	BalanceUnavailable
	FreezeLedgerUnavailable
)

func (reason NotFormedReason) String() string {
	switch reason {
	case ControlPolicyUnavailable:
		return "CONTROL_POLICY_UNAVAILABLE"
	case BalanceUnavailable:
		return "BALANCE_UNAVAILABLE"
	case FreezeLedgerUnavailable:
		return "FREEZE_LEDGER_UNAVAILABLE"
	default:
		return ""
	}
}

// ContinuationReference 让调用方把停下的控制请求重新接上。它属应用层而不属领域：领域拥有
// 冻结与限制，不认识依赖失败这回事。
type ContinuationReference struct{ value string }

func (reference ContinuationReference) String() string {
	return reference.value
}

type ApplyPreAcceptanceControlCommand struct {
	TenantID    domain.TenantID
	RequestID   domain.ControlRequestID
	Scope       domain.SettlementScope
	AmountMinor int64
	Association domain.BusinessAssociationReference
	AsOf        domain.ControlAsOf
}

// minimumIdentityEstablished 在读任何权威之前判断该不该读。金额算在受理条件里而不留给
// 领域构造期：非正数金额的请求根本不该去读这个客户的余额，而一次已经发出的读取收不回来。
func (command ApplyPreAcceptanceControlCommand) minimumIdentityEstablished() bool {
	return command.TenantID.String() != "" &&
		command.RequestID.String() != "" &&
		command.Scope.LegalEntity().String() != "" &&
		command.Scope.Account().String() != "" &&
		command.Scope.Currency().String() != "" &&
		command.Association.String() != "" &&
		command.AsOf.Valid() &&
		command.AmountMinor > 0
}

type ApplyPreAcceptanceControlResult struct {
	outcome      ControlOutcome
	freeze       domain.FundsFreeze
	hasFreeze    bool
	controlledAt time.Time
	asOf         domain.ControlAsOf
	basis        domain.ControlBasisReference
	reason       NotFormedReason
	continuation ContinuationReference
}

func (result ApplyPreAcceptanceControlResult) Outcome() ControlOutcome {
	return result.outcome
}

// Freeze 只在控制实际执行过时给出，`无控制`、冲突、未受理与待判断一律没有。交回一个零值
// 冻结，正是 CONTEXT 禁止的「用虚假冻结冒充控制」。
func (result ApplyPreAcceptanceControlResult) Freeze() (domain.FundsFreeze, bool) {
	return result.freeze, result.hasFreeze
}

// ControlledAt 是本次控制作出的时间，与 AsOf 分开：`asOf` 决定按哪一版策略判断，控制时间
// 说明资金何时被占用。压成一个会让重放看起来像一次新的控制。
func (result ApplyPreAcceptanceControlResult) ControlledAt() time.Time {
	return result.controlledAt
}

func (result ApplyPreAcceptanceControlResult) AsOf() domain.ControlAsOf {
	return result.asOf
}

// ControlBasis 只在`无控制`时给出。CONTEXT 要求这个结果携带商业不适用依据——没有依据的
// 「无控制」看起来像信用通过，而它其实是一次未执行的控制。
func (result ApplyPreAcceptanceControlResult) ControlBasis() domain.ControlBasisReference {
	return result.basis
}

func (result ApplyPreAcceptanceControlResult) NotFormedReason() NotFormedReason {
	return result.reason
}

func (result ApplyPreAcceptanceControlResult) ContinuationReference() ContinuationReference {
	return result.continuation
}

type ApplyPreAcceptanceControlHandler struct {
	policy  ports.PreAcceptanceControlPolicyView
	balance ports.OperationalBalanceView
	ledger  ports.FreezeLedgerRepository
	clock   ports.Clock
}

func NewApplyPreAcceptanceControlHandler(
	policy ports.PreAcceptanceControlPolicyView,
	balance ports.OperationalBalanceView,
	ledger ports.FreezeLedgerRepository,
	clock ports.Clock,
) *ApplyPreAcceptanceControlHandler {
	return &ApplyPreAcceptanceControlHandler{
		policy:  policy,
		balance: balance,
		ledger:  ledger,
		clock:   clock,
	}
}

// Handle 为一次委托接受前的财务控制请求占用资金。它不形成委托接受或拒绝：余额不足是本
// 上下文报告的业务限制，是否据此阻断委托由 parcel-shipment 决定。
func (handler *ApplyPreAcceptanceControlHandler) Handle(
	ctx context.Context,
	command ApplyPreAcceptanceControlCommand,
) (ApplyPreAcceptanceControlResult, error) {
	if !command.minimumIdentityEstablished() {
		return ApplyPreAcceptanceControlResult{outcome: ControlRequestNotAccepted}, nil
	}

	// 控制策略先于余额与登记册。合同规定本范围无财务控制时，连读余额都不该发生——那次
	// 读取既是白做的，也已经取了这个客户的资金状况。
	policy, err := handler.policy.LoadControlPolicy(ctx, command.TenantID, command.Scope)
	if err != nil {
		// 商业侧调不通形成待判断，不读成「不要求控制」。后者正是 CONTEXT 禁止的默认信用
		// 通过：一次商业故障会因此变成一个看起来通过了的接受前控制。
		return handler.notFormed(command, ControlPolicyUnavailable), nil
	}
	if !policy.ControlRequired() {
		return ApplyPreAcceptanceControlResult{
			outcome: ControlNotApplicable,
			asOf:    command.AsOf,
			basis:   policy.Basis(),
		}, nil
	}

	balance, err := handler.balance.LoadBalance(ctx, command.TenantID, command.Scope)
	if err != nil {
		return handler.notFormed(command, BalanceUnavailable), nil
	}

	ledger, err := handler.ledger.LoadForScope(ctx, command.TenantID, command.Scope)
	if err != nil || ledger == nil {
		return handler.notFormed(command, FreezeLedgerUnavailable), nil
	}

	controlledAt := handler.clock.Now()
	request, err := domain.NewFreezeRequest(
		command.RequestID,
		command.Scope,
		command.AmountMinor,
		command.Association,
		controlledAt,
	)
	if err != nil {
		return ApplyPreAcceptanceControlResult{outcome: ControlRequestNotAccepted}, nil
	}

	freeze, err := ledger.Freeze(request, balance)
	if err != nil {
		// 请求冲突是业务答案而非技术故障：调用方必须能据以纠正，而不是当作故障重试。
		// 作用域错配则是编程错误，它意味着取回的余额根本不属于这个请求，必须上抛。
		if errors.Is(err, domain.ErrControlRequestConflict) {
			return ApplyPreAcceptanceControlResult{outcome: ControlRequestConflict}, nil
		}
		return ApplyPreAcceptanceControlResult{}, fmt.Errorf("freeze funds: %w", err)
	}

	if err := handler.ledger.Save(ctx, command.TenantID, command.Scope, ledger); err != nil {
		// 控制没能落库就不算执行过。交回一个未落库的冻结，下游会引用一笔查不回来的占用。
		return handler.notFormed(command, FreezeLedgerUnavailable), nil
	}

	return ApplyPreAcceptanceControlResult{
		outcome:      ControlApplied,
		freeze:       freeze,
		hasFreeze:    true,
		controlledAt: controlledAt,
		asOf:         command.AsOf,
	}, nil
}

// notFormed 构造所有待判断共用的那一种形状，让它们全部带上原因与续办引用：一次停下的控制
// 请求能不能接回去，不该取决于它停在哪一步。
func (handler *ApplyPreAcceptanceControlHandler) notFormed(
	command ApplyPreAcceptanceControlCommand,
	reason NotFormedReason,
) ApplyPreAcceptanceControlResult {
	return ApplyPreAcceptanceControlResult{
		outcome:      ControlNotFormed,
		asOf:         command.AsOf,
		reason:       reason,
		continuation: continuationFor(command, reason),
	}
}

// continuationFor 由请求身份、作用域、金额与原因共同派生，因此同一请求因同一原因停滞时拿到
// 的引用始终相同——这正是调用方能查询原次尝试而不必靠猜的原因。
func continuationFor(command ApplyPreAcceptanceControlCommand, reason NotFormedReason) ContinuationReference {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		reason.String(),
		command.TenantID.String(),
		command.RequestID.String(),
		command.Scope.LegalEntity().String(),
		command.Scope.Account().String(),
		command.Scope.Currency().String(),
		fmt.Sprint(command.AmountMinor),
		command.Association.String(),
		command.AsOf.Semantic().String(),
		command.AsOf.StrategyVersion().String(),
		command.AsOf.At().Format(time.RFC3339Nano),
	}, "\x00")))
	return ContinuationReference{value: "CONT-" + hex.EncodeToString(digest[:8])}
}
