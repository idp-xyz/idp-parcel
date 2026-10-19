package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// Covers: ADR-0190 决定五的映射册半边——一行映射是（计价费用代码、费用项目、登记依据）三格，缺一不成；
// 任一格不同即不是同一份登记。
func TestAFeeItemMappingNeedsCodeFeeItemAndBasis(t *testing.T) {
	code := mustRef(t, domain.NewPricingChargeCode, "BASE_FREIGHT")
	feeItem := mustRef(t, domain.NewFeeItemReference, "SYN-FEE-FREIGHT")
	basis := mustRef(t, domain.NewFeeItemMappingBasisReference, "SYN-CONTRACT-CLAUSE-1")

	mapping, err := domain.NewFeeItemMapping(code, feeItem, basis)
	if err != nil {
		t.Fatalf("mapping: %v", err)
	}
	if mapping.ChargeCode() != code || mapping.FeeItem() != feeItem || mapping.Basis() != basis {
		t.Fatalf("mapping = %+v，想要原样保存三格", mapping)
	}

	for name, build := range map[string]func() (domain.FeeItemMapping, error){
		"缺计价费用代码": func() (domain.FeeItemMapping, error) {
			return domain.NewFeeItemMapping(domain.PricingChargeCode{}, feeItem, basis)
		},
		"缺费用项目": func() (domain.FeeItemMapping, error) {
			return domain.NewFeeItemMapping(code, domain.FeeItemReference{}, basis)
		},
		"缺登记依据": func() (domain.FeeItemMapping, error) {
			return domain.NewFeeItemMapping(code, feeItem, domain.FeeItemMappingBasisReference{})
		},
	} {
		if _, err := build(); !errors.Is(err, domain.ErrBlankValue) {
			t.Fatalf("%s err = %v，想要拒", name, err)
		}
	}
	for _, blank := range []string{"", " "} {
		if _, err := domain.NewPricingChargeCode(blank); err == nil {
			t.Fatalf("空白计价费用代码 %q 被收下了", blank)
		}
		if _, err := domain.NewFeeItemMappingBasisReference(blank); err == nil {
			t.Fatalf("空白登记依据 %q 被收下了", blank)
		}
	}

	same, _ := domain.NewFeeItemMapping(code, feeItem, basis)
	otherFeeItem, _ := domain.NewFeeItemMapping(code, mustRef(t, domain.NewFeeItemReference, "SYN-FEE-SURCHARGE"), basis)
	otherBasis, _ := domain.NewFeeItemMapping(code, feeItem, mustRef(t, domain.NewFeeItemMappingBasisReference, "SYN-CONTRACT-CLAUSE-2"))
	otherCode, _ := domain.NewFeeItemMapping(mustRef(t, domain.NewPricingChargeCode, "FUEL_SURCHARGE"), feeItem, basis)
	if !mapping.SameRegistration(same) {
		t.Fatal("同内容的两份不算同一份登记")
	}
	if mapping.SameRegistration(otherFeeItem) || mapping.SameRegistration(otherBasis) || mapping.SameRegistration(otherCode) {
		t.Fatal("换费用项目、换依据或换代码仍算同一份登记")
	}
}
