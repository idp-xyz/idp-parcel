package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidExternalResult = errors.New("customs compliance: invalid external result")
	ErrLayerConflict         = errors.New("customs compliance: conflicting facts on the same layer")
)

// ResultLayer 是外部监管结果的封闭六层（CONTEXT 硬句 185：「提交尝试、技术回执、
// 监管接收、业务受理、监管过程决定、监管核定税费、放行结果和监管处置决定必须分层
// 保存。任何前一层成功都不能自动生成后一层结果，也不能使用一个『清关成功』状态覆盖
// 各层事实」——提交尝试与技术回执在提交对象上，这里是监管侧六层）。类型上没有任何
// 从一层派生另一层的方法：不虚构缺失结果是结构性的。
type ResultLayer uint8

const (
	ResultLayerInvalid ResultLayer = iota
	RegulatoryReceiptLayer
	BusinessAcceptanceLayer
	ProcessDecisionLayer
	AssessedDutyLayer
	ReleaseResultLayer
	DispositionDecisionLayer
)

func (layer ResultLayer) valid() bool {
	return layer >= RegulatoryReceiptLayer && layer <= DispositionDecisionLayer
}

func (layer ResultLayer) String() string {
	switch layer {
	case RegulatoryReceiptLayer:
		return "REGULATORY_RECEIPT"
	case BusinessAcceptanceLayer:
		return "BUSINESS_ACCEPTANCE"
	case ProcessDecisionLayer:
		return "PROCESS_DECISION"
	case AssessedDutyLayer:
		return "ASSESSED_DUTY"
	case ReleaseResultLayer:
		return "RELEASE_RESULT"
	case DispositionDecisionLayer:
		return "DISPOSITION_DECISION"
	default:
		return ""
	}
}

// SourceAuthorityRole 指名来源在该结果层的权威角色。技术中介或报关服务商的响应只有
// 在来源语义明确代表监管机构时才能形成监管事实（186）——角色引用正是那个判断的落点。
type SourceAuthorityRole struct{ requiredValue }

func NewSourceAuthorityRole(value string) (SourceAuthorityRole, error) {
	required, err := newRequiredValue("source authority role", value)
	return SourceAuthorityRole{required}, err
}

// InterpretationRuleReference 指名实际采用的解释规则。
type InterpretationRuleReference struct{ requiredValue }

func NewInterpretationRuleReference(value string) (InterpretationRuleReference, error) {
	required, err := newRequiredValue("interpretation rule reference", value)
	return InterpretationRuleReference{required}, err
}

// ExternalResultSpec 是保存一项外部结果所需的全部输入（CONTEXT 硬句 186 八件：来源
// 身份、权威角色、原始语义、业务发生或适用时间、接收时间、解释规则、与提交版本/
// 尝试和明确结果范围的关系）。
type ExternalResultSpec struct {
	Layer        ResultLayer
	SourceID     string
	Role         SourceAuthorityRole
	RawSemantics string
	Rule         InterpretationRuleReference
	Version      SubmissionVersionID
	Attempt      int
	Scope        DecisionScopeReference
	OccurredAt   time.Time
	ReceivedAt   time.Time
}

// ExternalResult 是已接收并解释的一项外部监管事实。八件缺一立不起；查询请求、查询
// 执行状态与查询技术结果改变不了原提交的业务判断（188）——那些不是外部结果，构造
// 不出这个类型。
type ExternalResult struct {
	layer        ResultLayer
	sourceID     string
	role         SourceAuthorityRole
	rawSemantics string
	rule         InterpretationRuleReference
	version      SubmissionVersionID
	attempt      int
	scope        DecisionScopeReference
	occurredAt   time.Time
	receivedAt   time.Time
}

func InterpretExternalResult(spec ExternalResultSpec) (ExternalResult, error) {
	if !spec.Layer.valid() ||
		spec.SourceID == "" ||
		!spec.Role.valid() ||
		spec.RawSemantics == "" ||
		!spec.Rule.valid() ||
		!spec.Version.valid() ||
		spec.Attempt <= 0 ||
		!spec.Scope.valid() ||
		spec.OccurredAt.IsZero() ||
		spec.ReceivedAt.IsZero() {
		return ExternalResult{}, ErrInvalidExternalResult
	}
	return ExternalResult{
		layer:        spec.Layer,
		sourceID:     spec.SourceID,
		role:         spec.Role,
		rawSemantics: spec.RawSemantics,
		rule:         spec.Rule,
		version:      spec.Version,
		attempt:      spec.Attempt,
		scope:        spec.Scope,
		occurredAt:   spec.OccurredAt.UTC(),
		receivedAt:   spec.ReceivedAt.UTC(),
	}, nil
}

func (result ExternalResult) Layer() ResultLayer {
	return result.layer
}

// SourceID、Rule 与 Attempt 是持久化重建的必需读口——八件里这三件没有出口，登记册
// 适配器连原样写回都做不到。
func (result ExternalResult) SourceID() string {
	return result.sourceID
}

func (result ExternalResult) Rule() InterpretationRuleReference {
	return result.rule
}

func (result ExternalResult) Attempt() int {
	return result.attempt
}

func (result ExternalResult) Role() SourceAuthorityRole {
	return result.role
}

func (result ExternalResult) RawSemantics() string {
	return result.rawSemantics
}

func (result ExternalResult) Version() SubmissionVersionID {
	return result.version
}

func (result ExternalResult) Scope() DecisionScopeReference {
	return result.scope
}

func (result ExternalResult) OccurredAt() time.Time {
	return result.occurredAt
}

func (result ExternalResult) ReceivedAt() time.Time {
	return result.receivedAt
}

// CheckLayerConsistency 把新到结果与同层既有事实逐一比对（CONTEXT 硬句 187：「与
// 同层现有事实冲突时，不得据此猜测提交、补造缺失层次或按最后到达直接改变当前判断」）：
// 同层、同提交版本、同范围而原始语义不同即冲突——独立哨兵，调用方保留双方事实形成
// 冲突关系，不选边。不同层或不同范围的事实各归各位，不构成冲突。
func CheckLayerConsistency(existing []ExternalResult, incoming ExternalResult) error {
	if !incoming.layer.valid() {
		return ErrInvalidExternalResult
	}
	for _, result := range existing {
		if result.layer == incoming.layer &&
			result.version == incoming.version &&
			result.scope == incoming.scope &&
			result.rawSemantics != incoming.rawSemantics {
			return ErrLayerConflict
		}
	}
	return nil
}
