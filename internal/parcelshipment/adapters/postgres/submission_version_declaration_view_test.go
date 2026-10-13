package postgres_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件对真实 PostgreSQL 16 证 ports.SubmissionVersionDeclarationView（ADR-0179 决定四）：`已提交`的委托不经接受基线即按提交
// 版本答逐件测量与两段邮编；他租户、未知委托与未知版本同答不可见。

// Covers: 受理前的委托从快照答出那一版的原样申报——parcel-1 带画像、parcel-2 缺画像，两段邮编取委托级子段。
func TestASubmittedRequestAnswersItsVersionDeclarationFromTheSnapshot(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	ctx := t.Context()
	mustInsert(t, transactor, ctx, repository, submittedRequestWithElements(t, "req-key-1", "request-1"))

	declaration, found, err := repository.LoadSubmissionVersionDeclaration(ctx,
		mustBuild(t, domain.NewTenantID, "tenant-1"),
		mustBuild(t, domain.NewShipmentRequestID, "request-1"),
		mustBuild(t, domain.NewSubmissionVersionID, "version-1"))
	if err != nil || !found {
		t.Fatalf("found = %v err = %v, want the submitted version declaration", found, err)
	}
	parcels := declaration.Parcels()
	if len(parcels) != 2 {
		t.Fatalf("parcels = %d, want 2", len(parcels))
	}
	measurement, declared := parcels[0].Measurement()
	if !declared || measurement.Weight().Value().String() != "2.5" || measurement.Weight().Unit().String() != "kg" {
		t.Fatalf("parcel-1 measurement = %#v declared = %v, want 2.5 kg verbatim", measurement, declared)
	}
	if dimensions, present := measurement.Dimensions(); !present || dimensions.Length().String() != "30" {
		t.Fatalf("parcel-1 dimensions = %#v present = %v", dimensions, present)
	}
	if _, declared := parcels[1].Measurement(); declared {
		t.Fatal("parcel-2 没画像却读回了测量")
	}
	if origin, declared := declaration.OriginPostalCode(); !declared || origin != "SYN-200000" {
		t.Fatalf("origin = %q declared = %v", origin, declared)
	}
	if destination, declared := declaration.DestinationPostalCode(); !declared || destination != "SYN-100115" {
		t.Fatalf("destination = %q declared = %v", destination, declared)
	}
}

// Covers: 不可见统一答 false——他租户、未知委托、委托上没有的版本，三格不区分。
func TestAnInvisibleSubmissionVersionDeclarationAnswersNotFound(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	ctx := t.Context()
	mustInsert(t, transactor, ctx, repository, submittedRequestWithElements(t, "req-key-1", "request-1"))

	for name, key := range map[string][3]string{
		"他租户":  {"tenant-2", "request-1", "version-1"},
		"未知委托": {"tenant-1", "request-9", "version-1"},
		"未知版本": {"tenant-1", "request-1", "version-9"},
	} {
		_, found, err := repository.LoadSubmissionVersionDeclaration(ctx,
			mustBuild(t, domain.NewTenantID, key[0]),
			mustBuild(t, domain.NewShipmentRequestID, key[1]),
			mustBuild(t, domain.NewSubmissionVersionID, key[2]))
		if err != nil || found {
			t.Fatalf("%s: found = %v err = %v, want not found", name, found, err)
		}
	}

	if _, _, err := repository.LoadSubmissionVersionDeclaration(ctx,
		domain.TenantID{}, domain.ShipmentRequestID{}, domain.SubmissionVersionID{}); err == nil {
		t.Fatal("缺键的问话没被拒")
	}
}
