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
	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证落未核对行时自投的重核门铃怎样被接住（ADR-0199 决定三第三个触发，票 no-reception-source/05）：
// 与 PS 的门铃同一个 inbox 消费者，恰处理一次；载荷缺租户或判断版本即毒丸；指名那一件交给重核编排；重核失败整笔回滚、重投
// 再试。夹具全为合成（S 级）。

type namedRecheckDouble struct {
	commands []application.RecheckPendingIdentityCommand
	err      error
}

func (double *namedRecheckDouble) Recheck(
	_ context.Context, command application.RecheckPendingIdentityCommand,
) (application.RecheckPendingIdentityResult, error) {
	if double.err != nil {
		return application.RecheckPendingIdentityResult{}, double.err
	}
	double.commands = append(double.commands, command)
	return application.RecheckPendingIdentityResult{}, nil
}

type doorbellConsumers struct {
	unchecked *noinbox.UncheckedPendingIdentityRecordedConsumer
	relation  *noinbox.ExternalIdentifierRelationFormedConsumer
	recheck   *namedRecheckDouble
	formed    *formedRelationHandlerDouble
}

func newDoorbellConsumers(t *testing.T) doorbellConsumers {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	consumers := doorbellConsumers{recheck: &namedRecheckDouble{}, formed: &formedRelationHandlerDouble{}}
	processing, err := noinbox.NewRecheckNamedPendingIdentity(consumers.recheck)
	if err != nil {
		t.Fatalf("构造重核转交：%v", err)
	}
	if consumers.unchecked, err = noinbox.NewUncheckedPendingIdentityRecordedConsumer(db.Transactor(), store, processing); err != nil {
		t.Fatalf("构造消费者：%v", err)
	}
	if consumers.relation, err = noinbox.NewExternalIdentifierRelationFormedConsumer(db.Transactor(), store, consumers.formed); err != nil {
		t.Fatalf("构造 PS 门铃消费者：%v", err)
	}
	return consumers
}

func uncheckedDoorbell(t *testing.T, eventID string, payload map[string]string) eventing.Envelope {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("载荷：%v", err)
	}
	now := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	return eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       "idp-parcel/node-operations",
		Type:         noinbox.UncheckedPendingIdentityRecordedEventType,
		Version:      1,
		Scope:        "SYN-TENANT-NRS05",
		Subject:      "SYN-INTAKEV-NRS05",
		PartitionKey: "partition-" + eventID,
		OccurredAt:   now,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      raw,
	}
}

var syntheticUncheckedDoorbell = map[string]string{"tenantId": "SYN-TENANT-NRS05", "judgmentVersion": "SYN-INTAKEV-NRS05"}

// Covers: ADR-0199 决定三「NO 用同一个 inbox 消费者接，重核对象只看那一件」——门铃恰处理一次，转成只指名那一版的重核命令。
func TestAnUncheckedDoorbellRechecksTheNamedVersionExactlyOnce(t *testing.T) {
	consumers := newDoorbellConsumers(t)
	envelope := uncheckedDoorbell(t, "doorbell-1", syntheticUncheckedDoorbell)

	for range 2 {
		if err := consumers.unchecked.Consume(t.Context(), envelope); err != nil {
			t.Fatalf("投递：%v", err)
		}
	}
	if len(consumers.recheck.commands) != 1 {
		t.Fatalf("重核 %d 次, want 1", len(consumers.recheck.commands))
	}
	if got := consumers.recheck.commands[0]; got.TenantID.String() != "SYN-TENANT-NRS05" || got.Version.String() != "SYN-INTAKEV-NRS05" {
		t.Fatalf("重核命令 = %+v", got)
	}
}

// Covers: 两种门铃落在同一个 inbox 消费者名下，各自的信封身份不互相顶替：来源不同，同一串事件 ID 也是两把 inbox 键。
func TestBothDoorbellsShareTheConsumerWithoutShadowingEachOther(t *testing.T) {
	consumers := newDoorbellConsumers(t)
	if err := consumers.relation.Consume(t.Context(), formedRelationEnvelope(t, "shared-id", syntheticFormedRelation)); err != nil {
		t.Fatalf("PS 门铃：%v", err)
	}
	if err := consumers.unchecked.Consume(t.Context(), uncheckedDoorbell(t, "shared-id", syntheticUncheckedDoorbell)); err != nil {
		t.Fatalf("自投门铃：%v", err)
	}
	if len(consumers.formed.calls) != 1 || len(consumers.recheck.commands) != 1 {
		t.Fatalf("PS 门铃处理 %d 次、自投门铃 %d 次, want 各 1", len(consumers.formed.calls), len(consumers.recheck.commands))
	}
}

// Covers: 门铃缺租户或判断版本、或判断版本空白即毒丸：拒收入账、不转交、不报错。
func TestAnUncheckedDoorbellMissingItsVersionIsPoison(t *testing.T) {
	for name, payload := range map[string]map[string]string{
		"缺 tenantId":        {"judgmentVersion": "SYN-INTAKEV-NRS05"},
		"缺 judgmentVersion": {"tenantId": "SYN-TENANT-NRS05"},
		"判断版本空白":            {"tenantId": "SYN-TENANT-NRS05", "judgmentVersion": " "},
	} {
		t.Run(name, func(t *testing.T) {
			consumers := newDoorbellConsumers(t)
			for range 2 {
				if err := consumers.unchecked.Consume(t.Context(), uncheckedDoorbell(t, "poison", payload)); err != nil {
					t.Fatalf("毒丸应拒收入账而不是报错：%v", err)
				}
			}
			if len(consumers.recheck.commands) != 0 {
				t.Fatalf("重核 %d 次, want 0", len(consumers.recheck.commands))
			}
		})
	}
}

// Covers: ADR-0199 决定三「身份核对仍读不动就交回 error，信封留在 inbox 退避重投」——重核失败整笔回滚、inbox 无账，重投再试；
// 错误原样带着 ErrParcelIdentityUnreadable，装配据此记未决。
func TestAnUncheckedDoorbellWhoseRecheckFailsIsRetried(t *testing.T) {
	consumers := newDoorbellConsumers(t)
	envelope := uncheckedDoorbell(t, "doorbell-1", syntheticUncheckedDoorbell)

	consumers.recheck.err = application.ErrParcelIdentityUnreadable
	if err := consumers.unchecked.Consume(t.Context(), envelope); !errors.Is(err, application.ErrParcelIdentityUnreadable) {
		t.Fatalf("err = %v, want 带 ErrParcelIdentityUnreadable", err)
	}
	consumers.recheck.err = nil
	if err := consumers.unchecked.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("重投：%v", err)
	}
	if len(consumers.recheck.commands) != 1 {
		t.Fatalf("重核 %d 次, want 1", len(consumers.recheck.commands))
	}
}

// Covers: 认不得的类型响亮报错、不入账。
func TestAForeignTypeIsLoudOnTheUncheckedDoorbellConsumer(t *testing.T) {
	consumers := newDoorbellConsumers(t)
	foreign := uncheckedDoorbell(t, "foreign", syntheticUncheckedDoorbell)
	foreign.Type = noinbox.ExternalIdentifierRelationFormedEventType

	if err := consumers.unchecked.Consume(t.Context(), foreign); err == nil {
		t.Fatal("认不得的类型必须响亮报错")
	}
	if len(consumers.recheck.commands) != 0 {
		t.Fatalf("重核 %d 次, want 0", len(consumers.recheck.commands))
	}
}
