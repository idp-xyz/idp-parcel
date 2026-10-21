package application_test

import (
	"context"
	"errors"
	"fmt"
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
		Handoff:    handoffSpec(t, parcels...),
		Purpose:    value(t, domain.NewServicePurpose, "NETWORK_SERVICE"),
		Resolution: value(t, domain.NewCommercialResolutionReference, "RES-test-1"),
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
	return ports.InitialRouteEvidence{
		ServiceAreas:   coveringAreas(t, "candidate-1"),
		RankingForm:    domain.CostSingleDimensionRanking,
		CandidateCosts: []domain.CandidateCostFact{pricedCandidateCost(t, "candidate-1", 1000, "CNY")},
		Paths: []ports.CandidatePath{{
			Candidate: value(t, domain.NewCandidateID, "candidate-1"),
			Legs:      []domain.PlannedLeg{leg},
		}},
		Strategy:     value(t, domain.NewRouteStrategyReference, "strategy-1/v1"),
		ViewRevision: value(t, domain.NewNetworkViewRevision, "net-view-rev-1"),
	}
}

func pricedCandidateCost(t *testing.T, candidate string, amountMinor int64, currency string) domain.CandidateCostFact {
	t.Helper()
	fact, err := domain.NewPricedCandidateCost(value(t, domain.NewCandidateID, candidate), amountMinor, currency)
	if err != nil {
		t.Fatalf("new priced candidate cost: %v", err)
	}
	return fact
}

// tiedEvidence 让两条都可行的候选在成本单维下同价。
func cheaperAlternative(t *testing.T, amountMinor int64) ports.InitialRouteEvidence {
	t.Helper()
	evidence := routableEvidence(t)
	evidence.ServiceAreas = coveringAreas(t, "candidate-1", "candidate-2")
	evidence.CandidateCosts = []domain.CandidateCostFact{
		pricedCandidateCost(t, "candidate-1", 1000, "CNY"),
		pricedCandidateCost(t, "candidate-2", amountMinor, "CNY"),
	}
	evidence.Paths = append(evidence.Paths, ports.CandidatePath{
		Candidate: value(t, domain.NewCandidateID, "candidate-2"),
		Legs:      evidence.Paths[0].Legs,
	})
	return evidence
}

func tiedEvidence(t *testing.T) ports.InitialRouteEvidence {
	t.Helper()
	evidence := routableEvidence(t)
	evidence.ServiceAreas = coveringAreas(t, "candidate-1", "candidate-2")
	evidence.CandidateCosts = []domain.CandidateCostFact{
		pricedCandidateCost(t, "candidate-1", 4200, "CNY"),
		pricedCandidateCost(t, "candidate-2", 4200, "CNY"),
	}
	evidence.Paths = append(evidence.Paths, ports.CandidatePath{
		Candidate: value(t, domain.NewCandidateID, "candidate-2"),
		Legs:      evidence.Paths[0].Legs,
	})
	return evidence
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
	_ domain.CommercialResolutionReference,
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
	// unconfigured 按包裹标明「网络定义未配置」，与 errs 分开：那一格是依赖
	// 不可用，这一格是还没人登记过网络（ADR-0052）。
	unconfigured map[string]bool
	loaded       int
	// carried 记下最近一次取证据时交进来的携带内容。
	carried ports.RequestCarriedContent
}

func (double *routeEvidenceDouble) LoadInitialRouteEvidence(
	_ context.Context,
	key domain.InitialRouteJudgmentKey,
	carried ports.RequestCarriedContent,
) (ports.InitialRouteEvidence, bool, error) {
	double.loaded++
	double.carried = carried
	parcel := key.DeclaredParcelID.String()
	if err, present := double.errs[parcel]; present {
		return ports.InitialRouteEvidence{}, false, err
	}
	if double.unconfigured[parcel] {
		return ports.InitialRouteEvidence{}, false, nil
	}
	if queued, present := double.sequence[parcel]; present && len(queued) > 0 {
		next := queued[0]
		if len(queued) > 1 {
			double.sequence[parcel] = queued[1:]
		}
		return next, true, nil
	}
	return double.byParcel[parcel], true, nil
}

type routeStoreDouble struct {
	records     map[domain.InitialRouteJudgmentKey]ports.InitialRouteRecord
	saveOutcome map[string]ports.InitialRouteSaveOutcome
	saved       int
	findErr     error
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
	if double.findErr != nil {
		return ports.InitialRouteRecord{}, false, double.findErr
	}
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

// notFormedDouble 是未形成登记的替身：按判断键记最新一次，err 非空时写不进。
type notFormedDouble struct {
	records map[domain.InitialRouteJudgmentKey]ports.InitialRouteNotFormed
	writes  int
	err     error
}

func newNotFormed() *notFormedDouble {
	return &notFormedDouble{records: map[domain.InitialRouteJudgmentKey]ports.InitialRouteNotFormed{}}
}

func (double *notFormedDouble) RecordNotFormed(_ context.Context, record ports.InitialRouteNotFormed) error {
	if double.err != nil {
		return double.err
	}
	double.writes++
	double.records[record.Key] = record
	return nil
}

// only 交回唯一那一行；不是恰一行即失败。
func (double *notFormedDouble) only(t *testing.T) ports.InitialRouteNotFormed {
	t.Helper()
	if len(double.records) != 1 {
		t.Fatalf("未形成登记 %d 行, want 1", len(double.records))
	}
	for _, record := range double.records {
		return record
	}
	panic("unreachable")
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
	notFormed     *notFormedDouble
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
		evidence: &routeEvidenceDouble{
			byParcel:     map[string]ports.InitialRouteEvidence{},
			errs:         map[string]error{},
			unconfigured: map[string]bool{},
		},
		store:      newRouteStore(),
		notFormed:  newNotFormed(),
		log:        newHandoffLog(),
		downstream: &routeDownstreamDouble{},
	}
	fixture.handler = application.NewCreateInitialRouteHandler(application.CreateInitialRouteDeps{
		Applicability: fixture.applicability,
		Evidence:      fixture.evidence,
		Costs:         passthroughCosts{},
		Store:         fixture.store,
		NotFormed:     fixture.notFormed,
		Log:           fixture.log,
		Downstream:    fixture.downstream,
		Identities:    &routeIdentityDouble{},
		Clock:         fixedClock{at: routeJudgedAt},
	})
	return fixture
}

// passthroughCosts 是既有编排双轴测试的白盒替身：证据里带什么事实就交回什么事实，
// 不另合成——取数侧的合成正交地被其自身用例盖住，这里不重测一遍。
type passthroughCosts struct{}

func (passthroughCosts) LoadCandidateCosts(
	_ context.Context,
	_ domain.InitialRouteJudgmentKey,
	evidence ports.InitialRouteEvidence,
	_ ports.RequestCarriedContent,
) (ports.RouteCandidateCosts, error) {
	return ports.RouteCandidateCosts{Facts: evidence.CandidateCosts}, nil
}

// unconfiguredCosts 是候选成本取数侧没接计价输入时的如实形态（生产装配今天就是这一格）。
type unconfiguredCosts struct{}

func (unconfiguredCosts) LoadCandidateCosts(
	context.Context, domain.InitialRouteJudgmentKey, ports.InitialRouteEvidence, ports.RequestCarriedContent,
) (ports.RouteCandidateCosts, error) {
	return ports.RouteCandidateCosts{}, ports.ErrRouteCostSourceNotConfigured
}

// Covers: 证据齐、候选合格，而成本取数侧没接计价输入——包裹落`路由未形成` `COST_SOURCE_NOT_CONFIGURED`：不拿缺依据
// 的候选出价，不落判断库，不交下游意图（ADR-0175 Consequences「成本缺席时不能形成计划」）；没接是等部署，重投推不动，
// 原因落未形成登记（ADR-0273 决定二）。
func TestAnUnconfiguredCostSourceSettlesTheParcelAsNotFormed(t *testing.T) {
	fixture := newRouteFixture(t)
	fixture.handler = application.NewCreateInitialRouteHandler(application.CreateInitialRouteDeps{
		Applicability: fixture.applicability,
		Evidence:      fixture.evidence,
		Costs:         unconfiguredCosts{},
		Store:         fixture.store,
		NotFormed:     fixture.notFormed,
		Log:           fixture.log,
		Downstream:    fixture.downstream,
		Identities:    &routeIdentityDouble{},
		Clock:         fixedClock{at: routeJudgedAt},
	})
	fixture.evidence.byParcel["parcel-1"] = routableEvidence(t)

	result, err := fixture.handler.Handle(context.Background(), routeCommand(t, "parcel-1"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	parcels := result.Parcels()
	if len(parcels) != 1 || parcels[0].Outcome() != application.ParcelRouteNotFormed ||
		parcels[0].NotFormedReason() != ports.NotFormedCostSourceNotConfigured {
		t.Fatalf("parcels = %+v，想要 NOT_FORMED/COST_SOURCE_NOT_CONFIGURED", parcels)
	}
	if fixture.store.saved != 0 || len(fixture.downstream.intents) != 0 {
		t.Fatalf("saved = %d intents = %d：未形成不落判断库、不交意图", fixture.store.saved, len(fixture.downstream.intents))
	}
	if record := fixture.notFormed.only(t); record.Reason != ports.NotFormedCostSourceNotConfigured {
		t.Fatalf("未形成登记原因 = %s, want COST_SOURCE_NOT_CONFIGURED", record.Reason)
	}
}

// carriedRecordingCosts 照 passthroughCosts 交回证据里的事实，另记下每次交进来的携带内容。
type carriedRecordingCosts struct {
	carried []ports.RequestCarriedContent
}

func (costs *carriedRecordingCosts) LoadCandidateCosts(
	_ context.Context,
	_ domain.InitialRouteJudgmentKey,
	evidence ports.InitialRouteEvidence,
	carried ports.RequestCarriedContent,
) (ports.RouteCandidateCosts, error) {
	costs.carried = append(costs.carried, carried)
	return ports.RouteCandidateCosts{Facts: evidence.CandidateCosts}, nil
}

// Covers: `AT-NR-070` 的编排半边（票 routing-first-cut/20，ADR-0189 决定一）——随请求携带的地理投影原样交到候选成本取数侧，
// 与证据视图收到的是同一份：绑了分区目录的卡要靠它里面的邮编路线解区，编排不另读地址，也不在两口之间换一份。
func TestTheCarriedProjectionReachesTheCostSource(t *testing.T) {
	fixture := newRouteFixture(t)
	costs := &carriedRecordingCosts{}
	fixture.handler = application.NewCreateInitialRouteHandler(application.CreateInitialRouteDeps{
		Applicability: fixture.applicability,
		Evidence:      fixture.evidence,
		Costs:         costs,
		Store:         fixture.store,
		NotFormed:     fixture.notFormed,
		Log:           fixture.log,
		Downstream:    fixture.downstream,
		Identities:    &routeIdentityDouble{},
		Clock:         fixedClock{at: routeJudgedAt},
	})
	fixture.evidence.byParcel["parcel-1"] = routableEvidence(t)
	command := routeCommand(t, "parcel-1")
	command.Carried = ports.RequestCarriedContent{Geo: domain.NewGeoResolutionProjection(
		domain.NewGeoResolutionSide("CN", true, "200000", true),
		domain.NewGeoResolutionSide("SG", true, "018000", true))}

	if _, err := fixture.handler.Handle(context.Background(), command); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(costs.carried) == 0 {
		t.Fatal("候选成本取数侧没被问到")
	}
	for _, carried := range costs.carried {
		if carried.Geo.ContentDigest() != command.Carried.Geo.ContentDigest() ||
			carried.Geo.ContentDigest() != fixture.evidence.carried.Geo.ContentDigest() {
			t.Fatalf("取数侧收到的投影 %s，想要与请求、证据视图同一份 %s", carried.Geo.ContentDigest(), command.Carried.Geo.ContentDigest())
		}
	}
}

// citingCosts 照 passthroughCosts 交回证据里的事实，另按候选交回各自逐段的出处：几条候选都出价时取数侧就这样交。
type citingCosts struct {
	citations []ports.CandidateLegCostCitation
}

func (costs citingCosts) LoadCandidateCosts(
	_ context.Context,
	_ domain.InitialRouteJudgmentKey,
	evidence ports.InitialRouteEvidence,
	_ ports.RequestCarriedContent,
) (ports.RouteCandidateCosts, error) {
	return ports.RouteCandidateCosts{Facts: evidence.CandidateCosts, Citations: costs.citations}, nil
}

// legCostCitations 给一条候选逐段造出处。评价标识带候选名，计划里留下的是谁的一眼认得出。
func legCostCitations(t *testing.T, candidate string, legs int) []ports.CandidateLegCostCitation {
	t.Helper()
	citations := make([]ports.CandidateLegCostCitation, 0, legs)
	for ordinal := 1; ordinal <= legs; ordinal++ {
		citation, err := domain.NewPlannedLegCostCitation(domain.PlannedLegCostCitationSpec{
			Ordinal:            ordinal,
			EvaluationID:       "EST-" + candidate + "-" + string(rune('0'+ordinal)),
			PlanReference:      "plan-" + candidate + "/v1",
			ComparisonAmount:   "5",
			ComparisonCurrency: "CNY",
		})
		if err != nil {
			t.Fatalf("new leg cost citation: %v", err)
		}
		citations = append(citations, ports.CandidateLegCostCitation{
			Candidate:              value(t, domain.NewCandidateID, candidate),
			PlannedLegCostCitation: citation,
		})
	}
	return citations
}

// legChain 造一条首尾相接的节点间段链，段数由调用方定。
func legChain(t *testing.T, legs int) []domain.PlannedLeg {
	t.Helper()
	window, err := domain.NewPlannedTimeWindow(routeJudgedAt, routeJudgedAt.Add(48*time.Hour),
		value(t, domain.NewWindowBasisReference, "CALENDAR-V1/BUFFER-V1"))
	if err != nil {
		t.Fatalf("new window: %v", err)
	}
	chain := make([]domain.PlannedLeg, 0, legs)
	from := "node-origin"
	for ordinal := 1; ordinal <= legs; ordinal++ {
		to := "node-" + string(rune('0'+ordinal))
		leg, err := domain.NewPlannedLeg(domain.PlannedLegSpec{
			From:        value(t, domain.NewPlanNodeReference, from),
			To:          value(t, domain.NewPlanNodeReference, to),
			Responsible: value(t, domain.NewResponsiblePartyReference, "party-1"),
			Window:      window,
		})
		if err != nil {
			t.Fatalf("new leg: %v", err)
		}
		chain = append(chain, leg)
		from = to
	}
	return chain
}

// Covers: `AT-NR-073`（票 demo-chain-gaps/105）——两条候选都已出价、选中便宜的那条：计划形成，逐段成本出处只有选中候选
// 那几段，段号按它的段链计。另一条的出处不随计划留痕：段数相同时它的第 1 段与选中候选的第 1 段同一个段号，段数更多时它
// 的末段还越过选中候选的段链，两样都会让计划形成拒收、整次处理报错。
func TestTwoPricedCandidatesFormAPlanCitingOnlyTheSelectedOne(t *testing.T) {
	for name, test := range map[string]struct{ selectedLegs, otherLegs int }{
		"段数相同":     {1, 1},
		"选中那条段数更少": {1, 2},
		"选中那条段数更多": {2, 1},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newRouteFixture(t)
			fixture.handler = application.NewCreateInitialRouteHandler(application.CreateInitialRouteDeps{
				Applicability: fixture.applicability,
				Evidence:      fixture.evidence,
				Costs: citingCosts{citations: append(
					legCostCitations(t, "candidate-1", test.otherLegs),
					legCostCitations(t, "candidate-2", test.selectedLegs)...)},
				Store:      fixture.store,
				NotFormed:  fixture.notFormed,
				Log:        fixture.log,
				Downstream: fixture.downstream,
				Identities: &routeIdentityDouble{},
				Clock:      fixedClock{at: routeJudgedAt},
			})
			evidence := routableEvidence(t)
			evidence.ServiceAreas = coveringAreas(t, "candidate-1", "candidate-2")
			evidence.CandidateCosts = []domain.CandidateCostFact{
				pricedCandidateCost(t, "candidate-1", 1000, "CNY"),
				pricedCandidateCost(t, "candidate-2", 400, "CNY"),
			}
			evidence.Paths = []ports.CandidatePath{
				{Candidate: value(t, domain.NewCandidateID, "candidate-1"), Legs: legChain(t, test.otherLegs)},
				{Candidate: value(t, domain.NewCandidateID, "candidate-2"), Legs: legChain(t, test.selectedLegs)},
			}
			fixture.evidence.byParcel["parcel-1"] = evidence

			result, err := fixture.handler.Handle(context.Background(), routeCommand(t, "parcel-1"))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			plan, formed := result.Parcels()[0].Plan()
			if !formed || plan.SelectedCandidate().String() != "candidate-2" {
				t.Fatalf("parcel = %+v，want 形成计划、选中便宜的 candidate-2", result.Parcels()[0])
			}
			citations := plan.CostCitations()
			if len(citations) != test.selectedLegs {
				t.Fatalf("计划出处 %d 条，want 选中候选的 %d 段各一条：%+v", len(citations), test.selectedLegs, citations)
			}
			for index, citation := range citations {
				if citation.Ordinal() != index+1 || citation.PlanReference() != "plan-candidate-2/v1" {
					t.Fatalf("计划出处第 %d 条 = %+v，want 段号 %d、出自选中候选", index+1, citation, index+1)
				}
			}
			if fixture.store.saved != 1 || len(fixture.downstream.intents) != 1 {
				t.Fatalf("saved = %d intents = %d，want 计划落库一次、交一份意图", fixture.store.saved, len(fixture.downstream.intents))
			}
		})
	}
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

// Covers: ADR-0052 的「未配置」格在初始路由这一侧——登记册未配置的包裹落`路由未形成`、原因与对象落未形成登记，
// 既不落判断库也不交意图，且**不与同一委托里其他包裹的结果互相掩盖**（`AT-NR-012`、`AT-NR-088`）。专格的意义在恢复
// 动作：`ROUTE_EVIDENCE_NOT_CONFIGURED` 要租户去登记网络定义，而 `ROUTE_EVIDENCE_UNAVAILABLE` 要运维去救依赖；混成
// 一格会把这两拨人都指错方向。
func TestAnUnconfiguredCatalogueStallsOnlyItsOwnParcel(t *testing.T) {
	fixture := newRouteFixture(t)
	fixture.evidence.byParcel["parcel-1"] = routableEvidence(t)
	fixture.evidence.unconfigured["parcel-2"] = true

	result, err := fixture.handler.Handle(context.Background(), routeCommand(t, "parcel-1", "parcel-2"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	parcels := result.Parcels()
	if len(parcels) != 2 {
		t.Fatalf("parcels = %d, want 2", len(parcels))
	}
	if parcels[0].Outcome() != application.ParcelRouteFormed {
		t.Fatalf("parcel-1 = %q; 另一个包裹的未配置掩盖了它", parcels[0].Outcome())
	}
	if parcels[1].Outcome() != application.ParcelRouteNotFormed ||
		parcels[1].NotFormedReason() != ports.NotFormedEvidenceNotConfigured {
		t.Fatalf("parcel-2 = %q/%q, want NOT_FORMED/ROUTE_EVIDENCE_NOT_CONFIGURED",
			parcels[1].Outcome(), parcels[1].NotFormedReason())
	}
	if _, present := parcels[1].NoCurrentRoute(); present {
		t.Fatal("未配置被判成了`无当前有效路由`——那是领域从全部候选确定性淘汰得出的判断")
	}
	if parcels[1].ContinuationReference().String() == "" {
		t.Fatal("未配置无法安全续办")
	}
	if fixture.store.saved != 1 {
		t.Fatalf("saved = %d, want 1——未配置不落判断库", fixture.store.saved)
	}
	record := fixture.notFormed.only(t)
	if record.Key != parcels[1].Key() || record.Reason != ports.NotFormedEvidenceNotConfigured ||
		record.Correlation.String() != "route-handoff-1" || !record.AcceptedAt.Equal(routeJudgedAt.Add(-time.Hour)) ||
		!record.SettledAt.Equal(routeJudgedAt) || len(record.Candidates) != 0 || len(record.Tied) != 0 {
		t.Fatalf("未形成登记 = %+v, want parcel-2、证据未配置、交接关联与接受时刻、不带候选", record)
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

// Covers: `AT-NR-002`「同一已接受委托包含两个包裹，因服务区域或限制不同应走不同路线
// ——分别形成两个包裹级计划；不存在覆盖所有成员的委托级执行路线」。两份证据各选各的
// 候选，两个计划版本各自独立。
func TestTwoParcelsFormTwoIndependentPlans(t *testing.T) {
	fixture := newRouteFixture(t)
	first := routableEvidence(t)
	second := routableEvidence(t)
	// 第二个包裹的证据指向另一条候选与另一条段链。
	second.ServiceAreas = coveringAreas(t, "candidate-2")
	second.CandidateCosts = []domain.CandidateCostFact{pricedCandidateCost(t, "candidate-2", 2000, "CNY")}
	window, err := domain.NewPlannedTimeWindow(routeJudgedAt, routeJudgedAt.Add(72*time.Hour),
		value(t, domain.NewWindowBasisReference, "CALENDAR-V1/BUFFER-V1"))
	if err != nil {
		t.Fatalf("new window: %v", err)
	}
	leg, err := domain.NewPlannedLeg(domain.PlannedLegSpec{
		From:        value(t, domain.NewPlanNodeReference, "node-origin"),
		To:          value(t, domain.NewPlanNodeReference, "node-alternate"),
		Responsible: value(t, domain.NewResponsiblePartyReference, "party-2"),
		Window:      window,
	})
	if err != nil {
		t.Fatalf("new leg: %v", err)
	}
	second.Paths = []ports.CandidatePath{{
		Candidate: value(t, domain.NewCandidateID, "candidate-2"),
		Legs:      []domain.PlannedLeg{leg},
	}}
	fixture.evidence.byParcel["parcel-1"] = first
	fixture.evidence.byParcel["parcel-2"] = second

	result, err := fixture.handler.Handle(context.Background(), routeCommand(t, "parcel-1", "parcel-2"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	planOne, _ := result.Parcels()[0].Plan()
	planTwo, _ := result.Parcels()[1].Plan()
	if planOne.SelectedCandidate() == planTwo.SelectedCandidate() {
		t.Fatal("两个包裹被套上了同一条线路")
	}
	if planOne.Version() == planTwo.Version() {
		t.Fatal("两个包裹共用了一个计划版本——委托级执行路线正是被禁的")
	}
	if planTwo.Nodes()[1].String() != "node-alternate" {
		t.Fatalf("plan two nodes = %v", planTwo.Nodes())
	}
}

// Covers: `PAR-NET-16`「并列且无法选出唯一一条时为冲突，交人工裁决，不得任选」在初始路由这一侧
// ——最低成本并列既不形成计划也不形成`无当前有效路由`，落`路由未形成`、可续办；全部候选、并列的那几家与它们打平的
// 金额落未形成登记（`AT-NR-089`）；不落判断库、不交意图。人工选择入口不在本期（UC-NR-001）。
func TestALowestCostTieFormsNeitherPlanNorNoRoute(t *testing.T) {
	fixture := newRouteFixture(t)
	fixture.evidence.byParcel["parcel-1"] = tiedEvidence(t)

	result, err := fixture.handler.Handle(context.Background(), routeCommand(t, "parcel-1"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	parcel := result.Parcels()[0]
	if parcel.Outcome() != application.ParcelRouteNotFormed || parcel.NotFormedReason() != ports.NotFormedCandidatesTied {
		t.Fatalf("parcel = %q/%q, want NOT_FORMED/CANDIDATES_TIED", parcel.Outcome(), parcel.NotFormedReason())
	}
	if _, present := parcel.Plan(); present {
		t.Fatal("并列时形成了计划——那等于替授权角色任选了一家")
	}
	if _, present := parcel.NoCurrentRoute(); present {
		t.Fatal("并列时形成了无当前有效路由——候选明明可行")
	}
	if parcel.ContinuationReference().String() == "" {
		t.Fatal("并列的未决无法安全续办")
	}
	if tied := parcel.TiedCandidates(); len(tied) != 2 {
		t.Fatalf("tied = %v, want both candidates", tied)
	}
	if candidates := parcel.Candidates(); len(candidates) != 2 {
		t.Fatalf("candidates = %v; 全部候选要随并列理由一起交回", candidates)
	}
	if fixture.store.saved != 0 || len(fixture.downstream.intents) != 0 {
		t.Fatalf("saved = %d, intents = %d; 并列不落判断库也不交意图", fixture.store.saved, len(fixture.downstream.intents))
	}
	record := fixture.notFormed.only(t)
	if record.Reason != ports.NotFormedCandidatesTied || len(record.Candidates) != 2 || len(record.Tied) != 2 ||
		len(record.TiedCosts) != 2 {
		t.Fatalf("未形成登记 = %+v, want 并列、两家候选、两家并列与各自成本", record)
	}
	for _, cost := range record.TiedCosts {
		if cost.AmountMinor() != 4200 || cost.Currency() != "CNY" {
			t.Fatalf("并列理由 = %+v, want 两家都是 4200 CNY", record.TiedCosts)
		}
	}
}

// Covers: 关务资格状态未知（关务目录为空、来源未接）折成证据未知候选，选不出也立不成无路由：落`路由未形成`
// `CANDIDATE_EVIDENCE_INCOMPLETE`，全部候选与缺口落未形成登记（`AT-NR-090`）。状态未知不是依赖故障，重投推不动；关务
// 资格服务真读不动时取数侧交错误，落`ROUTE_EVIDENCE_UNAVAILABLE`、整封重投，那是 `AT-NR-006`（见
// TestThreeParcelsKeepThreeIndependentResults 的第三件）。两格都不记成无当前有效路由，也不选未经验证的口岸。
func TestUnknownCustomsEligibilityIsNotFormedNotNoRoute(t *testing.T) {
	fixture := newRouteFixture(t)
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

	result, err := fixture.handler.Handle(context.Background(), routeCommand(t, "parcel-1"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	parcel := result.Parcels()[0]
	if parcel.Outcome() != application.ParcelRouteNotFormed ||
		parcel.NotFormedReason() != ports.NotFormedCandidateEvidenceIncomplete {
		t.Fatalf("outcome = %q/%q, want NOT_FORMED/CANDIDATE_EVIDENCE_INCOMPLETE",
			parcel.Outcome(), parcel.NotFormedReason())
	}
	if fixture.store.saved != 0 {
		t.Fatal("资格未知被记成了领域结果")
	}
	record := fixture.notFormed.only(t)
	if len(record.Candidates) != 1 || record.Candidates[0].Outcome() != domain.CandidateEvidenceUnknown ||
		len(record.Gaps) != 1 || record.Gaps[0].Reference().String() != "CUSTOMS_ELIGIBILITY" ||
		record.Gaps[0].ReassessmentCondition().String() != "WHEN_CUSTOMS_AUTHORITY_ANSWERS" {
		t.Fatalf("未形成登记 = %+v, want 一家证据未知候选与关务资格那条缺口", record)
	}
}

// Covers: `AT-NR-011`「路由计划提交成功但事件首次投递失败——当前有效计划及全部依据保持
// 不变；重试同一发布意图，不重复形成计划」。首投失败留发布续办引用，重放按已有结果重发
// 同一份意图，计划不二次形成。
func TestAFailedIntentDeliveryIsRetriedWithoutASecondPlan(t *testing.T) {
	fixture := newRouteFixture(t)
	fixture.evidence.byParcel["parcel-1"] = routableEvidence(t)
	fixture.downstream.err = errors.New("downstream unreachable")

	first, err := fixture.handler.Handle(context.Background(), routeCommand(t, "parcel-1"))
	if err != nil {
		t.Fatalf("first handle: %v", err)
	}
	parcel := first.Parcels()[0]
	if parcel.Outcome() != application.ParcelRouteFormed {
		t.Fatalf("outcome = %q; 投递失败不得翻计划", parcel.Outcome())
	}
	if parcel.RouteHandoffReference().String() == "" {
		t.Fatal("首投失败没有留发布续办引用")
	}
	if fixture.store.saved != 1 {
		t.Fatalf("saved = %d", fixture.store.saved)
	}

	fixture.downstream.err = nil
	replay, err := fixture.handler.Handle(context.Background(), routeCommand(t, "parcel-1"))
	if err != nil {
		t.Fatalf("replay handle: %v", err)
	}
	if replay.Parcels()[0].Outcome() != application.ParcelExistingResult {
		t.Fatalf("replay = %q", replay.Parcels()[0].Outcome())
	}
	if len(fixture.downstream.intents) != 1 {
		t.Fatalf("intents = %d, want 1——重发的是同一份意图", len(fixture.downstream.intents))
	}
	if !fixture.downstream.intents[0].Record.HasPlan {
		t.Fatal("重发的意图不带原计划")
	}
	if fixture.store.saved != 1 {
		t.Fatal("重试形成了第二个计划")
	}
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

func TestMissingAcceptedResolutionStopsAtApplicabilityUnavailable(t *testing.T) {
	fixture := newRouteFixture(t)
	command := routeCommand(t, "parcel-1")
	command.Resolution = domain.CommercialResolutionReference{}

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.RouteHandoffUndecided ||
		result.UndecidedReason() != application.RoutingApplicabilityUnavailable {
		t.Fatalf("outcome = %q/%q; 命令缺解析标识不得静默跳过", result.Outcome(), result.UndecidedReason())
	}
	if fixture.applicability.asked != 0 {
		t.Fatal("缺解析标识仍问了适用性视图")
	}
}

// winnerUnreadableStore 演并发败方：写入答`已有记录`，读回赢家时那一行重建不出来。头一次按键找（判断之前）照常答没有。
type winnerUnreadableStore struct {
	*routeStoreDouble
	finds int
}

func (double *winnerUnreadableStore) FindByKey(
	ctx context.Context,
	key domain.InitialRouteJudgmentKey,
) (ports.InitialRouteRecord, bool, error) {
	double.finds++
	if double.finds > 1 {
		return ports.InitialRouteRecord{}, false, fmt.Errorf("stored row: %w", ports.ErrStoredRouteRecordUnreadable)
	}
	return double.routeStoreDouble.FindByKey(ctx, key)
}

// 路由库里这件包裹落了册的一行经领域构造门重建不出来是坏数据，重投推不动：响亮报错，不折成`路由判断未决`
// （ROUTE_STORE_UNAVAILABLE）回滚重投，与复核读同一类坏行同一口径。判断之前按键找、并发败方读回赢家两处都如此。读不回
// （依赖故障）仍答未决，重投推得动。
func TestAnUnrebuildableInitialRouteRowIsLoud(t *testing.T) {
	t.Run("判断之前按键找", func(t *testing.T) {
		fixture := newRouteFixture(t)
		fixture.evidence.byParcel["parcel-1"] = routableEvidence(t)
		fixture.store.findErr = fmt.Errorf("stored row: %w", ports.ErrStoredRouteRecordUnreadable)

		if _, err := fixture.handler.Handle(context.Background(), routeCommand(t, "parcel-1")); !errors.Is(err, ports.ErrStoredRouteRecordUnreadable) {
			t.Fatalf("err = %v, want ErrStoredRouteRecordUnreadable", err)
		}
		if fixture.store.saved != 0 || len(fixture.downstream.intents) != 0 {
			t.Fatalf("写了 %d 次、交了 %d 份意图, want 都是 0", fixture.store.saved, len(fixture.downstream.intents))
		}
	})

	t.Run("败方读回赢家", func(t *testing.T) {
		fixture := newRouteFixture(t)
		fixture.evidence.byParcel["parcel-1"] = routableEvidence(t)
		fixture.store.saveOutcome["parcel-1"] = ports.InitialRouteAlreadyRecorded
		store := &winnerUnreadableStore{routeStoreDouble: fixture.store}
		handler := application.NewCreateInitialRouteHandler(application.CreateInitialRouteDeps{
			Applicability: fixture.applicability,
			Evidence:      fixture.evidence,
			Costs:         passthroughCosts{},
			Store:         store,
			NotFormed:     fixture.notFormed,
			Log:           fixture.log,
			Downstream:    fixture.downstream,
			Identities:    &routeIdentityDouble{},
			Clock:         fixedClock{at: routeJudgedAt},
		})

		if _, err := handler.Handle(context.Background(), routeCommand(t, "parcel-1")); !errors.Is(err, ports.ErrStoredRouteRecordUnreadable) {
			t.Fatalf("err = %v, want ErrStoredRouteRecordUnreadable", err)
		}
		if len(fixture.downstream.intents) != 0 {
			t.Fatalf("交了 %d 份意图, want 0", len(fixture.downstream.intents))
		}
	})

	t.Run("读不回仍未决", func(t *testing.T) {
		fixture := newRouteFixture(t)
		fixture.evidence.byParcel["parcel-1"] = routableEvidence(t)
		fixture.store.findErr = errors.New("connection reset")

		result, err := fixture.handler.Handle(context.Background(), routeCommand(t, "parcel-1"))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if parcel := result.Parcels()[0]; parcel.Outcome() != application.ParcelRouteUndecided ||
			parcel.UndecidedReason() != application.RouteStoreUnavailable {
			t.Fatalf("包裹 = %q/%q, want UNDECIDED/ROUTE_STORE_UNAVAILABLE", parcel.Outcome(), parcel.UndecidedReason())
		}
	})
}
