package postgres_test

import (
	"errors"
	"testing"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件对真实 PostgreSQL 16 证 ports.CommercialResolutionReferenceView（票 ps-port-remainder/07 完成判据 2）：
// 已接受带回指 → 回指且同一委托两个成员同一个；从未声明 / 仅已提交 / 他租户 → 没有；已接受而快照上的决定缺
// 解析标识 → error（读面坏了，不折成没有）；两份已接受同时声明一件包裹 → 歧义 error。夹具走真接受（Decide → Save），
// 与 06 的读面同一条取行 + 重建的路。

var _ ports.CommercialResolutionReferenceView = (*adapter.ShipmentRequests)(nil)

func loadCommercialReference(
	t *testing.T,
	view ports.CommercialResolutionReferenceView,
	tenant, parcel string,
) (domain.CommercialResolutionID, bool) {
	t.Helper()
	reference, present, err := view.LoadCommercialResolutionReference(t.Context(),
		mustBuild(t, domain.NewTenantID, tenant), mustBuild(t, domain.NewDeclaredParcelID, parcel))
	if err != nil {
		t.Fatalf("取商业解析回指（%s / %s）：%v", tenant, parcel, err)
	}
	return reference, present
}

func TestAMemberOfAnAcceptedRequestGetsItsCommercialResolutionReference(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	acceptedRequestWithDeliveryPlaceChain(t, repository, transactor, "req-key-1", "request-1")

	first, present := loadCommercialReference(t, repository, "tenant-1", "parcel-1")
	if !present || first.String() != "RES-1" {
		t.Fatalf("reference = %q present = %v, want RES-1", first, present)
	}
	second, _ := loadCommercialReference(t, repository, "tenant-1", "parcel-2")
	if second != first {
		t.Fatalf("同一委托两个成员的回指不同：%q vs %q", first, second)
	}
}

func TestSourceDataAmendmentsLeaveTheCommercialResolutionReferenceUnchanged(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	acceptedRequestWithDeliveryPlaceChain(t, repository, transactor, "req-key-1", "request-1",
		[2]string{"dp-ver-1", ""}, [2]string{"dp-ver-2", "dp-ver-1"})

	reference, present := loadCommercialReference(t, repository, "tenant-1", "parcel-1")
	if !present || reference.String() != "RES-1" {
		t.Fatalf("reference after amendments = %q present = %v, want RES-1", reference, present)
	}
}

func TestAnObjectOutsideEveryAcceptedBaselineHasNoCommercialResolutionReference(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	acceptedRequestWithDeliveryPlaceChain(t, repository, transactor, "req-key-1", "request-1")
	mustInsert(t, transactor, t.Context(), repository, submittedShipmentRequest(t, "req-key-2", "request-2"))

	cases := map[string][2]string{
		"从未声明过的包裹":        {"tenant-1", "parcel-9"},
		"他租户问本租户已接受委托的成员": {"tenant-b", "parcel-1"},
	}
	for name, item := range cases {
		t.Run(name, func(t *testing.T) {
			reference, present := loadCommercialReference(t, repository, item[0], item[1])
			if present || reference.String() != "" {
				t.Fatalf("交回了回指 %q", reference)
			}
		})
	}
}

func TestAMemberOfAMerelySubmittedRequestHasNoCommercialResolutionReference(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	mustInsert(t, transactor, t.Context(), repository, submittedShipmentRequest(t, "req-key-1", "request-1"))

	if reference, present := loadCommercialReference(t, repository, "tenant-1", "parcel-1"); present || reference.String() != "" {
		t.Fatalf("仅已提交的委托的成员交回了回指 %q", reference)
	}
}

// 快照上的决定缺解析标识：这一行是坏的（接受流的装配缺陷或写坏的快照），读口上抛而不答「没有」。
func TestAnAcceptedRowWhoseDecisionLacksAResolutionIsAReadFaceFailure(t *testing.T) {
	repository, transactor, pool := newShipmentRequests(t)
	acceptedRequestWithDeliveryPlaceChain(t, repository, transactor, "req-key-1", "request-1")
	if _, err := pool.Exec(t.Context(),
		`UPDATE parcel_shipment.shipment_request
		    SET snapshot = jsonb_set(snapshot, '{decision,basis,resolutionId}', '""'::jsonb)
		  WHERE source_request_key = 'req-key-1'`); err != nil {
		t.Fatalf("抹掉快照上的解析标识：%v", err)
	}

	reference, present, err := repository.LoadCommercialResolutionReference(t.Context(),
		mustBuild(t, domain.NewTenantID, "tenant-1"), mustBuild(t, domain.NewDeclaredParcelID, "parcel-1"))
	if err == nil {
		t.Fatalf("坏行没有上抛：reference = %q present = %v", reference, present)
	}
	if present || reference.String() != "" {
		t.Fatalf("上抛的同时还交回了 present = %v reference = %q", present, reference)
	}
}

func TestTwoAcceptedRequestsClaimingOneParcelHaveNoSingleCommercialResolution(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	acceptedRequestWithDeliveryPlaceChain(t, repository, transactor, "req-key-1", "request-1")
	acceptedRequestWithDeliveryPlaceChain(t, repository, transactor, "req-key-2", "request-2")

	reference, present, err := repository.LoadCommercialResolutionReference(t.Context(),
		mustBuild(t, domain.NewTenantID, "tenant-1"), mustBuild(t, domain.NewDeclaredParcelID, "parcel-1"))
	if !errors.Is(err, domain.ErrAmbiguousParcelTarget) {
		t.Fatalf("err = %v, want ErrAmbiguousParcelTarget", err)
	}
	if present || reference.String() != "" {
		t.Fatalf("歧义时交回了 present = %v reference = %q", present, reference)
	}
}

func TestCommercialResolutionReferenceLookupRequiresTenantAndParcel(t *testing.T) {
	repository, _, _ := newShipmentRequests(t)
	cases := []struct {
		name   string
		tenant domain.TenantID
		parcel domain.DeclaredParcelID
	}{
		{"缺租户", domain.TenantID{}, mustBuild(t, domain.NewDeclaredParcelID, "parcel-1")},
		{"缺包裹", mustBuild(t, domain.NewTenantID, "tenant-1"), domain.DeclaredParcelID{}},
	}
	for _, item := range cases {
		reference, present, err := repository.LoadCommercialResolutionReference(t.Context(), item.tenant, item.parcel)
		if err == nil {
			t.Fatalf("%s 没有上抛，而是拿残缺的键去查了：%q %v", item.name, reference, present)
		}
		if present || reference.String() != "" {
			t.Fatalf("%s 上抛的同时还交回了 present = %v reference = %q", item.name, present, reference)
		}
	}
}
