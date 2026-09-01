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
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证「复核已完成」意图。它守的是 ADR-0086 那条裁决里最容易
// 悄悄失守的一格：完成落了库而信封没入队，停等复核的委托就再也没有投递来续办，而这个
// 失败在读面上看不出来——库里那一行照样是「等复核」，与真的还没人审长着同一张脸。
// 事件类型字面在这里再写一遍而不导入提供方常量：两串对不上要当场红，共用一个常量就
// 只能证明它等于自己。

const manualReviewCompletedEventType = "parcel-shipment.shipment-request.manual-review-completed"

type reviewCompletedClock struct{ at time.Time }

func (clock reviewCompletedClock) Now() time.Time { return clock.at }

type reviewCompletedFixture struct {
	requests   *adapter.ShipmentRequests
	handoff    *adapter.OutboxManualReviewCompletedHandoff
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newReviewCompletedFixture(t *testing.T) *reviewCompletedFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	requests, err := adapter.NewShipmentRequests(db)
	if err != nil {
		t.Fatalf("构造委托仓储：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	// 入队时刻取复核完成之后一小时：信封上的 occurredAt 是复核完成时刻、recordedAt 是
	// 入队时刻，两者拉开才验得出适配器没把它们混成一个。
	handoff, err := adapter.NewOutboxManualReviewCompletedHandoff(db, store, reviewCompletedClock{
		at: submittedAtFixture.Add(4 * time.Hour),
	})
	if err != nil {
		t.Fatalf("构造复核完成交接：%v", err)
	}
	return &reviewCompletedFixture{
		requests: requests, handoff: handoff, transactor: db.Transactor(), pool: pool,
	}
}

func (fixture *reviewCompletedFixture) within(t *testing.T, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

// seedAwaitingReview 把一份委托建到库里并推进到「等待人工复核」，交回读回来的那一份。
// 必须读回而不能直接拿内存里那个：后面还要 Save，乐观并发认的是库上那一版。
func (fixture *reviewCompletedFixture) seedAwaitingReview(
	t *testing.T,
	key, requestID string,
) (domain.SourceIdentity, domain.ShipmentRequest) {
	t.Helper()
	ctx := t.Context()
	identity := requestIdentity(t, key)

	mustInsert(t, fixture.transactor, ctx, fixture.requests, submittedShipmentRequest(t, key, requestID))
	loaded, found, err := fixture.requests.FindBySourceIdentity(ctx, identity)
	if err != nil || !found {
		t.Fatalf("读回 %s：found=%v err=%v", requestID, found, err)
	}
	mustSave(t, fixture.transactor, ctx, fixture.requests, identity, awaitReview(t, loaded, "review-"+key))

	waiting, found, err := fixture.requests.FindBySourceIdentity(ctx, identity)
	if err != nil || !found {
		t.Fatalf("读回等待复核的 %s：found=%v err=%v", requestID, found, err)
	}
	return identity, waiting
}

// reviewCompletedEventID 照 ADR-0043 的认领口径拼：来源身份 + 提交版本 + 类型段。版本段
// 不能省——换代后的再复核是另一份完成，省掉它就会被 EnqueueOnce 当重放吞掉。
func reviewCompletedEventID(identity domain.SourceIdentity, request domain.ShipmentRequest) string {
	return identity.TenantID().String() + "/" +
		identity.CustomerAccountID().String() + "/" +
		identity.Source().String() + "/" +
		identity.RequestKey().String() + "/" +
		request.CurrentSubmissionVersion().VersionID().String() + "/manual-review-completed"
}

func mustCompleteReview(t *testing.T, request domain.ShipmentRequest) domain.ShipmentRequest {
	t.Helper()
	reviewed, err := request.CompleteManualReview(completedReview(t))
	if err != nil {
		t.Fatalf("录复核完成：%v", err)
	}
	return reviewed
}

func countReviewCompletedIntents(t *testing.T, pool *pgxpool.Pool, eventID string) int {
	t.Helper()
	var count int
	err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox
		  WHERE event_id = $1 AND event_type = $2`,
		eventID, manualReviewCompletedEventType,
	).Scan(&count)
	if err != nil {
		t.Fatalf("统计 outbox 行数：%v", err)
	}
	return count
}

// Covers: ADR-0086 Decision 二 —— 完成与信封同事务成立。
func TestManualReviewCompletedIntentCommitsAtomicallyWithTheCompletion(t *testing.T) {
	fixture := newReviewCompletedFixture(t)
	ctx := t.Context()
	identity, waiting := fixture.seedAwaitingReview(t, "req-key-1", "request-1")
	reviewed := mustCompleteReview(t, waiting)
	eventID := reviewCompletedEventID(identity, reviewed)

	fixture.within(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.requests.Save(txCtx, identity, reviewed); err != nil {
			return err
		}
		return fixture.handoff.HandOffManualReviewCompleted(txCtx,
			ports.ManualReviewCompletedHandoffIntent{Identity: identity, Request: reviewed})
	})

	persisted, found, err := fixture.requests.FindBySourceIdentity(ctx, identity)
	if err != nil || !found {
		t.Fatalf("读回已录完成的委托：found=%v err=%v", found, err)
	}
	if _, done := persisted.AcceptanceDecisionTask().ManualReviewCompletion(); !done {
		t.Fatal("完成没落库")
	}
	if count := countReviewCompletedIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1", count)
	}
}

// Covers: ADR-0086 Decision 二的另一半 —— 一起消失。回滚后完成不该留在库上，否则委托
// 就成了「已复核但没有任何投递会来推它」的死格。
func TestManualReviewCompletedIntentRollbackDropsBoth(t *testing.T) {
	fixture := newReviewCompletedFixture(t)
	ctx := t.Context()
	identity, waiting := fixture.seedAwaitingReview(t, "req-key-1", "request-1")
	reviewed := mustCompleteReview(t, waiting)
	eventID := reviewCompletedEventID(identity, reviewed)
	rollback := errors.New("回滚")

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := fixture.requests.Save(txCtx, identity, reviewed); err != nil {
			return err
		}
		if err := fixture.handoff.HandOffManualReviewCompleted(txCtx,
			ports.ManualReviewCompletedHandoffIntent{Identity: identity, Request: reviewed}); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	persisted, found, err := fixture.requests.FindBySourceIdentity(ctx, identity)
	if err != nil || !found {
		t.Fatalf("读回委托：found=%v err=%v", found, err)
	}
	if _, done := persisted.AcceptanceDecisionTask().ManualReviewCompletion(); done {
		t.Error("回滚后完成仍留在库上")
	}
	if count := countReviewCompletedIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("回滚后 outbox 行数 = %d，want 0", count)
	}
}

// Covers: ADR-0043 —— 重放重发同一份。同一提交版本至多一次完成，认领键因此稳定。
func TestResendingTheSameManualReviewCompletedIntentIsIdempotent(t *testing.T) {
	fixture := newReviewCompletedFixture(t)
	ctx := t.Context()
	identity, waiting := fixture.seedAwaitingReview(t, "req-key-1", "request-1")
	reviewed := mustCompleteReview(t, waiting)
	eventID := reviewCompletedEventID(identity, reviewed)
	intent := ports.ManualReviewCompletedHandoffIntent{Identity: identity, Request: reviewed}

	fixture.within(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffManualReviewCompleted(txCtx, intent)
	})
	fixture.within(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffManualReviewCompleted(txCtx, intent)
	})
	if count := countReviewCompletedIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1——重发的必须是同一份", count)
	}
}

// Covers: PBC-08 行为面 —— 写口在无事务上下文必须被 RequireExecutor 拒绝。这一条随适配器
// 逐个成立，不能靠别的口的同款用例代证。
func TestManualReviewCompletedIntentRefusesToRunOutsideATransaction(t *testing.T) {
	fixture := newReviewCompletedFixture(t)
	identity, waiting := fixture.seedAwaitingReview(t, "req-key-1", "request-1")
	reviewed := mustCompleteReview(t, waiting)
	eventID := reviewCompletedEventID(identity, reviewed)

	err := fixture.handoff.HandOffManualReviewCompleted(t.Context(),
		ports.ManualReviewCompletedHandoffIntent{Identity: identity, Request: reviewed})
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
	if count := countReviewCompletedIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("被拒绝的入队仍然落库了：%d 行", count)
	}
}

// Covers: 聚合上没有完成却走到这个口 = 边界壳接错了地方。响亮报错而不是入一封没有依据
// 的信封——后者会让续办门推进一条根本没人审过的链。
func TestAManualReviewCompletedIntentWithoutCompletionIsLoud(t *testing.T) {
	fixture := newReviewCompletedFixture(t)
	ctx := t.Context()
	identity, waiting := fixture.seedAwaitingReview(t, "req-key-1", "request-1")

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffManualReviewCompleted(txCtx,
			ports.ManualReviewCompletedHandoffIntent{Identity: identity, Request: waiting})
	}); err == nil {
		t.Fatal("聚合上没有复核完成时必须响亮报错")
	}
	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffManualReviewCompleted(txCtx, ports.ManualReviewCompletedHandoffIntent{})
	}); err == nil {
		t.Fatal("缺来源身份的意图必须响亮报错")
	}
}
