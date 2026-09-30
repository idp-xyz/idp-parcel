package main

import (
	"context"
	"testing"
	"time"

	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 两轮判断被「提交接收」钉在同一时点。删掉推进编排里那处 FormedUnder 时，新解析的判断记不进账，
// 决定停在「形成于别的解析」，不会拿 RES-1 的不可达去接受。
var resolutionChangeAsOf = time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)

func TestAChangedResolutionAdvancesANewJudgmentTheDecisionReads(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	ctx := t.Context()
	fixture.submit(t, ctx)

	commercial := &resolutionChangeCommercial{t: t}
	reachability := resolutionChangeReachability{t: t}
	advance := psapplication.NewAdvanceAcceptanceJudgmentHandler(
		commercial, reachability, fixture.judgments, fixture.requests, systemClock{})
	decision := psapplication.NewFormAcceptanceDecisionHandler(psapplication.FormAcceptanceDecisionDeps{
		Requests:     fixture.requests,
		Commercial:   commercial,
		Reachability: synSReachabilityRevalidator{},
		Judgments:    fixture.judgments,
		Recorder:     fixture.judgments,
		Release:      synSControlRelease{},
		Downstream:   nopAcceptanceDecisionHandoff{},
		Identities:   synSDecisionIdentities{},
		Clock:        systemClock{},
	})

	request := fixture.mustLoadRequest(t, ctx)
	version := request.CurrentSubmissionVersion().VersionID()
	parcel := mustPS(t, psdomain.NewDeclaredParcelID, "SYN-PARCEL-01")
	command := psapplication.AdvanceAcceptanceJudgmentCommand{
		Identity:          fixture.identity,
		ShipmentRequestID: fixture.requestID,
		SubmissionVersion: version,
		DeclaredParcelID:  parcel,
	}
	decide := psapplication.FormAcceptanceDecisionCommand{
		Identity:          fixture.identity,
		ShipmentRequestID: fixture.requestID,
		SubmissionVersion: version,
	}

	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		return fixture.judgments.RecordFinancialControlResult(
			txCtx, fixture.identity.TenantID(), fixture.requestID, version,
			synHeldControl(t, "SYN-SAC-RES", synJudgmentAsOf(t, psdomain.FinancialControlJudgmentKind, resolutionChangeAsOf)))
	})

	first := runAdvance(t, fixture, ctx, advance, command)
	if first.Outcome() != psapplication.AcceptanceJudgmentAdvanced {
		t.Fatalf("首轮推进 = %s reason = %s，want ADVANCED", first.Outcome(), first.PendingReason())
	}

	commercial.second = true
	superseded := runDecision(t, fixture, ctx, decision, decide)
	if superseded.PendingReason() != psapplication.CommercialBasisSuperseded {
		t.Fatalf("依据被推翻后 reason = %s，want COMMERCIAL_BASIS_SUPERSEDED", superseded.PendingReason())
	}
	if got := loadAdopted(t, fixture, ctx, version); got != "SYN-RES-R2" {
		t.Fatalf("重解后采用的解析 = %s，want SYN-RES-R2", got)
	}

	stale := runDecision(t, fixture, ctx, decision, decide)
	if stale.PendingReason() != psapplication.ReachabilityJudgmentFormedUnderAnotherResolution {
		t.Fatalf("新解析下还没有判断时 reason = %s，want REACHABILITY_JUDGMENT_FORMED_UNDER_ANOTHER_RESOLUTION", stale.PendingReason())
	}
	if stale.PendingReason() == psapplication.ReachabilityJudgmentSuperseded {
		t.Fatal("解析换代与网络视图换代共用了一条未决原因")
	}
	if stale.State() != psdomain.ShipmentRequestSubmitted {
		t.Fatalf("旧判断被拿去决定了，状态 = %s", stale.State())
	}

	second := runAdvance(t, fixture, ctx, advance, command)
	if second.Outcome() != psapplication.AcceptanceJudgmentAdvanced {
		t.Fatalf("换解析后再推进 = %s reason = %s，want ADVANCED", second.Outcome(), second.PendingReason())
	}
	recorded := loadReachability(t, fixture, ctx, version)
	if recorded.JudgmentID().String() != "SYN-NRJ-RES-2" || recorded.FormedUnderResolution().String() != "SYN-RES-R2" {
		t.Fatalf("推进记下的判断 = %s / %s，want SYN-NRJ-RES-2 / SYN-RES-R2",
			recorded.JudgmentID(), recorded.FormedUnderResolution())
	}
	if recorded.Value() != psdomain.ReachabilityReachable {
		t.Fatalf("记下的取值 = %s，want REACHABLE——RES-1 的不可达还在被读", recorded.Value())
	}

	accepted := runDecision(t, fixture, ctx, decision, decide)
	if accepted.Outcome() != psapplication.AcceptanceDecided || accepted.State() != psdomain.ShipmentRequestAccepted {
		t.Fatalf("决定 = %s 状态 = %s reason = %s，want 按 RES-2 的可达接受",
			accepted.Outcome(), accepted.State(), accepted.PendingReason())
	}
}

func runAdvance(
	t *testing.T,
	fixture *synVerticalFixture,
	ctx context.Context,
	handler *psapplication.AdvanceAcceptanceJudgmentHandler,
	command psapplication.AdvanceAcceptanceJudgmentCommand,
) psapplication.AdvanceAcceptanceJudgmentResult {
	t.Helper()
	var result psapplication.AdvanceAcceptanceJudgmentResult
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		var err error
		result, err = handler.Handle(txCtx, command)
		return err
	})
	return result
}

func runDecision(
	t *testing.T,
	fixture *synVerticalFixture,
	ctx context.Context,
	handler *psapplication.FormAcceptanceDecisionHandler,
	command psapplication.FormAcceptanceDecisionCommand,
) psapplication.FormAcceptanceDecisionResult {
	t.Helper()
	var result psapplication.FormAcceptanceDecisionResult
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		var err error
		result, err = handler.Handle(txCtx, command)
		return err
	})
	return result
}

func loadAdopted(t *testing.T, fixture *synVerticalFixture, ctx context.Context, version psdomain.SubmissionVersionID) string {
	t.Helper()
	recorded, err := fixture.judgments.LoadRecordedJudgments(ctx, fixture.identity.TenantID(), fixture.requestID, version)
	if err != nil {
		t.Fatalf("读回采用的解析：%v", err)
	}
	return recorded.AdoptedCommercialResolution.String()
}

func loadReachability(t *testing.T, fixture *synVerticalFixture, ctx context.Context, version psdomain.SubmissionVersionID) psdomain.ReachabilityJudgment {
	t.Helper()
	recorded, err := fixture.judgments.LoadRecordedJudgments(ctx, fixture.identity.TenantID(), fixture.requestID, version)
	if err != nil {
		t.Fatalf("读回判断：%v", err)
	}
	if len(recorded.Reachability) != 1 {
		t.Fatalf("读回 %d 项可达性判断，want 1", len(recorded.Reachability))
	}
	return recorded.Reachability[0]
}

type resolutionChangeCommercial struct {
	t      *testing.T
	second bool
}

func (double *resolutionChangeCommercial) ResolveCommercialBasis(
	context.Context,
	psports.CommercialBasisQuery,
) (psports.CommercialBasisResolution, error) {
	id := "SYN-RES-R1"
	if double.second {
		id = "SYN-RES-R2"
	}
	return double.snapshot(id), nil
}

func (double *resolutionChangeCommercial) FormJudgmentAsOf(
	context.Context,
	psports.JudgmentAsOfQuery,
) (psports.JudgmentAsOfFormation, error) {
	return psports.JudgmentAsOfFormation{
		Outcome: psports.JudgmentAsOfFormed,
		AsOf:    synJudgmentAsOf(double.t, psdomain.ReachabilityJudgmentKind, resolutionChangeAsOf),
	}, nil
}

func (double *resolutionChangeCommercial) RevalidateCommercialBasis(
	_ context.Context,
	query psports.CommercialRevalidationQuery,
) (psports.CommercialRevalidation, error) {
	if double.second && query.Resolution.String() == "SYN-RES-R1" {
		return psports.CommercialRevalidation{
			Outcome:    psports.CommercialBasisSuperseded,
			Resolution: double.snapshot("SYN-RES-R1"),
		}, nil
	}
	id := "SYN-RES-R1"
	if double.second {
		id = "SYN-RES-R2"
	}
	return psports.CommercialRevalidation{
		Outcome:    psports.CommercialBasisStillValid,
		Resolution: double.snapshot(id),
	}, nil
}

func (double *resolutionChangeCommercial) snapshot(resolutionID string) psports.CommercialBasisResolution {
	double.t.Helper()
	groups, err := psdomain.NewApplicableCheckGroups(psdomain.NetworkReachabilityCheck)
	if err != nil {
		double.t.Fatalf("适用校验组：%v", err)
	}
	snapshot, err := psdomain.NewCommercialBasisSnapshot(psdomain.CommercialBasisSnapshotSpec{
		ResolutionID: mustPS(double.t, psdomain.NewCommercialResolutionID, resolutionID),
		RulePackage:  mustPS(double.t, psdomain.NewRulePackageReference, "SYN-RULES-RES/v1"),
		ViewRevision: mustPS(double.t, psdomain.NewCommercialViewRevision, "SYN-VIEW-RES"),
		DeclaredAsOf: []psdomain.DeclaredAsOf{
			synDeclaredAsOf(double.t, psdomain.ReachabilityJudgmentKind),
		},
		Applicable:   groups,
		ManualReview: psdomain.ManualReviewNotRequiredByRules,
	})
	if err != nil {
		double.t.Fatalf("商业依据快照：%v", err)
	}
	return psports.CommercialBasisResolution{
		Snapshot:      snapshot,
		Applicability: psdomain.CommerciallyApplicable,
	}
}

type resolutionChangeReachability struct{ t *testing.T }

func (double resolutionChangeReachability) AssessParcelReachability(
	_ context.Context,
	request psports.ReachabilityRequest,
) (psports.ReachabilityAssessment, error) {
	double.t.Helper()
	id, value := "SYN-NRJ-RES-1", psdomain.ReachabilityUnreachable
	if request.Resolution.String() == "SYN-RES-R2" {
		id, value = "SYN-NRJ-RES-2", psdomain.ReachabilityReachable
	}
	judgment, err := psdomain.NewReachabilityJudgment(psdomain.ReachabilityJudgmentSpec{
		JudgmentID: mustPS(double.t, psdomain.NewReachabilityJudgmentID, id),
		ParcelID:   request.DeclaredParcelID,
		Value:      value,
		AsOf:       request.AsOf,
	})
	if err != nil {
		double.t.Fatalf("可达性判断：%v", err)
	}
	return psports.ReachabilityAssessment{Outcome: psports.ReachabilityAssessed, Judgment: judgment}, nil
}

type nopAcceptanceDecisionHandoff struct{}

func (nopAcceptanceDecisionHandoff) HandOffAcceptanceDecision(context.Context, psports.AcceptanceDecisionHandoffIntent) error {
	return nil
}
