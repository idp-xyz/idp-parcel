package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/refdata"
	"go.idp.xyz/idp-parcel/internal/platform/refdata/refdatahttp"
)

// Covers: ADR-0335 决定六——端点表上的字面量路径与 refdatahttp 的路径常量是同一批。
func TestReferenceDataPathsMatchTheEndpointTable(t *testing.T) {
	want := map[string]string{
		"/reference-data/countries":  refdatahttp.CountriesPath,
		"/reference-data/currencies": refdatahttp.CurrenciesPath,
		"/reference-data/units":      refdatahttp.UnitsPath,
	}
	declared := declaredEndpointLines()
	for literal, constant := range want {
		if literal != constant {
			t.Errorf("端点表字面量 %s 与路径常量 %s 对不上", literal, constant)
		}
		line, found := declared[literal]
		if !found || !line.ownIntake || line.face.family != faceOperationsRead || line.method != http.MethodGet {
			t.Errorf("%s 应登记为操作者查阅读、自建 Intake 的 GET 行：%+v", literal, line)
		}
	}
}

type referenceDataBody struct {
	Outcome   string `json:"outcome"`
	Countries []struct {
		Code   string `json:"code"`
		NameZh string `json:"nameZh"`
		Region string `json:"region"`
	} `json:"countries"`
	Currencies []struct {
		Code           string `json:"code"`
		MinorUnitScale int    `json:"minorUnitScale"`
	} `json:"currencies"`
	Units []struct {
		Code         string `json:"code"`
		Category     string `json:"category"`
		BaseUnit     string `json:"baseUnit"`
		FactorToBase string `json:"factorToBase"`
	} `json:"units"`
}

func referenceDataRequest(t *testing.T, handler http.Handler, method, target, token string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, target, nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func decodeReferenceData(t *testing.T, response *httptest.ResponseRecorder) referenceDataBody {
	t.Helper()
	var body referenceDataBody
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是 JSON：%v %s", err, response.Body.String())
	}
	return body
}

// Covers: ADR-0335 决定六、ADR-0055——三口接在操作者查阅读上：发行方未配置答未配置；没带令牌答凭证被拒；
// 只有登记写授予而没有查阅读授予答未授予；有查阅读授予才答目录。
func TestReferenceDataEndpointsAnswerFromTheOperatorChannel(t *testing.T) {
	granted, err := buildOperatorMinter(acceptingVerifier{}, operationsReaderRegister{}, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	ungranted, err := buildOperatorMinter(acceptingVerifier{}, registryWriterRegister{}, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	unconfigured, err := buildOperatorMinter(accessidentity.UnconfiguredOperatorCredentialVerifier{}, operationsReaderRegister{}, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		minter *accessidentity.OperatorMinter
		token  string
		status int
		code   string
	}{
		{"issuer unconfigured", unconfigured, "presented.operator.token", http.StatusForbidden, "ACCESS_CHANNEL_NOT_CONFIGURED"},
		{"no bearer token", granted, "", http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED"},
		{"write grant is not a read grant", ungranted, "presented.operator.token", http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
		{"read grant answers the catalogue", granted, "presented.operator.token", http.StatusOK, ""},
	}
	for _, path := range []string{refdatahttp.CountriesPath, refdatahttp.CurrenciesPath, refdatahttp.UnitsPath} {
		for _, testCase := range cases {
			t.Run(path+" "+testCase.name, func(t *testing.T) {
				handlers, err := buildReferenceData(testCase.minter)
				if err != nil {
					t.Fatal(err)
				}
				handler := map[string]http.Handler{
					refdatahttp.CountriesPath:  handlers.countries,
					refdatahttp.CurrenciesPath: handlers.currencies,
					refdatahttp.UnitsPath:      handlers.units,
				}[path]
				// 自报的租户不算数：下面的答复不随它变（目录不分租户），而认证只看令牌。
				response := referenceDataRequest(t, handler, http.MethodGet, path+"?tenant=SYN-TENANT-02", testCase.token)
				if response.Code != testCase.status {
					t.Fatalf("status %d body %s", response.Code, response.Body.String())
				}
				if testCase.code != "" {
					if problemCode(t, response) != testCase.code {
						t.Fatalf("code %s body %s", problemCode(t, response), response.Body.String())
					}
					return
				}
				if decodeReferenceData(t, response).Outcome == "" {
					t.Fatalf("body %s", response.Body.String())
				}
			})
		}
	}

	if _, err := buildReferenceData(nil); err == nil {
		t.Fatal("nil minter was accepted")
	}
}

// Covers: ADR-0335 决定六——空检索答全表；按码或名称检索；大洲与类别过滤；写错的大洲或类别答 400 而不是空表；
// 只收 GET。
func TestReferenceDataEndpointsSearchFilterAndRefuse(t *testing.T) {
	minter, err := buildOperatorMinter(acceptingVerifier{}, operationsReaderRegister{}, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	handlers, err := buildReferenceData(minter)
	if err != nil {
		t.Fatal(err)
	}
	const token = "presented.operator.token"

	all := decodeReferenceData(t, referenceDataRequest(t, handlers.countries, http.MethodGet, "/reference-data/countries", token))
	if all.Outcome != "COUNTRIES_LISTED" || len(all.Countries) != len(refdata.Countries()) {
		t.Fatalf("空检索应答全表 %d 项，得到 %s %d 项", len(refdata.Countries()), all.Outcome, len(all.Countries))
	}
	byName := decodeReferenceData(t, referenceDataRequest(t, handlers.countries, http.MethodGet, "/reference-data/countries?q=%E6%96%B0%E5%8A%A0%E5%9D%A1", token))
	if len(byName.Countries) != 1 || byName.Countries[0].Code != "SG" {
		t.Fatalf("按中文名检索新加坡应只命中 SG：%+v", byName.Countries)
	}
	byCode := decodeReferenceData(t, referenceDataRequest(t, handlers.countries, http.MethodGet, "/reference-data/countries?q=cn&region=ASIA", token))
	if len(byCode.Countries) == 0 || byCode.Countries[0].Region != "ASIA" {
		t.Fatalf("按码检索加大洲过滤：%+v", byCode.Countries)
	}
	if response := referenceDataRequest(t, handlers.countries, http.MethodGet, "/reference-data/countries?region=MIDDLE_EARTH", token); response.Code != http.StatusBadRequest ||
		problemCode(t, response) != "REGION_NOT_RECOGNIZED" {
		t.Fatalf("写错的大洲应答 400 REGION_NOT_RECOGNIZED：%d %s", response.Code, response.Body.String())
	}

	currencies := decodeReferenceData(t, referenceDataRequest(t, handlers.currencies, http.MethodGet, "/reference-data/currencies?q=jpy", token))
	if currencies.Outcome != "CURRENCIES_LISTED" || len(currencies.Currencies) != 1 || currencies.Currencies[0].Code != "JPY" || currencies.Currencies[0].MinorUnitScale != 0 {
		t.Fatalf("按码检索 jpy：%+v", currencies.Currencies)
	}

	weights := decodeReferenceData(t, referenceDataRequest(t, handlers.units, http.MethodGet, "/reference-data/units?category=WEIGHT", token))
	if weights.Outcome != "UNITS_LISTED" || len(weights.Units) == 0 {
		t.Fatalf("按类别过滤重量：%+v", weights)
	}
	foundPound := false
	for _, unit := range weights.Units {
		if unit.Category != "WEIGHT" {
			t.Fatalf("类别过滤混进了 %s", unit.Code)
		}
		if unit.Code == "LB" {
			foundPound = unit.BaseUnit == "KG" && unit.FactorToBase == "0.45359237"
		}
	}
	if !foundPound {
		t.Fatal("重量类里应有 LB，基准 KG、系数 0.45359237")
	}
	if response := referenceDataRequest(t, handlers.units, http.MethodGet, "/reference-data/units?category=TEMPERATURE", token); response.Code != http.StatusBadRequest ||
		problemCode(t, response) != "CATEGORY_NOT_RECOGNIZED" {
		t.Fatalf("写错的类别应答 400 CATEGORY_NOT_RECOGNIZED：%d %s", response.Code, response.Body.String())
	}

	for _, handler := range []http.Handler{handlers.countries, handlers.currencies, handlers.units} {
		response := referenceDataRequest(t, handler, http.MethodPost, "/reference-data/units", token)
		if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodGet {
			t.Fatalf("非 GET 应答 405 并带 Allow: GET：%d %v", response.Code, response.Header())
		}
	}
}
