package postgres_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	adapter "go.idp.xyz/idp-parcel/internal/collectionremittance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/collectionremittance/domain"
	"go.idp.xyz/idp-parcel/internal/collectionremittance/ports"
)

// 本文件在真实 PostgreSQL 上证 ADR-0298 落在库上的几格（隔离合成 S）：认可的协议抵扣
// 一册的往返与按（认可引用，指令）唯一、已抵扣与认可抵扣依据的 CHECK、一条认可至多一笔
// 记账与一笔记账至多被冲正一次的唯一约束，以及按指令取记账时带上抵扣与它的冲正。

func fixtureOffset(
	t *testing.T, id, instruction, approval string, amountMinor int64,
) domain.AgreedOffset {
	t.Helper()
	offsetID, err := domain.NewAgreedOffsetID(id)
	if err != nil {
		t.Fatalf("构造抵扣标识：%v", err)
	}
	approvalRef, err := domain.NewApprovalReference(approval)
	if err != nil {
		t.Fatalf("构造认可引用：%v", err)
	}
	offset, err := domain.RegisterAgreedOffset(domain.AgreedOffsetSpec{
		ID:          offsetID,
		Instruction: fixtureInstructionID(t, instruction),
		Amount:      fixtureMoney(t, amountMinor),
		Approval:    approvalRef,
		ApprovedAt:  fixtureAt(t, "2026-08-25T00:00:00Z"),
	})
	if err != nil {
		t.Fatalf("登记认可抵扣：%v", err)
	}
	return offset
}

// 认可的协议抵扣往返不走样；同一份认可落到同一条指令上换个标识再登记，写口答已登记
// 而不落第二行，按标识也读不回它；同一份认可落到另一条指令上是另一行。
// Covers: AT-CR-024（库约束半边）
func TestAnAgreedOffsetRoundTripsAndTheDatabaseKeepsOneRowPerApprovalAndInstruction(t *testing.T) {
	db := newDB(t)
	tenant := fixtureTenant(t)
	collections, err := adapter.NewCollectionRegistrations(db)
	if err != nil {
		t.Fatalf("构造代收面写口：%v", err)
	}
	view, err := adapter.NewCollectionPointView(db)
	if err != nil {
		t.Fatalf("构造代收面读口：%v", err)
	}

	first := fixtureOffset(t, "SYN-OFFSET-1", "SYN-INSTR-1", "SYN-SA-APPROVAL-1", 300)
	if err := withinTransaction(t, db, func(ctx context.Context) error {
		for _, instruction := range []string{"SYN-INSTR-1", "SYN-INSTR-2"} {
			if _, err := collections.RegisterInstruction(
				ctx, tenant, fixtureInstruction(t, instruction, 12000)); err != nil {
				return err
			}
		}
		if outcome, err := collections.RegisterAgreedOffset(ctx, tenant, first); err != nil ||
			outcome != ports.SaveRegistered {
			t.Fatalf("登记认可抵扣 = %v（err=%v）", outcome, err)
		}
		if outcome, err := collections.RegisterAgreedOffset(ctx, tenant, first); err != nil ||
			outcome != ports.SaveAlreadyRegistered {
			t.Fatalf("重放登记 = %v（err=%v），要 AlreadyRegistered", outcome, err)
		}
		if outcome, err := collections.RegisterAgreedOffset(ctx, tenant,
			fixtureOffset(t, "SYN-OFFSET-1B", "SYN-INSTR-1", "SYN-SA-APPROVAL-1", 300)); err != nil ||
			outcome != ports.SaveAlreadyRegistered {
			t.Fatalf("换标识再登记同一份认可 = %v（err=%v），要 AlreadyRegistered", outcome, err)
		}
		if outcome, err := collections.RegisterAgreedOffset(ctx, tenant,
			fixtureOffset(t, "SYN-OFFSET-2", "SYN-INSTR-2", "SYN-SA-APPROVAL-1", 300)); err != nil ||
			outcome != ports.SaveRegistered {
			t.Fatalf("同认可引用另一条指令 = %v（err=%v），要 Registered", outcome, err)
		}
		return nil
	}); err != nil {
		t.Fatalf("登记：%v", err)
	}

	read, found, err := view.LoadAgreedOffset(t.Context(), tenant, first.ID())
	if err != nil || !found {
		t.Fatalf("读回认可抵扣：found=%v err=%v", found, err)
	}
	if read.ID() != first.ID() || read.Instruction() != first.Instruction() ||
		read.Amount() != first.Amount() || read.Approval() != first.Approval() ||
		!read.ApprovedAt().Equal(first.ApprovedAt()) {
		t.Fatalf("认可抵扣往返走样：写 %+v 读 %+v", first, read)
	}
	other, err := domain.NewAgreedOffsetID("SYN-OFFSET-1B")
	if err != nil {
		t.Fatalf("构造抵扣标识：%v", err)
	}
	if _, found, err := view.LoadAgreedOffset(t.Context(), tenant, other); err != nil || found {
		t.Fatalf("撞唯一键的第二行落库了：found=%v err=%v", found, err)
	}
}

// 已抵扣的几道门库上也拦得住：绕过写口的行照样进不来。
// Covers: AT-CR-024（库约束半边）
func TestTheDatabaseGatesTheAppliedOffsetPositionBothWays(t *testing.T) {
	fixture := newCatalogueFixture(t)
	fixture.seedSubledger(t, "tenant-a", "SYN-CUST-01", "SYN-LE-01", "SYN-CUR-01", "SYN-CHAN-01")

	refused := map[string]struct {
		from, to, basisKind string
		constraints         []string
	}{
		"已抵扣不凭认可的协议抵扣": {"IN_TRANSIT_AT_CHANNEL", "OFFSET_APPLIED", "DISCREPANCY",
			[]string{"subledger_posting_offset_needs_agreed_offset", "subledger_posting_agreed_offset_shape"}},
		"已抵扣自待清分落入": {"AWAITING_ALLOCATION", "OFFSET_APPLIED", "AGREED_OFFSET",
			[]string{"subledger_posting_offset_needs_agreed_offset", "subledger_posting_agreed_offset_shape"}},
		"已抵扣由冲正再进": {"IN_TRANSIT_AT_CHANNEL", "OFFSET_APPLIED", "CORRECTION",
			[]string{"subledger_posting_offset_needs_agreed_offset", "subledger_posting_agreed_offset_shape"}},
		"认可的协议抵扣落进短款": {"IN_TRANSIT_AT_CHANNEL", "SHORTFALL", "AGREED_OFFSET",
			[]string{"subledger_posting_agreed_offset_shape", "subledger_posting_variance_needs_discrepancy"}},
		"认可的协议抵扣入账": {"EXTERNAL_SOURCE", "IN_TRANSIT_AT_CHANNEL", "AGREED_OFFSET",
			[]string{"subledger_posting_agreed_offset_shape", "subledger_posting_intake_needs_collection_fact"}},
		"离开已抵扣不凭冲正": {"OFFSET_APPLIED", "PAYABLE_TO_CUSTOMER", "ALLOCATION",
			[]string{"subledger_posting_offset_leaves_only_by_correction"}},
		"依据种类不在词表": {"IN_TRANSIT_AT_CHANNEL", "OFFSET_APPLIED", "AGREED_OFFSETS",
			[]string{"subledger_posting_basis_kind_closed"}},
		"位置不在词表": {"IN_TRANSIT_AT_CHANNEL", "OFFSET_APPLIEDS", "ALLOCATION",
			[]string{"subledger_posting_to_position_closed"}},
	}
	for name, row := range refused {
		_, err := fixture.pool.Exec(t.Context(),
			`INSERT INTO collection_remittance.subledger_posting
				(tenant_id, posting_ref, customer_ref, legal_entity_ref, currency, channel_ref,
				 from_position, to_position, amount_minor, basis_kind, basis_ref, posted_at)
			 VALUES ('tenant-a', 'SYN-POST-BAD', 'SYN-CUST-01', 'SYN-LE-01', 'SYN-CUR-01', 'SYN-CHAN-01',
			         $1, $2, 100, $3, 'SYN-BASIS-1', $4)`,
			row.from, row.to, row.basisKind, catalogueBaseAt)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23514" || !slices.Contains(row.constraints, pgErr.ConstraintName) {
			t.Fatalf("%s：要撞 %v，实得 %v", name, row.constraints, err)
		}
	}

	for _, row := range []struct{ id, from, to, kind, basis string }{
		{"SYN-POST-OFFSET", "IN_TRANSIT_AT_CHANNEL", "OFFSET_APPLIED", "AGREED_OFFSET", "SYN-OFFSET-1"},
		{"SYN-POST-FIX", "OFFSET_APPLIED", "IN_TRANSIT_AT_CHANNEL", "CORRECTION", "SYN-POST-OFFSET"},
	} {
		if _, err := fixture.pool.Exec(t.Context(),
			`INSERT INTO collection_remittance.subledger_posting
				(tenant_id, posting_ref, customer_ref, legal_entity_ref, currency, channel_ref,
				 from_position, to_position, amount_minor, basis_kind, basis_ref, posted_at)
			 VALUES ('tenant-a', $1, 'SYN-CUST-01', 'SYN-LE-01', 'SYN-CUR-01', 'SYN-CHAN-01',
			         $2, $3, 100, $4, $5, $6)`,
			row.id, row.from, row.to, row.kind, row.basis, catalogueBaseAt); err != nil {
			t.Fatalf("合法的 %s 被库拒了：%v", row.id, err)
		}
	}
}

// 一条认可的抵扣至多支撑一笔记账、一笔记账至多被冲正一次，都由库上的部分唯一索引兜
// 住：绕过分户账锁的写入者也落不了第二笔。写口照 DO NOTHING 答`已登记`，编排按记账
// 标识读不回自己那一笔，答内容冲突。
// Covers: AT-CR-025（库约束半边）
func TestTheDatabaseKeepsOnePostingPerAgreedOffsetAndOneCorrectionPerOriginal(t *testing.T) {
	db := newDB(t)
	tenant := fixtureTenant(t)
	collections, err := adapter.NewCollectionRegistrations(db)
	if err != nil {
		t.Fatalf("构造代收面写口：%v", err)
	}
	subledgers, err := adapter.NewSubledgerStore(db)
	if err != nil {
		t.Fatalf("构造分户账写口：%v", err)
	}
	view, err := adapter.NewSubledgerBalanceView(db)
	if err != nil {
		t.Fatalf("构造分户账读口：%v", err)
	}

	if err := withinTransaction(t, db, func(ctx context.Context) error {
		if _, err := subledgers.OpenSubledger(ctx, tenant, fixtureSubledger(t)); err != nil {
			return err
		}
		if _, err := collections.RegisterInstruction(
			ctx, tenant, fixtureInstruction(t, "SYN-INSTR-1", 12000)); err != nil {
			return err
		}
		if _, err := collections.AcceptCollectionFact(ctx, tenant,
			fixtureFact(t, "SYN-FACT-REPORT", "SYN-INSTR-1", domain.ChannelCollectionReport, 12000)); err != nil {
			return err
		}
		if _, err := collections.RegisterAgreedOffset(ctx, tenant,
			fixtureOffset(t, "SYN-OFFSET-1", "SYN-INSTR-1", "SYN-SA-APPROVAL-1", 300)); err != nil {
			return err
		}
		steps := []struct {
			posting domain.SubledgerPosting
			want    ports.SaveOutcome
			name    string
		}{
			{fixturePosting(t, "SYN-POST-REPORT", domain.PositionExternalSource, domain.PositionInTransitAtChannel,
				12000, domain.BasisCollectionFact, "SYN-FACT-REPORT", "2026-08-24T01:10:00Z"),
				ports.SaveRegistered, "入账"},
			{fixturePosting(t, "SYN-POST-OFFSET", domain.PositionInTransitAtChannel, domain.PositionOffsetApplied,
				300, domain.BasisAgreedOffset, "SYN-OFFSET-1", "2026-08-25T01:10:00Z"),
				ports.SaveRegistered, "抵扣落账"},
			{fixturePosting(t, "SYN-POST-OFFSET-B", domain.PositionInTransitAtChannel, domain.PositionOffsetApplied,
				300, domain.BasisAgreedOffset, "SYN-OFFSET-1", "2026-08-25T01:20:00Z"),
				ports.SaveAlreadyRegistered, "换标识再落同一条认可"},
			{fixturePosting(t, "SYN-POST-FIX", domain.PositionOffsetApplied, domain.PositionInTransitAtChannel,
				300, domain.BasisCorrection, "SYN-POST-OFFSET", "2026-08-26T01:10:00Z"),
				ports.SaveRegistered, "冲正"},
			{fixturePosting(t, "SYN-POST-FIX-B", domain.PositionOffsetApplied, domain.PositionInTransitAtChannel,
				300, domain.BasisCorrection, "SYN-POST-OFFSET", "2026-08-26T01:20:00Z"),
				ports.SaveAlreadyRegistered, "换标识再冲正同一笔"},
		}
		for _, step := range steps {
			outcome, err := subledgers.AppendPosting(ctx, tenant, step.posting)
			if err != nil {
				return err
			}
			if outcome != step.want {
				t.Fatalf("%s = %v，要 %v", step.name, outcome, step.want)
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("落账：%v", err)
	}

	for _, id := range []string{"SYN-POST-OFFSET-B", "SYN-POST-FIX-B"} {
		if _, found, err := view.LoadPosting(t.Context(), tenant, fixturePostingID(t, id)); err != nil || found {
			t.Fatalf("%s 落库了：found=%v err=%v", id, found, err)
		}
	}
	balance, _, err := view.LoadBalance(t.Context(), tenant, fixtureLedgerKey(t))
	if err != nil {
		t.Fatalf("读回账面：%v", err)
	}
	if balance.PostingCount() != 3 || balance.IntakeTotal() != 12000 ||
		balance.At(domain.PositionOffsetApplied) != 0 || balance.At(domain.PositionInTransitAtChannel) != 12000 {
		t.Fatalf("账面走样：记账 %d 笔、入账 %d、已抵扣 %d、在途 %d",
			balance.PostingCount(), balance.IntakeTotal(),
			balance.At(domain.PositionOffsetApplied), balance.At(domain.PositionInTransitAtChannel))
	}

	reversed, found, err := view.LoadCorrectionOf(t.Context(), tenant, fixturePostingID(t, "SYN-POST-OFFSET"))
	if err != nil || !found || reversed.ID().String() != "SYN-POST-FIX" {
		t.Fatalf("冲正了抵扣的那一笔 = %v（found=%v err=%v），要 SYN-POST-FIX", reversed.ID(), found, err)
	}
	if _, found, err := view.LoadCorrectionOf(t.Context(), tenant, fixturePostingID(t, "SYN-POST-REPORT")); err != nil || found {
		t.Fatalf("没被冲正的记账读出了冲正：found=%v err=%v", found, err)
	}
}

// 按指令取记账带上凭它的认可抵扣与冲正抵扣的记账，不带同账别的指令的抵扣；派生出的
// 在途上限随抵扣落账变小、随冲正回到原值。
func TestInstructionReadsFollowItsAgreedOffsetsAndTheirReversals(t *testing.T) {
	db := newDB(t)
	tenant := fixtureTenant(t)
	collections, err := adapter.NewCollectionRegistrations(db)
	if err != nil {
		t.Fatalf("构造代收面写口：%v", err)
	}
	subledgers, err := adapter.NewSubledgerStore(db)
	if err != nil {
		t.Fatalf("构造分户账写口：%v", err)
	}
	subledgerView, err := adapter.NewSubledgerBalanceView(db)
	if err != nil {
		t.Fatalf("构造分户账读口：%v", err)
	}

	instructionX := fixtureInstruction(t, "SYN-INSTR-X", 100)
	if err := withinTransaction(t, db, func(ctx context.Context) error {
		if _, err := subledgers.OpenSubledger(ctx, tenant, fixtureSubledger(t)); err != nil {
			return err
		}
		for _, instruction := range []domain.CollectionInstruction{
			instructionX, fixtureInstruction(t, "SYN-INSTR-Y", 50),
		} {
			if _, err := collections.RegisterInstruction(ctx, tenant, instruction); err != nil {
				return err
			}
		}
		for _, fact := range []domain.CollectionFact{
			fixtureFact(t, "SYN-FACT-X-REPORT", "SYN-INSTR-X", domain.ChannelCollectionReport, 100),
			fixtureFact(t, "SYN-FACT-Y-REPORT", "SYN-INSTR-Y", domain.ChannelCollectionReport, 50),
		} {
			if _, err := collections.AcceptCollectionFact(ctx, tenant, fact); err != nil {
				return err
			}
		}
		for _, offset := range []domain.AgreedOffset{
			fixtureOffset(t, "SYN-OFFSET-X", "SYN-INSTR-X", "SYN-SA-APPROVAL-X", 3),
			fixtureOffset(t, "SYN-OFFSET-Y", "SYN-INSTR-Y", "SYN-SA-APPROVAL-Y", 2),
		} {
			if _, err := collections.RegisterAgreedOffset(ctx, tenant, offset); err != nil {
				return err
			}
		}
		for _, posting := range []domain.SubledgerPosting{
			fixturePosting(t, "SYN-POST-X-REPORT", domain.PositionExternalSource, domain.PositionInTransitAtChannel,
				100, domain.BasisCollectionFact, "SYN-FACT-X-REPORT", "2026-08-24T01:00:00Z"),
			fixturePosting(t, "SYN-POST-Y-REPORT", domain.PositionExternalSource, domain.PositionInTransitAtChannel,
				50, domain.BasisCollectionFact, "SYN-FACT-Y-REPORT", "2026-08-24T01:05:00Z"),
			fixturePosting(t, "SYN-POST-X-OFFSET", domain.PositionInTransitAtChannel, domain.PositionOffsetApplied,
				3, domain.BasisAgreedOffset, "SYN-OFFSET-X", "2026-08-25T01:00:00Z"),
			fixturePosting(t, "SYN-POST-Y-OFFSET", domain.PositionInTransitAtChannel, domain.PositionOffsetApplied,
				2, domain.BasisAgreedOffset, "SYN-OFFSET-Y", "2026-08-25T01:05:00Z"),
		} {
			if _, err := subledgers.AppendPosting(ctx, tenant, posting); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("登记与落账：%v", err)
	}

	postings, err := subledgerView.LoadInstructionPostings(t.Context(), tenant, instructionX)
	if err != nil {
		t.Fatalf("按指令取记账：%v", err)
	}
	if want := []string{"SYN-POST-X-REPORT", "SYN-POST-X-OFFSET"}; !slices.Equal(postingIDs(postings), want) {
		t.Fatalf("按指令取到的记账 = %v，要 %v", postingIDs(postings), want)
	}
	principal, err := domain.DeriveInstructionPrincipal(instructionX, nil, postings)
	if err != nil {
		t.Fatalf("派生指令名下本金：%v", err)
	}
	if principal.TransferCeiling() != 97 || principal.OffsetApplied() != 3 {
		t.Fatalf("X 的移位上限 %d、已抵扣 %d，要 97 与 3——Y 的抵扣 2 不算",
			principal.TransferCeiling(), principal.OffsetApplied())
	}

	if err := withinTransaction(t, db, func(ctx context.Context) error {
		_, err := subledgers.AppendPosting(ctx, tenant, fixturePosting(t, "SYN-POST-X-OFFSET-FIX",
			domain.PositionOffsetApplied, domain.PositionInTransitAtChannel,
			3, domain.BasisCorrection, "SYN-POST-X-OFFSET", "2026-08-26T01:00:00Z"))
		return err
	}); err != nil {
		t.Fatalf("冲正：%v", err)
	}
	postings, err = subledgerView.LoadInstructionPostings(t.Context(), tenant, instructionX)
	if err != nil {
		t.Fatalf("按指令取记账：%v", err)
	}
	if want := []string{"SYN-POST-X-REPORT", "SYN-POST-X-OFFSET", "SYN-POST-X-OFFSET-FIX"}; !slices.Equal(postingIDs(postings), want) {
		t.Fatalf("冲正后按指令取到的记账 = %v，要 %v（冲正沿链追溯到 X）", postingIDs(postings), want)
	}
	principal, err = domain.DeriveInstructionPrincipal(instructionX, nil, postings)
	if err != nil {
		t.Fatalf("派生指令名下本金：%v", err)
	}
	if principal.TransferCeiling() != 100 || principal.OffsetApplied() != 0 {
		t.Fatalf("冲正后 X 的移位上限 %d、已抵扣 %d，要 100 与 0",
			principal.TransferCeiling(), principal.OffsetApplied())
	}
}
