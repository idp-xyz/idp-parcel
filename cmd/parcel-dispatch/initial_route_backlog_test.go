package main

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/postgres/inbox"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	nrinbox "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/inbox"
	nrparcelshipment "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/parcelshipment"
	nrpostgres "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	nrapplication "go.idp.xyz/idp-parcel/internal/networkrouting/application"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件对真实 PostgreSQL 证初始路由计划的生效时刻（CONTEXT「路由计划适用性」）：路由交接积压，NR 晚于委托接受才处理时，
// 计划仍从接受的时刻起生效，此后业务时间落在两者之间的控制触发照常判。初始路由走生产的接受决定消费门、PS 委托仓储与 NR
// 路由库，复核走收寄采用那一路；只换证据视图、成本取数侧与商业适用性视图。夹具全部合成，只记 `S`。

const handoffBacklog = 10 * time.Hour

// backloggedClock 演积压：NR 处理路由交接的处理时钟比 PS 形成接受决定时晚一截。
type backloggedClock struct{ by time.Duration }

func (clock backloggedClock) Now() time.Time { return time.Now().Add(clock.by) }

// routingRequired 是商业适用性视图替身：这件委托要求网络路由。适用性的取数不在本文件的观察范围内。
type routingRequired struct{}

func (routingRequired) AssessRoutingApplicability(
	_ context.Context, _ nrdomain.InitialRouteJudgmentKey, _ nrdomain.CommercialResolutionReference,
) (nrdomain.NetworkEligibility, error) {
	return nrdomain.NewNetworkEligibility(nrdomain.NetworkJudgmentRequired, nrdomain.EligibilityBasisReference{})
}

// Covers: AT-NR-086（真库，收寄采用一路）——NR 在委托接受十小时之后才处理路由交接：落库的计划从接受基线固定的时刻起
// 生效，判断时刻是处理时钟。随后场外揽收的业务时间落在接受与处理之间、选中候选被有效硬限制淘汰：原计划在揽收的业务时间
// 失效，过得了离场门，信封入账，不回滚重投到 ABANDONED。
func TestUnderAHandoffBacklogTheInitialPlanTakesEffectAtAcceptanceOnTheDatabase(t *testing.T) {
	fixture, acceptedAt, plan := backloggedInitialRoute(t)
	syn := fixture.syn
	evidence := &fixture.evidence.evidence

	restriction, err := nrdomain.NewHardConstraintFinding(nrdomain.HardConstraintFindingSpec{
		Candidate:   mustNR(t, nrdomain.NewCandidateID, "SYN-CANDIDATE-01"),
		Outcome:     nrdomain.RestrictionApplies,
		Restriction: mustNR(t, nrdomain.NewRestrictionReference, "CUSTOMS-ELIGIBILITY/SYN-CC-60"),
	})
	if err != nil {
		t.Fatalf("硬限制：%v", err)
	}
	evidence.HardConstraints = []nrdomain.HardConstraintFinding{restriction}
	pickedUpAt := acceptedAt.Add(handoffBacklog / 2).Truncate(time.Microsecond)
	pickup := fixture.intakeAt(t, psdomain.OffsitePickupSource, pickupReassessVersion, pickupReassessPlace, pickedUpAt)
	deliver(t, syn, fixture.intake, pickup, pickup)
	if lapsed := reassessmentOf(t, syn, pickupReassessCorrel); lapsed.Conclusion != nrports.ReassessmentPlanLapsed ||
		lapsed.ReviewedPlan != plan.Version() {
		t.Fatalf("揽收复核 = %s / %s, want PLAN_LAPSED、复核 %s", lapsed.Conclusion, lapsed.ReviewedPlan, plan.Version())
	}
	applicabilities, err := nrpostgres.NewPlanApplicabilities(syn.db)
	if err != nil {
		t.Fatalf("构造适用性库：%v", err)
	}
	applicability, found, err := applicabilities.FindByPlan(t.Context(), plan.Version())
	if err != nil || !found || applicability.State() != nrdomain.PlanLapsed || !applicability.TransitionedAt().Equal(pickedUpAt) {
		t.Fatalf("原计划适用性 = %s @ %s（found %v err %v）, want LAPSED @ 揽收业务时间 %s",
			applicability.State(), applicability.TransitionedAt(), found, err, pickedUpAt)
	}
}

// backloggedInitialRoute 在 PS 真形成的接受决定上，经生产形状的初始路由消费门、积压 handoffBacklog 判出初始路由：计划沿上海
// 枢纽 → 新加坡枢纽 → 新加坡末端，选中候选在证据替身里有服务区域、成本与路径。交回两路复核夹具、接受基线固定的时刻与那份
// 计划，并已断言计划从接受起生效、判断不早于接受之后 handoffBacklog。
func backloggedInitialRoute(t *testing.T) (*reassessBothLanesFixture, time.Time, nrdomain.InitialRoutePlan) {
	t.Helper()
	syn := newSYNVerticalFixture(t)
	acceptWithDeclaredGeo(t, syn)
	fixture := reassessBothLanesOver(t, syn)
	evidence := &fixture.evidence.evidence
	evidence.RankingForm = nrdomain.CostSingleDimensionRanking
	area, err := nrdomain.NewServiceAreaResolution(nrdomain.ServiceAreaResolutionSpec{
		Candidate:   mustNR(t, nrdomain.NewCandidateID, "SYN-CANDIDATE-01"),
		Outcome:     nrdomain.AreaCoversDestination,
		AreaVersion: mustNR(t, nrdomain.NewServiceAreaVersionReference, "SYN-AREA/v1"),
	})
	if err != nil {
		t.Fatalf("服务区域：%v", err)
	}
	cost, err := nrdomain.NewPricedCandidateCost(mustNR(t, nrdomain.NewCandidateID, "SYN-CANDIDATE-01"), 4200, "CNY")
	if err != nil {
		t.Fatalf("候选成本：%v", err)
	}
	evidence.ServiceAreas = []nrdomain.ServiceAreaResolution{area}
	evidence.CandidateCosts = []nrdomain.CandidateCostFact{cost}
	evidence.Paths = []nrports.CandidatePath{{
		Candidate: mustNR(t, nrdomain.NewCandidateID, "SYN-CANDIDATE-01"),
		Legs:      legsThrough(t, "SYN-NODE-SHA-HUB", reassessHandoverReceiving, "SYN-NODE-SIN-LM"),
	}}

	request, found, err := syn.requests.FindBySourceIdentity(t.Context(), syn.identity)
	if err != nil || !found {
		t.Fatalf("读委托：found %v err %v", found, err)
	}
	baseline, present := request.AcceptanceBaseline()
	if !present {
		t.Fatal("已接受委托没有接受基线")
	}
	acceptedAt := baseline.FixedAt()

	routeBacklogged(t, fixture)
	routes, err := nrpostgres.NewInitialRoutes(syn.db)
	if err != nil {
		t.Fatalf("初始路由库：%v", err)
	}
	record, found, err := routes.FindByKey(t.Context(), reassessHandoverKey(t))
	if err != nil || !found || !record.HasPlan {
		t.Fatalf("初始路由 = %+v（found %v err %v）, want 一份计划", record, found, err)
	}
	plan := record.Plan
	if !plan.EffectiveFrom().Equal(acceptedAt) || plan.JudgedAt().Before(acceptedAt.Add(handoffBacklog)) {
		t.Fatalf("计划生效 %s、判断 %s, want 从接受 %s 起生效、判断不早于接受之后 %s",
			plan.EffectiveFrom(), plan.JudgedAt(), acceptedAt, handoffBacklog)
	}
	return fixture, acceptedAt, plan
}

// routeBacklogged 把 PS 交出的那封接受决定信封投给生产形状的初始路由消费门（接受决定消费门 → PS 委托仓储 → 初始路由编排
// → NR 路由库、交接登记、标识与发布意图），处理时钟比接受晚 handoffBacklog。
func routeBacklogged(t *testing.T, fixture *reassessBothLanesFixture) {
	t.Helper()
	syn := fixture.syn
	clock := backloggedClock{by: handoffBacklog}
	outboxStore, err := outbox.NewStore(syn.db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	inboxStore, err := inbox.NewStore(syn.db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	routeStore, err := nrpostgres.NewInitialRoutes(syn.db)
	if err != nil {
		t.Fatalf("初始路由库：%v", err)
	}
	handoffLog, err := nrpostgres.NewRouteHandoffLogs(syn.db)
	if err != nil {
		t.Fatalf("交接登记：%v", err)
	}
	identities, err := nrpostgres.NewRouteIdentities(syn.db)
	if err != nil {
		t.Fatalf("路由标识：%v", err)
	}
	downstream, err := nrpostgres.NewOutboxInitialRouteHandoff(syn.db, outboxStore, clock)
	if err != nil {
		t.Fatalf("初始路由交接：%v", err)
	}
	handler := nrapplication.NewCreateInitialRouteHandler(nrapplication.CreateInitialRouteDeps{
		Applicability: routingRequired{},
		Evidence:      fixture.evidence,
		Costs:         fixture.costs,
		Store:         routeStore,
		Log:           handoffLog,
		Downstream:    downstream,
		Identities:    identities,
		Clock:         clock,
	})
	decisions, err := nrparcelshipment.NewRouteOnAcceptanceAdapter(syn.requests, handler,
		mustNR(t, nrdomain.NewServicePurpose, "NETWORK_SERVICE"))
	if err != nil {
		t.Fatalf("装配接受决定适配器：%v", err)
	}
	consumer, err := nrinbox.NewAcceptanceConsumer(syn.db.Transactor(), inboxStore, decisions)
	if err != nil {
		t.Fatalf("装配接受决定消费门：%v", err)
	}
	accepted := syn.outboxIDsOfType(t, string(nrinbox.AcceptedDecisionEventType))
	if len(accepted) != 1 {
		t.Fatalf("接受决定信封 %d 封, want 1", len(accepted))
	}
	deliver(t, syn, consumer, accepted[0], accepted[0])
}
