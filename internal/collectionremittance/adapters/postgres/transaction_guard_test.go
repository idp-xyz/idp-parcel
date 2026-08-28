package postgres_test

import (
	"errors"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/collectionremittance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/collectionremittance/domain"
)

// PBC-08 行为面负向证据：本包七个写口在无事务上下文必须被 RequireExecutor 拒绝。
//
// 入参一律用构造门造出来的有效值而不是零值——AcceptCollectionFact、
// RegisterDiscrepancyItem、AppendPosting、FormBatch 与 HandOverBatchForPayment 都把
// 「封闭词表之外即拒」的入参门放在事务守卫之前，传零值会在守卫之前就被那道门拦下，
// 于是这条用例会绿得毫无意义：它证到的是入参门，不是事务守卫。
func TestCollectionRemittanceWritesRefuseToRunOutsideATransaction(t *testing.T) {
	db := newDB(t)
	ctx := t.Context()
	tenant := fixtureTenant(t)

	collections, err := adapter.NewCollectionRegistrations(db)
	if err != nil {
		t.Fatalf("构造代收面写口：%v", err)
	}
	if _, err := collections.RegisterInstruction(
		ctx, tenant, fixtureInstruction(t, "SYN-INSTR-1", 12000),
	); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记代收指令应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := collections.AcceptCollectionFact(
		ctx, tenant, fixtureFact(t, "SYN-FACT-1", "SYN-INSTR-1", domain.RecipientPayment, 12000),
	); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务接受代收事实应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := collections.RegisterDiscrepancyItem(
		ctx, tenant, fixtureDiscrepancy(t, "SYN-DIFF-1", "SYN-INSTR-1", 2000),
	); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记差异事项应返回 ErrTransactionRequired，实得：%v", err)
	}

	subledgers, err := adapter.NewSubledgerStore(db)
	if err != nil {
		t.Fatalf("构造分户账写口：%v", err)
	}
	if _, err := subledgers.OpenSubledger(
		ctx, tenant, fixtureSubledger(t),
	); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务开立分户账应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := subledgers.AppendPosting(ctx, tenant, fixturePosting(t, "SYN-POST-1",
		domain.PositionExternalSource, domain.PositionInTransitAtChannel,
		12000, domain.BasisCollectionFact, "SYN-FACT-1", "2026-08-24T01:10:00Z"),
	); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务追加记账应返回 ErrTransactionRequired，实得：%v", err)
	}

	remittances, err := adapter.NewRemittanceStore(db)
	if err != nil {
		t.Fatalf("构造回汇写口：%v", err)
	}
	if _, err := remittances.FormBatch(
		ctx, tenant, fixtureBatch(t, "SYN-BATCH-1"),
	); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务形成回汇批次应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := remittances.HandOverBatchForPayment(
		ctx, tenant, fixtureBatchID(t, "SYN-BATCH-1"),
	); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务交出汇付主张应返回 ErrTransactionRequired，实得：%v", err)
	}
}
