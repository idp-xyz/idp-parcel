package networkrouting_test

import (
	"context"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	nrpostgres "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/networkrouting"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// Covers: AT-NR-060 的运输履约一侧（ADR-0217、ADR-0131 决定一 / 三）——对真实 PostgreSQL 16：NR 落一份末段是交付段
// 的计划，派送窗口缝按登记方声明的交付段引用答 RESOLVED，窗口就是交付段那一段的；交付段之后的序位答 MISSING。本缝
// 不看段种类：声明的是节点间段，照样交那一段的窗口。
func TestTheDeliveryWindowSeamResolvesTheDeliveryLegOnARealPlan(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	routes, err := nrpostgres.NewInitialRoutes(db)
	if err != nil {
		t.Fatal(err)
	}
	windows, err := nrpostgres.NewPlannedLegWindows(db)
	if err != nil {
		t.Fatal(err)
	}
	seam, err := adapter.NewDeliveryWindows(windows)
	if err != nil {
		t.Fatal(err)
	}

	deliveryWindow, err := nrdomain.NewPlannedTimeWindow(routeJudgedAt.Add(12*time.Hour), routeJudgedAt.Add(15*time.Hour),
		nr(t, nrdomain.NewWindowBasisReference, "NODE/SYN-NODE-SIN-LM@2"))
	if err != nil {
		t.Fatalf("交付段窗口：%v", err)
	}
	delivery, err := nrdomain.NewDeliveryLeg(nrdomain.DeliveryLegSpec{
		From:        nr(t, nrdomain.NewPlanNodeReference, "SYN-NODE-SIN-LM"),
		Scope:       nr(t, nrdomain.NewServiceAreaVersionReference, "SYN-AREA-SG@1"),
		Responsible: nr(t, nrdomain.NewResponsiblePartyReference, "SYN-AREA-SG@1"),
		Window:      deliveryWindow,
	})
	if err != nil {
		t.Fatalf("交付段：%v", err)
	}
	record := formedRecord(t,
		legOn(t, "CONNECTION/SYN-CONN-SIN-LM@1", "SYN-NODE-SIN-HUB", "SYN-NODE-SIN-LM", 4*time.Hour),
		delivery,
	)
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		outcome, err := routes.Save(txCtx, record)
		if err == nil && outcome != nrports.InitialRouteSaved {
			t.Fatalf("落计划：%v", outcome)
		}
		return err
	}); err != nil {
		t.Fatalf("落计划：%v", err)
	}

	tenant, err := tfdomain.NewTenantID(formedRoute.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	ask := func(reference string) (time.Time, time.Time, tfports.RequirementResolution) {
		t.Helper()
		planned, err := tfdomain.NewPlannedSegmentReference(reference)
		if err != nil {
			t.Fatalf("段引用 %s：%v", reference, err)
		}
		from, to, resolution, err := seam.LoadDeliveryWindow(t.Context(), tenant, planned, true)
		if err != nil {
			t.Fatalf("问 %s：%v", reference, err)
		}
		return from, to, resolution
	}

	from, to, resolution := ask("RPV-7#2")
	if resolution != tfports.RequirementResolved ||
		!from.Equal(routeJudgedAt.Add(12*time.Hour)) || !to.Equal(routeJudgedAt.Add(15*time.Hour)) {
		t.Fatalf("交付段引用 = %s [%v, %v]，想要 RESOLVED 带交付段窗口", resolution, from, to)
	}
	if _, _, resolution := ask("RPV-7#3"); resolution != tfports.RequirementMissing {
		t.Fatalf("交付段之后的序位 = %s，想要 MISSING", resolution)
	}
	if from, _, resolution := ask("RPV-7#1"); resolution != tfports.RequirementResolved || !from.Equal(routeJudgedAt.Add(4*time.Hour)) {
		t.Fatalf("节点间段引用 = %s %v，本缝不按段种类分支", resolution, from)
	}
}
