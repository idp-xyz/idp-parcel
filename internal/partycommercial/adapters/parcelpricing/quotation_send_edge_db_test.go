package parcelpricing_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 发出记录与答复同一写法：两笔不同内容同时写，一方落行，另一方答冲突而不是库错误；
// 再交一次相同事实交回原条。纳秒时刻重放同样交回原条。

func TestConcurrentDifferentSendsLeaveOneAndAnswerConflict(t *testing.T) {
	desk := newQuotationDesk(t)
	plan := desk.approve(t, "SYN-PLAN-SEND-RACE", "15.00")
	id := quoteValue(t, domain.NewQuotationID, "QTS-SEND-RACE")
	firstCommand := quoteCommand(t, desk.tenant, desk.party, desk.entity, desk.sentAt, domain.QuotationPriceFromApprovedDraft, plan, plan.ContentDigest())
	firstCommand.ID = id
	firstCommand.Terms = "甲条款"
	secondCommand := firstCommand
	secondCommand.Terms = "乙条款"

	var sent, conflicted application.SendQuotationResult
	firstErr, secondErr := raceQuotationTransactions(t, desk.pool, desk.db,
		func(txCtx context.Context) error {
			var handleErr error
			sent, handleErr = desk.sender.Handle(txCtx, firstCommand)
			if handleErr != nil {
				return handleErr
			}
			if sent.Outcome != application.QuotationSent {
				return errQuote("先手 " + sent.Outcome.String() + " " + sent.Conclusion)
			}
			return nil
		},
		func(txCtx context.Context) error {
			var handleErr error
			conflicted, handleErr = desk.sender.Handle(txCtx, secondCommand)
			return handleErr
		},
	)
	if firstErr != nil || secondErr != nil {
		t.Fatalf("并发发出：先手 %v，后手 %v", firstErr, secondErr)
	}
	if conflicted.Outcome != application.QuotationSendConflict || conflicted.Conclusion != "这个报价号已经发出过不同的内容" {
		t.Fatalf("后手 = %s %s，想要内容冲突", conflicted.Outcome, conflicted.Conclusion)
	}
	if quotationRows(t, desk.sends, desk.tenant) != 1 {
		t.Fatalf("发出行数 = %d，想要 1", quotationRows(t, desk.sends, desk.tenant))
	}
	stored, found, err := desk.sends.GetQuotationSend(t.Context(), desk.tenant, id)
	if err != nil || !found || stored.Terms().Text() != "甲条款" {
		t.Fatalf("留下的不是先手：found %v terms %q err %v", found, stored.Terms().Text(), err)
	}

	replayed, err := sendQuote(t, desk.db, desk.sender, firstCommand)
	if err != nil || replayed.Outcome != application.QuotationSendReplay || !replayed.Send.SameFact(sent.Send) {
		t.Fatalf("相同事实再交 = %s %v，同一条 %v", replayed.Outcome, err, replayed.Send.SameFact(sent.Send))
	}
	if quotationRows(t, desk.sends, desk.tenant) != 1 {
		t.Fatal("重放又插了一行")
	}
}

func TestNanosecondSendTimeReplaysTheOriginalRow(t *testing.T) {
	desk := newQuotationDesk(t)
	plan := desk.approve(t, "SYN-PLAN-SEND-NANO", "9.50")
	sentAt := desk.sentAt.Add(1500 * time.Nanosecond)
	command := quoteCommand(t, desk.tenant, desk.party, desk.entity, sentAt, domain.QuotationPriceFromApprovedDraft, plan, plan.ContentDigest())
	command.ID = quoteValue(t, domain.NewQuotationID, "QTS-SEND-NANO")
	command.StartsAt = sentAt
	command.EndsAt = sentAt.Add(240 * time.Hour).Add(1800 * time.Nanosecond)
	first, err := sendQuote(t, desk.db, desk.sender, command)
	if err != nil || first.Outcome != application.QuotationSent {
		t.Fatalf("写入 = %s %v", first.Outcome, err)
	}
	if !first.Send.SentAt().Equal(sentAt.UTC().Truncate(time.Microsecond)) {
		t.Fatalf("落下的发出时刻 = %s，想要截到微秒", first.Send.SentAt())
	}
	again, err := sendQuote(t, desk.db, desk.sender, command)
	if err != nil || again.Outcome != application.QuotationSendReplay || !again.Send.SameFact(first.Send) {
		t.Fatalf("再交 = %s %v，同一条 %v", again.Outcome, err, again.Send.SameFact(first.Send))
	}
	if quotationRows(t, desk.sends, desk.tenant) != 1 {
		t.Fatal("重放又插了一行")
	}
	stored, found, err := desk.sends.GetQuotationSend(t.Context(), desk.tenant, command.ID)
	if err != nil || !found || !stored.SameFact(first.Send) {
		t.Fatal("没有交回原条")
	}
}
