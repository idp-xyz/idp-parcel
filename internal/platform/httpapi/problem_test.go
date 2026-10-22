package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.idp.xyz/idp-parcel/internal/platform/buildinfo"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type problemBody struct {
	Type      string  `json:"type"`
	Title     string  `json:"title"`
	Status    int     `json:"status"`
	Code      string  `json:"code"`
	Detail    *string `json:"detail"`
	RequestID string  `json:"requestId"`
	Error     any     `json:"error"`
	Outcome   any     `json:"outcome"`
}

func decodeProblem(t *testing.T, response *httptest.ResponseRecorder) problemBody {
	t.Helper()
	if got := response.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("Content-Type = %q, want application/problem+json", got)
	}
	var body problemBody
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	return body
}

// Covers: ADR-0140 决定三——4xx/5xx 是 RFC 9457 问题详情：`type` 一码一个稳定 URI、`title` 是码的固定短语、`status`、`code`
// 原样、`requestId` 取路由中间件已经生成的那个值；不带 `outcome`，也不带旧形状的 `error` 成员。
func TestAProblemCarriesTheRFC9457MembersAndTheRequestID(t *testing.T) {
	router := httpapi.NewWithEndpoints(buildinfo.Info{}, []httpapi.BusinessEndpoint{{
		Pattern: "/v1/things",
		Handler: http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			httpapi.WriteProblem(response, request, http.StatusForbidden, "ACCESS_CHANNEL_NOT_CONFIGURED")
		}),
	}})
	request := httptest.NewRequest(http.MethodPost, "/v1/things", nil)
	request.Header.Set("X-Request-Id", "SYN-REQ-1")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d", response.Code)
	}
	body := decodeProblem(t, response)
	if body.Type != "tag:idp.xyz,2026:problem:ACCESS_CHANNEL_NOT_CONFIGURED" || body.Title != "Access channel not configured" ||
		body.Status != http.StatusForbidden || body.Code != "ACCESS_CHANNEL_NOT_CONFIGURED" || body.RequestID != "SYN-REQ-1" {
		t.Fatalf("problem = %s", response.Body.Bytes())
	}
	if body.Detail != nil || body.Error != nil || body.Outcome != nil {
		t.Fatalf("problem 多带了成员：%s", response.Body.Bytes())
	}
}

// Covers: ADR-0140 决定三「`detail` 只随改请求才会好的 4xx 在场」——403、404 受探针同答约束，5xx 是依赖故障的内部原文，三者
// 一律不带；写问题体的一方交来了也不写出去。
func TestDetailOnlyTravelsWithAFixableFourHundred(t *testing.T) {
	cases := map[int]bool{
		http.StatusBadRequest:          true,
		http.StatusTooManyRequests:     true,
		http.StatusForbidden:           false,
		http.StatusNotFound:            false,
		http.StatusInternalServerError: false,
		http.StatusServiceUnavailable:  false,
	}
	for status, carried := range cases {
		response := httptest.NewRecorder()
		httpapi.WriteProblemWithDetail(response, httptest.NewRequest(http.MethodPost, "/", nil), status, "SOME_CODE", "parcels[0]: missing")
		body := decodeProblem(t, response)
		if (body.Detail != nil) != carried {
			t.Fatalf("status %d: detail = %v, want carried %v", status, body.Detail, carried)
		}
		if body.Status != status || response.Code != status {
			t.Fatalf("status %d: body status = %d, response = %d", status, body.Status, response.Code)
		}
	}
}

// Covers: `requestId` 是对账句柄，不是凭空造的值——没有经过路由中间件的请求没有这个值，整键不出现。
func TestARequestWithoutAnIDLeavesTheMemberOut(t *testing.T) {
	response := httptest.NewRecorder()
	httpapi.WriteProblem(response, httptest.NewRequest(http.MethodGet, "/", nil), http.StatusInternalServerError, "NO_ANSWER_FORMED")
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, present := fields["requestId"]; present {
		t.Fatalf("problem = %s", response.Body.Bytes())
	}
}
