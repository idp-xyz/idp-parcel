package main

import (
	"testing"

	nrinbox "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/inbox"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件经生产路由表证初始路由按恢复动作分格（ADR-0273 决定二）：重投推不动的那几格让接受决定信封入账定稿、原因落未形成
// 登记，不占着分区烧到 ABANDONED；依赖类未决照旧回滚重投、失败码 dispatch.consumer_undecided、不留未形成原因。

// notFormedReasonOf 读一件包裹在未形成登记里的原因；没有那一行答空串。
func notFormedReasonOf(t *testing.T, fixture *synVerticalFixture, parcel string) string {
	t.Helper()
	querier, err := fixture.db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	var reason string
	rows, err := querier.Query(t.Context(),
		`SELECT reason FROM network_routing.initial_route_not_formed WHERE declared_parcel_id = $1`, parcel)
	if err != nil {
		t.Fatalf("读未形成登记：%v", err)
	}
	defer rows.Close()
	for rows.Next() {
		if err := rows.Scan(&reason); err != nil {
			t.Fatalf("读未形成原因：%v", err)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("读未形成登记：%v", err)
	}
	return reason
}

// acceptSYNRequest 把 SYN 纵切的委托推到已接受，并种上 SYN-PC 适用性：接受决定信封入队，NR 那一路要形成路由判断。
func acceptSYNRequest(t *testing.T, fixture *synVerticalFixture) {
	t.Helper()
	ctx := t.Context()
	fixture.submit(t, ctx)
	fixture.recordPassingJudgments(t, ctx)
	if result := fixture.formDecision(t, ctx); result.State() != psdomain.ShipmentRequestAccepted {
		t.Fatalf("state = %q, want ACCEPTED；pending = %q", result.State(), result.PendingReason())
	}
}

// Covers: `AT-NR-088`、`AT-NR-090` 的派发半边——演示租户采用演示网络，SYN 纵切的委托却没申报地址要素：受理基线的地理
// 投影缺两端国家，演示线路那一条候选证据未知（地址信息不足、关务端点国家缺）。受理基线一经固定不再改写，重投推不动：接受
// 决定信封经生产路由表定稿 PUBLISHED，SYN-PARCEL-01 落未形成登记 CANDIDATE_EVIDENCE_INCOMPLETE，候选与两条缺口同行记下，
// 不形成计划、不交意图。
func TestAnUnresolvableCandidateSettlesTheAcceptanceThroughProductionRoutes(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	acceptSYNRequest(t, fixture)
	seedSYNPCEligibility(t, fixture)
	registerDemoNetworkSeeds(t, fixture.db)
	seedWiringCustomsCatalog(t, fixture.db, demoTenant, true)

	if _, err := fixture.beat.DispatchOnce(t.Context()); err != nil {
		t.Fatalf("第一拍：%v", err)
	}
	if status := outboxStatus(t, fixture.db, synV0DecisionID); status != "PUBLISHED" {
		t.Fatalf("接受信封 status = %q、失败码 %q, want PUBLISHED（候选证据不全入账，不回滚重投）",
			status, recordedFailureCode(t, fixture.db, synV0DecisionID))
	}
	if reason := notFormedReasonOf(t, fixture, "SYN-PARCEL-01"); reason != "CANDIDATE_EVIDENCE_INCOMPLETE" {
		t.Fatalf("未形成原因 = %q, want CANDIDATE_EVIDENCE_INCOMPLETE", reason)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM network_routing.initial_route_not_formed,
			jsonb_array_elements(gaps) AS gap
		WHERE gap->>'reference' LIKE 'GEO_ELEMENTS_MISSING/%'
		   OR gap->>'reference' LIKE 'CUSTOMS_ENDPOINT_COUNTRY_MISSING/%'`); n != 2 {
		t.Fatalf("缺口 %d 条, want 地理要素与关务端点国家两条", n)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM network_routing.initial_route_not_formed,
			jsonb_array_elements(candidates) AS candidate
		WHERE candidate->>'id' = 'SYN-LINE-CN-SG-01@1' AND candidate->>'outcome' = 'EVIDENCE_UNKNOWN'`); n != 1 {
		t.Fatalf("证据未知的演示候选 %d 家, want 1", n)
	}
	fixture.assertNoInitialRoute(t)
	if n := fixture.countInbox(t, synV0AcceptanceConsumer, synV0DecisionID); n != 1 {
		t.Fatalf("NR inbox 行数 = %d, want 1", n)
	}
	if n := fixture.countOutboxOfType(t, synV0InitialRouteType); n != 0 {
		t.Fatalf("初始路由信封 %d 封, want 0——未形成不交意图", n)
	}
}

// Covers: `AT-NR-091` 的派发半边——依赖类未决照旧回滚重投：没种 SYN-PC 适用性，解析库里没有 SYN-RES-01 那一行，适用性
// 视图答依赖不可用。接受决定信封的失败码是 dispatch.consumer_undecided，NR inbox 无账、交接登记无痕，未形成登记一行不留——
// 依赖恢复之后重投能判，留原因等于把一次故障记成等登记。
func TestADependencyFailureStillRetriesTheAcceptanceThroughProductionRoutes(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	acceptSYNRequest(t, fixture)

	published, err := fixture.beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("第一拍：%v", err)
	}
	if published != 0 {
		t.Fatalf("依赖不可用却定稿了 %d 条", published)
	}
	if got := recordedFailureCode(t, fixture.db, synV0DecisionID); got != "dispatch.consumer_undecided" {
		t.Fatalf("failure_code = %q, want dispatch.consumer_undecided", got)
	}
	if n := fixture.countInbox(t, synV0AcceptanceConsumer, synV0DecisionID); n != 0 {
		t.Fatalf("NR inbox 行数 = %d, want 0——未决回滚", n)
	}
	if n := fixture.countRouteHandoffLogs(t); n != 0 {
		t.Fatalf("route_handoff_log 行数 = %d, want 0", n)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM network_routing.initial_route_not_formed`); n != 0 {
		t.Fatalf("未形成登记 %d 行, want 0——依赖类未决不留原因", n)
	}
	if n := fixture.countOutboxOfType(t, string(nrinbox.AcceptedDecisionEventType)); n != 1 {
		t.Fatalf("接受信封 %d 封, want 1", n)
	}
}
