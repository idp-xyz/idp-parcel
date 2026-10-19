package application

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// 送站交接单的编排（UC-NO-006 应用流程步骤 3 至 6；ADR-0212 决定三、六；票 ps-external-mark-relations/12）：开立、归入、关闭是作业
// 事实登记面上的作业事实，绑定经运营决定或客户渠道作结论；声称的客户账户全部确认时范围成立，同一笔事务自投「交付范围成立」门铃。
// 认证、准入与载荷形状在各族 Intake 那一层答完，到这里的命令已带信封给出的租户、节点、操作者或客户账户。事务由进程级入口给出：交接
// 单册的写口无事务即拒。

// ErrDeliveryHandoverNoteNotFound 说明指名的交接单在本租户看不见：不存在；作业事实的作业范围不是交接单开立的节点；指名的客户账户
// 不在声称之列。几种同答，与越权探针同答（UC-NO-006「权限、隔离与审计」），不泄露别的客户账户在不在声称之列。
var ErrDeliveryHandoverNoteNotFound = errors.New("node operations: delivery handover note not visible")

// ErrDeliveryHandoverCommandRejected 说明命令形不成一份交接单事实：缺格、空白、声称为空或重复、结论不在词表一类。重发同样的内容不会
// 变好，在线口答坏载荷。
var ErrDeliveryHandoverCommandRejected = errors.New("node operations: delivery handover command rejected")

// DeliveryHandoverOutcome 是五口的封闭答案（UC-NO-006 结果语义契约），各格都是形成了的答案。
type DeliveryHandoverOutcome uint8

const (
	DeliveryHandoverOutcomeInvalid DeliveryHandoverOutcome = iota
	// DeliveryHandoverNoteOpened：送站交接单已开立。
	DeliveryHandoverNoteOpened
	// DeliveryHandoverUnitAssigned：一件作业实物已归入。
	DeliveryHandoverUnitAssigned
	// DeliveryHandoverNoteClosed：送站交接单已关闭，成员冻结。
	DeliveryHandoverNoteClosed
	// DeliveryHandoverBindingConfirmed：一个声称的客户账户的绑定已确认；范围是否因此成立另看成立时刻那一格。
	DeliveryHandoverBindingConfirmed
	// DeliveryHandoverBindingDenied：客户账户经客户渠道否认了绑定。
	DeliveryHandoverBindingDenied
	// DeliveryHandoverExistingResult：同一事实或同一结论原样重投，回显在册那一份，不出新版本、新门铃。
	DeliveryHandoverExistingResult
	// DeliveryHandoverExistingConclusion：这个客户账户的绑定已有结论，又来了不同的结论，回显在册结论、不覆盖。
	DeliveryHandoverExistingConclusion
	// DeliveryHandoverSourceConflict：同一个设备签发的事实身份换了内容（交接单标识、归入事实或关闭事实），回显在册那一份，什么也没落。
	DeliveryHandoverSourceConflict
	// DeliveryHandoverNoteStateMismatch：交接单状态不符——客户渠道对未关闭的交接单作结论、向已关闭的交接单归入、归入的实物不合条件；
	// 不符的是哪一样见 StateMismatch。
	DeliveryHandoverNoteStateMismatch
	// DeliveryHandoverNotAuthorized：运营决定确认时 party-commercial 答否，不落结论。
	DeliveryHandoverNotAuthorized
	// DeliveryHandoverAuthorizationNotConfigured：运营决定确认时本租户一条授权交付关系都没登记、交付方又不是该客户账户所属参与方本身，
	// 不落结论、不默认放行。
	DeliveryHandoverAuthorizationNotConfigured
)

func (outcome DeliveryHandoverOutcome) String() string {
	switch outcome {
	case DeliveryHandoverNoteOpened:
		return "DELIVERY_HANDOVER_NOTE_OPENED"
	case DeliveryHandoverUnitAssigned:
		return "UNIT_ASSIGNED"
	case DeliveryHandoverNoteClosed:
		return "DELIVERY_HANDOVER_NOTE_CLOSED"
	case DeliveryHandoverBindingConfirmed:
		return "BINDING_CONFIRMED"
	case DeliveryHandoverBindingDenied:
		return "BINDING_DENIED"
	case DeliveryHandoverExistingResult:
		return "EXISTING_RESULT"
	case DeliveryHandoverExistingConclusion:
		return "EXISTING_CONCLUSION"
	case DeliveryHandoverSourceConflict:
		return "SOURCE_CONFLICT"
	case DeliveryHandoverNoteStateMismatch:
		return "HANDOVER_NOTE_STATE_MISMATCH"
	case DeliveryHandoverNotAuthorized:
		return "NOT_IN_AUTHORIZED_DELIVERY_RELATIONSHIP"
	case DeliveryHandoverAuthorizationNotConfigured:
		return "AUTHORIZED_DELIVERY_NOT_CONFIGURED"
	default:
		return ""
	}
}

// HandoverStateMismatch 是「交接单状态不符」不符在哪一样。它是信息格，不是又一个 outcome：恢复动作各不同——已关闭的要另开一张、
// 未关闭的要先关、实物不合条件的要先看它此刻的判断——复核的人与设备据此分得开。
type HandoverStateMismatch uint8

const (
	HandoverStateMismatchAbsent HandoverStateMismatch = iota
	// HandoverNoteAlreadyClosed：向已关闭的交接单归入。
	HandoverNoteAlreadyClosed
	// HandoverNoteNotClosed：客户渠道对未关闭的交接单作结论——客户确认的是一份冻结的成员清单。
	HandoverNoteNotClosed
	// HandoverUnitNotAssignable：归入的实物在交接单开立的节点上没有当前收寄，或当前有效版本不属于重核对象（已关联、记了不只因
	// 歧义信号的冲突、来源未保全）。
	HandoverUnitNotAssignable
)

func (mismatch HandoverStateMismatch) String() string {
	switch mismatch {
	case HandoverNoteAlreadyClosed:
		return "NOTE_CLOSED"
	case HandoverNoteNotClosed:
		return "NOTE_NOT_CLOSED"
	case HandoverUnitNotAssignable:
		return "UNIT_NOT_ASSIGNABLE"
	default:
		return ""
	}
}

// OpenDeliveryHandoverNoteCommand 携带一次开立。租户与节点取自作业事实信封；交接单标识与开立时刻由设备签发（ADR-0023）。
type OpenDeliveryHandoverNoteCommand struct {
	TenantID        domain.TenantID
	Node            domain.NodeReference
	Note            domain.DeliveryHandoverNoteID
	DeliveringParty domain.DeliveringPartyReference
	ClaimedAccounts []domain.CustomerAccountReference
	OpenedAt        time.Time
}

// AssignToDeliveryHandoverNoteCommand 携带一次归入。FactID 是设备签发的归入事实身份，重放按它认；AssignedAt 是业务时间。
type AssignToDeliveryHandoverNoteCommand struct {
	TenantID   domain.TenantID
	Node       domain.NodeReference
	FactID     string
	Note       domain.DeliveryHandoverNoteID
	Unit       domain.HandlingUnitID
	AssignedAt time.Time
}

// CloseDeliveryHandoverNoteCommand 携带一次关闭。FactID 是设备签发的关闭事实身份。
type CloseDeliveryHandoverNoteCommand struct {
	TenantID domain.TenantID
	Node     domain.NodeReference
	FactID   string
	Note     domain.DeliveryHandoverNoteID
	ClosedAt time.Time
}

// ConfirmDeliveryHandoverCommand 携带一次运营决定确认。租户与提交操作者取自运营决定信封；交付方参与方是 party-commercial 的参与方
// 标识（不是设备上报的交付方线索），依据引用逐字保存、不判真伪。
type ConfirmDeliveryHandoverCommand struct {
	TenantID              domain.TenantID
	Operator              string
	Note                  domain.DeliveryHandoverNoteID
	Account               domain.CustomerAccountReference
	DeliveringParticipant domain.DeliveringPartyReference
	Basis                 string
}

// ConcludeDeliveryHandoverCommand 携带一次客户渠道结论。租户、客户账户、客户端（Channel）与来源请求键取自首方族信封。
type ConcludeDeliveryHandoverCommand struct {
	TenantID        domain.TenantID
	CustomerAccount domain.CustomerAccountReference
	Channel         string
	RequestKey      string
	Note            domain.DeliveryHandoverNoteID
	Conclusion      domain.HandoverBindingConclusion
}

// DeliveryHandoverAnswer 是一个答案所指的那几份事实：交接单（各格都带）、它的关闭、这一次所说的成员与结论、范围成立的时刻与状态
// 不符在哪。零值的格即不在场。
type DeliveryHandoverAnswer struct {
	Note               domain.DeliveryHandoverNote
	Closure            domain.DeliveryHandoverClosure
	Member             domain.DeliveryHandoverMember
	Binding            domain.DeliveryHandoverBinding
	ScopeEstablishedAt time.Time
	StateMismatch      HandoverStateMismatch
}

// DeliveryHandoverResult 是五口共用的答案。
type DeliveryHandoverResult struct {
	outcome DeliveryHandoverOutcome
	answer  DeliveryHandoverAnswer
}

// NewDeliveryHandoverResult 给传输层的替身造答案用；编排自己也经它出答案。
func NewDeliveryHandoverResult(outcome DeliveryHandoverOutcome, answer DeliveryHandoverAnswer) DeliveryHandoverResult {
	return DeliveryHandoverResult{outcome: outcome, answer: answer}
}

func (result DeliveryHandoverResult) Outcome() DeliveryHandoverOutcome  { return result.outcome }
func (result DeliveryHandoverResult) Note() domain.DeliveryHandoverNote { return result.answer.Note }
func (result DeliveryHandoverResult) StateMismatch() HandoverStateMismatch {
	return result.answer.StateMismatch
}

func (result DeliveryHandoverResult) Closure() (domain.DeliveryHandoverClosure, bool) {
	return result.answer.Closure, result.answer.Closure.FactID() != ""
}

func (result DeliveryHandoverResult) Member() (domain.DeliveryHandoverMember, bool) {
	return result.answer.Member, result.answer.Member.FactID() != ""
}

func (result DeliveryHandoverResult) Binding() (domain.DeliveryHandoverBinding, bool) {
	return result.answer.Binding, result.answer.Binding.Concluder() != ""
}

// ScopeEstablishedAt 交回范围成立的时刻；交接单此刻没成立范围答 false。
func (result DeliveryHandoverResult) ScopeEstablishedAt() (time.Time, bool) {
	return result.answer.ScopeEstablishedAt, !result.answer.ScopeEstablishedAt.IsZero()
}

type DeliveryHandoverDeps struct {
	Notes ports.DeliveryHandoverNoteRegister
	// Receptions 是归入判「这件实物在开立节点上当前有效版本属于重核对象」要问的收寄册，那一问取实物锁。
	Receptions ports.HandoverAssignmentReceptions
	// AuthorizedDelivery 是运营决定确认要问的授权交付判断（party-commercial，经本上下文的防腐适配器）。
	AuthorizedDelivery ports.AuthorizedDeliveryView
	Doorbell           ports.DeliveryScopeEstablishedDoorbell
	Clock              ports.Clock
}

type DeliveryHandoverNoteHandler struct {
	deps DeliveryHandoverDeps
}

// NewDeliveryHandoverNoteHandler 构造期拒缺件：漏装的册、收寄库、授权交付判断或门铃到第一份交接单才暴露，会被当成一次依赖故障。
func NewDeliveryHandoverNoteHandler(deps DeliveryHandoverDeps) (*DeliveryHandoverNoteHandler, error) {
	switch {
	case deps.Notes == nil:
		return nil, errors.New("node operations: delivery handover needs the handover note register")
	case deps.Receptions == nil:
		return nil, errors.New("node operations: delivery handover needs the reception register")
	case deps.AuthorizedDelivery == nil:
		return nil, errors.New("node operations: delivery handover needs the authorized delivery view")
	case deps.Doorbell == nil:
		return nil, errors.New("node operations: delivery handover needs the delivery scope doorbell")
	case deps.Clock == nil:
		return nil, errors.New("node operations: delivery handover needs a clock")
	}
	return &DeliveryHandoverNoteHandler{deps: deps}, nil
}

// Open 开立一张交接单。重放按设备签发的交接单标识认（ADR-0023）：同一标识同一内容答已有结果，换了内容答来源冲突，册上那一张都不被
// 顶替。节点取自作业事实信封：别的节点的设备用同一标识开立，内容就不同。
func (handler *DeliveryHandoverNoteHandler) Open(ctx context.Context, command OpenDeliveryHandoverNoteCommand) (DeliveryHandoverResult, error) {
	note, err := domain.OpenDeliveryHandoverNote(domain.DeliveryHandoverNoteSpec{
		TenantID:        command.TenantID,
		ID:              command.Note,
		Node:            command.Node,
		DeliveringParty: command.DeliveringParty,
		ClaimedAccounts: command.ClaimedAccounts,
		OpenedAt:        command.OpenedAt,
	})
	if err != nil {
		return DeliveryHandoverResult{}, fmt.Errorf("%w: %v", ErrDeliveryHandoverCommandRejected, err)
	}
	recorded, outcome, err := handler.deps.Notes.OpenDeliveryHandoverNote(ctx, note)
	if err != nil {
		return DeliveryHandoverResult{}, fmt.Errorf("open delivery handover note: %w", err)
	}
	switch outcome {
	case ports.DeliveryScopeAppended:
		return NewDeliveryHandoverResult(DeliveryHandoverNoteOpened, DeliveryHandoverAnswer{Note: recorded}), nil
	case ports.DeliveryScopeAlreadyRecorded:
		answer, err := handler.noteAnswer(ctx, recorded.TenantID(), recorded.ID())
		if err != nil {
			return DeliveryHandoverResult{}, err
		}
		if sameHandoverNote(recorded, note) {
			return NewDeliveryHandoverResult(DeliveryHandoverExistingResult, answer), nil
		}
		return NewDeliveryHandoverResult(DeliveryHandoverSourceConflict, answer), nil
	default:
		return DeliveryHandoverResult{}, fmt.Errorf("open delivery handover note: unexpected register outcome %q", outcome)
	}
}

// Assign 把一件作业实物归入开放的交接单（ADR-0212 决定三「成员」）。次序：交接单在这个节点看得见 → 同一归入事实已在册就按内容分
// 已有结果与来源冲突（重放在判条件之前：那件实物此后可能已被识别） → 交接单已关闭答状态不符 → 实物在开立节点上当前有效版本属于
// 重核对象（ADR-0199 决定三经 ADR-0211 放宽的那一组），否则答状态不符 → 落成员。归入只追加成员，不改写那件实物的来源、不追加判断
// 版本。交接单此刻已成立范围时，同一笔为那一件的当前有效版本自投门铃（ADR-0212 决定六）。
func (handler *DeliveryHandoverNoteHandler) Assign(ctx context.Context, command AssignToDeliveryHandoverNoteCommand) (DeliveryHandoverResult, error) {
	member, err := domain.FormDeliveryHandoverMember(domain.DeliveryHandoverMemberSpec{
		TenantID: command.TenantID,
		Note:     command.Note,
		Unit:     command.Unit,
		Kind:     domain.MembershipByAssignment,
		FactID:   strings.TrimSpace(command.FactID),
		FormedAt: command.AssignedAt,
	})
	if err == nil && strings.TrimSpace(command.Node.String()) == "" {
		err = errors.New("the envelope names no node")
	}
	if err != nil {
		return DeliveryHandoverResult{}, fmt.Errorf("%w: %v", ErrDeliveryHandoverCommandRejected, err)
	}
	state, err := handler.visibleAtNode(ctx, command.TenantID, command.Note, command.Node)
	if err != nil {
		return DeliveryHandoverResult{}, err
	}
	recorded, replayed, err := handler.deps.Notes.FindDeliveryHandoverMember(ctx, command.TenantID, domain.MembershipByAssignment, member.FactID())
	if err != nil {
		return DeliveryHandoverResult{}, fmt.Errorf("find the assignment: %w", err)
	}
	if replayed {
		return handler.memberReplay(ctx, recorded, member)
	}
	if state.Closed() {
		return mismatchOf(state, HandoverNoteAlreadyClosed), nil
	}
	judgments, err := handler.deps.Receptions.FindCurrentJudgmentsAtNode(ctx, command.TenantID, command.Unit, state.Note.Node())
	if err != nil {
		return DeliveryHandoverResult{}, fmt.Errorf("find the current judgments of the unit at the note's node: %w", err)
	}
	recheckable, assignable := assignableJudgments(judgments)
	if !assignable {
		return mismatchOf(state, HandoverUnitNotAssignable), nil
	}
	appended, outcome, err := handler.deps.Notes.AppendDeliveryHandoverMember(ctx, member)
	if err != nil {
		return DeliveryHandoverResult{}, fmt.Errorf("assign to the delivery handover note: %w", err)
	}
	switch outcome {
	case ports.DeliveryScopeAppended:
	case ports.DeliveryScopeAlreadyRecorded:
		// 并发里同一归入事实先一步落了：照重放答。
		return handler.memberReplay(ctx, appended, member)
	case ports.DeliveryScopeNoteClosed:
		// 读交接单之后、落成员之前另一笔关了它：关闭在交接单那一行上与加成员串行，这一次排在关闭之后。
		return mismatchOf(state, HandoverNoteAlreadyClosed), nil
	case ports.DeliveryScopeTargetNotFound:
		return DeliveryHandoverResult{}, ErrDeliveryHandoverNoteNotFound
	default:
		return DeliveryHandoverResult{}, fmt.Errorf("assign to the delivery handover note: unexpected register outcome %q", outcome)
	}
	// 落成员时已取交接单那一行的共享锁，与落结论的排他锁串行：此刻读到的结论里，要么已含让范围成立的那一个（这一笔替这件响门铃），
	// 要么那一个排在这一笔之后（它那一笔读得到这个成员，按交接单重核时带上这一件）。
	answer, err := handler.noteAnswer(ctx, command.TenantID, command.Note)
	if err != nil {
		return DeliveryHandoverResult{}, err
	}
	answer.Member = appended
	if !answer.ScopeEstablishedAt.IsZero() {
		for _, version := range recheckable {
			if err := handler.ring(ctx, command.TenantID, command.Note, version); err != nil {
				return DeliveryHandoverResult{}, err
			}
		}
	}
	return NewDeliveryHandoverResult(DeliveryHandoverUnitAssigned, answer), nil
}

// Close 关闭一张交接单，冻结成员。同一关闭事实原样重投答已有结果、换了关闭时刻答来源冲突；另一份关闭事实到来时交接单已关闭，答已有
// 结果并回显第一份——关闭是终态，两次关闭说的是同一件事。
func (handler *DeliveryHandoverNoteHandler) Close(ctx context.Context, command CloseDeliveryHandoverNoteCommand) (DeliveryHandoverResult, error) {
	closure, err := domain.FormDeliveryHandoverClosure(domain.DeliveryHandoverClosureSpec{
		TenantID: command.TenantID,
		Note:     command.Note,
		FactID:   strings.TrimSpace(command.FactID),
		ClosedAt: command.ClosedAt,
	})
	if err == nil && strings.TrimSpace(command.Node.String()) == "" {
		err = errors.New("the envelope names no node")
	}
	if err != nil {
		return DeliveryHandoverResult{}, fmt.Errorf("%w: %v", ErrDeliveryHandoverCommandRejected, err)
	}
	if _, err := handler.visibleAtNode(ctx, command.TenantID, command.Note, command.Node); err != nil {
		return DeliveryHandoverResult{}, err
	}
	recorded, outcome, err := handler.deps.Notes.CloseDeliveryHandoverNote(ctx, closure)
	if err != nil {
		return DeliveryHandoverResult{}, fmt.Errorf("close the delivery handover note: %w", err)
	}
	result := DeliveryHandoverExistingResult
	switch outcome {
	case ports.DeliveryScopeAppended:
		result = DeliveryHandoverNoteClosed
	case ports.DeliveryScopeAlreadyRecorded:
		if recorded.FactID() == closure.FactID() && !recorded.ClosedAt().Equal(closure.ClosedAt()) {
			result = DeliveryHandoverSourceConflict
		}
	case ports.DeliveryScopeTargetNotFound:
		return DeliveryHandoverResult{}, ErrDeliveryHandoverNoteNotFound
	default:
		return DeliveryHandoverResult{}, fmt.Errorf("close the delivery handover note: unexpected register outcome %q", outcome)
	}
	answer, err := handler.noteAnswer(ctx, command.TenantID, command.Note)
	if err != nil {
		return DeliveryHandoverResult{}, err
	}
	answer.Closure = recorded
	return NewDeliveryHandoverResult(result, answer), nil
}

// ConfirmByOperationDecision 是运营决定确认一个声称的客户账户的绑定（ADR-0212 决定三「运营决定」）。不限于已关闭的交接单：柜台上先
// 确认、再逐件收寄是常态。这个账户已有结论时按结论分已有结果与已有结论，不再问 party-commercial——重放的答案不随 PC 此刻的册变。否则
// 问授权交付判断（租户，交付方参与方，客户账户，开立节点，开立时刻）：是即落确认；否、未配置各答其格、不落结论；读不动不形成答案。
func (handler *DeliveryHandoverNoteHandler) ConfirmByOperationDecision(ctx context.Context, command ConfirmDeliveryHandoverCommand) (DeliveryHandoverResult, error) {
	operator, basis := strings.TrimSpace(command.Operator), strings.TrimSpace(command.Basis)
	if strings.TrimSpace(command.TenantID.String()) == "" || strings.TrimSpace(command.Note.String()) == "" ||
		strings.TrimSpace(command.Account.String()) == "" || strings.TrimSpace(command.DeliveringParticipant.String()) == "" ||
		operator == "" || basis == "" {
		return DeliveryHandoverResult{}, fmt.Errorf("%w: note, account, delivering participant, basis and operator are required", ErrDeliveryHandoverCommandRejected)
	}
	state, err := handler.claimedBy(ctx, command.TenantID, command.Note, command.Account)
	if err != nil {
		return DeliveryHandoverResult{}, err
	}
	if existing, concluded := state.BindingFor(command.Account); concluded {
		return conclusionReplay(state, existing, domain.BindingConfirmed), nil
	}
	answer, err := handler.deps.AuthorizedDelivery.JudgeAuthorizedDelivery(ctx, ports.AuthorizedDeliveryQuery{
		Tenant:          command.TenantID,
		DeliveringParty: command.DeliveringParticipant,
		CustomerAccount: command.Account.String(),
		Node:            state.Note.Node(),
		OpenedAt:        state.Note.OpenedAt(),
	})
	if err != nil {
		return DeliveryHandoverResult{}, fmt.Errorf("judge the authorized delivery: %w", err)
	}
	switch answer {
	case ports.DeliveryAuthorized:
	case ports.DeliveryNotAuthorized:
		return NewDeliveryHandoverResult(DeliveryHandoverNotAuthorized, answerOf(state)), nil
	case ports.DeliveryAuthorizationNotConfigured:
		return NewDeliveryHandoverResult(DeliveryHandoverAuthorizationNotConfigured, answerOf(state)), nil
	default:
		return DeliveryHandoverResult{}, fmt.Errorf("judge the authorized delivery: unexpected answer %q", answer)
	}
	binding, err := domain.FormDeliveryHandoverBinding(domain.DeliveryHandoverBindingSpec{
		TenantID:              command.TenantID,
		Note:                  command.Note,
		Account:               command.Account,
		Conclusion:            domain.BindingConfirmed,
		Route:                 domain.ConcludedViaOperationDecision,
		Concluder:             operator,
		DeliveringParticipant: command.DeliveringParticipant.String(),
		Basis:                 basis,
		ConcludedAt:           handler.deps.Clock.Now(),
	})
	if err != nil {
		return DeliveryHandoverResult{}, fmt.Errorf("%w: %v", ErrDeliveryHandoverCommandRejected, err)
	}
	return handler.conclude(ctx, binding)
}

// ConcludeByCustomer 是客户账户经客户渠道对交接单作结论（ADR-0212 决定三「客户渠道」）：只对已关闭的交接单——客户确认的是一份冻结的
// 成员清单；结论是确认或否认，都是终态。客户账户只取信封，不在声称之列与查无此单同答。
func (handler *DeliveryHandoverNoteHandler) ConcludeByCustomer(ctx context.Context, command ConcludeDeliveryHandoverCommand) (DeliveryHandoverResult, error) {
	channel, requestKey := strings.TrimSpace(command.Channel), strings.TrimSpace(command.RequestKey)
	if strings.TrimSpace(command.TenantID.String()) == "" || strings.TrimSpace(command.Note.String()) == "" ||
		strings.TrimSpace(command.CustomerAccount.String()) == "" || channel == "" || requestKey == "" || command.Conclusion.String() == "" {
		return DeliveryHandoverResult{}, fmt.Errorf("%w: note, conclusion and the envelope's client, source request key and account are required", ErrDeliveryHandoverCommandRejected)
	}
	state, err := handler.claimedBy(ctx, command.TenantID, command.Note, command.CustomerAccount)
	if err != nil {
		return DeliveryHandoverResult{}, err
	}
	if !state.Closed() {
		return mismatchOf(state, HandoverNoteNotClosed), nil
	}
	if existing, concluded := state.BindingFor(command.CustomerAccount); concluded {
		return conclusionReplay(state, existing, command.Conclusion), nil
	}
	binding, err := domain.FormDeliveryHandoverBinding(domain.DeliveryHandoverBindingSpec{
		TenantID:         command.TenantID,
		Note:             command.Note,
		Account:          command.CustomerAccount,
		Conclusion:       command.Conclusion,
		Route:            domain.ConcludedViaCustomerChannel,
		Concluder:        channel,
		SourceRequestKey: requestKey,
		ConcludedAt:      handler.deps.Clock.Now(),
	})
	if err != nil {
		return DeliveryHandoverResult{}, fmt.Errorf("%w: %v", ErrDeliveryHandoverCommandRejected, err)
	}
	return handler.conclude(ctx, binding)
}

// conclude 落一份结论；它是确认、且交接单因此成立范围时，同一笔自投门铃（租户，交接单）。落结论取交接单那一行的排他锁：声称了几个
// 账户、几个确认并发到来时它们排成一队，排在最后的那一笔读得到前面各笔的结论，恰有一笔看到范围成立。门铃的身份由交接单认领，
// 重放撞同一封。
func (handler *DeliveryHandoverNoteHandler) conclude(ctx context.Context, binding domain.DeliveryHandoverBinding) (DeliveryHandoverResult, error) {
	recorded, outcome, err := handler.deps.Notes.RecordDeliveryHandoverBinding(ctx, binding)
	if err != nil {
		return DeliveryHandoverResult{}, fmt.Errorf("record the binding conclusion: %w", err)
	}
	switch outcome {
	case ports.DeliveryScopeAppended:
	case ports.DeliveryScopeAlreadyRecorded:
		answer, err := handler.noteAnswer(ctx, binding.TenantID(), binding.Note())
		if err != nil {
			return DeliveryHandoverResult{}, err
		}
		answer.Binding = recorded
		if recorded.Conclusion() == binding.Conclusion() {
			return NewDeliveryHandoverResult(DeliveryHandoverExistingResult, answer), nil
		}
		return NewDeliveryHandoverResult(DeliveryHandoverExistingConclusion, answer), nil
	case ports.DeliveryScopeTargetNotFound:
		return DeliveryHandoverResult{}, ErrDeliveryHandoverNoteNotFound
	default:
		return DeliveryHandoverResult{}, fmt.Errorf("record the binding conclusion: unexpected register outcome %q", outcome)
	}
	answer, err := handler.noteAnswer(ctx, binding.TenantID(), binding.Note())
	if err != nil {
		return DeliveryHandoverResult{}, err
	}
	answer.Binding = recorded
	if recorded.Conclusion() == domain.BindingDenied {
		return NewDeliveryHandoverResult(DeliveryHandoverBindingDenied, answer), nil
	}
	if !answer.ScopeEstablishedAt.IsZero() {
		if err := handler.ring(ctx, binding.TenantID(), binding.Note(), domain.IntakeResultVersion{}); err != nil {
			return DeliveryHandoverResult{}, err
		}
	}
	return NewDeliveryHandoverResult(DeliveryHandoverBindingConfirmed, answer), nil
}

// ring 自投一封「交付范围成立」门铃。入不了队就不形成答案：没有门铃，范围成立了，名下的待识别也等不来重核。
func (handler *DeliveryHandoverNoteHandler) ring(ctx context.Context, tenant domain.TenantID, note domain.DeliveryHandoverNoteID, version domain.IntakeResultVersion) error {
	if err := handler.deps.Doorbell.RingDeliveryScopeEstablished(ctx, ports.DeliveryScopeEstablishedIntent{
		Tenant:     tenant,
		Note:       note,
		Version:    version,
		OccurredAt: handler.deps.Clock.Now(),
	}); err != nil {
		return fmt.Errorf("ring the delivery scope doorbell: %w", err)
	}
	return nil
}

// visibleAtNode 读一张作业事实指名的交接单：不在本租户、或不是这个节点开立的，答查无此单。作业事实信封的作业范围是一个节点，别的
// 节点的交接单不在这台设备的作业范围里。
func (handler *DeliveryHandoverNoteHandler) visibleAtNode(ctx context.Context, tenant domain.TenantID, note domain.DeliveryHandoverNoteID, node domain.NodeReference) (domain.DeliveryHandoverNoteState, error) {
	state, found, err := handler.deps.Notes.FindDeliveryHandoverNote(ctx, tenant, note)
	if err != nil {
		return domain.DeliveryHandoverNoteState{}, fmt.Errorf("find the delivery handover note: %w", err)
	}
	if !found || state.Note.Node() != node {
		return domain.DeliveryHandoverNoteState{}, ErrDeliveryHandoverNoteNotFound
	}
	return state, nil
}

// claimedBy 读一张结论指名的交接单：不在本租户、或客户账户不在声称之列，答查无此单。
func (handler *DeliveryHandoverNoteHandler) claimedBy(ctx context.Context, tenant domain.TenantID, note domain.DeliveryHandoverNoteID, account domain.CustomerAccountReference) (domain.DeliveryHandoverNoteState, error) {
	state, found, err := handler.deps.Notes.FindDeliveryHandoverNote(ctx, tenant, note)
	if err != nil {
		return domain.DeliveryHandoverNoteState{}, fmt.Errorf("find the delivery handover note: %w", err)
	}
	if !found || !state.Note.Claims(account) {
		return domain.DeliveryHandoverNoteState{}, ErrDeliveryHandoverNoteNotFound
	}
	return state, nil
}

// noteAnswer 读交接单此刻的证据状态，交回答复里交接单那几格。
func (handler *DeliveryHandoverNoteHandler) noteAnswer(ctx context.Context, tenant domain.TenantID, note domain.DeliveryHandoverNoteID) (DeliveryHandoverAnswer, error) {
	state, found, err := handler.deps.Notes.FindDeliveryHandoverNote(ctx, tenant, note)
	if err != nil {
		return DeliveryHandoverAnswer{}, fmt.Errorf("read the delivery handover note back: %w", err)
	}
	if !found {
		return DeliveryHandoverAnswer{}, fmt.Errorf("node operations: delivery handover note %q recorded but not readable", note)
	}
	return answerOf(state), nil
}

// memberReplay 按在册的那一份归入分已有结果与来源冲突：同一归入事实指名同一张交接单、同一件实物、同一业务时刻才算同一份。
func (handler *DeliveryHandoverNoteHandler) memberReplay(ctx context.Context, recorded, member domain.DeliveryHandoverMember) (DeliveryHandoverResult, error) {
	answer, err := handler.noteAnswer(ctx, recorded.TenantID(), recorded.Note())
	if err != nil {
		return DeliveryHandoverResult{}, err
	}
	answer.Member = recorded
	if recorded.Note() == member.Note() && recorded.Unit() == member.Unit() && recorded.FormedAt().Equal(member.FormedAt()) {
		return NewDeliveryHandoverResult(DeliveryHandoverExistingResult, answer), nil
	}
	return NewDeliveryHandoverResult(DeliveryHandoverSourceConflict, answer), nil
}

// conclusionReplay 按在册结论答：同结论答已有结果，异结论答已有结论，都回显在册那一份。
func conclusionReplay(state domain.DeliveryHandoverNoteState, existing domain.DeliveryHandoverBinding, asked domain.HandoverBindingConclusion) DeliveryHandoverResult {
	answer := answerOf(state)
	answer.Binding = existing
	if existing.Conclusion() == asked {
		return NewDeliveryHandoverResult(DeliveryHandoverExistingResult, answer)
	}
	return NewDeliveryHandoverResult(DeliveryHandoverExistingConclusion, answer)
}

func mismatchOf(state domain.DeliveryHandoverNoteState, mismatch HandoverStateMismatch) DeliveryHandoverResult {
	answer := answerOf(state)
	answer.StateMismatch = mismatch
	return NewDeliveryHandoverResult(DeliveryHandoverNoteStateMismatch, answer)
}

func answerOf(state domain.DeliveryHandoverNoteState) DeliveryHandoverAnswer {
	established, _ := state.ScopeEstablishedAt()
	return DeliveryHandoverAnswer{Note: state.Note, Closure: state.Closure, ScopeEstablishedAt: established}
}

// sameHandoverNote 比开立内容：节点、交付方线索、声称（已按标识定序）与开立时刻逐格相等。
func sameHandoverNote(left, right domain.DeliveryHandoverNote) bool {
	return left.Node() == right.Node() && left.DeliveringParty() == right.DeliveringParty() &&
		slices.Equal(left.ClaimedAccounts(), right.ClaimedAccounts()) && left.OpenedAt().Equal(right.OpenedAt())
}

// assignableJudgments 判一件实物在开立节点上能不能归入：要有当前收寄（形成或待识别的判断链），且每一条都属于重核对象——不带关联、
// 不记身份冲突或冲突来由是只因歧义信号、来源已保全（ADR-0212 决定三，重核对象照 ADR-0199 决定三经 ADR-0211 放宽）。只要有一条已
// 关联，这件实物在这个节点已被识别，归入补不了任何证据。交回那几版的判断版本：范围已成立时为它们各响一封门铃。
func assignableJudgments(judgments []ports.ReceptionRecord) ([]domain.IntakeResultVersion, bool) {
	var versions []domain.IntakeResultVersion
	for _, record := range judgments {
		if record.Kind != ports.RecordIntakeFormed && record.Kind != ports.RecordPendingIdentification {
			continue
		}
		_, associated := record.Intake.Association()
		contested := record.IdentityConflict && record.IdentityConflictCause != ports.ConflictByAmbiguityOnly
		if record.Kind != ports.RecordPendingIdentification || associated || contested || !record.Source.Preserved() {
			return nil, false
		}
		versions = append(versions, record.JudgmentVersion)
	}
	return versions, len(versions) > 0
}
