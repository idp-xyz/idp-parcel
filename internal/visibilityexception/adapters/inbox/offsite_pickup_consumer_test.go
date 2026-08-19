package veinbox_test

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
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"

	"go.idp.xyz/idp-bento-go/eventing"
)

// 本文件对真实 PostgreSQL 证 VE 对象级揽收消费门：译码、恰一次、毒丸、与 PS 采用账本分家。
// 不认尝试级 `offsite-pickup.formed`。

type veRegisteredPickupHandlerDouble struct {
	calls []veinbox.RegisteredOffsitePickup
	err   error
}

func (double *veRegisteredPickupHandlerDouble) HandleRegisteredOffsitePickup(
	_ context.Context, registered veinbox.RegisteredOffsitePickup,
) error {
	if double.err != nil {
		return double.err
	}
	double.calls = append(double.calls, registered)
	return nil
}

func newVEOffsitePickupFixture(t *testing.T) (*veinbox.OffsitePickupConsumer, *veRegisteredPickupHandlerDouble) {
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
	handler := &veRegisteredPickupHandlerDouble{}
	consumer, err := veinbox.NewOffsitePickupConsumer(db.Transactor(), store, handler)
	if err != nil {
		t.Fatalf("构造消费者：%v", err)
	}
	return consumer, handler
}

func veOffsitePickupEnvelope(t *testing.T, eventID string) eventing.Envelope {
	t.Helper()

	payload, err := json.Marshal(map[string]string{
		"tenantId": "tenant-a",
		"object":   "parcel-1",
		"attempt":  "attempt-1",
	})
	if err != nil {
		t.Fatalf("载荷：%v", err)
	}
	now := time.Date(2026, 8, 18, 15, 30, 0, 0, time.UTC)
	return eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       "idp-parcel/transport-fulfillment",
		Type:         veinbox.OffsitePickupRegisteredEventType,
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

func TestARegisteredOffsitePickupIsProcessedExactlyOnce(t *testing.T) {
	consumer, handler := newVEOffsitePickupFixture(t)
	envelope := veOffsitePickupEnvelope(t, "tenant-a/parcel-1/attempt-1/offsite-pickup-registration")

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

func TestAnOffsitePickupEnvelopeMissingAnyKeyDimensionIsPoison(t *testing.T) {
	for name, payload := range map[string]string{
		"缺 tenantId": `{"object":"parcel-1","attempt":"attempt-1"}`,
		"缺 object":   `{"tenantId":"tenant-a","attempt":"attempt-1"}`,
		"缺 attempt":  `{"tenantId":"tenant-a","object":"parcel-1"}`,
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newVEOffsitePickupFixture(t)
			poison := veOffsitePickupEnvelope(t, "poison-"+name)
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

func TestAFailedOffsitePickupHandlerRollsBackAndTheRedeliveryRetries(t *testing.T) {
	consumer, handler := newVEOffsitePickupFixture(t)
	envelope := veOffsitePickupEnvelope(t, "tenant-a/parcel-1/attempt-1/offsite-pickup-registration")

	handler.err = errors.New("pickup registration not yet visible")
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

func TestAPoisonOffsitePickupEnvelopeIsRejectedOnceAndStaysRejected(t *testing.T) {
	consumer, handler := newVEOffsitePickupFixture(t)
	poison := veOffsitePickupEnvelope(t, "poison")
	poison.Payload = json.RawMessage(`{"tenantId":""}`)

	if err := consumer.Consume(t.Context(), poison); err != nil {
		t.Fatalf("毒丸首投应拒收入账：%v", err)
	}
	if err := consumer.Consume(t.Context(), poison); err != nil {
		t.Fatalf("毒丸重投：%v", err)
	}
	if len(handler.calls) != 0 {
		t.Fatalf("处理次数 = %d, want 0", len(handler.calls))
	}
}

func TestForeignTypesAreLoudOnTheOffsitePickupConsumer(t *testing.T) {
	for name, eventType := range map[string]eventing.EventType{
		"尝试级揽收集合": "transport-fulfillment.offsite-pickup.formed",
		"有效交付登记":  "transport-fulfillment.effective-delivery.registered",
		"节点收寄形成":  veinbox.NodeIntakeFormedEventType,
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newVEOffsitePickupFixture(t)
			foreign := veOffsitePickupEnvelope(t, "foreign-"+name)
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

func TestOffsitePickupInboxLedgersAreSeparateFromParcelShipment(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}

	veHandler := &veRegisteredPickupHandlerDouble{}
	veConsumer, err := veinbox.NewOffsitePickupConsumer(db.Transactor(), store, veHandler)
	if err != nil {
		t.Fatalf("构造 VE 消费者：%v", err)
	}
	psHandler := &psRegisteredPickupHandlerDouble{}
	psConsumer, err := psinbox.NewOffsitePickupConsumer(db.Transactor(), store, psHandler)
	if err != nil {
		t.Fatalf("构造 PS 消费者：%v", err)
	}

	envelope := veOffsitePickupEnvelope(t, "tenant-a/parcel-1/attempt-1/offsite-pickup-registration")
	if err := veConsumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("VE 首投：%v", err)
	}
	if err := psConsumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("PS 首投：%v", err)
	}
	if len(veHandler.calls) != 1 {
		t.Fatalf("VE 处理次数 = %d, want 1", len(veHandler.calls))
	}
	if len(psHandler.calls) != 1 {
		t.Fatalf("PS 处理次数 = %d, want 1；共名会把对面的投递当重复跳过", len(psHandler.calls))
	}
}

type psRegisteredPickupHandlerDouble struct {
	calls []psinbox.RegisteredOffsitePickup
}

func (double *psRegisteredPickupHandlerDouble) HandleRegisteredOffsitePickup(
	_ context.Context, registered psinbox.RegisteredOffsitePickup,
) error {
	double.calls = append(double.calls, registered)
	return nil
}
