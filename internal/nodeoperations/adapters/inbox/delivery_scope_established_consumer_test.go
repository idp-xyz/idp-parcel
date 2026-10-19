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
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证「交付范围成立」门铃怎样被接住（ADR-0212 决定六；票 ps-external-mark-relations/12）：与别的重核门铃
// 同一个 inbox 消费者，恰处理一次；只指名交接单的转成按交接单重核，带判断版本的转成只重核那一件；缺租户或交接单即毒丸；重核失败整笔
// 回滚、重投再试。夹具全为合成（S 级）。

type scopeRecheckDouble struct {
	named []application.RecheckPendingIdentityCommand
	notes []string
	err   error
}

func (double *scopeRecheckDouble) Recheck(_ context.Context, command application.RecheckPendingIdentityCommand) (application.RecheckPendingIdentityResult, error) {
	if double.err != nil {
		return application.RecheckPendingIdentityResult{}, double.err
	}
	double.named = append(double.named, command)
	return application.RecheckPendingIdentityResult{}, nil
}

func (double *scopeRecheckDouble) RecheckByHandoverNote(_ context.Context, tenant domain.TenantID, note domain.DeliveryHandoverNoteID) ([]application.RecheckPendingIdentityResult, error) {
	if double.err != nil {
		return nil, double.err
	}
	double.notes = append(double.notes, tenant.String()+"|"+note.String())
	return nil, nil
}

func newScopeEstablishedConsumer(t *testing.T) (*noinbox.DeliveryScopeEstablishedConsumer, *scopeRecheckDouble) {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	recheck := &scopeRecheckDouble{}
	handler, err := noinbox.NewRecheckWithinEstablishedScope(recheck)
	if err != nil {
		t.Fatalf("构造重核转交：%v", err)
	}
	consumer, err := noinbox.NewDeliveryScopeEstablishedConsumer(db.Transactor(), store, handler)
	if err != nil {
		t.Fatalf("构造消费者：%v", err)
	}
	return consumer, recheck
}

func scopeEstablishedEnvelope(t *testing.T, eventID string, payload map[string]string) eventing.Envelope {
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
		Type:         noinbox.DeliveryScopeEstablishedEventType,
		Version:      1,
		Scope:        "SYN-TENANT-PSXM12",
		Subject:      "SYN-HANDOVER-PSXM12",
		PartitionKey: "partition-" + eventID,
		OccurredAt:   now,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      raw,
	}
}

// Covers: ADR-0212 决定六「NO 用同一个 inbox 消费者接，重核对象只看成员里属于那一组的，或归入的那一件」——只指名交接单的那一封转成
// 按交接单重核，带判断版本的那一封转成只指名那一版的重核；各恰处理一次。
func TestAScopeDoorbellRechecksTheNoteOrTheAssignedUnitExactlyOnce(t *testing.T) {
	consumer, recheck := newScopeEstablishedConsumer(t)
	whole := scopeEstablishedEnvelope(t, "scope-note", map[string]string{"tenantId": "SYN-TENANT-PSXM12", "handoverNote": "SYN-HANDOVER-PSXM12"})
	assigned := scopeEstablishedEnvelope(t, "scope-unit", map[string]string{
		"tenantId": "SYN-TENANT-PSXM12", "handoverNote": "SYN-HANDOVER-PSXM12", "judgmentVersion": "SYN-INTAKEV-PSXM12",
	})
	for range 2 {
		for _, envelope := range []eventing.Envelope{whole, assigned} {
			if err := consumer.Consume(t.Context(), envelope); err != nil {
				t.Fatalf("投递 %s：%v", envelope.ID, err)
			}
		}
	}
	if len(recheck.notes) != 1 || recheck.notes[0] != "SYN-TENANT-PSXM12|SYN-HANDOVER-PSXM12" {
		t.Fatalf("按交接单重核 = %v, want 恰一次", recheck.notes)
	}
	if len(recheck.named) != 1 || recheck.named[0].TenantID.String() != "SYN-TENANT-PSXM12" || recheck.named[0].Version.String() != "SYN-INTAKEV-PSXM12" {
		t.Fatalf("指名重核 = %+v, want 恰一次、指名那一版", recheck.named)
	}
}

// Covers: 门铃缺租户、缺交接单、判断版本给了却空白即毒丸：拒收入账、不转交、不报错。
func TestAScopeDoorbellMissingItsNoteIsPoison(t *testing.T) {
	for name, payload := range map[string]map[string]string{
		"缺 tenantId":     {"handoverNote": "SYN-HANDOVER-PSXM12"},
		"缺 handoverNote": {"tenantId": "SYN-TENANT-PSXM12"},
		"判断版本空白":         {"tenantId": "SYN-TENANT-PSXM12", "handoverNote": "SYN-HANDOVER-PSXM12", "judgmentVersion": " "},
	} {
		t.Run(name, func(t *testing.T) {
			consumer, recheck := newScopeEstablishedConsumer(t)
			for range 2 {
				if err := consumer.Consume(t.Context(), scopeEstablishedEnvelope(t, "poison", payload)); err != nil {
					t.Fatalf("毒丸应拒收入账而不是报错：%v", err)
				}
			}
			if len(recheck.notes)+len(recheck.named) != 0 {
				t.Fatalf("转交了 %v / %v", recheck.notes, recheck.named)
			}
		})
	}
}

// Covers: 重核失败（身份核对读不动）整笔回滚、inbox 无账，重投再试；错误原样带着 ErrParcelIdentityUnreadable，装配据此记未决。
func TestAScopeDoorbellWhoseRecheckFailsIsRetried(t *testing.T) {
	consumer, recheck := newScopeEstablishedConsumer(t)
	envelope := scopeEstablishedEnvelope(t, "scope-note", map[string]string{"tenantId": "SYN-TENANT-PSXM12", "handoverNote": "SYN-HANDOVER-PSXM12"})
	recheck.err = application.ErrParcelIdentityUnreadable
	if err := consumer.Consume(t.Context(), envelope); !errors.Is(err, application.ErrParcelIdentityUnreadable) {
		t.Fatalf("err = %v, want 带 ErrParcelIdentityUnreadable", err)
	}
	recheck.err = nil
	if err := consumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("重投：%v", err)
	}
	if len(recheck.notes) != 1 {
		t.Fatalf("按交接单重核 %d 次, want 1", len(recheck.notes))
	}
	foreign := scopeEstablishedEnvelope(t, "foreign", map[string]string{"tenantId": "SYN-TENANT-PSXM12", "handoverNote": "SYN-HANDOVER-PSXM12"})
	foreign.Type = noinbox.UncheckedPendingIdentityRecordedEventType
	if err := consumer.Consume(t.Context(), foreign); err == nil {
		t.Fatal("认不得的类型必须响亮报错")
	}
	if _, err := noinbox.NewRecheckWithinEstablishedScope(nil); err == nil {
		t.Fatal("缺重核编排也构造成了")
	}
}
