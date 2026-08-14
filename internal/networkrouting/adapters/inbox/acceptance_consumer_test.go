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

// 本文件对真实 PostgreSQL 16 证消费门的事务语义——投递侧（outbox 样板）之后，
// 消费侧的四条：恰一次处理、重复投递幂等跳过、处理失败回滚可重投、毒丸显式拒收。

type decisionHandlerDouble struct {
	calls []nrinbox.AcceptedDecision
	err   error
}

func (double *decisionHandlerDouble) HandleAcceptedDecision(
	_ context.Context,
	decision nrinbox.AcceptedDecision,
) error {
	if double.err != nil {
		return double.err
	}
	double.calls = append(double.calls, decision)
	return nil
}

func newConsumerFixture(t *testing.T) (*nrinbox.AcceptanceConsumer, *decisionHandlerDouble) {
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
	handler := &decisionHandlerDouble{}
	consumer, err := nrinbox.NewAcceptanceConsumer(db.Transactor(), store, handler)
	if err != nil {
		t.Fatalf("构造消费者：%v", err)
	}
	return consumer, handler
}

func decisionEnvelope(t *testing.T, eventID string) eventing.Envelope {
	t.Helper()

	payload, err := json.Marshal(map[string]string{
		"tenantId":          "tenant-a",
		"customerAccountId": "customer-a",
		"source":            "portal",
		"sourceRequestKey":  "source-key-1",
		"shipmentRequestId": "request-1",
		"submissionVersion": "submission-v1",
		"decisionId":        eventID,
		"state":             "ACCEPTED",
	})
	if err != nil {
		t.Fatalf("载荷：%v", err)
	}
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	return eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       "idp-parcel/parcel-shipment",
		Type:         "parcel-shipment.acceptance-decision.formed",
		Version:      1,
		Scope:        "tenant-a",
		Subject:      "request-1",
		PartitionKey: "tenant-a/request-1",
		OccurredAt:   now,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}
}

// TestADeliveryIsProcessedExactlyOnce 证恰一次处理：首投处理并入账，重复投递幂等
// 跳过——处理方不会被调第二次。
func TestADeliveryIsProcessedExactlyOnce(t *testing.T) {
	consumer, handler := newConsumerFixture(t)
	ctx := t.Context()
	envelope := decisionEnvelope(t, "decision-1")

	if err := consumer.Consume(ctx, envelope); err != nil {
		t.Fatalf("首投消费：%v", err)
	}
	if err := consumer.Consume(ctx, envelope); err != nil {
		t.Fatalf("重复投递：%v", err)
	}

	if len(handler.calls) != 1 {
		t.Fatalf("处理次数 = %d, want 1", len(handler.calls))
	}
	if handler.calls[0].DecisionID != "decision-1" || handler.calls[0].TenantID != "tenant-a" {
		t.Fatalf("译码结果 = %+v", handler.calls[0])
	}
	// 来源身份要一路带到处理方：没有它就取不回接受基线，而路由的业务输入是基线本体。
	if handler.calls[0].Source != "portal" || handler.calls[0].SourceRequestKey != "source-key-1" {
		t.Fatalf("来源身份没有译到处理方：%+v", handler.calls[0])
	}
}

// TestAnEnvelopeMissingItsSourceIdentityIsPoison 证缺来源身份即毒丸：处理方按它取
// 接受基线，取不着就永远路由不出东西——重投同样内容不会长出字段来。
func TestAnEnvelopeMissingItsSourceIdentityIsPoison(t *testing.T) {
	consumer, handler := newConsumerFixture(t)
	ctx := t.Context()

	poison := decisionEnvelope(t, "decision-no-source")
	poison.Payload = json.RawMessage(`{"tenantId":"tenant-a","customerAccountId":"customer-a",` +
		`"shipmentRequestId":"request-1","decisionId":"decision-no-source","state":"ACCEPTED"}`)

	if err := consumer.Consume(ctx, poison); err != nil {
		t.Fatalf("毒丸首投应拒收入账而不是报错：%v", err)
	}
	if len(handler.calls) != 0 {
		t.Fatalf("处理次数 = %d, want 0——取不回基线的信封不该到达处理方", len(handler.calls))
	}
}

// TestAFailedHandlerRollsBackAndTheRedeliveryRetries 证处理失败整体回滚：inbox
// 无痕，重投可以再试并成功——失败不吃掉投递。
func TestAFailedHandlerRollsBackAndTheRedeliveryRetries(t *testing.T) {
	consumer, handler := newConsumerFixture(t)
	ctx := t.Context()
	envelope := decisionEnvelope(t, "decision-1")

	handler.err = errors.New("downstream unavailable")
	if err := consumer.Consume(ctx, envelope); err == nil {
		t.Fatal("处理失败必须让消费报错——静默吞掉等于丢投递")
	}

	handler.err = nil
	if err := consumer.Consume(ctx, envelope); err != nil {
		t.Fatalf("重投消费：%v", err)
	}
	if len(handler.calls) != 1 {
		t.Fatalf("处理次数 = %d, want 1——失败那次不算处理", len(handler.calls))
	}
}

// TestAPoisonEnvelopeIsRejectedOnceAndStaysRejected 证毒丸显式拒收：拒收也是账，
// 重投不再处理也不再报错——不落账的拒收会让同一份毒丸永远重投。
func TestAPoisonEnvelopeIsRejectedOnceAndStaysRejected(t *testing.T) {
	consumer, handler := newConsumerFixture(t)
	ctx := t.Context()

	poison := decisionEnvelope(t, "decision-poison")
	poison.Payload = json.RawMessage(`{"tenantId":""}`)

	if err := consumer.Consume(ctx, poison); err != nil {
		t.Fatalf("毒丸首投应拒收入账而不是报错：%v", err)
	}
	if err := consumer.Consume(ctx, poison); err != nil {
		t.Fatalf("毒丸重投：%v", err)
	}
	if len(handler.calls) != 0 {
		t.Fatalf("处理次数 = %d, want 0——毒丸不该到达处理方", len(handler.calls))
	}
}

// TestAForeignEventTypeIsLoudNotRecorded 证认不得的类型响亮报错不入账——订阅面配置
// 宽了是装配问题，拒收会把别人的事件记进自己的账。
func TestAForeignEventTypeIsLoudNotRecorded(t *testing.T) {
	consumer, handler := newConsumerFixture(t)
	ctx := t.Context()

	foreign := decisionEnvelope(t, "decision-foreign")
	foreign.Type = "parcel-shipment.source-data-version.formed"

	if err := consumer.Consume(ctx, foreign); err == nil {
		t.Fatal("认不得的类型必须响亮报错")
	}
	if len(handler.calls) != 0 {
		t.Fatalf("处理次数 = %d, want 0", len(handler.calls))
	}
}
