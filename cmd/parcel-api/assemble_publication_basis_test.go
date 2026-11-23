package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pppostgres "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/pricecardtemplate"
	pricingapp "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	pricingdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	quotedprice "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/parcelpricing"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	pcapplication "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 走生产装配的发布：有发出记录且规则要求接受时，不带依据或依据对不上都不入册；对上的依据能读回来。
// 规则没登记、以及没有发出记录的草稿，不设这道门。

func TestTheWiredPublicationRecordsAnAcceptedQuotationAsBasis(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	publication, err := buildPriceCardDraftPublication(db, false)
	if err != nil {
		t.Fatal(err)
	}
	approval, err := buildPriceCardDraftApproval(db)
	if err != nil {
		t.Fatal(err)
	}
	drafts, err := pppostgres.NewPriceCardDrafts(db)
	if err != nil {
		t.Fatal(err)
	}
	cards, err := pppostgres.NewPriceCards(db)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := pppostgres.NewPriceCardApprovalDutyRules(db)
	if err != nil {
		t.Fatal(err)
	}
	fixture := newBasisFixture(t, db, drafts, cards)
	rule, err := pricingdomain.NewPriceCardApprovalDutyRule(fixture.pricingTenant, true, pricingdomain.OperatorGrant{})
	if err != nil {
		t.Fatal(err)
	}
	if err := inBasisTx(db, func(ctx context.Context) error {
		_, saveErr := rules.SavePriceCardApprovalDutyRule(ctx, rule)
		return saveErr
	}); err != nil {
		t.Fatal(err)
	}
	publishDirectionAuthorizationForCard(t, db, syntheticPriceCardRegistration(t))

	open := fixture.approve(t, approval, "v-open")
	if sent := fixture.send(t, open); sent.Outcome != pcapplication.QuotationSent {
		t.Fatalf("未登记规则前发出 = %s %s", sent.Outcome, sent.Conclusion)
	}
	published, err := publication.Handle(t.Context(), pricingapp.PublishPriceCardDraftCommand{Tenant: open.Tenant(), Plan: open.Plan()})
	if err != nil || published.Outcome != pricingapp.PriceCardDraftPublished {
		t.Fatalf("规则没登记 = %s %v", published.Outcome, err)
	}

	if err := fixture.requireAcceptance(t); err != nil {
		t.Fatal(err)
	}

	nosend := fixture.approve(t, approval, "v-nosend")
	landed, err := publication.Handle(t.Context(), pricingapp.PublishPriceCardDraftCommand{Tenant: nosend.Tenant(), Plan: nosend.Plan()})
	if err != nil || landed.Outcome != pricingapp.PriceCardDraftPublished {
		t.Fatalf("没有发出记录 = %s %v", landed.Outcome, err)
	}

	blocked := fixture.approve(t, approval, "v-block")
	if sent := fixture.send(t, blocked); sent.Outcome != pcapplication.QuotationSent {
		t.Fatalf("发出 = %s %s", sent.Outcome, sent.Conclusion)
	}
	refused, err := publication.Handle(t.Context(), pricingapp.PublishPriceCardDraftCommand{Tenant: blocked.Tenant(), Plan: blocked.Plan()})
	if err != nil || refused.Outcome != pricingapp.PriceCardPublicationAcceptanceRequired {
		t.Fatalf("不带依据 = %s %v", refused.Outcome, err)
	}
	fixture.stillApproved(t, blocked)
	fixture.notOnRegister(t, blocked)

	missing := fixture.approve(t, approval, "v-miss")
	if sent := fixture.send(t, missing); sent.Outcome != pcapplication.QuotationSent {
		t.Fatalf("发出 = %s %s", sent.Outcome, sent.Conclusion)
	}
	bad, err := publication.Handle(t.Context(), pricingapp.PublishPriceCardDraftCommand{Tenant: missing.Tenant(), Plan: missing.Plan(), Basis: "QTS-NO-SUCH"})
	if err != nil || bad.Outcome != pricingapp.PriceCardPublicationCitationRejected {
		t.Fatalf("不存在的依据 = %s %v", bad.Outcome, err)
	}
	fixture.stillApproved(t, missing)
	fixture.notOnRegister(t, missing)

	rejected := fixture.approve(t, approval, "v-rej")
	sentRejected := fixture.send(t, rejected)
	if sentRejected.Outcome != pcapplication.QuotationSent {
		t.Fatalf("发出 = %s %s", sentRejected.Outcome, sentRejected.Conclusion)
	}
	if reply := fixture.reply(t, sentRejected.Send, pcdomain.QuotationReplyRejected, "不要"); reply.Outcome != pcapplication.QuotationReplyRecorded {
		t.Fatalf("拒绝 = %s %s", reply.Outcome, reply.Conclusion)
	}
	bad, err = publication.Handle(t.Context(), pricingapp.PublishPriceCardDraftCommand{Tenant: rejected.Tenant(), Plan: rejected.Plan(), Basis: sentRejected.Send.ID().String()})
	if err != nil || bad.Outcome != pricingapp.PriceCardPublicationCitationRejected {
		t.Fatalf("被拒绝的依据 = %s %v", bad.Outcome, err)
	}
	fixture.notOnRegister(t, rejected)

	other := fixture.approve(t, approval, "v-other")
	sentOther := fixture.send(t, other)
	if sentOther.Outcome != pcapplication.QuotationSent {
		t.Fatalf("别的版本发出 = %s %s", sentOther.Outcome, sentOther.Conclusion)
	}
	if reply := fixture.reply(t, sentOther.Send, pcdomain.QuotationReplyAccepted, ""); reply.Outcome != pcapplication.QuotationReplyRecorded {
		t.Fatalf("别的版本接受 = %s %s", reply.Outcome, reply.Conclusion)
	}
	cross := fixture.approve(t, approval, "v-cross")
	if sent := fixture.send(t, cross); sent.Outcome != pcapplication.QuotationSent {
		t.Fatalf("发出 = %s %s", sent.Outcome, sent.Conclusion)
	}
	bad, err = publication.Handle(t.Context(), pricingapp.PublishPriceCardDraftCommand{Tenant: cross.Tenant(), Plan: cross.Plan(), Basis: sentOther.Send.ID().String()})
	if err != nil || bad.Outcome != pricingapp.PriceCardPublicationCitationRejected {
		t.Fatalf("别的方案版本 = %s %v", bad.Outcome, err)
	}
	fixture.notOnRegister(t, cross)

	mismatched := fixture.approve(t, approval, "v-digest")
	odd := fixture.sendWithDigest(t, mismatched, "not-the-digest")
	if reply := fixture.reply(t, odd, pcdomain.QuotationReplyAccepted, ""); reply.Outcome != pcapplication.QuotationReplyRecorded {
		t.Fatalf("摘要不同的接受 = %s %s", reply.Outcome, reply.Conclusion)
	}
	bad, err = publication.Handle(t.Context(), pricingapp.PublishPriceCardDraftCommand{Tenant: mismatched.Tenant(), Plan: mismatched.Plan(), Basis: odd.ID().String()})
	if err != nil || bad.Outcome != pricingapp.PriceCardPublicationCitationRejected {
		t.Fatalf("摘要不同 = %s %v", bad.Outcome, err)
	}
	fixture.notOnRegister(t, mismatched)

	good := fixture.approve(t, approval, "v-good")
	sentGood := fixture.send(t, good)
	if sentGood.Outcome != pcapplication.QuotationSent {
		t.Fatalf("发出 = %s %s", sentGood.Outcome, sentGood.Conclusion)
	}
	if reply := fixture.reply(t, sentGood.Send, pcdomain.QuotationReplyAccepted, ""); reply.Outcome != pcapplication.QuotationReplyRecorded {
		t.Fatalf("接受 = %s %s", reply.Outcome, reply.Conclusion)
	}
	landed, err = publication.Handle(t.Context(), pricingapp.PublishPriceCardDraftCommand{
		Tenant: good.Tenant(), Plan: good.Plan(), Basis: sentGood.Send.ID().String(),
	})
	if err != nil || landed.Outcome != pricingapp.PriceCardDraftPublished {
		t.Fatalf("有效接受 = %s %v", landed.Outcome, err)
	}
	content, found, err := cards.ReadPriceCardContent(t.Context(), good.Tenant(), good.Plan())
	if err != nil || !found || !content.HasPublicationBasis || content.PublicationBasis != sentGood.Send.ID().String() {
		t.Fatalf("价卡详情依据 = %q found %v err %v", content.PublicationBasis, found, err)
	}
	detail, err := fixture.reader.Get(t.Context(), fixture.tenant, sentGood.Send.ID(), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if !detail.Effectiveness.RecordedBasis || !detail.Effectiveness.PriceOnRegister || !detail.Effectiveness.ValidAcceptance {
		t.Fatalf("报价详情 = %+v", detail.Effectiveness)
	}
	if detail.Effectiveness.Requirement != pcapplication.QuotationPublicationRuleRequired {
		t.Fatalf("规则 = %s", detail.Effectiveness.Requirement)
	}
}

type basisFixture struct {
	db            *bentopg.DB
	drafts        *pppostgres.PriceCardDrafts
	cards         *pppostgres.PriceCards
	tenant        pcdomain.TenantID
	pricingTenant pricingdomain.TenantID
	party         pcdomain.PartyID
	entity        pcdomain.LegalEntityReference
	sender        *pcapplication.SendQuotationHandler
	replies       *pcapplication.RecordQuotationReplyHandler
	sends         *pcpostgres.QuotationSends
	reader        *pcapplication.ReadQuotationHandler
	view          *pcpostgres.QuotationPublication
}

func newBasisFixture(t *testing.T, db *bentopg.DB, drafts *pppostgres.PriceCardDrafts, cards *pppostgres.PriceCards) basisFixture {
	t.Helper()
	fixture := basisFixture{
		db: db, drafts: drafts, cards: cards,
		tenant:        mustValue(t, pcdomain.NewTenantID, "SYN-TENANT-1"),
		pricingTenant: mustValue(t, pricingdomain.NewTenantID, "SYN-TENANT-1"),
		party:         mustValue(t, pcdomain.NewPartyID, "SYN-PARTY-SHIPPER-01"),
		entity:        mustValue(t, pcdomain.NewLegalEntityReference, "SYN-LE-01"),
	}
	if err := inBasisTx(db, func(ctx context.Context) error {
		return saveBasisParties(ctx, db, fixture.tenant, fixture.party, fixture.entity)
	}); err != nil {
		t.Fatal(err)
	}
	prices, err := quotedprice.NewQuotedPrices(drafts, cards, pricecardtemplate.Reader{})
	if err != nil {
		t.Fatal(err)
	}
	identities, err := pcpostgres.NewPartyIdentityRegistrations(db)
	if err != nil {
		t.Fatal(err)
	}
	fixture.sends, err = pcpostgres.NewQuotationSends(db)
	if err != nil {
		t.Fatal(err)
	}
	fixture.sender, err = pcapplication.NewSendQuotationHandler(prices, identities, fixture.sends)
	if err != nil {
		t.Fatal(err)
	}
	replyStore, err := pcpostgres.NewQuotationReplies(db)
	if err != nil {
		t.Fatal(err)
	}
	fixture.replies, err = pcapplication.NewRecordQuotationReplyHandler(fixture.sends, replyStore, prices)
	if err != nil {
		t.Fatal(err)
	}
	fixture.view, err = pcpostgres.NewQuotationPublication(db)
	if err != nil {
		t.Fatal(err)
	}
	bases, err := pppostgres.NewPublicationBases(db)
	if err != nil {
		t.Fatal(err)
	}
	registered, err := quotedprice.NewRegisteredPrices(cards, bases)
	if err != nil {
		t.Fatal(err)
	}
	fixture.reader, err = pcapplication.NewReadQuotationHandler(fixture.sends, replyStore, prices, fixture.view, registered)
	if err != nil {
		t.Fatal(err)
	}
	return fixture
}

func (fixture basisFixture) approve(t *testing.T, approval transactionalPriceCardDraftApproval, version string) pricingdomain.PriceCardDraft {
	t.Helper()
	registration := syntheticPriceCardRegistrationVersion(t, version)
	draft, err := pricingdomain.SubmitPriceCardDraft(pricingdomain.PriceCardDraftSubmission{
		Tenant: registration.Tenant(), Plan: registration.Plan().Reference(), Source: registration.SourceFile(),
		Content:     &pricingdomain.PriceCardDraftContent{Plan: registration.Plan(), DirectionAuthorization: registration.DirectionAuthorization()},
		Submitter:   "https://id.syn.example/dex#SYN-OPERATOR-01",
		SubmittedAt: time.Now().UTC().Add(-3 * time.Hour).Truncate(time.Microsecond),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := inBasisTx(fixture.db, func(ctx context.Context) error {
		_, submitErr := fixture.drafts.SubmitDraft(ctx, draft)
		return submitErr
	}); err != nil {
		t.Fatal(err)
	}
	approver, err := pricingdomain.NewOperatorSubject("https://id.syn.example/dex#SYN-OPERATOR-02", nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := approval.Handle(t.Context(), pricingapp.ApprovePriceCardDraftCommand{Tenant: draft.Tenant(), Plan: draft.Plan(), Approver: approver})
	if err != nil || result.Outcome != pricingapp.PriceCardDraftApproved {
		t.Fatalf("批准 %s = %s %v", version, result.Outcome, err)
	}
	loaded, found, err := fixture.drafts.LoadPriceCardDraft(t.Context(), draft.Tenant(), draft.Plan())
	if err != nil || !found {
		t.Fatalf("读回已批准草稿 %s：%v", version, err)
	}
	return loaded
}

func (fixture basisFixture) send(t *testing.T, draft pricingdomain.PriceCardDraft) pcapplication.SendQuotationResult {
	t.Helper()
	return fixture.sendDigest(t, draft, fixture.digestOf(t, draft))
}

func (fixture basisFixture) digestOf(t *testing.T, draft pricingdomain.PriceCardDraft) string {
	t.Helper()
	loaded, found, err := fixture.drafts.LoadPriceCardDraft(t.Context(), draft.Tenant(), draft.Plan())
	content, ok := loaded.Content()
	if err != nil || !found || !ok {
		t.Fatalf("读内容摘要：found %v ok %v err %v", found, ok, err)
	}
	return content.Plan.ContentDigest()
}

func (fixture basisFixture) sendWithDigest(t *testing.T, draft pricingdomain.PriceCardDraft, digest string) pcdomain.QuotationSend {
	t.Helper()
	sentAt := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Microsecond)
	send, err := pcdomain.NewQuotationSend(pcdomain.QuotationSendSpec{
		ID:     mustValue(t, pcdomain.NewQuotationID, "QTS-ODD-"+draft.Plan().Version()),
		Tenant: fixture.tenant, Lane: pcdomain.QuotationPriceFromApprovedDraft,
		PlanID: draft.Plan().ID(), PlanVersion: draft.Plan().Version(),
		Canonicalization: "PPC-5", ContentDigest: digest,
		Counterparty: fixture.party, CounterpartyName: mustValue(t, pcdomain.NewPartyName, "合成货主"),
		LegalEntity: fixture.entity, LegalEntityName: mustValue(t, pcdomain.NewPartyName, "合成法人"),
		StartsAt: sentAt, EndsAt: sentAt.Add(240 * time.Hour), Terms: "",
		Sender: mustValue(t, pcdomain.NewOperatorSubjectReference, "https://id.syn.example/dex#SYN-OPERATOR-01"),
		SentAt: sentAt, Method: pcdomain.QuotationSendByWorkbookExport,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := inBasisTx(fixture.db, func(ctx context.Context) error {
		outcome, appendErr := fixture.sends.AppendQuotationSend(ctx, send)
		if appendErr != nil {
			return appendErr
		}
		if outcome != pcports.QuotationAppended {
			return fmt.Errorf("append %s", outcome)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return send
}

func (fixture basisFixture) sendDigest(t *testing.T, draft pricingdomain.PriceCardDraft, digest string) pcapplication.SendQuotationResult {
	t.Helper()
	sentAt := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Microsecond)
	var result pcapplication.SendQuotationResult
	err := inBasisTx(fixture.db, func(ctx context.Context) error {
		var handleErr error
		result, handleErr = fixture.sender.Handle(ctx, pcapplication.SendQuotationCommand{
			Tenant: fixture.tenant, Sender: mustValue(t, pcdomain.NewOperatorSubjectReference, "https://id.syn.example/dex#SYN-OPERATOR-01"),
			SentAt: sentAt, Lane: pcdomain.QuotationPriceFromApprovedDraft,
			PlanID: draft.Plan().ID(), PlanVersion: draft.Plan().Version(), ContentDigest: digest,
			Counterparty: fixture.party, LegalEntity: fixture.entity,
			StartsAt: sentAt, EndsAt: sentAt.Add(240 * time.Hour),
		})
		return handleErr
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func (fixture basisFixture) reply(t *testing.T, send pcdomain.QuotationSend, decision pcdomain.QuotationReplyDecision, reason string) pcapplication.RecordQuotationReplyResult {
	t.Helper()
	var result pcapplication.RecordQuotationReplyResult
	err := inBasisTx(fixture.db, func(ctx context.Context) error {
		var handleErr error
		result, handleErr = fixture.replies.Handle(ctx, pcapplication.RecordQuotationReplyCommand{
			Tenant: fixture.tenant, QuotationID: send.ID(), Decision: decision,
			RepliedAt: send.SentAt().Add(time.Hour), EvidenceIndex: "SYN-EVIDENCE-" + send.ID().String(),
			RejectionReason: reason, ContentDigest: send.ContentDigest(),
			Recorder:   mustValue(t, pcdomain.NewOperatorSubjectReference, "https://id.syn.example/dex#SYN-OPERATOR-02"),
			RecordedAt: send.SentAt().Add(2 * time.Hour),
		})
		return handleErr
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func (fixture basisFixture) requireAcceptance(t *testing.T) error {
	t.Helper()
	rule, err := pcdomain.NewQuotationPublicationRequirement(fixture.tenant, true)
	if err != nil {
		return err
	}
	return inBasisTx(fixture.db, func(ctx context.Context) error {
		outcome, saveErr := pcapplication.RegisterQuotationPublicationRequirement(ctx, fixture.view, rule)
		if saveErr != nil {
			return saveErr
		}
		if outcome != pcapplication.QuotationPublicationRequirementRecorded {
			return fmt.Errorf("requirement %s", outcome)
		}
		return nil
	})
}

func (fixture basisFixture) stillApproved(t *testing.T, draft pricingdomain.PriceCardDraft) {
	t.Helper()
	loaded, found, err := fixture.drafts.LoadPriceCardDraft(t.Context(), draft.Tenant(), draft.Plan())
	if err != nil || !found || loaded.Status() != pricingdomain.PriceCardDraftStatusApproved {
		t.Fatalf("草稿 = %s found %v err %v，want 仍是已批准", loaded.Status(), found, err)
	}
}

func (fixture basisFixture) notOnRegister(t *testing.T, draft pricingdomain.PriceCardDraft) {
	t.Helper()
	_, found, err := fixture.cards.ReadPriceCardContent(t.Context(), draft.Tenant(), draft.Plan())
	if err != nil || found {
		t.Fatalf("价卡册 found %v err %v，不应入册", found, err)
	}
}

func saveBasisParties(ctx context.Context, db *bentopg.DB, tenant pcdomain.TenantID, party pcdomain.PartyID, entity pcdomain.LegalEntityReference) error {
	registrations, err := pcpostgres.NewPartyIdentityRegistrations(db)
	if err != nil {
		return err
	}
	lifecycle, err := pcdomain.NewIdentityLifecycle(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		return err
	}
	basis, err := pcdomain.NewIdentityBasisReference("SYN-REG-BASIS-QUOTE")
	if err != nil {
		return err
	}
	shipperName, err := pcdomain.NewPartyName("合成货主")
	if err != nil {
		return err
	}
	operatorName, err := pcdomain.NewPartyName("合成运营方")
	if err != nil {
		return err
	}
	counterparty, err := pcdomain.NewBusinessParty(tenant, party, shipperName)
	if err != nil {
		return err
	}
	pinnedParty, err := pcdomain.NewPartyID("SYN-PARTY-OPERATOR-01")
	if err != nil {
		return err
	}
	pinned, err := pcdomain.NewBusinessParty(tenant, pinnedParty, operatorName)
	if err != nil {
		return err
	}
	partyReg, err := pcdomain.NewBusinessPartyRegistration(counterparty, 1, basis, lifecycle)
	if err != nil {
		return err
	}
	pinnedReg, err := pcdomain.NewBusinessPartyRegistration(pinned, 1, basis, lifecycle)
	if err != nil {
		return err
	}
	legal, err := pcdomain.NewResponsibleLegalEntity(tenant, entity, pinned)
	if err != nil {
		return err
	}
	legalReg, err := pcdomain.NewLegalEntityRegistration(legal, 1, basis, lifecycle)
	if err != nil {
		return err
	}
	for _, save := range []func() (pcports.PartyRegistrySaveOutcome, error){
		func() (pcports.PartyRegistrySaveOutcome, error) {
			return registrations.SaveBusinessParty(ctx, partyReg)
		},
		func() (pcports.PartyRegistrySaveOutcome, error) {
			return registrations.SaveBusinessParty(ctx, pinnedReg)
		},
		func() (pcports.PartyRegistrySaveOutcome, error) { return registrations.SaveLegalEntity(ctx, legalReg) },
	} {
		outcome, saveErr := save()
		if saveErr != nil {
			return saveErr
		}
		if outcome != pcports.PartyRegistrySaved {
			return fmt.Errorf("save identity %s", outcome)
		}
	}
	return nil
}

func inBasisTx(db *bentopg.DB, fn func(context.Context) error) error {
	return db.Transactor().WithinTransaction(context.Background(), fn)
}
