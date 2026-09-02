package domain

import (
	"errors"
	"time"
)

// 本文件是`面单继续尝试决定`登记册（ADR-0084 决定六留待的那一册）。
//
// **这里有两族东西，混成一族就全错了**：被追加的是`受控关闭决定`与`重开决定`，而
// `包裹级继续尝试判断`是从它们派生出来的值，封闭两格（`开放`／`受控关闭`）。CONTEXT 把判断
// 定义为「只由有效的关闭、重开决定及当前有效终局结果派生」——它没有自己的存储格。

var (
	ErrInvalidContinuedAttemptDecision = errors.New("parcel shipment: invalid continued attempt decision")
	// ErrContinuedAttemptDecisionNotAdmitted 说输入没问题，是这一册此刻不允许这一步
	// （ADR-0029 分格）：重开指不到一份仍然生效的关闭，或当前有效终局在场。续办动作与
	// 「改参数重试」完全不同——要去看这个包裹此刻停在哪一格。
	ErrContinuedAttemptDecisionNotAdmitted = errors.New("parcel shipment: continued attempt register does not admit this decision")
)

// ContinuedAttemptDecisionID 是一条决定的身份。
type ContinuedAttemptDecisionID struct{ requiredValue }

func NewContinuedAttemptDecisionID(value string) (ContinuedAttemptDecisionID, error) {
	required, err := newRequiredValue("continued attempt decision ID", value)
	return ContinuedAttemptDecisionID{required}, err
}

// AuthoritativeCutoffBoundary 是`权威业务截断边界`。
//
// **它不是时间戳。** CONTEXT 单列了它的定义并明写「客户端时间、消息到达时间、数据库写入时间
// 或外部墙钟均不能单独替代该边界」：生效时间说关闭从何时适用，截断边界裁决**并发的新尝试
// 是否合法**。做成一个时间字段就把这条定义删掉了，而删掉之后并发那一格只能靠比墙钟，那正是
// 它要挡住的做法。
type AuthoritativeCutoffBoundary struct{ requiredValue }

func NewAuthoritativeCutoffBoundary(value string) (AuthoritativeCutoffBoundary, error) {
	required, err := newRequiredValue("authoritative cutoff boundary", value)
	return AuthoritativeCutoffBoundary{required}, err
}

// ContinuedAttemptReasonReference 是结构化原因，不是自由文本。两种决定都必备。
type ContinuedAttemptReasonReference struct{ requiredValue }

func NewContinuedAttemptReasonReference(value string) (ContinuedAttemptReasonReference, error) {
	required, err := newRequiredValue("continued attempt reason reference", value)
	return ContinuedAttemptReasonReference{required}, err
}

// ClosureResponsibilitySourceReference 是`关闭责任来源`。它单独一格而不并进原因：CONTEXT 要求
// 重开时确认「原关闭责任来源对应或覆盖完整面单服务范围的限制已经解除」，那一步要指得出来源
// 本身，指不到就核不了。
type ClosureResponsibilitySourceReference struct{ requiredValue }

func NewClosureResponsibilitySourceReference(value string) (ClosureResponsibilitySourceReference, error) {
	required, err := newRequiredValue("closure responsibility source reference", value)
	return ClosureResponsibilitySourceReference{required}, err
}

// ContinuedAttemptAuthorityRoleReference 是形成本决定的授权角色。授权规则属 party-commercial，
// 本上下文只记所采用的那一个，不判断它够不够格。
type ContinuedAttemptAuthorityRoleReference struct{ requiredValue }

func NewContinuedAttemptAuthorityRoleReference(value string) (ContinuedAttemptAuthorityRoleReference, error) {
	required, err := newRequiredValue("continued attempt authority role reference", value)
	return ContinuedAttemptAuthorityRoleReference{required}, err
}

// ContinuedAttemptAuthoritySnapshot 是本决定所采用的授权依据快照。
type ContinuedAttemptAuthoritySnapshot struct{ requiredValue }

func NewContinuedAttemptAuthoritySnapshot(value string) (ContinuedAttemptAuthoritySnapshot, error) {
	required, err := newRequiredValue("continued attempt authority snapshot", value)
	return ContinuedAttemptAuthoritySnapshot{required}, err
}

// ContinuedAttemptDecisionKind 是被追加的决定的两格，取 CONTEXT 原词。
type ContinuedAttemptDecisionKind uint8

const (
	ContinuedAttemptDecisionKindInvalid ContinuedAttemptDecisionKind = iota
	ControlledClosureDecision
	ReopeningDecision
)

func (kind ContinuedAttemptDecisionKind) String() string {
	switch kind {
	case ControlledClosureDecision:
		return "CONTROLLED_CLOSURE"
	case ReopeningDecision:
		return "REOPENING"
	default:
		return ""
	}
}

func (kind ContinuedAttemptDecisionKind) valid() bool {
	return kind.String() != ""
}

// ContinuedAttemptJudgment 是`包裹级继续尝试判断`的封闭两格，取 CONTEXT 生命周期原词。
//
// **它没有第三格，也不该有。** 「没有人作过决定」与「最近适用决定为重开」在 CONTEXT 里派生出
// 的是同一格`开放`；要在页面上分辨两者，靠的是另外交代决定历史在不在，而不是给判断加一格——
// 加一格就是新造领域语言。
type ContinuedAttemptJudgment uint8

const (
	ContinuedAttemptJudgmentInvalid ContinuedAttemptJudgment = iota
	ContinuedAttemptOpen
	ContinuedAttemptControlledClosed
)

func (judgment ContinuedAttemptJudgment) String() string {
	switch judgment {
	case ContinuedAttemptOpen:
		return "OPEN"
	case ContinuedAttemptControlledClosed:
		return "CONTROLLED_CLOSED"
	default:
		return ""
	}
}

// ContinuedAttemptDecisionSpec 是追加一条决定的全部输入。
//
// 两种决定共用一个 spec 而由 Kind 分派校验，是因为它们追加进的是**同一条版本链**：拆成两个
// 类型，链上的顺序就得由调用方在外面维护，而顺序正是「最近适用决定是哪一条」的全部依据。
type ContinuedAttemptDecisionSpec struct {
	ID   ContinuedAttemptDecisionID
	Kind ContinuedAttemptDecisionKind
	// Requester 只有关闭这一格允许缺席（CONTEXT 原文「请求方（如有）」）——运营企业自行发起
	// 的关闭没有外部请求方。它与 Decider 分立是硬要求：「登录操作人可以作为操作证据，但不能
	// 替代实际决定方和授权角色」。
	Requester           RequesterReference
	Decider             DeciderReference
	AuthorityRole       ContinuedAttemptAuthorityRoleReference
	AuthoritySnapshot   ContinuedAttemptAuthoritySnapshot
	Reason              ContinuedAttemptReasonReference
	EffectiveAt         time.Time
	CutoffBoundary      AuthoritativeCutoffBoundary
	RelatedPriorClosure ContinuedAttemptDecisionID
}

// ContinuedAttemptDecision 是登记册上的一条决定。它只增不改：更正走版本链，不覆盖
// （CONTEXT「追加式、版本化」）。类型上没有任何改写方法。
type ContinuedAttemptDecision struct {
	id                  ContinuedAttemptDecisionID
	kind                ContinuedAttemptDecisionKind
	requester           RequesterReference
	decider             DeciderReference
	authorityRole       ContinuedAttemptAuthorityRoleReference
	authoritySnapshot   ContinuedAttemptAuthoritySnapshot
	reason              ContinuedAttemptReasonReference
	effectiveAt         time.Time
	cutoffBoundary      AuthoritativeCutoffBoundary
	relatedPriorClosure ContinuedAttemptDecisionID
}

func (decision ContinuedAttemptDecision) ID() ContinuedAttemptDecisionID {
	return decision.id
}

func (decision ContinuedAttemptDecision) Kind() ContinuedAttemptDecisionKind {
	return decision.kind
}

func (decision ContinuedAttemptDecision) Requester() RequesterReference {
	return decision.requester
}

func (decision ContinuedAttemptDecision) Decider() DeciderReference {
	return decision.decider
}

func (decision ContinuedAttemptDecision) AuthorityRole() ContinuedAttemptAuthorityRoleReference {
	return decision.authorityRole
}

func (decision ContinuedAttemptDecision) AuthoritySnapshot() ContinuedAttemptAuthoritySnapshot {
	return decision.authoritySnapshot
}

func (decision ContinuedAttemptDecision) Reason() ContinuedAttemptReasonReference {
	return decision.reason
}

func (decision ContinuedAttemptDecision) EffectiveAt() time.Time {
	return decision.effectiveAt
}

// CutoffBoundary 只有关闭这一格有值：截断边界是关闭生效时形成的，重开「只允许未来形成新交易」
// 而不裁决任何并发尝试的合法性。
func (decision ContinuedAttemptDecision) CutoffBoundary() AuthoritativeCutoffBoundary {
	return decision.cutoffBoundary
}

// RelatedPriorClosure 只有重开这一格有值：CONTEXT 要求重开「关联此前关闭」。
func (decision ContinuedAttemptDecision) RelatedPriorClosure() ContinuedAttemptDecisionID {
	return decision.relatedPriorClosure
}
