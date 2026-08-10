package application

import (
	"context"
	"fmt"

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

type AdvanceFinancialControlJudgmentHandler struct {
	commercial ports.CommercialBasisResolver
	controller ports.PreAcceptanceFinancialController
	recorder   ports.AcceptanceJudgmentRecorder
}

func NewAdvanceFinancialControlJudgmentHandler(
	commercial ports.CommercialBasisResolver,
	controller ports.PreAcceptanceFinancialController,
	recorder ports.AcceptanceJudgmentRecorder,
) *AdvanceFinancialControlJudgmentHandler {
	return &AdvanceFinancialControlJudgmentHandler{
		commercial: commercial,
		controller: controller,
		recorder:   recorder,
	}
}

// Handle 把接受判断任务的财务控制那条腿推进一步：采用唯一商业依据，按该依据为本类判断
// 声明的策略形成时点，再记录 settlement-accounting 返回的控制结果。
//
// 它不形成接受或拒绝，也不在任何一步替控制作答。没有唯一依据、或本类判断没有被声明时点
// 时，它在发起控制之前停下并保持可续办——用例对本步的要求是不得默认放行，而一次在无人
// 授权的时点上发出的资金占用，既收不回来也解释不了自己按哪一版策略执行。
func (handler *AdvanceFinancialControlJudgmentHandler) Handle(
	ctx context.Context,
	command AdvanceFinancialControlJudgmentCommand,
) (AdvanceFinancialControlJudgmentResult, error) {
	basis, err := handler.commercial.ResolveCommercialBasis(ctx, ports.CommercialBasisQuery{
		Identity:          command.Identity,
		ShipmentRequestID: command.ShipmentRequestID,
		SubmissionVersion: command.SubmissionVersion,
	})
	if err != nil {
		return AdvanceFinancialControlJudgmentResult{}, fmt.Errorf("resolve commercial basis: %w", err)
	}
	if basis.ResolutionID().String() == "" {
		return handler.undecided(command, CommercialBasisNotUnique), nil
	}

	asOf, declared := basis.AsOfFor(domain.FinancialControlJudgmentKind)
	if !declared {
		return handler.undecided(command, FinancialControlAsOfNotDeclared), nil
	}

	control, err := handler.controller.ApplyPreAcceptanceFinancialControl(ctx, ports.FinancialControlRequest{
		Identity:          command.Identity,
		ShipmentRequestID: command.ShipmentRequestID,
		SubmissionVersion: command.SubmissionVersion,
		AsOf:              asOf,
	})
	if err != nil {
		return AdvanceFinancialControlJudgmentResult{}, fmt.Errorf("apply pre-acceptance financial control: %w", err)
	}
	if err := handler.recorder.RecordFinancialControlResult(ctx, command.ShipmentRequestID, control); err != nil {
		return AdvanceFinancialControlJudgmentResult{}, fmt.Errorf("record financial control result: %w", err)
	}

	return AdvanceFinancialControlJudgmentResult{
		outcome:    AcceptanceJudgmentAdvanced,
		control:    control,
		hasControl: true,
	}, nil
}

func (handler *AdvanceFinancialControlJudgmentHandler) undecided(
	command AdvanceFinancialControlJudgmentCommand,
	reason JudgmentPendingReason,
) AdvanceFinancialControlJudgmentResult {
	return AdvanceFinancialControlJudgmentResult{
		outcome: AcceptanceJudgmentUndecided,
		reason:  reason,
		continuation: judgmentContinuation(
			reason,
			command.Identity.TenantID().String(),
			command.Identity.CustomerAccountID().String(),
			command.ShipmentRequestID.String(),
			command.SubmissionVersion.String(),
		),
	}
}
