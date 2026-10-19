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

// 本文件对真实 PostgreSQL 16 证 SELL 形成那一路的消费门与译码（票 sell-customer-charge/04；ADR-0190 决定六）：认的是
// 与 BUY 那一路同一种 parcel-pricing.evaluation.recorded，载荷两维缺一即毒丸，处理失败回滚重投；它另记自己的 inbox
// 账，同一封信先被 BUY 那一路收下，这一路照样收一次。方向与回指的分辨不在这里。

type recordedSellEvaluationHandlerDouble struct {
	calls []sainbox.RecordedSellEvaluation
	err   error
}

func (double *recordedSellEvaluationHandlerDouble) HandleRecordedSellEvaluation(
	_ context.Context, recorded sainbox.RecordedSellEvaluation,
) error {
	if double.err != nil {
		return double.err
	}
	double.calls = append(double.calls, recorded)
	return nil
}

func newSellEvaluationFixture(t *testing.T) (*sainbox.SellEvaluationRecordedConsumer, *recordedSellEvaluationHandlerDouble, *bentopg.DB, *inbox.Store) {
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
	handler := &recordedSellEvaluationHandlerDouble{}
	consumer, err := sainbox.NewSellEvaluationRecordedConsumer(db.Transactor(), store, handler)
	if err != nil {
		t.Fatalf("构造消费者：%v", err)
	}
	return consumer, handler, db, store
}

// 载荷键名在这里照提供方写一遍而不引提供方常量：它们就是跨上下文契约。
func sellEvaluationEnvelope(t *testing.T, eventID string) eventing.Envelope {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"tenantId": "tenant-a", "evaluationId": "SYN-EVAL-SELL-01"})
	if err != nil {
		t.Fatalf("载荷：%v", err)
	}
	now := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	return eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       "idp-parcel/parcel-pricing",
		Type:         "parcel-pricing.evaluation.recorded",
		Version:      1,
		Scope:        "tenant-a",
		Subject:      "SYN-EVAL-SELL-01",
		PartitionKey: "tenant-a/SYN-EVAL-SELL-01",
		OccurredAt:   now,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}
}

func TestARecordedSellEvaluationEnvelopeIsProcessedExactlyOnce(t *testing.T) {
	consumer, handler, _, _ := newSellEvaluationFixture(t)
	envelope := sellEvaluationEnvelope(t, "SYN-EVAL-SELL-01")

	if err := consumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("首投：%v", err)
	}
	if err := consumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("重复投递：%v", err)
	}
	if len(handler.calls) != 1 {
		t.Fatalf("处理次数 = %d, want 1", len(handler.calls))
	}
	if got := handler.calls[0]; got.TenantID != "tenant-a" || got.EvaluationID != "SYN-EVAL-SELL-01" {
		t.Fatalf("译码结果 = %+v", got)
	}
}

// Covers: 两路各记各的 inbox 账——同一封信先被 BUY 那一路收下，SELL 这一路照样收一次，反之亦然。
func TestTheSellAndBuyConsumersKeepSeparateInboxAccountsForTheSameEnvelope(t *testing.T) {
	sell, sellHandler, db, store := newSellEvaluationFixture(t)
	buyHandler := &recordedEvaluationHandlerDouble{}
	buy, err := sainbox.NewBuyEvaluationRecordedConsumer(db.Transactor(), store, buyHandler)
	if err != nil {
		t.Fatalf("构造 BUY 消费者：%v", err)
	}
	envelope := sellEvaluationEnvelope(t, "SYN-EVAL-SELL-01")

	if err := buy.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("BUY 那一路：%v", err)
	}
	if err := sell.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("SELL 这一路：%v", err)
	}
	if len(buyHandler.calls) != 1 || len(sellHandler.calls) != 1 {
		t.Fatalf("BUY 处理 %d 次、SELL 处理 %d 次，want 各一次", len(buyHandler.calls), len(sellHandler.calls))
	}
}

func TestARecordedSellEvaluationEnvelopeMissingEitherReferenceIsPoison(t *testing.T) {
	for name, payload := range map[string]string{
		"缺 tenantId":     `{"evaluationId":"SYN-EVAL-SELL-01"}`,
		"缺 evaluationId": `{"tenantId":"tenant-a"}`,
		"不是 JSON":        `{`,
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler, _, _ := newSellEvaluationFixture(t)
			poison := sellEvaluationEnvelope(t, "poison-"+name)
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

func TestAFailedSellEvaluationHandlerRollsBackAndTheRedeliveryRetries(t *testing.T) {
	consumer, handler, _, _ := newSellEvaluationFixture(t)
	envelope := sellEvaluationEnvelope(t, "SYN-EVAL-SELL-01")

	handler.err = errors.New("estimated customer charge formation is undecided")
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
}

func TestForeignTypesAreLoudOnTheSellEvaluationConsumer(t *testing.T) {
	for name, eventType := range map[string]eventing.EventType{
		"SELL 评价请求已提交": "settlement-accounting.sell-evaluation-request.submitted",
		"付款核对形成":       sainbox.DutyPaymentVerificationFormedEventType,
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler, _, _ := newSellEvaluationFixture(t)
			foreign := sellEvaluationEnvelope(t, "foreign-"+name)
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

func TestTheSellEvaluationConsumerRefusesNilDependencies(t *testing.T) {
	if _, err := sainbox.NewSellEvaluationRecordedConsumer(nil, nil, nil); err == nil {
		t.Fatal("nil 依赖被收下了")
	}
}
