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

// 本文件对真实 PostgreSQL 证 VE 关务案件消费门：五维键译码、恰一次、毒丸、与旅程
// 账本分家。载荷只带案件身份键，成员关联由处理方按键重读（ADR-0066 消费侧循环拆分）。

type veEstablishedCaseHandlerDouble struct {
	calls []veinbox.EstablishedCustomsCase
	err   error
}

func (double *veEstablishedCaseHandlerDouble) HandleEstablishedCustomsCase(
	_ context.Context, established veinbox.EstablishedCustomsCase,
) error {
	if double.err != nil {
		return double.err
	}
	double.calls = append(double.calls, established)
	return nil
}

func newVECustomsCaseFixture(t *testing.T) (*veinbox.CustomsCaseConsumer, *veEstablishedCaseHandlerDouble) {
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
	handler := &veEstablishedCaseHandlerDouble{}
	consumer, err := veinbox.NewCustomsCaseConsumer(db.Transactor(), store, handler)
	if err != nil {
		t.Fatalf("构造消费者：%v", err)
	}
	return consumer, handler
}

func veCustomsCaseEnvelope(t *testing.T, eventID string) eventing.Envelope {
	t.Helper()

	payload, err := json.Marshal(map[string]string{
		"tenantId":     "tenant-a",
		"jurisdiction": "US-CBP",
		"direction":    "IMPORT",
		"procedure":    "US-IMPORT/TYPE-86",
		"obligation":   "obligation-1",
	})
	if err != nil {
		t.Fatalf("载荷：%v", err)
	}
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	return eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       "idp-parcel/customs-compliance",
		Type:         veinbox.CustomsCaseEstablishedEventType,
		Version:      1,
		Scope:        "tenant-a",
		Subject:      "case-1",
		PartitionKey: eventID,
		OccurredAt:   now,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}
}

func TestAnEstablishedCustomsCaseIsProcessedExactlyOnce(t *testing.T) {
	consumer, handler := newVECustomsCaseFixture(t)
	envelope := veCustomsCaseEnvelope(t, "tenant-a/US-CBP/IMPORT/US-IMPORT/TYPE-86/obligation-1")

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
	if got.TenantID != "tenant-a" || got.Jurisdiction != "US-CBP" ||
		got.Direction != "IMPORT" || got.Procedure != "US-IMPORT/TYPE-86" ||
		got.Obligation != "obligation-1" {
		t.Fatalf("译码结果 = %+v；五维键须原样到达处理方", got)
	}
}

func TestACustomsCaseEnvelopeMissingAnyKeyDimensionIsPoison(t *testing.T) {
	for name, payload := range map[string]string{
		"缺 tenantId":     `{"jurisdiction":"US-CBP","direction":"IMPORT","procedure":"US-IMPORT/TYPE-86","obligation":"obligation-1"}`,
		"缺 jurisdiction": `{"tenantId":"tenant-a","direction":"IMPORT","procedure":"US-IMPORT/TYPE-86","obligation":"obligation-1"}`,
		"缺 direction":    `{"tenantId":"tenant-a","jurisdiction":"US-CBP","procedure":"US-IMPORT/TYPE-86","obligation":"obligation-1"}`,
		"缺 procedure":    `{"tenantId":"tenant-a","jurisdiction":"US-CBP","direction":"IMPORT","obligation":"obligation-1"}`,
		"缺 obligation":   `{"tenantId":"tenant-a","jurisdiction":"US-CBP","direction":"IMPORT","procedure":"US-IMPORT/TYPE-86"}`,
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newVECustomsCaseFixture(t)
			poison := veCustomsCaseEnvelope(t, "poison-"+name)
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

func TestAFailedCustomsCaseHandlerRollsBackAndTheRedeliveryRetries(t *testing.T) {
	consumer, handler := newVECustomsCaseFixture(t)
	envelope := veCustomsCaseEnvelope(t, "tenant-a/US-CBP/IMPORT/US-IMPORT/TYPE-86/obligation-1")

	handler.err = errors.New("customs case is not yet visible")
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

func TestForeignTypesAreLoudOnTheCustomsCaseConsumer(t *testing.T) {
	for name, eventType := range map[string]eventing.EventType{
		"同上下文申报提交口": "customs-compliance.declaration-submission.formed",
		"替代旅程启动":    veinbox.ExceptionJourneyRecordedEventType,
		"权威交接登记":    veinbox.TransportHandoverRegisteredEventType,
		"节点收寄形成":    veinbox.NodeIntakeFormedEventType,
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newVECustomsCaseFixture(t)
			foreign := veCustomsCaseEnvelope(t, "foreign-"+name)
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

func TestCustomsCaseInboxLedgersAreSeparateFromExceptionJourney(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}

	caseHandler := &veEstablishedCaseHandlerDouble{}
	caseConsumer, err := veinbox.NewCustomsCaseConsumer(db.Transactor(), store, caseHandler)
	if err != nil {
		t.Fatalf("构造案件消费者：%v", err)
	}
	journeyHandler := &veRecordedJourneyHandlerDouble{}
	journeyConsumer, err := veinbox.NewExceptionJourneyConsumer(db.Transactor(), store, journeyHandler)
	if err != nil {
		t.Fatalf("构造旅程消费者：%v", err)
	}

	const shared = "shared-event-id"
	caseEnvelope := veCustomsCaseEnvelope(t, shared)
	journeyEnvelope := veExceptionJourneyEnvelope(t, shared)
	// 对齐来源，让两本账只差消费者名——inbox 键由（消费者名+来源+事件 ID）认领。
	journeyEnvelope.Source = caseEnvelope.Source

	if err := caseConsumer.Consume(t.Context(), caseEnvelope); err != nil {
		t.Fatalf("案件投递：%v", err)
	}
	if err := journeyConsumer.Consume(t.Context(), journeyEnvelope); err != nil {
		t.Fatalf("旅程投递：%v", err)
	}
	if len(caseHandler.calls) != 1 || len(journeyHandler.calls) != 1 {
		t.Fatalf("两本账应各处理一次：案件 = %d，旅程 = %d",
			len(caseHandler.calls), len(journeyHandler.calls))
	}
}
