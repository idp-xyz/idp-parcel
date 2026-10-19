// trigger_buy_evaluation_on_occurrence_formed.go 编排 ADR-0208 的发起半边：运输履约交出一份「发生项已形成」之后，结算按
// 这份发生项决定要不要发起 BUY 评价请求。发生项、原因、版本、业务时间与供应商协议都取自发生项本身——协议是发生项形成时
// 就固定下来的来源事实，不是从登记册推出来的；费用项目取 BUY 触发册那一行；主要范围取那一版协议的商业范围。登记请求与
// 交信封照旧由 RequestBuyEvaluationHandler 做。
package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// ErrOccurrenceAgreementNotRegistered 说明发生项上固定的那一版供应商协议，party-commercial 不认。运输履约登记发生项时
// 已经核过协议，两侧对不上是矛盾，不是等谁：调用方上抛、落发布失败等人看，不进未决名单，重投同一份永远同一个结果。
var ErrOccurrenceAgreementNotRegistered = errors.New(
	"settlement accounting: the supplier agreement fixed on the charge occurrence is not registered in party-commercial")

// ErrUnexpectedBuyEvaluationRequestOutcome 说明请求编排交回了封闭集合以外的结果。静默入账等于替编排作判断。
var ErrUnexpectedBuyEvaluationRequestOutcome = errors.New("settlement accounting: unexpected buy evaluation request outcome")

// occurrenceFormedRequester 是本路登在请求上的请求方。本路没有操作者：发起它的是这一封信，登记的请求方就是这条路自己。
// 它是产品里的一个固定名，不是租户取值。
const occurrenceFormedRequester = "settlement-accounting/buy-evaluation-on-occurrence-formed"

// TriggerBuyEvaluationOnOccurrenceFormedCommand 是一份已形成的发生项在结算眼里的样子：引用四件（标识、原因、有效性
// 版本、业务时间）与它固定的供应商协议。费用项目与主要范围都不在命令上——前者归触发册，后者归协议，调用方带得了它们
// 就有了第二套口径。
//
// Revised 说这一版是不是有效性修订产生的。是就不发起，见 BuyEvaluationTriggerOccurrenceRevised。
type TriggerBuyEvaluationOnOccurrenceFormedCommand struct {
	TenantID   domain.TenantID
	Occurrence domain.TransportChargeOccurrence
	Agreement  domain.SupplierAgreementReference
	Revised    bool
}

// TriggerBuyEvaluationOnOccurrenceFormedDeps 三口都必备：缺触发册就只能替租户默认一个原因或费用项目；缺协议范围读口就
// 只能拿别的东西顶主要范围；缺请求编排这一路就走不完。
type TriggerBuyEvaluationOnOccurrenceFormedDeps struct {
	Triggers   ports.BuyEvaluationTriggerView
	Agreements ports.SupplierAgreementScopeView
	Requests   *RequestBuyEvaluationHandler
}

type TriggerBuyEvaluationOnOccurrenceFormedHandler struct {
	deps TriggerBuyEvaluationOnOccurrenceFormedDeps
}

// NewTriggerBuyEvaluationOnOccurrenceFormedHandler 构造期拒 nil，缺件包 ErrNilDependency 并点名，理由同
// NewRequestBuyEvaluationHandler。
func NewTriggerBuyEvaluationOnOccurrenceFormedHandler(
	deps TriggerBuyEvaluationOnOccurrenceFormedDeps,
) (*TriggerBuyEvaluationOnOccurrenceFormedHandler, error) {
	for _, dependency := range []struct {
		name    string
		missing bool
	}{
		{"buy evaluation trigger view", deps.Triggers == nil},
		{"supplier agreement scope view", deps.Agreements == nil},
		{"request buy evaluation handler", deps.Requests == nil},
	} {
		if dependency.missing {
			return nil, fmt.Errorf("%w: %s", ErrNilDependency, dependency.name)
		}
	}
	return &TriggerBuyEvaluationOnOccurrenceFormedHandler{deps: deps}, nil
}

// Trigger 走一拍：受理 → 修订版不发起 → 触发册（原因、费用项目）→ 协议范围 → 交请求编排。
//
// 未配置的两格先于协议读口判：没登的原因不必去问 party-commercial。答`未配置`的入账不重投，只有依赖答不出才是`未决`
// （ADR-0029、ADR-0094）。协议范围读口答出确定性的坏答案（ports.ErrSupplierAgreementScopeUntranslatable）不算答不出，
// 与协议查无一样原样上抛。
//
// 本编排只该跑在调用方的事务里：请求登记与信封同生同灭。请求编排在交信封失败时留续办引用、照答已请求，那个形状在
// 事务里会让一份请求行提交而信封缺席、再没有东西推它，所以这里把它改答`未决`，让调用方整笔回滚重来——与 SELL 那只
// 编排同一条理由（RequestSellEvaluationOnParcelMomentHandler.Handle 头注）。
func (handler *TriggerBuyEvaluationOnOccurrenceFormedHandler) Trigger(
	ctx context.Context,
	command TriggerBuyEvaluationOnOccurrenceFormedCommand,
) (BuyEvaluationTriggerResult, error) {
	if command.TenantID.String() == "" || command.Occurrence.ID().String() == "" ||
		command.Occurrence.Reason().String() == "" || command.Agreement.String() == "" {
		return BuyEvaluationTriggerResult{outcome: BuyEvaluationTriggerNotAccepted}, nil
	}
	if command.Revised {
		return BuyEvaluationTriggerResult{outcome: BuyEvaluationTriggerOccurrenceRevised}, nil
	}

	registration, found, err := handler.deps.Triggers.LoadBuyEvaluationTrigger(ctx, command.TenantID, command.Occurrence.Reason())
	if err != nil {
		return buyTriggerUndecided(BuyEvaluationTriggerRegisterUnavailable), nil
	}
	if !found || registration.Moment() != domain.BuyEvaluationTriggerOnOccurrenceFormed {
		return buyTriggerUnconfigured(BuyEvaluationTriggerReasonNotRegistered), nil
	}
	feeItem, registered := registration.FeeItem()
	if !registered {
		return buyTriggerUnconfigured(BuyEvaluationTriggerFeeItemNotRegistered), nil
	}

	scope, found, err := handler.deps.Agreements.LoadSupplierAgreementScope(ctx, command.TenantID, command.Agreement)
	if errors.Is(err, ports.ErrSupplierAgreementScopeUntranslatable) {
		return BuyEvaluationTriggerResult{}, fmt.Errorf("trigger buy evaluation on occurrence formed: occurrence %s agreement %s: %w",
			command.Occurrence.ID(), command.Agreement, err)
	}
	if err != nil {
		return buyTriggerUndecided(BuyEvaluationTriggerAgreementScopeUnavailable), nil
	}
	if !found {
		return BuyEvaluationTriggerResult{}, fmt.Errorf("%w: occurrence %s agreement %s",
			ErrOccurrenceAgreementNotRegistered, command.Occurrence.ID(), command.Agreement)
	}

	requester, err := domain.NewRequesterReference(occurrenceFormedRequester)
	if err != nil {
		return BuyEvaluationTriggerResult{}, fmt.Errorf("trigger buy evaluation on occurrence formed: requester: %w", err)
	}
	requested, err := handler.deps.Requests.Handle(ctx, RequestBuyEvaluationCommand{
		TenantID:    command.TenantID,
		Scope:       scope,
		Occurrence:  command.Occurrence,
		FeeItem:     feeItem,
		Agreement:   command.Agreement,
		RequestedBy: requester,
	})
	if err != nil {
		return BuyEvaluationTriggerResult{}, err
	}
	switch requested.Outcome() {
	case EvaluationRequested, EvaluationRequestExisting:
		if requested.EvaluationRequestHandoffReference() != "" {
			return BuyEvaluationTriggerResult{
				outcome:   BuyEvaluationTriggerUndecided,
				undecided: BuyEvaluationTriggerRequestHandoffUnavailable,
				request:   requested,
				has:       true,
			}, nil
		}
		return BuyEvaluationTriggerResult{outcome: BuyEvaluationTriggerFired, request: requested, has: true}, nil
	case EvaluationRequestUndecided:
		return BuyEvaluationTriggerResult{
			outcome:   BuyEvaluationTriggerUndecided,
			undecided: BuyEvaluationTriggerRequestUndecided,
			request:   requested,
			has:       true,
		}, nil
	case EvaluationRequestNotAccepted:
		return BuyEvaluationTriggerResult{outcome: BuyEvaluationTriggerNotAccepted, request: requested, has: true}, nil
	default:
		return BuyEvaluationTriggerResult{}, fmt.Errorf("%w: %q", ErrUnexpectedBuyEvaluationRequestOutcome, requested.Outcome())
	}
}

func buyTriggerUnconfigured(reason BuyEvaluationTriggerUnconfiguredReason) BuyEvaluationTriggerResult {
	return BuyEvaluationTriggerResult{outcome: BuyEvaluationTriggerUnconfigured, unconfigured: reason}
}

func buyTriggerUndecided(reason BuyEvaluationTriggerUndecidedReason) BuyEvaluationTriggerResult {
	return BuyEvaluationTriggerResult{outcome: BuyEvaluationTriggerUndecided, undecided: reason}
}
