package postgres_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 证参与方资料登记册（ADR-0369，迁移 0046）：各项往返重建、重放与冲突、按修订号升序交回全部修订、
// 空正文的快照不带任何资料键、跨租户读不到、无事务拒。

func newBusinessPartyProfiles(t *testing.T) (*adapter.BusinessPartyProfiles, *bentopg.DB) {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	profiles, err := adapter.NewBusinessPartyProfiles(db)
	if err != nil {
		t.Fatal(err)
	}
	return profiles, db
}

func fullPartyProfile(t *testing.T, tenant string, revision int, effectiveFrom time.Time) domain.BusinessPartyProfileRevision {
	t.Helper()
	address, err := domain.NewRegisteredAddress(pcValue(t, domain.NewRegistrationCountryCode, "CN"), []string{"上海市 SYN 路 1 号", "SYN 大厦 2 层"})
	if err != nil {
		t.Fatal(err)
	}
	office, err := domain.NewRegisteredAddress(pcValue(t, domain.NewRegistrationCountryCode, "SG"), []string{"SYN Road 9"})
	if err != nil {
		t.Fatal(err)
	}
	tax, err := domain.NewTaxRegistrationNumber(pcValue(t, domain.NewRegistrationNumberTypeCode, "SYN-VAT"), pcValue(t, domain.NewRegistrationNumber, "SYN-VAT-0001"))
	if err != nil {
		t.Fatal(err)
	}
	title, err := domain.NewInvoiceTitle("合成单位（演示）")
	if err != nil {
		t.Fatal(err)
	}
	invoicing, err := domain.NewInvoicingDetailsWithLocalizedTitles(title, "zh-Hans", []domain.LocalizedInvoiceTitle{{Language: "en", Title: "SYN Party Ltd."}})
	if err != nil {
		t.Fatal(err)
	}
	contact, err := domain.NewLegalEntityContactWithDetails("林悦", "finance@example.invalid", "021-0000-0201", domain.LegalEntityContactDetails{
		Department: "财务部", Purposes: []domain.ContactPurpose{domain.ContactFinance}, PrimaryFor: []domain.ContactPurpose{domain.ContactFinance},
		SocialAccounts: []domain.ContactSocialAccount{{Platform: "微信", Account: "syn-finance"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	content, err := domain.NewBusinessPartyProfileContent(domain.BusinessPartyProfileParts{
		Names:             []domain.LocalizedPartyName{{Language: "en", Name: "SYN Party Co., Ltd."}},
		RegisteredAddress: &address, OfficeAddress: &office,
		TaxNumbers: []domain.TaxRegistrationNumber{tax}, Invoicing: &invoicing, Contacts: []domain.LegalEntityContact{contact},
		BankAccounts: []domain.LegalEntityBankAccount{{HolderName: "合成单位", BankName: "SYN 银行", AccountNumber: "6222000000000001", Currency: "CNY"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	built, err := domain.NewBusinessPartyProfileRevision(pcTenant(t, tenant), pcValue(t, domain.NewPartyID, "party-1"), revision,
		pcValue(t, domain.NewBusinessPartyProfileBasisReference, "SYN-PROFILE-BASIS"), effectiveFrom, content)
	if err != nil {
		t.Fatal(err)
	}
	return built
}

func emptyPartyProfile(t *testing.T, revision int, effectiveFrom time.Time) domain.BusinessPartyProfileRevision {
	t.Helper()
	content, err := domain.NewBusinessPartyProfileContent(domain.BusinessPartyProfileParts{})
	if err != nil {
		t.Fatal(err)
	}
	built, err := domain.NewBusinessPartyProfileRevision(pcTenant(t, "tenant-1"), pcValue(t, domain.NewPartyID, "party-1"), revision,
		pcValue(t, domain.NewBusinessPartyProfileBasisReference, "SYN-PROFILE-BASIS"), effectiveFrom, content)
	if err != nil {
		t.Fatal(err)
	}
	return built
}

// Covers: 带全部各项的修订往返重建一字不差；重放答重复、同修订异内容答冲突；按修订号升序交回；空正文快照没有资料键；跨租户读不到。
func TestBusinessPartyProfileRegistryRoundTrip(t *testing.T) {
	profiles, db := newBusinessPartyProfiles(t)
	transactor := db.Transactor()
	ctx := t.Context()
	full := fullPartyProfile(t, "tenant-1", 1, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	save := func(revision domain.BusinessPartyProfileRevision) ports.BusinessPartyProfileSaveOutcome {
		var outcome ports.BusinessPartyProfileSaveOutcome
		mustWithinPublicationTransaction(t, transactor, ctx, func(txCtx context.Context) error {
			var err error
			outcome, err = profiles.SaveBusinessPartyProfile(txCtx, revision)
			return err
		})
		return outcome
	}
	if got := save(full); got != ports.BusinessPartyProfileSaved {
		t.Fatalf("首笔 = %v", got)
	}
	if got := save(full); got != ports.BusinessPartyProfileAlreadyRegistered {
		t.Fatalf("重放 = %v", got)
	}
	if got := save(emptyPartyProfile(t, 1, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))); got != ports.BusinessPartyProfileContentConflict {
		t.Fatalf("同修订异内容 = %v", got)
	}
	if got := save(emptyPartyProfile(t, 2, time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC))); got != ports.BusinessPartyProfileSaved {
		t.Fatalf("第二笔 = %v", got)
	}

	latest, found, err := profiles.LoadLatestBusinessPartyProfile(ctx, pcTenant(t, "tenant-1"), pcValue(t, domain.NewPartyID, "party-1"))
	if err != nil || !found || latest.Revision() != 2 {
		t.Fatalf("latest = %d %v %v", latest.Revision(), found, err)
	}
	history, err := profiles.ListBusinessPartyProfileRevisions(ctx, pcTenant(t, "tenant-1"), pcValue(t, domain.NewPartyID, "party-1"))
	if err != nil || len(history) != 2 || history[0].Revision.Revision() != 1 || history[1].Revision.Revision() != 2 {
		t.Fatalf("history = %+v err=%v", history, err)
	}
	if !reflectEqualParts(history[0].Revision.Content().Parts(), full.Content().Parts()) {
		t.Fatalf("往返重建不一致：\n%+v\n%+v", history[0].Revision.Content().Parts(), full.Content().Parts())
	}
	var snapshot string
	if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		executor, err := db.RequireExecutor(txCtx)
		if err != nil {
			return err
		}
		return executor.QueryRow(txCtx, `SELECT snapshot::text FROM party_commercial.business_party_profile_revision WHERE revision = 2`).Scan(&snapshot)
	}); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"names", "registeredAddress", "contacts", "bankAccounts", "taxRegistrationNumbers", "invoiceTitle"} {
		if strings.Contains(snapshot, `"`+key+`"`) {
			t.Fatalf("空正文的快照不该出现 %s：%s", key, snapshot)
		}
	}
	elsewhere, err := profiles.ListBusinessPartyProfileRevisions(ctx, pcTenant(t, "tenant-2"), pcValue(t, domain.NewPartyID, "party-1"))
	if err != nil || len(elsewhere) != 0 || elsewhere == nil {
		t.Fatalf("跨租户 = %+v err=%v（要空切片，不是 nil）", elsewhere, err)
	}
}

// Covers: PBC-08 行为面——参与方资料写口在无事务上下文被 RequireExecutor 拒绝。
func TestBusinessPartyProfileWritesRefuseToRunOutsideATransaction(t *testing.T) {
	profiles, _ := newBusinessPartyProfiles(t)
	if _, err := profiles.SaveBusinessPartyProfile(t.Context(), emptyPartyProfile(t, 1, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记参与方资料应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func reflectEqualParts(left, right domain.BusinessPartyProfileParts) bool {
	return fmtParts(left) == fmtParts(right)
}

func fmtParts(parts domain.BusinessPartyProfileParts) string {
	text := ""
	for _, name := range parts.Names {
		text += name.Language + "=" + name.Name + ";"
	}
	for _, address := range []*domain.RegisteredAddress{parts.RegisteredAddress, parts.OfficeAddress, parts.CorrespondenceAddress} {
		if address != nil {
			text += address.Country().String() + strings.Join(address.Lines(), "/") + ";"
		} else {
			text += "-;"
		}
	}
	for _, tax := range parts.TaxNumbers {
		text += tax.TypeCode().String() + tax.Number().String() + ";"
	}
	if parts.Invoicing != nil {
		text += parts.Invoicing.Title().String() + parts.Invoicing.TitleLanguage() + ";"
		for _, title := range parts.Invoicing.LocalizedTitles() {
			text += title.Language + title.Title + ";"
		}
	}
	for _, contact := range parts.Contacts {
		details := contact.Details()
		text += contact.Name() + contact.Email() + contact.Phone() + details.Department + ";"
		for _, account := range details.SocialAccounts {
			text += account.Platform + account.Account + ";"
		}
	}
	for _, account := range parts.BankAccounts {
		text += account.HolderName + account.BankName + account.AccountNumber + account.Currency + ";"
	}
	return text
}

// Covers: 拿公司主体钉参与方之前问的那一句——没登资料答否；最新一版带内容答是；再登一版空资料撤下后答否。
func TestHasBusinessPartyProfileReadsTheLatestRevision(t *testing.T) {
	profiles, db := newBusinessPartyProfiles(t)
	identities, err := adapter.NewPartyIdentityRegistrations(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	ask := func() bool {
		has, err := identities.HasBusinessPartyProfile(ctx, pcTenant(t, "tenant-1"), pcValue(t, domain.NewPartyID, "party-1"))
		if err != nil {
			t.Fatal(err)
		}
		return has
	}
	save := func(revision domain.BusinessPartyProfileRevision) {
		mustWithinPublicationTransaction(t, db.Transactor(), ctx, func(txCtx context.Context) error {
			_, err := profiles.SaveBusinessPartyProfile(txCtx, revision)
			return err
		})
	}
	if ask() {
		t.Fatal("没登资料应答否")
	}
	save(fullPartyProfile(t, "tenant-1", 1, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)))
	if !ask() {
		t.Fatal("登了带内容的一版应答是")
	}
	save(emptyPartyProfile(t, 2, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)))
	if ask() {
		t.Fatal("最新一版是空资料应答否")
	}
}
