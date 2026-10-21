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

// 本文件在真实 PostgreSQL 上证 ADR-0271 落在库上的几格（隔离合成 S）：凭代收事实至多
// 一笔记账的唯一约束、入账与差额来源位置的 CHECK，以及按指令取事实与记账的两个读口。

// 换记账标识再凭同一条事实落账，写口照 DO NOTHING 答`已登记`而不落第二行；编排按标识
// 读不回它，于是答内容冲突。
// Covers: AT-CR-008（库约束半边）
func TestTheDatabaseKeepsOnePostingPerCollectionFact(t *testing.T) {
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
		if outcome, err := subledgers.AppendPosting(ctx, tenant, fixturePosting(t, "SYN-POST-A",
			domain.PositionExternalSource, domain.PositionInTransitAtChannel,
			12000, domain.BasisCollectionFact, "SYN-FACT-REPORT", "2026-08-24T01:10:00Z")); err != nil ||
			outcome != ports.SaveRegistered {
			t.Fatalf("首笔入账 = %v（err=%v）", outcome, err)
		}
		outcome, err := subledgers.AppendPosting(ctx, tenant, fixturePosting(t, "SYN-POST-B",
			domain.PositionExternalSource, domain.PositionInTransitAtChannel,
			12000, domain.BasisCollectionFact, "SYN-FACT-REPORT", "2026-08-24T01:20:00Z"))
		if err != nil {
			return err
		}
		if outcome != ports.SaveAlreadyRegistered {
			t.Fatalf("换标识再凭同一条事实入账 = %v，要 AlreadyRegistered", outcome)
		}
		return nil
	}); err != nil {
		t.Fatalf("落账：%v", err)
	}

	if _, found, err := view.LoadPosting(t.Context(), tenant, fixturePostingID(t, "SYN-POST-B")); err != nil || found {
		t.Fatalf("第二笔落库了：found=%v err=%v", found, err)
	}
	balance, _, err := view.LoadBalance(t.Context(), tenant, fixtureLedgerKey(t))
	if err != nil {
		t.Fatalf("读回账面：%v", err)
	}
	if balance.IntakeTotal() != 12000 || balance.PostingCount() != 1 {
		t.Fatalf("入账 %d、记账 %d 笔，要 12000 与 1 笔", balance.IntakeTotal(), balance.PostingCount())
	}
}

// 领域构造门拦得住的形，库上也拦得住：绕过写口的行照样进不来。
// Covers: AT-CR-016（库约束半边）
func TestTheDatabaseFixesWhereIntakeAndVariancesComeFrom(t *testing.T) {
	fixture := newCatalogueFixture(t)
	fixture.seedSubledger(t, "tenant-a", "SYN-CUST-01", "SYN-LE-01", "SYN-CUR-01", "SYN-CHAN-01")

	// 入账一步落到待清分同时违反入账与凭代收事实两条，库先报哪一条不归本用例管。
	refused := map[string]struct {
		from, to, basisKind string
		constraints         []string
	}{
		"入账一步落到待清分": {"EXTERNAL_SOURCE", "AWAITING_ALLOCATION", "COLLECTION_FACT",
			[]string{"subledger_posting_intake_lands_in_transit", "subledger_posting_collection_fact_shapes"}},
		"凭代收事实自待清分移回在途": {"AWAITING_ALLOCATION", "IN_TRANSIT_AT_CHANNEL", "COLLECTION_FACT",
			[]string{"subledger_posting_collection_fact_shapes"}},
		"短款自待清分落入": {"AWAITING_ALLOCATION", "SHORTFALL", "DISCREPANCY",
			[]string{"subledger_posting_shortfall_from_in_transit"}},
		"溢款自渠道在途落入": {"IN_TRANSIT_AT_CHANNEL", "SURPLUS", "DISCREPANCY",
			[]string{"subledger_posting_surplus_from_awaiting_allocation"}},
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

	fixture.seed(t,
		`INSERT INTO collection_remittance.subledger_posting
			(tenant_id, posting_ref, customer_ref, legal_entity_ref, currency, channel_ref,
			 from_position, to_position, amount_minor, basis_kind, basis_ref, posted_at)
		 VALUES ('tenant-a', 'SYN-POST-1', 'SYN-CUST-01', 'SYN-LE-01', 'SYN-CUR-01', 'SYN-CHAN-01',
		         'EXTERNAL_SOURCE', 'IN_TRANSIT_AT_CHANNEL', 100, 'COLLECTION_FACT', 'SYN-FACT-1', $1)`,
		catalogueBaseAt)
	_, err := fixture.pool.Exec(t.Context(),
		`INSERT INTO collection_remittance.subledger_posting
			(tenant_id, posting_ref, customer_ref, legal_entity_ref, currency, channel_ref,
			 from_position, to_position, amount_minor, basis_kind, basis_ref, posted_at)
		 VALUES ('tenant-a', 'SYN-POST-2', 'SYN-CUST-01', 'SYN-LE-01', 'SYN-CUR-01', 'SYN-CHAN-01',
		         'IN_TRANSIT_AT_CHANNEL', 'AWAITING_ALLOCATION', 100, 'COLLECTION_FACT', 'SYN-FACT-1', $1)`,
		catalogueBaseAt)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" ||
		pgErr.ConstraintName != "subledger_posting_one_per_collection_fact" {
		t.Fatalf("同一条事实支撑第二笔记账：要撞唯一约束，实得 %v", err)
	}
}

// 按指令取记账：凭它的事实、凭它的差异事项、凭它清分的，以及沿冲正链冲正这些记账的；
// 同账别的指令的记账与凭批次的汇付不在其中。按指令取事实只交回它名下的那几条。
func TestInstructionReadsFollowItsFactsItemsAllocationsAndCorrections(t *testing.T) {
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
	remittances, err := adapter.NewRemittanceStore(db)
	if err != nil {
		t.Fatalf("构造回汇写口：%v", err)
	}
	collectionView, err := adapter.NewCollectionPointView(db)
	if err != nil {
		t.Fatalf("构造代收面读口：%v", err)
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
			fixtureFact(t, "SYN-FACT-X-CREDIT", "SYN-INSTR-X", domain.OperatorBankCredit, 96),
			fixtureFact(t, "SYN-FACT-X-POD", "SYN-INSTR-X", domain.RecipientPayment, 100),
			fixtureFact(t, "SYN-FACT-Y-REPORT", "SYN-INSTR-Y", domain.ChannelCollectionReport, 50),
		} {
			if _, err := collections.AcceptCollectionFact(ctx, tenant, fact); err != nil {
				return err
			}
		}
		if _, err := collections.RegisterDiscrepancyItem(ctx, tenant,
			fixtureDiscrepancy(t, "SYN-DIFF-X", "SYN-INSTR-X", 4)); err != nil {
			return err
		}
		if _, err := remittances.FormBatch(ctx, tenant, fixtureBatch(t, "SYN-BATCH-1")); err != nil {
			return err
		}
		for _, posting := range []domain.SubledgerPosting{
			fixturePosting(t, "SYN-POST-X-REPORT", domain.PositionExternalSource, domain.PositionInTransitAtChannel,
				100, domain.BasisCollectionFact, "SYN-FACT-X-REPORT", "2026-08-24T01:00:00Z"),
			fixturePosting(t, "SYN-POST-Y-REPORT", domain.PositionExternalSource, domain.PositionInTransitAtChannel,
				50, domain.BasisCollectionFact, "SYN-FACT-Y-REPORT", "2026-08-24T01:05:00Z"),
			fixturePosting(t, "SYN-POST-X-CREDIT", domain.PositionInTransitAtChannel, domain.PositionAwaitingAllocation,
				96, domain.BasisCollectionFact, "SYN-FACT-X-CREDIT", "2026-08-24T02:00:00Z"),
			fixturePosting(t, "SYN-POST-X-SHORT", domain.PositionInTransitAtChannel, domain.PositionShortfall,
				4, domain.BasisDiscrepancy, "SYN-DIFF-X", "2026-08-25T01:00:00Z"),
			fixturePosting(t, "SYN-POST-X-SHORT-FIX", domain.PositionShortfall, domain.PositionInTransitAtChannel,
				4, domain.BasisCorrection, "SYN-POST-X-SHORT", "2026-08-25T02:00:00Z"),
			fixturePosting(t, "SYN-POST-X-ALLOC", domain.PositionAwaitingAllocation, domain.PositionPayableToCustomer,
				96, domain.BasisAllocation, "SYN-INSTR-X", "2026-08-26T01:00:00Z"),
			fixturePosting(t, "SYN-POST-REMIT", domain.PositionPayableToCustomer, domain.PositionRemitted,
				96, domain.BasisRemittanceBatch, "SYN-BATCH-1", "2026-08-31T02:00:00Z"),
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
	want := []string{"SYN-POST-X-REPORT", "SYN-POST-X-CREDIT", "SYN-POST-X-SHORT", "SYN-POST-X-SHORT-FIX", "SYN-POST-X-ALLOC"}
	if len(postings) != len(want) {
		t.Fatalf("按指令取到 %d 笔，要 %d 笔：%v", len(postings), len(want), postingIDs(postings))
	}
	for index, id := range want {
		if postings[index].ID().String() != id {
			t.Fatalf("按指令取到的记账 = %v，要 %v（按记账时刻排）", postingIDs(postings), want)
		}
	}

	facts, err := collectionView.LoadInstructionFacts(t.Context(), tenant, instructionX.ID())
	if err != nil {
		t.Fatalf("按指令取事实：%v", err)
	}
	if len(facts) != 3 {
		t.Fatalf("按指令取到 %d 条事实，要 3 条", len(facts))
	}
	for _, fact := range facts {
		if fact.Instruction() != instructionX.ID() {
			t.Fatalf("取到别的指令的事实：%s", fact.ID())
		}
	}

	principal, err := domain.DeriveInstructionPrincipal(instructionX, facts, postings)
	if err != nil {
		t.Fatalf("派生指令名下本金：%v", err)
	}
	if principal.TransferCeiling() != 4 {
		t.Fatalf("X 的移位上限 = %d，要 4——冲正回在途的短款算回 X，Y 的 50 不算", principal.TransferCeiling())
	}
}

func fixturePostingID(t *testing.T, value string) domain.PostingID {
	t.Helper()
	id, err := domain.NewPostingID(value)
	if err != nil {
		t.Fatalf("构造记账标识 %q：%v", value, err)
	}
	return id
}

func postingIDs(postings []domain.SubledgerPosting) []string {
	ids := make([]string, 0, len(postings))
	for _, posting := range postings {
		ids = append(ids, posting.ID().String())
	}
	return ids
}
