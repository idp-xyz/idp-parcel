package application_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// fakeProfiles 是法人资料登记册的内存替身：同修订同内容答重放、同修订异内容答冲突，与真登记册同形。
type fakeProfiles struct {
	revisions map[int]domain.LegalEntityProfileRevision
	chainErr  error
}

var (
	_ ports.LegalEntityProfileRegistry  = (*fakeProfiles)(nil)
	_ ports.LegalEntityProfileChainRead = (*fakeProfiles)(nil)
)

func newFakeProfiles() *fakeProfiles {
	return &fakeProfiles{revisions: map[int]domain.LegalEntityProfileRevision{}}
}

func (profiles *fakeProfiles) SaveLegalEntityProfile(
	_ context.Context,
	revision domain.LegalEntityProfileRevision,
) (ports.LegalEntityProfileSaveOutcome, error) {
	if existing, found := profiles.revisions[revision.Revision()]; found {
		if reflect.DeepEqual(existing, revision) {
			return ports.LegalEntityProfileRegistryAlreadyRegistered, nil
		}
		return ports.LegalEntityProfileRegistryContentConflict, nil
	}
	profiles.revisions[revision.Revision()] = revision
	return ports.LegalEntityProfileRegistrySaved, nil
}

func (profiles *fakeProfiles) LoadLatestLegalEntityProfile(
	_ context.Context,
	_ domain.TenantID,
	_ domain.LegalEntityReference,
) (domain.LegalEntityProfileRevision, bool, error) {
	latest, found := domain.LegalEntityProfileRevision{}, false
	for _, revision := range profiles.revisions {
		if !found || revision.Revision() > latest.Revision() {
			latest, found = revision, true
		}
	}
	return latest, found, nil
}

func (profiles *fakeProfiles) LoadLegalEntityProfileChain(
	_ context.Context,
	_ domain.TenantID,
	_ domain.LegalEntityReference,
) ([]domain.LegalEntityProfileRevision, error) {
	if profiles.chainErr != nil {
		return nil, profiles.chainErr
	}
	chain := make([]domain.LegalEntityProfileRevision, 0, len(profiles.revisions))
	for _, revision := range profiles.revisions {
		chain = append(chain, revision)
	}
	return chain, nil
}

// fakeLegalEntities 只放一个法人 le-1；registration 为零值即未登记。
type fakeLegalEntities struct {
	registration domain.LegalEntityRegistration
	found        bool
}

var _ ports.LegalEntityRegistrationLookup = (*fakeLegalEntities)(nil)

func (entities *fakeLegalEntities) LoadLatestLegalEntity(
	_ context.Context,
	_ domain.TenantID,
	_ domain.LegalEntityReference,
) (domain.LegalEntityRegistration, bool, error) {
	return entities.registration, entities.found, nil
}

var profileEntityEffectiveFrom = time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

// profileLegalEntity 造 le-1：生效自二月，country 非空即带身份层（该国家 / 地区 + 一个合成终身注册号）。
func profileLegalEntity(t *testing.T, country string) domain.LegalEntityRegistration {
	t.Helper()
	party, err := domain.NewBusinessParty(
		identityValue(t, domain.NewTenantID, "tenant-1"),
		identityValue(t, domain.NewPartyID, "party-1"),
		identityValue(t, domain.NewPartyName, "SYN 参与方"),
	)
	if err != nil {
		t.Fatalf("new business party: %v", err)
	}
	entity, err := domain.NewResponsibleLegalEntity(
		identityValue(t, domain.NewTenantID, "tenant-1"),
		identityValue(t, domain.NewLegalEntityReference, "le-1"),
		party,
	)
	if err != nil {
		t.Fatalf("new legal entity: %v", err)
	}
	lifecycle, err := domain.NewIdentityLifecycle(profileEntityEffectiveFrom)
	if err != nil {
		t.Fatalf("new lifecycle: %v", err)
	}
	registration, err := domain.NewLegalEntityRegistration(
		entity, 1, identityValue(t, domain.NewIdentityBasisReference, "SYN-BASIS"), lifecycle,
	)
	if err != nil {
		t.Fatalf("new legal entity registration: %v", err)
	}
	if country == "" {
		return registration
	}
	number, err := domain.NewLifetimeRegistrationNumber(
		identityValue(t, domain.NewRegistrationNumberTypeCode, "SYN-XA-LIFETIME"),
		identityValue(t, domain.NewRegistrationNumber, "SYN-XA-000001"),
	)
	if err != nil {
		t.Fatalf("new lifetime number: %v", err)
	}
	layer, err := domain.NewLegalEntityIdentityLayer(
		identityValue(t, domain.NewRegistrationCountryCode, country), []domain.LifetimeRegistrationNumber{number},
	)
	if err != nil {
		t.Fatalf("new identity layer: %v", err)
	}
	registration, err = registration.WithIdentityLayer(layer, nil)
	if err != nil {
		t.Fatalf("with identity layer: %v", err)
	}
	return registration
}

// profileCommand 造 le-1 的一笔资料修订：地址在 country，税号按 typeCode/number 给一个（typeCode 为空即不带），
// 开票抬头 title 为空即不带开票资料。
func profileCommand(t *testing.T, revision int, country, typeCode, number, title string) application.RegisterLegalEntityProfileCommand {
	t.Helper()
	address, err := domain.NewRegisteredAddress(identityValue(t, domain.NewRegistrationCountryCode, country), []string{"SYN 一号路"})
	if err != nil {
		t.Fatalf("new address: %v", err)
	}
	command := application.RegisterLegalEntityProfileCommand{
		Tenant:        identityValue(t, domain.NewTenantID, "tenant-1"),
		Entity:        identityValue(t, domain.NewLegalEntityReference, "le-1"),
		Revision:      revision,
		Basis:         identityValue(t, domain.NewLegalEntityProfileBasisReference, "SYN-PROFILE-BASIS"),
		EffectiveFrom: profileEntityEffectiveFrom,
		Address:       address,
	}
	if typeCode != "" {
		tax, err := domain.NewTaxRegistrationNumber(
			identityValue(t, domain.NewRegistrationNumberTypeCode, typeCode),
			identityValue(t, domain.NewRegistrationNumber, number),
		)
		if err != nil {
			t.Fatalf("new tax number: %v", err)
		}
		command.TaxNumbers = []domain.TaxRegistrationNumber{tax}
	}
	if title != "" {
		details, err := domain.NewInvoicingDetails(identityValue(t, domain.NewInvoiceTitle, title))
		if err != nil {
			t.Fatalf("new invoicing details: %v", err)
		}
		command.Invoicing = &details
	}
	return command
}

func registerProfile(
	t *testing.T,
	handler *application.RegisterLegalEntityProfileHandler,
	command application.RegisterLegalEntityProfileCommand,
) application.LegalEntityProfileResult {
	t.Helper()
	result, err := handler.Register(context.Background(), command)
	if err != nil {
		t.Fatalf("register revision %d: %v", command.Revision, err)
	}
	return result
}

func TestRegisterLegalEntityProfileKeepsRevisionsContinuousAndImmutable(t *testing.T) {
	profiles := newFakeProfiles()
	handler := application.NewRegisterLegalEntityProfileHandler(
		profiles, &fakeLegalEntities{registration: profileLegalEntity(t, "XA"), found: true}, newFakeNumberTypes(t),
	)

	if got := registerProfile(t, handler, profileCommand(t, 2, "XA", "", "", "SYN 抬头")); got.Outcome() != application.LegalEntityProfileNotAccepted {
		t.Fatalf("first revision numbered 2: %s", got.Outcome())
	}
	first := profileCommand(t, 1, "XA", "SYN-XA-TAX", "SYN-XA-TAX-0001", "SYN 抬头")
	if got := registerProfile(t, handler, first); got.Outcome() != application.LegalEntityProfileRegistered {
		t.Fatalf("first revision: %s (%v)", got.Outcome(), got.Cause())
	}
	if got := registerProfile(t, handler, first); got.Outcome() != application.LegalEntityProfileAlreadyRegistered {
		t.Fatalf("replay: %s", got.Outcome())
	}
	if got := registerProfile(t, handler, profileCommand(t, 1, "XA", "", "", "SYN 另一抬头")); got.Outcome() != application.LegalEntityProfileContentConflict {
		t.Fatalf("same revision, other content: %s", got.Outcome())
	}
	if got := registerProfile(t, handler, profileCommand(t, 3, "XA", "", "", "SYN 抬头")); got.Outcome() != application.LegalEntityProfileNotAccepted ||
		!strings.Contains(got.Cause().Error(), "连续") {
		t.Fatalf("skipped revision: %s (%v)", got.Outcome(), got.Cause())
	}
	// 开票资料可以缺：登记时不拦，缺的后果在解析时才出现。
	if got := registerProfile(t, handler, profileCommand(t, 2, "XA", "", "", "")); got.Outcome() != application.LegalEntityProfileRegistered {
		t.Fatalf("second revision without invoicing details: %s (%v)", got.Outcome(), got.Cause())
	}
	if len(profiles.revisions) != 2 {
		t.Fatalf("register should hold two revisions, got %d", len(profiles.revisions))
	}
}

func TestRegisterLegalEntityProfileRefusesWhatTheProfileGatesReject(t *testing.T) {
	deactivated, err := profileLegalEntity(t, "XA").Deactivate(
		identityValue(t, domain.NewIdentityBasisReference, "SYN-DEACTIVATION"), profileEntityEffectiveFrom.AddDate(0, 6, 0),
	)
	if err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	for name, testCase := range map[string]struct {
		entities *fakeLegalEntities
		command  application.RegisterLegalEntityProfileCommand
		want     string
	}{
		"legal entity not registered": {
			&fakeLegalEntities{}, profileCommand(t, 1, "XA", "", "", "SYN 抬头"), "未登记",
		},
		"legal entity deactivated, even before its deactivation time": {
			&fakeLegalEntities{registration: deactivated, found: true}, profileCommand(t, 1, "XA", "", "", "SYN 抬头"), "已停用",
		},
		"identity layer not registered": {
			&fakeLegalEntities{registration: profileLegalEntity(t, ""), found: true}, profileCommand(t, 1, "XA", "", "", "SYN 抬头"), "身份层",
		},
		"address country differs from the registration country": {
			&fakeLegalEntities{registration: profileLegalEntity(t, "XA"), found: true}, profileCommand(t, 1, "XB", "", "", "SYN 抬头"), "不一致",
		},
		"identity-layer type used as a tax number": {
			&fakeLegalEntities{registration: profileLegalEntity(t, "XA"), found: true},
			profileCommand(t, 1, "XA", "SYN-XA-LIFETIME", "SYN-XA-000002", "SYN 抬头"), "资料层只收税务登记号",
		},
		"tax number type not registered": {
			&fakeLegalEntities{registration: profileLegalEntity(t, "XA"), found: true},
			profileCommand(t, 1, "XA", "SYN-XA-GST", "SYN-XA-GST-1", "SYN 抬头"), "未登记",
		},
		"tax number off its format": {
			&fakeLegalEntities{registration: profileLegalEntity(t, "XA"), found: true},
			profileCommand(t, 1, "XA", "SYN-XA-TAX", "SYN-XA-TAX-X", "SYN 抬头"), "格式",
		},
	} {
		t.Run(name, func(t *testing.T) {
			profiles := newFakeProfiles()
			handler := application.NewRegisterLegalEntityProfileHandler(profiles, testCase.entities, newFakeNumberTypes(t))
			got := registerProfile(t, handler, testCase.command)
			if got.Outcome() != application.LegalEntityProfileNotAccepted || !strings.Contains(got.Cause().Error(), testCase.want) {
				t.Fatalf("outcome %s (%v), want NOT_ACCEPTED naming %q", got.Outcome(), got.Cause(), testCase.want)
			}
			if len(profiles.revisions) != 0 {
				t.Fatal("a refused revision must not reach the register")
			}
		})
	}
}

func TestRegisterLegalEntityProfileReplaysUnderTodaysGates(t *testing.T) {
	profiles := newFakeProfiles()
	entities := &fakeLegalEntities{registration: profileLegalEntity(t, "XA"), found: true}
	handler := application.NewRegisterLegalEntityProfileHandler(profiles, entities, newFakeNumberTypes(t))
	first := profileCommand(t, 1, "XA", "", "", "SYN 抬头")
	if got := registerProfile(t, handler, first); got.Outcome() != application.LegalEntityProfileRegistered {
		t.Fatalf("first revision: %s (%v)", got.Outcome(), got.Cause())
	}

	deactivated, err := entities.registration.Deactivate(
		identityValue(t, domain.NewIdentityBasisReference, "SYN-DEACTIVATION"), profileEntityEffectiveFrom.AddDate(0, 1, 0),
	)
	if err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	entities.registration = deactivated
	if got := registerProfile(t, handler, first); got.Outcome() != application.LegalEntityProfileAlreadyRegistered {
		t.Fatalf("replay after the legal entity was deactivated: %s (%v)", got.Outcome(), got.Cause())
	}
	if got := registerProfile(t, handler, profileCommand(t, 2, "XA", "", "", "SYN 抬头")); got.Outcome() != application.LegalEntityProfileNotAccepted {
		t.Fatalf("new revision after deactivation: %s", got.Outcome())
	}
}

func TestRegisterLegalEntityProfileNeedsTheCatalogueOnlyForTaxNumbers(t *testing.T) {
	entities := &fakeLegalEntities{registration: profileLegalEntity(t, "XA"), found: true}
	handler := application.NewRegisterLegalEntityProfileHandler(newFakeProfiles(), entities, nil)
	if got := registerProfile(t, handler, profileCommand(t, 1, "XA", "", "", "SYN 抬头")); got.Outcome() != application.LegalEntityProfileRegistered {
		t.Fatalf("no tax numbers, no catalogue: %s (%v)", got.Outcome(), got.Cause())
	}
	_, err := handler.Register(context.Background(), profileCommand(t, 2, "XA", "SYN-XA-TAX", "SYN-XA-TAX-0001", "SYN 抬头"))
	if err == nil || !strings.Contains(err.Error(), "目录未装配") {
		t.Fatalf("tax numbers without a catalogue should be a technical error, got %v", err)
	}
}

func TestResolveLegalEntityProfileReadsTheEntityAndTheWholeChain(t *testing.T) {
	profiles := newFakeProfiles()
	entities := &fakeLegalEntities{}
	resolver := application.NewResolveLegalEntityProfileHandler(profiles, entities)
	tenant := identityValue(t, domain.NewTenantID, "tenant-1")
	entity := identityValue(t, domain.NewLegalEntityReference, "le-1")
	at := profileEntityEffectiveFrom.AddDate(0, 0, 1)

	resolution, err := resolver.Resolve(context.Background(), tenant, entity, at)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolution.Outcome() != domain.LegalEntityProfileEntityNotRegistered {
		t.Fatalf("unregistered legal entity: %s", resolution.Outcome())
	}

	entities.registration, entities.found = profileLegalEntity(t, "XA"), true
	registrar := application.NewRegisterLegalEntityProfileHandler(profiles, entities, newFakeNumberTypes(t))
	if got := registerProfile(t, registrar, profileCommand(t, 1, "XA", "", "", "SYN 抬头")); got.Outcome() != application.LegalEntityProfileRegistered {
		t.Fatalf("register: %s (%v)", got.Outcome(), got.Cause())
	}
	resolution, err = resolver.Resolve(context.Background(), tenant, entity, at)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if revision, ok := resolution.Resolved(); !ok || revision.Reference().Revision() != 1 {
		t.Fatalf("resolution: %s, want revision 1 resolved", resolution.Outcome())
	}

	profiles.chainErr = errors.New("synthetic read failure")
	if _, err := resolver.Resolve(context.Background(), tenant, entity, at); err == nil {
		t.Fatal("a failed chain read must surface as an error, not as an incomplete profile")
	}
}
