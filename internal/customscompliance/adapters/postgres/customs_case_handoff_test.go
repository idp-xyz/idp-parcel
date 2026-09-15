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
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证关务案件建立意图：与业务行同一提交、回滚一并消失、重发
// 同一份、无事务拒、缺案件键响亮报错。信封 ID 由案件键认领、折成定长指纹形（票 sa-cc/34 裁决 3），
// 分区键保留案件键的可读串接。入队走 EnqueueOnce。

type customsCaseHandoffClock struct{ at time.Time }

func (clock customsCaseHandoffClock) Now() time.Time { return clock.at }

type customsCaseHandoffFixture struct {
	cases      *adapter.CustomsCases
	handoff    *adapter.OutboxCustomsCaseHandoff
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newCustomsCaseHandoffFixture(t *testing.T) *customsCaseHandoffFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	cases, err := adapter.NewCustomsCases(db)
	if err != nil {
		t.Fatalf("构造案件库：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handoff, err := adapter.NewOutboxCustomsCaseHandoff(db, store, customsCaseHandoffClock{
		at: time.Date(2026, 8, 14, 20, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return &customsCaseHandoffFixture{cases: cases, handoff: handoff, transactor: db.Transactor(), pool: pool}
}

func (fixture *customsCaseHandoffFixture) inTx(t *testing.T, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func customsCaseIntent(t *testing.T, tenant string) ports.CustomsCaseHandoffIntent {
	t.Helper()
	key := caseKey(t, tenant)
	return ports.CustomsCaseHandoffIntent{
		Key:  key,
		Case: establishedCase(t, key, "case-1", nil),
	}
}

// customsCaseHandoffEventID 按生产同一公式重算信封 ID（票 sa-cc/34 裁决 3：口名 + 案件键五维全进哈希）。
func customsCaseHandoffEventID(key ports.CustomsCaseKey) string {
	return string(outboxintent.FingerprintEventID("customs-case",
		key.TenantID.String(), key.Jurisdiction.String(), key.Direction.String(), key.Procedure.String(), key.Obligation.String()))
}

// Covers: 票 sa-cc/34 判据 (1) / (2)——案件键四个引用维取到旧串接形必然超过 eventing.MaxEventIDLength 的长度，信封仍
// 入队成功、ID 定长在上限内、同输入同 ID；分区键仍是那串可读五维（顺序语义：同案件同分区），ID 已不是那串。
func TestOverlongCaseReferencesStillProduceAnEnvelopeIDWithinTheFrameworkLimit(t *testing.T) {
	fixture := newCustomsCaseHandoffFixture(t)
	ctx := t.Context()
	long := func(prefix string) string { return prefix + "-" + strings.Repeat("x", eventing.MaxEventIDLength) }
	key := ports.CustomsCaseKey{
		TenantID:     crgValue(t, domain.NewTenantID, "tenant-a"),
		Jurisdiction: crgValue(t, domain.NewRegulatoryJurisdictionReference, long("jurisdiction")),
		Direction:    domain.ImportManifest,
		Procedure:    crgValue(t, domain.NewCustomsProcedureReference, long("procedure")),
		Obligation:   crgValue(t, domain.NewObligationScopeReference, long("obligation")),
	}
	concatenated := key.TenantID.String() + "/" + key.Jurisdiction.String() + "/" +
		key.Direction.String() + "/" + key.Procedure.String() + "/" + key.Obligation.String()
	if len(concatenated) <= eventing.MaxEventIDLength {
		t.Fatalf("夹具没造出超长：串接形 %d 字节没超过上限 %d", len(concatenated), eventing.MaxEventIDLength)
	}
	intent := ports.CustomsCaseHandoffIntent{Key: key, Case: establishedCase(t, key, "case-long", nil)}

	fixture.inTx(t, ctx, func(txCtx context.Context) error { return fixture.handoff.HandOffCase(txCtx, intent) })
	fixture.inTx(t, ctx, func(txCtx context.Context) error { return fixture.handoff.HandOffCase(txCtx, intent) })

	eventID := customsCaseHandoffEventID(key)
	if len(eventID) > eventing.MaxEventIDLength || len(eventID) != len("customs-case/")+64 {
		t.Fatalf("ID = %q（%d 字节）；该是口名前缀加六十四位十六进制、在上限 %d 内", eventID, len(eventID), eventing.MaxEventIDLength)
	}
	if count := countCustomsCaseIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("超长引用下 outbox 行数 = %d，want 1", count)
	}
}

// Covers: 票 sa-cc/34 裁决 3 / 判据 (2)——本口在 `760332c7` 上把 ID 直接当分区键；解耦后分区键取那一串可读五维一字不变
// （字面断言，不用生产函数重算），ID 换成指纹形。分区键是顺序语义，改它会让换形前后同一案件落两个分区。
func TestTheCustomsCasePartitionKeyKeepsTheReadableConcatenationWhileTheIDIsFingerprinted(t *testing.T) {
	fixture := newCustomsCaseHandoffFixture(t)
	intent := customsCaseIntent(t, "tenant-a")
	fixture.inTx(t, t.Context(), func(txCtx context.Context) error { return fixture.handoff.HandOffCase(txCtx, intent) })

	eventID := customsCaseHandoffEventID(intent.Key)
	const wantPartitionKey = "tenant-a/jurisdiction/US/IMPORT/IMPORT_STANDARD/obligation/full"
	if got := partitionKeyOf(t, fixture.pool, eventID); got != wantPartitionKey {
		t.Fatalf("分区键 = %q，want %q——可读形一字不能变", got, wantPartitionKey)
	}
	if eventID == wantPartitionKey {
		t.Fatal("ID 仍是那串可读串接——没有换成指纹形")
	}
}

func TestCustomsCaseIntentCommitsAtomicallyWithTheCase(t *testing.T) {
	fixture := newCustomsCaseHandoffFixture(t)
	ctx := t.Context()
	intent := customsCaseIntent(t, "tenant-a")
	eventID := customsCaseHandoffEventID(intent.Key)

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.cases.Save(txCtx, intent.Key, intent.Case); err != nil {
			return err
		}
		return fixture.handoff.HandOffCase(txCtx, intent)
	})

	if _, exists, err := fixture.cases.FindByKey(ctx, intent.Key); err != nil || !exists {
		t.Fatalf("业务行不在：err=%v exists=%v", err, exists)
	}
	if count := countCustomsCaseIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1", count)
	}
	if got := customsCaseIntentType(t, fixture.pool, eventID); got != "customs-compliance.customs-case.established" {
		t.Fatalf("事件类型 = %q，不是本口的类型", got)
	}
}

func TestCustomsCaseIntentRollbackDropsBoth(t *testing.T) {
	fixture := newCustomsCaseHandoffFixture(t)
	ctx := t.Context()
	intent := customsCaseIntent(t, "tenant-a")
	eventID := customsCaseHandoffEventID(intent.Key)
	rollback := errors.New("回滚")

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := fixture.cases.Save(txCtx, intent.Key, intent.Case); err != nil {
			return err
		}
		if err := fixture.handoff.HandOffCase(txCtx, intent); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	if _, exists, err := fixture.cases.FindByKey(ctx, intent.Key); err != nil || exists {
		t.Fatalf("回滚后业务行仍在：err=%v exists=%v", err, exists)
	}
	if count := countCustomsCaseIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("回滚后 outbox 行数 = %d，want 0", count)
	}

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.cases.Save(txCtx, intent.Key, intent.Case); err != nil {
			return err
		}
		return fixture.handoff.HandOffCase(txCtx, intent)
	})
	if count := countCustomsCaseIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("回滚后再投 outbox 行数 = %d，want 1", count)
	}
}

func TestResendingTheSameCustomsCaseIntentIsIdempotent(t *testing.T) {
	fixture := newCustomsCaseHandoffFixture(t)
	ctx := t.Context()
	intent := customsCaseIntent(t, "tenant-a")
	eventID := customsCaseHandoffEventID(intent.Key)

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffCase(txCtx, intent)
	})
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffCase(txCtx, intent)
	})
	if count := countCustomsCaseIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1——重发的必须是同一份", count)
	}
}

func TestCustomsCaseIntentRefusesToRunOutsideATransaction(t *testing.T) {
	fixture := newCustomsCaseHandoffFixture(t)
	intent := customsCaseIntent(t, "tenant-a")
	eventID := customsCaseHandoffEventID(intent.Key)
	if err := fixture.handoff.HandOffCase(t.Context(), intent); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
	if count := countCustomsCaseIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("被拒绝的入队仍然落库了：%d 行", count)
	}
}

func TestAForeignCustomsCaseIntentIsLoud(t *testing.T) {
	fixture := newCustomsCaseHandoffFixture(t)
	ctx := t.Context()

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffCase(txCtx, ports.CustomsCaseHandoffIntent{
			Key: ports.CustomsCaseKey{
				TenantID: crgValue(t, domain.NewTenantID, "tenant-a"),
			},
		})
	}); err == nil {
		t.Fatal("缺案件键的意图必须响亮报错")
	}
}

func countCustomsCaseIntents(t *testing.T, pool *pgxpool.Pool, eventID string) int {
	t.Helper()
	var count int
	err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox
		  WHERE event_id = $1 AND event_type = $2`,
		eventID, "customs-compliance.customs-case.established",
	).Scan(&count)
	if err != nil {
		t.Fatalf("统计 outbox 行数：%v", err)
	}
	return count
}

func customsCaseIntentType(t *testing.T, pool *pgxpool.Pool, eventID string) string {
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
