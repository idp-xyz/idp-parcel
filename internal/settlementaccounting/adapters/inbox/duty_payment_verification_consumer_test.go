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

// 本文件对真实 PostgreSQL 16 证付款核对形成信封这一路的消费门与译码（票 sa-cc/09）：只认
// customs-compliance.duty-payment-verification.formed，载荷五维（tenantId / scope / duty / funds / digest）
// 缺一即毒丸，处理失败回滚重投。

type formedVerificationHandlerDouble struct {
	calls []sainbox.FormedDutyPaymentVerification
	err   error
}

func (double *formedVerificationHandlerDouble) HandleFormedDutyPaymentVerification(
	_ context.Context, formed sainbox.FormedDutyPaymentVerification,
) error {
	if double.err != nil {
		return double.err
	}
	double.calls = append(double.calls, formed)
	return nil
}

func newVerificationFixture(t *testing.T) (*sainbox.DutyPaymentVerificationConsumer, *formedVerificationHandlerDouble) {
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
	handler := &formedVerificationHandlerDouble{}
	consumer, err := sainbox.NewDutyPaymentVerificationConsumer(db.Transactor(), store, handler)
	if err != nil {
		t.Fatalf("构造消费者：%v", err)
	}
	return consumer, handler
}

func verificationEnvelope(t *testing.T, eventID string) eventing.Envelope {
	t.Helper()
	payload, err := json.Marshal(map[string]string{
		"tenantId": "tenant-a",
		"scope":    "SYN-UNIT-01",
		"duty":     "duty-1",
		"funds":    "bank-fact-1",
		"digest":   "digest-1",
	})
	if err != nil {
		t.Fatalf("载荷：%v", err)
	}
	now := time.Date(2026, 9, 11, 9, 0, 0, 0, time.UTC)
	return eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       "idp-parcel/customs-compliance",
		Type:         sainbox.DutyPaymentVerificationFormedEventType,
		Version:      1,
		Scope:        "tenant-a",
		Subject:      "SYN-UNIT-01/duty-1",
		PartitionKey: "tenant-a/duty-payment-verification/SYN-UNIT-01",
		OccurredAt:   now,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}
}

func TestAFormedVerificationEnvelopeIsProcessedExactlyOnce(t *testing.T) {
	consumer, handler := newVerificationFixture(t)
	envelope := verificationEnvelope(t, "tenant-a/duty-payment-verification/SYN-UNIT-01/duty-1/bank-fact-1/digest-1")

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
	if got.TenantID != "tenant-a" || got.Scope != "SYN-UNIT-01" || got.Duty != "duty-1" ||
		got.Funds != "bank-fact-1" || got.Digest != "digest-1" {
		t.Fatalf("译码结果 = %+v", got)
	}
}

func TestAFormedVerificationEnvelopeMissingAnyKeyDimensionIsPoison(t *testing.T) {
	for name, payload := range map[string]string{
		"缺 tenantId": `{"scope":"SYN-UNIT-01","duty":"duty-1","funds":"bank-fact-1","digest":"digest-1"}`,
		"缺 scope":    `{"tenantId":"tenant-a","duty":"duty-1","funds":"bank-fact-1","digest":"digest-1"}`,
		"缺 duty":     `{"tenantId":"tenant-a","scope":"SYN-UNIT-01","funds":"bank-fact-1","digest":"digest-1"}`,
		"缺 funds":    `{"tenantId":"tenant-a","scope":"SYN-UNIT-01","duty":"duty-1","digest":"digest-1"}`,
		"缺 digest":   `{"tenantId":"tenant-a","scope":"SYN-UNIT-01","duty":"duty-1","funds":"bank-fact-1"}`,
		"不是 JSON":    `{`,
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newVerificationFixture(t)
			poison := verificationEnvelope(t, "poison-"+name)
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

func TestAFailedVerificationHandlerRollsBackAndTheRedeliveryRetries(t *testing.T) {
	consumer, handler := newVerificationFixture(t)
	envelope := verificationEnvelope(t, "tenant-a/duty-payment-verification/SYN-UNIT-01/duty-1/bank-fact-1/digest-1")

	handler.err = errors.New("verification is not yet visible")
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

func TestForeignTypesAreLoudOnTheVerificationConsumer(t *testing.T) {
	for name, eventType := range map[string]eventing.EventType{
		"资金事实采用": "settlement-accounting.external-funds-fact.adopted",
		"案件关闭":   "customs-compliance.case-closure.decided",
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newVerificationFixture(t)
			foreign := verificationEnvelope(t, "foreign-"+name)
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

func TestTheVerificationConsumerRefusesNilDependencies(t *testing.T) {
	if _, err := sainbox.NewDutyPaymentVerificationConsumer(nil, nil, nil); err == nil {
		t.Fatal("nil 依赖被收下了")
	}
}
