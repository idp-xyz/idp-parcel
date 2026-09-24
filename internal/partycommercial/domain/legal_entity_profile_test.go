package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

var (
	profileJanuary  = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	profileFebruary = time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	profileMarch    = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
)

func profileAddress(t *testing.T, country string, lines ...string) domain.RegisteredAddress {
	t.Helper()
	address, err := domain.NewRegisteredAddress(commercialValue(t, domain.NewRegistrationCountryCode, country), lines)
	if err != nil {
		t.Fatalf("new registered address: %v", err)
	}
	return address
}

func taxNumber(t *testing.T, code, number string) domain.TaxRegistrationNumber {
	t.Helper()
	built, err := domain.NewTaxRegistrationNumber(
		commercialValue(t, domain.NewRegistrationNumberTypeCode, code),
		commercialValue(t, domain.NewRegistrationNumber, number),
	)
	if err != nil {
		t.Fatalf("new tax registration number: %v", err)
	}
	return built
}

func invoicing(t *testing.T, title string) *domain.InvoicingDetails {
	t.Helper()
	details, err := domain.NewInvoicingDetails(commercialValue(t, domain.NewInvoiceTitle, title))
	if err != nil {
		t.Fatalf("new invoicing details: %v", err)
	}
	return &details
}

func profileContent(t *testing.T, line string, invoicingDetails *domain.InvoicingDetails) domain.LegalEntityProfileContent {
	t.Helper()
	content, err := domain.NewLegalEntityProfileContent(profileAddress(t, "XA", line), nil, invoicingDetails, nil)
	if err != nil {
		t.Fatalf("new profile content: %v", err)
	}
	return content
}

func profileRevision(
	t *testing.T,
	revision int,
	effectiveFrom time.Time,
	content domain.LegalEntityProfileContent,
) domain.LegalEntityProfileRevision {
	t.Helper()
	built, err := domain.NewLegalEntityProfileRevision(
		commercialValue(t, domain.NewTenantID, "tenant-1"),
		commercialValue(t, domain.NewLegalEntityReference, "le-1"),
		revision,
		commercialValue(t, domain.NewLegalEntityProfileBasisReference, "SYN-PROFILE-BASIS"),
		effectiveFrom,
		content,
	)
	if err != nil {
		t.Fatalf("new profile revision %d: %v", revision, err)
	}
	return built
}

// profileEntity 造一笔法人登记：生效自 effectiveFrom，deactivatedAt 非零即自该时点停用。
func profileEntity(t *testing.T, effectiveFrom, deactivatedAt time.Time) domain.LegalEntityRegistration {
	t.Helper()
	entity, err := domain.NewResponsibleLegalEntity(
		commercialValue(t, domain.NewTenantID, "tenant-1"),
		commercialValue(t, domain.NewLegalEntityReference, "le-1"),
		party(t, "party-1", "本租参与方"),
	)
	if err != nil {
		t.Fatalf("new legal entity: %v", err)
	}
	lifecycle, err := domain.NewIdentityLifecycle(effectiveFrom)
	if err != nil {
		t.Fatalf("new lifecycle: %v", err)
	}
	registration, err := domain.NewLegalEntityRegistration(
		entity, 1, commercialValue(t, domain.NewIdentityBasisReference, "basis-1"), lifecycle,
	)
	if err != nil {
		t.Fatalf("new legal entity registration: %v", err)
	}
	if deactivatedAt.IsZero() {
		return registration
	}
	deactivated, err := registration.Deactivate(commercialValue(t, domain.NewIdentityBasisReference, "basis-2"), deactivatedAt)
	if err != nil {
		t.Fatalf("deactivate legal entity: %v", err)
	}
	return deactivated
}

func TestLegalEntityProfileContentGuardsItsShape(t *testing.T) {
	country := commercialValue(t, domain.NewRegistrationCountryCode, "XA")
	if _, err := domain.NewRegisteredAddress(domain.RegistrationCountryCode{}, []string{"一号"}); !errors.Is(err, domain.ErrInvalidLegalEntityProfile) {
		t.Fatalf("address without country: %v", err)
	}
	if _, err := domain.NewRegisteredAddress(country, nil); !errors.Is(err, domain.ErrInvalidLegalEntityProfile) {
		t.Fatalf("address without lines: %v", err)
	}
	if _, err := domain.NewRegisteredAddress(country, []string{"一号", "  "}); !errors.Is(err, domain.ErrInvalidLegalEntityProfile) {
		t.Fatalf("address with a blank line: %v", err)
	}
	if _, err := domain.NewLegalEntityContact(" ", "a@example.invalid", ""); !errors.Is(err, domain.ErrInvalidLegalEntityProfile) {
		t.Fatalf("contact without name: %v", err)
	}
	if _, err := domain.NewLegalEntityProfileContent(domain.RegisteredAddress{}, nil, nil, nil); !errors.Is(err, domain.ErrInvalidLegalEntityProfile) {
		t.Fatalf("content without address: %v", err)
	}

	address := profileAddress(t, "XA", "SYN 一号路", "SYN 二层")
	duplicated := []domain.TaxRegistrationNumber{taxNumber(t, "SYN-XA-TAX", "T-1"), taxNumber(t, "SYN-XA-TAX", "T-1")}
	if _, err := domain.NewLegalEntityProfileContent(address, duplicated, nil, nil); !errors.Is(err, domain.ErrInvalidLegalEntityProfile) {
		t.Fatalf("same tax number twice: %v", err)
	}

	contact, err := domain.NewLegalEntityContact("SYN 联系人", "", "  ")
	if err != nil {
		t.Fatalf("contact with name only: %v", err)
	}
	if contact.Email() != "" || contact.Phone() != "" {
		t.Fatalf("blank email and phone should read back empty, got %q / %q", contact.Email(), contact.Phone())
	}
	content, err := domain.NewLegalEntityProfileContent(
		address,
		[]domain.TaxRegistrationNumber{
			taxNumber(t, "SYN-XA-VAT", "V-1"),
			taxNumber(t, "SYN-XA-TAX", "T-2"),
			taxNumber(t, "SYN-XA-TAX", "T-1"),
		},
		nil,
		[]domain.LegalEntityContact{contact},
	)
	if err != nil {
		t.Fatalf("content: %v", err)
	}
	var order []string
	for _, number := range content.TaxNumbers() {
		order = append(order, number.TypeCode().String()+"/"+number.Number().String())
	}
	if got := strings.Join(order, ","); got != "SYN-XA-TAX/T-1,SYN-XA-TAX/T-2,SYN-XA-VAT/V-1" {
		t.Fatalf("tax numbers should be ordered by type then number (one type may carry several), got %s", got)
	}
	if _, has := content.Invoicing(); has {
		t.Fatal("content without invoicing details should say so")
	}
	if got := content.Address().Lines(); len(got) != 2 || got[0] != "SYN 一号路" {
		t.Fatalf("address lines should keep their order, got %v", got)
	}
}

func TestLegalEntityProfileRevisionRequiresBasisAndEffectiveFrom(t *testing.T) {
	tenant := commercialValue(t, domain.NewTenantID, "tenant-1")
	entity := commercialValue(t, domain.NewLegalEntityReference, "le-1")
	basis := commercialValue(t, domain.NewLegalEntityProfileBasisReference, "SYN-PROFILE-BASIS")
	content := profileContent(t, "SYN 一号路", nil)

	for name, build := range map[string]func() error{
		"revision zero": func() error {
			_, err := domain.NewLegalEntityProfileRevision(tenant, entity, 0, basis, profileJanuary, content)
			return err
		},
		"no basis": func() error {
			_, err := domain.NewLegalEntityProfileRevision(tenant, entity, 1, domain.LegalEntityProfileBasisReference{}, profileJanuary, content)
			return err
		},
		"no effective-from": func() error {
			_, err := domain.NewLegalEntityProfileRevision(tenant, entity, 1, basis, time.Time{}, content)
			return err
		},
		"no content": func() error {
			_, err := domain.NewLegalEntityProfileRevision(tenant, entity, 1, basis, profileJanuary, domain.LegalEntityProfileContent{})
			return err
		},
	} {
		if err := build(); !errors.Is(err, domain.ErrInvalidLegalEntityProfile) {
			t.Errorf("%s: %v", name, err)
		}
	}

	local := time.Date(2026, 1, 1, 8, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	revision, err := domain.NewLegalEntityProfileRevision(tenant, entity, 1, basis, local, content)
	if err != nil {
		t.Fatalf("revision: %v", err)
	}
	if revision.EffectiveFrom().Location() != time.UTC || !revision.EffectiveFrom().Equal(local) {
		t.Fatalf("effective-from should be kept as the same instant in UTC, got %v", revision.EffectiveFrom())
	}
	if reference := revision.Reference(); reference.Revision() != 1 || reference.LegalEntity() != entity || reference.Tenant() != tenant {
		t.Fatalf("reference should name tenant, legal entity and revision, got %+v", reference)
	}
}

func resolvedRevisionAt(t *testing.T, entity domain.LegalEntityRegistration, chain []domain.LegalEntityProfileRevision, at time.Time) int {
	t.Helper()
	resolution, err := domain.ResolveLegalEntityProfile(entity, chain, at)
	if err != nil {
		t.Fatalf("resolve at %v: %v", at, err)
	}
	revision, ok := resolution.Resolved()
	if !ok {
		t.Fatalf("resolve at %v: outcome %s (cause %s), want RESOLVED", at, resolution.Outcome(), resolution.IncompleteCause())
	}
	return revision.Revision()
}

func TestLegalEntityProfileResolutionTakesTheLatestRevisionInEffect(t *testing.T) {
	entity := profileEntity(t, profileJanuary, time.Time{})
	first := profileRevision(t, 1, profileJanuary, profileContent(t, "SYN 旧址", invoicing(t, "SYN 抬头")))
	second := profileRevision(t, 2, profileMarch, profileContent(t, "SYN 新址", invoicing(t, "SYN 抬头")))

	chain := []domain.LegalEntityProfileRevision{second, first}
	if got := resolvedRevisionAt(t, entity, chain, profileFebruary); got != 1 {
		t.Fatalf("before the second revision takes effect: revision %d, want 1", got)
	}
	if got := resolvedRevisionAt(t, entity, chain, profileMarch); got != 2 {
		t.Fatalf("at the second revision's effective time: revision %d, want 2", got)
	}

	// 追溯生效：第三笔登记在后、生效时点在前两笔之间。它自二月起取代全部前序修订，三月以后也不让第二笔冒出来。
	retroactive := profileRevision(t, 3, profileFebruary, profileContent(t, "SYN 更正址", invoicing(t, "SYN 抬头")))
	chain = append(chain, retroactive)
	if got := resolvedRevisionAt(t, entity, chain, profileJanuary.AddDate(0, 0, 14)); got != 1 {
		t.Fatalf("before the retroactive revision's effective time: revision %d, want 1", got)
	}
	if got := resolvedRevisionAt(t, entity, chain, profileFebruary.AddDate(0, 0, 14)); got != 3 {
		t.Fatalf("within the retroactive window: revision %d, want 3", got)
	}
	if got := resolvedRevisionAt(t, entity, chain, profileMarch.AddDate(0, 0, 14)); got != 3 {
		t.Fatalf("after the superseded revision's effective time: revision %d, want 3", got)
	}
}

func TestLegalEntityProfileResolutionAnswersNonSuccessExplicitly(t *testing.T) {
	withTitle := profileRevision(t, 1, profileFebruary, profileContent(t, "SYN 一号路", invoicing(t, "SYN 抬头")))
	withoutTitle := profileRevision(t, 2, profileMarch, profileContent(t, "SYN 一号路", nil))
	chain := []domain.LegalEntityProfileRevision{withTitle, withoutTitle}
	active := profileEntity(t, profileJanuary, time.Time{})

	resolution, err := domain.ResolveLegalEntityProfile(active, chain, profileJanuary)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolution.Outcome() != domain.LegalEntityProfileIncomplete || resolution.IncompleteCause() != domain.LegalEntityProfileNoEffectiveRevision {
		t.Fatalf("no revision in effect yet: %s / %s", resolution.Outcome(), resolution.IncompleteCause())
	}
	if _, ok := resolution.Resolved(); ok {
		t.Fatal("an incomplete profile must not hand out a revision to issue against")
	}

	resolution, err = domain.ResolveLegalEntityProfile(active, chain, profileMarch)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolution.Outcome() != domain.LegalEntityProfileIncomplete || resolution.IncompleteCause() != domain.LegalEntityProfileNoInvoicingDetails {
		t.Fatalf("revision in effect lacks invoicing details: %s / %s", resolution.Outcome(), resolution.IncompleteCause())
	}
	if _, ok := resolution.Resolved(); ok {
		t.Fatal("missing invoicing details must not fall back to an earlier revision or a default")
	}
	if effective, ok := resolution.EffectiveRevision(); !ok || effective.Revision() != 2 {
		t.Fatalf("the revision that lacks invoicing details should be named, got %d / %v", effective.Revision(), ok)
	}

	deactivated := profileEntity(t, profileJanuary, profileMarch)
	if got := resolvedRevisionAt(t, deactivated, chain[:1], profileFebruary); got != 1 {
		t.Fatalf("before deactivation the profile still resolves, got revision %d", got)
	}
	resolution, err = domain.ResolveLegalEntityProfile(deactivated, chain[:1], profileMarch)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolution.Outcome() != domain.LegalEntityProfileEntityDeactivated {
		t.Fatalf("from the deactivation time on: %s, want LEGAL_ENTITY_DEACTIVATED", resolution.Outcome())
	}

	notYet := profileEntity(t, profileMarch, time.Time{})
	resolution, err = domain.ResolveLegalEntityProfile(notYet, chain[:1], profileFebruary)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolution.Outcome() != domain.LegalEntityProfileEntityNotEffective {
		t.Fatalf("legal entity not yet effective: %s, want LEGAL_ENTITY_NOT_EFFECTIVE", resolution.Outcome())
	}

	if got := domain.NotRegisteredLegalEntityProfileResolution().Outcome(); got != domain.LegalEntityProfileEntityNotRegistered {
		t.Fatalf("not registered: %s", got)
	}
}

func TestLegalEntityProfileResolutionRefusesAChainItCannotTrust(t *testing.T) {
	entity := profileEntity(t, profileJanuary, time.Time{})
	first := profileRevision(t, 1, profileJanuary, profileContent(t, "SYN 一号路", invoicing(t, "SYN 抬头")))

	if _, err := domain.ResolveLegalEntityProfile(entity, []domain.LegalEntityProfileRevision{first, first}, profileFebruary); !errors.Is(err, domain.ErrInvalidLegalEntityProfileResolution) {
		t.Fatalf("duplicated revision: %v", err)
	}
	other, err := domain.NewLegalEntityProfileRevision(
		commercialValue(t, domain.NewTenantID, "tenant-1"),
		commercialValue(t, domain.NewLegalEntityReference, "le-2"),
		1,
		commercialValue(t, domain.NewLegalEntityProfileBasisReference, "SYN-PROFILE-BASIS"),
		profileJanuary,
		profileContent(t, "SYN 一号路", nil),
	)
	if err != nil {
		t.Fatalf("other revision: %v", err)
	}
	if _, err := domain.ResolveLegalEntityProfile(entity, []domain.LegalEntityProfileRevision{other}, profileFebruary); !errors.Is(err, domain.ErrInvalidLegalEntityProfileResolution) {
		t.Fatalf("revision of another legal entity: %v", err)
	}
	if _, err := domain.ResolveLegalEntityProfile(entity, nil, time.Time{}); !errors.Is(err, domain.ErrInvalidLegalEntityProfileResolution) {
		t.Fatalf("zero resolution time: %v", err)
	}
}
