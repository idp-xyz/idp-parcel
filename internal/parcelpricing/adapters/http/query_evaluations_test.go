package pricinghttp_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// 本文件对评价登记册端点（票 admin-skeleton-closure-batch/03）证传输面，判据与
// 本包两个目录端点同源：方法门、未配置 Intake 403、行体逐字段转写、空册答空数组、
// 读失败答 5xx。替身与断言助手复用 query_pricing_catalogue_test.go 的既有件。

type evaluationReaderDouble struct {
	tenant domain.TenantID
	rows   []ports.EvaluationCatalogueRow
	err    error
}

func (double *evaluationReaderDouble) ListEvaluations(
	_ context.Context,
	tenant domain.TenantID,
	_ int,
) ([]ports.EvaluationCatalogueRow, error) {
	if double.err != nil {
		return nil, double.err
	}
	if tenant != double.tenant {
		return nil, nil
	}
	return double.rows, nil
}

func TestEvaluationsEndpointOnlyAcceptsGet(t *testing.T) {
	endpoint := pricinghttp.NewQueryEvaluationsEndpoint(
		intakeDouble{query: catalogueQuery(t)},
		&evaluationReaderDouble{},
	)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/pricing-evaluations", nil))

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST 答 %d, want 405", recorder.Code)
	}
	if allow := recorder.Header().Get("Allow"); allow != http.MethodGet {
		t.Fatalf("Allow = %q, want GET", allow)
	}
}

func TestEvaluationsEndpointAnswersForbiddenWhenIntakeUnconfigured(t *testing.T) {
	endpoint := pricinghttp.NewQueryEvaluationsEndpoint(
		pricinghttp.UnconfiguredIntake{},
		&evaluationReaderDouble{},
	)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/pricing-evaluations", nil))

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("未配置 Intake 答 %d, want 403", recorder.Code)
	}
}

func TestEvaluationsEndpointTranscribesRowsVerbatim(t *testing.T) {
	query := catalogueQuery(t)
	rows := []ports.EvaluationCatalogueRow{
		{
			EvaluationID:      "SYN-EVAL-002",
			Status:            "UNRATABLE",
			SemanticDigest:    "sha256:syn-semantic-2",
			PlanContentDigest: "sha256:syn-plan-2",
			Canonicalization:  "c14n-v1",
			RecordedAt:        pricingBaseAt.Add(time.Minute),
		},
		{
			EvaluationID:      "SYN-EVAL-001",
			Status:            "COMPLETED",
			SemanticDigest:    "sha256:syn-semantic-1",
			PlanContentDigest: "sha256:syn-plan-1",
			Canonicalization:  "c14n-v1",
			RecordedAt:        pricingBaseAt,
		},
	}
	endpoint := pricinghttp.NewQueryEvaluationsEndpoint(
		intakeDouble{query: query},
		&evaluationReaderDouble{tenant: query.Scope.Tenant(), rows: rows},
	)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/pricing-evaluations", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("上列答 %d, want 200\n%s", recorder.Code, recorder.Body.String())
	}
	body := decodeBody(t, recorder)
	if body["outcome"] != "EVALUATIONS_LISTED" {
		t.Fatalf("outcome = %v", body["outcome"])
	}
	evaluations, ok := body["evaluations"].([]any)
	if !ok || len(evaluations) != 2 {
		t.Fatalf("evaluations 形状变形：%v", body["evaluations"])
	}
	first, _ := evaluations[0].(map[string]any)
	if first["evaluationId"] != "SYN-EVAL-002" || first["status"] != "UNRATABLE" ||
		first["semanticDigest"] != "sha256:syn-semantic-2" ||
		first["planContentDigest"] != "sha256:syn-plan-2" ||
		first["canonicalization"] != "c14n-v1" {
		t.Fatalf("行体转写变形：%v", first)
	}
	if _, present := first["recordedAt"].(string); !present {
		t.Fatalf("登记时间没透出：%v", first)
	}
}

func TestEvaluationsEndpointAnswersEmptyRegistryAsEmptyArray(t *testing.T) {
	query := catalogueQuery(t)
	endpoint := pricinghttp.NewQueryEvaluationsEndpoint(
		intakeDouble{query: query},
		&evaluationReaderDouble{tenant: query.Scope.Tenant(), rows: nil},
	)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/pricing-evaluations", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("空册答 %d, want 200", recorder.Code)
	}
	evaluations, ok := decodeBody(t, recorder)["evaluations"].([]any)
	if !ok {
		t.Fatal("空册没交回数组")
	}
	if len(evaluations) != 0 {
		t.Fatalf("空册交回 %d 行", len(evaluations))
	}
}

func TestEvaluationsEndpointAnswersServerErrorWhenReadFails(t *testing.T) {
	endpoint := pricinghttp.NewQueryEvaluationsEndpoint(
		intakeDouble{query: catalogueQuery(t)},
		&evaluationReaderDouble{err: errors.New("寄了")},
	)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/pricing-evaluations", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("读失败答 %d, want 500", recorder.Code)
	}
	if code := errorCode(t, recorder); code != "NO_ANSWER_FORMED" {
		t.Fatalf("错误码 = %q", code)
	}
}
