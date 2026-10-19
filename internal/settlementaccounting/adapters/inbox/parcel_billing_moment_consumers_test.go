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

// 本文件对真实 PostgreSQL 16 证包裹计费时点两路的消费门与译码（票 sell-customer-charge/02）：收寄采用与终局各认自己那
// 一种信封，载荷四维（tenantId / parcel / kind / version）缺一即毒丸，处理失败回滚重投，同一封只处理一次。

type recordedIntakeHandlerDouble struct {
	calls []sainbox.RecordedNetworkIntake
	err   error
}

func (double *recordedIntakeHandlerDouble) HandleRecordedNetworkIntake(
	_ context.Context, recorded sainbox.RecordedNetworkIntake,
) error {
	if double.err != nil {
		return double.err
	}
	double.calls = append(double.calls, recorded)
	return nil
}

type formedFinalHandlerDouble struct {
	calls []sainbox.FormedFinalOutcome
	err   error
}

func (double *formedFinalHandlerDouble) HandleFormedFinalOutcome(
	_ context.Context, formed sainbox.FormedFinalOutcome,
) error {
	if double.err != nil {
		return double.err
	}
	double.calls = append(double.calls, formed)
	return nil
}

// parcelMomentConsumer 统一两路的形状，好让同一组用例各跑一遍。
type parcelMomentConsumer interface {
	Consume(ctx context.Context, envelope eventing.Envelope) error
}

type parcelMomentLane struct {
	eventType eventing.EventType
	build     func(t *testing.T) (parcelMomentConsumer, func() int, func(error), func() [4]string)
}

func parcelMomentLanes() map[string]parcelMomentLane {
	return map[string]parcelMomentLane{
		"收寄采用": {
			eventType: sainbox.NetworkIntakeRecordedEventType,
			build: func(t *testing.T) (parcelMomentConsumer, func() int, func(error), func() [4]string) {
				db, store := parcelMomentInbox(t)
				handler := &recordedIntakeHandlerDouble{}
				consumer, err := sainbox.NewNetworkIntakeRecordedConsumer(db.Transactor(), store, handler)
				if err != nil {
					t.Fatalf("构造消费者：%v", err)
				}
				return consumer,
					func() int { return len(handler.calls) },
					func(err error) { handler.err = err },
					func() [4]string {
						got := handler.calls[0]
						return [4]string{got.TenantID, got.Parcel, got.Kind, got.Version}
					}
			},
		},
		"终局": {
			eventType: sainbox.FinalOutcomeFormedEventType,
			build: func(t *testing.T) (parcelMomentConsumer, func() int, func(error), func() [4]string) {
				db, store := parcelMomentInbox(t)
				handler := &formedFinalHandlerDouble{}
				consumer, err := sainbox.NewFinalOutcomeFormedConsumer(db.Transactor(), store, handler)
				if err != nil {
					t.Fatalf("构造消费者：%v", err)
				}
				return consumer,
					func() int { return len(handler.calls) },
					func(err error) { handler.err = err },
					func() [4]string {
						got := handler.calls[0]
						return [4]string{got.TenantID, got.Parcel, got.Kind, got.Version}
					}
			},
		},
	}
}

func parcelMomentInbox(t *testing.T) (*bentopg.DB, *inbox.Store) {
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
	return db, store
}

func parcelMomentEnvelope(t *testing.T, eventType eventing.EventType, eventID string) eventing.Envelope {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"tenantId": "tenant-a",
		"parcel":   "SYN-PARCEL-01",
		"kind":     "SYN-KIND",
		"version":  "SYN-V1",
		// 收寄采用信封上还带地理投影；本上下文不收它，带着也不该让译码出错。
		"geo": map[string]string{"origin": "SYN-ORIGIN"},
	})
	if err != nil {
		t.Fatalf("载荷：%v", err)
	}
	now := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	return eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       "idp-parcel/parcel-shipment",
		Type:         eventType,
		Version:      1,
		Scope:        "tenant-a",
		Subject:      "SYN-PARCEL-01",
		PartitionKey: "tenant-a/SYN-PARCEL-01",
		OccurredAt:   now,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}
}

func TestAParcelMomentEnvelopeIsProcessedExactlyOnce(t *testing.T) {
	for name, lane := range parcelMomentLanes() {
		t.Run(name, func(t *testing.T) {
			consumer, calls, _, first := lane.build(t)
			envelope := parcelMomentEnvelope(t, lane.eventType, "moment-1")
			if err := consumer.Consume(t.Context(), envelope); err != nil {
				t.Fatalf("首投：%v", err)
			}
			if err := consumer.Consume(t.Context(), envelope); err != nil {
				t.Fatalf("重复投递：%v", err)
			}
			if calls() != 1 {
				t.Fatalf("处理次数 = %d, want 1", calls())
			}
			if got := first(); got != [4]string{"tenant-a", "SYN-PARCEL-01", "SYN-KIND", "SYN-V1"} {
				t.Fatalf("译码结果 = %v", got)
			}
		})
	}
}

func TestAParcelMomentEnvelopeMissingAnyKeyDimensionIsPoison(t *testing.T) {
	payloads := map[string]string{
		"缺 tenantId": `{"parcel":"P","kind":"K","version":"V"}`,
		"缺 parcel":   `{"tenantId":"T","kind":"K","version":"V"}`,
		"缺 kind":     `{"tenantId":"T","parcel":"P","version":"V"}`,
		"缺 version":  `{"tenantId":"T","parcel":"P","kind":"K"}`,
		"不是 JSON":    `{`,
	}
	for laneName, lane := range parcelMomentLanes() {
		for name, payload := range payloads {
			t.Run(laneName+"/"+name, func(t *testing.T) {
				consumer, calls, _, _ := lane.build(t)
				poison := parcelMomentEnvelope(t, lane.eventType, "poison-"+name)
				poison.Payload = json.RawMessage(payload)
				if err := consumer.Consume(t.Context(), poison); err != nil {
					t.Fatalf("毒丸应拒收入账而不是报错：%v", err)
				}
				if calls() != 0 {
					t.Fatalf("处理次数 = %d, want 0", calls())
				}
			})
		}
	}
}

func TestAFailedParcelMomentHandlerRollsBackAndTheRedeliveryRetries(t *testing.T) {
	for name, lane := range parcelMomentLanes() {
		t.Run(name, func(t *testing.T) {
			consumer, calls, fail, _ := lane.build(t)
			envelope := parcelMomentEnvelope(t, lane.eventType, "moment-retry")
			fail(errors.New("syn: registry down"))
			if err := consumer.Consume(t.Context(), envelope); err == nil {
				t.Fatal("处理失败必须让消费报错，不落账")
			}
			fail(nil)
			if err := consumer.Consume(t.Context(), envelope); err != nil {
				t.Fatalf("重投：%v", err)
			}
			if calls() != 1 {
				t.Fatalf("处理次数 = %d, want 1", calls())
			}
		})
	}
}

// Covers: 两路互不认对方的信封——路由表挂错时要在这里响亮，而不是让一路把另一路的事实当成自己的计费时点。
func TestEachParcelMomentConsumerIsLoudOnTheOtherType(t *testing.T) {
	lanes := parcelMomentLanes()
	for name, lane := range lanes {
		t.Run(name, func(t *testing.T) {
			consumer, calls, _, _ := lane.build(t)
			foreignType := sainbox.FinalOutcomeFormedEventType
			if lane.eventType == foreignType {
				foreignType = sainbox.NetworkIntakeRecordedEventType
			}
			if err := consumer.Consume(t.Context(), parcelMomentEnvelope(t, foreignType, "foreign")); err == nil {
				t.Fatal("认不得的类型必须响亮报错")
			}
			if calls() != 0 {
				t.Fatalf("处理次数 = %d, want 0", calls())
			}
		})
	}
}

func TestTheParcelMomentConsumersRefuseNilDependencies(t *testing.T) {
	if _, err := sainbox.NewNetworkIntakeRecordedConsumer(nil, nil, nil); err == nil {
		t.Fatal("收寄采用消费者收下了 nil 依赖")
	}
	if _, err := sainbox.NewFinalOutcomeFormedConsumer(nil, nil, nil); err == nil {
		t.Fatal("终局消费者收下了 nil 依赖")
	}
}
