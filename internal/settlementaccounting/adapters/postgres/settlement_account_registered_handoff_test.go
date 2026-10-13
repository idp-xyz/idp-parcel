package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 本文件对真实 PostgreSQL 16 证「结算账户已登记」意图（票 routing-first-cut/19 结算账户那一笔）。它守的是
// ADR-0094 决定四最容易悄悄失守的一格：账户行落了库而信封没入队，停在作用域那一格的委托就再也没有投递来
// 续办——库里那一份照样是「等登记」，与真的还没人登记长着同一张脸。
//
// 事件类型、来源与载荷键名在这里再写一遍而不导入提供方常量：它们就是跨上下文契约（消费门在 parcel-shipment
// 按它们译码），两边对不上要当场红，共用一个常量只能证明它等于自己。
const (
	settlementAccountRegisteredEventType = "settlement-accounting.settlement-account.registered"
	settlementAccountRegisteredSource    = "idp-parcel/settlement-accounting"
)

var settlementAccountRegisteredAt = time.Date(2026, 9, 30, 9, 0, 0, 0, time.UTC)

// 入队时刻取登记时刻之后一小时：信封上的 occurredAt 是登记时刻、recordedAt 是入队时刻，两者拉开才验得出
// 适配器没把它们混成一个。
var settlementAccountEnqueuedAt = settlementAccountRegisteredAt.Add(time.Hour)

func newSettlementAccountRegisteredHandoffFixture(
	t *testing.T,
) (*adapter.OutboxSettlementAccountRegisteredHandoff, *bentopg.DB, *pgxpool.Pool) {
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
	handoff, err := adapter.NewOutboxSettlementAccountRegisteredHandoff(db, store, saHandoffClock{at: settlementAccountEnqueuedAt})
	if err != nil {
		t.Fatalf("构造结算账户已登记交接：%v", err)
	}
	return handoff, db, pool
}

func settlementAccountRegisteredIntent(t *testing.T, accountID string) ports.SettlementAccountRegisteredIntent {
	t.Helper()
	return ports.SettlementAccountRegisteredIntent{
		Tenant:       settlementTenant(t),
		Account:      mustAccountRef(t, domain.NewSettlementAccountID, accountID),
		RegisteredAt: settlementAccountRegisteredAt,
	}
}

// settlementAccountRegisteredEventID 按生产同一公式重算信封 ID：用例按 ID 查库，断的是「这一封能按（租户 / 账户）
// 重算出来找到」，不断指纹字面。
func settlementAccountRegisteredEventID(tenant, account string) string {
	return string(outboxintent.FingerprintEventID("settlement-account", tenant, account))
}

func claimSettlementAccountRegisteredEnvelope(t *testing.T, db *bentopg.DB, eventID string) eventing.Envelope {
	t.Helper()
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	deliveries, err := store.Claim(t.Context(), eventing.OutboxClaim{
		Now: time.Now().UTC().Add(time.Minute), Limit: 20, LeaseFor: time.Minute, MaxAttempts: 50,
	})
	if err != nil {
		t.Fatalf("认领待发信封：%v", err)
	}
	for _, delivery := range deliveries {
		if string(delivery.Envelope.ID) == eventID {
			return delivery.Envelope
		}
	}
	t.Fatalf("Outbox 里没有 %s", eventID)
	return eventing.Envelope{}
}

// Covers: 跨上下文契约——消费门（parcel-shipment）按这些字面译码。载荷只有租户与登记种类两键，账户的法人、
// 相对方、币种与结算政策一个都不带；账户只进 ID 与 Subject。分区键装租户，与 ID（带账户维）不同表达式。
func TestASettlementAccountRegisteredIntentCarriesExactlyTheContract(t *testing.T) {
	handoff, db, _ := newSettlementAccountRegisteredHandoffFixture(t)
	intent := settlementAccountRegisteredIntent(t, "ACCT-1")

	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return handoff.HandOffSettlementAccountRegistered(txCtx, intent)
	}); err != nil {
		t.Fatalf("入队：%v", err)
	}

	envelope := claimSettlementAccountRegisteredEnvelope(t, db, settlementAccountRegisteredEventID("tenant-a", "ACCT-1"))
	if string(envelope.Type) != settlementAccountRegisteredEventType {
		t.Fatalf("事件类型 = %q", envelope.Type)
	}
	if envelope.Source != settlementAccountRegisteredSource {
		t.Fatalf("来源 = %q", envelope.Source)
	}
	if envelope.Scope != "tenant-a" || envelope.PartitionKey != "tenant-a" {
		t.Fatalf("范围 = %q，分区键 = %q，want 都是租户", envelope.Scope, envelope.PartitionKey)
	}
	if envelope.Subject != "SETTLEMENT_ACCOUNT/ACCT-1" {
		t.Fatalf("主题 = %q", envelope.Subject)
	}
	if !envelope.OccurredAt.Equal(settlementAccountRegisteredAt) {
		t.Fatalf("发生时刻 = %s，want 登记时刻 %s", envelope.OccurredAt, settlementAccountRegisteredAt)
	}
	if !envelope.RecordedAt.Equal(settlementAccountEnqueuedAt) {
		t.Fatalf("入队时刻 = %s，want %s", envelope.RecordedAt, settlementAccountEnqueuedAt)
	}

	var payload map[string]string
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		t.Fatalf("载荷不是平面 JSON 对象：%v", err)
	}
	want := map[string]string{"tenantId": "tenant-a", "registrationKind": "SETTLEMENT_ACCOUNT"}
	if len(payload) != len(want) {
		t.Fatalf("载荷键 = %v，want 只有 %v——账户内容不进信封", payload, want)
	}
	for key, value := range want {
		if payload[key] != value {
			t.Fatalf("载荷 %s = %q, want %q", key, payload[key], value)
		}
	}
}

// Covers: ADR-0094 决定四——信封与登记行同一事务成立，回滚一起消失。
func TestASettlementAccountRegisteredIntentRollsBackWithItsTransaction(t *testing.T) {
	handoff, db, pool := newSettlementAccountRegisteredHandoffFixture(t)
	rollback := errors.New("回滚")

	err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		if err := handoff.HandOffSettlementAccountRegistered(txCtx, settlementAccountRegisteredIntent(t, "ACCT-1")); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if count := countSAIntents(t, pool, settlementAccountRegisteredEventID("tenant-a", "ACCT-1")); count != 0 {
		t.Fatalf("回滚后 outbox 行数 = %d，want 0", count)
	}
}

// Covers: ADR-0043——同一户重放重发同一封，Outbox 里只有一封；另一户是另一封、落同一分区（租户）。
func TestResendingTheSameSettlementAccountIsIdempotentAndAnotherAccountQueuesBehindIt(t *testing.T) {
	handoff, db, pool := newSettlementAccountRegisteredHandoffFixture(t)
	ctx := t.Context()

	for round := 0; round < 2; round++ {
		if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
			return handoff.HandOffSettlementAccountRegistered(txCtx, settlementAccountRegisteredIntent(t, "ACCT-1"))
		}); err != nil {
			t.Fatalf("第 %d 次入队：%v", round+1, err)
		}
	}
	if count := countSAIntents(t, pool, settlementAccountRegisteredEventID("tenant-a", "ACCT-1")); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1——重发的必须是同一封", count)
	}

	if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffSettlementAccountRegistered(txCtx, settlementAccountRegisteredIntent(t, "ACCT-2"))
	}); err != nil {
		t.Fatalf("另一户入队：%v", err)
	}
	second := settlementAccountRegisteredEventID("tenant-a", "ACCT-2")
	if count := countSAIntents(t, pool, second); count != 1 {
		t.Fatalf("另一户 outbox 行数 = %d，want 1", count)
	}
	if got := partitionKeyOf(t, pool, second); got != "tenant-a" {
		t.Fatalf("另一户分区键 = %q，want 同一租户", got)
	}
}

// Covers: PBC-08 行为面——写口在无事务上下文必须被 RequireExecutor 拒绝。
func TestASettlementAccountRegisteredIntentRefusesToRunOutsideATransaction(t *testing.T) {
	handoff, _, pool := newSettlementAccountRegisteredHandoffFixture(t)

	err := handoff.HandOffSettlementAccountRegistered(t.Context(), settlementAccountRegisteredIntent(t, "ACCT-1"))
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
	if count := countSAIntents(t, pool, settlementAccountRegisteredEventID("tenant-a", "ACCT-1")); count != 0 {
		t.Fatalf("被拒绝的入队仍然落库了：%d 行", count)
	}
}

// Covers: 账户标识长到主题过不了框架上限时，拒收是确定性的、与存储不可用分两格交出去（理由同 sa-cc/32 裁决 2）。
// ID 是定长指纹，所以超长只撞主题那一格。
func TestAnOverlongSettlementAccountIsRejectedDeterministically(t *testing.T) {
	handoff, db, pool := newSettlementAccountRegisteredHandoffFixture(t)
	overlong := "ACCT-" + strings.Repeat("x", eventing.MaxSubjectLength)
	eventID := settlementAccountRegisteredEventID("tenant-a", overlong)
	if len(eventID) > eventing.MaxEventIDLength {
		t.Fatalf("指纹 ID 长 %d，超过上限——这条用例要证的是主题那一格", len(eventID))
	}

	err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return handoff.HandOffSettlementAccountRegistered(txCtx, settlementAccountRegisteredIntent(t, overlong))
	})
	if !errors.Is(err, ports.ErrSettlementAccountHandoffRejected) {
		t.Fatalf("超长账户应交回确定性拒收哨兵，实得：%v", err)
	}
	if count := countSAIntents(t, pool, eventID); count != 0 {
		t.Fatalf("被拒收的信封落了库：%d 行", count)
	}
}

// Covers: 缺件的意图响亮报错而不是入一封没有依据的信封——零值租户、零值账户、零值时刻各是一种装配缺陷。
func TestAnIncompleteSettlementAccountRegisteredIntentIsLoud(t *testing.T) {
	handoff, db, pool := newSettlementAccountRegisteredHandoffFixture(t)
	complete := settlementAccountRegisteredIntent(t, "ACCT-1")

	cases := map[string]ports.SettlementAccountRegisteredIntent{
		"零值租户": {Account: complete.Account, RegisteredAt: complete.RegisteredAt},
		"零值账户": {Tenant: complete.Tenant, RegisteredAt: complete.RegisteredAt},
		"零值时刻": {Tenant: complete.Tenant, Account: complete.Account},
	}
	for name, intent := range cases {
		err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
			return handoff.HandOffSettlementAccountRegistered(txCtx, intent)
		})
		if err == nil {
			t.Errorf("%s：必须响亮报错", name)
		}
	}
	if count := countSAIntents(t, pool, settlementAccountRegisteredEventID("tenant-a", "ACCT-1")); count != 0 {
		t.Fatalf("缺件的意图落了库：%d 行", count)
	}
}
