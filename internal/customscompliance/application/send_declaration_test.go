package application_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 本文件是申报发送（SendDeclarationHandler）与技术回执接收（ReceiveTechnicalReceiptHandler）的用例。提交版本经真提交编排落进
// submit_declaration_test.go 的替身册；通道册、发送事实册与回执册是这里的三本内存册，代数照真库：插不进按键比对。

var sendAt = time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)

type sendBooksDouble struct {
	channels map[string]domain.DeclarationChannel
	facts    map[string]domain.SubmissionSendFact
	receipts map[string]domain.TechnicalReceipt
	failRead error
}

func newSendBooks() *sendBooksDouble {
	return &sendBooksDouble{channels: map[string]domain.DeclarationChannel{}, facts: map[string]domain.SubmissionSendFact{},
		receipts: map[string]domain.TechnicalReceipt{}}
}

func (books *sendBooksDouble) SaveDeclarationChannel(_ context.Context, _ domain.TenantID, channel domain.DeclarationChannel,
	_ time.Time) (ports.SubmissionSendEffect, error) {
	books.channels[channel.Target()] = channel
	return ports.SubmissionSendRegistered, nil
}

func (books *sendBooksDouble) LoadDeclarationChannel(_ context.Context, _ domain.TenantID, target string) (domain.DeclarationChannel, bool, error) {
	if books.failRead != nil {
		return domain.DeclarationChannel{}, false, books.failRead
	}
	channel, found := books.channels[target]
	return channel, found, nil
}

func factKey(version domain.SubmissionVersionID, sequence int) string {
	return fmt.Sprintf("%s/%d", version, sequence)
}

func (books *sendBooksDouble) SaveSendFact(_ context.Context, _ domain.TenantID, fact domain.SubmissionSendFact,
	_ time.Time) (ports.SubmissionSendEffect, error) {
	key := factKey(fact.Version(), fact.Sequence())
	if stored, found := books.facts[key]; found {
		if stored.Outcome() == fact.Outcome() && stored.ChannelMessage() == fact.ChannelMessage() {
			return ports.SubmissionSendReplayed, nil
		}
		return ports.SubmissionSendConflict, nil
	}
	// 照真库的部分唯一索引：同租户里一个消息号只指一份尝试。
	for _, stored := range books.facts {
		if fact.ChannelMessage() != "" && stored.ChannelMessage() == fact.ChannelMessage() {
			return ports.SubmissionSendConflict, nil
		}
	}
	books.facts[key] = fact
	return ports.SubmissionSendRegistered, nil
}

func (books *sendBooksDouble) FindSendFact(_ context.Context, _ domain.TenantID, version domain.SubmissionVersionID,
	sequence int) (domain.SubmissionSendFact, bool, error) {
	fact, found := books.facts[factKey(version, sequence)]
	return fact, found, nil
}

func (books *sendBooksDouble) FindSendFactByChannelMessage(_ context.Context, _ domain.TenantID, message string) (domain.SubmissionSendFact, bool, error) {
	for _, fact := range books.facts {
		if fact.ChannelMessage() == message {
			return fact, true, nil
		}
	}
	return domain.SubmissionSendFact{}, false, nil
}

func (books *sendBooksDouble) SaveTechnicalReceipt(_ context.Context, _ domain.TenantID, receipt domain.TechnicalReceipt,
	_ time.Time) (ports.SubmissionSendEffect, error) {
	if stored, found := books.receipts[receipt.Source()]; found {
		if stored.Digest() == receipt.Digest() {
			return ports.SubmissionSendReplayed, nil
		}
		return ports.SubmissionSendConflict, nil
	}
	books.receipts[receipt.Source()] = receipt
	return ports.SubmissionSendRegistered, nil
}

func (books *sendBooksDouble) ListTechnicalReceipts(_ context.Context, _ domain.TenantID, version domain.SubmissionVersionID,
	sequence int) ([]domain.TechnicalReceipt, error) {
	var receipts []domain.TechnicalReceipt
	for _, receipt := range books.receipts {
		if receipt.Version() == version && receipt.Sequence() == sequence {
			receipts = append(receipts, receipt)
		}
	}
	return receipts, nil
}

type senderDouble struct {
	answer   application.ChannelAnswer
	err      error
	packages []application.DeclarationPackage
}

func (sender *senderDouble) SendDeclaration(_ context.Context, _ domain.DeclarationChannel,
	pkg application.DeclarationPackage) (application.ChannelAnswer, error) {
	sender.packages = append(sender.packages, pkg)
	return sender.answer, sender.err
}

type sendFixture struct {
	declaration *declarationFixture
	books       *sendBooksDouble
	sender      *senderDouble
	send        *application.SendDeclarationHandler
	receive     *application.ReceiveTechnicalReceiptHandler
	tenant      domain.TenantID
	version     domain.SubmissionVersionID
}

// newSendFixture 先经真提交编排落一版首次尝试为已登记待发送的提交，再装发送与接收两只编排。
func newSendFixture(t *testing.T) *sendFixture {
	t.Helper()
	declaration := newDeclarationFixture(t)
	command := declarationCommand(t)
	command.InitialResult, command.SentAt = domain.AttemptRegisteredPendingSend, time.Time{}
	result, err := declaration.handler.Handle(context.Background(), command)
	if err != nil || result.Outcome() != application.DeclarationSubmitted {
		t.Fatalf("预置提交：outcome=%v err=%v", result.Outcome(), err)
	}
	record, _ := result.Record()
	fixture := &sendFixture{declaration: declaration, books: newSendBooks(),
		sender: &senderDouble{answer: application.ChannelAnswer{Answered: true, Accepted: true, Message: "MSG-1"}},
		tenant: command.TenantID, version: record.Version.ID()}
	send, err := application.NewSendDeclarationHandler(application.SendDeclarationDeps{Submissions: declaration.store,
		Channels: fixture.books, SendFacts: fixture.books, Readiness: declaration.readiness, Authority: declaration.authority,
		Sender: fixture.sender, Clock: fixedClock{at: sendAt}})
	if err != nil {
		t.Fatal(err)
	}
	receive, err := application.NewReceiveTechnicalReceiptHandler(application.ReceiveTechnicalReceiptDeps{
		SendFacts: fixture.books, Receipts: fixture.books, Clock: fixedClock{at: sendAt}})
	if err != nil {
		t.Fatal(err)
	}
	fixture.send, fixture.receive = send, receive
	return fixture
}

func (fixture *sendFixture) registerChannel(t *testing.T) {
	t.Helper()
	channel, err := domain.NewDeclarationChannel("customs-channel-1", "https://broker.example.invalid/declarations", "")
	if err != nil {
		t.Fatal(err)
	}
	fixture.books.channels[channel.Target()] = channel
}

// Covers: AT-CC-454（发送编排半边）
func TestAPendingSendAttemptGoesOutOnceThroughItsChannel(t *testing.T) {
	fixture := newSendFixture(t)
	ctx := context.Background()

	result, err := fixture.send.Send(ctx, fixture.tenant, fixture.version)
	if err != nil || result.Outcome() != application.DeclarationChannelUnconfigured || len(fixture.sender.packages) != 0 {
		t.Fatalf("没登通道 = %v err = %v 发了 %d 次", result.Outcome(), err, len(fixture.sender.packages))
	}

	fixture.registerChannel(t)
	result, err = fixture.send.Send(ctx, fixture.tenant, fixture.version)
	if err != nil || result.Outcome() != application.DeclarationSent || result.Fact().Outcome() != domain.AttemptAcknowledged ||
		result.Fact().ChannelMessage() != "MSG-1" {
		t.Fatalf("首发 = %v %+v err = %v", result.Outcome(), result.Fact(), err)
	}
	if key := fixture.sender.packages[0].IdempotencyKey; key != "tenant-1/"+fixture.version.String()+"/1" {
		t.Fatalf("幂等键 = %q", key)
	}
	result, err = fixture.send.Send(ctx, fixture.tenant, fixture.version)
	if err != nil || result.Outcome() != application.DeclarationAlreadySent || len(fixture.sender.packages) != 1 {
		t.Fatalf("重投 = %v err = %v 发了 %d 次", result.Outcome(), err, len(fixture.sender.packages))
	}
}

// Covers: AT-CC-454（编排半边：只有确知没发才重投）
func TestOnlyACertainNonSendIsRetried(t *testing.T) {
	ctx := context.Background()

	unanswered := newSendFixture(t)
	unanswered.registerChannel(t)
	unanswered.sender.answer = application.ChannelAnswer{Response: "HTTP 502"}
	result, err := unanswered.send.Send(ctx, unanswered.tenant, unanswered.version)
	if err != nil || result.Outcome() != application.DeclarationSent || result.Fact().Outcome() != domain.AttemptPendingConfirmation {
		t.Fatalf("没拿到答复应记待确认 = %v %v err = %v", result.Outcome(), result.Fact().Outcome(), err)
	}

	rejected := newSendFixture(t)
	rejected.registerChannel(t)
	rejected.sender.answer = application.ChannelAnswer{Answered: true, Message: "MSG-R"}
	if result, _ := rejected.send.Send(ctx, rejected.tenant, rejected.version); result.Fact().Outcome() != domain.AttemptFailed {
		t.Fatalf("拒收 = %v", result.Fact().Outcome())
	}

	notSent := newSendFixture(t)
	notSent.registerChannel(t)
	notSent.sender.err = application.ErrDeclarationNotSent
	result, err = notSent.send.Send(ctx, notSent.tenant, notSent.version)
	if err != nil || result.Outcome() != application.DeclarationSendUndecided || len(notSent.books.facts) != 0 {
		t.Fatalf("确知没发 = %v err = %v facts = %d", result.Outcome(), err, len(notSent.books.facts))
	}

	noCredential := newSendFixture(t)
	noCredential.registerChannel(t)
	noCredential.sender.err = application.ErrDeclarationCredentialMissing
	if result, _ := noCredential.send.Send(ctx, noCredential.tenant, noCredential.version); result.Outcome() != application.DeclarationChannelUnconfigured ||
		len(noCredential.books.facts) != 0 {
		t.Fatalf("凭据缺值 = %v", result.Outcome())
	}

	unready := newSendFixture(t)
	unready.registerChannel(t)
	unready.declaration.readiness.revoked = true
	if result, _ := unready.send.Send(ctx, unready.tenant, unready.version); result.Outcome() != application.DeclarationSendHeld ||
		len(unready.sender.packages) != 0 {
		t.Fatalf("登记之后不再就绪 = %v", result.Outcome())
	}
	revoked := newSendFixture(t)
	revoked.registerChannel(t)
	revoked.declaration.authority.revoked = true
	if result, _ := revoked.send.Send(ctx, revoked.tenant, revoked.version); result.Outcome() != application.DeclarationSendHeld ||
		len(revoked.sender.packages) != 0 {
		t.Fatalf("登记之后授权撤了 = %v", result.Outcome())
	}

	// 通道把已经给过另一份尝试的消息号又给了这一份：照样记下发送事实（不带消息号），不回滚、不重投。
	taken := newSendFixture(t)
	taken.registerChannel(t)
	other, _ := domain.NewSubmissionVersionID("SYN-OTHER-VERSION")
	earlier, _ := domain.RecordSubmissionSend(other, 1, sendAt, domain.AttemptAcknowledged, "MSG-1", "")
	taken.books.facts[fmt.Sprintf("%s/%d", other, 1)] = earlier
	result, err = taken.send.Send(ctx, taken.tenant, taken.version)
	if err != nil || result.Outcome() != application.DeclarationSent || result.Fact().ChannelMessage() != "" ||
		result.Fact().Outcome() != domain.AttemptAcknowledged {
		t.Fatalf("消息号已被占 = %v %+v err = %v", result.Outcome(), result.Fact(), err)
	}

	// 对端给了超长的消息号：同样不带它记，不让这一笔写不进库而回滚重发。
	oversized := newSendFixture(t)
	oversized.registerChannel(t)
	oversized.sender.answer = application.ChannelAnswer{Answered: true, Accepted: true, Message: strings.Repeat("M", 4096)}
	result, err = oversized.send.Send(ctx, oversized.tenant, oversized.version)
	if err != nil || result.Outcome() != application.DeclarationSent || result.Fact().ChannelMessage() != "" {
		t.Fatalf("超长消息号 = %v %+v err = %v", result.Outcome(), result.Fact(), err)
	}

	unreadable := newSendFixture(t)
	unreadable.books.failRead = errors.New("down")
	if result, _ := unreadable.send.Send(ctx, unreadable.tenant, unreadable.version); result.Outcome() != application.DeclarationSendUndecided {
		t.Fatalf("通道册读不动 = %v", result.Outcome())
	}
}

// Covers: AT-CC-455（接收编排半边）
func TestTechnicalReceiptsAttachByChannelMessageAndFoldTheCurrentJudgment(t *testing.T) {
	fixture := newSendFixture(t)
	fixture.registerChannel(t)
	fixture.sender.answer = application.ChannelAnswer{Response: "timeout"}
	ctx := context.Background()
	if _, err := fixture.send.Send(ctx, fixture.tenant, fixture.version); err != nil {
		t.Fatal(err)
	}
	// 待确认那一笔没有消息号，任何回执都关联不上。
	receipt := application.ReceiveTechnicalReceiptCommand{TenantID: fixture.tenant, Source: "RCPT-1", ChannelMessage: "MSG-1",
		Outcome: domain.AttemptAcknowledged, OccurredAt: sendAt, ReceivedAt: sendAt.Add(time.Minute), Digest: "TR-1:a"}
	if result, _ := fixture.receive.Handle(ctx, receipt); result.Outcome() != application.TechnicalReceiptUnattributable {
		t.Fatalf("无消息号 = %v", result.Outcome())
	}

	fixture = newSendFixture(t)
	fixture.registerChannel(t)
	fixture.sender.answer = application.ChannelAnswer{Answered: true, Accepted: true, Message: "MSG-1"}
	if _, err := fixture.send.Send(ctx, fixture.tenant, fixture.version); err != nil {
		t.Fatal(err)
	}
	failed := receipt
	failed.Outcome = domain.AttemptFailed
	result, err := fixture.receive.Handle(ctx, failed)
	if err != nil || result.Outcome() != application.TechnicalReceiptRecorded || result.Current() != domain.AttemptFailed {
		t.Fatalf("校验未过 = %v %v err = %v", result.Outcome(), result.Current(), err)
	}
	if version, sequence := result.Attempt(); version != fixture.version || sequence != 1 {
		t.Fatalf("关联到 %v/%d", version, sequence)
	}
	if result, _ := fixture.receive.Handle(ctx, failed); result.Outcome() != application.TechnicalReceiptExisting ||
		result.Current() != domain.AttemptFailed {
		t.Fatalf("同内容重到 = %v %v", result.Outcome(), result.Current())
	}
	conflicting := failed
	conflicting.Digest = "TR-1:b"
	if result, _ := fixture.receive.Handle(ctx, conflicting); result.Outcome() != application.TechnicalReceiptConflict {
		t.Fatalf("同身份异内容 = %v", result.Outcome())
	}
	later := receipt
	later.Source, later.ReceivedAt, later.Digest = "RCPT-2", sendAt.Add(2*time.Minute), "TR-1:c"
	if result, _ := fixture.receive.Handle(ctx, later); result.Current() != domain.AttemptAcknowledged {
		t.Fatalf("后到的明确回执改当前判断 = %v", result.Current())
	}
	if len(fixture.books.receipts) != 2 {
		t.Fatalf("回执不覆盖、逐份追加：%d", len(fixture.books.receipts))
	}
	if result, _ := fixture.receive.Handle(ctx, application.ReceiveTechnicalReceiptCommand{TenantID: fixture.tenant}); result.Outcome() != application.TechnicalReceiptNotAccepted {
		t.Fatalf("缺消息号 = %v", result.Outcome())
	}
}
