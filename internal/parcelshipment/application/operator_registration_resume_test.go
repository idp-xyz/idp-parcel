package application

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件钉住 ADR-0094 Decision 二在应用层的落点：哪些未决原因等的是一次**登记**，而不是
// 一次会自行恢复的重试。
//
// 判据是恢复动作，不是缺了什么：重试、客户补件、人工复核三者都推不动的，才是第四格。

// TestNotConfiguredReasonsResumeByOperatorRegistration 逐个列举，不按名字前缀推。
//
// 逐个列而不写成「凡 *NotConfigured 结尾者」是有意的：命名相近不构成同一个恢复动作，
// 而这份名单本身就是那条判据的落地——新增一个原因时要回答的是它等谁，不是它叫什么。
func TestNotConfiguredReasonsResumeByOperatorRegistration(t *testing.T) {
	waitingOnRegistration := []JudgmentPendingReason{
		RejectionAuthorityRulesNotConfigured,
		WithdrawalAuthorityRulesNotConfigured,
		SourceDataAmendmentAuthorityRulesNotConfigured,
		ReachabilityAsOfNotConfigured,
		FinancialControlAsOfNotConfigured,
	}

	for _, reason := range waitingOnRegistration {
		if got := reason.resumePath(); got != domain.ResumeByOperatorRegistration {
			t.Errorf("%s 等的是一次登记，续办路径应为 OPERATOR_REGISTRATION，实际 %s",
				reason.String(), got.String())
		}
	}
}

// TestUnavailableReasonsStayOnInternalRetry 守的是同一条判据的另一半。
//
// `*Unavailable` 那一族是权威一时答不出，会自行恢复，重试是对的。它与上面那一族的名字只差
// 一个词，而恢复动作相反——压成一格正是 ADR-0094 要拆开的那个错。
func TestUnavailableReasonsStayOnInternalRetry(t *testing.T) {
	selfHealing := []JudgmentPendingReason{
		RejectionAuthorityUnavailable,
		WithdrawalAuthorityUnavailable,
		SourceDataAmendmentAuthorityUnavailable,
		ReachabilityAsOfUnavailable,
		FinancialControlAsOfUnavailable,
		CommercialBasisUnavailable,
	}

	for _, reason := range selfHealing {
		if got := reason.resumePath(); got != domain.ResumeByInternalRetry {
			t.Errorf("%s 等的是权威恢复，续办路径应为 INTERNAL_RETRY，实际 %s",
				reason.String(), got.String())
		}
	}
}

// TestCustomerAndReviewPathsUnchanged 钉住 ADR-0094 没有改的两格。
//
// `等待受控补充`维持回滚重投那一格由 ADR-0086 判过，本轮不动它（理由记在 ADR-0094
// Decision 三与票 first-tenant-runway/09）；这里只确认它没被顺手改掉。
func TestCustomerAndReviewPathsUnchanged(t *testing.T) {
	if got := CustomerSupplementPending.resumePath(); got != domain.ResumeByCustomerSupplement {
		t.Errorf("客户补件那一格不该变，实际 %s", got.String())
	}
	if got := ManualReviewPending.resumePath(); got != domain.ResumeByManualReview {
		t.Errorf("人工复核那一格不该变，实际 %s", got.String())
	}
}
