package application_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// 已批准的价卡草稿可以撤下。撤下人取命令里的操作者主体，时刻取时钟，原因去首尾空白。尚未批准与已发布各答各的，
// 已撤下再撤不覆盖痕迹。发布已撤下的草稿答已撤下，不交给登记。

func newWithdrawHandler(t *testing.T, drafts *draftProgressFake) *application.WithdrawPriceCardDraftHandler {
	t.Helper()
	handler, err := application.NewWithdrawPriceCardDraftHandler(drafts, fixedClock{at: progressClockAt.Add(time.Minute)})
	if err != nil {
		t.Fatalf("构造撤下编排：%v", err)
	}
	return handler
}

func withdraw(t *testing.T, handler *application.WithdrawPriceCardDraftHandler, plan domain.VersionReference, reason string) application.WithdrawPriceCardDraftResult {
	t.Helper()
	result, err := handler.Handle(context.Background(), application.WithdrawPriceCardDraftCommand{
		Tenant: draftTenant(t), Plan: plan, Withdrawer: subject(t, "https://id.syn.example/dex#SYN-OPERATOR-03"), Reason: reason,
	})
	if err != nil {
		t.Fatalf("撤下编排：%v", err)
	}
	return result
}

func TestAnApprovedDraftIsWithdrawnWithoutAnApprovalDutyRule(t *testing.T) {
	approved := approvedPriceCardDraft(t)
	drafts := newDraftProgressFake(approved)
	result := withdraw(t, newWithdrawHandler(t, drafts), approved.Plan(), "  客户拒绝这套价  ")
	if result.Outcome != application.PriceCardDraftWithdrawn || result.Outcome.String() != "DRAFT_WITHDRAWN" || !result.HasDraft {
		t.Fatalf("outcome = %v has=%v，want DRAFT_WITHDRAWN", result.Outcome, result.HasDraft)
	}
	if result.Draft.Status() != domain.PriceCardDraftStatusWithdrawn || len(drafts.advanced) != 1 {
		t.Fatalf("没写下撤下：status=%v advanced=%d", result.Draft.Status(), len(drafts.advanced))
	}
	who, _ := result.Draft.Withdrawer()
	when, _ := result.Draft.WithdrawnAt()
	reason, _ := result.Draft.WithdrawalReason()
	if who != "https://id.syn.example/dex#SYN-OPERATOR-03" || !when.Equal(progressClockAt.Add(time.Minute)) || reason != "客户拒绝这套价" {
		t.Fatalf("痕迹 = %q %v %q", who, when, reason)
	}
	if approver, _ := result.Draft.Approver(); approver != draftApprover {
		t.Fatalf("批准痕迹被换掉：%q", approver)
	}
	content, ok := result.Draft.Content()
	approvedContent, _ := approved.Content()
	if !ok || content.Plan.ContentDigest() != approvedContent.Plan.ContentDigest() {
		t.Fatal("撤下丢掉了批准时的内容")
	}

	again := withdraw(t, newWithdrawHandler(t, drafts), approved.Plan(), "另一条原因")
	if again.Outcome.String() != "DRAFT_WITHDRAWN" || len(drafts.advanced) != 1 {
		t.Fatalf("再撤覆盖了痕迹：outcome=%v advanced=%d", again.Outcome, len(drafts.advanced))
	}
	if kept, _ := again.Draft.WithdrawalReason(); kept != "客户拒绝这套价" {
		t.Fatalf("再撤改了原因：%q", kept)
	}
}

func TestWithdrawalAnswersTheCellTheDraftIsIn(t *testing.T) {
	validated := validatedPriceCardDraft(t)
	published, err := approvedPriceCardDraft(t).MarkPublished(progressClockAt.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name  string
		draft domain.PriceCardDraft
		want  string
	}{
		{"草稿", problemPriceCardDraft(t, validated.Plan()), "DRAFT_NOT_APPROVED"},
		{"已校验", validated, "DRAFT_NOT_APPROVED"},
		{"已发布", published, "DRAFT_PUBLISHED"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			drafts := newDraftProgressFake(test.draft)
			result := withdraw(t, newWithdrawHandler(t, drafts), test.draft.Plan(), "客户拒绝这套价")
			if result.Outcome.String() != test.want || result.HasDraft || len(drafts.advanced) != 0 {
				t.Fatalf("outcome = %v has=%v advanced=%d，want %s 且不写", result.Outcome, result.HasDraft, len(drafts.advanced), test.want)
			}
		})
	}

	missing := withdraw(t, newWithdrawHandler(t, newDraftProgressFake()), validated.Plan(), "客户拒绝这套价")
	if missing.Outcome.String() != "DRAFT_NOT_FOUND" {
		t.Fatalf("册上没有：%v", missing.Outcome)
	}
	approved := approvedPriceCardDraft(t)
	for _, reason := range []string{"", "   ", "客户\n拒绝"} {
		drafts := newDraftProgressFake(approved)
		refused := withdraw(t, newWithdrawHandler(t, drafts), approved.Plan(), reason)
		if refused.Outcome.String() != "NOT_ACCEPTED" || len(drafts.advanced) != 0 {
			t.Fatalf("原因 %q：%v advanced=%d，want NOT_ACCEPTED 且不写", reason, refused.Outcome, len(drafts.advanced))
		}
	}
}

func TestPublishingAWithdrawnDraftDoesNotEnterTheRegister(t *testing.T) {
	approved := approvedPriceCardDraft(t)
	withdrawn, err := approved.Withdraw(subject(t, "https://id.syn.example/dex#SYN-OPERATOR-03"), "客户拒绝这套价", progressClockAt.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	catalog := &priceCardCatalogDouble{outcome: ports.PriceCardRegistered}
	result, err := newPublishHandler(t, newDraftProgressFake(withdrawn), catalog).Handle(context.Background(),
		application.PublishPriceCardDraftCommand{Tenant: draftTenant(t), Plan: withdrawn.Plan()})
	if err != nil {
		t.Fatalf("发布编排：%v", err)
	}
	if result.Outcome != application.PriceCardPublicationDraftWithdrawn || result.Outcome.String() != "DRAFT_WITHDRAWN" ||
		result.HasRegistration || len(catalog.registered) != 0 {
		t.Fatalf("outcome = %v registration=%v 登记收到 %d 份，want DRAFT_WITHDRAWN 且不入册", result.Outcome, result.HasRegistration, len(catalog.registered))
	}
}

func TestApprovingAWithdrawnDraftAnswersWithdrawn(t *testing.T) {
	approved := approvedPriceCardDraft(t)
	withdrawn, err := approved.Withdraw(subject(t, "https://id.syn.example/dex#SYN-OPERATOR-03"), "客户拒绝这套价", progressClockAt.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	rules := configuredRule(t, false, "")
	result := approve(t, newApproveHandler(t, newDraftProgressFake(withdrawn), rules), withdrawn.Plan(), subject(t, draftApprover))
	if result.Outcome.String() != "DRAFT_WITHDRAWN" || rules.reads != 0 {
		t.Fatalf("outcome = %v 规则读了 %d 次，want DRAFT_WITHDRAWN 且不套审批职责规则", result.Outcome, rules.reads)
	}
}
