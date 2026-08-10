package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// AcceptanceDecisionOutcome 是形成接受决定这一步的应用处理结果。它只有两个取值：形成了
// 决定，或者本轮没形成。接受与拒绝的区别在委托的生命周期状态里，不在这里再复制一份——
// 复制过来就得靠两处判断回答「到底决定了没有」。
type AcceptanceDecisionOutcome uint8

const (
	AcceptanceDecisionOutcomeInvalid AcceptanceDecisionOutcome = iota
	AcceptanceDecided
	AcceptanceUndecided
)

func (outcome AcceptanceDecisionOutcome) String() string {
	switch outcome {
	case AcceptanceDecided:
		return "DECIDED"
	case AcceptanceUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

type FormAcceptanceDecisionCommand struct {
	Identity          domain.SourceIdentity
	ShipmentRequestID domain.ShipmentRequestID
	SubmissionVersion domain.SubmissionVersionID
}

type FormAcceptanceDecisionResult struct {
	outcome      AcceptanceDecisionOutcome
	state        domain.ShipmentRequestState
	decision     domain.AcceptanceDecision
	hasDecision  bool
	reason       JudgmentPendingReason
	continuation domain.OwnershipContinuationReference
}

func (result FormAcceptanceDecisionResult) Outcome() AcceptanceDecisionOutcome {
	return result.outcome
}

// State 是本轮之后委托的生命周期状态。未形成决定时它是`已提交`——用例把`尚未决定`定为应用
// 处理结果而不是委托的新终局状态。
func (result FormAcceptanceDecisionResult) State() domain.ShipmentRequestState {
	return result.state
}

// AcceptanceDecision 只在本轮形成了决定时给出。未决时交回一个零值决定，下游会读到一份既非
// 接受也非拒绝的空决定，而空决定最容易被当成没有障碍。
func (result FormAcceptanceDecisionResult) AcceptanceDecision() (domain.AcceptanceDecision, bool) {
	return result.decision, result.hasDecision
}

func (result FormAcceptanceDecisionResult) PendingReason() JudgmentPendingReason {
	return result.reason
}

func (result FormAcceptanceDecisionResult) ContinuationReference() domain.OwnershipContinuationReference {
	return result.continuation
}

type FormAcceptanceDecisionHandler struct {
	requests   ports.ShipmentRequestRepository
	commercial ports.CommercialBasisResolver
	judgments  ports.RecordedJudgmentReader
	identities ports.AcceptanceDecisionIdentity
	clock      ports.Clock
}

func NewFormAcceptanceDecisionHandler(
	requests ports.ShipmentRequestRepository,
	commercial ports.CommercialBasisResolver,
	judgments ports.RecordedJudgmentReader,
	identities ports.AcceptanceDecisionIdentity,
	clock ports.Clock,
) *FormAcceptanceDecisionHandler {
	return &FormAcceptanceDecisionHandler{
		requests:   requests,
		commercial: commercial,
		judgments:  judgments,
		identities: identities,
		clock:      clock,
	}
}

// Handle 把接受判断任务上已采用的权威判断装配成校验结果，交给委托聚合形成一次接受、拒绝
// 或`尚未决定`。
//
// 它自己不判任何一组：三值可达性与财务控制结果由各自的权威上下文形成，本编排只做翻译与
// 装配。哪个取值算失败写在领域的翻译函数里，覆盖够不够写在聚合的 Decide 里——两者都不在
// 这里，因为它们是接受语言的一部分而不是编排顺序的一部分。
func (handler *FormAcceptanceDecisionHandler) Handle(
	ctx context.Context,
	command FormAcceptanceDecisionCommand,
) (FormAcceptanceDecisionResult, error) {
	request, found, err := handler.requests.FindBySourceIdentity(ctx, command.Identity)
	if err != nil {
		return handler.undecided(command, ShipmentRequestUnavailable), nil
	}
	if !found {
		// 命令指名了一份不存在的委托。这不是依赖答不出，而是调用方对世界的判断就是错的，
		// 因此上抛而不是形成未决。
		return FormAcceptanceDecisionResult{}, fmt.Errorf("form acceptance decision: %w", domain.ErrInvalidShipmentRequest)
	}

	basis, err := handler.commercial.ResolveCommercialBasis(ctx, ports.CommercialBasisQuery{
		Identity:          command.Identity,
		ShipmentRequestID: command.ShipmentRequestID,
		SubmissionVersion: command.SubmissionVersion,
	})
	if err != nil {
		return handler.undecided(command, CommercialBasisUnavailable), nil
	}
	if basis.ResolutionID().String() == "" {
		return handler.undecided(command, CommercialBasisNotUnique), nil
	}

	// 复核策略先于读回判断：规则包没有声明要不要复核时，这一轮无论如何都形成不了接受，
	// 再去读一遍判断是白做的。
	policy := basis.ManualReviewPolicy()
	if !policy.Declared() {
		return handler.undecided(command, ManualReviewPolicyNotDeclared), nil
	}

	recorded, err := handler.judgments.LoadRecordedJudgments(ctx, command.ShipmentRequestID)
	if err != nil {
		return handler.undecided(command, RecordedJudgmentsUnavailable), nil
	}
	checks, err := assembleChecks(recorded, basis.PendingRoutingAllowance())
	if err != nil {
		return FormAcceptanceDecisionResult{}, fmt.Errorf("assemble acceptance checks: %w", err)
	}

	decisionID, err := handler.identities.NextAcceptanceDecisionID(ctx)
	if err != nil {
		return handler.undecided(command, DecisionIdentityUnavailable), nil
	}

	decided, err := request.Decide(domain.AcceptanceDecisionSpec{
		DecisionID:   decisionID,
		Checks:       checks,
		ManualReview: manualReviewStateFor(policy),
		Basis:        basis,
		DecidedAt:    handler.clock.Now(),
	})
	if err != nil {
		// 同一提交版本已经有决定是业务答案而非故障：用例要求并发处理返回同一结果，调用方
		// 据以读取原决定，而不是当作故障重试。
		if errors.Is(err, domain.ErrDecisionAlreadyFormed) {
			return handler.existing(request), nil
		}
		return FormAcceptanceDecisionResult{}, fmt.Errorf("decide: %w", err)
	}

	decision, formed := decided.AcceptanceDecision()
	if !formed {
		// 聚合看过全部校验后仍未形成决定：有待判断的组、有未被判断的成员或适用组，或者
		// 规则要求的人工复核尚未完成。委托保持`已提交`，任务继续可续办。
		return handler.undecided(command, AcceptanceJudgmentIncomplete), nil
	}
	if err := handler.requests.Save(ctx, command.Identity, decided); err != nil {
		// 决定没能越过提交边界就不算形成。交回一个没落库的接受，下游会按一份查不回来的
		// 接受基线继续办。
		return handler.undecided(command, DecisionNotRecorded), nil
	}

	return FormAcceptanceDecisionResult{
		outcome:     AcceptanceDecided,
		state:       decided.State(),
		decision:    decision,
		hasDecision: true,
	}, nil
}

// assembleChecks 把已记录的判断逐条译成校验结果。有结果就记，没有就不记。
//
// 「控制从未形成因而不该接受」不在这里挡——聚合的适用组覆盖检查已经挡住了：本组被声明适用
// 却一项校验都没到场，它就不接受。在这里再塞一个`无法判定`是同一条规则的第二处实现，而且
// 塞错了更糟：本组没被声明适用时凭空塞一项永远满足不了的`无法判定`，会让一份合同本就不
// 要求财务控制的委托永远接受不了。
//
// 结果存在就一律记下，哪怕规则包没把本组列为适用：一次`业务限制`不该因为不在适用集合里就
// 被丢掉——声明只能增加要求，减不掉失败。
func assembleChecks(
	recorded ports.RecordedJudgments,
	pendingRouting domain.PendingRoutingAllowance,
) ([]domain.AcceptanceCheck, error) {
	checks := make([]domain.AcceptanceCheck, 0, len(recorded.Reachability)+1)
	for _, judgment := range recorded.Reachability {
		check, err := domain.ReachabilityCheckFor(judgment, pendingRouting)
		if err != nil {
			return nil, err
		}
		checks = append(checks, check)
	}

	if recorded.FinancialControl.Outcome() == domain.FinancialControlOutcomeInvalid {
		return checks, nil
	}
	control, err := domain.FinancialControlCheckFor(recorded.FinancialControl)
	if err != nil {
		return nil, err
	}
	return append(checks, control), nil
}

// manualReviewStateFor 把规则包的复核声明变成聚合要的状态。这里到不了`已完成`：完成是运营
// 发生的事，要由另一条授权路径写入，而那条路径的角色与条件仍是未确认参数（`BD-PS-002`）。
// 在这里凑一个`已完成`就等于替运营签了字。
func manualReviewStateFor(policy domain.ManualReviewPolicy) domain.ManualReviewState {
	if policy == domain.ManualReviewRequiredByRules {
		return domain.ManualReviewRequired
	}
	return domain.ManualReviewNotRequired
}

// existing 交回该提交版本已经形成的那一个决定。用例要求并发处理返回同一结果或明确冲突，
// 不能一边接受一边拒绝。
func (handler *FormAcceptanceDecisionHandler) existing(request domain.ShipmentRequest) FormAcceptanceDecisionResult {
	decision, formed := request.AcceptanceDecision()
	return FormAcceptanceDecisionResult{
		outcome:     AcceptanceDecided,
		state:       request.State(),
		decision:    decision,
		hasDecision: formed,
	}
}

func (handler *FormAcceptanceDecisionHandler) undecided(
	command FormAcceptanceDecisionCommand,
	reason JudgmentPendingReason,
) FormAcceptanceDecisionResult {
	return FormAcceptanceDecisionResult{
		outcome: AcceptanceUndecided,
		state:   domain.ShipmentRequestSubmitted,
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
