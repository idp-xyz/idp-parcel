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

// 本文件对真实 PostgreSQL 证 VE 投影派生消费门：三维引用译码、恰一次、毒丸、与其余
// 消费账分家。载荷只带（租户+包裹+投影版本）引用，投影本体由处理方按键读回当前版。

type veDerivedProjectionHandlerDouble struct {
	calls []veinbox.DerivedTrackingProjection
	err   error
}

func (double *veDerivedProjectionHandlerDouble) HandleDerivedTrackingProjection(
	_ context.Context, derived veinbox.DerivedTrackingProjection,
) error {
	if double.err != nil {
		return double.err
	}
	double.calls = append(double.calls, derived)
	return nil
}

func newVETrackingProjectionFixture(t *testing.T) (*veinbox.TrackingProjectionConsumer, *veDerivedProjectionHandlerDouble) {
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
	handler := &veDerivedProjectionHandlerDouble{}
	consumer, err := veinbox.NewTrackingProjectionConsumer(db.Transactor(), store, handler)
	if err != nil {
		t.Fatalf("构造消费者：%v", err)
	}
	return consumer, handler
}

func veTrackingProjectionEnvelope(t *testing.T, eventID string) eventing.Envelope {
	t.Helper()

	payload, err := json.Marshal(map[string]string{
		"tenantId":  "tenant-a",
		"parcel":    "parcel-1",
		"versionId": "projection-1",
	})
	if err != nil {
		t.Fatalf("载荷：%v", err)
	}
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	return eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       "idp-parcel/visibility-exception",
		Type:         veinbox.TrackingProjectionDerivedEventType,
		Version:      1,
		Scope:        "tenant-a",
		Subject:      "projection-1",
		PartitionKey: "tenant-a/parcel-1",
		OccurredAt:   now,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}
}

func TestADerivedProjectionIsProcessedExactlyOnce(t *testing.T) {
	consumer, handler := newVETrackingProjectionFixture(t)
	envelope := veTrackingProjectionEnvelope(t, "projection-1")

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
	if got.TenantID != "tenant-a" || got.Parcel != "parcel-1" || got.VersionID != "projection-1" {
		t.Fatalf("译码结果 = %+v；三维引用须原样到达处理方", got)
	}
}

func TestATrackingProjectionEnvelopeMissingAnyDimensionIsPoison(t *testing.T) {
	for name, payload := range map[string]string{
		"缺 tenantId":  `{"parcel":"parcel-1","versionId":"projection-1"}`,
		"缺 parcel":    `{"tenantId":"tenant-a","versionId":"projection-1"}`,
		"缺 versionId": `{"tenantId":"tenant-a","parcel":"parcel-1"}`,
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newVETrackingProjectionFixture(t)
			poison := veTrackingProjectionEnvelope(t, "poison-"+name)
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

func TestAFailedDerivedProjectionHandlerRollsBackAndTheRedeliveryRetries(t *testing.T) {
	consumer, handler := newVETrackingProjectionFixture(t)
	envelope := veTrackingProjectionEnvelope(t, "projection-1")

	handler.err = errors.New("customer account lookup is unavailable")
	if err := consumer.Consume(t.Context(), envelope); err == nil {
		t.Fatal("处理失败必须让消费报错——依赖缺件不落毒丸账")
	}

	handler.err = nil
	if err := consumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("重投：%v", err)
	}
	if len(handler.calls) != 1 {
		t.Fatalf("处理次数 = %d, want 1", len(handler.calls))
	}
}

func TestForeignTypesAreLoudOnTheTrackingProjectionConsumer(t *testing.T) {
	for name, eventType := range map[string]eventing.EventType{
		"同上下文视图发布口": "visibility-exception.customer-view.published",
		"关务案件建立":    veinbox.CustomsCaseEstablishedEventType,
		"替代旅程启动":    veinbox.ExceptionJourneyRecordedEventType,
		"权威交接登记":    veinbox.TransportHandoverRegisteredEventType,
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newVETrackingProjectionFixture(t)
			foreign := veTrackingProjectionEnvelope(t, "foreign-"+name)
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

func TestTrackingProjectionInboxLedgersAreSeparateFromExceptionJourney(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}

	projectionHandler := &veDerivedProjectionHandlerDouble{}
	projectionConsumer, err := veinbox.NewTrackingProjectionConsumer(db.Transactor(), store, projectionHandler)
	if err != nil {
		t.Fatalf("构造投影消费者：%v", err)
	}
	journeyHandler := &veRecordedJourneyHandlerDouble{}
	journeyConsumer, err := veinbox.NewExceptionJourneyConsumer(db.Transactor(), store, journeyHandler)
	if err != nil {
		t.Fatalf("构造旅程消费者：%v", err)
	}

	// 对齐来源，让两本账只差消费者名——inbox 键由（消费者名+来源+事件 ID）认领。
	const shared = "shared-event-id"
	projectionEnvelope := veTrackingProjectionEnvelope(t, shared)
	journeyEnvelope := veExceptionJourneyEnvelope(t, shared)
	journeyEnvelope.Source = projectionEnvelope.Source

	if err := projectionConsumer.Consume(t.Context(), projectionEnvelope); err != nil {
		t.Fatalf("投影投递：%v", err)
	}
	if err := journeyConsumer.Consume(t.Context(), journeyEnvelope); err != nil {
		t.Fatalf("旅程投递：%v", err)
	}
	if len(projectionHandler.calls) != 1 || len(journeyHandler.calls) != 1 {
		t.Fatalf("两本账应各处理一次：投影 = %d，旅程 = %d",
			len(projectionHandler.calls), len(journeyHandler.calls))
	}
}
