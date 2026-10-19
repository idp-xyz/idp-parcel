package veinbox_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
)

// 本文件对真实 PostgreSQL 证 VE 移动事实消费门（票 demo-chain-gaps/33）：三维（含 version）译码、恰一次、毒丸、处理失败回滚
// 后重投再来、认不得的类型响亮报错，以及与交接投影账本分家。

type veRecordedMovementHandlerDouble struct {
	calls []veinbox.RecordedMovementFact
	err   error
}

func (double *veRecordedMovementHandlerDouble) HandleRecordedMovementFact(
	_ context.Context, recorded veinbox.RecordedMovementFact,
) error {
	if double.err != nil {
		return double.err
	}
	double.calls = append(double.calls, recorded)
	return nil
}

func newVEMovementFactFixture(t *testing.T) (*veinbox.MovementFactConsumer, *veRecordedMovementHandlerDouble) {
	t.Helper()

	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	handler := &veRecordedMovementHandlerDouble{}
	consumer, err := veinbox.NewMovementFactConsumer(db.Transactor(), store, handler)
	if err != nil {
		t.Fatalf("构造消费者：%v", err)
	}
	return consumer, handler
}

func veMovementFactEnvelope(t *testing.T, eventID string) eventing.Envelope {
	t.Helper()

	payload, err := json.Marshal(map[string]string{"tenantId": "tenant-a", "fact": "move-1", "version": "v1"})
	if err != nil {
		t.Fatalf("载荷：%v", err)
	}
	now := time.Date(2026, 10, 21, 14, 5, 0, 0, time.UTC)
	return eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       "idp-parcel/transport-fulfillment",
		Type:         veinbox.MovementFactRecordedEventType,
		Version:      1,
		Scope:        "tenant-a",
		Subject:      "move-1",
		PartitionKey: "tenant-a/movement-fact/move-1",
		OccurredAt:   now,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}
}

func TestARecordedMovementFactIsProcessedExactlyOnce(t *testing.T) {
	consumer, handler := newVEMovementFactFixture(t)
	envelope := veMovementFactEnvelope(t, "movement-fact-1")

	if err := consumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("首投：%v", err)
	}
	if err := consumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("重复投递：%v", err)
	}
	if len(handler.calls) != 1 {
		t.Fatalf("处理次数 = %d, want 1", len(handler.calls))
	}
	if got := handler.calls[0]; got.TenantID != "tenant-a" || got.Fact != "move-1" || got.Version != "v1" {
		t.Fatalf("译码结果 = %+v；version 必须作为第三维进引用", got)
	}
}

func TestAMovementFactEnvelopeMissingAnyKeyDimensionIsPoison(t *testing.T) {
	for name, payload := range map[string]string{
		"缺 tenantId": `{"fact":"move-1","version":"v1"}`,
		"缺 fact":     `{"tenantId":"tenant-a","version":"v1"}`,
		"缺 version":  `{"tenantId":"tenant-a","fact":"move-1"}`,
		"不是 JSON":    `not-json`,
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newVEMovementFactFixture(t)
			poison := veMovementFactEnvelope(t, "poison-"+name)
			poison.Payload = json.RawMessage(payload)

			if err := consumer.Consume(t.Context(), poison); err != nil {
				t.Fatalf("毒丸首投应拒收入账而不是报错：%v", err)
			}
			if err := consumer.Consume(t.Context(), poison); err != nil {
				t.Fatalf("毒丸重投：%v", err)
			}
			if len(handler.calls) != 0 {
				t.Fatalf("处理次数 = %d, want 0", len(handler.calls))
			}
		})
	}
}

func TestAFailedMovementFactHandlerRollsBackAndTheRedeliveryRetries(t *testing.T) {
	consumer, handler := newVEMovementFactFixture(t)
	envelope := veMovementFactEnvelope(t, "movement-fact-1")

	handler.err = errors.New("movement fact is not yet visible")
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

func TestForeignTypesAreLoudOnTheMovementFactConsumer(t *testing.T) {
	for name, eventType := range map[string]eventing.EventType{
		"权威交接登记": veinbox.TransportHandoverRegisteredEventType,
		"有效交付登记": veinbox.EffectiveDeliveryRegisteredEventType,
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newVEMovementFactFixture(t)
			foreign := veMovementFactEnvelope(t, "foreign-"+name)
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

func TestMovementFactInboxLedgerIsSeparateFromTransportHandover(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	movementHandler := &veRecordedMovementHandlerDouble{}
	movementConsumer, err := veinbox.NewMovementFactConsumer(db.Transactor(), store, movementHandler)
	if err != nil {
		t.Fatalf("构造移动事实消费者：%v", err)
	}
	handoverHandler := &veRegisteredHandoverHandlerDouble{}
	handoverConsumer, err := veinbox.NewTransportHandoverConsumer(db.Transactor(), store, handoverHandler)
	if err != nil {
		t.Fatalf("构造交接消费者：%v", err)
	}

	const shared = "shared-event-id"
	if err := movementConsumer.Consume(t.Context(), veMovementFactEnvelope(t, shared)); err != nil {
		t.Fatalf("移动事实投递：%v", err)
	}
	if err := handoverConsumer.Consume(t.Context(), veTransportHandoverEnvelope(t, shared)); err != nil {
		t.Fatalf("交接投递：%v", err)
	}
	if len(movementHandler.calls) != 1 || len(handoverHandler.calls) != 1 {
		t.Fatalf("两本账应各处理一次：移动 = %d，交接 = %d", len(movementHandler.calls), len(handoverHandler.calls))
	}
}
