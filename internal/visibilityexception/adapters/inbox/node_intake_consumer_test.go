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

// 本文件对真实 PostgreSQL 证 VE 节点收寄消费门：译码、恰一次、毒丸、与 PS 采用账本分家。

type formedHandlerDouble struct {
	calls []veinbox.FormedNodeIntake
	err   error
}

func (double *formedHandlerDouble) HandleFormedNodeIntake(
	_ context.Context, formed veinbox.FormedNodeIntake,
) error {
	if double.err != nil {
		return double.err
	}
	double.calls = append(double.calls, formed)
	return nil
}

type psFormedHandlerDouble struct {
	calls []psinbox.FormedNodeIntake
}

func (double *psFormedHandlerDouble) HandleFormedNodeIntake(
	_ context.Context, formed psinbox.FormedNodeIntake,
) error {
	double.calls = append(double.calls, formed)
	return nil
}

func newNodeIntakeFixture(t *testing.T) (*veinbox.NodeIntakeConsumer, *formedHandlerDouble) {
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
	handler := &formedHandlerDouble{}
	consumer, err := veinbox.NewNodeIntakeConsumer(db.Transactor(), store, handler)
	if err != nil {
		t.Fatalf("构造消费者：%v", err)
	}
	return consumer, handler
}

func nodeIntakeEnvelope(t *testing.T, eventID string) eventing.Envelope {
	t.Helper()

	payload, err := json.Marshal(map[string]string{
		"tenantId": "tenant-a",
		"sourceId": "source-1",
	})
	if err != nil {
		t.Fatalf("载荷：%v", err)
	}
	now := time.Date(2026, 8, 18, 17, 0, 0, 0, time.UTC)
	return eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       "idp-parcel/node-operations",
		Type:         veinbox.NodeIntakeFormedEventType,
		Version:      1,
		Scope:        "tenant-a",
		Subject:      "source-1",
		PartitionKey: "tenant-a/source-1",
		OccurredAt:   now,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}
}

func TestANodeIntakeDeliveryIsProcessedExactlyOnce(t *testing.T) {
	consumer, handler := newNodeIntakeFixture(t)
	envelope := nodeIntakeEnvelope(t, "tenant-a/source-1")

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
	if got.TenantID != "tenant-a" || got.SourceID != "source-1" {
		t.Fatalf("译码结果 = %+v", got)
	}
}

func TestANodeIntakeEnvelopeMissingAnyKeyDimensionIsPoison(t *testing.T) {
	for name, payload := range map[string]string{
		"缺 tenantId": `{"sourceId":"source-1"}`,
		"缺 sourceId": `{"tenantId":"tenant-a"}`,
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newNodeIntakeFixture(t)
			poison := nodeIntakeEnvelope(t, "poison-"+name)
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

func TestAFailedNodeIntakeHandlerRollsBackAndTheRedeliveryRetries(t *testing.T) {
	consumer, handler := newNodeIntakeFixture(t)
	envelope := nodeIntakeEnvelope(t, "tenant-a/source-1")

	handler.err = errors.New("reception not yet visible")
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

func TestAPoisonNodeIntakeEnvelopeIsRejectedOnceAndStaysRejected(t *testing.T) {
	consumer, handler := newNodeIntakeFixture(t)
	poison := nodeIntakeEnvelope(t, "poison")
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

func TestAForeignTypeIsLoudOnTheNodeIntakeConsumer(t *testing.T) {
	consumer, handler := newNodeIntakeFixture(t)
	foreign := nodeIntakeEnvelope(t, "foreign")
	foreign.Type = "parcel-shipment.acceptance-decision.formed"

	if err := consumer.Consume(t.Context(), foreign); err == nil {
		t.Fatal("认不得的类型必须响亮报错")
	}
	if len(handler.calls) != 0 {
		t.Fatalf("处理次数 = %d, want 0", len(handler.calls))
	}
}

func TestNodeIntakeInboxLedgersAreSeparateFromParcelShipment(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}

	veHandler := &formedHandlerDouble{}
	veConsumer, err := veinbox.NewNodeIntakeConsumer(db.Transactor(), store, veHandler)
	if err != nil {
		t.Fatalf("构造 VE 消费者：%v", err)
	}
	psHandler := &psFormedHandlerDouble{}
	psConsumer, err := psinbox.NewNodeIntakeConsumer(db.Transactor(), store, psHandler)
	if err != nil {
		t.Fatalf("构造 PS 消费者：%v", err)
	}

	envelope := nodeIntakeEnvelope(t, "tenant-a/source-1")
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
