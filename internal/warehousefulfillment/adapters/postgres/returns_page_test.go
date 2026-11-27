package postgres_test

import (
	"testing"

	adapter "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

func TestReturnListsPageAndFilterByTracking(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	handler := world.returns(t)
	world.acceptReturn(t, handler, "return-a", "TRK-A")
	world.acceptReturn(t, handler, "return-b", "TRK-B")
	reads, err := adapter.NewWorkspaceReads(world.db)
	if err != nil {
		t.Fatal(err)
	}
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	first, err := reads.ReadReturns(t.Context(), tenant, ports.ReturnsQuery{
		View: ports.ReturnsViewAdvices, Page: 1, PageSize: 1, Sort: "newest",
	})
	if err != nil || first.Total != 2 || len(first.PreAdvices) != 1 || first.Counts["AWAITING_ARRIVAL"] != 2 {
		t.Fatalf("第一页 %+v %v", first, err)
	}
	if len(first.PreAdvices[0].Lines) != 1 {
		t.Fatalf("商品行 %d", len(first.PreAdvices[0].Lines))
	}
	second, err := reads.ReadReturns(t.Context(), tenant, ports.ReturnsQuery{
		View: ports.ReturnsViewAdvices, Page: 2, PageSize: 1, Sort: "newest",
	})
	if err != nil || len(second.PreAdvices) != 1 || second.PreAdvices[0].AdviceID == first.PreAdvices[0].AdviceID {
		t.Fatalf("第二页 %+v %v", second, err)
	}
	found, err := reads.ReadReturns(t.Context(), tenant, ports.ReturnsQuery{
		View: ports.ReturnsViewAdvices, Page: 1, PageSize: 20, Sort: "newest", Keyword: "TRK-A",
	})
	if err != nil || found.Total != 1 || len(found.PreAdvices) != 1 || found.PreAdvices[0].TrackingNumber != "TRK-A" {
		t.Fatalf("按单号 %d %v", found.Total, err)
	}
	one, err := reads.ReadReturns(t.Context(), tenant, ports.ReturnsQuery{
		View: ports.ReturnsViewAdvice, Key: found.PreAdvices[0].DocumentNumber,
	})
	if err != nil || len(one.PreAdvices) != 1 || one.PreAdvices[0].AdviceID != found.PreAdvices[0].AdviceID {
		t.Fatalf("按一张 %+v %v", one, err)
	}
}
