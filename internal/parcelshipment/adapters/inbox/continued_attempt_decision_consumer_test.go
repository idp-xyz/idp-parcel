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
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"

	"go.idp.xyz/idp-bento-go/eventing"
)

// 本文件对真实 PostgreSQL 16 证关闭 / 重开决定判断意图这一路的消费门与译码（lc/27，ADR-0134 决定一）：
// 只认 continued-attempt-decision.judgment-due，载荷三维（租户 + 包裹 + 决定标识）缺一即毒丸，kind 不进
// 译码——两种决定都触发、分格归 JudgeLabelServiceFinal（票面红线「不在触发处按决定种类挑」）。

type continuedAttemptDueHandlerDouble struct {
	calls []psinbox.ContinuedAttemptDecisionJudgmentDue
	err   error
}

func (double *continuedAttemptDueHandlerDouble) HandleContinuedAttemptDecisionJudgmentDue(
	_ context.Context, due psinbox.ContinuedAttemptDecisionJudgmentDue,
) error {
	if double.err != nil {
		return double.err
	}
	double.calls = append(double.calls, due)
	return nil
}

func newContinuedAttemptDueFixture(t *testing.T) (*psinbox.ContinuedAttemptDecisionJudgmentConsumer, *continuedAttemptDueHandlerDouble) {
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
	handler := &continuedAttemptDueHandlerDouble{}
	consumer, err := psinbox.NewContinuedAttemptDecisionJudgmentConsumer(db.Transactor(), store, handler)
	if err != nil {
		t.Fatalf("构造消费者：%v", err)
	}
	return consumer, handler
}

func continuedAttemptDueEnvelope(t *testing.T, eventID string) eventing.Envelope {
	t.Helper()

	payload, err := json.Marshal(map[string]any{
		"tenantId": "tenant-a",
		"parcel":   "parcel-1",
		"decision": "SYN-CADN-001",
		"kind":     "CONTROLLED_CLOSURE",
	})
	if err != nil {
		t.Fatalf("载荷：%v", err)
	}
	now := time.Date(2026, 9, 11, 10, 0, 0, 0, time.UTC)
	return eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       "idp-parcel/parcel-shipment",
		Type:         psinbox.ContinuedAttemptDecisionJudgmentDueEventType,
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

// Covers: 消费者认的类型串与写侧（lc/30 的 OutboxContinuedAttemptDecisionHandoff）交出的是同一个。
// 消费方按 inbox 包惯例自写这个串、不 import 提供方适配器；两串各改一边这里就红。
func TestTheContinuedAttemptDueConsumerAcceptsTheTypeTheWriterEmits(t *testing.T) {
	if string(psinbox.ContinuedAttemptDecisionJudgmentDueEventType) != pspostgres.ContinuedAttemptDecisionEventType {
		t.Fatalf("消费者认 %q，写侧交 %q", psinbox.ContinuedAttemptDecisionJudgmentDueEventType,
			pspostgres.ContinuedAttemptDecisionEventType)
	}
}

func TestAContinuedAttemptDueEnvelopeIsProcessedExactlyOnce(t *testing.T) {
	consumer, handler := newContinuedAttemptDueFixture(t)
	envelope := continuedAttemptDueEnvelope(t, "tenant-a/continued-attempt-decision/parcel-1/SYN-CADN-001/judgment-due")

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
	if got.TenantID != "tenant-a" || got.Parcel != "parcel-1" || got.Decision != "SYN-CADN-001" {
		t.Fatalf("译码结果 = %+v", got)
	}
}

// Covers: 关过—重开—再关三条决定各自成封靠事件 ID 里的决定标识；同一包裹的重开那一封是另一个 ID，
// 两扇门都要处理——重开也触发，判断自己答 NOT_FINAL（票面「两种决定都触发」）。
func TestAClosureAndItsReopeningAreTwoDeliveries(t *testing.T) {
	consumer, handler := newContinuedAttemptDueFixture(t)
	closure := continuedAttemptDueEnvelope(t, "tenant-a/continued-attempt-decision/parcel-1/SYN-CADN-001/judgment-due")
	reopening := continuedAttemptDueEnvelope(t, "tenant-a/continued-attempt-decision/parcel-1/SYN-CADN-002/judgment-due")
	reopening.Payload = json.RawMessage(
		`{"tenantId":"tenant-a","parcel":"parcel-1","decision":"SYN-CADN-002","kind":"REOPENING"}`)

	for _, envelope := range []eventing.Envelope{closure, reopening} {
		if err := consumer.Consume(t.Context(), envelope); err != nil {
			t.Fatalf("投递 %s：%v", envelope.ID, err)
		}
	}
	if len(handler.calls) != 2 {
		t.Fatalf("处理次数 = %d, want 2——关闭与重开各自成一次消费", len(handler.calls))
	}
	if handler.calls[1].Decision != "SYN-CADN-002" {
		t.Fatalf("第二封译码 = %+v", handler.calls[1])
	}
}

func TestAContinuedAttemptDueEnvelopeMissingAnyKeyDimensionIsPoison(t *testing.T) {
	for name, payload := range map[string]string{
		"缺 tenantId": `{"parcel":"parcel-1","decision":"SYN-CADN-001","kind":"CONTROLLED_CLOSURE"}`,
		"缺 parcel":   `{"tenantId":"tenant-a","decision":"SYN-CADN-001","kind":"CONTROLLED_CLOSURE"}`,
		"缺 decision": `{"tenantId":"tenant-a","parcel":"parcel-1","kind":"CONTROLLED_CLOSURE"}`,
		"不是 JSON":    `not json`,
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newContinuedAttemptDueFixture(t)
			poison := continuedAttemptDueEnvelope(t, "poison-"+name)
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

// Covers: kind 不是引用维——缺了照样处理，带了也不进译码结果。消费者不按决定种类分支。
func TestAContinuedAttemptDuePayloadWithoutKindStillDecodes(t *testing.T) {
	consumer, handler := newContinuedAttemptDueFixture(t)
	envelope := continuedAttemptDueEnvelope(t, "tenant-a/continued-attempt-decision/parcel-1/SYN-CADN-001/judgment-due")
	envelope.Payload = json.RawMessage(`{"tenantId":"tenant-a","parcel":"parcel-1","decision":"SYN-CADN-001"}`)

	if err := consumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("只带三维键的载荷应照常处理：%v", err)
	}
	if len(handler.calls) != 1 {
		t.Fatalf("处理次数 = %d, want 1", len(handler.calls))
	}
}

func TestAFailedContinuedAttemptDueHandlerRollsBackAndTheRedeliveryRetries(t *testing.T) {
	consumer, handler := newContinuedAttemptDueFixture(t)
	envelope := continuedAttemptDueEnvelope(t, "tenant-a/continued-attempt-decision/parcel-1/SYN-CADN-001/judgment-due")

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

// Covers: 认不得的类型响亮报错——含同族的面单交易判断意图：两封都指「判一次终局」，但各走各的门，
// 这扇门吃了那一封就等于替 26 的消费者记了账。
func TestForeignTypesAreLoudOnTheContinuedAttemptDueConsumer(t *testing.T) {
	for name, eventType := range map[string]eventing.EventType{
		"面单交易判断意图": psinbox.LabelTransactionJudgmentDueEventType,
		"终局已形成":    "parcel-shipment.final-outcome.formed",
		"有效交付登记":   psinbox.EffectiveDeliveryRegisteredEventType,
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newContinuedAttemptDueFixture(t)
			foreign := continuedAttemptDueEnvelope(t, "foreign-"+name)
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

// Covers: 本消费者与面单交易判断意图消费者的 Inbox 键只差消费者名。同一 source 与同一事件 ID 分别投给
// 两扇门，两侧都必须处理一次；共名会让第二扇门把它当重复投递跳过。
func TestTheContinuedAttemptDueConsumerKeepsASeparateInboxAccountFromTheTransactionOne(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}

	transactionHandler := &judgmentDueHandlerDouble{}
	transactionConsumer, err := psinbox.NewLabelTransactionJudgmentConsumer(db.Transactor(), store, transactionHandler)
	if err != nil {
		t.Fatalf("构造面单交易判断意图消费者：%v", err)
	}
	decisionHandler := &continuedAttemptDueHandlerDouble{}
	decisionConsumer, err := psinbox.NewContinuedAttemptDecisionJudgmentConsumer(db.Transactor(), store, decisionHandler)
	if err != nil {
		t.Fatalf("构造关闭 / 重开决定判断意图消费者：%v", err)
	}

	const shared = "shared-event-id"
	transactionEnvelope := judgmentDueEnvelope(t, shared)
	decisionEnvelope := continuedAttemptDueEnvelope(t, shared)
	decisionEnvelope.Source = transactionEnvelope.Source

	if err := transactionConsumer.Consume(t.Context(), transactionEnvelope); err != nil {
		t.Fatalf("面单交易判断意图投递：%v", err)
	}
	if err := decisionConsumer.Consume(t.Context(), decisionEnvelope); err != nil {
		t.Fatalf("关闭 / 重开决定判断意图投递：%v", err)
	}

	if len(transactionHandler.calls) != 1 || len(decisionHandler.calls) != 1 {
		t.Fatalf("两本账应各处理一次：交易 = %d，决定 = %d",
			len(transactionHandler.calls), len(decisionHandler.calls))
	}
}

func TestTheContinuedAttemptDueConsumerRefusesNilDependencies(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	if _, err := psinbox.NewContinuedAttemptDecisionJudgmentConsumer(nil, store, &continuedAttemptDueHandlerDouble{}); err == nil {
		t.Fatal("nil transactor 被接受了")
	}
	if _, err := psinbox.NewContinuedAttemptDecisionJudgmentConsumer(db.Transactor(), nil, &continuedAttemptDueHandlerDouble{}); err == nil {
		t.Fatal("nil inbox store 被接受了")
	}
	if _, err := psinbox.NewContinuedAttemptDecisionJudgmentConsumer(db.Transactor(), store, nil); err == nil {
		t.Fatal("nil handler 被接受了")
	}
}
