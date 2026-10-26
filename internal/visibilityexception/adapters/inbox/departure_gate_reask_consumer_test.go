package veinbox_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
)

// 本文件对真实 PostgreSQL 证 VE 出发门禁补问消费门（ADR-0305）：五维译码、恰一次、缺一维即毒丸入账，账本与移动事实那一路分家。

type veDepartureGateReaskHandlerDouble struct {
	calls []veinbox.RecordedDepartureGateReask
}

func (double *veDepartureGateReaskHandlerDouble) HandleRecordedDepartureGateReask(
	_ context.Context, recorded veinbox.RecordedDepartureGateReask,
) error {
	double.calls = append(double.calls, recorded)
	return nil
}

func newVEDepartureGateReaskFixture(t *testing.T) (*veinbox.DepartureGateReaskConsumer, *veinbox.MovementFactConsumer,
	*veDepartureGateReaskHandlerDouble, *veRecordedMovementHandlerDouble) {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	handler := &veDepartureGateReaskHandlerDouble{}
	consumer, err := veinbox.NewDepartureGateReaskConsumer(db.Transactor(), store, handler)
	if err != nil {
		t.Fatalf("构造补问消费者：%v", err)
	}
	movementHandler := &veRecordedMovementHandlerDouble{}
	movementConsumer, err := veinbox.NewMovementFactConsumer(db.Transactor(), store, movementHandler)
	if err != nil {
		t.Fatalf("构造移动事实消费者：%v", err)
	}
	return consumer, movementConsumer, handler, movementHandler
}

func veDepartureGateReaskEnvelope(t *testing.T, eventID string, payload string) eventing.Envelope {
	t.Helper()
	now := time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC)
	return eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       "idp-parcel/transport-fulfillment",
		Type:         veinbox.DepartureGateReaskRecordedEventType,
		Version:      1,
		Scope:        "tenant-a",
		Subject:      "move-1",
		PartitionKey: "tenant-a/movement-fact/move-1",
		OccurredAt:   now,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      json.RawMessage(payload),
	}
}

// Covers: AT-VE-207 消费门半边——五维译码、同一封只处理一次；缺一维或不是 JSON 是毒丸，拒收入账不报错；与移动事实那一路各记各的账。
func TestADepartureGateReaskEnvelopeIsDecodedOnceAndPoisonIsAccountedFor(t *testing.T) {
	consumer, movementConsumer, handler, movementHandler := newVEDepartureGateReaskFixture(t)
	const complete = `{"tenantId":"tenant-a","fact":"move-1","version":"v1","object":"parcel-1","sequence":"2"}`
	envelope := veDepartureGateReaskEnvelope(t, "shared-event-id", complete)
	for range 2 {
		if err := consumer.Consume(t.Context(), envelope); err != nil {
			t.Fatalf("投递：%v", err)
		}
	}
	if len(handler.calls) != 1 {
		t.Fatalf("处理次数 = %d, want 1", len(handler.calls))
	}
	if got := handler.calls[0]; got.TenantID != "tenant-a" || got.Fact != "move-1" || got.Version != "v1" ||
		got.Object != "parcel-1" || got.Sequence != "2" {
		t.Fatalf("译码 = %+v", got)
	}
	if err := movementConsumer.Consume(t.Context(), veMovementFactEnvelope(t, "shared-event-id")); err != nil {
		t.Fatalf("移动事实投递：%v", err)
	}
	if len(movementHandler.calls) != 1 {
		t.Fatalf("同一事件 ID 在移动事实那一本账上被当成了重复：%d", len(movementHandler.calls))
	}

	for name, payload := range map[string]string{
		"缺 object":   `{"tenantId":"tenant-a","fact":"move-1","version":"v1","sequence":"1"}`,
		"缺 sequence": `{"tenantId":"tenant-a","fact":"move-1","version":"v1","object":"parcel-1"}`,
		"不是 JSON":    `not-json`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := consumer.Consume(t.Context(), veDepartureGateReaskEnvelope(t, "poison-"+name, payload)); err != nil {
				t.Fatalf("毒丸应拒收入账而不是报错：%v", err)
			}
			if len(handler.calls) != 1 {
				t.Fatalf("毒丸也被处理了：%d", len(handler.calls))
			}
		})
	}
}
