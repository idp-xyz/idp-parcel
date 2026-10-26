package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
)

// 本文件对真库走生产装配证逐对象结果「待确认」与揽收「执行方拒收」（票 docs-consistency/code/01）：作业事实渠道收下两格
// 词与来源，库面 CHECK 收下并读回，交付口与冲突判定照常作答。测试输入是合成数据，只记 `S`。

func attemptObjectsByName(t *testing.T, answered *httptest.ResponseRecorder) map[string]map[string]string {
	t.Helper()
	var body struct {
		Objects []map[string]string `json:"objects"`
	}
	if err := json.Unmarshal(answered.Body.Bytes(), &body); err != nil {
		t.Fatalf("解答复：%v；body=%s", err, answered.Body.String())
	}
	objects := map[string]map[string]string{}
	for _, object := range body.Objects {
		objects[object["object"]] = object
	}
	return objects
}

// Covers: `AT-TF-018`「只有车辆到场和司机扫描 → 保存来源，结果待确认，不推导实际接收」与 `AT-TF-017`「包装不合格且执行方
// 明确拒收 → 保存对象级拒收和证据，控制不转移」的生产装配半边——一次到访两件经作业事实渠道落库：待确认带来源与缺口、
// 拒收带依据，两件都没有揽收版本；同一来源身份换成「揽收到手」再交答冲突，不按最后消息覆盖。
func TestAPendingPickupAndAnExecutorRefusalRunThroughTheProductionFace(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	work := workFactTransportForTest(t)
	controlFacts, err := buildControlFactOrchestrations(db)
	if err != nil {
		t.Fatalf("装配控制事实编排：%v", err)
	}
	endpoint := tfhttp.NewPerformOffsitePickupEndpoint(work, controlFacts.pickupAttempt)
	post := func(body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/transport-fulfillment/offsite-pickup-attempts", strings.NewReader(body))
		stampWorkFact(request, "OFFSITE", "SYN-OFFSITE-01", "SYN-DEVICE-OFFSITE")
		recorder := httptest.NewRecorder()
		endpoint.ServeHTTP(recorder, request)
		return recorder
	}
	const visit = `{"sourceId":"SYN-DEVICE-OFFSITE/attempt-18","task":"SYN-TASK-18","attempt":"SYN-ATTEMPT-18",` +
		`"executedBy":"SYN-COURIER-18","place":"SYN-PLACE/shipper-dock-18","plannedFrom":"2026-09-24T08:00:00+08:00",` +
		`"plannedTo":"2026-09-24T10:00:00+08:00","arrivedAt":"2026-09-24T08:40:00+08:00","evidence":"SYN-EVIDENCE/visit-18",` +
		`"segment":"SYN-SEGMENT-18","objects":[` +
		`{"object":"SYN-PARCEL-18-01","outcome":"PENDING_CONFIRMATION","source":"SYN-SOURCE/vehicle-arrival+driver-scan",` +
		`"basis":"SYN-GAP/no-receipt","occurredAt":"2026-09-24T08:45:00+08:00"},` +
		`{"object":"SYN-PARCEL-18-02","outcome":"EXECUTOR_REFUSED","basis":"SYN-REASON/packaging-unacceptable+photo-1",` +
		`"occurredAt":"2026-09-24T08:46:00+08:00"}]}`

	recorded := post(visit)
	if recorded.Code != http.StatusCreated || registrationOutcome(t, recorded) != "ATTEMPT_RECORDED" {
		t.Fatalf("首登答 %d %s，want 201 ATTEMPT_RECORDED", recorded.Code, recorded.Body.String())
	}
	objects := attemptObjectsByName(t, recorded)
	if got := objects["SYN-PARCEL-18-01"]; got["outcome"] != "PENDING_CONFIRMATION" ||
		got["source"] != "SYN-SOURCE/vehicle-arrival+driver-scan" || got["basis"] != "SYN-GAP/no-receipt" || got["pickupVersion"] != "" {
		t.Fatalf("待确认答复 = %v", got)
	}
	if got := objects["SYN-PARCEL-18-02"]; got["outcome"] != "EXECUTOR_REFUSED" ||
		got["basis"] != "SYN-REASON/packaging-unacceptable+photo-1" || got["pickupVersion"] != "" {
		t.Fatalf("拒收答复 = %v", got)
	}
	if strings.Contains(recorded.Body.String(), "segmentEntries") {
		t.Fatalf("没有取得控制的对象却报了进段：%s", recorded.Body.String())
	}

	replayed := post(visit)
	if replayed.Code != http.StatusOK || registrationOutcome(t, replayed) != "EXISTING_RESULT" {
		t.Fatalf("同内容重投答 %d %s，want 200 EXISTING_RESULT（读回经库面与构造门）", replayed.Code, replayed.Body.String())
	}
	if got := attemptObjectsByName(t, replayed)["SYN-PARCEL-18-01"]; got["source"] != "SYN-SOURCE/vehicle-arrival+driver-scan" {
		t.Fatalf("重投读回的待确认丢了来源：%v", got)
	}

	flipped := strings.Replace(visit,
		`"outcome":"PENDING_CONFIRMATION","source":"SYN-SOURCE/vehicle-arrival+driver-scan","basis":"SYN-GAP/no-receipt"`,
		`"outcome":"PICKED_UP","control":"SYN-CONTROL/signed-18"`, 1)
	if conflict := post(flipped); conflict.Code != http.StatusOK || registrationOutcome(t, conflict) != "SOURCE_CONFLICT" {
		t.Fatalf("同一来源改成揽收到手答 %d %s，want 200 SOURCE_CONFLICT", conflict.Code, conflict.Body.String())
	}
}

// Covers: `AT-TF-070`「本人签收和地址来源冲突 → 形成冲突或待确认，不按最后消息覆盖」的生产装配半边——冲突的对象记待确认
// 落库；交付口对它答`未生效`（控制不转移）；同一尝试再交一份「妥投」答冲突，交付口仍答`未生效`。
func TestAPendingDeliveryRunsThroughTheProductionFacesAndIsNotOverwritten(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	decisions := operatorDecisionTransportForTest(t, db)
	work := workFactTransportForTest(t)
	segmentOps, err := buildSegmentOperations(db)
	if err != nil {
		t.Fatalf("装配段运营编排：%v", err)
	}
	attempts, err := buildDeliveryAttemptOrchestration(db)
	if err != nil {
		t.Fatalf("装配派送尝试编排：%v", err)
	}
	delivery, err := buildDeliveryOrchestration(db)
	if err != nil {
		t.Fatalf("装配交付编排：%v", err)
	}
	post := func(endpoint http.Handler, pattern, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, pattern, strings.NewReader(body))
		stampWorkFact(request, "OFFSITE", "SYN-OFFSITE-01", "SYN-DEVICE-OFFSITE")
		recorder := httptest.NewRecorder()
		endpoint.ServeHTTP(recorder, request)
		return recorder
	}
	step := func(name string, answered *httptest.ResponseRecorder, status int, outcome string) {
		t.Helper()
		if answered.Code != status || registrationOutcome(t, answered) != outcome {
			t.Fatalf("%s：答 %d %s，want %d %s", name, answered.Code, answered.Body.String(), status, outcome)
		}
	}
	attemptEndpoint := tfhttp.NewRecordDeliveryAttemptEndpoint(work, attempts)
	deliveryEndpoint := tfhttp.NewRegisterEffectiveDeliveryEndpoint(work, delivery)
	const attemptBody = `{"task":"SYN-DISPATCH-70","attempt":"SYN-DELIVERY-ATTEMPT-70","executedBy":"SYN-COURIER-70",` +
		`"place":"SYN-PLACE/consignee-70","plannedFrom":"2026-09-25T09:00:00+08:00","plannedTo":"2026-09-25T12:00:00+08:00",` +
		`"arrivedAt":"2026-09-25T10:00:00+08:00","evidence":"SYN-EVIDENCE/arrival-70","objects":[{"object":"SYN-PARCEL-70-01",` +
		`"outcome":"PENDING_CONFIRMATION","source":"SYN-SOURCE/signature-scan+address-geofence",` +
		`"basis":"SYN-CONFLICT/signature-vs-address","occurredAt":"2026-09-25T10:05:00+08:00"}]}`
	const deliveryBody = `{"attempt":"SYN-DELIVERY-ATTEMPT-70","object":"SYN-PARCEL-70-01","method":"HANDED_TO_RECIPIENT",` +
		`"recipient":"SYN-RECIPIENT/consignee-70","proof":"SYN-POD/signature-70"}`

	step("建派送任务",
		post(tfhttp.NewOpenDispatchTaskEndpoint(decisions, segmentOps.opener), "/transport-fulfillment-dispatch-task-registrations",
			`{"task":"SYN-DISPATCH-70","kind":"DELIVERY","objects":["SYN-PARCEL-70-01"],"place":"SYN-PLACE/consignee-70",`+
				`"windowFrom":"2026-09-25T09:00:00+08:00","windowTo":"2026-09-25T12:00:00+08:00",`+
				`"conditions":"SYN-CONDITION/signature-required","openedAt":"2026-09-24T20:00:00+08:00"}`),
		http.StatusCreated, "DISPATCH_TASK_OPENED")

	recorded := post(attemptEndpoint, "/transport-fulfillment/delivery-attempts", attemptBody)
	step("登记一次待确认", recorded, http.StatusCreated, "ATTEMPT_RECORDED")
	if got := attemptObjectsByName(t, recorded)["SYN-PARCEL-70-01"]; got["outcome"] != "PENDING_CONFIRMATION" ||
		got["source"] != "SYN-SOURCE/signature-scan+address-geofence" || got["basis"] != "SYN-CONFLICT/signature-vs-address" {
		t.Fatalf("待确认答复 = %v", got)
	}

	step("待确认之上的交付", post(deliveryEndpoint, "/transport-fulfillment/deliveries", deliveryBody), http.StatusOK, "NOT_EFFECTIVE")

	step("同一尝试改说妥投",
		post(attemptEndpoint, "/transport-fulfillment/delivery-attempts",
			strings.Replace(attemptBody,
				`"outcome":"PENDING_CONFIRMATION","source":"SYN-SOURCE/signature-scan+address-geofence","basis":"SYN-CONFLICT/signature-vs-address"`,
				`"outcome":"DELIVERED"`, 1)),
		http.StatusOK, "SOURCE_CONFLICT")

	step("冲突之后的交付", post(deliveryEndpoint, "/transport-fulfillment/deliveries", deliveryBody), http.StatusOK, "NOT_EFFECTIVE")
}
