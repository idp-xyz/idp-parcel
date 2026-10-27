package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

var (
	// ErrBuyEvaluationRequestNotRecorded 说明评价回指了一份 BUY 评价请求，册上却没有它。请求与它的信封在同一事务里
	// 先于评价提交，计价按回指读回请求才形成评价，所以这里不存在可见性滞后：读不到只能是两侧数据矛盾（请求被删、
	// 回指被改、或回指指进了别的租户）。重投不自愈，作错误交人看。
	ErrBuyEvaluationRequestNotRecorded = errors.New("settlement accounting: the buy evaluation request the evaluation refers to is not recorded")
	// ErrSupplierCostVersionContradiction 说明由请求派生的版本标识上已有另一份内容。计价按回指至多为一份请求形成一份
	// 评价，请求与评价都不可变，同一标识只可能对应同一份内容；撞上另一份是库被改过或另有一条路占了这个标识。
	ErrSupplierCostVersionContradiction = errors.New("settlement accounting: the derived supplier cost version already carries other content")
	// ErrBuyEvaluationRequestStoppedOtherwise 说明 BUY 请求册上已是另一个停因，或已记`负向结束（零额）`。同一份评价折出的
	// 去向是确定的，撞上另一个只可能是册被改过，或另一条路给同一份请求记了别的去向；不覆盖，作错误交人看。
	ErrBuyEvaluationRequestStoppedOtherwise = errors.New("settlement accounting: the buy evaluation request already carries another stop cause")
	// ErrUnexpectedRequestedExpectedCostFormation 说明形成编排答了本外层不认的格：未受理（例如同币种两额不等、原币为零
	// 而结算币不为零，形成门拒），或封闭集合以外的结果与未决原因。评价不可变，重投还是同一个答案，所以不作未决。合计为零
	// 不在此列，记`负向结束（零额）`（ADR-0306 决定三）。
	ErrUnexpectedRequestedExpectedCostFormation = errors.New("settlement accounting: unexpected requested supplier expected cost formation")
	// ErrBuyEvaluationRequestStoppedBeforeZeroTotal 说明要给一份请求记`负向结束（零额）`，册上却已有停因。同一份评价只折得出
	// 一种去向，撞上另一种只可能是册被改过，或另一条路给同一份请求记了停因；不覆盖，作错误交人看。
	ErrBuyEvaluationRequestStoppedBeforeZeroTotal = errors.New("settlement accounting: the buy evaluation request already carries a stop cause, not a zero total")
)

// RequestedSupplierExpectedCostOutcome 是一份 BUY·供应商成本评价已记录之后，结算这一侧的去向。
type RequestedSupplierExpectedCostOutcome uint8

const (
	RequestedSupplierExpectedCostOutcomeInvalid RequestedSupplierExpectedCostOutcome = iota
	// RequestedSupplierExpectedCostAdopted：派生版本上有了这份评价的预期成本首版——本次形成，或早已形成（重放）。
	RequestedSupplierExpectedCostAdopted
	// RequestedSupplierExpectedCostFirstVersionExists：同一版发生项、同一费用项目与规则版本上，另一份请求已先形成了首版（版本标识不同）。
	// 不得重复形成首版。点名发生项另一版的请求不落这一格：链头已撤下时接在后面形成纠错版本（ADR-0232 决定八）。
	RequestedSupplierExpectedCostFirstVersionExists
	// RequestedSupplierExpectedCostEvaluationNotAdopted：这份评价形成不了预期成本，不写成本行，只在 BUY 请求册上记停因——
	// 不可计价、评价冲突、评价待判断、评价未形成、跨币种却没带换算步骤、价卡没声明金额取整。评价不可变，计价对同一份
	// 请求也不再形成第二份评价，重投这一封不会变；补登之后由「参数已登记」按停因另起下一代（ADR-0229 决定二、三）。
	// 哪一格看 Formation 与 StopCause。
	RequestedSupplierExpectedCostEvaluationNotAdopted
	// RequestedSupplierExpectedCostNoBuyRequest：评价没有回指（计价内部或测试路径形成）。不归本编排，零写入。
	RequestedSupplierExpectedCostNoBuyRequest
	// RequestedSupplierExpectedCostUndecided：某个依赖答不出，形成与否未知，整笔回滚等重投。
	RequestedSupplierExpectedCostUndecided
	// RequestedSupplierExpectedCostOccurrenceInvalidated：请求所依的那一版发生项已失效，评价不再形成成本，零写入（ADR-0232 决定七）。
	RequestedSupplierExpectedCostOccurrenceInvalidated
	// RequestedSupplierExpectedCostZeroTotal：评价已完成而合计为零。不写成本行，在 BUY 请求册上记`负向结束（零额）`；不是停因，
	// 不进续办队列（ADR-0306 决定三）。
	RequestedSupplierExpectedCostZeroTotal
)

func (outcome RequestedSupplierExpectedCostOutcome) String() string {
	switch outcome {
	case RequestedSupplierExpectedCostAdopted:
		return "SUPPLIER_EXPECTED_COST_ADOPTED"
	case RequestedSupplierExpectedCostFirstVersionExists:
		return "SUPPLIER_EXPECTED_COST_FIRST_VERSION_EXISTS"
	case RequestedSupplierExpectedCostEvaluationNotAdopted:
		return "BUY_EVALUATION_NOT_ADOPTED"
	case RequestedSupplierExpectedCostNoBuyRequest:
		return "NO_BUY_REQUEST"
	case RequestedSupplierExpectedCostUndecided:
		return "SUPPLIER_EXPECTED_COST_UNDECIDED"
	case RequestedSupplierExpectedCostOccurrenceInvalidated:
		return "OCCURRENCE_INVALIDATED"
	case RequestedSupplierExpectedCostZeroTotal:
		return "BUY_EVALUATION_ZERO_TOTAL"
	default:
		return ""
	}
}

// FormRequestedSupplierExpectedCostCommand 是 BUY 消费门读回的那份采用快照。金额在快照上，但本编排不用它：形成编排
// 按评价引用自己再读一次（FormSupplierExpectedCostCommand 头注「金额……不在命令上」）。
type FormRequestedSupplierExpectedCostCommand struct {
	TenantID   domain.TenantID
	Evaluation ports.BuyEvaluationAdoption
}

// RequestedSupplierExpectedCostResult 是本编排的结果。Formation 是形成编排那一层的答案，在`已采用`、`已有首版`与
// `评价采用不了`三格有意义；未决时 UndecidedReason 与 UndecidedCause 指名停在哪个依赖上、它答了什么。
type RequestedSupplierExpectedCostResult struct {
	outcome   RequestedSupplierExpectedCostOutcome
	formation ExpectedCostResult
	stopCause domain.EvaluationStopCause
	reason    ExpectedCostUndecidedReason
	cause     error
}

// StopCause 在`评价采用不了`一格交回记在请求册上的停因；其余各格第二个返回值为假。
func (result RequestedSupplierExpectedCostResult) StopCause() (domain.EvaluationStopCause, bool) {
	return result.stopCause, result.stopCause != domain.EvaluationStopCauseInvalid
}

func (result RequestedSupplierExpectedCostResult) Outcome() RequestedSupplierExpectedCostOutcome {
	return result.outcome
}

func (result RequestedSupplierExpectedCostResult) Formation() ExpectedCostResult {
	return result.formation
}

func (result RequestedSupplierExpectedCostResult) UndecidedReason() ExpectedCostUndecidedReason {
	return result.reason
}

func (result RequestedSupplierExpectedCostResult) UndecidedCause() error {
	return result.cause
}

// FormRequestedSupplierExpectedCostDeps 是本编排的依赖。Costs 是既有的形成编排本体：读评价、分格、建成本、登记都归它，
// 本编排只决定调不调它、拿哪份请求的来源引用与哪个版本标识调。Stops 记评价采用不了时的停因，ZeroTotals 记评价已完成而
// 合计为零的`负向结束（零额）`，Clock 给停下的时刻。
type FormRequestedSupplierExpectedCostDeps struct {
	Requests   ports.EvaluationRequestView
	Costs      *FormSupplierExpectedCostHandler
	Stops      ports.EvaluationRequestStopRecorder
	ZeroTotals ports.EvaluationRequestZeroTotalRecorder
	Clock      ports.Clock
}

// FormRequestedSupplierExpectedCostHandler 是 UC-SA-002 步 5 BUY 方向的形成编排外层（ADR-0200）：BUY·供应商
// 成本评价已记录之后，按评价回指读回 BUY 评价请求，以请求上的合格来源引用与由请求派生的版本标识调形成编排。评价采用
// 不了的那几格在请求册上记停因（ADR-0229 决定三）；评价状态与问题项仍在计价那一侧的评价上，按回指读。
type FormRequestedSupplierExpectedCostHandler struct {
	deps FormRequestedSupplierExpectedCostDeps
}

func NewFormRequestedSupplierExpectedCostHandler(deps FormRequestedSupplierExpectedCostDeps) (*FormRequestedSupplierExpectedCostHandler, error) {
	if deps.Requests == nil || deps.Costs == nil || deps.Stops == nil || deps.ZeroTotals == nil || deps.Clock == nil {
		return nil, fmt.Errorf("%w: requested supplier expected cost", ErrNilDependency)
	}
	return &FormRequestedSupplierExpectedCostHandler{deps: deps}, nil
}

// Handle 读回指 → 读请求 → 组命令 → 形成 → 折结果。
//
// 去向按恢复动作分（ADR-0029、ADR-0094「失败预算从此只花在真会自愈的依赖上」）：只有读口、登记面答不出才是未决、整笔
// 回滚等重投；评价本身停下的各格记停因后入账——评价不可变，计价按回指对同一份请求也不再形成第二份评价，重投这一封
// 只会读到同一个答案，要再问是下一代的事。评价已完成而合计为零记`负向结束（零额）`后入账，不记停因（ADR-0306 决定三）。
func (handler *FormRequestedSupplierExpectedCostHandler) Handle(
	ctx context.Context,
	command FormRequestedSupplierExpectedCostCommand,
) (RequestedSupplierExpectedCostResult, error) {
	adoption := command.Evaluation
	if adoption.Request.String() == "" {
		return RequestedSupplierExpectedCostResult{outcome: RequestedSupplierExpectedCostNoBuyRequest}, nil
	}
	key := ports.EvaluationRequestKey{TenantID: command.TenantID, Request: adoption.Request}
	record, found, err := handler.deps.Requests.FindByID(ctx, key)
	if err != nil {
		return RequestedSupplierExpectedCostResult{
			outcome: RequestedSupplierExpectedCostUndecided, reason: BuyEvaluationRequestViewUnavailable, cause: err,
		}, nil
	}
	if !found {
		return RequestedSupplierExpectedCostResult{}, fmt.Errorf("%w: evaluation %s refers to request %s",
			ErrBuyEvaluationRequestNotRecorded, adoption.Evaluation, adoption.Request)
	}

	request := record.Request
	sources := request.Sources()
	version := request.SupplierExpectedCostVersionID()
	formed, err := handler.deps.Costs.Handle(ctx, FormSupplierExpectedCostCommand{
		TenantID:   command.TenantID,
		Version:    version,
		Occurrence: sources.Occurrence,
		FeeItem:    sources.FeeItem,
		Agreement:  sources.Agreement,
		Evaluation: adoption.Evaluation,
	})
	if err != nil {
		return RequestedSupplierExpectedCostResult{}, err
	}
	switch formed.Outcome() {
	case ExpectedCostFormed, ExpectedCostExistingResult:
		return RequestedSupplierExpectedCostResult{outcome: RequestedSupplierExpectedCostAdopted, formation: formed}, nil
	case ExpectedCostAlreadyFormed:
		return RequestedSupplierExpectedCostResult{outcome: RequestedSupplierExpectedCostFirstVersionExists, formation: formed}, nil
	case ExpectedCostUnratable:
		return handler.stop(ctx, key, formed, domain.StopEvaluationUnratable)
	case EvaluationConflict:
		return handler.stop(ctx, key, formed, domain.StopEvaluationConflict)
	case ExpectedCostOccurrenceInvalidated:
		// 发生项那一版已失效不是停办：没有哪一种登记补得回它，记成停因只会让续办去重跑一份注定不形成成本的请求。
		return RequestedSupplierExpectedCostResult{outcome: RequestedSupplierExpectedCostOccurrenceInvalidated, formation: formed}, nil
	case ExpectedCostZeroTotal:
		return handler.zeroTotal(ctx, key, formed)
	case ExpectedCostUndecided:
		folded, err := foldUndecidedFormation(formed, version)
		if err != nil || folded.outcome != RequestedSupplierExpectedCostEvaluationNotAdopted {
			return folded, err
		}
		return handler.stop(ctx, key, formed, folded.stopCause)
	case ExpectedCostConflict:
		return RequestedSupplierExpectedCostResult{}, fmt.Errorf("%w: version %s", ErrSupplierCostVersionContradiction, version)
	default:
		return RequestedSupplierExpectedCostResult{}, fmt.Errorf("%w: %s for version %s",
			ErrUnexpectedRequestedExpectedCostFormation, formed.Outcome(), version)
	}
}

// foldUndecidedFormation 把形成编排的`未决`按原因分两边：依赖答不出的仍是未决、带上根因；评价自己停下的各格是定局。
// default 报错，不静默落进任一边：形成编排日后多一个原因时要有人来定它归哪边——那一边决定的是烧不烧重投预算。
func foldUndecidedFormation(
	formed ExpectedCostResult,
	version domain.SupplierCostVersionID,
) (RequestedSupplierExpectedCostResult, error) {
	switch formed.UndecidedReason() {
	case EvaluationViewUnavailable, ExpectedCostRegistryUnavailable, OccurrenceInvalidationNotRecorded:
		return RequestedSupplierExpectedCostResult{
			outcome:   RequestedSupplierExpectedCostUndecided,
			formation: formed,
			reason:    formed.UndecidedReason(),
			cause:     formed.UndecidedCause(),
		}, nil
	case EvaluationPending:
		return notAdopted(formed, pendingJudgmentCauseOf(formed.PendingCause())), nil
	case EvaluationNotFormed:
		return notAdopted(formed, domain.StopEvaluationFailed), nil
	case ConversionStepMissing:
		return notAdopted(formed, domain.StopConversionStepMissing), nil
	case EvaluationAmountPrecisionUndeclared:
		return notAdopted(formed, domain.StopAmountRoundingUndeclared), nil
	default:
		return RequestedSupplierExpectedCostResult{}, fmt.Errorf("%w: undecided reason %q for version %s",
			ErrUnexpectedRequestedExpectedCostFormation, formed.UndecidedReason(), version)
	}
}

func notAdopted(formed ExpectedCostResult, cause domain.EvaluationStopCause) RequestedSupplierExpectedCostResult {
	return RequestedSupplierExpectedCostResult{
		outcome:   RequestedSupplierExpectedCostEvaluationNotAdopted,
		formation: formed,
		stopCause: cause,
	}
}

// zeroTotal 在请求册上记`负向结束（零额）`。记不下是依赖答不出，答未决整笔回滚；册上已有停因是两侧说法矛盾，作错误上抛。
func (handler *FormRequestedSupplierExpectedCostHandler) zeroTotal(
	ctx context.Context,
	key ports.EvaluationRequestKey,
	formed ExpectedCostResult,
) (RequestedSupplierExpectedCostResult, error) {
	effect, err := handler.deps.ZeroTotals.RecordZeroTotal(ctx, key)
	if err != nil {
		return RequestedSupplierExpectedCostResult{
			outcome: RequestedSupplierExpectedCostUndecided, formation: formed,
			reason: BuyEvaluationRequestRegisterUnavailable, cause: err,
		}, nil
	}
	switch effect {
	case ports.EvaluationRequestZeroTotalRecorded, ports.EvaluationRequestZeroTotalReplayed:
		return RequestedSupplierExpectedCostResult{outcome: RequestedSupplierExpectedCostZeroTotal, formation: formed}, nil
	case ports.EvaluationRequestAlreadyStopped:
		return RequestedSupplierExpectedCostResult{}, fmt.Errorf("%w: request %s", ErrBuyEvaluationRequestStoppedBeforeZeroTotal, key.Request)
	default:
		return RequestedSupplierExpectedCostResult{}, fmt.Errorf("%w: zero total effect %d for request %s",
			ErrUnexpectedRequestedExpectedCostFormation, effect, key.Request)
	}
}

// stop 在请求册上记停因。记不下是依赖答不出，答未决整笔回滚；册上已是另一个停因是两侧说法矛盾——同一份评价折出的停因
// 是确定的——作错误上抛。
func (handler *FormRequestedSupplierExpectedCostHandler) stop(
	ctx context.Context,
	key ports.EvaluationRequestKey,
	formed ExpectedCostResult,
	cause domain.EvaluationStopCause,
) (RequestedSupplierExpectedCostResult, error) {
	effect, err := handler.deps.Stops.RecordStop(ctx, key, cause, handler.deps.Clock.Now())
	if err != nil {
		return RequestedSupplierExpectedCostResult{
			outcome: RequestedSupplierExpectedCostUndecided, formation: formed,
			reason: BuyEvaluationRequestRegisterUnavailable, cause: err,
		}, nil
	}
	switch effect {
	case ports.EvaluationRequestStopped, ports.EvaluationRequestStopReplayed:
		return notAdopted(formed, cause), nil
	case ports.EvaluationRequestStoppedOtherwise:
		return RequestedSupplierExpectedCostResult{}, fmt.Errorf("%w: request %s, wanted %s",
			ErrBuyEvaluationRequestStoppedOtherwise, key.Request, cause)
	default:
		return RequestedSupplierExpectedCostResult{}, fmt.Errorf("%w: stop effect %d for request %s",
			ErrUnexpectedRequestedExpectedCostFormation, effect, key.Request)
	}
}
