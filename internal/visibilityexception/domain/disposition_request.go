package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidDispositionRequest = errors.New("visibility exception: invalid disposition request")
	ErrRequestWindowExpired      = errors.New("visibility exception: the acceptance window has expired")
	ErrRequestAlreadyJudged      = errors.New("visibility exception: the request is already judged")
	ErrRequestNotJudged          = errors.New("visibility exception: the request is not judged yet")
)

// DispositionRequestID 是处置请求的稳定身份。
type DispositionRequestID struct{ requiredValue }

func NewDispositionRequestID(value string) (DispositionRequestID, error) {
	required, err := newRequiredValue("disposition request ID", value)
	return DispositionRequestID{required}, err
}

// RequestedActionReference 指名请求动作（改路、拦截、重作业、重新履约、重新申报）。
// 动作语汇属各目标上下文，这里是开放引用。
type RequestedActionReference struct{ requiredValue }

func NewRequestedActionReference(value string) (RequestedActionReference, error) {
	required, err := newRequiredValue("requested action reference", value)
	return RequestedActionReference{required}, err
}

// RequestScopeReference 指名请求的明确范围。
type RequestScopeReference struct{ requiredValue }

func NewRequestScopeReference(value string) (RequestScopeReference, error) {
	required, err := newRequiredValue("request scope reference", value)
	return RequestScopeReference{required}, err
}

// RequestEvidenceReference 指名随请求携带的证据。
type RequestEvidenceReference struct{ requiredValue }

func NewRequestEvidenceReference(value string) (RequestEvidenceReference, error) {
	required, err := newRequiredValue("request evidence reference", value)
	return RequestEvidenceReference{required}, err
}

// SourceJudgment 是源上下文对请求的判断封闭四走向（CONTEXT 生命周期：「接受、部分
// 接受、拒绝或要求补充：结果只表达请求判断，不证明实际执行」）。
type SourceJudgment uint8

const (
	SourceJudgmentInvalid SourceJudgment = iota
	RequestAccepted
	RequestPartiallyAccepted
	RequestRefused
	SupplementRequired
)

func (judgment SourceJudgment) valid() bool {
	return judgment >= RequestAccepted && judgment <= SupplementRequired
}

func (judgment SourceJudgment) String() string {
	switch judgment {
	case RequestAccepted:
		return "ACCEPTED"
	case RequestPartiallyAccepted:
		return "PARTIALLY_ACCEPTED"
	case RequestRefused:
		return "REFUSED"
	case SupplementRequired:
		return "SUPPLEMENT_REQUIRED"
	default:
		return ""
	}
}

// CancellationAnswer 是目标上下文对取消/替代意图的答复封闭四值（CONTEXT 硬句 149：
// 「目标上下文必须分别返回取消已接受、部分取消、已无法取消或拒绝取消」）。
type CancellationAnswer uint8

const (
	CancellationAnswerInvalid CancellationAnswer = iota
	CancellationAcceptedByTarget
	PartiallyCancelled
	NoLongerCancellable
	CancellationRefusedByTarget
)

func (answer CancellationAnswer) valid() bool {
	return answer >= CancellationAcceptedByTarget && answer <= CancellationRefusedByTarget
}

func (answer CancellationAnswer) String() string {
	switch answer {
	case CancellationAcceptedByTarget:
		return "CANCELLATION_ACCEPTED"
	case PartiallyCancelled:
		return "PARTIALLY_CANCELLED"
	case NoLongerCancellable:
		return "NO_LONGER_CANCELLABLE"
	case CancellationRefusedByTarget:
		return "CANCELLATION_REFUSED"
	default:
		return ""
	}
}

// DispositionRequestSpec 是形成一份处置请求所需的全部输入。
type DispositionRequestSpec struct {
	ID               DispositionRequestID
	Case             CaseID
	Target           SourceContext
	Action           RequestedActionReference
	Scope            RequestScopeReference
	Reason           string
	Evidence         RequestEvidenceReference
	IntentVersion    int
	SentAt           time.Time
	AcceptanceWindow time.Time
}

// DispositionRequest 是异常案件向源业务上下文提出的结构化业务请求。请求已发送、源
// 上下文已接受、取消/替代意图、目标方取消结果和实际动作已完成是不同结果（CONTEXT
// 语言）——本类型只管前四样，实际执行结果由目标上下文按事实返回，这里没有它的字段；
// 案件也不能因请求已发送或已接受而推定动作完成。
type DispositionRequest struct {
	id            DispositionRequestID
	caseID        CaseID
	target        SourceContext
	action        RequestedActionReference
	scope         RequestScopeReference
	reason        string
	evidence      RequestEvidenceReference
	intentVersion int
	sentAt        time.Time
	window        time.Time
	judgment      SourceJudgment
	judgedAt      time.Time
	cancellation  CancellationAnswer
	supersededBy  DispositionRequestID
}

// SendDispositionRequest 形成并发送一份请求：目标、动作、范围、原因、证据与时限
// 缺一不可（CONTEXT 硬句 145）。受理有效期可缺席（如适用）。
func SendDispositionRequest(spec DispositionRequestSpec) (*DispositionRequest, error) {
	if !spec.ID.valid() ||
		!spec.Case.valid() ||
		!spec.Target.valid() ||
		!spec.Action.valid() ||
		!spec.Scope.valid() ||
		spec.Reason == "" ||
		!spec.Evidence.valid() ||
		spec.IntentVersion <= 0 ||
		spec.SentAt.IsZero() {
		return nil, ErrInvalidDispositionRequest
	}
	if !spec.AcceptanceWindow.IsZero() && !spec.AcceptanceWindow.After(spec.SentAt) {
		return nil, ErrInvalidDispositionRequest
	}
	return &DispositionRequest{
		id:            spec.ID,
		caseID:        spec.Case,
		target:        spec.Target,
		action:        spec.Action,
		scope:         spec.Scope,
		reason:        spec.Reason,
		evidence:      spec.Evidence,
		intentVersion: spec.IntentVersion,
		sentAt:        spec.SentAt.UTC(),
		window:        spec.AcceptanceWindow.UTC(),
	}, nil
}

func (request *DispositionRequest) ID() DispositionRequestID {
	return request.id
}

func (request *DispositionRequest) Case() CaseID {
	return request.caseID
}

func (request *DispositionRequest) Target() SourceContext {
	return request.target
}

// Action 与 Scope 是（案件+动作+范围）幂等键的两维，适配器按它们建索引——不导出，
// 存储连键都立不起来。
func (request *DispositionRequest) Action() RequestedActionReference {
	return request.action
}

func (request *DispositionRequest) Scope() RequestScopeReference {
	return request.scope
}

func (request *DispositionRequest) IntentVersion() int {
	return request.intentVersion
}

// Judgment 报告源上下文的判断及是否已作出。
func (request *DispositionRequest) Judgment() (SourceJudgment, bool) {
	return request.judgment, request.judgment.valid()
}

// CancellationOutcome 报告目标上下文对取消意图的答复及是否在场。
func (request *DispositionRequest) CancellationOutcome() (CancellationAnswer, bool) {
	return request.cancellation, request.cancellation.valid()
}

// SupersededBy 只在被替代的请求上给出。
func (request *DispositionRequest) SupersededBy() (DispositionRequestID, bool) {
	return request.supersededBy, request.supersededBy.valid()
}

// DispositionRequestSnapshot 是持久化层重建处置请求所需的全量状态。判断、取消答复
// 与替代指向都是已发生的交互历史——重建不重演（重演需要按原次序原时间走一遍，而库里
// 只有结果）。
type DispositionRequestSnapshot struct {
	ID               DispositionRequestID
	Case             CaseID
	Target           SourceContext
	Action           RequestedActionReference
	Scope            RequestScopeReference
	Reason           string
	Evidence         RequestEvidenceReference
	IntentVersion    int
	SentAt           time.Time
	AcceptanceWindow time.Time
	Judgment         SourceJudgment
	JudgedAt         time.Time
	Cancellation     CancellationAnswer
	SupersededBy     DispositionRequestID
}

// Snapshot 折出处置请求的全量状态供持久化。
func (request *DispositionRequest) Snapshot() DispositionRequestSnapshot {
	return DispositionRequestSnapshot{
		ID:               request.id,
		Case:             request.caseID,
		Target:           request.target,
		Action:           request.action,
		Scope:            request.scope,
		Reason:           request.reason,
		Evidence:         request.evidence,
		IntentVersion:    request.intentVersion,
		SentAt:           request.sentAt,
		AcceptanceWindow: request.window,
		Judgment:         request.judgment,
		JudgedAt:         request.judgedAt,
		Cancellation:     request.cancellation,
		SupersededBy:     request.supersededBy,
	}
}

// RehydrateDispositionRequest 从快照重建处置请求并重验交互历史形状：判断与判断时间
// 同在场、答复必在判断之后、替代不指自己——一次坏写入不得变成一段看起来合法的交互。
func RehydrateDispositionRequest(snapshot DispositionRequestSnapshot) (*DispositionRequest, error) {
	request, err := SendDispositionRequest(DispositionRequestSpec{
		ID:               snapshot.ID,
		Case:             snapshot.Case,
		Target:           snapshot.Target,
		Action:           snapshot.Action,
		Scope:            snapshot.Scope,
		Reason:           snapshot.Reason,
		Evidence:         snapshot.Evidence,
		IntentVersion:    snapshot.IntentVersion,
		SentAt:           snapshot.SentAt,
		AcceptanceWindow: snapshot.AcceptanceWindow,
	})
	if err != nil {
		return nil, err
	}
	if snapshot.Judgment.valid() != !snapshot.JudgedAt.IsZero() {
		return nil, ErrInvalidDispositionRequest
	}
	if snapshot.Judgment.valid() && snapshot.JudgedAt.Before(snapshot.SentAt) {
		return nil, ErrInvalidDispositionRequest
	}
	if snapshot.Cancellation.valid() && !snapshot.Judgment.valid() {
		return nil, ErrInvalidDispositionRequest
	}
	if snapshot.SupersededBy.valid() && snapshot.SupersededBy == snapshot.ID {
		return nil, ErrInvalidDispositionRequest
	}
	request.judgment = snapshot.Judgment
	request.judgedAt = snapshot.JudgedAt.UTC()
	request.cancellation = snapshot.Cancellation
	request.supersededBy = snapshot.SupersededBy
	return request, nil
}

// RecordSourceJudgment 记录源上下文的判断。受理有效期届满后，尚未被接受的范围不得
// 再按旧请求启动（CONTEXT 硬句 150）——过期请求不再吸收接受；已判断的请求不判第二次。
func (request *DispositionRequest) RecordSourceJudgment(judgment SourceJudgment, at time.Time) error {
	if _, judged := request.Judgment(); judged {
		return ErrRequestAlreadyJudged
	}
	if !judgment.valid() || at.IsZero() || at.Before(request.sentAt) {
		return ErrInvalidDispositionRequest
	}
	if !request.window.IsZero() && at.After(request.window) &&
		(judgment == RequestAccepted || judgment == RequestPartiallyAccepted) {
		return ErrRequestWindowExpired
	}
	request.judgment = judgment
	request.judgedAt = at.UTC()
	return nil
}

// RecordCancellationAnswer 记录目标上下文对取消/替代意图的答复。取消只改变未来意图，
// 不撤销已经发生的源业务事实（CONTEXT 硬句 149）——答复是四值封闭，「已无法取消」和
// 「部分取消」都是如实结果；已接受或已开始的范围继续按事实返回执行结果，那不在这里。
// 只有已判断的请求才谈得上取消——还没人接的请求撤回是另一回事（未判断即无外部意图）。
func (request *DispositionRequest) RecordCancellationAnswer(answer CancellationAnswer, at time.Time) error {
	if _, judged := request.Judgment(); !judged {
		return ErrRequestNotJudged
	}
	if _, answered := request.CancellationOutcome(); answered {
		return ErrRequestAlreadyJudged
	}
	if !answer.valid() || at.IsZero() || at.Before(request.judgedAt) {
		return ErrInvalidDispositionRequest
	}
	request.cancellation = answer
	return nil
}

// SupersedeWith 以新请求替代本请求的未来意图：新请求必须是不同身份、更高意图版本。
// 原请求与其已有判断原样保留——替代不是删除，案件状态变化也不能静默使旧请求失效
// （CONTEXT 硬句 148）。
func (request *DispositionRequest) SupersedeWith(successor *DispositionRequest) error {
	if successor == nil ||
		successor.id == request.id ||
		successor.caseID != request.caseID ||
		successor.intentVersion <= request.intentVersion {
		return ErrInvalidDispositionRequest
	}
	if _, superseded := request.SupersededBy(); superseded {
		return ErrRequestAlreadyJudged
	}
	request.supersededBy = successor.id
	return nil
}
