package nrinbox_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	nrinbox "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/inbox"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"

	"go.idp.xyz/idp-bento-go/eventing"
)

// 本文件对真实 PostgreSQL 证交接触发那扇消费门（ADR-0215）：译码四维、恰一次、失败回滚、毒丸拒收、认不得的类型响亮。

type transportHandoverHandlerDouble struct {
	calls []nrinbox.RegisteredTransportHandover
	err   error
}

func (double *transportHandoverHandlerDouble) HandleRegisteredTransportHandover(
	_ context.Context,
	registered nrinbox.RegisteredTransportHandover,
) error {
	if double.err != nil {
		return double.err
	}
	double.calls = append(double.calls, registered)
	return nil
}

func newTransportHandoverFixture(t *testing.T) (*nrinbox.TransportHandoverConsumer, *transportHandoverHandlerDouble) {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	handler := &transportHandoverHandlerDouble{}
	consumer, err := nrinbox.NewTransportHandoverConsumer(db.Transactor(), store, handler)
	if err != nil {
		t.Fatalf("构造消费者：%v", err)
	}
	return consumer, handler
}

func transportHandoverEnvelope(t *testing.T, eventID, payload string) eventing.Envelope {
	t.Helper()
	at := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	return eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       "idp-parcel/transport-fulfillment",
		Type:         nrinbox.TransportHandoverRegisteredEventType,
		Version:      1,
		Scope:        "tenant-a",
		Subject:      "parcel-1",
		PartitionKey: "tenant-a/parcel-1/transport-handover-registration",
		OccurredAt:   at,
		RecordedAt:   at,
		ContentType:  eventing.JSONContentType,
		Payload:      json.RawMessage(payload),
	}
}

const wellFormedHandoverPayload = `{"tenantId":"tenant-a","object":"parcel-1","scope":"scope-1","version":"handover/v1"}`

// Covers: AT-NR-068（消费门那一半）——同一封信恰处理一次，重投在门上跳过；四维一路带到处理方，处理方按它们读回交接。
func TestATransportHandoverDeliveryIsProcessedExactlyOnce(t *testing.T) {
	consumer, handler := newTransportHandoverFixture(t)
	envelope := transportHandoverEnvelope(t, "handover-1", wellFormedHandoverPayload)

	for range 2 {
		if err := consumer.Consume(t.Context(), envelope); err != nil {
			t.Fatalf("消费：%v", err)
		}
	}
	if len(handler.calls) != 1 {
		t.Fatalf("处理次数 = %d, want 1", len(handler.calls))
	}
	want := nrinbox.RegisteredTransportHandover{
		TenantID: "tenant-a", Object: "parcel-1", Scope: "scope-1", Version: "handover/v1",
	}
	if handler.calls[0] != want {
		t.Fatalf("译码结果 = %+v, want %+v", handler.calls[0], want)
	}
}

// 四维缺一即毒丸：处理方按整键读回交接，缺任一维都取不着，重投同样内容不会长出字段来。拒收也是账，重投不再报错。
func TestATransportHandoverEnvelopeMissingAnyKeyDimensionIsPoison(t *testing.T) {
	for name, payload := range map[string]string{
		"缺 tenantId": `{"object":"parcel-1","scope":"scope-1","version":"handover/v1"}`,
		"缺 object":   `{"tenantId":"tenant-a","scope":"scope-1","version":"handover/v1"}`,
		"缺 scope":    `{"tenantId":"tenant-a","object":"parcel-1","version":"handover/v1"}`,
		"缺 version":  `{"tenantId":"tenant-a","object":"parcel-1","scope":"scope-1"}`,
		"不是 JSON":    `{`,
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newTransportHandoverFixture(t)
			poison := transportHandoverEnvelope(t, "handover-"+name, payload)
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

// Covers: AT-NR-067（消费门那一半）——处理方报错整体回滚，inbox 无痕，重投再试。复核未决、交接读不回、PS 读不动都走这一格。
func TestAFailedTransportHandoverHandlerRollsBackAndTheRedeliveryRetries(t *testing.T) {
	consumer, handler := newTransportHandoverFixture(t)
	envelope := transportHandoverEnvelope(t, "handover-1", wellFormedHandoverPayload)

	handler.err = errors.New("reassessment undecided")
	if err := consumer.Consume(t.Context(), envelope); err == nil {
		t.Fatal("处理失败必须让消费报错")
	}
	handler.err = nil
	if err := consumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("重投：%v", err)
	}
	if len(handler.calls) != 1 {
		t.Fatalf("处理次数 = %d, want 1——失败那次不算处理", len(handler.calls))
	}
}

// 认不得的类型响亮报错不入账。拿收寄采用的类型：路由表把两类都投给本进程，串线时错的是收件人，拒收会把收寄采用记进
// 交接复核的账。
func TestAForeignTypeIsLoudOnTheTransportHandoverConsumer(t *testing.T) {
	consumer, handler := newTransportHandoverFixture(t)
	foreign := transportHandoverEnvelope(t, "handover-foreign", wellFormedHandoverPayload)
	foreign.Type = nrinbox.AdoptedNetworkIntakeEventType

	if err := consumer.Consume(t.Context(), foreign); err == nil {
		t.Fatal("认不得的类型必须响亮报错")
	}
	if len(handler.calls) != 0 {
		t.Fatalf("处理次数 = %d, want 0", len(handler.calls))
	}
}
