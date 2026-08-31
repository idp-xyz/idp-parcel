package tfhttp_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
)

// Covers: ADR-0078 — 隔离读准入是注入式放行：作用域与页大小整组来自装配点，请求里
// 的自报（查询串、头部）一概不读；它只进查阅面，命令面的 DeliveryIntake 不受影响。

func isolatedIntake(t *testing.T) tfhttp.IsolatedOperationsReadIntake {
	t.Helper()
	intake, err := tfhttp.NewIsolatedOperationsReadIntake("OPS-SCOPE-DEV", "TENANT-DEV", 50)
	if err != nil {
		t.Fatalf("构造隔离读准入：%v", err)
	}
	return intake
}

func TestIsolatedReadIntakeGrantsTheInjectedScopeIgnoringTheRequest(t *testing.T) {
	intake := isolatedIntake(t)
	request := httptest.NewRequest(http.MethodGet,
		"/transport-fulfillment-records?registry=transport-schedule&tenant=TENANT-EVIL&limit=999", nil)
	request.Header.Set("X-Tenant", "TENANT-EVIL")

	query, err := intake.IntakeCatalogueQuery(request.Context(), request)
	if err != nil {
		t.Fatalf("准入失败：%v", err)
	}
	if query.Scope.Tenant().String() != "TENANT-DEV" {
		t.Errorf("tenant = %q，自报值不该被采信", query.Scope.Tenant().String())
	}
	if query.Scope.Reference().String() != "OPS-SCOPE-DEV" {
		t.Errorf("reference = %q", query.Scope.Reference().String())
	}
	if query.Limit != 50 {
		t.Errorf("limit = %d，自报值不该被采信", query.Limit)
	}
}

func TestIsolatedReadIntakeRejectsUnassemblableConfiguration(t *testing.T) {
	cases := map[string]struct {
		reference, tenant string
		limit             int
	}{
		"空作用域引用": {"", "TENANT-DEV", 50},
		"空租户":    {"OPS-SCOPE-DEV", "", 50},
		"零页大小":   {"OPS-SCOPE-DEV", "TENANT-DEV", 0},
		"负页大小":   {"OPS-SCOPE-DEV", "TENANT-DEV", -1},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := tfhttp.NewIsolatedOperationsReadIntake(tc.reference, tc.tenant, tc.limit); err == nil {
				t.Error("立不起来的装配没有在构造时被拒")
			}
		})
	}
}

// Covers: 装进端点后整链放行：注入作用域直达读口，答案照册转写。
func TestIsolatedReadIntakeServesTheRecordsEndpoint(t *testing.T) {
	register := &stubReviewCatalogue{}
	endpoint := tfhttp.NewQueryTransportFulfillmentRecordsEndpoint(isolatedIntake(t), register)

	response := serveGet(endpoint, "/transport-fulfillment-records?registry=effective-delivery")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	if register.gotTenant != "TENANT-DEV" || register.gotLimit != 50 {
		t.Fatalf("读口收到的键走样：tenant=%q limit=%d", register.gotTenant, register.gotLimit)
	}
	if !strings.Contains(response.Body.String(), `"EFFECTIVE_DELIVERIES_LISTED"`) {
		t.Fatalf("响应走样：%s", response.Body.String())
	}
}
