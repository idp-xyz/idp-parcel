package shipmenthttp_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
)

// Covers: ADR-0078 Decision 二 — 作用域整组（含可见客户账户维）来自注入，请求里的
// 自报被整个无视。命令 Intake 装不进本类型由编译期决定（isolated_read_intake.go 的
// 类型断言只声明 ShipmentRequestViewsIntake）。
func TestIsolatedOperationsReadIntakeGrantsInjectedScope(t *testing.T) {
	intake, err := shipmenthttp.NewIsolatedOperationsReadIntake(
		"SYN-SCOPE-ISOLATED-READ", "SYN-TENANT-01", []string{"SYN-ACCOUNT-01"}, 25)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/shipment-request-views?tenant=TENANT-9", nil)
	request.Header.Set("X-Reported-Tenant", "TENANT-9")
	query, err := intake.IntakeListQuery(context.Background(), request)
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if got := query.Scope.TenantID().String(); got != "SYN-TENANT-01" {
		t.Fatalf("tenant = %q, want SYN-TENANT-01（注入值）", got)
	}
	accounts := query.Scope.CustomerAccountIDs()
	if len(accounts) != 1 || accounts[0].String() != "SYN-ACCOUNT-01" {
		t.Fatalf("accounts = %v, want 注入的 [SYN-ACCOUNT-01]", accounts)
	}
	if query.Limit != 25 {
		t.Fatalf("limit = %d, want 25（注入值）", query.Limit)
	}
}

// Covers: ADR-0078 Decision 二 — 定位标识属传输形状：详情查询解析 shipmentRequestId
// 只为定位候选对象，权限仍在注入的作用域上；坏定位按坏请求拒，不折成授权问题。
func TestIsolatedOperationsReadIntakeDetailParsesLocatorOnly(t *testing.T) {
	intake, err := shipmenthttp.NewIsolatedOperationsReadIntake(
		"SYN-SCOPE-ISOLATED-READ", "SYN-TENANT-01", []string{"SYN-ACCOUNT-01"}, 25)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}

	good := httptest.NewRequest(http.MethodGet, "/shipment-request-views?shipmentRequestId=REQ-1", nil)
	query, err := intake.IntakeDetailQuery(context.Background(), good)
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if got := query.RequestID.String(); got != "REQ-1" {
		t.Fatalf("requestID = %q, want REQ-1（定位参数）", got)
	}
	if got := query.Scope.TenantID().String(); got != "SYN-TENANT-01" {
		t.Fatalf("tenant = %q, want SYN-TENANT-01（注入值，不来自请求）", got)
	}

	bad := httptest.NewRequest(http.MethodGet, "/shipment-request-views?shipmentRequestId=", nil)
	if _, err := intake.IntakeDetailQuery(context.Background(), bad); !errors.Is(err, shipmenthttp.ErrMalformedRequest) {
		t.Fatalf("空定位标识：err = %v, want ErrMalformedRequest", err)
	}
}

// Covers: ADR-0078 Decision 二 — 立不起来的注入在构造时拒，不等第一个请求。
func TestNewIsolatedOperationsReadIntakeRejectsBrokenInjection(t *testing.T) {
	if _, err := shipmenthttp.NewIsolatedOperationsReadIntake(
		"", "SYN-TENANT-01", []string{"SYN-ACCOUNT-01"}, 25); err == nil {
		t.Fatal("空作用域引用被接受")
	}
	if _, err := shipmenthttp.NewIsolatedOperationsReadIntake(
		"SYN-SCOPE-ISOLATED-READ", "", []string{"SYN-ACCOUNT-01"}, 25); err == nil {
		t.Fatal("空租户被接受")
	}
	if _, err := shipmenthttp.NewIsolatedOperationsReadIntake(
		"SYN-SCOPE-ISOLATED-READ", "SYN-TENANT-01", nil, 25); err == nil {
		t.Fatal("空可见账户集被接受：授权能力答「什么都看不见」该在接入处拒")
	}
	if _, err := shipmenthttp.NewIsolatedOperationsReadIntake(
		"SYN-SCOPE-ISOLATED-READ", "SYN-TENANT-01", []string{"SYN-ACCOUNT-01"}, 0); err == nil {
		t.Fatal("非正页大小被接受")
	}
}
