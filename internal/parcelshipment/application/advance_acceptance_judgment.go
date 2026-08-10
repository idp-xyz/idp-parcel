package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// CommercialBasisOutcome 报告是否采用了唯一商业依据。parcel-shipment 刻意不镜像
// party-commercial 的完整结果代数：无适用依据、适用冲突与解析未决之间的区分属那个
// 上下文的语言，复制过来就等于在两处维护同一套口径。
type CommercialBasisOutcome uint8

const (
	CommercialBasisOutcomeInvalid CommercialBasisOutcome = iota
	CommercialBasisUnique
	CommercialBasisNotApplicable
	CommercialBasisConflict
	CommercialBasisPending
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
// 它不形成接受或拒绝，也绝不拿自己的时钟顶替声明的时点。没有唯一依据、或本类判断没有
// 被声明时点时，它停下并保持可续办，而不是在一个无人授权的时刻上继续。
func (handler *AdvanceAcceptanceJudgmentHandler) Handle(
	ctx context.Context,
	command AdvanceAcceptanceJudgmentCommand,
) (AdvanceAcceptanceJudgmentResult, error) {
	basis, err := handler.commercial.ResolveCommercialBasis(ctx, ports.CommercialBasisQuery{
		Identity:          command.Identity,
		ShipmentRequestID: command.ShipmentRequestID,
		SubmissionVersion: command.SubmissionVersion,
	})
	if err != nil {
		return AdvanceAcceptanceJudgmentResult{}, fmt.Errorf("resolve commercial basis: %w", err)
	}
	if basis.ResolutionID().String() == "" {
		return handler.undecided(command, CommercialBasisNotUnique), nil
	}

	asOf, declared := basis.AsOfFor(domain.ReachabilityJudgmentKind)
	if !declared {
		return handler.undecided(command, ReachabilityAsOfNotDeclared), nil
	}

	judgment, err := handler.reachability.AssessParcelReachability(ctx, ports.ReachabilityRequest{
		Identity:          command.Identity,
		ShipmentRequestID: command.ShipmentRequestID,
		SubmissionVersion: command.SubmissionVersion,
		DeclaredParcelID:  command.DeclaredParcelID,
		AsOf:              asOf,
	})
	if err != nil {
		return AdvanceAcceptanceJudgmentResult{}, fmt.Errorf("assess parcel reachability: %w", err)
	}
	if err := handler.recorder.RecordReachabilityJudgment(ctx, command.ShipmentRequestID, judgment); err != nil {
		return AdvanceAcceptanceJudgmentResult{}, fmt.Errorf("record reachability judgment: %w", err)
	}

	return AdvanceAcceptanceJudgmentResult{
		outcome:     AcceptanceJudgmentAdvanced,
		judgment:    judgment,
		hasJudgment: true,
	}, nil
}

func (handler *AdvanceAcceptanceJudgmentHandler) undecided(
	command AdvanceAcceptanceJudgmentCommand,
	reason JudgmentPendingReason,
) AdvanceAcceptanceJudgmentResult {
	return AdvanceAcceptanceJudgmentResult{
		outcome: AcceptanceJudgmentUndecided,
		reason:  reason,
		continuation: judgmentContinuation(
			reason,
			command.Identity.TenantID().String(),
			command.Identity.CustomerAccountID().String(),
			command.ShipmentRequestID.String(),
			command.SubmissionVersion.String(),
			command.DeclaredParcelID.String(),
		),
	}
}
