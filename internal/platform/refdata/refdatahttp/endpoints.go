// Package refdatahttp 是产品内置基础数据目录（国家 / 地区、币种、计量单位，ADR-0335）的三个运营查阅读口。
//
// 契约（读口形状，ADR-0335 决定六；管理台镜像此处，改这里须同改 apps/admin-web/src/pages/reference-data/api.ts）：
//
//	GET /reference-data/countries?q=&region=
//	  200 {"outcome":"COUNTRIES_LISTED","countries":[{"code","nameZh","nameEn","region"}]}
//	GET /reference-data/currencies?q=
//	  200 {"outcome":"CURRENCIES_LISTED","currencies":[{"code","nameZh","nameEn","symbol","minorUnitScale"}]}
//	GET /reference-data/units?q=&category=
//	  200 {"outcome":"UNITS_LISTED","units":[{"code","nameZh","category","baseUnit","factorToBase"}]}
//
// q 按码或名称检索（不分大小写、子串匹配），空检索答全表；region 是封闭七词、category 是封闭四词，写错答 400 而不是空表。
// 失败一律 {"error":{"code"}}：400 REGION_NOT_RECOGNIZED / CATEGORY_NOT_RECOGNIZED、401 OPERATOR_CREDENTIAL_REJECTED、
// 403 OPERATOR_NOT_GRANTED / ACCESS_CHANNEL_NOT_CONFIGURED、405 METHOD_NOT_ALLOWED、503 IDENTITY_DEPENDENCY_UNAVAILABLE。
//
// 租户只取自认证信封：请求里自报的租户（查询串、头）一概不采信，也不送给目录——目录本身不分租户，
// 信封在这里只证明“这是一个绑了租户、持有主数据与运营查阅读授予的操作者”。只读，不触发判断、决定或披露。
package refdatahttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/platform/refdata"
)

// 三个读口的路径。parcel-api 端点表按字面量登记同一批路径（管理台路径门禁只认端点表上的字符串，不认常量引用）。
const (
	CountriesPath  = "/reference-data/countries"
	CurrenciesPath = "/reference-data/currencies"
	UnitsPath      = "/reference-data/units"
)

const (
	outcomeCountriesListed  = "COUNTRIES_LISTED"
	outcomeCurrenciesListed = "CURRENCIES_LISTED"
	outcomeUnitsListed      = "UNITS_LISTED"
)

const (
	codeMethodNotAllowed              = "METHOD_NOT_ALLOWED"
	codeAccessChannelNotConfigured    = "ACCESS_CHANNEL_NOT_CONFIGURED"
	codeOperatorCredentialRejected    = "OPERATOR_CREDENTIAL_REJECTED"
	codeOperatorNotGranted            = "OPERATOR_NOT_GRANTED"
	codeIdentityDependencyUnavailable = "IDENTITY_DEPENDENCY_UNAVAILABLE"
	codeIntakeFailed                  = "INTAKE_FAILED"
	codeRegionNotRecognized           = "REGION_NOT_RECOGNIZED"
	codeCategoryNotRecognized         = "CATEGORY_NOT_RECOGNIZED"
)

// 认证件交回的四格，各对一种恢复动作（ADR-0029）。从共享接入身份译过来的那一层在组合根（cmd/parcel-api）：
// platform 不得导入 accessidentity，所以这里只认自己的哨兵。
var (
	// ErrAccessChannelNotConfigured：发行方参数未设（403）——去配置接入渠道。
	ErrAccessChannelNotConfigured = errors.New("reference data http: access channel not configured")
	// ErrOperatorCredentialRejected：令牌缺失、过期或校验不过（401）——持令牌的人去登录或换令牌。
	ErrOperatorCredentialRejected = errors.New("reference data http: operator credential rejected")
	// ErrOperatorNotGranted：令牌有效，但不在册、不绑租户或没有主数据与运营查阅读的授予（403）——管授予的人去登记。
	ErrOperatorNotGranted = errors.New("reference data http: operator holds no master data and operations read grant")
	// ErrIdentityDependencyUnavailable：发行方公钥集或操作者册读不动（503）——运维去救依赖。
	ErrIdentityDependencyUnavailable = errors.New("reference data http: identity dependency unavailable")
)

// OperationsReadAuthenticator 认证一次主数据与运营查阅读出示，交回信封上的租户。失败只交回本包的四格。
type OperationsReadAuthenticator interface {
	AuthenticateOperationsRead(ctx context.Context, bearerToken string) (tenant string, err error)
}

// NewCountriesEndpoint 交回国家 / 地区目录查阅的 HTTP 入口。
func NewCountriesEndpoint(authenticator OperationsReadAuthenticator) (http.Handler, error) {
	return newEndpoint(authenticator, func(response http.ResponseWriter, request *http.Request) {
		region := refdata.Region(request.URL.Query().Get("region"))
		if region != "" && !region.Valid() {
			writeProblem(response, http.StatusBadRequest, codeRegionNotRecognized)
			return
		}
		rows := refdata.SearchCountries(request.URL.Query().Get("q"), region)
		bodies := make([]countryBody, 0, len(rows))
		for _, row := range rows {
			bodies = append(bodies, countryBody{Code: row.Code, NameZh: row.NameZh, NameEn: row.NameEn, Region: string(row.Region)})
		}
		writeJSON(response, http.StatusOK, countryListResponse{Outcome: outcomeCountriesListed, Countries: bodies})
	})
}

// NewCurrenciesEndpoint 交回币种目录查阅的 HTTP 入口。
func NewCurrenciesEndpoint(authenticator OperationsReadAuthenticator) (http.Handler, error) {
	return newEndpoint(authenticator, func(response http.ResponseWriter, request *http.Request) {
		rows := refdata.SearchCurrencies(request.URL.Query().Get("q"))
		bodies := make([]currencyBody, 0, len(rows))
		for _, row := range rows {
			bodies = append(bodies, currencyBody{
				Code: row.Code, NameZh: row.NameZh, NameEn: row.NameEn, Symbol: row.Symbol, MinorUnitScale: row.MinorUnitScale,
			})
		}
		writeJSON(response, http.StatusOK, currencyListResponse{Outcome: outcomeCurrenciesListed, Currencies: bodies})
	})
}

// NewUnitsEndpoint 交回计量单位目录查阅的 HTTP 入口。
func NewUnitsEndpoint(authenticator OperationsReadAuthenticator) (http.Handler, error) {
	return newEndpoint(authenticator, func(response http.ResponseWriter, request *http.Request) {
		category := refdata.UnitCategory(request.URL.Query().Get("category"))
		if category != "" && !category.Valid() {
			writeProblem(response, http.StatusBadRequest, codeCategoryNotRecognized)
			return
		}
		rows := refdata.SearchUnits(request.URL.Query().Get("q"), category)
		bodies := make([]unitBody, 0, len(rows))
		for _, row := range rows {
			bodies = append(bodies, unitBody{
				Code: row.Code, NameZh: row.NameZh, Category: string(row.Category), BaseUnit: row.BaseUnit, FactorToBase: row.FactorToBase,
			})
		}
		writeJSON(response, http.StatusOK, unitListResponse{Outcome: outcomeUnitsListed, Units: bodies})
	})
}

// newEndpoint 收拢三口共有的外壳：方法、认证、再交给各口自己的取数。认证在读参数之前——没有授予的人
// 连 400（参数不成立）都不该看见，与其余目录读口先认证后读参数同序。
func newEndpoint(authenticator OperationsReadAuthenticator, serve http.HandlerFunc) (http.Handler, error) {
	if authenticator == nil {
		return nil, errors.New("reference data http: endpoint needs an authenticator")
	}
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		tenant, err := authenticator.AuthenticateOperationsRead(request.Context(), httpapi.BearerToken(request))
		if err != nil {
			writeAuthenticationProblem(response, err)
			return
		}
		// 铸出的信封没有租户是认证件的毛病，不是调用方的：不放行一个没绑租户的“操作者”。
		if tenant == "" {
			writeProblem(response, http.StatusInternalServerError, codeIntakeFailed)
			return
		}
		serve(response, request)
	}), nil
}

// HandlerOrUnconfigured 把已装好的处理器交回端点表；没装时答未配置（装配测试不建铸造器，仍要这一行在表上，
// 答复与发行方未设同一格）。
func HandlerOrUnconfigured(wired http.Handler) http.Handler {
	if wired == nil {
		return unconfigured()
	}
	return wired
}

func unconfigured() http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		writeProblem(response, http.StatusForbidden, codeAccessChannelNotConfigured)
	})
}

func writeAuthenticationProblem(response http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrAccessChannelNotConfigured):
		writeProblem(response, http.StatusForbidden, codeAccessChannelNotConfigured)
	case errors.Is(err, ErrOperatorCredentialRejected):
		writeProblem(response, http.StatusUnauthorized, codeOperatorCredentialRejected)
	case errors.Is(err, ErrOperatorNotGranted):
		writeProblem(response, http.StatusForbidden, codeOperatorNotGranted)
	case errors.Is(err, ErrIdentityDependencyUnavailable):
		writeProblem(response, http.StatusServiceUnavailable, codeIdentityDependencyUnavailable)
	default:
		writeProblem(response, http.StatusInternalServerError, codeIntakeFailed)
	}
}

type countryBody struct {
	Code   string `json:"code"`
	NameZh string `json:"nameZh"`
	NameEn string `json:"nameEn"`
	Region string `json:"region"`
}

type countryListResponse struct {
	Outcome   string        `json:"outcome"`
	Countries []countryBody `json:"countries"`
}

type currencyBody struct {
	Code           string `json:"code"`
	NameZh         string `json:"nameZh"`
	NameEn         string `json:"nameEn"`
	Symbol         string `json:"symbol"`
	MinorUnitScale uint8  `json:"minorUnitScale"`
}

type currencyListResponse struct {
	Outcome    string         `json:"outcome"`
	Currencies []currencyBody `json:"currencies"`
}

type unitBody struct {
	Code         string `json:"code"`
	NameZh       string `json:"nameZh"`
	Category     string `json:"category"`
	BaseUnit     string `json:"baseUnit"`
	FactorToBase string `json:"factorToBase"`
}

type unitListResponse struct {
	Outcome string     `json:"outcome"`
	Units   []unitBody `json:"units"`
}

type problemResponse struct {
	Error problemDetail `json:"error"`
}

type problemDetail struct {
	Code string `json:"code"`
}

func writeProblem(response http.ResponseWriter, status int, code string) {
	writeJSON(response, status, problemResponse{Error: problemDetail{Code: code}})
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}
