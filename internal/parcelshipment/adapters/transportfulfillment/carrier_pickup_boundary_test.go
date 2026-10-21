package transportfulfillment_test

import (
	"context"
	"errors"
	"testing"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/transportfulfillment"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

type currentCarrierPickupDouble struct {
	record tfports.CarrierFirstEffectivePickupRecord
	found  bool
	err    error
	tenant string
	object string
}

func (double *currentCarrierPickupDouble) FindCurrentByObject(
	_ context.Context, tenant tfdomain.TenantID, object tfdomain.CarriedObjectReference,
) (tfports.CarrierFirstEffectivePickupRecord, bool, error) {
	double.tenant, double.object = tenant.String(), object.String()
	return double.record, double.found, double.err
}

// Covers: 取消提交边界上读面单渠道服务的首次有效收寄链尾（PS CONTEXT 取消一条，ADR-0254 决定二第 1 格）——
// 按（租户，包裹）原样作载运对象查链尾；已形成交回事实、版本与 TF 判过的业务发生时间（ADR-0135 决定三）；
// 待确认答待确认；链尾失效与从未有过同答没有（ADR-0135 决定六）；读不动交 error，由编排停成未决。
func TestTheCarrierPickupBoundaryReadsTheChainTail(t *testing.T) {
	tenant := value(t, psdomain.NewTenantID, "tenant-1")
	parcel := value(t, psdomain.NewDeclaredParcelID, "parcel-1")
	formed := formedCarrierPickup(t, "parcel-1")

	t.Run("formed", func(t *testing.T) {
		finder := &currentCarrierPickupDouble{record: recordOf(t, formed), found: true}
		view, err := adapter.NewCarrierPickupBoundaryAdapter(finder)
		if err != nil {
			t.Fatalf("new adapter: %v", err)
		}
		boundary, err := view.FindCurrentCarrierPickup(context.Background(), tenant, parcel)
		if err != nil {
			t.Fatalf("find: %v", err)
		}
		if finder.tenant != "tenant-1" || finder.object != "parcel-1" {
			t.Fatalf("查的是 %s/%s，want tenant-1/parcel-1", finder.tenant, finder.object)
		}
		if boundary.State != psports.CarrierPickupBoundaryFormed ||
			boundary.Pickup.Fact.String() != "CFEP-1" || boundary.Pickup.Version.String() != "CFEV-1" ||
			!boundary.Pickup.EffectiveAt.Equal(firstPickupAt) {
			t.Fatalf("boundary = %+v", boundary)
		}
	})

	t.Run("pending", func(t *testing.T) {
		pending, err := tfdomain.HoldCarrierFirstEffectivePickupPending(tfdomain.PendingCarrierFirstEffectivePickupSpec{
			TenantID: value(t, tfdomain.NewTenantID, "tenant-1"),
			Object:   value(t, tfdomain.NewCarriedObjectReference, "parcel-1"),
			Fact:     value(t, tfdomain.NewCarrierFirstEffectivePickupReference, "CFEP-1"),
			Version:  value(t, tfdomain.NewCarrierFirstEffectivePickupVersion, "CFEV-P"),
			Reason:   tfdomain.PickupSourceConflict,
			Material: "承运商 X",
			JudgedAt: pickupJudgedAt,
			Bases:    carrierPickupBases(t, "v1"),
		})
		if err != nil {
			t.Fatalf("hold pending: %v", err)
		}
		view, _ := adapter.NewCarrierPickupBoundaryAdapter(&currentCarrierPickupDouble{record: recordOf(t, pending), found: true})
		boundary, err := view.FindCurrentCarrierPickup(context.Background(), tenant, parcel)
		if err != nil || boundary.State != psports.CarrierPickupBoundaryPending {
			t.Fatalf("boundary = %+v err = %v, want PENDING", boundary, err)
		}
	})

	t.Run("voided tail and absence are both absent", func(t *testing.T) {
		voided, err := formed.Void(tfdomain.CarrierPickupVoiding{
			Version:  value(t, tfdomain.NewCarrierFirstEffectivePickupVersion, "CFEV-2"),
			JudgedAt: pickupJudgedAt.Add(1),
			Bases:    carrierPickupBases(t, "v2"),
		})
		if err != nil {
			t.Fatalf("void: %v", err)
		}
		for name, finder := range map[string]*currentCarrierPickupDouble{
			"voided": {record: recordOf(t, voided), found: true},
			"absent": {},
		} {
			view, _ := adapter.NewCarrierPickupBoundaryAdapter(finder)
			boundary, err := view.FindCurrentCarrierPickup(context.Background(), tenant, parcel)
			if err != nil || boundary.State != psports.CarrierPickupBoundaryAbsent {
				t.Fatalf("%s: boundary = %+v err = %v, want ABSENT", name, boundary, err)
			}
		}
	})

	t.Run("an unreadable register is an error", func(t *testing.T) {
		view, _ := adapter.NewCarrierPickupBoundaryAdapter(&currentCarrierPickupDouble{err: errors.New("down")})
		if _, err := view.FindCurrentCarrierPickup(context.Background(), tenant, parcel); err == nil {
			t.Fatal("读不动却答了")
		}
	})

	t.Run("a tail of another object is a broken register", func(t *testing.T) {
		view, _ := adapter.NewCarrierPickupBoundaryAdapter(&currentCarrierPickupDouble{
			record: recordOf(t, formedCarrierPickup(t, "parcel-9")), found: true})
		if _, err := view.FindCurrentCarrierPickup(context.Background(), tenant, parcel); !errors.Is(err, adapter.ErrCarrierPickupRecordInconsistent) {
			t.Fatalf("err = %v, want ErrCarrierPickupRecordInconsistent", err)
		}
	})
}
