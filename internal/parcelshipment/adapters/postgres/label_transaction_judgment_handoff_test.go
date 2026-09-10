package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

func newLabelTransactionJudgmentHandoffFixture(t *testing.T) (*adapter.OutboxLabelTransactionJudgmentHandoff, *bentopg.DB, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handoff, err := adapter.NewOutboxLabelTransactionJudgmentHandoff(db, store, handoffClock{
		at: time.Date(2026, 9, 10, 18, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造判断意图适配器：%v", err)
	}
	return handoff, db, pool
}

func labelTransactionJudgmentIntent(
	t *testing.T,
	transaction, parcel string,
	revision int64,
	beat ports.LabelTransactionBeat,
) ports.LabelTransactionJudgmentIntent {
	t.Helper()
	return ports.LabelTransactionJudgmentIntent{
		Tenant:        mustBuild(t, domain.NewTenantID, "tenant-a"),
		TransactionID: mustBuild(t, domain.NewLabelTransactionID, transaction),
		Parcel:        mustBuild(t, domain.NewDeclaredParcelID, parcel),
		Revision:      revision,
		Beat:          beat,
		OccurredAt:    time.Date(2026, 9, 10, 17, 30, 0, 0, time.UTC),
	}
}

func labelTransactionJudgmentEventID(transaction, parcel string, revision string) string {
	return "tenant-a/label-transaction/" + transaction + "/" + parcel + "/" + revision + "/judgment-due"
}

// TestTheWriteSideEnqueuesEachBeatInTheSameTransactionAsItsSave 把真编排、真仓储与真 outbox 适配器接在
// 一起，证 ADR-0134 决定三的三件：意图与结果同一事务落地（版本取 Save 成功那一代，两拍相邻各自成封）；
// 事务回滚则两样都不在；不在事务里调用在 Save 处响亮失败、outbox 零行——同事务不是编排自己开事务，
// 是仓储与 EnqueueOnce 都从 ctx 取同一个执行器这一既有约束的直接结果。
func TestTheWriteSideEnqueuesEachBeatInTheSameTransactionAsItsSave(t *testing.T) {
	handoff, db, pool := newLabelTransactionJudgmentHandoffFixture(t)
	repository, err := adapter.NewLabelTransactions(db)
	if err != nil {
		t.Fatalf("构造面单交易仓储：%v", err)
	}
	clock := handoffClock{at: time.Date(2026, 9, 10, 18, 0, 0, 0, time.UTC)}
	handler := psapplication.NewLabelTransactionHandler(psapplication.LabelTransactionDeps{
		Transactions: repository,
		Judgments:    handoff,
		Clock:        clock,
	})
	ctx := t.Context()
	transactor := db.Transactor()
	tenant := mustBuild(t, domain.NewTenantID, "tenant-a")
	transactionID := mustBuild(t, domain.NewLabelTransactionID, "LT-1")

	mustInsertLabelTransaction(t, transactor, ctx, repository,
		establishedLabelTransactionFixture(t, "tenant-a", "LT-1", "parcel-1", "parcel-2"))
	var submitted psapplication.LabelTransactionResult
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) (err error) {
		submitted, err = handler.SubmitToChannel(txCtx, psapplication.SubmitLabelTransactionCommand{
			Tenant: tenant, TransactionID: transactionID,
		})
		return err
	})
	if submitted.Outcome() != psapplication.LabelTransactionApplied {
		t.Fatalf("提交 outcome = %q, want APPLIED", submitted.Outcome())
	}
	if count := countOutboxEventsByType(t, pool, "parcel-shipment.label-transaction.judgment-due"); count != 0 {
		t.Fatalf("建立与提交交出了 %d 封判断意图——前三步不触发", count)
	}

	recordCommand := psapplication.RecordLabelChannelResultCommand{
		Tenant:        tenant,
		TransactionID: transactionID,
		Outcome:       domain.LabelTransactionSucceeded,
		ParcelResults: []domain.LabelTransactionParcelResultSpec{
			{Parcel: mustBuild(t, domain.NewDeclaredParcelID, "parcel-1"), Accepted: true,
				Identifier: mustBuild(t, domain.NewChannelParcelIdentifier, "channel-parcel-1")},
			{Parcel: mustBuild(t, domain.NewDeclaredParcelID, "parcel-2"), Accepted: true,
				Identifier: mustBuild(t, domain.NewChannelParcelIdentifier, "channel-parcel-2")},
		},
		ObservedAt: clock.at.Add(2 * time.Minute),
	}

	// 不在事务里：Save 处响亮失败，outbox 零行，交易仍停在`已提交渠道`。
	if _, err := handler.RecordChannelResult(ctx, recordCommand); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务记录结果应在 Save 处报 ErrTransactionRequired，实得：%v", err)
	}
	if count := countOutboxEventsByType(t, pool, "parcel-shipment.label-transaction.judgment-due"); count != 0 {
		t.Fatalf("无事务调用仍留下 %d 封意图", count)
	}

	// 事务回滚：结果与意图两样都不在。
	rollback := errors.New("回滚")
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := handler.RecordChannelResult(txCtx, recordCommand); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if count := countOutboxEventsByType(t, pool, "parcel-shipment.label-transaction.judgment-due"); count != 0 {
		t.Fatalf("回滚后仍有 %d 封意图", count)
	}
	if state := storedLabelTransactionState(t, transactor, ctx, repository, tenant, transactionID); state != domain.LabelTransactionSubmitted {
		t.Fatalf("回滚后交易状态 = %q, want SUBMITTED——结果不该落库", state)
	}

	// 两拍先后落地：Insert 写 1、提交写 2，记录结果那一拍写 3、作废那一拍写 4——两封各自成封、同分区。
	var recorded psapplication.LabelTransactionResult
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) (err error) {
		recorded, err = handler.RecordChannelResult(txCtx, recordCommand)
		return err
	})
	if recorded.Outcome() != psapplication.LabelTransactionApplied {
		t.Fatalf("记录结果 outcome = %q, want APPLIED", recorded.Outcome())
	}
	var voided psapplication.LabelTransactionResult
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) (err error) {
		voided, err = handler.AppendFollowUpAction(txCtx, psapplication.AppendLabelFollowUpActionCommand{
			Tenant:        tenant,
			TransactionID: transactionID,
			Kind:          domain.ChannelVoidAction,
			Parcels:       []domain.DeclaredParcelID{mustBuild(t, domain.NewDeclaredParcelID, "parcel-1")},
			Reason:        mustBuild(t, domain.NewChannelResultReasonReference, "CUSTOMER_WITHDREW"),
			OccurredAt:    clock.at.Add(3 * time.Minute),
		})
		return err
	})
	if voided.Outcome() != psapplication.LabelTransactionApplied {
		t.Fatalf("作废 outcome = %q, want APPLIED", voided.Outcome())
	}
	for _, parcel := range []string{"parcel-1", "parcel-2"} {
		for _, revision := range []string{"3", "4"} {
			eventID := labelTransactionJudgmentEventID("LT-1", parcel, revision)
			if count := countOutboxEventsIn(t, pool, eventID); count != 1 {
				t.Fatalf("%s 行数 = %d, want 1", eventID, count)
			}
		}
		if partitionKeyOf(t, pool, labelTransactionJudgmentEventID("LT-1", parcel, "3")) !=
			partitionKeyOf(t, pool, labelTransactionJudgmentEventID("LT-1", parcel, "4")) {
			t.Fatalf("%s 的两拍落在不同分区", parcel)
		}
	}
}

// TestAFailedJudgmentHandoffRollsTheSaveBack 证 ADR-0134 决定三的另一半：入队失败即整步失败，事务
// 壳回滚后结果行也不在——不留「结果已落、意图未交」的中间态。用一只失败替身代替 outbox，理由是
// 真 outbox 在健康的库上失败不了。
func TestAFailedJudgmentHandoffRollsTheSaveBack(t *testing.T) {
	repository, _, transactor := newLabelTransactions(t)
	failing := errors.New("outbox unavailable")
	clock := handoffClock{at: time.Date(2026, 9, 10, 18, 0, 0, 0, time.UTC)}
	handler := psapplication.NewLabelTransactionHandler(psapplication.LabelTransactionDeps{
		Transactions: repository,
		Judgments:    failingJudgmentHandoff{err: failing},
		Clock:        clock,
	})
	ctx := t.Context()
	tenant := mustBuild(t, domain.NewTenantID, "tenant-a")
	transactionID := mustBuild(t, domain.NewLabelTransactionID, "LT-fail")

	mustInsertLabelTransaction(t, transactor, ctx, repository,
		establishedLabelTransactionFixture(t, "tenant-a", "LT-fail", "parcel-1"))
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := handler.SubmitToChannel(txCtx, psapplication.SubmitLabelTransactionCommand{
			Tenant: tenant, TransactionID: transactionID,
		})
		return err
	})

	err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		_, err := handler.RecordChannelResult(txCtx, psapplication.RecordLabelChannelResultCommand{
			Tenant:        tenant,
			TransactionID: transactionID,
			Outcome:       domain.LabelTransactionSucceeded,
			ParcelResults: []domain.LabelTransactionParcelResultSpec{{
				Parcel: mustBuild(t, domain.NewDeclaredParcelID, "parcel-1"), Accepted: true,
				Identifier: mustBuild(t, domain.NewChannelParcelIdentifier, "channel-parcel-1"),
			}},
			ObservedAt: clock.at.Add(2 * time.Minute),
		})
		return err
	})
	if !errors.Is(err, failing) {
		t.Fatalf("入队失败应原样上抛并让事务回滚，实得：%v", err)
	}
	if state := storedLabelTransactionState(t, transactor, ctx, repository, tenant, transactionID); state != domain.LabelTransactionSubmitted {
		t.Fatalf("入队失败后交易状态 = %q, want SUBMITTED——结果行随事务回滚", state)
	}
}

// storedLabelTransactionState 在自己的事务里读回交易此刻落库的状态。闭包只做 IO，断言留在外面。
func storedLabelTransactionState(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	repository *adapter.LabelTransactions,
	tenant domain.TenantID,
	transactionID domain.LabelTransactionID,
) domain.LabelTransactionState {
	t.Helper()
	var stored domain.LabelTransaction
	var found bool
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) (err error) {
		stored, found, err = repository.FindByID(txCtx, tenant, transactionID)
		return err
	})
	if !found {
		t.Fatalf("读回交易 %s：不在库里", transactionID)
	}
	return stored.State()
}

type failingJudgmentHandoff struct{ err error }

func (handoff failingJudgmentHandoff) HandOffLabelTransactionJudgment(
	context.Context, ports.LabelTransactionJudgmentIntent,
) error {
	return handoff.err
}

// countOutboxEventsByType 数某一类型的信封有几封——用在「一封都不该有」那类断言上，按 ID 数会
// 漏掉 ID 拼错的那一封。
func countOutboxEventsByType(t *testing.T, pool *pgxpool.Pool, eventType string) int {
	t.Helper()

	var count int
	err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE event_type = $1`,
		eventType,
	).Scan(&count)
	if err != nil {
		t.Fatalf("按类型数 outbox：%v", err)
	}
	return count
}

// TestLabelTransactionJudgmentFollowsTheTransactionalTemplate 证判断意图复现样板四条：首发一行、
// 回滚无痕、重发同一份、无事务拒。信封 ID 由（租户 + 交易 + 包裹 + 版本）认领并加类型段。
func TestLabelTransactionJudgmentFollowsTheTransactionalTemplate(t *testing.T) {
	handoff, db, pool := newLabelTransactionJudgmentHandoffFixture(t)
	ctx := t.Context()
	transactor := db.Transactor()

	first := labelTransactionJudgmentIntent(t, "LT-1", "parcel-1", 2, ports.LabelTransactionResultRecorded)
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffLabelTransactionJudgment(txCtx, first)
	}); err != nil {
		t.Fatalf("首发：%v", err)
	}
	if count := countOutboxEventsIn(t, pool, labelTransactionJudgmentEventID("LT-1", "parcel-1", "2")); count != 1 {
		t.Fatalf("首发行数 = %d, want 1", count)
	}

	rollback := errors.New("回滚")
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := handoff.HandOffLabelTransactionJudgment(txCtx,
			labelTransactionJudgmentIntent(t, "LT-rollback", "parcel-1", 2, ports.LabelTransactionResultRecorded)); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if count := countOutboxEventsIn(t, pool, labelTransactionJudgmentEventID("LT-rollback", "parcel-1", "2")); count != 0 {
		t.Fatalf("回滚后行数 = %d, want 0", count)
	}

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffLabelTransactionJudgment(txCtx, first)
	}); err != nil {
		t.Fatalf("重发：%v", err)
	}
	if count := countOutboxEventsIn(t, pool, labelTransactionJudgmentEventID("LT-1", "parcel-1", "2")); count != 1 {
		t.Fatalf("重发后行数 = %d, want 1——重发的必须是同一份", count)
	}

	if err := handoff.HandOffLabelTransactionJudgment(ctx,
		labelTransactionJudgmentIntent(t, "LT-ntx", "parcel-1", 2, ports.LabelTransactionResultRecorded)); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// TestTwoBeatsOfOneTransactionEachEnqueueBehindTheSameParcel 钉住 ADR-0134 决定一的两半：定案那一拍
// 与作废那一拍**各自入队**（版本进 ID，EnqueueOnce 不把第二拍当重放吞掉）**且同分区**（分区键取
// 租户 + 包裹，同一包裹的多拍在一条队里先后消费）；不同包裹各自成区，一件的失败不拖累另一件。
func TestTwoBeatsOfOneTransactionEachEnqueueBehindTheSameParcel(t *testing.T) {
	handoff, db, pool := newLabelTransactionJudgmentHandoffFixture(t)
	ctx := t.Context()

	beats := []ports.LabelTransactionJudgmentIntent{
		labelTransactionJudgmentIntent(t, "LT-1", "parcel-1", 2, ports.LabelTransactionResultRecorded),
		labelTransactionJudgmentIntent(t, "LT-1", "parcel-1", 3, ports.LabelTransactionFollowUpAppended),
		labelTransactionJudgmentIntent(t, "LT-1", "parcel-2", 2, ports.LabelTransactionResultRecorded),
	}
	for _, beat := range beats {
		if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
			return handoff.HandOffLabelTransactionJudgment(txCtx, beat)
		}); err != nil {
			t.Fatalf("入队 %s@%d：%v", beat.Parcel, beat.Revision, err)
		}
	}

	resultBeat := labelTransactionJudgmentEventID("LT-1", "parcel-1", "2")
	followUpBeat := labelTransactionJudgmentEventID("LT-1", "parcel-1", "3")
	for _, eventID := range []string{resultBeat, followUpBeat} {
		if count := countOutboxEventsIn(t, pool, eventID); count != 1 {
			t.Fatalf("%s 行数 = %d, want 1——两拍必须各自成封", eventID, count)
		}
	}
	if partitionKeyOf(t, pool, resultBeat) != partitionKeyOf(t, pool, followUpBeat) {
		t.Fatalf("同一包裹的两拍落在不同分区：%q 与 %q——后一拍会与前一拍失去先后",
			partitionKeyOf(t, pool, resultBeat), partitionKeyOf(t, pool, followUpBeat))
	}
	if other := partitionKeyOf(t, pool, labelTransactionJudgmentEventID("LT-1", "parcel-2", "2")); other == partitionKeyOf(t, pool, resultBeat) {
		t.Fatalf("两个包裹共用分区 %q——一件的失败会拖住另一件", other)
	}
}

func TestLabelTransactionJudgmentRefusesAnIncompleteIntent(t *testing.T) {
	handoff, db, _ := newLabelTransactionJudgmentHandoffFixture(t)
	for name, intent := range map[string]ports.LabelTransactionJudgmentIntent{
		"blank":                {},
		"unpersisted revision": labelTransactionJudgmentIntent(t, "LT-1", "parcel-1", 0, ports.LabelTransactionResultRecorded),
		"no beat":              labelTransactionJudgmentIntent(t, "LT-1", "parcel-1", 2, ports.LabelTransactionBeatInvalid),
	} {
		err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
			return handoff.HandOffLabelTransactionJudgment(txCtx, intent)
		})
		if err == nil {
			t.Fatalf("%s：残缺的判断意图入了队", name)
		}
	}
}
