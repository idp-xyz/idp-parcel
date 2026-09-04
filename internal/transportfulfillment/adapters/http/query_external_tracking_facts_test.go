package tfhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证外部承运轨迹事实查阅端点（GET /transport-fulfillment-external-tracking-facts，label-channel/21）
// 的传输层纪律：两格视图各有自己的 outcome 词、行照实转写（待判断不带有效时间、判断版本带依据与前版）、
// 源与视图缺席或集外 400 且不触读口、405/403/500 分法、未配置 Intake 在读口之前。

type stubFactReview struct {
	rows []ports.ExternalTrackingFactReviewRow
	err  error

	gotTenant string
	gotSource string
	gotFilter ports.EffectiveTimeReviewFilter
	gotLimit  int
	calls     int
}

func (stub *stubFactReview) ListCurrentExternalTrackingFacts(
	_ context.Context,
	tenant domain.TenantID,
	source domain.TrackingSourceReference,
	filter ports.EffectiveTimeReviewFilter,
	limit int,
) ([]ports.ExternalTrackingFactReviewRow, error) {
	stub.calls++
	stub.gotTenant, stub.gotSource, stub.gotFilter, stub.gotLimit = tenant.String(), source.String(), filter, limit
	return stub.rows, stub.err
}

var factReviewOccurredAt = time.Date(2026, 9, 4, 6, 0, 0, 0, time.UTC)

func pendingReviewRow() ports.ExternalTrackingFactReviewRow {
	return ports.ExternalTrackingFactReviewRow{
		Fact:           "EXTF-1",
		Version:        "EXTV-1",
		Source:         "aggregator-a",
		Credential:     "carrier-x/1Z999",
		Object:         "PCL-1",
		SourceEvent:    "evt-1",
		Status:         "IN_TRANSIT",
		OccurredAt:     factReviewOccurredAt,
		ReceivedAt:     factReviewOccurredAt.Add(90 * time.Second),
		EffectiveBasis: "PENDING",
		Origin:         "MATERIAL",
		RecordedAt:     factReviewOccurredAt.Add(91 * time.Second),
	}
}

func judgedReviewRow() ports.ExternalTrackingFactReviewRow {
	effectiveAt := factReviewOccurredAt.Add(10 * time.Minute)
	row := pendingReviewRow()
	row.Fact, row.Version = "EXTF-2", "EXTV-2b"
	row.EffectiveBasis = "JUDGED_BY_RULE"
	row.EffectiveAt = &effectiveAt
	row.EffectiveRule, row.EffectiveRuleVersion = "aggregator-a", "ETR-1"
	row.Supersedes = "EXTV-2a"
	row.Origin = "JUDGMENT"
	return row
}

type factReviewView struct {
	Outcome string `json:"outcome"`
	Facts   []struct {
		Fact                 string  `json:"fact"`
		Version              string  `json:"version"`
		Source               string  `json:"source"`
		Credential           string  `json:"credential"`
		Object               string  `json:"object"`
		SourceEvent          *string `json:"sourceEvent"`
		Status               string  `json:"status"`
		OccurredAt           string  `json:"occurredAt"`
		ReceivedAt           string  `json:"receivedAt"`
		EffectiveBasis       string  `json:"effectiveBasis"`
		EffectiveAt          *string `json:"effectiveAt"`
		EffectiveRule        *string `json:"effectiveRule"`
		EffectiveRuleVersion *string `json:"effectiveRuleVersion"`
		Supersedes           *string `json:"supersedes"`
		Origin               string  `json:"origin"`
		RecordedAt           string  `json:"recordedAt"`
	} `json:"facts"`
}

func getFacts(t *testing.T, handler http.Handler, target string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
	return recorder
}

func decodeFactReview(t *testing.T, recorder *httptest.ResponseRecorder) factReviewView {
	t.Helper()
	var view factReviewView
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode: %v body=%s", err, recorder.Body.String())
	}
	return view
}

// Covers: 待判断视图——outcome 词、读口收到（租户，源，待判断过滤，页大小）、行照实转写且不带有效时间、
// 规则、前版三键（缺席是真话：待判断就是没有）。
func TestPendingViewListsFactsWithoutAnyEffectiveTime(t *testing.T) {
	reader := &stubFactReview{rows: []ports.ExternalTrackingFactReviewRow{pendingReviewRow()}}
	endpoint := tfhttp.NewQueryExternalTrackingFactsEndpoint(grantedIntake(), reader)

	response := getFacts(t, endpoint, "/x?source=aggregator-a&view=pending")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	if reader.gotTenant != "TENANT-1" || reader.gotSource != "aggregator-a" ||
		reader.gotFilter != ports.PendingEffectiveTimeOnly || reader.gotLimit != 25 {
		t.Fatalf("读口收到的不是接入面给的作用域与请求指名的源：%+v", reader)
	}
	view := decodeFactReview(t, response)
	if view.Outcome != "PENDING_EFFECTIVE_TIME_FACTS_LISTED" || len(view.Facts) != 1 {
		t.Fatalf("view = %+v", view)
	}
	fact := view.Facts[0]
	if fact.Fact != "EXTF-1" || fact.Version != "EXTV-1" || fact.Source != "aggregator-a" ||
		fact.Credential != "carrier-x/1Z999" || fact.Object != "PCL-1" || fact.Status != "IN_TRANSIT" ||
		fact.SourceEvent == nil || *fact.SourceEvent != "evt-1" || fact.Origin != "MATERIAL" {
		t.Fatalf("身份与源给的内容没有原样透出：%+v", fact)
	}
	if fact.OccurredAt != "2026-09-04T06:00:00Z" || fact.ReceivedAt != "2026-09-04T06:01:30Z" ||
		fact.RecordedAt != "2026-09-04T06:01:31Z" {
		t.Fatalf("三个时刻没有按 RFC 3339 UTC 透出：%+v", fact)
	}
	if fact.EffectiveBasis != "PENDING" || fact.EffectiveAt != nil || fact.EffectiveRule != nil ||
		fact.EffectiveRuleVersion != nil || fact.Supersedes != nil {
		t.Fatalf("待判断行带了有效时间、规则或前版：%+v", fact)
	}
}

// Covers: 全部当前版视图——outcome 词区分、判断版本带依据、有效时间、规则版本与前版（票 21 红线：再判前
// 要看得见「已经判过、按哪个依据判的」）；源未给事件标识的行 sourceEvent 整键缺席。
func TestCurrentViewCarriesTheJudgmentBasisAndThePriorVersion(t *testing.T) {
	noEvent := pendingReviewRow()
	noEvent.Fact, noEvent.Version, noEvent.SourceEvent = "EXTF-3", "EXTV-3", ""
	reader := &stubFactReview{rows: []ports.ExternalTrackingFactReviewRow{judgedReviewRow(), noEvent}}
	endpoint := tfhttp.NewQueryExternalTrackingFactsEndpoint(grantedIntake(), reader)

	response := getFacts(t, endpoint, "/x?source=aggregator-a&view=current")

	if response.Code != http.StatusOK || reader.gotFilter != ports.EveryCurrentVersion {
		t.Fatalf("status = %d filter = %v body = %s", response.Code, reader.gotFilter, response.Body.String())
	}
	view := decodeFactReview(t, response)
	if view.Outcome != "CURRENT_EXTERNAL_TRACKING_FACTS_LISTED" || len(view.Facts) != 2 {
		t.Fatalf("view = %+v", view)
	}
	judged := view.Facts[0]
	if judged.EffectiveBasis != "JUDGED_BY_RULE" || judged.EffectiveAt == nil || *judged.EffectiveAt != "2026-09-04T06:10:00Z" ||
		judged.EffectiveRule == nil || *judged.EffectiveRule != "aggregator-a" ||
		judged.EffectiveRuleVersion == nil || *judged.EffectiveRuleVersion != "ETR-1" ||
		judged.Supersedes == nil || *judged.Supersedes != "EXTV-2a" || judged.Origin != "JUDGMENT" {
		t.Fatalf("判断版本的依据、规则版本与前版没有透出：%+v", judged)
	}
	if view.Facts[1].SourceEvent != nil {
		t.Fatalf("源未给事件标识时 sourceEvent 键应整键缺席：%+v", view.Facts[1])
	}
}

// Covers: 源与视图是请求形状的事，缺席或集外 400 且不触读口；判它不需要先知道调用方是谁。
func TestFactQueryRefusesMissingSourceOrUnknownViewBeforeReadingAnything(t *testing.T) {
	for name, target := range map[string]string{
		"source missing":  "/x?view=pending",
		"source blank":    "/x?source=%20&view=pending",
		"view missing":    "/x?source=aggregator-a",
		"view outside":    "/x?source=aggregator-a&view=all",
		"view mixed case": "/x?source=aggregator-a&view=Pending",
	} {
		t.Run(name, func(t *testing.T) {
			reader := &stubFactReview{}
			endpoint := tfhttp.NewQueryExternalTrackingFactsEndpoint(failingCatalogueIntake{err: errors.New("must not be asked")}, reader)
			response := getFacts(t, endpoint, target)
			if response.Code != http.StatusBadRequest || problemCode(t, response) != "MALFORMED_REQUEST" {
				t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
			}
			if reader.calls != 0 {
				t.Fatal("坏请求触到了读口")
			}
		})
	}
}

func TestFactQueryTransportProblemsCarryOnlyStableCodes(t *testing.T) {
	t.Run("method not allowed", func(t *testing.T) {
		endpoint := tfhttp.NewQueryExternalTrackingFactsEndpoint(grantedIntake(), &stubFactReview{})
		recorder := httptest.NewRecorder()
		endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/x?source=a&view=pending", nil))
		if recorder.Code != http.StatusMethodNotAllowed || recorder.Header().Get("Allow") != http.MethodGet {
			t.Fatalf("status = %d allow = %q", recorder.Code, recorder.Header().Get("Allow"))
		}
	})
	t.Run("unconfigured intake refuses before the reader", func(t *testing.T) {
		reader := &stubFactReview{}
		endpoint := tfhttp.NewQueryExternalTrackingFactsEndpoint(tfhttp.UnconfiguredIntake{}, reader)
		response := getFacts(t, endpoint, "/x?source=aggregator-a&view=pending")
		if response.Code != http.StatusForbidden || problemCode(t, response) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
			t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
		}
		assertNoOutcome(t, response)
		if reader.calls != 0 {
			t.Fatal("未配置 Intake 之后读口仍被触到")
		}
	})
	t.Run("an intake infrastructure failure is 500 INTAKE_FAILED", func(t *testing.T) {
		endpoint := tfhttp.NewQueryExternalTrackingFactsEndpoint(failingCatalogueIntake{err: errors.New("session store down")}, &stubFactReview{})
		response := getFacts(t, endpoint, "/x?source=aggregator-a&view=pending")
		if response.Code != http.StatusInternalServerError || problemCode(t, response) != "INTAKE_FAILED" {
			t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
		}
	})
	t.Run("a reader failure is 500 NO_ANSWER_FORMED without an outcome", func(t *testing.T) {
		endpoint := tfhttp.NewQueryExternalTrackingFactsEndpoint(grantedIntake(), &stubFactReview{err: errors.New("db down")})
		response := getFacts(t, endpoint, "/x?source=aggregator-a&view=current")
		if response.Code != http.StatusInternalServerError || problemCode(t, response) != "NO_ANSWER_FORMED" {
			t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
		}
		assertNoOutcome(t, response)
	})
	t.Run("an empty source answers an empty list, not an error", func(t *testing.T) {
		endpoint := tfhttp.NewQueryExternalTrackingFactsEndpoint(grantedIntake(), &stubFactReview{})
		response := getFacts(t, endpoint, "/x?source=aggregator-a&view=pending")
		view := decodeFactReview(t, response)
		if response.Code != http.StatusOK || view.Facts == nil || len(view.Facts) != 0 {
			t.Fatalf("空册应是 200 与空数组（不是 null）：%d %s", response.Code, response.Body.String())
		}
	})
}
