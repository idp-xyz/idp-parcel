package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件对真实 PostgreSQL 16 证提交条件落库往返（ADR-0376 决定一，票 declared-goods-lines/09）：依据快照上的条件随决定与预计
// 承诺一起存回；受理任务停在等待受控补充时缺的各项存回、读回逐件不丢。

func basisWithConditions(t *testing.T) domain.CommercialBasisSnapshot {
	t.Helper()
	base := acceptanceBasis(t)
	conditions, err := domain.NewSubmissionConditions(domain.PlatformShipmentDeclaredCondition, domain.EveryParcelHasPlatformBoxLabelCondition)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := domain.NewCommercialBasisSnapshot(domain.CommercialBasisSnapshotSpec{
		ResolutionID:         base.ResolutionID(),
		RulePackage:          base.RulePackage(),
		ViewRevision:         base.ViewRevision(),
		DeclaredAsOf:         base.DeclaredAsOf(),
		Applicable:           base.Applicable(),
		ManualReview:         base.ManualReviewPolicy(),
		SubmissionConditions: conditions,
	})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestSubmissionConditionsAndSupplementGapsSurviveTheRoundTrip(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	ctx := t.Context()
	mustInsert(t, transactor, ctx, repository, submittedShipmentRequest(t, "req-gap-1", "request-gap-1"))

	var checks []domain.AcceptanceCheck
	for _, check := range allGroupsPassing(t) {
		if check.Group() != domain.RequiredDocumentCheck {
			checks = append(checks, check)
		}
	}
	missing, err := domain.NewUndeterminedAcceptanceCheck(domain.RequiredDocumentCheck, mustBuild(t, domain.NewDeclaredParcelID, "parcel-2"),
		mustBuild(t, domain.NewCheckReason, "EVERY_PARCEL_HAS_PLATFORM_BOX_LABEL"), domain.ResumeByCustomerSupplement)
	if err != nil {
		t.Fatal(err)
	}
	waiting, err := mustFind(t, repository, ctx, "req-gap-1").Decide(domain.AcceptanceDecisionSpec{
		DecisionID: mustBuild(t, domain.NewAcceptanceDecisionID, "accept-gap-1"),
		Checks:     append(checks, missing),
		Basis:      basisWithConditions(t),
		DecidedAt:  submittedAtFixture.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	mustSave(t, transactor, ctx, repository, requestIdentity(t, "req-gap-1"), waiting)

	stored := mustFind(t, repository, ctx, "req-gap-1")
	gaps := stored.AcceptanceDecisionTask().SupplementGaps()
	if len(gaps) != 1 || gaps[0].Group() != domain.RequiredDocumentCheck || gaps[0].Reason().String() != "EVERY_PARCEL_HAS_PLATFORM_BOX_LABEL" {
		t.Fatalf("读回的缺口 = %+v", gaps)
	}
	if parcel, named := gaps[0].Parcel(); !named || parcel.String() != "parcel-2" {
		t.Fatalf("读回的缺口包裹 = %v", parcel)
	}

	accepted, err := stored.Decide(domain.AcceptanceDecisionSpec{
		DecisionID: mustBuild(t, domain.NewAcceptanceDecisionID, "accept-gap-2"),
		Checks:     append(checks, versionCheck(t, domain.RequiredDocumentCheck, domain.CheckPassed, "")),
		Basis:      basisWithConditions(t),
		DecidedAt:  submittedAtFixture.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	mustSave(t, transactor, ctx, repository, requestIdentity(t, "req-gap-1"), accepted)
	reloaded := mustFind(t, repository, ctx, "req-gap-1")
	if gaps := reloaded.AcceptanceDecisionTask().SupplementGaps(); len(gaps) != 0 {
		t.Fatalf("接受之后还留着缺口：%+v", gaps)
	}
	decision, formed := reloaded.AcceptanceDecision()
	if !formed {
		t.Fatal("决定没读回来")
	}
	conditions := decision.Basis().SubmissionConditions()
	if len(conditions.Conditions()) != 2 || !conditions.Has(domain.EveryParcelHasPlatformBoxLabelCondition) {
		t.Fatalf("决定依据上的条件 = %v", conditions.Conditions())
	}
}
