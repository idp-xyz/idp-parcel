package partycommercial_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/partycommercial"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 本文件证协议供应商那一缝的翻译（ADR-0358 决定一）：成本所依的协议指称原样交给 party-commercial，协议正文上的往来单位标识
// 原样译成账单上的供应商引用；查无交 found=false，读不动原样上抛，指称歧义包确定性哨兵。

type agreementSupplierSourceDouble struct {
	asked, tenant string
	supplier      string
	found         bool
	err           error
}

func (double *agreementSupplierSourceDouble) LoadSupplierAgreementSupplier(
	_ context.Context, tenant pcdomain.TenantID, agreement pcdomain.CommercialVersionLabel,
) (pcdomain.PartyID, bool, error) {
	double.asked, double.tenant = agreement.String(), tenant.String()
	if double.err != nil || !double.found {
		return pcdomain.PartyID{}, false, double.err
	}
	party, err := pcdomain.NewPartyID(double.supplier)
	return party, err == nil, err
}

func supplierAdapter(t *testing.T, source *agreementSupplierSourceDouble) *adapter.SupplierAgreementSupplier {
	t.Helper()
	built, err := adapter.NewSupplierAgreementSupplier(source)
	if err != nil {
		t.Fatalf("构造协议供应商适配器：%v", err)
	}
	return built
}

func TestTheAgreementSupplierBecomesTheBillSupplierReference(t *testing.T) {
	source := &agreementSupplierSourceDouble{supplier: "SYN-PARTY-CARRIER-TRUNK-01", found: true}
	supplier, found, err := supplierAdapter(t, source).LoadSupplierAgreementSupplier(
		t.Context(), saTenant(t, "SYN-TENANT-01"), saAgreement(t, "SYN-SUPPLIER-TRUNK-CN-SG-01/v1"))
	if err != nil || !found || supplier.String() != "SYN-PARTY-CARRIER-TRUNK-01" {
		t.Fatalf("供应商 = %q found=%v err=%v", supplier, found, err)
	}
	if source.asked != "SYN-SUPPLIER-TRUNK-CN-SG-01/v1" || source.tenant != "SYN-TENANT-01" {
		t.Fatalf("问 PC 的是 %q / %q，want 成本所依的协议指称原样", source.tenant, source.asked)
	}
}

func TestAnUnknownAgreementHasNoSupplierAndAReadFailureIsReturned(t *testing.T) {
	if _, found, err := supplierAdapter(t, &agreementSupplierSourceDouble{}).LoadSupplierAgreementSupplier(
		t.Context(), saTenant(t, "SYN-TENANT-01"), saAgreement(t, "SYN-UNKNOWN/v1")); err != nil || found {
		t.Fatalf("查无：found=%v err=%v", found, err)
	}
	down := errors.New("party commercial down")
	if _, _, err := supplierAdapter(t, &agreementSupplierSourceDouble{err: down}).LoadSupplierAgreementSupplier(
		t.Context(), saTenant(t, "SYN-TENANT-01"), saAgreement(t, "SYN-SUPPLIER-TRUNK-CN-SG-01/v1")); !errors.Is(err, down) ||
		errors.Is(err, saports.ErrSupplierAgreementSupplierUntranslatable) {
		t.Fatalf("读不动 err = %v, want 原样上抛、不带确定性哨兵", err)
	}
	if _, err := adapter.NewSupplierAgreementSupplier(nil); err == nil {
		t.Fatal("读口缺席却构造成功")
	}
}

// Covers: PC 答指称比中不止一版——包 saports.ErrSupplierAgreementSupplierUntranslatable，编排凭它上抛、补记凭它留空列出。
func TestAnAmbiguousAgreementLabelNamesNoSupplier(t *testing.T) {
	ambiguous := fmt.Errorf("load supplier agreement supplier: %w", pcports.ErrSupplierAgreementLabelAmbiguous)
	_, found, err := supplierAdapter(t, &agreementSupplierSourceDouble{err: ambiguous}).LoadSupplierAgreementSupplier(
		t.Context(), saTenant(t, "SYN-TENANT-01"), saAgreement(t, "SYN-SUPPLIER/TRUNK/v1"))
	if !errors.Is(err, saports.ErrSupplierAgreementSupplierUntranslatable) || found {
		t.Fatalf("歧义指称：found=%v err=%v, want ErrSupplierAgreementSupplierUntranslatable", found, err)
	}
}
