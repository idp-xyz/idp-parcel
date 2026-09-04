package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证「参数已登记」意图（ADR-0094 决定四的 party-commercial 半边，
// 票 first-tenant-runway/07 D4）。它守的是那条裁决最容易悄悄失守的一格：时点策略落了库而信封
// 没入队，停在`等待运营登记`的委托就再也没有投递来续办——库里那一行照样是「等登记」，与真的
// 还没人登记长着同一张脸。
//
// 事件类型与载荷键名在这里再写一遍而不导入提供方常量：这两串就是跨上下文契约（消费门在
// parcel-shipment 按它们译码），两边对不上要当场红，共用一个常量只能证明它等于自己。
const (
	operatorRegistrationCompletedEventType = "party-commercial.commercial-authority.operator-registration-completed"
	operatorRegistrationEventSource        = "idp-parcel/party-commercial"
)

type registrationClock struct{ at time.Time }

func (clock registrationClock) Now() time.Time { return clock.at }

type registrationHandoffFixture struct {
	handoff    *adapter.OutboxOperatorRegistrationCompletedHandoff
	store      *outbox.Store
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
	enqueuedAt time.Time
}

func newRegistrationHandoffFixture(t *testing.T) *registrationHandoffFixture {
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
	// 入队时刻取登记时刻之后一小时：信封上的 occurredAt 是登记时刻、recordedAt 是入队时刻，
	// 两者拉开才验得出适配器没把它们混成一个。
	enqueuedAt := publishedAtRow.Add(time.Hour)
	handoff, err := adapter.NewOutboxOperatorRegistrationCompletedHandoff(db, store, registrationClock{at: enqueuedAt})
	if err != nil {
		t.Fatalf("构造参数已登记交接：%v", err)
	}
	return &registrationHandoffFixture{
		handoff: handoff, store: store, transactor: db.Transactor(), pool: pool, enqueuedAt: enqueuedAt,
	}
}

func (fixture *registrationHandoffFixture) within(t *testing.T, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

// asOfRegistrationIntent 造一份「规则包 rules-1@v1 的时点策略已登记」意图，承载版本经领域重建门
// 造成`已生效`——发布编排只对取效后的版本交意图，夹具照那个形状给。
func asOfRegistrationIntent(t *testing.T) ports.OperatorRegistrationCompletedIntent {
	t.Helper()
	return ports.OperatorRegistrationCompletedIntent{
		Kind:         ports.AsOfPolicyRegistered,
		Registration: rulePackageInTenant(t, "tenant-1", "rules-1", "v1", "digest-1"),
		RegisteredAt: publishedAtRow,
	}
}

// operatorRegistrationEventID 照 ADR-0043 的认领口径拼：租户 + 种类 + 承载版本（对象/版本号）
// + 类型段。版本段不能省——同一规则包发新版本再声明一次时点策略是另一次登记，省掉它就会被
// EnqueueOnce 当重放吞掉。
func operatorRegistrationEventID(intent ports.OperatorRegistrationCompletedIntent) string {
	version := intent.Registration
	return version.Tenant().String() + "/" + intent.Kind.String() + "/" +
		version.ObjectID().String() + "/" + version.Version().String() + "/operator-registration-completed"
}

func countOperatorRegistrationIntents(t *testing.T, pool *pgxpool.Pool, eventID string) int {
	t.Helper()
	var count int
	err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox
		  WHERE event_id = $1 AND event_type = $2`,
		eventID, operatorRegistrationCompletedEventType,
	).Scan(&count)
	if err != nil {
		t.Fatalf("统计 outbox 行数：%v", err)
	}
	return count
}

// drainOperatorRegistrationEnvelopes 把 Outbox 里本口的信封按出队顺序全部认领出来。同一分区
// 一次只放行队头一封，所以要认领、定稿、再认领，直到队空——顺序本身就是分区保序的证据。
func drainOperatorRegistrationEnvelopes(t *testing.T, store *outbox.Store) []eventing.Envelope {
	t.Helper()
	var drained []eventing.Envelope
	for round := 0; round < 10; round++ {
		now := time.Now().UTC().Add(time.Duration(round+1) * time.Minute)
		deliveries, err := store.Claim(t.Context(), eventing.OutboxClaim{
			Now: now, Limit: 20, LeaseFor: time.Minute, MaxAttempts: 50,
		})
		if err != nil {
			t.Fatalf("认领待发信封：%v", err)
		}
		if len(deliveries) == 0 {
			return drained
		}
		for _, delivery := range deliveries {
			if string(delivery.Envelope.Type) == operatorRegistrationCompletedEventType {
				drained = append(drained, delivery.Envelope)
			}
			if err := store.MarkPublished(t.Context(), delivery.Ref, now); err != nil {
				t.Fatalf("定稿信封：%v", err)
			}
		}
	}
	t.Fatal("十轮认领仍未把 Outbox 排空")
	return nil
}

// claimOperatorRegistrationEnvelope 认领出本口指定 ID 的那一封，好逐字段核契约。
func claimOperatorRegistrationEnvelope(t *testing.T, store *outbox.Store, eventID string) eventing.Envelope {
	t.Helper()
	for _, envelope := range drainOperatorRegistrationEnvelopes(t, store) {
		if string(envelope.ID) == eventID {
			return envelope
		}
	}
	t.Fatalf("Outbox 里没有 %s", eventID)
	return eventing.Envelope{}
}

// Covers: 跨上下文契约 —— 消费门（parcel-shipment）按这些字面译码。载荷只带租户与登记种类两键，
// 不带任何规则正文；承载版本只进 ID 与 Subject。分区键装租户：同一租户的参数登记信封一条队，
// 与 ID（带版本维）不同表达式。
func TestAnOperatorRegistrationIntentCarriesExactlyTheContract(t *testing.T) {
	fixture := newRegistrationHandoffFixture(t)
	ctx := t.Context()
	intent := asOfRegistrationIntent(t)
	eventID := operatorRegistrationEventID(intent)

	fixture.within(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffOperatorRegistrationCompleted(txCtx, intent)
	})

	envelope := claimOperatorRegistrationEnvelope(t, fixture.store, eventID)
	if string(envelope.Type) != operatorRegistrationCompletedEventType {
		t.Fatalf("事件类型 = %q", envelope.Type)
	}
	if envelope.Source != operatorRegistrationEventSource {
		t.Fatalf("来源 = %q", envelope.Source)
	}
	if envelope.Scope != "tenant-1" || envelope.PartitionKey != "tenant-1" {
		t.Fatalf("范围 = %q，分区键 = %q，want 都是租户", envelope.Scope, envelope.PartitionKey)
	}
	if envelope.Subject != "ACCEPTANCE_RULE_PACKAGE/rules-1@v1" {
		t.Fatalf("主题 = %q", envelope.Subject)
	}
	if !envelope.OccurredAt.Equal(publishedAtRow) {
		t.Fatalf("发生时刻 = %s，want 登记时刻 %s", envelope.OccurredAt, publishedAtRow)
	}
	if !envelope.RecordedAt.Equal(fixture.enqueuedAt) {
		t.Fatalf("入队时刻 = %s，want %s", envelope.RecordedAt, fixture.enqueuedAt)
	}

	var payload map[string]string
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		t.Fatalf("载荷不是平面 JSON 对象：%v", err)
	}
	want := map[string]string{"tenantId": "tenant-1", "registrationKind": "AS_OF_POLICY"}
	if len(payload) != len(want) {
		t.Fatalf("载荷键 = %v，want 只有 %v——规则正文不进信封", payload, want)
	}
	for key, value := range want {
		if payload[key] != value {
			t.Fatalf("载荷 %s = %q, want %q", key, payload[key], value)
		}
	}
}

// Covers: ADR-0094 决定四 —— 信封与登记同一事务成立，回滚一起消失。
func TestAnOperatorRegistrationIntentRollsBackWithItsTransaction(t *testing.T) {
	fixture := newRegistrationHandoffFixture(t)
	intent := asOfRegistrationIntent(t)
	eventID := operatorRegistrationEventID(intent)
	rollback := errors.New("回滚")

	err := fixture.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		if err := fixture.handoff.HandOffOperatorRegistrationCompleted(txCtx, intent); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if count := countOperatorRegistrationIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("回滚后 outbox 行数 = %d，want 0", count)
	}
}

// Covers: ADR-0043 —— 重放重发同一份。同一版规则包的时点声明重放，认领键不变，Outbox 里只有一封。
func TestResendingTheSameOperatorRegistrationIntentIsIdempotent(t *testing.T) {
	fixture := newRegistrationHandoffFixture(t)
	ctx := t.Context()
	intent := asOfRegistrationIntent(t)
	eventID := operatorRegistrationEventID(intent)

	fixture.within(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffOperatorRegistrationCompleted(txCtx, intent)
	})
	fixture.within(t, ctx, func(txCtx context.Context) error {
		return fixture.handoff.HandOffOperatorRegistrationCompleted(txCtx, intent)
	})
	if count := countOperatorRegistrationIntents(t, fixture.pool, eventID); count != 1 {
		t.Fatalf("outbox 行数 = %d，want 1——重发的必须是同一份", count)
	}
}

// Covers: 同一规则包发新版本再声明一次是另一次登记，ID 带版本维所以不被吞；两封落同一分区
// （租户）所以保序——envelope 门禁守「ID 与分区键不同源」，这条断言守「主体取得对不对」。
func TestANewVersionOfTheSameRulePackageIsAnotherIntentInTheSamePartition(t *testing.T) {
	fixture := newRegistrationHandoffFixture(t)
	ctx := t.Context()
	first := asOfRegistrationIntent(t)
	second := ports.OperatorRegistrationCompletedIntent{
		Kind:         ports.AsOfPolicyRegistered,
		Registration: rulePackageInTenant(t, "tenant-1", "rules-1", "v2", "digest-2"),
		RegisteredAt: publishedAtRow.Add(24 * time.Hour),
	}

	fixture.within(t, ctx, func(txCtx context.Context) error {
		if err := fixture.handoff.HandOffOperatorRegistrationCompleted(txCtx, first); err != nil {
			return err
		}
		return fixture.handoff.HandOffOperatorRegistrationCompleted(txCtx, second)
	})

	drained := drainOperatorRegistrationEnvelopes(t, fixture.store)
	if len(drained) != 2 {
		t.Fatalf("排出 %d 封，want 2——第二版若被当重放吞掉就只剩一封", len(drained))
	}
	one, two := drained[0], drained[1]
	if string(one.ID) != operatorRegistrationEventID(first) || string(two.ID) != operatorRegistrationEventID(second) {
		t.Fatalf("出队顺序 = [%s, %s]，want 先 v1 后 v2——同一分区必须按登记先后出队", one.ID, two.ID)
	}
	if one.PartitionKey != two.PartitionKey {
		t.Fatalf("两版落在不同分区（%q / %q），同一租户的登记信封应排一条队", one.PartitionKey, two.PartitionKey)
	}
}

// Covers: PBC-08 行为面 —— 写口在无事务上下文必须被 RequireExecutor 拒绝。
func TestAnOperatorRegistrationIntentRefusesToRunOutsideATransaction(t *testing.T) {
	fixture := newRegistrationHandoffFixture(t)
	intent := asOfRegistrationIntent(t)

	err := fixture.handoff.HandOffOperatorRegistrationCompleted(t.Context(), intent)
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
	if count := countOperatorRegistrationIntents(t, fixture.pool, operatorRegistrationEventID(intent)); count != 0 {
		t.Fatalf("被拒绝的入队仍然落库了：%d 行", count)
	}
}

// Covers: 缺件的意图响亮报错而不是入一封没有依据的信封——集合外的种类、零值版本、零值时刻各是
// 一种装配缺陷，任何一种都不该让消费门白跑一轮。
func TestAnIncompleteOperatorRegistrationIntentIsLoud(t *testing.T) {
	fixture := newRegistrationHandoffFixture(t)
	complete := asOfRegistrationIntent(t)

	cases := map[string]ports.OperatorRegistrationCompletedIntent{
		"集合外的种类": {Kind: ports.OperatorRegistrationKindInvalid, Registration: complete.Registration, RegisteredAt: complete.RegisteredAt},
		"零值版本":   {Kind: complete.Kind, RegisteredAt: complete.RegisteredAt},
		"零值时刻":   {Kind: complete.Kind, Registration: complete.Registration},
	}
	for name, intent := range cases {
		err := fixture.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
			return fixture.handoff.HandOffOperatorRegistrationCompleted(txCtx, intent)
		})
		if err == nil {
			t.Errorf("%s：必须响亮报错", name)
		}
	}
	if count := countOperatorRegistrationIntents(t, fixture.pool, operatorRegistrationEventID(complete)); count != 0 {
		t.Fatalf("缺件的意图落了库：%d 行", count)
	}
}
