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

// 本文件对真实 PostgreSQL 证 VE 外部承运轨迹事实消费门：译码、恰一次、毒丸、处理失败回滚重投、
// 与交付/交接两路分账。三维（租户+事实+版本）缺一即毒丸。

type veJudgedTrackingHandlerDouble struct {
	calls []veinbox.JudgedExternalTracking
	err   error
}

func (double *veJudgedTrackingHandlerDouble) HandleJudgedExternalTracking(
	_ context.Context, judged veinbox.JudgedExternalTracking,
) error {
	if double.err != nil {
		return double.err
	}
	double.calls = append(double.calls, judged)
	return nil
}

func newVEExternalTrackingFixture(t *testing.T) (*veinbox.ExternalTrackingConsumer, *veJudgedTrackingHandlerDouble) {
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
	handler := &veJudgedTrackingHandlerDouble{}
	consumer, err := veinbox.NewExternalTrackingConsumer(db.Transactor(), store, handler)
	if err != nil {
		t.Fatalf("构造消费者：%v", err)
	}
	return consumer, handler
}

func veExternalTrackingEnvelope(t *testing.T, eventID string) eventing.Envelope {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"tenantId": "tenant-a", "fact": "EXTF-1", "version": "EXTV-2"})
	if err != nil {
		t.Fatalf("载荷：%v", err)
	}
	now := time.Date(2026, 9, 3, 9, 0, 0, 0, time.UTC)
	return eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       "idp-parcel/transport-fulfillment",
		Type:         veinbox.ExternalCarrierTrackingJudgedEventType,
		Version:      1,
		Scope:        "tenant-a",
		Subject:      "EXTF-1/EXTV-2",
		PartitionKey: "tenant-a/parcel-1/external-carrier-tracking",
		OccurredAt:   now,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}
}

func TestAJudgedExternalTrackingIsProcessedExactlyOnce(t *testing.T) {
	consumer, handler := newVEExternalTrackingFixture(t)
	envelope := veExternalTrackingEnvelope(t, "tenant-a/EXTF-1/EXTV-2/external-carrier-tracking")

	if err := consumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("首投：%v", err)
	}
	if err := consumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("重复投递：%v", err)
	}
	if len(handler.calls) != 1 {
		t.Fatalf("处理次数 = %d, want 1", len(handler.calls))
	}
	if got := handler.calls[0]; got.TenantID != "tenant-a" || got.Fact != "EXTF-1" || got.Version != "EXTV-2" {
		t.Fatalf("译码结果 = %+v", got)
	}
}

func TestAnExternalTrackingEnvelopeMissingAnyReferenceDimensionIsPoison(t *testing.T) {
	for name, payload := range map[string]string{
		"缺 tenantId": `{"fact":"EXTF-1","version":"EXTV-2"}`,
		"缺 fact":     `{"tenantId":"tenant-a","version":"EXTV-2"}`,
		"缺 version":  `{"tenantId":"tenant-a","fact":"EXTF-1"}`,
		"不是 JSON":    `not json`,
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newVEExternalTrackingFixture(t)
			poison := veExternalTrackingEnvelope(t, "poison-"+name)
			poison.Payload = json.RawMessage(payload)

			if err := consumer.Consume(t.Context(), poison); err != nil {
				t.Fatalf("毒丸首投应拒收入账而不是报错：%v", err)
			}
			if err := consumer.Consume(t.Context(), poison); err != nil {
				t.Fatalf("毒丸重投：%v", err)
			}
			if len(handler.calls) != 0 {
				t.Fatalf("处理次数 = %d, want 0", len(handler.calls))
			}
		})
	}
}

func TestAFailedExternalTrackingHandlerRollsBackAndTheRedeliveryRetries(t *testing.T) {
	consumer, handler := newVEExternalTrackingFixture(t)
	envelope := veExternalTrackingEnvelope(t, "tenant-a/EXTF-1/EXTV-2/external-carrier-tracking")

	handler.err = errors.New("external tracking fact is not yet visible")
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

func TestForeignTypesAreLoudOnTheExternalTrackingConsumer(t *testing.T) {
	for name, eventType := range map[string]eventing.EventType{
		"有效交付登记": veinbox.EffectiveDeliveryRegisteredEventType,
		"控制转出交接": veinbox.TransportHandoverRegisteredEventType,
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newVEExternalTrackingFixture(t)
			foreign := veExternalTrackingEnvelope(t, "foreign-"+name)
			foreign.Type = eventType
			if err := consumer.Consume(t.Context(), foreign); err == nil {
				t.Fatal("异类事件应响亮报错——它们不是外部承运轨迹事实")
			}
			if len(handler.calls) != 0 {
				t.Fatalf("处理次数 = %d, want 0", len(handler.calls))
			}
		})
	}
}
