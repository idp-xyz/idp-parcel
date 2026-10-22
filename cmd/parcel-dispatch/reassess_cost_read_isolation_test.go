package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/postgres/inbox"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	nrparcelshipment "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/parcelshipment"
	nrpostgres "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件对真实 PostgreSQL 证候选成本在库里读失败不连累判断那一笔事务：复核与初始路由都跑在消费门的 inbox 事务里，取数侧的仓储
// 读口走同一笔事务，PostgreSQL 上一条语句失败即让整笔进入中止态。失效线上失效照落、候选评估记未决；仍适用线上保留原计划；
// 两格都入账。初始路由那一件读不回照旧回滚重投，未决原因如实指向取数侧。夹具全部合成，只记 `S`。

// failingRead 在调用方的 ctx 上发一条注定失败的 SQL：事务里发时，它让那笔事务进入中止态，与生产取数侧遇上一张读不动的表同一种
// 后果。
func failingRead(fixture *reassessBothLanesFixture) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		querier, err := fixture.syn.db.ReadExecutor(ctx)
		if err != nil {
			return err
		}
		var never int
		return querier.QueryRow(ctx, `SELECT (1/0)::int`).Scan(&never)
	}
}

// applicabilityOfTheOriginalPlan 读两腿夹具里原计划 SYN-RPV-63 的适用性；没有登记行即当前有效（失效线改路与首个计划形成时
// 都不写行）。
func applicabilityOfTheOriginalPlan(t *testing.T, fixture *reassessBothLanesFixture) (nrdomain.PlanApplicability, bool) {
	t.Helper()
	applicabilities, err := nrpostgres.NewPlanApplicabilities(fixture.syn.db)
	if err != nil {
		t.Fatalf("构造适用性库：%v", err)
	}
	applicability, found, err := applicabilities.FindByPlan(t.Context(), mustNR(t, nrdomain.NewRoutePlanVersionID, "SYN-RPV-63"))
	if err != nil {
		t.Fatalf("读原计划适用性：%v", err)
	}
	return applicability, found
}

// Covers: NR CONTEXT「原计划已被硬依据证明失效时立即退出当前有效范围，即使替代候选评估仍未决也形成无当前有效路由」（真库，交接
// 一路）——两条候选都合格、排序形态已声明，包裹交进吉隆坡枢纽（不在计划上）；取数侧在 inbox 那一笔事务里读库失败。失效照落：
// 复核册一行`已失效`、候选评估未决、不挂建议也不形成新计划；适用性册里原计划已失效、离场于触发的业务时间；信封入账。
func TestALapseLandsWhenTheCandidateCostReadFailsOnTheDatabase(t *testing.T) {
	fixture := newReassessBothLanesFixture(t)
	declareImprovementReroute(t, fixture, 1000)
	fixture.costs.read = failingRead(fixture)

	astray := fixture.handoverInto(t, "astray", "SYN-NODE-KUL-HUB", reassessHandoverAt)
	if err := fixture.handover.Consume(t.Context(), outboxEnvelope(t, fixture.syn, astray, astray)); err != nil {
		t.Fatalf("投递交接 = %v, want 入账——候选成本读失败不得拦失效落库", err)
	}
	if fixture.costs.asked == 0 {
		t.Fatal("取数侧一次也没被问到，用例没走到候选成本那一步")
	}
	if status := inboxStatusOf(t, fixture.syn, reassessOnHandoverConsumerName, astray); status != "PROCESSED" {
		t.Fatalf("inbox status = %q, want PROCESSED", status)
	}
	lapsed := reassessmentOf(t, fixture.syn, handoverCorrelation("astray"))
	if lapsed.Conclusion != nrports.ReassessmentPlanLapsed || lapsed.ReviewedPlan.String() != "SYN-RPV-63" ||
		lapsed.CandidateState != nrports.CandidateReviewUndecided || lapsed.HasSuggestion || lapsed.HasNewPlan {
		t.Fatalf("交进吉隆坡 = %s 复核 %s、候选评估 %s、建议 %v、新计划 %v, want PLAN_LAPSED、复核 SYN-RPV-63、"+
			"CANDIDATE_REVIEW_UNDECIDED、无建议、无新计划",
			lapsed.Conclusion, lapsed.ReviewedPlan, lapsed.CandidateState, lapsed.HasSuggestion, lapsed.HasNewPlan)
	}
	original, found := applicabilityOfTheOriginalPlan(t, fixture)
	if !found || original.State() != nrdomain.PlanLapsed || !original.TransitionedAt().Equal(reassessHandoverAt) {
		t.Fatalf("原计划适用性 = %s 于 %s（found %v）, want LAPSED、于触发业务时间", original.State(), original.TransitionedAt(), found)
	}
}

// Covers: 仍适用线「成本读不回就判不出改善，原计划照旧保留」（真库，交接一路）——策略版本声明了自动改路、阈值 1000 分，原计划
// 仍可执行，包裹交进新加坡枢纽、未越过冻结边界；取数侧在 inbox 那一笔事务里读库失败。复核册一行`仍适用`、复核原计划、不形成
// 新计划；适用性册里原计划没有被替代；信封入账。
func TestAStillApplicablePlanIsKeptWhenTheCandidateCostReadFailsOnTheDatabase(t *testing.T) {
	fixture := newReassessBothLanesFixture(t)
	declareImprovementReroute(t, fixture, 1000)
	fixture.costs.read = failingRead(fixture)

	hub := fixture.handoverInto(t, "hub", reassessHandoverReceiving, reassessHandoverAt)
	if err := fixture.handover.Consume(t.Context(), outboxEnvelope(t, fixture.syn, hub, hub)); err != nil {
		t.Fatalf("投递交接 = %v, want 入账——仍适用已判定，成本读失败只让可选的切换不做", err)
	}
	if fixture.costs.asked == 0 {
		t.Fatal("取数侧一次也没被问到，用例没走到改善比较那一步")
	}
	if status := inboxStatusOf(t, fixture.syn, reassessOnHandoverConsumerName, hub); status != "PROCESSED" {
		t.Fatalf("inbox status = %q, want PROCESSED", status)
	}
	kept := reassessmentOf(t, fixture.syn, handoverCorrelation("hub"))
	if kept.Conclusion != nrports.ReassessmentStillApplicable || kept.ReviewedPlan.String() != "SYN-RPV-63" ||
		kept.HasNewPlan || kept.HasDecision {
		t.Fatalf("交进新加坡 = %s 复核 %s、新计划 %v、决定 %v, want STILL_APPLICABLE、复核 SYN-RPV-63、不改路",
			kept.Conclusion, kept.ReviewedPlan, kept.HasNewPlan, kept.HasDecision)
	}
	if original, found := applicabilityOfTheOriginalPlan(t, fixture); found && original.State() != nrdomain.PlanCurrentlyEffective {
		t.Fatalf("原计划适用性 = %s, want 仍当前有效", original.State())
	}
}

// withholdPriceCardVersions 在用例独占的库上把 parcel-pricing 的价卡版本表挪开、用例结束前挪回：此后取数侧读价卡时
// PostgreSQL 答「表不存在」，与生产上一张读不动的表同一种语句级失败。证据视图与判断库都不读这张表，失败只落在取数侧。
func withholdPriceCardVersions(t *testing.T, syn *synVerticalFixture) (restore func()) {
	t.Helper()
	if _, err := syn.pool.Exec(t.Context(),
		`ALTER TABLE parcel_pricing.price_card_version RENAME TO price_card_version_withheld`); err != nil {
		t.Fatalf("挪开价卡版本表：%v", err)
	}
	restored := false
	restore = func() {
		if restored {
			return
		}
		restored = true
		if _, err := syn.pool.Exec(context.WithoutCancel(t.Context()),
			`ALTER TABLE parcel_pricing.price_card_version_withheld RENAME TO price_card_version`); err != nil {
			t.Fatalf("挪回价卡版本表：%v", err)
		}
	}
	t.Cleanup(restore)
	return restore
}

// Covers: AT-NR-053（真库，生产装配，候选评估因成本读失败而未决的那一种）——演示租户采用了演示网络、登了与演示成本卡同身份的 CNY 卡，委托已接受且带客户声明
// 2.5 KG；证据视图、关务来源、候选成本取数侧与复核编排全是生产接法，经生产路由表一拍。
//  1. 价卡版本表读不动时交进吉隆坡枢纽：信封入账；原计划失效，候选评估未决，不挂建议；
//  2. 表挪回之后交进新加坡枢纽：走无路由那条线，形成首个当前有效路由计划——上一次没判成的候选评估由下一次控制触发补上。
func TestALapseLandsThroughTheProductionRouteWhenPriceCardsAreUnreadable(t *testing.T) {
	syn := newSYNVerticalFixture(t)
	acceptDemoParcelWithGeoAndWeight(t, syn)
	registerDemoNetworkSeeds(t, syn.db)
	registerCostCard(t, syn.db, demoShapedCostCard(t, "CNY", ppdomain.PricingPlanStructures{}))
	seedWiringCustomsCatalog(t, syn.db, demoTenant, true)
	savePlan(t, syn, "SYN-LINE-CN-SG-01@1",
		"SYN-NODE-SHA-HUB", "SYN-NODE-SZX-GATE", reassessHandoverReceiving, "SYN-NODE-SIN-LM")
	store, err := outbox.NewStore(syn.db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	register := newHandoverRegistration(t, syn.db, store)
	handover := func(tag, receiving string, at time.Time) {
		command := handoverCommand(t, reassessHandoverParcel, "SYN-HANDOVER/132/"+tag, tfdomain.ObjectHandedOver, "", receiving)
		command.Scope = reassessHandoverScope + "-" + tag
		command.JudgedAt = at
		eventID := registerHandover(t, syn, register, command)
		if _, err := syn.beat.DispatchOnce(t.Context()); err != nil {
			t.Fatalf("一拍：%v", err)
		}
		if status := outboxStatus(t, syn.db, eventID); status != "PUBLISHED" {
			t.Fatalf("交接 %s 信封 status = %s，失败码 = %q, want PUBLISHED", tag, status, recordedFailureCode(t, syn.db, eventID))
		}
	}

	restore := withholdPriceCardVersions(t, syn)
	handover("astray", "SYN-NODE-KUL-HUB", reassessHandoverAt.Add(-time.Hour))
	lapsed := reassessmentOf(t, syn, handoverCorrelation("astray"))
	if lapsed.Conclusion != nrports.ReassessmentPlanLapsed || lapsed.CandidateState != nrports.CandidateReviewUndecided ||
		lapsed.HasSuggestion {
		t.Fatalf("交进吉隆坡 = %s、候选评估 %s、建议 %v, want PLAN_LAPSED、CANDIDATE_REVIEW_UNDECIDED、无建议",
			lapsed.Conclusion, lapsed.CandidateState, lapsed.HasSuggestion)
	}

	restore()
	handover("back", reassessHandoverReceiving, reassessHandoverAt)
	formed := reassessmentOf(t, syn, handoverCorrelation("back"))
	if formed.Conclusion != nrports.ReassessmentFirstPlanFormed || !formed.HasNewPlan ||
		formed.NewPlan.SelectedCandidate().String() != "SYN-LINE-CN-SG-01@1" {
		t.Fatalf("交进新加坡 = %s、选中 %s, want FIRST_PLAN_FORMED、演示线路", formed.Conclusion, formed.NewPlan.SelectedCandidate())
	}
}

// Covers: 初始路由那一件读不回（生产装配，UC-NR-001）——与上一条同一份演示网络与成本卡，SYN-RES-01 照 SYN-PC-PRODUCT 种子要求
// 网络路由判断；接受决定信封交给生产装配的接受决定消费者，价卡版本表读不动。信封照旧回滚重投（成本读不回是依赖故障），未决原因如实指向取数侧：取成本之后编排还要重读一次证据
// 做提交前重校，事务若已中毒，那一读跟着失败，原因就被报成网络证据读不动，运维会去查网络目录而不是 parcel-pricing。
func TestAnInitialRouteCostReadFailureIsReportedAsTheCostSource(t *testing.T) {
	syn := newSYNVerticalFixture(t)
	acceptDemoParcelWithGeoAndWeight(t, syn)
	registerDemoNetworkSeeds(t, syn.db)
	registerCostCard(t, syn.db, demoShapedCostCard(t, "CNY", ppdomain.PricingPlanStructures{}))
	seedWiringCustomsCatalog(t, syn.db, demoTenant, true)
	seedSYNPCEligibility(t, syn)
	outboxStore, err := outbox.NewStore(syn.db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	inboxStore, err := inbox.NewStore(syn.db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	consumer, err := acceptanceConsumer(syn.db, outboxStore, inboxStore,
		dispatchSettings{purpose: mustNR(t, nrdomain.NewServicePurpose, "NETWORK_SERVICE")}, systemClock{})
	if err != nil {
		t.Fatalf("装配接受决定消费者：%v", err)
	}

	withholdPriceCardVersions(t, syn)
	err = consumer.Consume(t.Context(), outboxEnvelope(t, syn, synV0DecisionID, synV0DecisionID))
	if !errors.Is(err, nrparcelshipment.ErrRouteHandoffUndecided) {
		t.Fatalf("投递接受决定 = %v, want 未决、回滚重投", err)
	}
	if !strings.Contains(err.Error(), "COST_SOURCE_UNAVAILABLE") {
		t.Fatalf("未决原因 = %v, want COST_SOURCE_UNAVAILABLE——读不动的是价卡，不是网络证据", err)
	}
}
