package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 票 legal-entity-profile/03 的真库用例：法人资料经应用层用例落进 0036，再经解析用例按时点读回。法人 le-1 由
// newLegalEntityIdentityFixture 的身份登记用例登成带身份层（XA）的责任法人，生效自 2026-02-01。国家 / 地区取
// ISO 3166 用户自定义码，号、地址与抬头全为合成值。

type legalEntityProfileFixture struct {
	legalEntityIdentityFixture
	profiles  *adapter.LegalEntityProfiles
	registrar *application.RegisterLegalEntityProfileHandler
	resolver  *application.ResolveLegalEntityProfileHandler
}

func newLegalEntityProfileFixture(t *testing.T) legalEntityProfileFixture {
	t.Helper()
	identity := newLegalEntityIdentityFixture(t)
	if got := identity.register(t, identityCommand(t, 1, "XA", "SYN-XA-LIFETIME", "SYN-XA-000001")); got.Outcome() != application.PartyIdentityRegistered {
		t.Fatalf("登记法人 le-1：%s（%v）", got.Outcome(), got.Cause())
	}
	profiles, err := adapter.NewLegalEntityProfiles(identity.db)
	if err != nil {
		t.Fatalf("构造法人资料登记册：%v", err)
	}
	return legalEntityProfileFixture{
		legalEntityIdentityFixture: identity,
		profiles:                   profiles,
		registrar:                  application.NewRegisterLegalEntityProfileHandler(profiles, identity.identities, identity.types),
		resolver:                   application.NewResolveLegalEntityProfileHandler(profiles, identity.identities),
	}
}

func (fixture legalEntityProfileFixture) registerProfile(
	t *testing.T,
	command application.RegisterLegalEntityProfileCommand,
) application.LegalEntityProfileResult {
	t.Helper()
	var result application.LegalEntityProfileResult
	mustWithinPublicationTransaction(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		var err error
		result, err = fixture.registrar.Register(txCtx, command)
		return err
	})
	return result
}

func (fixture legalEntityProfileFixture) resolveAt(t *testing.T, at time.Time) domain.LegalEntityProfileResolution {
	t.Helper()
	resolution, err := fixture.resolver.Resolve(t.Context(), pcTenant(t, "tenant-1"), pcValue(t, domain.NewLegalEntityReference, "le-1"), at)
	if err != nil {
		t.Fatalf("解析 %s：%v", at.Format(time.DateOnly), err)
	}
	return resolution
}

// profileAt 造 le-1 的一笔资料修订：地址在 country、一行 line；title 为空即不带开票资料；带一个合成税号。
func profileAt(t *testing.T, revision int, effectiveFrom time.Time, country, line, title string) application.RegisterLegalEntityProfileCommand {
	t.Helper()
	address, err := domain.NewRegisteredAddress(pcValue(t, domain.NewRegistrationCountryCode, country), []string{line})
	if err != nil {
		t.Fatalf("new address: %v", err)
	}
	tax, err := domain.NewTaxRegistrationNumber(
		pcValue(t, domain.NewRegistrationNumberTypeCode, "SYN-XA-TAX"),
		pcValue(t, domain.NewRegistrationNumber, "SYN-XA-TAX-0001"),
	)
	if err != nil {
		t.Fatalf("new tax number: %v", err)
	}
	contact, err := domain.NewLegalEntityContact("SYN 联系人", "syn@example.invalid", "")
	if err != nil {
		t.Fatalf("new contact: %v", err)
	}
	command := application.RegisterLegalEntityProfileCommand{
		Tenant:        pcTenant(t, "tenant-1"),
		Entity:        pcValue(t, domain.NewLegalEntityReference, "le-1"),
		Revision:      revision,
		Basis:         pcValue(t, domain.NewLegalEntityProfileBasisReference, "SYN-PROFILE-BASIS"),
		EffectiveFrom: effectiveFrom,
		Address:       address,
		TaxNumbers:    []domain.TaxRegistrationNumber{tax},
		Contacts:      []domain.LegalEntityContact{contact},
	}
	if title != "" {
		details, err := domain.NewInvoicingDetails(pcValue(t, domain.NewInvoiceTitle, title))
		if err != nil {
			t.Fatalf("new invoicing details: %v", err)
		}
		command.Invoicing = &details
	}
	return command
}

func day(month time.Month, date int) time.Time {
	return time.Date(2026, month, date, 0, 0, 0, 0, time.UTC)
}

func expectResolvedRevision(t *testing.T, resolution domain.LegalEntityProfileResolution, want int, line string) {
	t.Helper()
	revision, ok := resolution.Resolved()
	if !ok {
		t.Fatalf("outcome %s（%s），want revision %d resolved", resolution.Outcome(), resolution.IncompleteCause(), want)
	}
	if revision.Revision() != want || revision.Content().Address().Lines()[0] != line {
		t.Fatalf("resolved revision %d %v, want revision %d at %q", revision.Revision(), revision.Content().Address().Lines(), want, line)
	}
}

func expectProfileRefused(t *testing.T, result application.LegalEntityProfileResult, mentions string) {
	t.Helper()
	if result.Outcome() != application.LegalEntityProfileNotAccepted || result.Cause() == nil ||
		!strings.Contains(result.Cause().Error(), mentions) {
		t.Fatalf("outcome %s（%v），want NOT_ACCEPTED mentioning %q", result.Outcome(), result.Cause(), mentions)
	}
}

// Covers: 票 legal-entity-profile/03 完成判据「真库用例：前后两修订按生效时点切换；未来生效的修订在生效前不参与解析；
// 追溯生效的修订登记后，按时点重读答新值——而固定过的引用仍指向旧修订；法人停用后不再参与新的解析；地址国家不符、
// 法人已停用各拒一条；『资料不全』两种成因各一条」。
func TestLegalEntityProfileResolvesByEffectiveTimeAgainstTheRealRegister(t *testing.T) {
	fixture := newLegalEntityProfileFixture(t)
	register := func(command application.RegisterLegalEntityProfileCommand) {
		t.Helper()
		if got := fixture.registerProfile(t, command); got.Outcome() != application.LegalEntityProfileRegistered {
			t.Fatalf("登记资料修订 %d：%s（%v）", command.Revision, got.Outcome(), got.Cause())
		}
	}

	register(profileAt(t, 1, day(time.March, 1), "XA", "SYN 旧址", "SYN 抬头"))
	resolution := fixture.resolveAt(t, day(time.February, 15))
	if resolution.Outcome() != domain.LegalEntityProfileIncomplete || resolution.IncompleteCause() != domain.LegalEntityProfileNoEffectiveRevision {
		t.Fatalf("法人已生效、资料修订未到生效时点：%s / %s，want PROFILE_INCOMPLETE / NO_EFFECTIVE_REVISION",
			resolution.Outcome(), resolution.IncompleteCause())
	}

	register(profileAt(t, 2, day(time.June, 1), "XA", "SYN 新址", "SYN 抬头"))
	expectResolvedRevision(t, fixture.resolveAt(t, day(time.April, 1)), 1, "SYN 旧址")
	fixed, _ := fixture.resolveAt(t, day(time.July, 1)).Resolved()
	if fixed.Revision() != 2 {
		t.Fatalf("七月应切到第二笔，得到修订 %d", fixed.Revision())
	}
	fixedReference := fixed.Reference()

	// 追溯生效：第三笔五月起生效，取代其后的全部前序修订——七月重读答它；而七月初开出、固定了第二笔引用的单据
	// 照旧指向第二笔，第二笔的内容一个字节没变。
	register(profileAt(t, 3, day(time.May, 1), "XA", "SYN 更正址", "SYN 抬头"))
	expectResolvedRevision(t, fixture.resolveAt(t, day(time.July, 1)), 3, "SYN 更正址")
	expectResolvedRevision(t, fixture.resolveAt(t, day(time.April, 1)), 1, "SYN 旧址")
	history, err := fixture.profiles.ListLegalEntityProfileRevisions(t.Context(), fixedReference.Tenant(), fixedReference.LegalEntity())
	if err != nil {
		t.Fatalf("读修订历史：%v", err)
	}
	if len(history) != 3 || history[fixedReference.Revision()-1].Revision != 2 || history[1].AddressLines[0] != "SYN 新址" {
		t.Fatalf("固定过的引用（修订 %d）应仍指向原内容，历史为 %+v", fixedReference.Revision(), history)
	}

	register(profileAt(t, 4, day(time.September, 1), "XA", "SYN 更正址", ""))
	resolution = fixture.resolveAt(t, day(time.October, 1))
	if resolution.Outcome() != domain.LegalEntityProfileIncomplete || resolution.IncompleteCause() != domain.LegalEntityProfileNoInvoicingDetails {
		t.Fatalf("有效修订缺开票资料：%s / %s，want PROFILE_INCOMPLETE / NO_INVOICING_DETAILS", resolution.Outcome(), resolution.IncompleteCause())
	}
	if _, ok := resolution.Resolved(); ok {
		t.Fatal("缺开票资料不得退回前一笔修订，也不得以默认值补齐")
	}

	expectProfileRefused(t, fixture.registerProfile(t, profileAt(t, 5, day(time.November, 1), "XB", "SYN 他国址", "SYN 抬头")), "不一致")

	var deactivation application.PartyRegistryResult
	mustWithinPublicationTransaction(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		var err error
		deactivation, err = fixture.handler.Deactivate(txCtx, application.DeactivatePartyIdentityCommand{
			Tenant:   pcTenant(t, "tenant-1"),
			Kind:     application.LegalEntityIdentity,
			ID:       "le-1",
			Revision: 2,
			Basis:    pcValue(t, domain.NewIdentityBasisReference, "SYN-DEACTIVATION"),
			At:       day(time.December, 1),
		})
		return err
	})
	if deactivation.Outcome() != application.PartyIdentityDeactivated {
		t.Fatalf("停用法人：%s（%v）", deactivation.Outcome(), deactivation.Cause())
	}
	if got := fixture.resolveAt(t, day(time.December, 15)).Outcome(); got != domain.LegalEntityProfileEntityDeactivated {
		t.Fatalf("法人停用之后：%s，want LEGAL_ENTITY_DEACTIVATED", got)
	}
	expectProfileRefused(t, fixture.registerProfile(t, profileAt(t, 5, day(time.November, 1), "XA", "SYN 更正址", "SYN 抬头")), "已停用")

	var rows int
	if err := fixture.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM party_commercial.legal_entity_profile_revision WHERE tenant_id = 'tenant-1' AND legal_entity_id = 'le-1'`,
	).Scan(&rows); err != nil {
		t.Fatalf("数行：%v", err)
	}
	if rows != 4 {
		t.Fatalf("两条被拒的修订一个字节都不该写，册上应只有四笔，得到 %d", rows)
	}
}

func TestLegalEntityProfileRegisterReplaysAndReadsBack(t *testing.T) {
	fixture := newLegalEntityProfileFixture(t)
	first := profileAt(t, 1, day(time.March, 1), "XA", "SYN 旧址", "SYN 抬头")
	if got := fixture.registerProfile(t, first); got.Outcome() != application.LegalEntityProfileRegistered {
		t.Fatalf("登记：%s（%v）", got.Outcome(), got.Cause())
	}
	if got := fixture.registerProfile(t, first); got.Outcome() != application.LegalEntityProfileAlreadyRegistered {
		t.Fatalf("重放：%s", got.Outcome())
	}
	if got := fixture.registerProfile(t, profileAt(t, 1, day(time.March, 1), "XA", "SYN 旧址", "SYN 另一抬头")); got.Outcome() != application.LegalEntityProfileContentConflict {
		t.Fatalf("同修订异内容：%s", got.Outcome())
	}

	latest, found, err := fixture.profiles.LoadLatestLegalEntityProfile(t.Context(), pcTenant(t, "tenant-1"), pcValue(t, domain.NewLegalEntityReference, "le-1"))
	if err != nil || !found {
		t.Fatalf("取最新修订：found=%v err=%v", found, err)
	}
	content := latest.Content()
	details, hasInvoicing := content.Invoicing()
	contacts := content.Contacts()
	if !hasInvoicing || details.Title().String() != "SYN 抬头" || len(content.TaxNumbers()) != 1 ||
		len(contacts) != 1 || contacts[0].Email() != "syn@example.invalid" || contacts[0].Phone() != "" ||
		!latest.EffectiveFrom().Equal(day(time.March, 1)) {
		t.Fatalf("快照读回走样：%+v", latest)
	}

	rows, err := fixture.profiles.ListLegalEntityProfileRevisions(t.Context(), pcTenant(t, "tenant-1"), pcValue(t, domain.NewLegalEntityReference, "le-1"))
	if err != nil || len(rows) != 1 {
		t.Fatalf("修订历史：%d 行，err=%v", len(rows), err)
	}
	if row := rows[0]; !row.HasInvoicing || row.InvoiceTitle != "SYN 抬头" || row.AddressCountry != "XA" ||
		len(row.TaxNumbers) != 1 || row.TaxNumbers[0].Number != "SYN-XA-TAX-0001" || row.RegisteredAt.IsZero() {
		t.Fatalf("历史行转写走样：%+v", row)
	}
	other, err := fixture.profiles.ListLegalEntityProfileRevisions(t.Context(), pcTenant(t, "tenant-2"), pcValue(t, domain.NewLegalEntityReference, "le-1"))
	if err != nil || len(other) != 0 {
		t.Fatalf("跨租户应答零行无错：%d 行，err=%v", len(other), err)
	}

	withoutCollections := profileAt(t, 2, day(time.April, 1), "XA", "SYN 旧址", "")
	withoutCollections.TaxNumbers, withoutCollections.Contacts = nil, nil
	if got := fixture.registerProfile(t, withoutCollections); got.Outcome() != application.LegalEntityProfileRegistered {
		t.Fatalf("不带税号与联系人：%s（%v）", got.Outcome(), got.Cause())
	}
	var taxShape, contactShape string
	var invoiceTitleIsNull bool
	if err := fixture.pool.QueryRow(t.Context(),
		`SELECT jsonb_typeof(tax_registration_numbers), jsonb_typeof(contacts), invoice_title IS NULL
		   FROM party_commercial.legal_entity_profile_revision
		  WHERE tenant_id = 'tenant-1' AND legal_entity_id = 'le-1' AND revision = 2`,
	).Scan(&taxShape, &contactShape, &invoiceTitleIsNull); err != nil {
		t.Fatalf("读结构化列：%v", err)
	}
	if taxShape != "array" || contactShape != "array" || !invoiceTitleIsNull {
		t.Fatalf("空集合应落成空数组、缺开票资料应落成 NULL，得到 %s / %s / %v", taxShape, contactShape, invoiceTitleIsNull)
	}
}
