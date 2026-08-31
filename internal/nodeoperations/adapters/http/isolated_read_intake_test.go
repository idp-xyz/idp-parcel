package nodeopshttp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
)

// Covers: ADR-0078 Decision 二 — 作用域整组来自注入，请求里的自报被整个无视。
func TestIsolatedOperationsReadIntakeGrantsInjectedScope(t *testing.T) {
	intake, err := nodeopshttp.NewIsolatedOperationsReadIntake("SYN-SCOPE-ISOLATED-READ", "SYN-TENANT-01", 25)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/node-operations-records?registry=reception&tenant=TENANT-9", nil)
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
	if _, err := nodeopshttp.NewIsolatedOperationsReadIntake("", "SYN-TENANT-01", 25); err == nil {
		t.Fatal("空作用域引用被接受")
	}
	if _, err := nodeopshttp.NewIsolatedOperationsReadIntake("SYN-SCOPE-ISOLATED-READ", "", 25); err == nil {
		t.Fatal("空租户被接受")
	}
	if _, err := nodeopshttp.NewIsolatedOperationsReadIntake("SYN-SCOPE-ISOLATED-READ", "SYN-TENANT-01", 0); err == nil {
		t.Fatal("非正页大小被接受")
	}
}

// Covers: ADR-0078 Decision 二 — 注入式放行装进端点后，读口收到的仍是注入的那对键；
// 请求里的自报租户与页大小不起作用。
func TestIsolatedReadIntakeFeedsTheEndpointTheInjectedKeys(t *testing.T) {
	intake, err := nodeopshttp.NewIsolatedOperationsReadIntake("SYN-SCOPE-ISOLATED-READ", "SYN-TENANT-01", 7)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}

	register := &stubReviewCatalogue{}
	endpoint := nodeopshttp.NewQueryNodeOperationsRecordsEndpoint(intake, register)
	response := serveGet(endpoint, "/node-operations-records?registry=reception&tenant=TENANT-9&limit=9999")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	if register.gotTenant != "SYN-TENANT-01" || register.gotLimit != 7 {
		t.Fatalf("读口收到 tenant=%q limit=%d，want SYN-TENANT-01/7（自报被采信了）",
			register.gotTenant, register.gotLimit)
	}
}
