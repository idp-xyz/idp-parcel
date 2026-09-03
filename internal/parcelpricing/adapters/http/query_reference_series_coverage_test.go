package pricinghttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// 本文件对覆盖地平线端点（票 pricing-reference-series-operations/05 切片 05a）证传输面：
// 方法门、未配置 Intake 一律 403、asOf 如实回显所用时刻、以及那一处最要紧的分格——
// 「没有在用版本」与「在用但没有终点」不得挤成同一个可观察形状。

var coverageAt = time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)

type coverageClockDouble struct{ at time.Time }

func (clock coverageClockDouble) Now() time.Time { return clock.at }

type coverageReaderDouble struct {
	tenant domain.TenantID
	at     time.Time
	rows   []ports.ReferenceSeriesCoverageRow
	err    error
}

func (double *coverageReaderDouble) ListReferenceSeriesCoverage(
	_ context.Context,
	tenant domain.TenantID,
	at time.Time,
	_ int,
) ([]ports.ReferenceSeriesCoverageRow, error) {
	if double.err != nil {
		return nil, double.err
	}
	double.tenant = tenant
	double.at = at
	return double.rows, nil
}

func coverageEndpoint(rows []ports.ReferenceSeriesCoverageRow) (http.Handler, *coverageReaderDouble) {
	reader := &coverageReaderDouble{rows: rows}
	endpoint := pricinghttp.NewQueryReferenceSeriesCoverageEndpoint(
		intakeDouble{query: pricinghttp.CatalogueQuery{}},
		reader,
		coverageClockDouble{at: coverageAt},
	)
	return endpoint, reader
}

func coverageBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("解响应体：%v（%s）", err, recorder.Body.String())
	}
	return body
}

func coverageRows(t *testing.T, recorder *httptest.ResponseRecorder) []any {
	t.Helper()
	series, ok := coverageBody(t, recorder)["series"].([]any)
	if !ok {
		t.Fatalf("series 不是数组：%s", recorder.Body.String())
	}
	return series
}

// Covers: 方法门——本端点只收 GET，其余答 405 并带 Allow。
func TestCoverageEndpointTakesOnlyGET(t *testing.T) {
	endpoint, _ := coverageEndpoint(nil)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/pricing-reference-series-coverage", nil))

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", recorder.Code)
	}
	if allow := recorder.Header().Get("Allow"); allow != http.MethodGet {
		t.Fatalf("Allow = %q", allow)
	}
}

// Covers: ADR-0055/ADR-0077——PAR-INT-01 未登记时生产装配的 Intake 一律 403，不读业务内容。
func TestCoverageEndpointAnswersUnconfiguredIntakeWith403(t *testing.T) {
	endpoint := pricinghttp.NewQueryReferenceSeriesCoverageEndpoint(
		pricinghttp.UnconfiguredIntake{},
		&coverageReaderDouble{},
		coverageClockDouble{at: coverageAt},
	)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/pricing-reference-series-coverage", nil))

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d", recorder.Code)
	}
	if code := errorCode(t, recorder); code != "ACCESS_CHANNEL_NOT_CONFIGURED" {
		t.Fatalf("code = %q", code)
	}
}

// Covers: 端点把时钟给的那一刻同时交给读口与响应体。**回显不是装饰**：「在用」只对某一刻
// 成立，不说出那一刻，页面上就会出现一个看起来永久的权威答案。
func TestCoverageEndpointEchoesTheMomentItResolvedAt(t *testing.T) {
	endpoint, reader := coverageEndpoint(nil)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/pricing-reference-series-coverage", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	if !reader.at.Equal(coverageAt) {
		t.Fatalf("交给读口的时刻 = %v，想要 %v", reader.at, coverageAt)
	}
	if asOf, _ := coverageBody(t, recorder)["asOf"].(string); asOf != "2026-08-25T09:00:00Z" {
		t.Fatalf("asOf = %q", asOf)
	}
	if len(coverageRows(t, recorder)) != 0 {
		t.Fatal("空册应答空数组，不是 null 也不是缺席")
	}
}

// Covers: **本端点最要紧的一处分格。** 「这条序列没有在用版本」与「有在用版本但它没有
// 终点」都会让止点缺席，而两者一个是「今天没得用」、一个是「用着且不会到期」，续办动作
// 相反。若把止点平铺成一个可缺席的键，两者在响应体上一模一样。
func TestCoverageEndpointTellsNoInForceApartFromOpenEnded(t *testing.T) {
	from := coverageAt.Add(-72 * time.Hour)
	to := coverageAt.Add(72 * time.Hour)
	endpoint, _ := coverageEndpoint([]ports.ReferenceSeriesCoverageRow{
		{
			SeriesID: "SYN-NONE", Kind: "FUEL_RATE",
			RegisteredVersionCount: 2, UnreviewedVersionCount: 2,
		},
		{
			SeriesID: "SYN-OPEN", Kind: "FUEL_RATE", RegisteredVersionCount: 1,
			InForceVersion: "v1", HasInForceVersion: true, InForceEffectiveFrom: from,
		},
		{
			SeriesID: "SYN-BOUNDED", Kind: "EXCHANGE_RATE", RegisteredVersionCount: 1,
			InForceVersion: "v9", HasInForceVersion: true, InForceEffectiveFrom: from,
			InForceEffectiveTo: to, HasInForceEffectiveTo: true,
		},
	})
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/pricing-reference-series-coverage", nil))

	rows := coverageRows(t, recorder)
	if len(rows) != 3 {
		t.Fatalf("行数 = %d", len(rows))
	}

	none := rows[0].(map[string]any)
	if none["inForceResolved"] != false {
		t.Fatalf("无在用那行 inForceResolved = %v", none["inForceResolved"])
	}
	if _, present := none["inForce"]; present {
		t.Fatalf("无在用却长出了 inForce 节：%v", none)
	}

	open := rows[1].(map[string]any)
	if open["inForceResolved"] != true {
		t.Fatalf("无上界那行 inForceResolved = %v", open["inForceResolved"])
	}
	openInForce, ok := open["inForce"].(map[string]any)
	if !ok {
		t.Fatalf("无上界那行缺 inForce 节：%v", open)
	}
	if openInForce["openEnded"] != true {
		t.Fatalf("openEnded = %v，想要 true", openInForce["openEnded"])
	}
	if _, present := openInForce["effectiveTo"]; present {
		t.Fatalf("没有终点却报了 effectiveTo：%v", openInForce)
	}

	bounded := rows[2].(map[string]any)
	boundedInForce := bounded["inForce"].(map[string]any)
	if boundedInForce["openEnded"] != false {
		t.Fatalf("有上界那行 openEnded = %v，想要 false", boundedInForce["openEnded"])
	}
	if boundedInForce["effectiveTo"] != "2026-08-28T09:00:00Z" {
		t.Fatalf("effectiveTo = %v", boundedInForce["effectiveTo"])
	}
	if boundedInForce["version"] != "v9" {
		t.Fatalf("version = %v", boundedInForce["version"])
	}
}

// Covers: 两笔欠账逐字透出且各自成键——等复核人与等登记方更正是两种续办动作，页面要能
// 分开显示；最近一次复核连结论一起给，退回也算「有人看过」。
func TestCoverageEndpointTranscribesBothDebtsAndTheLastReview(t *testing.T) {
	reviewedAt := coverageAt.Add(-24 * time.Hour)
	endpoint, _ := coverageEndpoint([]ports.ReferenceSeriesCoverageRow{{
		SeriesID: "SYN-DEBTS", Kind: "FUEL_RATE", RegisteredVersionCount: 3,
		UnreviewedVersionCount: 1, ReturnedVersionCount: 1,
		LastReviewedAt: reviewedAt, LastReviewDecision: "RETURNED", HasReview: true,
	}})
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/pricing-reference-series-coverage", nil))

	row := coverageRows(t, recorder)[0].(map[string]any)
	if row["unreviewedVersionCount"] != float64(1) || row["returnedVersionCount"] != float64(1) {
		t.Fatalf("两笔欠账 = %v / %v", row["unreviewedVersionCount"], row["returnedVersionCount"])
	}
	if row["registeredVersionCount"] != float64(3) {
		t.Fatalf("登记版本数 = %v", row["registeredVersionCount"])
	}
	if row["lastReviewDecision"] != "RETURNED" || row["lastReviewedAt"] != "2026-08-24T09:00:00Z" {
		t.Fatalf("最近一次复核 = %v / %v", row["lastReviewDecision"], row["lastReviewedAt"])
	}
}

// Covers: 一条复核都没有的序列，两个复核键如实缺席——不补空串。空串会被页面当成
// 「复核过、结论是空」，而那不是登记册说的话。
func TestCoverageEndpointOmitsTheReviewKeysWhenNobodyHasReviewed(t *testing.T) {
	endpoint, _ := coverageEndpoint([]ports.ReferenceSeriesCoverageRow{{
		SeriesID: "SYN-FRESH", Kind: "FUEL_RATE", RegisteredVersionCount: 1, UnreviewedVersionCount: 1,
	}})
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/pricing-reference-series-coverage", nil))

	row := coverageRows(t, recorder)[0].(map[string]any)
	if _, present := row["lastReviewedAt"]; present {
		t.Fatalf("没人复核过却报了时刻：%v", row)
	}
	if _, present := row["lastReviewDecision"]; present {
		t.Fatalf("没人复核过却报了结论：%v", row)
	}
}

// Covers: 读失败答 5xx 的「没形成答案」，与渠道未配置（403）分格——两者续办动作不同。
func TestCoverageEndpointAnswersAReadFailureWithNoAnswerFormed(t *testing.T) {
	endpoint := pricinghttp.NewQueryReferenceSeriesCoverageEndpoint(
		intakeDouble{query: pricinghttp.CatalogueQuery{}},
		&coverageReaderDouble{err: errors.New("库连不上")},
		coverageClockDouble{at: coverageAt},
	)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/pricing-reference-series-coverage", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", recorder.Code)
	}
	if code := errorCode(t, recorder); code != "NO_ANSWER_FORMED" {
		t.Fatalf("code = %q", code)
	}
}
