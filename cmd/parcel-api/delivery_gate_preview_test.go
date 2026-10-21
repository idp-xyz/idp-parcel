package main

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
)

// 派送出发前交付门禁问询的真库用例（ADR-0238；票 demo-chain-gaps/94）。站在票 66 那一段尾程上：新加坡枢纽交给尾程（带交出节点
// 声明）立起尾程段，节点关务区域照演示种子登；问询与有效交付都经生产装配与作业事实渠道。证据只记 `S`。

// deliveryPreviewRig 在 deliveryGateRig 上多装一只生产问询编排，同挂作业事实渠道。
type deliveryPreviewRig struct {
	deliveryGateRig
	preview http.Handler
}

func newDeliveryPreviewRig(t *testing.T) (deliveryPreviewRig, func(string) int) {
	t.Helper()
	rig, judgements := newDeliveryGateRig(t)
	preview, err := buildDeliveryGatePreview(rig.db)
	if err != nil {
		t.Fatalf("装配交付前问询：%v", err)
	}
	return deliveryPreviewRig{
		deliveryGateRig: rig,
		preview:         tfhttp.NewPreviewDeliveryGateEndpoint(workFactTransportForTest(t), preview),
	}, judgements
}

// deliveryPreviewDeliversAt 是 seedDeliveryResultRow 种下的那件对象结果的发生时刻：同参的问询按它问。
var deliveryPreviewDeliversAt = deliveryAttemptArrivedAt.Add(15 * time.Minute)

// ask 以演示场外设备的出示问一件；scopeRef 换成别的就是在那个作业范围上没有授予。
func (rig deliveryPreviewRig) ask(scopeRef, object string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/transport-fulfillment/delivery-gate-previews", strings.NewReader(
		`{"object":"`+object+`","deliversAt":"`+deliveryPreviewDeliversAt.Format(time.RFC3339)+`"}`))
	stampWorkFact(request, "OFFSITE", scopeRef, "SYN-DEVICE-OFFSITE")
	response := httptest.NewRecorder()
	rig.preview.ServeHTTP(response, request)
	return response
}

func (rig deliveryPreviewRig) snapshot(t *testing.T) map[string]string {
	t.Helper()
	return terminationChannel{pool: rig.pool}.fingerprint(t)
}

// Covers: AT-TF-322、AT-TF-323、AT-TF-324 真库（证据 S）——尾程两件包裹：一件落了约束最终交付的限制，一件没有。派送出发之前逐件问：
// 受限那一件答 200 `DELIVERY_GATE_PREVIEWED`、读数阻断，那一件列在逐对象那一格里；另一件答不受门禁、没有逐对象那一格（ADR-0237）。
// 两问之后整库只有关务的动作准入判断册变了：没有落交付、没有结束参与、没有入队任何信封。随后两件按问询的时点登有效交付，答复里的
// `gateReading` 与各自的问询逐字节相同；关务对同参交付命中问询那一次记下的判断，判断册不多一行。
func TestACourierAsksBeforeDispatchingAndTheDeliveryReadsTheSame(t *testing.T) {
	rig, judgements := newDeliveryPreviewRig(t)
	const (
		held  = "SYN-PARCEL-DCG94-HELD"
		clean = "SYN-PARCEL-DCG94-CLEAN"
	)
	rig.enterLastMile(t, held, "SYN-NODE-SIN-HUB")
	rig.enterLastMile(t, clean, "SYN-NODE-SIN-HUB")
	if _, err := rig.pool.Exec(t.Context(),
		`INSERT INTO customs_compliance.regulatory_restriction
			(tenant_id, restriction_id, decision_id, scope_ref, constrains, effective_at)
		 VALUES ($1, 'SYN-RESTRICTION-DCG94-01', 'SYN-DECISION-HOLD', $2, jsonb_build_array('FINAL_DELIVERY'), $3)`,
		handoverNodesTenant, held, deliveryAttemptArrivedAt.Add(-3*time.Hour)); err != nil {
		t.Fatalf("登限制：%v", err)
	}

	before := rig.snapshot(t)
	askedHeld, askedClean := rig.ask("SYN-OFFSITE-01", held), rig.ask("SYN-OFFSITE-01", clean)
	for _, asked := range []*httptest.ResponseRecorder{askedHeld, askedClean} {
		if asked.Code != http.StatusOK || !strings.Contains(asked.Body.String(), `"outcome":"DELIVERY_GATE_PREVIEWED"`) {
			t.Fatalf("问询 HTTP %d %s, want 200 DELIVERY_GATE_PREVIEWED", asked.Code, asked.Body.String())
		}
	}
	if reading := deliveryGateIn(t, map[string]string{"gateReading": gateReadingBytes(t, askedHeld)}); reading.Answer != "BLOCKED" ||
		reading.At != "SYN-NODE-SIN-HUB" || len(reading.Objects) != 1 || reading.Objects[0].Object != held || reading.Objects[0].Answer != "BLOCKED" {
		t.Fatalf("受限那一件的问询 = %s", askedHeld.Body.String())
	}
	if reading := deliveryGateIn(t, map[string]string{"gateReading": gateReadingBytes(t, askedClean)}); reading.Answer != "NOT_GUARDED" ||
		reading.At != "SYN-NODE-SIN-HUB" || strings.Contains(askedClean.Body.String(), `"objects"`) {
		t.Fatalf("放行那一件的问询 = %s", askedClean.Body.String())
	}
	for _, leaked := range []string{"AADJ-", "FINAL_DELIVERY", "SYN-RESTRICTION"} {
		if strings.Contains(askedHeld.Body.String(), leaked) {
			t.Fatalf("问询答复带出了关务判断标识或依据原文 %s：%s", leaked, askedHeld.Body.String())
		}
	}
	afterPreview := rig.snapshot(t)
	if changed := changedSince(before, afterPreview); !slices.Equal(changed, []string{"customs_compliance.action_admission_judgement"}) {
		t.Fatalf("问询之后变了的表 = %v, want 只有关务判断册——本上下文零写入", changed)
	}
	if judgements(held) != 1 || judgements(clean) != 1 {
		t.Fatalf("关务判断册：受限 %d 行、放行 %d 行，want 各 1", judgements(held), judgements(clean))
	}

	for object, asked := range map[string]*httptest.ResponseRecorder{held: askedHeld, clean: askedClean} {
		status, answer := rig.deliver(t, object)
		if status != http.StatusCreated || answer["outcome"] != "DELIVERY_REGISTERED" {
			t.Fatalf("%s 交付 HTTP %d %v", object, status, answer)
		}
		if previewed := gateReadingBytes(t, asked); answer["gateReading"] != previewed {
			t.Fatalf("%s 问询与同参交付的 gateReading 不是同一份：\n问询 %s\n交付 %s", object, previewed, answer["gateReading"])
		}
		if got := judgements(object); got != 1 {
			t.Fatalf("%s 同参交付让关务判断册变成 %d 行——同一请求在同样三源上该命中问询那一次的判断", object, got)
		}
	}
}

// Covers: AT-TF-325 真库半边——在这个作业范围上没有作业事实登记授予，答 403 `OPERATOR_NOT_GRANTED`，整库一张表都不变：连关务也没
// 被问到。
func TestADeliveryPreviewWithoutTheGrantIsRefusedAndAsksNoOne(t *testing.T) {
	rig, _ := newDeliveryPreviewRig(t)
	rig.enterLastMile(t, "SYN-PARCEL-DCG94-UNGRANTED", "SYN-NODE-SIN-HUB")

	before := rig.snapshot(t)
	refused := rig.ask("SYN-OFFSITE-99", "SYN-PARCEL-DCG94-UNGRANTED")
	if refused.Code != http.StatusForbidden || problemCode(t, refused) != "OPERATOR_NOT_GRANTED" {
		t.Fatalf("没有授予 = %d %s, want 403 OPERATOR_NOT_GRANTED", refused.Code, refused.Body.String())
	}
	if changed := changedSince(before, rig.snapshot(t)); len(changed) != 0 {
		t.Fatalf("拒掉的问询改了 %v", changed)
	}
}
