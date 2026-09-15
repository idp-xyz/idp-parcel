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

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证后续动作目标意图：与业务行同一提交、回滚一并消失、重发
// 同一份、无事务拒、缺目标键响亮报错。信封 ID 由后续动作目标键认领。入队走 EnqueueOnce。

type followUpHandoffClock struct{ at time.Time }

func (clock followUpHandoffClock) Now() time.Time { return clock.at }

type followUpHandoffFixture struct {
	followUps  *adapter.FollowUps
	handoff    *adapter.OutboxFollowUpHandoff
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newFollowUpHandoffFixture(t *testing.T) *followUpHandoffFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	followUps, err := adapter.NewFollowUps(db)
	if err != nil {
		t.Fatalf("构造后续动作库：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handoff, err := adapter.NewOutboxFollowUpHandoff(db, store, followUpHandoffClock{
		at: time.Date(2026, 8, 14, 17, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return &followUpHandoffFixture{
		followUps:  followUps,
		handoff:    handoff,
		transactor: db.Transactor(),
		pool:       pool,
	}
}

func (fixture *followUpHandoffFixture) inTx(t *testing.T, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func followUpIntent(t *testing.T, tenant string) ports.FollowUpHandoffIntent {
	t.Helper()
	return ports.FollowUpHandoffIntent{
		Key:    followUpKey(t, tenant),
		Target: formedTarget(t),
	}
}

// followUpHandoffEventID 按生产同一公式重算某一拍的信封 ID（票 sa-cc/34 裁决 3：口名 + 目标键四维 + 状态段全进哈希）。
// 状态段与各拍事件类型的尾词同词：recorded / replacement-proposed / replacement-effective。
func followUpHandoffEventID(key ports.FollowUpTargetKey, beat string) string {
	return string(outboxintent.FingerprintEventID("follow-up",
		key.TenantID.String(), key.Trigger.String(), key.Version.String(), key.Kind.String(), beat))
}

// followUpHandoffPartitionKey 是目标键四维的可读串接——分区键不随 ID 换形。
func followUpHandoffPartitionKey(key ports.FollowUpTargetKey) string {
	return key.TenantID.String() + "/" + key.Trigger.String() + "/" +
		key.Version.String() + "/" + key.Kind.String()
}

// TestEachFollowUpBeatEnqueuesItsOwnEnvelopeInTheSamePartition 钉住两个字段的分工。
//
// 同一目标键上有三拍都交意图（ManageFollowUpHandler.FormTarget → .Propose → .RecordEffect），
// 两件都要成立：**各自入队**（ID 带状态段，后两拍不被 EnqueueOnce 当成重放吞掉——否则
// 拟替代与生效替代永远到不了下游）**且同分区**（分区键只到目标键，三拍排一条队）。
// 状态段照 statement_handoff 的 /voided 现成形状：首拍裸键，后两拍各带后缀、各换类型。
func TestEachFollowUpBeatEnqueuesItsOwnEnvelopeInTheSamePartition(t *testing.T) {
	fixture := newFollowUpHandoffFixture(t)
	ctx := t.Context()

	key := followUpKey(t, "tenant-a")
	target := formedTarget(t)
	formed := ports.FollowUpHandoffIntent{Key: key, Target: target}

	proposed, err := domain.ProposeReplacement(target,
		fmcValue(t, domain.NewDeclarationUnitID, "declaration-unit-2"))
	if err != nil {
		t.Fatalf("拟替代：%v", err)
	}
	proposal := ports.FollowUpHandoffIntent{Key: key, Target: target, Relation: &proposed}

	effective, err := proposed.TakeEffect("external-result/approved", fmcBaseAt.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("替代生效：%v", err)
	}
	took := ports.FollowUpHandoffIntent{Key: key, Target: target, Relation: &effective}

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if err := fixture.handoff.HandOffFollowUp(txCtx, formed); err != nil {
			return err
		}
		if err := fixture.handoff.HandOffFollowUp(txCtx, proposal); err != nil {
			return err
		}
		return fixture.handoff.HandOffFollowUp(txCtx, took)
	})

	recordedID := followUpHandoffEventID(key, "recorded")
	proposedID := followUpHandoffEventID(key, "replacement-proposed")
	effectiveID := followUpHandoffEventID(key, "replacement-effective")
	if recordedID == proposedID || proposedID == effectiveID || recordedID == effectiveID {
		t.Fatal("三拍算出了相同的 ID——状态段没进哈希，后两拍会被 EnqueueOnce 吞掉（票 sa-cc/34 判据 (1)）")
	}
	partitionKey := followUpHandoffPartitionKey(key)

	for _, row := range []struct {
		id, eventType string
	}{
		{recordedID, "customs-compliance.follow-up.recorded"},
		{proposedID, "customs-compliance.follow-up.replacement-proposed"},
		{effectiveID, "customs-compliance.follow-up.replacement-effective"},
	} {
		if got := followUpIntentType(t, fixture.pool, row.id); got != row.eventType {
			t.Fatalf("%s 的事件类型 = %q, want %q——后两拍不入队或类型没换都算失败", row.id, got, row.eventType)
		}
		if got := partitionKeyOf(t, fixture.pool, row.id); got != partitionKey {
			t.Fatalf("%s 的分区键 = %q, want %q；三拍不同分区就没有先后可言", row.id, got, partitionKey)
		}
	}
}

func TestFollowUpIntentCommitsAtomicallyWithTheTarget(t *testing.T) {
	fixture := newFollowUpHandoffFixture(t)
	ctx := t.Context()
	intent := followUpIntent(t, "tenant-a")
	eventID := followUpHandoffEventID(intent.Key, "recorded")

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.followUps.SaveTarget(txCtx, intent.Key, intent.Target); err != nil {
			return err
		}
		return fixture.handoff.HandOffFollowUp(txCtx, intent)
	})

	if _, exists, err := fixture.followUps.FindTarget(ctx, intent.Key); err != nil || !exists {
		t.Fatalf("业务行不在：err=%v exists=%v", err, exists)
	}
	if count := countFollowUpIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1", count)
	}
	if got := followUpIntentType(t, fixture.pool, eventID); got != "customs-compliance.follow-up.recorded" {
		t.Fatalf("事件类型 = %q，不是本口的类型", got)
	}
}

func TestFollowUpIntentRollbackDropsBoth(t *testing.T) {
	fixture := newFollowUpHandoffFixture(t)
	ctx := t.Context()
	intent := followUpIntent(t, "tenant-a")
	eventID := followUpHandoffEventID(intent.Key, "recorded")
	rollback := errors.New("回滚")

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := fixture.followUps.SaveTarget(txCtx, intent.Key, intent.Target); err != nil {
			return err
		}
		if err := fixture.handoff.HandOffFollowUp(txCtx, intent); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	if _, exists, err := fixture.followUps.FindTarget(ctx, intent.Key); err != nil || exists {
		t.Fatalf("回滚后业务行仍在：err=%v exists=%v", err, exists)
	}
	if count := countFollowUpIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("回滚后 outbox 行数 = %d，want 0", count)
	}

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.followUps.SaveTarget(txCtx, intent.Key, intent.Target); err != nil {
			return err
		}
		return fixture.handoff.HandOffFollowUp(txCtx, intent)
	})
	if count := countFollowUpIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("回滚后再投 outbox 行数 = %d，want 1", count)
	}
}

func TestResendingTheSameFollowUpIntentIsIdempotent(t *testing.T) {
	fixture := newFollowUpHandoffFixture(t)
	ctx := t.Context()
	intent := followUpIntent(t, "tenant-a")
	eventID := followUpHandoffEventID(intent.Key, "recorded")

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffFollowUp(txCtx, intent)
	})
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffFollowUp(txCtx, intent)
	})
	if count := countFollowUpIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1——重发的必须是同一份", count)
	}
}

func TestFollowUpIntentRefusesToRunOutsideATransaction(t *testing.T) {
	fixture := newFollowUpHandoffFixture(t)
	intent := followUpIntent(t, "tenant-a")
	eventID := followUpHandoffEventID(intent.Key, "recorded")
	if err := fixture.handoff.HandOffFollowUp(t.Context(), intent); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
	if count := countFollowUpIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("被拒绝的入队仍然落库了：%d 行", count)
	}
}

func TestAForeignFollowUpIntentIsLoud(t *testing.T) {
	fixture := newFollowUpHandoffFixture(t)
	ctx := t.Context()

	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffFollowUp(txCtx, ports.FollowUpHandoffIntent{
			Key: ports.FollowUpTargetKey{
				TenantID: fmcValue(t, domain.NewTenantID, "tenant-a"),
			},
		})
	}); err == nil {
		t.Fatal("缺目标键的意图必须响亮报错")
	}
}

func countFollowUpIntents(t *testing.T, pool *pgxpool.Pool, eventID string) int {
	t.Helper()
	var count int
	err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox
		  WHERE event_id = $1 AND event_type = $2`,
		eventID, "customs-compliance.follow-up.recorded",
	).Scan(&count)
	if err != nil {
		t.Fatalf("统计 outbox 行数：%v", err)
	}
	return count
}

func followUpIntentType(t *testing.T, pool *pgxpool.Pool, eventID string) string {
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
