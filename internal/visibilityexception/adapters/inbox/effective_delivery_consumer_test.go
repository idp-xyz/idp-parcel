package veinbox_test

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
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"

	"go.idp.xyz/idp-bento-go/eventing"
)

// 本文件对真实 PostgreSQL 证 VE 有效交付消费门：译码、恰一次、毒丸、与 PS 终局账本分家。
// 结果版本是译码必备的引用维——每份信封代表一代；不认控制转出与揽收形成信封。

type veRegisteredDeliveryHandlerDouble struct {
	calls []veinbox.RegisteredEffectiveDelivery
	err   error
}

func (double *veRegisteredDeliveryHandlerDouble) HandleRegisteredEffectiveDelivery(
	_ context.Context, registered veinbox.RegisteredEffectiveDelivery,
) error {
	if double.err != nil {
		return double.err
	}
	double.calls = append(double.calls, registered)
	return nil
}

func newVEEffectiveDeliveryFixture(t *testing.T) (*veinbox.EffectiveDeliveryConsumer, *veRegisteredDeliveryHandlerDouble) {
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
	handler := &veRegisteredDeliveryHandlerDouble{}
	consumer, err := veinbox.NewEffectiveDeliveryConsumer(db.Transactor(), store, handler)
	if err != nil {
		t.Fatalf("构造消费者：%v", err)
	}
	return consumer, handler
}

func veEffectiveDeliveryEnvelope(t *testing.T, eventID string) eventing.Envelope {
	t.Helper()

	payload, err := json.Marshal(map[string]string{
		"tenantId": "tenant-a",
		"object":   "parcel-1",
		"attempt":  "attempt-1",
		"version":  "delivery-result/v1",
	})
	if err != nil {
		t.Fatalf("载荷：%v", err)
	}
	now := time.Date(2026, 8, 18, 16, 0, 0, 0, time.UTC)
	return eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       "idp-parcel/transport-fulfillment",
		Type:         veinbox.EffectiveDeliveryRegisteredEventType,
		Version:      1,
		Scope:        "tenant-a",
		Subject:      "parcel-1/attempt-1",
		PartitionKey: eventID,
		OccurredAt:   now,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}
}

func TestARegisteredEffectiveDeliveryIsProcessedExactlyOnce(t *testing.T) {
	consumer, handler := newVEEffectiveDeliveryFixture(t)
	envelope := veEffectiveDeliveryEnvelope(t, "tenant-a/parcel-1/attempt-1/delivery-result/v1/effective-delivery")

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
	if got.TenantID != "tenant-a" || got.Object != "parcel-1" || got.Attempt != "attempt-1" ||
		got.Version != "delivery-result/v1" {
		t.Fatalf("译码结果 = %+v", got)
	}
}

// Covers: 结果版本随载荷进引用——TF 每一代交付结果各入队一份信封，处理方按（键+版本）
// 读回信封指名的那一代。「版本只进 ID 不进载荷」的旧口径已废：按键读当前版会让更正
// 之前入队的首登信封也读成更正后那一代，先到的那一代从此不进投影。
func TestTheDecodedReferenceNamesTheResultVersion(t *testing.T) {
	consumer, handler := newVEEffectiveDeliveryFixture(t)
	envelope := veEffectiveDeliveryEnvelope(t, "tenant-a/parcel-1/attempt-1/delivery-result/v2/effective-delivery")
	envelope.Payload = json.RawMessage(
		`{"tenantId":"tenant-a","object":"parcel-1","attempt":"attempt-1","version":"delivery-result/v2"}`)

	if err := consumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("消费：%v", err)
	}
	if len(handler.calls) != 1 {
		t.Fatalf("处理次数 = %d, want 1", len(handler.calls))
	}
	got := handler.calls[0]
	if got.Version != "delivery-result/v2" {
		t.Fatalf("译码版本 = %q, want delivery-result/v2——版本是引用维，处理方按它指名读回那一代",
			got.Version)
	}
}

func TestAnEffectiveDeliveryEnvelopeMissingAnyReferenceDimensionIsPoison(t *testing.T) {
	for name, payload := range map[string]string{
		"缺 tenantId": `{"object":"parcel-1","attempt":"attempt-1","version":"delivery-result/v1"}`,
		"缺 object":   `{"tenantId":"tenant-a","attempt":"attempt-1","version":"delivery-result/v1"}`,
		"缺 attempt":  `{"tenantId":"tenant-a","object":"parcel-1","version":"delivery-result/v1"}`,
		"缺 version":  `{"tenantId":"tenant-a","object":"parcel-1","attempt":"attempt-1"}`,
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newVEEffectiveDeliveryFixture(t)
			poison := veEffectiveDeliveryEnvelope(t, "poison-"+name)
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

func TestAFailedEffectiveDeliveryHandlerRollsBackAndTheRedeliveryRetries(t *testing.T) {
	consumer, handler := newVEEffectiveDeliveryFixture(t)
	envelope := veEffectiveDeliveryEnvelope(t, "tenant-a/parcel-1/attempt-1/delivery-result/v1/effective-delivery")

	handler.err = errors.New("effective delivery is not yet visible")
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

func TestAPoisonEffectiveDeliveryEnvelopeIsRejectedOnceAndStaysRejected(t *testing.T) {
	consumer, handler := newVEEffectiveDeliveryFixture(t)
	poison := veEffectiveDeliveryEnvelope(t, "poison")
	poison.Payload = json.RawMessage(`{"tenantId":""}`)

	if err := consumer.Consume(t.Context(), poison); err != nil {
		t.Fatalf("毒丸首投应拒收入账：%v", err)
	}
	if err := consumer.Consume(t.Context(), poison); err != nil {
		t.Fatalf("毒丸重投：%v", err)
	}
	if len(handler.calls) != 0 {
		t.Fatalf("处理次数 = %d, want 0", len(handler.calls))
	}
}

func TestForeignTypesAreLoudOnTheEffectiveDeliveryConsumer(t *testing.T) {
	for name, eventType := range map[string]eventing.EventType{
		"控制转出交接":  "transport-fulfillment.transport-handover.registered",
		"对象级揽收登记": "transport-fulfillment.offsite-pickup.registered",
		"尝试级揽收集合": "transport-fulfillment.offsite-pickup.formed",
		"派送尝试未生效": "transport-fulfillment.delivery-attempt-result.recorded",
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newVEEffectiveDeliveryFixture(t)
			foreign := veEffectiveDeliveryEnvelope(t, "foreign-"+name)
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

func TestEffectiveDeliveryInboxLedgersAreSeparateFromParcelShipment(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}

	veHandler := &veRegisteredDeliveryHandlerDouble{}
	veConsumer, err := veinbox.NewEffectiveDeliveryConsumer(db.Transactor(), store, veHandler)
	if err != nil {
		t.Fatalf("构造 VE 消费者：%v", err)
	}
	psHandler := &psRegisteredDeliveryHandlerDouble{}
	psConsumer, err := psinbox.NewEffectiveDeliveryConsumer(db.Transactor(), store, psHandler)
	if err != nil {
		t.Fatalf("构造 PS 消费者：%v", err)
	}

	envelope := veEffectiveDeliveryEnvelope(t, "tenant-a/parcel-1/attempt-1/delivery-result/v1/effective-delivery")
	if err := veConsumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("VE 首投：%v", err)
	}
	if err := psConsumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("PS 首投：%v", err)
	}
	if len(veHandler.calls) != 1 {
		t.Fatalf("VE 处理次数 = %d, want 1", len(veHandler.calls))
	}
	if len(psHandler.calls) != 1 {
		t.Fatalf("PS 处理次数 = %d, want 1；共名会把对面的投递当重复跳过", len(psHandler.calls))
	}
}

type psRegisteredDeliveryHandlerDouble struct {
	calls []psinbox.RegisteredEffectiveDelivery
}

func (double *psRegisteredDeliveryHandlerDouble) HandleRegisteredEffectiveDelivery(
	_ context.Context, registered psinbox.RegisteredEffectiveDelivery,
) error {
	double.calls = append(double.calls, registered)
	return nil
}
