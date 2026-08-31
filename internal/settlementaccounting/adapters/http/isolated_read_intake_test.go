package settlementhttp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	settlementhttp "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/http"
)

// Covers: ADR-0078 Decision 二 — 作用域整组来自注入，请求里的自报被整个无视。
func TestIsolatedOperationsReadIntakeGrantsInjectedScope(t *testing.T) {
	intake, err := settlementhttp.NewIsolatedOperationsReadIntake("SYN-SCOPE-ISOLATED-READ", "SYN-TENANT-01", 25)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/settlement-charges?registry=customer-charge&tenant=TENANT-9", nil)
	request.Header.Set("X-Reported-Tenant", "TENANT-9")
	query, err := intake.IntakeCatalogueQuery(context.Background(), request)
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if got := query.Scope.Tenant().String(); got != "SYN-TENANT-01" {
		t.Fatalf("tenant = %q, want SYN-TENANT-01（注入值）", got)
	}
	if got := query.Scope.Reference().String(); got != "SYN-SCOPE-ISOLATED-READ" {
		t.Fatalf("scope reference = %q, want SYN-SCOPE-ISOLATED-READ（注入值）", got)
	}
	if query.Limit != 25 {
		t.Fatalf("limit = %d, want 25（注入值）", query.Limit)
	}
}

// Covers: ADR-0078 Decision 二 — 立不起来的注入在构造时拒，不等第一个请求。
func TestNewIsolatedOperationsReadIntakeRejectsBrokenInjection(t *testing.T) {
	if _, err := settlementhttp.NewIsolatedOperationsReadIntake("", "SYN-TENANT-01", 25); err == nil {
		t.Fatal("空作用域引用被接受")
	}
	if _, err := settlementhttp.NewIsolatedOperationsReadIntake("SYN-SCOPE-ISOLATED-READ", "", 25); err == nil {
		t.Fatal("空租户被接受")
	}
	if _, err := settlementhttp.NewIsolatedOperationsReadIntake("SYN-SCOPE-ISOLATED-READ", "SYN-TENANT-01", 0); err == nil {
		t.Fatal("非正页大小被接受")
	}
}

// Covers: ADR-0078 Decision 二 — 注入式放行装进四个端点后，读口收到的仍是注入的那对
// 键；四个端点共用一个 Intake，不会有哪一个偷偷读了请求。
func TestIsolatedReadIntakeFeedsEverySettlementEndpointTheInjectedKeys(t *testing.T) {
	intake, err := settlementhttp.NewIsolatedOperationsReadIntake("SYN-SCOPE-ISOLATED-READ", "SYN-TENANT-01", 7)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}

	charges := &stubChargeCatalogue{}
	statements := &stubStatementCatalogue{}
	funds := &stubFundsApplicationCatalogue{}
	operating := &stubOperatingCatalogue{}

	cases := []struct {
		name    string
		target  string
		handler http.Handler
		tenant  *string
		limit   *int
	}{
		{
			"charges", "/settlement-charges?registry=customer-charge&tenant=TENANT-9&limit=9999",
			settlementhttp.NewQuerySettlementChargesEndpoint(intake, charges),
			&charges.gotTenant, &charges.gotLimit,
		},
		{
			"statements", "/settlement-statements?registry=customer-statement&tenant=TENANT-9",
			settlementhttp.NewQuerySettlementStatementsEndpoint(intake, statements),
			&statements.gotTenant, &statements.gotLimit,
		},
		{
			"funds-applications", "/settlement-funds-applications?tenant=TENANT-9",
			settlementhttp.NewQuerySettlementFundsApplicationsEndpoint(intake, funds),
			&funds.gotTenant, &funds.gotLimit,
		},
		{
			"operating-results", "/settlement-operating-results?registry=operating-result&tenant=TENANT-9",
			settlementhttp.NewQuerySettlementOperatingResultsEndpoint(intake, operating),
			&operating.gotTenant, &operating.gotLimit,
		},
	}
	for _, endpoint := range cases {
		t.Run(endpoint.name, func(t *testing.T) {
			response := serveGet(endpoint.handler, endpoint.target)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
			}
			if *endpoint.tenant != "SYN-TENANT-01" || *endpoint.limit != 7 {
				t.Fatalf("读口收到 tenant=%q limit=%d，want SYN-TENANT-01/7（自报被采信了）",
					*endpoint.tenant, *endpoint.limit)
			}
		})
	}
}
