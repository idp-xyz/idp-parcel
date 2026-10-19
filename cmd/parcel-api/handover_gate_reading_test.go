package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	customsapp "go.idp.xyz/idp-parcel/internal/customscompliance/application"
	customsdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
)

// 本文件在生产装配的交接编排、作业事实 Intake、关务动作准入判断与真库上证交出门禁读数（ADR-0213 决定四；票
// demo-chain-gaps/65）。节点关务区域照演示种子登（深圳口岸、上海枢纽在 CN，新加坡枢纽在 SG），输入都是合成串，证据只记 `S`。

type gateReadingReply struct {
	Answer            string `json:"answer"`
	At                string `json:"at"`
	Onward            string `json:"onward"`
	OnwardNotDeclared bool   `json:"onwardNotDeclared"`
	Objects           []struct {
		Object string `json:"object"`
		Answer string `json:"answer"`
	} `json:"objects"`
}

func gateReadingIn(t *testing.T, fields map[string]string) gateReadingReply {
	t.Helper()
	var reading gateReadingReply
	if err := json.Unmarshal([]byte(fields["gateReading"]), &reading); err != nil {
		t.Fatalf("答复里没有成形的 gateReading：%v（%v）", err, fields)
	}
	return reading
}

// registerDemoNodeZones 照演示种子给三个节点登关务区域；appliesFrom 让用例能在动作时点之前再登一版。
func registerDemoNodeZones(t *testing.T, db *bentopg.DB, appliesFrom time.Time, zones map[string]string) {
	t.Helper()
	orchestration, err := buildCustomsAdmissionOrchestration(db)
	if err != nil {
		t.Fatalf("装配关务：%v", err)
	}
	tenant, err := customsdomain.NewTenantID(handoverNodesTenant)
	if err != nil {
		t.Fatal(err)
	}
	for node, zone := range zones {
		nodeRef, err := customsdomain.NewNetworkNodeReference(node)
		if err != nil {
			t.Fatal(err)
		}
		zoneRef, err := customsdomain.NewCustomsZoneReference(zone)
		if err != nil {
			t.Fatal(err)
		}
		outcome, err := orchestration.nodeZoneRegistration.Handle(t.Context(), customsapp.RegisterNodeCustomsZoneCommand{
			TenantID: tenant, Node: nodeRef, Zone: zoneRef, AppliesFrom: appliesFrom,
		})
		if err != nil || outcome != customsapp.ConfigurationRegistered {
			t.Fatalf("登 %s 区域：%s / %v", node, outcome, err)
		}
	}
}

// Covers: AT-TF-236、AT-TF-241、AT-TF-242 真库半边，票 demo-chain-gaps/65 完成判据的演示一条——深圳口岸交给干线承运商
// （releasingNode=SYN-NODE-SZX-GATE、onwardNode=SYN-NODE-SIN-HUB）：答 201 `HANDOVER_REGISTERED`，gateReading 未决，依据
// 是跨关务区域移动无案（对象不在任何案卷）；关务判断落进关务的判断册，依据原文随交接落册、不回给登记方。之后把新加坡枢纽
// 改登进 CN、让关务对同一问改答不受门禁：原样重发答 200 `EXISTING_VERSION`、回显首次读数，关务判断册不多一行——没有重问。
// 待确认更正成`已交接`经同一装配照承继的意图问关务，更正版带自己的读数；查阅册列出整格。
func TestTheWiredHandoverRecordsTheOutboundReleaseGateReadingAgainstARealDatabase(t *testing.T) {
	orchestrations, pool := wiredHandoverOrchestrations(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registerDemoNodeZones(t, db, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), map[string]string{
		"SYN-NODE-SHA-HUB": "SYN-ZONE-CN", "SYN-NODE-SZX-GATE": "SYN-ZONE-CN", "SYN-NODE-SIN-HUB": "SYN-ZONE-SG",
	})
	intake := workFactTransportForTest(t)
	register := tfhttp.NewRegisterTransportHandoverEndpoint(intake, orchestrations.handover)
	correct := tfhttp.NewCorrectTransportHandoverEndpoint(intake, orchestrations.handover)
	toTrunk := map[string]string{
		"releasedBy":    "SYN-PARTY-GATE-SZX",
		"receivedBy":    "SYN-PARTY-CARRIER-TRUNK-01",
		"judgedAt":      "2026-10-18T20:00:00+08:00",
		"releasingNode": "SYN-NODE-SZX-GATE",
		"onwardNode":    "SYN-NODE-SIN-HUB",
	}
	body := handoverNodesBody(t, "SYN-PARCEL-DCG65-01", toTrunk)

	status, answer := postWorkFact(t, register, "/transport-fulfillment/handovers", body)
	reading := gateReadingIn(t, answer)
	if status != http.StatusCreated || answer["outcome"] != "HANDOVER_REGISTERED" || reading.Answer != "UNDECIDED" ||
		reading.At != "SYN-NODE-SZX-GATE" || reading.Onward != "SYN-NODE-SIN-HUB" || reading.OnwardNotDeclared ||
		len(reading.Objects) != 1 || reading.Objects[0].Object != "SYN-PARCEL-DCG65-01" || reading.Objects[0].Answer != "UNDECIDED" {
		t.Fatalf("交给干线承运商 HTTP %d %v", status, answer)
	}

	judgements := func() int {
		var count int
		if err := pool.QueryRow(t.Context(),
			`SELECT count(*) FROM customs_compliance.action_admission_judgement
			  WHERE tenant_id = $1 AND object_ref = 'SYN-PARCEL-DCG65-01' AND intent_kind = 'HANDOVER'`,
			handoverNodesTenant).Scan(&count); err != nil {
			t.Fatalf("数关务判断：%v", err)
		}
		return count
	}
	if got := judgements(); got != 1 {
		t.Fatalf("关务判断册里这一件的交出判断 %d 行，want 1", got)
	}
	var stored string
	var detail struct {
		Finding struct {
			Judgement string `json:"judgement"`
			Bases     []struct {
				Action string `json:"action"`
				Cause  string `json:"cause"`
			} `json:"bases"`
		} `json:"finding"`
	}
	var raw []byte
	if err := pool.QueryRow(t.Context(),
		`SELECT gate_reading, gate_reading_detail FROM transport_fulfillment.transport_handover
		  WHERE tenant_id = $1 AND object_ref = 'SYN-PARCEL-DCG65-01' AND handover_version = 'v1'`,
		handoverNodesTenant).Scan(&stored, &raw); err != nil {
		t.Fatalf("读回读数：%v", err)
	}
	if err := json.Unmarshal(raw, &detail); err != nil {
		t.Fatalf("明细：%v", err)
	}
	if stored != "UNDECIDED" || len(detail.Finding.Bases) != 1 || detail.Finding.Bases[0].Action != "CROSS_CUSTOMS_MOVEMENT" ||
		detail.Finding.Bases[0].Cause != "CUSTOMS_SCOPE_UNRESOLVED" || !strings.HasPrefix(detail.Finding.Judgement, "AADJ-") {
		t.Fatalf("落册的读数 %s %s", stored, raw)
	}
	if strings.Contains(answer["gateReading"], "AADJ-") || strings.Contains(answer["gateReading"], "CROSS_CUSTOMS_MOVEMENT") {
		t.Fatalf("答复带出了关务判断标识或依据原文：%s", answer["gateReading"])
	}

	registerDemoNodeZones(t, db, time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), map[string]string{"SYN-NODE-SIN-HUB": "SYN-ZONE-CN"})
	status, answer = postWorkFact(t, register, "/transport-fulfillment/handovers", body)
	if replayed := gateReadingIn(t, answer); status != http.StatusOK || answer["outcome"] != "EXISTING_VERSION" || replayed.Answer != "UNDECIDED" {
		t.Fatalf("原样重发 HTTP %d %v", status, answer)
	}
	if got := judgements(); got != 1 {
		t.Fatalf("原样重发重问了关务：判断 %d 行", got)
	}
	status, answer = postWorkFact(t, register, "/transport-fulfillment/handovers", handoverNodesBody(t, "SYN-PARCEL-DCG65-02", toTrunk))
	if fresh := gateReadingIn(t, answer); status != http.StatusCreated || fresh.Answer != "NOT_GUARDED" {
		t.Fatalf("改登区域之后另一件 HTTP %d %v，want 关务改答不受门禁（证明上面的重发确实没重问）", status, answer)
	}

	pending := map[string]string{"verdict": "PENDING_CONFIRMATION", "rule": "", "basis": "SYN-BASIS-DCG65-COUNT"}
	for key, value := range toTrunk {
		pending[key] = value
	}
	status, answer = postWorkFact(t, register, "/transport-fulfillment/handovers", handoverNodesBody(t, "SYN-PARCEL-DCG65-03", pending))
	if _, present := answer["gateReading"]; status != http.StatusCreated || present {
		t.Fatalf("待确认 HTTP %d %v，want 201 且不带读数", status, answer)
	}
	confirm := `{"object":"SYN-PARCEL-DCG65-03","scope":"SYN-HANDOVER-SCOPE-DCG59","predecessorVersion":"v1",` +
		`"verdict":"HANDED_OVER","releasingEvidence":"SYN-EVIDENCE-RELEASE-DCG65","receivingEvidence":"SYN-EVIDENCE-RECEIVE-DCG65",` +
		`"rule":"SYN-HANDOVER-RULE/v1","newVersion":"v2","correctedAt":"2026-10-18T22:00:00+08:00"}`
	status, answer = postWorkFact(t, correct, "/transport-fulfillment/handover-corrections", confirm)
	if confirmed := gateReadingIn(t, answer); status != http.StatusCreated || answer["outcome"] != "HANDOVER_CORRECTED" ||
		confirmed.Answer != "NOT_GUARDED" || confirmed.At != "SYN-NODE-SZX-GATE" || confirmed.Onward != "SYN-NODE-SIN-HUB" {
		t.Fatalf("待确认更正成已交接 HTTP %d %v", status, answer)
	}

	records, err := tfpostgres.NewReviewCatalogue(db)
	if err != nil {
		t.Fatalf("查阅目录：%v", err)
	}
	list := httptest.NewRecorder()
	tfhttp.NewQueryTransportFulfillmentRecordsEndpoint(handoverNodesCatalogueIntake{}, records).
		ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/transport-fulfillment-records?registry=transport-handover", nil))
	var listed struct {
		Handovers []map[string]string `json:"handovers"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listed); err != nil || list.Code != http.StatusOK {
		t.Fatalf("交接册 HTTP %d %s：%v", list.Code, list.Body.String(), err)
	}
	want := map[string]string{
		"SYN-PARCEL-DCG65-01/v1": "UNDECIDED", "SYN-PARCEL-DCG65-02/v1": "NOT_GUARDED",
		"SYN-PARCEL-DCG65-03/v1": "", "SYN-PARCEL-DCG65-03/v2": "NOT_GUARDED",
	}
	for _, row := range listed.Handovers {
		if expected, mine := want[row["object"]+"/"+row["version"]]; mine && row["gateReading"] != expected {
			t.Fatalf("册面 %s/%s 读数列 = %q，want %q", row["object"], row["version"], row["gateReading"], expected)
		}
	}
}
