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

// 本文件对真实 PostgreSQL 证 VE 终局判断消费门：四维译码、恰一次、毒丸、与同包其余
// 投影账本分家。终局是 parcel-shipment 自家事实，来源位与 TF/NO 三路都不同。

type veFormedFinalOutcomeHandlerDouble struct {
	calls []veinbox.FormedFinalOutcome
	err   error
}

func (double *veFormedFinalOutcomeHandlerDouble) HandleFormedFinalOutcome(
	_ context.Context, formed veinbox.FormedFinalOutcome,
) error {
	if double.err != nil {
		return double.err
	}
	double.calls = append(double.calls, formed)
	return nil
}

func newVEFinalOutcomeFixture(t *testing.T) (*veinbox.FinalOutcomeConsumer, *veFormedFinalOutcomeHandlerDouble) {
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
	handler := &veFormedFinalOutcomeHandlerDouble{}
	consumer, err := veinbox.NewFinalOutcomeConsumer(db.Transactor(), store, handler)
	if err != nil {
		t.Fatalf("构造消费者：%v", err)
	}
	return consumer, handler
}

func veFinalOutcomeEnvelope(t *testing.T, eventID string) eventing.Envelope {
	t.Helper()

	payload, err := json.Marshal(map[string]string{
		"tenantId": "tenant-a",
		"parcel":   "parcel-1",
		"kind":     "TF-DELIVERY",
		"version":  "responsibility-outcome/v1",
	})
	if err != nil {
		t.Fatalf("载荷：%v", err)
	}
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	return eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       "idp-parcel/parcel-shipment",
		Type:         veinbox.FinalOutcomeFormedEventType,
		Version:      1,
		Scope:        "tenant-a",
		Subject:      "parcel-1",
		PartitionKey: "tenant-a/parcel-1",
		OccurredAt:   now,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}
}

func TestAFormedFinalOutcomeIsProcessedExactlyOnce(t *testing.T) {
	consumer, handler := newVEFinalOutcomeFixture(t)
	envelope := veFinalOutcomeEnvelope(t, "tenant-a/final-outcome/parcel-1/TF-DELIVERY/responsibility-outcome/v1")

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
	if got.TenantID != "tenant-a" || got.Parcel != "parcel-1" ||
		got.Kind != "TF-DELIVERY" || got.Version != "responsibility-outcome/v1" {
		t.Fatalf("译码结果 = %+v；四维必须齐备才取得回终局登记", got)
	}
}

func TestAFinalOutcomeEnvelopeMissingAnyKeyDimensionIsPoison(t *testing.T) {
	for name, payload := range map[string]string{
		"缺 tenantId": `{"parcel":"parcel-1","kind":"TF-DELIVERY","version":"responsibility-outcome/v1"}`,
		"缺 parcel":   `{"tenantId":"tenant-a","kind":"TF-DELIVERY","version":"responsibility-outcome/v1"}`,
		"缺 kind":     `{"tenantId":"tenant-a","parcel":"parcel-1","version":"responsibility-outcome/v1"}`,
		"缺 version":  `{"tenantId":"tenant-a","parcel":"parcel-1","kind":"TF-DELIVERY"}`,
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newVEFinalOutcomeFixture(t)
			poison := veFinalOutcomeEnvelope(t, "poison-"+name)
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

func TestAFailedFinalOutcomeHandlerRollsBackAndTheRedeliveryRetries(t *testing.T) {
	consumer, handler := newVEFinalOutcomeFixture(t)
	envelope := veFinalOutcomeEnvelope(t, "tenant-a/final-outcome/parcel-1/TF-DELIVERY/responsibility-outcome/v1")

	handler.err = errors.New("final outcome is not yet visible")
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

// 终局的上游是有效交付，两者同包同租户、时间也相近。认错一个就会让同一次交付在追踪
// 上出现两个终局，所以这条把「不认上游、不认旁路」钉死。
func TestForeignTypesAreLoudOnTheFinalOutcomeConsumer(t *testing.T) {
	for name, eventType := range map[string]eventing.EventType{
		"有效交付登记（终局的上游）": veinbox.EffectiveDeliveryRegisteredEventType,
		"权威交接登记":        veinbox.TransportHandoverRegisteredEventType,
		"对象级揽收登记":       veinbox.OffsitePickupRegisteredEventType,
		"节点收寄形成":        veinbox.NodeIntakeFormedEventType,
		"取消决定":          "parcel-shipment.parcel-cancellation.recorded",
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newVEFinalOutcomeFixture(t)
			foreign := veFinalOutcomeEnvelope(t, "foreign-"+name)
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

// 终局与有效交付是同一条业务链的两拍，事件 ID 撞车最有可能发生在这两者之间。
func TestFinalOutcomeInboxLedgersAreSeparateFromEffectiveDelivery(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}

	finalHandler := &veFormedFinalOutcomeHandlerDouble{}
	finalConsumer, err := veinbox.NewFinalOutcomeConsumer(db.Transactor(), store, finalHandler)
	if err != nil {
		t.Fatalf("构造终局消费者：%v", err)
	}
	deliveryHandler := &veRegisteredDeliveryHandlerDouble{}
	deliveryConsumer, err := veinbox.NewEffectiveDeliveryConsumer(db.Transactor(), store, deliveryHandler)
	if err != nil {
		t.Fatalf("构造交付消费者：%v", err)
	}

	const shared = "shared-final-and-delivery-event-id"
	finalEnvelope := veFinalOutcomeEnvelope(t, shared)
	deliveryEnvelope := veEffectiveDeliveryEnvelope(t, shared)
	deliveryEnvelope.Source = finalEnvelope.Source

	if err := finalConsumer.Consume(t.Context(), finalEnvelope); err != nil {
		t.Fatalf("终局投递：%v", err)
	}
	if err := deliveryConsumer.Consume(t.Context(), deliveryEnvelope); err != nil {
		t.Fatalf("交付投递：%v", err)
	}
	if len(finalHandler.calls) != 1 || len(deliveryHandler.calls) != 1 {
		t.Fatalf("两本账应各处理一次：终局 = %d，交付 = %d",
			len(finalHandler.calls), len(deliveryHandler.calls))
	}
}
