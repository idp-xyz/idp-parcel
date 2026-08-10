package domain

import (
	"errors"
	"time"
)

var ErrInvalidActiveRejection = errors.New("parcel shipment: invalid active rejection")

// 主动拒绝的四项留痕分属两边：授权规则与原因目录属 party-commercial，实际决定方与证据属
// 运营留痕。本上下文只记引用——它不拥有角色等级，也不判断某个原因码是否在目录里，那些是
// `BD-PS-002` 的未确认参数。

type RejectionAuthorityReference struct{ requiredValue }

func NewRejectionAuthorityReference(value string) (RejectionAuthorityReference, error) {
	required, err := newRequiredValue("rejection authority reference", value)
	return RejectionAuthorityReference{required}, err
}

type DeciderReference struct{ requiredValue }

func NewDeciderReference(value string) (DeciderReference, error) {
	required, err := newRequiredValue("decider reference", value)
	return DeciderReference{required}, err
}

// RejectionReasonReference 是结构化原因，不是自由文本。用例要求拒绝按原因维度可统计，
// 一句自由说明既统计不了，也无从判断它是否落在授权目录内。
type RejectionReasonReference struct{ requiredValue }

func NewRejectionReasonReference(value string) (RejectionReasonReference, error) {
	required, err := newRequiredValue("rejection reason reference", value)
	return RejectionReasonReference{required}, err
}

type RejectionEvidenceReference struct{ requiredValue }

func NewRejectionEvidenceReference(value string) (RejectionEvidenceReference, error) {
	required, err := newRequiredValue("rejection evidence reference", value)
	return RejectionEvidenceReference{required}, err
}

// ActiveRejection 是运营企业不承担该服务请求的决定依据。它与规则形成的拒绝分开保存：后者
// 的解释在校验结果里，前者的解释在授权与原因里，混成一种会让「规则不允许」与「我们不接这单」
// 在事后分不开。
type ActiveRejection struct {
	authority RejectionAuthorityReference
	decider   DeciderReference
	reason    RejectionReasonReference
	evidence  RejectionEvidenceReference
}

func (rejection ActiveRejection) Authority() RejectionAuthorityReference {
	return rejection.authority
}

func (rejection ActiveRejection) Decider() DeciderReference {
	return rejection.decider
}

func (rejection ActiveRejection) Reason() RejectionReasonReference {
	return rejection.reason
}

func (rejection ActiveRejection) Evidence() RejectionEvidenceReference {
	return rejection.evidence
}

func (rejection ActiveRejection) formed() bool {
	return rejection.authority.valid() && rejection.decider.valid() &&
		rejection.reason.valid() && rejection.evidence.valid()
}

type ActiveRejectionSpec struct {
	DecisionID AcceptanceDecisionID
	Authority  RejectionAuthorityReference
	Decider    DeciderReference
	Reason     RejectionReasonReference
	Evidence   RejectionEvidenceReference
	// Basis 可选：主动拒绝不要求商业依据已经唯一解析，运营企业可以在依据尚未解析时就
	// 决定不接这单。解析到了就一并保存。
	Basis     CommercialBasisSnapshot
	DecidedAt time.Time
}

// RejectByAuthority 由授权角色对仍为`已提交`的委托形成主动拒绝。
//
// 它与自动接受竞争同一个决定提交边界，因此复用同一个决定槽位：先到的决定获胜，后到的只能
// 读取既有结果。分成两个槽位会让一份委托同时既被接受又被拒绝，而用例要求当前提交版本只有
// 一个合法决定。
//
// 它不判断这个角色够不够格：授权规则属 party-commercial，编排消费那边的授权结果后才到这里，
// 本方法保存所采用的授权引用。四项留痕全部必填——缺任一项，这次拒绝就与一句「不做了」分不开，
// 而 CONTEXT 明禁「普通备注、口头意见或未经授权的操作」形成拒绝事实。
//
// 拒绝不形成接受基线与预计承诺：没有承诺可作，凭空造一个会让下游按一份不存在的承诺办事。
func (request ShipmentRequest) RejectByAuthority(spec ActiveRejectionSpec) (ShipmentRequest, error) {
	if request.decisionFormed {
		return ShipmentRequest{}, ErrDecisionAlreadyFormed
	}
	if request.state != ShipmentRequestSubmitted {
		return ShipmentRequest{}, ErrInvalidShipmentRequest
	}
	if !spec.DecisionID.valid() || spec.DecidedAt.IsZero() {
		return ShipmentRequest{}, ErrInvalidAcceptanceDecision
	}

	rejection := ActiveRejection{
		authority: spec.Authority,
		decider:   spec.Decider,
		reason:    spec.Reason,
		evidence:  spec.Evidence,
	}
	if !rejection.formed() {
		return ShipmentRequest{}, ErrInvalidActiveRejection
	}

	request.state = ShipmentRequestRejected
	request.decision = AcceptanceDecision{
		decisionID:      spec.DecisionID,
		basis:           spec.Basis,
		decidedAt:       spec.DecidedAt,
		activeRejection: rejection,
	}
	request.decisionFormed = true
	request.acceptanceTask.state = AcceptanceTaskComplete
	return request, nil
}

// ActiveRejection 区分「规则形成的拒绝」与「运营企业主动不接」。零值即前者——规则拒绝没有
// 授权引用可带，也不该被读成有人拍过板。
func (decision AcceptanceDecision) ActiveRejection() (ActiveRejection, bool) {
	return decision.activeRejection, decision.activeRejection.formed()
}
