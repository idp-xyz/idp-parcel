package psinbox_test

import (
	"encoding/json"
	"errors"
	"testing"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证计价「参数已登记」那一份登记续办门自己的几件事（ADR-0383 决定二）：认 parcel-pricing 写的那一串
// 类型、只有 SELL 重驱、BUY 入账不问队列、方向漂了是毒丸、账本记在自己名下。重驱纪律与另几份同一份代码。

const pricingParameterResumeConsumer = "parcel-shipment/advance-acceptance-chain-on-pricing-parameter-registration"

func newPricingParameterGateFixture(
	t *testing.T,
	queue *queueDouble,
	advancer *perRequestAdvancer,
) (*psinbox.OperatorRegistrationCompletedConsumer, *bentopg.DB) {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	consumer, err := psinbox.NewPricingParameterRegisteredConsumer(db.Transactor(), store, queue, advancer, 50)
	if err != nil {
		t.Fatalf("构造计价参数续办门：%v", err)
	}
	return consumer, db
}

// pricingParameterEnvelope 是发布侧 OutboxEvaluationResumptionHandoffs.HandOffPricingParameterRegistered 写出的形状：载荷四键。
func pricingParameterEnvelope(t *testing.T, eventID, direction string) eventing.Envelope {
	t.Helper()
	envelope := registrationEnvelope(t, eventID)
	envelope.Source = "idp-parcel/parcel-pricing"
	envelope.Type = psinbox.PricingParameterRegisteredEventType
	envelope.Subject = "PRICE_CARD"
	payload, err := json.Marshal(map[string]string{
		"tenantId": "tenant-a", "registrationKind": "PRICE_CARD", "pricingScope": "SYN-SCOPE-01", "direction": direction,
	})
	if err != nil {
		t.Fatal(err)
	}
	envelope.Payload = payload
	return envelope
}

// Covers: 消费方与发布方各写各的事件类型串——本门认的那一串是 parcel-pricing 写的字面，且与另几份登记续办门不共串。两串
// 在真交接口上对得上由 cmd/parcel-dispatch 的组合根用例钉住。
func TestThePricingParameterGateAcceptsTheTypeThePublisherWrites(t *testing.T) {
	const published = "parcel-pricing.pricing-parameter.registered"
	if got := string(psinbox.PricingParameterRegisteredEventType); got != published {
		t.Fatalf("消费侧认 %q，发布侧写 %q", got, published)
	}
	for _, other := range []eventing.EventType{
		psinbox.OperatorRegistrationCompletedEventType,
		psinbox.SettlementAccountRegisteredEventType,
		psinbox.PreAcceptanceEstimateAdoptionRegisteredEventType,
	} {
		if psinbox.PricingParameterRegisteredEventType == other {
			t.Fatalf("与 %q 共用一串类型——路由表分不开几扇门", other)
		}
	}
}

// Covers: ADR-0383 决定二——SELL 那一封译出租户去问同一张`等待运营登记`队列、逐份驱到；账本记在本门名下，另几份名下无痕。
func TestASellPricingParameterEnvelopeRedrivesTheTenantQueueUnderItsOwnLedgerName(t *testing.T) {
	queue := &queueDouble{pages: [][]ports.OperatorRegistrationQueueRecord{
		{waitingRecord(t, "request-1"), waitingRecord(t, "request-2")},
	}}
	advancer := &perRequestAdvancer{answers: map[string]error{}}
	consumer, _ := newPricingParameterGateFixture(t, queue, advancer)
	err := consumer.Consume(t.Context(), pricingParameterEnvelope(t, "pricing-parameter-1", "SELL"))
	if !errors.Is(err, psinbox.ErrAcceptanceChainUndecided) {
		t.Fatalf("两份都停在会自愈的依赖上，整封应交回未决哨兵等重投，实际 err = %v", err)
	}
	if len(queue.tenants) == 0 || queue.tenants[0] != "tenant-a" || len(advancer.driven) != 2 {
		t.Fatalf("队列问的是 %v、被驱的是 %v，要 tenant-a 且两份都到", queue.tenants, advancer.driven)
	}

	settled, db := newPricingParameterGateFixture(t, &queueDouble{}, &perRequestAdvancer{answers: map[string]error{}})
	if err := settled.Consume(t.Context(), pricingParameterEnvelope(t, "pricing-parameter-2", "SELL")); err != nil {
		t.Fatalf("空队列应入账：%v", err)
	}
	if n := countInboxRows(t, db, pricingParameterResumeConsumer, "pricing-parameter-2"); n != 1 {
		t.Fatalf("本门名下 inbox 行数 = %d，要 1", n)
	}
	for _, other := range []string{
		"parcel-shipment/advance-acceptance-chain-on-operator-registration",
		"parcel-shipment/advance-acceptance-chain-on-settlement-account-registration",
		"parcel-shipment/advance-acceptance-chain-on-pre-acceptance-estimate-adoption",
	} {
		if n := countInboxRows(t, db, other, "pricing-parameter-2"); n != 0 {
			t.Fatalf("%s 名下记了 %d 行——几扇门共了账本名", other, n)
		}
	}
}

// Covers: ADR-0383 决定二「只认 SELL 方向」——BUY 那一封入账、不问队列、不驱链；方向缺席或不在 BUY/SELL 里是契约漂了，
// 拒收入账、重投也不问队列：当成 BUY 放过，一次 SELL 登记就会静默地叫不醒任何委托。
func TestABuyPricingParameterEnvelopeSettlesWithoutAskingTheQueue(t *testing.T) {
	queue := &queueDouble{pages: [][]ports.OperatorRegistrationQueueRecord{{waitingRecord(t, "request-1")}}}
	advancer := &perRequestAdvancer{answers: map[string]error{}}
	consumer, db := newPricingParameterGateFixture(t, queue, advancer)

	if err := consumer.Consume(t.Context(), pricingParameterEnvelope(t, "pricing-parameter-buy", "BUY")); err != nil {
		t.Fatalf("BUY 那一封应入账：%v", err)
	}
	if n := countInboxRows(t, db, pricingParameterResumeConsumer, "pricing-parameter-buy"); n != 1 {
		t.Fatalf("BUY 那一封 inbox 行数 = %d，要 1", n)
	}
	for _, direction := range []string{"", "SIDEWAYS"} {
		id := "pricing-parameter-poison-" + direction
		poison := pricingParameterEnvelope(t, id, direction)
		if err := consumer.Consume(t.Context(), poison); err != nil {
			t.Fatalf("方向 %q 首投应拒收入账：%v", direction, err)
		}
		if err := consumer.Consume(t.Context(), poison); err != nil {
			t.Fatalf("方向 %q 重投：%v", direction, err)
		}
		if n := countInboxRows(t, db, pricingParameterResumeConsumer, id); n != 1 {
			t.Fatalf("方向 %q inbox 行数 = %d，要 1（拒收入账）", direction, n)
		}
	}
	if queue.calls != 0 || len(advancer.driven) != 0 {
		t.Fatalf("BUY 与毒丸都不该问队列也不该驱链：队列 %d 次，驱 %v", queue.calls, advancer.driven)
	}
}
