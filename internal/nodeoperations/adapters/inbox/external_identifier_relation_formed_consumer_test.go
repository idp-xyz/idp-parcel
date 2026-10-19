package noinbox_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	noinbox "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/inbox"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证 node-operations 第一个 inbox 消费门（ADR-0199 决定三）：PS 外部标识关联已形成的门铃投到这里，
// 恰处理一次；载荷缺租户或号码即毒丸；处理失败整笔回滚、重投再试；认不得的类型响亮报错。舞步在 platform/inboxconsume，
// 本包只注入名字、类型与译码。夹具全为合成（S 级）。

type formedRelationHandlerDouble struct {
	calls []noinbox.FormedExternalIdentifierRelation
	err   error
}

func (double *formedRelationHandlerDouble) HandleFormedExternalIdentifierRelation(
	_ context.Context, formed noinbox.FormedExternalIdentifierRelation,
) error {
	if double.err != nil {
		return double.err
	}
	double.calls = append(double.calls, formed)
	return nil
}

func newFormedRelationFixture(t *testing.T) (*noinbox.ExternalIdentifierRelationFormedConsumer, *formedRelationHandlerDouble) {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	handler := &formedRelationHandlerDouble{}
	consumer, err := noinbox.NewExternalIdentifierRelationFormedConsumer(db.Transactor(), store, handler)
	if err != nil {
		t.Fatalf("构造消费者：%v", err)
	}
	return consumer, handler
}

func formedRelationEnvelope(t *testing.T, eventID string, payload map[string]string) eventing.Envelope {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("载荷：%v", err)
	}
	now := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	return eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       "idp-parcel/parcel-shipment",
		Type:         noinbox.ExternalIdentifierRelationFormedEventType,
		Version:      1,
		Scope:        "SYN-TENANT-NRS03",
		Subject:      "SYN-MARK-NRS03",
		PartitionKey: "partition-" + eventID,
		OccurredAt:   now,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      raw,
	}
}

var syntheticFormedRelation = map[string]string{"tenantId": "SYN-TENANT-NRS03", "number": " SYN-MARK-NRS03\t"}

// Covers: 一封门铃恰处理一次，号码原样转交（不去空白、不改写：重核逐字比对存下的号）。
func TestAFormedRelationDoorbellIsHandledExactlyOnce(t *testing.T) {
	consumer, handler := newFormedRelationFixture(t)
	envelope := formedRelationEnvelope(t, "relation-1", syntheticFormedRelation)

	if err := consumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("首投：%v", err)
	}
	if err := consumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("重复投递：%v", err)
	}
	if len(handler.calls) != 1 {
		t.Fatalf("处理次数 = %d, want 1", len(handler.calls))
	}
	if got := handler.calls[0]; got.TenantID != "SYN-TENANT-NRS03" || got.Number != " SYN-MARK-NRS03\t" {
		t.Fatalf("译码结果 = %+v", got)
	}
}

// Covers: 门铃缺租户或号码即毒丸：拒收入账、不转交、不报错——重投同样内容不会长出字段来。
func TestADoorbellMissingTheTenantOrTheNumberIsPoison(t *testing.T) {
	for name, payload := range map[string]map[string]string{
		"缺 tenantId": {"number": "SYN-MARK-NRS03"},
		"缺 number":   {"tenantId": "SYN-TENANT-NRS03"},
		"号码为空":       {"tenantId": "SYN-TENANT-NRS03", "number": ""},
	} {
		t.Run(name, func(t *testing.T) {
			consumer, handler := newFormedRelationFixture(t)
			poison := formedRelationEnvelope(t, "poison", payload)
			for range 2 {
				if err := consumer.Consume(t.Context(), poison); err != nil {
					t.Fatalf("毒丸应拒收入账而不是报错：%v", err)
				}
			}
			if len(handler.calls) != 0 {
				t.Fatalf("处理次数 = %d, want 0", len(handler.calls))
			}
		})
	}
}

// Covers: 重核没形成答案（身份核对读不动）时整笔回滚、inbox 无账，重投再试。
func TestAFailedRecheckRollsBackAndTheRedeliveryRetries(t *testing.T) {
	consumer, handler := newFormedRelationFixture(t)
	envelope := formedRelationEnvelope(t, "relation-1", syntheticFormedRelation)

	handler.err = errors.New("parcel identity unreadable")
	if err := consumer.Consume(t.Context(), envelope); err == nil {
		t.Fatal("处理失败必须让消费报错")
	}
	handler.err = nil
	if err := consumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("重投：%v", err)
	}
	if len(handler.calls) != 1 {
		t.Fatalf("处理次数 = %d, want 1", len(handler.calls))
	}
}

// Covers: 认不得的类型响亮报错、不入账。
func TestAForeignTypeIsLoudOnTheDoorbellConsumer(t *testing.T) {
	consumer, handler := newFormedRelationFixture(t)
	foreign := formedRelationEnvelope(t, "foreign", syntheticFormedRelation)
	foreign.Type = "node-operations.node-intake.formed"

	if err := consumer.Consume(t.Context(), foreign); err == nil {
		t.Fatal("认不得的类型必须响亮报错")
	}
	if len(handler.calls) != 0 {
		t.Fatalf("处理次数 = %d, want 0", len(handler.calls))
	}
}
