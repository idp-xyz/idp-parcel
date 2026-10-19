package tfinbox_test

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
	tfinbox "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/inbox"
)

// 本文件对真实 PostgreSQL 证初始路由那一路的消费门与译码（ADR-0201 决定七）：六维缺一即毒丸入账，处理失败回滚重投，同一封
// 只处理一次，认不得的类型响亮报错。夹具全部合成。

type formedRouteHandlerDouble struct {
	calls []tfinbox.FormedInitialRoute
	err   error
}

func (double *formedRouteHandlerDouble) HandleFormedInitialRoute(_ context.Context, formed tfinbox.FormedInitialRoute) error {
	if double.err != nil {
		return double.err
	}
	double.calls = append(double.calls, formed)
	return nil
}

func newRouteConsumer(t *testing.T) (*tfinbox.InitialRouteFormedConsumer, *formedRouteHandlerDouble) {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	handler := &formedRouteHandlerDouble{}
	consumer, err := tfinbox.NewInitialRouteFormedConsumer(db.Transactor(), store, handler)
	if err != nil {
		t.Fatalf("构造消费者：%v", err)
	}
	return consumer, handler
}

func routeEnvelope(eventID string, eventType eventing.EventType, payload string) eventing.Envelope {
	now := time.Date(2026, 10, 21, 2, 0, 0, 0, time.UTC)
	return eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       "idp-parcel/network-routing",
		Type:         eventType,
		Version:      1,
		Scope:        "SYN-TENANT-01",
		Subject:      "SYN-PARCEL-01",
		PartitionKey: "SYN-TENANT-01/initial-route/SYN-PARCEL-01",
		OccurredAt:   now,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      json.RawMessage(payload),
	}
}

const formedRoutePayload = `{"tenantId":"SYN-TENANT-01","customerAccountId":"SYN-CUSTOMER-01","shipment":"SYN-REQ-01",` +
	`"parcel":"SYN-PARCEL-01","baseline":"SYN-VER-01","purpose":"NETWORK_SERVICE","correlation":"SYN-CORR-01"}`

// Covers: AT-TF-126（消费门）——同一封只处理一次，六维照载荷译出，correlation 不读。
func TestAFormedInitialRouteIsHandledExactlyOnce(t *testing.T) {
	consumer, handler := newRouteConsumer(t)
	envelope := routeEnvelope("route-1", tfinbox.InitialRouteFormedEventType, formedRoutePayload)
	for range 2 {
		if err := consumer.Consume(t.Context(), envelope); err != nil {
			t.Fatalf("投递：%v", err)
		}
	}
	want := tfinbox.FormedInitialRoute{
		TenantID: "SYN-TENANT-01", CustomerAccountID: "SYN-CUSTOMER-01", Shipment: "SYN-REQ-01",
		Parcel: "SYN-PARCEL-01", Baseline: "SYN-VER-01", Purpose: "NETWORK_SERVICE",
	}
	if len(handler.calls) != 1 || handler.calls[0] != want {
		t.Fatalf("处理 = %+v", handler.calls)
	}
}

// Covers: 毒丸拒收入账不交处理方；处理失败回滚、重投再试；认不得的类型不入账、响亮报错。
func TestPoisonFailureAndForeignTypesAreKeptApart(t *testing.T) {
	consumer, handler := newRouteConsumer(t)
	if err := consumer.Consume(t.Context(), routeEnvelope("route-poison", tfinbox.InitialRouteFormedEventType,
		`{"tenantId":"SYN-TENANT-01","parcel":"SYN-PARCEL-01"}`)); err != nil {
		t.Fatalf("毒丸应入账不报错：%v", err)
	}
	if len(handler.calls) != 0 {
		t.Fatal("毒丸交给了处理方")
	}

	handler.err = errors.New("undecided")
	envelope := routeEnvelope("route-2", tfinbox.InitialRouteFormedEventType, formedRoutePayload)
	if err := consumer.Consume(t.Context(), envelope); !errors.Is(err, handler.err) {
		t.Fatalf("处理失败 = %v", err)
	}
	handler.err = nil
	if err := consumer.Consume(t.Context(), envelope); err != nil || len(handler.calls) != 1 {
		t.Fatalf("回滚后重投 = %v，处理 %d 次", err, len(handler.calls))
	}

	if err := consumer.Consume(t.Context(), routeEnvelope("route-3", "network-routing.something-else", formedRoutePayload)); err == nil {
		t.Fatal("认不得的类型没报错")
	}
}
