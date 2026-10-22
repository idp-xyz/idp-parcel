package application_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// Covers: PS CONTEXT 词条「控制项结果」与生命周期「判断遇到客户可补充缺口 → 等待受控补充」、ADR-0132 决定一的理由与
// ADR-0106 决定三——停住的那一版只由新提交版本的信封续办，它不会再被判；占着客户的资金或额度等一份不会到来的接受
// 没有依据。所以暂停落库之后，本版本已成立的占用按原关联释放，暂停照旧交回。
func TestACustomerSupplementPauseReleasesThePausedVersionsOccupation(t *testing.T) {
	fixture := newDecisionFixture(t)
	fixture.judgments.reachability["parcel-2"] = domain.ReachabilityInsufficientEvidence

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.PendingReason() != application.CustomerSupplementPending {
		t.Fatalf("pending reason = %q, want CUSTOMER_SUPPLEMENT_PENDING", result.PendingReason())
	}
	if fixture.requests.saved == nil {
		t.Fatal("暂停没落库")
	}
	if fixture.release.calls != 1 {
		t.Fatalf("release calls = %d, want 1——停住的那一版不会再被判，它占下的那一笔要放", fixture.release.calls)
	}
	if got := fixture.release.versions[0]; got != fixture.command(t).SubmissionVersion {
		t.Fatalf("释放指名的版本 = %q, want 停住的那一版 %q", got, fixture.command(t).SubmissionVersion)
	}
	if fixture.release.controlResultID != "SAC-1" {
		t.Fatalf("释放认领的控制结果 = %q, want 原关联 SAC-1", fixture.release.controlResultID)
	}
	if result.CompensationReference().String() != "" {
		t.Fatalf("释放成功却留了补偿引用 %q", result.CompensationReference())
	}
}

// Covers: 同上那一格的「释放失败按补偿续办」与 CONTEXT「撤回提交和释放属于可补偿编排」——放不掉不撤销暂停：等待态
// 照旧落库、原因照旧是`等待受控补充`（消费门据它入账），补偿引用与那一版若被拒绝时派生的逐字一致。
func TestAFailedPauseReleaseKeepsThePauseAndLeavesCompensationPending(t *testing.T) {
	paused := newDecisionFixture(t)
	paused.judgments.reachability["parcel-2"] = domain.ReachabilityInsufficientEvidence
	paused.release.err = errors.New("settlement authority unavailable")

	result, err := paused.handler.Handle(context.Background(), paused.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.PendingReason() != application.CustomerSupplementPending {
		t.Fatalf("pending reason = %q, want CUSTOMER_SUPPLEMENT_PENDING——一次补偿失败改写了暂停", result.PendingReason())
	}
	if paused.requests.saved == nil {
		t.Fatal("释放失败撤掉了已落库的暂停")
	}
	if waiting, present := paused.requests.saved.AcceptanceDecisionTask().WaitingOn(); !present || waiting != domain.ResumeByCustomerSupplement {
		t.Fatalf("saved waitingOn = (%v, %v), want CUSTOMER_SUPPLEMENT", waiting, present)
	}
	if result.CompensationReference().String() == "" {
		t.Fatal("释放没收口却没有补偿引用")
	}

	rejected := newDecisionFixture(t)
	rejected.judgments.reachability["parcel-2"] = domain.ReachabilityUnreachable
	rejected.release.err = errors.New("settlement authority unavailable")
	byRejection, err := rejected.handler.Handle(context.Background(), rejected.command(t))
	if err != nil {
		t.Fatalf("handle rejection: %v", err)
	}
	if result.CompensationReference().String() != byRejection.CompensationReference().String() {
		t.Fatalf("暂停 %q, 拒绝 %q——同一笔占用因同一原因没放掉，两条路必须给出同一个引用",
			result.CompensationReference(), byRejection.CompensationReference())
	}
}

// Covers: PS CONTEXT「没有任何控制项成立时不发释放」——停等补充的那一版没占下什么（明确无控制）就不发释放。
func TestACustomerSupplementPauseWithoutOccupationSendsNoRelease(t *testing.T) {
	fixture := newDecisionFixture(t)
	fixture.judgments.reachability["parcel-2"] = domain.ReachabilityInsufficientEvidence
	fixture.judgments.controlOutcome = domain.FinancialControlNotApplicable

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.PendingReason() != application.CustomerSupplementPending {
		t.Fatalf("pending reason = %q, want CUSTOMER_SUPPLEMENT_PENDING", result.PendingReason())
	}
	if fixture.release.calls != 0 {
		t.Fatalf("release calls = %d, want 0——对着一笔不存在的占用发了释放", fixture.release.calls)
	}
}

// Covers: 暂停没落库就不放——那一轮整体回滚重投，交回的是保存那一格的原因而不是`等待受控补充`；此刻放掉，等于在
// 一份不知道停没停住的版本上提前宣告它不会再被判。
func TestAPauseThatFailedToPersistReleasesNothing(t *testing.T) {
	unreachable := newDecisionFixture(t)
	unreachable.judgments.reachability["parcel-2"] = domain.ReachabilityInsufficientEvidence
	unreachable.requests.err = errors.New("storage unreachable")
	if _, err := unreachable.handler.Handle(context.Background(), unreachable.command(t)); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if unreachable.release.calls != 0 {
		t.Fatalf("保存失败仍发了 %d 次释放", unreachable.release.calls)
	}

	outraced := newDecisionFixture(t)
	outraced.judgments.reachability["parcel-2"] = domain.ReachabilityInsufficientEvidence
	outraced.requests.conflict = true
	if _, err := outraced.handler.Handle(context.Background(), outraced.command(t)); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if outraced.release.calls != 0 {
		t.Fatalf("版本冲突仍发了 %d 次释放——抢先那一方写下的可能是接受", outraced.release.calls)
	}
}

// Covers: 只有`等待受控补充`随暂停释放——`等待人工复核`那一版复核完成后仍由任务按规则判、可以接受，那时那笔占用是
// 合法的；`等待授权处置`由处置命令自己放（ADR-0132 决定一），形成决定这一步再放一次就是两条路认领同一笔。
func TestOnlyTheCustomerSupplementPauseReleases(t *testing.T) {
	review := newDecisionFixture(t)
	review.commercial.manualReview = domain.ManualReviewRequiredByRules
	result, err := review.handler.Handle(context.Background(), review.command(t))
	if err != nil {
		t.Fatalf("handle review: %v", err)
	}
	if result.PendingReason() != application.ManualReviewPending {
		t.Fatalf("pending reason = %q, want MANUAL_REVIEW_PENDING", result.PendingReason())
	}
	if review.release.calls != 0 {
		t.Fatalf("等待人工复核放了 %d 次——复核完成后这一版仍可能被接受", review.release.calls)
	}

	disposition := newDecisionFixture(t)
	control := awaitingDispositionControl(t)
	disposition.judgments.control = &control
	result, err = disposition.handler.Handle(context.Background(), disposition.command(t))
	if err != nil {
		t.Fatalf("handle disposition: %v", err)
	}
	if result.PendingReason() != application.AuthorizedDispositionPending {
		t.Fatalf("pending reason = %q, want AUTHORIZED_DISPOSITION_PENDING", result.PendingReason())
	}
	if disposition.release.calls != 0 {
		t.Fatalf("等待授权处置放了 %d 次——那一笔归处置命令放", disposition.release.calls)
	}
}
