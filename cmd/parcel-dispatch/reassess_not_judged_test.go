package main

import (
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/postgres/outbox"

	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件对真实 PostgreSQL 证复核未判（ADR-0235）：重投推不动的几格——没有路由历史、证据未配置、冻结形态未配置、原计划
// 适用性看不清（ADR-0250）——入账，
// 原因落 NR 的未判登记，不登本路未决哨兵；携带同一触发的另一封信封再来时重新判一遍。复核路与生产同形（同一个 Over、同一份哨兵名单），
// 只把证据视图与成本取数侧换成替身；失败分区那一条全走生产路由表。依赖类未决仍回滚重投，由 AT-NR-067 那条用例钉。夹具全部合成，
// 只记 `S`。

type notJudgedRow struct {
	correlation, reason string
	triggerAt           time.Time
	positionAt          *time.Time
	gap, condition      *string
}

func notJudgedRows(t *testing.T, syn *synVerticalFixture) []notJudgedRow {
	t.Helper()
	querier, err := syn.db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	rows, err := querier.Query(t.Context(),
		`SELECT correlation_id, reason, trigger_occurred_at, current_position_at, evidence_gap, reassessment_condition
		   FROM network_routing.route_reassessment_not_judged WHERE tenant_id = $1 ORDER BY correlation_id`, "SYN-TENANT-01")
	if err != nil {
		t.Fatalf("列未判：%v", err)
	}
	defer rows.Close()
	var found []notJudgedRow
	for rows.Next() {
		var row notJudgedRow
		if err := rows.Scan(&row.correlation, &row.reason, &row.triggerAt, &row.positionAt, &row.gap, &row.condition); err != nil {
			t.Fatalf("扫未判行：%v", err)
		}
		found = append(found, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("遍历未判：%v", err)
	}
	return found
}

// inboxStatusOf 读这一路名下这封信的入账状态：拒收同样入账，只数行分不出处理完毕与毒丸。
func inboxStatusOf(t *testing.T, syn *synVerticalFixture, consumer, eventID string) string {
	t.Helper()
	querier, err := syn.db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	var status string
	if err := querier.QueryRow(t.Context(),
		`SELECT status FROM `+migrate.SchemaBento+`.inbox WHERE consumer = $1 AND event_id = $2`, consumer, eventID,
	).Scan(&status); err != nil {
		t.Fatalf("%s 名下 %s 的 inbox 行读不回：%v", consumer, eventID, err)
	}
	return status
}

// Covers: AT-NR-074、AT-NR-075、AT-NR-076、AT-NR-078、AT-NR-051——没有路由历史、证据未配置、冻结形态未配置、原计划适用性
// 看不清（选中候选的限制状态未知，这里演关务来源未接）：投递入账（处理完毕，不翻成未决），未判登记一行、原因点名那一格、
// 业务时间取交接，适用性看不清那一格另带取数侧交回的缺口与再判条件，零复核；同一版交接另一封信封再投，仍判不成，再入账，
// 未判仍一行。
func TestReassessmentsThatCannotBeJudgedYetSettleWithTheirReasonOnTheDatabase(t *testing.T) {
	cases := map[string]struct {
		withPlan     bool
		unconfigured bool
		undeclared   bool
		unclear      bool
		want         string
	}{
		"没有路由历史":  {withPlan: false, want: "NO_ROUTING_HISTORY"},
		"证据未配置":   {withPlan: true, unconfigured: true, want: "REASSESS_EVIDENCE_NOT_CONFIGURED"},
		"冻结形态未配置": {withPlan: true, undeclared: true, want: "FREEZE_FORM_UNCONFIGURED"},
		"适用性看不清":  {withPlan: true, unclear: true, want: "PLAN_APPLICABILITY_UNCLEAR"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newReassessOnHandoverFixture(t, true, tc.withPlan)
			fixture.evidence.unconfigured = tc.unconfigured
			if tc.undeclared {
				fixture.evidence.evidence.FreezeRemainingSegmentLimit = nil
				fixture.evidence.evidence.FreezeForm = nrdomain.FreezeFormUndeclared
			}
			if tc.unclear {
				unknown, err := nrdomain.NewHardConstraintFinding(nrdomain.HardConstraintFindingSpec{
					Candidate: mustNR(t, nrdomain.NewCandidateID, "SYN-CANDIDATE-01"),
					Outcome:   nrdomain.ConstraintStatusUnknown,
					Missing:   mustNR(t, nrdomain.NewEvidenceGapReference, "CUSTOMS_APPLICABILITY_NOT_CONNECTED"),
					Reassess:  mustNR(t, nrdomain.NewReassessmentCondition, "CUSTOMS_APPLICABILITY_SOURCE_CONNECTED"),
				})
				if err != nil {
					t.Fatalf("限制状态未知：%v", err)
				}
				fixture.evidence.evidence.HardConstraints = []nrdomain.HardConstraintFinding{unknown}
			}
			eventID := registerHandover(t, fixture.syn, fixture.register, handoverCommand(t, reassessHandoverParcel,
				"SYN-HANDOVER/63/v1", tfdomain.ObjectHandedOver, "", reassessHandoverReceiving))

			for _, asID := range []string{eventID, eventID + "/again"} {
				if err := fixture.lane.Consume(t.Context(), outboxEnvelope(t, fixture.syn, eventID, asID)); err != nil {
					t.Fatalf("投递 %s = %v, want 入账", asID, err)
				}
				if status := inboxStatusOf(t, fixture.syn, reassessOnHandoverConsumerName, asID); status != "PROCESSED" {
					t.Fatalf("%s 的 inbox status = %q, want PROCESSED", asID, status)
				}
			}
			if rows := reassessmentRows(t, fixture.syn); len(rows) != 0 {
				t.Fatalf("未判却落了复核：%+v", rows)
			}
			rows := notJudgedRows(t, fixture.syn)
			if len(rows) != 1 || rows[0].reason != tc.want || !rows[0].triggerAt.Equal(reassessHandoverAt) {
				t.Fatalf("未判 = %+v, want 一行 %s、业务时间 %s", rows, tc.want, reassessHandoverAt)
			}
			if want := "handover/13:SYN-TENANT-01/13:SYN-PARCEL-01/21:SYN-HANDOVER-SCOPE-63/18:SYN-HANDOVER/63/v1"; rows[0].correlation != want {
				t.Fatalf("未判关联 = %q, want %q", rows[0].correlation, want)
			}
			gapped := rows[0].gap != nil && rows[0].condition != nil &&
				*rows[0].gap == "CUSTOMS_APPLICABILITY_NOT_CONNECTED" && *rows[0].condition == "CUSTOMS_APPLICABILITY_SOURCE_CONNECTED"
			if tc.unclear != gapped || (!tc.unclear && (rows[0].gap != nil || rows[0].condition != nil)) {
				t.Fatalf("缺口 = %v、再判条件 = %v；适用性看不清必带取数侧那一对，其余几格不带", rows[0].gap, rows[0].condition)
			}
		})
	}
}

// Covers: AT-NR-078——入账未判之后前提补齐（这件包裹有了计划），同一版交接再来一封信封：重新判，形成一条仍适用的复核；
// 未判那一行照留，原因仍是当时那一格。
func TestANotJudgedHandoverIsJudgedWhenItArrivesAgainAfterItsPrerequisiteOnTheDatabase(t *testing.T) {
	fixture := newReassessOnHandoverFixture(t, true, false)
	eventID := registerHandover(t, fixture.syn, fixture.register, handoverCommand(t, reassessHandoverParcel,
		"SYN-HANDOVER/63/v1", tfdomain.ObjectHandedOver, "", reassessHandoverReceiving))
	if err := fixture.lane.Consume(t.Context(), outboxEnvelope(t, fixture.syn, eventID, eventID)); err != nil {
		t.Fatalf("第一封 = %v, want 入账", err)
	}

	saveThroughSingaporePlan(t, fixture.syn)
	if err := fixture.lane.Consume(t.Context(), outboxEnvelope(t, fixture.syn, eventID, eventID+"/after-plan")); err != nil {
		t.Fatalf("计划落库之后那一封 = %v", err)
	}
	reassessed := reassessmentRows(t, fixture.syn)
	if len(reassessed) != 1 || reassessed[0].conclusion != "STILL_APPLICABLE" {
		t.Fatalf("复核 = %+v, want 前提补齐后判成一条仍适用", reassessed)
	}
	if rows := notJudgedRows(t, fixture.syn); len(rows) != 1 || rows[0].reason != "NO_ROUTING_HISTORY" {
		t.Fatalf("未判 = %+v, want 当时那一行照留", rows)
	}
}

// Covers: AT-NR-077——生产路由表上，已接受包裹没有路由历史时交进节点：复核这一路入账，信封一拍定稿 PUBLISHED，不再
// 占着交接分区重投到 ABANDONED；同一对象随后的第二次交接（同一分区）跟着定稿，也入账未判。证据视图、复核编排与 PS
// 读取全是生产接法；判据只在复核编排里，生产装配与替身装配是同一份。
func TestAHandoverWithoutRoutingHistoryNoLongerHoldsItsPartitionThroughTheProductionRoute(t *testing.T) {
	syn := newSYNVerticalFixture(t)
	acceptWithDeclaredGeo(t, syn)
	store, err := outbox.NewStore(syn.db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	register := newHandoverRegistration(t, syn.db, store)
	first := registerHandover(t, syn, register, handoverCommand(t,
		reassessHandoverParcel, "SYN-HANDOVER/63/v1", tfdomain.ObjectHandedOver, "", reassessHandoverReceiving))
	next := handoverCommand(t, reassessHandoverParcel, "SYN-HANDOVER/63/next", tfdomain.ObjectHandedOver,
		reassessHandoverReceiving, "SYN-NODE-SIN-LM")
	next.Scope = reassessHandoverScope + "-NEXT"
	next.JudgedAt = reassessHandoverAt.Add(6 * time.Hour)
	second := registerHandover(t, syn, register, next)

	// 拍数取派发预算之上：这一路若答未决，预算耗尽之前同一分区的第二封认领不到，第一封落 ABANDONED。
	for range 6 {
		if _, err := syn.beat.DispatchOnce(t.Context()); err != nil {
			t.Fatalf("一拍：%v", err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	for _, eventID := range []string{first, second} {
		if status := outboxStatus(t, syn.db, eventID); status != "PUBLISHED" {
			t.Fatalf("交接信封 %s status = %s，失败码 = %q, want PUBLISHED", eventID, status, recordedFailureCode(t, syn.db, eventID))
		}
		if status := inboxStatusOf(t, syn, reassessOnHandoverConsumerName, eventID); status != "PROCESSED" {
			t.Fatalf("%s 在复核这一路的 inbox status = %q, want PROCESSED", eventID, status)
		}
	}
	if rows := reassessmentRows(t, syn); len(rows) != 0 {
		t.Fatalf("没有路由历史却复核了：%+v", rows)
	}
	rows := notJudgedRows(t, syn)
	if len(rows) != 2 || rows[0].reason != "NO_ROUTING_HISTORY" || rows[1].reason != "NO_ROUTING_HISTORY" {
		t.Fatalf("未判 = %+v, want 两次交接各一行 NO_ROUTING_HISTORY", rows)
	}
}
