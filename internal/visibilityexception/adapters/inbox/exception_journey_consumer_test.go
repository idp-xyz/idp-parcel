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

// 本文件对真实 PostgreSQL 证 VE 替代/退运旅程消费门：四维键译码、恰一次、毒丸、与
// 交接账本分家。载荷只带旅程幂等键，成员由处理方按键重读（ADR-0066 消费侧循环拆分）。

type veRecordedJourneyHandlerDouble struct {
	calls []veinbox.RecordedExceptionJourney
	err   error
}

func (double *veRecordedJourneyHandlerDouble) HandleRecordedExceptionJourney(
	_ context.Context, recorded veinbox.RecordedExceptionJourney,
) error {
	if double.err != nil {
		return double.err
	}
	double.calls = append(double.calls, recorded)
	return nil
}

func newVEExceptionJourneyFixture(t *testing.T) (*veinbox.ExceptionJourneyConsumer, *veRecordedJourneyHandlerDouble) {
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
	handler := &veRecordedJourneyHandlerDouble{}
	consumer, err := veinbox.NewExceptionJourneyConsumer(db.Transactor(), store, handler)
	if err != nil {
		t.Fatalf("构造消费者：%v", err)
	}
	return consumer, handler
}

func veExceptionJourneyEnvelope(t *testing.T, eventID string) eventing.Envelope {
	t.Helper()

	payload, err := json.Marshal(map[string]string{
		"tenantId": "tenant-a",
		"original": "journey-original-1",
		"purpose":  "ALTERNATE",
		"basis":    "DISPOSITION/decision-1",
	})
	if err != nil {
		t.Fatalf("载荷：%v", err)
	}
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	return eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       "idp-parcel/transport-fulfillment",
		Type:         veinbox.ExceptionJourneyRecordedEventType,
		Version:      1,
		Scope:        "tenant-a",
		Subject:      "journey-original-1/ALTERNATE/DISPOSITION/decision-1",
		PartitionKey: eventID,
		OccurredAt:   now,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}
}

func TestARecordedExceptionJourneyIsProcessedExactlyOnce(t *testing.T) {
	consumer, handler := newVEExceptionJourneyFixture(t)
	envelope := veExceptionJourneyEnvelope(t, "tenant-a/journey-original-1/ALTERNATE/DISPOSITION/decision-1/exception-journey")

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
	if got.TenantID != "tenant-a" || got.Original != "journey-original-1" ||
		got.Purpose != "ALTERNATE" || got.Basis != "DISPOSITION/decision-1" {
		t.Fatalf("译码结果 = %+v；四维键须原样到达处理方", got)
	}
}

func TestAnExceptionJourneyEnvelopeMissingAnyKeyDimensionIsPoison(t *testing.T) {
	for name, payload := range map[string]string{
		"缺 tenantId": `{"original":"journey-original-1","purpose":"ALTERNATE","basis":"DISPOSITION/decision-1"}`,
		"缺 original": `{"tenantId":"tenant-a","purpose":"ALTERNATE","basis":"DISPOSITION/decision-1"}`,
		"缺 purpose":  `{"tenantId":"tenant-a","original":"journey-original-1","basis":"DISPOSITION/decision-1"}`,
		"缺 basis":    `{"tenantId":"tenant-a","original":"journey-original-1","purpose":"ALTERNATE"}`,
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newVEExceptionJourneyFixture(t)
			poison := veExceptionJourneyEnvelope(t, "poison-"+name)
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

func TestAFailedExceptionJourneyHandlerRollsBackAndTheRedeliveryRetries(t *testing.T) {
	consumer, handler := newVEExceptionJourneyFixture(t)
	envelope := veExceptionJourneyEnvelope(t, "tenant-a/journey-original-1/ALTERNATE/DISPOSITION/decision-1/exception-journey")

	handler.err = errors.New("alternate journey is not yet visible")
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

func TestForeignTypesAreLoudOnTheExceptionJourneyConsumer(t *testing.T) {
	for name, eventType := range map[string]eventing.EventType{
		"权威交接登记":   veinbox.TransportHandoverRegisteredEventType,
		"对象级揽收登记":  veinbox.OffsitePickupRegisteredEventType,
		"同聚合处置执行口": "transport-fulfillment.disposition-execution.recorded",
		"节点收寄形成":   veinbox.NodeIntakeFormedEventType,
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newVEExceptionJourneyFixture(t)
			foreign := veExceptionJourneyEnvelope(t, "foreign-"+name)
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

func TestExceptionJourneyInboxLedgersAreSeparateFromTransportHandover(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}

	journeyHandler := &veRecordedJourneyHandlerDouble{}
	journeyConsumer, err := veinbox.NewExceptionJourneyConsumer(db.Transactor(), store, journeyHandler)
	if err != nil {
		t.Fatalf("构造旅程消费者：%v", err)
	}
	handoverHandler := &veRegisteredHandoverHandlerDouble{}
	handoverConsumer, err := veinbox.NewTransportHandoverConsumer(db.Transactor(), store, handoverHandler)
	if err != nil {
		t.Fatalf("构造交接消费者：%v", err)
	}

	const shared = "shared-event-id"
	journeyEnvelope := veExceptionJourneyEnvelope(t, shared)
	handoverEnvelope := veTransportHandoverEnvelope(t, shared)

	if err := journeyConsumer.Consume(t.Context(), journeyEnvelope); err != nil {
		t.Fatalf("旅程投递：%v", err)
	}
	if err := handoverConsumer.Consume(t.Context(), handoverEnvelope); err != nil {
		t.Fatalf("交接投递：%v", err)
	}
	if len(journeyHandler.calls) != 1 || len(handoverHandler.calls) != 1 {
		t.Fatalf("两本账应各处理一次：旅程 = %d，交接 = %d",
			len(journeyHandler.calls), len(handoverHandler.calls))
	}
}
