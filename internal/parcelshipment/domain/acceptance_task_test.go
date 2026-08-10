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

// Covers: UC-PS-001「已经建立委托时保持`已提交`，建立或续办独立接受判断任务并追加判断与
// 处理尝试」— 追加是追加，第二次尝试不覆盖第一次。覆盖了就说不出这份委托卡过几轮、各卡在
// 哪里，而用例要求未决按原因分类统计。
func TestProcessingAttemptsAccumulateRatherThanOverwrite(t *testing.T) {
	request := submitted(t)

	first, err := request.RecordProcessingAttempt(internalAttempt(t, "COMMERCIAL_BASIS_UNAVAILABLE", firstAttemptAt))
	if err != nil {
		t.Fatalf("record first attempt: %v", err)
	}
	second, err := first.RecordProcessingAttempt(
		internalAttempt(t, "RECORDED_JUDGMENTS_UNAVAILABLE", firstAttemptAt.Add(time.Hour)),
	)
	if err != nil {
		t.Fatalf("record second attempt: %v", err)
	}

	attempts := second.AcceptanceDecisionTask().ProcessingAttempts()
	if len(attempts) != 2 {
		t.Fatalf("attempts = %d, want 2", len(attempts))
	}
	if attempts[0].Reason().String() != "COMMERCIAL_BASIS_UNAVAILABLE" {
		t.Fatalf("first attempt = %q; the earlier attempt was overwritten", attempts[0].Reason())
	}
	// 委托没有因为记录尝试而离开`已提交`：尝试是处理记录，不是业务决定。
	if second.State() != domain.ShipmentRequestSubmitted {
		t.Fatalf("state = %q, want SUBMITTED", second.State())
	}
}

// Covers: CONTEXT 接受判断任务生命周期「判断任务 → 已完成：只有接受或拒绝决定已经越过提交
// 边界时完成」— 已完成的任务不再累积尝试。允许追加等于让一份已决委托看起来还在处理中。
func TestACompletedTaskRefusesFurtherAttempts(t *testing.T) {
	decided, err := submitted(t).Decide(decisionSpec(t, allGroupsPassing(t)))
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	if !decided.AcceptanceDecisionTask().IsComplete() {
		t.Fatal("a formed decision left the acceptance judgment task incomplete")
	}

	if _, err := decided.RecordProcessingAttempt(
		internalAttempt(t, "RECORDED_JUDGMENTS_UNAVAILABLE", firstAttemptAt),
	); !errors.Is(err, domain.ErrAcceptanceTaskComplete) {
		t.Fatalf("error = %v, want ErrAcceptanceTaskComplete", err)
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
