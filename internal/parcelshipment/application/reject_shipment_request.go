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
	ActiveRejectionTaskAlreadyClosed
)

func (outcome ActiveRejectionOutcome) String() string {
	switch outcome {
	case ActiveRejectionFormed:
		return "FORMED"
	case ActiveRejectionNotAuthorized:
		return "NOT_AUTHORIZED"
	case ActiveRejectionUndecided:
		return "UNDECIDED"
	case ActiveRejectionTaskAlreadyClosed:
		return "TASK_ALREADY_CLOSED"
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
	outcome        ActiveRejectionOutcome
	state          domain.ShipmentRequestState
	decision       domain.AcceptanceDecision
	hasDecision    bool
	reason         JudgmentPendingReason
	continuation   domain.OwnershipContinuationReference
	compensation   domain.OwnershipContinuationReference
	releaseRefused bool
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

// ControlReleaseNotAccepted 报告拒绝已经成立、而随附释放里有一笔提供方确定不受理，已记进释放未受理册待人处理
// （ADR-0282 决定二）。理由同 WithdrawShipmentRequestResult 的同名方法。
func (result RejectShipmentRequestResult) ControlReleaseNotAccepted() bool {
	return result.releaseRefused
}

type RejectShipmentRequestDeps struct {
	Requests   ports.ShipmentRequestRepository
	Authorizer ports.ActiveRejectionAuthorizer
	Judgments  ports.RecordedJudgmentReader
	Recorder   ports.AcceptanceJudgmentRecorder
	Release    ports.PreAcceptanceControlRelease
	// Refusals 是释放未受理册：随附释放被提供方确定不受理时记在这里交人处理。
	Refusals ports.ControlReleaseRefusalRegister
	// ScopeLock 在保存委托之前取这份委托各版占用所在作用域的锁，锁序见 ports.PreAcceptanceControlScopeLock 头注。
	ScopeLock  ports.PreAcceptanceControlScopeLock
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
		return handler.undecided(ctx, command, ShipmentRequestUnavailable, domain.ShipmentRequestStateInvalid), nil
	}
	if !found {
		// 指名一份查不到的委托是调用方的错，不是业务结果：答`统一不可见结果`（ErrShipmentRequestNotVisible）。
		return RejectShipmentRequestResult{}, fmt.Errorf("reject shipment request: %w", ErrShipmentRequestNotVisible)
	}

	authority, err := handler.deps.Authorizer.AuthorizeActiveRejection(ctx, ports.ActiveRejectionAuthorizationQuery{
		Identity:          command.Identity,
		ShipmentRequestID: command.ShipmentRequestID,
		SubmissionVersion: command.SubmissionVersion,
		Decider:           command.Decider,
		Reason:            command.Reason,
	})
	if err != nil {
		return handler.undecided(ctx, command, RejectionAuthorityUnavailable, request.State()), nil
	}
	switch authority.Outcome {
	case ports.AuthorizationGranted:
	case ports.AuthorizationRefused:
		// 未获授权不是未决：它是一个确定的业务答案，续办也补不出授权来。
		return RejectShipmentRequestResult{
			outcome: ActiveRejectionNotAuthorized,
			state:   request.State(),
		}, nil
	case ports.AuthorizationRulesNotConfigured:
		// 运营侧拒绝权同样是 `PAR-COM-14` 待提供的实例参数。一条规则都没登记时说不出
		// 「这个人不能拒」——那句话要有一份规则作依据才成立。
		return handler.undecided(ctx, command, RejectionAuthorityRulesNotConfigured, request.State()), nil
	case ports.AuthorizationApplicabilityConflict:
		// 权威允许了这次拒绝，却答不出凭哪一版规则——决定上要记那一版，挑一版就是替商业责任方作答。
		return handler.undecided(ctx, command, RejectionAuthorityApplicabilityConflict, request.State()), nil
	default:
		return RejectShipmentRequestResult{}, ErrUnexpectedAuthorizationOutcome
	}

	if err := request.ActiveRejectionAdmissible(); err != nil {
		return handler.inadmissible(request, err)
	}

	// 保存委托之前先取作用域锁，理由与位置同 WithdrawShipmentRequestHandler.decide：先 S 后 R 与接受链同序，且在领取决定
	// 标识之前。已决态在上面的前置里就答了，不保存委托，不取锁。取不到锁与保存不了同一格。
	if err := handler.occupation(command).lockScopesBeforeSave(ctx, request); err != nil {
		return handler.undecided(ctx, command, DecisionNotRecorded, request.State()), nil
	}

	decisionID, err := handler.deps.Identities.NextAcceptanceDecisionID(ctx)
	if err != nil {
		return handler.undecided(ctx, command, DecisionIdentityUnavailable, request.State()), nil
	}

	rejected, err := request.RejectByAuthority(domain.ActiveRejectionSpec{
		DecisionID: decisionID,
		Authority:  authority.Authority,
		Decider:    command.Decider,
		Reason:     command.Reason,
		Evidence:   command.Evidence,
		DecidedAt:  handler.deps.Clock.Now(),
	})
	if err != nil {
		// 前置刚问过，这里再撞上只会是防御：同一份读回的委托不会在两次询问之间变样，但转移的答复仍按同一张表译。
		return handler.inadmissible(request, err)
	}

	saved, err := handler.deps.Requests.Save(ctx, command.Identity, rejected)
	if err != nil {
		// 拒绝没落库，因此存着的仍是保存前那一份：交回 request 的状态而不是 rejected 的。
		return handler.undecided(ctx, command, DecisionNotRecorded, request.State()), nil
	}
	if saved != ports.ShipmentRequestSaved {
		// 本方这次拒绝确定没落库。抢先那一方写下的可能是接受，因此这里不发释放——那笔冻结
		// 归真正成立的那条决定路径处置。交回的 state 是读取时那一份、已知不是库里此刻那一份，
		// 理由见 form_acceptance_decision.go 的同一支。
		reason, err := saveStallReason(saved)
		if err != nil {
			return RejectShipmentRequestResult{}, err
		}
		return handler.undecided(ctx, command, reason, request.State()), nil
	}

	decision, _ := rejected.AcceptanceDecision()
	compensation, refused := handler.releaseOccupation(ctx, command, rejected)
	return RejectShipmentRequestResult{
		outcome:        ActiveRejectionFormed,
		state:          rejected.State(),
		decision:       decision,
		hasDecision:    true,
		compensation:   compensation,
		releaseRefused: refused,
	}, nil
}

// inadmissible 把「此刻不能形成主动拒绝」的领域答复译成本命令的结果。已越过决定边界答`任务已完结`并交回那一个
// 决定，与复核完成、授权处置两口同名同义（ADR-0132 决定二）；不答`已形成`——那一格只留给本命令确实形成了拒绝
// 的那一次，否则换了人、换了原因的后到者会把没被采纳的拒绝读成成功。也不分出「同内容重放」与「输给了别人」：
// 一槽一个决定，异内容不可能是新决定，调用方凭交回的决定方与原因自己比。
func (handler *RejectShipmentRequestHandler) inadmissible(
	request domain.ShipmentRequest,
	err error,
) (RejectShipmentRequestResult, error) {
	if !errors.Is(err, domain.ErrDecisionAlreadyFormed) {
		return RejectShipmentRequestResult{}, fmt.Errorf("reject by authority: %w", err)
	}
	decision, formed := request.AcceptanceDecision()
	return RejectShipmentRequestResult{
		outcome:     ActiveRejectionTaskAlreadyClosed,
		state:       request.State(),
		decision:    decision,
		hasDecision: formed,
	}, nil
}

// releaseOccupation 在主动拒绝越过提交边界后按原关联解除资金控制。主动拒绝同样是「接受确定未
// 成立」，冻结不能因为拒绝出自运营之手就留在原处占着货主的钱。
//
// 放的是这份委托各版已成立的占用，理由同 WithdrawShipmentRequestHandler.releaseOccupation：拒绝是终局，之后再没有
// 轮次去放被取代那一版。放不掉交回补偿续办引用，由续办去补；提供方确定不受理的记进释放未受理册，拒绝不回滚。
func (handler *RejectShipmentRequestHandler) releaseOccupation(
	ctx context.Context,
	command RejectShipmentRequestCommand,
	request domain.ShipmentRequest,
) (domain.OwnershipContinuationReference, bool) {
	scope := handler.occupation(command)
	return scope.settle(ctx, scope.releaseEvery(ctx, request, command.SubmissionVersion))
}

func (handler *RejectShipmentRequestHandler) occupation(command RejectShipmentRequestCommand) occupationRelease {
	return occupationRelease{
		judgments: handler.deps.Judgments,
		release:   handler.deps.Release,
		identity:  command.Identity,
		request:   command.ShipmentRequestID,
		refusals:  handler.deps.Refusals,
		clock:     handler.deps.Clock,
		scopeLock: handler.deps.ScopeLock,
	}
}

// undecided 交回本轮的未决结果。state 由调用点给出而不是在这里假定`已提交`：未决要交回的是
// 已知事实，而委托当前是什么状态正是本轮可能已经查到的事实之一。取不到委托的那一轮传零值。
func (handler *RejectShipmentRequestHandler) undecided(
	ctx context.Context,
	command RejectShipmentRequestCommand,
	reason JudgmentPendingReason,
	state domain.ShipmentRequestState,
) RejectShipmentRequestResult {
	continuation := judgmentContinuation(
		reason,
		command.Identity.TenantID().String(),
		command.Identity.CustomerAccountID().String(),
		command.ShipmentRequestID.String(),
		command.SubmissionVersion.String(),
	)
	recordAttempt(ctx, handler.deps.Recorder, handler.deps.Clock,
		command.Identity.TenantID(), command.ShipmentRequestID, reason, continuation)

	return RejectShipmentRequestResult{
		outcome:      ActiveRejectionUndecided,
		state:        state,
		reason:       reason,
		continuation: continuation,
	}
}
