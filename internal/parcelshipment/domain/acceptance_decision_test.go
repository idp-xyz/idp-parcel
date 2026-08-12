package domain_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

var decidedAt = time.Date(2026, 8, 7, 13, 0, 0, 0, time.UTC)

func versionCheck(t *testing.T, group domain.AcceptanceCheckGroup, outcome domain.CheckOutcome, reason string) domain.AcceptanceCheck {
	t.Helper()
	check, err := domain.NewAcceptanceCheck(group, domain.DeclaredParcelID{}, outcome, checkReason(t, reason))
	if err != nil {
		t.Fatalf("new acceptance check: %v", err)
	}
	return check
}

func parcelCheck(t *testing.T, group domain.AcceptanceCheckGroup, parcel string, outcome domain.CheckOutcome, reason string) domain.AcceptanceCheck {
	t.Helper()
	check, err := domain.NewAcceptanceCheck(group, mustValue(t, domain.NewDeclaredParcelID, parcel), outcome, checkReason(t, reason))
	if err != nil {
		t.Fatalf("new acceptance check: %v", err)
	}
	return check
}

func undeterminedCheck(
	t *testing.T,
	group domain.AcceptanceCheckGroup,
	reason string,
	resumePath domain.ResumePath,
) domain.AcceptanceCheck {
	t.Helper()
	check, err := domain.NewUndeterminedAcceptanceCheck(
		group,
		domain.DeclaredParcelID{},
		checkReason(t, reason),
		resumePath,
	)
	if err != nil {
		t.Fatalf("new undetermined acceptance check: %v", err)
	}
	return check
}

func checkReason(t *testing.T, reason string) domain.CheckReason {
	t.Helper()
	if reason == "" {
		return domain.CheckReason{}
	}
	return mustValue(t, domain.NewCheckReason, reason)
}

func allGroupsPassing(t *testing.T) []domain.AcceptanceCheck {
	t.Helper()
	groups := []domain.AcceptanceCheckGroup{
		domain.CustomerRelationshipCheck,
		domain.LegalEntityAndContractCheck,
		domain.ProductAndServiceCheck,
		domain.MemberBaselineCheck,
		domain.RequiredDocumentCheck,
		domain.PreAcceptanceFinancialControlCheck,
	}
	checks := make([]domain.AcceptanceCheck, 0, len(groups)+2)
	for _, group := range groups {
		checks = append(checks, versionCheck(t, group, domain.CheckPassed, ""))
	}
	for _, parcel := range []string{"parcel-1", "parcel-2"} {
		checks = append(checks, parcelCheck(t, domain.NetworkReachabilityCheck, parcel, domain.CheckPassed, ""))
	}
	return checks
}

func decisionSpec(t *testing.T, checks []domain.AcceptanceCheck) domain.AcceptanceDecisionSpec {
	t.Helper()
	return domain.AcceptanceDecisionSpec{
		DecisionID: mustValue(t, domain.NewAcceptanceDecisionID, "decision-1"),
		Checks:     checks,
		Basis:      acceptanceBasis(t),
		DecidedAt:  decidedAt,
	}
}

func acceptanceBasis(t *testing.T) domain.CommercialBasisSnapshot {
	t.Helper()
	return basisDeclaring(t, allApplicableGroups...)
}

var allApplicableGroups = []domain.AcceptanceCheckGroup{
	domain.CustomerRelationshipCheck,
	domain.LegalEntityAndContractCheck,
	domain.ProductAndServiceCheck,
	domain.MemberBaselineCheck,
	domain.RequiredDocumentCheck,
	domain.PreAcceptanceFinancialControlCheck,
	domain.NetworkReachabilityCheck,
}

// basisDeclaring 建一份声明了指定适用校验组的商业依据快照。适用集合是规则包的声明，因此
// 由夹具给出而不是由被测代码兜底——这正是接受路径可测而生产侧没有默认集合的原因。
func basisDeclaring(t *testing.T, groups ...domain.AcceptanceCheckGroup) domain.CommercialBasisSnapshot {
	t.Helper()
	applicable, err := domain.NewApplicableCheckGroups(groups...)
	if err != nil {
		t.Fatalf("new applicable check groups: %v", err)
	}
	return basisWithApplicable(t, applicable)
}

func basisWithApplicable(t *testing.T, applicable domain.ApplicableCheckGroups) domain.CommercialBasisSnapshot {
	t.Helper()
	return basisWithReviewPolicy(t, applicable, domain.ManualReviewNotRequiredByRules)
}

func basisRequiringReview(t *testing.T, groups ...domain.AcceptanceCheckGroup) domain.CommercialBasisSnapshot {
	t.Helper()
	applicable, err := domain.NewApplicableCheckGroups(groups...)
	if err != nil {
		t.Fatalf("new applicable check groups: %v", err)
	}
	return basisWithReviewPolicy(t, applicable, domain.ManualReviewRequiredByRules)
}

func basisWithReviewPolicy(
	t *testing.T,
	applicable domain.ApplicableCheckGroups,
	policy domain.ManualReviewPolicy,
) domain.CommercialBasisSnapshot {
	t.Helper()
	snapshot, err := domain.NewCommercialBasisSnapshot(domain.CommercialBasisSnapshotSpec{
		ResolutionID: mustValue(t, domain.NewCommercialResolutionID, "RES-1"),
		RulePackage:  mustValue(t, domain.NewRulePackageReference, "rules-1/v1"),
		ViewRevision: mustValue(t, domain.NewCommercialViewRevision, "VIEW-1"),
		Applicable:   applicable,
		ManualReview: policy,
	})
	if err != nil {
		t.Fatalf("new commercial basis snapshot: %v", err)
	}
	return snapshot
}

// Covers: 本仓对时刻值的既有约定 —— `DeclaredAsOf`、`ProcessingAttempt`、`ManualReviewCompletion`
// 与各上下文的时刻值对象都在构造期规范化到 UTC。决定时间是同一类值，而同一个聚合上的三条决定
// 路径必须一致：一份委托上的接受时间带时区、撤回时间不带，事后比对与序列化就要分两套处理。
func TestEveryDecisionPathNormalisesItsDecisionTimeToUTC(t *testing.T) {
	zone := time.FixedZone("UTC+8", 8*60*60)
	local := decidedAt.In(zone)

	assertUTC := func(t *testing.T, got time.Time) {
		t.Helper()
		if got.Location() != time.UTC {
			t.Fatalf("decided at = %v (location %v), want it normalised to UTC", got, got.Location())
		}
		if !got.Equal(decidedAt) {
			t.Fatalf("decided at = %v, want the same instant as %v", got, decidedAt)
		}
	}

	t.Run("acceptance", func(t *testing.T) {
		spec := decisionSpec(t, allGroupsPassing(t))
		spec.DecidedAt = local
		accepted, err := submitted(t).Decide(spec)
		if err != nil {
			t.Fatalf("decide: %v", err)
		}
		decision, _ := accepted.AcceptanceDecision()
		assertUTC(t, decision.DecidedAt())
	})

	t.Run("active rejection", func(t *testing.T) {
		spec := activeRejectionSpec(t)
		spec.DecidedAt = local
		rejected, err := submitted(t).RejectByAuthority(spec)
		if err != nil {
			t.Fatalf("reject by authority: %v", err)
		}
		decision, _ := rejected.AcceptanceDecision()
		assertUTC(t, decision.DecidedAt())
	})

	t.Run("customer withdrawal", func(t *testing.T) {
		spec := withdrawalSpec(t)
		spec.DecidedAt = local
		withdrawn, err := submitted(t).WithdrawByCustomer(spec)
		if err != nil {
			t.Fatalf("withdraw by customer: %v", err)
		}
		record, _ := withdrawn.Withdrawal()
		assertUTC(t, record.DecidedAt())
	})
}

func submitted(t *testing.T) domain.ShipmentRequest {
	t.Helper()
	request, err := domain.SubmitShipmentRequest(submitSpec(t, "parcel-1", "parcel-2"))
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	return request
}

// Covers: CONTEXT 已提交 → 已接受，以及 UC-PS-001 步骤 8「硬规则全部通过时自动接受」。
// 接受基线必须覆盖当前提交版本的完整声明成员，预计承诺必须引用当时的商业依据。
//
// Covers: `AT-PS-001`「多包裹资料完整且每包可达 → 整份委托被接受，成员基线和预计承诺
// 冻结」——基线全员与承诺引用当时依据正是本用例的断言。「没有虚构收寄、正式承诺、容量
// 或实际履约」半边是结构性事实：本上下文没有那些对象可造，无从用断言钉。
func TestAllChecksPassingAcceptsAndFixesBaselineAndCommitment(t *testing.T) {
	decided, err := submitted(t).Decide(decisionSpec(t, allGroupsPassing(t)))
	if err != nil {
		t.Fatalf("decide: %v", err)
	}

	if decided.State() != domain.ShipmentRequestAccepted {
		t.Fatalf("state = %q, want ACCEPTED", decided.State())
	}
	baseline, present := decided.AcceptanceBaseline()
	if !present {
		t.Fatal("an accepted request has no acceptance baseline")
	}
	if got, want := stringValues(baseline.DeclaredParcelIDs()), []string{"parcel-1", "parcel-2"}; !slices.Equal(got, want) {
		t.Fatalf("baseline covers %v, want the complete member set %v", got, want)
	}
	commitment, present := decided.ExpectedCommitment()
	if !present {
		t.Fatal("an accepted request formed no expected commitment")
	}
	if commitment.Basis().ResolutionID() != acceptanceBasis(t).ResolutionID() {
		t.Fatal("the expected commitment does not reference the basis in force at acceptance")
	}
	if !commitment.FormedAt().Equal(decidedAt) {
		t.Fatalf("commitment formed at %v, want %v", commitment.FormedAt(), decidedAt)
	}
	if decided.AcceptanceDecisionTask().IsComplete() != true {
		t.Fatal("a decided request left its acceptance task open")
	}
}

// Covers: UC-PS-001「本产品不支持成员级部分接受」— 任一成员确定性不满足时整份当前提交
// 版本不能接受，且不得形成只覆盖部分成员的基线。
//
// Covers: `AT-PS-002` 后半「第二份当前提交版本整体不能接受，且不得静默删除失败成员」——
// 拒绝记下的恰是那一条失败校验，成员没有被删去重判。前半「两份委托分别判断、不因批次
// 关系全批回滚」是结构性事实：编排按单委托 Handle，批次只归组，不存在全批回滚的路径。
func TestOneFailingMemberBlocksTheWholeSubmissionVersion(t *testing.T) {
	checks := allGroupsPassing(t)
	checks = append(checks, parcelCheck(t, domain.NetworkReachabilityCheck, "parcel-2", domain.CheckFailed, "UNREACHABLE"))

	decided, err := submitted(t).Decide(decisionSpec(t, checks))
	if err != nil {
		t.Fatalf("decide: %v", err)
	}

	if decided.State() != domain.ShipmentRequestRejected {
		t.Fatalf("state = %q, want REJECTED", decided.State())
	}
	if _, present := decided.AcceptanceBaseline(); present {
		t.Fatal("a rejected version formed an acceptance baseline")
	}
	if _, present := decided.ExpectedCommitment(); present {
		t.Fatal("a rejected version formed an expected commitment")
	}
	decision, present := decided.AcceptanceDecision()
	if !present || len(decision.FailedChecks()) != 1 {
		t.Fatalf("rejection did not record exactly the failing check: %#v", decision)
	}
}

// Covers: UC-PS-001 结果语义「尚未决定」— 判断未完成不是拒绝，委托保持已提交且任务未完成。
func TestAnUndeterminedCheckLeavesTheRequestSubmitted(t *testing.T) {
	checks := allGroupsPassing(t)
	checks = append(checks, undeterminedCheck(
		t,
		domain.RequiredDocumentCheck,
		"AWAITING_CUSTOMER_DATA",
		domain.ResumeByCustomerSupplement,
	))

	decided, err := submitted(t).Decide(decisionSpec(t, checks))
	if err != nil {
		t.Fatalf("decide: %v", err)
	}

	if decided.State() != domain.ShipmentRequestSubmitted {
		t.Fatalf("state = %q; an undetermined check formed a lifecycle decision", decided.State())
	}
	if _, present := decided.AcceptanceDecision(); present {
		t.Fatal("an undetermined pass recorded an acceptance decision")
	}
	if decided.AcceptanceDecisionTask().IsComplete() {
		t.Fatal("an undetermined pass closed the acceptance task")
	}
}

// requiringReview 交回一份规则包要求人工复核的商业依据。复核要求来自规则包声明，不由
// 调用方在决定时随手指定——那正是「人工绕过硬规则」得以发生的入口。
func requiringReview(t *testing.T) domain.CommercialBasisSnapshot {
	t.Helper()
	return basisRequiringReview(t, allApplicableGroups...)
}

// Covers: UC-PS-001 步骤 8「规则显式要求时进入人工复核」与「人工不得覆盖硬规则」。复核要求
// 取自商业依据快照、完成情况取自聚合自己的任务，两个来源都不是调用方传进来的——传进来就能
// 靠谎报一个`已完成`换取一次接受。
func TestManualReviewGatesAcceptanceButCannotOverrideAFailure(t *testing.T) {
	t.Run("required and outstanding stays undecided", func(t *testing.T) {
		spec := decisionSpec(t, allGroupsPassing(t))
		spec.Basis = requiringReview(t)

		decided, err := submitted(t).Decide(spec)
		if err != nil {
			t.Fatalf("decide: %v", err)
		}
		if decided.State() != domain.ShipmentRequestSubmitted {
			t.Fatalf("state = %q; acceptance did not wait for the required review", decided.State())
		}
	})

	t.Run("completed review accepts", func(t *testing.T) {
		spec := decisionSpec(t, allGroupsPassing(t))
		spec.Basis = requiringReview(t)
		reviewed, err := submitted(t).CompleteManualReview(reviewCompletion(t))
		if err != nil {
			t.Fatalf("complete manual review: %v", err)
		}

		decided, err := reviewed.Decide(spec)
		if err != nil {
			t.Fatalf("decide: %v", err)
		}
		if decided.State() != domain.ShipmentRequestAccepted {
			t.Fatalf("state = %q, want ACCEPTED", decided.State())
		}
	})

	t.Run("completed review cannot rescue a hard failure", func(t *testing.T) {
		checks := allGroupsPassing(t)
		checks = append(checks, versionCheck(t, domain.LegalEntityAndContractCheck, domain.CheckFailed, "CONTRACT_EXPIRED"))
		spec := decisionSpec(t, checks)
		spec.Basis = requiringReview(t)
		reviewed, err := submitted(t).CompleteManualReview(reviewCompletion(t))
		if err != nil {
			t.Fatalf("complete manual review: %v", err)
		}

		decided, err := reviewed.Decide(spec)
		if err != nil {
			t.Fatalf("decide: %v", err)
		}
		if decided.State() != domain.ShipmentRequestRejected {
			t.Fatalf("state = %q; a completed manual review overrode a hard-rule failure", decided.State())
		}
	})
}

// Covers: UC-PS-001「一份委托的同一提交版本只能形成一个当前有效接受或拒绝决定」，以及
// CONTEXT「拒绝不得原地重开为已提交」。
func TestASubmissionVersionAcceptsOnlyOneDecision(t *testing.T) {
	accepted, err := submitted(t).Decide(decisionSpec(t, allGroupsPassing(t)))
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	if _, err := accepted.Decide(decisionSpec(t, allGroupsPassing(t))); !errors.Is(err, domain.ErrDecisionAlreadyFormed) {
		t.Fatalf("error = %v, want ErrDecisionAlreadyFormed on an accepted request", err)
	}

	rejectedChecks := append(allGroupsPassing(t), versionCheck(t, domain.ProductAndServiceCheck, domain.CheckFailed, "PRODUCT_NOT_APPLICABLE"))
	rejected, err := submitted(t).Decide(decisionSpec(t, rejectedChecks))
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if _, err := rejected.Decide(decisionSpec(t, allGroupsPassing(t))); !errors.Is(err, domain.ErrDecisionAlreadyFormed) {
		t.Fatalf("error = %v; a rejected request was reopened in place", err)
	}
}

// Covers: CONTEXT「委托接受时至少包含一个客户声明包裹」，以及每个成员都必须被判断过。
func TestAcceptanceRequiresEveryDeclaredMemberToBeJudged(t *testing.T) {
	checks := make([]domain.AcceptanceCheck, 0)
	for _, check := range allGroupsPassing(t) {
		// 去掉 parcel-2 的可达性检查，留一个成员未被判断。
		if check.Group() == domain.NetworkReachabilityCheck && check.DeclaredParcelID().String() == "parcel-2" {
			continue
		}
		checks = append(checks, check)
	}

	decided, err := submitted(t).Decide(decisionSpec(t, checks))
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	if decided.State() != domain.ShipmentRequestSubmitted {
		t.Fatalf("state = %q; a member was accepted without ever being judged", decided.State())
	}
}

// Covers: UC-PS-001 接受条件「每个适用校验组都必须通过」与 PC-RULE 对适用性的所有权 —
// 规则包没有声明适用校验组时无从知道该判哪些组，因此不接受。这里不许有默认集合：拟一个
// 出来就是把合同范围的事写成了本上下文的生产默认值。
func TestAcceptanceWaitsUntilTheRulePackageDeclaresWhichGroupsApply(t *testing.T) {
	spec := decisionSpec(t, allGroupsPassing(t))
	spec.Basis = basisWithApplicable(t, domain.ApplicableCheckGroups{})

	decided, err := submitted(t).Decide(spec)
	if err != nil {
		t.Fatalf("decide: %v", err)
	}

	if decided.State() != domain.ShipmentRequestSubmitted {
		t.Fatalf("state = %q; acceptance proceeded without knowing which groups apply", decided.State())
	}
	if decided.AcceptanceDecisionTask().IsComplete() {
		t.Fatal("an undeclared applicable set closed the acceptance task")
	}
}

// Covers: UC-PS-001 接受条件「每个适用校验组都必须通过」— 已声明适用的组从未出现过校验
// 时不得接受。未被判断的组不等于通过的组，当作通过就是以遗漏方式实现的接受。
func TestAnApplicableGroupThatWasNeverJudgedBlocksAcceptance(t *testing.T) {
	checks := make([]domain.AcceptanceCheck, 0)
	for _, check := range allGroupsPassing(t) {
		if check.Group() == domain.RequiredDocumentCheck {
			continue
		}
		checks = append(checks, check)
	}

	decided, err := submitted(t).Decide(decisionSpec(t, checks))
	if err != nil {
		t.Fatalf("decide: %v", err)
	}

	if decided.State() != domain.ShipmentRequestSubmitted {
		t.Fatalf("state = %q; an applicable group was accepted without ever being judged", decided.State())
	}
}

// Covers: UC-PS-001「任一成员确定性不满足接受条件时，整份当前提交版本不能接受」— 声明
// 只能增加要求，不能消掉失败。缩小适用集合若能甩掉一个已经失败的校验，调整声明就成了
// 绕过硬规则的后门。
func TestNarrowingTheApplicableSetCannotDiscardAFailure(t *testing.T) {
	spec := decisionSpec(t, []domain.AcceptanceCheck{
		versionCheck(t, domain.PreAcceptanceFinancialControlCheck, domain.CheckPassed, ""),
		parcelCheck(t, domain.NetworkReachabilityCheck, "parcel-1", domain.CheckPassed, ""),
		parcelCheck(t, domain.NetworkReachabilityCheck, "parcel-2", domain.CheckPassed, ""),
		versionCheck(t, domain.LegalEntityAndContractCheck, domain.CheckFailed, "CONTRACT_EXPIRED"),
	})
	spec.Basis = basisDeclaring(t, domain.PreAcceptanceFinancialControlCheck, domain.NetworkReachabilityCheck)

	decided, err := submitted(t).Decide(spec)
	if err != nil {
		t.Fatalf("decide: %v", err)
	}

	if decided.State() != domain.ShipmentRequestRejected {
		t.Fatalf("state = %q; a failing check was discarded by narrowing the applicable set", decided.State())
	}
}

// Covers: PC-RULE 拥有适用性 — 一个不要求任何校验的规则包等于无条件接受，那不是规则包该
// 能表达的东西，所以空集在构造期就不成立。
func TestAnEmptyApplicableSetCannotBeDeclared(t *testing.T) {
	if _, err := domain.NewApplicableCheckGroups(); !errors.Is(err, domain.ErrInvalidApplicableCheckGroups) {
		t.Fatalf("error = %v, want ErrInvalidApplicableCheckGroups", err)
	}
}

// Covers: AT-PC-020「同一范围没有适用合同 → 返回无适用依据，不由 PC 形成委托拒绝」— 拒绝
// 由本上下文形成，而这种拒绝恰恰没有商业依据可带：要求它带一份，等于让「没有适用合同」这个
// 结论永远形成不了决定。
func TestARejectionStandsWithoutACommercialBasis(t *testing.T) {
	spec := decisionSpec(t, []domain.AcceptanceCheck{
		versionCheck(t, domain.LegalEntityAndContractCheck, domain.CheckFailed, "NO_APPLICABLE_BASIS"),
	})
	spec.Basis = domain.CommercialBasisSnapshot{}

	decided, err := submitted(t).Decide(spec)
	if err != nil {
		t.Fatalf("decide: %v", err)
	}

	if decided.State() != domain.ShipmentRequestRejected {
		t.Fatalf("state = %q, want REJECTED", decided.State())
	}
	decision, present := decided.AcceptanceDecision()
	if !present || len(decision.FailedChecks()) != 1 {
		t.Fatalf("rejection did not record the failing check: %#v", decision)
	}
}

// Covers: CONTEXT「委托接受时……固定接受基线、预计承诺以及适用的产品、合同、映射和授权
// 依据」— 接受仍然离不开商业依据。预计承诺保存的就是当时的依据快照，没有依据的接受形成
// 不了承诺，因此这条路径必须走不通。
func TestAcceptanceStillRequiresACommercialBasis(t *testing.T) {
	spec := decisionSpec(t, allGroupsPassing(t))
	spec.Basis = domain.CommercialBasisSnapshot{}

	decided, err := submitted(t).Decide(spec)
	if err != nil {
		t.Fatalf("decide: %v", err)
	}

	if decided.State() != domain.ShipmentRequestSubmitted {
		t.Fatalf("state = %q; a request was accepted without any commercial basis", decided.State())
	}
	if _, present := decided.ExpectedCommitment(); present {
		t.Fatal("an expected commitment was formed with no basis to reference")
	}
}

// Covers: UC-PS-001 拒绝语义 — 拒绝必须保存结构化原因，不得以空原因成立。
func TestAFailedCheckWithoutAReasonCannotBeBuilt(t *testing.T) {
	if _, err := domain.NewAcceptanceCheck(
		domain.RequiredDocumentCheck,
		domain.DeclaredParcelID{},
		domain.CheckFailed,
		domain.CheckReason{},
	); !errors.Is(err, domain.ErrInvalidAcceptanceCheck) {
		t.Fatalf("error = %v, want ErrInvalidAcceptanceCheck", err)
	}
}
