package veinbox_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"

	"go.idp.xyz/idp-bento-go/eventing"
)

// 本文件对真实 PostgreSQL 证 VE 权威交接消费门：四维（含 version）译码、恰一次、毒丸、
// 与交付投影账本分家。不认交付登记与揽收信封。

type veRegisteredHandoverHandlerDouble struct {
	calls []veinbox.RegisteredTransportHandover
	err   error
}

func (double *veRegisteredHandoverHandlerDouble) HandleRegisteredTransportHandover(
	_ context.Context, registered veinbox.RegisteredTransportHandover,
) error {
	if double.err != nil {
		return double.err
	}
	double.calls = append(double.calls, registered)
	return nil
}

func newVETransportHandoverFixture(t *testing.T) (*veinbox.TransportHandoverConsumer, *veRegisteredHandoverHandlerDouble) {
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
	handler := &veRegisteredHandoverHandlerDouble{}
	consumer, err := veinbox.NewTransportHandoverConsumer(db.Transactor(), store, handler)
	if err != nil {
		t.Fatalf("构造消费者：%v", err)
	}
	return consumer, handler
}

func veTransportHandoverEnvelope(t *testing.T, eventID string) eventing.Envelope {
	t.Helper()

	payload, err := json.Marshal(map[string]string{
		"tenantId": "tenant-a",
		"object":   "parcel-1",
		"scope":    "scope-1",
		"version":  "handover-result/v1",
	})
	if err != nil {
		t.Fatalf("载荷：%v", err)
	}
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	return eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       "idp-parcel/transport-fulfillment",
		Type:         veinbox.TransportHandoverRegisteredEventType,
		Version:      1,
		Scope:        "tenant-a",
		Subject:      "parcel-1/scope-1/handover-result/v1",
		PartitionKey: "tenant-a/parcel-1",
		OccurredAt:   now,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}
}

func TestARegisteredTransportHandoverIsProcessedExactlyOnce(t *testing.T) {
	consumer, handler := newVETransportHandoverFixture(t)
	envelope := veTransportHandoverEnvelope(t, "tenant-a/parcel-1/scope-1/handover-result/v1")

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
	if got.TenantID != "tenant-a" || got.Object != "parcel-1" ||
		got.Scope != "scope-1" || got.Version != "handover-result/v1" {
		t.Fatalf("译码结果 = %+v；version 必须作为第四维进引用", got)
	}
}

func TestATransportHandoverEnvelopeMissingAnyKeyDimensionIsPoison(t *testing.T) {
	for name, payload := range map[string]string{
		"缺 tenantId": `{"object":"parcel-1","scope":"scope-1","version":"handover-result/v1"}`,
		"缺 object":   `{"tenantId":"tenant-a","scope":"scope-1","version":"handover-result/v1"}`,
		"缺 scope":    `{"tenantId":"tenant-a","object":"parcel-1","version":"handover-result/v1"}`,
		"缺 version":  `{"tenantId":"tenant-a","object":"parcel-1","scope":"scope-1"}`,
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newVETransportHandoverFixture(t)
			poison := veTransportHandoverEnvelope(t, "poison-"+name)
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

func TestAFailedTransportHandoverHandlerRollsBackAndTheRedeliveryRetries(t *testing.T) {
	consumer, handler := newVETransportHandoverFixture(t)
	envelope := veTransportHandoverEnvelope(t, "tenant-a/parcel-1/scope-1/handover-result/v1")

	handler.err = errors.New("transport handover is not yet visible")
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

func TestAPoisonTransportHandoverEnvelopeIsRejectedOnceAndStaysRejected(t *testing.T) {
	consumer, handler := newVETransportHandoverFixture(t)
	poison := veTransportHandoverEnvelope(t, "poison")
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

func TestForeignTypesAreLoudOnTheTransportHandoverConsumer(t *testing.T) {
	for name, eventType := range map[string]eventing.EventType{
		"有效交付登记":  veinbox.EffectiveDeliveryRegisteredEventType,
		"对象级揽收登记": veinbox.OffsitePickupRegisteredEventType,
		"尝试级揽收集合": "transport-fulfillment.offsite-pickup.formed",
		"节点收寄形成":  veinbox.NodeIntakeFormedEventType,
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newVETransportHandoverFixture(t)
			foreign := veTransportHandoverEnvelope(t, "foreign-"+name)
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

func TestTransportHandoverInboxLedgersAreSeparateFromEffectiveDelivery(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}

	handoverHandler := &veRegisteredHandoverHandlerDouble{}
	handoverConsumer, err := veinbox.NewTransportHandoverConsumer(db.Transactor(), store, handoverHandler)
	if err != nil {
		t.Fatalf("构造交接消费者：%v", err)
	}
	deliveryHandler := &veRegisteredDeliveryHandlerDouble{}
	deliveryConsumer, err := veinbox.NewEffectiveDeliveryConsumer(db.Transactor(), store, deliveryHandler)
	if err != nil {
		t.Fatalf("构造交付消费者：%v", err)
	}

	const shared = "shared-event-id"
	handoverEnvelope := veTransportHandoverEnvelope(t, shared)
	deliveryEnvelope := veEffectiveDeliveryEnvelope(t, shared)
	deliveryEnvelope.Source = handoverEnvelope.Source

	if err := handoverConsumer.Consume(t.Context(), handoverEnvelope); err != nil {
		t.Fatalf("交接投递：%v", err)
	}
	if err := deliveryConsumer.Consume(t.Context(), deliveryEnvelope); err != nil {
		t.Fatalf("交付投递：%v", err)
	}
	if len(handoverHandler.calls) != 1 || len(deliveryHandler.calls) != 1 {
		t.Fatalf("两本账应各处理一次：交接 = %d，交付 = %d",
			len(handoverHandler.calls), len(deliveryHandler.calls))
	}
}
