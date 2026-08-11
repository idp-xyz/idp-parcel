package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// JudgmentPendingReason 指名本轮为何有一件事没有办完：多数取值说的是接受判断任务没能推进，
// `ControlReleasePending` 说的是决定已经成立、随附的补偿没能确定完成。两类共用一个封闭集合，
// 是因为续办引用由「原因 + 范围」派生，原因分成两套会让同一范围派生出两族互不相认的引用。
//
// 它是封闭集合而非自由文本，这样未决才能按依赖阶段分类统计——用例的可观测性一节明禁用自由
// 文本聚合原因维度。取值与产生它的那条路径同时出现。
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
	ControlReleasePending
	RejectionAuthorityUnavailable
	CustomerSupplementPending
	ManualReviewPending
	WithdrawalAuthorityUnavailable
	SourceDataAmendmentAuthorityUnavailable
	SourceDataRuleUnavailable
	SourceDataVersionIdentityUnavailable
	AmendedRequestNotSaved
	SourceDataVersionNotHandedOff
)

// resumePath 由未决原因导出续办方，取值与 CONTEXT 接受判断任务的三个等待态一一对应。
//
// 它是原因的全函数，而不是调用点上的常量：写成常量，新增一个原因就会静默继承上一个调用点的
// 路径，而路径错了等于催错人——依赖抖动去催客户补件，或者对着一件只有人能推进的复核无休止
// 地内部重试。
func (reason JudgmentPendingReason) resumePath() domain.ResumePath {
	switch reason {
	case CustomerSupplementPending:
		return domain.ResumeByCustomerSupplement
	case ManualReviewPending:
		return domain.ResumeByManualReview
	default:
		// 其余取值全是依赖答不出或声明未到，只有本方推得动。这一条不靠任何未确认规则：
		// 客户和复核角色都补不出一个查不回来的授权，或者一次没落库的保存。
		return domain.ResumeByInternalRetry
	}
}

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
	case ControlReleasePending:
		return "CONTROL_RELEASE_PENDING"
	case RejectionAuthorityUnavailable:
		return "REJECTION_AUTHORITY_UNAVAILABLE"
	case CustomerSupplementPending:
		return "CUSTOMER_SUPPLEMENT_PENDING"
	case ManualReviewPending:
		return "MANUAL_REVIEW_PENDING"
	case WithdrawalAuthorityUnavailable:
		return "WITHDRAWAL_AUTHORITY_UNAVAILABLE"
	case SourceDataAmendmentAuthorityUnavailable:
		return "SOURCE_DATA_AMENDMENT_AUTHORITY_UNAVAILABLE"
	case SourceDataRuleUnavailable:
		return "SOURCE_DATA_RULE_UNAVAILABLE"
	case SourceDataVersionIdentityUnavailable:
		return "SOURCE_DATA_VERSION_IDENTITY_UNAVAILABLE"
	case AmendedRequestNotSaved:
		return "AMENDED_REQUEST_NOT_SAVED"
	case SourceDataVersionNotHandedOff:
		return "SOURCE_DATA_VERSION_NOT_HANDED_OFF"
	default:
		return ""
	}
}

// recordAttempt 把没能推进的这一轮追加到接受判断任务上。用例要求任务同时留下判断与处理
// 尝试，只留成功的判断会让一份卡了十轮的委托看起来和刚建单的一样。
//
// 记录失败不改写本轮的未决原因：原因说的是判断为何没推进，用「记录失败」顶替它会把真实
// 缺口藏起来，而调用方正是按原因决定该补缺口还是该重试依赖。这条记录本身按同一续办引用
// 补写。
//
// 续办路径由原因导出，不在这里给定值：谁能补上这个缺口是原因自带的属性，写死在记录处会让
// 同一个原因在不同调用点走不同路径。
func recordAttempt(
	ctx context.Context,
	recorder ports.AcceptanceJudgmentRecorder,
	clock ports.Clock,
	requestID domain.ShipmentRequestID,
	reason JudgmentPendingReason,
	continuation domain.OwnershipContinuationReference,
) {
	attemptReason, err := domain.NewProcessingAttemptReason(reason.String())
	if err != nil {
		return
	}
	attempt, err := domain.NewProcessingAttempt(domain.ProcessingAttemptSpec{
		Reason:       attemptReason,
		ResumePath:   reason.resumePath(),
		Continuation: continuation,
		AttemptedAt:  clock.Now(),
	})
	if err != nil {
		return
	}
	_ = recorder.RecordProcessingAttempt(ctx, requestID, attempt)
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
