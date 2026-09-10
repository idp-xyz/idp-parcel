package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidAuthorizedDisposition      = errors.New("parcel shipment: invalid authorized disposition")
	ErrNotWaitingOnAuthorizedDisposition = errors.New("parcel shipment: the acceptance judgment task is not waiting on authorized disposition")
	// ErrAuthorizedDispositionAlreadyRecorded 与 ErrManualReviewAlreadyCompleted 同一分格理由：第二次要么是
	// 重复提交要么是换人改主意，两者都不该静默覆盖第一次的留痕。
	ErrAuthorizedDispositionAlreadyRecorded = errors.New("parcel shipment: an authorized disposition is already recorded for this submission version")
)

// 处置的四项引用都属别处：授权规则属 party-commercial，处置方、原因目录与证据属运营留痕。本上下文
// 只记引用，不拥有角色目录，也不校验某个角色够不够格——与复核完成、主动拒绝同一条纪律。四个类型
// 不复用主动拒绝那一组：处置权与拒绝权在 PC CONTEXT 里互不蕴含，共用类型会让「谁授的处置权」在
// 类型上就与「谁授的拒绝权」分不开。

type DispositionAuthorityReference struct{ requiredValue }

func NewDispositionAuthorityReference(value string) (DispositionAuthorityReference, error) {
	required, err := newRequiredValue("disposition authority reference", value)
	return DispositionAuthorityReference{required}, err
}

type DisposerReference struct{ requiredValue }

func NewDisposerReference(value string) (DisposerReference, error) {
	required, err := newRequiredValue("disposer reference", value)
	return DisposerReference{required}, err
}

// DispositionReasonReference 是结构化原因，不是自由文本：处置`拒绝`要按原因维度可统计，与主动拒绝同理。
type DispositionReasonReference struct{ requiredValue }

func NewDispositionReasonReference(value string) (DispositionReasonReference, error) {
	required, err := newRequiredValue("disposition reason reference", value)
	return DispositionReasonReference{required}, err
}

type DispositionEvidenceReference struct{ requiredValue }

func NewDispositionEvidenceReference(value string) (DispositionEvidenceReference, error) {
	required, err := newRequiredValue("disposition evidence reference", value)
	return DispositionEvidenceReference{required}, err
}

// AuthorizedDispositionChoice 是授权角色对当前提交版本选定的去向，封闭两值（ADR-0132 决定一）。
//
// 「放行」刻意不在集内：CONTEXT「人工处理不得绕过硬规则或把缺少的权威结果改成通过」，而已记录的
// `业务限制`是 settlement-accounting 交回的权威结果。`补资金后重判`也不在：它的触发（资金事实到本
// 上下文）今天没有信封承载，ADR-0094 决定四不许只开格不给触发——那是后继，不是否决。
type AuthorizedDispositionChoice uint8

const (
	AuthorizedDispositionChoiceInvalid AuthorizedDispositionChoice = iota
	// DisposeByRejection 形成授权角色拒绝决定：留痕同主动拒绝，依据经接受前财务控制采用结果回指。
	DisposeByRejection
	// DisposeByCustomerSupplement 让本版本不再判，任务转入`等待受控补充`，由既有「新提交版本已形成」
	// 信封续办（ADR-0106）。补充越出委托边界时按 CONTEXT 既有规则形成关联新委托——那是受控补充的
	// 一种结果，不是第三个去向。
	DisposeByCustomerSupplement
)

func (choice AuthorizedDispositionChoice) valid() bool {
	return choice == DisposeByRejection || choice == DisposeByCustomerSupplement
}

func (choice AuthorizedDispositionChoice) String() string {
	switch choice {
	case DisposeByRejection:
		return "REJECT"
	case DisposeByCustomerSupplement:
		return "CUSTOMER_SUPPLEMENT"
	default:
		return ""
	}
}

type AuthorizedDispositionSpec struct {
	Choice     AuthorizedDispositionChoice
	Authority  DispositionAuthorityReference
	Disposer   DisposerReference
	Reason     DispositionReasonReference
	Evidence   DispositionEvidenceReference
	DisposedAt time.Time
}

// AuthorizedDisposition 是一次已作出的授权处置：去向 × 实际处置方 × 授权引用 × 原因引用 × 证据引用 ×
// 处置时点。引用全部必填——少授权说不出这次处置凭什么算数，少处置方无从追责，少原因统计不了，少
// 证据与「有人点了一下」分不开。
type AuthorizedDisposition struct {
	choice     AuthorizedDispositionChoice
	authority  DispositionAuthorityReference
	disposer   DisposerReference
	reason     DispositionReasonReference
	evidence   DispositionEvidenceReference
	disposedAt time.Time
}

func NewAuthorizedDisposition(spec AuthorizedDispositionSpec) (AuthorizedDisposition, error) {
	if !spec.Choice.valid() || !spec.Authority.valid() || !spec.Disposer.valid() ||
		!spec.Reason.valid() || !spec.Evidence.valid() || spec.DisposedAt.IsZero() {
		return AuthorizedDisposition{}, ErrInvalidAuthorizedDisposition
	}
	return AuthorizedDisposition{
		choice:     spec.Choice,
		authority:  spec.Authority,
		disposer:   spec.Disposer,
		reason:     spec.Reason,
		evidence:   spec.Evidence,
		disposedAt: spec.DisposedAt.UTC(),
	}, nil
}

func (disposition AuthorizedDisposition) Choice() AuthorizedDispositionChoice {
	return disposition.choice
}

func (disposition AuthorizedDisposition) Authority() DispositionAuthorityReference {
	return disposition.authority
}

func (disposition AuthorizedDisposition) Disposer() DisposerReference {
	return disposition.disposer
}

func (disposition AuthorizedDisposition) Reason() DispositionReasonReference {
	return disposition.reason
}

func (disposition AuthorizedDisposition) Evidence() DispositionEvidenceReference {
	return disposition.evidence
}

func (disposition AuthorizedDisposition) DisposedAt() time.Time {
	return disposition.disposedAt
}

// recorded 以去向在集内为界：包外只能经全校验的构造器造出非零值，所以去向在场即整份在场。
func (disposition AuthorizedDisposition) recorded() bool {
	return disposition.choice.valid()
}

// AuthorizedDisposition 交回本任务上已记录的授权处置。未处置时报告缺席而不是给零值——零值与
// 「处置了但没带授权」在读的人眼里一样。
func (task AcceptanceDecisionTask) AuthorizedDisposition() (AuthorizedDisposition, bool) {
	return task.authorizedDisposition, task.authorizedDisposition.recorded()
}

type DisposeUnderAuthoritySpec struct {
	Disposition AuthorizedDisposition
	// DecisionID 只在去向为`拒绝`时收：那一支在本转移里形成拒绝决定，决定要有自己的标识。
	// `交客户补充`不形成决定，不收标识。
	DecisionID AcceptanceDecisionID
}

// DisposeUnderAuthority 由授权处置角色对一份停在`等待授权处置`的委托选定去向（ADR-0132）。
//
// 它不判断这个角色够不够格：授权规则属 party-commercial，编排消费那边的授权结果后才到这里，本方法
// 保存所采用的授权引用。也不重读策略正文：进不进这一格已由受限项上的采用引用在形成控制判断时定下。
//
// 四条前置各自说出真实原因：已越过决定边界（`任务已完结`，编排据以交回那一个决定）、任务已收工、
// 一版至多一次（第二次不覆盖第一次的留痕）、没停在等处置（停在别的等待态或根本没停）。同一版本已
// 处置的判在等待态之前：`交客户补充`会把等待态带到`等待受控补充`，只看等待态会把第二次处置误报成
// 「没停在等处置」，而真实原因是这一版已经处置过了。
//
// `拒绝`复用主动拒绝形成决定的那道门：实际决定方即处置方，依据经接受前财务控制采用结果回指、不复制
// 校验明细进决定（ADR-0132 决定四）。`交客户补充`只写等待态：不动状态、不形成决定、不动版本，与
// AwaitOperatorRegistration 同形——之后由「新提交版本已形成」信封续办。两个去向都不在这里释放占用：
// 释放是向 settlement-accounting 发的请求，归编排在同一命令事务里按 OccupationFormed 处理。
func (request ShipmentRequest) DisposeUnderAuthority(spec DisposeUnderAuthoritySpec) (ShipmentRequest, error) {
	if !spec.Disposition.recorded() {
		return ShipmentRequest{}, ErrInvalidAuthorizedDisposition
	}
	if request.decisionFormed {
		return ShipmentRequest{}, ErrDecisionAlreadyFormed
	}
	if request.state != ShipmentRequestSubmitted {
		return ShipmentRequest{}, ErrInvalidShipmentRequest
	}
	if !request.acceptanceTask.running() {
		return ShipmentRequest{}, ErrAcceptanceTaskComplete
	}
	if request.acceptanceTask.authorizedDisposition.recorded() {
		return ShipmentRequest{}, ErrAuthorizedDispositionAlreadyRecorded
	}
	if request.acceptanceTask.waitingOn != ResumeByAuthorizedDisposition {
		return ShipmentRequest{}, ErrNotWaitingOnAuthorizedDisposition
	}

	switch spec.Disposition.choice {
	case DisposeByRejection:
		if !spec.DecisionID.valid() {
			return ShipmentRequest{}, ErrInvalidAcceptanceDecision
		}
		// 处置的引用逐格译成拒绝留痕：两组类型分立是为了让处置权与拒绝权在类型上分得开，而形成
		// 的拒绝决定本身与主动拒绝同形——读决定的人看到的是「授权角色依据结构化原因和证据决定不
		// 承担」，处置权由任务上的处置记录说明。
		request = request.formActiveRejection(spec.DecisionID, ActiveRejection{
			authority: RejectionAuthorityReference{spec.Disposition.authority.requiredValue},
			decider:   DeciderReference{spec.Disposition.disposer.requiredValue},
			reason:    RejectionReasonReference{spec.Disposition.reason.requiredValue},
			evidence:  RejectionEvidenceReference{spec.Disposition.evidence.requiredValue},
		}, CommercialBasisSnapshot{}, spec.Disposition.disposedAt)
	case DisposeByCustomerSupplement:
		request.acceptanceTask.waitingOn = ResumeByCustomerSupplement
	default:
		return ShipmentRequest{}, ErrInvalidAuthorizedDisposition
	}
	request.acceptanceTask.authorizedDisposition = spec.Disposition
	return request, nil
}
