package postgres_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证外部结果接收意图：与业务行同一提交、回滚一并消失、
// 重发同一份、无事务拒、归属不上的记录响亮报错。信封 ID 折成定长指纹形、分区键保留
// 「租户 / 来源标识」可读串接（票 sa-cc/34 裁决 3）。入队走 EnqueueOnce。

type resultHandoffClock struct{ at time.Time }

func (clock resultHandoffClock) Now() time.Time { return clock.at }

type resultHandoffFixture struct {
	results    *adapter.ExternalResults
	handoff    *adapter.OutboxExternalResultHandoff
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newResultHandoffFixture(t *testing.T) *resultHandoffFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	results, err := adapter.NewExternalResults(db)
	if err != nil {
		t.Fatalf("构造登记册：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handoff, err := adapter.NewOutboxExternalResultHandoff(db, store, resultHandoffClock{
		at: time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return &resultHandoffFixture{
		results:    results,
		handoff:    handoff,
		transactor: db.Transactor(),
		pool:       pool,
	}
}

func (fixture *resultHandoffFixture) inTx(t *testing.T, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func resultIntent(t *testing.T, tenant, sourceID, version string) ports.ExternalResultHandoffIntent {
	t.Helper()
	return ports.ExternalResultHandoffIntent{Record: attributedRecord(t, tenant, sourceID, version)}
}

// resultEventID 按生产同一公式重算信封 ID（票 sa-cc/34 裁决 3：口名 + 接收幂等键两维全进哈希）。
func resultEventID(tenant, sourceID string) string {
	return string(outboxintent.FingerprintEventID("external-result", tenant, sourceID))
}

// Covers: 票 sa-cc/34 判据 (1)——SourceID 是裸字符串、没有构造门，取到旧串接形必然超过 eventing.MaxEventIDLength 的
// 长度，信封仍入队成功、ID 定长在上限内、重发同一份仍一行。分区键仍取「租户 / 来源标识」可读串接、不随 ID 换形，
// 所以这里的 SourceID 只顶过 ID 上限、不顶过 eventing.MaxPartitionKeyLength——顶过后者的响法归 /customs/external-results
// 那一路的用例。
func TestOverlongSourceIDsStillProduceAnEnvelopeIDWithinTheFrameworkLimit(t *testing.T) {
	fixture := newResultHandoffFixture(t)
	ctx := t.Context()
	sourceID := "resp-" + strings.Repeat("x", eventing.MaxEventIDLength)
	if concatenated := "tenant-a/" + sourceID; len(concatenated) <= eventing.MaxEventIDLength {
		t.Fatalf("夹具没造出超长：串接形 %d 字节没超过上限 %d", len(concatenated), eventing.MaxEventIDLength)
	}
	intent := resultIntent(t, "tenant-a", sourceID, "submission-1")

	fixture.inTx(t, ctx, func(txCtx context.Context) error { return fixture.handoff.HandOffExternalResult(txCtx, intent) })
	fixture.inTx(t, ctx, func(txCtx context.Context) error { return fixture.handoff.HandOffExternalResult(txCtx, intent) })

	eventID := resultEventID("tenant-a", sourceID)
	if len(eventID) > eventing.MaxEventIDLength || len(eventID) != len("external-result/")+64 {
		t.Fatalf("ID = %q（%d 字节）；该是口名前缀加六十四位十六进制、在上限 %d 内", eventID, len(eventID), eventing.MaxEventIDLength)
	}
	if count := countResultIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("超长来源标识下 outbox 行数 = %d，want 1", count)
	}
}

// Covers: 票 sa-cc/34 裁决 4 首句——分区键保留可读串接，所以 SourceID 长到把「租户 / 来源标识」顶过
// eventing.MaxPartitionKeyLength 时框架 Envelope.Validate 确定性拒收；本口把它与存储不可用分两格交出
// （ports.ErrHandoffEnvelopeRejected 包住原因），编排才有东西可判、才谈得上整笔不作答。夹具只顶过分区键上限、
// 不顶过 eventing.MaxSubjectLength，证的正是分区键那一维。
func TestAPartitionKeyOverTheFrameworkLimitIsRejectedAsAHandoffEnvelopeRejection(t *testing.T) {
	fixture := newResultHandoffFixture(t)
	const tenant = "tenant-a"
	sourceID := strings.Repeat("x", eventing.MaxPartitionKeyLength-len(tenant+"/")+1)
	if partitionKey := tenant + "/" + sourceID; len(partitionKey) <= eventing.MaxPartitionKeyLength || len(sourceID) > eventing.MaxSubjectLength {
		t.Fatalf("夹具没造对：分区键 %d 字节（上限 %d）、主体 %d 字节（上限 %d）",
			len(partitionKey), eventing.MaxPartitionKeyLength, len(sourceID), eventing.MaxSubjectLength)
	}
	intent := resultIntent(t, tenant, sourceID, "submission-1")

	err := fixture.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return fixture.handoff.HandOffExternalResult(txCtx, intent)
	})
	if !errors.Is(err, ports.ErrHandoffEnvelopeRejected) || !errors.Is(err, eventing.ErrInvalidEnvelope) {
		t.Fatalf("确定性拒收该分格交出、带原因：err = %v", err)
	}
	if count := countResultIntents(t, fixture.pool, resultEventID(tenant, sourceID)); count != 0 {
		t.Fatalf("被拒的信封落库了：%d 行", count)
	}
}

// Covers: 票 sa-cc/34 裁决 3 / 判据 (2)——本口在 `760332c7` 上把 ID 直接当分区键；解耦后分区键取「租户 / 来源标识」
// 一字不变（字面断言），ID 换成指纹形。
func TestTheExternalResultPartitionKeyKeepsTheReadableConcatenationWhileTheIDIsFingerprinted(t *testing.T) {
	fixture := newResultHandoffFixture(t)
	intent := resultIntent(t, "tenant-a", "resp-1", "submission-1")
	fixture.inTx(t, t.Context(), func(txCtx context.Context) error { return fixture.handoff.HandOffExternalResult(txCtx, intent) })

	eventID := resultEventID("tenant-a", "resp-1")
	const wantPartitionKey = "tenant-a/resp-1"
	if got := partitionKeyOf(t, fixture.pool, eventID); got != wantPartitionKey {
		t.Fatalf("分区键 = %q，want %q——可读形一字不能变", got, wantPartitionKey)
	}
	if eventID == wantPartitionKey {
		t.Fatal("ID 仍是那串可读串接——没有换成指纹形")
	}
}

func TestExternalResultIntentCommitsAtomicallyWithTheRecord(t *testing.T) {
	fixture := newResultHandoffFixture(t)
	ctx := t.Context()
	intent := resultIntent(t, "tenant-a", "resp-1", "submission-1")

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.results.Save(txCtx, intent.Record); err != nil {
			return err
		}
		return fixture.handoff.HandOffExternalResult(txCtx, intent)
	})

	if _, exists, err := fixture.results.FindByKey(ctx, intent.Record.Key); err != nil || !exists {
		t.Fatalf("业务行不在：err=%v exists=%v", err, exists)
	}
	eventID := resultEventID("tenant-a", "resp-1")
	if count := countResultIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1", count)
	}
	if got := resultIntentType(t, fixture.pool, eventID); got != "customs-compliance.external-result.received" {
		t.Fatalf("事件类型 = %q，不是本口的类型", got)
	}
}

func TestExternalResultIntentRollbackDropsBoth(t *testing.T) {
	fixture := newResultHandoffFixture(t)
	ctx := t.Context()
	intent := resultIntent(t, "tenant-a", "resp-1", "submission-1")
	rollback := errors.New("回滚")
	eventID := resultEventID("tenant-a", "resp-1")

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := fixture.results.Save(txCtx, intent.Record); err != nil {
			return err
		}
		if err := fixture.handoff.HandOffExternalResult(txCtx, intent); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	if _, exists, err := fixture.results.FindByKey(ctx, intent.Record.Key); err != nil || exists {
		t.Fatalf("回滚后业务行仍在：err=%v exists=%v", err, exists)
	}
	if count := countResultIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("回滚后 outbox 行数 = %d，want 0", count)
	}

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.results.Save(txCtx, intent.Record); err != nil {
			return err
		}
		return fixture.handoff.HandOffExternalResult(txCtx, intent)
	})
	if count := countResultIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("回滚后再投 outbox 行数 = %d，want 1", count)
	}
}

func TestResendingTheSameExternalResultIntentIsIdempotent(t *testing.T) {
	fixture := newResultHandoffFixture(t)
	ctx := t.Context()
	intent := resultIntent(t, "tenant-a", "resp-1", "submission-1")

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffExternalResult(txCtx, intent)
	})
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffExternalResult(txCtx, intent)
	})
	if count := countResultIntents(t, fixture.pool, resultEventID("tenant-a", "resp-1")); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1——重发的必须是同一份", count)
	}
}

func TestExternalResultIntentRefusesToRunOutsideATransaction(t *testing.T) {
	fixture := newResultHandoffFixture(t)
	if err := fixture.handoff.HandOffExternalResult(t.Context(), resultIntent(t, "tenant-a", "resp-1", "submission-1")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
	if count := countResultIntents(t, fixture.pool, resultEventID("tenant-a", "resp-1")); count != 0 {
		t.Fatalf("被拒绝的入队仍然落库了：%d 行", count)
	}
}

func TestAnUnattributableExternalResultIntentIsLoud(t *testing.T) {
	fixture := newResultHandoffFixture(t)
	ctx := t.Context()
	intent := ports.ExternalResultHandoffIntent{Record: unattributableRecord(t, "tenant-a", "resp-orphan")}

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffExternalResult(txCtx, intent)
	}); err == nil {
		t.Fatal("归属不上的记录必须响亮报错")
	}
	if count := countResultIntents(t, fixture.pool, resultEventID("tenant-a", "resp-orphan")); count != 0 {
		t.Fatalf("异类意图入队了：%d 行", count)
	}
}

func countResultIntents(t *testing.T, pool *pgxpool.Pool, eventID string) int {
	t.Helper()
	var count int
	err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox
		  WHERE event_id = $1 AND event_type = $2`,
		eventID, "customs-compliance.external-result.received",
	).Scan(&count)
	if err != nil {
		t.Fatalf("统计 outbox 行数：%v", err)
	}
	return count
}

func resultIntentType(t *testing.T, pool *pgxpool.Pool, eventID string) string {
	t.Helper()
	var eventType string
	err := pool.QueryRow(t.Context(),
		`SELECT event_type FROM `+migrate.SchemaBento+`.outbox WHERE event_id = $1`,
		eventID,
	).Scan(&eventType)
	if err != nil {
		t.Fatalf("读事件类型：%v", err)
	}
	return eventType
}
