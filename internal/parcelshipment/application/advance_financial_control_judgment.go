package application

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// AdvanceFinancialControlJudgmentCommand 按当前提交版本推进，不带声明包裹：接受前财务
// 控制作用在整份委托上，逐成员发起会把一份委托的资金占用重复成成员份数。
type AdvanceFinancialControlJudgmentCommand struct {
	Identity          domain.SourceIdentity
	ShipmentRequestID domain.ShipmentRequestID
	SubmissionVersion domain.SubmissionVersionID
}

type AdvanceFinancialControlJudgmentResult struct {
	outcome      AcceptanceJudgmentOutcome
	control      domain.FinancialControlResult
	hasControl   bool
	reason       JudgmentPendingReason
	continuation domain.OwnershipContinuationReference
}

func (result AdvanceFinancialControlJudgmentResult) Outcome() AcceptanceJudgmentOutcome {
	return result.outcome
}

// PendingReason 说明本轮停在哪一步。用例要求`尚未决定`返回未决原因，而不是只给一个续办
// 引用：调用方据原因决定该补缺口还是该重试依赖。
func (result AdvanceFinancialControlJudgmentResult) PendingReason() JudgmentPendingReason {
	return result.reason
}

// FinancialControlResult 只在控制实际形成时给出。未决时交回一个零值结果，下游会读到一个
// 既非`已冻结`也非`明确无控制`的空结论，而空结论最容易被当成没有障碍。
func (result AdvanceFinancialControlJudgmentResult) FinancialControlResult() (domain.FinancialControlResult, bool) {
	return result.control, result.hasControl
}

func (result AdvanceFinancialControlJudgmentResult) ContinuationReference() domain.OwnershipContinuationReference {
	return result.continuation
}

// State 报告本轮之后委托的生命周期状态，恒为`已提交`：`业务限制`是本步取得的一项判断，
// 据不据它拒绝由接受决定那一步回答。
func (result AdvanceFinancialControlJudgmentResult) State() domain.ShipmentRequestState {
	return domain.ShipmentRequestSubmitted
}

// financialControlAsOfReasons 是接受前财务控制在第二阶段四种未成形上的未决原因。与可达性
// 那一套分开：两者停在同一阶段时续办引用要分得开，否则一次财务侧的停顿会被按可达性去续办。
var financialControlAsOfReasons = asOfPendingReasons{
	basisNotResolved: FinancialControlAsOfBasisNotResolved,
	notConfigured:    FinancialControlAsOfNotConfigured,
	unavailable:      FinancialControlAsOfUnavailable,
	valueRejected:    FinancialControlAsOfValueRejected,
	inputNotAccepted: FinancialControlAsOfInputNotAccepted,
}

// controlStallReasons 与可达性那张表同一机制。这里尤其要紧：一次`请求冲突`若被当成故障
// 重试，被重试的是一次占用客户资金的请求。
var controlStallReasons = map[ports.PreAcceptanceControlOutcome]JudgmentPendingReason{
	ports.PreAcceptanceControlNotFormed:          FinancialControlNotFormed,
	ports.PreAcceptanceControlRequestConflict:    FinancialControlRequestConflict,
	ports.PreAcceptanceControlRequestNotAccepted: FinancialControlRequestNotAccepted,
}

type AdvanceFinancialControlJudgmentHandler struct {
	commercial   ports.CommercialBasisResolver
	controller   ports.PreAcceptanceFinancialController
	dispositions ports.ControlDispositionView
	recorder     ports.AcceptanceJudgmentRecorder
	requests     ports.ShipmentRequestRepository
	clock        ports.Clock
}

// 时钟只用于处理尝试的发生时间。它与判断时点分开：后者由规则包声明的策略形成，本地时钟
// 顶替它就是用例禁止的「用一个全局时间代替不同判断」。
//
// requests 的用处与可达性那一支相同：只在`判断时点未配置`那一停上把`等待运营登记`落库
// （ADR-0094 Decision 五），控制结果仍经 recorder 记到任务上。
//
// dispositions 只在 settlement-accounting 交回含受限项的结果时被问（ADR-0132 决定三）：读回受限项
// 在策略正文里登记的失败处置与责任引用，作为采用引用记在受限项上、随控制结果落库。它是形成控制
// 判断这一步的一部分而不是形成决定那一步的：处置是这份判断当时按哪一版正文形成的一部分，Decide 时
// 再读正文会让策略换版改写一份已形成的判断。
func NewAdvanceFinancialControlJudgmentHandler(
	commercial ports.CommercialBasisResolver,
	controller ports.PreAcceptanceFinancialController,
	dispositions ports.ControlDispositionView,
	recorder ports.AcceptanceJudgmentRecorder,
	requests ports.ShipmentRequestRepository,
	clock ports.Clock,
) *AdvanceFinancialControlJudgmentHandler {
	return &AdvanceFinancialControlJudgmentHandler{
		commercial:   commercial,
		controller:   controller,
		dispositions: dispositions,
		recorder:     recorder,
		requests:     requests,
		clock:        clock,
	}
}

// Handle 把接受判断任务的财务控制那条腿推进一步：采用唯一商业依据，按该依据为本类判断
// 声明的策略形成时点，再记录 settlement-accounting 返回的控制结果。
//
// 它不形成接受或拒绝，也不在任何一步替控制作答。没有唯一依据、或本类判断没有被声明时点
// 时，它在发起控制之前停下并保持可续办——用例对本步的要求是不得默认放行，而一次在无人
// 授权的时点上发出的资金占用，既收不回来也解释不了自己按哪一版策略执行。
//
// 控制端口答不出同样形成未决而不上抛，未决原因指名是这个依赖停了。它与`明确无控制`因此
// 分得开：后者是合同声明并带商业依据，前者是问过了没答案。
func (handler *AdvanceFinancialControlJudgmentHandler) Handle(
	ctx context.Context,
	command AdvanceFinancialControlJudgmentCommand,
) (AdvanceFinancialControlJudgmentResult, error) {
	// 前半段与可达性那一支共用：任何一处停下都不发起控制——一次没有适用合同的范围，没有
	// 理由去占用这个客户的资金；一个本方自造的时刻上发出的占用，既收不回来也解释不了按哪
	// 一版策略执行。
	adopted, stall, err := formAdoptedBasis(
		ctx,
		handler.commercial,
		handler.recorder,
		commercialBasisScope{
			Identity:          command.Identity,
			ShipmentRequestID: command.ShipmentRequestID,
			SubmissionVersion: command.SubmissionVersion,
		},
		domain.FinancialControlJudgmentKind,
		FinancialControlAsOfNotDeclared,
		financialControlAsOfReasons,
	)
	if err != nil {
		return AdvanceFinancialControlJudgmentResult{}, err
	}
	if stall.stopped() {
		reason, err := awaitOperatorRegistration(ctx, handler.requests, command.Identity, stall.reason)
		if err != nil {
			return AdvanceFinancialControlJudgmentResult{}, err
		}
		scope := stall.scope
		if reason != stall.reason {
			// 理由同可达性那一支：原因换了，范围随之放下。
			scope = nil
		}
		return handler.undecided(ctx, command, reason, scope...), nil
	}

	assessment, err := handler.controller.ApplyPreAcceptanceFinancialControl(ctx, ports.FinancialControlRequest{
		Identity:          command.Identity,
		ShipmentRequestID: command.ShipmentRequestID,
		SubmissionVersion: command.SubmissionVersion,
		AsOf:              adopted.asOf,
	})
	if err != nil {
		return handler.undecided(ctx, command, FinancialControlUnavailable), nil
	}
	if assessment.Outcome != ports.PreAcceptanceControlFormed {
		reason, ok := controlStallReasons[assessment.Outcome]
		if !ok {
			return AdvanceFinancialControlJudgmentResult{}, ErrUnexpectedAssessmentOutcome
		}
		return handler.undecided(ctx, command, reason), nil
	}
	control := assessment.Result
	if control.Outcome() == domain.FinancialControlRestricted {
		// 有项受限才读处置：成立与`明确无控制`都没有去向可问。读到的处置在记录之前采用到受限项上，
		// 因为它们要随同一份结果落库；读不到就停在这里不记录——记一份不带处置的受限结果，接受那一步
		// 会按 ADR-0125 的过渡口径把正文登了`进入授权处置`的合同也拒掉。
		adopted, stall := handler.adoptDispositions(ctx, command, adoptedBasisOf(adopted), control)
		if stall != PendingReasonNone {
			return handler.undecided(ctx, command, stall), nil
		}
		control = adopted
	}
	// 控制结果没能记到任务上就不算推进。交回一条没记下的控制，接受那一步会引用一次查不
	// 回来的资金占用。
	if err := handler.recorder.RecordFinancialControlResult(
		ctx, command.Identity.TenantID(), command.ShipmentRequestID, command.SubmissionVersion, control); err != nil {
		return handler.undecided(ctx, command, JudgmentNotRecorded), nil
	}

	return AdvanceFinancialControlJudgmentResult{
		outcome:    AcceptanceJudgmentAdvanced,
		control:    control,
		hasControl: true,
	}, nil
}

// adoptedBasisOf 取本轮采用的商业解析标识——处置读口凭它回指闭包，与 SA 执行控制时读的是同一份。
func adoptedBasisOf(adopted adoptedBasis) domain.CommercialResolutionID {
	return adopted.snapshot.ResolutionID()
}

// adoptDispositions 经本上下文自己的商业缝读受限项的失败处置与责任引用，采用到受限项上（ADR-0132 决定三）。
//
// 两种停法各归一格：读口调不通是`读口答不出`（等它恢复）；正文那一侧没有可读的行、或范围下缺受限项
// 那一种类的行以致领域采用不成立是`未形成`——SA 刚按同一份正文执行完控制，这只可能是换版竞争或坏数据，
// 恢复动作是重读，**不折成任一去向**：折成 REJECT 是替租户拒单，折成授权处置是凭空给一项`业务限制`开
// 一条人工的路。
func (handler *AdvanceFinancialControlJudgmentHandler) adoptDispositions(
	ctx context.Context,
	command AdvanceFinancialControlJudgmentCommand,
	resolution domain.CommercialResolutionID,
	control domain.FinancialControlResult,
) (domain.FinancialControlResult, JudgmentPendingReason) {
	dispositions, found, err := handler.dispositions.LoadControlDispositions(ctx, ports.ControlDispositionQuery{
		Identity:   command.Identity,
		Resolution: resolution,
	})
	if err != nil {
		return domain.FinancialControlResult{}, ControlDispositionUnavailable
	}
	if !found {
		return domain.FinancialControlResult{}, ControlDispositionNotFormed
	}
	adopted, err := control.AdoptControlDispositions(dispositions)
	if err != nil {
		return domain.FinancialControlResult{}, ControlDispositionNotFormed
	}
	return adopted, PendingReasonNone
}

// undecided 的 scope 与可达性那一支同义：同一未决原因下的两种缺口靠提供方的原因引用分开。
func (handler *AdvanceFinancialControlJudgmentHandler) undecided(
	ctx context.Context,
	command AdvanceFinancialControlJudgmentCommand,
	reason JudgmentPendingReason,
	scope ...string,
) AdvanceFinancialControlJudgmentResult {
	continuation := judgmentContinuation(
		reason,
		append([]string{
			command.Identity.TenantID().String(),
			command.Identity.CustomerAccountID().String(),
			command.ShipmentRequestID.String(),
			command.SubmissionVersion.String(),
		}, scope...)...,
	)
	recordAttempt(ctx, handler.recorder, handler.clock,
		command.Identity.TenantID(), command.ShipmentRequestID, reason, continuation)

	return AdvanceFinancialControlJudgmentResult{
		outcome:      AcceptanceJudgmentUndecided,
		reason:       reason,
		continuation: continuation,
	}
}
