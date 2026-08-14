package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// DispositionOutcome 是处置请求编排三个入口共用的应用处理结果。发送、替代与两类应答
// 回填各占其格：「请求发送、源上下文接受、拒绝、部分接受、要求补充以及实际执行结果
// 分别记录」（CONTEXT）——分别记录的前提是分别答复。
type DispositionOutcome uint8

const (
	DispositionOutcomeInvalid DispositionOutcome = iota
	DispositionRequestSent
	DispositionRequestSuperseded
	DispositionExistingResult
	SourceJudgmentRecorded
	SourceJudgmentAlreadyRecorded
	AcceptanceWindowExpired
	CancellationAnswerRecorded
	CancellationAnswerAlreadyRecorded
	DispositionUndecided
	DispositionNotAccepted
)

func (outcome DispositionOutcome) String() string {
	switch outcome {
	case DispositionRequestSent:
		return "SENT"
	case DispositionRequestSuperseded:
		return "SUPERSEDED"
	case DispositionExistingResult:
		return "EXISTING_RESULT"
	case SourceJudgmentRecorded:
		return "JUDGMENT_RECORDED"
	case SourceJudgmentAlreadyRecorded:
		return "JUDGMENT_ALREADY_RECORDED"
	case AcceptanceWindowExpired:
		return "ACCEPTANCE_WINDOW_EXPIRED"
	case CancellationAnswerRecorded:
		return "CANCELLATION_RECORDED"
	case CancellationAnswerAlreadyRecorded:
		return "CANCELLATION_ALREADY_RECORDED"
	case DispositionUndecided:
		return "UNDECIDED"
	case DispositionNotAccepted:
		return "NOT_ACCEPTED"
	default:
		return ""
	}
}

// DispositionUndecidedReason 指名本轮停在哪一步。
type DispositionUndecidedReason uint8

const (
	DispositionUndecidedReasonNone DispositionUndecidedReason = iota
	DispositionCaseViewUnavailable
	DispositionStoreUnavailable
	DispositionIdentityUnavailable
)

func (reason DispositionUndecidedReason) String() string {
	switch reason {
	case DispositionCaseViewUnavailable:
		return "CASE_VIEW_UNAVAILABLE"
	case DispositionStoreUnavailable:
		return "DISPOSITION_STORE_UNAVAILABLE"
	case DispositionIdentityUnavailable:
		return "DISPOSITION_IDENTITY_UNAVAILABLE"
	default:
		return ""
	}
}

// SendDispositionRequestCommand 携带一份拟发送的处置请求。七件里除受理有效期外缺一
// 即未受理（领域构造器已钉，编排在门口先答）。Supersedes 可缺席：给出时表示以本请求
// 替代那份既有请求的未来意图——已发出的不召回。租户显式随命令到达（ADR-0003）。
type SendDispositionRequestCommand struct {
	TenantID         domain.TenantID
	Case             domain.CaseID
	Target           domain.SourceContext
	Action           domain.RequestedActionReference
	Scope            domain.RequestScopeReference
	Reason           string
	Evidence         domain.RequestEvidenceReference
	AcceptanceWindow time.Time
	Supersedes       domain.DispositionRequestID
}

// RecordSourceJudgmentCommand 回填源上下文对请求的判断。时间取对方业务答复的时间，
// 不取本方时钟——答复何时作出是对方的事实。
type RecordSourceJudgmentCommand struct {
	TenantID domain.TenantID
	Request  domain.DispositionRequestID
	Judgment domain.SourceJudgment
	JudgedAt time.Time
}

// RecordCancellationAnswerCommand 回填目标上下文对取消/替代意图的答复。
type RecordCancellationAnswerCommand struct {
	TenantID   domain.TenantID
	Request    domain.DispositionRequestID
	Answer     domain.CancellationAnswer
	AnsweredAt time.Time
}

type DispositionResult struct {
	outcome    DispositionOutcome
	request    *domain.DispositionRequest
	reason     DispositionUndecidedReason
	handoffRef string
}

func (result DispositionResult) Outcome() DispositionOutcome {
	return result.outcome
}

// Request 只在请求成立（本轮或此前）时给出。
func (result DispositionResult) Request() (*domain.DispositionRequest, bool) {
	return result.request, result.request != nil
}

func (result DispositionResult) UndecidedReason() DispositionUndecidedReason {
	return result.reason
}

// HandoffReference 非空说明请求已成立但发送意图还没交出去，重放会重发同一份。
func (result DispositionResult) HandoffReference() string {
	return result.handoffRef
}

type SendDispositionRequestDeps struct {
	Cases      ports.ActiveCaseView
	Requests   ports.DispositionRequestStore
	Identities ports.DispositionRequestIdentityFactory
	Downstream ports.DispositionHandoff
	Clock      ports.Clock
}

type SendDispositionRequestHandler struct {
	deps SendDispositionRequestDeps
}

func NewSendDispositionRequestHandler(deps SendDispositionRequestDeps) *SendDispositionRequestHandler {
	return &SendDispositionRequestHandler{deps: deps}
}

// Handle 发送或替代一份处置请求：受理（七件与活案件——请求只能挂在活案件下）→ 幂等按
// （案件+动作+范围），同一请求不重发只重发同一份意图 → 替代走 SupersedeWith，只改未来
// 意图、被替代者与后继同一提交 → 发送意图（真实目标上下文的受理在对方，这里只记发送）。
func (handler *SendDispositionRequestHandler) Handle(
	ctx context.Context,
	command SendDispositionRequestCommand,
) (DispositionResult, error) {
	if command.TenantID.String() == "" ||
		command.Case.String() == "" ||
		command.Target.String() == "" ||
		command.Action.String() == "" ||
		command.Scope.String() == "" ||
		command.Reason == "" ||
		command.Evidence.String() == "" {
		return DispositionResult{outcome: DispositionNotAccepted}, nil
	}

	active, found, err := handler.deps.Cases.CaseActive(ctx, command.Case)
	if err != nil {
		return DispositionResult{outcome: DispositionUndecided, reason: DispositionCaseViewUnavailable}, nil
	}
	if !found || !active {
		// 挂不上活案件：案件不存在与已关闭同答未受理——关闭前的未完成请求盘点
		// （CONTEXT）已经把「关闭后还要动作」引去关联新案件，这里不开旁门。
		return DispositionResult{outcome: DispositionNotAccepted}, nil
	}

	if command.Supersedes.String() != "" {
		return handler.supersede(ctx, command)
	}

	existing, found, err := handler.deps.Requests.FindCurrent(
		ctx, command.TenantID, command.Case, command.Action, command.Scope)
	if err != nil {
		return DispositionResult{outcome: DispositionUndecided, reason: DispositionStoreUnavailable}, nil
	}
	if found {
		// 同（案件+动作+范围）重复到达：不重发请求，把同一份发送意图再交一次
		// （ADR-0043——只答已有结果就收工，一份首次发送失败的请求会永远停在
		// 「本方已成立、对方不知道」）。
		return DispositionResult{
			outcome:    DispositionExistingResult,
			request:    existing,
			handoffRef: handler.handOffRequest(ctx, command.TenantID, existing),
		}, nil
	}

	request, err := handler.send(ctx, command, 1)
	if err != nil {
		return DispositionResult{}, err
	}
	if request == nil {
		return DispositionResult{outcome: DispositionUndecided, reason: DispositionIdentityUnavailable}, nil
	}
	saved, err := handler.deps.Requests.Save(ctx, command.TenantID, request)
	if err != nil {
		return DispositionResult{outcome: DispositionUndecided, reason: DispositionStoreUnavailable}, nil
	}
	switch saved {
	case ports.DispositionSaved:
		return DispositionResult{
			outcome:    DispositionRequestSent,
			request:    request,
			handoffRef: handler.handOffRequest(ctx, command.TenantID, request),
		}, nil
	case ports.DispositionAlreadyRecorded:
		existing, found, err := handler.deps.Requests.FindCurrent(
			ctx, command.TenantID, command.Case, command.Action, command.Scope)
		if err != nil || !found {
			return DispositionResult{outcome: DispositionUndecided, reason: DispositionStoreUnavailable}, nil
		}
		return DispositionResult{
			outcome:    DispositionExistingResult,
			request:    existing,
			handoffRef: handler.handOffRequest(ctx, command.TenantID, existing),
		}, nil
	default:
		return DispositionResult{}, fmt.Errorf("send disposition request: unexpected save outcome %d", saved)
	}
}

// supersede 以新请求替代既有请求的未来意图。被替代者已有判断原样保留——替代不是删除，
// 已发出的也不召回；取消/替代对既发范围的效力由目标上下文按四值答复（另一入口回填）。
func (handler *SendDispositionRequestHandler) supersede(
	ctx context.Context,
	command SendDispositionRequestCommand,
) (DispositionResult, error) {
	prior, found, err := handler.deps.Requests.FindByID(ctx, command.TenantID, command.Supersedes)
	if err != nil {
		return DispositionResult{outcome: DispositionUndecided, reason: DispositionStoreUnavailable}, nil
	}
	if !found || prior.Case() != command.Case {
		// 指名一份不存在的请求，或想跨案件替代：都是调用方对世界的判断错了，如实拒。
		return DispositionResult{outcome: DispositionNotAccepted}, nil
	}
	if successorID, superseded := prior.SupersededBy(); superseded {
		// 已被替代：读回赢家再交一次它的意图，不叠第二层替代。
		successor, found, err := handler.deps.Requests.FindByID(ctx, command.TenantID, successorID)
		if err != nil || !found {
			// 记录说被替代、后继却读不到，是竞争窗口里的暂态：停在未决，重试自然读到。
			return DispositionResult{outcome: DispositionUndecided, reason: DispositionStoreUnavailable}, nil
		}
		return DispositionResult{
			outcome:    DispositionExistingResult,
			request:    successor,
			handoffRef: handler.handOffRequest(ctx, command.TenantID, successor),
		}, nil
	}

	successor, err := handler.send(ctx, command, prior.IntentVersion()+1)
	if err != nil {
		return DispositionResult{}, err
	}
	if successor == nil {
		return DispositionResult{outcome: DispositionUndecided, reason: DispositionIdentityUnavailable}, nil
	}
	if err := prior.SupersedeWith(successor); err != nil {
		return DispositionResult{}, fmt.Errorf("supersede disposition request: %w", err)
	}
	if err := handler.deps.Requests.SaveSupersession(ctx, command.TenantID, prior, successor); err != nil {
		return DispositionResult{outcome: DispositionUndecided, reason: DispositionStoreUnavailable}, nil
	}
	return DispositionResult{
		outcome:    DispositionRequestSuperseded,
		request:    successor,
		handoffRef: handler.handOffRequest(ctx, command.TenantID, successor),
	}, nil
}

// send 签发身份并构造请求。身份签不出交回 (nil, nil)，由调用处形成未决——放在这里
// 判会让两条调用路径各写一遍同样的分支。
func (handler *SendDispositionRequestHandler) send(
	ctx context.Context,
	command SendDispositionRequestCommand,
	intentVersion int,
) (*domain.DispositionRequest, error) {
	requestID, err := handler.deps.Identities.NextDispositionRequestID(ctx)
	if err != nil {
		return nil, nil
	}
	request, err := domain.SendDispositionRequest(domain.DispositionRequestSpec{
		ID:               requestID,
		Case:             command.Case,
		Target:           command.Target,
		Action:           command.Action,
		Scope:            command.Scope,
		Reason:           command.Reason,
		Evidence:         command.Evidence,
		IntentVersion:    intentVersion,
		SentAt:           handler.deps.Clock.Now(),
		AcceptanceWindow: command.AcceptanceWindow,
	})
	if err != nil {
		// 七件在门口验过，走到这里还构不成只剩时限先于发送时刻这类坏输入——上抛不吞。
		return nil, fmt.Errorf("send disposition request: %w", err)
	}
	return request, nil
}

// RecordSourceJudgment 回填源上下文的判断：应答不可覆盖，重复应答拒；受理有效期届满
// 后的接受不再入账（「待处理且到达受理有效期 → 已到期：该范围不得再被新接受」）。两者
// 都是业务答案不是故障——把它们答成错误，回填方会把一次如实的拒收当成要重试的失败。
func (handler *SendDispositionRequestHandler) RecordSourceJudgment(
	ctx context.Context,
	command RecordSourceJudgmentCommand,
) (DispositionResult, error) {
	if command.TenantID.String() == "" || command.Request.String() == "" || command.JudgedAt.IsZero() {
		return DispositionResult{outcome: DispositionNotAccepted}, nil
	}
	request, found, err := handler.deps.Requests.FindByID(ctx, command.TenantID, command.Request)
	if err != nil {
		return DispositionResult{outcome: DispositionUndecided, reason: DispositionStoreUnavailable}, nil
	}
	if !found {
		return DispositionResult{outcome: DispositionNotAccepted}, nil
	}

	if err := request.RecordSourceJudgment(command.Judgment, command.JudgedAt); err != nil {
		switch {
		case errors.Is(err, domain.ErrRequestAlreadyJudged):
			return DispositionResult{outcome: SourceJudgmentAlreadyRecorded, request: request}, nil
		case errors.Is(err, domain.ErrRequestWindowExpired):
			return DispositionResult{outcome: AcceptanceWindowExpired, request: request}, nil
		default:
			return DispositionResult{}, fmt.Errorf("record source judgment: %w", err)
		}
	}
	if _, err := handler.deps.Requests.Save(ctx, command.TenantID, request); err != nil {
		return DispositionResult{outcome: DispositionUndecided, reason: DispositionStoreUnavailable}, nil
	}
	return DispositionResult{outcome: SourceJudgmentRecorded, request: request}, nil
}

// RecordCancellationAnswer 回填目标上下文对取消/替代意图的四值答复：未判断的请求谈不上
// 取消（如实拒），已答过的不覆盖。「只有明确取消成功的未消耗范围停止未来执行」——效力
// 判读在答复值本身，这里只如实入账。
func (handler *SendDispositionRequestHandler) RecordCancellationAnswer(
	ctx context.Context,
	command RecordCancellationAnswerCommand,
) (DispositionResult, error) {
	if command.TenantID.String() == "" || command.Request.String() == "" || command.AnsweredAt.IsZero() {
		return DispositionResult{outcome: DispositionNotAccepted}, nil
	}
	request, found, err := handler.deps.Requests.FindByID(ctx, command.TenantID, command.Request)
	if err != nil {
		return DispositionResult{outcome: DispositionUndecided, reason: DispositionStoreUnavailable}, nil
	}
	if !found {
		return DispositionResult{outcome: DispositionNotAccepted}, nil
	}

	if err := request.RecordCancellationAnswer(command.Answer, command.AnsweredAt); err != nil {
		switch {
		case errors.Is(err, domain.ErrRequestNotJudged):
			return DispositionResult{outcome: DispositionNotAccepted}, nil
		case errors.Is(err, domain.ErrRequestAlreadyJudged):
			return DispositionResult{outcome: CancellationAnswerAlreadyRecorded, request: request}, nil
		default:
			return DispositionResult{}, fmt.Errorf("record cancellation answer: %w", err)
		}
	}
	if _, err := handler.deps.Requests.Save(ctx, command.TenantID, request); err != nil {
		return DispositionResult{outcome: DispositionUndecided, reason: DispositionStoreUnavailable}, nil
	}
	return DispositionResult{outcome: CancellationAnswerRecorded, request: request}, nil
}

// handOffRequest 把已成立的请求交给发送侧下游，交不出去时交回发送续办引用。失败不改写
// 请求，也不算进未决——请求已经成立，要续办的是发送（ADR-0043）。
func (handler *SendDispositionRequestHandler) handOffRequest(
	ctx context.Context,
	tenant domain.TenantID,
	request *domain.DispositionRequest,
) string {
	if err := handler.deps.Downstream.HandOffDispositionRequest(ctx, ports.DispositionHandoffIntent{
		TenantID: tenant,
		Request:  request,
	}); err != nil {
		return "CONT-" + shortDigest("DISPOSITION_HANDOFF", request.ID().String())
	}
	return ""
}
