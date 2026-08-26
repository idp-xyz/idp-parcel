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
	ControlPolicyNotConfigured
	BalanceUnavailable
	FreezeLedgerUnavailable
	CreditStandingUnavailable
	ExposureLedgerUnavailable
)

func (reason NotFormedReason) String() string {
	switch reason {
	case ControlPolicyUnavailable:
		return "CONTROL_POLICY_UNAVAILABLE"
	case ControlPolicyNotConfigured:
		return "CONTROL_POLICY_NOT_CONFIGURED"
	case BalanceUnavailable:
		return "BALANCE_UNAVAILABLE"
	case FreezeLedgerUnavailable:
		return "FREEZE_LEDGER_UNAVAILABLE"
	case CreditStandingUnavailable:
		return "CREDIT_STANDING_UNAVAILABLE"
	case ExposureLedgerUnavailable:
		return "EXPOSURE_LEDGER_UNAVAILABLE"
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
	// Resolution 回指调用方那次商业解析，控制策略视图凭它向商业侧提问
	// （sa-preacceptance-policy-view/01 裁决）。作用域与它同源：作用域正是从这次解析的
	// 结算政策回显派生的，所以要求调用方一并带上不是多一次索取，是把已有的那一份写明。
	Resolution domain.CommercialResolutionReference
}

// minimumIdentityEstablished 在读任何权威之前判断该不该读。金额算在受理条件里而不留给
// 领域构造期：非正数金额的请求根本不该去读这个客户的余额，而一次已经发出的读取收不回来。
//
// 商业解析回指同理算在受理条件里，而且理由更硬：少了它，控制策略视图根本无从提问，
// 而它答出的任何一格都会是假话——`未登记`会把「没问成」说成「商业侧没登记过」，等来的
// 是租户去补一份其实已经存在的声明。停在`未受理`才说得清是调用方少给了键。
func (command ApplyPreAcceptanceControlCommand) minimumIdentityEstablished() bool {
	return command.TenantID.String() != "" &&
		command.RequestID.String() != "" &&
		command.Scope.LegalEntity().String() != "" &&
		command.Scope.Account().String() != "" &&
		command.Scope.Currency().String() != "" &&
		command.Association.String() != "" &&
		command.AsOf.Valid() &&
		command.AmountMinor > 0 &&
		command.Resolution.String() != ""
}

type ApplyPreAcceptanceControlResult struct {
	outcome       ControlOutcome
	freeze        domain.FundsFreeze
	hasFreeze     bool
	exposure      domain.CreditExposure
	hasExposure   bool
	method        domain.SettlementMethod
	adoptedPolicy domain.AdoptedPolicyReference
	controlledAt  time.Time
	asOf          domain.ControlAsOf
	basis         domain.ControlBasisReference
	reason        NotFormedReason
	continuation  ContinuationReference
}

func (result ApplyPreAcceptanceControlResult) Outcome() ControlOutcome {
	return result.outcome
}

// Freeze 只在预付控制实际执行过时给出，`无控制`、冲突、未受理与待判断一律没有。交回一个
// 零值冻结，正是 CONTEXT 禁止的「用虚假冻结冒充控制」。
func (result ApplyPreAcceptanceControlResult) Freeze() (domain.FundsFreeze, bool) {
	return result.freeze, result.hasFreeze
}

// Exposure 只在账期控制实际执行过时给出（ADR-0047）。冻结与暴露不会同时在场：一次控制
// 请求按解析出的方式走且只走一条路。
func (result ApplyPreAcceptanceControlResult) Exposure() (domain.CreditExposure, bool) {
	return result.exposure, result.hasExposure
}

// Method 与 AdoptedPolicy 在控制执行过时携带实际采用的方式与结算政策引用——CONTEXT 要求
// 每项冻结与信用暴露都保存它们。
func (result ApplyPreAcceptanceControlResult) Method() domain.SettlementMethod {
	return result.method
}

func (result ApplyPreAcceptanceControlResult) AdoptedPolicy() domain.AdoptedPolicyReference {
	return result.adoptedPolicy
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
	policy    ports.PreAcceptanceControlPolicyView
	balance   ports.OperationalBalanceView
	ledger    ports.FreezeLedgerRepository
	credit    ports.CreditStandingView
	exposures ports.CreditExposureLedgerRepository
	clock     ports.Clock
}

type ApplyPreAcceptanceControlDeps struct {
	Policy    ports.PreAcceptanceControlPolicyView
	Balance   ports.OperationalBalanceView
	Freezes   ports.FreezeLedgerRepository
	Credit    ports.CreditStandingView
	Exposures ports.CreditExposureLedgerRepository
	Clock     ports.Clock
}

func NewApplyPreAcceptanceControlHandler(deps ApplyPreAcceptanceControlDeps) *ApplyPreAcceptanceControlHandler {
	return &ApplyPreAcceptanceControlHandler{
		policy:    deps.Policy,
		balance:   deps.Balance,
		ledger:    deps.Freezes,
		credit:    deps.Credit,
		exposures: deps.Exposures,
		clock:     deps.Clock,
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
	policy, configured, err := handler.policy.LoadControlPolicy(
		ctx, command.TenantID, command.Scope, command.Resolution)
	if err != nil {
		// 商业侧调不通形成待判断，不读成「不要求控制」。后者正是 CONTEXT 禁止的默认信用
		// 通过：一次商业故障会因此变成一个看起来通过了的接受前控制。
		return handler.notFormed(command, ControlPolicyUnavailable), nil
	}
	if !configured {
		// 未登记同样不是`无控制`（ADR-0054）。两者的恢复动作相反：这一格等商业侧登记
		// PAR-COM-15，而`无控制`是合同已经说过的话，据它可以放行接受判断。
		return handler.notFormed(command, ControlPolicyNotConfigured), nil
	}
	if !policy.ControlRequired() {
		return ApplyPreAcceptanceControlResult{
			outcome: ControlNotApplicable,
			asOf:    command.AsOf,
			basis:   policy.Basis(),
		}, nil
	}

	// 按解析出的结算方式分支（ADR-0047）：预付占资金、账期占额度，两本账互不借用。
	// 方式在政策构造期已保证有效，落到 default 只能是新增取值没接分支——编程错误上抛。
	switch policy.Method() {
	case domain.PrepaidSettlement:
		return handler.applyPrepaidFreeze(ctx, command, policy)
	case domain.TermsSettlement:
		return handler.applyTermsExposure(ctx, command, policy)
	default:
		return ApplyPreAcceptanceControlResult{}, fmt.Errorf(
			"pre-acceptance control: unhandled settlement method %d", uint8(policy.Method()))
	}
}

func (handler *ApplyPreAcceptanceControlHandler) applyPrepaidFreeze(
	ctx context.Context,
	command ApplyPreAcceptanceControlCommand,
	policy domain.PreAcceptanceControlPolicy,
) (ApplyPreAcceptanceControlResult, error) {
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
		outcome:       ControlApplied,
		freeze:        freeze,
		hasFreeze:     true,
		method:        policy.Method(),
		adoptedPolicy: policy.AdoptedPolicy(),
		controlledAt:  controlledAt,
		asOf:          command.AsOf,
	}, nil
}

// applyTermsExposure 是账期分支：读信用状况、在暴露账本上占用额度。逾期与超额形成
// `业务限制`装在暴露的状态里，与预付分支的余额不足同构。
func (handler *ApplyPreAcceptanceControlHandler) applyTermsExposure(
	ctx context.Context,
	command ApplyPreAcceptanceControlCommand,
	policy domain.PreAcceptanceControlPolicy,
) (ApplyPreAcceptanceControlResult, error) {
	standing, err := handler.credit.LoadCreditStanding(ctx, command.TenantID, command.Scope)
	if err != nil {
		return handler.notFormed(command, CreditStandingUnavailable), nil
	}

	ledger, err := handler.exposures.LoadForScope(ctx, command.TenantID, command.Scope)
	if err != nil || ledger == nil {
		return handler.notFormed(command, ExposureLedgerUnavailable), nil
	}

	controlledAt := handler.clock.Now()
	request, err := domain.NewExposureRequest(
		command.RequestID,
		command.Scope,
		command.AmountMinor,
		command.Association,
		controlledAt,
	)
	if err != nil {
		return ApplyPreAcceptanceControlResult{outcome: ControlRequestNotAccepted}, nil
	}

	exposure, err := ledger.Expose(request, standing)
	if err != nil {
		if errors.Is(err, domain.ErrControlRequestConflict) {
			return ApplyPreAcceptanceControlResult{outcome: ControlRequestConflict}, nil
		}
		return ApplyPreAcceptanceControlResult{}, fmt.Errorf("expose credit: %w", err)
	}

	if err := handler.exposures.Save(ctx, command.TenantID, command.Scope, ledger); err != nil {
		return handler.notFormed(command, ExposureLedgerUnavailable), nil
	}

	return ApplyPreAcceptanceControlResult{
		outcome:       ControlApplied,
		exposure:      exposure,
		hasExposure:   true,
		method:        policy.Method(),
		adoptedPolicy: policy.AdoptedPolicy(),
		controlledAt:  controlledAt,
		asOf:          command.AsOf,
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
//
// 商业解析回指也在摘要里：它决定策略视图问到的是哪一份合同，换了回指就是换了一次问答，
// 两者共用一条续办引用会让续办方按引用查回来的是另一份商业依据下的停摆。
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
		command.Resolution.String(),
	}, "\x00")))
	return ContinuationReference{value: "CONT-" + hex.EncodeToString(digest[:8])}
}
