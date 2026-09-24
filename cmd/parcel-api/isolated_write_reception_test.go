package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// Covers: 票 operator-channel/08 完成判据「隔离环境里上列各口对合成写答业务结果而不是 ACCESS_CHANNEL_NOT_CONFIGURED」——
// 节点收寄口。Intake 走进程自己那道门（buildIsolatedWriteAdmission），编排走生产装配（buildReceptionOrchestration），中间是
// 生产的端点构造函数：三件都是 main 交给路由的那一份。
//
// 明确拒收不经身份核对，真实落库答`未收寄`；明确接收撞上身份核对缝显式未配置，如实答`收寄待确认`——两格都是编排形成的
// 业务答案（ADR-0022：2xx 带 outcome），不是渠道层的 403。带 tenantId 的载荷在 Intake 就拒，编排一次也不该被调到。
// 测试输入是隔离合成，只记 `S`。
func TestIsolatedReceptionAnswersBusinessOutcomesAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	reception, err := buildReceptionOrchestration(db)
	if err != nil {
		t.Fatalf("装配收寄编排：%v", err)
	}
	endpoint := nodeopshttp.NewReceiveDeliveredUnitEndpoint(isolatedWriteAdmissionForTest(t).nodeOperationsIntake(), reception)
	post := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		recorder := httptest.NewRecorder()
		endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/node-operations/receptions", strings.NewReader(body)))
		return recorder
	}

	refused := post(`{"sourceId":"SYN-DEVICE-08/000001","deliveredBy":"SYN-COURIER-08","handlingUnit":"SYN-HU-08-0001",` +
		`"claim":"REFUSED","refusalReason":"SYN 外包装破损拒收","occurredAt":"2026-09-24T09:00:00+08:00"}`)
	if refused.Code != http.StatusOK || registrationOutcome(t, refused) != "INTAKE_NOT_FORMED" {
		t.Fatalf("明确拒收答 %d %s，want 200 INTAKE_NOT_FORMED", refused.Code, refused.Body.String())
	}

	received := post(`{"sourceId":"SYN-DEVICE-08/000002","deliveredBy":"SYN-COURIER-08","handlingUnit":"SYN-HU-08-0002",` +
		`"mark":"SYN-MARK-08-0002","claim":"RECEIVED","evidenceRef":"SYN-EVIDENCE/tally-08-0002","occurredAt":"2026-09-24T09:05:00+08:00"}`)
	if received.Code != http.StatusOK || registrationOutcome(t, received) != "RECEPTION_UNDECIDED" {
		t.Fatalf("明确接收答 %d %s，want 200 RECEPTION_UNDECIDED（身份核对缝显式未配置）", received.Code, received.Body.String())
	}

	selfReported := post(`{"tenantId":"SYN-TENANT-01","sourceId":"SYN-DEVICE-08/000003","deliveredBy":"SYN-COURIER-08",` +
		`"handlingUnit":"SYN-HU-08-0003","claim":"REFUSED","refusalReason":"r","occurredAt":"2026-09-24T09:10:00+08:00"}`)
	if selfReported.Code != http.StatusBadRequest || problemCode(t, selfReported) != "MALFORMED_REQUEST" {
		t.Fatalf("带 tenantId 的载荷答 %d %s，want 400 MALFORMED_REQUEST", selfReported.Code, selfReported.Body.String())
	}
	assertNoOutcome(t, selfReported, "/node-operations/receptions")
}
