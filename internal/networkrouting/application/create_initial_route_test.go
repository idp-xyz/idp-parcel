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

var routeJudgedAt = time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)

func handoffSpec(t *testing.T, parcels ...string) domain.RouteHandoffSpec {
	t.Helper()
	declared := make([]domain.DeclaredParcelID, 0, len(parcels))
	for _, parcel := range parcels {
		declared = append(declared, value(t, domain.NewDeclaredParcelID, parcel))
	}
	return domain.RouteHandoffSpec{
		Correlation:        value(t, domain.NewRequestCorrelationID, "route-handoff-1"),
		TenantID:           value(t, domain.NewTenantID, "tenant-1"),
		CustomerAccountID:  value(t, domain.NewCustomerAccountID, "customer-1"),
		ShipmentRequestID:  value(t, domain.NewShipmentRequestID, "request-1"),
		AcceptanceDecision: value(t, domain.NewAcceptanceDecisionReference, "decision-1"),
		AcceptanceBaseline: value(t, domain.NewAcceptanceBaselineReference, "baseline-1/v1"),
		Parcels:            declared,
		AcceptedAt:         routeJudgedAt.Add(-time.Hour),
	}
}

func routeCommand(t *testing.T, parcels ...string) application.CreateInitialRouteCommand {
	t.Helper()
	return application.CreateInitialRouteCommand{
		Handoff: handoffSpec(t, parcels...),
		Purpose: value(t, domain.NewServicePurpose, "NETWORK_SERVICE"),
	}
}

func routableEvidence(t *testing.T) ports.InitialRouteEvidence {
	t.Helper()
	window, err := domain.NewPlannedTimeWindow(routeJudgedAt, routeJudgedAt.Add(48*time.Hour),
		value(t, domain.NewWindowBasisReference, "CALENDAR-V1/BUFFER-V1"))
	if err != nil {
		t.Fatalf("new window: %v", err)
	}
	leg, err := domain.NewPlannedLeg(domain.PlannedLegSpec{
		From:        value(t, domain.NewPlanNodeReference, "node-origin"),
		To:          value(t, domain.NewPlanNodeReference, "node-destination"),
		Responsible: value(t, domain.NewResponsiblePartyReference, "party-1"),
		Window:      window,
	})
	if err != nil {
		t.Fatalf("new leg: %v", err)
	}
	score, err := domain.NewCriterionScore(value(t, domain.NewRankingCriterion, "TIMELINESS"), 1)
	if err != nil {
		t.Fatalf("new score: %v", err)
	}
	scores, err := domain.NewCandidateScores(value(t, domain.NewCandidateID, "candidate-1"),
		[]domain.CriterionScore{score})
	if err != nil {
		t.Fatalf("new candidate scores: %v", err)
	}
	return ports.InitialRouteEvidence{
		ServiceAreas: coveringAreas(t, "candidate-1"),
		Scores:       []domain.CandidateScores{scores},
		Priority:     []domain.RankingCriterion{value(t, domain.NewRankingCriterion, "TIMELINESS")},
		Paths: []ports.CandidatePath{{
			Candidate: value(t, domain.NewCandidateID, "candidate-1"),
			Legs:      []domain.PlannedLeg{leg},
		}},
		Strategy:     value(t, domain.NewRouteStrategyReference, "strategy-1/v1"),
		ViewRevision: value(t, domain.NewNetworkViewRevision, "net-view-rev-1"),
	}
}

func unroutableEvidence(t *testing.T) ports.InitialRouteEvidence {
	t.Helper()
	excluded, err := domain.NewServiceAreaResolution(domain.ServiceAreaResolutionSpec{
		Candidate:   value(t, domain.NewCandidateID, "candidate-1"),
		Outcome:     domain.AreaExcludesDestination,
		AreaVersion: value(t, domain.NewServiceAreaVersionReference, "AREA-V1"),
	})
	if err != nil {
		t.Fatalf("new excluding resolution: %v", err)
	}
	evidence := routableEvidence(t)
	evidence.ServiceAreas = []domain.ServiceAreaResolution{excluded}
	return evidence
}

type applicabilityDouble struct {
	eligibility domain.NetworkEligibility
	err         error
	asked       int
}

func (double *applicabilityDouble) AssessRoutingApplicability(
	_ context.Context,
	_ domain.InitialRouteJudgmentKey,
) (domain.NetworkEligibility, error) {
	double.asked++
	if double.err != nil {
		return domain.NetworkEligibility{}, double.err
	}
	return double.eligibility, nil
}

type routeEvidenceDouble struct {
	byParcel map[string]ports.InitialRouteEvidence
	// sequence 按次弹出，弹尽停在最后一份——演练「选出后、提交前视图换代」要让两次
	// 取回答出不同的修订。
	sequence map[string][]ports.InitialRouteEvidence
	errs     map[string]error
	loaded   int
}

func (double *routeEvidenceDouble) LoadInitialRouteEvidence(
	_ context.Context,
	key domain.InitialRouteJudgmentKey,
) (ports.InitialRouteEvidence, error) {
	double.loaded++
	parcel := key.DeclaredParcelID.String()
	if err, present := double.errs[parcel]; present {
		return ports.InitialRouteEvidence{}, err
	}
	if queued, present := double.sequence[parcel]; present && len(queued) > 0 {
		next := queued[0]
		if len(queued) > 1 {
			double.sequence[parcel] = queued[1:]
		}
		return next, nil
	}
	return double.byParcel[parcel], nil
}

type routeStoreDouble struct {
	records     map[domain.InitialRouteJudgmentKey]ports.InitialRouteRecord
	saveOutcome map[string]ports.InitialRouteSaveOutcome
	saved       int
}

func newRouteStore() *routeStoreDouble {
	return &routeStoreDouble{
		records:     map[domain.InitialRouteJudgmentKey]ports.InitialRouteRecord{},
		saveOutcome: map[string]ports.InitialRouteSaveOutcome{},
	}
}

func (double *routeStoreDouble) FindByKey(
	_ context.Context,
	key domain.InitialRouteJudgmentKey,
) (ports.InitialRouteRecord, bool, error) {
	record, found := double.records[key]
	return record, found, nil
}

func (double *routeStoreDouble) Save(
	_ context.Context,
	record ports.InitialRouteRecord,
) (ports.InitialRouteSaveOutcome, error) {
	if outcome, present := double.saveOutcome[record.Key.DeclaredParcelID.String()]; present {
		return outcome, nil
	}
	double.saved++
	double.records[record.Key] = record
	return ports.InitialRouteSaved, nil
}

type handoffLogDouble struct {
	digests map[domain.RequestCorrelationID]string
	findErr error
}

func newHandoffLog() *handoffLogDouble {
	return &handoffLogDouble{digests: map[domain.RequestCorrelationID]string{}}
}

func (double *handoffLogDouble) FindDigest(
	_ context.Context,
	_ domain.TenantID,
	correlation domain.RequestCorrelationID,
) (string, bool, error) {
	if double.findErr != nil {
		return "", false, double.findErr
	}
	digest, found := double.digests[correlation]
	return digest, found, nil
}

func (double *handoffLogDouble) Append(
	_ context.Context,
	_ domain.TenantID,
	correlation domain.RequestCorrelationID,
	digest string,
) error {
	double.digests[correlation] = digest
	return nil
}

type routeDownstreamDouble struct {
	intents []ports.InitialRouteHandoffIntent
	err     error
}

func (double *routeDownstreamDouble) HandOffInitialRoute(
	_ context.Context,
	intent ports.InitialRouteHandoffIntent,
) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

type routeIdentityDouble struct{ next int }

func (double *routeIdentityDouble) NextRoutePlanVersionID(_ context.Context) (domain.RoutePlanVersionID, error) {
	double.next++
	return domain.NewRoutePlanVersionID("plan-" + string(rune('0'+double.next)))
}

type routeFixture struct {
	handler       *application.CreateInitialRouteHandler
	applicability *applicabilityDouble
	evidence      *routeEvidenceDouble
	store         *routeStoreDouble
	log           *handoffLogDouble
	downstream    *routeDownstreamDouble
}

func newRouteFixture(t *testing.T) *routeFixture {
	t.Helper()
	eligibility, err := domain.NewNetworkEligibility(domain.NetworkJudgmentRequired, domain.EligibilityBasisReference{})
	if err != nil {
		t.Fatalf("new eligibility: %v", err)
	}
	fixture := &routeFixture{
		applicability: &applicabilityDouble{eligibility: eligibility},
		evidence:      &routeEvidenceDouble{byParcel: map[string]ports.InitialRouteEvidence{}, errs: map[string]error{}},
		store:         newRouteStore(),
		log:           newHandoffLog(),
		downstream:    &routeDownstreamDouble{},
	}
	fixture.handler = application.NewCreateInitialRouteHandler(application.CreateInitialRouteDeps{
		Applicability: fixture.applicability,
		Evidence:      fixture.evidence,
		Store:         fixture.store,
		Log:           fixture.log,
		Downstream:    fixture.downstream,
		Identities:    &routeIdentityDouble{},
		Clock:         fixedClock{at: routeJudgedAt},
	})
	return fixture
}

// Covers: `AT-NR-012`「同一委托三个包裹分别可路由、确定无路由和依赖未决——三个包裹分别
// 保留计划、无路由和未决结果；任一结果不回滚或掩盖其他结果」，兼 `AT-NR-001`（计划保留
// 全部候选依据）与 `AT-NR-005`（无路由保存逐候选淘汰依据）的编排半边。
func TestThreeParcelsKeepThreeIndependentResults(t *testing.T) {
	fixture := newRouteFixture(t)
	fixture.evidence.byParcel["parcel-1"] = routableEvidence(t)
	fixture.evidence.byParcel["parcel-2"] = unroutableEvidence(t)
	fixture.evidence.errs["parcel-3"] = errors.New("evidence down")

	result, err := fixture.handler.Handle(context.Background(), routeCommand(t, "parcel-1", "parcel-2", "parcel-3"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.RouteHandoffProcessed {
		t.Fatalf("outcome = %q, want PROCESSED", result.Outcome())
	}
	parcels := result.Parcels()
	if len(parcels) != 3 {
		t.Fatalf("parcels = %d, want 3", len(parcels))
	}

	if parcels[0].Outcome() != application.ParcelRouteFormed {
		t.Fatalf("parcel-1 = %q, want ROUTE_FORMED", parcels[0].Outcome())
	}
	plan, present := parcels[0].Plan()
	if !present || plan.SelectedCandidate().String() != "candidate-1" {
		t.Fatalf("plan = %#v present = %v", plan, present)
	}
	if len(plan.Nodes()) != 2 {
		t.Fatalf("nodes = %v", plan.Nodes())
	}

	if parcels[1].Outcome() != application.ParcelNoCurrentRoute {
		t.Fatalf("parcel-2 = %q, want NO_CURRENT_ROUTE", parcels[1].Outcome())
	}
	noRoute, present := parcels[1].NoCurrentRoute()
	if !present || len(noRoute.Candidates()) != 1 {
		t.Fatalf("no-route = %#v present = %v; 逐候选淘汰依据必须随判断保全", noRoute, present)
	}

	if parcels[2].Outcome() != application.ParcelRouteUndecided ||
		parcels[2].UndecidedReason() != application.RouteEvidenceUnavailable {
		t.Fatalf("parcel-3 = %q/%q, want UNDECIDED/ROUTE_EVIDENCE_UNAVAILABLE",
			parcels[2].Outcome(), parcels[2].UndecidedReason())
	}
	if parcels[2].ContinuationReference().String() == "" {
		t.Fatal("未决无法安全续办")
	}
	if fixture.store.saved != 2 {
		t.Fatalf("saved = %d, want 2——未决不落库", fixture.store.saved)
	}
	if len(fixture.downstream.intents) != 2 {
		t.Fatalf("intents = %d, want 2——计划与无路由各交一份意图", len(fixture.downstream.intents))
	}
}

// Covers: `AT-NR-003`「同一接受交接以相同内容重复投递——返回已有包裹结果，不创建第二个
// 并行有效计划」与步骤 2 的冲突分界：同关联异内容是冲突，原交接与原结果不被覆盖。
func TestAReplayReturnsExistingResultsAndAConflictOverwritesNothing(t *testing.T) {
	fixture := newRouteFixture(t)
	fixture.evidence.byParcel["parcel-1"] = routableEvidence(t)

	first, err := fixture.handler.Handle(context.Background(), routeCommand(t, "parcel-1"))
	if err != nil {
		t.Fatalf("first handle: %v", err)
	}
	firstPlan, _ := first.Parcels()[0].Plan()
	loadedAfterFirst := fixture.evidence.loaded

	replay, err := fixture.handler.Handle(context.Background(), routeCommand(t, "parcel-1"))
	if err != nil {
		t.Fatalf("replay handle: %v", err)
	}
	if replay.Parcels()[0].Outcome() != application.ParcelExistingResult {
		t.Fatalf("replay = %q, want EXISTING_RESULT", replay.Parcels()[0].Outcome())
	}
	replayPlan, _ := replay.Parcels()[0].Plan()
	if replayPlan.Version() != firstPlan.Version() {
		t.Fatal("重放交回了另一个计划版本")
	}
	if fixture.evidence.loaded != loadedAfterFirst {
		t.Fatal("重放重新取证据评估了一遍")
	}
	if fixture.store.saved != 1 {
		t.Fatalf("saved = %d; 重放不得形成第二个并行计划", fixture.store.saved)
	}

	conflicting := routeCommand(t, "parcel-1", "parcel-9")
	conflict, err := fixture.handler.Handle(context.Background(), conflicting)
	if err != nil {
		t.Fatalf("conflict handle: %v", err)
	}
	if conflict.Outcome() != application.RouteHandoffConflict {
		t.Fatalf("outcome = %q, want CONFLICT", conflict.Outcome())
	}
	if fixture.store.saved != 1 {
		t.Fatal("冲突覆盖了原结果")
	}
}

// Covers: `AT-NR-004`「同一包裹发生并发初始路由计算——只有一个结果越过提交边界；其他
// 处理返回已有结果」：写入答`已有记录`时读回赢家作答，不覆盖。
func TestAConcurrentLoserReadsBackTheWinner(t *testing.T) {
	fixture := newRouteFixture(t)
	fixture.evidence.byParcel["parcel-1"] = routableEvidence(t)
	fixture.store.saveOutcome["parcel-1"] = ports.InitialRouteAlreadyRecorded

	winner := ports.InitialRouteRecord{HasNoRoute: true}
	judgment, err := domain.FormNoCurrentRouteJudgment(domain.NoCurrentRouteJudgmentSpec{
		Key: domain.InitialRouteJudgmentKey{
			TenantID:           value(t, domain.NewTenantID, "tenant-1"),
			CustomerAccountID:  value(t, domain.NewCustomerAccountID, "customer-1"),
			ShipmentRequestID:  value(t, domain.NewShipmentRequestID, "request-1"),
			AcceptanceBaseline: value(t, domain.NewAcceptanceBaselineReference, "baseline-1/v1"),
			DeclaredParcelID:   value(t, domain.NewDeclaredParcelID, "parcel-1"),
			ServicePurpose:     value(t, domain.NewServicePurpose, "NETWORK_SERVICE"),
		},
		Candidates: []domain.RouteCandidate{
			eliminated(t, "candidate-1", "HARD_CONSTRAINT_RESTRICTION/CUSTOMS-1"),
		},
		Strategy:     value(t, domain.NewRouteStrategyReference, "strategy-1/v1"),
		ViewRevision: value(t, domain.NewNetworkViewRevision, "net-view-rev-1"),
		JudgedAt:     routeJudgedAt,
	})
	if err != nil {
		t.Fatalf("form winner judgment: %v", err)
	}
	winner.Key = judgment.Key()
	winner.NoRoute = judgment
	fixture.store.records[judgment.Key()] = winner

	result, err := fixture.handler.Handle(context.Background(), routeCommand(t, "parcel-1"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	parcel := result.Parcels()[0]
	if parcel.Outcome() != application.ParcelExistingResult {
		t.Fatalf("outcome = %q, want EXISTING_RESULT——败方读回赢家", parcel.Outcome())
	}
	if _, present := parcel.NoCurrentRoute(); !present {
		t.Fatal("读回的不是赢家那份无路由判断")
	}
}

// 找回已有记录的路径不经过 Save；本测试的 records 预置正是并发赢家已提交的样子。
func eliminated(t *testing.T, id, reason string) domain.RouteCandidate {
	t.Helper()
	candidate, err := domain.NewRouteCandidate(
		value(t, domain.NewCandidateID, id),
		domain.CandidateEliminated,
		value(t, domain.NewCandidateReason, reason),
	)
	if err != nil {
		t.Fatalf("new eliminated candidate: %v", err)
	}
	return candidate
}

// Covers: `AT-NR-010`「候选选出后、提交前适用线路被有效关闭——原候选不得提交为当前
// 计划；使用新依据重新判断，不能完成时保持未决」。第一份证据选出候选，提交前重读发现
// 修订换代且线路已被排除：提交的是按新依据重判的`无当前有效路由`，不是旧计划。
func TestAPreCommitRevisionChangeRejudgesWithTheNewEvidence(t *testing.T) {
	fixture := newRouteFixture(t)
	closed := unroutableEvidence(t)
	closed.ViewRevision = value(t, domain.NewNetworkViewRevision, "net-view-rev-2")
	fixture.evidence.sequence = map[string][]ports.InitialRouteEvidence{
		"parcel-1": {routableEvidence(t), closed, closed},
	}

	result, err := fixture.handler.Handle(context.Background(), routeCommand(t, "parcel-1"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	parcel := result.Parcels()[0]
	if parcel.Outcome() != application.ParcelNoCurrentRoute {
		t.Fatalf("outcome = %q, want NO_CURRENT_ROUTE——旧候选不得提交为当前计划", parcel.Outcome())
	}
	noRoute, present := parcel.NoCurrentRoute()
	if !present || noRoute.ViewRevision().String() != "net-view-rev-2" {
		t.Fatalf("no-route revision = %#v present = %v; 提交的必须是按新依据的重判", noRoute, present)
	}
	if fixture.store.saved != 1 {
		t.Fatalf("saved = %d, want 1", fixture.store.saved)
	}
}

// Covers: `AT-NR-010` 的另半句「不能完成时保持未决」——视图连续滚动时每轮重判都在提交
// 前再次失配，两轮后停在未决而不是追着一个动的目标提交。
func TestARollingViewKeepsTheJudgmentUndecided(t *testing.T) {
	fixture := newRouteFixture(t)
	second := routableEvidence(t)
	second.ViewRevision = value(t, domain.NewNetworkViewRevision, "net-view-rev-2")
	third := routableEvidence(t)
	third.ViewRevision = value(t, domain.NewNetworkViewRevision, "net-view-rev-3")
	fixture.evidence.sequence = map[string][]ports.InitialRouteEvidence{
		"parcel-1": {routableEvidence(t), second, third},
	}

	result, err := fixture.handler.Handle(context.Background(), routeCommand(t, "parcel-1"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	parcel := result.Parcels()[0]
	if parcel.Outcome() != application.ParcelRouteUndecided ||
		parcel.UndecidedReason() != application.RouteEvidenceSuperseded {
		t.Fatalf("outcome = %q/%q, want UNDECIDED/ROUTE_EVIDENCE_SUPERSEDED",
			parcel.Outcome(), parcel.UndecidedReason())
	}
	if fixture.store.saved != 0 {
		t.Fatal("追着滚动的视图提交了结果")
	}
	if len(fixture.downstream.intents) != 0 {
		t.Fatal("未决发布了意图")
	}
}

// Covers: `AT-NR-009`「仅面单渠道服务，运营企业不控制端到端网络——返回不适用并保留产品
// 依据，不虚构节点」；读不回适用性形成未决而不是`不适用`。
func TestAWaybillOnlyServiceIsNotApplicableWithItsBasis(t *testing.T) {
	fixture := newRouteFixture(t)
	basis, err := domain.NewEligibilityBasisReference("PRODUCT-WAYBILL-ONLY")
	if err != nil {
		t.Fatalf("new basis: %v", err)
	}
	notRequired, err := domain.NewNetworkEligibility(domain.NetworkJudgmentNotRequired, basis)
	if err != nil {
		t.Fatalf("new eligibility: %v", err)
	}
	fixture.applicability.eligibility = notRequired

	result, err := fixture.handler.Handle(context.Background(), routeCommand(t, "parcel-1"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.RouteHandoffNotApplicable {
		t.Fatalf("outcome = %q, want NOT_APPLICABLE", result.Outcome())
	}
	if result.ApplicabilityBasis().String() != "PRODUCT-WAYBILL-ONLY" {
		t.Fatal("不适用没带产品依据")
	}
	if fixture.evidence.loaded != 0 {
		t.Fatal("不适用还去取了网络证据")
	}

	fixture.applicability.err = errors.New("commercial down")
	undecided, err := fixture.handler.Handle(context.Background(), routeCommand(t, "parcel-1"))
	if err != nil {
		t.Fatalf("handle unavailable: %v", err)
	}
	if undecided.Outcome() != application.RouteHandoffUndecided ||
		undecided.UndecidedReason() != application.RoutingApplicabilityUnavailable {
		t.Fatalf("outcome = %q/%q; 读不回不得读成不适用", undecided.Outcome(), undecided.UndecidedReason())
	}
}
