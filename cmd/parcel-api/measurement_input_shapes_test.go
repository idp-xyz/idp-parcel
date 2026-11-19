package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// measurementQuantities 是称重量方载荷里的六个格：重量与单位、三边与它们共用的单位。
type measurementQuantities struct {
	weightValue, weightUnit, length, width, height, dimensionUnit string
}

func validQuantities() measurementQuantities {
	return measurementQuantities{"2.10", "KG", "20", "15", "10", "CM"}
}

func measurementPayload(t *testing.T, sourceID string, quantities measurementQuantities) string {
	t.Helper()
	encoded, err := json.Marshal(map[string]string{
		"sourceId": sourceID, "objectKind": "HANDLING_UNIT", "objectId": "SYN-HU-NB2-0001",
		"location": "SYN-BENCH-01", "basis": "SYN-PARCEL-NB2", "performedBy": "SYN-SCALE-01", "evidenceRef": "WEIGH/1",
		"occurredAt":  "2026-11-17T02:00:00Z",
		"weightValue": quantities.weightValue, "weightUnit": quantities.weightUnit,
		"length": quantities.length, "width": quantities.width, "height": quantities.height,
		"dimensionUnit": quantities.dimensionUnit,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

// Covers: 称重量方登记口对各种坏输入的答复：缺必填格（缺、空白）在口上答 400 MALFORMED_REQUEST；数量格式不对、单位不在产品内置
// 目录上、单位类别不对都过得了口、到编排才不收，答 200 NOT_ACCEPTED；两种都不落一行。合法输入答 201 MEASUREMENT_RECORDED，
// 带首尾空白的数量与单位落下去掉空白的一行。编排走生产装配，登记册是真实 PostgreSQL，落下的行逐列核对。测试输入是隔离合成，只记 `S`。
func TestMalformedMeasurementsAnswerTheirStatusAndOutcomeAndWriteNothingAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
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
	post := func(body string) (status int, outcome, problem string) {
		t.Helper()
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/node-operations/measurements", strings.NewReader(body))
		stampWorkFact(request, "NODE", "SYN-NODE-SHA-HUB", "SYN-DEVICE-NODE")
		record.ServeHTTP(recorder, request)
		var answer struct {
			Outcome string `json:"outcome"`
			Error   struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
			t.Fatalf("答复不是 JSON：%s：%v", recorder.Body.String(), err)
		}
		return recorder.Code, answer.Outcome, answer.Error.Code
	}
	with := func(edit func(*measurementQuantities)) measurementQuantities {
		quantities := validQuantities()
		edit(&quantities)
		return quantities
	}

	missing := map[string]measurementQuantities{
		"缺重量":      with(func(q *measurementQuantities) { q.weightValue = "" }),
		"缺重量单位":    with(func(q *measurementQuantities) { q.weightUnit = "" }),
		"缺长":       with(func(q *measurementQuantities) { q.length = "" }),
		"宽只有空白":    with(func(q *measurementQuantities) { q.width = "  " }),
		"缺高":       with(func(q *measurementQuantities) { q.height = "" }),
		"缺尺寸单位":    with(func(q *measurementQuantities) { q.dimensionUnit = "" }),
		"重量单位只有空白": with(func(q *measurementQuantities) { q.weightUnit = " \t" }),
		"尺寸单位只有空白": with(func(q *measurementQuantities) { q.dimensionUnit = "  " }),
	}
	notAccepted := map[string]measurementQuantities{
		"重量不是数":     with(func(q *measurementQuantities) { q.weightValue = "abc" }),
		"重量为零":      with(func(q *measurementQuantities) { q.weightValue = "0" }),
		"重量为负":      with(func(q *measurementQuantities) { q.weightValue = "-2.10" }),
		"长是指数写法":    with(func(q *measurementQuantities) { q.length = "1e2" }),
		"宽小数点后没有数":  with(func(q *measurementQuantities) { q.width = "15." }),
		"高是千分位":     with(func(q *measurementQuantities) { q.height = "1,000" }),
		"重量单位不在目录上": with(func(q *measurementQuantities) { q.weightUnit = "斤" }),
		"尺寸单位不在目录上": with(func(q *measurementQuantities) { q.dimensionUnit = "尺" }),
		"重量格报了长度单位": with(func(q *measurementQuantities) { q.weightUnit = "CM" }),
		"尺寸格报了重量单位": with(func(q *measurementQuantities) { q.dimensionUnit = "KG" }),
		"单位大小写不对":   with(func(q *measurementQuantities) { q.weightUnit = "kg" }),
		"单位带不可见字符":  with(func(q *measurementQuantities) { q.dimensionUnit = "C\u00a0M" }),
		"数字之间夹着空白":  with(func(q *measurementQuantities) { q.weightValue = "2 10" }),
		"全角数字":      with(func(q *measurementQuantities) { q.length = "２０" }),
		"带正号":       with(func(q *measurementQuantities) { q.height = "+10" }),
		"小数点前没有数":   with(func(q *measurementQuantities) { q.width = ".5" }),
		"两个小数点":     with(func(q *measurementQuantities) { q.weightValue = "2.1.0" }),
		"单位多了一个字母":  with(func(q *measurementQuantities) { q.weightUnit = "KGS" }),
	}
	index := 0
	for name, quantities := range missing {
		index++
		status, outcome, problem := post(measurementPayload(t, "SYN-DEVICE-NB2/MISSING-"+strconv.Itoa(index), quantities))
		if status != http.StatusBadRequest || outcome != "" || problem != "MALFORMED_REQUEST" {
			t.Errorf("%s：答 %d %q %q，want 400 MALFORMED_REQUEST", name, status, outcome, problem)
		}
	}
	for name, quantities := range notAccepted {
		index++
		status, outcome, problem := post(measurementPayload(t, "SYN-DEVICE-NB2/BAD-"+strconv.Itoa(index), quantities))
		if status != http.StatusOK || outcome != "NOT_ACCEPTED" || problem != "" {
			t.Errorf("%s：答 %d %q %q，want 200 NOT_ACCEPTED", name, status, outcome, problem)
		}
	}

	var rows int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM node_operations.actual_measurement`).Scan(&rows); err != nil {
		t.Fatalf("数册上的行：%v", err)
	}
	if rows != 0 {
		t.Fatalf("坏输入落了 %d 行，want 0（共 %d 条坏输入）", rows, index)
	}

	columns := func(sourceID string) [6]string {
		t.Helper()
		var got [6]string
		if err := pool.QueryRow(context.Background(),
			`SELECT weight_value, weight_unit, length_value, width_value, height_value, dimension_unit
			   FROM node_operations.actual_measurement WHERE tenant_id = 'SYN-TENANT-01' AND source_id = $1`, sourceID,
		).Scan(&got[0], &got[1], &got[2], &got[3], &got[4], &got[5]); err != nil {
			t.Fatalf("读 %s 落下的行：%v", sourceID, err)
		}
		return got
	}
	if status, outcome, _ := post(measurementPayload(t, "SYN-DEVICE-NB2/CLEAN", validQuantities())); status != http.StatusCreated || outcome != "MEASUREMENT_RECORDED" {
		t.Fatalf("合法的一条 = %d %s，want 201 MEASUREMENT_RECORDED", status, outcome)
	}
	if got, want := columns("SYN-DEVICE-NB2/CLEAN"), [6]string{"2.10", "KG", "20", "15", "10", "CM"}; got != want {
		t.Fatalf("合法的一条落下 %v，want %v", got, want)
	}
	padded := measurementQuantities{" 2.10 ", " KG ", "\t20", "15\n", " 10", " CM "}
	if status, outcome, _ := post(measurementPayload(t, "SYN-DEVICE-NB2/PADDED", padded)); status != http.StatusCreated || outcome != "MEASUREMENT_RECORDED" {
		t.Fatalf("带空白的一条 = %d %s，want 201 MEASUREMENT_RECORDED", status, outcome)
	}
	if got, want := columns("SYN-DEVICE-NB2/PADDED"), [6]string{"2.10", "KG", "20", "15", "10", "CM"}; got != want {
		t.Fatalf("带空白的一条落下 %v，want 去掉空白的 %v", got, want)
	}
	lb := measurementQuantities{"2.10", "LB", "20", "15", "10", "IN"}
	if status, outcome, _ := post(measurementPayload(t, "SYN-DEVICE-NB2/OTHER-UNITS", lb)); status != http.StatusCreated || outcome != "MEASUREMENT_RECORDED" {
		t.Fatalf("目录上别的单位 = %d %s，want 201 MEASUREMENT_RECORDED", status, outcome)
	}
	if got, want := columns("SYN-DEVICE-NB2/OTHER-UNITS"), [6]string{"2.10", "LB", "20", "15", "10", "IN"}; got != want {
		t.Fatalf("别的单位落下 %v，want %v", got, want)
	}
}
