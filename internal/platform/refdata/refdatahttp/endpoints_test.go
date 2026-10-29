package refdatahttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.idp.xyz/idp-parcel/internal/platform/refdata"
	"go.idp.xyz/idp-parcel/internal/platform/refdata/refdatahttp"
)

// spyAuthenticator 记下每次认证收到的令牌——它是认证件唯一收到的东西，请求里别的自报内容碰不到它。
type spyAuthenticator struct {
	tenant string
	err    error
	tokens []string
}

func (spy *spyAuthenticator) AuthenticateOperationsRead(_ context.Context, bearerToken string) (string, error) {
	spy.tokens = append(spy.tokens, bearerToken)
	return spy.tenant, spy.err
}

type endpointConstructor func(refdatahttp.OperationsReadAuthenticator) (http.Handler, error)

var allEndpoints = map[string]endpointConstructor{
	refdatahttp.CountriesPath:  refdatahttp.NewCountriesEndpoint,
	refdatahttp.CurrenciesPath: refdatahttp.NewCurrenciesEndpoint,
	refdatahttp.UnitsPath:      refdatahttp.NewUnitsEndpoint,
}

func serve(t *testing.T, handler http.Handler, method, target string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, target, nil)
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func problemCode(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是问题体：%v %s", err, response.Body.String())
	}
	return body.Error.Code
}

// Covers: ADR-0335 决定六——租户取自信封：认证件只收到令牌，请求自报的租户（查询串、头）不进认证也不改答复。
func TestTenantComesFromTheEnvelopeAndNeverFromTheRequest(t *testing.T) {
	t.Parallel()

	for path, construct := range allEndpoints {
		spy := &spyAuthenticator{tenant: "SYN-TENANT-01"}
		handler, err := construct(spy)
		if err != nil {
			t.Fatal(err)
		}
		plain := serve(t, handler, http.MethodGet, path, map[string]string{"Authorization": "Bearer token-1"})
		selfReported := serve(t, handler, http.MethodGet, path+"?tenant=SYN-TENANT-02&tenantId=X",
			map[string]string{"Authorization": "Bearer token-1", "X-Tenant": "SYN-TENANT-02", "X-Tenant-Id": "SYN-TENANT-02"})
		if plain.Code != http.StatusOK || selfReported.Code != http.StatusOK {
			t.Fatalf("%s：status %d / %d", path, plain.Code, selfReported.Code)
		}
		if plain.Body.String() != selfReported.Body.String() {
			t.Errorf("%s：自报租户改变了答复", path)
		}
		if fmt.Sprint(spy.tokens) != "[token-1 token-1]" {
			t.Errorf("%s：认证件收到的应只有令牌，得到 %v", path, spy.tokens)
		}
	}
}

// Covers: ADR-0335 决定六——认证在读参数之前：没有授予的人连“参数不成立”都看不见；认证件的四格各对一种状态码；
// 信封没有租户不放行。
func TestAuthenticationAnswersBeforeParametersAreRead(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		spy    *spyAuthenticator
		status int
		code   string
	}{
		{"unconfigured", &spyAuthenticator{err: refdatahttp.ErrAccessChannelNotConfigured}, http.StatusForbidden, "ACCESS_CHANNEL_NOT_CONFIGURED"},
		{"rejected", &spyAuthenticator{err: refdatahttp.ErrOperatorCredentialRejected}, http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED"},
		{"not granted", &spyAuthenticator{err: fmt.Errorf("wrapped: %w", refdatahttp.ErrOperatorNotGranted)}, http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
		{"dependency down", &spyAuthenticator{err: refdatahttp.ErrIdentityDependencyUnavailable}, http.StatusServiceUnavailable, "IDENTITY_DEPENDENCY_UNAVAILABLE"},
		{"unclassified failure", &spyAuthenticator{err: errors.New("boom")}, http.StatusInternalServerError, "INTAKE_FAILED"},
		{"envelope without tenant", &spyAuthenticator{tenant: ""}, http.StatusInternalServerError, "INTAKE_FAILED"},
	}
	for _, testCase := range cases {
		// 参数故意写错：认证失败时必须答认证的格，而不是 400。
		handler, err := refdatahttp.NewCountriesEndpoint(testCase.spy)
		if err != nil {
			t.Fatal(err)
		}
		response := serve(t, handler, http.MethodGet, refdatahttp.CountriesPath+"?region=NOWHERE", nil)
		if response.Code != testCase.status || problemCode(t, response) != testCase.code {
			t.Errorf("%s：status %d code %s，应为 %d %s", testCase.name, response.Code, problemCode(t, response), testCase.status, testCase.code)
		}
	}
}

// Covers: ADR-0335 决定六——空检索答全表，不分页；检索与大洲、类别过滤交给目录；写错的大洲 / 类别答 400。
func TestListingSearchAndFilters(t *testing.T) {
	t.Parallel()

	spy := &spyAuthenticator{tenant: "SYN-TENANT-01"}
	headers := map[string]string{"Authorization": "Bearer token-1"}

	countries, _ := refdatahttp.NewCountriesEndpoint(spy)
	var countryBody struct {
		Outcome   string                `json:"outcome"`
		Countries []refdata.CountryInfo `json:"countries"`
	}
	response := serve(t, countries, http.MethodGet, refdatahttp.CountriesPath, headers)
	if err := json.Unmarshal(response.Body.Bytes(), &countryBody); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || countryBody.Outcome != "COUNTRIES_LISTED" || len(countryBody.Countries) != len(refdata.Countries()) {
		t.Fatalf("国家空检索应答全表：%d %s %d 项", response.Code, countryBody.Outcome, len(countryBody.Countries))
	}

	response = serve(t, countries, http.MethodGet, refdatahttp.CountriesPath+"?q=Germany&region=EUROPE", headers)
	countryBody.Countries = nil
	if err := json.Unmarshal(response.Body.Bytes(), &countryBody); err != nil {
		t.Fatal(err)
	}
	if len(countryBody.Countries) != 1 {
		t.Errorf("按英文名 Germany 加大洲过滤应只命中一项：%s", response.Body.String())
	}
	response = serve(t, countries, http.MethodGet, refdatahttp.CountriesPath+"?q=zzzzzz", headers)
	if response.Code != http.StatusOK || !json.Valid(response.Body.Bytes()) || !containsEmptyList(response.Body.Bytes(), "countries") {
		t.Errorf("无匹配应答 200 加空数组而不是 null：%d %s", response.Code, response.Body.String())
	}
	if response := serve(t, countries, http.MethodGet, refdatahttp.CountriesPath+"?region=NOWHERE", headers); response.Code != http.StatusBadRequest ||
		problemCode(t, response) != "REGION_NOT_RECOGNIZED" {
		t.Errorf("写错的大洲应答 400：%d %s", response.Code, response.Body.String())
	}

	units, _ := refdatahttp.NewUnitsEndpoint(spy)
	if response := serve(t, units, http.MethodGet, refdatahttp.UnitsPath+"?category=NOPE", headers); response.Code != http.StatusBadRequest ||
		problemCode(t, response) != "CATEGORY_NOT_RECOGNIZED" {
		t.Errorf("写错的类别应答 400：%d %s", response.Code, response.Body.String())
	}
}

func containsEmptyList(body []byte, key string) bool {
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(body, &decoded); err != nil {
		return false
	}
	return string(decoded[key]) == "[]"
}

// Covers: ADR-0055——没装认证件的一行答未配置，且仍守方法闸；构造时认证件不许缺。
func TestUnconfiguredHandlerAndConstructorGuard(t *testing.T) {
	t.Parallel()

	handler := refdatahttp.HandlerOrUnconfigured(nil)
	if response := serve(t, handler, http.MethodGet, refdatahttp.UnitsPath, nil); response.Code != http.StatusForbidden ||
		problemCode(t, response) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
		t.Errorf("未装配应答 403 未配置：%d %s", response.Code, response.Body.String())
	}
	if response := serve(t, handler, http.MethodPost, refdatahttp.UnitsPath, nil); response.Code != http.StatusMethodNotAllowed ||
		response.Header().Get("Allow") != http.MethodGet {
		t.Errorf("未装配仍应守方法闸：%d %v", response.Code, response.Header())
	}
	wired, _ := refdatahttp.NewUnitsEndpoint(&spyAuthenticator{tenant: "T"})
	if refdatahttp.HandlerOrUnconfigured(wired) == nil {
		t.Error("已装好的处理器应原样交回")
	}
	for path, construct := range allEndpoints {
		if _, err := construct(nil); err == nil {
			t.Errorf("%s：缺认证件的构造应报错", path)
		}
	}
}
