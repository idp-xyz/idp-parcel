package warehousehttp_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/platform/buildinfo"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
)

// switchedOffLines 是仓这一册的各口与各自那一行声明的方法。
var switchedOffLines = []struct {
	pattern string
	method  string
}{
	{registrationsPath, http.MethodPost},
	{listPath, http.MethodGet},
	{revisionsPath, http.MethodGet},
}

// switchedOffRouter 经真路由挂开关关着时各口的固定处理器：requestId 由路由中间件逐请求生成，与装配后的运行时同形。
func switchedOffRouter() http.Handler {
	var endpoints []httpapi.BusinessEndpoint
	for _, line := range switchedOffLines {
		endpoints = append(endpoints, httpapi.BusinessEndpoint{Pattern: line.pattern, Handler: warehousehttp.NewNotEnabledEndpoint(line.method)})
	}
	return httpapi.NewWithEndpoints(buildinfo.Info{}, endpoints)
}

func withoutRequestID(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	object := decodeObject(t, response)
	delete(object, "requestId")
	canonical, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	return string(canonical)
}

// Covers: AT-WF-012（端点那一半）——开关关着时仓这一册的各口答 403 问题详情（application/problem+json），码
// WAREHOUSE_FULFILLMENT_NOT_ENABLED，不带 outcome，也不带 detail（403 受探针同答约束），不是 404。
func TestSwitchedOffLinesAnswer403ProblemDetailsWithTheNotEnabledCode(t *testing.T) {
	router := switchedOffRouter()
	for _, line := range switchedOffLines {
		request := httptest.NewRequest(line.method, line.pattern, nil)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)

		object := assertProblem(t, line.pattern, response, http.StatusForbidden, "WAREHOUSE_FULFILLMENT_NOT_ENABLED")
		if response.Code == http.StatusNotFound {
			t.Fatalf("%s：开关关着不得答 404", line.pattern)
		}
		if stringMember(t, object, "type") != "tag:idp.xyz,2026:problem:WAREHOUSE_FULFILLMENT_NOT_ENABLED" ||
			stringMember(t, object, "title") != "Warehouse fulfillment not enabled" || string(object["status"]) != "403" {
			t.Fatalf("%s：问题详情成员 = %s", line.pattern, response.Body.String())
		}
		if _, present := object["detail"]; present {
			t.Fatalf("%s：403 不带 detail：%s", line.pattern, response.Body.String())
		}
		if stringMember(t, object, "requestId") == "" {
			t.Fatalf("%s：经路由的问题详情要带 requestId", line.pattern)
		}
	}
}

// Covers: AT-WF-012（端点那一半）——对任何调用方、任何请求内容都是同一份：带不带令牌、令牌是什么、报文合不合法、查询串写得对不对、
// 带不带自报租户的头，除逐请求不同的 requestId 外答复逐字相同。固定处理器不依赖任何认证、读面或编排，所以也没有东西能让答复随
// 它们变。
func TestSwitchedOffLinesAnswerEveryCallerAndEveryRequestAlike(t *testing.T) {
	router := switchedOffRouter()
	valid := syntheticWarehouse("SYN-WH-01", 1).String()
	for _, line := range switchedOffLines {
		variants := map[string]func() *http.Request{
			"bare": func() *http.Request { return httptest.NewRequest(line.method, line.pattern, nil) },
			"bearer token": func() *http.Request {
				request := httptest.NewRequest(line.method, line.pattern, strings.NewReader(valid))
				request.Header.Set("Authorization", "Bearer presented.operator.token")
				return request
			},
			"garbage token, broken body": func() *http.Request {
				request := httptest.NewRequest(line.method, line.pattern, strings.NewReader(`{not json`))
				request.Header.Set("Authorization", "Bearer ")
				return request
			},
			"self-reported tenant": func() *http.Request {
				request := httptest.NewRequest(line.method, line.pattern, strings.NewReader(`{"tenantId":"`+tenantB+`"}`))
				request.Header.Set("X-Reported-Tenant", tenantB)
				return request
			},
			"query the catalogue would refuse": func() *http.Request {
				return httptest.NewRequest(line.method, line.pattern+"?deactivated=maybe&warehouseCode=", nil)
			},
			"query that names a warehouse": func() *http.Request {
				return httptest.NewRequest(line.method, line.pattern+"?warehouseCode=SYN-WH-01", nil)
			},
		}
		var baseline *httptest.ResponseRecorder
		for name, build := range variants {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, build())
			assertProblem(t, line.pattern+" "+name, response, http.StatusForbidden, "WAREHOUSE_FULFILLMENT_NOT_ENABLED")
			if baseline == nil {
				baseline = response
				continue
			}
			if response.Code != baseline.Code || withoutRequestID(t, response) != withoutRequestID(t, baseline) {
				t.Fatalf("%s %s：答复 %d %s 与别的请求的 %d %s 不同", line.pattern, name,
					response.Code, response.Body.String(), baseline.Code, baseline.Body.String())
			}
		}
	}
}

// Covers: AT-WF-012（端点那一半）——不读报文：开关关着时请求体一个字节都不读；也不碰任何认证与读写面——处理器的构造函数不收任何
// 依赖，所以这是结构，不是约定。
func TestSwitchedOffLinesNeverReadTheBody(t *testing.T) {
	handler := warehousehttp.NewNotEnabledEndpoint(http.MethodPost)
	body := newCountingBody(syntheticWarehouse("SYN-WH-01", 1).String())
	request := httptest.NewRequest(http.MethodPost, registrationsPath, body)
	request.Header.Set("Authorization", "Bearer presented.operator.token")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	assertProblem(t, "关着", response, http.StatusForbidden, "WAREHOUSE_FULFILLMENT_NOT_ENABLED")
	if body.reads != 0 {
		t.Fatalf("开关关着仍读了请求体 %d 次", body.reads)
	}
}

// Covers: ADR-0055 「未配置不改变方法约束」在开关关着时的对应——方法不对仍由处理器自己答 405 并带 Allow（与开关开着时同一份），
// 不答 403 抢答，更不答 404；方法对的才答 NOT_ENABLED。
func TestSwitchedOffLinesKeepTheMethodGateOfTheirLine(t *testing.T) {
	router := switchedOffRouter()
	for _, line := range switchedOffLines {
		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(method, line.pattern, nil))
			if response.Code == http.StatusNotFound {
				t.Fatalf("%s %s：不得答 404", method, line.pattern)
			}
			if method == line.method {
				assertProblem(t, method+" "+line.pattern, response, http.StatusForbidden, "WAREHOUSE_FULFILLMENT_NOT_ENABLED")
				continue
			}
			assertProblem(t, method+" "+line.pattern, response, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
			if allow := response.Header().Get("Allow"); allow != line.method {
				t.Fatalf("%s %s：Allow = %q，want %s", method, line.pattern, allow, line.method)
			}
		}
	}
}
