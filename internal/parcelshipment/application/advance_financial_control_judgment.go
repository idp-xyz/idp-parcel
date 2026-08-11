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
	commercial ports.CommercialBasisResolver
	controller ports.PreAcceptanceFinancialController
	recorder   ports.AcceptanceJudgmentRecorder
	clock      ports.Clock
}

// 时钟只用于处理尝试的发生时间。它与判断时点分开：后者由规则包声明的策略形成，本地时钟
// 顶替它就是用例禁止的「用一个全局时间代替不同判断」。
func NewAdvanceFinancialControlJudgmentHandler(
	commercial ports.CommercialBasisResolver,
	controller ports.PreAcceptanceFinancialController,
	recorder ports.AcceptanceJudgmentRecorder,
	clock ports.Clock,
) *AdvanceFinancialControlJudgmentHandler {
	return &AdvanceFinancialControlJudgmentHandler{
		commercial: commercial,
		controller: controller,
		recorder:   recorder,
		clock:      clock,
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
		return handler.undecided(ctx, command, stall.reason, stall.scope...), nil
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
	// 控制结果没能记到任务上就不算推进。交回一条没记下的控制，接受那一步会引用一次查不
	// 回来的资金占用。
	if err := handler.recorder.RecordFinancialControlResult(ctx, command.ShipmentRequestID, control); err != nil {
		return handler.undecided(ctx, command, JudgmentNotRecorded), nil
	}

	return AdvanceFinancialControlJudgmentResult{
		outcome:    AcceptanceJudgmentAdvanced,
		control:    control,
		hasControl: true,
	}, nil
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
	recordAttempt(ctx, handler.recorder, handler.clock, command.ShipmentRequestID, reason, continuation)

	return AdvanceFinancialControlJudgmentResult{
		outcome:      AcceptanceJudgmentUndecided,
		reason:       reason,
		continuation: continuation,
	}
}
