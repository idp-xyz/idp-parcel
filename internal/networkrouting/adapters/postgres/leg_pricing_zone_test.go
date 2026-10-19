package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// Covers: 迁移 0019 的段计价区域册（ADR-0189 决定二）——只登在挂了成本依据的段上（没挂依据的段整笔拒、一行不写），
// 按段号升序读回，按租户与线路版本隔离，没登是空列表不是错误。
func TestLegPricingZonesRegisterOnlyOnSegmentsWithACostBasis(t *testing.T) {
	catalog, transactor, _ := newNetworkCatalog(t)
	ctx := t.Context()
	tenant := scalar(t, domain.NewTenantID, "tenant-1")

	within(t, transactor, ctx, func(txCtx context.Context) error {
		if err := catalog.RegisterLineVersion(txCtx, tenant, ports.LineDefinitionVersion{
			Code: "line-three-legs", Version: 1, Segments: []string{"conn-a", "conn-b", "conn-c"},
			BusinessTimezone: "Asia/Shanghai", ApplicableScope: "purpose-export",
			EffectiveFrom: catalogAsOf.Add(-time.Hour),
		}); err != nil {
			return err
		}
		return catalog.RegisterLineCostBases(txCtx, tenant, "line-three-legs", 1, []ports.LineSegmentCostBasis{
			{SegmentIndex: 0, Kind: ports.SupplierBuyPlanBasis, Reference: "plan-buy/v1"},
			{SegmentIndex: 1, Kind: ports.SupplierBuyPlanBasis, Reference: "plan-buy/v1"},
		})
	})

	var refusal error
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		refusal = catalog.RegisterLegPricingZones(txCtx, tenant, "line-three-legs", 1, []ports.LegPricingZone{
			{SegmentIndex: 0, Zone: "Z1", Basis: "basis-0"},
			{SegmentIndex: 2, Zone: "Z1", Basis: "basis-2"},
		})
		return refusal
	}); !errors.Is(err, ports.ErrLegPricingZoneWithoutCostBasis) || !errors.Is(refusal, ports.ErrLegPricingZoneWithoutCostBasis) {
		t.Fatalf("没挂依据的段应整笔拒：err = %v", err)
	}
	if zones, err := catalog.LoadLegPricingZones(ctx, tenant, "line-three-legs", 1); err != nil || len(zones) != 0 {
		t.Fatalf("被拒的那一笔落了行：%+v err=%v", zones, err)
	}

	within(t, transactor, ctx, func(txCtx context.Context) error {
		return catalog.RegisterLegPricingZones(txCtx, tenant, "line-three-legs", 1, []ports.LegPricingZone{
			{SegmentIndex: 1, Zone: "Z2", Basis: "basis-1"},
			{SegmentIndex: 0, Zone: "Z1", Basis: "basis-0"},
		})
	})
	zones, err := catalog.LoadLegPricingZones(ctx, tenant, "line-three-legs", 1)
	if err != nil {
		t.Fatalf("读段计价区域：%v", err)
	}
	if len(zones) != 2 || zones[0] != (ports.LegPricingZone{SegmentIndex: 0, Zone: "Z1", Basis: "basis-0"}) ||
		zones[1] != (ports.LegPricingZone{SegmentIndex: 1, Zone: "Z2", Basis: "basis-1"}) {
		t.Fatalf("应按段号升序原样读回，实得 %+v", zones)
	}
	if other, err := catalog.LoadLegPricingZones(ctx, scalar(t, domain.NewTenantID, "tenant-2"), "line-three-legs", 1); err != nil || len(other) != 0 {
		t.Fatalf("别的租户读到了：%+v err=%v", other, err)
	}
	if later, err := catalog.LoadLegPricingZones(ctx, tenant, "line-three-legs", 2); err != nil || len(later) != 0 {
		t.Fatalf("别的线路版本读到了：%+v err=%v", later, err)
	}
}

// Covers: 迁移 0019 头注「登记只增不改」——同一段再登撞主键，整笔失败，先登的那一行原样留着。
func TestLegPricingZoneOnTheSameSegmentIsNotRegisteredTwice(t *testing.T) {
	catalog, transactor, _ := newNetworkCatalog(t)
	ctx := t.Context()
	tenant := scalar(t, domain.NewTenantID, "tenant-1")

	within(t, transactor, ctx, func(txCtx context.Context) error {
		if err := catalog.RegisterLineVersion(txCtx, tenant, ports.LineDefinitionVersion{
			Code: "line-one-leg", Version: 1, Segments: []string{"conn-a"},
			BusinessTimezone: "Asia/Shanghai", ApplicableScope: "purpose-export",
			EffectiveFrom: catalogAsOf.Add(-time.Hour),
		}); err != nil {
			return err
		}
		if err := catalog.RegisterLineCostBases(txCtx, tenant, "line-one-leg", 1, []ports.LineSegmentCostBasis{
			{SegmentIndex: 0, Kind: ports.SupplierBuyPlanBasis, Reference: "plan-buy/v1"},
		}); err != nil {
			return err
		}
		return catalog.RegisterLegPricingZones(txCtx, tenant, "line-one-leg", 1, []ports.LegPricingZone{
			{SegmentIndex: 0, Zone: "Z1", Basis: "basis-first"},
		})
	})

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return catalog.RegisterLegPricingZones(txCtx, tenant, "line-one-leg", 1, []ports.LegPricingZone{
			{SegmentIndex: 0, Zone: "Z2", Basis: "basis-second"},
		})
	}); err == nil {
		t.Fatal("同一段再登应撞主键失败")
	}
	zones, err := catalog.LoadLegPricingZones(ctx, tenant, "line-one-leg", 1)
	if err != nil {
		t.Fatalf("读段计价区域：%v", err)
	}
	if len(zones) != 1 || zones[0] != (ports.LegPricingZone{SegmentIndex: 0, Zone: "Z1", Basis: "basis-first"}) {
		t.Fatalf("先登的那一行应原样留着，实得 %+v", zones)
	}
}

func TestLegPricingZonesRefuseToRunOutsideATransaction(t *testing.T) {
	catalog, _, _ := newNetworkCatalog(t)

	err := catalog.RegisterLegPricingZones(t.Context(),
		scalar(t, domain.NewTenantID, "tenant-1"), "line-x", 1,
		[]ports.LegPricingZone{{SegmentIndex: 0, Zone: "Z1", Basis: "basis-0"}})
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记应返回 ErrTransactionRequired，实得：%v", err)
	}
}
