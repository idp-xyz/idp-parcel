package partycommercial_test

import (
	"context"
	"errors"
	"testing"

	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/partycommercial"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 本文件证协议范围那一缝的翻译（ADR-0208 决定四）：发生项上的协议指称原样交给 party-commercial，协议的商业范围原样
// 译成主要范围；查无交 found=false 让编排判矛盾，读不动原样上抛让编排答未决。

type agreementScopeSourceDouble struct {
	asked  string
	scope  string
	found  bool
	err    error
	tenant string
}

func (double *agreementScopeSourceDouble) LoadSupplierAgreementScope(
	_ context.Context, tenant pcdomain.TenantID, agreement pcdomain.CommercialVersionLabel,
) (pcdomain.CommercialScopeReference, bool, error) {
	double.asked, double.tenant = agreement.String(), tenant.String()
	if double.err != nil || !double.found {
		return pcdomain.CommercialScopeReference{}, false, double.err
	}
	scope, err := pcdomain.NewCommercialScopeReference(double.scope)
	return scope, err == nil, err
}

func scopeAdapter(t *testing.T, source *agreementScopeSourceDouble) *adapter.SupplierAgreementScope {
	t.Helper()
	built, err := adapter.NewSupplierAgreementScope(source)
	if err != nil {
		t.Fatalf("构造协议范围适配器：%v", err)
	}
	return built
}

func saAgreement(t *testing.T, value string) sadomain.SupplierAgreementReference {
	t.Helper()
	agreement, err := sadomain.NewSupplierAgreementReference(value)
	if err != nil {
		t.Fatal(err)
	}
	return agreement
}

func TestTheAgreementScopeBecomesThePrimaryScope(t *testing.T) {
	source := &agreementScopeSourceDouble{scope: "SYN-SCOPE-01", found: true}
	scope, found, err := scopeAdapter(t, source).LoadSupplierAgreementScope(
		t.Context(), saTenant(t, "SYN-TENANT-01"), saAgreement(t, "SYN-SUPPLIER-TRUNK-CN-SG-01/v1"))
	if err != nil || !found || scope.String() != "SYN-SCOPE-01" {
		t.Fatalf("范围 = %q found=%v err=%v", scope, found, err)
	}
	if source.asked != "SYN-SUPPLIER-TRUNK-CN-SG-01/v1" || source.tenant != "SYN-TENANT-01" {
		t.Fatalf("问 PC 的是 %q / %q，want 发生项上的协议指称原样", source.tenant, source.asked)
	}
}

func TestAnUnknownAgreementIsNotFoundAndAReadFailureIsReturned(t *testing.T) {
	if _, found, err := scopeAdapter(t, &agreementScopeSourceDouble{}).LoadSupplierAgreementScope(
		t.Context(), saTenant(t, "SYN-TENANT-01"), saAgreement(t, "SYN-UNKNOWN/v1")); err != nil || found {
		t.Fatalf("查无：found=%v err=%v", found, err)
	}
	down := errors.New("party commercial down")
	if _, _, err := scopeAdapter(t, &agreementScopeSourceDouble{err: down}).LoadSupplierAgreementScope(
		t.Context(), saTenant(t, "SYN-TENANT-01"), saAgreement(t, "SYN-SUPPLIER-TRUNK-CN-SG-01/v1")); !errors.Is(err, down) {
		t.Fatalf("读不动 err = %v", err)
	}
	if _, err := adapter.NewSupplierAgreementScope(nil); err == nil {
		t.Fatal("读口缺席却构造成功")
	}
}
