package psinbox_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"

	"go.idp.xyz/idp-bento-go/eventing"
)

// 本文件对真实 PostgreSQL 16 证面单交易判断意图这一路的消费门与译码（lc/26，ADR-0134 决定一）：
// 只认 label-transaction.judgment-due，载荷三维（租户 + 交易 + 包裹）缺一即毒丸，revision 与 beat
// 不进译码——版本是事件 ID 的事，哪一拍只作追溯、消费者不据它分支。

type judgmentDueHandlerDouble struct {
	calls []psinbox.LabelTransactionJudgmentDue
	err   error
}

func (double *judgmentDueHandlerDouble) HandleLabelTransactionJudgmentDue(
	_ context.Context, due psinbox.LabelTransactionJudgmentDue,
) error {
	if double.err != nil {
		return double.err
	}
	double.calls = append(double.calls, due)
	return nil
}

func newJudgmentDueFixture(t *testing.T) (*psinbox.LabelTransactionJudgmentConsumer, *judgmentDueHandlerDouble) {
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
	handler := &judgmentDueHandlerDouble{}
	consumer, err := psinbox.NewLabelTransactionJudgmentConsumer(db.Transactor(), store, handler)
	if err != nil {
		t.Fatalf("构造消费者：%v", err)
	}
	return consumer, handler
}

func judgmentDueEnvelope(t *testing.T, eventID string) eventing.Envelope {
	t.Helper()

	payload, err := json.Marshal(map[string]any{
		"tenantId":    "tenant-a",
		"transaction": "LT-1",
		"parcel":      "parcel-1",
		"revision":    3,
		"beat":        "RESULT_RECORDED",
	})
	if err != nil {
		t.Fatalf("载荷：%v", err)
	}
	now := time.Date(2026, 9, 10, 18, 0, 0, 0, time.UTC)
	return eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       "idp-parcel/parcel-shipment",
		Type:         psinbox.LabelTransactionJudgmentDueEventType,
		Version:      1,
		Scope:        "tenant-a",
		Subject:      "parcel-1",
		PartitionKey: "tenant-a/parcel-1",
		OccurredAt:   now,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}
}

func TestAJudgmentDueEnvelopeIsProcessedExactlyOnce(t *testing.T) {
	consumer, handler := newJudgmentDueFixture(t)
	envelope := judgmentDueEnvelope(t, "tenant-a/label-transaction/LT-1/parcel-1/3/judgment-due")

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
	if got.TenantID != "tenant-a" || got.Transaction != "LT-1" || got.Parcel != "parcel-1" {
		t.Fatalf("译码结果 = %+v", got)
	}
}

// Covers: 两拍各自成封靠事件 ID 里的版本；同一交易同一包裹的第二拍是另一个 ID，两扇门都要处理。
func TestTwoBeatsOfOneParcelAreTwoDeliveries(t *testing.T) {
	consumer, handler := newJudgmentDueFixture(t)
	first := judgmentDueEnvelope(t, "tenant-a/label-transaction/LT-1/parcel-1/3/judgment-due")
	second := judgmentDueEnvelope(t, "tenant-a/label-transaction/LT-1/parcel-1/4/judgment-due")
	second.Payload = json.RawMessage(
		`{"tenantId":"tenant-a","transaction":"LT-1","parcel":"parcel-1","revision":4,"beat":"FOLLOW_UP_APPENDED"}`)

	for _, envelope := range []eventing.Envelope{first, second} {
		if err := consumer.Consume(t.Context(), envelope); err != nil {
			t.Fatalf("投递 %s：%v", envelope.ID, err)
		}
	}
	if len(handler.calls) != 2 {
		t.Fatalf("处理次数 = %d, want 2——两拍各自成一次消费", len(handler.calls))
	}
}

func TestAJudgmentDueEnvelopeMissingAnyKeyDimensionIsPoison(t *testing.T) {
	for name, payload := range map[string]string{
		"缺 tenantId":    `{"transaction":"LT-1","parcel":"parcel-1","revision":3,"beat":"RESULT_RECORDED"}`,
		"缺 transaction": `{"tenantId":"tenant-a","parcel":"parcel-1","revision":3,"beat":"RESULT_RECORDED"}`,
		"缺 parcel":      `{"tenantId":"tenant-a","transaction":"LT-1","revision":3,"beat":"RESULT_RECORDED"}`,
		"不是 JSON":       `not json`,
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newJudgmentDueFixture(t)
			poison := judgmentDueEnvelope(t, "poison-"+name)
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

// Covers: revision 与 beat 不是引用维——缺了照样处理。版本在事件 ID 上区分两拍，拍只作追溯。
func TestAJudgmentDuePayloadWithoutRevisionOrBeatStillDecodes(t *testing.T) {
	consumer, handler := newJudgmentDueFixture(t)
	envelope := judgmentDueEnvelope(t, "tenant-a/label-transaction/LT-1/parcel-1/3/judgment-due")
	envelope.Payload = json.RawMessage(`{"tenantId":"tenant-a","transaction":"LT-1","parcel":"parcel-1"}`)

	if err := consumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("只带三维键的载荷应照常处理：%v", err)
	}
	if len(handler.calls) != 1 {
		t.Fatalf("处理次数 = %d, want 1", len(handler.calls))
	}
}

func TestAFailedJudgmentDueHandlerRollsBackAndTheRedeliveryRetries(t *testing.T) {
	consumer, handler := newJudgmentDueFixture(t)
	envelope := judgmentDueEnvelope(t, "tenant-a/label-transaction/LT-1/parcel-1/3/judgment-due")

	handler.err = errors.New("judgment undecided")
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

func TestForeignTypesAreLoudOnTheJudgmentDueConsumer(t *testing.T) {
	for name, eventType := range map[string]eventing.EventType{
		"终局已形成":   "parcel-shipment.final-outcome.formed",
		"有效交付登记":  psinbox.EffectiveDeliveryRegisteredEventType,
		"资料版本已形成": "parcel-shipment.source-data-version.formed",
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newJudgmentDueFixture(t)
			foreign := judgmentDueEnvelope(t, "foreign-"+name)
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

// Covers: 判断意图消费者与有效交付消费者的 Inbox 键只差消费者名。同一 source 与同一事件 ID 分别投给
// 两扇门，两侧都必须处理一次；共名会让第二扇门把它当重复投递跳过。
func TestTheJudgmentDueConsumerKeepsASeparateInboxAccountFromDelivery(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}

	deliveryHandler := &registeredDeliveryHandlerDouble{}
	deliveryConsumer, err := psinbox.NewEffectiveDeliveryConsumer(db.Transactor(), store, deliveryHandler)
	if err != nil {
		t.Fatalf("构造有效交付消费者：%v", err)
	}
	judgmentHandler := &judgmentDueHandlerDouble{}
	judgmentConsumer, err := psinbox.NewLabelTransactionJudgmentConsumer(db.Transactor(), store, judgmentHandler)
	if err != nil {
		t.Fatalf("构造判断意图消费者：%v", err)
	}

	const shared = "shared-event-id"
	deliveryEnvelope := effectiveDeliveryEnvelope(t, shared)
	judgmentEnvelope := judgmentDueEnvelope(t, shared)
	judgmentEnvelope.Source = deliveryEnvelope.Source

	if err := deliveryConsumer.Consume(t.Context(), deliveryEnvelope); err != nil {
		t.Fatalf("有效交付投递：%v", err)
	}
	if err := judgmentConsumer.Consume(t.Context(), judgmentEnvelope); err != nil {
		t.Fatalf("判断意图投递：%v", err)
	}

	if len(deliveryHandler.calls) != 1 || len(judgmentHandler.calls) != 1 {
		t.Fatalf("两本账应各处理一次：交付 = %d，判断 = %d",
			len(deliveryHandler.calls), len(judgmentHandler.calls))
	}
}
