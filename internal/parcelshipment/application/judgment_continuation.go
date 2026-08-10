package application

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// JudgmentPendingReason 指名接受判断任务本轮为何没有推进。它是封闭集合而非自由文本，
// 这样未决才能按依赖阶段分类统计——用例的可观测性一节明禁用自由文本聚合原因维度。
// 取值与产生它的那条路径同时出现。
//
// 依赖调不通也在这里取值，而不是上抛技术错误。用例把`依赖不可用`列为`尚未决定`的成因，
// 并要求该结果返回未决原因与安全续办引用，而一个 error 两样都给不出。指名是哪个依赖停了，
// 也比一句错误文本更能保住「权威答不出」与「本地坏了」的区别——后者正是上抛想护住的东西。
//
// 这不等于吞掉错误：破坏不变量的失败仍然上抛。分界与 settlement-accounting 一致——依赖
// 答不出是业务结果，取回的东西根本不属于这个请求则是编程错误。
type JudgmentPendingReason uint8

const (
	PendingReasonNone JudgmentPendingReason = iota
	CommercialBasisNotUnique
	CommercialBasisUnavailable
	ReachabilityAsOfNotDeclared
	ReachabilityAuthorityUnavailable
	FinancialControlAsOfNotDeclared
	FinancialControlUnavailable
	JudgmentNotRecorded
	ShipmentRequestUnavailable
	ManualReviewPolicyNotDeclared
	RecordedJudgmentsUnavailable
	DecisionIdentityUnavailable
	AcceptanceJudgmentIncomplete
	DecisionNotRecorded
)

func (reason JudgmentPendingReason) String() string {
	switch reason {
	case CommercialBasisNotUnique:
		return "COMMERCIAL_BASIS_NOT_UNIQUE"
	case CommercialBasisUnavailable:
		return "COMMERCIAL_BASIS_UNAVAILABLE"
	case ReachabilityAsOfNotDeclared:
		return "REACHABILITY_AS_OF_NOT_DECLARED"
	case ReachabilityAuthorityUnavailable:
		return "REACHABILITY_AUTHORITY_UNAVAILABLE"
	case FinancialControlAsOfNotDeclared:
		return "FINANCIAL_CONTROL_AS_OF_NOT_DECLARED"
	case FinancialControlUnavailable:
		return "FINANCIAL_CONTROL_UNAVAILABLE"
	case JudgmentNotRecorded:
		return "JUDGMENT_NOT_RECORDED"
	case ShipmentRequestUnavailable:
		return "SHIPMENT_REQUEST_UNAVAILABLE"
	case ManualReviewPolicyNotDeclared:
		return "MANUAL_REVIEW_POLICY_NOT_DECLARED"
	case RecordedJudgmentsUnavailable:
		return "RECORDED_JUDGMENTS_UNAVAILABLE"
	case DecisionIdentityUnavailable:
		return "DECISION_IDENTITY_UNAVAILABLE"
	case AcceptanceJudgmentIncomplete:
		return "ACCEPTANCE_JUDGMENT_INCOMPLETE"
	case DecisionNotRecorded:
		return "DECISION_NOT_RECORDED"
	default:
		return ""
	}
}

// judgmentContinuation 由未决原因与判断范围共同派生，因此同一范围因同一原因停滞时拿到的
// 引用始终相同——这正是调用方能查询原次尝试而不必靠猜的原因。原因参与派生也意味着停在
// 不同阶段的两次未决给出不同引用，用例要求二者分别统计、各走各的续办路径。
func judgmentContinuation(reason JudgmentPendingReason, scope ...string) domain.OwnershipContinuationReference {
	digest := sha256.Sum256([]byte(strings.Join(append([]string{reason.String()}, scope...), "\x00")))
	continuation, err := domain.NewOwnershipContinuationReference("CONT-" + hex.EncodeToString(digest[:8]))
	if err != nil {
		return domain.OwnershipContinuationReference{}
	}
	return continuation
}
