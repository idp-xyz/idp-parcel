package ccinbox_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	ccinbox "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/inbox"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证资金事实采用信封这一路的消费门与译码（票 sa-cc/03）：只认
// settlement-accounting.external-funds-fact.adopted，载荷三维（tenantId / fact / version）缺一即毒丸，
// corrects 可缺席且不进译码——更正回指由处理方按引用读事实本体时取，不是消费者的事。

type adoptedFundsFactHandlerDouble struct {
	calls []ccinbox.AdoptedExternalFundsFact
	err   error
}

func (double *adoptedFundsFactHandlerDouble) HandleAdoptedExternalFundsFact(
	_ context.Context, adopted ccinbox.AdoptedExternalFundsFact,
) error {
	if double.err != nil {
		return double.err
	}
	double.calls = append(double.calls, adopted)
	return nil
}

func newExternalFundsFactFixture(t *testing.T) (*ccinbox.ExternalFundsFactConsumer, *adoptedFundsFactHandlerDouble) {
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
	handler := &adoptedFundsFactHandlerDouble{}
	consumer, err := ccinbox.NewExternalFundsFactConsumer(db.Transactor(), store, handler)
	if err != nil {
		t.Fatalf("构造消费者：%v", err)
	}
	return consumer, handler
}

func externalFundsFactEnvelope(t *testing.T, eventID string) eventing.Envelope {
	t.Helper()
	payload, err := json.Marshal(map[string]string{
		"tenantId": "tenant-a",
		"fact":     "bank-fact-1",
		"version":  "bank-fact/v1",
	})
	if err != nil {
		t.Fatalf("载荷：%v", err)
	}
	now := time.Date(2026, 9, 10, 21, 0, 0, 0, time.UTC)
	return eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       "idp-parcel/settlement-accounting",
		Type:         ccinbox.ExternalFundsFactAdoptedEventType,
		Version:      1,
		Scope:        "tenant-a",
		Subject:      "bank-fact-1/bank-fact/v1",
		PartitionKey: "tenant-a/funds-fact/bank-fact-1",
		OccurredAt:   now,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}
}

func TestAnAdoptedFundsFactEnvelopeIsProcessedExactlyOnce(t *testing.T) {
	consumer, handler := newExternalFundsFactFixture(t)
	envelope := externalFundsFactEnvelope(t, "tenant-a/funds-fact/bank-fact-1/bank-fact/v1")

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
	if got.TenantID != "tenant-a" || got.Fact != "bank-fact-1" || got.Version != "bank-fact/v1" {
		t.Fatalf("译码结果 = %+v", got)
	}
}

// Covers: 更正版本同型再发一封、载荷带 corrects（票 sa-cc/02 裁决 2）——消费者照样只按三维译，
// corrects 不进译码：处理方按引用读事实本体时自会看到回指，消费者不替它转述。
func TestACorrectionEnvelopeDecodesByTheSameThreeDimensions(t *testing.T) {
	consumer, handler := newExternalFundsFactFixture(t)
	envelope := externalFundsFactEnvelope(t, "tenant-a/funds-fact/bank-fact-1/bank-fact/v2")
	envelope.Payload = json.RawMessage(
		`{"tenantId":"tenant-a","fact":"bank-fact-1","version":"bank-fact/v2","corrects":"bank-fact/v1"}`)

	if err := consumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("带 corrects 的载荷仍应按三维处理：%v", err)
	}
	if len(handler.calls) != 1 {
		t.Fatalf("处理次数 = %d, want 1", len(handler.calls))
	}
	if got := handler.calls[0]; got.TenantID != "tenant-a" || got.Fact != "bank-fact-1" || got.Version != "bank-fact/v2" {
		t.Fatalf("译码结果 = %+v", got)
	}
}

func TestAnAdoptedFundsFactEnvelopeMissingAnyKeyDimensionIsPoison(t *testing.T) {
	for name, payload := range map[string]string{
		"缺 tenantId": `{"fact":"bank-fact-1","version":"bank-fact/v1"}`,
		"缺 fact":     `{"tenantId":"tenant-a","version":"bank-fact/v1"}`,
		"缺 version":  `{"tenantId":"tenant-a","fact":"bank-fact-1"}`,
		"不是 JSON":    `{`,
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newExternalFundsFactFixture(t)
			poison := externalFundsFactEnvelope(t, "poison-"+name)
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

func TestAFailedAdoptedFundsFactHandlerRollsBackAndTheRedeliveryRetries(t *testing.T) {
	consumer, handler := newExternalFundsFactFixture(t)
	envelope := externalFundsFactEnvelope(t, "tenant-a/funds-fact/bank-fact-1/bank-fact/v1")

	handler.err = errors.New("adopted fact is not yet visible")
	if err := consumer.Consume(t.Context(), envelope); err == nil {
		t.Fatal("处理失败必须让消费报错——可见性滞后不落毒丸账")
	}

	handler.err = nil
	if err := consumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("重投：%v", err)
	}
	if len(handler.calls) != 1 {
		t.Fatalf("处理次数 = %d, want 1", len(handler.calls))
	}
}

func TestForeignTypesAreLoudOnTheExternalFundsFactConsumer(t *testing.T) {
	for name, eventType := range map[string]eventing.EventType{
		"核销已施加":   "settlement-accounting.settlement-application.applied",
		"供应商账单接收": "settlement-accounting.supplier-bill.received",
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newExternalFundsFactFixture(t)
			foreign := externalFundsFactEnvelope(t, "foreign-"+name)
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

func TestTheExternalFundsFactConsumerRefusesNilDependencies(t *testing.T) {
	if _, err := ccinbox.NewExternalFundsFactConsumer(nil, nil, nil); err == nil {
		t.Fatal("nil 依赖被收下了")
	}
}
