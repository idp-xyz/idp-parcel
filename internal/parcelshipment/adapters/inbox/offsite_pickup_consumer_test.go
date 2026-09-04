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
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"

	"go.idp.xyz/idp-bento-go/eventing"
)

// 本文件对真实 PostgreSQL 16 证场外揽收登记这一路的消费门与译码，并证两路采用消费者
// 各记一本 Inbox 账——共名等于一条把另一条的投递消费掉。

type registeredHandlerDouble struct {
	calls []psinbox.RegisteredOffsitePickup
	err   error
}

func (double *registeredHandlerDouble) HandleRegisteredOffsitePickup(
	_ context.Context, registered psinbox.RegisteredOffsitePickup,
) error {
	if double.err != nil {
		return double.err
	}
	double.calls = append(double.calls, registered)
	return nil
}

func newOffsitePickupFixture(t *testing.T) (*psinbox.OffsitePickupConsumer, *registeredHandlerDouble) {
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
	handler := &registeredHandlerDouble{}
	consumer, err := psinbox.NewOffsitePickupConsumer(db.Transactor(), store, handler)
	if err != nil {
		t.Fatalf("构造消费者：%v", err)
	}
	return consumer, handler
}

func offsitePickupEnvelope(t *testing.T, eventID string) eventing.Envelope {
	t.Helper()

	payload, err := json.Marshal(map[string]string{
		"tenantId":      "tenant-a",
		"object":        "parcel-1",
		"attempt":       "attempt-1",
		"pickupVersion": "PRV-000000000001",
	})
	if err != nil {
		t.Fatalf("载荷：%v", err)
	}
	now := time.Date(2026, 8, 18, 15, 30, 0, 0, time.UTC)
	return eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       "idp-parcel/transport-fulfillment",
		Type:         psinbox.OffsitePickupRegisteredEventType,
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

func TestARegisteredOffsitePickupDeliveryIsProcessedExactlyOnce(t *testing.T) {
	consumer, handler := newOffsitePickupFixture(t)
	envelope := offsitePickupEnvelope(t, "tenant-a/parcel-1/attempt-1/offsite-pickup-registration")

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
	if got.TenantID != "tenant-a" || got.Object != "parcel-1" || got.Attempt != "attempt-1" || got.PickupVersion != "PRV-000000000001" {
		t.Fatalf("译码结果 = %+v", got)
	}
}

// 四维缺一即毒丸：键三维缺了取不回登记；版本缺了分不出这封信说的是哪一代（ADR-0117 决定四），
// 而 TF 自 tf/08 起总带它，重投同样内容不会长出字段。
func TestAnOffsitePickupEnvelopeMissingAnyKeyDimensionIsPoison(t *testing.T) {
	for name, payload := range map[string]string{
		"缺 tenantId":      `{"object":"parcel-1","attempt":"attempt-1","pickupVersion":"PRV-000000000001"}`,
		"缺 object":        `{"tenantId":"tenant-a","attempt":"attempt-1","pickupVersion":"PRV-000000000001"}`,
		"缺 attempt":       `{"tenantId":"tenant-a","object":"parcel-1","pickupVersion":"PRV-000000000001"}`,
		"缺 pickupVersion": `{"tenantId":"tenant-a","object":"parcel-1","attempt":"attempt-1"}`,
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newOffsitePickupFixture(t)
			poison := offsitePickupEnvelope(t, "poison-"+name)
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

func TestAFailedOffsitePickupHandlerRollsBackAndTheRedeliveryRetries(t *testing.T) {
	consumer, handler := newOffsitePickupFixture(t)
	envelope := offsitePickupEnvelope(t, "tenant-a/parcel-1/attempt-1/offsite-pickup-registration")

	handler.err = errors.New("pickup registration not yet visible")
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

func TestAForeignTypeIsLoudOnTheOffsitePickupConsumer(t *testing.T) {
	consumer, handler := newOffsitePickupFixture(t)
	foreign := offsitePickupEnvelope(t, "foreign")
	foreign.Type = psinbox.NodeIntakeFormedEventType

	if err := consumer.Consume(t.Context(), foreign); err == nil {
		t.Fatal("认不得的类型必须响亮报错")
	}
	if len(handler.calls) != 0 {
		t.Fatalf("处理次数 = %d, want 0", len(handler.calls))
	}
}

// Covers: 两路采用消费者的 Inbox 键只差消费者名。同一 source 与同一事件 ID 分别投给
// 两扇门，两侧都必须处理一次；共名会让第二扇门把它当重复投递跳过。
func TestTheTwoAdoptionConsumersKeepSeparateInboxAccounts(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}

	nodeHandler := &formedHandlerDouble{}
	nodeConsumer, err := psinbox.NewNodeIntakeConsumer(db.Transactor(), store, nodeHandler)
	if err != nil {
		t.Fatalf("构造节点收寄消费者：%v", err)
	}
	pickupHandler := &registeredHandlerDouble{}
	pickupConsumer, err := psinbox.NewOffsitePickupConsumer(db.Transactor(), store, pickupHandler)
	if err != nil {
		t.Fatalf("构造揽收登记消费者：%v", err)
	}

	const shared = "shared-event-id"
	nodeEnvelope := nodeIntakeEnvelope(t, shared)
	pickupEnvelope := offsitePickupEnvelope(t, shared)
	pickupEnvelope.Source = nodeEnvelope.Source

	if err := nodeConsumer.Consume(t.Context(), nodeEnvelope); err != nil {
		t.Fatalf("节点收寄投递：%v", err)
	}
	if err := pickupConsumer.Consume(t.Context(), pickupEnvelope); err != nil {
		t.Fatalf("揽收登记投递：%v", err)
	}

	if len(nodeHandler.calls) != 1 || len(pickupHandler.calls) != 1 {
		t.Fatalf("两本账应各处理一次：节点 = %d，揽收 = %d",
			len(nodeHandler.calls), len(pickupHandler.calls))
	}
}
