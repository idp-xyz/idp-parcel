package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 答复指名一条发出，只记下接受或拒绝、客户时刻、证据索引、代录人与登记时刻。
// 没有状态列。读法按已经取到的事实派生，不改发出记录。

func TestQuotationEvidenceIndexKeepsOnlyANonEmptyIndex(t *testing.T) {
	index, err := domain.NewQuotationEvidenceIndex("  SYN-EVIDENCE-01  ")
	if err != nil || index.Text() != "SYN-EVIDENCE-01" {
		t.Fatalf("索引 = %q, %v", index.Text(), err)
	}
	if _, err := domain.NewQuotationEvidenceIndex("  \n\t "); !errors.Is(err, domain.ErrQuotationNotAccepted) {
		t.Fatalf("空白索引 = %v，想要未受理", err)
	}
	if _, err := domain.NewQuotationEvidenceIndex("SYN\u0000EVIDENCE"); !errors.Is(err, domain.ErrQuotationNotAccepted) {
		t.Fatalf("控制字符 = %v，想要未受理", err)
	}
}

func TestQuotationRejectionReasonMayBeEmpty(t *testing.T) {
	blank, err := domain.NewQuotationRejectionReason("  \n ")
	if err != nil || blank.Text() != "" {
		t.Fatalf("空白原因 = %q, %v，想要空串", blank.Text(), err)
	}
	kept, err := domain.NewQuotationRejectionReason("  价格过高  ")
	if err != nil || kept.Text() != "价格过高" {
		t.Fatalf("原因 = %q, %v", kept.Text(), err)
	}
	if _, err := domain.NewQuotationRejectionReason("价格\u0007过高"); !errors.Is(err, domain.ErrQuotationNotAccepted) {
		t.Fatalf("控制字符 = %v，想要未受理", err)
	}
}

func TestAQuotationReplyRecordsAcceptanceWithoutAReason(t *testing.T) {
	send := quotationSend(t, "")
	repliedAt := send.Validity().StartsAt().Add(time.Hour)
	recordedAt := repliedAt.Add(time.Minute)
	reply := quotationReply(t, send, domain.QuotationReplyAccepted, repliedAt, recordedAt, "SYN-EVIDENCE-01", "")
	if reply.Decision() != domain.QuotationReplyAccepted || reply.Decision().String() != "ACCEPTED" {
		t.Fatalf("决定 = %s", reply.Decision())
	}
	if reply.QuotationID() != send.ID() || !reply.RepliedAt().Equal(repliedAt) || reply.EvidenceIndex().Text() != "SYN-EVIDENCE-01" {
		t.Fatal("没有指名这条发出，或客户时刻、索引没记下")
	}
	if reply.RecordedBy().String() != "op-reply" || !reply.RecordedAt().Equal(recordedAt) || reply.RejectionReason().Text() != "" {
		t.Fatal("代录人、登记时刻或空原因没记下")
	}
	if _, err := domain.NewQuotationReply(replySpec(t, send, domain.QuotationReplyAccepted, repliedAt, recordedAt, "SYN-EVIDENCE-01", "不该有")); !errors.Is(err, domain.ErrQuotationNotAccepted) {
		t.Fatalf("接受带原因 = %v，想要未受理", err)
	}
}

func TestQuotationReplySameFactIgnoresWhoRecordedIt(t *testing.T) {
	send := quotationSend(t, "")
	repliedAt := send.Validity().StartsAt()
	recordedAt := repliedAt.Add(time.Minute)
	first := quotationReply(t, send, domain.QuotationReplyRejected, repliedAt, recordedAt, "SYN-EVIDENCE-01", "价格过高")
	againSpec := replySpec(t, send, domain.QuotationReplyRejected, repliedAt, recordedAt.Add(time.Hour), "  SYN-EVIDENCE-01 ", " 价格过高 ")
	againSpec.RecordedBy = commercialValue(t, domain.NewOperatorSubjectReference, "op-other")
	again, err := domain.NewQuotationReply(againSpec)
	if err != nil {
		t.Fatal(err)
	}
	if !first.SameFact(again) {
		t.Fatal("决定、客户时刻、索引、拒绝原因相同就应当是同一条，代录时刻不同也不另算")
	}
	other := quotationReply(t, send, domain.QuotationReplyRejected, repliedAt, recordedAt, "SYN-EVIDENCE-01", "别的原因")
	if first.SameFact(other) {
		t.Fatal("拒绝原因不同不应当当成同一条")
	}
}

func TestQuotationReplyStringNamesEveryNonZeroConstant(t *testing.T) {
	if domain.QuotationReplyRejected.String() != "REJECTED" {
		t.Fatalf("拒绝 = %s", domain.QuotationReplyRejected)
	}
	if domain.QuotationReplyDecisionInvalid.String() != "" || domain.QuotationReadingStatusInvalid.String() != "" {
		t.Fatal("零值不该有名字")
	}
	if domain.QuotationReadingPending.String() != "PENDING" ||
		domain.QuotationReadingAccepted.String() != "ACCEPTED" ||
		domain.QuotationReadingRejected.String() != "REJECTED" ||
		domain.QuotationReadingExpired.String() != "EXPIRED" ||
		domain.QuotationReadingPriceWithdrawn.String() != "REFERENCED_PRICE_WITHDRAWN" {
		t.Fatal("读法的名字没覆盖全部取值")
	}
}

func TestQuotationReadingFollowsReplyThenWithdrawalThenExpiry(t *testing.T) {
	send := quotationSend(t, "")
	start := send.Validity().StartsAt()
	end := send.Validity().EndsAt()
	reply := quotationReply(t, send, domain.QuotationReplyAccepted, start, start.Add(time.Minute), "SYN-EVIDENCE-01", "")
	accepted, err := domain.ReadQuotation(send, reply, true, true, end)
	if err != nil || accepted.Status != domain.QuotationReadingAccepted || !accepted.ReferencedDraftWithdrawn {
		t.Fatalf("已接受之后撤下 = %+v, %v", accepted, err)
	}
	rejected := quotationReply(t, send, domain.QuotationReplyRejected, start, start, "SYN-EVIDENCE-01", "")
	rejection, err := domain.ReadQuotation(send, rejected, true, true, end)
	if err != nil || rejection.Status != domain.QuotationReadingRejected || rejection.ReferencedDraftWithdrawn {
		t.Fatalf("已拒绝 = %+v, %v", rejection, err)
	}
	withdrawn, err := domain.ReadQuotation(send, domain.QuotationReply{}, false, true, start)
	if err != nil || withdrawn.Status != domain.QuotationReadingPriceWithdrawn {
		t.Fatalf("没有答复且已撤下 = %+v, %v", withdrawn, err)
	}
	expired, err := domain.ReadQuotation(send, domain.QuotationReply{}, false, false, end)
	if err != nil || expired.Status != domain.QuotationReadingExpired {
		t.Fatalf("终点不算在内 = %+v, %v", expired, err)
	}
	pending, err := domain.ReadQuotation(send, domain.QuotationReply{}, false, false, start)
	if err != nil || pending.Status != domain.QuotationReadingPending || pending.ReferencedDraftWithdrawn {
		t.Fatalf("待答复 = %+v, %v", pending, err)
	}
	card := quotationSendOnLane(t, domain.QuotationPriceFromRegisteredCard)
	cardReading, err := domain.ReadQuotation(card, domain.QuotationReply{}, false, true, card.Validity().StartsAt())
	if err != nil || cardReading.Status != domain.QuotationReadingPending {
		t.Fatalf("价卡路没有撤下这一格 = %+v, %v", cardReading, err)
	}
}

func quotationReply(
	t *testing.T,
	send domain.QuotationSend,
	decision domain.QuotationReplyDecision,
	repliedAt, recordedAt time.Time,
	evidence, reason string,
) domain.QuotationReply {
	t.Helper()
	reply, err := domain.NewQuotationReply(replySpec(t, send, decision, repliedAt, recordedAt, evidence, reason))
	if err != nil {
		t.Fatal(err)
	}
	return reply
}

func replySpec(
	t *testing.T,
	send domain.QuotationSend,
	decision domain.QuotationReplyDecision,
	repliedAt, recordedAt time.Time,
	evidence, reason string,
) domain.QuotationReplySpec {
	t.Helper()
	return domain.QuotationReplySpec{
		QuotationID:     send.ID(),
		Tenant:          send.Tenant(),
		Decision:        decision,
		RepliedAt:       repliedAt,
		EvidenceIndex:   evidence,
		RejectionReason: reason,
		RecordedBy:      commercialValue(t, domain.NewOperatorSubjectReference, "op-reply"),
		RecordedAt:      recordedAt,
	}
}

func quotationSendOnLane(t *testing.T, lane domain.QuotationPriceLane) domain.QuotationSend {
	t.Helper()
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	send, err := domain.NewQuotationSend(domain.QuotationSendSpec{
		ID:               commercialValue(t, domain.NewQuotationID, "QTS-CARD"),
		Tenant:           commercialValue(t, domain.NewTenantID, "tenant-1"),
		Lane:             lane,
		PlanID:           "PLAN-1",
		PlanVersion:      "v1",
		Canonicalization: "PPC-5",
		ContentDigest:    "digest-1",
		Counterparty:     commercialValue(t, domain.NewPartyID, "party-1"),
		CounterpartyName: commercialValue(t, domain.NewPartyName, "相对方"),
		LegalEntity:      commercialValue(t, domain.NewLegalEntityReference, "le-1"),
		LegalEntityName:  commercialValue(t, domain.NewPartyName, "责任法人"),
		StartsAt:         start,
		EndsAt:           start.Add(24 * time.Hour),
		Terms:            "",
		Sender:           commercialValue(t, domain.NewOperatorSubjectReference, "op-1"),
		SentAt:           start.Add(time.Hour),
		Method:           domain.QuotationSendByWorkbookExport,
	})
	if err != nil {
		t.Fatal(err)
	}
	return send
}
