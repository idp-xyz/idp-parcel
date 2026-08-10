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
type JudgmentPendingReason uint8

const (
	PendingReasonNone JudgmentPendingReason = iota
	CommercialBasisNotUnique
	ReachabilityAsOfNotDeclared
	FinancialControlAsOfNotDeclared
)

func (reason JudgmentPendingReason) String() string {
	switch reason {
	case CommercialBasisNotUnique:
		return "COMMERCIAL_BASIS_NOT_UNIQUE"
	case ReachabilityAsOfNotDeclared:
		return "REACHABILITY_AS_OF_NOT_DECLARED"
	case FinancialControlAsOfNotDeclared:
		return "FINANCIAL_CONTROL_AS_OF_NOT_DECLARED"
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
