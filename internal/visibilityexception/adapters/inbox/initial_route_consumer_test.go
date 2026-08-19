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

// 本文件对真实 PostgreSQL 证 VE 初始路由消费门：六维（含 customerAccountId）译码、
// 恰一次、毒丸、与其它投影账本分家。correlation 是载荷附带的交接关联，不进引用。

type veFormedInitialRouteHandlerDouble struct {
	calls []veinbox.FormedInitialRoute
	err   error
}

func (double *veFormedInitialRouteHandlerDouble) HandleFormedInitialRoute(
	_ context.Context, formed veinbox.FormedInitialRoute,
) error {
	if double.err != nil {
		return double.err
	}
	double.calls = append(double.calls, formed)
	return nil
}

func newVEInitialRouteFixture(t *testing.T) (*veinbox.InitialRouteConsumer, *veFormedInitialRouteHandlerDouble) {
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
	handler := &veFormedInitialRouteHandlerDouble{}
	consumer, err := veinbox.NewInitialRouteConsumer(db.Transactor(), store, handler)
	if err != nil {
		t.Fatalf("构造消费者：%v", err)
	}
	return consumer, handler
}

func veInitialRouteEnvelope(t *testing.T, eventID string) eventing.Envelope {
	t.Helper()

	// correlation 有意入载荷：译码必须容忍并忽略它——它是交接关联，不是判断键维。
	payload, err := json.Marshal(map[string]string{
		"tenantId":          "tenant-a",
		"customerAccountId": "customer-a",
		"shipment":          "request-1",
		"parcel":            "parcel-1",
		"baseline":          "baseline-v1",
		"purpose":           "LAST_MILE_DELIVERY",
		"correlation":       "corr-1",
	})
	if err != nil {
		t.Fatalf("载荷：%v", err)
	}
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	return eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       "idp-parcel/network-routing",
		Type:         veinbox.InitialRouteFormedEventType,
		Version:      1,
		Scope:        "tenant-a",
		Subject:      "parcel-1",
		PartitionKey: eventID,
		OccurredAt:   now,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}
}

func TestAFormedInitialRouteIsProcessedExactlyOnce(t *testing.T) {
	consumer, handler := newVEInitialRouteFixture(t)
	envelope := veInitialRouteEnvelope(t, "tenant-a/initial-route/request-1/parcel-1/baseline-v1/LAST_MILE_DELIVERY")

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
	if got.TenantID != "tenant-a" || got.CustomerAccountID != "customer-a" ||
		got.Shipment != "request-1" || got.Parcel != "parcel-1" ||
		got.Baseline != "baseline-v1" || got.Purpose != "LAST_MILE_DELIVERY" {
		t.Fatalf("译码结果 = %+v；六维（含 customerAccountId）都必须进引用", got)
	}
}

func TestAnInitialRouteEnvelopeMissingAnyKeyDimensionIsPoison(t *testing.T) {
	for name, payload := range map[string]string{
		"缺 tenantId":          `{"customerAccountId":"customer-a","shipment":"request-1","parcel":"parcel-1","baseline":"baseline-v1","purpose":"LAST_MILE_DELIVERY"}`,
		"缺 customerAccountId": `{"tenantId":"tenant-a","shipment":"request-1","parcel":"parcel-1","baseline":"baseline-v1","purpose":"LAST_MILE_DELIVERY"}`,
		"缺 shipment":          `{"tenantId":"tenant-a","customerAccountId":"customer-a","parcel":"parcel-1","baseline":"baseline-v1","purpose":"LAST_MILE_DELIVERY"}`,
		"缺 parcel":            `{"tenantId":"tenant-a","customerAccountId":"customer-a","shipment":"request-1","baseline":"baseline-v1","purpose":"LAST_MILE_DELIVERY"}`,
		"缺 baseline":          `{"tenantId":"tenant-a","customerAccountId":"customer-a","shipment":"request-1","parcel":"parcel-1","purpose":"LAST_MILE_DELIVERY"}`,
		"缺 purpose":           `{"tenantId":"tenant-a","customerAccountId":"customer-a","shipment":"request-1","parcel":"parcel-1","baseline":"baseline-v1"}`,
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newVEInitialRouteFixture(t)
			poison := veInitialRouteEnvelope(t, "poison-"+name)
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

func TestAFailedInitialRouteHandlerRollsBackAndTheRedeliveryRetries(t *testing.T) {
	consumer, handler := newVEInitialRouteFixture(t)
	envelope := veInitialRouteEnvelope(t, "tenant-a/initial-route/request-1/parcel-1/baseline-v1/LAST_MILE_DELIVERY")

	handler.err = errors.New("initial route judgment is not yet visible")
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

func TestAPoisonInitialRouteEnvelopeIsRejectedOnceAndStaysRejected(t *testing.T) {
	consumer, handler := newVEInitialRouteFixture(t)
	poison := veInitialRouteEnvelope(t, "poison")
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

func TestForeignTypesAreLoudOnTheInitialRouteConsumer(t *testing.T) {
	for name, eventType := range map[string]eventing.EventType{
		"权威交接登记":  veinbox.TransportHandoverRegisteredEventType,
		"有效交付登记":  veinbox.EffectiveDeliveryRegisteredEventType,
		"对象级揽收登记": veinbox.OffsitePickupRegisteredEventType,
		"节点收寄形成":  veinbox.NodeIntakeFormedEventType,
		"可达性判断":   "network-routing.reachability-judgment.formed",
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newVEInitialRouteFixture(t)
			foreign := veInitialRouteEnvelope(t, "foreign-"+name)
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

func TestInitialRouteInboxLedgersAreSeparateFromTransportHandover(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}

	routeHandler := &veFormedInitialRouteHandlerDouble{}
	routeConsumer, err := veinbox.NewInitialRouteConsumer(db.Transactor(), store, routeHandler)
	if err != nil {
		t.Fatalf("构造初始路由消费者：%v", err)
	}
	handoverHandler := &veRegisteredHandoverHandlerDouble{}
	handoverConsumer, err := veinbox.NewTransportHandoverConsumer(db.Transactor(), store, handoverHandler)
	if err != nil {
		t.Fatalf("构造交接消费者：%v", err)
	}

	const shared = "shared-event-id"
	routeEnvelope := veInitialRouteEnvelope(t, shared)
	handoverEnvelope := veTransportHandoverEnvelope(t, shared)
	handoverEnvelope.Source = routeEnvelope.Source

	if err := routeConsumer.Consume(t.Context(), routeEnvelope); err != nil {
		t.Fatalf("初始路由投递：%v", err)
	}
	if err := handoverConsumer.Consume(t.Context(), handoverEnvelope); err != nil {
		t.Fatalf("交接投递：%v", err)
	}
	if len(routeHandler.calls) != 1 || len(handoverHandler.calls) != 1 {
		t.Fatalf("两本账应各处理一次：初始路由 = %d，交接 = %d",
			len(routeHandler.calls), len(handoverHandler.calls))
	}
}
