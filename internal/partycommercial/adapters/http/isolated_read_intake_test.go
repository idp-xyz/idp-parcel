package commercialhttp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
)

// Covers: ADR-0078 Decision 二 — 作用域整组来自注入，请求里的自报被整个无视。
func TestIsolatedOperationsReadIntakeGrantsInjectedScope(t *testing.T) {
	intake, err := commercialhttp.NewIsolatedOperationsReadIntake("SYN-SCOPE-ISOLATED-READ", "SYN-TENANT-01", 25)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/commercial-service-products?tenant=TENANT-9", nil)
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
	if _, err := commercialhttp.NewIsolatedOperationsReadIntake("", "SYN-TENANT-01", 25); err == nil {
		t.Fatal("空作用域引用被接受")
	}
	if _, err := commercialhttp.NewIsolatedOperationsReadIntake("SYN-SCOPE-ISOLATED-READ", "", 25); err == nil {
		t.Fatal("空租户被接受")
	}
	if _, err := commercialhttp.NewIsolatedOperationsReadIntake("SYN-SCOPE-ISOLATED-READ", "SYN-TENANT-01", 0); err == nil {
		t.Fatal("非正页大小被接受")
	}
}
