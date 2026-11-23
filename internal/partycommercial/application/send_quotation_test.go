package application_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/spreadsheet"
)

// 发出按价格内容的处境、方向、摘要和身份时点分格。拒绝不落行。再导出不落行。

func TestApprovedSellDraftSendsAndASecondExportDoesNotAppend(t *testing.T) {
	book := newQuotationBook(t)
	sentAt := time.Date(2026, 11, 23, 4, 0, 0, 0, time.UTC)
	result, err := book.sender.Handle(context.Background(), book.command(t, sentAt, "digest-sell", "  另加条款 "))
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != application.QuotationSent || result.DeliveryRecorded || result.Send.DeliveryRecorded() {
		t.Fatalf("发出 = %s，送达 %v", result.Outcome, result.DeliveryRecorded)
	}
	if result.Send.Canonicalization() != "PPC-5" || result.Send.ContentDigest() != "digest-sell" {
		t.Fatalf("记下的版本或摘要不对：%s %s", result.Send.Canonicalization(), result.Send.ContentDigest())
	}
	if result.Send.Terms().Text() != "另加条款" || !strings.HasPrefix(result.Send.ID().String(), "QTS-") {
		t.Fatalf("条款或报价号：%q %s", result.Send.Terms().Text(), result.Send.ID())
	}
	if !bytesContain(result.Workbook, "另加条款") || !bytesContain(result.Workbook, "41.50") {
		t.Fatal("发出的文件应当带条款和价格金额")
	}
	if bytesContain(result.Workbook, "digest-sell") || bytesContain(result.Workbook, "PPC-5") {
		t.Fatal("对客文件不该带摘要或规范化版本")
	}
	if book.store.appends != 1 {
		t.Fatalf("发出落了 %d 行", book.store.appends)
	}

	exported, err := book.exporter.Handle(context.Background(), book.tenant, result.Send.ID())
	if err != nil {
		t.Fatal(err)
	}
	if exported.Outcome != application.QuotationExportReady || book.store.appends != 1 {
		t.Fatalf("再导出 = %s，行数 %d", exported.Outcome, book.store.appends)
	}
	if !bytesContain(exported.Body, "41.50") || bytesContain(exported.Body, "digest-sell") {
		t.Fatal("再导出的价格表应当还是那一格金额，并且不带摘要")
	}
}

func TestQuotationSendRefusalsDoNotAppend(t *testing.T) {
	book := newQuotationBook(t)
	sentAt := time.Date(2026, 11, 23, 4, 0, 0, 0, time.UTC)
	cases := []struct {
		name    string
		mutate  func(*application.SendQuotationCommand)
		prepare func()
		outcome application.SendQuotationOutcome
	}{
		{name: "已校验", prepare: func() { book.prices.facts["PLAN"] = notFixedFact() }, outcome: application.QuotationSendContentNotFixed},
		{name: "已撤下", prepare: func() { book.prices.facts["PLAN"] = standingFact(ports.QuotedPriceWithdrawn) }, outcome: application.QuotationSendWithdrawn},
		{name: "已发布改走价卡", prepare: func() { book.prices.facts["PLAN"] = standingFact(ports.QuotedPriceUseRegisteredCard) }, outcome: application.QuotationSendAlreadyRegistered},
		{name: "不是销售", prepare: func() { fact := readyFact(); fact.DirectionSell = false; book.prices.facts["PLAN"] = fact }, outcome: application.QuotationSendNotSell},
		{name: "摘要不符", mutate: func(command *application.SendQuotationCommand) { command.ContentDigest = "other" }, outcome: application.QuotationSendContentChanged},
		{name: "不在册", prepare: func() { delete(book.prices.facts, "PLAN") }, outcome: application.QuotationSendNotRegistered},
		{name: "缺终点", mutate: func(command *application.SendQuotationCommand) { command.EndsAt = time.Time{} }, outcome: application.QuotationSendNotAccepted},
		{name: "相对方未生效", prepare: func() { book.parties.shift(book.party, time.Hour) }, outcome: application.QuotationSendCounterpartyNotYetEffective},
		{name: "相对方已停用", prepare: func() { book.parties.deactivate(t, book.party, sentAt) }, outcome: application.QuotationSendCounterpartyDeactivated},
		{name: "责任法人不在册", mutate: func(command *application.SendQuotationCommand) {
			command.LegalEntity = commercialAppValue(t, domain.NewLegalEntityReference, "missing-le")
		}, outcome: application.QuotationSendLegalEntityNotRegistered},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			book.resetPrices()
			book.parties.reset(t, sentAt)
			if item.prepare != nil {
				item.prepare()
			}
			command := book.command(t, sentAt, "digest-sell", "")
			if item.mutate != nil {
				item.mutate(&command)
			}
			before := book.store.appends
			result, err := book.sender.Handle(context.Background(), command)
			if err != nil {
				t.Fatal(err)
			}
			if result.Outcome != item.outcome || book.store.appends != before || len(result.Workbook) != 0 {
				t.Fatalf("结果 = %s，行 %d→%d，文件 %d 字节", result.Outcome, before, book.store.appends, len(result.Workbook))
			}
		})
	}
}

func TestQuotationPriceReadFailureIsNotARefusal(t *testing.T) {
	book := newQuotationBook(t)
	book.prices.err = errors.New("price book down")
	_, err := book.sender.Handle(context.Background(), book.command(t, time.Date(2026, 11, 23, 4, 0, 0, 0, time.UTC), "digest-sell", ""))
	if err == nil || book.store.appends != 0 {
		t.Fatalf("err = %v，行数 %d", err, book.store.appends)
	}
}

func TestRegisteredCardQueryDoesNotCarrySelectionAxes(t *testing.T) {
	query := ports.QuotedPriceQuery{Lane: domain.QuotationPriceFromRegisteredCard, PlanID: "PLAN", PlanVersion: "v1"}
	if query.Lane.String() != "REGISTERED_PRICE_CARD" || query.PlanID == "" {
		t.Fatal(query.Lane)
	}
}

type quotationBook struct {
	tenant   domain.TenantID
	party    domain.PartyID
	entity   domain.LegalEntityReference
	sender   *application.SendQuotationHandler
	exporter *application.ExportQuotationHandler
	prices   *fakeQuotedPrices
	parties  *fakeIdentityBook
	store    *fakeQuotationStore
}

func newQuotationBook(t *testing.T) *quotationBook {
	t.Helper()
	sentAt := time.Date(2026, 11, 23, 4, 0, 0, 0, time.UTC)
	prices := &fakeQuotedPrices{facts: map[string]ports.QuotedPriceFact{}}
	parties := newIdentityBook(t, sentAt)
	store := &fakeQuotationStore{}
	sender, err := application.NewSendQuotationHandler(prices, parties, store)
	if err != nil {
		t.Fatal(err)
	}
	exporter, err := application.NewExportQuotationHandler(store, prices)
	if err != nil {
		t.Fatal(err)
	}
	book := &quotationBook{
		tenant: commercialAppValue(t, domain.NewTenantID, "tenant-1"),
		party:  commercialAppValue(t, domain.NewPartyID, "party-1"),
		entity: commercialAppValue(t, domain.NewLegalEntityReference, "le-1"),
		sender: sender, exporter: exporter, prices: prices, parties: parties, store: store,
	}
	book.resetPrices()
	return book
}

func (book *quotationBook) resetPrices() {
	book.prices.facts = map[string]ports.QuotedPriceFact{"PLAN": readyFact()}
	book.prices.err = nil
}

func (book *quotationBook) command(t *testing.T, sentAt time.Time, digest, terms string) application.SendQuotationCommand {
	t.Helper()
	return application.SendQuotationCommand{
		Tenant: book.tenant, Sender: commercialAppValue(t, domain.NewOperatorSubjectReference, "op-1"),
		SentAt: sentAt, Lane: domain.QuotationPriceFromApprovedDraft,
		PlanID: "PLAN", PlanVersion: "v1", ContentDigest: digest,
		Counterparty: book.party, LegalEntity: book.entity,
		StartsAt: sentAt, EndsAt: sentAt.Add(24 * time.Hour), Terms: terms,
	}
}

func readyFact() ports.QuotedPriceFact {
	return ports.QuotedPriceFact{
		Found: true, Standing: ports.QuotedPriceReady, DirectionSell: true,
		Canonicalization: "PPC-5", ContentDigest: "digest-sell",
		Workbook: tinyRateSheet(), MediaType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	}
}

func notFixedFact() ports.QuotedPriceFact {
	return ports.QuotedPriceFact{Found: true, Standing: ports.QuotedPriceNotFixed}
}

func standingFact(standing ports.QuotedPriceStanding) ports.QuotedPriceFact {
	return ports.QuotedPriceFact{Found: true, Standing: standing, DirectionSell: true, ContentDigest: "digest-sell", Canonicalization: "PPC-5"}
}

func tinyRateSheet() []byte {
	var body bytes.Buffer
	err := spreadsheet.Write(&body, []spreadsheet.WriteSheet{{
		Name:    "rates",
		Columns: []spreadsheet.WriteColumn{{Header: "amount", Text: true}},
		Rows:    [][]string{{"41.50"}},
	}})
	if err != nil {
		panic(err)
	}
	return body.Bytes()
}

func bytesContain(body []byte, text string) bool {
	return strings.Contains(string(body), text)
}

type fakeQuotedPrices struct {
	facts map[string]ports.QuotedPriceFact
	err   error
}

func (fake *fakeQuotedPrices) ReadQuotedPrice(_ context.Context, query ports.QuotedPriceQuery) (ports.QuotedPriceFact, error) {
	if fake.err != nil {
		return ports.QuotedPriceFact{}, fake.err
	}
	fact, found := fake.facts[query.PlanID]
	if !found {
		return ports.QuotedPriceFact{}, nil
	}
	return fact, nil
}

type fakeIdentityBook struct {
	parties  map[string]domain.BusinessPartyRegistration
	entities map[string]domain.LegalEntityRegistration
}

func newIdentityBook(t *testing.T, at time.Time) *fakeIdentityBook {
	t.Helper()
	tenant := commercialAppValue(t, domain.NewTenantID, "tenant-1")
	partyID := commercialAppValue(t, domain.NewPartyID, "party-1")
	pinnedID := commercialAppValue(t, domain.NewPartyID, "party-le")
	lifecycle, err := domain.NewIdentityLifecycle(at.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	basis := commercialAppValue(t, domain.NewIdentityBasisReference, "basis-1")
	party, err := domain.NewBusinessParty(tenant, partyID, commercialAppValue(t, domain.NewPartyName, "相对方"))
	if err != nil {
		t.Fatal(err)
	}
	pinned, err := domain.NewBusinessParty(tenant, pinnedID, commercialAppValue(t, domain.NewPartyName, "责任法人"))
	if err != nil {
		t.Fatal(err)
	}
	partyReg, err := domain.NewBusinessPartyRegistration(party, 1, basis, lifecycle)
	if err != nil {
		t.Fatal(err)
	}
	pinnedReg, err := domain.NewBusinessPartyRegistration(pinned, 1, basis, lifecycle)
	if err != nil {
		t.Fatal(err)
	}
	entity, err := domain.NewResponsibleLegalEntity(tenant, commercialAppValue(t, domain.NewLegalEntityReference, "le-1"), pinned)
	if err != nil {
		t.Fatal(err)
	}
	entityReg, err := domain.NewLegalEntityRegistration(entity, 1, basis, lifecycle)
	if err != nil {
		t.Fatal(err)
	}
	return &fakeIdentityBook{
		parties:  map[string]domain.BusinessPartyRegistration{partyID.String(): partyReg, pinnedID.String(): pinnedReg},
		entities: map[string]domain.LegalEntityRegistration{"le-1": entityReg},
	}
}

func (book *fakeIdentityBook) reset(t *testing.T, at time.Time) {
	t.Helper()
	fresh := newIdentityBook(t, at)
	book.parties = fresh.parties
	book.entities = fresh.entities
}

func (book *fakeIdentityBook) shift(party domain.PartyID, _ time.Duration) {
	current := book.parties[party.String()]
	later, err := domain.NewIdentityLifecycle(time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		panic(err)
	}
	replaced, err := domain.NewBusinessPartyRegistration(current.Party(), current.Revision(), current.Basis(), later)
	if err != nil {
		panic(err)
	}
	book.parties[party.String()] = replaced
}

func (book *fakeIdentityBook) deactivate(t *testing.T, party domain.PartyID, at time.Time) {
	t.Helper()
	current := book.parties[party.String()]
	next, err := current.Deactivate(commercialAppValue(t, domain.NewIdentityBasisReference, "stop"), at)
	if err != nil {
		t.Fatal(err)
	}
	book.parties[party.String()] = next
}

func (book *fakeIdentityBook) LoadLatestBusinessParty(_ context.Context, _ domain.TenantID, party domain.PartyID) (domain.BusinessPartyRegistration, bool, error) {
	registration, found := book.parties[party.String()]
	return registration, found, nil
}

func (book *fakeIdentityBook) LoadLatestLegalEntity(_ context.Context, _ domain.TenantID, entity domain.LegalEntityReference) (domain.LegalEntityRegistration, bool, error) {
	registration, found := book.entities[entity.String()]
	return registration, found, nil
}

type fakeQuotationStore struct {
	rows    map[string]domain.QuotationSend
	appends int
}

func (store *fakeQuotationStore) AppendQuotationSend(_ context.Context, send domain.QuotationSend) (ports.QuotationAppendOutcome, error) {
	if store.rows == nil {
		store.rows = map[string]domain.QuotationSend{}
	}
	key := send.Tenant().String() + "/" + send.ID().String()
	if existing, found := store.rows[key]; found {
		if existing.SameFact(send) {
			return ports.QuotationAppendReplay, nil
		}
		return ports.QuotationAppendConflict, nil
	}
	store.rows[key] = send
	store.appends++
	return ports.QuotationAppended, nil
}

func (store *fakeQuotationStore) GetQuotationSend(_ context.Context, tenant domain.TenantID, id domain.QuotationID) (domain.QuotationSend, bool, error) {
	send, found := store.rows[tenant.String()+"/"+id.String()]
	return send, found, nil
}

func (store *fakeQuotationStore) ListQuotationSends(context.Context, domain.TenantID, int, ports.QuotationSendCursor) (ports.QuotationSendPage, error) {
	return ports.QuotationSendPage{}, nil
}

func commercialAppValue[T interface{ String() string }](t *testing.T, constructor func(string) (T, error), value string) T {
	t.Helper()
	built, err := constructor(value)
	if err != nil {
		t.Fatal(err)
	}
	return built
}
