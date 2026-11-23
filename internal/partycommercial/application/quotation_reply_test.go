package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 代录按证据、客户时刻、摘要、撤下和是否已有答复分格。拒绝不落行。
// 读法不写库：一条被接受不改另一条发出的读法。

func TestAcceptingAQuotationRecordsTheEnvelopeRecorder(t *testing.T) {
	book := newReplyBook(t)
	sent := book.sendOne(t)
	repliedAt := sent.Send.Validity().StartsAt().Add(time.Hour)
	result, err := book.recorder.Handle(context.Background(), book.accept(t, sent, repliedAt))
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != application.QuotationReplyRecorded || result.Reply.RecordedBy().String() != "op-reply" {
		t.Fatalf("接受 = %s，代录人 %s", result.Outcome, result.Reply.RecordedBy())
	}
	if !result.Reply.RepliedAt().Equal(repliedAt) || result.Reply.EvidenceIndex().Text() != "SYN-EVIDENCE-01" {
		t.Fatal("客户时刻或证据索引没记下")
	}
	if book.replies.appends != 1 || book.store.appends != 1 {
		t.Fatalf("答复 %d 行，发出 %d 行", book.replies.appends, book.store.appends)
	}
}

func TestQuotationReplyRefusalsDoNotAppend(t *testing.T) {
	book := newReplyBook(t)
	sent := book.sendOne(t)
	inside := sent.Send.Validity().StartsAt().Add(time.Hour)
	cases := []struct {
		name    string
		mutate  func(*application.RecordQuotationReplyCommand)
		prepare func()
		outcome application.RecordQuotationReplyOutcome
	}{
		{name: "不带索引", mutate: func(command *application.RecordQuotationReplyCommand) { command.EvidenceIndex = "  " }, outcome: application.QuotationReplyNotAccepted},
		{name: "缺客户时刻", mutate: func(command *application.RecordQuotationReplyCommand) { command.RepliedAt = time.Time{} }, outcome: application.QuotationReplyNotAccepted},
		{name: "接受带原因", mutate: func(command *application.RecordQuotationReplyCommand) { command.RejectionReason = "不该有" }, outcome: application.QuotationReplyNotAccepted},
		{name: "有效期外", mutate: func(command *application.RecordQuotationReplyCommand) {
			command.RepliedAt = sent.Send.Validity().EndsAt()
		}, outcome: application.QuotationReplyExpired},
		{name: "摘要不符", mutate: func(command *application.RecordQuotationReplyCommand) { command.ContentDigest = "other" }, outcome: application.QuotationReplyContentChanged},
		{name: "草稿已撤下", prepare: func() { book.prices.facts["PLAN"] = standingFact(ports.QuotedPriceWithdrawn) }, outcome: application.QuotationReplyWithdrawn},
		{name: "没有这条发出", mutate: func(command *application.RecordQuotationReplyCommand) {
			command.QuotationID = commercialAppValue(t, domain.NewQuotationID, "QTS-MISSING")
		}, outcome: application.QuotationReplyNotFound},
		{name: "代录人首尾空白", mutate: func(command *application.RecordQuotationReplyCommand) {
			command.Recorder = commercialAppValue(t, domain.NewOperatorSubjectReference, "  op-reply  ")
		}, outcome: application.QuotationReplyNotAccepted},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			book.prices.facts["PLAN"] = readyFact()
			book.prices.err = nil
			if item.prepare != nil {
				item.prepare()
			}
			command := book.accept(t, sent, inside)
			if item.mutate != nil {
				item.mutate(&command)
			}
			before := book.replies.appends
			result, err := book.recorder.Handle(context.Background(), command)
			if err != nil {
				t.Fatal(err)
			}
			if result.Outcome != item.outcome || book.replies.appends != before {
				t.Fatalf("结果 = %s，行 %d→%d", result.Outcome, before, book.replies.appends)
			}
		})
	}
}

func TestADifferentReplyDoesNotReplaceTheFirst(t *testing.T) {
	book := newReplyBook(t)
	sent := book.sendOne(t)
	inside := sent.Send.Validity().StartsAt().Add(time.Hour)
	if _, err := book.recorder.Handle(context.Background(), book.accept(t, sent, inside)); err != nil {
		t.Fatal(err)
	}
	command := book.accept(t, sent, inside)
	command.Decision = domain.QuotationReplyRejected
	command.RejectionReason = "价格过高"
	result, err := book.recorder.Handle(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != application.QuotationReplyAlreadyReplied || book.replies.appends != 1 {
		t.Fatalf("不同答复 = %s，行 %d", result.Outcome, book.replies.appends)
	}
	stored, found, err := book.replies.GetQuotationReply(context.Background(), book.tenant, sent.Send.ID())
	if err != nil || !found || stored.Decision() != domain.QuotationReplyAccepted {
		t.Fatalf("原答复被换掉了：found %v decision %s err %v", found, stored.Decision(), err)
	}
}

func TestReplayingTheSameReplyReturnsTheOriginalRecorder(t *testing.T) {
	book := newReplyBook(t)
	sent := book.sendOne(t)
	inside := sent.Send.Validity().StartsAt().Add(time.Hour)
	first, err := book.recorder.Handle(context.Background(), book.accept(t, sent, inside))
	if err != nil {
		t.Fatal(err)
	}
	book.prices.facts["PLAN"] = standingFact(ports.QuotedPriceWithdrawn)
	again := book.accept(t, sent, inside)
	again.Recorder = commercialAppValue(t, domain.NewOperatorSubjectReference, "op-other")
	again.RecordedAt = inside.Add(2 * time.Hour)
	result, err := book.recorder.Handle(context.Background(), again)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != application.QuotationReplyReplay || book.replies.appends != 1 || result.Reply.RecordedBy() != first.Reply.RecordedBy() {
		t.Fatalf("重放 = %s，代录人 %s，行 %d", result.Outcome, result.Reply.RecordedBy(), book.replies.appends)
	}
}

func TestRejectionWithAnEmptyReasonIsRecorded(t *testing.T) {
	book := newReplyBook(t)
	sent := book.sendOne(t)
	command := book.accept(t, sent, sent.Send.Validity().StartsAt())
	command.Decision = domain.QuotationReplyRejected
	command.RejectionReason = "  "
	result, err := book.recorder.Handle(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != application.QuotationReplyRecorded || result.Reply.RejectionReason().Text() != "" || result.Reply.Decision() != domain.QuotationReplyRejected {
		t.Fatalf("拒绝 = %s，原因 %q", result.Outcome, result.Reply.RejectionReason().Text())
	}
}

func TestRegisteredCardReplyDoesNotReadWithdrawal(t *testing.T) {
	book := newReplyBook(t)
	sentAt := time.Date(2026, 11, 23, 4, 0, 0, 0, time.UTC)
	command := book.command(t, sentAt, "digest-sell", "")
	command.Lane = domain.QuotationPriceFromRegisteredCard
	sent, err := book.sender.Handle(context.Background(), command)
	if err != nil || sent.Outcome != application.QuotationSent {
		t.Fatalf("价卡发出 = %s %v", sent.Outcome, err)
	}
	book.prices.err = errors.New("price book down")
	result, err := book.recorder.Handle(context.Background(), book.accept(t, sent, sentAt.Add(time.Hour)))
	if err != nil || result.Outcome != application.QuotationReplyRecorded {
		t.Fatalf("价卡答复 = %s %v", result.Outcome, err)
	}
}

func TestQuotationReplyPriceReadFailureIsNotARefusal(t *testing.T) {
	book := newReplyBook(t)
	sent := book.sendOne(t)
	book.prices.err = errors.New("price book down")
	_, err := book.recorder.Handle(context.Background(), book.accept(t, sent, sent.Send.Validity().StartsAt()))
	if err == nil || book.replies.appends != 0 {
		t.Fatalf("err = %v，行 %d", err, book.replies.appends)
	}
}

func TestQuotationReadingDerivesWithoutWriting(t *testing.T) {
	book := newReplyBook(t)
	sent := book.sendOne(t)
	other := book.sendOne(t)
	inside := sent.Send.Validity().StartsAt().Add(time.Hour)
	if _, err := book.recorder.Handle(context.Background(), book.accept(t, sent, inside)); err != nil {
		t.Fatal(err)
	}
	before := sent.Send
	book.prices.facts["PLAN"] = standingFact(ports.QuotedPriceWithdrawn)
	reading, err := book.reader.Get(context.Background(), book.tenant, sent.Send.ID(), inside)
	if err != nil {
		t.Fatal(err)
	}
	if reading.Status != domain.QuotationReadingAccepted || !reading.ReferencedDraftWithdrawn || reading.Conclusion != "已接受。所引草稿此刻已撤下。" {
		t.Fatalf("已接受之后撤下 = %s %v %s", reading.Status, reading.ReferencedDraftWithdrawn, reading.Conclusion)
	}
	if !reading.Send.SameFact(before) || book.replies.appends != 1 || book.store.appends != 2 {
		t.Fatal("读改了发出或答复")
	}
	otherReading, err := book.reader.Get(context.Background(), book.tenant, other.Send.ID(), inside)
	if err != nil || otherReading.Status != domain.QuotationReadingPriceWithdrawn {
		t.Fatalf("另一条 = %s %v", otherReading.Status, err)
	}
	book.prices.facts["PLAN"] = readyFact()
	expired, err := book.reader.Get(context.Background(), book.tenant, other.Send.ID(), other.Send.Validity().EndsAt())
	if err != nil || expired.Status != domain.QuotationReadingExpired || expired.HasReply {
		t.Fatalf("过期 = %s hasReply %v %v", expired.Status, expired.HasReply, err)
	}
	if !expired.Send.SameFact(other.Send) || book.replies.appends != 1 {
		t.Fatal("过期的读法改了库")
	}
}

func TestQuotationReadingPriceFailureIsAnError(t *testing.T) {
	book := newReplyBook(t)
	sent := book.sendOne(t)
	book.prices.err = errors.New("draft book down")
	_, err := book.reader.Get(context.Background(), book.tenant, sent.Send.ID(), sent.Send.Validity().StartsAt())
	if err == nil {
		t.Fatal("读不动草稿册应当是错")
	}
}

func TestRejectedReadingDoesNotNeedThePriceBook(t *testing.T) {
	book := newReplyBook(t)
	sent := book.sendOne(t)
	command := book.accept(t, sent, sent.Send.Validity().StartsAt())
	command.Decision = domain.QuotationReplyRejected
	if _, err := book.recorder.Handle(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	book.prices.err = errors.New("draft book down")
	reading, err := book.reader.Get(context.Background(), book.tenant, sent.Send.ID(), sent.Send.Validity().EndsAt())
	if err != nil || reading.Status != domain.QuotationReadingRejected {
		t.Fatalf("已拒绝 = %s %v", reading.Status, err)
	}
}

type replyBook struct {
	*quotationBook
	replies  *fakeReplyStore
	recorder *application.RecordQuotationReplyHandler
	reader   *application.ReadQuotationHandler
}

func newReplyBook(t *testing.T) *replyBook {
	t.Helper()
	base := newQuotationBook(t)
	replies := &fakeReplyStore{}
	recorder, err := application.NewRecordQuotationReplyHandler(base.store, replies, base.prices)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := application.NewReadQuotationHandler(base.store, replies, base.prices, nil)
	if err != nil {
		t.Fatal(err)
	}
	return &replyBook{quotationBook: base, replies: replies, recorder: recorder, reader: reader}
}

func (book *replyBook) sendOne(t *testing.T) application.SendQuotationResult {
	t.Helper()
	sentAt := time.Date(2026, 11, 23, 4, 0, 0, 0, time.UTC)
	result, err := book.sender.Handle(context.Background(), book.command(t, sentAt, "digest-sell", ""))
	if err != nil || result.Outcome != application.QuotationSent {
		t.Fatalf("发出 = %s %v", result.Outcome, err)
	}
	return result
}

func (book *replyBook) accept(t *testing.T, sent application.SendQuotationResult, at time.Time) application.RecordQuotationReplyCommand {
	t.Helper()
	return application.RecordQuotationReplyCommand{
		Tenant: book.tenant, QuotationID: sent.Send.ID(), Decision: domain.QuotationReplyAccepted,
		RepliedAt: at, EvidenceIndex: "SYN-EVIDENCE-01", ContentDigest: sent.Send.ContentDigest(),
		Recorder:   commercialAppValue(t, domain.NewOperatorSubjectReference, "op-reply"),
		RecordedAt: at.Add(time.Minute),
	}
}

type fakeReplyStore struct {
	rows    map[string]domain.QuotationReply
	appends int
}

func (store *fakeReplyStore) AppendQuotationReply(_ context.Context, reply domain.QuotationReply) (ports.QuotationReplyAppendOutcome, error) {
	if store.rows == nil {
		store.rows = map[string]domain.QuotationReply{}
	}
	key := reply.Tenant().String() + "/" + reply.QuotationID().String()
	if existing, found := store.rows[key]; found {
		if existing.SameFact(reply) {
			return ports.QuotationReplyAppendReplay, nil
		}
		return ports.QuotationReplyAppendConflict, nil
	}
	store.rows[key] = reply
	store.appends++
	return ports.QuotationReplyAppended, nil
}

func (store *fakeReplyStore) GetQuotationReply(_ context.Context, tenant domain.TenantID, id domain.QuotationID) (domain.QuotationReply, bool, error) {
	reply, found := store.rows[tenant.String()+"/"+id.String()]
	return reply, found, nil
}
