package commercialhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// 本文件对货主客户账户目录端点（票 admin-write-faces/04）证传输面：行体逐字段转写、
// 可缺席字段（参与方名称、停用两件）如实缺席、空册答空数组、读不回答 5xx、非 GET 拒在
// 方法门。判据与另两册身份端点同一套。

type customerAccountReaderDouble struct {
	tenant   domain.TenantID
	accounts []ports.CustomerAccountRow
	err      error
}

func (double *customerAccountReaderDouble) ListCustomerAccounts(
	_ context.Context, tenant domain.TenantID, _ int, _ cataloguepage.Query,
) (ports.CataloguePage[ports.CustomerAccountRow], error) {
	if double.err != nil {
		return ports.CataloguePage[ports.CustomerAccountRow]{}, double.err
	}
	if tenant != double.tenant {
		return ports.CataloguePage[ports.CustomerAccountRow]{}, nil
	}
	return ports.CataloguePage[ports.CustomerAccountRow]{
		Rows: double.accounts, Total: int64(len(double.accounts)),
	}, nil
}

var accountListedAt = time.Date(2026, 9, 3, 9, 0, 0, 0, time.UTC)

// Covers: 三级边界里唯一看不见的一级有了读面之后，生命周期三格与「名称是否查得到」都
// 要原样透出：已生效行带参与方名称、不带停用两件；已停用行的两件在场；悬空参与方的名称
// 如实缺席——布尔为假且无占位文本，页面按缺席显示，不拿空串去推。
func TestCustomerAccountsEndpointTranscribesTheLifecycleCellsAndTheDanglingParty(t *testing.T) {
	reader := &customerAccountReaderDouble{
		tenant: catValue(t, domain.NewTenantID, "tenant-1"),
		accounts: []ports.CustomerAccountRow{
			{
				TenantID:          "tenant-1",
				AccountID:         "account-effective",
				CustomerPartyID:   "party-cust",
				CustomerPartyName: "货主客户参与方",
				HasPartyName:      true,
				Status:            "EFFECTIVE",
				Revision:          1,
				Basis:             "basis-effective",
				EffectiveFrom:     accountListedAt,
				RegisteredAt:      accountListedAt,
			},
			{
				TenantID:        "tenant-1",
				AccountID:       "account-future",
				CustomerPartyID: "party-cust",
				Status:          "REGISTERED",
				Revision:        1,
				Basis:           "basis-future",
				EffectiveFrom:   accountListedAt.Add(72 * time.Hour),
				RegisteredAt:    accountListedAt,
			},
			{
				TenantID:          "tenant-1",
				AccountID:         "account-retired",
				CustomerPartyID:   "party-gone",
				Status:            "DEACTIVATED",
				Revision:          2,
				Basis:             "basis-retired",
				EffectiveFrom:     accountListedAt,
				DeactivatedAt:     accountListedAt.Add(time.Hour),
				DeactivationBasis: "basis-deact",
				HasDeactivation:   true,
				RegisteredAt:      accountListedAt,
			},
		},
	}
	endpoint := commercialhttp.NewQueryCustomerAccountsEndpoint(
		intakeDouble{query: catalogueQuery(t)}, reader)

	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/commercial-customer-accounts", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}

	var body struct {
		Outcome  string `json:"outcome"`
		Accounts []struct {
			TenantID               string `json:"tenantId"`
			AccountID              string `json:"accountId"`
			CustomerPartyID        string `json:"customerPartyId"`
			CustomerPartyName      string `json:"customerPartyName"`
			CustomerPartyNameKnown bool   `json:"customerPartyNameKnown"`
			Status                 string `json:"status"`
			Revision               int    `json:"revision"`
			Basis                  string `json:"basis"`
			EffectiveFrom          string `json:"effectiveFrom"`
			DeactivatedAt          string `json:"deactivatedAt"`
			DeactivationBasis      string `json:"deactivationBasis"`
			RegisteredAt           string `json:"registeredAt"`
		} `json:"accounts"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", recorder.Body.Bytes(), err)
	}
	if body.Outcome != "CUSTOMER_ACCOUNTS_LISTED" || len(body.Accounts) != 3 {
		t.Fatalf("body = %+v", body)
	}

	live := body.Accounts[0]
	if live.TenantID != "tenant-1" || live.AccountID != "account-effective" ||
		live.CustomerPartyID != "party-cust" || !live.CustomerPartyNameKnown ||
		live.CustomerPartyName != "货主客户参与方" || live.Status != "EFFECTIVE" ||
		live.Revision != 1 || live.Basis != "basis-effective" ||
		live.EffectiveFrom != accountListedAt.Format(time.RFC3339Nano) ||
		live.RegisteredAt != accountListedAt.Format(time.RFC3339Nano) ||
		live.DeactivatedAt != "" || live.DeactivationBasis != "" {
		t.Fatalf("已生效那一行 = %+v；未停用的行不该带停用两件", live)
	}
	if future := body.Accounts[1]; future.Status != "REGISTERED" || future.DeactivatedAt != "" {
		t.Fatalf("未来生效那一行 = %+v", future)
	}
	// 悬空参与方：名称如实缺席（布尔 false 且无占位文本）；停用两件在场。
	gone := body.Accounts[2]
	if gone.CustomerPartyNameKnown || gone.CustomerPartyName != "" ||
		gone.Status != "DEACTIVATED" || gone.Revision != 2 ||
		gone.DeactivationBasis != "basis-deact" ||
		gone.DeactivatedAt != accountListedAt.Add(time.Hour).Format(time.RFC3339Nano) {
		t.Fatalf("已停用那一行 = %+v", gone)
	}
}

// Covers: 空册是答案不是错误（ADR-0077 Decision 四），读不回才是 5xx；非 GET 拒在方法门。
// 空册要答空数组而不是 null——页面按数组长度分「目录为空」与「没拿到答案」两态。
func TestCustomerAccountsEndpointAnswersEmptyAndFailureApart(t *testing.T) {
	empty := commercialhttp.NewQueryCustomerAccountsEndpoint(
		intakeDouble{query: catalogueQuery(t)},
		&customerAccountReaderDouble{tenant: catValue(t, domain.NewTenantID, "tenant-1")})
	recorder := httptest.NewRecorder()
	empty.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/commercial-customer-accounts", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("空册 status = %d", recorder.Code)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(recorder.Body.Bytes(), &fields); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(fields["accounts"]) != "[]" {
		t.Fatalf("accounts = %s, want []", fields["accounts"])
	}

	failing := commercialhttp.NewQueryCustomerAccountsEndpoint(
		intakeDouble{query: catalogueQuery(t)},
		&customerAccountReaderDouble{err: errors.New("库连不上")})
	failed := httptest.NewRecorder()
	failing.ServeHTTP(failed, httptest.NewRequest(http.MethodGet, "/commercial-customer-accounts", nil))
	if failed.Code != http.StatusInternalServerError {
		t.Fatalf("读失败 status = %d, want 500（读不回不是空册）", failed.Code)
	}

	posted := httptest.NewRecorder()
	empty.ServeHTTP(posted, httptest.NewRequest(http.MethodPost, "/commercial-customer-accounts", nil))
	if posted.Code != http.StatusMethodNotAllowed || posted.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("POST status = %d, allow = %q", posted.Code, posted.Header().Get("Allow"))
	}
}
