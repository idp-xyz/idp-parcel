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

// Covers: 称重量方登记口与作废口收下带纳秒的发生时间（RFC 3339 放行）之后，原样重放答 200 EXISTING_RESULT，而不是在读回比对上
// 答 500。编排走生产装配（buildActualMeasurementOrchestration），登记册是真实 PostgreSQL：库列只存到微秒，形成处不截到微秒，
// 重放读回的摘要就与这次形成的对不上。测试输入是隔离合成，只记 `S`。
func TestTheMeasurementLinesReplayAnOccurrenceWithNanosecondsAgainstARealDatabase(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	handler, err := buildActualMeasurementOrchestration(db)
	if err != nil {
		t.Fatalf("装配测量编排：%v", err)
	}
	intake, ok := workFactReceptionForTest(t).(nodeopshttp.MeasurementIntake)
	if !ok {
		t.Fatal("作业事实渠道的入口不是测量入口")
	}
	record := nodeopshttp.NewRecordMeasurementEndpoint(intake, handler)
	invalidate := nodeopshttp.NewInvalidateMeasurementEndpoint(intake, handler)

	post := func(endpoint http.Handler, path, body string) (int, string) {
		t.Helper()
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		stampWorkFact(request, "NODE", "SYN-NODE-SHA-HUB", "SYN-DEVICE-NODE")
		endpoint.ServeHTTP(recorder, request)
		return recorder.Code, registrationOutcome(t, recorder)
	}
	measurement := func(sourceID, occurredAt string) string {
		return `{"sourceId":"` + sourceID + `","objectKind":"HANDLING_UNIT","objectId":"SYN-HU-08-0001",` +
			`"location":"SYN-BENCH-01","basis":"SYN-PARCEL-08","performedBy":"SYN-SCALE-01","evidenceRef":"WEIGH/1",` +
			`"occurredAt":"` + occurredAt + `","weightValue":"2.10","weightUnit":"KG",` +
			`"length":"20","width":"15","height":"10","dimensionUnit":"CM"}`
	}
	const measurements = "/node-operations/measurements"
	const invalidations = "/node-operations/measurement-invalidations"

	nanos := measurement("SYN-DEVICE-08/WEIGH-1", "2026-11-17T02:00:00.123456789Z")
	if status, outcome := post(record, measurements, nanos); status != http.StatusCreated || outcome != "MEASUREMENT_RECORDED" {
		t.Fatalf("带纳秒的登记 = %d %s，want 201 MEASUREMENT_RECORDED", status, outcome)
	}
	if status, outcome := post(record, measurements, nanos); status != http.StatusOK || outcome != "EXISTING_RESULT" {
		t.Fatalf("原样重放 = %d %s，want 200 EXISTING_RESULT", status, outcome)
	}
	if status, outcome := post(record, measurements, measurement("SYN-DEVICE-08/WEIGH-2", "2026-11-17T02:30:00Z")); status != http.StatusCreated || outcome != "MEASUREMENT_RECORDED" {
		t.Fatalf("同一件实物上整秒的复测 = %d %s，want 201 MEASUREMENT_RECORDED", status, outcome)
	}

	void := `{"sourceId":"SYN-DEVICE-08/VOID-1","targetSourceId":"SYN-DEVICE-08/WEIGH-1","reason":"称错件",` +
		`"performedBy":"SYN-SCALE-01","evidenceRef":"WEIGH/2","occurredAt":"2026-11-17T03:00:00.987654321Z"}`
	if status, outcome := post(invalidate, invalidations, void); status != http.StatusCreated || outcome != "INVALIDATION_RECORDED" {
		t.Fatalf("带纳秒的作废 = %d %s，want 201 INVALIDATION_RECORDED", status, outcome)
	}
	if status, outcome := post(invalidate, invalidations, void); status != http.StatusOK || outcome != "EXISTING_RESULT" {
		t.Fatalf("作废原样重放 = %d %s，want 200 EXISTING_RESULT", status, outcome)
	}
}
