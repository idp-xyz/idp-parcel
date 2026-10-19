package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// Covers: 迁移 0019 的段计价区域册（ADR-0189 决定二）——只登在挂了成本依据的段上（没挂依据的段整笔拒、一行不写），
// 按段号升序读回，租户隔离，没登是空列表不是错误。
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
}
