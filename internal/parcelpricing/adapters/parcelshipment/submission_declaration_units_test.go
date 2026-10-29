package parcelshipment_test

import (
	"context"
	"testing"

	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// Covers: ADR-0346 决定一——适配器只做单位符号的译码，不换算：申报的磅与英寸按原词、原值进计价的封闭单位表，
// 单位与方案查表单位不同由评价那一层换算（换算在 EvaluatePricing，适配器不替它算）。
func TestAPoundAndInchDeclarationIsHandedOnInItsOwnUnitsWithoutConversion(t *testing.T) {
	request := submittedRequest(t, []string{"parcel-1"}, postalCodes(t, "200000", "90210"),
		profileFor(t, "parcel-1", "5", "LB", "12", "10", "8", "IN"))
	declaration, found, err := newAdapter(t, &declarationViewDouble{request: request}).LoadSubmissionDeclaration(
		context.Background(), value(t, ppdomain.NewTenantID, "tenant-1"), "request-1", "version-1")
	if err != nil || !found || len(declaration.Parcels) != 1 {
		t.Fatalf("declaration = %+v found = %t err = %v", declaration, found, err)
	}
	parcel := declaration.Parcels[0]
	if parcel.Weight == nil || parcel.Weight.Value().String() != "5" || parcel.Weight.Unit() != ppdomain.WeightUnitPound {
		t.Fatalf("weight = %+v, 想要原样 5 LB", parcel.Weight)
	}
	if parcel.Dimensions == nil || parcel.Dimensions.Unit() != ppdomain.LengthUnitInch || parcel.Dimensions.LongestSide().Value().String() != "12" {
		t.Fatalf("dimensions = %+v, 想要原样 12 × 10 × 8 IN", parcel.Dimensions)
	}
}

// Covers: ADR-0346 决定六（本轮范围）——目录里有、计价封闭单位表里没有的单位（吨、毫米）仍当表外：评价只换算
// 计价表里的重量与长度单位，适配器的译码口径不随目录放宽，猜错单位估错的是要占用客户的那笔钱。
func TestAUnitInTheCatalogueButOutsideThePricingTableStaysUnmeasured(t *testing.T) {
	for name, profile := range map[string]psdomain.DeclaredParcelProfile{
		"重量单位吨":  profileFor(t, "parcel-1", "0.002", "T"),
		"外廓单位毫米": profileFor(t, "parcel-1", "3", "KG", "200", "150", "100", "MM"),
	} {
		t.Run(name, func(t *testing.T) {
			request := submittedRequest(t, []string{"parcel-1"}, postalCodes(t, "200000", "90210"), profile)
			declaration, found, err := newAdapter(t, &declarationViewDouble{request: request}).LoadSubmissionDeclaration(
				context.Background(), value(t, ppdomain.NewTenantID, "tenant-1"), "request-1", "version-1")
			if err != nil || !found || len(declaration.Parcels) != 1 {
				t.Fatalf("declaration = %+v found = %t err = %v", declaration, found, err)
			}
			if parcel := declaration.Parcels[0]; parcel.Weight != nil || parcel.Dimensions != nil {
				t.Fatalf("parcel = %+v, 想要整件当没有测量", parcel)
			}
		})
	}
}
