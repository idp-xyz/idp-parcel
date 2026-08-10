package application

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// AcceptanceJudgmentOutcome 是接受判断任务推进一轮的应用处理结果。两个取值都不是接受
// 或拒绝：任务保持可续办，委托保持`已提交`。
type AcceptanceJudgmentOutcome uint8

const (
	AcceptanceJudgmentOutcomeInvalid AcceptanceJudgmentOutcome = iota
	AcceptanceJudgmentAdvanced
	AcceptanceJudgmentUndecided
)

func (outcome AcceptanceJudgmentOutcome) String() string {
	switch outcome {
	case AcceptanceJudgmentAdvanced:
		return "ADVANCED"
	case AcceptanceJudgmentUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

type AdvanceAcceptanceJudgmentCommand struct {
	Identity          domain.SourceIdentity
	ShipmentRequestID domain.ShipmentRequestID
	SubmissionVersion domain.SubmissionVersionID
	DeclaredParcelID  domain.DeclaredParcelID
}

type AdvanceAcceptanceJudgmentResult struct {
	outcome      AcceptanceJudgmentOutcome
	judgment     domain.ReachabilityJudgment
	hasJudgment  bool
	reason       JudgmentPendingReason
	continuation domain.OwnershipContinuationReference
}

func (result AdvanceAcceptanceJudgmentResult) Outcome() AcceptanceJudgmentOutcome {
	return result.outcome
}

// PendingReason 说明本轮停在哪一步。用例要求`尚未决定`返回未决原因，而不是只给一个续办
// 引用：调用方据原因决定该补缺口还是该重试依赖。
func (result AdvanceAcceptanceJudgmentResult) PendingReason() JudgmentPendingReason {
	return result.reason
}

func (result AdvanceAcceptanceJudgmentResult) ReachabilityJudgment() (domain.ReachabilityJudgment, bool) {
	return result.judgment, result.hasJudgment
}

func (result AdvanceAcceptanceJudgmentResult) ContinuationReference() domain.OwnershipContinuationReference {
	return result.continuation
}

// State 报告本轮之后委托的生命周期状态，恒为`已提交`：接受判断任务推进了一步——无论
// 判断结果如何——都不是接受也不是拒绝。
func (result AdvanceAcceptanceJudgmentResult) State() domain.ShipmentRequestState {
	return domain.ShipmentRequestSubmitted
}

type AdvanceAcceptanceJudgmentHandler struct {
	commercial   ports.CommercialBasisResolver
	reachability ports.ReachabilityAssessor
	recorder     ports.AcceptanceJudgmentRecorder
	clock        ports.Clock
}

func NewAdvanceAcceptanceJudgmentHandler(
	commercial ports.CommercialBasisResolver,
	reachability ports.ReachabilityAssessor,
	recorder ports.AcceptanceJudgmentRecorder,
	clock ports.Clock,
) *AdvanceAcceptanceJudgmentHandler {
	return &AdvanceAcceptanceJudgmentHandler{
		commercial:   commercial,
		reachability: reachability,
		recorder:     recorder,
		clock:        clock,
	}
}

// Handle 把一个接受判断任务推进一步：采用唯一商业依据，按该依据声明的策略形成可达性
// 判断时点，再记录路由权威返回的判断。
//
// 它不形成接受或拒绝，也绝不拿自己的时钟顶替声明的时点。没有唯一依据、本类判断没有被
// 声明时点、或者依赖答不出时，它停下并保持可续办，而不是在一个无人授权的时刻上继续。
//
// 依赖答不出形成未决而不上抛：用例把`依赖不可用`列为`尚未决定`的成因，并要求该结果带上
// 未决原因与安全续办引用。未决原因指名是哪个依赖停了，因此「权威答不出」与「权威答了
// 不可达」仍然分得开——这正是本步绝不形成判断的原因。
func (handler *AdvanceAcceptanceJudgmentHandler) Handle(
	ctx context.Context,
	command AdvanceAcceptanceJudgmentCommand,
) (AdvanceAcceptanceJudgmentResult, error) {
	resolution, err := handler.commercial.ResolveCommercialBasis(ctx, ports.CommercialBasisQuery{
		Identity:          command.Identity,
		ShipmentRequestID: command.ShipmentRequestID,
		SubmissionVersion: command.SubmissionVersion,
	})
	if err != nil {
		return handler.undecided(ctx, command, CommercialBasisUnavailable), nil
	}
	// 本步只推进判断，不形成决定，因此`确定不适用`与`解析未决`在这里同样停下：据不据一次
	// 确定性商业失败拒绝，由形成决定那一步回答。
	if resolution.Applicability != domain.CommerciallyApplicable {
		return handler.undecided(ctx, command, CommercialBasisNotUnique), nil
	}
	basis := resolution.Snapshot

	asOf, declared := basis.AsOfFor(domain.ReachabilityJudgmentKind)
	if !declared {
		return handler.undecided(ctx, command, ReachabilityAsOfNotDeclared), nil
	}

	judgment, err := handler.reachability.AssessParcelReachability(ctx, ports.ReachabilityRequest{
		Identity:          command.Identity,
		ShipmentRequestID: command.ShipmentRequestID,
		SubmissionVersion: command.SubmissionVersion,
		DeclaredParcelID:  command.DeclaredParcelID,
		AsOf:              asOf,
	})
	if err != nil {
		return handler.undecided(ctx, command, ReachabilityAuthorityUnavailable), nil
	}
	// 判断没能记到任务上就不算推进。交回一个没记下的判断，接受那一步会引用一条查不回来
	// 的依据。
	if err := handler.recorder.RecordReachabilityJudgment(ctx, command.ShipmentRequestID, judgment); err != nil {
		return handler.undecided(ctx, command, JudgmentNotRecorded), nil
	}

	return AdvanceAcceptanceJudgmentResult{
		outcome:     AcceptanceJudgmentAdvanced,
		judgment:    judgment,
		hasJudgment: true,
	}, nil
}

func (handler *AdvanceAcceptanceJudgmentHandler) undecided(
	ctx context.Context,
	command AdvanceAcceptanceJudgmentCommand,
	reason JudgmentPendingReason,
) AdvanceAcceptanceJudgmentResult {
	continuation := judgmentContinuation(
		reason,
		command.Identity.TenantID().String(),
		command.Identity.CustomerAccountID().String(),
		command.ShipmentRequestID.String(),
		command.SubmissionVersion.String(),
		command.DeclaredParcelID.String(),
	)
	recordAttempt(ctx, handler.recorder, handler.clock, command.ShipmentRequestID, reason, continuation)

	return AdvanceAcceptanceJudgmentResult{
		outcome:      AcceptanceJudgmentUndecided,
		reason:       reason,
		continuation: continuation,
	}
}
