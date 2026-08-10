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
		DecisionID:   mustValue(t, domain.NewAcceptanceDecisionID, "decision-1"),
		Checks:       checks,
		ManualReview: domain.ManualReviewNotRequired,
		Basis:        acceptanceBasis(t),
		DecidedAt:    decidedAt,
	}
}

func acceptanceBasis(t *testing.T) domain.CommercialBasisSnapshot {
	t.Helper()
	snapshot, err := domain.NewCommercialBasisSnapshot(
		mustValue(t, domain.NewCommercialResolutionID, "RES-1"),
		mustValue(t, domain.NewRulePackageReference, "rules-1/v1"),
		mustValue(t, domain.NewCommercialViewRevision, "VIEW-1"),
		nil,
	)
	if err != nil {
		t.Fatalf("new commercial basis snapshot: %v", err)
	}
	return snapshot
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
	checks = append(checks, versionCheck(t, domain.RequiredDocumentCheck, domain.CheckUndetermined, "AWAITING_CUSTOMER_DATA"))

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

// Covers: UC-PS-001 步骤 8「规则显式要求时进入人工复核」与「人工不得覆盖硬规则」。
func TestManualReviewGatesAcceptanceButCannotOverrideAFailure(t *testing.T) {
	t.Run("required and outstanding stays undecided", func(t *testing.T) {
		spec := decisionSpec(t, allGroupsPassing(t))
		spec.ManualReview = domain.ManualReviewRequired

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
		spec.ManualReview = domain.ManualReviewCompleted

		decided, err := submitted(t).Decide(spec)
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
		spec.ManualReview = domain.ManualReviewCompleted

		decided, err := submitted(t).Decide(spec)
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
		// Drop parcel-2's reachability check, leaving one member unjudged.
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
