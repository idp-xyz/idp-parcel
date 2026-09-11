package sainbox_test

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
	sainbox "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/inbox"
)

// 本文件对真实 PostgreSQL 16 证 BUY 评价已记录信封这一路的消费门与译码（票 sa-cc/01）：只认
// parcel-pricing.evaluation.recorded，载荷两维（tenantId / evaluationId）缺一即毒丸，处理失败回滚重投。
// 方向 / 目的的分辨不在这里——信封只带评价引用，是不是 BUY·SUPPLIER_COST 要处理方回查提供方才知道。

type recordedEvaluationHandlerDouble struct {
	calls []sainbox.RecordedBuyEvaluation
	err   error
}

func (double *recordedEvaluationHandlerDouble) HandleRecordedBuyEvaluation(
	_ context.Context, recorded sainbox.RecordedBuyEvaluation,
) error {
	if double.err != nil {
		return double.err
	}
	double.calls = append(double.calls, recorded)
	return nil
}

func newEvaluationFixture(t *testing.T) (*sainbox.BuyEvaluationRecordedConsumer, *recordedEvaluationHandlerDouble) {
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
	handler := &recordedEvaluationHandlerDouble{}
	consumer, err := sainbox.NewBuyEvaluationRecordedConsumer(db.Transactor(), store, handler)
	if err != nil {
		t.Fatalf("构造消费者：%v", err)
	}
	return consumer, handler
}

func evaluationEnvelope(t *testing.T, eventID string) eventing.Envelope {
	t.Helper()
	payload, err := json.Marshal(map[string]string{
		"tenantId":     "tenant-a",
		"evaluationId": "SYN-EVAL-01",
	})
	if err != nil {
		t.Fatalf("载荷：%v", err)
	}
	now := time.Date(2026, 9, 11, 9, 0, 0, 0, time.UTC)
	return eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       "idp-parcel/parcel-pricing",
		Type:         sainbox.BuyEvaluationRecordedEventType,
		Version:      1,
		Scope:        "tenant-a",
		Subject:      "SYN-EVAL-01",
		PartitionKey: "tenant-a/SYN-EVAL-01",
		OccurredAt:   now,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}
}

func TestARecordedEvaluationEnvelopeIsProcessedExactlyOnce(t *testing.T) {
	consumer, handler := newEvaluationFixture(t)
	envelope := evaluationEnvelope(t, "SYN-EVAL-01")

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
	if got.TenantID != "tenant-a" || got.EvaluationID != "SYN-EVAL-01" {
		t.Fatalf("译码结果 = %+v", got)
	}
}

func TestARecordedEvaluationEnvelopeMissingEitherReferenceIsPoison(t *testing.T) {
	for name, payload := range map[string]string{
		"缺 tenantId":     `{"evaluationId":"SYN-EVAL-01"}`,
		"缺 evaluationId": `{"tenantId":"tenant-a"}`,
		"不是 JSON":        `{`,
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newEvaluationFixture(t)
			poison := evaluationEnvelope(t, "poison-"+name)
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

func TestAFailedEvaluationHandlerRollsBackAndTheRedeliveryRetries(t *testing.T) {
	consumer, handler := newEvaluationFixture(t)
	envelope := evaluationEnvelope(t, "SYN-EVAL-01")

	handler.err = errors.New("evaluation is not yet visible")
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

func TestForeignTypesAreLoudOnTheEvaluationConsumer(t *testing.T) {
	for name, eventType := range map[string]eventing.EventType{
		"评价请求已提交": "settlement-accounting.evaluation-request.submitted",
		"付款核对形成":  sainbox.DutyPaymentVerificationFormedEventType,
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newEvaluationFixture(t)
			foreign := evaluationEnvelope(t, "foreign-"+name)
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

func TestTheEvaluationConsumerRefusesNilDependencies(t *testing.T) {
	if _, err := sainbox.NewBuyEvaluationRecordedConsumer(nil, nil, nil); err == nil {
		t.Fatal("nil 依赖被收下了")
	}
}
