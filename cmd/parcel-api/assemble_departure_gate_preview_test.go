package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
)

// 设备出发前门禁问询的真库用例（ADR-0219；票 demo-chain-gaps/69）。都站在演示动线那一段上：按种子登关务节点区域与上海→深圳的
// 周期安排、显式登班次、交接立段（newDepartureLine），问询与出发都经生产装配与作业事实渠道。证据只记 `S`。

// previewLine 在 departureLine 上多装一只生产问询编排，两口都挂作业事实渠道。
type previewLine struct {
	*departureLine
	preview  http.Handler
	movement http.Handler
}

func newPreviewLine(t *testing.T) *previewLine {
	t.Helper()
	line := newDepartureLine(t)
	preview, err := buildDepartureGatePreview(line.db)
	if err != nil {
		t.Fatalf("装配出发前问询：%v", err)
	}
	channel := workFactTransportForTest(t)
	return &previewLine{
		departureLine: line,
		preview:       tfhttp.NewPreviewDepartureGateEndpoint(channel, preview),
		movement:      tfhttp.NewRecordMovementFactEndpoint(channel, line.movement),
	}
}

// post 以演示场外设备的出示（OFFSITE 作业范围 SYN-OFFSITE-01）打一口；scopeRef 换成别的就是在那个作业范围上没有授予。
func (line *previewLine) post(endpoint http.Handler, path, scopeRef, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	stampWorkFact(request, "OFFSITE", scopeRef, "SYN-DEVICE-OFFSITE")
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, request)
	return response
}

func (line *previewLine) ask(segment string) *httptest.ResponseRecorder {
	return line.post(line.preview, "/transport-fulfillment/departure-gate-previews", "SYN-OFFSITE-01",
		`{"schedule":"`+departureLineSchedule+`","segment":"`+segment+`","departsAt":"2026-10-21T22:05:00+08:00"}`)
}

func (line *previewLine) departAs(fact, segment string) *httptest.ResponseRecorder {
	return line.post(line.movement, "/transport-fulfillment/movement-facts", "SYN-OFFSITE-01",
		`{"fact":"`+fact+`","schedule":"`+departureLineSchedule+`","kind":"DEPARTURE","location":"SYN-NODE-SHA-HUB",`+
			`"version":"v1","occurredAt":"2026-10-21T22:05:00+08:00","segment":"`+segment+`"}`)
}

// snapshot 取整库指纹（每张基表整表摘要与全部序列，借 participation_termination_echo_test.go 那一份）：零写入要按整库证，
// 只数几张表证不了「哪儿都没写」。
func (line *previewLine) snapshot() map[string]string {
	line.t.Helper()
	return terminationChannel{pool: line.pool}.fingerprint(line.t)
}

// Covers: AT-TF-226（演示判据，证据 S）与 AT-TF-229——演示动线上自营 SHA→SZX：设备出发前问询答 200 `DEPARTURE_GATE_PREVIEWED`，
// 读数 `NOT_GUARDED`；问询之后整库只有关务的动作准入判断册多了一行（本上下文零写入，信封表不动）。随即同参出发答 201，
// `gateReading` 与问询逐字节相同；关务对同参出发命中问询那一次记下的判断（ADR-0206 决定六「已有判断」），判断册不再多行，
// 出发落册的逐对象依据指回的正是那一行。
func TestADemoDeviceAsksBeforeDepartingAndTheDepartureReadsTheSame(t *testing.T) {
	line := newPreviewLine(t)
	line.handOver("SYN-SEG-SHA-SZX-69", "SYN-PARCEL-69-01")

	before := line.snapshot()
	asked := line.ask("SYN-SEG-SHA-SZX-69")
	if asked.Code != http.StatusOK {
		t.Fatalf("问询 HTTP %d %s, want 200", asked.Code, asked.Body.String())
	}
	var answer struct {
		Outcome     string `json:"outcome"`
		GateReading struct {
			Answer  string           `json:"answer"`
			From    string           `json:"from"`
			To      string           `json:"to"`
			Objects []map[string]any `json:"objects"`
		} `json:"gateReading"`
	}
	if err := json.Unmarshal(asked.Body.Bytes(), &answer); err != nil {
		t.Fatalf("decode %s：%v", asked.Body.String(), err)
	}
	reading := answer.GateReading
	if answer.Outcome != "DEPARTURE_GATE_PREVIEWED" || reading.Answer != "NOT_GUARDED" ||
		reading.From != "SYN-NODE-SHA-HUB" || reading.To != "SYN-NODE-SZX-GATE" ||
		len(reading.Objects) != 1 || reading.Objects[0]["object"] != "SYN-PARCEL-69-01" || len(reading.Objects[0]) != 2 {
		t.Fatalf("问询答复 = %s", asked.Body.String())
	}
	afterPreview := line.snapshot()
	if changed := changedSince(before, afterPreview); !slices.Equal(changed, []string{"customs_compliance.action_admission_judgement"}) {
		t.Fatalf("问询之后变了的表 = %v, want 只有关务判断册——本上下文零写入", changed)
	}

	departed := line.departAs("SYN-MOVE-69-01", "SYN-SEG-SHA-SZX-69")
	if departed.Code != http.StatusCreated {
		t.Fatalf("出发 HTTP %d %s, want 201", departed.Code, departed.Body.String())
	}
	if previewed, recorded := gateReadingBytes(t, asked), gateReadingBytes(t, departed); previewed != recorded {
		t.Fatalf("问询与同参出发的 gateReading 不是同一份：\n问询 %s\n出发 %s", previewed, recorded)
	}
	if changed := changedSince(afterPreview, line.snapshot()); slices.Contains(changed, "customs_compliance.action_admission_judgement") {
		t.Fatalf("同参出发让关务判断册又变了：%v——同一请求在同样三源上该命中问询那一次的判断", changed)
	}
	var judgements int
	if err := line.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM customs_compliance.action_admission_judgement j
		   JOIN transport_fulfillment.transport_movement_fact f
		     ON f.gate_reading_detail #>> '{findings,0,judgement}' = j.judgement_id
		  WHERE f.fact_ref = 'SYN-MOVE-69-01' AND j.tenant_id = $1`, departureLineTenant).Scan(&judgements); err != nil {
		t.Fatalf("查判断：%v", err)
	}
	if judgements != 1 {
		t.Fatalf("出发落册的逐对象依据指回 %d 条关务判断, want 问询那一条", judgements)
	}
}

// Covers: AT-TF-227 真库半边与 AT-TF-231——包裹上有约束装载出发的限制：问询照样 200、读数阻断，挡不挡归设备与现场；同参出发
// 照样 201、读数逐字节相同。另一段问询答不受门禁之后、出发之前落了一条限制：出发按它自己那一刻重问，读数阻断，不沿用问询
// 那一份——读到放行不等于出发那一刻仍放行。
func TestAPreviewIsAReadingNotAPermit(t *testing.T) {
	line := newPreviewLine(t)
	restrict := func(object string) {
		t.Helper()
		line.seed(`INSERT INTO customs_compliance.regulatory_restriction
				(tenant_id, restriction_id, decision_id, scope_ref, constrains, effective_at)
			 VALUES ($1, $2, 'SYN-DECISION-HOLD-69', $3, jsonb_build_array('LOADING_DEPARTURE'), $4)`,
			departureLineTenant, "SYN-RESTRICTION-"+object, object, departureLineDepartedAt.Add(-3*time.Hour))
	}

	line.handOver("SYN-SEG-69-BLOCKED", "SYN-PARCEL-69-BLOCKED")
	restrict("SYN-PARCEL-69-BLOCKED")
	asked := line.ask("SYN-SEG-69-BLOCKED")
	if asked.Code != http.StatusOK || !strings.Contains(asked.Body.String(), `"answer":"BLOCKED"`) {
		t.Fatalf("问询 HTTP %d %s, want 200 读数阻断", asked.Code, asked.Body.String())
	}
	departed := line.departAs("SYN-MOVE-69-BLOCKED", "SYN-SEG-69-BLOCKED")
	if departed.Code != http.StatusCreated {
		t.Fatalf("出发 HTTP %d %s, want 201", departed.Code, departed.Body.String())
	}
	if previewed, recorded := gateReadingBytes(t, asked), gateReadingBytes(t, departed); previewed != recorded {
		t.Fatalf("阻断时两口读数不同：\n问询 %s\n出发 %s", previewed, recorded)
	}

	line.handOver("SYN-SEG-69-LATE", "SYN-PARCEL-69-LATE")
	if asked := line.ask("SYN-SEG-69-LATE"); !strings.Contains(asked.Body.String(), `"answer":"NOT_GUARDED"`) {
		t.Fatalf("落限制之前问询 = %s, want 不受门禁", asked.Body.String())
	}
	restrict("SYN-PARCEL-69-LATE")
	late := line.departAs("SYN-MOVE-69-LATE", "SYN-SEG-69-LATE")
	if late.Code != http.StatusCreated || !strings.Contains(gateReadingBytes(t, late), `"answer":"BLOCKED"`) {
		t.Fatalf("问询之后落了限制，出发 = %d %s, want 201 读数阻断", late.Code, late.Body.String())
	}
}

// Covers: AT-TF-230 真库半边——在这个作业范围上没有作业事实登记授予，答 403 `OPERATOR_NOT_GRANTED`，整库一张表都不变：连关务
// 也没被问到。
func TestAPreviewWithoutTheGrantIsRefusedAndAsksNoOne(t *testing.T) {
	line := newPreviewLine(t)
	line.handOver("SYN-SEG-69-UNGRANTED", "SYN-PARCEL-69-UNGRANTED")

	before := line.snapshot()
	refused := line.post(line.preview, "/transport-fulfillment/departure-gate-previews", "SYN-OFFSITE-99",
		`{"schedule":"`+departureLineSchedule+`","segment":"SYN-SEG-69-UNGRANTED","departsAt":"2026-10-21T22:05:00+08:00"}`)
	if refused.Code != http.StatusForbidden || problemCode(t, refused) != "OPERATOR_NOT_GRANTED" {
		t.Fatalf("没有授予 = %d %s, want 403 OPERATOR_NOT_GRANTED", refused.Code, refused.Body.String())
	}
	if changed := changedSince(before, line.snapshot()); len(changed) != 0 {
		t.Fatalf("拒掉的问询改了 %v", changed)
	}
}

// gateReadingBytes 取答复里 `gateReading` 那一段原样字节。
func gateReadingBytes(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var body map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s：%v", response.Body.String(), err)
	}
	return string(body["gateReading"])
}
