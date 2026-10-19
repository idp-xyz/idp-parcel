package sainbox_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"

	sainbox "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/inbox"
)

// 本文件对真实 PostgreSQL 16 证发生项已形成那一路的消费门与译码（ADR-0208）：载荷三维（tenantId / occurrence / version）
// 缺一即毒丸，处理失败回滚重投，同一封只处理一次，认不得的类型响亮报错。

type formedOccurrenceHandlerDouble struct {
	calls []sainbox.FormedChargeOccurrence
	err   error
}

func (double *formedOccurrenceHandlerDouble) HandleFormedChargeOccurrence(
	_ context.Context, formed sainbox.FormedChargeOccurrence,
) error {
	if double.err != nil {
		return double.err
	}
	double.calls = append(double.calls, formed)
	return nil
}

func newOccurrenceFormedConsumer(t *testing.T) (*sainbox.ChargeOccurrenceFormedConsumer, *formedOccurrenceHandlerDouble) {
	t.Helper()
	db, store := parcelMomentInbox(t)
	handler := &formedOccurrenceHandlerDouble{}
	consumer, err := sainbox.NewChargeOccurrenceFormedConsumer(db.Transactor(), store, handler)
	if err != nil {
		t.Fatalf("构造消费者：%v", err)
	}
	return consumer, handler
}

func occurrenceFormedEnvelope(t *testing.T, eventID, payload string) eventing.Envelope {
	t.Helper()
	now := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	return eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       "idp-parcel/transport-fulfillment",
		Type:         sainbox.ChargeOccurrenceFormedEventType,
		Version:      1,
		Scope:        "tenant-a",
		Subject:      "SYN-OCC-01",
		PartitionKey: "tenant-a/charge-occurrence/SYN-OCC-01",
		OccurredAt:   now,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      json.RawMessage(payload),
	}
}

const formedOccurrencePayload = `{"tenantId":"tenant-a","occurrence":"SYN-OCC-01","version":"V1"}`

func TestAChargeOccurrenceFormedEnvelopeIsProcessedExactlyOnce(t *testing.T) {
	consumer, handler := newOccurrenceFormedConsumer(t)
	envelope := occurrenceFormedEnvelope(t, "occurrence-1", formedOccurrencePayload)
	if err := consumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("首投：%v", err)
	}
	if err := consumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("重复投递：%v", err)
	}
	if len(handler.calls) != 1 {
		t.Fatalf("处理次数 = %d, want 1", len(handler.calls))
	}
	if got := handler.calls[0]; got != (sainbox.FormedChargeOccurrence{TenantID: "tenant-a", Occurrence: "SYN-OCC-01", Version: "V1"}) {
		t.Fatalf("译码结果 = %+v", got)
	}
}

func TestAChargeOccurrenceFormedEnvelopeMissingAnyKeyDimensionIsPoison(t *testing.T) {
	for name, payload := range map[string]string{
		"缺 tenantId":   `{"occurrence":"O","version":"V"}`,
		"缺 occurrence": `{"tenantId":"T","version":"V"}`,
		"缺 version":    `{"tenantId":"T","occurrence":"O"}`,
		"不是 JSON":      `{`,
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newOccurrenceFormedConsumer(t)
			if err := consumer.Consume(t.Context(), occurrenceFormedEnvelope(t, "poison-"+name, payload)); err != nil {
				t.Fatalf("毒丸应拒收入账而不是报错：%v", err)
			}
			if len(handler.calls) != 0 {
				t.Fatalf("处理次数 = %d, want 0", len(handler.calls))
			}
		})
	}
}

func TestAFailedChargeOccurrenceHandlerRollsBackAndTheRedeliveryRetries(t *testing.T) {
	consumer, handler := newOccurrenceFormedConsumer(t)
	envelope := occurrenceFormedEnvelope(t, "occurrence-retry", formedOccurrencePayload)
	handler.err = errors.New("syn: trigger register down")
	if err := consumer.Consume(t.Context(), envelope); err == nil {
		t.Fatal("处理失败必须让消费报错，不落账")
	}
	handler.err = nil
	if err := consumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("重投：%v", err)
	}
	if len(handler.calls) != 1 {
		t.Fatalf("处理次数 = %d, want 1", len(handler.calls))
	}
}

func TestTheChargeOccurrenceConsumerIsLoudOnAnotherTypeAndRefusesNilDependencies(t *testing.T) {
	consumer, handler := newOccurrenceFormedConsumer(t)
	foreign := occurrenceFormedEnvelope(t, "foreign", formedOccurrencePayload)
	foreign.Type = sainbox.BuyEvaluationRecordedEventType
	if err := consumer.Consume(t.Context(), foreign); err == nil {
		t.Fatal("认不得的类型必须响亮报错")
	}
	if len(handler.calls) != 0 {
		t.Fatalf("处理次数 = %d, want 0", len(handler.calls))
	}
	if _, err := sainbox.NewChargeOccurrenceFormedConsumer(nil, nil, nil); err == nil {
		t.Fatal("消费者收下了 nil 依赖")
	}
}
