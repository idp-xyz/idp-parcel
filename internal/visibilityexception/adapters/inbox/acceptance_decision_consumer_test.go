package veinbox_test

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
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"

	"go.idp.xyz/idp-bento-go/eventing"
)

// 本文件对真实 PostgreSQL 证 VE 接受决定消费门：四维引用译码、恰一次、毒丸、与 NR
// 同信封不同账本。载荷带（租户+账户+委托+状态字）——账户只作一致性校验的比对值，
// 权威账户维由处理方经 PS 反查口取回（ADR-0060）。

type veAcceptanceHandlerDouble struct {
	calls []veinbox.FormedAcceptanceDecision
	err   error
}

func (double *veAcceptanceHandlerDouble) HandleFormedAcceptanceDecision(
	_ context.Context, formed veinbox.FormedAcceptanceDecision,
) error {
	if double.err != nil {
		return double.err
	}
	double.calls = append(double.calls, formed)
	return nil
}

func newVEAcceptanceDecisionFixture(t *testing.T) (*veinbox.AcceptanceDecisionConsumer, *veAcceptanceHandlerDouble) {
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
	handler := &veAcceptanceHandlerDouble{}
	consumer, err := veinbox.NewAcceptanceDecisionConsumer(db.Transactor(), store, handler)
	if err != nil {
		t.Fatalf("构造消费者：%v", err)
	}
	return consumer, handler
}

func veAcceptanceDecisionEnvelope(t *testing.T, eventID string) eventing.Envelope {
	t.Helper()

	payload, err := json.Marshal(map[string]string{
		"tenantId":          "tenant-a",
		"customerAccountId": "customer-1",
		"source":            "source-1",
		"sourceRequestKey":  "key-1",
		"shipmentRequestId": "request-1",
		"submissionVersion": "version-1",
		"decisionId":        eventID,
		"state":             "ACCEPTED",
	})
	if err != nil {
		t.Fatalf("载荷：%v", err)
	}
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	return eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       "idp-parcel/parcel-shipment",
		Type:         veinbox.AcceptanceDecisionFormedEventType,
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

func TestAnAcceptanceDecisionIsProcessedExactlyOnce(t *testing.T) {
	consumer, handler := newVEAcceptanceDecisionFixture(t)
	envelope := veAcceptanceDecisionEnvelope(t, "decision-1")

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
	if got.TenantID != "tenant-a" || got.CustomerAccountID != "customer-1" ||
		got.ShipmentRequestID != "request-1" || got.State != "ACCEPTED" {
		t.Fatalf("译码结果 = %+v；四维引用须原样到达处理方", got)
	}
}

func TestAnAcceptanceEnvelopeMissingAnyRequiredFieldIsPoison(t *testing.T) {
	for name, payload := range map[string]string{
		"缺 tenantId":          `{"customerAccountId":"customer-1","shipmentRequestId":"request-1","state":"ACCEPTED"}`,
		"缺 customerAccountId": `{"tenantId":"tenant-a","shipmentRequestId":"request-1","state":"ACCEPTED"}`,
		"缺 shipmentRequestId": `{"tenantId":"tenant-a","customerAccountId":"customer-1","state":"ACCEPTED"}`,
		"缺 state":             `{"tenantId":"tenant-a","customerAccountId":"customer-1","shipmentRequestId":"request-1"}`,
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newVEAcceptanceDecisionFixture(t)
			poison := veAcceptanceDecisionEnvelope(t, "poison-"+name)
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

func TestAFailedAcceptanceHandlerRollsBackAndTheRedeliveryRetries(t *testing.T) {
	consumer, handler := newVEAcceptanceDecisionFixture(t)
	envelope := veAcceptanceDecisionEnvelope(t, "decision-1")

	handler.err = errors.New("declared parcels view is unavailable")
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

func TestForeignTypesAreLoudOnTheAcceptanceDecisionConsumer(t *testing.T) {
	for name, eventType := range map[string]eventing.EventType{
		"投影派生":   veinbox.TrackingProjectionDerivedEventType,
		"权威交接登记": veinbox.TransportHandoverRegisteredEventType,
		"终局形成":   veinbox.FinalOutcomeFormedEventType,
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newVEAcceptanceDecisionFixture(t)
			foreign := veAcceptanceDecisionEnvelope(t, "foreign-"+name)
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

// nrAcceptanceHandlerDouble 是 NR 侧处理方替身，只数调用——本文件只关心两本账分家。
type nrAcceptanceHandlerDouble struct{ calls int }

func (double *nrAcceptanceHandlerDouble) HandleAcceptedDecision(
	_ context.Context, _ nrinbox.AcceptedDecision,
) error {
	double.calls++
	return nil
}

// Covers: FanOut 的前提——同一封接受决定信封，VE 与 NR 两本 inbox 互不隶属，各自
// 恰一次。塞进同一本账会让先到的那路把后到那路的投递当重复跳过。
func TestAcceptanceInboxLedgersAreSeparateBetweenVEAndNR(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}

	veHandler := &veAcceptanceHandlerDouble{}
	veConsumer, err := veinbox.NewAcceptanceDecisionConsumer(db.Transactor(), store, veHandler)
	if err != nil {
		t.Fatalf("构造 VE 消费者：%v", err)
	}
	nrHandler := &nrAcceptanceHandlerDouble{}
	nrConsumer, err := nrinbox.NewAcceptanceConsumer(db.Transactor(), store, nrHandler)
	if err != nil {
		t.Fatalf("构造 NR 消费者：%v", err)
	}

	envelope := veAcceptanceDecisionEnvelope(t, "shared-decision-1")
	if err := veConsumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("VE 投递：%v", err)
	}
	if err := nrConsumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("NR 投递：%v", err)
	}
	if len(veHandler.calls) != 1 || nrHandler.calls != 1 {
		t.Fatalf("两本账应各处理一次：VE = %d，NR = %d", len(veHandler.calls), nrHandler.calls)
	}
}
