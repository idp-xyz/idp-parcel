package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/application"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

var reassessedAt = time.Date(2026, 8, 9, 9, 0, 0, 0, time.UTC)

func reassessKey(t *testing.T) domain.InitialRouteJudgmentKey {
	t.Helper()
	return domain.InitialRouteJudgmentKey{
		TenantID:           value(t, domain.NewTenantID, "tenant-1"),
		CustomerAccountID:  value(t, domain.NewCustomerAccountID, "customer-1"),
		ShipmentRequestID:  value(t, domain.NewShipmentRequestID, "request-1"),
		AcceptanceBaseline: value(t, domain.NewAcceptanceBaselineReference, "baseline-1/v1"),
		DeclaredParcelID:   value(t, domain.NewDeclaredParcelID, "parcel-1"),
		ServicePurpose:     value(t, domain.NewServicePurpose, "NETWORK_SERVICE"),
	}
}

func evidenceBeforeFreeze(t *testing.T) ports.InitialRouteEvidence {
	t.Helper()
	evidence := routableEvidence(t)
	limit := 0
	evidence.FreezeForm = domain.RemainingSegmentCountFreeze
	evidence.FreezeRemainingSegmentLimit = &limit
	return evidence
}

func reassessCommand(t *testing.T, location string) application.ReassessRouteCommand {
	t.Helper()
	return application.ReassessRouteCommand{Trigger: domain.ReassessmentTriggerSpec{
		Correlation:   value(t, domain.NewRequestCorrelationID, "trigger-1"),
		Key:           reassessKey(t),
		Control:       domain.NodeIntakeControl,
		Location:      value(t, domain.NewActualLocationReference, location),
		SourceVersion: value(t, domain.NewSourceFactVersionReference, "intake-fact/v1"),
		OccurredAt:    reassessedAt,
	}}
}

// currentPlanRecord 把一份真经领域形成的计划放进路由历史。
func currentPlanRecord(t *testing.T) ports.InitialRouteRecord {
	t.Helper()
	evidence := routableEvidence(t)
	plan, err := domain.FormInitialRoutePlan(domain.InitialRoutePlanSpec{
		Key:      reassessKey(t),
		Version:  value(t, domain.NewRoutePlanVersionID, "plan-1/v1"),
		Selected: value(t, domain.NewCandidateID, "candidate-1"),
		Candidates: []domain.RouteCandidate{
			mustQualified(t, "candidate-1"),
		},
		Legs:          evidence.Paths[0].Legs,
		Strategy:      evidence.Strategy,
		ViewRevision:  evidence.ViewRevision,
		JudgedAt:      reassessedAt.Add(-time.Hour),
		EffectiveFrom: reassessedAt.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("form plan: %v", err)
	}
	return ports.InitialRouteRecord{Key: reassessKey(t), Plan: plan, HasPlan: true}
}

func mustQualified(t *testing.T, id string) domain.RouteCandidate {
	t.Helper()
	candidate, err := domain.NewRouteCandidate(
		value(t, domain.NewCandidateID, id), domain.CandidateQualified, domain.CandidateReason{})
	if err != nil {
		t.Fatalf("new qualified candidate: %v", err)
	}
	return candidate
}

type applicabilityStoreDouble struct {
	byPlan map[domain.RoutePlanVersionID]domain.PlanApplicability
	saved  []domain.PlanApplicability
}

func newApplicabilityStore() *applicabilityStoreDouble {
	return &applicabilityStoreDouble{byPlan: map[domain.RoutePlanVersionID]domain.PlanApplicability{}}
}

func (double *applicabilityStoreDouble) FindByPlan(
	_ context.Context,
	plan domain.RoutePlanVersionID,
) (domain.PlanApplicability, bool, error) {
	applicability, found := double.byPlan[plan]
	return applicability, found, nil
}

func (double *applicabilityStoreDouble) Save(_ context.Context, applicability domain.PlanApplicability) error {
	double.byPlan[applicability.Plan()] = applicability
	double.saved = append(double.saved, applicability)
	return nil
}

type reassessmentStoreDouble struct {
	byCorrelation map[domain.RequestCorrelationID]ports.ReassessmentRecord
	notJudged     map[domain.RequestCorrelationID]ports.NotJudgedTrigger
	notJudgedErr  error
	saved         int
}

func newReassessmentStore() *reassessmentStoreDouble {
	return &reassessmentStoreDouble{
		byCorrelation: map[domain.RequestCorrelationID]ports.ReassessmentRecord{},
		notJudged:     map[domain.RequestCorrelationID]ports.NotJudgedTrigger{},
	}
}

// RecordNotJudged 与真库同一条：同一关联已有一行时保留先到那一行。
func (double *reassessmentStoreDouble) RecordNotJudged(_ context.Context, record ports.NotJudgedTrigger) error {
	if double.notJudgedErr != nil {
		return double.notJudgedErr
	}
	if _, exists := double.notJudged[record.Correlation]; !exists {
		double.notJudged[record.Correlation] = record
	}
	return nil
}

func (double *reassessmentStoreDouble) FindByCorrelation(
	_ context.Context,
	_ domain.TenantID,
	correlation domain.RequestCorrelationID,
) (ports.ReassessmentRecord, bool, error) {
	record, found := double.byCorrelation[correlation]
	return record, found, nil
}

func (double *reassessmentStoreDouble) Save(
	_ context.Context,
	correlation domain.RequestCorrelationID,
	record ports.ReassessmentRecord,
) (ports.ReassessmentSaveOutcome, error) {
	if _, exists := double.byCorrelation[correlation]; exists {
		return ports.ReassessmentAlreadyRecorded, nil
	}
	double.byCorrelation[correlation] = record
	double.saved++
	return ports.ReassessmentSaved, nil
}

type reassessFixture struct {
	handler       *application.ReassessRouteHandler
	routes        *routeStoreDouble
	evidence      *routeEvidenceDouble
	applicability *applicabilityStoreDouble
	store         *reassessmentStoreDouble
	log           *handoffLogDouble
}

func newReassessFixture(t *testing.T) *reassessFixture {
	t.Helper()
	fixture := &reassessFixture{
		routes:        newRouteStore(),
		evidence:      &routeEvidenceDouble{byParcel: map[string]ports.InitialRouteEvidence{}, errs: map[string]error{}},
		applicability: newApplicabilityStore(),
		store:         newReassessmentStore(),
		log:           newHandoffLog(),
	}
	fixture.handler = application.NewReassessRouteHandler(application.ReassessRouteDeps{
		Routes:        fixture.routes,
		Evidence:      fixture.evidence,
		Applicability: fixture.applicability,
		Store:         fixture.store,
		Log:           fixture.log,
		Identities:    &routeIdentityDouble{},
		Clock:         fixedClock{at: reassessedAt},
	})
	return fixture
}

// Covers: ADR-0075 决定一——复核与初始路由一样按随请求携带的地理投影取证据：触发带来的投影原样交到取数侧，
// 关务适用性才问得到两端国家。同一触发换了投影不是重放，答触发冲突，理由同初始路由的交接指纹。
func TestAReassessmentLoadsItsEvidenceWithTheCarriedGeoProjection(t *testing.T) {
	fixture := newReassessFixture(t)
	record := currentPlanRecord(t)
	fixture.routes.records[record.Key] = record
	fixture.evidence.byParcel["parcel-1"] = evidenceBeforeFreeze(t)

	command := reassessCommand(t, "node-origin")
	command.Carried = ports.RequestCarriedContent{Geo: domain.NewGeoResolutionProjection(
		domain.NewGeoResolutionSide("CN", true, "200000", true),
		domain.NewGeoResolutionSide("SG", true, "018956", true),
	)}
	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.ReassessedStillApplicable {
		t.Fatalf("outcome = %q, want STILL_APPLICABLE", result.Outcome())
	}
	if fixture.evidence.carried.Geo.ContentDigest() != command.Carried.Geo.ContentDigest() {
		t.Fatalf("取证据时的投影 = %q，want 触发带来的 %q",
			fixture.evidence.carried.Geo.ContentDigest(), command.Carried.Geo.ContentDigest())
	}

	moved := command
	moved.Carried = ports.RequestCarriedContent{Geo: domain.NewGeoResolutionProjection(
		domain.NewGeoResolutionSide("CN", true, "200000", true),
		domain.NewGeoResolutionSide("MY", true, "50000", true),
	)}
	conflict, err := fixture.handler.Handle(context.Background(), moved)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if conflict.Outcome() != application.ReassessTriggerConflict {
		t.Fatalf("outcome = %q, want TRIGGER_CONFLICT——同一触发换了投影", conflict.Outcome())
	}
}

// Covers: UC-NR-003 结果「原路由仍适用——复核后当前计划仍满足硬约束……不创建语义重复的
// 新计划」：实际位置在计划上且无失效依据，结论仍适用引用原计划版本。
func TestAReassessmentKeepsAStillApplicablePlan(t *testing.T) {
	fixture := newReassessFixture(t)
	record := currentPlanRecord(t)
	fixture.routes.records[record.Key] = record
	fixture.evidence.byParcel["parcel-1"] = evidenceBeforeFreeze(t)

	result, err := fixture.handler.Handle(context.Background(), reassessCommand(t, "node-origin"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.ReassessedStillApplicable {
		t.Fatalf("outcome = %q, want STILL_APPLICABLE", result.Outcome())
	}
	kept, present := result.Record()
	if !present || kept.ReviewedPlan.String() != "plan-1/v1" || kept.HasNewPlan {
		t.Fatalf("record = %#v; 仍适用不得制造新计划", kept)
	}
}

// Covers: AT-NR-041 已过冻结边界且原计划仍可执行时，不因改善改路。
func TestAFrozenPlanIsKeptWhenItIsStillExecutable(t *testing.T) {
	fixture := newReassessFixture(t)
	record := currentPlanRecord(t)
	fixture.routes.records[record.Key] = record
	evidence := declareCostImprovement(cheaperAlternative(t, 100), 0)
	limit := 1
	evidence.FreezeForm = domain.RemainingSegmentCountFreeze
	evidence.FreezeRemainingSegmentLimit = &limit
	fixture.evidence.byParcel["parcel-1"] = evidence

	result, err := fixture.handler.Handle(context.Background(), reassessCommand(t, "node-origin"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.ReassessedStillApplicable {
		t.Fatalf("outcome = %q, want STILL_APPLICABLE", result.Outcome())
	}
	kept, present := result.Record()
	if !present || kept.HasNewPlan || kept.HasDecision {
		t.Fatal("越过冻结边界仍造了新计划")
	}
}

// Covers: 仍可执行且未越过冻结边界时，成本改善严格大于已登记阈值才自动切换。
// 阈值 0 是本测试交入的租户取值。
func TestACostImprovementBeforeTheFreezeBoundaryFormsANewPlan(t *testing.T) {
	fixture := newReassessFixture(t)
	record := currentPlanRecord(t)
	fixture.routes.records[record.Key] = record
	evidence := declareCostImprovement(cheaperAlternative(t, 100), 0)
	limit := 0
	evidence.FreezeForm = domain.RemainingSegmentCountFreeze
	evidence.FreezeRemainingSegmentLimit = &limit
	fixture.evidence.byParcel["parcel-1"] = evidence

	result, err := fixture.handler.Handle(context.Background(), reassessCommand(t, "node-origin"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.ReassessedRerouted {
		t.Fatalf("outcome = %q, want REROUTED", result.Outcome())
	}
	switched, present := result.Record()
	if !present || !switched.HasNewPlan || switched.NewPlan.SelectedCandidate().String() != "candidate-2" {
		t.Fatalf("new plan = %#v", switched.NewPlan)
	}
	if len(fixture.applicability.saved) != 2 {
		t.Fatalf("saved applicability = %d, want superseded original and successor", len(fixture.applicability.saved))
	}
}

// Covers: AT-NR-076——原计划仍可执行、策略版本没声明冻结形态：不当成未冻结，也不答未决；入账未判，原因落未判登记，
// 业务时间取触发那一刻，原计划与适用性都不动。
func TestAnUndeclaredFreezeFormDoesNotCountAsUnfrozen(t *testing.T) {
	fixture := newReassessFixture(t)
	record := currentPlanRecord(t)
	fixture.routes.records[record.Key] = record
	fixture.evidence.byParcel["parcel-1"] = routableEvidence(t)

	result, err := fixture.handler.Handle(context.Background(), reassessCommand(t, "node-origin"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.ReassessTriggerNotJudged ||
		result.NotJudgedReason() != ports.NotJudgedFreezeFormUnconfigured {
		t.Fatalf("outcome = %q reason = %q, want TRIGGER_NOT_JUDGED/FREEZE_FORM_UNCONFIGURED",
			result.Outcome(), result.NotJudgedReason())
	}
	assertOnlyNotJudged(t, fixture, ports.NotJudgedFreezeFormUnconfigured)
	if len(fixture.applicability.saved) != 0 {
		t.Fatalf("未判却动了适用性：%d 次", len(fixture.applicability.saved))
	}
}

// assertOnlyNotJudged 钉一次未判只留下一行未判原因：判断键与触发业务时间取自触发，入账时刻取处理时钟，没有复核结论。
func assertOnlyNotJudged(t *testing.T, fixture *reassessFixture, want ports.NotJudgedReason) {
	t.Helper()
	if fixture.store.saved != 0 {
		t.Fatalf("未判却落了 %d 条复核", fixture.store.saved)
	}
	if len(fixture.store.notJudged) != 1 {
		t.Fatalf("未判登记 = %d 行, want 1", len(fixture.store.notJudged))
	}
	noted := fixture.store.notJudged[value(t, domain.NewRequestCorrelationID, "trigger-1")]
	if noted.Reason != want || noted.Key != reassessKey(t) ||
		!noted.TriggerOccurredAt.Equal(reassessedAt) || !noted.SettledAt.Equal(reassessedAt) {
		t.Fatalf("未判登记 = %+v, want 原因 %s、键与触发业务时间取自触发", noted, want)
	}
}

// Covers: AT-NR-075——判断时点没有适用的网络证据：等租户登记，重投推不动，入账未判、原因落未判登记；证据读不动是依赖
// 故障，照旧答未决等重投，不落未判（AT-NR-067）。
func TestUnconfiguredEvidenceIsNotJudgedWhileUnreadableEvidenceStaysUndecided(t *testing.T) {
	unconfigured := newReassessFixture(t)
	record := currentPlanRecord(t)
	unconfigured.routes.records[record.Key] = record
	unconfigured.evidence.unconfigured = map[string]bool{"parcel-1": true}

	result, err := unconfigured.handler.Handle(context.Background(), reassessCommand(t, "node-origin"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.ReassessTriggerNotJudged ||
		result.NotJudgedReason() != ports.NotJudgedEvidenceNotConfigured {
		t.Fatalf("outcome = %q reason = %q, want TRIGGER_NOT_JUDGED/REASSESS_EVIDENCE_NOT_CONFIGURED",
			result.Outcome(), result.NotJudgedReason())
	}
	assertOnlyNotJudged(t, unconfigured, ports.NotJudgedEvidenceNotConfigured)

	unreadable := newReassessFixture(t)
	unreadable.routes.records[record.Key] = record
	unreadable.evidence.errs["parcel-1"] = errors.New("catalog read timed out")
	stalled, err := unreadable.handler.Handle(context.Background(), reassessCommand(t, "node-origin"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if stalled.Outcome() != application.ReassessUndecided ||
		stalled.UndecidedReason() != application.ReassessEvidenceUnavailable {
		t.Fatalf("outcome = %q reason = %q, want UNDECIDED/REASSESS_EVIDENCE_UNAVAILABLE",
			stalled.Outcome(), stalled.UndecidedReason())
	}
	if len(unreadable.store.notJudged) != 0 {
		t.Fatalf("依赖故障落了未判：%+v", unreadable.store.notJudged)
	}
}

// Covers: AT-NR-078——同一触发入账未判之后再投：不答「登记册读不回」，重新判一遍；前提补齐了就判、落一条复核，未判
// 登记只留第一次那一行。未判原因落不了库时不得答未判：改答登记册那一格的未决，照旧回滚重投。
func TestANotJudgedTriggerIsJudgedAgainWhenItIsDeliveredAgain(t *testing.T) {
	fixture := newReassessFixture(t)
	fixture.evidence.byParcel["parcel-1"] = evidenceBeforeFreeze(t)

	first, err := fixture.handler.Handle(context.Background(), reassessCommand(t, "node-origin"))
	if err != nil {
		t.Fatalf("first handle: %v", err)
	}
	if first.Outcome() != application.ReassessTriggerNotJudged {
		t.Fatalf("first outcome = %q, want TRIGGER_NOT_JUDGED", first.Outcome())
	}
	again, err := fixture.handler.Handle(context.Background(), reassessCommand(t, "node-origin"))
	if err != nil {
		t.Fatalf("again handle: %v", err)
	}
	if again.Outcome() != application.ReassessTriggerNotJudged {
		t.Fatalf("again outcome = %q, want TRIGGER_NOT_JUDGED——前提仍缺，再入账", again.Outcome())
	}

	record := currentPlanRecord(t)
	fixture.routes.records[record.Key] = record
	resumed, err := fixture.handler.Handle(context.Background(), reassessCommand(t, "node-origin"))
	if err != nil {
		t.Fatalf("resumed handle: %v", err)
	}
	if resumed.Outcome() != application.ReassessedStillApplicable {
		t.Fatalf("resumed outcome = %q, want STILL_APPLICABLE——历史补齐之后再投就判", resumed.Outcome())
	}
	if fixture.store.saved != 1 || len(fixture.store.notJudged) != 1 {
		t.Fatalf("复核 %d 条、未判 %d 行, want 1 与 1", fixture.store.saved, len(fixture.store.notJudged))
	}
	if noted := fixture.store.notJudged[value(t, domain.NewRequestCorrelationID, "trigger-1")]; noted.Reason != ports.NotJudgedNoRoutingHistory {
		t.Fatalf("未判原因 = %s, want 第一次那一格 NO_ROUTING_HISTORY", noted.Reason)
	}

	unrecorded := newReassessFixture(t)
	unrecorded.store.notJudgedErr = errors.New("pool closed")
	stalled, err := unrecorded.handler.Handle(context.Background(), reassessCommand(t, "node-origin"))
	if err != nil {
		t.Fatalf("unrecorded handle: %v", err)
	}
	if stalled.Outcome() != application.ReassessUndecided ||
		stalled.UndecidedReason() != application.ReassessStoreUnavailable {
		t.Fatalf("outcome = %q reason = %q, want UNDECIDED/REASSESS_STORE_UNAVAILABLE——原因没落库不得入账",
			stalled.Outcome(), stalled.UndecidedReason())
	}
}

// Covers: 硬句「原计划已被硬限制证明失效，即使替代候选暂时无法完成评估，也不能把它继续
// 标记为适用；应提交『原计划已失效 + 无当前有效路由 + 替代候选评估未决』」——实际接货
// 位置不在计划上即失效；证据里的未知限制让候选评估停在未决；三件同录一份记录，适用性
// 转移落库。
func TestALapseIsCommittedEvenWhileCandidatesStayUndecided(t *testing.T) {
	fixture := newReassessFixture(t)
	record := currentPlanRecord(t)
	fixture.routes.records[record.Key] = record
	evidence := routableEvidence(t)
	unknown, err := domain.NewHardConstraintFinding(domain.HardConstraintFindingSpec{
		Candidate: value(t, domain.NewCandidateID, "candidate-1"),
		Outcome:   domain.ConstraintStatusUnknown,
		Missing:   value(t, domain.NewEvidenceGapReference, "CUSTOMS_ELIGIBILITY"),
		Reassess:  value(t, domain.NewReassessmentCondition, "WHEN_CUSTOMS_AUTHORITY_ANSWERS"),
	})
	if err != nil {
		t.Fatalf("new unknown constraint: %v", err)
	}
	evidence.HardConstraints = []domain.HardConstraintFinding{unknown}
	fixture.evidence.byParcel["parcel-1"] = evidence

	result, err := fixture.handler.Handle(context.Background(), reassessCommand(t, "node-actual-elsewhere"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.ReassessedPlanLapsed {
		t.Fatalf("outcome = %q, want PLAN_LAPSED——候选未决拦不住失效", result.Outcome())
	}
	committed, _ := result.Record()
	if committed.LapseBasis.String() != "PLAN_ORIGIN_MISMATCH/node-actual-elsewhere" {
		t.Fatalf("lapse basis = %s", committed.LapseBasis)
	}
	if committed.CandidateState != ports.CandidateReviewUndecided {
		t.Fatalf("candidate state = %q, want CANDIDATE_REVIEW_UNDECIDED——评估未决单独记录", committed.CandidateState)
	}
	saved := fixture.applicability.byPlan[value(t, domain.NewRoutePlanVersionID, "plan-1/v1")]
	if saved.State() != domain.PlanLapsed {
		t.Fatalf("applicability = %q, want LAPSED——适用性转移必须落库", saved.State())
	}
}

// Covers: CONTEXT 生命周期「此前处于无当前有效路由时，候选评估完成且有合格候选则形成
// 首个当前有效路由计划」与结果「新路由已形成：原先无路由且现在有合格候选」。
func TestAFormerNoRouteFormsItsFirstCurrentPlan(t *testing.T) {
	fixture := newReassessFixture(t)
	noRoute, err := domain.FormNoCurrentRouteJudgment(domain.NoCurrentRouteJudgmentSpec{
		Key: reassessKey(t),
		Candidates: []domain.RouteCandidate{
			eliminated(t, "candidate-1", "PATH_NOT_EXECUTABLE/SCHED-V3"),
		},
		Strategy:     value(t, domain.NewRouteStrategyReference, "strategy-1/v1"),
		ViewRevision: value(t, domain.NewNetworkViewRevision, "net-view-rev-1"),
		JudgedAt:     reassessedAt.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("form prior no-route: %v", err)
	}
	fixture.routes.records[reassessKey(t)] = ports.InitialRouteRecord{
		Key: reassessKey(t), NoRoute: noRoute, HasNoRoute: true,
	}
	fixture.evidence.byParcel["parcel-1"] = routableEvidence(t)

	result, err := fixture.handler.Handle(context.Background(), reassessCommand(t, "node-origin"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.ReassessedFirstPlanFormed {
		t.Fatalf("outcome = %q, want FIRST_PLAN_FORMED", result.Outcome())
	}
	record, _ := result.Record()
	if !record.HasNewPlan || record.NewPlan.SelectedCandidate().String() != "candidate-1" {
		t.Fatalf("record = %#v; 首个当前有效计划没形成", record)
	}
}

// Covers: 原先无路由、复核时最低成本并列——不形成首个计划（那等于任选一家），保持无当前有效
// 路由并把候选评估记为未决，不冒充「没有合格候选」。这条线没有被复核计划，改路三件落不了库
// （`route_reassessment_reroute_on_reviewed_plan`），所以不挂建议。
func TestAFormerNoRouteWithATiedLowestCostFormsNoFirstPlan(t *testing.T) {
	fixture := newReassessFixture(t)
	noRoute, err := domain.FormNoCurrentRouteJudgment(domain.NoCurrentRouteJudgmentSpec{
		Key: reassessKey(t),
		Candidates: []domain.RouteCandidate{
			eliminated(t, "candidate-1", "PATH_NOT_EXECUTABLE/SCHED-V3"),
		},
		Strategy:     value(t, domain.NewRouteStrategyReference, "strategy-1/v1"),
		ViewRevision: value(t, domain.NewNetworkViewRevision, "net-view-rev-1"),
		JudgedAt:     reassessedAt.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("form prior no-route: %v", err)
	}
	fixture.routes.records[reassessKey(t)] = ports.InitialRouteRecord{
		Key: reassessKey(t), NoRoute: noRoute, HasNoRoute: true,
	}
	fixture.evidence.byParcel["parcel-1"] = tiedEvidence(t)

	result, err := fixture.handler.Handle(context.Background(), reassessCommand(t, "node-origin"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.ReassessedPlanLapsed {
		t.Fatalf("outcome = %q, want PLAN_LAPSED——仍无当前有效路由", result.Outcome())
	}
	record, _ := result.Record()
	if record.HasNewPlan || record.HasSuggestion || record.HasDecision {
		t.Fatalf("new plan = %v, suggestion = %v, decision = %v; 并列不形成首个计划",
			record.HasNewPlan, record.HasSuggestion, record.HasDecision)
	}
	if record.CandidateState != ports.CandidateReviewUndecided {
		t.Fatalf("candidate state = %q, want CANDIDATE_REVIEW_UNDECIDED", record.CandidateState)
	}
}

// Covers: 结果「已有结果：同一触发和输入版本已经处理——不重复决定」与「触发冲突：同一
// 触发身份携带不同事实或版本——不覆盖原判断」；没有历史结果时不得凭空假定初始计划，
// 入账未判（AT-NR-074）。
func TestReplayConflictAndMissingHistoryStayDisciplined(t *testing.T) {
	fixture := newReassessFixture(t)
	record := currentPlanRecord(t)
	fixture.routes.records[record.Key] = record
	fixture.evidence.byParcel["parcel-1"] = evidenceBeforeFreeze(t)

	if _, err := fixture.handler.Handle(context.Background(), reassessCommand(t, "node-origin")); err != nil {
		t.Fatalf("first handle: %v", err)
	}

	replay, err := fixture.handler.Handle(context.Background(), reassessCommand(t, "node-origin"))
	if err != nil {
		t.Fatalf("replay handle: %v", err)
	}
	if replay.Outcome() != application.ReassessedStillApplicable {
		t.Fatalf("replay outcome = %q（按原记录作答）", replay.Outcome())
	}
	if fixture.store.saved != 1 {
		t.Fatalf("saved = %d; 重复触发不得重复决定", fixture.store.saved)
	}

	conflict, err := fixture.handler.Handle(context.Background(), reassessCommand(t, "node-different"))
	if err != nil {
		t.Fatalf("conflict handle: %v", err)
	}
	if conflict.Outcome() != application.ReassessTriggerConflict {
		t.Fatalf("outcome = %q, want TRIGGER_CONFLICT", conflict.Outcome())
	}

	bare := newReassessFixture(t)
	bare.evidence.byParcel["parcel-1"] = routableEvidence(t)
	missing, err := bare.handler.Handle(context.Background(), reassessCommand(t, "node-origin"))
	if err != nil {
		t.Fatalf("missing history handle: %v", err)
	}
	if missing.Outcome() != application.ReassessTriggerNotJudged ||
		missing.NotJudgedReason() != ports.NotJudgedNoRoutingHistory {
		t.Fatalf("outcome = %q/%q, want TRIGGER_NOT_JUDGED/NO_ROUTING_HISTORY", missing.Outcome(), missing.NotJudgedReason())
	}
	assertOnlyNotJudged(t, bare, ports.NotJudgedNoRoutingHistory)

	hint := reassessCommand(t, "node-origin")
	hint.Trigger.Control = domain.SourceHint
	refused, err := fixture.handler.Handle(context.Background(), hint)
	if err != nil {
		t.Fatalf("hint handle: %v", err)
	}
	if refused.Outcome() != application.ReassessTriggerNotAccepted {
		t.Fatalf("outcome = %q; 线索立不成触发", refused.Outcome())
	}
}
