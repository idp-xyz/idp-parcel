package main

import (
	"testing"
	"time"

	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	tfapplication "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// Covers: AT-TF-201（ADR-0217）——初始路由计划末尾带交付段，经生产路由表到 TF 运输准备那一路：只为节点间段形成准备结果，
// 交付段不形成结果；信封定稿、TF 与 VE 两个 inbox 各入账一行，不成毒丸。生产 wireDispatcher、生产 NR 交接口、真库，
// 夹具全为合成值（S 级）。
func TestAPlanEndingWithADeliveryLegPreparesOnlyTheLegsBetweenNodes(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	now := time.Now().UTC().Truncate(time.Minute)
	registerSYNTemplate(t, fixture, "SYN-TPL-64-A", "SYN-CONN-64-A", "SYN-NODE-64-1", "SYN-NODE-64-2",
		now.Add(3*time.Hour), time.Hour,
		tfapplication.CapacityDeclarationInput{Dimension: "PIECES", Unit: "PCS", Capacity: 10})

	window, err := nrdomain.NewPlannedTimeWindow(now.Add(8*time.Hour), now.Add(12*time.Hour),
		mustNR(t, nrdomain.NewWindowBasisReference, "NODE/SYN-NODE-64-2@2"))
	if err != nil {
		t.Fatalf("交付段窗口：%v", err)
	}
	delivery, err := nrdomain.NewDeliveryLeg(nrdomain.DeliveryLegSpec{
		From:        mustNR(t, nrdomain.NewPlanNodeReference, "SYN-NODE-64-2"),
		Scope:       mustNR(t, nrdomain.NewServiceAreaVersionReference, "SYN-AREA-64@1"),
		Responsible: mustNR(t, nrdomain.NewResponsiblePartyReference, "SYN-AREA-64@1"),
		Window:      window,
	})
	if err != nil {
		t.Fatalf("交付段：%v", err)
	}
	eventID := recordPlannedLegs(t, fixture, now, []nrdomain.PlannedLeg{
		plannedLegOn(t, "SYN-CONN-64-A", "SYN-NODE-64-1", "SYN-NODE-64-2", now.Add(2*time.Hour), now.Add(6*time.Hour)),
		delivery,
	})

	published, err := fixture.beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("一拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("published = %d, want 1；失败码 = %q", published, recordedFailureCode(t, fixture.db, eventID))
	}
	got := preparationOutcomes(t, fixture)
	if len(got) != 1 || got["SYN-RPV-0018#1"] != "OPPORTUNITY_PREPARED" {
		t.Fatalf("准备结果 = %v，想要只有节点间段 #1 一份", got)
	}
	if n := fixture.countInbox(t, tfPreparationConsumerName, eventID); n != 1 {
		t.Fatalf("TF inbox 行数 = %d, want 1", n)
	}
	if n := fixture.countInbox(t, deriveInitialRouteConsumerName, eventID); n != 1 {
		t.Fatalf("VE inbox 行数 = %d, want 1", n)
	}
}
