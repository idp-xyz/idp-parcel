package noinbox_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	noinbox "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/inbox"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证节点控制转出那一扇消费门（ADR-0214 决定六）：交接登记信封恰处理一次、四维原样转交；缺一维即毒丸；
// 处理失败整笔回滚、重投再试；认不得的类型响亮报错。夹具全为合成（S 级）。

type registeredHandoverHandlerDouble struct {
	calls []noinbox.RegisteredTransportHandover
	err   error
}

func (double *registeredHandoverHandlerDouble) HandleRegisteredTransportHandover(
	_ context.Context, registered noinbox.RegisteredTransportHandover,
) error {
	if double.err != nil {
		return double.err
	}
	double.calls = append(double.calls, registered)
	return nil
}

func newRegisteredHandoverFixture(t *testing.T) (*noinbox.TransportHandoverRegisteredConsumer, *registeredHandoverHandlerDouble) {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	handler := &registeredHandoverHandlerDouble{}
	consumer, err := noinbox.NewTransportHandoverRegisteredConsumer(db.Transactor(), store, handler)
	if err != nil {
		t.Fatalf("构造消费者：%v", err)
	}
	return consumer, handler
}

func registeredHandoverEnvelope(t *testing.T, eventID string, payload map[string]string) eventing.Envelope {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("载荷：%v", err)
	}
	now := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	return eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       "idp-parcel/transport-fulfillment",
		Type:         noinbox.TransportHandoverRegisteredEventType,
		Version:      1,
		Scope:        "SYN-TENANT-61",
		Subject:      "SYN-PARCEL-61",
		PartitionKey: "partition-" + eventID,
		OccurredAt:   now,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      raw,
	}
}

var syntheticRegisteredHandover = map[string]string{
	"tenantId": "SYN-TENANT-61", "object": "SYN-PARCEL-61", "scope": "SYN-SCOPE-61", "version": "SYN-HANDOVER-61/v1",
}

// Covers: 一封交接信封恰处理一次，四维原样转交。
func TestARegisteredHandoverIsHandledExactlyOnce(t *testing.T) {
	consumer, handler := newRegisteredHandoverFixture(t)
	envelope := registeredHandoverEnvelope(t, "handover-61", syntheticRegisteredHandover)

	for range 2 {
		if err := consumer.Consume(t.Context(), envelope); err != nil {
			t.Fatalf("投递：%v", err)
		}
	}
	want := noinbox.RegisteredTransportHandover{
		TenantID: "SYN-TENANT-61", Object: "SYN-PARCEL-61", Scope: "SYN-SCOPE-61", Version: "SYN-HANDOVER-61/v1",
	}
	if len(handler.calls) != 1 || handler.calls[0] != want {
		t.Fatalf("处理 = %+v, want 恰一次 %+v", handler.calls, want)
	}
}

// Covers: 四维缺一或空白即毒丸：拒收入账、不转交、不报错——重投同样内容不会长出字段来。
func TestARegisteredHandoverMissingAKeyFieldIsPoison(t *testing.T) {
	for _, missing := range []string{"tenantId", "object", "scope", "version"} {
		t.Run(missing, func(t *testing.T) {
			consumer, handler := newRegisteredHandoverFixture(t)
			payload := map[string]string{}
			for key, value := range syntheticRegisteredHandover {
				payload[key] = value
			}
			payload[missing] = "  "
			poison := registeredHandoverEnvelope(t, "poison", payload)
			for range 2 {
				if err := consumer.Consume(t.Context(), poison); err != nil {
					t.Fatalf("毒丸应拒收入账而不是报错：%v", err)
				}
			}
			if len(handler.calls) != 0 {
				t.Fatalf("处理次数 = %d, want 0", len(handler.calls))
			}
		})
	}
}

// Covers: 处理方没形成答案时整笔回滚、inbox 无账，重投再试；认不得的类型响亮报错、不入账。
func TestAFailedTransferRollsBackAndAForeignTypeIsLoud(t *testing.T) {
	consumer, handler := newRegisteredHandoverFixture(t)
	envelope := registeredHandoverEnvelope(t, "handover-61", syntheticRegisteredHandover)

	handler.err = errors.New("transport handover is not yet visible")
	if err := consumer.Consume(t.Context(), envelope); err == nil {
		t.Fatal("处理失败必须让消费报错")
	}
	handler.err = nil
	if err := consumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("重投：%v", err)
	}
	if len(handler.calls) != 1 {
		t.Fatalf("处理次数 = %d, want 1", len(handler.calls))
	}

	foreign := registeredHandoverEnvelope(t, "foreign", syntheticRegisteredHandover)
	foreign.Type = "transport-fulfillment.effective-delivery.registered"
	if err := consumer.Consume(t.Context(), foreign); err == nil {
		t.Fatal("认不得的类型必须响亮报错")
	}
}
