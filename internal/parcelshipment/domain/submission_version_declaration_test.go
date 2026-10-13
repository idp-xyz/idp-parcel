package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// Covers: ADR-0179 决定四——按提交版本取申报，不经接受基线：`已提交`的委托逐件答测量（缺画像的那件如实缺席），两段邮编取这一版
// 的委托级子段；版本不在本委托上答 false。
func TestASubmittedVersionAnswersItsDeclarationPerParcelWithoutABaseline(t *testing.T) {
	spec := submitSpec(t, "parcel-1", "parcel-2")
	spec.Profiles = []domain.DeclaredParcelProfile{profileOf(t, "parcel-1", "1.50")}
	spec.Elements = domain.NewDeclaredAddressElements(
		domain.AddressElementsOf(domain.SenderPlaceDataGroup(), []domain.CanonicalContentEntry{
			addressEntry(t, domain.SenderPlaceDataGroup(), domain.PostalCodeElement, "SYN-200000"),
		}),
		domain.AddressElementsOf(domain.DeliveryPlaceDataGroup(), []domain.CanonicalContentEntry{
			addressEntry(t, domain.DeliveryPlaceDataGroup(), domain.PostalCodeElement, "SYN-100115"),
		}),
	)
	request, err := domain.SubmitShipmentRequest(spec)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	declaration, found := request.SubmissionVersionDeclarationFor(mustValue(t, domain.NewSubmissionVersionID, "version-1"))
	if !found {
		t.Fatal("已提交委托的当前版本读不到申报")
	}
	parcels := declaration.Parcels()
	if len(parcels) != 2 || parcels[0].Parcel().String() != "parcel-1" || parcels[1].Parcel().String() != "parcel-2" {
		t.Fatalf("parcels = %#v, want parcel-1, parcel-2 in member order", parcels)
	}
	if measurement, declared := parcels[0].Measurement(); !declared || measurement.Weight().Value().String() != "1.50" {
		t.Fatalf("parcel-1 measurement = %#v declared = %v", measurement, declared)
	}
	if _, declared := parcels[1].Measurement(); declared {
		t.Fatal("没申报测量的那件凭空有了测量")
	}
	if origin, declared := declaration.OriginPostalCode(); !declared || origin != "SYN-200000" {
		t.Fatalf("origin = %q declared = %v", origin, declared)
	}
	if destination, declared := declaration.DestinationPostalCode(); !declared || destination != "SYN-100115" {
		t.Fatalf("destination = %q declared = %v", destination, declared)
	}

	if _, found := request.SubmissionVersionDeclarationFor(mustValue(t, domain.NewSubmissionVersionID, "version-9")); found {
		t.Fatal("不在本委托上的版本答出了申报")
	}
}

// Covers: 客户没报邮编是常态——两段都缺席时如实答缺，不顶默认。
func TestAVersionWithoutAddressElementsAnswersBothPostalCodesAbsent(t *testing.T) {
	request, err := domain.SubmitShipmentRequest(submitSpec(t, "parcel-1"))
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	declaration, found := request.SubmissionVersionDeclarationFor(mustValue(t, domain.NewSubmissionVersionID, "version-1"))
	if !found {
		t.Fatal("当前版本读不到申报")
	}
	if _, declared := declaration.OriginPostalCode(); declared {
		t.Fatal("没报寄件邮编却在场")
	}
	if _, declared := declaration.DestinationPostalCode(); declared {
		t.Fatal("没报收件邮编却在场")
	}
}
