package application_test

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/application"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

var reassessedAt = time.Date(2026, 8, 9, 9, 0, 0, 0, time.UTC)

// reassessAcceptedAt 是复核夹具里委托接受的时刻，早于各用例的触发与手放的计划：复核只判不早于接受的触发（ADR-0279），
// 专测早于接受的用例另给。
var reassessAcceptedAt = routeJudgedAt

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
		AcceptedAt:    reassessAcceptedAt,
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
	byPlan  map[domain.RoutePlanVersionID]domain.PlanApplicability
	saved   []domain.PlanApplicability
	findErr error
}

func newApplicabilityStore() *applicabilityStoreDouble {
	return &applicabilityStoreDouble{byPlan: map[domain.RoutePlanVersionID]domain.PlanApplicability{}}
}

func (double *applicabilityStoreDouble) FindByPlan(
	_ context.Context,
	plan domain.RoutePlanVersionID,
) (domain.PlanApplicability, bool, error) {
	if double.findErr != nil {
		return domain.PlanApplicability{}, false, double.findErr
	}
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
	latestErr     error
	newPlanErr    error
	// onLatest 在取判断键锁那一刻插进来，演锁前另一笔事务刚提交的东西。
	onLatest func()
	saved    int
}

func newReassessmentStore() *reassessmentStoreDouble {
	return &reassessmentStoreDouble{
		byCorrelation: map[domain.RequestCorrelationID]ports.ReassessmentRecord{},
		notJudged:     map[domain.RequestCorrelationID]ports.NotJudgedTrigger{},
	}
}

// LatestTriggerAt 与真库同一口径：同判断键、带业务时间的复核里取最晚那一次。
func (double *reassessmentStoreDouble) LatestTriggerAt(
	_ context.Context,
	key domain.InitialRouteJudgmentKey,
) (time.Time, bool, error) {
	if double.latestErr != nil {
		return time.Time{}, false, double.latestErr
	}
	if double.onLatest != nil {
		double.onLatest()
	}
	return latestTriggerAt(double.byCorrelation, key)
}

// FormedPlans 交回同判断键、带新计划的复核里的全部新计划，先后与真库同一口径：按触发业务时间，同一时刻按处理时刻。
func (double *reassessmentStoreDouble) FormedPlans(
	_ context.Context,
	key domain.InitialRouteJudgmentKey,
) ([]domain.InitialRoutePlan, error) {
	if double.newPlanErr != nil {
		return nil, double.newPlanErr
	}
	var formed []ports.ReassessmentRecord
	for _, record := range double.byCorrelation {
		if record.Key == key && record.HasNewPlan {
			formed = append(formed, record)
		}
	}
	sort.Slice(formed, func(i, j int) bool {
		if !formed[i].TriggerOccurredAt.Equal(formed[j].TriggerOccurredAt) {
			return formed[i].TriggerOccurredAt.Before(formed[j].TriggerOccurredAt)
		}
		return formed[i].ReassessedAt.Before(formed[j].ReassessedAt)
	})
	plans := make([]domain.InitialRoutePlan, 0, len(formed))
	for _, record := range formed {
		plans = append(plans, record.NewPlan)
	}
	return plans, nil
}

func latestTriggerAt(
	records map[domain.RequestCorrelationID]ports.ReassessmentRecord,
	key domain.InitialRouteJudgmentKey,
) (time.Time, bool, error) {
	var latest time.Time
	for _, record := range records {
		if record.Key == key && record.TriggerOccurredAt.After(latest) {
			latest = record.TriggerOccurredAt
		}
	}
	return latest, !latest.IsZero(), nil
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
	costs         *reassessCosts
}

func newReassessFixture(t *testing.T) *reassessFixture {
	t.Helper()
	fixture := &reassessFixture{
		routes:        newRouteStore(),
		evidence:      &routeEvidenceDouble{byParcel: map[string]ports.InitialRouteEvidence{}, errs: map[string]error{}},
		applicability: newApplicabilityStore(),
		store:         newReassessmentStore(),
		log:           newHandoffLog(),
		costs:         &reassessCosts{},
	}
	fixture.handler = application.NewReassessRouteHandler(application.ReassessRouteDeps{
		Routes:        fixture.routes,
		Evidence:      fixture.evidence,
		Costs:         fixture.costs,
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

// Covers: 冻结事实不成立（段数为负）不是「冻结形态未配置」：策略版本册不收这种写入，证据视图交出它是装配坏了——
// 响亮报错，不落未判，免得未判登记里多一条点名租户没登记的假原因。
func TestInvalidFreezeFactsAreLoudRatherThanUnconfigured(t *testing.T) {
	fixture := newReassessFixture(t)
	record := currentPlanRecord(t)
	fixture.routes.records[record.Key] = record
	evidence := routableEvidence(t)
	negative := -1
	evidence.FreezeForm = domain.RemainingSegmentCountFreeze
	evidence.FreezeRemainingSegmentLimit = &negative
	fixture.evidence.byParcel["parcel-1"] = evidence

	_, err := fixture.handler.Handle(context.Background(), reassessCommand(t, "node-origin"))
	if !errors.Is(err, domain.ErrUnknownFreezeForm) {
		t.Fatalf("err = %v, want ErrUnknownFreezeForm", err)
	}
	if len(fixture.store.notJudged) != 0 || fixture.store.saved != 0 {
		t.Fatalf("装配缺陷却落了未判 %d 行、复核 %d 条", len(fixture.store.notJudged), fixture.store.saved)
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

// Covers: AT-NR-078——同一触发入账未判之后由另一封信封再带来：不答「登记册读不回」，重新判一遍；前提补齐了就判、落一条
// 复核，未判登记只留第一次那一行。未判原因落不了库时不得答未判：改答登记册那一格的未决，照旧回滚重投。
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

// reassessCommandAt 是另一次控制触发：自己的关联、位置与业务时间。
func reassessCommandAt(t *testing.T, correlation, location string, at time.Time) application.ReassessRouteCommand {
	t.Helper()
	command := reassessCommand(t, location)
	command.Trigger.Correlation = value(t, domain.NewRequestCorrelationID, correlation)
	command.Trigger.SourceVersion = value(t, domain.NewSourceFactVersionReference, correlation+"/fact")
	command.Trigger.OccurredAt = at
	return command
}

func handleOK(t *testing.T, fixture *reassessFixture, command application.ReassessRouteCommand) application.ReassessRouteResult {
	t.Helper()
	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle %s: %v", command.Trigger.Correlation, err)
	}
	return result
}

// Covers: AT-NR-079——包裹已在较晚的位置上判过（仍适用），较早那次触发后到：入账为「触发早于当前位置」，不判——原计划
// 不因那个已经过去的位置失效，适用性不动；未判登记点名比较所用的当前位置时刻。复核记录带上所判触发的业务时间。
func TestATriggerEarlierThanTheCurrentPositionIsNotJudged(t *testing.T) {
	fixture := newReassessFixture(t)
	record := currentPlanRecord(t)
	fixture.routes.records[record.Key] = record
	fixture.evidence.byParcel["parcel-1"] = evidenceBeforeFreeze(t)

	later := handleOK(t, fixture, reassessCommandAt(t, "trigger-later", "node-origin", reassessedAt))
	if later.Outcome() != application.ReassessedStillApplicable {
		t.Fatalf("较晚那次 = %q, want STILL_APPLICABLE", later.Outcome())
	}
	if kept, _ := later.Record(); !kept.TriggerOccurredAt.Equal(reassessedAt) {
		t.Fatalf("复核记录的触发业务时间 = %s, want %s", kept.TriggerOccurredAt, reassessedAt)
	}

	earlier := reassessCommandAt(t, "trigger-earlier", "node-actual-elsewhere", reassessedAt.Add(-time.Hour))
	behind := handleOK(t, fixture, earlier)
	if behind.Outcome() != application.ReassessTriggerNotJudged ||
		behind.NotJudgedReason() != ports.NotJudgedTriggerPrecedesCurrentPosition {
		t.Fatalf("较早那次 = %q/%q, want TRIGGER_NOT_JUDGED/TRIGGER_PRECEDES_CURRENT_POSITION",
			behind.Outcome(), behind.NotJudgedReason())
	}
	if fixture.store.saved != 1 || len(fixture.applicability.saved) != 0 {
		t.Fatalf("复核 %d 条、适用性写 %d 次, want 1 与 0——较早的位置不得据以失效", fixture.store.saved, len(fixture.applicability.saved))
	}
	noted := fixture.store.notJudged[value(t, domain.NewRequestCorrelationID, "trigger-earlier")]
	if !noted.CurrentPositionAt.Equal(reassessedAt) || !noted.TriggerOccurredAt.Equal(reassessedAt.Add(-time.Hour)) {
		t.Fatalf("未判登记 = %+v, want 当前位置 %s、触发 %s", noted, reassessedAt, reassessedAt.Add(-time.Hour))
	}

	// 再投（同一触发另一封信封）同答，不判、不多一行。
	if again := handleOK(t, fixture, earlier); again.Outcome() != application.ReassessTriggerNotJudged {
		t.Fatalf("再投 = %q, want TRIGGER_NOT_JUDGED", again.Outcome())
	}
	if fixture.store.saved != 1 || len(fixture.store.notJudged) != 1 {
		t.Fatalf("再投之后复核 %d 条、未判 %d 行, want 1 与 1", fixture.store.saved, len(fixture.store.notJudged))
	}
}

// Covers: AT-NR-081——较新的触发照判；与当前位置同一时刻的触发不算更早，也照判（两次控制同一时刻没有先后可比）。已判过的
// 触发后来被更晚的复核超过，再投仍读回原结果，不改答「早于当前位置」。
func TestNewerAndSimultaneousTriggersAreJudgedAndAJudgedTriggerKeepsItsResult(t *testing.T) {
	fixture := newReassessFixture(t)
	record := currentPlanRecord(t)
	fixture.routes.records[record.Key] = record
	fixture.evidence.byParcel["parcel-1"] = evidenceBeforeFreeze(t)

	first := reassessCommandAt(t, "trigger-first", "node-origin", reassessedAt)
	if result := handleOK(t, fixture, first); result.Outcome() != application.ReassessedStillApplicable {
		t.Fatalf("第一次 = %q, want STILL_APPLICABLE", result.Outcome())
	}
	if result := handleOK(t, fixture, reassessCommandAt(t, "trigger-same-instant", "node-origin", reassessedAt)); result.Outcome() != application.ReassessedStillApplicable {
		t.Fatalf("同一时刻 = %q, want STILL_APPLICABLE——同一时刻不算更早", result.Outcome())
	}
	if result := handleOK(t, fixture, reassessCommandAt(t, "trigger-newer", "node-origin", reassessedAt.Add(time.Hour))); result.Outcome() != application.ReassessedStillApplicable {
		t.Fatalf("较新 = %q, want STILL_APPLICABLE", result.Outcome())
	}
	if replay := handleOK(t, fixture, first); replay.Outcome() != application.ReassessedStillApplicable {
		t.Fatalf("第一次再投 = %q, want 读回原结果 STILL_APPLICABLE", replay.Outcome())
	}
	if fixture.store.saved != 3 || len(fixture.store.notJudged) != 0 {
		t.Fatalf("复核 %d 条、未判 %d 行, want 3 与 0", fixture.store.saved, len(fixture.store.notJudged))
	}
}

// Covers: AT-NR-082——乱序到达有确定答案：较早的触发若在较新的触发之后才到，入账不判；两种到达顺序下，最后一次已判的都是
// 业务时间较晚的那一次，包裹的当前位置不倒退。读不出当前位置是依赖故障：答登记册那一格的未决，回滚重投，不跳过比较。
func TestOutOfOrderArrivalHasADeterminedAnswer(t *testing.T) {
	latestJudged := func(fixture *reassessFixture) string {
		var latest ports.ReassessmentRecord
		for _, record := range fixture.store.byCorrelation {
			if record.TriggerOccurredAt.After(latest.TriggerOccurredAt) {
				latest = record
			}
		}
		return latest.Correlation.String()
	}
	for name, order := range map[string][]string{
		"按业务时间到达": {"trigger-intake", "trigger-handover"},
		"较早的后到":   {"trigger-handover", "trigger-intake"},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newReassessFixture(t)
			record := currentPlanRecord(t)
			fixture.routes.records[record.Key] = record
			fixture.evidence.byParcel["parcel-1"] = evidenceBeforeFreeze(t)
			commands := map[string]application.ReassessRouteCommand{
				"trigger-intake":   reassessCommandAt(t, "trigger-intake", "node-origin", reassessedAt),
				"trigger-handover": reassessCommandAt(t, "trigger-handover", "node-origin", reassessedAt.Add(4*time.Hour)),
			}
			for _, correlation := range order {
				handleOK(t, fixture, commands[correlation])
			}
			if got := latestJudged(fixture); got != "trigger-handover" {
				t.Fatalf("最后一次已判 = %q, want trigger-handover", got)
			}
			_, intakeJudged := fixture.store.byCorrelation[value(t, domain.NewRequestCorrelationID, "trigger-intake")]
			_, intakeBehind := fixture.store.notJudged[value(t, domain.NewRequestCorrelationID, "trigger-intake")]
			if intakeJudged == intakeBehind {
				t.Fatalf("较早那次判了 = %v、入账未判 = %v, want 二者恰居其一", intakeJudged, intakeBehind)
			}
		})
	}

	unreadable := newReassessFixture(t)
	unreadable.routes.records[currentPlanRecord(t).Key] = currentPlanRecord(t)
	unreadable.evidence.byParcel["parcel-1"] = evidenceBeforeFreeze(t)
	unreadable.store.latestErr = errors.New("pool closed")
	stalled := handleOK(t, unreadable, reassessCommand(t, "node-origin"))
	if stalled.Outcome() != application.ReassessUndecided || stalled.UndecidedReason() != application.ReassessStoreUnavailable {
		t.Fatalf("读不出当前位置 = %q/%q, want UNDECIDED/REASSESS_STORE_UNAVAILABLE", stalled.Outcome(), stalled.UndecidedReason())
	}
}
