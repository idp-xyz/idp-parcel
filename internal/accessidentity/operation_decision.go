package accessidentity

import (
	"errors"
	"strings"
	"time"
)

// ErrDecisionKindUnknown 表示给出的决定种类不在 ADR-0151 决定一列出的那几种里。
var ErrDecisionKindUnknown = errors.New("access identity: decision kind is unknown")

// ErrDecisionKindRequired 表示「运营决定」这一格没带决定种类：它的授予按租户 × 决定种类登记
// （ADR-0151 决定一），不带种类的一笔等于把所有决定一并授出。经 NewOperationDecisionGrant 建。
var ErrDecisionKindRequired = errors.New("access identity: operation decision grant needs a decision kind")

// DecisionKind 是「运营决定」能力面下的决定种类（ADR-0151 决定一）。
//
// 种类是册的列，由产品的角色模型定；某租户把哪位操作者授予哪一种决定，才是租户取值。待识别重核是复核页上对指名的一件待识别
// 实物按存下的号再问一次 PS（ADR-0199 决定三），管理台上作判断的口照决定一归这一面。
// 身份冲突处置是复核页上对一件判身份冲突的作业实物指定一件候选或确认都不是（ADR-0210 决定一）；授予就是它全部的授权门。
// 运输委托提交是运营人员向外部服务提供方
// 提出明确的运输范围与条件（ADR-0202 决定一），委托、订舱、取消各口每口一种，授予的粒度等于权力的粒度。
type DecisionKind string

const (
	DecisionManualReviewCompletion              DecisionKind = "MANUAL_REVIEW_COMPLETION"
	DecisionActiveRejection                     DecisionKind = "ACTIVE_REJECTION"
	DecisionAuthorizedDisposition               DecisionKind = "AUTHORIZED_DISPOSITION"
	DecisionControlledClosure                   DecisionKind = "CONTROLLED_CLOSURE"
	DecisionControlledReopening                 DecisionKind = "CONTROLLED_REOPENING"
	DecisionSegmentClosure                      DecisionKind = "SEGMENT_CLOSURE"
	DecisionDispatchTaskRegistration            DecisionKind = "DISPATCH_TASK_REGISTRATION"
	DecisionLoadAssignment                      DecisionKind = "LOAD_ASSIGNMENT"
	DecisionParticipationTermination            DecisionKind = "PARTICIPATION_TERMINATION"
	DecisionEffectiveTimeJudgment               DecisionKind = "EFFECTIVE_TIME_JUDGMENT"
	DecisionCarrierFirstEffectivePickupJudgment DecisionKind = "CARRIER_FIRST_EFFECTIVE_PICKUP_JUDGMENT"
	DecisionPendingIdentityRecheck              DecisionKind = "PENDING_IDENTITY_RECHECK"
	DecisionIdentityConflictDisposition         DecisionKind = "IDENTITY_CONFLICT_DISPOSITION"
	DecisionTransportCommissionSubmission       DecisionKind = "TRANSPORT_COMMISSION_SUBMISSION"
)

// DecisionTransportChargeOccurrenceRegistration 是运输收费发生项登记（ADR-0207 决定六）：对已发生的履约事实作一次
// 采购判断——断言采用协议的发生条件已满足、指名服务方与协议版本。它不是现场作业事实，所以归运营决定而不归作业事实面。
const DecisionTransportChargeOccurrenceRegistration DecisionKind = "TRANSPORT_CHARGE_OCCURRENCE_REGISTRATION"

// 容量预占与容量释放（ADR-0203 决定三）：为一个业务范围占住一个池的一段容量，与把其中未消耗的部分还回去。两者是运营
// 人员作出的生产决定，各成一种：能占的不因此能放，授予的粒度等于权力的粒度。
const (
	DecisionCapacityReservation DecisionKind = "CAPACITY_RESERVATION"
	DecisionCapacityRelease     DecisionKind = "CAPACITY_RELEASE"
)

var decisionKinds = []DecisionKind{
	DecisionManualReviewCompletion, DecisionActiveRejection, DecisionAuthorizedDisposition,
	DecisionControlledClosure, DecisionControlledReopening,
	DecisionSegmentClosure, DecisionDispatchTaskRegistration, DecisionLoadAssignment, DecisionParticipationTermination,
	DecisionEffectiveTimeJudgment, DecisionCarrierFirstEffectivePickupJudgment,
	DecisionPendingIdentityRecheck,
	DecisionIdentityConflictDisposition,
	DecisionTransportCommissionSubmission,
	DecisionTransportChargeOccurrenceRegistration,
	DecisionCapacityReservation, DecisionCapacityRelease,
}

// ParseDecisionKind 按字面取值认决定种类，大小写不折叠：册里存的就是这些字面量。
func ParseDecisionKind(value string) (DecisionKind, error) {
	for _, kind := range decisionKinds {
		if string(kind) == value {
			return kind, nil
		}
	}
	return "", ErrDecisionKindUnknown
}

func (kind DecisionKind) String() string { return string(kind) }

// NewOperationDecisionGrant 登一笔「运营决定」授予：能力面固定为 CapabilityOperationDecision，
// 决定种类必填且须是已知的一种。其余各件的纪律同 NewOperatorGrant。
func NewOperationDecisionGrant(
	tenantID string,
	grantID string,
	subject OperatorSubject,
	kind DecisionKind,
	interval EffectiveInterval,
	basis string,
) (OperatorGrant, error) {
	if _, err := ParseDecisionKind(string(kind)); err != nil {
		return OperatorGrant{}, err
	}
	tenant := strings.TrimSpace(tenantID)
	id := strings.TrimSpace(grantID)
	reference := strings.TrimSpace(basis)
	if tenant == "" || id == "" || subject.issuer == "" || interval.startsAt.IsZero() || reference == "" {
		return OperatorGrant{}, ErrIncompleteOperatorRegistration
	}
	return OperatorGrant{
		tenantID:     tenant,
		grantID:      id,
		subject:      subject,
		face:         CapabilityOperationDecision,
		decisionKind: kind,
		interval:     interval,
		basis:        reference,
	}, nil
}

// DecisionKind 交回「运营决定」授予的决定种类；别的能力面上为空。
func (grant OperatorGrant) DecisionKind() DecisionKind { return grant.decisionKind }

// HoldsDecisionAt 答这个操作者在 at 那一刻是否持有 kind 这一种运营决定的生效授予。
func (standing OperatorStanding) HoldsDecisionAt(kind DecisionKind, at time.Time) bool {
	if kind == "" {
		return false
	}
	for _, recorded := range standing.grants {
		if recorded.grant.face == CapabilityOperationDecision && recorded.grant.decisionKind == kind && recorded.EffectiveAt(at) {
			return true
		}
	}
	return false
}
