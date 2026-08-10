package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// ActiveRejectionOutcome 是主动拒绝这一步的应用处理结果。`未获授权`单列而不是并进未决：
// 未决是「还不知道」，未获授权是「知道了，不行」，两者的后续动作相反——一个该重试，一个
// 该停手并去补授权。
type ActiveRejectionOutcome uint8

const (
	ActiveRejectionOutcomeInvalid ActiveRejectionOutcome = iota
	ActiveRejectionFormed
	ActiveRejectionNotAuthorized
	ActiveRejectionUndecided
)

func (outcome ActiveRejectionOutcome) String() string {
	switch outcome {
	case ActiveRejectionFormed:
		return "FORMED"
	case ActiveRejectionNotAuthorized:
		return "NOT_AUTHORIZED"
	case ActiveRejectionUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

// RejectShipmentRequestCommand 携带谁、以什么结构化原因、凭什么证据要拒这一单。它不带授权
// 引用：那由 party-commercial 签发，本编排去问，不由调用方声明。
type RejectShipmentRequestCommand struct {
	Identity          domain.SourceIdentity
	ShipmentRequestID domain.ShipmentRequestID
	SubmissionVersion domain.SubmissionVersionID
	Decider           domain.DeciderReference
	Reason            domain.RejectionReasonReference
	Evidence          domain.RejectionEvidenceReference
}

type RejectShipmentRequestResult struct {
	outcome      ActiveRejectionOutcome
	state        domain.ShipmentRequestState
	decision     domain.AcceptanceDecision
	hasDecision  bool
	reason       JudgmentPendingReason
	continuation domain.OwnershipContinuationReference
	compensation domain.OwnershipContinuationReference
}

func (result RejectShipmentRequestResult) Outcome() ActiveRejectionOutcome {
	return result.outcome
}

func (result RejectShipmentRequestResult) State() domain.ShipmentRequestState {
	return result.state
}

// AcceptanceDecision 交回该提交版本上的决定。主动拒绝撞上一个已成立的决定时交回的是**那一个**
// ——可能是接受，也可能是先到的另一次拒绝；用例要求后到者只能读取既有结果。
func (result RejectShipmentRequestResult) AcceptanceDecision() (domain.AcceptanceDecision, bool) {
	return result.decision, result.hasDecision
}

func (result RejectShipmentRequestResult) PendingReason() JudgmentPendingReason {
	return result.reason
}

func (result RejectShipmentRequestResult) ContinuationReference() domain.OwnershipContinuationReference {
	return result.continuation
}

func (result RejectShipmentRequestResult) CompensationReference() domain.OwnershipContinuationReference {
	return result.compensation
}

type RejectShipmentRequestDeps struct {
	Requests   ports.ShipmentRequestRepository
	Authorizer ports.ActiveRejectionAuthorizer
	Judgments  ports.RecordedJudgmentReader
	Recorder   ports.AcceptanceJudgmentRecorder
	Release    ports.PreAcceptanceControlRelease
	Identities ports.AcceptanceDecisionIdentity
	Clock      ports.Clock
}

type RejectShipmentRequestHandler struct {
	deps RejectShipmentRequestDeps
}

func NewRejectShipmentRequestHandler(deps RejectShipmentRequestDeps) *RejectShipmentRequestHandler {
	return &RejectShipmentRequestHandler{deps: deps}
}

// Handle 让授权角色对一份仍为`已提交`的委托形成主动拒绝。
//
// 授权先于一切写动作：未获授权时既不签发决定标识，也不碰聚合。顺序反过来的话，一次未获授权
// 的尝试仍会消耗一个决定标识，而标识是本上下文签发的稀缺身份。
//
// 它与自动接受竞争同一个决定提交边界，所以撞上已成立的决定时交回那一个，而不是报错重试——
// 用例要求后到者只能读取既有结果。
func (handler *RejectShipmentRequestHandler) Handle(
	ctx context.Context,
	command RejectShipmentRequestCommand,
) (RejectShipmentRequestResult, error) {
	request, found, err := handler.deps.Requests.FindBySourceIdentity(ctx, command.Identity)
	if err != nil {
		return handler.undecided(ctx, command, ShipmentRequestUnavailable), nil
	}
	if !found {
		return RejectShipmentRequestResult{}, fmt.Errorf("reject shipment request: %w", domain.ErrInvalidShipmentRequest)
	}

	authority, err := handler.deps.Authorizer.AuthorizeActiveRejection(ctx, ports.ActiveRejectionAuthorizationQuery{
		Identity:          command.Identity,
		ShipmentRequestID: command.ShipmentRequestID,
		SubmissionVersion: command.SubmissionVersion,
		Decider:           command.Decider,
		Reason:            command.Reason,
	})
	if err != nil {
		return handler.undecided(ctx, command, RejectionAuthorityUnavailable), nil
	}
	if authority.String() == "" {
		// 未获授权不是未决：它是一个确定的业务答案，续办也补不出授权来。
		return RejectShipmentRequestResult{
			outcome: ActiveRejectionNotAuthorized,
			state:   request.State(),
		}, nil
	}

	decisionID, err := handler.deps.Identities.NextAcceptanceDecisionID(ctx)
	if err != nil {
		return handler.undecided(ctx, command, DecisionIdentityUnavailable), nil
	}

	rejected, err := request.RejectByAuthority(domain.ActiveRejectionSpec{
		DecisionID: decisionID,
		Authority:  authority,
		Decider:    command.Decider,
		Reason:     command.Reason,
		Evidence:   command.Evidence,
		DecidedAt:  handler.deps.Clock.Now(),
	})
	if err != nil {
		if errors.Is(err, domain.ErrDecisionAlreadyFormed) {
			decision, formed := request.AcceptanceDecision()
			return RejectShipmentRequestResult{
				outcome:     ActiveRejectionFormed,
				state:       request.State(),
				decision:    decision,
				hasDecision: formed,
			}, nil
		}
		return RejectShipmentRequestResult{}, fmt.Errorf("reject by authority: %w", err)
	}

	if err := handler.deps.Requests.Save(ctx, command.Identity, rejected); err != nil {
		return handler.undecided(ctx, command, DecisionNotRecorded), nil
	}

	decision, _ := rejected.AcceptanceDecision()
	return RejectShipmentRequestResult{
		outcome:      ActiveRejectionFormed,
		state:        rejected.State(),
		decision:     decision,
		hasDecision:  true,
		compensation: handler.releaseFreeze(ctx, command),
	}, nil
}

// releaseFreeze 在主动拒绝越过提交边界后按原关联解除资金控制。主动拒绝同样是「接受确定未
// 成立」，冻结不能因为拒绝出自运营之手就留在原处占着货主的钱。
//
// 读不回已记录的判断时不发释放：不知道关联就发，settlement-accounting 无从认领哪一笔。这一
// 轮交回补偿续办引用，由续办去补。
func (handler *RejectShipmentRequestHandler) releaseFreeze(
	ctx context.Context,
	command RejectShipmentRequestCommand,
) domain.OwnershipContinuationReference {
	recorded, err := handler.deps.Judgments.LoadRecordedJudgments(ctx, command.ShipmentRequestID)
	if err != nil {
		return handler.compensationReference(command, RecordedJudgmentsUnavailable)
	}
	if recorded.FinancialControl.Outcome() != domain.FinancialControlHeld {
		return domain.OwnershipContinuationReference{}
	}

	if err := handler.deps.Release.ReleasePreAcceptanceControl(ctx, ports.ControlReleaseRequest{
		Identity:          command.Identity,
		ShipmentRequestID: command.ShipmentRequestID,
		SubmissionVersion: command.SubmissionVersion,
		ControlResultID:   recorded.FinancialControl.ResultID(),
	}); err != nil {
		return handler.compensationReference(command, ControlReleasePending)
	}
	return domain.OwnershipContinuationReference{}
}

func (handler *RejectShipmentRequestHandler) compensationReference(
	command RejectShipmentRequestCommand,
	reason JudgmentPendingReason,
) domain.OwnershipContinuationReference {
	return judgmentContinuation(
		reason,
		command.Identity.TenantID().String(),
		command.Identity.CustomerAccountID().String(),
		command.ShipmentRequestID.String(),
		command.SubmissionVersion.String(),
	)
}

func (handler *RejectShipmentRequestHandler) undecided(
	ctx context.Context,
	command RejectShipmentRequestCommand,
	reason JudgmentPendingReason,
) RejectShipmentRequestResult {
	continuation := judgmentContinuation(
		reason,
		command.Identity.TenantID().String(),
		command.Identity.CustomerAccountID().String(),
		command.ShipmentRequestID.String(),
		command.SubmissionVersion.String(),
	)
	recordAttempt(ctx, handler.deps.Recorder, handler.deps.Clock, command.ShipmentRequestID, reason, continuation)

	return RejectShipmentRequestResult{
		outcome:      ActiveRejectionUndecided,
		state:        domain.ShipmentRequestSubmitted,
		reason:       reason,
		continuation: continuation,
	}
}
