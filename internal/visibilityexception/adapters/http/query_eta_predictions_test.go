package visibilityhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

type etaPredictionReaderDouble struct {
	rows  []ports.ETAPredictionReviewRow
	err   error
	calls int
	query cataloguepage.Query
}

func (double *etaPredictionReaderDouble) ListCurrentETAPredictions(
	_ context.Context, _ domain.TenantID, _ int, query cataloguepage.Query,
) (cataloguepage.Result[ports.ETAPredictionReviewRow], error) {
	double.calls++
	double.query = query
	if double.err != nil {
		return cataloguepage.Result[ports.ETAPredictionReviewRow]{}, double.err
	}
	return cataloguepage.Result[ports.ETAPredictionReviewRow]{Rows: double.rows, Total: int64(len(double.rows))}, nil
}

func serveETAPredictions(
	intake visibilityhttp.OperationsTrackingIntake, reader visibilityhttp.ETAPredictionReviewReader, method, target string,
) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	visibilityhttp.NewQueryETAPredictionsEndpoint(intake, reader).ServeHTTP(recorder, httptest.NewRequest(method, target, nil))
	return recorder
}

// Covers: ETA 预测查阅逐字段透出当前版；人工录入的带 manualEntry（谁、原件、录入时刻），预测来源送来的整键缺席；
// 首版没有 priorVersion；空册答空数组不是 null。
func TestETAPredictionRecordsCarryManualEntryOnlyOnManuallyEnteredVersions(t *testing.T) {
	at := time.Date(2026, 11, 13, 6, 0, 0, 0, time.UTC)
	row := func(version string) ports.ETAPredictionReviewRow {
		return ports.ETAPredictionReviewRow{
			Parcel: "PCL-1", Milestone: "DELIVERED", Source: "CARRIER_PROVIDED", Version: version, Inputs: "inputs-" + version,
			Model: "MANUAL_ENTRY", RangeFrom: at.Add(24 * time.Hour), RangeTo: at.Add(30 * time.Hour), Confidence: "HIGH", PredictedAt: at,
		}
	}
	manual := row("ETAV-2")
	manual.PriorVersion = "ETAV-1"
	manual.ManualEntry = &ports.ManualETAEntryNote{EnteredBy: "SYN-OP-6", Original: "承运商到港通知.pdf", EnteredAt: at}
	reader := &etaPredictionReaderDouble{rows: []ports.ETAPredictionReviewRow{manual, row("ETAV-9")}}

	response := serveETAPredictions(operationsIntakeDouble{query: operationsScope(t)}, reader, http.MethodGet, "/eta-prediction-records?q=PCL")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	if reader.query.Keyword != "PCL" {
		t.Fatalf("q 没交给读口：%+v", reader.query)
	}
	var body struct {
		Outcome     string                       `json:"outcome"`
		Predictions []map[string]json.RawMessage `json:"predictions"`
		Page        json.RawMessage              `json:"page"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.Outcome != "ETA_PREDICTIONS_LISTED" ||
		len(body.Predictions) != 2 || len(body.Page) == 0 {
		t.Fatalf("解不出回答：%v %s", err, response.Body.String())
	}
	entered, sourced := body.Predictions[0], body.Predictions[1]
	if string(entered["manualEntry"]) != `{"enteredBy":"SYN-OP-6","original":"承运商到港通知.pdf","enteredAt":"2026-11-13T06:00:00Z"}` ||
		string(entered["priorVersion"]) != `"ETAV-1"` || string(entered["rangeFrom"]) != `"2026-11-14T06:00:00Z"` ||
		string(entered["rangeTo"]) != `"2026-11-14T12:00:00Z"` || string(entered["model"]) != `"MANUAL_ENTRY"` {
		t.Fatalf("人工录入的那一行走样：%s", response.Body.String())
	}
	for _, key := range []string{"manualEntry", "priorVersion"} {
		if _, present := sourced[key]; present {
			t.Fatalf("预测来源送来的首版不得带 %s：%s", key, response.Body.String())
		}
	}

	empty := serveETAPredictions(operationsIntakeDouble{query: operationsScope(t)}, &etaPredictionReaderDouble{}, http.MethodGet, "/eta-prediction-records")
	var emptyBody struct {
		Predictions json.RawMessage `json:"predictions"`
	}
	if err := json.Unmarshal(empty.Body.Bytes(), &emptyBody); err != nil || string(emptyBody.Predictions) != "[]" {
		t.Fatalf("空册应答 []：%s", empty.Body.String())
	}
}

// Covers: 方法不对 405；集外排序 400 且不触读口；读口失败 500 只带稳定码。
func TestETAPredictionRecordsRefuseBadRequestsBeforeReading(t *testing.T) {
	intake := operationsIntakeDouble{query: operationsScope(t)}
	reader := &etaPredictionReaderDouble{}
	if response := serveETAPredictions(intake, reader, http.MethodPost, "/eta-prediction-records"); response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d", response.Code)
	}
	if response := serveETAPredictions(intake, reader, http.MethodGet, "/eta-prediction-records?sort=parcel"); response.Code != http.StatusBadRequest {
		t.Fatalf("集外排序 status = %d body = %s", response.Code, response.Body.String())
	}
	if response := serveETAPredictions(intake, reader, http.MethodGet, "/eta-prediction-records?source=x"); response.Code != http.StatusBadRequest {
		t.Fatalf("集外参数 status = %d body = %s", response.Code, response.Body.String())
	}
	if reader.calls != 0 {
		t.Fatalf("坏请求触到了读口 %d 次", reader.calls)
	}
	failing := &etaPredictionReaderDouble{err: errors.New("SYN-DB-DOWN")}
	response := serveETAPredictions(intake, failing, http.MethodGet, "/eta-prediction-records")
	if response.Code != http.StatusInternalServerError || !json.Valid(response.Body.Bytes()) ||
		contains(response.Body.String(), "SYN-DB-DOWN") {
		t.Fatalf("读口失败应答 500 且不带内部错误：%d %s", response.Code, response.Body.String())
	}
}

func contains(text, part string) bool {
	for index := 0; index+len(part) <= len(text); index++ {
		if text[index:index+len(part)] == part {
			return true
		}
	}
	return false
}
