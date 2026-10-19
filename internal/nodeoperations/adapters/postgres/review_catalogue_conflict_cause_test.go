package postgres_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// Covers: ADR-0211 决定一的复核目录存储面——待识别册每一行带它记下的冲突来由；不记冲突的、与来由这一格落地之前的冲突行都不带。
func TestUnidentifiedItemsCarryTheirConflictCause(t *testing.T) {
	catalogue, receptions, _, transactor := newReviewCatalogue(t)
	ctx := t.Context()
	const tenant, mark = "SYN-TENANT-NRI07", "SYN-MARK-NRI07"
	ambiguous := contestedPending(t, tenant, "SYN-SCAN-NRI07-AMB", "SYN-INTAKEV-NRI07-AMB", mark, ports.ConflictByAmbiguityOnly)
	legacy := contestedPending(t, tenant, "SYN-SCAN-NRI07-OLD", "SYN-INTAKEV-NRI07-OLD", mark, ports.IdentityConflictCauseAbsent)
	pending := recheckablePending(t, tenant, "SYN-SCAN-NRI07-ZERO", "SYN-INTAKEV-NRI07-ZERO", mark)
	saveReceptions(t, transactor, ctx, receptions, ambiguous, legacy, pending)

	rows, err := catalogue.ListUnidentifiedItems(ctx, ref(t, domain.NewTenantID, tenant), 10)
	if err != nil {
		t.Fatalf("上列待识别册：%v", err)
	}
	want := map[string]ports.IdentityConflictCause{
		"SYN-SCAN-NRI07-AMB":  ports.ConflictByAmbiguityOnly,
		"SYN-SCAN-NRI07-OLD":  ports.IdentityConflictCauseAbsent,
		"SYN-SCAN-NRI07-ZERO": ports.IdentityConflictCauseAbsent,
	}
	if len(rows) != len(want) {
		t.Fatalf("待识别册 %d 行, want %d", len(rows), len(want))
	}
	for _, row := range rows {
		if row.IdentityConflictCause != want[row.SourceID] {
			t.Errorf("%s 来由 = %q, want %q", row.SourceID, row.IdentityConflictCause, want[row.SourceID])
		}
	}
}
