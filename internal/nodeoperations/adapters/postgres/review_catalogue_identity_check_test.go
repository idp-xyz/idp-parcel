package postgres_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// Covers: 票 no-reception-source/05「复核目录待识别行带核对态」的存储面（ADR-0199 决定五）——待识别册与收寄册上，未核对与
// 零候选的已核对两行候选同为空，只有核对态把它们分开；关联收寄行不带。
func TestCataloguesCarryTheIdentityCheckOfPendingUnits(t *testing.T) {
	catalogue, receptions, _, transactor := newReviewCatalogue(t)
	ctx := t.Context()
	const tenant = "SYN-TENANT-NRS05"
	unchecked := uncheckedPending(t, tenant, "SYN-SCAN-NRS05-UNCHECKED", "SYN-INTAKEV-NRS05-UNCHECKED")
	checked := recheckablePending(t, tenant, "SYN-SCAN-NRS05-CHECKED", "SYN-INTAKEV-NRS05-CHECKED", "SYN-MARK-NRS05")
	checked.IdentityCheck = ports.IdentityChecked
	formed := formedRecord(t, tenant, "SYN-SCAN-NRS05-FORMED")
	saveReceptions(t, transactor, ctx, receptions, unchecked, checked, formed)
	want := map[string]ports.IdentityCheck{
		"SYN-SCAN-NRS05-UNCHECKED": ports.IdentityUnchecked,
		"SYN-SCAN-NRS05-CHECKED":   ports.IdentityChecked,
		"SYN-SCAN-NRS05-FORMED":    ports.IdentityCheckAbsent,
	}

	items, err := catalogue.ListUnidentifiedItems(ctx, ref(t, domain.NewTenantID, tenant), 10)
	if err != nil {
		t.Fatalf("上列待识别册：%v", err)
	}
	if len(items) != 2 {
		t.Fatalf("待识别册 %d 行, want 2", len(items))
	}
	for _, item := range items {
		if item.IdentityCheck != want[item.SourceID] || len(item.Candidates) != 0 {
			t.Errorf("待识别册 %s 核对态 %q 候选 %v, want %q、无候选", item.SourceID, item.IdentityCheck, item.Candidates, want[item.SourceID])
		}
	}

	rows, err := catalogue.ListReceptions(ctx, ref(t, domain.NewTenantID, tenant), 10)
	if err != nil {
		t.Fatalf("上列收寄册：%v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("收寄册 %d 行, want 3", len(rows))
	}
	for _, row := range rows {
		if row.IdentityCheck != want[row.SourceID] {
			t.Errorf("收寄册 %s 核对态 %q, want %q", row.SourceID, row.IdentityCheck, want[row.SourceID])
		}
	}
}
