package parcelshipment_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/parcelshipment"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件证消费侧适配器 AcceptedServiceProducts 与 SubmissionDeclarations 把申报的服务产品译成计价的产品引用（ADR-0382 决定三）：值照搬；没申报是零值；写成计价引用立不起来的值
// （首尾空白）任何一张卡都指名不了它，折成零值——与没申报落在同一层；读面坏了照报。

type acceptedProductViewDouble struct {
	product psdomain.DeclaredServiceProduct
	err     error
}

func (double acceptedProductViewDouble) LoadAcceptedServiceProduct(context.Context, psdomain.TenantID, psdomain.DeclaredParcelID) (psdomain.DeclaredServiceProduct, error) {
	return double.product, double.err
}

func declaredProduct(t *testing.T, raw string) psdomain.DeclaredServiceProduct {
	t.Helper()
	entry, err := psdomain.NewCanonicalContentEntry(psdomain.RequestedServiceProductEntryName, raw)
	if err != nil {
		t.Fatalf("content entry: %v", err)
	}
	return psdomain.RequestedServiceProductOf([]psdomain.CanonicalContentEntry{entry})
}

func TestTheAcceptedServiceProductIsTranslatedAsIsOrFoldedToNone(t *testing.T) {
	tenant := value(t, ppdomain.NewTenantID, "tenant-1")
	for name, test := range map[string]struct {
		declared psdomain.DeclaredServiceProduct
		want     string
	}{
		"申报了":   {declaredProduct(t, "SYN-PROD-CN-SG-FBA"), "SYN-PROD-CN-SG-FBA"},
		"没申报":   {psdomain.DeclaredServiceProduct{}, ""},
		"带首尾空白": {declaredProduct(t, " SYN-PROD-CN-SG-FBA "), ""},
	} {
		adapter, err := parcelshipment.NewAcceptedServiceProducts(acceptedProductViewDouble{product: test.declared})
		if err != nil {
			t.Fatal(err)
		}
		product, err := adapter.LoadAcceptedServiceProduct(context.Background(), tenant, "parcel-1")
		if err != nil || product.String() != test.want || product.Declared() != (test.want != "") {
			t.Fatalf("%s：%q err = %v，want %q", name, product.String(), err, test.want)
		}
	}

	broken, _ := parcelshipment.NewAcceptedServiceProducts(acceptedProductViewDouble{err: errors.New("read face down")})
	if _, err := broken.LoadAcceptedServiceProduct(context.Background(), tenant, "parcel-1"); err == nil {
		t.Fatal("读面坏了却答了没有产品")
	}
	if _, err := parcelshipment.NewAcceptedServiceProducts(nil); err == nil {
		t.Fatal("nil 读口被收下了")
	}
}

func TestTheSubmissionDeclarationCarriesTheVersionsServiceProduct(t *testing.T) {
	request := submittedRequestWithProduct(t, "SYN-PROD-CN-SG-FBA")
	declaration, found, err := newAdapter(t, &declarationViewDouble{request: request}).LoadSubmissionDeclaration(context.Background(),
		value(t, ppdomain.NewTenantID, "tenant-1"), "request-1", "version-1")
	if err != nil || !found || declaration.ServiceProduct.String() != "SYN-PROD-CN-SG-FBA" {
		t.Fatalf("申报的产品 = %q found = %v err = %v", declaration.ServiceProduct.String(), found, err)
	}

	plain := submittedRequest(t, []string{"parcel-1"}, postalCodes(t, "200000", "90210"), profileFor(t, "parcel-1", "1", "KG"))
	declaration, _, _ = newAdapter(t, &declarationViewDouble{request: plain}).LoadSubmissionDeclaration(context.Background(),
		value(t, ppdomain.NewTenantID, "tenant-1"), "request-1", "version-1")
	if declaration.ServiceProduct.Declared() {
		t.Fatalf("没申报却带了产品 %q", declaration.ServiceProduct.String())
	}
}
