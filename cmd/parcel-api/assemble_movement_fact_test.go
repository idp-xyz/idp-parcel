package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	ccregistration "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfcustoms "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/customscompliance"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

var movementFactOccurredAt = time.Date(2026, 8, 22, 6, 0, 0, 0, time.UTC)

// Covers: `/transport-fulfillment/movement-facts` 的第二参是真编排（票 tf-segment-lifecycle-closure/05、demo-chain-gaps/32）——移动
// 事实登记册在真实 PostgreSQL 上装得起来，事务边界成立（重放走已有版本，证首笔真的提交了）。出发一律落册：没声明段、班次也没登，
// 照样 MOVEMENT_FACT_RECORDED，读数停在本上下文这一侧答未决，与事实同一行（AT-TF-114 真库半边）。测试输入是隔离合成，只记 `S`。
func TestTheWiredMovementFactAnswersHonestlyAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}

	movement, err := buildMovementFactOrchestration(db)
	if err != nil {
		t.Fatalf("装配移动事实编排：%v", err)
	}

	arrival := recordMovementFactCommand(t, "SYN-MOVEMENT-ARR-1", tfdomain.ArrivalFact)
	recorded, err := movement.Record(t.Context(), arrival)
	if err != nil {
		t.Fatalf("登记到达：%v", err)
	}
	if got := recorded.Outcome(); got != tfapp.MovementFactRecorded {
		t.Fatalf("outcome = %v, want MOVEMENT_FACT_RECORDED", got)
	}
	if record, has := recorded.Record(); !has {
		t.Fatal("登记成功却没带回记录")
	} else if _, read := record.Fact.GateReading(); read {
		t.Fatal("到达带了门禁读数")
	}

	replay, err := movement.Record(t.Context(), arrival)
	if err != nil {
		t.Fatalf("重放到达：%v", err)
	}
	if got := replay.Outcome(); got != tfapp.MovementFactVersionExists {
		t.Fatalf("outcome = %v, want MOVEMENT_FACT_VERSION_EXISTS——重放没走已有版本，首笔事务没有提交", got)
	}

	departure := recordMovementFactCommand(t, "SYN-MOVEMENT-DEP-1", tfdomain.DepartureFact)
	result, err := movement.Record(t.Context(), departure)
	if err != nil {
		t.Fatalf("登记出发：%v", err)
	}
	if got := result.Outcome(); got != tfapp.MovementFactRecorded {
		t.Fatalf("outcome = %v, want MOVEMENT_FACT_RECORDED——出发一律落册", got)
	}
	record, _ := result.Record()
	reading, read := record.Fact.GateReading()
	if !read || reading.Answer() != tfdomain.DepartureGateUndecided || len(reading.Stops()) != 2 {
		t.Fatalf("读数 = %s %v, want 未决、停在段未声明与班次不在册", reading.Answer(), reading.Stops())
	}
	if stored := storedGateReading(t, pool, "SYN-MOVEMENT-DEP-1"); stored != "UNDECIDED" {
		t.Fatalf("库面读数 = %q, want UNDECIDED（与事实同一行）", stored)
	}
}

func recordMovementFactCommand(t *testing.T, fact string, kind tfdomain.MovementFactKind) tfapp.RecordMovementFactCommand {
	t.Helper()
	return tfapp.RecordMovementFactCommand{
		TenantID:   mustValue(t, tfdomain.NewTenantID, "SYN-TENANT-1"),
		Fact:       fact,
		Schedule:   "SYN-SCHEDULE-1",
		Kind:       kind,
		Location:   "SYN-HUB-1",
		Source:     "SYN-OWN-FLEET-SCAN/v1",
		Version:    "SYN-MFV-000000000001",
		OccurredAt: movementFactOccurredAt,
	}
}

// storedGateReading 读一条事实首版那一行的读数列；出发的读数与事实同一行，读得到就是同一笔落下的。
func storedGateReading(t *testing.T, pool *pgxpool.Pool, fact string) string {
	t.Helper()
	var reading *string
	if err := pool.QueryRow(t.Context(),
		`SELECT gate_reading FROM transport_fulfillment.transport_movement_fact WHERE fact_ref = $1`, fact).Scan(&reading); err != nil {
		t.Fatalf("查读数：%v", err)
	}
	if reading == nil {
		return ""
	}
	return *reading
}

// ---- 演示动线上的自营 SHA→SZX 出发：生产装配、演示种子、真库 ----

const (
	departureLineTenant   = "SYN-TENANT-01"
	departureLineSchedule = "SYN-SCHED-SHA-SZX-DAILY/2026-10-21"
	departureLineUnit     = "SYN-UNIT-DEP-LINE-01"
	departureLineCase     = "SYN-CASE-DEP-LINE-01"
	departureLineProc     = "SYN-PROC-CN-EXPORT"
)

// 演示班次 2026-10-21 当地 22:00 自上海枢纽开出（周期安排种子），交接比它早一小时，出发晚五分钟。
var (
	departureLineHandedOverAt = time.Date(2026, 10, 21, 13, 0, 0, 0, time.UTC)
	departureLineDepartedAt   = time.Date(2026, 10, 21, 14, 5, 0, 0, time.UTC)
)

type departureLine struct {
	t        *testing.T
	pool     *pgxpool.Pool
	db       *bentopg.DB
	movement tfhttp.MovementFactHandler
	handover tfhttp.HandoverHandler
}

// newDepartureLine 按演示种子原样登关务节点区域（customs/27–30）与上海→深圳的周期安排（transport/01），再按运行日期显式登一个
// 班次——与 seed.sh 和演示脚本走的是同一批登记用例。
func newDepartureLine(t *testing.T) *departureLine {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	ctx := t.Context()

	customs, err := buildCustomsAdmissionOrchestration(db)
	if err != nil {
		t.Fatalf("装配关务准入：%v", err)
	}
	for _, name := range []string{"27-node-zone-sha-hub.json", "28-node-zone-szx-gate.json", "29-node-zone-sin-hub.json", "30-node-zone-sin-lm.json"} {
		command, err := ccregistration.NodeCustomsZoneFromJSON(demoSeed(t, "customs", name))
		if err != nil {
			t.Fatalf("译 %s：%v", name, err)
		}
		if _, err := customs.nodeZoneRegistration.Handle(ctx, command); err != nil {
			t.Fatalf("登 %s：%v", name, err)
		}
	}

	schedules, err := buildTransportScheduleRegistration(db)
	if err != nil {
		t.Fatalf("装配班次登记：%v", err)
	}
	var template struct {
		TenantID string `json:"tenantId"`
		tfhttp.ScheduleTemplateRegistrationPayload
	}
	if err := json.Unmarshal(demoSeed(t, "transport", "01-schedule-template-sha-szx.json"), &template); err != nil {
		t.Fatalf("读周期安排种子：%v", err)
	}
	tenant := mustValue(t, tfdomain.NewTenantID, template.TenantID)
	templateCommand, err := template.ScheduleTemplateRegistrationPayload.Command(tenant)
	if err != nil {
		t.Fatalf("译周期安排：%v", err)
	}
	if result, err := schedules.RegisterTemplate(ctx, templateCommand); err != nil || result.Outcome() != tfapp.ScheduleTemplateRegistered {
		t.Fatalf("登周期安排：%s / %v", result.Outcome(), err)
	}
	scheduleCommand, err := tfhttp.TransportScheduleRegistrationPayload{Template: template.Template, RunningDate: "2026-10-21"}.Command(tenant)
	if err != nil {
		t.Fatalf("译班次：%v", err)
	}
	if result, err := schedules.RegisterSchedule(ctx, scheduleCommand); err != nil || result.Outcome() != tfapp.TransportScheduleRegistered {
		t.Fatalf("登班次：%s / %v", result.Outcome(), err)
	}

	controlFacts, err := buildControlFactOrchestrations(db)
	if err != nil {
		t.Fatalf("装配控制事实：%v", err)
	}
	movement, err := buildMovementFactOrchestration(db)
	if err != nil {
		t.Fatalf("装配移动事实：%v", err)
	}
	return &departureLine{t: t, pool: pool, db: db, movement: movement, handover: controlFacts.handover}
}

func demoSeed(t *testing.T, family, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "scripts", "demo-seeds", "data", family, name))
	if err != nil {
		t.Fatalf("读演示种子 %s/%s：%v", family, name, err)
	}
	return raw
}

// handOver 在上海枢纽把一件交给自营干线，立段——演示动线上「用现有交接口立 SHA→SZX 自营段」那一步。
func (line *departureLine) handOver(segment, object string) {
	line.t.Helper()
	command, err := tfhttp.HandoverRegistrationPayload{
		Object: object, Scope: "SYN-SCOPE/sha-hub-dock", ReleasedBy: "SYN-PARTY/sha-hub", ReceivedBy: "SYN-PARTY/own-linehaul",
		Verdict: "HANDED_OVER", ReleasingEvidence: "SYN-EVIDENCE/release-" + object, ReceivingEvidence: "SYN-EVIDENCE/receive-" + object,
		Rule: "SYN-RULE/handover@v1", Version: "v1", JudgedAt: departureLineHandedOverAt.Format(time.RFC3339), Segment: segment,
	}.Command(mustValue(line.t, tfdomain.NewTenantID, departureLineTenant))
	if err != nil {
		line.t.Fatalf("译交接：%v", err)
	}
	if result, err := line.handover.Register(line.t.Context(), command); err != nil || result.Outcome() != tfapp.HandoverRegistered {
		line.t.Fatalf("交接 %s：%s / %v", object, result.Outcome(), err)
	}
}

func (line *departureLine) departureCommand(fact, segment string) tfapp.RecordMovementFactCommand {
	return tfapp.RecordMovementFactCommand{
		TenantID: mustValue(line.t, tfdomain.NewTenantID, departureLineTenant), Fact: fact, Schedule: departureLineSchedule,
		Kind: tfdomain.DepartureFact, Location: "SYN-NODE-SHA-HUB", Source: frontlineMovementSource, Version: "v1",
		OccurredAt: departureLineDepartedAt, Segment: segment,
	}
}

func (line *departureLine) depart(fact, segment string) tfdomain.DepartureGateReading {
	line.t.Helper()
	result, err := line.movement.Record(line.t.Context(), line.departureCommand(fact, segment))
	if err != nil {
		line.t.Fatalf("出发 %s：%v", fact, err)
	}
	if result.Outcome() != tfapp.MovementFactRecorded {
		line.t.Fatalf("出发 %s：outcome = %s, want MOVEMENT_FACT_RECORDED", fact, result.Outcome())
	}
	record, _ := result.Record()
	reading, _ := record.Fact.GateReading()
	if stored := storedGateReading(line.t, line.pool, fact); stored != reading.Answer().String() {
		line.t.Fatalf("出发 %s：库面读数 %q 与答复 %q 不是同一份", fact, stored, reading.Answer())
	}
	return reading
}

func (line *departureLine) seed(sql string, args ...any) {
	line.t.Helper()
	if _, err := line.pool.Exec(line.t.Context(), sql, args...); err != nil {
		line.t.Fatalf("播种：%v", err)
	}
}

// Covers: AT-TF-110（演示判据，证据 S）——自营 SHA→SZX：按演示种子登了节点区域与周期安排，交接立段后，一线设备在生产渠道上报出发
// 答 201，读数不受门禁：关务按区域只适用装载出发，包裹不在任何案卷里。起讫取班次方向；答复里逐对象只有对象与那一格，关务判断
// 标识与依据原文落在库面那一行（ADR-0205 决定一）。
func TestADemoSelfOperatedDepartureFromShanghaiToShenzhenAnswersCreatedNotGuarded(t *testing.T) {
	line := newDepartureLine(t)
	line.handOver("SYN-SEG-SHA-SZX-DEMO", "SYN-PARCEL-DEMO-01")

	endpoint := tfhttp.NewRecordMovementFactEndpoint(workFactTransportForTest(t), line.movement)
	request := httptest.NewRequest(http.MethodPost, "/transport-fulfillment/movement-facts", strings.NewReader(
		`{"fact":"SYN-MOVE-DEMO-01","schedule":"`+departureLineSchedule+`","kind":"DEPARTURE","location":"SYN-NODE-SHA-HUB",`+
			`"version":"v1","occurredAt":"2026-10-21T22:05:00+08:00","segment":"SYN-SEG-SHA-SZX-DEMO","gateRequired":false}`))
	stampWorkFact(request, "OFFSITE", "SYN-OFFSITE-01", "SYN-DEVICE-OFFSITE")
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("HTTP %d %s, want 201", response.Code, response.Body.String())
	}
	var answer struct {
		Outcome     string `json:"outcome"`
		Segment     string `json:"segment"`
		GateReading struct {
			Answer  string           `json:"answer"`
			From    string           `json:"from"`
			To      string           `json:"to"`
			Objects []map[string]any `json:"objects"`
		} `json:"gateReading"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		t.Fatalf("decode %s：%v", response.Body.String(), err)
	}
	reading := answer.GateReading
	if answer.Outcome != "MOVEMENT_FACT_RECORDED" || answer.Segment != "SYN-SEG-SHA-SZX-DEMO" || reading.Answer != "NOT_GUARDED" ||
		reading.From != "SYN-NODE-SHA-HUB" || reading.To != "SYN-NODE-SZX-GATE" {
		t.Fatalf("答复 = %s", response.Body.String())
	}
	if len(reading.Objects) != 1 || reading.Objects[0]["object"] != "SYN-PARCEL-DEMO-01" || reading.Objects[0]["answer"] != "NOT_GUARDED" ||
		len(reading.Objects[0]) != 2 {
		t.Fatalf("逐对象那一格只该有对象与答复 = %+v", reading.Objects)
	}
	if stored := storedGateReading(t, line.pool, "SYN-MOVE-DEMO-01"); stored != "NOT_GUARDED" {
		t.Fatalf("库面读数 = %q", stored)
	}
	var judgement, action, cause string
	if err := line.pool.QueryRow(t.Context(),
		`SELECT gate_reading_detail #>> '{findings,0,judgement}', gate_reading_detail #>> '{findings,0,bases,0,action}',
		        gate_reading_detail #>> '{findings,0,bases,0,cause}'
		   FROM transport_fulfillment.transport_movement_fact WHERE fact_ref = 'SYN-MOVE-DEMO-01'`).Scan(&judgement, &action, &cause); err != nil {
		t.Fatalf("查库面依据：%v", err)
	}
	if !strings.HasPrefix(judgement, "AADJ-") || action != "LOADING_DEPARTURE" || cause != "OUTSIDE_CUSTOMS_BOOKS" {
		t.Fatalf("库面逐对象依据 = %s %s %s，关务判断标识与依据原文只落册", judgement, action, cause)
	}
}

// Covers: AT-TF-113 真库半边与票 demo-chain-gaps/68 的演示判据（证据 S）——演示动线上同一段、同一刻的三条出发，载荷分别带
// `"gateRequired":true`、带 `false`、不带，经生产作业事实渠道各答 201 MOVEMENT_FACT_RECORDED，答复除事实标识外逐字相同：读数、段与
// 放行凭据都在答复里，放行凭据也照存在库面那一行。内容摘要两侧现算、不存，于是拿重放验它：用带 `true` 那一条的身份换另两种写法
// 再报，答已有版本并回显同一份读数——退役键若进了摘要，这里会答 SOURCE_CONFLICT。
func TestTheRetiredGateRequiredKeyChangesNothingOnTheDemoDeparture(t *testing.T) {
	line := newDepartureLine(t)
	line.handOver("SYN-SEG-SHA-SZX-68", "SYN-PARCEL-68-01")
	endpoint := tfhttp.NewRecordMovementFactEndpoint(workFactTransportForTest(t), line.movement)
	post := func(fact, retiredKey string) (int, map[string]any) {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/transport-fulfillment/movement-facts", strings.NewReader(
			`{"fact":"`+fact+`","schedule":"`+departureLineSchedule+`","kind":"DEPARTURE","location":"SYN-NODE-SHA-HUB",`+
				`"version":"v1","occurredAt":"2026-10-21T22:05:00+08:00","segment":"SYN-SEG-SHA-SZX-68",`+
				`"gateClearance":"SYN-GATE/release-68"`+retiredKey+`}`))
		stampWorkFact(request, "OFFSITE", "SYN-OFFSITE-01", "SYN-DEVICE-OFFSITE")
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, request)
		var answer map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
			t.Fatalf("decode %s：%v", response.Body.String(), err)
		}
		return response.Code, answer
	}
	// 去掉事实标识与答格，余下的按键排序成串：读数、段、放行凭据、班次、地点、业务时间都在里面。
	rest := func(answer map[string]any) string {
		t.Helper()
		kept := map[string]any{}
		for key, value := range answer {
			if key != "fact" && key != "outcome" {
				kept[key] = value
			}
		}
		raw, err := json.Marshal(kept)
		if err != nil {
			t.Fatalf("encode：%v", err)
		}
		return string(raw)
	}

	variants := []struct{ fact, retiredKey string }{
		{"SYN-MOVE-68-TRUE", `,"gateRequired":true`},
		{"SYN-MOVE-68-FALSE", `,"gateRequired":false`},
		{"SYN-MOVE-68-ABSENT", ``},
	}
	var first string
	for _, variant := range variants {
		status, answer := post(variant.fact, variant.retiredKey)
		if status != http.StatusCreated || answer["outcome"] != "MOVEMENT_FACT_RECORDED" {
			t.Fatalf("%s：HTTP %d %v, want 201 MOVEMENT_FACT_RECORDED", variant.fact, status, answer)
		}
		reading, _ := answer["gateReading"].(map[string]any)
		if reading["answer"] != "NOT_GUARDED" || answer["gateClearance"] != "SYN-GATE/release-68" {
			t.Fatalf("%s：答复 = %v，want 读数不受门禁、放行凭据照回", variant.fact, answer)
		}
		if first == "" {
			first = rest(answer)
		} else if got := rest(answer); got != first {
			t.Fatalf("%s 的答复与带 true 那一条不同：\n%s\n%s", variant.fact, got, first)
		}
		var stored, clearance string
		if err := line.pool.QueryRow(t.Context(),
			`SELECT gate_reading, gate_clearance FROM transport_fulfillment.transport_movement_fact WHERE fact_ref = $1`,
			variant.fact).Scan(&stored, &clearance); err != nil {
			t.Fatalf("查库面 %s：%v", variant.fact, err)
		}
		if stored != "NOT_GUARDED" || clearance != "SYN-GATE/release-68" {
			t.Fatalf("%s 库面 = %s / %s", variant.fact, stored, clearance)
		}
	}

	for _, retiredKey := range []string{`,"gateRequired":false`, ``} {
		status, answer := post("SYN-MOVE-68-TRUE", retiredKey)
		if status != http.StatusOK || answer["outcome"] != "MOVEMENT_FACT_VERSION_EXISTS" {
			t.Fatalf("换写法%q重报：HTTP %d %v, want 200 MOVEMENT_FACT_VERSION_EXISTS——退役键进了内容摘要", retiredKey, status, answer)
		}
		if got := rest(answer); got != first {
			t.Fatalf("重放回显的不是首次那一份：\n%s\n%s", got, first)
		}
	}
}

// Covers: 票 demo-chain-gaps/32 完成判据「五格读数与事实同一笔（真库）」「CC 读不动也落库」「段不在册照记」；AT-TF-111、AT-TF-112、
// AT-TF-114 真库半边——生产装配下五格各落一条出发：
//   - 不受门禁：不在案的包裹同区出发；
//   - 放行：在案在单元、装载出发那一道门禁满足、出发前已有全部放行；
//   - 阻断：直接落在包裹上的装载出发限制——照记，不拒收；
//   - 未决：声明的段不在册——照记；
//   - 读不动：关务的限制册读不到——照记，指名关务那一口。
//
// 每条的读数与事实在同一行，答复与库面是同一份。
func TestEveryGateReadingIsRecordedInTheSameRowAsItsDeparture(t *testing.T) {
	line := newDepartureLine(t)

	line.handOver("SYN-SEG-NOT-GUARDED", "SYN-PARCEL-NOT-GUARDED")
	if got := line.depart("SYN-MOVE-NOT-GUARDED", "SYN-SEG-NOT-GUARDED").Answer(); got != tfdomain.DepartureGateNotGuarded {
		t.Fatalf("不在案同区出发 = %s", got)
	}

	line.handOver("SYN-SEG-ADMITTED", "SYN-PARCEL-ADMITTED")
	line.seed(`INSERT INTO customs_compliance.customs_case
			(tenant_id, jurisdiction_ref, direction, procedure_ref, obligation_ref, case_id, parcels, roles, established_at)
		 VALUES ($1, 'SYN-JUR-CN', 'EXPORT', $2, 'SYN-OBLIGATION-01', $3,
		         jsonb_build_array(jsonb_build_object('parcel', 'SYN-PARCEL-ADMITTED', 'customer', 'SYN-ACCOUNT-01', 'sourceRef', 'SYN-SRC-01')),
		         '[]'::jsonb, $4)`,
		departureLineTenant, departureLineProc, departureLineCase, departureLineDepartedAt.Add(-48*time.Hour))
	line.seed(`INSERT INTO customs_compliance.declaration_unit (tenant_id, unit_id, case_id, procedure_ref, members, formed_at)
		 VALUES ($1, $2, $3, $4, jsonb_build_array('SYN-PARCEL-ADMITTED'), $5)`,
		departureLineTenant, departureLineUnit, departureLineCase, departureLineProc, departureLineDepartedAt.Add(-47*time.Hour))
	line.seed(`INSERT INTO customs_compliance.gate_condition_catalog (tenant_id, scope_ref, action, boundary_ref, registered_at)
		 VALUES ($1, $2, 'LOADING_DEPARTURE', $3, $4)`,
		departureLineTenant, departureLineUnit, departureLineProc, departureLineDepartedAt.Add(-46*time.Hour))
	line.seed(`INSERT INTO customs_compliance.gate_condition_finding
			(tenant_id, scope_ref, action, boundary_ref, precondition_ref, finding_state)
		 VALUES ($1, $2, 'LOADING_DEPARTURE', $3, 'SYN-PRE-EXPORT-DECL-RELEASED', 'MET')`,
		departureLineTenant, departureLineUnit, departureLineProc)
	line.seed(`INSERT INTO customs_compliance.gate_condition_duty_payment_rule
			(tenant_id, scope_ref, action, boundary_ref, not_a_precondition, accept_coverage, accept_delta, accept_validity, registered_at)
		 VALUES ($1, $2, 'LOADING_DEPARTURE', $3, true, '[]', '[]', '[]', $4)`,
		departureLineTenant, departureLineUnit, departureLineProc, departureLineDepartedAt.Add(-46*time.Hour))
	releasedAt := departureLineDepartedAt.Add(-2 * time.Hour)
	line.seed(`INSERT INTO customs_compliance.external_result
			(tenant_id, source_id, content_digest, unattributable, layer_conflict, result, submission_version, recorded_at)
		 VALUES ($1, 'SYN-SRC-FULL-DEP', 'syn-digest', false, false,
		         jsonb_build_object('layer', 'RELEASE', 'sourceId', 'SYN-SRC-FULL-DEP', 'scope', $2::text,
		                            'occurredAt', $3::timestamptz, 'receivedAt', $4::timestamptz),
		         'SYN-SUBV-01', $4)`,
		departureLineTenant, departureLineUnit, releasedAt, releasedAt.Add(time.Minute))
	line.seed(`INSERT INTO customs_compliance.release_outcome (tenant_id, source_id, kind, authority_ref, scope_ref, condition, received_at)
		 VALUES ($1, 'SYN-SRC-FULL-DEP', 'FULL', 'SYN-AUTHORITY-CN', $2, '', $3)`,
		departureLineTenant, departureLineUnit, releasedAt.Add(time.Minute))
	if got := line.depart("SYN-MOVE-ADMITTED", "SYN-SEG-ADMITTED").Answer(); got != tfdomain.DepartureGateAdmitted {
		t.Fatalf("门禁满足且已全部放行 = %s", got)
	}

	line.handOver("SYN-SEG-BLOCKED", "SYN-PARCEL-BLOCKED")
	line.seed(`INSERT INTO customs_compliance.regulatory_restriction
			(tenant_id, restriction_id, decision_id, scope_ref, constrains, effective_at)
		 VALUES ($1, 'SYN-RESTRICTION-DEP-LINE', 'SYN-DECISION-HOLD', 'SYN-PARCEL-BLOCKED', jsonb_build_array('LOADING_DEPARTURE'), $2)`,
		departureLineTenant, departureLineDepartedAt.Add(-3*time.Hour))
	blocked := line.depart("SYN-MOVE-BLOCKED", "SYN-SEG-BLOCKED")
	if blocked.Answer() != tfdomain.DepartureGateBlocked {
		t.Fatalf("包裹上有装载出发限制 = %s", blocked.Answer())
	}
	if bases := blocked.Findings()[0].Bases(); bases[0].Cause != "RESTRICTION_IN_FORCE" || bases[0].Sources[0] != "SYN-RESTRICTION-DEP-LINE" {
		t.Fatalf("阻断依据 = %+v", bases)
	}

	undecided := line.depart("SYN-MOVE-SEGMENT-UNKNOWN", "SYN-SEG-NOT-REGISTERED")
	if undecided.Answer() != tfdomain.DepartureGateUndecided || undecided.Stops()[0] != tfdomain.DepartureSegmentNotRegistered {
		t.Fatalf("段不在册 = %s %v", undecided.Answer(), undecided.Stops())
	}

	line.handOver("SYN-SEG-UNREADABLE", "SYN-PARCEL-UNREADABLE")
	line.seed(`ALTER TABLE customs_compliance.regulatory_restriction RENAME TO regulatory_restriction_unreachable`)
	unreadable := line.depart("SYN-MOVE-UNREADABLE", "SYN-SEG-UNREADABLE")
	if unreadable.Answer() != tfdomain.DepartureGateUnreadable {
		t.Fatalf("关务限制册读不到 = %s", unreadable.Answer())
	}
	if bases := unreadable.Findings()[0].Bases(); bases[0].Cause != "RESTRICTION_VIEW_UNAVAILABLE" {
		t.Fatalf("读不动要指名关务那一口：%+v", bases)
	}
}

// 出发登记在写事务之前问关务：判断在关务自己的事务里记下，出发那一笔另起。关务判断册里留下的正是逐对象那一格指回的判断。
func TestTheCustomsJudgementBehindADepartureIsKeptInItsOwnRegister(t *testing.T) {
	line := newDepartureLine(t)
	line.handOver("SYN-SEG-JUDGED", "SYN-PARCEL-JUDGED")
	reading := line.depart("SYN-MOVE-JUDGED", "SYN-SEG-JUDGED")
	judgement, judged := reading.Findings()[0].Judgement()
	if !judged {
		t.Fatal("逐对象那一格没有关务判断标识")
	}
	var answer string
	if err := line.pool.QueryRow(t.Context(),
		`SELECT answer FROM customs_compliance.action_admission_judgement WHERE tenant_id = $1 AND judgement_id = $2`,
		departureLineTenant, judgement).Scan(&answer); err != nil {
		t.Fatalf("关务判断册里查不到 %s：%v", judgement, err)
	}
	if answer != "NOT_GUARDED" {
		t.Fatalf("关务判断册里的答复 = %s", answer)
	}
}

// countingDepartureGate 数关务被问了几次，问法原样转给生产适配器。
type countingDepartureGate struct {
	inner tfports.DepartureGateView
	asks  int
}

func (gate *countingDepartureGate) ReadDepartureGate(
	ctx context.Context,
	query tfports.DepartureGateQuery,
) (tfdomain.DepartureGateFinding, error) {
	gate.asks++
	return gate.inner.ReadDepartureGate(ctx, query)
}

// Covers: AT-TF-116 真库半边——离线队列重投同一条出发：装配点在问关务之前就从池上认出是重放，关务不重问，答已有版本
// 并回显首次读数；换了资源的重投照常问，再答内容冲突。读数编排换成数关务问了几次的那一份，其余都是生产装配。
func TestAReplayedDepartureDoesNotAskCustomsAgain(t *testing.T) {
	line := newDepartureLine(t)
	line.handOver("SYN-SEG-REPLAY", "SYN-PARCEL-REPLAY")
	movement, ok := line.movement.(transactionalMovementFact)
	if !ok {
		t.Fatalf("生产装配交回的不是 transactionalMovementFact：%T", line.movement)
	}
	customsAdmission, err := buildCustomsAdmissionOrchestration(line.db)
	if err != nil {
		t.Fatalf("装配关务准入：%v", err)
	}
	view, err := tfcustoms.NewDepartureGateView(customsAdmission.actionAdmission)
	if err != nil {
		t.Fatalf("装关务适配器：%v", err)
	}
	segments, err := tfpostgres.NewFulfillmentSegments(line.db)
	if err != nil {
		t.Fatalf("段册：%v", err)
	}
	schedules, err := tfpostgres.NewTransportSchedules(line.db)
	if err != nil {
		t.Fatalf("班次册：%v", err)
	}
	counting := &countingDepartureGate{inner: view}
	if movement.gate, err = tfapp.NewDepartureGateReader(tfapp.DepartureGateReadDeps{
		Segments: segments, Schedules: schedules, Customs: counting,
	}); err != nil {
		t.Fatalf("装读数编排：%v", err)
	}

	command := line.departureCommand("SYN-MOVE-REPLAY", "SYN-SEG-REPLAY")
	first, err := movement.Record(t.Context(), command)
	if err != nil || first.Outcome() != tfapp.MovementFactRecorded || counting.asks != 1 {
		t.Fatalf("首登：outcome=%s err=%v 问关务 %d 次, want 落册、问一次", first.Outcome(), err, counting.asks)
	}

	replay, err := movement.Record(t.Context(), command)
	if err != nil || replay.Outcome() != tfapp.MovementFactVersionExists {
		t.Fatalf("重投：outcome=%s err=%v, want 已有版本", replay.Outcome(), err)
	}
	if counting.asks != 1 {
		t.Fatalf("同内容重投又问了关务：共 %d 次", counting.asks)
	}
	record, _ := replay.Record()
	if reading, read := record.Fact.GateReading(); !read || reading.Answer() != tfdomain.DepartureGateNotGuarded {
		t.Fatalf("重投回显的读数 = %s, want 首次的 NOT_GUARDED", reading.Answer())
	}

	changed := command
	changed.Resource = "SYN-TRUCK-9"
	conflict, err := movement.Record(t.Context(), changed)
	if err != nil || conflict.Outcome() != tfapp.MovementFactSourceConflict || counting.asks != 2 {
		t.Fatalf("换资源重投：outcome=%s err=%v 问关务 %d 次, want 照常问一次再答内容冲突", conflict.Outcome(), err, counting.asks)
	}
}
