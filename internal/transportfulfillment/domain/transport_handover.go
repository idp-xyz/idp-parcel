package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidTransportHandover = errors.New("transport fulfillment: invalid transport handover")
	// ErrMixedHandoverScopes：整批/整车/整袋结论只能由**同一交接范围**的对象级结果派生；
	// 混入另一范围（或另一租户）的结果，汇总答的就不再是这一次交接。
	ErrMixedHandoverScopes = errors.New("transport fulfillment: handover results span different scopes")
)

// HandoverVerdict 是权威运输交接对单个载运对象的封闭三值裁决（CONTEXT 交接节：
// 「按载运对象形成待确认、已拒收或已交接判断」）。
type HandoverVerdict uint8

const (
	HandoverVerdictInvalid HandoverVerdict = iota
	ObjectHandedOver
	HandoverRefused
	HandoverPendingConfirmation
)

func (verdict HandoverVerdict) valid() bool {
	return verdict >= ObjectHandedOver && verdict <= HandoverPendingConfirmation
}

func (verdict HandoverVerdict) String() string {
	switch verdict {
	case ObjectHandedOver:
		return "HANDED_OVER"
	case HandoverRefused:
		return "REFUSED"
	case HandoverPendingConfirmation:
		return "PENDING_CONFIRMATION"
	default:
		return ""
	}
}

// HandoverScopeReference 指名一次交接的共同范围（一车、一批、一次交出）。双方证据都
// 引用它，但范围本身不承载结论——结论逐对象成立。
type HandoverScopeReference struct{ requiredValue }

func NewHandoverScopeReference(value string) (HandoverScopeReference, error) {
	required, err := newRequiredValue("handover scope reference", value)
	return HandoverScopeReference{required}, err
}

// HandoverPartyReference 指名交出方或接收方。
type HandoverPartyReference struct{ requiredValue }

func NewHandoverPartyReference(value string) (HandoverPartyReference, error) {
	required, err := newRequiredValue("handover party reference", value)
	return HandoverPartyReference{required}, err
}

// HandoverEvidenceReference 指名一侧的交接证据（节点交出、运输方接收、封签、点验……）。
type HandoverEvidenceReference struct{ requiredValue }

func NewHandoverEvidenceReference(value string) (HandoverEvidenceReference, error) {
	required, err := newRequiredValue("handover evidence reference", value)
	return HandoverEvidenceReference{required}, err
}

// HandoverRuleReference 指名裁决本次交接所适用的规则版本。
type HandoverRuleReference struct{ requiredValue }

func NewHandoverRuleReference(value string) (HandoverRuleReference, error) {
	required, err := newRequiredValue("handover rule reference", value)
	return HandoverRuleReference{required}, err
}

// HandoverBasisReference 指名拒收原因或待确认缺口/冲突。已交接没有它——那一格的依据
// 是双方证据加适用规则。
type HandoverBasisReference struct{ requiredValue }

func NewHandoverBasisReference(value string) (HandoverBasisReference, error) {
	required, err := newRequiredValue("handover basis reference", value)
	return HandoverBasisReference{required}, err
}

// HandoverResultVersion 是交接判断的版本标识：更正形成新版本使原结果失效或被替代，
// 不删除原交接（CONTEXT 交接节）。
type HandoverResultVersion struct{ requiredValue }

func NewHandoverResultVersion(value string) (HandoverResultVersion, error) {
	required, err := newRequiredValue("handover result version", value)
	return HandoverResultVersion{required}, err
}

// TransportHandoverSpec 是形成一个对象级交接判断所需的全部输入。
type TransportHandoverSpec struct {
	TenantID          TenantID
	Object            CarriedObjectReference
	Scope             HandoverScopeReference
	ReleasedBy        HandoverPartyReference
	ReceivedBy        HandoverPartyReference
	Verdict           HandoverVerdict
	ReleasingEvidence HandoverEvidenceReference
	ReceivingEvidence HandoverEvidenceReference
	Rule              HandoverRuleReference
	Basis             HandoverBasisReference
	Version           HandoverResultVersion
	JudgedAt          time.Time
}

// TransportHandover 是针对明确载运对象、依据双方证据与适用规则形成的唯一权威控制转移
// 判断（CONTEXT「权威运输交接结果」；本上下文独占所有权，node-operations 只引用）。
//
// 双方不共同编辑一个结果：两侧证据作为不可变字段在构造期一次进入，结果是值——没有任何
// 入口让某一方事后改写另一方看到的结论。
type TransportHandover struct {
	tenantID          TenantID
	object            CarriedObjectReference
	scope             HandoverScopeReference
	releasedBy        HandoverPartyReference
	receivedBy        HandoverPartyReference
	verdict           HandoverVerdict
	releasingEvidence HandoverEvidenceReference
	receivingEvidence HandoverEvidenceReference
	rule              HandoverRuleReference
	basis             HandoverBasisReference
	version           HandoverResultVersion
	judgedAt          time.Time
	corrects          HandoverResultVersion
	correctedAt       time.Time
}

// FormTransportHandover 逐格校验三值各自的完备性：
//   - `已交接`必须同时有双方证据与适用规则（UC-TF-005：必须保存双方证据、业务时间、
//     结果版本），且不携带拒收/待确认依据；
//   - `已拒收`与`待确认`必须携带依据（拒收原因、缺口或冲突引用）——没有原因的拒收与
//     数据丢失无从分辨；证据允许只有一侧（另一侧可能正是缺的那份）。
func FormTransportHandover(spec TransportHandoverSpec) (TransportHandover, error) {
	if !spec.TenantID.valid() ||
		!spec.Object.valid() ||
		!spec.Scope.valid() ||
		!spec.ReleasedBy.valid() ||
		!spec.ReceivedBy.valid() ||
		!spec.Verdict.valid() ||
		!spec.Version.valid() ||
		spec.JudgedAt.IsZero() {
		return TransportHandover{}, ErrInvalidTransportHandover
	}
	if spec.Verdict == ObjectHandedOver {
		if !spec.ReleasingEvidence.valid() || !spec.ReceivingEvidence.valid() || !spec.Rule.valid() {
			return TransportHandover{}, ErrInvalidTransportHandover
		}
		if spec.Basis.valid() {
			return TransportHandover{}, ErrInvalidTransportHandover
		}
	} else if !spec.Basis.valid() {
		return TransportHandover{}, ErrInvalidTransportHandover
	}
	return TransportHandover{
		tenantID:          spec.TenantID,
		object:            spec.Object,
		scope:             spec.Scope,
		releasedBy:        spec.ReleasedBy,
		receivedBy:        spec.ReceivedBy,
		verdict:           spec.Verdict,
		releasingEvidence: spec.ReleasingEvidence,
		receivingEvidence: spec.ReceivingEvidence,
		rule:              spec.Rule,
		basis:             spec.Basis,
		version:           spec.Version,
		judgedAt:          spec.JudgedAt.UTC(),
	}, nil
}

func (handover TransportHandover) TenantID() TenantID {
	return handover.tenantID
}

func (handover TransportHandover) Object() CarriedObjectReference {
	return handover.object
}

func (handover TransportHandover) Scope() HandoverScopeReference {
	return handover.scope
}

func (handover TransportHandover) ReleasedBy() HandoverPartyReference {
	return handover.releasedBy
}

func (handover TransportHandover) ReceivedBy() HandoverPartyReference {
	return handover.receivedBy
}

func (handover TransportHandover) Verdict() HandoverVerdict {
	return handover.verdict
}

// ReleasingEvidence 与 ReceivingEvidence 各自可缺席（拒收/待确认可能正缺某一侧）。
func (handover TransportHandover) ReleasingEvidence() (HandoverEvidenceReference, bool) {
	if !handover.releasingEvidence.valid() {
		return HandoverEvidenceReference{}, false
	}
	return handover.releasingEvidence, true
}

func (handover TransportHandover) ReceivingEvidence() (HandoverEvidenceReference, bool) {
	if !handover.receivingEvidence.valid() {
		return HandoverEvidenceReference{}, false
	}
	return handover.receivingEvidence, true
}

func (handover TransportHandover) Rule() (HandoverRuleReference, bool) {
	if !handover.rule.valid() {
		return HandoverRuleReference{}, false
	}
	return handover.rule, true
}

// Basis 在拒收与待确认时交回原因/缺口依据；已交接没有它。
func (handover TransportHandover) Basis() (HandoverBasisReference, bool) {
	if !handover.basis.valid() {
		return HandoverBasisReference{}, false
	}
	return handover.basis, true
}

func (handover TransportHandover) Version() HandoverResultVersion {
	return handover.version
}

func (handover TransportHandover) JudgedAt() time.Time {
	return handover.judgedAt
}

// Corrects 交回本版本更正的前一版本（若本版本由更正产生）。
func (handover TransportHandover) Corrects() (HandoverResultVersion, bool) {
	if !handover.corrects.valid() {
		return HandoverResultVersion{}, false
	}
	return handover.corrects, true
}

func (handover TransportHandover) CorrectedAt() (time.Time, bool) {
	if handover.correctedAt.IsZero() {
		return time.Time{}, false
	}
	return handover.correctedAt, true
}

// HandoverCorrection 携带一次更正给出的新裁决与新证据。对象、范围、双方与业务时间不在
// 其中——更正改的是判断，不是那次交接发生过什么。
type HandoverCorrection struct {
	Verdict           HandoverVerdict
	ReleasingEvidence HandoverEvidenceReference
	ReceivingEvidence HandoverEvidenceReference
	Rule              HandoverRuleReference
	Basis             HandoverBasisReference
	Version           HandoverResultVersion
	CorrectedAt       time.Time
}

// Correct 依据更正证据形成新判断版本：保留原事实和原判断（值语义，接收者不动），新版本
// 回指被更正版本（CONTEXT「来源证据被更正时，保留原事实和原判断，形成失效、替代及重新
// 派生结果」，AT-TF-062）。沿用原版本号就是覆盖，构造期拒绝；每格的完备性要求与首次
// 裁决相同——更正成`已交接`同样要双方证据加规则。
//
// `已交接`被更正为拒收/待确认时，新版本自然给不出转出引用；node-operations 按原版本
// 已经转出的控制不可逆，那是它拥有的事实——转出依据失效后的控制来源链重算由后续处置
// 对象表达（AT-TF-062「允许当前实物方明确但来源冲突，不简单回退早期控制」），本类型
// 只保证版本链可追与原判断不被改写。
func (handover TransportHandover) Correct(correction HandoverCorrection) (TransportHandover, error) {
	if !handover.version.valid() {
		return TransportHandover{}, ErrInvalidTransportHandover
	}
	if !correction.Version.valid() || correction.CorrectedAt.IsZero() {
		return TransportHandover{}, ErrInvalidTransportHandover
	}
	if correction.Version == handover.version {
		return TransportHandover{}, ErrInvalidTransportHandover
	}
	if correction.CorrectedAt.Before(handover.judgedAt) {
		return TransportHandover{}, ErrInvalidTransportHandover
	}
	corrected, err := FormTransportHandover(TransportHandoverSpec{
		TenantID:          handover.tenantID,
		Object:            handover.object,
		Scope:             handover.scope,
		ReleasedBy:        handover.releasedBy,
		ReceivedBy:        handover.receivedBy,
		Verdict:           correction.Verdict,
		ReleasingEvidence: correction.ReleasingEvidence,
		ReceivingEvidence: correction.ReceivingEvidence,
		Rule:              correction.Rule,
		Basis:             correction.Basis,
		Version:           correction.Version,
		JudgedAt:          handover.judgedAt,
	})
	if err != nil {
		return TransportHandover{}, err
	}
	corrected.corrects = handover.version
	corrected.correctedAt = correction.CorrectedAt.UTC()
	return corrected, nil
}

// TransfersControl 只在`已交接`时为真：已拒收或待确认不转出控制，此前控制方在结果
// 成立前继续保持控制（CONTEXT 交接节）。
func (handover TransportHandover) TransfersControl() bool {
	return handover.verdict == ObjectHandedOver
}

// TransferOutBasis 交回可供 node-operations 包装成 TransferOutReference 的稳定引用——
// 只有`已交接`给得出。NO 侧注释写明「那些结果在 TF 的权威交接对象上就不是`已交接`，
// 从源头就构造不出这个引用」，这里就是那个源头。
func (handover TransportHandover) TransferOutBasis() (string, bool) {
	if !handover.TransfersControl() {
		return "", false
	}
	return "TRANSPORT-HANDOVER/" + handover.version.String(), true
}

// HandoverScopeSummary 是同一交接范围内对象级结果的派生汇总。它只有计数与派生问答，
// 没有自己的裁决字段——整批/整车/整袋结论只能由对象级结果派生（CONTEXT 交接硬句），
// 想给批次一个独立结论在类型上就无处可写。
type HandoverScopeSummary struct {
	scope       HandoverScopeReference
	handedOver  int
	refused     int
	unconfirmed int
}

// SummarizeHandovers 派生一个交接范围的汇总。空集不成立汇总；跨范围或跨租户混入即
// 拒绝——那不是「这一次交接」的成员。
func SummarizeHandovers(results []TransportHandover) (HandoverScopeSummary, error) {
	if len(results) == 0 {
		return HandoverScopeSummary{}, ErrInvalidTransportHandover
	}
	first := results[0]
	summary := HandoverScopeSummary{scope: first.scope}
	for _, result := range results {
		if result.scope != first.scope || result.tenantID != first.tenantID {
			return HandoverScopeSummary{}, ErrMixedHandoverScopes
		}
		switch result.verdict {
		case ObjectHandedOver:
			summary.handedOver++
		case HandoverRefused:
			summary.refused++
		case HandoverPendingConfirmation:
			summary.unconfirmed++
		default:
			return HandoverScopeSummary{}, ErrInvalidTransportHandover
		}
	}
	return summary, nil
}

func (summary HandoverScopeSummary) Scope() HandoverScopeReference {
	return summary.scope
}

func (summary HandoverScopeSummary) HandedOver() int {
	return summary.handedOver
}

func (summary HandoverScopeSummary) Refused() int {
	return summary.refused
}

func (summary HandoverScopeSummary) Unconfirmed() int {
	return summary.unconfirmed
}

func (summary HandoverScopeSummary) Total() int {
	return summary.handedOver + summary.refused + summary.unconfirmed
}

// AllHandedOver 是唯一的「整批成功」问法，且它只是成员计数的派生——部分接收时它为假，
// 已接收对象照常进入履约，拒收对象继续由原控制方保管（AT-TF-052）。
func (summary HandoverScopeSummary) AllHandedOver() bool {
	return summary.refused == 0 && summary.unconfirmed == 0 && summary.handedOver > 0
}
