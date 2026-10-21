package main

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件对真实 PostgreSQL 证场外揽收触发的复核（ADR-0249）：收寄采用那一路与生产同形（采用仓储 → 两层适配器 → 消费门 →
// 同一份复核编排），只换证据视图与成本取数侧。揽收地点取客户场所，不是节点码。夹具全部合成，只记 `S`。

const (
	pickupReassessPlace   = "customer-warehouse-2"
	pickupReassessVersion = "SYN-PICKUP/16/v1"
	pickupReassessCorrel  = "intake/SYN-TENANT-01/SYN-PARCEL-01/OFFSITE_PICKUP/" + pickupReassessVersion
)

var pickupReassessAt = reassessHandoverAt.Add(-48 * time.Hour)

type pickupReassessmentRow struct {
	conclusion, reviewedPlan string
	lapseBasis, rerouteState *string
	blockers                 *string
	hasSuggestion            bool
	hasNewPlan, hasDecision  bool
	triggerAt                time.Time
}

func pickupReassessmentRowOf(t *testing.T, syn *synVerticalFixture, correlation string) (pickupReassessmentRow, bool) {
	t.Helper()
	querier, err := syn.db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	rows, err := querier.Query(t.Context(),
		`SELECT conclusion, reviewed_plan, lapse_basis, reroute_state, reroute_blockers::text,
		        suggestion IS NOT NULL, new_plan IS NOT NULL, decision IS NOT NULL, trigger_occurred_at
		   FROM network_routing.route_reassessment WHERE tenant_id = $1 AND correlation_id = $2`,
		"SYN-TENANT-01", correlation)
	if err != nil {
		t.Fatalf("读复核：%v", err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			t.Fatalf("读复核：%v", err)
		}
		return pickupReassessmentRow{}, false
	}
	var row pickupReassessmentRow
	if err := rows.Scan(&row.conclusion, &row.reviewedPlan, &row.lapseBasis, &row.rerouteState, &row.blockers,
		&row.hasSuggestion, &row.hasNewPlan, &row.hasDecision, &row.triggerAt); err != nil {
		t.Fatalf("扫复核行：%v", err)
	}
	return row, true
}

// Covers: AT-NR-034（真库，收寄采用一路）——场外揽收采用记录（地点是客户场所）经生产同形的消费门触发复核：原计划仍适用，
// 复核册落一行 `STILL_APPLICABLE`、引用原计划版本，触发业务时间等于揽收时间（揽收复核计入当前控制位置）；策略版本没声明
// 冻结形态也不落 `FREEZE_FORM_UNCONFIGURED`——揽收不建立可控节点，不判冻结；信封入账。
func TestAnOffsitePickupKeepsThePlanOnTheDatabase(t *testing.T) {
	fixture := newReassessBothLanesFixture(t)
	fixture.evidence.evidence.FreezeForm = nrdomain.FreezeFormUndeclared
	fixture.evidence.evidence.FreezeRemainingSegmentLimit = nil
	pickup := fixture.intakeAt(t, psdomain.OffsitePickupSource, pickupReassessVersion, pickupReassessPlace, pickupReassessAt)

	deliver(t, fixture.syn, fixture.intake, pickup, pickup)

	row, found := pickupReassessmentRowOf(t, fixture.syn, pickupReassessCorrel)
	if !found {
		t.Fatalf("复核册没有揽收那一行；未判 = %+v", notJudgedRows(t, fixture.syn))
	}
	if row.conclusion != "STILL_APPLICABLE" || row.reviewedPlan != "SYN-RPV-63" || row.lapseBasis != nil {
		t.Fatalf("复核 = %+v, want STILL_APPLICABLE、引用 SYN-RPV-63、无失效依据——揽收地点不与计划节点比", row)
	}
	if !row.triggerAt.Equal(pickupReassessAt) {
		t.Fatalf("触发业务时间 = %s, want 揽收时间 %s", row.triggerAt, pickupReassessAt)
	}
	if row.hasNewPlan || row.hasDecision || row.hasSuggestion || row.rerouteState != nil {
		t.Fatalf("复核 = %+v; 仍适用不形成改路三件", row)
	}
	for _, noted := range notJudgedRows(t, fixture.syn) {
		if noted.reason == "FREEZE_FORM_UNCONFIGURED" {
			t.Fatalf("未判 = %+v; 揽收复核不判冻结", noted)
		}
	}
	if status := inboxStatusOf(t, fixture.syn, "network-routing/reassess-on-network-intake", pickup); status != "PROCESSED" {
		t.Fatalf("揽收采用信封的 inbox status = %q, want PROCESSED", status)
	}
}

// Covers: AT-NR-084（真库，收寄采用一路）——揽收复核时原计划的选中候选被有效硬限制淘汰，另有一条更便宜的合格候选，策略版本
// 允许自动改路（阈值 0 是本测试交入的租户取值）：原计划失效、依据点名限制；包裹不在可控节点上，只形成改路建议，阻塞只有
// `NOT_AT_CONTROLLED_NODE`（已执行前缀为空，不得出 `EXECUTED_PREFIX_AFFECTED`），不形成决定与新计划。这一行过得了复核册
// 的逐结论 CHECK。
func TestAnOffsitePickupWithARestrictedPlanOnlySuggestsOnTheDatabase(t *testing.T) {
	fixture := newReassessBothLanesFixture(t)
	evidence := &fixture.evidence.evidence
	threshold := 0
	evidence.RankingForm = nrdomain.CostSingleDimensionRanking
	evidence.AutoRerouteForm = nrdomain.CostImprovementAutoReroute
	evidence.AutoRerouteImprovementThresholdMinor = &threshold
	paths := map[string][]string{
		"SYN-CANDIDATE-01": {"SYN-NODE-SHA-HUB", reassessHandoverReceiving, "SYN-NODE-SIN-LM"},
		"SYN-CANDIDATE-02": {"SYN-NODE-SHA-HUB", "SYN-NODE-KUL-HUB", "SYN-NODE-SIN-LM"},
	}
	for candidate, amount := range map[string]int64{"SYN-CANDIDATE-01": 4200, "SYN-CANDIDATE-02": 3900} {
		area, err := nrdomain.NewServiceAreaResolution(nrdomain.ServiceAreaResolutionSpec{
			Candidate:   mustNR(t, nrdomain.NewCandidateID, candidate),
			Outcome:     nrdomain.AreaCoversDestination,
			AreaVersion: mustNR(t, nrdomain.NewServiceAreaVersionReference, "SYN-AREA/v1"),
		})
		if err != nil {
			t.Fatalf("服务区域：%v", err)
		}
		cost, err := nrdomain.NewPricedCandidateCost(mustNR(t, nrdomain.NewCandidateID, candidate), amount, "CNY")
		if err != nil {
			t.Fatalf("候选成本：%v", err)
		}
		evidence.ServiceAreas = append(evidence.ServiceAreas, area)
		evidence.CandidateCosts = append(evidence.CandidateCosts, cost)
		evidence.Paths = append(evidence.Paths, nrports.CandidatePath{
			Candidate: mustNR(t, nrdomain.NewCandidateID, candidate),
			Legs:      legsThrough(t, paths[candidate]...),
		})
	}
	restriction, err := nrdomain.NewHardConstraintFinding(nrdomain.HardConstraintFindingSpec{
		Candidate:   mustNR(t, nrdomain.NewCandidateID, "SYN-CANDIDATE-01"),
		Outcome:     nrdomain.RestrictionApplies,
		Restriction: mustNR(t, nrdomain.NewRestrictionReference, "CUSTOMS-ELIGIBILITY/SYN-CC-16"),
	})
	if err != nil {
		t.Fatalf("硬限制：%v", err)
	}
	evidence.HardConstraints = append(evidence.HardConstraints, restriction)
	pickup := fixture.intakeAt(t, psdomain.OffsitePickupSource, pickupReassessVersion, pickupReassessPlace, pickupReassessAt)

	deliver(t, fixture.syn, fixture.intake, pickup, pickup)

	row, found := pickupReassessmentRowOf(t, fixture.syn, pickupReassessCorrel)
	if !found {
		t.Fatalf("复核册没有揽收那一行；未判 = %+v", notJudgedRows(t, fixture.syn))
	}
	if row.conclusion != "PLAN_LAPSED" || row.reviewedPlan != "SYN-RPV-63" ||
		row.lapseBasis == nil || *row.lapseBasis != "HARD_CONSTRAINT_RESTRICTION/CUSTOMS-ELIGIBILITY/SYN-CC-16" {
		t.Fatalf("复核 = %+v, want PLAN_LAPSED、依据点名限制", row)
	}
	if row.rerouteState == nil || *row.rerouteState != nrdomain.SuggestionOnly.String() ||
		!row.hasSuggestion || row.hasDecision || row.hasNewPlan {
		t.Fatalf("复核 = %+v, want 只形成建议", row)
	}
	var blockers []string
	if row.blockers == nil || json.Unmarshal([]byte(*row.blockers), &blockers) != nil ||
		!slices.Equal(blockers, []string{"NOT_AT_CONTROLLED_NODE"}) {
		t.Fatalf("阻塞 = %v, want [NOT_AT_CONTROLLED_NODE]", row.blockers)
	}
}
