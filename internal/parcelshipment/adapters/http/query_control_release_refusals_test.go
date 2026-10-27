package shipmenthttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件证释放未受理册的运营查阅端点（ADR-0293 决定一）：租户与页大小只来自 Intake（不采信请求）；清单逐字段透出，没存作用域的存量行
// 如实缺席；计数不被页大小截断且告诉调用方清单是否被截断；读不回答 500 而不是空册；非 GET 405；Intake 的各格原样映射。

type refusalIntakeDouble struct {
	tenant domain.TenantID
	limit  int
	err    error
}

func (double refusalIntakeDouble) IntakeControlReleaseRefusalQuery(
	context.Context,
	*http.Request,
) (shipmenthttp.ControlReleaseRefusalQuery, error) {
	if double.err != nil {
		return shipmenthttp.ControlReleaseRefusalQuery{}, double.err
	}
	return shipmenthttp.ControlReleaseRefusalQuery{Tenant: double.tenant, Limit: double.limit}, nil
}

type refusalCatalogueDouble struct {
	listed ports.UnhandledControlReleaseRefusals
	err    error

	tenant domain.TenantID
	limit  int
}

func (double *refusalCatalogueDouble) ListUnhandledControlReleaseRefusals(
	_ context.Context,
	tenant domain.TenantID,
	limit int,
) (ports.UnhandledControlReleaseRefusals, error) {
	double.tenant, double.limit = tenant, limit
	return double.listed, double.err
}

func refusalRow(t *testing.T, request, version string, scoped bool) ports.ControlReleaseRefusal {
	t.Helper()
	row := ports.ControlReleaseRefusal{
		TenantID:          mustValue(t, domain.NewTenantID, "TENANT-1"),
		CustomerAccountID: mustValue(t, domain.NewCustomerAccountID, "ACCOUNT-1"),
		ShipmentRequestID: mustValue(t, domain.NewShipmentRequestID, request),
		SubmissionVersion: mustValue(t, domain.NewSubmissionVersionID, version),
		ControlResultID:   mustValue(t, domain.NewFinancialControlResultID, request+"/"+version),
		Reason:            mustValue(t, domain.NewCheckReason, "SA-RELEASE_REQUEST_NOT_ACCEPTED"),
		RecordedAt:        time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC),
	}
	if scoped {
		scope, err := domain.NewOccupationScope("LE-1", "ACCT-1", "CNY")
		if err != nil {
			t.Fatal(err)
		}
		row.Scope = scope
	}
	return row
}

func getRefusals(t *testing.T, intake shipmenthttp.ControlReleaseRefusalQueryIntake, reader shipmenthttp.ControlReleaseRefusalCatalogueReader, method string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	shipmenthttp.NewQueryControlReleaseRefusalsEndpoint(intake, reader).
		ServeHTTP(response, httptest.NewRequest(method, "/control-release-refusals", nil))
	return response
}

// Covers: ADR-0293 决定一——清单逐字段透出；租户与页大小取 Intake 交出的、原样递给读口；存下作用域的行带三段，没存的存量行
// scopeRecorded=false 且没有 occupationScope；清单被页大小截断时 truncated=true 而计数是未处理的全部行数。
func TestTheRefusalQueryListsUnhandledRowsWithTheirCountAndTruncationFlag(t *testing.T) {
	reader := &refusalCatalogueDouble{listed: ports.UnhandledControlReleaseRefusals{
		Refusals: []ports.ControlReleaseRefusal{refusalRow(t, "REQUEST-1", "VERSION-1", false), refusalRow(t, "REQUEST-2", "VERSION-2", true)},
		Count:    5,
	}}
	intake := refusalIntakeDouble{tenant: mustValue(t, domain.NewTenantID, "TENANT-1"), limit: 2}

	response := getRefusals(t, intake, reader, http.MethodGet)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", response.Code, response.Body.String())
	}
	if reader.tenant.String() != "TENANT-1" || reader.limit != 2 {
		t.Fatalf("读口收到 tenant=%q limit=%d, want Intake 交出的 TENANT-1 与 2", reader.tenant, reader.limit)
	}
	var body struct {
		Outcome        string `json:"outcome"`
		UnhandledCount int    `json:"unhandledCount"`
		Truncated      bool   `json:"truncated"`
		Refusals       []struct {
			ShipmentRequestID   string `json:"shipmentRequestId"`
			SubmissionVersionID string `json:"submissionVersionId"`
			ControlResultID     string `json:"controlResultId"`
			CustomerAccountID   string `json:"customerAccountId"`
			ScopeRecorded       bool   `json:"scopeRecorded"`
			OccupationScope     *struct {
				LegalEntity       string `json:"legalEntity"`
				SettlementAccount string `json:"settlementAccount"`
				Currency          string `json:"currency"`
			} `json:"occupationScope"`
			Reason     string `json:"reason"`
			RecordedAt string `json:"recordedAt"`
		} `json:"refusals"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是 JSON：%v\n%s", err, response.Body.String())
	}
	if body.Outcome != "CONTROL_RELEASE_REFUSALS_LISTED" || body.UnhandledCount != 5 || !body.Truncated || len(body.Refusals) != 2 {
		t.Fatalf("响应 = %+v, want LISTED、计数 5、被截断、两行", body)
	}
	legacy, scoped := body.Refusals[0], body.Refusals[1]
	if legacy.ShipmentRequestID != "REQUEST-1" || legacy.SubmissionVersionID != "VERSION-1" || legacy.ControlResultID != "REQUEST-1/VERSION-1" ||
		legacy.CustomerAccountID != "ACCOUNT-1" || legacy.Reason != "SA-RELEASE_REQUEST_NOT_ACCEPTED" ||
		legacy.RecordedAt != "2026-10-26T09:00:00Z" {
		t.Fatalf("行没有逐字段透出：%+v", legacy)
	}
	if legacy.ScopeRecorded || legacy.OccupationScope != nil {
		t.Fatalf("没存作用域的存量行带了作用域：%+v", legacy)
	}
	if !scoped.ScopeRecorded || scoped.OccupationScope == nil ||
		scoped.OccupationScope.LegalEntity != "LE-1" || scoped.OccupationScope.SettlementAccount != "ACCT-1" || scoped.OccupationScope.Currency != "CNY" {
		t.Fatalf("存下的作用域没有三段透出：%+v", scoped)
	}
}

// Covers: ADR-0293 决定一——没有被截断时 truncated=false；空册答空数组而非 null 与零计数（「没有积压」是如实答案）。
func TestTheRefusalQueryAnswersAnEmptyRegisterAsAnEmptyArray(t *testing.T) {
	reader := &refusalCatalogueDouble{}
	intake := refusalIntakeDouble{tenant: mustValue(t, domain.NewTenantID, "TENANT-1"), limit: 50}
	response := getRefusals(t, intake, reader, http.MethodGet)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"refusals":[]`) ||
		!strings.Contains(response.Body.String(), `"unhandledCount":0`) || !strings.Contains(response.Body.String(), `"truncated":false`) {
		t.Fatalf("空册 = %d %s", response.Code, response.Body.String())
	}
}

// Covers: ADR-0029——读不回是答案未形成（500 NO_ANSWER_FORMED），不伪装成空册；Intake 各格原样映射，读口一次也不被调；非 GET 答 405。
func TestTheRefusalQueryMapsFailuresToTheirOwnCells(t *testing.T) {
	tenant := mustValue(t, domain.NewTenantID, "TENANT-1")

	failing := &refusalCatalogueDouble{err: errors.New("read failed")}
	response := getRefusals(t, refusalIntakeDouble{tenant: tenant, limit: 50}, failing, http.MethodGet)
	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "NO_ANSWER_FORMED") {
		t.Fatalf("读不回 = %d %s, want 500 NO_ANSWER_FORMED", response.Code, response.Body.String())
	}

	for _, test := range []struct {
		err    error
		status int
		code   string
	}{
		{shipmenthttp.ErrAccessChannelNotConfigured, http.StatusForbidden, "ACCESS_CHANNEL_NOT_CONFIGURED"},
		{shipmenthttp.ErrOperatorCredentialRejected, http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED"},
		{shipmenthttp.ErrOperatorNotGranted, http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
		{shipmenthttp.ErrIdentityDependencyUnavailable, http.StatusServiceUnavailable, "IDENTITY_DEPENDENCY_UNAVAILABLE"},
	} {
		reader := &refusalCatalogueDouble{}
		response := getRefusals(t, refusalIntakeDouble{err: test.err}, reader, http.MethodGet)
		if response.Code != test.status || !strings.Contains(response.Body.String(), test.code) {
			t.Errorf("Intake %v = %d %s, want %d %s", test.err, response.Code, response.Body.String(), test.status, test.code)
		}
		if reader.limit != 0 {
			t.Errorf("Intake %v 之后读口仍被调了", test.err)
		}
	}

	reader := &refusalCatalogueDouble{}
	response = getRefusals(t, refusalIntakeDouble{tenant: tenant, limit: 50}, reader, http.MethodPost)
	if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("POST = %d Allow=%q, want 405 与 Allow: GET", response.Code, response.Header().Get("Allow"))
	}
}

// Covers: ADR-0055——未配置的 Intake 对查阅如实答未配置（403），不读请求。
func TestTheRefusalQueryIsUnconfiguredUntilAChannelIsWired(t *testing.T) {
	reader := &refusalCatalogueDouble{}
	response := getRefusals(t, shipmenthttp.UnconfiguredIntake{}, reader, http.MethodGet)
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "ACCESS_CHANNEL_NOT_CONFIGURED") {
		t.Fatalf("未配置 = %d %s", response.Code, response.Body.String())
	}
}

type operationsReadAuthenticatorDouble struct {
	tenant domain.TenantID
	err    error
	token  string
}

func (double *operationsReadAuthenticatorDouble) AuthenticateOperationsRead(_ context.Context, bearerToken string) (domain.TenantID, error) {
	double.token = bearerToken
	return double.tenant, double.err
}

// Covers: ADR-0100 决定四、ADR-0293 决定一——操作者查阅 Intake 的租户只取自认证结果、页大小是渠道产品策略；认证失败原样交回、不构造查询。
func TestTheOperatorReadIntakeTakesTheRefusalQueryScopeFromTheEnvelopeOnly(t *testing.T) {
	authenticator := &operationsReadAuthenticatorDouble{tenant: mustValue(t, domain.NewTenantID, "TENANT-9")}
	intake, err := shipmenthttp.NewOperatorOperationsReadIntake(authenticator)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/control-release-refusals?tenant=TENANT-1&limit=1", nil)
	request.Header.Set("Authorization", "Bearer operator.token")
	query, err := intake.IntakeControlReleaseRefusalQuery(t.Context(), request)
	if err != nil {
		t.Fatalf("认证出的操作者查阅：%v", err)
	}
	if query.Tenant.String() != "TENANT-9" || query.Limit != 200 || authenticator.token != "operator.token" {
		t.Fatalf("查询 = %+v（令牌 %q）, want 信封里的 TENANT-9、页大小 200，请求里自报的租户与页大小一概不采信", query, authenticator.token)
	}

	authenticator.err = shipmenthttp.ErrOperatorNotGranted
	if _, err := intake.IntakeControlReleaseRefusalQuery(t.Context(), request); !errors.Is(err, shipmenthttp.ErrOperatorNotGranted) {
		t.Fatalf("认证失败 = %v, want 原样交回", err)
	}
}
