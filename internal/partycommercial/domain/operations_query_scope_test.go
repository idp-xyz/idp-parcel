package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// Covers: ADR-0077 Decision 二——运营查阅作用域(作用域引用,租户)两维非零,缺一即拒。
// 空作用域立不起来,读口就不必各自决定「空租户」意味着什么。

func TestOperationsQueryScopeCarriesReferenceAndTenant(t *testing.T) {
	reference, err := domain.NewOperationsScopeReference("ops-scope-1")
	if err != nil {
		t.Fatalf("作用域引用:%v", err)
	}
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("租户:%v", err)
	}

	scope, err := domain.NewOperationsQueryScope(reference, tenant)
	if err != nil {
		t.Fatalf("构造作用域:%v", err)
	}
	if scope.Reference() != reference {
		t.Fatalf("reference = %q", scope.Reference())
	}
	if scope.Tenant() != tenant {
		t.Fatalf("tenant = %q", scope.Tenant())
	}
}

func TestOperationsQueryScopeRejectsMissingDimensions(t *testing.T) {
	reference, err := domain.NewOperationsScopeReference("ops-scope-1")
	if err != nil {
		t.Fatalf("作用域引用:%v", err)
	}
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("租户:%v", err)
	}

	if _, err := domain.NewOperationsQueryScope(domain.OperationsScopeReference{}, tenant); !errors.Is(err, domain.ErrInvalidOperationsQueryScope) {
		t.Fatalf("缺作用域引用未被拒:%v", err)
	}
	if _, err := domain.NewOperationsQueryScope(reference, domain.TenantID{}); !errors.Is(err, domain.ErrInvalidOperationsQueryScope) {
		t.Fatalf("缺租户未被拒:%v", err)
	}
}
