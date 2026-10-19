package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
)

// 本文件在真渠道真库上证票 tf-face-algebra/01：零时刻之后不足一微秒的业务时间、含 NUL 的引用都答 200 `未受理`
// 而不是写进去读不回或 500，库零行。装载分配走生产端点构造、生产准入桥与 buildSegmentOperations；移动事实、派送尝试走
// WorkFactCommandIntake 与生产编排。NUL 在 JSON 里写作 \u0000，解码后进命令的是真 NUL。测试输入是隔离合成，只记 `S`。

var channelRegistryZero = map[string]string{
	"零点后 1ns":            "0001-01-01T00:00:00.000000001Z",
	"零点后 999ns":          "0001-01-01T00:00:00.000000999Z",
	"+14:00 写法零点后 500ns": "0001-01-01T14:00:00.0000005+14:00",
}

func tableRows(t *testing.T, pool *pgxpool.Pool, table string) int {
	t.Helper()
	var rows int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM transport_fulfillment.`+table).Scan(&rows); err != nil {
		t.Fatalf("数 %s：%v", table, err)
	}
	return rows
}

// notAcceptedOnTheChannel 断言`未受理`那一格：装载分配与移动事实写作 INPUT_NOT_ACCEPTED，派送尝试写作 SOURCE_NOT_ACCEPTED。
func notAcceptedOnTheChannel(t *testing.T, name, want string, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Code != http.StatusOK || registrationOutcome(t, response) != want {
		t.Fatalf("%s：答 %d %s，want 200 %s", name, response.Code, response.Body.String(), want)
	}
}

// Covers: 票 tf-face-algebra/01 完成判据「HTTP 真渠道回归」——装载分配零点格与版本、成员含 NUL 答 200 INPUT_NOT_ACCEPTED，
// 版本行与成员行零行；零点后一微秒照常形成，原样重放答已在册。
func TestLoadAssignmentUnstorableValuesAreNotAcceptedOnTheProductionChannel(t *testing.T) {
	fixture := newChainFaceFixture(t)
	post := func(body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/transport-fulfillment-load-assignment-registrations", strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer presented.operator.token")
		response := httptest.NewRecorder()
		fixture.endpoint.ServeHTTP(response, request)
		return response
	}
	for name, at := range channelRegistryZero {
		notAcceptedOnTheChannel(t, name, "INPUT_NOT_ACCEPTED", post(chainFaceBody("SYN-LA-01-ZERO", "SYN-SCHEDULE-01", "SYN-LA-V1", at, "SYN-PARCEL-01")))
	}
	notAcceptedOnTheChannel(t, "版本含 NUL", "INPUT_NOT_ACCEPTED", post(
		`{"assignment":"SYN-LA-01-NUL-V","schedule":"SYN-SCHEDULE-01","members":["SYN-PARCEL-01"],"version":"SYN-LA-V1\u0000","assignedAt":"2026-09-25T10:00:00Z"}`))
	notAcceptedOnTheChannel(t, "成员含 NUL", "INPUT_NOT_ACCEPTED", post(
		`{"assignment":"SYN-LA-01-NUL-M","schedule":"SYN-SCHEDULE-01","members":["SYN-PARCEL-\u000001"],"version":"SYN-LA-V1","assignedAt":"2026-09-25T10:00:00Z"}`))
	for _, table := range []string{"load_assignment", "load_assignment_member"} {
		if rows := tableRows(t, fixture.pool, table); rows != 0 {
			t.Fatalf("%s 有 %d 行，期望零行", table, rows)
		}
	}

	micro := chainFaceBody("SYN-LA-01-MICRO", "SYN-SCHEDULE-01", "SYN-LA-V1", "0001-01-01T00:00:00.000001Z", "SYN-PARCEL-01")
	if first := post(micro); first.Code != http.StatusCreated || registrationOutcome(t, first) != "LOAD_ASSIGNMENT_FORMED" {
		t.Fatalf("零点后 1µs 首登：答 %d %s", first.Code, first.Body.String())
	}
	if replay := post(micro); replay.Code != http.StatusOK || registrationOutcome(t, replay) != "LOAD_ASSIGNMENT_VERSION_EXISTS" {
		t.Fatalf("零点后 1µs 原样重放：答 %d %s", replay.Code, replay.Body.String())
	}
}

// Covers: 票 tf-face-algebra/01 完成判据「HTTP 真渠道回归」——移动事实零点格与地点含 NUL 答 200 INPUT_NOT_ACCEPTED，事实行零行。
func TestMovementFactUnstorableValuesAreNotAcceptedOnTheProductionChannel(t *testing.T) {
	pool, _, endpoint := sameIdentityProductionEndpoint(t)
	post := func(body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/transport-fulfillment/movement-facts", strings.NewReader(body))
		stampWorkFact(request, "OFFSITE", "SYN-OFFSITE-01", "SYN-DEVICE-A")
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, request)
		return response
	}
	for name, at := range channelRegistryZero {
		notAcceptedOnTheChannel(t, name, "INPUT_NOT_ACCEPTED", post(
			`{"fact":"SYN-MOVE-01-ZERO","schedule":"SYN-SCHEDULE-01","kind":"ARRIVAL","location":"SYN-NODE-01","version":"1","occurredAt":"`+at+`"}`))
	}
	notAcceptedOnTheChannel(t, "地点含 NUL", "INPUT_NOT_ACCEPTED", post(
		`{"fact":"SYN-MOVE-01-NUL","schedule":"SYN-SCHEDULE-01","kind":"ARRIVAL","location":"SYN-NODE-\u000001","version":"1","occurredAt":"2026-09-25T10:00:00Z"}`))
	if rows := tableRows(t, pool, "transport_movement_fact"); rows != 0 {
		t.Fatalf("transport_movement_fact 有 %d 行，期望零行", rows)
	}
}

// Covers: 票 tf-face-algebra/01 完成判据「HTTP 真渠道回归」——任务在册时派送尝试证据含 NUL 答 200 SOURCE_NOT_ACCEPTED，尝试行与结果行零行。
func TestDeliveryAttemptEvidenceWithNULIsNotAcceptedOnTheProductionChannel(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
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

	open := httptest.NewRequest(http.MethodPost, "/transport-fulfillment-dispatch-task-registrations", strings.NewReader(
		`{"task":"SYN-DISPATCH-01-NUL","kind":"DELIVERY","objects":["SYN-PARCEL-01-NUL"],"place":"SYN-PLACE/consignee-01",`+
			`"windowFrom":"2026-09-25T09:00:00+08:00","windowTo":"2026-09-25T12:00:00+08:00",`+
			`"conditions":"SYN-CONDITION/signature-required","openedAt":"2026-09-24T20:00:00+08:00"}`))
	stampWorkFact(open, "OFFSITE", "SYN-OFFSITE-01", "SYN-DEVICE-OFFSITE")
	opened := httptest.NewRecorder()
	tfhttp.NewOpenDispatchTaskEndpoint(decisions, segmentOps.opener).ServeHTTP(opened, open)
	if opened.Code != http.StatusCreated || registrationOutcome(t, opened) != "DISPATCH_TASK_OPENED" {
		t.Fatalf("建派送任务：答 %d %s", opened.Code, opened.Body.String())
	}

	request := httptest.NewRequest(http.MethodPost, "/transport-fulfillment/delivery-attempts", strings.NewReader(
		`{"task":"SYN-DISPATCH-01-NUL","attempt":"SYN-DELIVERY-ATTEMPT-01-NUL","executedBy":"SYN-COURIER-01",`+
			`"place":"SYN-PLACE/consignee-01","plannedFrom":"2026-09-25T09:00:00+08:00","plannedTo":"2026-09-25T12:00:00+08:00",`+
			`"arrivedAt":"2026-09-25T10:00:00+08:00","evidence":"SYN-EVIDENCE/arrival-\u000001",`+
			`"objects":[{"object":"SYN-PARCEL-01-NUL","outcome":"DELIVERED","occurredAt":"2026-09-25T10:05:00+08:00"}]}`))
	stampWorkFact(request, "OFFSITE", "SYN-OFFSITE-01", "SYN-DEVICE-OFFSITE")
	response := httptest.NewRecorder()
	tfhttp.NewRecordDeliveryAttemptEndpoint(work, attempts).ServeHTTP(response, request)
	notAcceptedOnTheChannel(t, "派送尝试证据含 NUL", "SOURCE_NOT_ACCEPTED", response)
	for _, table := range []string{"delivery_attempt", "delivery_attempt_result"} {
		if rows := tableRows(t, pool, table); rows != 0 {
			t.Fatalf("%s 有 %d 行，期望零行", table, rows)
		}
	}
}
