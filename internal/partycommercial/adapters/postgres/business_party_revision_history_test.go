package postgres_test

import (
	"context"
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件对真实 PostgreSQL 16 证业务参与方修订历史读口（票 admin-web-group-legal-entities/12）：
// 同一参与方的全部修订按修订号升序交回、名称随每一笔走、停用两件只落在停用那一笔、不在册答空、跨租户读不到。

// Covers: 票 12 完成判据「在册多笔按序」——序按修订号而不是按落库时刻：这里刻意先落修订 2 再落修订 1，
// recorded_at 的序与修订号的序相反，读口若按落库时刻排就会颠倒。同租户另一参与方的修订不得混入。
// 名称是本册比法人册多出的一格，每一笔都要带回来。
func TestBusinessPartyRevisionHistoryListsAllRevisionsByRevisionNumber(t *testing.T) {
	registrations, catalogue, transactor := newPartyIdentityRegistrations(t)
	ctx := t.Context()

	first := partyRegistrationFixture(t, "tenant-1", "party-1", "参与方一")
	deactivated, err := first.Deactivate(
		pcValue(t, domain.NewIdentityBasisReference, "basis-deact"), identityDeactivateAt)
	if err != nil {
		t.Fatalf("deactivate party: %v", err)
	}
	mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
		return registrations.SaveBusinessParty(txCtx, deactivated)
	})
	mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
		return registrations.SaveBusinessParty(txCtx, first)
	})
	other := partyRegistrationFixture(t, "tenant-1", "party-2", "参与方二")
	mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
		return registrations.SaveBusinessParty(txCtx, other)
	})

	rows, err := catalogue.ListBusinessPartyRevisions(
		ctx, pcTenant(t, "tenant-1"), pcValue(t, domain.NewPartyID, "party-1"))
	if err != nil {
		t.Fatalf("list revisions: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2（party-2 的修订不得混入）", len(rows))
	}

	initial := rows[0]
	if initial.Revision != 1 || initial.PartyID != "party-1" || initial.TenantID != "tenant-1" ||
		initial.PartyName != "参与方一" || initial.Basis != "basis-party-1" ||
		!initial.EffectiveFrom.Equal(identityEffectiveFrom) || initial.HasDeactivation ||
		initial.RegisteredAt.IsZero() {
		t.Fatalf("修订 1 = %+v", initial)
	}
	// 停用那一笔：修订 2 带停用两件；修订 1 上一件都没有——停用是修订链上新的一笔，不回写旧笔。
	retired := rows[1]
	if retired.Revision != 2 || !retired.HasDeactivation ||
		retired.DeactivationBasis != "basis-deact" || !retired.DeactivatedAt.Equal(identityDeactivateAt) ||
		retired.PartyName != "参与方一" || retired.Basis != "basis-party-1" {
		t.Fatalf("修订 2 = %+v", retired)
	}
}

// Covers: 票 12 裁决「参与方不在册 → 空数组」的库半边，以及跨租户不可见（ADR-0003 隔离边界在 SQL
// 条件上）：两者在读口上同形——都是零行、无错——传输层因此分不出「没有」与「别家的」，
// 也就不会泄露跨租户存在性。
func TestBusinessPartyRevisionHistoryAnswersUnknownAndForeignAsEmpty(t *testing.T) {
	registrations, catalogue, transactor := newPartyIdentityRegistrations(t)
	ctx := t.Context()

	party := partyRegistrationFixture(t, "tenant-1", "party-1", "参与方一")
	mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
		return registrations.SaveBusinessParty(txCtx, party)
	})

	unknown, err := catalogue.ListBusinessPartyRevisions(
		ctx, pcTenant(t, "tenant-1"), pcValue(t, domain.NewPartyID, "party-none"))
	if err != nil || len(unknown) != 0 {
		t.Fatalf("不在册 = (%d, %v)，want (0, nil)", len(unknown), err)
	}
	if unknown == nil {
		t.Fatal("不在册要交回空切片而不是 nil：传输层据此编成 [] 而不是 null")
	}

	foreign, err := catalogue.ListBusinessPartyRevisions(
		ctx, pcTenant(t, "tenant-b"), pcValue(t, domain.NewPartyID, "party-1"))
	if err != nil || len(foreign) != 0 {
		t.Fatalf("跨租户 = (%d, %v)，want (0, nil)", len(foreign), err)
	}
}
