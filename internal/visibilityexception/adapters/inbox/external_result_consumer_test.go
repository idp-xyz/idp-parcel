package veinbox_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"

	"go.idp.xyz/idp-bento-go/eventing"
)

// 本文件对真实 PostgreSQL 证 VE 外部结果消费门：接收幂等键两维译码、恰一次、毒丸、
// 处理失败回滚重投、与同源申报提交账本分家。载荷只带键，层次、范围与成员由处理方
// 按键重读（ADR-0066 消费侧逐成员拆分）。

type veReceivedResultHandlerDouble struct {
	calls []veinbox.ReceivedExternalResult
	err   error
}

func (double *veReceivedResultHandlerDouble) HandleReceivedExternalResult(
	_ context.Context, received veinbox.ReceivedExternalResult,
) error {
	if double.err != nil {
		return double.err
	}
	double.calls = append(double.calls, received)
	return nil
}

func newVEExternalResultFixture(t *testing.T) (*veinbox.ExternalResultConsumer, *veReceivedResultHandlerDouble) {
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
	handler := &veReceivedResultHandlerDouble{}
	consumer, err := veinbox.NewExternalResultConsumer(db.Transactor(), store, handler)
	if err != nil {
		t.Fatalf("构造消费者：%v", err)
	}
	return consumer, handler
}

func veExternalResultEnvelope(t *testing.T, eventID string) eventing.Envelope {
	t.Helper()

	payload, err := json.Marshal(map[string]string{
		"tenantId": "tenant-a",
		"sourceId": "result-1",
	})
	if err != nil {
		t.Fatalf("载荷：%v", err)
	}
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	return eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       "idp-parcel/customs-compliance",
		Type:         veinbox.ExternalResultReceivedEventType,
		Version:      1,
		Scope:        "tenant-a",
		Subject:      "result-1",
		PartitionKey: "tenant-a/result-1",
		OccurredAt:   now,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}
}

func TestAReceivedExternalResultIsProcessedExactlyOnce(t *testing.T) {
	consumer, handler := newVEExternalResultFixture(t)
	envelope := veExternalResultEnvelope(t, "external-result/one")

	if err := consumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("首投：%v", err)
	}
	if err := consumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("重复投递：%v", err)
	}
	if len(handler.calls) != 1 {
		t.Fatalf("处理次数 = %d, want 1", len(handler.calls))
	}
	if got := handler.calls[0]; got.TenantID != "tenant-a" || got.SourceID != "result-1" {
		t.Fatalf("译码结果 = %+v；接收幂等键两维须原样到达处理方", got)
	}
}

func TestAnExternalResultEnvelopeMissingAKeyDimensionIsPoison(t *testing.T) {
	for name, payload := range map[string]string{
		"缺 tenantId": `{"sourceId":"result-1"}`,
		"缺 sourceId": `{"tenantId":"tenant-a"}`,
		"不是 JSON":    `not-json`,
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newVEExternalResultFixture(t)
			poison := veExternalResultEnvelope(t, "poison-"+name)
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

func TestAFailedExternalResultHandlerRollsBackAndTheRedeliveryRetries(t *testing.T) {
	consumer, handler := newVEExternalResultFixture(t)
	envelope := veExternalResultEnvelope(t, "external-result/retry")

	handler.err = errors.New("declaration unit is not yet visible")
	if err := consumer.Consume(t.Context(), envelope); err == nil {
		t.Fatal("处理失败必须让消费报错——业务缺件不落毒丸账")
	}

	handler.err = nil
	if err := consumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("重投：%v", err)
	}
	if len(handler.calls) != 1 {
		t.Fatalf("处理次数 = %d, want 1", len(handler.calls))
	}
}

func TestForeignTypesAreLoudOnTheExternalResultConsumer(t *testing.T) {
	for name, eventType := range map[string]eventing.EventType{
		"同上下文申报提交口": veinbox.DeclarationSubmissionFormedEventType,
		"同上下文案件建立口": veinbox.CustomsCaseEstablishedEventType,
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newVEExternalResultFixture(t)
			foreign := veExternalResultEnvelope(t, "foreign-"+name)
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

func TestExternalResultInboxLedgerIsSeparateFromDeclarationSubmission(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}

	resultHandler := &veReceivedResultHandlerDouble{}
	resultConsumer, err := veinbox.NewExternalResultConsumer(db.Transactor(), store, resultHandler)
	if err != nil {
		t.Fatalf("构造外部结果消费者：%v", err)
	}
	submissionHandler := &veFormedSubmissionHandlerDouble{}
	submissionConsumer, err := veinbox.NewDeclarationSubmissionConsumer(db.Transactor(), store, submissionHandler)
	if err != nil {
		t.Fatalf("构造提交消费者：%v", err)
	}

	// 两封信同源同事件 ID，两本账只差消费者名——inbox 键由（消费者名+来源+事件 ID）认领。
	const shared = "shared-event-id"
	if err := resultConsumer.Consume(t.Context(), veExternalResultEnvelope(t, shared)); err != nil {
		t.Fatalf("外部结果投递：%v", err)
	}
	if err := submissionConsumer.Consume(t.Context(), veDeclarationSubmissionEnvelope(t, shared)); err != nil {
		t.Fatalf("提交投递：%v", err)
	}
	if len(resultHandler.calls) != 1 || len(submissionHandler.calls) != 1 {
		t.Fatalf("两本账应各处理一次：外部结果 = %d，提交 = %d",
			len(resultHandler.calls), len(submissionHandler.calls))
	}
}

func TestTheExternalResultConsumerRefusesMissingCollaborators(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	handler := &veReceivedResultHandlerDouble{}
	if _, err := veinbox.NewExternalResultConsumer(nil, store, handler); err == nil {
		t.Fatal("缺事务器必须拒绝构造")
	}
	if _, err := veinbox.NewExternalResultConsumer(db.Transactor(), nil, handler); err == nil {
		t.Fatal("缺 inbox 库必须拒绝构造")
	}
	if _, err := veinbox.NewExternalResultConsumer(db.Transactor(), store, nil); err == nil {
		t.Fatal("缺处理方必须拒绝构造")
	}
}
