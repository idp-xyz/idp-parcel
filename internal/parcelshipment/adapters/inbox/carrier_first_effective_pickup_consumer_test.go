package psinbox_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"

	"go.idp.xyz/idp-bento-go/eventing"
)

// 本文件对真实 PostgreSQL 16 证实际承运商首次有效收寄这一路的消费门与译码（lc/25，ADR-0135 决定七）：
// 只认 carrier-first-effective-pickup.registered，载荷三维键（租户 + 事实 + 版本）缺一即毒丸，载运对象随
// 载荷带过来但不是键的一维——本体由处理方按键取回指名那一代。

type registeredCarrierPickupHandlerDouble struct {
	calls []psinbox.RegisteredCarrierFirstEffectivePickup
	err   error
}

func (double *registeredCarrierPickupHandlerDouble) HandleRegisteredCarrierFirstEffectivePickup(
	_ context.Context, registered psinbox.RegisteredCarrierFirstEffectivePickup,
) error {
	if double.err != nil {
		return double.err
	}
	double.calls = append(double.calls, registered)
	return nil
}

func newCarrierPickupFixture(t *testing.T) (*psinbox.CarrierFirstEffectivePickupConsumer, *registeredCarrierPickupHandlerDouble) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	handler := &registeredCarrierPickupHandlerDouble{}
	consumer, err := psinbox.NewCarrierFirstEffectivePickupConsumer(db.Transactor(), store, handler)
	if err != nil {
		t.Fatalf("构造消费者：%v", err)
	}
	return consumer, handler
}

func carrierPickupEnvelope(t *testing.T, eventID string) eventing.Envelope {
	t.Helper()

	payload, err := json.Marshal(map[string]string{
		"tenantId": "tenant-a",
		"fact":     "CFEP-1",
		"version":  "CFEV-1",
		"object":   "parcel-1",
	})
	if err != nil {
		t.Fatalf("载荷：%v", err)
	}
	now := time.Date(2026, 9, 11, 20, 0, 0, 0, time.UTC)
	return eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       "idp-parcel/transport-fulfillment",
		Type:         psinbox.CarrierFirstEffectivePickupRegisteredEventType,
		Version:      1,
		Scope:        "tenant-a",
		Subject:      "parcel-1/CFEP-1",
		PartitionKey: "tenant-a/parcel-1/carrier-first-effective-pickup",
		OccurredAt:   now,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}
}

// Covers: 消费者认的类型串就是 TF 侧 OutboxCarrierFirstEffectivePickupHandoff 写出的那一个（ADR-0135 决定七）。
// 提供方那个常量未导出、消费方按本包惯例自写，两串各改一边这里就红。
func TestTheCarrierPickupConsumerAcceptsTheTypeTheWriterEmits(t *testing.T) {
	if got := string(psinbox.CarrierFirstEffectivePickupRegisteredEventType); got != "transport-fulfillment.carrier-first-effective-pickup.registered" {
		t.Fatalf("消费者认 %q", got)
	}
}

func TestARegisteredCarrierPickupIsProcessedExactlyOnce(t *testing.T) {
	consumer, handler := newCarrierPickupFixture(t)
	envelope := carrierPickupEnvelope(t, "tenant-a/CFEP-1/CFEV-1/carrier-first-effective-pickup")

	if err := consumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("首投：%v", err)
	}
	if err := consumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("重复投递：%v", err)
	}
	if len(handler.calls) != 1 {
		t.Fatalf("处理次数 = %d, want 1", len(handler.calls))
	}
	got := handler.calls[0]
	if got.TenantID != "tenant-a" || got.Fact != "CFEP-1" || got.Version != "CFEV-1" || got.Object != "parcel-1" {
		t.Fatalf("译码结果 = %+v", got)
	}
}

// Covers: 一条链的替代 / 失效版本走同一个事实身份、版本进事件 ID（TF 侧 carrierFirstEffectivePickupEventID 那
// 一句），两代各自成封、两扇都要处理——处理方按信封所指那一代取回，不由消费者合并。
func TestTwoGenerationsOfOnePickupChainAreTwoDeliveries(t *testing.T) {
	consumer, handler := newCarrierPickupFixture(t)
	first := carrierPickupEnvelope(t, "tenant-a/CFEP-1/CFEV-1/carrier-first-effective-pickup")
	second := carrierPickupEnvelope(t, "tenant-a/CFEP-1/CFEV-2/carrier-first-effective-pickup")
	second.Payload = json.RawMessage(`{"tenantId":"tenant-a","fact":"CFEP-1","version":"CFEV-2","object":"parcel-1"}`)

	for _, envelope := range []eventing.Envelope{first, second} {
		if err := consumer.Consume(t.Context(), envelope); err != nil {
			t.Fatalf("投递 %s：%v", envelope.ID, err)
		}
	}
	if len(handler.calls) != 2 {
		t.Fatalf("处理次数 = %d, want 2——同一条链的两代各自成一次消费", len(handler.calls))
	}
	if handler.calls[1].Version != "CFEV-2" {
		t.Fatalf("第二封译码 = %+v", handler.calls[1])
	}
}

func TestACarrierPickupEnvelopeMissingAnyKeyDimensionIsPoison(t *testing.T) {
	for name, payload := range map[string]string{
		"缺 tenantId": `{"fact":"CFEP-1","version":"CFEV-1","object":"parcel-1"}`,
		"缺 fact":     `{"tenantId":"tenant-a","version":"CFEV-1","object":"parcel-1"}`,
		"缺 version":  `{"tenantId":"tenant-a","fact":"CFEP-1","object":"parcel-1"}`,
		"不是 JSON":    `not json`,
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newCarrierPickupFixture(t)
			poison := carrierPickupEnvelope(t, "poison-"+name)
			poison.Payload = json.RawMessage(payload)

			if err := consumer.Consume(t.Context(), poison); err != nil {
				t.Fatalf("毒丸首投应拒收入账而不是报错：%v", err)
			}
			if len(handler.calls) != 0 {
				t.Fatalf("处理次数 = %d, want 0", len(handler.calls))
			}
		})
	}
}

// Covers: 载运对象不是键的一维——缺了照样处理（处理方从取回的本体读对象），带了原样交给处理方核一致。
func TestACarrierPickupPayloadWithoutObjectStillDecodes(t *testing.T) {
	consumer, handler := newCarrierPickupFixture(t)
	envelope := carrierPickupEnvelope(t, "tenant-a/CFEP-1/CFEV-1/carrier-first-effective-pickup")
	envelope.Payload = json.RawMessage(`{"tenantId":"tenant-a","fact":"CFEP-1","version":"CFEV-1"}`)

	if err := consumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("只带三维键的载荷应照常处理：%v", err)
	}
	if len(handler.calls) != 1 || handler.calls[0].Object != "" {
		t.Fatalf("译码结果 = %+v，want 一次、对象为空", handler.calls)
	}
}

func TestAFailedCarrierPickupHandlerRollsBackAndTheRedeliveryRetries(t *testing.T) {
	consumer, handler := newCarrierPickupFixture(t)
	envelope := carrierPickupEnvelope(t, "tenant-a/CFEP-1/CFEV-1/carrier-first-effective-pickup")

	handler.err = errors.New("carrier pickup is not yet visible")
	if err := consumer.Consume(t.Context(), envelope); err == nil {
		t.Fatal("处理失败必须让消费报错——业务缺件不落毒丸账")
	}

	handler.err = nil
	if err := consumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("重投：%v", err)
	}
	if len(handler.calls) != 1 {
		t.Fatalf("处理次数 = %d, want 1", len(handler.calls))
	}
}

// Covers: 认不得的类型响亮报错——含同为 TF 出向的有效交付登记与外部承运轨迹事实：后者按 TF CONTEXT 不构成收寄，
// 这扇门吃了它就是 PS 替 TF 判「这条状态词算收寄」（票面红线）。
func TestForeignTypesAreLoudOnTheCarrierPickupConsumer(t *testing.T) {
	for name, eventType := range map[string]eventing.EventType{
		"有效交付登记":   psinbox.EffectiveDeliveryRegisteredEventType,
		"外部承运轨迹事实": "transport-fulfillment.external-carrier-tracking.judged",
		"对象级揽收登记":  psinbox.OffsitePickupRegisteredEventType,
		"面单交易判断意图": psinbox.LabelTransactionJudgmentDueEventType,
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newCarrierPickupFixture(t)
			foreign := carrierPickupEnvelope(t, "foreign-"+name)
			foreign.Type = eventType

			if err := consumer.Consume(t.Context(), foreign); err == nil {
				t.Fatal("认不得的类型必须响亮报错")
			}
			if len(handler.calls) != 0 {
				t.Fatalf("处理次数 = %d, want 0", len(handler.calls))
			}
		})
	}
}

// Covers: 本消费者与有效交付那一路的 Inbox 键只差消费者名。同一 source 与同一事件 ID 分别投给两扇门，
// 两侧都必须处理一次；共名会让第二扇门把它当重复投递跳过。
func TestTheCarrierPickupConsumerKeepsASeparateInboxAccountFromDelivery(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}

	deliveryHandler := &registeredDeliveryHandlerDouble{}
	deliveryConsumer, err := psinbox.NewEffectiveDeliveryConsumer(db.Transactor(), store, deliveryHandler)
	if err != nil {
		t.Fatalf("构造有效交付消费者：%v", err)
	}
	pickupHandler := &registeredCarrierPickupHandlerDouble{}
	pickupConsumer, err := psinbox.NewCarrierFirstEffectivePickupConsumer(db.Transactor(), store, pickupHandler)
	if err != nil {
		t.Fatalf("构造收寄消费者：%v", err)
	}

	const shared = "shared-event-id"
	deliveryEnvelope := effectiveDeliveryEnvelope(t, shared)
	pickupEnvelope := carrierPickupEnvelope(t, shared)
	pickupEnvelope.Source = deliveryEnvelope.Source

	if err := deliveryConsumer.Consume(t.Context(), deliveryEnvelope); err != nil {
		t.Fatalf("有效交付投递：%v", err)
	}
	if err := pickupConsumer.Consume(t.Context(), pickupEnvelope); err != nil {
		t.Fatalf("收寄投递：%v", err)
	}

	if len(deliveryHandler.calls) != 1 || len(pickupHandler.calls) != 1 {
		t.Fatalf("两本账应各处理一次：交付 = %d，收寄 = %d",
			len(deliveryHandler.calls), len(pickupHandler.calls))
	}
}

func TestTheCarrierPickupConsumerRefusesNilDependencies(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	if _, err := psinbox.NewCarrierFirstEffectivePickupConsumer(nil, store, &registeredCarrierPickupHandlerDouble{}); err == nil {
		t.Fatal("nil transactor 被接受了")
	}
	if _, err := psinbox.NewCarrierFirstEffectivePickupConsumer(db.Transactor(), nil, &registeredCarrierPickupHandlerDouble{}); err == nil {
		t.Fatal("nil inbox store 被接受了")
	}
	if _, err := psinbox.NewCarrierFirstEffectivePickupConsumer(db.Transactor(), store, nil); err == nil {
		t.Fatal("nil handler 被接受了")
	}
}
