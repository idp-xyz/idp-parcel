package inboxconsume_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/platform/inboxconsume"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"

	"go.idp.xyz/idp-bento-go/eventing"
)

// 本文件对真实 PostgreSQL 16 证消费门本身：恰一次、重复跳过、毒丸拒收、处理失败回滚。
// 三个上下文的 inbox 适配器都走这一扇门，各自只注入名字、类型与译码。

const fixtureType eventing.EventType = "platform.inboxconsume.fixture"

type payload struct {
	Token string `json:"token"`
}

type handlerDouble struct {
	calls []payload
	err   error
}

func (double *handlerDouble) handle(_ context.Context, decoded payload) error {
	if double.err != nil {
		return double.err
	}
	double.calls = append(double.calls, decoded)
	return nil
}

func newGate(t *testing.T, name string, handler *handlerDouble) *inboxconsume.Gate[payload] {
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
	gate, err := inboxconsume.New(inboxconsume.Spec[payload]{
		Transactor:     db.Transactor(),
		Store:          store,
		Name:           name,
		EventType:      fixtureType,
		Decode:         decodePayload,
		Handle:         handler.handle,
		UnexpectedType: "inbox consume fixture",
		HandleVerb:     "handle fixture",
	})
	if err != nil {
		t.Fatalf("构造消费门：%v", err)
	}
	return gate
}

func decodePayload(raw []byte) (payload, error) {
	var body payload
	if err := json.Unmarshal(raw, &body); err != nil {
		return payload{}, err
	}
	if body.Token == "" {
		return payload{}, errors.New("missing token")
	}
	return body, nil
}

func fixtureEnvelope(t *testing.T, eventID, token string) eventing.Envelope {
	t.Helper()
	raw, err := json.Marshal(payload{Token: token})
	if err != nil {
		t.Fatalf("载荷：%v", err)
	}
	now := time.Date(2026, 8, 18, 13, 0, 0, 0, time.UTC)
	return eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       "idp-parcel/platform",
		Type:         fixtureType,
		Version:      1,
		Scope:        "tenant-a",
		Subject:      token,
		PartitionKey: "tenant-a/" + token,
		OccurredAt:   now,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      raw,
	}
}

func TestADeliveryIsProcessedExactlyOnce(t *testing.T) {
	handler := &handlerDouble{}
	gate := newGate(t, "platform/inboxconsume-once", handler)
	envelope := fixtureEnvelope(t, "event-1", "token-1")

	if err := gate.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("首投：%v", err)
	}
	if err := gate.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("重复投递：%v", err)
	}
	if len(handler.calls) != 1 || handler.calls[0].Token != "token-1" {
		t.Fatalf("处理 = %+v, want 恰一次 token-1", handler.calls)
	}
}

func TestDistinctConsumerNamesKeepSeparateLedgers(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	first := &handlerDouble{}
	second := &handlerDouble{}
	mustGate := func(name string, handler *handlerDouble) *inboxconsume.Gate[payload] {
		t.Helper()
		gate, err := inboxconsume.New(inboxconsume.Spec[payload]{
			Transactor:     db.Transactor(),
			Store:          store,
			Name:           name,
			EventType:      fixtureType,
			Decode:         decodePayload,
			Handle:         handler.handle,
			UnexpectedType: "inbox consume fixture",
			HandleVerb:     "handle fixture",
		})
		if err != nil {
			t.Fatalf("构造消费门：%v", err)
		}
		return gate
	}
	envelope := fixtureEnvelope(t, "event-shared", "token-shared")
	if err := mustGate("platform/inboxconsume-a", first).Consume(t.Context(), envelope); err != nil {
		t.Fatalf("第一本账：%v", err)
	}
	if err := mustGate("platform/inboxconsume-b", second).Consume(t.Context(), envelope); err != nil {
		t.Fatalf("第二本账：%v", err)
	}
	if len(first.calls) != 1 || len(second.calls) != 1 {
		t.Fatalf("两本账应各处理一次，得 %d 与 %d", len(first.calls), len(second.calls))
	}
}

func TestAFailedHandlerRollsBackAndTheRedeliveryRetries(t *testing.T) {
	handler := &handlerDouble{err: errors.New("downstream unavailable")}
	gate := newGate(t, "platform/inboxconsume-retry", handler)
	envelope := fixtureEnvelope(t, "event-1", "token-1")

	if err := gate.Consume(t.Context(), envelope); err == nil {
		t.Fatal("处理失败必须让消费报错")
	}

	handler.err = nil
	if err := gate.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("重投：%v", err)
	}
	if len(handler.calls) != 1 {
		t.Fatalf("处理次数 = %d, want 1", len(handler.calls))
	}
}

func TestAPoisonEnvelopeIsRejectedOnceAndStaysRejected(t *testing.T) {
	handler := &handlerDouble{}
	gate := newGate(t, "platform/inboxconsume-poison", handler)
	poison := fixtureEnvelope(t, "event-poison", "token-1")
	poison.Payload = json.RawMessage(`{"token":""}`)

	if err := gate.Consume(t.Context(), poison); err != nil {
		t.Fatalf("毒丸首投应拒收入账：%v", err)
	}
	if err := gate.Consume(t.Context(), poison); err != nil {
		t.Fatalf("毒丸重投：%v", err)
	}
	if len(handler.calls) != 0 {
		t.Fatalf("处理次数 = %d, want 0", len(handler.calls))
	}
}

func TestAForeignEventTypeIsLoudNotRecorded(t *testing.T) {
	handler := &handlerDouble{}
	gate := newGate(t, "platform/inboxconsume-foreign", handler)
	foreign := fixtureEnvelope(t, "event-foreign", "token-1")
	foreign.Type = "some.other.event"

	if err := gate.Consume(t.Context(), foreign); err == nil {
		t.Fatal("认不得的类型必须响亮报错")
	}
	if len(handler.calls) != 0 {
		t.Fatalf("处理次数 = %d, want 0", len(handler.calls))
	}
}
