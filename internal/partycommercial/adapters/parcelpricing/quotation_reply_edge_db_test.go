package parcelpricing_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 真库：两条不同答复同时写，一方落行，另一方答已有答复而不是库错误；
// 再交一次相同事实答已经记下并交回原条。纳秒时刻重放同样交回原条。
// 代录人带首尾空白不插行，也不是未处理的库错误。

func TestConcurrentDifferentRepliesLeaveOneAndAnswerAlreadyReplied(t *testing.T) {
	desk := newQuotationDesk(t)
	plan := desk.approve(t, "SYN-PLAN-REPLY-RACE", "12.00")
	sent, err := sendQuote(t, desk.db, desk.sender, quoteCommand(t, desk.tenant, desk.party, desk.entity, desk.sentAt, domain.QuotationPriceFromApprovedDraft, plan, plan.ContentDigest()))
	if err != nil || sent.Outcome != application.QuotationSent {
		t.Fatalf("发出 = %s %v", sent.Outcome, err)
	}
	inside := sent.Send.Validity().StartsAt().Add(time.Hour)
	recorderRef := quoteValue(t, domain.NewOperatorSubjectReference, "https://id.syn.example/dex#SYN-OPERATOR-02")
	accept := replyCommand(t, sent, inside, recorderRef)
	reject := accept
	reject.Decision = domain.QuotationReplyRejected
	reject.RejectionReason = "价格过高"

	var accepted, rejected application.RecordQuotationReplyResult
	firstErr, secondErr := raceQuotationTransactions(t, desk.pool, desk.db,
		func(txCtx context.Context) error {
			var handleErr error
			accepted, handleErr = desk.recorder.Handle(txCtx, accept)
			if handleErr != nil {
				return handleErr
			}
			if accepted.Outcome != application.QuotationReplyRecorded {
				return errQuote("先手 " + accepted.Outcome.String() + " " + accepted.Conclusion)
			}
			return nil
		},
		func(txCtx context.Context) error {
			var handleErr error
			rejected, handleErr = desk.recorder.Handle(txCtx, reject)
			return handleErr
		},
	)
	if firstErr != nil || secondErr != nil {
		t.Fatalf("并发答复：先手 %v，后手 %v", firstErr, secondErr)
	}
	if rejected.Outcome != application.QuotationReplyAlreadyReplied || rejected.Conclusion != "已有答复" {
		t.Fatalf("后手 = %s %s，想要已有答复", rejected.Outcome, rejected.Conclusion)
	}
	if !replyExists(t, desk.replies, desk.tenant, sent.Send.ID()) {
		t.Fatal("先手没有落行")
	}
	kept, found, err := desk.replies.GetQuotationReply(t.Context(), desk.tenant, sent.Send.ID())
	if err != nil || !found || kept.Decision() != domain.QuotationReplyAccepted {
		t.Fatalf("留下的不是先手那条接受：found %v decision %s err %v", found, kept.Decision(), err)
	}

	replayed, err := recordReply(t, desk.db, desk.recorder, accept)
	if err != nil || replayed.Outcome != application.QuotationReplyReplay || replayed.Conclusion != "已经记下" {
		t.Fatalf("相同事实再交 = %s %s %v", replayed.Outcome, replayed.Conclusion, err)
	}
	if replayed.Reply.RecordedBy() != accepted.Reply.RecordedBy() || replayed.Reply.Decision() != accepted.Reply.Decision() ||
		!replayed.Reply.RepliedAt().Equal(accepted.Reply.RepliedAt()) || !replayed.Reply.RecordedAt().Equal(accepted.Reply.RecordedAt()) {
		t.Fatal("没有交回原条")
	}
}

func TestNanosecondReplyTimeReplaysTheOriginalRow(t *testing.T) {
	desk := newQuotationDesk(t)
	plan := desk.approve(t, "SYN-PLAN-REPLY-NANO", "8.00")
	sent, err := sendQuote(t, desk.db, desk.sender, quoteCommand(t, desk.tenant, desk.party, desk.entity, desk.sentAt, domain.QuotationPriceFromApprovedDraft, plan, plan.ContentDigest()))
	if err != nil || sent.Outcome != application.QuotationSent {
		t.Fatalf("发出 = %s %v", sent.Outcome, err)
	}
	repliedAt := sent.Send.Validity().StartsAt().Add(time.Hour).Add(1500 * time.Nanosecond)
	command := replyCommand(t, sent, repliedAt, quoteValue(t, domain.NewOperatorSubjectReference, "https://id.syn.example/dex#SYN-OPERATOR-02"))
	command.RecordedAt = repliedAt.Add(time.Minute).Add(2500 * time.Nanosecond)
	first, err := recordReply(t, desk.db, desk.recorder, command)
	if err != nil || first.Outcome != application.QuotationReplyRecorded {
		t.Fatalf("写入 = %s %v", first.Outcome, err)
	}
	if !first.Reply.RepliedAt().Equal(repliedAt.UTC().Truncate(time.Microsecond)) {
		t.Fatalf("落下的客户时刻 = %s，想要截到微秒", first.Reply.RepliedAt())
	}
	again, err := recordReply(t, desk.db, desk.recorder, command)
	if err != nil || again.Outcome != application.QuotationReplyReplay || again.Conclusion != "已经记下" {
		t.Fatalf("再交 = %s %s %v", again.Outcome, again.Conclusion, err)
	}
	if again.Reply.RecordedBy() != first.Reply.RecordedBy() || again.Reply.Decision() != first.Reply.Decision() ||
		!again.Reply.RepliedAt().Equal(first.Reply.RepliedAt()) || !again.Reply.RecordedAt().Equal(first.Reply.RecordedAt()) {
		t.Fatal("没有交回原条")
	}
}

func TestRecorderSurroundingSpaceDoesNotInsert(t *testing.T) {
	desk := newQuotationDesk(t)
	plan := desk.approve(t, "SYN-PLAN-REPLY-SPACE", "6.00")
	sent, err := sendQuote(t, desk.db, desk.sender, quoteCommand(t, desk.tenant, desk.party, desk.entity, desk.sentAt, domain.QuotationPriceFromApprovedDraft, plan, plan.ContentDigest()))
	if err != nil || sent.Outcome != application.QuotationSent {
		t.Fatalf("发出 = %s %v", sent.Outcome, err)
	}
	command := replyCommand(t, sent, sent.Send.Validity().StartsAt().Add(time.Hour), quoteValue(t, domain.NewOperatorSubjectReference, "  https://id.syn.example/dex#SYN-OPERATOR-02  "))
	result, err := recordReply(t, desk.db, desk.recorder, command)
	if err != nil || result.Outcome != application.QuotationReplyNotAccepted || replyExists(t, desk.replies, desk.tenant, sent.Send.ID()) {
		t.Fatalf("代录人首尾空白 = %s %v，落行 %v", result.Outcome, err, replyExists(t, desk.replies, desk.tenant, sent.Send.ID()))
	}
}
