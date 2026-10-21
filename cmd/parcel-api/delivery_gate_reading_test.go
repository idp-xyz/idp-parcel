package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
)

// 本文件在生产装配的交接与交付编排、作业事实 Intake、关务动作准入判断与真库上证交付门禁读数（ADR-0213 决定五；票
// demo-chain-gaps/66）。节点关务区域照演示种子登，输入都是合成串，证据只记 `S`。

type deliveryGateWire struct {
	Answer  string   `json:"answer"`
	At      string   `json:"at"`
	Stops   []string `json:"stops"`
	Objects []struct {
		Object string `json:"object"`
		Answer string `json:"answer"`
	} `json:"objects"`
}

func deliveryGateIn(t *testing.T, fields map[string]string) deliveryGateWire {
	t.Helper()
	var reading deliveryGateWire
	if err := json.Unmarshal([]byte(fields["gateReading"]), &reading); err != nil {
		t.Fatalf("答复里没有成形的 gateReading：%v（%v）", err, fields)
	}
	return reading
}

// deliveryGateRig 是同一库上的生产装配：交接（立段、带交出节点声明）与交付两条编排，作业事实口的两只端点。
type deliveryGateRig struct {
	db       *bentopg.DB
	pool     *pgxpool.Pool
	handover http.Handler
	delivery http.Handler
}

func newDeliveryGateRig(t *testing.T) (deliveryGateRig, func(string) int) {
	t.Helper()
	orchestrations, pool := wiredHandoverOrchestrations(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registerDemoNodeZones(t, db, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), map[string]string{
		"SYN-NODE-SHA-HUB": "SYN-ZONE-CN", "SYN-NODE-SZX-GATE": "SYN-ZONE-CN", "SYN-NODE-SIN-HUB": "SYN-ZONE-SG",
	})
	delivery, err := buildDeliveryOrchestration(db)
	if err != nil {
		t.Fatalf("装配交付编排：%v", err)
	}
	intake := workFactTransportForTest(t)
	judgements := func(object string) int {
		t.Helper()
		var count int
		if err := pool.QueryRow(t.Context(),
			`SELECT count(*) FROM customs_compliance.action_admission_judgement
			  WHERE tenant_id = $1 AND object_ref = $2 AND intent_kind = 'DELIVERY'`,
			handoverNodesTenant, object).Scan(&count); err != nil {
			t.Fatalf("数关务判断：%v", err)
		}
		return count
	}
	rig := deliveryGateRig{
		db:       db,
		pool:     pool,
		handover: tfhttp.NewRegisterTransportHandoverEndpoint(intake, orchestrations.handover),
		delivery: tfhttp.NewRegisterEffectiveDeliveryEndpoint(intake, delivery),
	}
	return rig, judgements
}

// enterLastMile 让一件对象凭`已交接`进尾程段：新加坡枢纽交给尾程，交接业务时间早于派送到场。releasing 为空即不声明
// 交出节点。
func (rig deliveryGateRig) enterLastMile(t *testing.T, object, releasing string) {
	t.Helper()
	fields := map[string]string{
		"scope":         "SYN-HANDOVER-SCOPE-DCG66",
		"releasedBy":    "SYN-PARTY-HUB-SIN",
		"receivedBy":    "SYN-PARTY-COURIER-SG-LM",
		"judgedAt":      "2026-08-20T08:00:00+08:00",
		"segment":       "SYN-SEG-DCG66-" + object,
		"releasingNode": releasing,
	}
	status, answer := postWorkFact(t, rig.handover, "/transport-fulfillment/handovers", handoverNodesBody(t, object, fields))
	if status != http.StatusCreated || answer["outcome"] != "HANDOVER_REGISTERED" {
		t.Fatalf("尾程交接 HTTP %d %v", status, answer)
	}
}

func (rig deliveryGateRig) deliver(t *testing.T, object string) (int, map[string]string) {
	t.Helper()
	seedDeliveryAttemptRow(t, rig.pool, handoverNodesTenant, "SYN-ATTEMPT-"+object, []string{object})
	seedDeliveryResultRow(t, rig.pool, handoverNodesTenant, "SYN-ATTEMPT-"+object, object, "DELIVERED", nil)
	return rig.redeliver(t, object)
}

func (rig deliveryGateRig) redeliver(t *testing.T, object string) (int, map[string]string) {
	t.Helper()
	body := `{"attempt":"SYN-ATTEMPT-` + object + `","object":"` + object +
		`","method":"SYN-METHOD-SIGNATURE","recipient":"SYN-RECIPIENT-1","proof":"SYN-POD-` + object + `"}`
	return postWorkFact(t, rig.delivery, "/transport-fulfillment/deliveries", body)
}

// Covers: `AT-TF-256`、`AT-TF-263`、`AT-TF-265` 生产装配真库半边，票 demo-chain-gaps/66 完成判据的演示一条——
// 新加坡枢纽交给尾程（releasingNode=SYN-NODE-SIN-HUB）立起尾程段，之后有效交付答 201 `DELIVERY_REGISTERED`，gateReading
// 为 `NOT_GUARDED`，at 是新加坡枢纽：包裹不在任何关务案卷，关务只适用最终交付那一道、答不受门禁；判断落进关务的判断册，
// 依据原文随交付落册、不回给登记方，得出放行的那件对象也不列在逐对象那一格里（`AT-TF-321` 真库半边，ADR-0237）。参与随交付结束，交付意图照常入队交给 PS 终局。原样重发答 200
// `EXISTING_VERSION`、回显首次读数，关务判断册不多一行——此刻参与已结束，重读只会读成「不在任何段里」。查阅册列出整格。
func TestTheWiredDeliveryRecordsAFinalDeliveryGateReadingAgainstARealDatabase(t *testing.T) {
	rig, judgements := newDeliveryGateRig(t)
	const object = "SYN-PARCEL-DCG66-01"
	rig.enterLastMile(t, object, "SYN-NODE-SIN-HUB")

	status, answer := rig.deliver(t, object)
	reading := deliveryGateIn(t, answer)
	if status != http.StatusCreated || answer["outcome"] != "DELIVERY_REGISTERED" || reading.Answer != "NOT_GUARDED" ||
		reading.At != "SYN-NODE-SIN-HUB" || len(reading.Stops) != 0 || len(reading.Objects) != 0 {
		t.Fatalf("有效交付 HTTP %d %v", status, answer)
	}
	if answer["participationEnd"] != "PARTICIPATION_ENDED" {
		t.Fatalf("participationEnd = %q，want 交付结束尾程段的参与", answer["participationEnd"])
	}
	var intents int
	if err := rig.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE event_type = 'transport-fulfillment.effective-delivery.registered'`).
		Scan(&intents); err != nil || intents != 1 {
		t.Fatalf("交给 PS 终局的有效交付意图 %d 封（%v），want 1——读数不改交付信封", intents, err)
	}
	if strings.Contains(answer["gateReading"], "AADJ-") || strings.Contains(answer["gateReading"], "FINAL_DELIVERY") {
		t.Fatalf("答复带出了关务判断标识或依据原文：%s", answer["gateReading"])
	}
	if got := judgements(object); got != 1 {
		t.Fatalf("关务判断册里这一件的交付判断 %d 行，want 1", got)
	}

	var stored string
	var raw []byte
	if err := rig.pool.QueryRow(t.Context(),
		`SELECT gate_reading, gate_reading_detail FROM transport_fulfillment.effective_delivery
		  WHERE tenant_id = $1 AND object_ref = $2 AND is_current`,
		handoverNodesTenant, object).Scan(&stored, &raw); err != nil {
		t.Fatalf("读回读数：%v", err)
	}
	var detail struct {
		Node    string `json:"node"`
		Finding struct {
			Judgement string `json:"judgement"`
			Bases     []struct {
				Action string `json:"action"`
				Cause  string `json:"cause"`
			} `json:"bases"`
		} `json:"finding"`
	}
	if err := json.Unmarshal(raw, &detail); err != nil {
		t.Fatalf("明细：%v", err)
	}
	if stored != "NOT_GUARDED" || detail.Node != "SYN-NODE-SIN-HUB" || len(detail.Finding.Bases) != 1 ||
		detail.Finding.Bases[0].Action != "FINAL_DELIVERY" || detail.Finding.Bases[0].Cause != "OUTSIDE_CUSTOMS_BOOKS" ||
		!strings.HasPrefix(detail.Finding.Judgement, "AADJ-") {
		t.Fatalf("落册的读数 %s %s", stored, raw)
	}

	status, answer = rig.redeliver(t, object)
	if replayed := deliveryGateIn(t, answer); status != http.StatusOK || answer["outcome"] != "EXISTING_VERSION" ||
		replayed.Answer != "NOT_GUARDED" || replayed.At != "SYN-NODE-SIN-HUB" {
		t.Fatalf("原样重发 HTTP %d %v", status, answer)
	}
	if got := judgements(object); got != 1 {
		t.Fatalf("原样重发重问了关务：判断 %d 行", got)
	}

	records, err := tfpostgres.NewReviewCatalogue(rig.db)
	if err != nil {
		t.Fatalf("查阅目录：%v", err)
	}
	list := httptest.NewRecorder()
	tfhttp.NewQueryTransportFulfillmentRecordsEndpoint(handoverNodesCatalogueIntake{}, records).
		ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/transport-fulfillment-records?registry=effective-delivery", nil))
	var listed struct {
		Deliveries []map[string]string `json:"deliveries"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listed); err != nil || list.Code != http.StatusOK {
		t.Fatalf("交付册 HTTP %d %s：%v", list.Code, list.Body.String(), err)
	}
	if len(listed.Deliveries) != 1 || listed.Deliveries[0]["gateReading"] != "NOT_GUARDED" {
		t.Fatalf("交付册 = %v，want 一行、读数列 NOT_GUARDED", listed.Deliveries)
	}
}

// Covers: `AT-TF-257`、`AT-TF-261`、`AT-TF-262` 生产装配真库半边——阻断照样 201：包裹上落了最终交付限制，读数
// 阻断、那件对象列在逐对象那一格里（`AT-TF-321` 真库半边），交付照登、参与照常结束。进段那次交接没声明交出节点：未决「交出节点未声明」，不问关务。对象从没进过段：未决「此刻
// 不在任何段里」，不问关务，交付照登。
func TestTheWiredDeliveryRegistersWhateverTheGateReads(t *testing.T) {
	rig, judgements := newDeliveryGateRig(t)

	const blocked = "SYN-PARCEL-DCG66-02"
	rig.enterLastMile(t, blocked, "SYN-NODE-SIN-HUB")
	if _, err := rig.pool.Exec(t.Context(),
		`INSERT INTO customs_compliance.regulatory_restriction
			(tenant_id, restriction_id, decision_id, scope_ref, constrains, effective_at)
		 VALUES ($1, 'SYN-RESTRICTION-DCG66-01', 'SYN-DECISION-HOLD', $2, jsonb_build_array('FINAL_DELIVERY'), $3)`,
		handoverNodesTenant, blocked, deliveryAttemptArrivedAt.Add(-3*time.Hour)); err != nil {
		t.Fatalf("登限制：%v", err)
	}
	status, answer := rig.deliver(t, blocked)
	if reading := deliveryGateIn(t, answer); status != http.StatusCreated || answer["outcome"] != "DELIVERY_REGISTERED" ||
		reading.Answer != "BLOCKED" || answer["participationEnd"] != "PARTICIPATION_ENDED" ||
		len(reading.Objects) != 1 || reading.Objects[0].Object != blocked || reading.Objects[0].Answer != "BLOCKED" {
		t.Fatalf("阻断 HTTP %d %v", status, answer)
	}

	const undeclared = "SYN-PARCEL-DCG66-03"
	rig.enterLastMile(t, undeclared, "")
	status, answer = rig.deliver(t, undeclared)
	if reading := deliveryGateIn(t, answer); status != http.StatusCreated || reading.Answer != "UNDECIDED" ||
		len(reading.Stops) != 1 || reading.Stops[0] != "RELEASING_NODE_NOT_DECLARED" || reading.At != "" {
		t.Fatalf("没声明交出节点 HTTP %d %v", status, answer)
	}
	if got := judgements(undeclared); got != 0 {
		t.Fatalf("没声明交出节点却问了关务：%d 行", got)
	}

	const outside = "SYN-PARCEL-DCG66-04"
	status, answer = rig.deliver(t, outside)
	if reading := deliveryGateIn(t, answer); status != http.StatusCreated || reading.Answer != "UNDECIDED" ||
		len(reading.Stops) != 1 || reading.Stops[0] != "NO_ACTIVE_SEGMENT" || answer["participationEnd"] != "NO_ACTIVE_PARTICIPATION" {
		t.Fatalf("不在任何段里 HTTP %d %v", status, answer)
	}
	if got := judgements(outside); got != 0 {
		t.Fatalf("不在任何段里却问了关务：%d 行", got)
	}
}
