package domain

import (
	"errors"
	"testing"
)

// Covers: ADR-0077 Decision 二 — 运营作用域(作用域引用,租户)两维、两样必须非零。
func TestOperationsQueryScopeRequiresBothDimensions(t *testing.T) {
	reference, err := NewOperationsScopeReference("OPS-SCOPE-1")
	if err != nil {
		t.Fatalf("NewOperationsScopeReference: %v", err)
	}
	tenant, err := NewTenantID("TENANT-1")
	if err != nil {
		t.Fatalf("NewTenantID: %v", err)
	}

	scope, err := NewOperationsQueryScope(reference, tenant)
	if err != nil {
		t.Fatalf("NewOperationsQueryScope: %v", err)
	}
	if scope.Reference() != reference || scope.Tenant() != tenant {
		t.Fatalf("scope = %+v, want reference %v tenant %v", scope, reference, tenant)
	}

	// 空租户拒构造:授权能力答不出租户时该在接入处拒绝,不造空作用域。
	if _, err := NewOperationsQueryScope(reference, TenantID{}); !errors.Is(err, ErrInvalidOperationsQueryScope) {
		t.Fatalf("blank tenant: err = %v, want ErrInvalidOperationsQueryScope", err)
	}
	if _, err := NewOperationsQueryScope(OperationsScopeReference{}, tenant); !errors.Is(err, ErrInvalidOperationsQueryScope) {
		t.Fatalf("blank reference: err = %v, want ErrInvalidOperationsQueryScope", err)
	}
}

// Covers: ADR-0077 Decision 二 — 空白串等同缺席:引用与租户都以非空白为有效判据。
func TestOperationsScopeReferenceRejectsBlankValue(t *testing.T) {
	if _, err := NewOperationsScopeReference("   "); err == nil {
		t.Fatal("blank operations scope reference must be rejected")
	}
}
