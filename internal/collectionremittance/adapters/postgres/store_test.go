package postgres_test

import (
	"context"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/collectionremittance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/collectionremittance/domain"
	"go.idp.xyz/idp-parcel/internal/collectionremittance/ports"
)

// 本文件在真实 PostgreSQL 上证六张表的写口与点读口往返（隔离合成 S）。

func withinTransaction(t *testing.T, db *bentopg.DB, body func(ctx context.Context) error) error {
	t.Helper()
	return db.Transactor().WithinTransaction(t.Context(), body)
}

// 四层来源在真库上往返不串：读回的层级必须还是写下去的那一层。串了的话，一笔真实
// 到账在读回后会变成渠道在途，而两者撑得起的下一步不同。
func TestEachSourceLayerSurvivesTheRoundTrip(t *testing.T) {
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

	wantIntake := map[domain.CollectionSourceLayer]domain.FundPosition{
		domain.RecipientPayment:        domain.PositionInTransitAtChannel,
		domain.ChannelCollectionReport: domain.PositionInTransitAtChannel,
		domain.ChannelRemittanceNotice: domain.PositionInTransitAtChannel,
		domain.OperatorBankCredit:      domain.PositionAwaitingAllocation,
	}
	if err := withinTransaction(t, db, func(ctx context.Context) error {
		if _, err := collections.RegisterInstruction(
			ctx, tenant, fixtureInstruction(t, "SYN-INSTR-1", 12000)); err != nil {
			return err
		}
		for layer := range wantIntake {
			if _, err := collections.AcceptCollectionFact(ctx, tenant,
				fixtureFact(t, "SYN-FACT-"+layer.String(), "SYN-INSTR-1", layer, 12000)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("登记代收面：%v", err)
	}

	for layer, want := range wantIntake {
		fact, found, err := view.LoadCollectionFact(
			t.Context(), tenant, fixtureFactID(t, "SYN-FACT-"+layer.String()))
		if err != nil || !found {
			t.Fatalf("读回 %s 事实：found=%v err=%v", layer, found, err)
		}
		if fact.Layer() != layer {
			t.Fatalf("%s 读回成了 %s", layer, fact.Layer())
		}
		if got := fact.IntakePosition(); got != want {
			t.Fatalf("%s 读回后的入账位置 = %s，要 %s", layer, got, want)
		}
	}
}

// 指令与分户账往返逐字段对上；同键重放答`已登记`，写口一律不覆盖。
func TestRegistrationsRoundTripAndNeverOverwrite(t *testing.T) {
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
	collectionView, err := adapter.NewCollectionPointView(db)
	if err != nil {
		t.Fatalf("构造代收面读口：%v", err)
	}
	subledgerView, err := adapter.NewSubledgerBalanceView(db)
	if err != nil {
		t.Fatalf("构造分户账读口：%v", err)
	}

	if err := withinTransaction(t, db, func(ctx context.Context) error {
		if outcome, err := subledgers.OpenSubledger(ctx, tenant, fixtureSubledger(t)); err != nil ||
			outcome != ports.SaveRegistered {
			t.Fatalf("开立分户账 = %v（err=%v）", outcome, err)
		}
		if outcome, err := subledgers.OpenSubledger(ctx, tenant, fixtureSubledger(t)); err != nil ||
			outcome != ports.SaveAlreadyRegistered {
			t.Fatalf("重放开立 = %v（err=%v），要 AlreadyRegistered", outcome, err)
		}
		if outcome, err := collections.RegisterInstruction(
			ctx, tenant, fixtureInstruction(t, "SYN-INSTR-1", 12000)); err != nil ||
			outcome != ports.SaveRegistered {
			t.Fatalf("登记指令 = %v（err=%v）", outcome, err)
		}
		// 同键异内容：写口照样只答`已登记`，绝不覆盖——比对是编排的事。
		if outcome, err := collections.RegisterInstruction(
			ctx, tenant, fixtureInstruction(t, "SYN-INSTR-1", 13000)); err != nil ||
			outcome != ports.SaveAlreadyRegistered {
			t.Fatalf("同键异内容 = %v（err=%v），要 AlreadyRegistered", outcome, err)
		}
		return nil
	}); err != nil {
		t.Fatalf("登记：%v", err)
	}

	instruction, found, err := collectionView.LoadInstruction(
		t.Context(), tenant, fixtureInstructionID(t, "SYN-INSTR-1"))
	if err != nil || !found {
		t.Fatalf("读回指令：found=%v err=%v", found, err)
	}
	if instruction.Amount().AmountMinor() != 12000 {
		t.Fatalf("金额被第二次登记顶掉了：%d", instruction.Amount().AmountMinor())
	}
	if instruction.Ledger() != fixtureLedgerKey(t) {
		t.Fatal("分户账四维键往返后不同")
	}

	ledger, opened, err := subledgerView.LoadSubledger(t.Context(), tenant, fixtureLedgerKey(t))
	if err != nil || !opened {
		t.Fatalf("读回分户账：opened=%v err=%v", opened, err)
	}
	if ledger.CustodyBasis().String() != "SYN-COD-RESPONSIBILITY-V1" {
		t.Fatalf("受托依据往返后不同：%s", ledger.CustodyBasis())
	}
}

// 「已开立但当期无记账」与「未开立」在读口上是两个答案，不是同一个空。
func TestAnOpenedLedgerWithoutPostingsIsNotTheSameAsNoLedger(t *testing.T) {
	db := newDB(t)
	tenant := fixtureTenant(t)
	subledgers, err := adapter.NewSubledgerStore(db)
	if err != nil {
		t.Fatalf("构造分户账写口：%v", err)
	}
	view, err := adapter.NewSubledgerBalanceView(db)
	if err != nil {
		t.Fatalf("构造分户账读口：%v", err)
	}

	if _, opened, err := view.LoadBalance(t.Context(), tenant, fixtureLedgerKey(t)); err != nil || opened {
		t.Fatalf("未开立的账答了余额：opened=%v err=%v", opened, err)
	}

	if err := withinTransaction(t, db, func(ctx context.Context) error {
		_, err := subledgers.OpenSubledger(ctx, tenant, fixtureSubledger(t))
		return err
	}); err != nil {
		t.Fatalf("开立分户账：%v", err)
	}

	balance, opened, err := view.LoadBalance(t.Context(), tenant, fixtureLedgerKey(t))
	if err != nil || !opened {
		t.Fatalf("已开立的账读不出余额：opened=%v err=%v", opened, err)
	}
	if balance.PostingCount() != 0 || balance.IntakeTotal() != 0 {
		t.Fatalf("零记账账面不为零：笔数 %d 入账 %d", balance.PostingCount(), balance.IntakeTotal())
	}
}

// 记账在真库上追加、按分户账键派生余额、单笔点读往返；批次状态单向推进。
func TestPostingsAccumulateAndBatchStateAdvancesOnce(t *testing.T) {
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
	subledgerView, err := adapter.NewSubledgerBalanceView(db)
	if err != nil {
		t.Fatalf("构造分户账读口：%v", err)
	}
	batchView, err := adapter.NewRemittanceBatchView(db)
	if err != nil {
		t.Fatalf("构造回汇读口：%v", err)
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
			fixtureFact(t, "SYN-FACT-CREDIT", "SYN-INSTR-1", domain.OperatorBankCredit, 12000)); err != nil {
			return err
		}
		if _, err := remittances.FormBatch(ctx, tenant, fixtureBatch(t, "SYN-BATCH-1")); err != nil {
			return err
		}
		if _, err := subledgers.AppendPosting(ctx, tenant, fixturePosting(t, "SYN-POST-1",
			domain.PositionExternalSource, domain.PositionAwaitingAllocation,
			12000, domain.BasisCollectionFact, "SYN-FACT-CREDIT", "2026-08-24T01:10:00Z")); err != nil {
			return err
		}
		if _, err := subledgers.AppendPosting(ctx, tenant, fixturePosting(t, "SYN-POST-2",
			domain.PositionAwaitingAllocation, domain.PositionPayableToCustomer,
			8000, domain.BasisAllocation, "SYN-INSTR-1", "2026-08-24T03:00:00Z")); err != nil {
			return err
		}
		if _, err := subledgers.AppendPosting(ctx, tenant, fixturePosting(t, "SYN-POST-3",
			domain.PositionPayableToCustomer, domain.PositionRemitted,
			5000, domain.BasisRemittanceBatch, "SYN-BATCH-1", "2026-08-31T02:00:00Z")); err != nil {
			return err
		}
		// 重放同一笔记账：追加式写口答`已登记`，不写第二行。
		outcome, err := subledgers.AppendPosting(ctx, tenant, fixturePosting(t, "SYN-POST-3",
			domain.PositionPayableToCustomer, domain.PositionRemitted,
			5000, domain.BasisRemittanceBatch, "SYN-BATCH-1", "2026-08-31T02:00:00Z"))
		if err != nil {
			return err
		}
		if outcome != ports.SaveAlreadyRegistered {
			t.Fatalf("重放记账 = %v，要 AlreadyRegistered", outcome)
		}
		return nil
	}); err != nil {
		t.Fatalf("落账：%v", err)
	}

	balance, opened, err := subledgerView.LoadBalance(t.Context(), tenant, fixtureLedgerKey(t))
	if err != nil || !opened {
		t.Fatalf("读回账面：opened=%v err=%v", opened, err)
	}
	if balance.PostingCount() != 3 {
		t.Fatalf("记账笔数 = %d，要 3（重放没写第二行）", balance.PostingCount())
	}
	if balance.At(domain.PositionAwaitingAllocation) != 4000 ||
		balance.At(domain.PositionPayableToCustomer) != 3000 ||
		balance.At(domain.PositionRemitted) != 5000 ||
		balance.IntakeTotal() != 12000 {
		t.Fatalf("余额派生不对：待清分 %d 应付客户 %d 已汇付 %d 入账 %d",
			balance.At(domain.PositionAwaitingAllocation),
			balance.At(domain.PositionPayableToCustomer),
			balance.At(domain.PositionRemitted), balance.IntakeTotal())
	}

	postingID, err := domain.NewPostingID("SYN-POST-2")
	if err != nil {
		t.Fatalf("构造记账标识：%v", err)
	}
	posting, found, err := subledgerView.LoadPosting(t.Context(), tenant, postingID)
	if err != nil || !found {
		t.Fatalf("点读记账：found=%v err=%v", found, err)
	}
	if posting.BasisKind() != domain.BasisAllocation || posting.Basis().String() != "SYN-INSTR-1" {
		t.Fatalf("记账依据往返后不同：%s / %s", posting.BasisKind(), posting.Basis())
	}
	if posting.Ledger() != fixtureLedgerKey(t) {
		t.Fatal("记账所属分户账往返后不同")
	}

	// 批次状态：一次推进、再交回`已交出`、不在册答无对象。
	if err := withinTransaction(t, db, func(ctx context.Context) error {
		outcome, err := remittances.HandOverBatchForPayment(ctx, tenant, fixtureBatchID(t, "SYN-BATCH-1"))
		if err != nil {
			return err
		}
		if outcome != ports.HandedOver {
			t.Fatalf("首次交出 = %v", outcome)
		}
		outcome, err = remittances.HandOverBatchForPayment(ctx, tenant, fixtureBatchID(t, "SYN-BATCH-1"))
		if err != nil {
			return err
		}
		if outcome != ports.AlreadyHandedOver {
			t.Fatalf("重复交出 = %v，要 AlreadyHandedOver——零行的两种成因要分得开", outcome)
		}
		outcome, err = remittances.HandOverBatchForPayment(ctx, tenant, fixtureBatchID(t, "SYN-BATCH-NEVER"))
		if err != nil {
			return err
		}
		if outcome != ports.HandOverTargetMissing {
			t.Fatalf("不在册的批次 = %v，要 HandOverTargetMissing", outcome)
		}
		return nil
	}); err != nil {
		t.Fatalf("交出汇付主张：%v", err)
	}

	batch, found, err := batchView.LoadBatch(t.Context(), tenant, fixtureBatchID(t, "SYN-BATCH-1"))
	if err != nil || !found {
		t.Fatalf("读回批次：found=%v err=%v", found, err)
	}
	if batch.State() != domain.BatchHandedForPayment {
		t.Fatalf("批次状态读回成了 %s——已交出的批次不该退回已归集", batch.State())
	}
	if batch.AcceptsRemittance() {
		t.Fatal("已交出主张的批次读回后仍收成员")
	}
}

// 无来源的本金进不了库：孤儿事实撞外键防线，写口据实上抛而不是折成一个业务答案。
func TestAnOrphanFactHitsTheForeignKeyRatherThanLanding(t *testing.T) {
	db := newDB(t)
	tenant := fixtureTenant(t)
	collections, err := adapter.NewCollectionRegistrations(db)
	if err != nil {
		t.Fatalf("构造代收面写口：%v", err)
	}

	err = withinTransaction(t, db, func(ctx context.Context) error {
		_, err := collections.AcceptCollectionFact(ctx, tenant,
			fixtureFact(t, "SYN-FACT-ORPHAN", "SYN-INSTR-NEVER", domain.RecipientPayment, 12000))
		return err
	})
	if err == nil {
		t.Fatal("孤儿事实落库了——没有代收指令就没有代收义务")
	}
}
