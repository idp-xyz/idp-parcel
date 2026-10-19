package ppinbox_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	ppinbox "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/inbox"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证 SA SELL 评价请求已提交信封这一路的消费门与译码（ADR-0190 决定四）：只认
// settlement-accounting.sell-evaluation-request.submitted，载荷两维（tenantId / sellEvaluationRequestId）缺一即毒丸，
// 处理失败回滚重投、重复投递跳过；BUY 那一路的信封类型在这里同样是认不得的类型。

type submittedSellRequestHandlerDouble struct {
	calls []ppinbox.SubmittedSellEvaluationRequest
	err   error
}

func (double *submittedSellRequestHandlerDouble) HandleSubmittedSellEvaluationRequest(
	_ context.Context, submitted ppinbox.SubmittedSellEvaluationRequest,
) error {
	if double.err != nil {
		return double.err
	}
	double.calls = append(double.calls, submitted)
	return nil
}

func newSellRequestFixture(t *testing.T) (*ppinbox.SellEvaluationRequestSubmittedConsumer, *submittedSellRequestHandlerDouble) {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	handler := &submittedSellRequestHandlerDouble{}
	consumer, err := ppinbox.NewSellEvaluationRequestSubmittedConsumer(db.Transactor(), store, handler)
	if err != nil {
		t.Fatalf("构造消费者：%v", err)
	}
	return consumer, handler
}

func sellRequestEnvelope(t *testing.T, eventID string) eventing.Envelope {
	t.Helper()
	payload, err := json.Marshal(map[string]string{
		"tenantId":                "tenant-a",
		"sellEvaluationRequestId": "SELLREQ-SYN-01",
	})
	if err != nil {
		t.Fatalf("载荷：%v", err)
	}
	now := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	return eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       "idp-parcel/settlement-accounting",
		Type:         ppinbox.SellEvaluationRequestSubmittedEventType,
		Version:      1,
		Scope:        "tenant-a",
		Subject:      "SELLREQ-SYN-01",
		PartitionKey: "tenant-a/sell-evaluation-request/SYN-PARCEL-01",
		OccurredAt:   now,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}
}

func TestASubmittedSellRequestEnvelopeIsProcessedExactlyOnce(t *testing.T) {
	consumer, handler := newSellRequestFixture(t)
	envelope := sellRequestEnvelope(t, "sell-request-01")

	if err := consumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("首投：%v", err)
	}
	if err := consumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("重复投递：%v", err)
	}
	if len(handler.calls) != 1 {
		t.Fatalf("处理次数 = %d, want 1", len(handler.calls))
	}
	if got := handler.calls[0]; got.TenantID != "tenant-a" || got.SellEvaluationRequestID != "SELLREQ-SYN-01" {
		t.Fatalf("译码结果 = %+v", got)
	}
}

func TestASubmittedSellRequestEnvelopeMissingEitherReferenceIsPoison(t *testing.T) {
	for name, payload := range map[string]string{
		"缺 tenantId":                `{"sellEvaluationRequestId":"SELLREQ-SYN-01"}`,
		"缺 sellEvaluationRequestId": `{"tenantId":"tenant-a"}`,
		"带的是 BUY 那一路的键":             `{"tenantId":"tenant-a","evaluationRequestId":"SELLREQ-SYN-01"}`,
		"不是 JSON":                   `{`,
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newSellRequestFixture(t)
			poison := sellRequestEnvelope(t, "poison-"+name)
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

func TestAFailedSellRequestHandlerRollsBackAndTheRedeliveryRetries(t *testing.T) {
	consumer, handler := newSellRequestFixture(t)
	envelope := sellRequestEnvelope(t, "sell-request-01")

	handler.err = errors.New("no price card is in force")
	if err := consumer.Consume(t.Context(), envelope); err == nil {
		t.Fatal("处理失败必须让消费报错——未决不落毒丸账")
	}

	handler.err = nil
	if err := consumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("重投：%v", err)
	}
	if len(handler.calls) != 1 {
		t.Fatalf("处理次数 = %d, want 1", len(handler.calls))
	}
}

func TestForeignTypesAreLoudOnTheSellRequestConsumer(t *testing.T) {
	for name, eventType := range map[string]eventing.EventType{
		"BUY 评价请求已提交": ppinbox.EvaluationRequestSubmittedEventType,
		"评价已记录":       "parcel-pricing.evaluation.recorded",
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newSellRequestFixture(t)
			foreign := sellRequestEnvelope(t, "foreign-"+name)
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

func TestTheSellRequestConsumerRefusesNilDependencies(t *testing.T) {
	if _, err := ppinbox.NewSellEvaluationRequestSubmittedConsumer(nil, nil, nil); err == nil {
		t.Fatal("nil 依赖被收下了")
	}
}
