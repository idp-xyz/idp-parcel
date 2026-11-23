package parcelpricing_test

import (
	"context"
	"fmt"
	"strings"
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
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 真库：已批准的销售草稿与已登记的销售价卡都能发出并导出，文件里没有内部信息；
// 已校验、已撤下、摘要不符都不落行；再导出不增行。已登记的卡不另加在用门。

const (
	hiddenSourceDraft = "SYN-HIDDEN-SOURCE-DRAFT.xlsx"
	hiddenSourceCard  = "SYN-HIDDEN-SOURCE-CARD.xlsx"
	hiddenSHA         = "abababababababababababababababababababababababababababababababab"
	hiddenAuthDraft   = "SYN-AUTH-HIDDEN-DRAFT"
	hiddenAuthCard    = "SYN-AUTH-HIDDEN-CARD"
	hiddenEnterer     = "https://id.syn.example/dex#SYN-HIDDEN-ENTERER"
	hiddenApprover    = "https://id.syn.example/dex#SYN-HIDDEN-APPROVER"
)

func TestQuotationSendOnTheRealDatabase(t *testing.T) {
	db := newQuotationDB(t)
	ctx := t.Context()
	tenant := quoteValue(t, domain.NewTenantID, "SYN-TENANT-01")
	party := quoteValue(t, domain.NewPartyID, "SYN-PARTY-SHIPPER-01")
	entity := quoteValue(t, domain.NewLegalEntityReference, "SYN-LE-01")
	sentAt := time.Date(2026, 11, 23, 4, 0, 0, 0, time.UTC)
	mustQuotationTx(t, db, func(txCtx context.Context) error {
		return saveQuotationIdentities(txCtx, db, tenant, party, entity, sentAt.Add(-24*time.Hour))
	})

	draftPlan := quoteSellPlan(t, "SYN-PLAN-QUOTE-DRAFT", "41.50")
	cardPlan := quoteSellPlan(t, "SYN-PLAN-QUOTE-CARD", "18.00")
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
	store, err := pcpostgres.NewQuotationSends(db)
	if err != nil {
		t.Fatal(err)
	}
	identities, err := pcpostgres.NewPartyIdentityRegistrations(db)
	if err != nil {
		t.Fatal(err)
	}
	sender, err := application.NewSendQuotationHandler(prices, identities, store)
	if err != nil {
		t.Fatal(err)
	}
	exporter, err := application.NewExportQuotationHandler(store, prices)
	if err != nil {
		t.Fatal(err)
	}

	draftSource := quoteSource(t, hiddenSourceDraft)
	cardSource := quoteSource(t, hiddenSourceCard)
	validated := quoteValidatedDraft(t, tenant, draftPlan, draftSource, hiddenAuthDraft, sentAt.Add(-time.Hour))
	mustQuotationTx(t, db, func(txCtx context.Context) error {
		outcome, submitErr := drafts.SubmitDraft(txCtx, validated)
		if submitErr != nil {
			return submitErr
		}
		if outcome != ppports.PriceCardDraftSaved {
			return errQuote(outcome.String())
		}
		return nil
	})

	refused, err := sendQuote(t, db, sender, quoteCommand(t, tenant, party, entity, sentAt, domain.QuotationPriceFromApprovedDraft, draftPlan, "ignored"))
	if err != nil {
		t.Fatal(err)
	}
	if refused.Outcome != application.QuotationSendContentNotFixed || quotationRows(t, store, tenant) != 0 {
		t.Fatalf("已校验 = %s，行 %d", refused.Outcome, quotationRows(t, store, tenant))
	}

	approved := quoteApprove(t, validated, sentAt.Add(-30*time.Minute))
	mustQuotationTx(t, db, func(txCtx context.Context) error {
		outcome, advanceErr := drafts.AdvancePriceCardDraft(txCtx, approved)
		if advanceErr != nil {
			return advanceErr
		}
		if outcome != ppports.PriceCardDraftAdvanced {
			return errQuote(outcome.String())
		}
		return nil
	})
	mismatched, err := sendQuote(t, db, sender, quoteCommand(t, tenant, party, entity, sentAt, domain.QuotationPriceFromApprovedDraft, draftPlan, "not-the-digest"))
	if err != nil {
		t.Fatal(err)
	}
	if mismatched.Outcome != application.QuotationSendContentChanged || quotationRows(t, store, tenant) != 0 {
		t.Fatalf("摘要不符 = %s，行 %d", mismatched.Outcome, quotationRows(t, store, tenant))
	}

	sent, err := sendQuote(t, db, sender, quoteCommand(t, tenant, party, entity, sentAt, domain.QuotationPriceFromApprovedDraft, draftPlan, draftPlan.ContentDigest()))
	if err != nil {
		t.Fatal(err)
	}
	if sent.Outcome != application.QuotationSent || quotationRows(t, store, tenant) != 1 {
		t.Fatalf("草稿发出 = %s，行 %d，结论 %s", sent.Outcome, quotationRows(t, store, tenant), sent.Conclusion)
	}
	assertCustomerFile(t, sent.Workbook, draftPlan, hiddenSourceDraft, hiddenAuthDraft)

	exported, err := exporter.Handle(ctx, tenant, sent.Send.ID())
	if err != nil {
		t.Fatal(err)
	}
	if exported.Outcome != application.QuotationExportReady || quotationRows(t, store, tenant) != 1 {
		t.Fatalf("再导出 = %s，行 %d", exported.Outcome, quotationRows(t, store, tenant))
	}
	assertCustomerFile(t, exported.Body, draftPlan, hiddenSourceDraft, hiddenAuthDraft)

	withdrawn := quoteWithdraw(t, approved, sentAt.Add(time.Minute))
	mustQuotationTx(t, db, func(txCtx context.Context) error {
		outcome, advanceErr := drafts.AdvancePriceCardDraft(txCtx, withdrawn)
		if advanceErr != nil {
			return advanceErr
		}
		if outcome != ppports.PriceCardDraftAdvanced {
			return errQuote(outcome.String())
		}
		return nil
	})
	again, err := sendQuote(t, db, sender, quoteCommand(t, tenant, party, entity, sentAt.Add(2*time.Minute), domain.QuotationPriceFromApprovedDraft, draftPlan, draftPlan.ContentDigest()))
	if err != nil {
		t.Fatal(err)
	}
	if again.Outcome != application.QuotationSendWithdrawn || quotationRows(t, store, tenant) != 1 {
		t.Fatalf("已撤下 = %s，行 %d", again.Outcome, quotationRows(t, store, tenant))
	}

	registration := quoteRegistration(t, tenant, cardPlan, cardSource, hiddenAuthCard)
	mustQuotationTx(t, db, func(txCtx context.Context) error {
		outcome, registerErr := cards.Register(txCtx, registration)
		if registerErr != nil {
			return registerErr
		}
		if outcome != ppports.PriceCardRegistered {
			return fmt.Errorf("register %d", outcome)
		}
		return nil
	})
	cardSent, err := sendQuote(t, db, sender, quoteCommand(t, tenant, party, entity, sentAt, domain.QuotationPriceFromRegisteredCard, cardPlan, cardPlan.ContentDigest()))
	if err != nil {
		t.Fatal(err)
	}
	if cardSent.Outcome != application.QuotationSent || quotationRows(t, store, tenant) != 2 {
		t.Fatalf("价卡发出 = %s，行 %d，结论 %s", cardSent.Outcome, quotationRows(t, store, tenant), cardSent.Conclusion)
	}
	assertCustomerFile(t, cardSent.Workbook, cardPlan, hiddenSourceCard, hiddenAuthCard)
	cardExported, err := exporter.Handle(ctx, tenant, cardSent.Send.ID())
	if err != nil {
		t.Fatal(err)
	}
	if cardExported.Outcome != application.QuotationExportReady || quotationRows(t, store, tenant) != 2 {
		t.Fatalf("价卡再导出 = %s，行 %d", cardExported.Outcome, quotationRows(t, store, tenant))
	}
	assertCustomerFile(t, cardExported.Body, cardPlan, hiddenSourceCard, hiddenAuthCard)
}

func assertCustomerFile(t *testing.T, body []byte, plan ppdomain.PricingPlanVersion, sourceName, authID string) {
	t.Helper()
	text := string(body)
	for _, hidden := range []string{
		sourceName, hiddenSHA, authID, "directionAuthorizationId", "directionAuthorizationVersion",
		hiddenEnterer, hiddenApprover, plan.CanonicalizationVersion(),
	} {
		if strings.Contains(text, hidden) {
			t.Fatalf("对客文件含有内部信息 %q", hidden)
		}
	}
	full, err := pricecardtemplate.Reader{}.ProjectPriceCardWorkbook(plan, quoteAuth(t, authID), ppports.PriceCardWorkbookSpreadsheet)
	if err != nil || full.Gap.Where != "" {
		t.Fatalf("完整投影：%v %s", err, full.Gap.Conclusion)
	}
	if !strings.Contains(string(full.Document.Body), authID) {
		t.Fatal("完整投影应当带着价格方向授权，否则看不出对客文件删掉了什么")
	}
	amount := plan.RateTable().Entries()[0].Amount().Amount().String()
	if !strings.Contains(text, amount) || !strings.Contains(string(full.Document.Body), amount) {
		t.Fatalf("金额 %s 应当在对客文件和完整投影里都在", amount)
	}
}

func newQuotationDB(t *testing.T) *bentopg.DB {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func mustQuotationTx(t *testing.T, db *bentopg.DB, fn func(context.Context) error) {
	t.Helper()
	if err := db.Transactor().WithinTransaction(t.Context(), fn); err != nil {
		t.Fatalf("事务：%v", err)
	}
}

func sendQuote(t *testing.T, db *bentopg.DB, sender *application.SendQuotationHandler, command application.SendQuotationCommand) (application.SendQuotationResult, error) {
	t.Helper()
	var result application.SendQuotationResult
	err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var handleErr error
		result, handleErr = sender.Handle(txCtx, command)
		return handleErr
	})
	return result, err
}

func quotationRows(t *testing.T, store *pcpostgres.QuotationSends, tenant domain.TenantID) int {
	t.Helper()
	page, err := store.ListQuotationSends(t.Context(), tenant, 100, ports.QuotationSendCursor{})
	if err != nil {
		t.Fatal(err)
	}
	return len(page.Sends)
}

func saveQuotationIdentities(ctx context.Context, db *bentopg.DB, tenant domain.TenantID, party domain.PartyID, entity domain.LegalEntityReference, from time.Time) error {
	registrations, err := pcpostgres.NewPartyIdentityRegistrations(db)
	if err != nil {
		return err
	}
	lifecycle, err := domain.NewIdentityLifecycle(from)
	if err != nil {
		return err
	}
	basis, err := domain.NewIdentityBasisReference("SYN-REG-BASIS-QUOTE")
	if err != nil {
		return err
	}
	counterparty, err := domain.NewBusinessParty(tenant, party, mustName("合成货主一号（演示）"))
	if err != nil {
		return err
	}
	pinnedParty, err := domain.NewPartyID("SYN-PARTY-OPERATOR-01")
	if err != nil {
		return err
	}
	pinned, err := domain.NewBusinessParty(tenant, pinnedParty, mustName("合成运营方一号（演示）"))
	if err != nil {
		return err
	}
	partyReg, err := domain.NewBusinessPartyRegistration(counterparty, 1, basis, lifecycle)
	if err != nil {
		return err
	}
	pinnedReg, err := domain.NewBusinessPartyRegistration(pinned, 1, basis, lifecycle)
	if err != nil {
		return err
	}
	legal, err := domain.NewResponsibleLegalEntity(tenant, entity, pinned)
	if err != nil {
		return err
	}
	legalReg, err := domain.NewLegalEntityRegistration(legal, 1, basis, lifecycle)
	if err != nil {
		return err
	}
	for _, save := range []func() (ports.PartyRegistrySaveOutcome, error){
		func() (ports.PartyRegistrySaveOutcome, error) { return registrations.SaveBusinessParty(ctx, partyReg) },
		func() (ports.PartyRegistrySaveOutcome, error) { return registrations.SaveBusinessParty(ctx, pinnedReg) },
		func() (ports.PartyRegistrySaveOutcome, error) { return registrations.SaveLegalEntity(ctx, legalReg) },
	} {
		outcome, saveErr := save()
		if saveErr != nil {
			return saveErr
		}
		if outcome != ports.PartyRegistrySaved {
			return errQuote(outcome.String())
		}
	}
	return nil
}

func quoteCommand(t *testing.T, tenant domain.TenantID, party domain.PartyID, entity domain.LegalEntityReference, sentAt time.Time, lane domain.QuotationPriceLane, plan ppdomain.PricingPlanVersion, digest string) application.SendQuotationCommand {
	t.Helper()
	return application.SendQuotationCommand{
		Tenant: tenant, Sender: quoteValue(t, domain.NewOperatorSubjectReference, "https://id.syn.example/dex#SYN-OPERATOR-01"),
		SentAt: sentAt, Lane: lane, PlanID: plan.Reference().ID(), PlanVersion: plan.Reference().Version(),
		ContentDigest: digest, Counterparty: party, LegalEntity: entity,
		StartsAt: sentAt, EndsAt: sentAt.Add(240 * time.Hour), Terms: "SYN 演示条款",
	}
}

func quoteSellPlan(t *testing.T, planID, amount string) ppdomain.PricingPlanVersion {
	t.Helper()
	currency := quotePP(t, ppdomain.NewCurrency, "CNY")
	entry, err := ppdomain.NewRateEntry(
		quotePP(t, ppdomain.NewRateEntryID, "entry-"+planID),
		"Z1",
		quoteWeight(t, "0"),
		quoteWeight(t, "10"),
		quoteMoney(t, amount, currency),
	)
	if err != nil {
		t.Fatal(err)
	}
	period, err := ppdomain.NewEffectivePeriod(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	tableRef, err := ppdomain.NewVersionReferenceIdentity(ppdomain.ArtifactRateTable, "table-"+planID, "v1")
	if err != nil {
		t.Fatal(err)
	}
	table, err := ppdomain.NewRateTableVersion(tableRef, ppdomain.RateTableFamilyWeightZone, currency, ppdomain.WeightUnitKilogram, period, []ppdomain.RateEntry{entry})
	if err != nil {
		t.Fatal(err)
	}
	rounding, err := ppdomain.NewWeightRoundingPolicy(ppdomain.RoundingCeiling, quoteWeight(t, "0.5"))
	if err != nil {
		t.Fatal(err)
	}
	weightRef, err := ppdomain.NewVersionReferenceIdentity(ppdomain.ArtifactWeightPolicy, "weight-"+planID, "v1")
	if err != nil {
		t.Fatal(err)
	}
	weight, err := ppdomain.NewPricingWeightPolicy(weightRef, ppdomain.PricingWeightActualOnly, rounding, nil)
	if err != nil {
		t.Fatal(err)
	}
	planRef, err := ppdomain.NewVersionReferenceIdentity(ppdomain.ArtifactPricingPlan, planID, "v1")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := ppdomain.NewPricingPlanVersion(
		planRef, quotePP(t, ppdomain.NewPricingScopeID, "scope-1"),
		ppdomain.PricingDirectionSell, ppdomain.PricingPurposeCustomerCharge,
		quotePP(t, ppdomain.NewChargeCode, "BASE_FREIGHT"),
		period, table, weight, nil, ppdomain.PricingPlanStructures{},
	)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func quoteValidatedDraft(t *testing.T, tenant domain.TenantID, plan ppdomain.PricingPlanVersion, source ppdomain.SourceFileIdentity, authID string, at time.Time) ppdomain.PriceCardDraft {
	t.Helper()
	pricingTenant := quotePP(t, ppdomain.NewTenantID, tenant.String())
	draft, err := ppdomain.SubmitPriceCardDraft(ppdomain.PriceCardDraftSubmission{
		Tenant: pricingTenant, Plan: plan.Reference(), Source: source,
		Content:     &ppdomain.PriceCardDraftContent{Plan: plan, DirectionAuthorization: quoteAuth(t, authID)},
		Submitter:   hiddenEnterer,
		SubmittedAt: at,
	})
	if err != nil {
		t.Fatal(err)
	}
	return draft
}

func quoteApprove(t *testing.T, draft ppdomain.PriceCardDraft, at time.Time) ppdomain.PriceCardDraft {
	t.Helper()
	subject, err := ppdomain.NewOperatorSubject(hiddenApprover, nil)
	if err != nil {
		t.Fatal(err)
	}
	rule, err := ppdomain.NewPriceCardApprovalDutyRule(draft.Tenant(), false, ppdomain.OperatorGrant{})
	if err != nil {
		t.Fatal(err)
	}
	approved, err := draft.Approve(subject, rule, at)
	if err != nil {
		t.Fatal(err)
	}
	return approved
}

func quoteWithdraw(t *testing.T, approved ppdomain.PriceCardDraft, at time.Time) ppdomain.PriceCardDraft {
	t.Helper()
	subject, err := ppdomain.NewOperatorSubject(hiddenApprover, nil)
	if err != nil {
		t.Fatal(err)
	}
	withdrawn, err := approved.Withdraw(subject, "客户不要这一版", at)
	if err != nil {
		t.Fatal(err)
	}
	return withdrawn
}

func quoteRegistration(t *testing.T, tenant domain.TenantID, plan ppdomain.PricingPlanVersion, source ppdomain.SourceFileIdentity, authID string) ppdomain.PriceCardRegistration {
	t.Helper()
	registration, err := ppdomain.NewPriceCardRegistration(
		quotePP(t, ppdomain.NewTenantID, tenant.String()), plan, source, quoteAuth(t, authID), hiddenApprover,
	)
	if err != nil {
		t.Fatal(err)
	}
	return registration
}

func quoteAuth(t *testing.T, id string) ppdomain.VersionReference {
	t.Helper()
	auth, err := ppdomain.NewVersionReferenceIdentity(ppdomain.ArtifactCommercialAuthorization, id, "v1")
	if err != nil {
		t.Fatal(err)
	}
	return auth
}

func quoteSource(t *testing.T, name string) ppdomain.SourceFileIdentity {
	t.Helper()
	source, err := ppdomain.NewSourceFileIdentity(name, hiddenSHA)
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func quoteWeight(t *testing.T, value string) ppdomain.Weight {
	t.Helper()
	decimal, err := ppdomain.ParseDecimal(value)
	if err != nil {
		t.Fatal(err)
	}
	weight, err := ppdomain.NewWeight(decimal, ppdomain.WeightUnitKilogram)
	if err != nil {
		t.Fatal(err)
	}
	return weight
}

func quoteMoney(t *testing.T, value string, currency ppdomain.Currency) ppdomain.Money {
	t.Helper()
	money, err := ppdomain.NewMoneyFromString(value, currency)
	if err != nil {
		t.Fatal(err)
	}
	return money
}

func quoteValue[T any](t *testing.T, constructor func(string) (T, error), value string) T {
	t.Helper()
	built, err := constructor(value)
	if err != nil {
		t.Fatal(err)
	}
	return built
}

func quotePP[T any](t *testing.T, constructor func(string) (T, error), value string) T {
	return quoteValue(t, constructor, value)
}

func mustName(value string) domain.PartyName {
	name, err := domain.NewPartyName(value)
	if err != nil {
		panic(err)
	}
	return name
}

type quoteFailure string

func errQuote(text string) error { return quoteFailure(text) }

func (err quoteFailure) Error() string { return string(err) }
