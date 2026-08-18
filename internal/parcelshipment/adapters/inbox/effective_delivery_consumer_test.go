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

// 本文件对真实 PostgreSQL 16 证有效交付登记这一路的消费门与译码：只认
// effective-delivery.registered，载荷三维缺一即毒丸，结果版本不进译码。

type registeredDeliveryHandlerDouble struct {
	calls []psinbox.RegisteredEffectiveDelivery
	err   error
}

func (double *registeredDeliveryHandlerDouble) HandleRegisteredEffectiveDelivery(
	_ context.Context, registered psinbox.RegisteredEffectiveDelivery,
) error {
	if double.err != nil {
		return double.err
	}
	double.calls = append(double.calls, registered)
	return nil
}

func newEffectiveDeliveryFixture(t *testing.T) (*psinbox.EffectiveDeliveryConsumer, *registeredDeliveryHandlerDouble) {
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
	handler := &registeredDeliveryHandlerDouble{}
	consumer, err := psinbox.NewEffectiveDeliveryConsumer(db.Transactor(), store, handler)
	if err != nil {
		t.Fatalf("构造消费者：%v", err)
	}
	return consumer, handler
}

func effectiveDeliveryEnvelope(t *testing.T, eventID string) eventing.Envelope {
	t.Helper()

	payload, err := json.Marshal(map[string]string{
		"tenantId": "tenant-a",
		"object":   "parcel-1",
		"attempt":  "attempt-1",
	})
	if err != nil {
		t.Fatalf("载荷：%v", err)
	}
	now := time.Date(2026, 8, 18, 16, 0, 0, 0, time.UTC)
	return eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       "idp-parcel/transport-fulfillment",
		Type:         psinbox.EffectiveDeliveryRegisteredEventType,
		Version:      1,
		Scope:        "tenant-a",
		Subject:      "parcel-1/attempt-1",
		PartitionKey: eventID,
		OccurredAt:   now,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}
}

func TestARegisteredEffectiveDeliveryIsProcessedExactlyOnce(t *testing.T) {
	consumer, handler := newEffectiveDeliveryFixture(t)
	envelope := effectiveDeliveryEnvelope(t, "tenant-a/parcel-1/attempt-1/delivery-result/v1/effective-delivery")

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
	if got.TenantID != "tenant-a" || got.Object != "parcel-1" || got.Attempt != "attempt-1" {
		t.Fatalf("译码结果 = %+v", got)
	}
}

// Covers: 提供方把结果版本放进事件 ID 以区分两代入队，载荷只带键。译码不得把 version
// 采成引用维——处理方按键读当前版本，对准同源重派生。
func TestAnEffectiveDeliveryPayloadDoesNotDecodeResultVersion(t *testing.T) {
	consumer, handler := newEffectiveDeliveryFixture(t)
	envelope := effectiveDeliveryEnvelope(t, "tenant-a/parcel-1/attempt-1/delivery-result/v2/effective-delivery")
	envelope.Payload = json.RawMessage(
		`{"tenantId":"tenant-a","object":"parcel-1","attempt":"attempt-1","version":"delivery-result/v2"}`)

	if err := consumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("带多余 version 字段的载荷仍应只按三维键处理：%v", err)
	}
	if len(handler.calls) != 1 {
		t.Fatalf("处理次数 = %d, want 1", len(handler.calls))
	}
	got := handler.calls[0]
	if got.TenantID != "tenant-a" || got.Object != "parcel-1" || got.Attempt != "attempt-1" {
		t.Fatalf("译码结果 = %+v", got)
	}
}

func TestAnEffectiveDeliveryEnvelopeMissingAnyKeyDimensionIsPoison(t *testing.T) {
	for name, payload := range map[string]string{
		"缺 tenantId": `{"object":"parcel-1","attempt":"attempt-1"}`,
		"缺 object":   `{"tenantId":"tenant-a","attempt":"attempt-1"}`,
		"缺 attempt":  `{"tenantId":"tenant-a","object":"parcel-1"}`,
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newEffectiveDeliveryFixture(t)
			poison := effectiveDeliveryEnvelope(t, "poison-"+name)
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

func TestAFailedEffectiveDeliveryHandlerRollsBackAndTheRedeliveryRetries(t *testing.T) {
	consumer, handler := newEffectiveDeliveryFixture(t)
	envelope := effectiveDeliveryEnvelope(t, "tenant-a/parcel-1/attempt-1/delivery-result/v1/effective-delivery")

	handler.err = errors.New("effective delivery is not yet visible")
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

func TestForeignTypesAreLoudOnTheEffectiveDeliveryConsumer(t *testing.T) {
	for name, eventType := range map[string]eventing.EventType{
		"控制转出交接":  "transport-fulfillment.transport-handover.registered",
		"对象级揽收登记": "transport-fulfillment.offsite-pickup.registered",
		"尝试级揽收集合": "transport-fulfillment.offsite-pickup.formed",
		"派送尝试未生效": "transport-fulfillment.delivery-attempt-result.recorded",
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newEffectiveDeliveryFixture(t)
			foreign := effectiveDeliveryEnvelope(t, "foreign-"+name)
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

// Covers: 终局消费者与揽收采用消费者的 Inbox 键只差消费者名。同一 source 与同一事件 ID
// 分别投给两扇门，两侧都必须处理一次；共名会让第二扇门把它当重复投递跳过。
func TestTheFinalConsumerKeepsASeparateInboxAccountFromPickup(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}

	pickupHandler := &registeredHandlerDouble{}
	pickupConsumer, err := psinbox.NewOffsitePickupConsumer(db.Transactor(), store, pickupHandler)
	if err != nil {
		t.Fatalf("构造揽收登记消费者：%v", err)
	}
	deliveryHandler := &registeredDeliveryHandlerDouble{}
	deliveryConsumer, err := psinbox.NewEffectiveDeliveryConsumer(db.Transactor(), store, deliveryHandler)
	if err != nil {
		t.Fatalf("构造有效交付消费者：%v", err)
	}

	const shared = "shared-event-id"
	pickupEnvelope := offsitePickupEnvelope(t, shared)
	deliveryEnvelope := effectiveDeliveryEnvelope(t, shared)
	deliveryEnvelope.Source = pickupEnvelope.Source

	if err := pickupConsumer.Consume(t.Context(), pickupEnvelope); err != nil {
		t.Fatalf("揽收登记投递：%v", err)
	}
	if err := deliveryConsumer.Consume(t.Context(), deliveryEnvelope); err != nil {
		t.Fatalf("有效交付投递：%v", err)
	}

	if len(pickupHandler.calls) != 1 || len(deliveryHandler.calls) != 1 {
		t.Fatalf("两本账应各处理一次：揽收 = %d，交付 = %d",
			len(pickupHandler.calls), len(deliveryHandler.calls))
	}
}
