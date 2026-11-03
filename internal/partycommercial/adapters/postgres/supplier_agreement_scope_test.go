package postgres_test

import (
	"context"
	"errors"
	"testing"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
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

// Covers: 对象标识或版本自带斜杠时，一份指称比中两版协议——报 ports.ErrSupplierAgreementLabelAmbiguous，不挑一行。消费方
// 凭这枚哨兵把它与读不动分开：前者重读永远同一个答案。
func TestAQualifiedLabelNamingTwoAgreementVersionsIsAmbiguous(t *testing.T) {
	repository, transactor, db := newDeclarationFixture(t)
	ctx := t.Context()
	scopes, err := adapter.NewSupplierAgreementScopes(db)
	if err != nil {
		t.Fatalf("构造协议范围读口：%v", err)
	}

	slashedObject := effectiveVersionOfKind(t, domain.SupplierAgreementObject, "agreement/a", "v1", "digest-a1")
	slashedVersion := effectiveVersionOfKind(t, domain.SupplierAgreementObject, "agreement", "a/v1", "digest-a2")
	for _, version := range []domain.CommercialVersion{slashedObject, slashedVersion} {
		mustSaveVersion(t, transactor, ctx, repository, version)
		mustSaveSupplierAgreement(t, transactor, ctx, repository, supplierAgreementOn(t, version, "supplier-1", "plan-buy-1"))
	}
	if slashedObject.QualifiedLabel() != slashedVersion.QualifiedLabel() {
		t.Fatalf("夹具没拼出同一个指称：%q / %q", slashedObject.QualifiedLabel(), slashedVersion.QualifiedLabel())
	}

	_, found, err := scopes.LoadSupplierAgreementScope(ctx, pcTenant(t, "tenant-1"), slashedObject.QualifiedLabel())
	if !errors.Is(err, ports.ErrSupplierAgreementLabelAmbiguous) || found {
		t.Fatalf("歧义指称：found=%v err=%v, want ErrSupplierAgreementLabelAmbiguous", found, err)
	}
}

// Covers: 供应商读口（ADR-0358 决定一）与范围读口同一种比法：指称命中那一版正文，交回正文上的供应商；同一对象的另一版各答
// 各的（后继版本可以声明另一家，读口不替它们统一）；只有壳的版本、别的租户、不是两段式的指称答查无。
func TestTheSupplierOfASupplierAgreementVersionIsReadByItsQualifiedLabel(t *testing.T) {
	repository, transactor, db := newDeclarationFixture(t)
	ctx := t.Context()
	suppliers, err := adapter.NewSupplierAgreementSuppliers(db)
	if err != nil {
		t.Fatalf("构造协议供应商读口：%v", err)
	}

	first := effectiveVersionOfKind(t, domain.SupplierAgreementObject, "agreement-supplied", "v1", "digest-p1")
	mustSaveVersion(t, transactor, ctx, repository, first)
	mustSaveSupplierAgreement(t, transactor, ctx, repository, supplierAgreementOn(t, first, "supplier-1", "plan-buy-1"))
	second := effectiveVersionOfKind(t, domain.SupplierAgreementObject, "agreement-supplied", "v2", "digest-p2")
	mustSaveVersion(t, transactor, ctx, repository, second)
	mustSaveSupplierAgreement(t, transactor, ctx, repository, supplierAgreementOn(t, second, "supplier-2", "plan-buy-1"))
	bare := effectiveVersionOfKind(t, domain.SupplierAgreementObject, "agreement-supplied", "v3", "digest-p3")
	mustSaveVersion(t, transactor, ctx, repository, bare)

	for label, want := range map[domain.CommercialVersionLabel]string{
		first.QualifiedLabel():  "supplier-1",
		second.QualifiedLabel(): "supplier-2",
	} {
		supplier, found, err := suppliers.LoadSupplierAgreementSupplier(ctx, pcTenant(t, "tenant-1"), label)
		if err != nil || !found || supplier.String() != want {
			t.Fatalf("%s 的供应商 = %q found=%v err=%v, want %s", label, supplier, found, err, want)
		}
	}
	for name, testCase := range map[string]struct {
		tenant string
		label  domain.CommercialVersionLabel
	}{
		"只有壳没有正文的那一版": {"tenant-1", bare.QualifiedLabel()},
		"别的租户":        {"tenant-2", first.QualifiedLabel()},
		"不是两段式的指称":    {"tenant-1", pcValue(t, domain.NewCommercialVersionLabel, "agreement-supplied")},
	} {
		if _, found, err := suppliers.LoadSupplierAgreementSupplier(ctx, pcTenant(t, testCase.tenant), testCase.label); err != nil || found {
			t.Fatalf("%s: found=%v err=%v, want 查无且不报错", name, found, err)
		}
	}
}

// Covers: 一份指称比中两版协议时，供应商读口同范围读口一样报 ports.ErrSupplierAgreementLabelAmbiguous，不挑一家。
func TestAnAmbiguousLabelNamesNoSupplier(t *testing.T) {
	repository, transactor, db := newDeclarationFixture(t)
	ctx := t.Context()
	suppliers, err := adapter.NewSupplierAgreementSuppliers(db)
	if err != nil {
		t.Fatalf("构造协议供应商读口：%v", err)
	}
	slashedObject := effectiveVersionOfKind(t, domain.SupplierAgreementObject, "agreement/b", "v1", "digest-b1")
	slashedVersion := effectiveVersionOfKind(t, domain.SupplierAgreementObject, "agreement", "b/v1", "digest-b2")
	for _, version := range []domain.CommercialVersion{slashedObject, slashedVersion} {
		mustSaveVersion(t, transactor, ctx, repository, version)
		mustSaveSupplierAgreement(t, transactor, ctx, repository, supplierAgreementOn(t, version, "supplier-1", "plan-buy-1"))
	}
	_, found, err := suppliers.LoadSupplierAgreementSupplier(ctx, pcTenant(t, "tenant-1"), slashedObject.QualifiedLabel())
	if !errors.Is(err, ports.ErrSupplierAgreementLabelAmbiguous) || found {
		t.Fatalf("歧义指称：found=%v err=%v, want ErrSupplierAgreementLabelAmbiguous", found, err)
	}
}

// Covers: 库里那一版正文存的供应商立不成往来单位标识（库上 btrim 只削空格，制表符过得了检查）——报
// ports.ErrSupplierAgreementSupplierMalformed，与读不动分开，消费方凭它把那一版当确定性的坏答案（ADR-0358 决定三）。
func TestAMalformedStoredSupplierIsReportedAsSuch(t *testing.T) {
	repository, transactor, db := newDeclarationFixture(t)
	ctx := t.Context()
	suppliers, err := adapter.NewSupplierAgreementSuppliers(db)
	if err != nil {
		t.Fatalf("构造协议供应商读口：%v", err)
	}
	version := effectiveVersionOfKind(t, domain.SupplierAgreementObject, "agreement-malformed", "v1", "digest-m1")
	mustSaveVersion(t, transactor, ctx, repository, version)
	mustSaveSupplierAgreement(t, transactor, ctx, repository, supplierAgreementOn(t, version, "supplier-1", "plan-buy-1"))
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		executor, err := db.RequireExecutor(txCtx)
		if err != nil {
			return err
		}
		_, err = executor.Exec(txCtx, `UPDATE party_commercial.supplier_agreement SET supplier_party_id = E'\t' WHERE object_id = 'agreement-malformed'`)
		return err
	}); err != nil {
		t.Fatalf("把供应商改成立不成标识的值：%v", err)
	}
	_, found, err := suppliers.LoadSupplierAgreementSupplier(ctx, pcTenant(t, "tenant-1"), version.QualifiedLabel())
	if !errors.Is(err, ports.ErrSupplierAgreementSupplierMalformed) || found {
		t.Fatalf("存的供应商立不成标识：found=%v err=%v, want ErrSupplierAgreementSupplierMalformed", found, err)
	}
}
