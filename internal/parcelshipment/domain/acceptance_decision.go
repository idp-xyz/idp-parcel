package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidAcceptanceCheck    = errors.New("parcel shipment: invalid acceptance check")
	ErrInvalidAcceptanceDecision = errors.New("parcel shipment: invalid acceptance decision")
	ErrDecisionAlreadyFormed     = errors.New("parcel shipment: a decision already exists for this submission version")
)

type AcceptanceDecisionID struct{ requiredValue }

func NewAcceptanceDecisionID(value string) (AcceptanceDecisionID, error) {
	required, err := newRequiredValue("acceptance decision ID", value)
	return AcceptanceDecisionID{required}, err
}

// CheckReason 是某个校验组未通过或无法判定的结构化原因。它是引用而非自由文本，这样
// 拒绝才能按原因维度统计。
type CheckReason struct{ requiredValue }

func NewCheckReason(value string) (CheckReason, error) {
	required, err := newRequiredValue("check reason", value)
	return CheckReason{required}, err
}

// AcceptanceCheckGroup 是 `UC-PS-001` 声明的校验组封闭集合。作用其上的规则是统一的
// ——每个适用校验组都必须通过——所以这里放齐全集，尽管目前只有部分组有生产者。
type AcceptanceCheckGroup uint8

const (
	AcceptanceCheckGroupInvalid AcceptanceCheckGroup = iota
	CustomerRelationshipCheck
	LegalEntityAndContractCheck
	ProductAndServiceCheck
	MemberBaselineCheck
	RequiredDocumentCheck
	PreAcceptanceFinancialControlCheck
	NetworkReachabilityCheck
)

func (group AcceptanceCheckGroup) valid() bool {
	return group >= CustomerRelationshipCheck && group <= NetworkReachabilityCheck
}

func (group AcceptanceCheckGroup) String() string {
	switch group {
	case CustomerRelationshipCheck:
		return "CUSTOMER_RELATIONSHIP"
	case LegalEntityAndContractCheck:
		return "LEGAL_ENTITY_AND_CONTRACT"
	case ProductAndServiceCheck:
		return "PRODUCT_AND_SERVICE"
	case MemberBaselineCheck:
		return "MEMBER_BASELINE"
	case RequiredDocumentCheck:
		return "REQUIRED_DOCUMENT"
	case PreAcceptanceFinancialControlCheck:
		return "PRE_ACCEPTANCE_FINANCIAL_CONTROL"
	case NetworkReachabilityCheck:
		return "NETWORK_REACHABILITY"
	default:
		return ""
	}
}

type CheckOutcome uint8

const (
	CheckOutcomeInvalid CheckOutcome = iota
	CheckPassed
	CheckFailed
	CheckUndetermined
)

func (outcome CheckOutcome) valid() bool {
	return outcome >= CheckPassed && outcome <= CheckUndetermined
}

func (outcome CheckOutcome) String() string {
	switch outcome {
	case CheckPassed:
		return "PASSED"
	case CheckFailed:
		return "FAILED"
	case CheckUndetermined:
		return "UNDETERMINED"
	default:
		return ""
	}
}

// AcceptanceCheck 是一个校验组的结果。包裹标识为零值表示该校验作用于整份提交版本，
// 有值则表示只针对该成员。除通过外的结果都必须携带原因。
type AcceptanceCheck struct {
	group    AcceptanceCheckGroup
	parcelID DeclaredParcelID
	outcome  CheckOutcome
	reason   CheckReason
}

func NewAcceptanceCheck(
	group AcceptanceCheckGroup,
	parcelID DeclaredParcelID,
	outcome CheckOutcome,
	reason CheckReason,
) (AcceptanceCheck, error) {
	if !group.valid() || !outcome.valid() {
		return AcceptanceCheck{}, ErrInvalidAcceptanceCheck
	}
	if outcome != CheckPassed && !reason.valid() {
		return AcceptanceCheck{}, ErrInvalidAcceptanceCheck
	}
	return AcceptanceCheck{group: group, parcelID: parcelID, outcome: outcome, reason: reason}, nil
}

func (check AcceptanceCheck) Group() AcceptanceCheckGroup {
	return check.group
}

func (check AcceptanceCheck) DeclaredParcelID() DeclaredParcelID {
	return check.parcelID
}

func (check AcceptanceCheck) Outcome() CheckOutcome {
	return check.outcome
}

func (check AcceptanceCheck) Reason() CheckReason {
	return check.reason
}

// ManualReviewState 记录所采用的接单规则包是否要求人工复核。复核是接受的一道门，但
// 绝不替代硬规则：已完成的复核不能把一个确定性失败变成接受。
type ManualReviewState uint8

const (
	ManualReviewStateInvalid ManualReviewState = iota
	ManualReviewNotRequired
	ManualReviewRequired
	ManualReviewCompleted
)

func (state ManualReviewState) valid() bool {
	return state >= ManualReviewNotRequired && state <= ManualReviewCompleted
}

// AcceptanceBaseline 是接受时固定下来的不可覆盖成员集合，恒覆盖该提交版本的完整声明
// 成员：本产品不支持成员级部分接受，因此一份只覆盖部分成员的基线只可能意味着有条规则
// 被跳过了。
type AcceptanceBaseline struct {
	declaredParcelIDs []DeclaredParcelID
	submissionVersion SubmissionVersionID
	fixedAt           time.Time
}

func (baseline AcceptanceBaseline) DeclaredParcelIDs() []DeclaredParcelID {
	return append([]DeclaredParcelID(nil), baseline.declaredParcelIDs...)
}

func (baseline AcceptanceBaseline) SubmissionVersionID() SubmissionVersionID {
	return baseline.submissionVersion
}

func (baseline AcceptanceBaseline) FixedAt() time.Time {
	return baseline.fixedAt
}

// ExpectedCommitment 是运营企业在接受时作出的承诺。它保存的是当时生效的商业依据快照
// 而非一个算出来的日期：承诺的数值活在依据指向的产品与合同版本里，因此日后那些版本
// 变化也改写不了当时承诺过什么。
type ExpectedCommitment struct {
	basis    CommercialBasisSnapshot
	formedAt time.Time
}

func (commitment ExpectedCommitment) Basis() CommercialBasisSnapshot {
	return commitment.basis
}

func (commitment ExpectedCommitment) FormedAt() time.Time {
	return commitment.formedAt
}

type AcceptanceDecision struct {
	decisionID   AcceptanceDecisionID
	accepted     bool
	checks       []AcceptanceCheck
	basis        CommercialBasisSnapshot
	manualReview ManualReviewState
	decidedAt    time.Time
}

func (decision AcceptanceDecision) DecisionID() AcceptanceDecisionID {
	return decision.decisionID
}

func (decision AcceptanceDecision) Accepted() bool {
	return decision.accepted
}

func (decision AcceptanceDecision) Checks() []AcceptanceCheck {
	return append([]AcceptanceCheck(nil), decision.checks...)
}

func (decision AcceptanceDecision) FailedChecks() []AcceptanceCheck {
	failed := make([]AcceptanceCheck, 0, len(decision.checks))
	for _, check := range decision.checks {
		if check.outcome == CheckFailed {
			failed = append(failed, check)
		}
	}
	return failed
}

func (decision AcceptanceDecision) Basis() CommercialBasisSnapshot {
	return decision.basis
}

func (decision AcceptanceDecision) DecidedAt() time.Time {
	return decision.decidedAt
}

type AcceptanceDecisionSpec struct {
	DecisionID   AcceptanceDecisionID
	Checks       []AcceptanceCheck
	ManualReview ManualReviewState
	Basis        CommercialBasisSnapshot
	DecidedAt    time.Time
}

// Decide 为当前提交版本至多形成一个接受或拒绝。三种结果里只有两种是决定：无法判定的
// 一轮让委托保持`已提交`、接受判断任务保持未完成，因为`尚未决定`是应用处理结果而不是
// 生命周期结果。
//
// 任何确定性失败——无论落在整份版本还是单个成员——都拒绝整份版本。本产品不支持成员级
// 部分接受，所以这里没有任何一条「留下合格成员、丢掉失败那个」的路径。
func (request ShipmentRequest) Decide(spec AcceptanceDecisionSpec) (ShipmentRequest, error) {
	// 已决定的情形先判，这样对已接受或已拒绝版本的第二次尝试会说出真实原因，而不是
	// 返回一个读起来像输入格式错误的泛化「无效委托」。
	if request.decisionFormed {
		return ShipmentRequest{}, ErrDecisionAlreadyFormed
	}
	if request.state != ShipmentRequestSubmitted {
		return ShipmentRequest{}, ErrInvalidShipmentRequest
	}
	if !spec.DecisionID.valid() || !spec.ManualReview.valid() || !spec.Basis.valid() || spec.DecidedAt.IsZero() {
		return ShipmentRequest{}, ErrInvalidAcceptanceDecision
	}

	failed, undetermined := 0, 0
	judged := make(map[DeclaredParcelID]struct{}, len(request.currentVersion.declaredParcelIDs))
	for _, check := range spec.Checks {
		if !check.group.valid() || !check.outcome.valid() {
			return ShipmentRequest{}, ErrInvalidAcceptanceCheck
		}
		switch check.outcome {
		case CheckFailed:
			failed++
		case CheckUndetermined:
			undetermined++
		}
		if check.parcelID.valid() && check.outcome == CheckPassed {
			judged[check.parcelID] = struct{}{}
		}
	}

	decision := AcceptanceDecision{
		decisionID:   spec.DecisionID,
		checks:       append([]AcceptanceCheck(nil), spec.Checks...),
		basis:        spec.Basis,
		manualReview: spec.ManualReview,
		decidedAt:    spec.DecidedAt,
	}

	if failed > 0 {
		request.state = ShipmentRequestRejected
		request.decision = decision
		request.decisionFormed = true
		request.acceptanceTask.complete = true
		return request, nil
	}
	if undetermined > 0 ||
		spec.ManualReview == ManualReviewRequired ||
		!request.everyMemberJudged(judged) {
		return request, nil
	}

	decision.accepted = true
	request.state = ShipmentRequestAccepted
	request.decision = decision
	request.decisionFormed = true
	request.acceptanceTask.complete = true
	request.baseline = AcceptanceBaseline{
		declaredParcelIDs: request.currentVersion.DeclaredParcelIDs(),
		submissionVersion: request.currentVersion.versionID,
		fixedAt:           spec.DecidedAt,
	}
	request.commitment = ExpectedCommitment{basis: spec.Basis, formedAt: spec.DecidedAt}
	return request, nil
}

// everyMemberJudged 挡住「某个声明成员从未被判断过却接受了整份版本」。未被判断的成员
// 不等于通过的成员，当作通过就是以遗漏方式实现的成员级接受。
func (request ShipmentRequest) everyMemberJudged(judged map[DeclaredParcelID]struct{}) bool {
	for _, parcelID := range request.currentVersion.declaredParcelIDs {
		if _, present := judged[parcelID]; !present {
			return false
		}
	}
	return true
}

func (request ShipmentRequest) AcceptanceDecision() (AcceptanceDecision, bool) {
	return request.decision, request.decisionFormed
}

func (request ShipmentRequest) AcceptanceBaseline() (AcceptanceBaseline, bool) {
	if len(request.baseline.declaredParcelIDs) == 0 {
		return AcceptanceBaseline{}, false
	}
	return request.baseline, true
}

func (request ShipmentRequest) ExpectedCommitment() (ExpectedCommitment, bool) {
	if request.commitment.formedAt.IsZero() {
		return ExpectedCommitment{}, false
	}
	return request.commitment, true
}
