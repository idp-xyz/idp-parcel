package postgres_test

import (
	"context"
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件对真实 PostgreSQL 16 证责任法人修订历史读口（票 admin-web-group-legal-entities/03）：
// 同一法人的全部修订按修订号升序交回、停用两件只落在停用那一笔、不在册答空、跨租户读不到。

// Covers: 票 03 完成判据「在册法人多笔按序」——序按修订号而不是按落库时刻：这里刻意先落修订 2
// 再落修订 1，recorded_at 的序与修订号的序相反，读口若按落库时刻排就会颠倒。同租户另一法人
// 的修订不得混入。
func TestLegalEntityRevisionHistoryListsAllRevisionsByRevisionNumber(t *testing.T) {
	registrations, catalogue, transactor := newPartyIdentityRegistrations(t)
	ctx := t.Context()

	first := legalEntityRegistrationFixture(t, "tenant-1", "le-1", "party-le")
	deactivated, err := first.Deactivate(
		pcValue(t, domain.NewIdentityBasisReference, "basis-deact"), identityDeactivateAt)
	if err != nil {
		t.Fatalf("deactivate entity: %v", err)
	}
	mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
		return registrations.SaveLegalEntity(txCtx, deactivated)
	})
	mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
		return registrations.SaveLegalEntity(txCtx, first)
	})
	other := legalEntityRegistrationFixture(t, "tenant-1", "le-2", "party-le")
	mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
		return registrations.SaveLegalEntity(txCtx, other)
	})

	rows, err := catalogue.ListLegalEntityRevisions(
		ctx, pcTenant(t, "tenant-1"), pcValue(t, domain.NewLegalEntityReference, "le-1"))
	if err != nil {
		t.Fatalf("list revisions: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2（le-2 的修订不得混入）", len(rows))
	}

	initial := rows[0]
	if initial.Revision != 1 || initial.LegalEntityID != "le-1" || initial.TenantID != "tenant-1" ||
		initial.PartyID != "party-le" || initial.Basis != "basis-le-1" ||
		!initial.EffectiveFrom.Equal(identityEffectiveFrom) || initial.HasDeactivation ||
		initial.RegisteredAt.IsZero() {
		t.Fatalf("修订 1 = %+v", initial)
	}
	// 停用那一笔：修订 2 带停用两件；修订 1 上一件都没有——停用是修订链上新的一笔，不回写旧笔。
	retired := rows[1]
	if retired.Revision != 2 || !retired.HasDeactivation ||
		retired.DeactivationBasis != "basis-deact" || !retired.DeactivatedAt.Equal(identityDeactivateAt) ||
		retired.Basis != "basis-le-1" || retired.PartyID != "party-le" {
		t.Fatalf("修订 2 = %+v", retired)
	}
}

// Covers: 票 03 裁决「法人不在册 → 空数组」的库半边，以及跨租户不可见（ADR-0003 隔离边界在 SQL
// 条件上）：两者在读口上同形——都是零行、无错——传输层因此分不出「没有」与「别家的」，
// 也就不会泄露跨租户存在性。
func TestLegalEntityRevisionHistoryAnswersUnknownAndForeignAsEmpty(t *testing.T) {
	registrations, catalogue, transactor := newPartyIdentityRegistrations(t)
	ctx := t.Context()

	entity := legalEntityRegistrationFixture(t, "tenant-1", "le-1", "party-le")
	mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
		return registrations.SaveLegalEntity(txCtx, entity)
	})

	unknown, err := catalogue.ListLegalEntityRevisions(
		ctx, pcTenant(t, "tenant-1"), pcValue(t, domain.NewLegalEntityReference, "le-none"))
	if err != nil || len(unknown) != 0 {
		t.Fatalf("不在册 = (%d, %v)，want (0, nil)", len(unknown), err)
	}
	if unknown == nil {
		t.Fatal("不在册要交回空切片而不是 nil：传输层据此编成 [] 而不是 null")
	}

	foreign, err := catalogue.ListLegalEntityRevisions(
		ctx, pcTenant(t, "tenant-b"), pcValue(t, domain.NewLegalEntityReference, "le-1"))
	if err != nil || len(foreign) != 0 {
		t.Fatalf("跨租户 = (%d, %v)，want (0, nil)", len(foreign), err)
	}
}
