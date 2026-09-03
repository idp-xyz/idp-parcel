package networkhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	networkhttp "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/http"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// 本文件对路由判断两册端点（票 admin-skeleton-closure-batch/03）证传输面，判据与
// 本包网络目录端点同源：方法门、register 封闭两册缺席按坏请求拒且不触读口、未配置
// Intake 403、行体逐字段转写且可缺席组如实缺席、空册答空数组、读失败答 5xx。
// Intake 替身复用 query_network_catalog_test.go 的既有件。

type stubRoutePlanReader struct {
	judgments     []ports.InitialRouteCatalogueRow
	reassessments []ports.RouteReassessmentCatalogueRow
	err           error

	gotTenant string
	gotLimit  int
}

func (stub *stubRoutePlanReader) ListInitialRoutes(
	_ context.Context, tenant domain.TenantID, limit int,
) ([]ports.InitialRouteCatalogueRow, error) {
	stub.gotTenant, stub.gotLimit = tenant.String(), limit
	return stub.judgments, stub.err
}

func (stub *stubRoutePlanReader) ListRouteReassessments(
	_ context.Context, tenant domain.TenantID, limit int,
) ([]ports.RouteReassessmentCatalogueRow, error) {
	stub.gotTenant, stub.gotLimit = tenant.String(), limit
	return stub.reassessments, stub.err
}

// unreachableRoutePlanReader 断言读口未被触到：传输形状的拒绝与未配置格都发生在
// 读库之前。
type unreachableRoutePlanReader struct{ t *testing.T }

func (reader unreachableRoutePlanReader) ListInitialRoutes(
	_ context.Context, _ domain.TenantID, _ int,
) ([]ports.InitialRouteCatalogueRow, error) {
	reader.t.Fatal("读口不该被触到")
	return nil, nil
}

func (reader unreachableRoutePlanReader) ListRouteReassessments(
	_ context.Context, _ domain.TenantID, _ int,
) ([]ports.RouteReassessmentCatalogueRow, error) {
	reader.t.Fatal("读口不该被触到")
	return nil, nil
}

func routePlanBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是 JSON：%v\n%s", err, recorder.Body.String())
	}
	return body
}

func TestRoutePlansEndpointRejectsNonGetAndUnknownRegister(t *testing.T) {
	endpoint := networkhttp.NewQueryRoutePlansEndpoint(
		grantedCatalogueIntake{tenant: "tenant-1", limit: 50},
		unreachableRoutePlanReader{t: t},
	)

	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/route-plans?register=initial-route", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST 答 %d, want 405", recorder.Code)
	}

	for _, target := range []string{"/route-plans", "/route-plans?register=plans"} {
		recorder := httptest.NewRecorder()
		endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s 答 %d, want 400（缺席按坏请求拒：替调用方默认一册就是替它猜）",
				target, recorder.Code)
		}
	}
}

func TestRoutePlansEndpointAnswersForbiddenWhenIntakeUnconfigured(t *testing.T) {
	endpoint := networkhttp.NewQueryRoutePlansEndpoint(
		networkhttp.UnconfiguredIntake{},
		unreachableRoutePlanReader{t: t},
	)
	for _, register := range []string{"initial-route", "reassessment"} {
		recorder := httptest.NewRecorder()
		endpoint.ServeHTTP(recorder,
			httptest.NewRequest(http.MethodGet, "/route-plans?register="+register, nil))
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("未配置 Intake 对 %s 答 %d, want 403", register, recorder.Code)
		}
	}
}

func TestRoutePlansEndpointTranscribesInitialRoutesWithApplicability(t *testing.T) {
	reader := &stubRoutePlanReader{judgments: []ports.InitialRouteCatalogueRow{
		{
			CustomerAccountID:      "SYN-ACC-1",
			ShipmentRequestID:      "SYN-REQ-1",
			AcceptanceBaseline:     "SYN-BASELINE-1",
			DeclaredParcelID:       "SYN-PARCEL-1",
			ServicePurpose:         "DELIVERY",
			Conclusion:             "ROUTE_FORMED",
			PlanVersion:            "SYN-PLAN-1#v1",
			HasPlanVersion:         true,
			HasApplicability:       true,
			ApplicabilityState:     "SUPERSEDED",
			ApplicabilityBasis:     "SYN-REROUTE/network-lapse",
			HasApplicabilityBasis:  true,
			ApplicabilitySuccessor: "SYN-PLAN-1#v2",
			HasSuccessor:           true,
			ApplicabilityChangedAt: endpointBaseAt.Add(time.Hour),
			RecordedAt:             endpointBaseAt,
		},
		{
			CustomerAccountID:  "SYN-ACC-1",
			ShipmentRequestID:  "SYN-REQ-2",
			AcceptanceBaseline: "SYN-BASELINE-1",
			DeclaredParcelID:   "SYN-PARCEL-2",
			ServicePurpose:     "DELIVERY",
			Conclusion:         "NO_CURRENT_ROUTE",
			RecordedAt:         endpointBaseAt,
		},
	}}
	endpoint := networkhttp.NewQueryRoutePlansEndpoint(
		grantedCatalogueIntake{tenant: "tenant-1", limit: 25}, reader)

	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/route-plans?register=initial-route", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("上列答 %d, want 200\n%s", recorder.Code, recorder.Body.String())
	}
	if reader.gotTenant != "tenant-1" || reader.gotLimit != 25 {
		t.Fatalf("读口收到 tenant=%q limit=%d", reader.gotTenant, reader.gotLimit)
	}
	body := routePlanBody(t, recorder)
	if body["outcome"] != "INITIAL_ROUTES_LISTED" {
		t.Fatalf("outcome = %v", body["outcome"])
	}
	judgments, ok := body["judgments"].([]any)
	if !ok || len(judgments) != 2 {
		t.Fatalf("judgments 形状变形：%v", body["judgments"])
	}

	formed, _ := judgments[0].(map[string]any)
	if formed["declaredParcelId"] != "SYN-PARCEL-1" || formed["conclusion"] != "ROUTE_FORMED" ||
		formed["planVersion"] != "SYN-PLAN-1#v1" || formed["servicePurpose"] != "DELIVERY" {
		t.Fatalf("成计划行转写变形：%v", formed)
	}
	applicability, ok := formed["applicability"].(map[string]any)
	if !ok || applicability["state"] != "SUPERSEDED" ||
		applicability["basis"] != "SYN-REROUTE/network-lapse" ||
		applicability["successor"] != "SYN-PLAN-1#v2" {
		t.Fatalf("适用性组转写变形：%v", formed["applicability"])
	}

	noRoute, _ := judgments[1].(map[string]any)
	if noRoute["conclusion"] != "NO_CURRENT_ROUTE" {
		t.Fatalf("无路可走行转写变形：%v", noRoute)
	}
	if _, present := noRoute["planVersion"]; present {
		t.Fatalf("无路可走行长出了计划版本：%v", noRoute)
	}
	if _, present := noRoute["applicability"]; present {
		t.Fatalf("无路可走行长出了适用性：%v", noRoute)
	}

	// 上面逐键断言过的这份响应体原样钉成契约夹具：两行一成计划带适用性、一无路可走，
	// 可缺席键因此各出场一次，前端对着它校类型时每一格都有实例可核。
	assertContractFixture(t, "route_plans_initial_route.json", recorder.Body.Bytes())
}

func TestRoutePlansEndpointTranscribesReassessmentsAndAnswersEmptyRegistry(t *testing.T) {
	reader := &stubRoutePlanReader{reassessments: []ports.RouteReassessmentCatalogueRow{
		{
			CorrelationID:      "SYN-CORR-1",
			CustomerAccountID:  "SYN-ACC-1",
			ShipmentRequestID:  "SYN-REQ-1",
			AcceptanceBaseline: "SYN-BASELINE-1",
			DeclaredParcelID:   "SYN-PARCEL-1",
			ServicePurpose:     "DELIVERY",
			Conclusion:         "PLAN_LAPSED",
			ReviewedPlan:       "SYN-PLAN-1#v1",
			HasReviewedPlan:    true,
			LapseBasis:         "SYN-LAPSE/line-withdrawn",
			HasLapseBasis:      true,
			CandidateState:     "NO_QUALIFIED_CANDIDATES",
			HasCandidateState:  true,
			ReassessedAt:       endpointBaseAt,
			RecordedAt:         endpointBaseAt.Add(time.Minute),
		},
		// 已改路的一行：四个可缺席键同时在场——这是唯一能让 rerouteState 上列的走向，
		// 契约夹具要靠它给前端一个 rerouteState 的实例。
		{
			CorrelationID:      "SYN-CORR-2",
			CustomerAccountID:  "SYN-ACC-1",
			ShipmentRequestID:  "SYN-REQ-2",
			AcceptanceBaseline: "SYN-BASELINE-1",
			DeclaredParcelID:   "SYN-PARCEL-2",
			ServicePurpose:     "DELIVERY",
			Conclusion:         "REROUTED",
			ReviewedPlan:       "SYN-PLAN-2#v1",
			HasReviewedPlan:    true,
			LapseBasis:         "SYN-LAPSE/closure-7",
			HasLapseBasis:      true,
			CandidateState:     "CANDIDATES_AVAILABLE",
			HasCandidateState:  true,
			RerouteState:       "AUTOMATIC_ALLOWED",
			HasRerouteState:    true,
			ReassessedAt:       endpointBaseAt.Add(2 * time.Minute),
			RecordedAt:         endpointBaseAt.Add(3 * time.Minute),
		},
	}}
	endpoint := networkhttp.NewQueryRoutePlansEndpoint(
		grantedCatalogueIntake{tenant: "tenant-1", limit: 25}, reader)

	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/route-plans?register=reassessment", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("上列答 %d, want 200\n%s", recorder.Code, recorder.Body.String())
	}
	body := routePlanBody(t, recorder)
	if body["outcome"] != "ROUTE_REASSESSMENTS_LISTED" {
		t.Fatalf("outcome = %v", body["outcome"])
	}
	reassessments, ok := body["reassessments"].([]any)
	if !ok || len(reassessments) != 2 {
		t.Fatalf("reassessments 形状变形：%v", body["reassessments"])
	}
	lapsed, _ := reassessments[0].(map[string]any)
	if lapsed["correlationId"] != "SYN-CORR-1" || lapsed["conclusion"] != "PLAN_LAPSED" ||
		lapsed["reviewedPlan"] != "SYN-PLAN-1#v1" || lapsed["lapseBasis"] != "SYN-LAPSE/line-withdrawn" ||
		lapsed["candidateState"] != "NO_QUALIFIED_CANDIDATES" {
		t.Fatalf("复核行转写变形：%v", lapsed)
	}
	if _, present := lapsed["rerouteState"]; present {
		t.Fatalf("未评估的改路判定长出来了：%v", lapsed)
	}
	rerouted, _ := reassessments[1].(map[string]any)
	if rerouted["conclusion"] != "REROUTED" || rerouted["rerouteState"] != "AUTOMATIC_ALLOWED" ||
		rerouted["candidateState"] != "CANDIDATES_AVAILABLE" {
		t.Fatalf("已改路行转写变形：%v", rerouted)
	}

	// 上面逐键断言过的这份响应体原样钉成契约夹具：一行失效、一行已改路，可缺席键都至少
	// 出场一次（rerouteState 还兼有缺席的实例），前端对着它校类型时每一格都有得核。
	assertContractFixture(t, "route_plans_reassessment.json", recorder.Body.Bytes())

	// 空册答空数组：换一个没有预置行的读口再问一次。
	empty := networkhttp.NewQueryRoutePlansEndpoint(
		grantedCatalogueIntake{tenant: "tenant-1", limit: 25}, &stubRoutePlanReader{})
	recorder = httptest.NewRecorder()
	empty.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/route-plans?register=reassessment", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("空册答 %d, want 200", recorder.Code)
	}
	rows, ok := routePlanBody(t, recorder)["reassessments"].([]any)
	if !ok {
		t.Fatal("空册没交回数组")
	}
	if len(rows) != 0 {
		t.Fatalf("空册交回 %d 行", len(rows))
	}
}

func TestRoutePlansEndpointAnswersServerErrorWhenReadFails(t *testing.T) {
	endpoint := networkhttp.NewQueryRoutePlansEndpoint(
		grantedCatalogueIntake{tenant: "tenant-1", limit: 25},
		&stubRoutePlanReader{err: errors.New("寄了")},
	)
	for _, register := range []string{"initial-route", "reassessment"} {
		recorder := httptest.NewRecorder()
		endpoint.ServeHTTP(recorder,
			httptest.NewRequest(http.MethodGet, "/route-plans?register="+register, nil))
		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("读失败对 %s 答 %d, want 500", register, recorder.Code)
		}
	}
}
