package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

type lodgementLedgerDouble struct {
	lodgements []ports.ManualLodgement
	err        error
}

func (double *lodgementLedgerDouble) RecordManualLodgement(_ context.Context, lodgement ports.ManualLodgement) error {
	if double.err != nil {
		return double.err
	}
	double.lodgements = append(double.lodgements, lodgement)
	return nil
}

func manualLodgementHandler(t *testing.T, fixture *sendFixture, ledger *lodgementLedgerDouble) *application.RecordManualLodgementHandler {
	t.Helper()
	handler, err := application.NewRecordManualLodgementHandler(application.RecordManualLodgementDeps{
		Submissions: fixture.declaration.store, SendFacts: fixture.books, Readiness: fixture.declaration.readiness,
		Authority: fixture.declaration.authority, Lodgements: ledger, Clock: fixedClock{at: sendAt},
	})
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func lodgementCommand(fixture *sendFixture) application.RecordManualLodgementCommand {
	return application.RecordManualLodgementCommand{
		TenantID: fixture.tenant, Version: fixture.version.String(), LodgedAt: sendAt.Add(-2 * time.Hour),
		Reference: "SYN-CUSTOMS-ACK-001", Original: "SYN-LODGEMENT-SLIP-20261028", LodgedBy: "OPERATOR/https://issuer#alice",
	}
}

// Covers: dox/decisions.md「产品形态」——申报在线下递交之后记「已递交」：落的是与系统发送同一种发送事实，结果恒为待确认，
// 发送时刻取实际递交的时刻，通道消息号留空（受理号记在留痕上，不占技术回执的关联键）；另记一条留痕（递交人、递交凭据与受理号）。再记一次答已发过并交回原来那一笔；之后
// 系统那一路也不再发。
func TestAnOfflineLodgementIsRecordedAsTheSameSendFactWithItsOwnTrail(t *testing.T) {
	fixture := newSendFixture(t)
	fixture.registerChannel(t)
	ledger := &lodgementLedgerDouble{}
	handler := manualLodgementHandler(t, fixture, ledger)

	result, err := handler.Record(context.Background(), lodgementCommand(fixture))
	fact, has := result.Fact()
	if err != nil || result.Outcome() != application.ManualLodgementRecorded || !has {
		t.Fatalf("记线下递交：err=%v outcome=%q", err, result.Outcome())
	}
	if fact.Outcome() != domain.AttemptPendingConfirmation || !fact.SentAt().Equal(sendAt.Add(-2*time.Hour)) || fact.ChannelMessage() != "" ||
		fact.Response() != "MANUAL_LODGEMENT" {
		t.Fatalf("发送事实 = %+v", fact)
	}
	if len(ledger.lodgements) != 1 || ledger.lodgements[0].LodgedBy != "OPERATOR/https://issuer#alice" ||
		ledger.lodgements[0].Original != "SYN-LODGEMENT-SLIP-20261028" || ledger.lodgements[0].Version != fixture.version ||
		ledger.lodgements[0].Reference != "SYN-CUSTOMS-ACK-001" {
		t.Fatalf("留痕 = %+v", ledger.lodgements)
	}

	again, err := handler.Record(context.Background(), lodgementCommand(fixture))
	if stored, has := again.Fact(); err != nil || again.Outcome() != application.ManualLodgementAlreadySent || !has || !stored.SentAt().Equal(fact.SentAt()) ||
		len(ledger.lodgements) != 1 {
		t.Fatalf("再记一次：err=%v outcome=%q 留痕 %d 条", err, again.Outcome(), len(ledger.lodgements))
	}
	if sent, err := fixture.send.Send(context.Background(), fixture.tenant, fixture.version); err != nil || sent.Outcome() != application.DeclarationAlreadySent ||
		len(fixture.sender.packages) != 0 {
		t.Fatalf("线下递交之后系统再发：err=%v outcome=%q 发了 %d 次", err, sent.Outcome(), len(fixture.sender.packages))
	}
}

// Covers: 线下递交标记守的是系统发送的同一组门，外加人工录入自己的几条——没有递交凭据、递交时刻在未来、版本不在册
// 不受理；登记之后单元不再就绪或授权已失效不记；留痕落不下答未决；系统已经发过的不再记。
func TestAnOfflineLodgementIsHeldToTheSameGatesAsASystemSend(t *testing.T) {
	ctx := context.Background()
	for name, mutate := range map[string]func(*application.RecordManualLodgementCommand){
		"没有递交凭据":  func(command *application.RecordManualLodgementCommand) { command.Original = " " },
		"没有递交人":   func(command *application.RecordManualLodgementCommand) { command.LodgedBy = "" },
		"递交时刻在未来": func(command *application.RecordManualLodgementCommand) { command.LodgedAt = sendAt.Add(time.Minute) },
		"版本不在册":   func(command *application.RecordManualLodgementCommand) { command.Version = "SYN-VERSION-NOWHERE" },
		"递交早于成版": func(command *application.RecordManualLodgementCommand) {
			command.LodgedAt = time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC)
		},
	} {
		fixture := newSendFixture(t)
		ledger := &lodgementLedgerDouble{}
		command := lodgementCommand(fixture)
		mutate(&command)
		result, err := manualLodgementHandler(t, fixture, ledger).Record(ctx, command)
		if err != nil || result.Outcome() != application.ManualLodgementNotAccepted || len(fixture.books.facts) != 0 || len(ledger.lodgements) != 0 {
			t.Fatalf("%s：err=%v outcome=%q", name, err, result.Outcome())
		}
	}

	unready := newSendFixture(t)
	unready.declaration.readiness.revoked = true
	if result, err := manualLodgementHandler(t, unready, &lodgementLedgerDouble{}).Record(ctx, lodgementCommand(unready)); err != nil ||
		result.Outcome() != application.ManualLodgementHeld || len(unready.books.facts) != 0 {
		t.Fatalf("不再就绪：err=%v outcome=%q", err, result.Outcome())
	}
	revoked := newSendFixture(t)
	revoked.declaration.authority.revoked = true
	if result, err := manualLodgementHandler(t, revoked, &lodgementLedgerDouble{}).Record(ctx, lodgementCommand(revoked)); err != nil ||
		result.Outcome() != application.ManualLodgementHeld {
		t.Fatalf("授权已失效：err=%v outcome=%q", err, result.Outcome())
	}

	failing := newSendFixture(t)
	if result, err := manualLodgementHandler(t, failing, &lodgementLedgerDouble{err: errors.New("ledger down")}).Record(ctx, lodgementCommand(failing)); err != nil ||
		result.Outcome() != application.ManualLodgementUndecided {
		t.Fatalf("留痕落不下：err=%v outcome=%q", err, result.Outcome())
	}

	sentBySystem := newSendFixture(t)
	sentBySystem.registerChannel(t)
	if sent, err := sentBySystem.send.Send(ctx, sentBySystem.tenant, sentBySystem.version); err != nil || sent.Outcome() != application.DeclarationSent {
		t.Fatalf("系统先发：err=%v outcome=%q", err, sent.Outcome())
	}
	ledger := &lodgementLedgerDouble{}
	if result, err := manualLodgementHandler(t, sentBySystem, ledger).Record(ctx, lodgementCommand(sentBySystem)); err != nil ||
		result.Outcome() != application.ManualLodgementAlreadySent || len(ledger.lodgements) != 0 {
		t.Fatalf("系统已经发过再标记：err=%v outcome=%q", err, result.Outcome())
	}
}

type sendLockDouble struct{ locked []string }

func (double *sendLockDouble) LockSubmissionSend(_ context.Context, tenant domain.TenantID, version domain.SubmissionVersionID) error {
	double.locked = append(double.locked, tenant.String()+"|"+version.String())
	return nil
}

// Covers: 系统发送与线下递交标记都先取同一把锁再过门——没有它，系统那一路等通道答复的当口人来标记，申报实际发了
// 两次而册上只留一笔。这里验两条路都取了锁、键相同；真正排队由真库上的并发用例验。
func TestSystemSendAndOfflineLodgementTakeTheSameLockBeforeTheirGates(t *testing.T) {
	fixture := newSendFixture(t)
	fixture.registerChannel(t)
	lock := &sendLockDouble{}
	send, err := application.NewSendDeclarationHandler(application.SendDeclarationDeps{Submissions: fixture.declaration.store,
		Channels: fixture.books, SendFacts: fixture.books, Readiness: fixture.declaration.readiness, Authority: fixture.declaration.authority,
		Sender: fixture.sender, Clock: fixedClock{at: sendAt}, Lock: lock})
	if err != nil {
		t.Fatal(err)
	}
	manual, err := application.NewRecordManualLodgementHandler(application.RecordManualLodgementDeps{
		Submissions: fixture.declaration.store, SendFacts: fixture.books, Readiness: fixture.declaration.readiness,
		Authority: fixture.declaration.authority, Lodgements: &lodgementLedgerDouble{}, Clock: fixedClock{at: sendAt}, Lock: lock,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := send.Send(context.Background(), fixture.tenant, fixture.version); err != nil {
		t.Fatal(err)
	}
	if _, err := manual.Record(context.Background(), lodgementCommand(fixture)); err != nil {
		t.Fatal(err)
	}
	if len(lock.locked) != 2 || lock.locked[0] != lock.locked[1] || lock.locked[0] == "|" {
		t.Fatalf("锁 = %v，想要两条路各取一次、键相同", lock.locked)
	}
}
