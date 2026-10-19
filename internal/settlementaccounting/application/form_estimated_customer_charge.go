package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

var (
	// ErrUnexpectedSellEvaluationOutcome 说明采用快照带来了封闭集合以外的评价结果。
	ErrUnexpectedSellEvaluationOutcome = errors.New("settlement accounting: unexpected sell evaluation outcome")
	// ErrUnexpectedSellChargeFormation 说明形成编排对一份刚读到`已完成`的评价答了形成与未决以外的结果：未受理（例如
	// 合计为零，形成门拒）、不可计价或冲突。评价不可变，重投还是同一个答案，所以不作未决。
	ErrUnexpectedSellChargeFormation = errors.New("settlement accounting: unexpected sell charge formation outcome")
	// ErrSellEvaluationRequestConcludedOtherwise 说明本编排刚在同一事务里读到`已提交`，记结论时册上却已是另一份结论。
	// 同一份评价算出的结论是确定的，撞上另一份只可能是映射册在两拍之间变了而两拍又并发；整笔回滚，不覆盖先到者。
	ErrSellEvaluationRequestConcludedOtherwise = errors.New("settlement accounting: sell evaluation request already carries another conclusion")
	// ErrUnexpectedSellRequestConclusionEffect 说明请求册交回了封闭集合以外的改记落点。
	ErrUnexpectedSellRequestConclusionEffect = errors.New("settlement accounting: unexpected sell evaluation request conclusion effect")
	// ErrSellEvaluationRequestMissing 说明评价回指了一份请求，SELL 请求册上却没有它。SELL·客户费用评价只按本上下文的
	// 请求形成，请求又先于请求信封在同一库提交，所以这不是可见性滞后，是两侧说法矛盾；按「不归本编排」入账会把矛盾
	// 静默吞掉、事后也无从重投。
	ErrSellEvaluationRequestMissing = errors.New("settlement accounting: sell evaluation refers to a request absent from the sell request register")
	// ErrSellEvaluationCurrencyMismatch 说明评价的结算币种与请求快照上的那一格不同。计价按请求上快照的结算币种形成评价
	// （ADR-0190 决定四），对不上就是计价侧没守住这一格；记`待判断`会把它永久藏在请求册上，所以上抛。
	ErrSellEvaluationCurrencyMismatch = errors.New("settlement accounting: sell evaluation settlement currency differs from the request snapshot")
)

// EstimatedCustomerChargeOutcome 是一份 SELL·客户费用评价已记录之后，结算这一侧的去向。
type EstimatedCustomerChargeOutcome uint8

const (
	EstimatedCustomerChargeOutcomeInvalid EstimatedCustomerChargeOutcome = iota
	// EstimatedCustomerChargeConcluded：请求册上记下了结论——`已形成`（连同费用）、`待判断`、`不可计价`或`冲突`。
	EstimatedCustomerChargeConcluded
	// EstimatedCustomerChargeAlreadyConcluded：请求早已有结论，本次什么都不写（重放；或 ADR-0190 越权风险点 3 的「请求
	// 卡死」：一份请求只下一次结论，补登映射之后也不回补）。
	EstimatedCustomerChargeAlreadyConcluded
	// EstimatedCustomerChargeNoSellRequest：评价没有回指。这份评价不是为包裹计费时点形成的，不归本编排，零写入。回指了
	// 却在册上查不到是另一回事，见 ErrSellEvaluationRequestMissing。
	EstimatedCustomerChargeNoSellRequest
	// EstimatedCustomerChargeEvaluationNotFormed：计价侧这份评价落了`失败`。请求不改记、停在`已提交`——请求册不设「未形成」
	// 那一格（ADR-0190 决定五「计价侧没形成评价时……请求停在已提交」），评价不可变，重投也不会变。
	EstimatedCustomerChargeEvaluationNotFormed
	// EstimatedCustomerChargeUndecided：某个依赖答不出，形成与否未知，整笔回滚等重投。
	EstimatedCustomerChargeUndecided
)

func (outcome EstimatedCustomerChargeOutcome) String() string {
	switch outcome {
	case EstimatedCustomerChargeConcluded:
		return "SELL_REQUEST_CONCLUDED"
	case EstimatedCustomerChargeAlreadyConcluded:
		return "SELL_REQUEST_ALREADY_CONCLUDED"
	case EstimatedCustomerChargeNoSellRequest:
		return "NO_SELL_REQUEST"
	case EstimatedCustomerChargeEvaluationNotFormed:
		return "SELL_EVALUATION_NOT_FORMED"
	case EstimatedCustomerChargeUndecided:
		return "ESTIMATED_CUSTOMER_CHARGE_UNDECIDED"
	default:
		return ""
	}
}

// EstimatedCustomerChargeUndecidedReason 指名停在哪一个依赖上。
type EstimatedCustomerChargeUndecidedReason uint8

const (
	EstimatedCustomerChargeUndecidedReasonNone EstimatedCustomerChargeUndecidedReason = iota
	SellEvaluationRequestViewUnavailable
	FeeItemMappingViewUnavailable
	SellChargeFormationUndecided
	SellEvaluationRequestRegisterUnavailable
)

func (reason EstimatedCustomerChargeUndecidedReason) String() string {
	switch reason {
	case SellEvaluationRequestViewUnavailable:
		return "SELL_EVALUATION_REQUEST_VIEW_UNAVAILABLE"
	case FeeItemMappingViewUnavailable:
		return "FEE_ITEM_MAPPING_VIEW_UNAVAILABLE"
	case SellChargeFormationUndecided:
		return "SELL_CHARGE_FORMATION_UNDECIDED"
	case SellEvaluationRequestRegisterUnavailable:
		return "SELL_EVALUATION_REQUEST_REGISTER_UNAVAILABLE"
	default:
		return ""
	}
}

// FormEstimatedCustomerChargeCommand 是 SELL 消费门读回的那份采用快照。金额在快照上，但本编排不用它：形成编排按评价
// 引用自己再读一次（FormSellCustomerChargeCommand 头注「金额不在命令上」）。
type FormEstimatedCustomerChargeCommand struct {
	TenantID   domain.TenantID
	Evaluation ports.SellEvaluationAdoption
}

// EstimatedCustomerChargeResult 是本编排的结果。State 在`已记结论`与`早有结论`两格有意义，Charge 只在`已形成`时有。
// 未决时 cause 带着依赖原样答出的错误：原因枚举只说停在哪个依赖上，外键违例、空键这类重投不自愈的错误要靠原文才看得出。
type EstimatedCustomerChargeResult struct {
	outcome EstimatedCustomerChargeOutcome
	state   domain.SellEvaluationRequestState
	charge  domain.CustomerChargeID
	reason  EstimatedCustomerChargeUndecidedReason
	cause   error
}

func (result EstimatedCustomerChargeResult) Outcome() EstimatedCustomerChargeOutcome {
	return result.outcome
}

func (result EstimatedCustomerChargeResult) State() domain.SellEvaluationRequestState {
	return result.state
}

func (result EstimatedCustomerChargeResult) Charge() (domain.CustomerChargeID, bool) {
	return result.charge, result.state.RecordsFormedCharge()
}

func (result EstimatedCustomerChargeResult) UndecidedReason() EstimatedCustomerChargeUndecidedReason {
	return result.reason
}

func (result EstimatedCustomerChargeResult) UndecidedCause() error {
	return result.cause
}

// FormEstimatedCustomerChargeDeps 是本编排的依赖。Charges 是既有的形成编排本体：建费用、落预估都归它，本编排只
// 决定调不调它、拿什么费用标识与费用项目调。
type FormEstimatedCustomerChargeDeps struct {
	Requests    ports.SellEvaluationRequestView
	Conclusions ports.SellEvaluationRequestConclusionRecorder
	Mappings    ports.FeeItemMappingView
	Charges     *FormSellCustomerChargeHandler
	Clock       ports.Clock
}

// FormEstimatedCustomerChargeHandler 是 ADR-0190 决定五的形成编排外层：SELL·客户费用评价已记录之后，按评价回指读回
// SELL 请求，经费用项目映射册把全部费用行映到同一个费用项目，以请求派生的费用标识调形成编排，再在请求册上记结论。
// 费用行与请求改记在调用方的同一事务里：外键要求费用先落，任一步失败整笔回滚。
type FormEstimatedCustomerChargeHandler struct {
	deps FormEstimatedCustomerChargeDeps
}

func NewFormEstimatedCustomerChargeHandler(deps FormEstimatedCustomerChargeDeps) (*FormEstimatedCustomerChargeHandler, error) {
	if deps.Requests == nil || deps.Conclusions == nil || deps.Mappings == nil || deps.Charges == nil || deps.Clock == nil {
		return nil, fmt.Errorf("%w: estimated customer charge", ErrNilDependency)
	}
	return &FormEstimatedCustomerChargeHandler{deps: deps}, nil
}

// Handle 读请求 → 看评价结果 → 映费用项目 → 形成 → 记结论。
//
// 请求已不是`已提交`就什么都不做：一份请求只下一次结论，重放在这里落定，不再碰费用库。各种评价结果的去向：
//   - 待判断、金额未声明（卡没声明金额取整，租户取值缺席）、费用行映不全或映到多个费用项目（AT-SA-174）——一律记
//     `待判断`，不建占位费用，入账不重投。卡与映射是租户取值，补登之后这一份也不回补（ADR-0190 越权风险点 3）。
//   - 不可计价、冲突——照记那一格（AT-SA-175：不形成任何费用行，不转待判断反复重试）。
//   - 失败——请求停在`已提交`，见 EstimatedCustomerChargeEvaluationNotFormed。
//
// 两侧说法矛盾的格上抛错误、不入账：回指的请求不在册上，结算币种对不上请求快照。
func (handler *FormEstimatedCustomerChargeHandler) Handle(
	ctx context.Context,
	command FormEstimatedCustomerChargeCommand,
) (EstimatedCustomerChargeResult, error) {
	adoption := command.Evaluation
	if adoption.Request.String() == "" {
		return EstimatedCustomerChargeResult{outcome: EstimatedCustomerChargeNoSellRequest}, nil
	}
	key := ports.SellEvaluationRequestKey{TenantID: command.TenantID, Request: adoption.Request}
	record, found, err := handler.deps.Requests.FindByID(ctx, key)
	if err != nil {
		return undecidedEstimatedCharge(SellEvaluationRequestViewUnavailable, err), nil
	}
	if !found {
		return EstimatedCustomerChargeResult{}, fmt.Errorf("%w: request %s", ErrSellEvaluationRequestMissing, adoption.Request)
	}
	if record.State != domain.SellRequestSubmitted {
		return EstimatedCustomerChargeResult{
			outcome: EstimatedCustomerChargeAlreadyConcluded,
			state:   record.State,
			charge:  record.FormedCharge,
		}, nil
	}

	switch adoption.Outcome {
	case ports.SellEvaluationCompleted:
	case ports.SellEvaluationPending, ports.SellEvaluationAmountUndeclared:
		return handler.conclude(ctx, key, domain.SellRequestPendingJudgment, domain.CustomerChargeID{})
	case ports.SellEvaluationUnratable:
		return handler.conclude(ctx, key, domain.SellRequestUnratable, domain.CustomerChargeID{})
	case ports.SellEvaluationConflict:
		return handler.conclude(ctx, key, domain.SellRequestConflict, domain.CustomerChargeID{})
	case ports.SellEvaluationNotFormed:
		return EstimatedCustomerChargeResult{outcome: EstimatedCustomerChargeEvaluationNotFormed}, nil
	default:
		return EstimatedCustomerChargeResult{}, fmt.Errorf("%w: %d", ErrUnexpectedSellEvaluationOutcome, adoption.Outcome)
	}

	if adoption.SettlementCurrency != record.Request.SettlementCurrency() {
		return EstimatedCustomerChargeResult{}, fmt.Errorf("%w: request %s wants %s, evaluation %s carries %s",
			ErrSellEvaluationCurrencyMismatch, adoption.Request, record.Request.SettlementCurrency(), adoption.Evaluation, adoption.SettlementCurrency)
	}
	feeItem, mapped, err := handler.singleFeeItem(ctx, command.TenantID, adoption.ChargeCodes)
	if err != nil {
		return undecidedEstimatedCharge(FeeItemMappingViewUnavailable, err), nil
	}
	if !mapped {
		return handler.conclude(ctx, key, domain.SellRequestPendingJudgment, domain.CustomerChargeID{})
	}

	charge := record.Request.EstimatedCustomerChargeID()
	formed, err := handler.deps.Charges.Handle(ctx, FormSellCustomerChargeCommand{
		TenantID:   command.TenantID,
		Charge:     charge.String(),
		FeeItem:    feeItem.String(),
		Evaluation: adoption.Evaluation.String(),
		FormedAt:   handler.deps.Clock.Now(),
	})
	if err != nil {
		return EstimatedCustomerChargeResult{}, err
	}
	switch formed.Outcome() {
	case SellChargeFormed, SellChargeAlreadyFormed:
		return handler.conclude(ctx, key, domain.SellRequestFormed, charge)
	case SellChargeUndecided:
		return undecidedEstimatedCharge(SellChargeFormationUndecided, fmt.Errorf("sell charge formation: %s", formed.UndecidedReason())), nil
	default:
		return EstimatedCustomerChargeResult{}, fmt.Errorf("%w: %s", ErrUnexpectedSellChargeFormation, formed.Outcome())
	}
}

// singleFeeItem 把全部费用代码经映射册映到费用项目（ADR-0190 决定五，产品策略「一份评价映一个费用项目」）：每个代码都
// 有映射、且映到的是同一个费用项目，才交回它。任一代码没登记、映到多于一个项目、或评价没有费用行，第二个返回值为假
// ——拿价格代码当费用项目、或挑其中一个项目，都是在替租户登映射。
func (handler *FormEstimatedCustomerChargeHandler) singleFeeItem(
	ctx context.Context,
	tenant domain.TenantID,
	codes []domain.PricingChargeCode,
) (domain.FeeItemReference, bool, error) {
	var (
		chosen domain.FeeItemReference
		seen   = map[string]bool{}
	)
	for _, code := range codes {
		if seen[code.String()] {
			continue
		}
		seen[code.String()] = true
		mapping, found, err := handler.deps.Mappings.FindFeeItemMapping(ctx, tenant, code)
		if err != nil {
			return domain.FeeItemReference{}, false, err
		}
		if !found {
			return domain.FeeItemReference{}, false, nil
		}
		if chosen.String() != "" && chosen.String() != mapping.FeeItem().String() {
			return domain.FeeItemReference{}, false, nil
		}
		chosen = mapping.FeeItem()
	}
	return chosen, chosen.String() != "", nil
}

// conclude 在请求册上记结论。另有结论不覆盖，作错误整笔回滚——见 ErrSellEvaluationRequestConcludedOtherwise。
func (handler *FormEstimatedCustomerChargeHandler) conclude(
	ctx context.Context,
	key ports.SellEvaluationRequestKey,
	state domain.SellEvaluationRequestState,
	charge domain.CustomerChargeID,
) (EstimatedCustomerChargeResult, error) {
	conclusion, err := domain.NewSellEvaluationRequestConclusion(state, charge)
	if err != nil {
		return EstimatedCustomerChargeResult{}, err
	}
	effect, err := handler.deps.Conclusions.RecordConclusion(ctx, key, conclusion)
	if err != nil {
		return undecidedEstimatedCharge(SellEvaluationRequestRegisterUnavailable, err), nil
	}
	switch effect {
	case ports.SellEvaluationRequestConcluded, ports.SellEvaluationRequestConclusionReplayed:
		return EstimatedCustomerChargeResult{outcome: EstimatedCustomerChargeConcluded, state: state, charge: charge}, nil
	case ports.SellEvaluationRequestConcludedOtherwise:
		return EstimatedCustomerChargeResult{}, fmt.Errorf("%w: request %s, wanted %s", ErrSellEvaluationRequestConcludedOtherwise, key.Request, state)
	default:
		return EstimatedCustomerChargeResult{}, fmt.Errorf("%w: %d", ErrUnexpectedSellRequestConclusionEffect, effect)
	}
}

func undecidedEstimatedCharge(reason EstimatedCustomerChargeUndecidedReason, cause error) EstimatedCustomerChargeResult {
	return EstimatedCustomerChargeResult{outcome: EstimatedCustomerChargeUndecided, reason: reason, cause: cause}
}
