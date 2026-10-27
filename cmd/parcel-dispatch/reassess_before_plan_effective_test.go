package main

import (
	"testing"
	"time"

	nrapplication "go.idp.xyz/idp-parcel/internal/networkrouting/application"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件对真实 PostgreSQL 证复核的第三道业务时间下限（ADR-0292）：授权角色的决定形成的计划从决定的时刻起生效（ADR-0284 决定四），
// 一次业务时间落在原计划失效与这一决定之间、处理却在决定之后的控制触发，起点是从决定才生效的新计划。判出失效时离场早于生效，
// 离场门拒——修前整封信封响亮失败、按派发预算重投到 ABANDONED，占着交接分区；修后入账为「触发早于计划生效」，信封收工。揽收复核、
// 决定与交接复核与生产同形（复核两路、决定编排的判断库各册），只换证据视图与成本取数侧。夹具全部合成，只记 `S`。

// notJudgedPlanEffectiveOf 读一行未判的原因与比较所用的计划生效时刻，连同另两格比较时刻（应为空）；读不着即失败。
func notJudgedPlanEffectiveOf(t *testing.T, syn *synVerticalFixture, correlation string) (string, *time.Time, *time.Time, *time.Time) {
	t.Helper()
	querier, err := syn.db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	var reason string
	var effectiveAt, acceptedAt, positionAt *time.Time
	if err := querier.QueryRow(t.Context(),
		`SELECT reason, plan_effective_at, shipment_accepted_at, current_position_at
		   FROM network_routing.route_reassessment_not_judged
		  WHERE tenant_id = $1 AND correlation_id = $2`, "SYN-TENANT-01", correlation,
	).Scan(&reason, &effectiveAt, &acceptedAt, &positionAt); err != nil {
		t.Fatalf("读 %s 的未判：%v；未判 = %+v", correlation, err, notJudgedRows(t, syn))
	}
	return reason, effectiveAt, acceptedAt, positionAt
}

// decidedAfterPickup 走票面时序到决定为止：揽收（T1）时原计划所选候选被硬限制淘汰，复核失效并形成建议；授权角色在 T3（T1 之后
// 一小时）择另一条合格候选形成决定，新计划从 T3 起生效。交回夹具、决定形成的计划与 T1、T3。
func decidedAfterPickup(t *testing.T) (*reassessBothLanesFixture, nrdomain.InitialRoutePlan, time.Time, time.Time) {
	t.Helper()
	fixture := newReassessBothLanesFixture(t)
	declareOneRestrictedOfTwoCandidates(t, fixture)
	pickup := fixture.intakeAt(t, psdomain.OffsitePickupSource, pickupReassessVersion, pickupReassessPlace, pickupReassessAt)
	deliver(t, fixture.syn, fixture.intake, pickup, pickup)
	if lapsed := reassessmentOf(t, fixture.syn, pickupReassessCorrel); lapsed.Conclusion != nrports.ReassessmentPlanLapsed || !lapsed.HasSuggestion {
		t.Fatalf("揽收复核 = %s，建议 %v, want PLAN_LAPSED 带建议", lapsed.Conclusion, lapsed.HasSuggestion)
	}

	decidedAt := pickupReassessAt.Add(time.Hour)
	decider := rerouteDecisionHandlerOn(t, fixture, decidedAt)
	result := decideWithin(t, fixture, decider, nrapplication.DecideRerouteCommand{
		Tenant:     mustNR(t, nrdomain.NewTenantID, "SYN-TENANT-01"),
		Parcel:     mustNR(t, nrdomain.NewDeclaredParcelID, reassessHandoverParcel),
		Suggestion: mustNR(t, nrdomain.NewRequestCorrelationID, pickupReassessCorrel),
		Candidate:  mustNR(t, nrdomain.NewCandidateID, "SYN-CANDIDATE-02"),
		Basis:      mustNR(t, nrdomain.NewRerouteDecisionBasisReference, "SYN-APPROVAL-100"),
		DecidedBy:  mustNR(t, nrdomain.NewAuthorizedRoleReference, "OPERATOR/http://127.0.0.1:5556/dex#SYN-OPERATOR-CONFIG-01"),
	})
	if result.Outcome() != nrapplication.RerouteDecided {
		t.Fatalf("outcome = %q reason = %q, want REROUTE_DECIDED", result.Outcome(), result.UndecidedReason())
	}
	recorded, _ := result.Decision()
	plan := recorded.Decision.NewPlan()
	if !plan.EffectiveFrom().Equal(decidedAt) {
		t.Fatalf("决定形成的计划生效 %s, want 决定的时刻 %s", plan.EffectiveFrom(), decidedAt)
	}
	return fixture, plan, pickupReassessAt, decidedAt
}

// Covers: AT-NR-104（真库，交接一路）——票面时序：原计划在 T1 因硬限制失效、复核形成改路建议，授权角色在 T3 据它决定，新计划从 T3 起
// 生效；一次判断时刻 T2（T1 < T2 < T3）的交接（进吉隆坡枢纽，不在计划上）在决定之后才登记处理：入账为「触发早于计划生效」，
// 未判登记点名 T3，复核册只有揽收那一行，决定形成的计划仍当前有效、适用性没有登记行；信封入账 PROCESSED（处理完毕，不回滚、
// 不重投、不占分区），另一封信封带来同一版交接同答。随后一次不早于 T3 的交接（同样进吉隆坡枢纽）照判：决定形成的计划失效，
// 引用它——下一次控制触发从它的位置判。
func TestAHandoverBetweenASuggestionAndItsDecisionIsNotJudgedOnTheDatabase(t *testing.T) {
	fixture, plan, pickedUpAt, decidedAt := decidedAfterPickup(t)
	syn := fixture.syn
	between := fixture.handoverInto(t, "between", "SYN-NODE-KUL-HUB", pickedUpAt.Add(30*time.Minute))

	deliver(t, syn, fixture.handover, between, between)
	deliver(t, syn, fixture.handover, between, between+"/again")

	if rows := reassessmentRows(t, syn); len(rows) != 1 {
		t.Fatalf("复核 = %+v, want 只有揽收那一行——T2 的交接不判", rows)
	}
	requirePlanUntouched(t, syn, plan)
	reason, effectiveAt, acceptedAt, positionAt := notJudgedPlanEffectiveOf(t, syn, handoverCorrelation("between"))
	if reason != "TRIGGER_PRECEDES_PLAN_EFFECTIVE_FROM" || effectiveAt == nil || !effectiveAt.Equal(decidedAt.Truncate(time.Microsecond)) ||
		acceptedAt != nil || positionAt != nil {
		t.Fatalf("未判 = %s / 生效 %v / 接受 %v / 当前位置 %v, want TRIGGER_PRECEDES_PLAN_EFFECTIVE_FROM、生效 %s、不带另两格",
			reason, effectiveAt, acceptedAt, positionAt, decidedAt)
	}
	for _, id := range []string{between, between + "/again"} {
		if status := inboxStatusOf(t, syn, reassessOnHandoverConsumerName, id); status != "PROCESSED" {
			t.Fatalf("%s 的 inbox status = %q, want PROCESSED", id, status)
		}
	}

	after := fixture.handoverInto(t, "after", "SYN-NODE-KUL-HUB", decidedAt.Add(time.Hour))
	deliver(t, syn, fixture.handover, after, after)
	lapsed := reassessmentOf(t, syn, handoverCorrelation("after"))
	if lapsed.Conclusion != nrports.ReassessmentPlanLapsed || lapsed.ReviewedPlan != plan.Version() {
		t.Fatalf("决定之后的交接复核 = %s / %s, want PLAN_LAPSED、复核决定形成的 %s", lapsed.Conclusion, lapsed.ReviewedPlan, plan.Version())
	}
}

// Covers: AT-NR-105（真库，交接一路）——同样在 T1 < T2 < T3 之间、决定之后才处理的交接，进的是决定形成的计划上的节点（新加坡
// 枢纽）、没有改善：判出仍适用，照常记「仍适用」、引用决定形成的计划，不入账为未判；决定形成的计划不被动。
func TestAHandoverBetweenASuggestionAndItsDecisionThatKeepsThePlanIsStillJudgedOnTheDatabase(t *testing.T) {
	fixture, plan, pickedUpAt, _ := decidedAfterPickup(t)
	syn := fixture.syn
	between := fixture.handoverInto(t, "between-on-plan", reassessHandoverReceiving, pickedUpAt.Add(30*time.Minute))

	deliver(t, syn, fixture.handover, between, between)

	kept := reassessmentOf(t, syn, handoverCorrelation("between-on-plan"))
	if kept.Conclusion != nrports.ReassessmentStillApplicable || kept.ReviewedPlan != plan.Version() {
		t.Fatalf("T2 的交接复核 = %s / %s, want STILL_APPLICABLE、复核决定形成的 %s", kept.Conclusion, kept.ReviewedPlan, plan.Version())
	}
	if rows := notJudgedRows(t, syn); len(rows) != 0 {
		t.Fatalf("未判 = %+v, want 零行——判出仍适用不撞", rows)
	}
	requirePlanUntouched(t, syn, plan)
}
