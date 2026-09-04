package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// Covers: UC-PS-001 步骤 8「适用硬规则和所需判断全部通过时自动接受」与 AT-PS-033 — 规则
// 没有要求人工复核时系统自行形成接受，不等一个无依据的人工审批。
func TestAllAdoptedJudgmentsPassingFormsAnAcceptance(t *testing.T) {
	fixture := newDecisionFixture(t)

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AcceptanceDecided {
		t.Fatalf("outcome = %q, want DECIDED", result.Outcome())
	}
	if result.State() != domain.ShipmentRequestAccepted {
		t.Fatalf("state = %q, want ACCEPTED", result.State())
	}
	decision, present := result.AcceptanceDecision()
	if !present || !decision.Accepted() {
		t.Fatalf("decision = %#v present = %v", decision, present)
	}
	if fixture.requests.saved == nil {
		t.Fatal("an acceptance was formed but never saved")
	}
	if fixture.requests.saved.State() != domain.ShipmentRequestAccepted {
		t.Fatalf("saved state = %q, want ACCEPTED", fixture.requests.saved.State())
	}
}

// Covers: UC-PS-001 接受条件`接受前财务控制`「不得默认放行」— 控制从未形成时接受不成立。
// 这是本编排最容易出错的一步：漏掉那一项校验，Decide 会看到「没有失败也没有待判断」而径直
// 接受，而那是一次以遗漏方式实现的默认放行。
func TestAMissingFinancialControlBlocksAcceptance(t *testing.T) {
	fixture := newDecisionFixture(t)
	fixture.judgments.controlOutcome = domain.FinancialControlOutcomeInvalid

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AcceptanceUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
	}
	if result.State() != domain.ShipmentRequestSubmitted {
		t.Fatalf("state = %q; a never-formed control let the request leave SUBMITTED", result.State())
	}
	if _, present := result.AcceptanceDecision(); present {
		t.Fatal("an undecided round recorded an acceptance decision")
	}
}

// Covers: UC-PS-001 接受条件「每个适用校验组都必须通过」的反面 — 规则包没把接受前财务
// 控制列为适用时，不得凭空塞一项永远满足不了的`无法判定`。一份合同本就不要求财务控制的
// 委托会因此永远接受不了，那是把「不适用」读成了「缺一项」。
func TestAnInapplicableFinancialControlDoesNotDeadlockAcceptance(t *testing.T) {
	fixture := newDecisionFixture(t)
	fixture.commercial.applicable = []domain.AcceptanceCheckGroup{domain.NetworkReachabilityCheck}
	fixture.judgments.controlOutcome = domain.FinancialControlOutcomeInvalid

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.State() != domain.ShipmentRequestAccepted {
		t.Fatalf("state = %q; a control the rules never required blocked acceptance", result.State())
	}
}

// Covers: UC-PS-001「任一必需控制不通过时按策略拒绝」— 声明只能增加要求，减不掉失败。
// 规则包没把财务控制列为适用，但控制确实跑出了`业务限制`时，那次失败照样拒掉整份版本。
func TestARestrictedControlStillRejectsEvenWhenTheGroupIsNotDeclaredApplicable(t *testing.T) {
	fixture := newDecisionFixture(t)
	fixture.commercial.applicable = []domain.AcceptanceCheckGroup{domain.NetworkReachabilityCheck}
	fixture.judgments.controlOutcome = domain.FinancialControlRestricted

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.State() != domain.ShipmentRequestRejected {
		t.Fatalf("state = %q; a business restriction was discarded because its group was not declared applicable", result.State())
	}
}

// Covers: UC-PS-001 步骤 8「规则显式要求时进入人工复核」与 CONTEXT 所有权 — 规则包没有
// 声明复核策略时不接受，也不替它在「要求」与「不要求」之间挑一个。
func TestAnUndeclaredManualReviewPolicyBlocksAcceptance(t *testing.T) {
	fixture := newDecisionFixture(t)
	fixture.commercial.manualReview = domain.ManualReviewNotDeclaredByRules

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AcceptanceUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
	}
	if result.PendingReason() != application.ManualReviewPolicyNotDeclared {
		t.Fatalf("pending reason = %q, want MANUAL_REVIEW_POLICY_NOT_DECLARED", result.PendingReason())
	}
	if fixture.requests.saved != nil {
		t.Fatal("an undecided round saved the request")
	}
}

// Covers: UC-PS-001 接受条件矩阵`标准网络可达性`「资料不足不得映射为不可达」与 AT-PS-006
// —— 资料不足保持未决，绝不写成拒绝。
func TestInsufficientEvidenceLeavesTheRequestSubmittedRatherThanRejected(t *testing.T) {
	fixture := newDecisionFixture(t)
	fixture.judgments.reachability["parcel-2"] = domain.ReachabilityInsufficientEvidence

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.State() == domain.ShipmentRequestRejected {
		t.Fatal("insufficient evidence was written straight into a rejection")
	}
	if result.Outcome() != application.AcceptanceUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
	}
}

// Covers: UC-PS-001 接受条件矩阵`标准网络可达性`「明确不可达时整份当前提交版本不能直接
// 接受」与 AT-PS-005 — 一个成员不可达拒掉整份版本，不做成员级部分接受。
func TestOneUnreachableMemberRejectsTheWholeSubmissionVersion(t *testing.T) {
	fixture := newDecisionFixture(t)
	fixture.judgments.reachability["parcel-2"] = domain.ReachabilityUnreachable

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.State() != domain.ShipmentRequestRejected {
		t.Fatalf("state = %q, want REJECTED", result.State())
	}
	decision, present := result.AcceptanceDecision()
	if !present || len(decision.FailedChecks()) != 1 {
		t.Fatalf("rejection did not record exactly the failing check: %#v", decision)
	}
}

// Covers: UC-PS-001 AT-PS-035「接受提交未成立时按原关联释放冻结」— 拒绝就是接受确定未成立，
// 冻结不能留在原处占着货主的钱。释放按原控制结果关联发起，本上下文不拥有金额或账户。
func TestARejectionReleasesTheFreezeByItsOriginalAssociation(t *testing.T) {
	fixture := newDecisionFixture(t)
	fixture.judgments.reachability["parcel-2"] = domain.ReachabilityUnreachable

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.State() != domain.ShipmentRequestRejected {
		t.Fatalf("state = %q, want REJECTED", result.State())
	}
	if fixture.release.calls != 1 {
		t.Fatalf("release calls = %d, want exactly 1", fixture.release.calls)
	}
	if fixture.release.controlResultID != "SAC-1" {
		t.Fatalf("released %q, want the original control association SAC-1", fixture.release.controlResultID)
	}
}

// Covers: UC-PS-001:112「客户可补充缺口与系统依赖重试必须使用不同原因和续办路径」、AT-PS-006
// 「资料不足…不映射为不可达」与 CONTEXT 接受判断任务「客户可补充缺口 → 等待受控补充」——
// 权威已经把话说完了，说的正是声明资料不够判，所以这一轮等的是客户而不是本方重试。
func TestAnInsufficientEvidenceJudgmentWaitsOnTheCustomerNotAnInternalRetry(t *testing.T) {
	fixture := newDecisionFixture(t)
	fixture.judgments.reachability["parcel-2"] = domain.ReachabilityInsufficientEvidence

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AcceptanceUndecided {
		t.Fatalf("outcome = %q; insufficient evidence formed a lifecycle decision", result.Outcome())
	}
	if result.PendingReason() != application.CustomerSupplementPending {
		t.Fatalf(
			"pending reason = %q, want CUSTOMER_SUPPLEMENT_PENDING; retrying an unchanged declaration never resolves a customer data gap",
			result.PendingReason(),
		)
	}
	if len(fixture.recorder.recordedAttempts) != 1 {
		t.Fatalf("recorded %d attempts, want exactly 1", len(fixture.recorder.recordedAttempts))
	}
	if path := fixture.recorder.recordedAttempts[0].ResumePath(); path != domain.ResumeByCustomerSupplement {
		t.Fatalf("resume path = %q, want CUSTOMER_SUPPLEMENT; the task would wait on a retry nobody can make succeed", path)
	}
}

// Covers: CONTEXT 接受判断任务「全部所需权威结果已经到齐通过、但适用规则要求的人工复核尚未
// 完成 → 等待人工复核」— 复核推不动于内部重试，也补不出于客户，所以它是第三条路径。
func TestAPendingManualReviewWaitsOnTheReviewerNotAnInternalRetry(t *testing.T) {
	fixture := newDecisionFixture(t)
	fixture.commercial.manualReview = domain.ManualReviewRequiredByRules

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AcceptanceUndecided {
		t.Fatalf("outcome = %q; an incomplete review formed a lifecycle decision", result.Outcome())
	}
	if result.PendingReason() != application.ManualReviewPending {
		t.Fatalf(
			"pending reason = %q, want MANUAL_REVIEW_PENDING; an internal retry never completes a human review",
			result.PendingReason(),
		)
	}
	if len(fixture.recorder.recordedAttempts) != 1 {
		t.Fatalf("recorded %d attempts, want exactly 1", len(fixture.recorder.recordedAttempts))
	}
	if path := fixture.recorder.recordedAttempts[0].ResumePath(); path != domain.ResumeByManualReview {
		t.Fatalf("resume path = %q, want MANUAL_REVIEW; the task would retry something only a reviewer can advance", path)
	}
}

// Covers: ADR-0086 Decision 一前半——`等待人工复核`在交回之前先把带等待态的聚合落库，
// 且委托仍是`已提交`。三个等待态里只有它落库：它的续办方是第三方，重投推不动（预算烧穿后
// 信封停投，完成的复核再也没有投递来续办），不落库则队列读面结构上列不出「等复核的都有谁」。
// 排队的委托与没人管的委托不能在库里长同一张脸。
func TestAPendingManualReviewPersistsThePauseItReports(t *testing.T) {
	fixture := newDecisionFixture(t)
	fixture.commercial.manualReview = domain.ManualReviewRequiredByRules

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.PendingReason() != application.ManualReviewPending {
		t.Fatalf("pending reason = %q, want MANUAL_REVIEW_PENDING", result.PendingReason())
	}
	if fixture.requests.saved == nil {
		t.Fatal("暂停没落库就交回了`等待人工复核`——消费门会据此提交入账，队列从此列不出这份委托")
	}
	waiting, present := fixture.requests.saved.AcceptanceDecisionTask().WaitingOn()
	if !present || waiting != domain.ResumeByManualReview {
		t.Fatalf("saved waitingOn = (%v, %v), want MANUAL_REVIEW——落库的聚合没带等待态，投影列会写 0", waiting, present)
	}
	if fixture.requests.saved.State() != domain.ShipmentRequestSubmitted {
		t.Fatalf("saved state = %q, want SUBMITTED——暂停不是决定，生命周期不得离开已提交", fixture.requests.saved.State())
	}
}

// Covers: ADR-0086 Decision 一后半——暂停没落库时**不得**交回`等待人工复核`。那个原因是
// 消费门提交入账的凭据：暂停没写进去就交它，等待态随本轮回滚蒸发且投递已被记为完毕，这份
// 委托从此既不在队列里也不再有重投。改交保存那一格自己的原因，消费门照旧回滚重投。
func TestAPauseThatFailedToPersistDoesNotReportManualReviewPending(t *testing.T) {
	unreachable := newDecisionFixture(t)
	unreachable.commercial.manualReview = domain.ManualReviewRequiredByRules
	unreachable.requests.err = errors.New("storage unreachable")

	result, err := unreachable.handler.Handle(context.Background(), unreachable.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.PendingReason() == application.ManualReviewPending {
		t.Fatal("保存失败仍交回`等待人工复核`——消费门会提交一份没落库的暂停")
	}
	if result.PendingReason() != application.DecisionNotRecorded {
		t.Fatalf("pending reason = %q, want DECISION_NOT_RECORDED", result.PendingReason())
	}

	outraced := newDecisionFixture(t)
	outraced.commercial.manualReview = domain.ManualReviewRequiredByRules
	outraced.requests.conflict = true

	lost, err := outraced.handler.Handle(context.Background(), outraced.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if lost.PendingReason() == application.ManualReviewPending {
		t.Fatal("版本冲突仍交回`等待人工复核`——抢先那一方写了什么本方并不知道")
	}
	if lost.PendingReason() != application.StaleShipmentRequestRevision {
		t.Fatalf("pending reason = %q, want STALE_SHIPMENT_REQUEST_REVISION", lost.PendingReason())
	}
}

// Covers: ADR-0106 Decision 二前半——`等待受控补充`在交回之前先把带等待态的聚合落库，且委托
// 仍是`已提交`。它的续办方是客户，本进程重试推不动；不落库则「等客户补件的都有谁」这个队列
// 在库里结构上恒空——票 first-tenant-runway/09 第三问答出的正是这一格。
func TestAPendingCustomerSupplementPersistsThePauseItReports(t *testing.T) {
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
		t.Fatal("暂停没落库就交回了`等待受控补充`——消费门会据此提交入账，队列从此列不出这份委托")
	}
	waiting, present := fixture.requests.saved.AcceptanceDecisionTask().WaitingOn()
	if !present || waiting != domain.ResumeByCustomerSupplement {
		t.Fatalf("saved waitingOn = (%v, %v), want CUSTOMER_SUPPLEMENT——落库的聚合没带等待态，投影列会写 0", waiting, present)
	}
	if fixture.requests.saved.State() != domain.ShipmentRequestSubmitted {
		t.Fatalf("saved state = %q, want SUBMITTED——暂停不是决定，生命周期不得离开已提交", fixture.requests.saved.State())
	}
}

// Covers: ADR-0106 Decision 二后半（ADR-0086 Decision 一的护栏原样扩用）——暂停没落库时**不得**
// 交回`等待受控补充`，改交保存那一格自己的原因，消费门照旧回滚重投。
func TestAPauseThatFailedToPersistDoesNotReportCustomerSupplementPending(t *testing.T) {
	unreachable := newDecisionFixture(t)
	unreachable.judgments.reachability["parcel-2"] = domain.ReachabilityInsufficientEvidence
	unreachable.requests.err = errors.New("storage unreachable")

	result, err := unreachable.handler.Handle(context.Background(), unreachable.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.PendingReason() == application.CustomerSupplementPending {
		t.Fatal("保存失败仍交回`等待受控补充`——消费门会提交一份没落库的暂停")
	}
	if result.PendingReason() != application.DecisionNotRecorded {
		t.Fatalf("pending reason = %q, want DECISION_NOT_RECORDED", result.PendingReason())
	}

	outraced := newDecisionFixture(t)
	outraced.judgments.reachability["parcel-2"] = domain.ReachabilityInsufficientEvidence
	outraced.requests.conflict = true

	lost, err := outraced.handler.Handle(context.Background(), outraced.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if lost.PendingReason() == application.CustomerSupplementPending {
		t.Fatal("版本冲突仍交回`等待受控补充`——抢先那一方写了什么本方并不知道")
	}
	if lost.PendingReason() != application.StaleShipmentRequestRevision {
		t.Fatalf("pending reason = %q, want STALE_SHIPMENT_REQUEST_REVISION", lost.PendingReason())
	}
}

// Covers: ADR-0031「版本冲突自占一格未决原因，续办路径仍是内部重试」。
//
// 尾段那次比对是本用例真正承重的地方。`版本冲突`与`决定没落库`若共用一个原因，两者会派生出
// 同一条续办引用——而它们的运维含义相反：一个是库坏了要去查，一个是正常竞争等下一轮重放。
// 续办方按引用查回来的会是另一种缺口，而这件事不会有任何东西变红。
func TestASaveLostToAConcurrentWriterIsItsOwnPendingReason(t *testing.T) {
	fixture := newDecisionFixture(t)
	fixture.requests.conflict = true

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AcceptanceUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
	}
	if result.PendingReason() != application.StaleShipmentRequestRevision {
		t.Fatalf("pending reason = %q, want STALE_SHIPMENT_REQUEST_REVISION", result.PendingReason())
	}
	if _, present := result.AcceptanceDecision(); present {
		t.Fatal("一次没能越过提交边界的接受被报成已形成；下游会按一份查不回来的接受基线继续办")
	}
	if fixture.requests.saved != nil {
		t.Fatal("输给并发写入的这一轮仍然存下了聚合")
	}
	if len(fixture.recorder.recordedAttempts) != 1 {
		t.Fatalf("recorded %d attempts, want exactly 1", len(fixture.recorder.recordedAttempts))
	}
	// 恢复动作是重读再重放，而本编排以 FindBySourceIdentity 开头，因此内部续办重入天然就
	// 重读了一遍。客户与复核角色都补不出一份被别人抢先写掉的版本。
	if path := fixture.recorder.recordedAttempts[0].ResumePath(); path != domain.ResumeByInternalRetry {
		t.Fatalf("resume path = %q, want INTERNAL_RETRY", path)
	}

	unreachable := newDecisionFixture(t)
	unreachable.requests.err = errors.New("storage unreachable")
	notRecorded, err := unreachable.handler.Handle(context.Background(), unreachable.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if notRecorded.PendingReason() != application.DecisionNotRecorded {
		t.Fatalf("pending reason = %q, want DECISION_NOT_RECORDED", notRecorded.PendingReason())
	}
	if result.ContinuationReference() == notRecorded.ContinuationReference() {
		t.Fatal("版本冲突与决定没落库派生出了同一条续办引用；续办方按引用查回来的会是另一种缺口")
	}
}

// Covers: UC-PS-001 AT-PS-035「接受已经成立…不重复控制或释放合法冻结」— 接受成立时那笔冻结
// 是合法的，转接受后流程，不能在这里放掉。
func TestAnAcceptanceDoesNotReleaseTheFreeze(t *testing.T) {
	fixture := newDecisionFixture(t)

	if _, err := fixture.handler.Handle(context.Background(), fixture.command(t)); err != nil {
		t.Fatalf("handle: %v", err)
	}

	if fixture.release.calls != 0 {
		t.Fatalf("release calls = %d; an accepted request had its lawful freeze released", fixture.release.calls)
	}
}

// Covers: UC-PS-001 AT-PS-035「释放失败…保持补偿未决」与 CONTEXT「撤回提交和释放属于可补偿
// 编排」— 释放失败不回滚已经越过提交边界的拒绝，只把补偿留成可续办。
func TestAFailedReleaseKeepsTheRejectionAndLeavesCompensationPending(t *testing.T) {
	fixture := newDecisionFixture(t)
	fixture.judgments.reachability["parcel-2"] = domain.ReachabilityUnreachable
	fixture.release.err = errors.New("settlement authority unavailable")

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.State() != domain.ShipmentRequestRejected {
		t.Fatalf("state = %q; a failed release rolled back a rejection that already crossed the boundary", result.State())
	}
	if result.CompensationReference().String() == "" {
		t.Fatal("a failed release left no continuation to resume the compensation")
	}
}

// Covers: AT-PC-020「同一范围没有适用合同 → 返回无适用依据，不由 PC 形成委托拒绝」与
// UC-PS-001 接受条件矩阵前三行「确定性不通过时按适用规则拒绝」— 拒绝由本上下文形成。
//
// 压成未决会让一个根本没有适用合同的客户永远等下去：未决的含义是「还没判出来」，而这里
// 权威已经把话说完了。
func TestNoApplicableCommercialBasisRejectsRatherThanStalling(t *testing.T) {
	fixture := newDecisionFixture(t)
	fixture.commercial.applicability = domain.CommerciallyNotApplicable

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.State() != domain.ShipmentRequestRejected {
		t.Fatalf("state = %q; a determinate commercial answer was parked as undecided", result.State())
	}
	decision, present := result.AcceptanceDecision()
	if !present {
		t.Fatal("a rejection was reported without a decision")
	}
	if len(decision.FailedChecks()) == 0 {
		t.Fatal("the rejection recorded no failing check")
	}
	for _, check := range decision.FailedChecks() {
		if check.Reason().String() == "" {
			t.Fatalf("failing check %q carries no structured reason", check.Group())
		}
	}
}

// Covers: AT-PC-027「权威读取超时 → 返回解析未决，不冒充无适用依据」的镜像 — 解析未决
// 仍然保持未决。它与上一条走同一个入口，区别只在权威有没有把话说完。
func TestAnUndeterminedCommercialResolutionStaysUndecided(t *testing.T) {
	fixture := newDecisionFixture(t)
	fixture.commercial.applicability = domain.CommercialApplicabilityUndetermined

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AcceptanceUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
	}
	if result.State() == domain.ShipmentRequestRejected {
		t.Fatal("an unfinished resolution was written into a rejection")
	}
}

// Covers: AT-PC-026「解析后合同被当前修订替代」与 AT-PS-035 — 控制先前已经形成、随后商业
// 依据不再适用时，拒绝仍要按原关联解除那笔冻结。资金不会因为解析结论变了就自己回来。
func TestARejectionOnLostBasisStillReleasesAnExistingFreeze(t *testing.T) {
	fixture := newDecisionFixture(t)
	fixture.commercial.applicability = domain.CommerciallyNotApplicable
	fixture.judgments.controlOutcome = domain.FinancialControlHeld

	if _, err := fixture.handler.Handle(context.Background(), fixture.command(t)); err != nil {
		t.Fatalf("handle: %v", err)
	}

	if fixture.release.calls != 1 {
		t.Fatalf("release calls = %d; a rejection left the funds frozen", fixture.release.calls)
	}
}

// Covers: UC-PC-002 步骤 8「业务决定提交前校验解析和关键结果仍相容」与 AT-PC-026 —— 判断
// 已经在某次解析下形成过时，提交决定前走的必须是按那一份重解，而不是再解析一次。
//
// 再解析一次拿回的是决定时刻的新依据，与它自己比永远相容，提交前失效那个窗口就永远抓不到，
// 而判断的时点策略来自旧依据。所以这里既断言走了重解，也断言带出去的是记下的那个标识。
func TestADecisionRevalidatesTheBasisItsJudgmentsWereFormedUnder(t *testing.T) {
	fixture := newDecisionFixture(t)

	if _, err := fixture.handler.Handle(context.Background(), fixture.command(t)); err != nil {
		t.Fatalf("handle: %v", err)
	}

	if fixture.commercial.revalidateCalls != 1 {
		t.Fatalf("revalidate calls = %d, want 1——提交决定前重新解析了一次，而不是按原解析重解", fixture.commercial.revalidateCalls)
	}
	adopted := mustValue(t, domain.NewCommercialResolutionID, "RES-1")
	if fixture.commercial.lastRevalidation.Resolution != adopted {
		t.Fatalf("revalidated %q, want the adopted %q", fixture.commercial.lastRevalidation.Resolution, adopted)
	}
}

// Covers: `AT-PS-013` 的意图半边——已成立的决定交出恰好一份由决定标识认领的发布意图，
// 交付成功不留发布续办引用。State 随意图一起交出，下游据以分流而不必回读。
func TestAFormedDecisionHandsOffOneIntentClaimedByTheDecision(t *testing.T) {
	fixture := newDecisionFixture(t)

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	decision, present := result.AcceptanceDecision()
	if !present {
		t.Fatalf("no decision formed: %#v", result)
	}
	if len(fixture.downstream.intents) != 1 {
		t.Fatalf("handed off %d intents, want exactly one", len(fixture.downstream.intents))
	}
	intent := fixture.downstream.intents[0]
	if intent.DecisionID != decision.DecisionID() {
		t.Fatalf("intent claims %q, want the decision %q", intent.DecisionID, decision.DecisionID())
	}
	if intent.State != domain.ShipmentRequestAccepted {
		t.Fatalf("intent state = %q, want ACCEPTED", intent.State)
	}
	if result.DecisionHandoffReference().String() != "" {
		t.Fatal("交付成功仍留下了发布续办引用——调用方会去重放一件已经办完的事")
	}
}

// Covers: `AT-PS-013`「接受决定提交成功但事件首次投递失败 → 委托仍为已接受且基线、预计
// 承诺完整；重试同一发布意图，不重复接受或回退决定」——失败不改写业务结果，发布续办与
// 判断续办分开交回。
func TestAnUndeliveredDecisionHandoffKeepsTheAcceptanceWithAResumableIntent(t *testing.T) {
	fixture := newDecisionFixture(t)
	fixture.downstream.err = errors.New("downstream unreachable")

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AcceptanceDecided {
		t.Fatalf("outcome = %q, want DECIDED——投递失败改写了业务结果", result.Outcome())
	}
	if result.State() != domain.ShipmentRequestAccepted {
		t.Fatalf("state = %q, want ACCEPTED", result.State())
	}
	if fixture.requests.saved == nil || fixture.requests.saved.State() != domain.ShipmentRequestAccepted {
		t.Fatal("投递失败回退了已落库的接受决定")
	}
	if _, present := fixture.requests.saved.AcceptanceBaseline(); !present {
		t.Fatal("接受基线没有保住")
	}
	if _, present := fixture.requests.saved.ExpectedCommitment(); !present {
		t.Fatal("预计承诺没有保住")
	}
	if result.DecisionHandoffReference().String() == "" {
		t.Fatal("首次投递失败没留下发布续办引用；没有引用，这份意图不会有人再交一次")
	}
	if result.ContinuationReference().String() != "" {
		t.Fatal("投递失败混进了判断续办——决定已经形成，没有什么要重判")
	}
}

// Covers: `AT-PS-013`「重试同一发布意图，不重复接受或回退决定」——重放走已有决定路径，
// 重发同一决定标识认领的意图，不再签发新决定身份。两次交出的标识不同就说明重放又形成了
// 一份决定，那是第二份意图而不是同一份的重试。
func TestAReplayResendsTheSameDecisionIntentWithoutDecidingAgain(t *testing.T) {
	fixture := newDecisionFixture(t)
	fixture.requests.sticky = true
	fixture.downstream.err = errors.New("downstream unreachable")

	if _, err := fixture.handler.Handle(context.Background(), fixture.command(t)); err != nil {
		t.Fatalf("first handle: %v", err)
	}
	second, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("second handle: %v", err)
	}

	if second.Outcome() != application.AcceptanceDecided {
		t.Fatalf("second outcome = %q, want DECIDED", second.Outcome())
	}
	if len(fixture.downstream.intents) != 2 {
		t.Fatalf("handed off %d intents, want the retry to resend exactly once more", len(fixture.downstream.intents))
	}
	if fixture.downstream.intents[0].DecisionID != fixture.downstream.intents[1].DecisionID {
		t.Fatal("重放交出了另一份决定标识——那是第二份意图，不是同一份的重试")
	}
	if fixture.identities.issued != 1 {
		t.Fatalf("issued %d decision IDs, want 1——重放又签发了决定身份", fixture.identities.issued)
	}
	if second.DecisionHandoffReference().String() == "" {
		t.Fatal("重试仍未交出，发布续办引用不该消失")
	}
}

// Covers: AT-PC-025「解析后合同退役，但委托接受已提交 → 历史决定保留原快照，不追溯改写」。
//
// 与 AT-PC-026 的分野在是否已经越过提交边界：提交前依据失效要重解（或未决）；提交后同一
// 失效信号不得把已形成的接受改写成未决，也不得用新解析覆盖当时快照。Find 必须 sticky——
// 夹具默认恒交回崭新`已提交`，会把「已有决定」这条路径整个藏掉。
func TestAnAcceptedDecisionSurvivesRetiredBasisOnReplay(t *testing.T) {
	fixture := newDecisionFixture(t)
	fixture.requests.sticky = true

	first, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("first handle: %v", err)
	}
	firstDecision, present := first.AcceptanceDecision()
	if !present || !firstDecision.Accepted() {
		t.Fatalf("first decision = %#v present = %v; 本用例要的是接受已提交之后的回放", firstDecision, present)
	}
	originalBasis := firstDecision.Basis().ResolutionID()
	revalidateBeforeReplay := fixture.commercial.revalidateCalls

	fixture.commercial.revalidationOutcome = ports.CommercialBasisSuperseded

	second, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("second handle: %v", err)
	}
	if second.Outcome() != application.AcceptanceDecided {
		t.Fatalf("second outcome = %q pending = %q; AT-PC-025 要求已提交的接受交回原决定，不因合同退役改写/抹掉",
			second.Outcome(), second.PendingReason())
	}
	secondDecision, present := second.AcceptanceDecision()
	if !present || !secondDecision.Accepted() {
		t.Fatalf("second decision = %#v present = %v", secondDecision, present)
	}
	if secondDecision.Basis().ResolutionID() != originalBasis {
		t.Fatalf("replayed basis = %q, want original %q——追溯改写了历史快照",
			secondDecision.Basis().ResolutionID(), originalBasis)
	}
	if fixture.commercial.revalidateCalls != revalidateBeforeReplay {
		t.Fatalf("revalidate calls grew from %d to %d——接受已提交后仍去重校验，等于拿退役后的视图审历史决定",
			revalidateBeforeReplay, fixture.commercial.revalidateCalls)
	}
}

// Covers: UC-PC-002 结果语义`已失效`「重新解析；不能继续使用或覆盖原历史」与 AT-PC-026 ——
// 原解析被推翻时要回第一阶段重解，并把新解析记为所采用的那一份。
//
// 不换掉所记标识，下一轮又会拿同一个失效标识去重校验，永远得到`已失效`——那是一个死循环，
// 而它看起来只是「一直未决」。所以这里既断言重解发生了，也断言标识被换掉了。
func TestASupersededBasisIsResolvedAgainInsteadOfRetried(t *testing.T) {
	fixture := newDecisionFixture(t)
	fixture.commercial.revalidationOutcome = ports.CommercialBasisSuperseded

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if fixture.commercial.calls == 0 {
		t.Fatal("原解析已失效却没有回第一阶段重解——重试同一次重校验只会一直失效")
	}
	if len(fixture.recorder.adoptedResolution) != 1 {
		t.Fatalf("recorded %d adopted resolutions, want the re-resolved one——标识没换掉，下一轮还会拿失效的那个去比",
			len(fixture.recorder.adoptedResolution))
	}
	if result.PendingReason() != application.CommercialBasisSuperseded {
		t.Fatalf("pending reason = %q, want COMMERCIAL_BASIS_SUPERSEDED", result.PendingReason())
	}
	if _, present := result.AcceptanceDecision(); present {
		t.Fatal("判断是在旧依据下形成的，却拿它们配新依据作出了一次决定")
	}
}

// Covers: UC-PC-002 结果语义`依据未解析`「消费方回指的解析标识不指向一份它可用的原解析 →
// 回第一阶段重新解析；不重试同一次调用」，以及该用例交接「消费者端口必须返回结构化的……
// 依据未解析和已失效结果」。
//
// 恢复动作与`已失效`相同，所以走同一条重解并换标识的路——不换标识就是死循环。但未决原因
// 必须分开：`已失效`是一桩能拿去跟客户解释的商业事实，这一格却意味着本方记下的采用标识本身
// 可疑（写坏、串号，或指向了别人的解析）。并进`已失效`，本方的记录缺陷就会计进失效统计，
// 而那份统计要答的是商业修订有多频繁。
func TestAnUnrecognisedAdoptedResolutionResolvesAgainUnderItsOwnReason(t *testing.T) {
	fixture := newDecisionFixture(t)
	fixture.commercial.revalidationOutcome = ports.CommercialRevalidationBasisNotResolved

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if fixture.commercial.calls == 0 {
		t.Fatal("权威不认这个标识却没有回第一阶段重解——重试同一次重校验只会一直不认")
	}
	if len(fixture.recorder.adoptedResolution) != 1 {
		t.Fatalf("recorded %d adopted resolutions, want the re-resolved one——标识没换掉，下一轮还会拿它去比",
			len(fixture.recorder.adoptedResolution))
	}
	if result.PendingReason() != application.CommercialRevalidationBasisNotResolved {
		t.Fatalf("pending reason = %q, want COMMERCIAL_REVALIDATION_BASIS_NOT_RESOLVED", result.PendingReason())
	}
	if result.PendingReason() == application.CommercialBasisSuperseded {
		t.Fatal("本方记坏一个标识被报成了商业依据失效")
	}
	if _, present := result.AcceptanceDecision(); present {
		t.Fatal("原解析都不被承认，却据它作出了一次决定")
	}
}

// Covers: UC-PC-002 结果语义`输入未受理`「最小租户、客户、范围或锚点身份无法建立 → 不查询
// 或泄露候选商业对象」，以及 ADR-0029「`输入未受理`保留给短路支，不并入取回失败那两格」。
//
// 这一格与上一条的分野在动作而不只在措辞：短路支是本方连身份都立不起来，回第一阶段解析要拿
// 同一个立不起来的身份去问，只会再停一轮。所以它不重解，也绝不改写所记标识——那份标识没有
// 任何证据表明它坏了。
func TestARevalidationRejectedBeforeAnyQueryDoesNotResolveAgain(t *testing.T) {
	fixture := newDecisionFixture(t)
	fixture.commercial.revalidationOutcome = ports.CommercialRevalidationInputNotAccepted

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if fixture.commercial.calls != 0 {
		t.Fatal("身份都立不起来却回第一阶段重解了——同一个身份在那边一样立不起来")
	}
	if len(fixture.recorder.adoptedResolution) != 0 {
		t.Fatal("查询根本没发生，所采用的解析却被改写了")
	}
	if result.PendingReason() != application.CommercialRevalidationInputNotAccepted {
		t.Fatalf("pending reason = %q, want COMMERCIAL_REVALIDATION_INPUT_NOT_ACCEPTED", result.PendingReason())
	}
	if _, present := result.AcceptanceDecision(); present {
		t.Fatal("重校验被短路拒绝，却仍然作出了一次决定")
	}
}

// Covers: ADR-0025「翻译必须是全函数」— 第三阶段每个封闭取值都要有明确落点，而不同取值不得
// 落在同一个未决原因上。
//
// 三种停法的恢复动作都由本方发起，因此最容易被压成一格。压了之后续办引用也一样，调用方按
// 引用查回来的是另一种缺口，而未决统计再也分不出「商业修订推翻了依据」与「我们记坏了标识」。
func TestEachRevalidationOutcomeStallsUnderItsOwnReason(t *testing.T) {
	cases := map[ports.CommercialRevalidationOutcome]application.JudgmentPendingReason{
		ports.CommercialBasisSuperseded:              application.CommercialBasisSuperseded,
		ports.CommercialRevalidationUndetermined:     application.CommercialBasisUndetermined,
		ports.CommercialRevalidationBasisNotResolved: application.CommercialRevalidationBasisNotResolved,
		ports.CommercialRevalidationInputNotAccepted: application.CommercialRevalidationInputNotAccepted,
	}

	seen := make(map[application.JudgmentPendingReason]ports.CommercialRevalidationOutcome, len(cases))
	for outcome, want := range cases {
		t.Run(outcome.String(), func(t *testing.T) {
			fixture := newDecisionFixture(t)
			fixture.commercial.revalidationOutcome = outcome

			result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			if result.PendingReason() != want {
				t.Fatalf("pending reason = %q, want %q", result.PendingReason(), want)
			}
		})
		if first, duplicated := seen[want]; duplicated {
			t.Errorf("%q 与 %q 落在同一个未决原因 %q 上；两者的续办引用会完全相同",
				outcome, first, want)
		}
		seen[want] = outcome
	}
}

// Covers: UC-PC-002 一致性一节「缓存过期、读取失败或修订无法确认只能形成解析未决」——
// 重校验时权威读不到不是失效。当成失效会去重解，而重解可能选中另一份依据，等于用一次读取
// 失败换掉了原依据。
func TestAnUnreadableAuthorityDuringRevalidationDoesNotReplaceTheBasis(t *testing.T) {
	fixture := newDecisionFixture(t)
	fixture.commercial.revalidationOutcome = ports.CommercialRevalidationUndetermined

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if fixture.commercial.calls != 0 {
		t.Fatal("一次读取失败触发了重解——原依据可能因此被换掉")
	}
	if len(fixture.recorder.adoptedResolution) != 0 {
		t.Fatal("权威读不到却改写了所采用的解析")
	}
	if result.PendingReason() != application.CommercialBasisUndetermined {
		t.Fatalf("pending reason = %q, want COMMERCIAL_BASIS_UNDETERMINED", result.PendingReason())
	}
}

// Covers: UC-PC-002 结果语义`无适用依据`「由消费方按自身规则判断拒绝」—— 还没有任何一轮
// 采用过依据时走首次解析，而不是停下等一个不存在的原解析。
//
// 停下会让一个「权威确定这个范围没有适用合同」的委托从被拒绝变成永远挂着，而那正是本步该
// 给出结论的场合。
func TestADecisionWithoutAPriorResolutionStillResolves(t *testing.T) {
	fixture := newDecisionFixture(t)
	fixture.judgments.noAdoptedResolution = true

	if _, err := fixture.handler.Handle(context.Background(), fixture.command(t)); err != nil {
		t.Fatalf("handle: %v", err)
	}

	if fixture.commercial.revalidateCalls != 0 {
		t.Fatal("没有原解析可比对却仍去重解")
	}
	if fixture.commercial.calls == 0 {
		t.Fatal("既不重解也不解析，这份委托就此挂住")
	}
}

type decisionFixture struct {
	handler      *application.FormAcceptanceDecisionHandler
	commercial   *commercialBasisDouble
	reachability *reachabilityRevalidatorDouble
	judgments    *recordedJudgmentsDouble
	requests     *decidableRequestStore
	release      *controlReleaseDouble
	recorder     *judgmentRequestStore
	downstream   *decisionHandoffDouble
	identities   *decisionIdentityFactory
}

func newDecisionFixture(t *testing.T) *decisionFixture {
	t.Helper()
	value := &decisionFixture{}
	value.commercial = &commercialBasisDouble{
		t:                            t,
		applicability:                domain.CommerciallyApplicable,
		declaresReachabilityAsOf:     true,
		declaresFinancialControlAsOf: true,
		manualReview:                 domain.ManualReviewNotRequiredByRules,
		record:                       func(string) {},
	}
	value.judgments = &recordedJudgmentsDouble{
		t: t,
		reachability: map[string]domain.ReachabilityValue{
			"parcel-1": domain.ReachabilityReachable,
			"parcel-2": domain.ReachabilityReachable,
		},
		controlOutcome: domain.FinancialControlHeld,
	}
	value.requests = &decidableRequestStore{t: t}
	value.release = &controlReleaseDouble{}
	value.recorder = &judgmentRequestStore{}
	value.downstream = &decisionHandoffDouble{}
	value.identities = &decisionIdentityFactory{t: t}
	value.reachability = &reachabilityRevalidatorDouble{outcome: ports.ReachabilityJudgmentStillCurrent}
	value.handler = application.NewFormAcceptanceDecisionHandler(application.FormAcceptanceDecisionDeps{
		Requests:     value.requests,
		Commercial:   value.commercial,
		Reachability: value.reachability,
		Judgments:    value.judgments,
		Recorder:     value.recorder,
		Release:      value.release,
		Downstream:   value.downstream,
		Identities:   value.identities,
		Clock:        fixedClock{at: handlerClockAt},
	})
	return value
}

// reachabilityRevalidatorDouble 缺省答`仍然当前`：重校是决定路上的新门，既有的接受/拒绝
// 用例要能原样通过。留住查询，因为「问的是那一份判断的时点」只有在实际入参上验得出来。
type reachabilityRevalidatorDouble struct {
	outcome ports.ReachabilityRevalidationOutcome
	err     error
	asked   []ports.ReachabilityRevalidationQuery
}

func (double *reachabilityRevalidatorDouble) RevalidateReachabilityJudgment(
	_ context.Context,
	query ports.ReachabilityRevalidationQuery,
) (ports.ReachabilityRevalidation, error) {
	double.asked = append(double.asked, query)
	if double.err != nil {
		return ports.ReachabilityRevalidation{}, double.err
	}
	return ports.ReachabilityRevalidation{Outcome: double.outcome}, nil
}

// decisionHandoffDouble 留住每一份交出的意图。计数与标识都要：意图由决定标识认领，
// 「重试同一份」与「第二份」只有标识分得开（AT-PS-013）。
type decisionHandoffDouble struct {
	err     error
	intents []ports.AcceptanceDecisionHandoffIntent
}

func (double *decisionHandoffDouble) HandOffAcceptanceDecision(
	_ context.Context,
	intent ports.AcceptanceDecisionHandoffIntent,
) error {
	double.intents = append(double.intents, intent)
	return double.err
}

// controlReleaseDouble 留住释放请求所携带的原控制关联。断言这个而不是"调用过就行"，是因为
// 释放错一笔冻结与不释放同样糟——货主的另一份委托会被无故解冻。
type controlReleaseDouble struct {
	calls           int
	controlResultID string
	err             error
}

func (double *controlReleaseDouble) ReleasePreAcceptanceControl(
	_ context.Context,
	request ports.ControlReleaseRequest,
) error {
	double.calls++
	double.controlResultID = request.ControlResultID.String()
	return double.err
}

func (value *decisionFixture) command(t *testing.T) application.FormAcceptanceDecisionCommand {
	t.Helper()
	return application.FormAcceptanceDecisionCommand{
		Identity:          sourceIdentity(t, "tenant-1", "customer-1", "source-a", "key-1"),
		ShipmentRequestID: mustValue(t, domain.NewShipmentRequestID, "request-1"),
		SubmissionVersion: mustValue(t, domain.NewSubmissionVersionID, "version-1"),
	}
}

// recordedJudgmentsDouble 用 controlOutcome 的零值表示「控制从未形成」，与领域侧同一约定：
// 另设一个布尔会让两处可以互相矛盾。
type recordedJudgmentsDouble struct {
	t              *testing.T
	reachability   map[string]domain.ReachabilityValue
	controlOutcome domain.FinancialControlOutcome
	err            error
	// noAdoptedResolution 表示还没有任何一轮采用过商业依据。默认相反，因为多数用例是在
	// 判断推进过之后才形成决定的。
	noAdoptedResolution bool
	// loadedTenants 收下每次读取时编排给出的租户，供断言「租户确实传下去了」。
	loadedTenants []domain.TenantID
}

func (double *recordedJudgmentsDouble) LoadRecordedJudgments(
	_ context.Context,
	tenant domain.TenantID,
	_ domain.ShipmentRequestID,
) (ports.RecordedJudgments, error) {
	double.loadedTenants = append(double.loadedTenants, tenant)
	double.t.Helper()
	if double.err != nil {
		return ports.RecordedJudgments{}, double.err
	}

	recorded := ports.RecordedJudgments{}
	if !double.noAdoptedResolution {
		recorded.AdoptedCommercialResolution = mustValue(double.t, domain.NewCommercialResolutionID, "RES-1")
	}
	for _, parcel := range []string{"parcel-1", "parcel-2"} {
		value, present := double.reachability[parcel]
		if !present {
			continue
		}
		spec := domain.ReachabilityJudgmentSpec{
			JudgmentID: mustValue(double.t, domain.NewReachabilityJudgmentID, "NRJ-"+parcel),
			ParcelID:   mustValue(double.t, domain.NewDeclaredParcelID, parcel),
			Value:      value,
			AsOf:       formedAsOfFor(double.t, domain.ReachabilityJudgmentKind, policyFormedAsOf),
		}
		if value == domain.ReachabilityNotApplicable {
			spec.JudgmentID = domain.ReachabilityJudgmentID{}
			spec.Basis = mustValue(double.t, domain.NewReachabilityBasisReference, "LABEL_ONLY_CHANNEL_SERVICE")
		}
		judgment, err := domain.NewReachabilityJudgment(spec)
		if err != nil {
			double.t.Fatalf("new reachability judgement: %v", err)
		}
		recorded.Reachability = append(recorded.Reachability, judgment)
	}

	if double.controlOutcome != domain.FinancialControlOutcomeInvalid {
		basis := domain.ControlBasisReference{}
		if double.controlOutcome != domain.FinancialControlHeld {
			basis = mustValue(double.t, domain.NewControlBasisReference, "PC-CONTROL-BASIS-1")
		}
		control, err := domain.NewFinancialControlResult(
			mustValue(double.t, domain.NewFinancialControlResultID, "SAC-1"),
			double.controlOutcome,
			basis,
			formedAsOfFor(double.t, domain.FinancialControlJudgmentKind, controlPolicyFormedAsOf),
		)
		if err != nil {
			double.t.Fatalf("new financial control result: %v", err)
		}
		recorded.FinancialControl = control
	}
	return recorded, nil
}

// decidableRequestStore 交回一份已提交、含两个声明成员的委托，并留住被决定后保存的那一份。
//
// conflict 用布尔而不是直接收一个 ports.ShipmentRequestSaveOutcome：那个枚举的零值是`未设`，
// 收它会让每一处没显式设过的构造都变成一次端口坏了。
//
// sticky 为真时 Find 交回已保存的那一份。默认关着，是因为多数用例测的是「尚未决定」窗口；
// AT-PC-025 这类回放必须打开，否则永远到不了`已有决定`。
type decidableRequestStore struct {
	t        *testing.T
	saved    *domain.ShipmentRequest
	err      error
	conflict bool
	sticky   bool
}

func (store *decidableRequestStore) FindBySourceIdentity(
	_ context.Context,
	_ domain.SourceIdentity,
) (domain.ShipmentRequest, bool, error) {
	store.t.Helper()
	if store.sticky && store.saved != nil {
		return *store.saved, true, nil
	}
	return submittedRequest(store.t), true, nil
}

func (store *decidableRequestStore) Insert(
	_ context.Context,
	_ domain.SourceIdentity,
	_ domain.ShipmentRequest,
) (ports.ShipmentRequestInsertOutcome, error) {
	return ports.ShipmentRequestInserted, nil
}

func (store *decidableRequestStore) Save(
	_ context.Context,
	_ domain.SourceIdentity,
	request domain.ShipmentRequest,
) (ports.ShipmentRequestSaveOutcome, error) {
	if store.err != nil {
		return ports.ShipmentRequestSaveOutcomeInvalid, store.err
	}
	if store.conflict {
		// 抢先那一方已经落库，本方这一份不写进去：留住它会让断言读到一份其实没落库的聚合。
		return ports.ShipmentRequestRevisionConflict, nil
	}
	store.saved = &request
	return ports.ShipmentRequestSaved, nil
}

// submittedRequest 直接经领域构造一份含两个声明成员的`已提交`委托。不走提交编排，是因为
// 本编排的被测行为从委托已经存在开始，把建单那一段拉进来只会让失败原因难定位。
func submittedRequest(t *testing.T) domain.ShipmentRequest {
	t.Helper()
	scope, err := domain.NewAdmissionScope(
		mustValue(t, domain.NewAdmissionScopeReference, "scope-ref-1"),
		mustValue(t, domain.NewAdmissionScopeDigest, "scope-1"),
	)
	if err != nil {
		t.Fatalf("new admission scope: %v", err)
	}
	decidedAt := time.Date(2026, 8, 7, 11, 0, 0, 0, time.UTC)
	decision, err := domain.NewProductionOwnershipDecision(domain.ProductionOwnershipDecisionSpec{
		DecisionID:       mustValue(t, domain.NewProductionOwnershipDecisionID, "decision-1"),
		Scope:            scope,
		Authority:        domain.ProductionAuthorityIDPParcel,
		AdmissionControl: domain.AdmissionControlOpen,
		RuleVersion:      mustValue(t, domain.NewProductionOwnershipRuleVersion, "rule-1"),
		AsOf:             decidedAt,
		Validity:         validity(t),
		Revision:         mustValue(t, domain.NewProductionOwnershipRevision, "rev-1"),
		DecisionAt:       decidedAt,
	})
	if err != nil {
		t.Fatalf("new ownership decision: %v", err)
	}
	gate, err := domain.EvaluateFutureSubmissionGate(
		decision,
		scope.Digest(),
		mustValue(t, domain.NewProductionOwnershipRevision, "rev-1"),
		decidedAt,
	)
	if err != nil {
		t.Fatalf("evaluate gate: %v", err)
	}
	candidate, err := domain.NewSubmissionCandidate(
		submittedFingerprint(t),
		mustValue(t, domain.NewSubmissionBatchID, "batch-1"),
		mustValue(t, domain.NewShipmentRequestID, "request-1"),
		[]domain.DeclaredParcelID{
			mustValue(t, domain.NewDeclaredParcelID, "parcel-1"),
			mustValue(t, domain.NewDeclaredParcelID, "parcel-2"),
		},
	)
	if err != nil {
		t.Fatalf("new submission candidate: %v", err)
	}
	request, err := domain.SubmitShipmentRequest(domain.SubmitShipmentRequestSpec{
		Candidate:   candidate,
		Gate:        gate,
		VersionID:   mustValue(t, domain.NewSubmissionVersionID, "version-1"),
		TaskID:      mustValue(t, domain.NewAcceptanceDecisionTaskID, "task-1"),
		SubmittedAt: decidedAt,
	})
	if err != nil {
		t.Fatalf("submit shipment request: %v", err)
	}
	return request
}

func submittedFingerprint(t *testing.T) domain.SourceSubmissionFingerprint {
	t.Helper()
	value, err := domain.NewSourceSubmissionFingerprint(
		sourceIdentity(t, "tenant-1", "customer-1", "source-a", "key-1"),
		mustValue(t, domain.NewPayloadDigest, "digest-1"),
		time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 7, 10, 0, 1, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("new source fingerprint: %v", err)
	}
	return value
}

// decisionIdentityFactory 数着签发次数：重放不得再签发决定身份，而「签发过几次」只有
// 计数分得开。
type decisionIdentityFactory struct {
	t      *testing.T
	issued int
}

func (factory *decisionIdentityFactory) NextAcceptanceDecisionID(
	_ context.Context,
) (domain.AcceptanceDecisionID, error) {
	factory.t.Helper()
	factory.issued++
	return mustValue(factory.t, domain.NewAcceptanceDecisionID, "decision-1"), nil
}

var (
	_ ports.ShipmentRequestRepository   = (*decidableRequestStore)(nil)
	_ ports.RecordedJudgmentReader      = (*recordedJudgmentsDouble)(nil)
	_ ports.AcceptanceDecisionIdentity  = (*decisionIdentityFactory)(nil)
	_ ports.PreAcceptanceControlRelease = (*controlReleaseDouble)(nil)
	_ ports.AcceptanceDecisionHandoff   = (*decisionHandoffDouble)(nil)
	_ ports.ReachabilityRevalidator     = (*reachabilityRevalidatorDouble)(nil)
)

// Covers: `AT-PS-037`「可达性或其他关键判断形成后、接受提交前被有效新版本或限制推翻 →
// 原结果不再用于接受并重新判断；暂时无法取得新结果时保持已提交和未决」——已换代的判断
// 停住本轮且不形成决定；查询携带那一份判断自己的成员与时点（拿本轮新时点去问，问到的就
// 是另一次判断）。提交后半边由 TestAnAcceptedDecisionSurvivesRetiredBasisOnReplay 承重。
func TestADecisionStopsWhenAReachabilityJudgmentWasSuperseded(t *testing.T) {
	fixture := newDecisionFixture(t)
	fixture.reachability.outcome = ports.ReachabilityJudgmentSuperseded

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AcceptanceUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED——被推翻的判断仍被用于接受", result.Outcome())
	}
	if result.PendingReason() != application.ReachabilityJudgmentSuperseded {
		t.Fatalf("reason = %q, want REACHABILITY_JUDGMENT_SUPERSEDED", result.PendingReason())
	}
	if _, present := result.AcceptanceDecision(); present {
		t.Fatal("判断已被推翻却仍形成了决定")
	}
	if len(fixture.reachability.asked) == 0 {
		t.Fatal("重校端口没有被问到")
	}
	asked := fixture.reachability.asked[0]
	if asked.DeclaredParcelID.String() == "" || asked.AsOf.At().IsZero() {
		t.Fatalf("query = %#v; 重校必须携带那一份判断自己的成员与时点", asked)
	}
}

// Covers: 同一 AT 的其余落点——未找回/无法判定/入参未受理/答不出各停各格，续办引用互不
// 相认（原因参与派生）。
func TestEveryReachabilityRevalidationAnswerLandsOnItsOwnPendingReason(t *testing.T) {
	cases := map[string]struct {
		arrange func(*decisionFixture)
		want    application.JudgmentPendingReason
	}{
		"judgment not found": {
			arrange: func(fixture *decisionFixture) {
				fixture.reachability.outcome = ports.ReachabilityRevalidationJudgmentNotFound
			},
			want: application.ReachabilityRevalidationJudgmentNotFound,
		},
		"undetermined": {
			arrange: func(fixture *decisionFixture) {
				fixture.reachability.outcome = ports.ReachabilityRevalidationUndetermined
			},
			want: application.ReachabilityRevalidationUndetermined,
		},
		"input not accepted": {
			arrange: func(fixture *decisionFixture) {
				fixture.reachability.outcome = ports.ReachabilityRevalidationInputNotAccepted
			},
			want: application.ReachabilityRevalidationInputNotAccepted,
		},
		"revalidator unavailable": {
			arrange: func(fixture *decisionFixture) {
				fixture.reachability.err = errors.New("revalidator down")
			},
			want: application.ReachabilityRevalidationUnavailable,
		},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newDecisionFixture(t)
			testCase.arrange(fixture)

			result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}

			if result.Outcome() != application.AcceptanceUndecided {
				t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
			}
			if result.PendingReason() != testCase.want {
				t.Fatalf("reason = %q, want %q", result.PendingReason(), testCase.want)
			}
			if result.ContinuationReference().String() == "" {
				t.Fatal("停下的重校没留续办引用")
			}
		})
	}
}
