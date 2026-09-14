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

// 本文件对真实 PostgreSQL 16 证 SA 评价请求已提交信封这一路的消费门与译码（票 sa-cc/11 裁决 5）：只认
// settlement-accounting.evaluation-request.submitted，载荷两维（tenantId / evaluationRequestId）缺一即毒丸，
// 处理失败回滚重投、重复投递跳过。请求内容（主要范围 / 目的 / 来源引用）不在这里——信封只带引用，处理方按
// 引用向 SA 读口取那一份。

type submittedRequestHandlerDouble struct {
	calls []ppinbox.SubmittedEvaluationRequest
	err   error
}

func (double *submittedRequestHandlerDouble) HandleSubmittedEvaluationRequest(
	_ context.Context, submitted ppinbox.SubmittedEvaluationRequest,
) error {
	if double.err != nil {
		return double.err
	}
	double.calls = append(double.calls, submitted)
	return nil
}

func newRequestFixture(t *testing.T) (*ppinbox.EvaluationRequestSubmittedConsumer, *submittedRequestHandlerDouble) {
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
	handler := &submittedRequestHandlerDouble{}
	consumer, err := ppinbox.NewEvaluationRequestSubmittedConsumer(db.Transactor(), store, handler)
	if err != nil {
		t.Fatalf("构造消费者：%v", err)
	}
	return consumer, handler
}

func requestEnvelope(t *testing.T, eventID string) eventing.Envelope {
	t.Helper()
	payload, err := json.Marshal(map[string]string{
		"tenantId":            "tenant-a",
		"evaluationRequestId": "EVREQ-SYN-01",
	})
	if err != nil {
		t.Fatalf("载荷：%v", err)
	}
	now := time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC)
	return eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       "idp-parcel/settlement-accounting",
		Type:         ppinbox.EvaluationRequestSubmittedEventType,
		Version:      1,
		Scope:        "tenant-a",
		Subject:      "EVREQ-SYN-01",
		PartitionKey: "tenant-a/evaluation-request/EVREQ-SYN-01",
		OccurredAt:   now,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}
}

func TestASubmittedRequestEnvelopeIsProcessedExactlyOnce(t *testing.T) {
	consumer, handler := newRequestFixture(t)
	envelope := requestEnvelope(t, "tenant-a/evaluation-request/EVREQ-SYN-01/submitted")

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
	if got.TenantID != "tenant-a" || got.EvaluationRequestID != "EVREQ-SYN-01" {
		t.Fatalf("译码结果 = %+v", got)
	}
}

func TestASubmittedRequestEnvelopeMissingEitherReferenceIsPoison(t *testing.T) {
	for name, payload := range map[string]string{
		"缺 tenantId":            `{"evaluationRequestId":"EVREQ-SYN-01"}`,
		"缺 evaluationRequestId": `{"tenantId":"tenant-a"}`,
		"不是 JSON":               `{`,
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newRequestFixture(t)
			poison := requestEnvelope(t, "poison-"+name)
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

func TestAFailedRequestHandlerRollsBackAndTheRedeliveryRetries(t *testing.T) {
	consumer, handler := newRequestFixture(t)
	envelope := requestEnvelope(t, "tenant-a/evaluation-request/EVREQ-SYN-01/submitted")

	handler.err = errors.New("evaluation request is not yet visible")
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

func TestForeignTypesAreLoudOnTheRequestConsumer(t *testing.T) {
	for name, eventType := range map[string]eventing.EventType{
		"评价已记录":  "parcel-pricing.evaluation.recorded",
		"付款核对形成": "settlement-accounting.duty-payment-verification.formed",
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newRequestFixture(t)
			foreign := requestEnvelope(t, "foreign-"+name)
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

func TestTheRequestConsumerRefusesNilDependencies(t *testing.T) {
	if _, err := ppinbox.NewEvaluationRequestSubmittedConsumer(nil, nil, nil); err == nil {
		t.Fatal("nil 依赖被收下了")
	}
}
