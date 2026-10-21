package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// Covers: ADR-0231 决定五与 ADR-0232 决定二共用的那张单子（票 demo-chain-gaps/160 的列段那一半）——按委托列段只列此刻仍采用它的各段、
// 各段只交当前那一代：一段解除之后不再列，再采用之后列出的是第 2 代。换版补形成与解除之后的重新派生读的是同一口。
func TestListingByCommissionShowsOnlyTheCurrentAdoptions(t *testing.T) {
	rig := newWithdrawalRig(t)
	late := activeMember(t, "parcel-2")
	late.EnteredAt = segmentEnteredAtFixture.Add(2 * time.Hour)
	mustSaveSegment(t, rig.stores.transactor, t.Context(), rig.stores.segments, segmentRecord(t, "SEG-LATE", late))
	if rig.adopt(t, "SEG-ADOPT", 0) != application.CommissionAdopted || rig.adopt(t, "SEG-LATE", 0) != application.CommissionAdopted {
		t.Fatal("两段采用不成")
	}
	key := ports.TransportCommissionKey{TenantID: rig.tenant, Commission: deliveryValue(t, domain.NewTransportCommissionReference, "commission-1")}
	listed := func() map[string]int {
		t.Helper()
		records, err := rig.stores.adoptions.ListByCommission(t.Context(), key)
		if err != nil {
			t.Fatalf("按委托列段：%v", err)
		}
		generations := map[string]int{}
		for _, record := range records {
			generations[record.Key.Segment.String()] = record.Generation
		}
		return generations
	}
	if got := listed(); len(got) != 2 || got["SEG-ADOPT"] != 1 || got["SEG-LATE"] != 1 {
		t.Fatalf("两段都采用着 = %v", got)
	}
	if result := rig.withdraw(t, "SEG-ADOPT", 1); result.Outcome() != application.AdoptionWithdrawn {
		t.Fatalf("解除 = %s", result.Outcome())
	}
	if got := listed(); len(got) != 1 || got["SEG-LATE"] != 1 {
		t.Fatalf("解除之后 = %v, want 只剩 SEG-LATE", got)
	}
	if rig.adopt(t, "SEG-ADOPT", 1) != application.CommissionAdopted {
		t.Fatal("再采用不成")
	}
	if got := listed(); len(got) != 2 || got["SEG-ADOPT"] != 2 {
		t.Fatalf("再采用之后 = %v, want SEG-ADOPT 第 2 代", got)
	}
}
