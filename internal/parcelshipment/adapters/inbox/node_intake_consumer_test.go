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

// 本文件对真实 PostgreSQL 16 证第三个消费方向的消费门四条，以及节点收寄信封的译码。
// 舞步与 NR 两例共用 platform/inboxconsume，本包只注入名字、类型与译码。

type formedHandlerDouble struct {
	calls []psinbox.FormedNodeIntake
	err   error
}

func (double *formedHandlerDouble) HandleFormedNodeIntake(
	_ context.Context, formed psinbox.FormedNodeIntake,
) error {
	if double.err != nil {
		return double.err
	}
	double.calls = append(double.calls, formed)
	return nil
}

func newNodeIntakeFixture(t *testing.T) (*psinbox.NodeIntakeConsumer, *formedHandlerDouble) {
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
	consumer, err := psinbox.NewNodeIntakeConsumer(db.Transactor(), store, handler)
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
	now := time.Date(2026, 8, 18, 13, 30, 0, 0, time.UTC)
	return eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       "idp-parcel/node-operations",
		Type:         psinbox.NodeIntakeFormedEventType,
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
