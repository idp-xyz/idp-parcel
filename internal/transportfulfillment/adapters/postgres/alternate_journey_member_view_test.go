package postgres_test

import (
	"context"
	"sort"
	"testing"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// Covers: AT-TF-252 的库面半边（ADR-0218）——旅程册按成员读：交回这个对象出现在其中的每一趟，键与内容经构造门原样读回；
// 成员按整串认（`parcel-10` 不算 `parcel-1`），别的租户的不交回，没有就是空。
func TestAlternateJourneysAreFoundByMember(t *testing.T) {
	journeys, _, _, transactor, _ := newJourneyScheduleCapacityStores(t)
	ctx := t.Context()

	alternate := formedJourney(t, "tenant-a", domain.AlternateJourneyPurpose, domain.ServiceDispositionDecision, "journey-alt-1")
	returned := formedJourney(t, "tenant-a", domain.ReturnJourneyPurpose, domain.ServiceDispositionDecision, "journey-return-1")
	elsewhere := formedJourney(t, "tenant-b", domain.ReturnJourneyPurpose, domain.ServiceDispositionDecision, "journey-return-b")
	lookalike, err := domain.FormAlternateJourney(domain.AlternateJourneySpec{
		TenantID:        deliveryValue(t, domain.NewTenantID, "tenant-a"),
		Journey:         deliveryValue(t, domain.NewJourneyReference, "journey-alt-10"),
		Purpose:         domain.AlternateJourneyPurpose,
		OriginalJourney: deliveryValue(t, domain.NewJourneyReference, "journey-original-10"),
		BasisKind:       domain.ServiceDispositionDecision,
		Basis:           deliveryValue(t, domain.NewDispositionBasisReference, "DISPOSITION/decision-10"),
		Members:         []domain.CarriedObjectReference{deliveryValue(t, domain.NewCarriedObjectReference, "parcel-10")},
		StartedAt:       journeyStartedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	lookalikeRecord := ports.AlternateJourneyRecord{
		Key: ports.AlternateJourneyKey{
			TenantID: lookalike.TenantID(), Original: lookalike.OriginalJourney(), Purpose: lookalike.Purpose(), Basis: lookalike.Basis(),
		},
		ContentDigest: "digest-journey-alt-10",
		Journey:       lookalike,
		RecordedAt:    journeyStartedAt,
	}
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		for _, record := range []ports.AlternateJourneyRecord{alternate, returned, elsewhere, lookalikeRecord} {
			if outcome, err := journeys.Save(txCtx, record); err != nil || outcome != ports.AlternateJourneySaved {
				t.Fatalf("落旅程 %s：%v %d", record.Journey.Journey(), err, outcome)
			}
		}
		return nil
	})

	found, err := journeys.FindByMember(ctx, deliveryValue(t, domain.NewTenantID, "tenant-a"),
		deliveryValue(t, domain.NewCarriedObjectReference, "parcel-1"))
	if err != nil {
		t.Fatalf("按成员读：%v", err)
	}
	var names []string
	for _, record := range found {
		names = append(names, record.Journey.Journey().String())
		want := alternate
		if record.Journey.Journey() == returned.Journey.Journey() {
			want = returned
		}
		if record.Key != want.Key || record.ContentDigest != want.ContentDigest || !record.Journey.StartedAt().Equal(journeyStartedAt) {
			t.Fatalf("读回的 %s 与落下的不同：key=%+v digest=%q", record.Journey.Journey(), record.Key, record.ContentDigest)
		}
	}
	sort.Strings(names)
	if len(names) != 2 || names[0] != "journey-alt-1" || names[1] != "journey-return-1" {
		t.Fatalf("parcel-1 的旅程 = %v, want journey-alt-1 与 journey-return-1", names)
	}

	none, err := journeys.FindByMember(ctx, deliveryValue(t, domain.NewTenantID, "tenant-a"),
		deliveryValue(t, domain.NewCarriedObjectReference, "parcel-2"))
	if err != nil || len(none) != 0 {
		t.Fatalf("不在任何旅程里的对象 = %d 趟 %v, want 0", len(none), err)
	}
}
