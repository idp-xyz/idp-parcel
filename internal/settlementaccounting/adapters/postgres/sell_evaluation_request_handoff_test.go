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

// 本文件对真实 PostgreSQL 16 证 SELL 评价请求的 Outbox 交接（ADR-0190 决定一）：信封类型
// settlement-accounting.sell-evaluation-request.submitted；载荷只带（租户、请求标识）；ID 是（租户、请求）的定长
// 指纹；分区主体是「租户/包裹」。请求行与信封同一事务同生同灭，重发同一份不翻倍，无事务拒，超长确定性拒收。
//
// 事件类型与载荷键名在这里再写一遍而不导入提供方常量：它们就是跨上下文契约（计价按它们译码），两边对不上要当场
// 红，共用一个常量只能证明它等于自己。
const sellEvaluationRequestEventType = "settlement-accounting.sell-evaluation-request.submitted"

var sellRequestEnqueuedAt = sellRequestRecordedAt.Add(time.Hour)

type sellRequestFixture struct {
	registry *adapter.SellEvaluationRequests
	handoff  *adapter.OutboxSellEvaluationRequestHandoff
	db       *bentopg.DB
	pool     *pgxpool.Pool
}

func newSellRequestFixture(t *testing.T) sellRequestFixture {
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
	handoff, err := adapter.NewOutboxSellEvaluationRequestHandoff(db, store, saHandoffClock{at: sellRequestEnqueuedAt})
	if err != nil {
		t.Fatalf("构造 SELL 评价请求交接：%v", err)
	}
	registry, err := adapter.NewSellEvaluationRequests(db)
	if err != nil {
		t.Fatalf("构造 SELL 评价请求登记册：%v", err)
	}
	return sellRequestFixture{registry: registry, handoff: handoff, db: db, pool: pool}
}

// sellRequestEventID 按生产同一公式重算信封 ID：用例按 ID 查库，断的是「这一封能按（租户 / 请求）重算出来找到」，
// 不断指纹字面。
func sellRequestEventID(tenant, request string) string {
	return string(outboxintent.FingerprintEventID("sell-evaluation-request", tenant, request, "submitted"))
}

func sellRequestRecordFor(t *testing.T, id, parcel string) ports.SellEvaluationRequestRecord {
	t.Helper()
	return sellEvaluationRequestRecord(t, "tenant-a", id, parcel, domain.BillingMomentNetworkIntakeAdopted, sellRequestRecordedAt)
}

func countSellRequestRows(t *testing.T, pool *pgxpool.Pool, request string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM settlement_accounting.sell_evaluation_request WHERE request_id = $1`, request,
	).Scan(&count); err != nil {
		t.Fatalf("数请求行：%v", err)
	}
	return count
}

// requestAndHandOff 是编排在一笔里做的两步：登记请求、交信封。两步共一个事务是决定一的前提。
func (fixture sellRequestFixture) requestAndHandOff(txCtx context.Context, record ports.SellEvaluationRequestRecord) error {
	if _, err := fixture.registry.Save(txCtx, record); err != nil {
		return err
	}
	return fixture.handoff.HandOffSellEvaluationRequest(txCtx, ports.SellEvaluationRequestIntent{Record: record})
}

func claimSellRequestEnvelope(t *testing.T, db *bentopg.DB, eventID string) eventing.Envelope {
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

// Covers: 跨上下文契约——载荷只有（tenantId、sellEvaluationRequestId）两键，包裹、账户、计价范围、时点与来源事实一个
// 都不带（计价按 ID 读请求册）；分区键「租户/包裹」，与 ID 不同表达式；发生时刻是登记时刻，入队时刻取时钟。
func TestASellEvaluationRequestEnvelopeCarriesExactlyTheContract(t *testing.T) {
	fixture := newSellRequestFixture(t)
	record := sellRequestRecordFor(t, "SELLREQ-SYN-1", "SYN-PARCEL-1")

	if err := fixture.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return fixture.requestAndHandOff(txCtx, record)
	}); err != nil {
		t.Fatalf("登记并入队：%v", err)
	}

	envelope := claimSellRequestEnvelope(t, fixture.db, sellRequestEventID("tenant-a", "SELLREQ-SYN-1"))
	if string(envelope.Type) != sellEvaluationRequestEventType {
		t.Fatalf("事件类型 = %q", envelope.Type)
	}
	if envelope.Source != "idp-parcel/settlement-accounting" {
		t.Fatalf("来源 = %q", envelope.Source)
	}
	if envelope.Scope != "tenant-a" || envelope.Subject != "SELLREQ-SYN-1" {
		t.Fatalf("范围 = %q，主题 = %q", envelope.Scope, envelope.Subject)
	}
	if envelope.PartitionKey != "tenant-a/sell-evaluation-request/SYN-PARCEL-1" {
		t.Fatalf("分区键 = %q，want 租户/包裹", envelope.PartitionKey)
	}
	if !envelope.OccurredAt.Equal(sellRequestRecordedAt) || !envelope.RecordedAt.Equal(sellRequestEnqueuedAt) {
		t.Fatalf("发生时刻 = %s、入队时刻 = %s，want %s、%s",
			envelope.OccurredAt, envelope.RecordedAt, sellRequestRecordedAt, sellRequestEnqueuedAt)
	}

	var payload map[string]string
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		t.Fatalf("载荷不是平面 JSON 对象：%v", err)
	}
	want := map[string]string{"tenantId": "tenant-a", "sellEvaluationRequestId": "SELLREQ-SYN-1"}
	if len(payload) != len(want) {
		t.Fatalf("载荷键 = %v，want 只有 %v——请求内容不进信封", payload, want)
	}
	for key, value := range want {
		if payload[key] != value {
			t.Fatalf("载荷 %s = %q，want %q", key, payload[key], value)
		}
	}
}

// Covers: 请求行与信封同一事务——首发一行一封；回滚两样都不留；重发（Save 答`已存在`后照样交同一份）不翻倍；
// 无事务拒。
func TestASellEvaluationRequestAndItsEnvelopeLiveAndDieTogether(t *testing.T) {
	fixture := newSellRequestFixture(t)
	transactor := fixture.db.Transactor()
	ctx := t.Context()

	first := sellRequestRecordFor(t, "SELLREQ-SYN-1", "SYN-PARCEL-1")
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.requestAndHandOff(txCtx, first)
	}); err != nil {
		t.Fatalf("首发：%v", err)
	}
	firstID := sellRequestEventID("tenant-a", "SELLREQ-SYN-1")
	if rows, envelopes := countSellRequestRows(t, fixture.pool, "SELLREQ-SYN-1"), countSAIntents(t, fixture.pool, firstID); rows != 1 || envelopes != 1 {
		t.Fatalf("首发后请求行 = %d、信封 = %d，各要 1", rows, envelopes)
	}

	rollback := errors.New("回滚")
	second := sellRequestRecordFor(t, "SELLREQ-SYN-2", "SYN-PARCEL-2")
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := fixture.requestAndHandOff(txCtx, second); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if rows, envelopes := countSellRequestRows(t, fixture.pool, "SELLREQ-SYN-2"), countSAIntents(t, fixture.pool, sellRequestEventID("tenant-a", "SELLREQ-SYN-2")); rows != 0 || envelopes != 0 {
		t.Fatalf("回滚后请求行 = %d、信封 = %d，各要 0", rows, envelopes)
	}

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.requestAndHandOff(txCtx, first)
	}); err != nil {
		t.Fatalf("重发：%v", err)
	}
	if rows, envelopes := countSellRequestRows(t, fixture.pool, "SELLREQ-SYN-1"), countSAIntents(t, fixture.pool, firstID); rows != 1 || envelopes != 1 {
		t.Fatalf("重发后请求行 = %d、信封 = %d，各要仍是 1", rows, envelopes)
	}

	third := sellRequestRecordFor(t, "SELLREQ-SYN-3", "SYN-PARCEL-3")
	if err := fixture.handoff.HandOffSellEvaluationRequest(ctx, ports.SellEvaluationRequestIntent{Record: third}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
	if count := countSAIntents(t, fixture.pool, sellRequestEventID("tenant-a", "SELLREQ-SYN-3")); count != 0 {
		t.Fatalf("被拒绝的入队仍然落库了：%d 行", count)
	}
}

// Covers: 分区主体是包裹——同一件包裹两种时点的请求同区，不同包裹各自成区。
func TestSellEvaluationRequestsQueueByParcel(t *testing.T) {
	fixture := newSellRequestFixture(t)
	intake := sellRequestRecordFor(t, "SELLREQ-SYN-1", "SYN-PARCEL-1")
	delivery := sellEvaluationRequestRecord(t, "tenant-a", "SELLREQ-SYN-2", "SYN-PARCEL-1",
		domain.BillingMomentEffectiveDeliveryFinalized, sellRequestRecordedAt)
	elsewhere := sellRequestRecordFor(t, "SELLREQ-SYN-3", "SYN-PARCEL-2")

	if err := fixture.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		for _, record := range []ports.SellEvaluationRequestRecord{intake, delivery, elsewhere} {
			if err := fixture.requestAndHandOff(txCtx, record); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("三份入队：%v", err)
	}
	one := partitionKeyOf(t, fixture.pool, sellRequestEventID("tenant-a", "SELLREQ-SYN-1"))
	two := partitionKeyOf(t, fixture.pool, sellRequestEventID("tenant-a", "SELLREQ-SYN-2"))
	three := partitionKeyOf(t, fixture.pool, sellRequestEventID("tenant-a", "SELLREQ-SYN-3"))
	if one != two {
		t.Fatalf("同一件包裹两份请求分了区：%q / %q", one, two)
	}
	if one == three {
		t.Fatalf("两件包裹共用了分区 %q", one)
	}
}

// Covers: 包裹标识长到分区键过不了框架上限时，拒收是确定性的、与依赖不可用分两格交出去。ID 是定长指纹，所以
// 超长只撞分区键那一格。
func TestAnOverlongParcelIsRejectedDeterministically(t *testing.T) {
	fixture := newSellRequestFixture(t)
	record := sellRequestRecordFor(t, "SELLREQ-SYN-1", "SYN-PARCEL-"+strings.Repeat("x", eventing.MaxPartitionKeyLength))
	eventID := sellRequestEventID("tenant-a", "SELLREQ-SYN-1")
	if len(eventID) > eventing.MaxEventIDLength {
		t.Fatalf("指纹 ID 长 %d，超过上限——这条用例要证的是分区键那一格", len(eventID))
	}

	err := fixture.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return fixture.handoff.HandOffSellEvaluationRequest(txCtx, ports.SellEvaluationRequestIntent{Record: record})
	})
	if !errors.Is(err, ports.ErrSellEvaluationRequestHandoffRejected) {
		t.Fatalf("超长包裹应交回确定性拒收哨兵，实得：%v", err)
	}
	if count := countSAIntents(t, fixture.pool, eventID); count != 0 {
		t.Fatalf("被拒收的信封落了库：%d 行", count)
	}
}

// Covers: 缺件的意图响亮报错而不是入一封没有依据的信封——零值租户、零值请求、零值登记时刻各是一种装配缺陷。
func TestAnIncompleteSellEvaluationRequestIntentIsLoud(t *testing.T) {
	fixture := newSellRequestFixture(t)
	complete := sellRequestRecordFor(t, "SELLREQ-SYN-1", "SYN-PARCEL-1")

	noTenant := complete
	noTenant.Key.TenantID = domain.TenantID{}
	noRequest := complete
	noRequest.Key.Request = domain.SellEvaluationRequestID{}
	noMoment := complete
	noMoment.RecordedAt = time.Time{}
	for name, record := range map[string]ports.SellEvaluationRequestRecord{
		"零值租户": noTenant, "零值请求": noRequest, "零值登记时刻": noMoment,
	} {
		err := fixture.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
			return fixture.handoff.HandOffSellEvaluationRequest(txCtx, ports.SellEvaluationRequestIntent{Record: record})
		})
		if err == nil {
			t.Errorf("%s：必须响亮报错", name)
		}
	}
	if count := countSAIntents(t, fixture.pool, sellRequestEventID("tenant-a", "SELLREQ-SYN-1")); count != 0 {
		t.Fatalf("缺件的意图落了库：%d 行", count)
	}
}
