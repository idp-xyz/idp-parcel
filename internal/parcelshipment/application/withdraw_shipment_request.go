package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// WithdrawalOutcome 是撤回这一步的应用处理结果。`已有决定`独立成一个取值而不是并进`已成立`：
// 后到的请求读到的是别人的决定，把它记成撤回成功会让客户以为自己取消掉了一份已经接受的委托。
type WithdrawalOutcome uint8

const (
	WithdrawalOutcomeInvalid WithdrawalOutcome = iota
	WithdrawalFormed
	WithdrawalNotAuthorized
	WithdrawalDecisionAlreadyFormed
	WithdrawalUndecided
	WithdrawalSourceConflict
)

func (outcome WithdrawalOutcome) String() string {
	switch outcome {
	case WithdrawalFormed:
		return "FORMED"
	case WithdrawalNotAuthorized:
		return "NOT_AUTHORIZED"
	case WithdrawalDecisionAlreadyFormed:
		return "DECISION_ALREADY_FORMED"
	case WithdrawalUndecided:
		return "UNDECIDED"
	case WithdrawalSourceConflict:
		return "SOURCE_CONFLICT"
	default:
		return ""
	}
}

type WithdrawShipmentRequestCommand struct {
	// Identity 是产生委托的那一次提交的来源身份，用来找到目标委托。
	Identity domain.SourceIdentity
	// WithdrawalIdentity 是撤回请求自己的来源身份。它与 Identity 分开，因为同一个客户就同一份
	// 委托先提交后撤回是两次来源请求：合用一个身份会让撤回请求被判成原提交的重放，从此再也
	// 分不出「这单重发了」与「这单要撤」。
	WithdrawalIdentity domain.SourceIdentity
	PayloadDigest      domain.PayloadDigest
	OccurredAt         time.Time
	ReceivedAt         time.Time
	ShipmentRequestID  domain.ShipmentRequestID
	SubmissionVersion  domain.SubmissionVersionID
	Requester          domain.WithdrawalRequesterReference
	Reason             domain.WithdrawalReasonReference
}

type WithdrawShipmentRequestResult struct {
	outcome       WithdrawalOutcome
	state         domain.ShipmentRequestState
	withdrawal    domain.Withdrawal
	hasWithdrawal bool
	decision      domain.AcceptanceDecision
	hasDecision   bool
	reason        JudgmentPendingReason
	continuation  domain.OwnershipContinuationReference
	compensation  domain.OwnershipContinuationReference
}

func (result WithdrawShipmentRequestResult) Outcome() WithdrawalOutcome {
	return result.outcome
}

func (result WithdrawShipmentRequestResult) State() domain.ShipmentRequestState {
	return result.state
}

func (result WithdrawShipmentRequestResult) Withdrawal() (domain.Withdrawal, bool) {
	return result.withdrawal, result.hasWithdrawal
}

// AcceptanceDecision 只在撤回撞上一个已经成立的接受或拒绝时给出。撞上一次既有撤回时它缺席
// ——那种情况下要读的是撤回记录，不是接受决定。
func (result WithdrawShipmentRequestResult) AcceptanceDecision() (domain.AcceptanceDecision, bool) {
	return result.decision, result.hasDecision
}

func (result WithdrawShipmentRequestResult) PendingReason() JudgmentPendingReason {
	return result.reason
}

func (result WithdrawShipmentRequestResult) ContinuationReference() domain.OwnershipContinuationReference {
	return result.continuation
}

// CompensationReference 只在撤回已经成立、而随附的资金释放没能确定完成时给出。撤回本身有效，
// 续办的是补偿而不是这个决定。
func (result WithdrawShipmentRequestResult) CompensationReference() domain.OwnershipContinuationReference {
	return result.compensation
}

type WithdrawShipmentRequestDeps struct {
	Sources    ports.SourceSubmissionRepository
	Requests   ports.ShipmentRequestRepository
	Authorizer ports.WithdrawalAuthorizer
	Judgments  ports.RecordedJudgmentReader
	Recorder   ports.AcceptanceJudgmentRecorder
	Release    ports.PreAcceptanceControlRelease
	Identities ports.AcceptanceDecisionIdentity
	Clock      ports.Clock
}

type WithdrawShipmentRequestHandler struct {
	deps WithdrawShipmentRequestDeps
}

func NewWithdrawShipmentRequestHandler(deps WithdrawShipmentRequestDeps) *WithdrawShipmentRequestHandler {
	return &WithdrawShipmentRequestHandler{deps: deps}
}

// Handle 让客户或其当前有效授权代表撤回一份仍为`已提交`的待决委托。
//
// 授权先于一切写动作：未获授权时既不签发决定标识，也不碰聚合。顺序反过来的话，一次未获授权
// 的尝试仍会消耗一个决定标识，而标识是本上下文签发的稀缺身份。
//
// 它与自动接受、主动拒绝竞争同一个决定提交边界，所以撞上已成立的决定时交回那一个，而不是报错
// 重试——用例要求后到者只能读取既有结果。
func (handler *WithdrawShipmentRequestHandler) Handle(
	ctx context.Context,
	command WithdrawShipmentRequestCommand,
) (WithdrawShipmentRequestResult, error) {
	// 步骤 1 先于一切业务判断：一次没能形成撤回的请求，也必须留下它到达过的事实，否则事后
	// 连「客户到底提没提过」都无从追溯。
	incoming, err := domain.NewSourceSubmissionFingerprint(
		command.WithdrawalIdentity,
		command.PayloadDigest,
		command.OccurredAt,
		command.ReceivedAt,
	)
	if err != nil {
		return WithdrawShipmentRequestResult{}, fmt.Errorf("preserve withdrawal source: %w", err)
	}
	preserved, replayed, err := handler.deps.Sources.FindPreserved(ctx, command.WithdrawalIdentity)
	if err != nil {
		return WithdrawShipmentRequestResult{}, fmt.Errorf("find preserved withdrawal source: %w", err)
	}
	if replayed {
		return handler.resolvePreserved(ctx, command, preserved, incoming)
	}
	if err := handler.deps.Sources.Preserve(ctx, incoming); err != nil {
		return WithdrawShipmentRequestResult{}, fmt.Errorf("preserve withdrawal source: %w", err)
	}

	request, found, err := handler.deps.Requests.FindBySourceIdentity(ctx, command.Identity)
	if err != nil {
		return handler.undecided(ctx, command, ShipmentRequestUnavailable, domain.ShipmentRequestStateInvalid), nil
	}
	if !found {
		// 与形成决定那一步同一判断：指名一份查不到的委托是调用方的错，不是业务结果。接
		// HTTP 时同样必须映射为`统一不可见结果`，理由见 form_acceptance_decision.go。
		// `AT-PS-075` 要求越权撤回不泄露对象存在与否，走的正是这条出口。
		return WithdrawShipmentRequestResult{}, fmt.Errorf("withdraw shipment request: %w", domain.ErrInvalidShipmentRequest)
	}

	authority, err := handler.deps.Authorizer.AuthorizeWithdrawal(ctx, ports.WithdrawalAuthorizationQuery{
		Identity:          command.Identity,
		ShipmentRequestID: command.ShipmentRequestID,
		SubmissionVersion: command.SubmissionVersion,
		Requester:         command.Requester,
		Reason:            command.Reason,
	})
	if err != nil {
		return handler.undecided(ctx, command, WithdrawalAuthorityUnavailable, request.State()), nil
	}
	switch authority.Outcome {
	case ports.AuthorizationGranted:
	case ports.AuthorizationRefused:
		// 未获授权不是未决：它是一个确定的业务答案，续办也补不出授权来。
		return WithdrawShipmentRequestResult{
			outcome: WithdrawalNotAuthorized,
			state:   request.State(),
		}, nil
	case ports.AuthorizationRulesNotConfigured:
		// 这个范围此刻一条现行授权规则都没有。它不是对客户的判定——`UC-PS-005` 明禁默认
		// 任何角色有撤回权，而把「没有规则」答成`未获授权`同样是一次默认，只是方向朝紧。
		return handler.undecided(ctx, command, WithdrawalAuthorityRulesNotConfigured, request.State()), nil
	default:
		// 逐取值分派，不留兜底：端口日后多一种答复时这里报错，而不是静默归入上面某一格。
		return WithdrawShipmentRequestResult{}, ErrUnexpectedAuthorizationOutcome
	}

	// 用例把「读取已提交决定并返回既有结果」放在提交撤回之前（步骤 3 先于步骤 4），所以这里
	// 先短路：一份已经决定的委托不该再消耗一个决定标识，而标识是本上下文签发的稀缺身份。
	// 这不替代提交边界上的重查——聚合的 `decisionFormed` 闸门仍然是裁决并发竞争的那一道。
	if request.State() != domain.ShipmentRequestSubmitted {
		return handler.existing(ctx, command, request), nil
	}

	decisionID, err := handler.deps.Identities.NextAcceptanceDecisionID(ctx)
	if err != nil {
		return handler.undecided(ctx, command, DecisionIdentityUnavailable, request.State()), nil
	}

	withdrawn, err := request.WithdrawByCustomer(domain.WithdrawalSpec{
		DecisionID: decisionID,
		Authority:  authority.Authority,
		Requester:  command.Requester,
		Reason:     command.Reason,
		DecidedAt:  handler.deps.Clock.Now(),
	})
	if err != nil {
		if errors.Is(err, domain.ErrDecisionAlreadyFormed) {
			return handler.existing(ctx, command, request), nil
		}
		return WithdrawShipmentRequestResult{}, fmt.Errorf("withdraw by customer: %w", err)
	}

	saved, err := handler.deps.Requests.Save(ctx, command.Identity, withdrawn)
	if err != nil {
		// 撤回没落库，因此存着的仍是保存前那一份：交回 request 的状态而不是 withdrawn 的。
		return handler.undecided(ctx, command, DecisionNotRecorded, request.State()), nil
	}
	if saved != ports.ShipmentRequestSaved {
		// 本方这次撤回确定没落库，抢先那一方写下的可能是接受，因此不发释放。客户重试这次撤回
		// 时会重读到新的那一份，走 existing 那条路读回既有结果——交回的 state 是读取时那一份、
		// 已知不是库里此刻那一份，理由见 form_acceptance_decision.go 的同一支。
		reason, err := saveStallReason(saved)
		if err != nil {
			return WithdrawShipmentRequestResult{}, err
		}
		return handler.undecided(ctx, command, reason, request.State()), nil
	}

	record, _ := withdrawn.Withdrawal()
	return WithdrawShipmentRequestResult{
		outcome:       WithdrawalFormed,
		state:         withdrawn.State(),
		withdrawal:    record,
		hasWithdrawal: true,
		compensation:  handler.releaseOccupation(ctx, command),
	}, nil
}

// resolvePreserved 回答一个来源身份已被保全过的撤回请求。
//
// 同身份同内容是重复到达：追加一次观察，然后交回原处理结果，绝不再走一遍授权与决定边界。
// 走第二遍不只是浪费——授权在两次之间可能已经失效，同一个已经成立的撤回会因此读出两种回执。
//
// 同身份不同内容是来源冲突：原请求不被覆盖，也不形成第二次撤回。它与「决定已成立」分成两个
// 结果，因为客户要做的事不同——一个是纠正自己这次请求的内容，一个是接受这单已经有结论了。
func (handler *WithdrawShipmentRequestHandler) resolvePreserved(
	ctx context.Context,
	command WithdrawShipmentRequestCommand,
	preserved domain.SourceSubmissionFingerprint,
	incoming domain.SourceSubmissionFingerprint,
) (WithdrawShipmentRequestResult, error) {
	classification, err := domain.ClassifySourceSubmission(preserved, incoming)
	if err != nil {
		return WithdrawShipmentRequestResult{}, fmt.Errorf("classify withdrawal source: %w", err)
	}
	if classification == domain.SourceConflict {
		return WithdrawShipmentRequestResult{outcome: WithdrawalSourceConflict}, nil
	}

	if err := handler.deps.Sources.AppendObservation(ctx, incoming); err != nil {
		return WithdrawShipmentRequestResult{}, fmt.Errorf("append withdrawal source observation: %w", err)
	}

	request, found, err := handler.deps.Requests.FindBySourceIdentity(ctx, command.Identity)
	if err != nil {
		return handler.undecided(ctx, command, ShipmentRequestUnavailable, domain.ShipmentRequestStateInvalid), nil
	}
	if !found {
		return WithdrawShipmentRequestResult{}, fmt.Errorf("withdraw shipment request: %w", domain.ErrInvalidShipmentRequest)
	}
	return handler.existing(ctx, command, request), nil
}

// existing 交回那个先到的决定。它可能是接受、拒绝，也可能是本方此前形成的一次撤回——
// `AT-PS-071`/`AT-PS-072` 要求返回既有接受或拒绝，`AT-PS-068` 要求重复撤回返回原撤回及原补偿
// 关联。已撤回委托也不在这里原地恢复：`AT-PS-076` 要求客户重新提出需求时建立关联新委托。
//
// 撞上的是既有撤回时才重跑释放。步骤 6 本就要求「按原业务关联幂等形成适用冻结释放」，所以
// 重跑是它的语义而不是副作用；补偿引用由原因与范围派生，因此重复请求拿到的就是原补偿关联，
// 不必为此在聚合上再存一份待补偿状态。
//
// 撞上接受时绝不释放：`AT-PS-071` 明说接受已经合法提交时，后到的决定前撤回请求不得释放合法
// 冻结。撞上拒绝时也不释放——那笔冻结归形成拒绝的那条路径处置，在这里再发一次等于两条路径
// 同时认领同一笔补偿。
func (handler *WithdrawShipmentRequestHandler) existing(
	ctx context.Context,
	command WithdrawShipmentRequestCommand,
	request domain.ShipmentRequest,
) WithdrawShipmentRequestResult {
	decision, hasDecision := request.AcceptanceDecision()
	record, hasWithdrawal := request.Withdrawal()

	compensation := domain.OwnershipContinuationReference{}
	if hasWithdrawal {
		compensation = handler.releaseOccupation(ctx, command)
	}

	return WithdrawShipmentRequestResult{
		outcome:       WithdrawalDecisionAlreadyFormed,
		state:         request.State(),
		withdrawal:    record,
		hasWithdrawal: hasWithdrawal,
		decision:      decision,
		hasDecision:   hasDecision,
		compensation:  compensation,
	}
}

// releaseOccupation 在撤回越过提交边界后按原关联解除资金控制。撤回同样是「接受确定未成立」，
// 冻结不能因为终止请求出自客户之手就留在原处占着他的钱。
//
// 释放失败不回滚撤回：`UC-PS-005` 要求「撤回提交成功后不等待财务释放才生效」，且不得为了保持
// 表面原子性把委托改回`已提交`。这一轮交回补偿续办引用，由续办去补。
//
// 读不回已记录的判断时不发释放：不知道关联就发，settlement-accounting 无从认领哪一笔。
// 从未形成过冻结时既不发释放也不留补偿引用——`AT-PS-074` 要的是撤回成立并保存财务补偿不适用
// 依据，凭空造一个待续补偿会让对账去追一笔不存在的释放。
func (handler *WithdrawShipmentRequestHandler) releaseOccupation(
	ctx context.Context,
	command WithdrawShipmentRequestCommand,
) domain.OwnershipContinuationReference {
	// 释放的是本版形成的那次冻结：ControlReleaseRequest 本就按提交版本关联，读判断也按同一版。
	recorded, err := handler.deps.Judgments.LoadRecordedJudgments(
		ctx, command.Identity.TenantID(), command.ShipmentRequestID, command.SubmissionVersion)
	if err != nil {
		return handler.compensationReference(command, RecordedJudgmentsUnavailable)
	}
	// 按「有没有成立的项」发而不按结论，理由同 FormAcceptanceDecisionHandler.releaseIfRejected。
	if !recorded.FinancialControl.OccupationFormed() {
		return domain.OwnershipContinuationReference{}
	}

	if err := handler.deps.Release.ReleasePreAcceptanceControl(ctx, ports.ControlReleaseRequest{
		Identity:          command.Identity,
		ShipmentRequestID: command.ShipmentRequestID,
		SubmissionVersion: command.SubmissionVersion,
		ControlResultID:   recorded.FinancialControl.ResultID(),
	}); err != nil {
		return handler.compensationReference(
			command,
			ControlReleasePending,
			recorded.FinancialControl.ResultID().String(),
		)
	}
	return domain.OwnershipContinuationReference{}
}

// compensationReference 派生一次补偿的续办引用，范围与两条拒绝路径逐字一致：同一笔冻结因同一
// 原因停下，无论停在接受、拒绝还是撤回哪条路上，续办方都必须拿到同一个引用。
func (handler *WithdrawShipmentRequestHandler) compensationReference(
	command WithdrawShipmentRequestCommand,
	reason JudgmentPendingReason,
	scope ...string,
) domain.OwnershipContinuationReference {
	return judgmentContinuation(
		reason,
		append([]string{
			command.Identity.TenantID().String(),
			command.Identity.CustomerAccountID().String(),
			command.ShipmentRequestID.String(),
			command.SubmissionVersion.String(),
		}, scope...)...,
	)
}

// undecided 交回本轮的未决结果。state 由调用点给出而不是在这里假定`已提交`：未决要交回的是
// 已知事实，而委托当前是什么状态正是本轮可能已经查到的事实之一。取不到委托的那一轮传零值，
// 那时状态确实未知——给一份查都没查到的委托安上生命周期状态是编造。
func (handler *WithdrawShipmentRequestHandler) undecided(
	ctx context.Context,
	command WithdrawShipmentRequestCommand,
	reason JudgmentPendingReason,
	state domain.ShipmentRequestState,
) WithdrawShipmentRequestResult {
	continuation := judgmentContinuation(
		reason,
		command.Identity.TenantID().String(),
		command.Identity.CustomerAccountID().String(),
		command.ShipmentRequestID.String(),
		command.SubmissionVersion.String(),
	)
	recordAttempt(ctx, handler.deps.Recorder, handler.deps.Clock,
		command.Identity.TenantID(), command.ShipmentRequestID, reason, continuation)

	return WithdrawShipmentRequestResult{
		outcome:      WithdrawalUndecided,
		state:        state,
		reason:       reason,
		continuation: continuation,
	}
}
