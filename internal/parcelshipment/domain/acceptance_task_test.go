package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

var firstAttemptAt = time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)

func internalAttempt(t *testing.T, reason string, at time.Time) domain.ProcessingAttempt {
	t.Helper()
	attempt, err := domain.NewProcessingAttempt(domain.ProcessingAttemptSpec{
		Reason:       mustValue(t, domain.NewProcessingAttemptReason, reason),
		ResumePath:   domain.ResumeByInternalRetry,
		Continuation: mustValue(t, domain.NewOwnershipContinuationReference, "CONT-"+reason),
		AttemptedAt:  at,
	})
	if err != nil {
		t.Fatalf("new processing attempt: %v", err)
	}
	return attempt
}

// Covers: ADR-0383 决定三——处理尝试在原因、续办方、续办引用、时刻之外另记结算的未形成原因与这一版申报的服务产品；
// 两格都可缺，缺即那一轮没有，不代拟。
func TestAProcessingAttemptCarriesTheSettlementReasonAndTheServiceProductOnlyWhenGiven(t *testing.T) {
	bare := internalAttempt(t, "COMMERCIAL_BASIS_UNAVAILABLE", firstAttemptAt)
	if reason, present := bare.SettlementReason(); present || reason.String() != "" {
		t.Fatalf("settlement reason = %q（present=%v），want 缺席", reason, present)
	}
	if bare.ServiceProduct().Declared() {
		t.Fatalf("service product = %q，want 没申报", bare.ServiceProduct())
	}

	entry, err := domain.NewCanonicalContentEntry(domain.RequestedServiceProductEntryName, "SYN-PROD-01")
	if err != nil {
		t.Fatalf("canonical entry: %v", err)
	}
	product := domain.RequestedServiceProductOf([]domain.CanonicalContentEntry{entry})
	detailed, err := domain.NewProcessingAttempt(domain.ProcessingAttemptSpec{
		Reason:           mustValue(t, domain.NewProcessingAttemptReason, "FINANCIAL_CONTROL_NOT_CONFIGURED"),
		ResumePath:       domain.ResumeByOperatorRegistration,
		Continuation:     mustValue(t, domain.NewOwnershipContinuationReference, "CONT-PRICE-CARD"),
		AttemptedAt:      firstAttemptAt,
		SettlementReason: mustValue(t, domain.NewSettlementNotFormedReason, "ESTIMATE_PRICE_CARD_NOT_CONFIGURED"),
		ServiceProduct:   product,
	})
	if err != nil {
		t.Fatalf("new processing attempt: %v", err)
	}
	if reason, present := detailed.SettlementReason(); !present || reason.String() != "ESTIMATE_PRICE_CARD_NOT_CONFIGURED" {
		t.Fatalf("settlement reason = %q（present=%v），want ESTIMATE_PRICE_CARD_NOT_CONFIGURED", reason, present)
	}
	if detailed.ServiceProduct().String() != "SYN-PROD-01" {
		t.Fatalf("service product = %q，want SYN-PROD-01", detailed.ServiceProduct())
	}
	if _, err := domain.NewSettlementNotFormedReason("  "); err == nil {
		t.Fatal("空白的结算原因不该立得起来：空白与缺席读起来一样")
	}
}

func reviewCompletion(t *testing.T) domain.ManualReviewCompletion {
	t.Helper()
	completion, err := domain.NewManualReviewCompletion(domain.ManualReviewCompletionSpec{
		Authority:   mustValue(t, domain.NewReviewAuthorityReference, "PC-REVIEW-ROLE-1"),
		Reviewer:    mustValue(t, domain.NewReviewerReference, "OPERATOR-1"),
		Evidence:    mustValue(t, domain.NewReviewEvidenceReference, "EVID-1"),
		CompletedAt: firstAttemptAt,
	})
	if err != nil {
		t.Fatalf("new manual review completion: %v", err)
	}
	return completion
}

// Covers: UC-PS-001 AT-PS-034「只由规则授权的角色按证据完成复核」与 CONTEXT「普通备注、口头
// 意见或未经授权的操作不能形成拒绝事实」— 授权依据、实际复核方与证据缺一不可。缺了就与
// 「有人点了一下」分不开，而用例要求复核留痕到可追责。
func TestAManualReviewCompletionWithoutAuthorityOrEvidenceCannotBeBuilt(t *testing.T) {
	full := domain.ManualReviewCompletionSpec{
		Authority:   mustValue(t, domain.NewReviewAuthorityReference, "PC-REVIEW-ROLE-1"),
		Reviewer:    mustValue(t, domain.NewReviewerReference, "OPERATOR-1"),
		Evidence:    mustValue(t, domain.NewReviewEvidenceReference, "EVID-1"),
		CompletedAt: firstAttemptAt,
	}
	missing := map[string]func(domain.ManualReviewCompletionSpec) domain.ManualReviewCompletionSpec{
		"authority": func(s domain.ManualReviewCompletionSpec) domain.ManualReviewCompletionSpec {
			s.Authority = domain.ReviewAuthorityReference{}
			return s
		},
		"reviewer": func(s domain.ManualReviewCompletionSpec) domain.ManualReviewCompletionSpec {
			s.Reviewer = domain.ReviewerReference{}
			return s
		},
		"evidence": func(s domain.ManualReviewCompletionSpec) domain.ManualReviewCompletionSpec {
			s.Evidence = domain.ReviewEvidenceReference{}
			return s
		},
	}

	for name, drop := range missing {
		t.Run(name, func(t *testing.T) {
			if _, err := domain.NewManualReviewCompletion(drop(full)); !errors.Is(
				err, domain.ErrInvalidManualReviewCompletion,
			) {
				t.Fatalf("error = %v, want ErrInvalidManualReviewCompletion", err)
			}
		})
	}
}

// Covers: CONTEXT「判断任务 → 已完成：只有接受或拒绝决定已经越过提交边界时完成」— 决定形成
// 之后不能再补一次复核完成。允许补录等于让复核去追认一个已经形成的结果。
func TestACompletedTaskRefusesAManualReviewCompletion(t *testing.T) {
	decided, err := submitted(t).Decide(decisionSpec(t, allGroupsPassing(t)))
	if err != nil {
		t.Fatalf("decide: %v", err)
	}

	if _, err := decided.CompleteManualReview(reviewCompletion(t)); !errors.Is(
		err, domain.ErrAcceptanceTaskComplete,
	) {
		t.Fatalf("error = %v, want ErrAcceptanceTaskComplete", err)
	}
}

// Covers: CONTEXT「客户可补充的资料缺口与系统内部查询或重试必须使用不同原因和续办路径」—
// 续办路径是尝试的必填部分。缺了它，一次等客户补资料的停顿与一次等依赖恢复的停顿在记录上
// 分不开，而这两者的后续动作完全不同。
func TestAnAttemptWithoutAResumePathCannotBeBuilt(t *testing.T) {
	if _, err := domain.NewProcessingAttempt(domain.ProcessingAttemptSpec{
		Reason:       mustValue(t, domain.NewProcessingAttemptReason, "COMMERCIAL_BASIS_UNAVAILABLE"),
		Continuation: mustValue(t, domain.NewOwnershipContinuationReference, "CONT-1"),
		AttemptedAt:  firstAttemptAt,
	}); !errors.Is(err, domain.ErrInvalidProcessingAttempt) {
		t.Fatalf("error = %v, want ErrInvalidProcessingAttempt", err)
	}
}
