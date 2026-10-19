package postgres_test

import (
	"testing"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件对真实 PostgreSQL 16 证协议范围读口（ADR-0208）：按「对象/版本」两段式指称读回那一版正文上的适用范围；指称
// 由 NewQualifiedVersionLabel 那一处拼；同对象的另一版、没有正文的版本、别的租户都答查无。

// Covers: 指称命中那一版正文，交回正文上的范围；同一对象的另一版各答各的。
func TestTheScopeOfASupplierAgreementVersionIsReadByItsQualifiedLabel(t *testing.T) {
	repository, transactor, db := newDeclarationFixture(t)
	ctx := t.Context()
	scopes, err := adapter.NewSupplierAgreementScopes(db)
	if err != nil {
		t.Fatalf("构造协议范围读口：%v", err)
	}

	first := effectiveVersionOfKind(t, domain.SupplierAgreementObject, "agreement-scoped", "v1", "digest-s1")
	mustSaveVersion(t, transactor, ctx, repository, first)
	mustSaveSupplierAgreement(t, transactor, ctx, repository, supplierAgreementOn(t, first, "supplier-1", "plan-buy-1"))
	bare := effectiveVersionOfKind(t, domain.SupplierAgreementObject, "agreement-scoped", "v2", "digest-s2")
	mustSaveVersion(t, transactor, ctx, repository, bare)

	scope, found, err := scopes.LoadSupplierAgreementScope(ctx, pcTenant(t, "tenant-1"), first.QualifiedLabel())
	if err != nil || !found || scope.String() != "scope-procurement" {
		t.Fatalf("v1 范围 = %q found=%v err=%v, want 正文上的 scope-procurement", scope, found, err)
	}
	for name, testCase := range map[string]struct {
		tenant string
		label  domain.CommercialVersionLabel
	}{
		"只有壳没有正文的那一版": {"tenant-1", bare.QualifiedLabel()},
		"别的租户":        {"tenant-2", first.QualifiedLabel()},
		"不是两段式的指称":    {"tenant-1", pcValue(t, domain.NewCommercialVersionLabel, "agreement-scoped")},
	} {
		if _, found, err := scopes.LoadSupplierAgreementScope(ctx, pcTenant(t, testCase.tenant), testCase.label); err != nil || found {
			t.Fatalf("%s: found=%v err=%v, want 查无且不报错", name, found, err)
		}
	}
}
