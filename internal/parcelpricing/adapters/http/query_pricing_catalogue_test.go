package pricinghttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// 本文件对两个目录端点(ADR-0077,票 master-data-wiring/02)证传输面:方法门、
// 未配置 Intake 一律 403、行体逐字段转写且可缺席键如实缺席、空目录答空数组、
// 读失败答 5xx、坏请求 400。

var pricingBaseAt = time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)

func pricingValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}

type intakeDouble struct {
	query pricinghttp.CatalogueQuery
	err   error
}

func (double intakeDouble) IntakeCatalogueQuery(
	_ context.Context,
	_ *http.Request,
) (pricinghttp.CatalogueQuery, error) {
	if double.err != nil {
		return pricinghttp.CatalogueQuery{}, double.err
	}
	return double.query, nil
}

func catalogueQuery(t *testing.T) pricinghttp.CatalogueQuery {
	t.Helper()
	scope, err := domain.NewOperationsQueryScope(
		pricingValue(t, domain.NewOperationsScopeReference, "ops-scope-1"),
		pricingValue(t, domain.NewTenantID, "tenant-1"),
	)
	if err != nil {
		t.Fatalf("构造作用域:%v", err)
	}
	return pricinghttp.CatalogueQuery{Scope: scope, Limit: 50}
}

type cardReaderDouble struct {
	tenant domain.TenantID
	rows   []ports.PriceCardCatalogueRow
	err    error
}

func (double *cardReaderDouble) ListPriceCards(
	_ context.Context,
	tenant domain.TenantID,
	_ int,
) ([]ports.PriceCardCatalogueRow, error) {
	if double.err != nil {
		return nil, double.err
	}
	if tenant != double.tenant {
		return nil, nil
	}
	return double.rows, nil
}

type seriesReaderDouble struct {
	tenant domain.TenantID
	rows   []ports.ReferenceSeriesCatalogueRow
	err    error
}

func (double *seriesReaderDouble) ListReferenceSeries(
	_ context.Context,
	tenant domain.TenantID,
	_ int,
) ([]ports.ReferenceSeriesCatalogueRow, error) {
	if double.err != nil {
		return nil, double.err
	}
	if tenant != double.tenant {
		return nil, nil
	}
	return double.rows, nil
}

func decodeBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是 JSON:%v\n%s", err, recorder.Body.String())
	}
	return body
}

func errorCode(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	body := decodeBody(t, recorder)
	detail, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("响应没有 error 体:%s", recorder.Body.String())
	}
	code, _ := detail["code"].(string)
	return code
}

func TestPriceCardsEndpointOnlyAcceptsGet(t *testing.T) {
	endpoint := pricinghttp.NewQueryPriceCardsEndpoint(
		intakeDouble{query: catalogueQuery(t)},
		&cardReaderDouble{},
	)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/pricing-price-cards", nil))

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", recorder.Code)
	}
	if allow := recorder.Header().Get("Allow"); allow != http.MethodGet {
		t.Fatalf("Allow = %q", allow)
	}
}

// Covers: ADR-0055/ADR-0077 Decision 三——PAR-INT-01 未登记,生产装配的 Intake 对
// 每个请求一律 403,不读业务内容;两个端点同答。
func TestPricingEndpointsAnswerUnconfiguredIntakeWith403(t *testing.T) {
	cards := pricinghttp.NewQueryPriceCardsEndpoint(pricinghttp.UnconfiguredIntake{}, &cardReaderDouble{})
	series := pricinghttp.NewQueryReferenceSeriesEndpoint(pricinghttp.UnconfiguredIntake{}, &seriesReaderDouble{})

	for name, endpoint := range map[string]http.Handler{
		"/pricing-price-cards":      cards,
		"/pricing-reference-series": series,
	} {
		recorder := httptest.NewRecorder()
		endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, name, nil))
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("%s status = %d", name, recorder.Code)
		}
		if code := errorCode(t, recorder); code != "ACCESS_CHANNEL_NOT_CONFIGURED" {
			t.Fatalf("%s code = %q", name, code)
		}
	}
}

// Covers: 价卡行体逐字段转写;无上界适用期的 effectiveTo 键如实缺席。
func TestPriceCardsEndpointTranscribesRows(t *testing.T) {
	query := catalogueQuery(t)
	reader := &cardReaderDouble{
		tenant: query.Scope.Tenant(),
		rows: []ports.PriceCardCatalogueRow{
			{
				PlanID:               "plan-1",
				PlanVersion:          "v1",
				Direction:            "SELL",
				Purpose:              "CUSTOMER_CHARGE",
				Scope:                "scope-1",
				RateTableID:          "table-1",
				RateTableVersion:     "v1",
				EffectiveFrom:        pricingBaseAt,
				EffectiveTo:          pricingBaseAt.Add(90 * 24 * time.Hour),
				HasEffectiveTo:       true,
				Canonicalization:     "PPC-1",
				ContentDigest:        "digest-1",
				SourceFileName:       "SYN-PRC-CARD.xlsx",
				SourceFileSHA256:     "abc123",
				AuthorizationID:      "SYN-PRC-GRANT",
				AuthorizationVersion: "v1",
				PublicationApprover:  "SYN-PRC-GOVERNANCE",
				RegisteredAt:         pricingBaseAt,
			},
			{
				PlanID:        "plan-2",
				PlanVersion:   "v1",
				Direction:     "BUY",
				Purpose:       "SUPPLIER_COST",
				Scope:         "scope-1",
				EffectiveFrom: pricingBaseAt,
				RegisteredAt:  pricingBaseAt,
			},
		},
	}
	endpoint := pricinghttp.NewQueryPriceCardsEndpoint(intakeDouble{query: query}, reader)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/pricing-price-cards", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	body := decodeBody(t, recorder)
	if body["outcome"] != "PRICE_CARDS_LISTED" {
		t.Fatalf("outcome = %v", body["outcome"])
	}
	cards, ok := body["cards"].([]any)
	if !ok || len(cards) != 2 {
		t.Fatalf("cards = %v", body["cards"])
	}

	first := cards[0].(map[string]any)
	if first["planId"] != "plan-1" || first["direction"] != "SELL" ||
		first["purpose"] != "CUSTOMER_CHARGE" || first["publicationApprover"] != "SYN-PRC-GOVERNANCE" ||
		first["sourceFileSha256"] != "abc123" {
		t.Fatalf("首行变形:%v", first)
	}
	if first["effectiveFrom"] != "2026-08-25T09:00:00Z" {
		t.Fatalf("effectiveFrom = %v", first["effectiveFrom"])
	}
	if _, has := first["effectiveTo"]; !has {
		t.Fatal("有界适用期的 effectiveTo 没透出")
	}

	second := cards[1].(map[string]any)
	if _, has := second["effectiveTo"]; has {
		t.Fatal("无上界适用期长出了 effectiveTo 键")
	}
}

// Covers: 序列行体转写——口径两键与更正两键成对在场或成对缺席,证据等级照透。
func TestReferenceSeriesEndpointTranscribesRows(t *testing.T) {
	query := catalogueQuery(t)
	reader := &seriesReaderDouble{
		tenant: query.Scope.Tenant(),
		rows: []ports.ReferenceSeriesCatalogueRow{
			{
				SeriesID:          "SYN-PRC-USD-CNY",
				SeriesVersion:     "v1",
				Kind:              "EXCHANGE_RATE",
				SourceIdentifier:  "SYN-FINANCE/usd-cny-daily",
				Registrant:        "SYN-PRC-SERIES-REGISTRAR",
				QuoteBasisID:      "SYN-PRC-FX-POLICY",
				QuoteBasisVersion: "v1",
				HasQuoteBasis:     true,
				EffectiveFrom:     pricingBaseAt,
				EffectiveTo:       pricingBaseAt.Add(7 * 24 * time.Hour),
				HasEffectiveTo:    true,
				EvidenceGrade:     "VERIFIABLE",
				Canonicalization:  "PRS-1",
				ContentDigest:     "digest-fx",
				RegisteredAt:      pricingBaseAt,
			},
			{
				SeriesID:         "SYN-PRC-FUEL-WEEKLY",
				SeriesVersion:    "v2",
				Kind:             "FUEL_RATE",
				SourceIdentifier: "SYN-CARRIER/fuel-weekly-bulletin",
				Registrant:       "SYN-PRC-SERIES-REGISTRAR",
				EffectiveFrom:    pricingBaseAt,
				EvidenceGrade:    "ASSERTED",
				PriorVersion:     "v1",
				CorrectionBasis:  "SYN-CORRECTION/fuel-w32",
				IsCorrection:     true,
				Canonicalization: "PRS-1",
				ContentDigest:    "digest-fuel",
				RegisteredAt:     pricingBaseAt,
			},
		},
	}
	endpoint := pricinghttp.NewQueryReferenceSeriesEndpoint(intakeDouble{query: query}, reader)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/pricing-reference-series", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	body := decodeBody(t, recorder)
	if body["outcome"] != "REFERENCE_SERIES_LISTED" {
		t.Fatalf("outcome = %v", body["outcome"])
	}
	series, ok := body["series"].([]any)
	if !ok || len(series) != 2 {
		t.Fatalf("series = %v", body["series"])
	}

	fx := series[0].(map[string]any)
	if fx["kind"] != "EXCHANGE_RATE" || fx["evidenceGrade"] != "VERIFIABLE" ||
		fx["quoteBasisId"] != "SYN-PRC-FX-POLICY" || fx["quoteBasisVersion"] != "v1" {
		t.Fatalf("汇率行变形:%v", fx)
	}
	if _, has := fx["priorVersion"]; has {
		t.Fatal("非更正版长出了 priorVersion 键")
	}

	fuel := series[1].(map[string]any)
	if fuel["evidenceGrade"] != "ASSERTED" ||
		fuel["priorVersion"] != "v1" || fuel["correctionBasis"] != "SYN-CORRECTION/fuel-w32" {
		t.Fatalf("更正行变形:%v", fuel)
	}
	if _, has := fuel["quoteBasisId"]; has {
		t.Fatal("燃油行长出了口径键")
	}
	if _, has := fuel["effectiveTo"]; has {
		t.Fatal("无上界包络长出了 effectiveTo 键")
	}
}

// Covers: 票 pricing-reference-series-operations/08 件①——行体多出复核事实、引用 digest 与期次:
// 复核两计数总在场(无复核是 0 不是缺键),最近一次复核两键成对在场或成对缺席;期次逐期
// 转写,止点与凭证按在场与否给键;**行体上没有任何「在用」键**。
func TestReferenceSeriesEndpointTranscribesReviewFactsAndPeriods(t *testing.T) {
	query := catalogueQuery(t)
	reader := &seriesReaderDouble{
		tenant: query.Scope.Tenant(),
		rows: []ports.ReferenceSeriesCatalogueRow{
			{
				SeriesID:            "SYN-PRC-FUEL-WEEKLY",
				SeriesVersion:       "v1",
				Kind:                "FUEL_RATE",
				SourceIdentifier:    "SYN-CARRIER/fuel-weekly-bulletin",
				Registrant:          "SYN-PRC-SERIES-REGISTRAR",
				EffectiveFrom:       pricingBaseAt,
				EvidenceGrade:       "ASSERTED",
				Canonicalization:    "PRS-1",
				ContentDigest:       "digest-fuel",
				ReferenceDigest:     "sha256:syn-SYN-PRC-FUEL-WEEKLY-v1",
				RegisteredAt:        pricingBaseAt,
				ReviewCount:         2,
				ApprovedReviewCount: 1,
				LastReviewedAt:      pricingBaseAt.Add(48 * time.Hour),
				LastReviewDecision:  "APPROVED",
				HasReview:           true,
				Periods: []ports.ReferenceSeriesPeriodRow{
					{StartsAt: pricingBaseAt, EndsAt: pricingBaseAt.Add(7 * 24 * time.Hour), HasEndsAt: true,
						Value: "0.22", EvidenceRef: "SYN-EVIDENCE/fuel-2026-W32", HasEvidence: true},
					{StartsAt: pricingBaseAt.Add(7 * 24 * time.Hour), Value: "0.24"},
				},
			},
			{
				SeriesID:         "SYN-PRC-FUEL-WEEKLY",
				SeriesVersion:    "v2",
				Kind:             "FUEL_RATE",
				SourceIdentifier: "SYN-CARRIER/fuel-weekly-bulletin",
				Registrant:       "SYN-PRC-SERIES-REGISTRAR",
				EffectiveFrom:    pricingBaseAt,
				EvidenceGrade:    "VERIFIABLE",
				Canonicalization: "PRS-1",
				ContentDigest:    "digest-fuel-2",
				ReferenceDigest:  "sha256:syn-SYN-PRC-FUEL-WEEKLY-v2",
				RegisteredAt:     pricingBaseAt,
				Periods: []ports.ReferenceSeriesPeriodRow{
					{StartsAt: pricingBaseAt, Value: "0.23", EvidenceRef: "SYN-EVIDENCE/fuel-2026-W32-reissued", HasEvidence: true},
				},
			},
		},
	}
	endpoint := pricinghttp.NewQueryReferenceSeriesEndpoint(intakeDouble{query: query}, reader)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/pricing-reference-series", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	series := decodeBody(t, recorder)["series"].([]any)

	reviewed := series[0].(map[string]any)
	if reviewed["referenceDigest"] != "sha256:syn-SYN-PRC-FUEL-WEEKLY-v1" || reviewed["contentDigest"] != "digest-fuel" {
		t.Fatalf("两种摘要没分开透出:%v", reviewed)
	}
	if reviewed["reviewCount"] != float64(2) || reviewed["approvedReviewCount"] != float64(1) {
		t.Fatalf("复核计数变形:%v", reviewed)
	}
	if reviewed["lastReviewedAt"] != pricingBaseAt.Add(48*time.Hour).UTC().Format(time.RFC3339Nano) ||
		reviewed["lastReviewDecision"] != "APPROVED" {
		t.Fatalf("最近一次复核变形:%v", reviewed)
	}
	for _, forbidden := range []string{"inForce", "inForceVersion", "status"} {
		if _, has := reviewed[forbidden]; has {
			t.Fatalf("目录行体长出了 %s 键——在用归覆盖读口,目录页没有时刻源", forbidden)
		}
	}
	periods, ok := reviewed["periods"].([]any)
	if !ok || len(periods) != 2 {
		t.Fatalf("periods = %v", reviewed["periods"])
	}
	head := periods[0].(map[string]any)
	if head["startsAt"] != pricingBaseAt.UTC().Format(time.RFC3339Nano) ||
		head["endsAt"] != pricingBaseAt.Add(7*24*time.Hour).UTC().Format(time.RFC3339Nano) ||
		head["value"] != "0.22" || head["evidenceRef"] != "SYN-EVIDENCE/fuel-2026-W32" {
		t.Fatalf("首期变形:%v", head)
	}
	tail := periods[1].(map[string]any)
	if tail["value"] != "0.24" {
		t.Fatalf("末期变形:%v", tail)
	}
	if _, has := tail["endsAt"]; has {
		t.Fatal("无上界期次长出了 endsAt 键")
	}
	if _, has := tail["evidenceRef"]; has {
		t.Fatal("缺凭证期次长出了 evidenceRef 键")
	}

	unreviewed := series[1].(map[string]any)
	if unreviewed["reviewCount"] != float64(0) || unreviewed["approvedReviewCount"] != float64(0) {
		t.Fatalf("无复核版本的计数应为 0 且在场:%v", unreviewed)
	}
	if _, has := unreviewed["lastReviewedAt"]; has {
		t.Fatal("无复核版本长出了 lastReviewedAt 键")
	}
	if _, has := unreviewed["lastReviewDecision"]; has {
		t.Fatal("无复核版本长出了 lastReviewDecision 键")
	}
}

// Covers: ADR-0077 Decision 四——空目录走 2xx 成格、空数组不是 null,也不折成未配置。
func TestPricingEndpointsAnswerAnEmptyCatalogueWithAnEmptyArray(t *testing.T) {
	query := catalogueQuery(t)
	cards := pricinghttp.NewQueryPriceCardsEndpoint(
		intakeDouble{query: query}, &cardReaderDouble{tenant: query.Scope.Tenant()})
	series := pricinghttp.NewQueryReferenceSeriesEndpoint(
		intakeDouble{query: query}, &seriesReaderDouble{tenant: query.Scope.Tenant()})

	recorder := httptest.NewRecorder()
	cards.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/pricing-price-cards", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"cards":[]`) {
		t.Fatalf("空价卡目录不是空数组:%d %s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	series.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/pricing-reference-series", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"series":[]`) {
		t.Fatalf("空序列册不是空数组:%d %s", recorder.Code, recorder.Body.String())
	}
}

func TestPricingEndpointsAnswerReadFailureWith500(t *testing.T) {
	query := catalogueQuery(t)
	cards := pricinghttp.NewQueryPriceCardsEndpoint(
		intakeDouble{query: query}, &cardReaderDouble{err: errors.New("库不可达")})
	series := pricinghttp.NewQueryReferenceSeriesEndpoint(
		intakeDouble{query: query}, &seriesReaderDouble{err: errors.New("库不可达")})

	for name, endpoint := range map[string]http.Handler{
		"/pricing-price-cards":      cards,
		"/pricing-reference-series": series,
	} {
		recorder := httptest.NewRecorder()
		endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, name, nil))
		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("%s status = %d", name, recorder.Code)
		}
		if code := errorCode(t, recorder); code != "NO_ANSWER_FORMED" {
			t.Fatalf("%s code = %q", name, code)
		}
	}
}

// Covers: Intake 交回 ErrMalformedRequest 时按坏请求答——重发同样内容不会变好,
// 与渠道未配置(403)和翻译故障(5xx)分格。
func TestPriceCardsEndpointAnswersMalformedIntakeWith400(t *testing.T) {
	endpoint := pricinghttp.NewQueryPriceCardsEndpoint(
		intakeDouble{err: pricinghttp.ErrMalformedRequest},
		&cardReaderDouble{},
	)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/pricing-price-cards", nil))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", recorder.Code)
	}
	if code := errorCode(t, recorder); code != "MALFORMED_REQUEST" {
		t.Fatalf("code = %q", code)
	}
}
