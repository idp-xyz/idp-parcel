package main

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/postgres/outbox"

	nrpostgres "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapplication "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件证初始路由信封 FanOut 给 TF 运输准备那一路（票 demo-chain-gaps/18，ADR-0201 决定七）：生产 wireDispatcher、生产 NR
// 交接口入队、真库。夹具全为合成值（S 级）；PS 侧不提交委托，申报读口答「无」，需求量只有件数一维。

const tfPreparationConsumerName = "transport-fulfillment/prepare-opportunity-from-initial-route"

// registerSYNTemplate 用生产登记编排在 fixture 的库里登一份周期安排。出发时刻按 UTC 当地时刻写，物化落在调用方算好的那一刻。
func registerSYNTemplate(
	t *testing.T,
	fixture *synVerticalFixture,
	code, connection, from, to string,
	departs time.Time,
	duration time.Duration,
	capacities ...tfapplication.CapacityDeclarationInput,
) {
	t.Helper()
	templates, err := tfpostgres.NewScheduleTemplates(fixture.db)
	if err != nil {
		t.Fatalf("构造周期安排册：%v", err)
	}
	schedules, err := tfpostgres.NewTransportSchedules(fixture.db)
	if err != nil {
		t.Fatalf("构造班次册：%v", err)
	}
	pools, err := tfpostgres.NewCapacityPools(fixture.db)
	if err != nil {
		t.Fatalf("构造池册：%v", err)
	}
	handler := tfapplication.NewRegisterTransportScheduleHandler(tfapplication.RegisterTransportScheduleDeps{
		Templates: templates, Schedules: schedules, Pools: pools, Clock: systemClock{},
	})
	departs = departs.UTC()
	tenant := mustTF(t, tfdomain.NewTenantID, fixture.identity.TenantID().String())
	var result tfapplication.ScheduleRegistrationResult
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		var registerErr error
		result, registerErr = handler.RegisterTemplate(txCtx, tfapplication.RegisterScheduleTemplateCommand{
			TenantID:           tenant,
			Template:           code,
			Connection:         connection,
			FromNode:           from,
			ToNode:             to,
			Weekdays:           []string{"MON", "TUE", "WED", "THU", "FRI", "SAT", "SUN"},
			DepartureLocalTime: departs.Format("15:04"),
			DurationMinutes:    int64(duration / time.Minute),
			TimeZone:           "UTC",
			ServiceScope:       "SYN-SCOPE-01",
			EffectiveFrom:      departs.AddDate(0, 0, -7).Format("2006-01-02"),
			Capacities:         capacities,
		})
		return registerErr
	})
	if result.Outcome() != tfapplication.ScheduleTemplateRegistered {
		t.Fatalf("登周期安排 %s = %s", code, result.Outcome())
	}
}

// recordPlannedLegs 落一份带给定段链的初始路由判断，连同生产交接口入队的信封。键与 synInitialRouteKey 同一份。
func recordPlannedLegs(t *testing.T, fixture *synVerticalFixture, judgedAt time.Time, legs []nrdomain.PlannedLeg) string {
	t.Helper()
	store, err := outbox.NewStore(fixture.db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	routes, err := nrpostgres.NewInitialRoutes(fixture.db)
	if err != nil {
		t.Fatalf("构造初始路由库：%v", err)
	}
	handoff, err := nrpostgres.NewOutboxInitialRouteHandoff(fixture.db, store, systemClock{})
	if err != nil {
		t.Fatalf("构造初始路由交接：%v", err)
	}
	key := synInitialRouteKey(t, fixture)
	plan, err := nrdomain.FormInitialRoutePlan(nrdomain.InitialRoutePlanSpec{
		Key:           key,
		Version:       mustNR(t, nrdomain.NewRoutePlanVersionID, "SYN-RPV-0018"),
		Selected:      mustNR(t, nrdomain.NewCandidateID, "SYN-CAND-01"),
		Candidates:    synInitialRouteCandidates(t),
		Legs:          legs,
		Strategy:      mustNR(t, nrdomain.NewRouteStrategyReference, "SYN-STRATEGY/v1"),
		ViewRevision:  mustNR(t, nrdomain.NewNetworkViewRevision, "SYN-NETVIEW-01"),
		JudgedAt:      judgedAt,
		EffectiveFrom: judgedAt,
	})
	if err != nil {
		t.Fatalf("形成计划：%v", err)
	}
	record := nrports.InitialRouteRecord{Key: key, Plan: plan, HasPlan: true}
	correlation := mustNR(t, nrdomain.NewRequestCorrelationID, "SYN-ROUTE-CORR-18")
	var outcome nrports.InitialRouteSaveOutcome
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		var saveErr error
		if outcome, saveErr = routes.Save(txCtx, record); saveErr != nil {
			return saveErr
		}
		return handoff.HandOffInitialRoute(txCtx, nrports.InitialRouteHandoffIntent{Correlation: correlation, Record: record})
	})
	if outcome != nrports.InitialRouteSaved {
		t.Fatalf("save outcome = %v, want 已写入", outcome)
	}
	return synInitialRouteEventID(fixture)
}

// plannedLegOn 造一段计划履约段，窗口依据照 NR 计划段的拼写指名连接。
func plannedLegOn(t *testing.T, connection, from, to string, earliest, latest time.Time) nrdomain.PlannedLeg {
	t.Helper()
	window, err := nrdomain.NewPlannedTimeWindow(earliest, latest,
		mustNR(t, nrdomain.NewWindowBasisReference, "CONNECTION/"+connection+"@1"))
	if err != nil {
		t.Fatalf("构造窗口：%v", err)
	}
	leg, err := nrdomain.NewPlannedLeg(nrdomain.PlannedLegSpec{
		From:        mustNR(t, nrdomain.NewPlanNodeReference, from),
		To:          mustNR(t, nrdomain.NewPlanNodeReference, to),
		Responsible: mustNR(t, nrdomain.NewResponsiblePartyReference, connection+"@1"),
		Window:      window,
	})
	if err != nil {
		t.Fatalf("构造段：%v", err)
	}
	return leg
}

// preparationOutcomes 交回本 fixture 租户名下各段的准备结论，键是段引用。
func preparationOutcomes(t *testing.T, fixture *synVerticalFixture) map[string]string {
	t.Helper()
	querier, err := fixture.db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	rows, err := querier.Query(t.Context(),
		`SELECT planned_leg_ref, outcome FROM transport_fulfillment.transport_preparation WHERE tenant_id = $1`,
		fixture.identity.TenantID().String())
	if err != nil {
		t.Fatalf("读准备结果：%v", err)
	}
	defer rows.Close()
	outcomes := map[string]string{}
	for rows.Next() {
		var leg, outcome string
		if err := rows.Scan(&leg, &outcome); err != nil {
			t.Fatalf("扫准备结果：%v", err)
		}
		outcomes[leg] = outcome
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("读准备结果：%v", err)
	}
	return outcomes
}

// Covers: AT-TF-126（ADR-0201 决定六至八经生产路由表）——一份三段的初始路由判断，TF 那一路逐段形成结果：登了件数池的那一连接答
// `运输机会已准备`（需求量只有件数一维，评估时物化、没登记的那一班也能判）；空册的连接答`未配置`入账；只声明了重量池的
// 连接答`容量待确认`（申报读口答「无」，重量待补充）。三格都入账，信封定稿。
func TestAFormedInitialRoutePreparesEveryLegThroughTheRouteTable(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	now := time.Now().UTC().Truncate(time.Minute)
	registerSYNTemplate(t, fixture, "SYN-TPL-18-A", "SYN-CONN-18-A", "SYN-NODE-18-1", "SYN-NODE-18-2",
		now.Add(3*time.Hour), time.Hour,
		tfapplication.CapacityDeclarationInput{Dimension: "PIECES", Unit: "PCS", Capacity: 10})
	registerSYNTemplate(t, fixture, "SYN-TPL-18-C", "SYN-CONN-18-C", "SYN-NODE-18-3", "SYN-NODE-18-4",
		now.Add(3*time.Hour), time.Hour,
		tfapplication.CapacityDeclarationInput{Dimension: "WEIGHT", Unit: "KG", Capacity: 1000})
	eventID := recordPlannedLegs(t, fixture, now, []nrdomain.PlannedLeg{
		plannedLegOn(t, "SYN-CONN-18-A", "SYN-NODE-18-1", "SYN-NODE-18-2", now.Add(2*time.Hour), now.Add(6*time.Hour)),
		plannedLegOn(t, "SYN-CONN-18-B", "SYN-NODE-18-2", "SYN-NODE-18-3", now.Add(6*time.Hour), now.Add(8*time.Hour)),
		plannedLegOn(t, "SYN-CONN-18-C", "SYN-NODE-18-3", "SYN-NODE-18-4", now.Add(2*time.Hour), now.Add(30*time.Hour)),
	})

	published, err := fixture.beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("一拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("published = %d, want 1；失败码 = %q", published, recordedFailureCode(t, fixture.db, eventID))
	}
	want := map[string]string{
		"SYN-RPV-0018#1": "OPPORTUNITY_PREPARED",
		"SYN-RPV-0018#2": "NOT_CONFIGURED",
		"SYN-RPV-0018#3": "CAPACITY_UNCONFIRMED",
	}
	got := preparationOutcomes(t, fixture)
	if len(got) != len(want) {
		t.Fatalf("准备结果 = %v, want %v", got, want)
	}
	for leg, outcome := range want {
		if got[leg] != outcome {
			t.Fatalf("段 %s = %q, want %q（全部 %v）", leg, got[leg], outcome, got)
		}
	}
	if n := fixture.countInbox(t, tfPreparationConsumerName, eventID); n != 1 {
		t.Fatalf("TF inbox 行数 = %d, want 1", n)
	}
	if n := fixture.countInbox(t, deriveInitialRouteConsumerName, eventID); n != 1 {
		t.Fatalf("VE inbox 行数 = %d, want 1", n)
	}
}

// Covers: AT-TF-127——无当前路由的判断在 TF 那一路零写入：没有段，就没有可挂准备结果的身份；消费照样入账、信封定稿。
func TestANoCurrentRouteJudgmentWritesNoPreparation(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	eventID := recordInitialRouteJudgment(t, fixture, false)

	published, err := fixture.beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("一拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("published = %d, want 1；失败码 = %q", published, recordedFailureCode(t, fixture.db, eventID))
	}
	if got := preparationOutcomes(t, fixture); len(got) != 0 {
		t.Fatalf("无路线判断写了准备结果：%v", got)
	}
	if n := fixture.countInbox(t, tfPreparationConsumerName, eventID); n != 1 {
		t.Fatalf("TF inbox 行数 = %d, want 1——零写入不是不入账", n)
	}
}

// Covers: AT-TF-134（计划 D01-1「各自 inbox、各列未决哨兵」）——TF 那一路停在未决（班次册读不动：一份周期安排的时区在库里坏了），
// VE 那一路照样投影并入账；重投时 VE 按 inbox 跳过、不重复投影，TF 那一路在班次册恢复后形成结果。
func TestATransportPreparationUndecidedRedeliveryDoesNotReprojectVisibility(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	now := time.Now().UTC().Truncate(time.Minute)
	registerSYNTemplate(t, fixture, "SYN-TPL-18-A", "SYN-CONN-18-A", "SYN-NODE-18-1", "SYN-NODE-18-2",
		now.Add(3*time.Hour), time.Hour)
	setTemplateTimeZone(t, fixture, "SYN-TPL-18-A", "SYN/NOWHERE")
	eventID := recordPlannedLegs(t, fixture, now, []nrdomain.PlannedLeg{
		plannedLegOn(t, "SYN-CONN-18-A", "SYN-NODE-18-1", "SYN-NODE-18-2", now.Add(2*time.Hour), now.Add(6*time.Hour)),
	})

	published, err := fixture.beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("第一拍：%v", err)
	}
	if published != 0 {
		t.Fatalf("TF 未决却定稿了 %d 条", published)
	}
	if got := recordedFailureCode(t, fixture.db, eventID); got != "dispatch.consumer_undecided" {
		t.Fatalf("failure_code = %q, want dispatch.consumer_undecided", got)
	}
	if n := fixture.countInbox(t, deriveInitialRouteConsumerName, eventID); n != 1 {
		t.Fatalf("VE inbox 行数 = %d, want 1——VE 那一路不该被 TF 的未决拖住", n)
	}
	if n := fixture.countInbox(t, tfPreparationConsumerName, eventID); n != 0 {
		t.Fatalf("TF inbox 行数 = %d, want 0——未决必须回滚", n)
	}
	derived := fixture.countOutboxOfType(t, trackingProjectionDerivedType)
	if derived != 1 {
		t.Fatalf("投影交接信封 = %d, want 1", derived)
	}

	setTemplateTimeZone(t, fixture, "SYN-TPL-18-A", "UTC")
	// 重拍里同时还有第一拍 VE 派生交出的那封投影信封，拍内的定稿数分不出「哪一封」，按 event_id 看状态。
	if _, err := fixture.beat.DispatchOnce(t.Context()); err != nil {
		t.Fatalf("重拍：%v", err)
	}
	if status := outboxStatus(t, fixture.db, eventID); status != "PUBLISHED" {
		t.Fatalf("班次册恢复后重投没定稿：status = %q；失败码 = %q", status, recordedFailureCode(t, fixture.db, eventID))
	}
	if n := fixture.countInbox(t, deriveInitialRouteConsumerName, eventID); n != 1 {
		t.Fatalf("重投后 VE inbox 行数 = %d, want 1", n)
	}
	if n := fixture.countOutboxOfType(t, trackingProjectionDerivedType); n != derived {
		t.Fatalf("重投后投影交接信封 = %d, want %d——VE 重复投影了", n, derived)
	}
	assertUnclassifiedInitialRouteProjection(t, fixture, "initial-route-formed")
	if got := preparationOutcomes(t, fixture); got["SYN-RPV-0018#1"] != "CAPACITY_UNCONFIRMED" {
		t.Fatalf("恢复后的准备结果 = %v, want 段 1 容量待确认（班次不带池）", got)
	}
}

// setTemplateTimeZone 绕过登记口直接改库里一份周期安排的时区：库上不复制时区表，坏时区只在读回过构造门时才响。
func setTemplateTimeZone(t *testing.T, fixture *synVerticalFixture, code, zone string) {
	t.Helper()
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		executor, err := fixture.db.RequireExecutor(txCtx)
		if err != nil {
			return err
		}
		_, err = executor.Exec(txCtx,
			`UPDATE transport_fulfillment.schedule_template SET time_zone = $3 WHERE tenant_id = $1 AND template_code = $2`,
			fixture.identity.TenantID().String(), code, zone)
		return err
	})
}
