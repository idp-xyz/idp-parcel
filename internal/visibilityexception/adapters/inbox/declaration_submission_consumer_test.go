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

// 本文件对真实 PostgreSQL 证 VE 申报提交消费门：三维键加版本维译码、恰一次、毒丸、
// 与同源案件账本分家。载荷只带引用，成员快照由处理方按键重读（ADR-0066 消费侧循环
// 拆分）；版本身份的权威来源是载荷 versionId——信封 ID 今天不含版本维，已立案挂账。

type veFormedSubmissionHandlerDouble struct {
	calls []veinbox.FormedDeclarationSubmission
	err   error
}

func (double *veFormedSubmissionHandlerDouble) HandleFormedDeclarationSubmission(
	_ context.Context, formed veinbox.FormedDeclarationSubmission,
) error {
	if double.err != nil {
		return double.err
	}
	double.calls = append(double.calls, formed)
	return nil
}

func newVEDeclarationSubmissionFixture(t *testing.T) (*veinbox.DeclarationSubmissionConsumer, *veFormedSubmissionHandlerDouble) {
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
	handler := &veFormedSubmissionHandlerDouble{}
	consumer, err := veinbox.NewDeclarationSubmissionConsumer(db.Transactor(), store, handler)
	if err != nil {
		t.Fatalf("构造消费者：%v", err)
	}
	return consumer, handler
}

func veDeclarationSubmissionEnvelope(t *testing.T, eventID string) eventing.Envelope {
	t.Helper()

	payload, err := json.Marshal(map[string]string{
		"tenantId":  "tenant-a",
		"unitId":    "unit-1",
		"procedure": "US-IMPORT/TYPE-86",
		"versionId": "version-1",
	})
	if err != nil {
		t.Fatalf("载荷：%v", err)
	}
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	return eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       "idp-parcel/customs-compliance",
		Type:         veinbox.DeclarationSubmissionFormedEventType,
		Version:      1,
		Scope:        "tenant-a",
		Subject:      "version-1",
		PartitionKey: eventID,
		OccurredAt:   now,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}
}

func TestAFormedDeclarationSubmissionIsProcessedExactlyOnce(t *testing.T) {
	consumer, handler := newVEDeclarationSubmissionFixture(t)
	envelope := veDeclarationSubmissionEnvelope(t, "tenant-a/unit-1/US-IMPORT/TYPE-86")

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
	if got.TenantID != "tenant-a" || got.UnitID != "unit-1" ||
		got.Procedure != "US-IMPORT/TYPE-86" || got.VersionID != "version-1" {
		t.Fatalf("译码结果 = %+v；三维键与版本维须原样到达处理方", got)
	}
}

func TestADeclarationSubmissionEnvelopeMissingAnyKeyDimensionIsPoison(t *testing.T) {
	for name, payload := range map[string]string{
		"缺 tenantId":  `{"unitId":"unit-1","procedure":"US-IMPORT/TYPE-86","versionId":"version-1"}`,
		"缺 unitId":    `{"tenantId":"tenant-a","procedure":"US-IMPORT/TYPE-86","versionId":"version-1"}`,
		"缺 procedure": `{"tenantId":"tenant-a","unitId":"unit-1","versionId":"version-1"}`,
		"缺 versionId": `{"tenantId":"tenant-a","unitId":"unit-1","procedure":"US-IMPORT/TYPE-86"}`,
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newVEDeclarationSubmissionFixture(t)
			poison := veDeclarationSubmissionEnvelope(t, "poison-"+name)
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

func TestAFailedDeclarationSubmissionHandlerRollsBackAndTheRedeliveryRetries(t *testing.T) {
	consumer, handler := newVEDeclarationSubmissionFixture(t)
	envelope := veDeclarationSubmissionEnvelope(t, "tenant-a/unit-1/US-IMPORT/TYPE-86")

	handler.err = errors.New("declaration submission is not yet visible")
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

func TestForeignTypesAreLoudOnTheDeclarationSubmissionConsumer(t *testing.T) {
	for name, eventType := range map[string]eventing.EventType{
		"同上下文案件建立口": veinbox.CustomsCaseEstablishedEventType,
		"替代旅程启动":    veinbox.ExceptionJourneyRecordedEventType,
		"权威交接登记":    veinbox.TransportHandoverRegisteredEventType,
		"节点收寄形成":    veinbox.NodeIntakeFormedEventType,
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newVEDeclarationSubmissionFixture(t)
			foreign := veDeclarationSubmissionEnvelope(t, "foreign-"+name)
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

func TestDeclarationSubmissionInboxLedgersAreSeparateFromCustomsCase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}

	submissionHandler := &veFormedSubmissionHandlerDouble{}
	submissionConsumer, err := veinbox.NewDeclarationSubmissionConsumer(db.Transactor(), store, submissionHandler)
	if err != nil {
		t.Fatalf("构造提交消费者：%v", err)
	}
	caseHandler := &veEstablishedCaseHandlerDouble{}
	caseConsumer, err := veinbox.NewCustomsCaseConsumer(db.Transactor(), store, caseHandler)
	if err != nil {
		t.Fatalf("构造案件消费者：%v", err)
	}

	// 两封信同源（都是 customs-compliance）同事件 ID，两本账只差消费者名——inbox 键
	// 由（消费者名+来源+事件 ID）认领。
	const shared = "shared-event-id"
	submissionEnvelope := veDeclarationSubmissionEnvelope(t, shared)
	caseEnvelope := veCustomsCaseEnvelope(t, shared)

	if err := submissionConsumer.Consume(t.Context(), submissionEnvelope); err != nil {
		t.Fatalf("提交投递：%v", err)
	}
	if err := caseConsumer.Consume(t.Context(), caseEnvelope); err != nil {
		t.Fatalf("案件投递：%v", err)
	}
	if len(submissionHandler.calls) != 1 || len(caseHandler.calls) != 1 {
		t.Fatalf("两本账应各处理一次：提交 = %d，案件 = %d",
			len(submissionHandler.calls), len(caseHandler.calls))
	}
}
