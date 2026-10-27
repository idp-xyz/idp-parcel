package psinbox_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证「取消被认定不成立」续办门（ADR-0295 决定四）的消费门各条与载荷译码。舞步在 platform/inboxconsume，本包只注入名字、类型与译码。

type notUpheldHandlerDouble struct {
	calls []psinbox.NotUpheldCancellation
	err   error
}

func (double *notUpheldHandlerDouble) HandleNotUpheldCancellation(_ context.Context, notUpheld psinbox.NotUpheldCancellation) error {
	if double.err != nil {
		return double.err
	}
	double.calls = append(double.calls, notUpheld)
	return nil
}

func newNotUpheldConsumerFixture(t *testing.T) (*psinbox.CancellationNotUpheldConsumer, *notUpheldHandlerDouble) {
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
	handler := &notUpheldHandlerDouble{}
	consumer, err := psinbox.NewCancellationNotUpheldConsumer(db.Transactor(), store, handler)
	if err != nil {
		t.Fatalf("构造消费者：%v", err)
	}
	return consumer, handler
}

func notUpheldEnvelope(t *testing.T, eventID string) eventing.Envelope {
	t.Helper()
	payload, err := json.Marshal(map[string]string{
		"tenantId": "tenant-a", "parcel": "parcel-1", "cancellation": "PCXL-1",
		"boundaryKind": "NETWORK_INTAKE", "boundaryRef": "NODE_INTAKE/v1",
	})
	if err != nil {
		t.Fatalf("载荷：%v", err)
	}
	now := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	return eventing.Envelope{
		SpecVersion: eventing.SpecVersion, ID: eventing.EventID(eventID), Source: "idp-parcel/parcel-shipment",
		Type: psinbox.CancellationNotUpheldEventType, Version: 1, Scope: "tenant-a", Subject: "parcel-1",
		PartitionKey: "tenant-a/parcel-1", OccurredAt: now, RecordedAt: now,
		ContentType: eventing.JSONContentType, Payload: payload,
	}
}

func TestANotUpheldCancellationDeliveryIsProcessedExactlyOnce(t *testing.T) {
	consumer, handler := newNotUpheldConsumerFixture(t)
	envelope := notUpheldEnvelope(t, "evt-1")

	if err := consumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("首投：%v", err)
	}
	if err := consumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("重复投递：%v", err)
	}
	if len(handler.calls) != 1 {
		t.Fatalf("处理次数 = %d, want 1", len(handler.calls))
	}
	want := psinbox.NotUpheldCancellation{
		TenantID: "tenant-a", Parcel: "parcel-1", Cancellation: "PCXL-1", BoundaryKind: "NETWORK_INTAKE", BoundaryRef: "NODE_INTAKE/v1",
	}
	if handler.calls[0] != want {
		t.Fatalf("译码结果 = %+v, want %+v", handler.calls[0], want)
	}
}

func TestANotUpheldEnvelopeMissingAnyDimensionIsPoison(t *testing.T) {
	for name, payload := range map[string]string{
		"缺 tenantId":     `{"parcel":"p","cancellation":"c","boundaryKind":"NETWORK_INTAKE","boundaryRef":"r"}`,
		"缺 parcel":       `{"tenantId":"t","cancellation":"c","boundaryKind":"NETWORK_INTAKE","boundaryRef":"r"}`,
		"缺 cancellation": `{"tenantId":"t","parcel":"p","boundaryKind":"NETWORK_INTAKE","boundaryRef":"r"}`,
		"缺 boundaryKind": `{"tenantId":"t","parcel":"p","cancellation":"c","boundaryRef":"r"}`,
		"缺 boundaryRef":  `{"tenantId":"t","parcel":"p","cancellation":"c","boundaryKind":"NETWORK_INTAKE"}`,
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newNotUpheldConsumerFixture(t)
			poison := notUpheldEnvelope(t, "poison-"+name)
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

func TestAFailedNotUpheldHandlerRollsBackAndTheRedeliveryRetries(t *testing.T) {
	consumer, handler := newNotUpheldConsumerFixture(t)
	envelope := notUpheldEnvelope(t, "evt-1")

	handler.err = errors.New("target not yet visible")
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

func TestAForeignTypeIsLoudOnTheNotUpheldConsumer(t *testing.T) {
	consumer, handler := newNotUpheldConsumerFixture(t)
	foreign := notUpheldEnvelope(t, "foreign")
	foreign.Type = "parcel-shipment.acceptance-decision.formed"

	if err := consumer.Consume(t.Context(), foreign); err == nil {
		t.Fatal("认不得的类型必须响亮报错")
	}
	if len(handler.calls) != 0 {
		t.Fatalf("处理次数 = %d, want 0", len(handler.calls))
	}
}
