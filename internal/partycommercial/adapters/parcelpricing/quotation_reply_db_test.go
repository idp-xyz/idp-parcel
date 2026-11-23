package parcelpricing_test

import (
	"context"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pricingpostgres "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/pricecardtemplate"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	ppports "go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	quotedprice "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/parcelpricing"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 真库：不带索引、有效期外、摘要不符、已撤下都不落行；接受记下代录人；
// 不同答复不替换；过了终点还没答复的读出来是已过期，发出那一行不改。

func TestQuotationReplyOnTheRealDatabase(t *testing.T) {
	db := newQuotationDB(t)
	tenant := quoteValue(t, domain.NewTenantID, "SYN-TENANT-01")
	party := quoteValue(t, domain.NewPartyID, "SYN-PARTY-SHIPPER-01")
	entity := quoteValue(t, domain.NewLegalEntityReference, "SYN-LE-01")
	sentAt := time.Date(2026, 11, 23, 4, 0, 0, 0, time.UTC)
	mustQuotationTx(t, db, func(txCtx context.Context) error {
		return saveQuotationIdentities(txCtx, db, tenant, party, entity, sentAt.Add(-24*time.Hour))
	})
	drafts, err := pricingpostgres.NewPriceCardDrafts(db)
	if err != nil {
		t.Fatal(err)
	}
	cards, err := pricingpostgres.NewPriceCards(db)
	if err != nil {
		t.Fatal(err)
	}
	prices, err := quotedprice.NewQuotedPrices(drafts, cards, pricecardtemplate.Reader{})
	if err != nil {
		t.Fatal(err)
	}
	sends, err := pcpostgres.NewQuotationSends(db)
	if err != nil {
		t.Fatal(err)
	}
	replies, err := pcpostgres.NewQuotationReplies(db)
	if err != nil {
		t.Fatal(err)
	}
	identities, err := pcpostgres.NewPartyIdentityRegistrations(db)
	if err != nil {
		t.Fatal(err)
	}
	sender, err := application.NewSendQuotationHandler(prices, identities, sends)
	if err != nil {
		t.Fatal(err)
	}
	recorder, err := application.NewRecordQuotationReplyHandler(sends, replies, prices)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := application.NewReadQuotationHandler(sends, replies, prices, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	recorderRef := quoteValue(t, domain.NewOperatorSubjectReference, "https://id.syn.example/dex#SYN-OPERATOR-02")

	sent, _ := sendApproved(t, db, drafts, sender, tenant, party, entity, sentAt, "SYN-PLAN-REPLY", "12.00", "reply.xlsx", "SYN-AUTH-REPLY")
	inside := sent.Send.Validity().StartsAt().Add(time.Hour)

	blank := replyCommand(t, sent, inside, recorderRef)
	blank.EvidenceIndex = "  "
	if result, err := recordReply(t, db, recorder, blank); err != nil || result.Outcome != application.QuotationReplyNotAccepted || replyExists(t, replies, tenant, sent.Send.ID()) {
		t.Fatalf("不带索引 = %s %v", result.Outcome, err)
	}
	outside := replyCommand(t, sent, sent.Send.Validity().EndsAt(), recorderRef)
	if result, err := recordReply(t, db, recorder, outside); err != nil || result.Outcome != application.QuotationReplyExpired || replyExists(t, replies, tenant, sent.Send.ID()) {
		t.Fatalf("有效期外 = %s %v", result.Outcome, err)
	}
	changed := replyCommand(t, sent, inside, recorderRef)
	changed.ContentDigest = "other"
	if result, err := recordReply(t, db, recorder, changed); err != nil || result.Outcome != application.QuotationReplyContentChanged || replyExists(t, replies, tenant, sent.Send.ID()) {
		t.Fatalf("摘要不符 = %s %v", result.Outcome, err)
	}

	accepted, err := recordReply(t, db, recorder, replyCommand(t, sent, inside, recorderRef))
	if err != nil || accepted.Outcome != application.QuotationReplyRecorded || accepted.Reply.RecordedBy() != recorderRef || !replyExists(t, replies, tenant, sent.Send.ID()) {
		t.Fatalf("接受 = %s 代录人 %s %v", accepted.Outcome, accepted.Reply.RecordedBy(), err)
	}
	other := replyCommand(t, sent, inside, recorderRef)
	other.Decision = domain.QuotationReplyRejected
	if result, err := recordReply(t, db, recorder, other); err != nil || result.Outcome != application.QuotationReplyAlreadyReplied {
		t.Fatalf("不同答复 = %s %v", result.Outcome, err)
	}
	kept, found, err := replies.GetQuotationReply(t.Context(), tenant, sent.Send.ID())
	if err != nil || !found || kept.Decision() != domain.QuotationReplyAccepted {
		t.Fatalf("原答复被换掉了：%v %v %s", found, err, kept.Decision())
	}

	quiet, _ := sendApproved(t, db, drafts, sender, tenant, party, entity, sentAt, "SYN-PLAN-REPLY-QUIET", "9.00", "quiet.xlsx", "SYN-AUTH-QUIET")
	before, _, err := sends.GetQuotationSend(t.Context(), tenant, quiet.Send.ID())
	if err != nil {
		t.Fatal(err)
	}
	reading, err := reader.Get(t.Context(), tenant, quiet.Send.ID(), quiet.Send.Validity().EndsAt())
	if err != nil || reading.Status != domain.QuotationReadingExpired || reading.HasReply {
		t.Fatalf("过期读法 = %s has %v %v", reading.Status, reading.HasReply, err)
	}
	after, _, err := sends.GetQuotationSend(t.Context(), tenant, quiet.Send.ID())
	if err != nil || !after.SameFact(before) || replyExists(t, replies, tenant, quiet.Send.ID()) {
		t.Fatal("过期的读法改了库")
	}

	gone, approved := sendApproved(t, db, drafts, sender, tenant, party, entity, sentAt, "SYN-PLAN-REPLY-GONE", "7.00", "gone.xlsx", "SYN-AUTH-GONE")
	mustQuotationTx(t, db, func(txCtx context.Context) error {
		outcome, advanceErr := drafts.AdvancePriceCardDraft(txCtx, quoteWithdraw(t, approved, sentAt.Add(time.Minute)))
		if advanceErr != nil {
			return advanceErr
		}
		if outcome != ppports.PriceCardDraftAdvanced {
			return errQuote(outcome.String())
		}
		return nil
	})
	if result, err := recordReply(t, db, recorder, replyCommand(t, gone, inside, recorderRef)); err != nil || result.Outcome != application.QuotationReplyWithdrawn || replyExists(t, replies, tenant, gone.Send.ID()) {
		t.Fatalf("已撤下 = %s %v", result.Outcome, err)
	}
}

func sendApproved(
	t *testing.T,
	db *bentopg.DB,
	drafts *pricingpostgres.PriceCardDrafts,
	sender *application.SendQuotationHandler,
	tenant domain.TenantID,
	party domain.PartyID,
	entity domain.LegalEntityReference,
	sentAt time.Time,
	planID, amount, fileName, authID string,
) (application.SendQuotationResult, ppdomain.PriceCardDraft) {
	t.Helper()
	plan := quoteSellPlan(t, planID, amount)
	validated := quoteValidatedDraft(t, tenant, plan, quoteSource(t, fileName), authID, sentAt.Add(-time.Hour))
	approved := quoteApprove(t, validated, sentAt.Add(-30*time.Minute))
	mustQuotationTx(t, db, func(txCtx context.Context) error {
		if _, submitErr := drafts.SubmitDraft(txCtx, validated); submitErr != nil {
			return submitErr
		}
		outcome, advanceErr := drafts.AdvancePriceCardDraft(txCtx, approved)
		if advanceErr != nil {
			return advanceErr
		}
		if outcome != ppports.PriceCardDraftAdvanced {
			return errQuote(outcome.String())
		}
		return nil
	})
	sent, err := sendQuote(t, db, sender, quoteCommand(t, tenant, party, entity, sentAt, domain.QuotationPriceFromApprovedDraft, plan, plan.ContentDigest()))
	if err != nil || sent.Outcome != application.QuotationSent {
		t.Fatalf("%s 发出 = %s %v", planID, sent.Outcome, err)
	}
	return sent, approved
}

func replyCommand(
	t *testing.T,
	sent application.SendQuotationResult,
	at time.Time,
	recorder domain.OperatorSubjectReference,
) application.RecordQuotationReplyCommand {
	t.Helper()
	return application.RecordQuotationReplyCommand{
		Tenant: sent.Send.Tenant(), QuotationID: sent.Send.ID(), Decision: domain.QuotationReplyAccepted,
		RepliedAt: at, EvidenceIndex: "SYN-EVIDENCE-01", ContentDigest: sent.Send.ContentDigest(),
		Recorder: recorder, RecordedAt: at.Add(time.Minute),
	}
}

func recordReply(
	t *testing.T,
	db *bentopg.DB,
	recorder *application.RecordQuotationReplyHandler,
	command application.RecordQuotationReplyCommand,
) (application.RecordQuotationReplyResult, error) {
	t.Helper()
	var result application.RecordQuotationReplyResult
	err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var handleErr error
		result, handleErr = recorder.Handle(txCtx, command)
		return handleErr
	})
	return result, err
}

func replyExists(t *testing.T, replies *pcpostgres.QuotationReplies, tenant domain.TenantID, id domain.QuotationID) bool {
	t.Helper()
	_, found, err := replies.GetQuotationReply(t.Context(), tenant, id)
	if err != nil {
		t.Fatal(err)
	}
	return found
}
