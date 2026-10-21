package main

import (
	"testing"
	"time"

	nrpostgres "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件对真实 PostgreSQL 证复核的业务时间下限（ADR-0279）：初始路由从委托接受起生效，一次业务时间早于接受的控制触发在
// 初始路由形成之后才处理时，入账为「触发早于委托接受」，不判、不回滚、不重投——据它判出失效，离场就早于生效。接受决定由
// PS 真形成，初始路由经生产形状的消费门判出；收寄采用与交接两路接同一份复核编排，接受时刻由两路适配器各自从 PS 委托仓储
// 读来。夹具全部合成，只记 `S`。

// notJudgedAcceptanceOf 读一行未判的原因与比较所用的委托接受时刻；读不着即失败。接受时刻取自 parcel-shipment、带纳秒，落库
// 只留到微秒，比对时同样截到微秒。
func notJudgedAcceptanceOf(t *testing.T, syn *synVerticalFixture, correlation string) (string, *time.Time, *time.Time) {
	t.Helper()
	querier, err := syn.db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	var reason string
	var acceptedAt, positionAt *time.Time
	if err := querier.QueryRow(t.Context(),
		`SELECT reason, shipment_accepted_at, current_position_at FROM network_routing.route_reassessment_not_judged
		  WHERE tenant_id = $1 AND correlation_id = $2`, "SYN-TENANT-01", correlation,
	).Scan(&reason, &acceptedAt, &positionAt); err != nil {
		t.Fatalf("读 %s 的未判：%v；未判 = %+v", correlation, err, notJudgedRows(t, syn))
	}
	return reason, acceptedAt, positionAt
}

// requirePlanUntouched 断言那份初始计划的适用性没有登记行：仍当前有效，没有被早于接受的触发离场。
func requirePlanUntouched(t *testing.T, syn *synVerticalFixture, plan nrdomain.InitialRoutePlan) {
	t.Helper()
	applicabilities, err := nrpostgres.NewPlanApplicabilities(syn.db)
	if err != nil {
		t.Fatalf("构造适用性库：%v", err)
	}
	if applicability, found, err := applicabilities.FindByPlan(t.Context(), plan.Version()); err != nil || found {
		t.Fatalf("原计划适用性 = %s @ %s（found %v err %v）, want 没有登记行——仍当前有效",
			applicability.State(), applicability.TransitionedAt(), found, err)
	}
}

// Covers: AT-NR-087（真库，交接一路）——委托接受之后形成初始路由，一次判断时刻早于接受两小时的交接（进吉隆坡枢纽，不在计划
// 上）在那之后才登记处理：入账为「触发早于委托接受」，未判登记点名比较所用的委托接受时刻；复核册零行，原计划不失效；信封
// 入账 PROCESSED，另一封信封带来同一版交接同答。
func TestAHandoverEarlierThanTheAcceptanceIsNotJudgedOnTheDatabase(t *testing.T) {
	fixture, acceptedAt, plan := backloggedInitialRoute(t)
	syn := fixture.syn
	handover := fixture.handoverInto(t, "early", "SYN-NODE-KUL-HUB", acceptedAt.Add(-2*time.Hour))

	deliver(t, syn, fixture.handover, handover, handover)
	deliver(t, syn, fixture.handover, handover, handover+"/again")

	if rows := reassessmentRows(t, syn); len(rows) != 0 {
		t.Fatalf("复核 = %+v, want 零行——早于接受的交接不判", rows)
	}
	requirePlanUntouched(t, syn, plan)
	reason, recordedAcceptance, positionAt := notJudgedAcceptanceOf(t, syn, handoverCorrelation("early"))
	if reason != "TRIGGER_PRECEDES_SHIPMENT_ACCEPTANCE" || recordedAcceptance == nil ||
		!recordedAcceptance.Equal(acceptedAt.Truncate(time.Microsecond)) || positionAt != nil {
		t.Fatalf("未判 = %s / 接受 %v / 当前位置 %v, want TRIGGER_PRECEDES_SHIPMENT_ACCEPTANCE、接受 %s、不带当前位置",
			reason, recordedAcceptance, positionAt, acceptedAt)
	}
	for _, id := range []string{handover, handover + "/again"} {
		if status := inboxStatusOf(t, syn, reassessOnHandoverConsumerName, id); status != "PROCESSED" {
			t.Fatalf("%s 的 inbox status = %q, want PROCESSED", id, status)
		}
	}
}

// Covers: AT-NR-087（真库，收寄采用一路）——委托接受之后形成初始路由，一份业务时间早于接受两小时的场外揽收采用在那之后才
// 处理，选中候选被有效硬限制淘汰：入账为「触发早于委托接受」，复核册零行，原计划不失效——据这次揽收失效，离场就早于
// 生效；收寄采用信封入账 PROCESSED。
func TestAnIntakeEarlierThanTheAcceptanceIsNotJudgedOnTheDatabase(t *testing.T) {
	fixture, acceptedAt, plan := backloggedInitialRoute(t)
	syn := fixture.syn
	restriction, err := nrdomain.NewHardConstraintFinding(nrdomain.HardConstraintFindingSpec{
		Candidate:   mustNR(t, nrdomain.NewCandidateID, "SYN-CANDIDATE-01"),
		Outcome:     nrdomain.RestrictionApplies,
		Restriction: mustNR(t, nrdomain.NewRestrictionReference, "CUSTOMS-ELIGIBILITY/SYN-CC-69"),
	})
	if err != nil {
		t.Fatalf("硬限制：%v", err)
	}
	fixture.evidence.evidence.HardConstraints = []nrdomain.HardConstraintFinding{restriction}
	pickedUpAt := acceptedAt.Add(-2 * time.Hour)
	pickup := fixture.intakeAt(t, psdomain.OffsitePickupSource, pickupReassessVersion, pickupReassessPlace, pickedUpAt)

	deliver(t, syn, fixture.intake, pickup, pickup)

	if rows := reassessmentRows(t, syn); len(rows) != 0 {
		t.Fatalf("复核 = %+v, want 零行——早于接受的揽收不判", rows)
	}
	requirePlanUntouched(t, syn, plan)
	reason, recordedAcceptance, positionAt := notJudgedAcceptanceOf(t, syn, pickupReassessCorrel)
	if reason != "TRIGGER_PRECEDES_SHIPMENT_ACCEPTANCE" || recordedAcceptance == nil ||
		!recordedAcceptance.Equal(acceptedAt.Truncate(time.Microsecond)) || positionAt != nil {
		t.Fatalf("未判 = %s / 接受 %v / 当前位置 %v, want TRIGGER_PRECEDES_SHIPMENT_ACCEPTANCE、接受 %s、不带当前位置",
			reason, recordedAcceptance, positionAt, acceptedAt)
	}
	if status := inboxStatusOf(t, syn, "network-routing/reassess-on-network-intake", pickup); status != "PROCESSED" {
		t.Fatalf("收寄采用信封的 inbox status = %q, want PROCESSED", status)
	}
}
