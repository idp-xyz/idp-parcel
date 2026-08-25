package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
)

// Covers: ADR-0077 Decision 二 — (作用域引用,租户)两维非零;两维俱备时作用域成立,
// 两个访问器逐维交回原值。
func TestOperationsQueryScopeCarriesReferenceAndTenant(t *testing.T) {
	reference := mustValue(t, domain.NewOperationsScopeReference, "ops-scope-1")
	tenant := mustValue(t, domain.NewTenantID, "tenant-1")

	scope, err := domain.NewOperationsQueryScope(reference, tenant)
	if err != nil {
		t.Fatalf("构造作用域：%v", err)
	}
	if scope.Reference().String() != "ops-scope-1" || scope.Tenant().String() != "tenant-1" {
		t.Fatalf("作用域走样：reference=%q tenant=%q", scope.Reference(), scope.Tenant())
	}
}

// Covers: NewOperationsQueryScope 的拒绝半边 — 空引用或空租户都构造不出作用域:
// 授权能力答不出租户时该在接入处拒绝,不造空作用域让读口各自解释。
func TestOperationsQueryScopeRejectsMissingDimensions(t *testing.T) {
	reference := mustValue(t, domain.NewOperationsScopeReference, "ops-scope-1")
	tenant := mustValue(t, domain.NewTenantID, "tenant-1")

	if _, err := domain.NewOperationsQueryScope(domain.OperationsScopeReference{}, tenant); !errors.Is(err, domain.ErrInvalidOperationsQueryScope) {
		t.Fatalf("空引用应拒绝，实得：%v", err)
	}
	if _, err := domain.NewOperationsQueryScope(reference, domain.TenantID{}); !errors.Is(err, domain.ErrInvalidOperationsQueryScope) {
		t.Fatalf("空租户应拒绝，实得：%v", err)
	}
}
