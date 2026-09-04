package pricinghttp_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// 本文件对被挂起评价联动端点（ADR-0105 Decision 五；票 05 第 2 项）证传输面：方法门、未配置 403、asOf 回显、
// 空册答空数组而不是缺席、读口故障答 NO_ANSWER_FORMED。

type pendingCountsReaderDouble struct {
	tenant domain.TenantID
	counts []ports.PendingSeriesEvaluationCount
	err    error
}

func (double *pendingCountsReaderDouble) CountPendingSeriesEvaluations(_ context.Context, tenant domain.TenantID) ([]ports.PendingSeriesEvaluationCount, error) {
	if double.err != nil {
		return nil, double.err
	}
	double.tenant = tenant
	return double.counts, nil
}

func pendingEndpoint(counts []ports.PendingSeriesEvaluationCount, err error) (http.Handler, *pendingCountsReaderDouble) {
	reader := &pendingCountsReaderDouble{counts: counts, err: err}
	return pricinghttp.NewQueryPendingSeriesEvaluationsEndpoint(intakeDouble{query: pricinghttp.CatalogueQuery{}}, reader, coverageClockDouble{at: coverageAt}), reader
}

func TestPendingEndpointTakesOnlyGET(t *testing.T) {
	endpoint, _ := pendingEndpoint(nil, nil)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/pricing-pending-series-evaluations", nil))
	if recorder.Code != http.StatusMethodNotAllowed || recorder.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("status = %d Allow = %q", recorder.Code, recorder.Header().Get("Allow"))
	}
}

func TestPendingEndpointAnswersUnconfiguredIntakeWith403(t *testing.T) {
	endpoint := pricinghttp.NewQueryPendingSeriesEvaluationsEndpoint(pricinghttp.UnconfiguredIntake{}, &pendingCountsReaderDouble{}, coverageClockDouble{at: coverageAt})
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/pricing-pending-series-evaluations", nil))
	if recorder.Code != http.StatusForbidden || errorCode(t, recorder) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
}

// Covers: 空册答空数组、asOf 回显；有数时逐种类透出。
func TestPendingEndpointEchoesAsOfAndTranscribesCounts(t *testing.T) {
	endpoint, _ := pendingEndpoint(nil, nil)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/pricing-pending-series-evaluations", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	body := coverageBody(t, recorder)
	if body["outcome"] != "PENDING_SERIES_EVALUATIONS_COUNTED" || body["asOf"] != "2026-08-25T09:00:00Z" {
		t.Fatalf("body = %#v", body)
	}
	counts, ok := body["counts"].([]any)
	if !ok || len(counts) != 0 {
		t.Fatalf("empty register must answer an empty array, got %#v", body["counts"])
	}

	endpoint, _ = pendingEndpoint([]ports.PendingSeriesEvaluationCount{{Kind: "FUEL_RATE", EvaluationCount: 2}}, nil)
	recorder = httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/pricing-pending-series-evaluations", nil))
	counts = coverageBody(t, recorder)["counts"].([]any)
	if len(counts) != 1 || counts[0].(map[string]any)["kind"] != "FUEL_RATE" || counts[0].(map[string]any)["evaluationCount"] != float64(2) {
		t.Fatalf("counts = %#v", counts)
	}
}

func TestPendingEndpointReportsReaderFailureAsNoAnswer(t *testing.T) {
	endpoint, _ := pendingEndpoint(nil, errors.New("down"))
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/pricing-pending-series-evaluations", nil))
	if recorder.Code != http.StatusInternalServerError || errorCode(t, recorder) != "NO_ANSWER_FORMED" {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
}
