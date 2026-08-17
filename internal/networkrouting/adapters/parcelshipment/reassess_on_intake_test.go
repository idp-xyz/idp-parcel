package parcelshipment_test

import (
	"context"
	"errors"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/parcelshipment"
	nrapplication "go.idp.xyz/idp-parcel/internal/networkrouting/application"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

var intakeAt = time.Date(2026, 8, 9, 7, 30, 0, 0, time.UTC)

func value[T interface{ String() string }](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return built
}

// adoptedRecord 造一份真经 PS 领域形成的采用记录（假记录钉不住翻译取值的来处）。
func adoptedRecord(t *testing.T) psports.IntakeAdoptionRecord {
	t.Helper()
	source, err := psdomain.NewIntakeSource(psdomain.IntakeSourceSpec{
		Kind:       psdomain.NodeIntakeSource,
		Object:     value(t, psdomain.NewSourceObjectReference, "unit-1"),
		Parcel:     value(t, psdomain.NewDeclaredParcelID, "parcel-1"),
		Place:      value(t, psdomain.NewIntakePlaceReference, "node-origin"),
		Control:    value(t, psdomain.NewIntakeControlReference, "NODE-INTAKE/SIGN-7"),
		Version:    value(t, psdomain.NewSourceResultVersion, "intake-result/v1"),
		OccurredAt: intakeAt,
	})
	if err != nil {
		t.Fatalf("new intake source: %v", err)
	}
	intake, err := psdomain.AdoptNetworkIntake(source, value(t, psdomain.NewSubmissionVersionID, "version-1"))
	if err != nil {
		t.Fatalf("adopt network intake: %v", err)
	}
	commitment, err := psdomain.FormFormalCommitment(
		value(t, psdomain.NewCommitmentVersionID, "commitment-1/v1"),
		intake,
		value(t, psdomain.NewExpectedCommitmentReference, "expected/RES-1"),
	)
	if err != nil {
		t.Fatalf("form formal commitment: %v", err)
	}
	return psports.IntakeAdoptionRecord{
		Key: psports.IntakeAdoptionKey{
			TenantID: value(t, psdomain.NewTenantID, "tenant-1"),
			Parcel:   value(t, psdomain.NewDeclaredParcelID, "parcel-1"),
			Kind:     psdomain.NodeIntakeSource,
			Version:  value(t, psdomain.NewSourceResultVersion, "intake-result/v1"),
		},
		CustomerAccountID: value(t, psdomain.NewCustomerAccountID, "customer-1"),
		ShipmentRequestID: value(t, psdomain.NewShipmentRequestID, "request-1"),
		ContentDigest:     "digest-1",
		Adopted:           true,
		Intake:            intake,
		Commitment:        commitment,
		AdoptedAt:         intakeAt.Add(time.Minute),
	}
}

// reassessKey 是翻译后应落进 NR 的判断键。
func reassessKey(t *testing.T) nrdomain.InitialRouteJudgmentKey {
	t.Helper()
	return nrdomain.InitialRouteJudgmentKey{
		TenantID:           value(t, nrdomain.NewTenantID, "tenant-1"),
		CustomerAccountID:  value(t, nrdomain.NewCustomerAccountID, "customer-1"),
		ShipmentRequestID:  value(t, nrdomain.NewShipmentRequestID, "request-1"),
		AcceptanceBaseline: value(t, nrdomain.NewAcceptanceBaselineReference, "version-1"),
		DeclaredParcelID:   value(t, nrdomain.NewDeclaredParcelID, "parcel-1"),
		ServicePurpose:     value(t, nrdomain.NewServicePurpose, "NETWORK_SERVICE"),
	}
}

type routeStoreDouble struct {
	records map[nrdomain.InitialRouteJudgmentKey]nrports.InitialRouteRecord
}

func (double *routeStoreDouble) FindByKey(
	_ context.Context,
	key nrdomain.InitialRouteJudgmentKey,
) (nrports.InitialRouteRecord, bool, error) {
	record, found := double.records[key]
	return record, found, nil
}

func (double *routeStoreDouble) Save(
	_ context.Context, _ nrports.InitialRouteRecord,
) (nrports.InitialRouteSaveOutcome, error) {
	return 0, errors.New("not part of this seam")
}

type evidenceDouble struct{ evidence nrports.InitialRouteEvidence }

func (double evidenceDouble) LoadInitialRouteEvidence(
	_ context.Context, _ nrdomain.InitialRouteJudgmentKey,
) (nrports.InitialRouteEvidence, bool, error) {
	return double.evidence, true, nil
}

type applicabilityStoreDouble struct {
	byPlan map[nrdomain.RoutePlanVersionID]nrdomain.PlanApplicability
}

func (double *applicabilityStoreDouble) FindByPlan(
	_ context.Context, plan nrdomain.RoutePlanVersionID,
) (nrdomain.PlanApplicability, bool, error) {
	applicability, found := double.byPlan[plan]
	return applicability, found, nil
}

func (double *applicabilityStoreDouble) Save(_ context.Context, applicability nrdomain.PlanApplicability) error {
	double.byPlan[applicability.Plan()] = applicability
	return nil
}

type reassessStoreDouble struct {
	byCorrelation map[nrdomain.RequestCorrelationID]nrports.ReassessmentRecord
}

func (double *reassessStoreDouble) FindByCorrelation(
	_ context.Context, _ nrdomain.TenantID, correlation nrdomain.RequestCorrelationID,
) (nrports.ReassessmentRecord, bool, error) {
	record, found := double.byCorrelation[correlation]
	return record, found, nil
}

func (double *reassessStoreDouble) Save(
	_ context.Context, correlation nrdomain.RequestCorrelationID, record nrports.ReassessmentRecord,
) (nrports.ReassessmentSaveOutcome, error) {
	double.byCorrelation[correlation] = record
	return nrports.ReassessmentSaved, nil
}

type logDouble struct {
	digests map[nrdomain.RequestCorrelationID]string
}

func (double *logDouble) FindDigest(
	_ context.Context, _ nrdomain.TenantID, correlation nrdomain.RequestCorrelationID,
) (string, bool, error) {
	digest, found := double.digests[correlation]
	return digest, found, nil
}

func (double *logDouble) Append(
	_ context.Context, _ nrdomain.TenantID, correlation nrdomain.RequestCorrelationID, digest string,
) error {
	double.digests[correlation] = digest
	return nil
}

type identityDouble struct{ next int }

func (double *identityDouble) NextRoutePlanVersionID(_ context.Context) (nrdomain.RoutePlanVersionID, error) {
	double.next++
	return nrdomain.NewRoutePlanVersionID("plan-" + string(rune('0'+double.next)))
}

type fixedClock struct{ at time.Time }

func (clock fixedClock) Now() time.Time { return clock.at }

// planOnFile 把一份真经 NR 领域形成的计划放进路由历史（首节点即收寄节点，复核应答
// 仍适用）。
func planOnFile(t *testing.T) nrports.InitialRouteRecord {
	t.Helper()
	window, err := nrdomain.NewPlannedTimeWindow(intakeAt, intakeAt.Add(48*time.Hour),
		value(t, nrdomain.NewWindowBasisReference, "CALENDAR-V1"))
	if err != nil {
		t.Fatalf("new window: %v", err)
	}
	leg, err := nrdomain.NewPlannedLeg(nrdomain.PlannedLegSpec{
		From:        value(t, nrdomain.NewPlanNodeReference, "node-origin"),
		To:          value(t, nrdomain.NewPlanNodeReference, "node-destination"),
		Responsible: value(t, nrdomain.NewResponsiblePartyReference, "party-1"),
		Window:      window,
	})
	if err != nil {
		t.Fatalf("new leg: %v", err)
	}
	qualified, err := nrdomain.NewRouteCandidate(
		value(t, nrdomain.NewCandidateID, "candidate-1"),
		nrdomain.CandidateQualified,
		nrdomain.CandidateReason{},
	)
	if err != nil {
		t.Fatalf("new candidate: %v", err)
	}
	plan, err := nrdomain.FormInitialRoutePlan(nrdomain.InitialRoutePlanSpec{
		Key:           reassessKey(t),
		Version:       value(t, nrdomain.NewRoutePlanVersionID, "plan-1/v1"),
		Selected:      value(t, nrdomain.NewCandidateID, "candidate-1"),
		Candidates:    []nrdomain.RouteCandidate{qualified},
		Legs:          []nrdomain.PlannedLeg{leg},
		Strategy:      value(t, nrdomain.NewRouteStrategyReference, "strategy-1/v1"),
		ViewRevision:  value(t, nrdomain.NewNetworkViewRevision, "net-view-rev-1"),
		JudgedAt:      intakeAt.Add(-time.Hour),
		EffectiveFrom: intakeAt.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("form plan: %v", err)
	}
	return nrports.InitialRouteRecord{Key: reassessKey(t), Plan: plan, HasPlan: true}
}

// Covers: UC-PS-003 步骤 8「network-routing 消费有效网络收寄及原物理位置/控制引用，
// 通过 UC-NR-003 重新校验路由」经适配器端到端——采用记录逐维译成复核触发（位置=实际
// 收寄地点、来源版本=收寄结果版本、控制=节点收寄格），走完真实复核编排答「仍适用」。
func TestAnAdoptedIntakeTriggersARealReassessment(t *testing.T) {
	handler := nrapplication.NewReassessRouteHandler(nrapplication.ReassessRouteDeps{
		Routes: &routeStoreDouble{records: map[nrdomain.InitialRouteJudgmentKey]nrports.InitialRouteRecord{reassessKey(t): planOnFile(t)}},
		Evidence: evidenceDouble{evidence: nrports.InitialRouteEvidence{
			Strategy:     value(t, nrdomain.NewRouteStrategyReference, "strategy-1/v1"),
			ViewRevision: value(t, nrdomain.NewNetworkViewRevision, "net-view-rev-1"),
		}},
		Applicability: &applicabilityStoreDouble{byPlan: map[nrdomain.RoutePlanVersionID]nrdomain.PlanApplicability{}},
		Store:         &reassessStoreDouble{byCorrelation: map[nrdomain.RequestCorrelationID]nrports.ReassessmentRecord{}},
		Log:           &logDouble{digests: map[nrdomain.RequestCorrelationID]string{}},
		Identities:    &identityDouble{},
		Clock:         fixedClock{at: intakeAt.Add(2 * time.Minute)},
	})
	subject := adapter.NewReassessOnIntakeAdapter(handler, value(t, nrdomain.NewServicePurpose, "NETWORK_SERVICE"))

	result, err := subject.TriggerReassessment(context.Background(), adoptedRecord(t))
	if err != nil {
		t.Fatalf("trigger reassessment: %v", err)
	}

	if result.Outcome() != nrapplication.ReassessedStillApplicable {
		t.Fatalf("outcome = %q, want STILL_APPLICABLE——收寄点就在计划首节点上", result.Outcome())
	}
	record, present := result.Record()
	if !present || record.ReviewedPlan.String() != "plan-1/v1" {
		t.Fatalf("record = %#v present = %v", record, present)
	}
}

// Covers: 分界防线——不采用记录不触发复核（没有责任起点成立，独立哨兵与翻译故障分开）；
// 服务目的未配置停下不猜。
func TestRefusalsAndMissingPurposeDoNotTrigger(t *testing.T) {
	handler := nrapplication.NewReassessRouteHandler(nrapplication.ReassessRouteDeps{
		Routes:        &routeStoreDouble{records: map[nrdomain.InitialRouteJudgmentKey]nrports.InitialRouteRecord{}},
		Evidence:      evidenceDouble{},
		Applicability: &applicabilityStoreDouble{byPlan: map[nrdomain.RoutePlanVersionID]nrdomain.PlanApplicability{}},
		Store:         &reassessStoreDouble{byCorrelation: map[nrdomain.RequestCorrelationID]nrports.ReassessmentRecord{}},
		Log:           &logDouble{digests: map[nrdomain.RequestCorrelationID]string{}},
		Identities:    &identityDouble{},
		Clock:         fixedClock{at: intakeAt},
	})

	refused := adoptedRecord(t)
	refused.Adopted = false
	subject := adapter.NewReassessOnIntakeAdapter(handler, value(t, nrdomain.NewServicePurpose, "NETWORK_SERVICE"))
	if _, err := subject.TriggerReassessment(context.Background(), refused); !errors.Is(err, adapter.ErrRefusalDoesNotTrigger) {
		t.Fatalf("err = %v, want ErrRefusalDoesNotTrigger", err)
	}

	unconfigured := adapter.NewReassessOnIntakeAdapter(handler, nrdomain.ServicePurpose{})
	if _, err := unconfigured.TriggerReassessment(context.Background(), adoptedRecord(t)); !errors.Is(err, adapter.ErrUntranslatableAnswer) {
		t.Fatalf("err = %v, want ErrUntranslatableAnswer", err)
	}
}
