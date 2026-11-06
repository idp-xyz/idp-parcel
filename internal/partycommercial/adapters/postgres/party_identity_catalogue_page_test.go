package postgres_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// 本文件对真实 PostgreSQL 证身份三册（业务参与方、集团法人、参与方关系）按 ADR-0144 分页（票
// catalogue-read-pagination-rollout/11）：翻页中途登记新行不重不漏、筛选与 q 收窄的总数与本页同一组条件、q 里的 % 按字面、
// 空册答空页。状态维的口径与客户账户册同一条 CASE，三格各有实例。

func rolledRelationshipFixture(
	t *testing.T,
	tenant, id, holder, counterparty string,
	role domain.PartyRole,
	approved bool,
) domain.PartyRelationshipRegistration {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(relationshipStartsAt, time.Time{})
	if err != nil {
		t.Fatalf("new interval: %v", err)
	}
	relationship, err := domain.NewCandidateRelationship(domain.PartyRelationshipSpec{
		Holder:       pcValue(t, domain.NewPartyID, holder),
		Counterparty: pcValue(t, domain.NewPartyID, counterparty),
		Role:         role,
		Scope:        pcScope(t),
		Basis:        pcValue(t, domain.NewRelationshipBasisReference, "SYN-CONTRACT-"+id),
		Effective:    interval,
	})
	if err != nil {
		t.Fatalf("new candidate relationship: %v", err)
	}
	if approved {
		relationship, err = relationship.Approve(pcValue(t, domain.NewApprovalReference, "SYN-APPROVAL-"+id), relationshipStartsAt)
		if err != nil {
			t.Fatalf("approve relationship: %v", err)
		}
	}
	registration, err := domain.NewPartyRelationshipRegistration(
		pcTenant(t, tenant), pcValue(t, domain.NewRelationshipID, id), 1, relationship,
	)
	if err != nil {
		t.Fatalf("new relationship registration: %v", err)
	}
	return registration
}

// Covers: 完成判据「翻页期间插入新行不重不漏」与「空册答空页」，身份三册各跑一遍。
func TestPartyIdentityCataloguesPageWithoutRepeatsOrGapsWhileRowsArrive(t *testing.T) {
	registrations, catalogue, transactor := newPartyIdentityRegistrations(t)
	ctx := t.Context()
	tenant := pcTenant(t, "tenant-1")
	empty := pcTenant(t, "SYN-TENANT-EMPTY")

	parties := func(tenant domain.TenantID) pageLister[ports.BusinessPartyRow] {
		return func(query cataloguepage.Query, limit int) (cataloguepage.Result[ports.BusinessPartyRow], error) {
			return catalogue.ListBusinessParties(ctx, tenant, limit, query)
		}
	}
	assertPagesWithoutRepeatsOrGaps(t, ports.BusinessPartyPages, parties(tenant), func(id string) {
		party := partyRegistrationFixture(t, "tenant-1", id, "SYN 参与方 "+id)
		mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
			return registrations.SaveBusinessParty(txCtx, party)
		})
	}, func(row ports.BusinessPartyRow) string { return row.PartyID }, "SYN-PARTY-")
	assertEmptyCatalogue(t, "业务参与方", ports.BusinessPartyPages, parties(empty))

	entities := func(tenant domain.TenantID) pageLister[ports.GroupLegalEntityRow] {
		return func(query cataloguepage.Query, limit int) (cataloguepage.Result[ports.GroupLegalEntityRow], error) {
			return catalogue.ListGroupLegalEntities(ctx, tenant, limit, query)
		}
	}
	assertPagesWithoutRepeatsOrGaps(t, ports.GroupLegalEntityPages, entities(tenant), func(id string) {
		entity := legalEntityRegistrationFixture(t, "tenant-1", id, "SYN-PARTY-1")
		mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
			return registrations.SaveLegalEntity(txCtx, entity)
		})
	}, func(row ports.GroupLegalEntityRow) string { return row.LegalEntityID }, "SYN-LE-")
	assertEmptyCatalogue(t, "集团法人", ports.GroupLegalEntityPages, entities(empty))

	relations := func(tenant domain.TenantID) pageLister[ports.PartyRelationshipRow] {
		return func(query cataloguepage.Query, limit int) (cataloguepage.Result[ports.PartyRelationshipRow], error) {
			return catalogue.ListPartyRelationships(ctx, tenant, limit, query)
		}
	}
	assertPagesWithoutRepeatsOrGaps(t, ports.PartyRelationshipPages, relations(tenant), func(id string) {
		relationship := relationshipRegistrationFixture(t, "tenant-1", id, "SYN-PARTY-1", "SYN-PARTY-2", true)
		mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
			return registrations.SaveRelationship(txCtx, relationship)
		})
	}, func(row ports.PartyRelationshipRow) string { return row.RelationshipID }, "SYN-REL-")
	assertEmptyCatalogue(t, "参与方关系", ports.PartyRelationshipPages, relations(empty))
}

// Covers: 完成判据「筛选维内取并、维间取交，total 随筛选与 q 变；q 里的 % 按字面」——业务参与方册。状态三格各有实例；
// q 命中名称、识别号与外部编码，不命中快照里的键名。
func TestBusinessPartyCatalogueFiltersAndSearches(t *testing.T) {
	registrations, catalogue, transactor := newPartyIdentityRegistrations(t)
	ctx := t.Context()
	save := func(registration domain.BusinessPartyRegistration) {
		mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
			return registrations.SaveBusinessParty(txCtx, registration)
		})
	}

	layer := identityLayerFixture(t, "CN", "SYN-USCC", "SYN-91310000-A")
	save(identifiedPartyFixture(t, "tenant-1", "SYN-PARTY-A", &layer, referenceFixture(t, "SYN-ERP", "CUSTOMER", "SYN-ERP-777")))
	save(futurePartyRegistrationFixture(t, "tenant-1", "SYN-PARTY-B", "SYN 华南货主"))
	retiring := partyRegistrationFixture(t, "tenant-1", "SYN-PARTY-C", "SYN 停用单位")
	save(retiring)
	retired, err := retiring.Deactivate(pcValue(t, domain.NewIdentityBasisReference, "SYN-BASIS-DEACT"), identityDeactivateAt)
	if err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	save(retired)
	save(partyRegistrationFixture(t, "tenant-1", "SYN-PARTY-D", "SYN 折扣 5%单位"))

	tenant := pcTenant(t, "tenant-1")
	assertFilteredPages(t, "业务参与方", ports.BusinessPartyPages,
		func(query cataloguepage.Query, limit int) (cataloguepage.Result[ports.BusinessPartyRow], error) {
			return catalogue.ListBusinessParties(ctx, tenant, limit, query)
		},
		func(row ports.BusinessPartyRow) string { return row.PartyID },
		[]pageCase{
			{"sort=partyId", "SYN-PARTY-A,SYN-PARTY-B,SYN-PARTY-C,SYN-PARTY-D"},
			{"sort=-partyId", "SYN-PARTY-D,SYN-PARTY-C,SYN-PARTY-B,SYN-PARTY-A"},
			{"sort=partyId&status=EFFECTIVE", "SYN-PARTY-A,SYN-PARTY-D"},
			{"sort=partyId&status=EFFECTIVE&status=REGISTERED", "SYN-PARTY-A,SYN-PARTY-B,SYN-PARTY-D"},
			{"sort=partyId&status=DEACTIVATED", "SYN-PARTY-C"},
			{"sort=partyId&q=华南", "SYN-PARTY-B"},
			{"sort=partyId&q=syn-91310000", "SYN-PARTY-A"},
			{"sort=partyId&q=SYN-ERP-777", "SYN-PARTY-A"},
			{"sort=partyId&q=单位", "SYN-PARTY-A,SYN-PARTY-C,SYN-PARTY-D"},
			{"sort=partyId&status=EFFECTIVE&q=单位", "SYN-PARTY-A,SYN-PARTY-D"},
			{"sort=partyId&q=%25", "SYN-PARTY-D"},
			{"sort=partyId&q=_", ""},
			{"sort=partyId&q=number", ""},
		})
}

// Covers: 同上，集团法人册。注册国家是开放取值的筛选维；q 命中法人标识、所属参与方的标识与名称、注册国家与登记号。
func TestGroupLegalEntityCatalogueFiltersAndSearches(t *testing.T) {
	registrations, catalogue, transactor := newPartyIdentityRegistrations(t)
	ctx := t.Context()
	save := func(registration domain.LegalEntityRegistration) {
		mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
			return registrations.SaveLegalEntity(txCtx, registration)
		})
	}
	withLayer := func(registration domain.LegalEntityRegistration, country, number string) domain.LegalEntityRegistration {
		layered, err := registration.WithIdentityLayer(identityLayerFixture(t, country, "SYN-USCC", number), nil)
		if err != nil {
			t.Fatalf("with identity layer: %v", err)
		}
		return layered
	}

	owner := partyRegistrationFixture(t, "tenant-1", "SYN-PARTY-A", "SYN 华东运营法人")
	mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
		return registrations.SaveBusinessParty(txCtx, owner)
	})
	save(withLayer(legalEntityRegistrationFixture(t, "tenant-1", "SYN-LE-1", "SYN-PARTY-A"), "CN", "SYN-LE-NO-1"))
	retiring := withLayer(legalEntityRegistrationFixture(t, "tenant-1", "SYN-LE-2", "SYN-PARTY-X"), "SG", "SYN-LE-NO-2")
	save(retiring)
	retired, err := retiring.Deactivate(pcValue(t, domain.NewIdentityBasisReference, "SYN-BASIS-DEACT"), identityDeactivateAt)
	if err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	save(retired)
	save(legalEntityRegistrationFixture(t, "tenant-1", "SYN-LE-3", "SYN-PARTY-A"))

	tenant := pcTenant(t, "tenant-1")
	assertFilteredPages(t, "集团法人", ports.GroupLegalEntityPages,
		func(query cataloguepage.Query, limit int) (cataloguepage.Result[ports.GroupLegalEntityRow], error) {
			return catalogue.ListGroupLegalEntities(ctx, tenant, limit, query)
		},
		func(row ports.GroupLegalEntityRow) string { return row.LegalEntityID },
		[]pageCase{
			{"sort=legalEntityId", "SYN-LE-1,SYN-LE-2,SYN-LE-3"},
			{"sort=legalEntityId&registrationCountry=CN", "SYN-LE-1"},
			{"sort=legalEntityId&registrationCountry=CN&registrationCountry=SG", "SYN-LE-1,SYN-LE-2"},
			{"sort=legalEntityId&status=EFFECTIVE", "SYN-LE-1,SYN-LE-3"},
			{"sort=legalEntityId&status=DEACTIVATED", "SYN-LE-2"},
			{"sort=legalEntityId&status=EFFECTIVE&registrationCountry=SG", ""},
			{"sort=legalEntityId&q=华东", "SYN-LE-1,SYN-LE-3"},
			{"sort=legalEntityId&q=syn-le-no-2", "SYN-LE-2"},
			{"sort=legalEntityId&q=SYN-PARTY-X", "SYN-LE-2"},
			{"sort=legalEntityId&q=%25", ""},
		})
}

// Covers: 同上，参与方关系册。状态与角色两维；q 命中关系标识、双方标识与双方名称（缺名的一方不挡另一方命中）。
func TestPartyRelationshipCatalogueFiltersAndSearches(t *testing.T) {
	registrations, catalogue, transactor := newPartyIdentityRegistrations(t)
	ctx := t.Context()
	for _, party := range []domain.BusinessPartyRegistration{
		partyRegistrationFixture(t, "tenant-1", "SYN-PARTY-A", "SYN 华东货主"),
		partyRegistrationFixture(t, "tenant-1", "SYN-PARTY-B", "SYN 华南货主"),
		partyRegistrationFixture(t, "tenant-1", "SYN-PARTY-C", "SYN 承运代理"),
	} {
		mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
			return registrations.SaveBusinessParty(txCtx, party)
		})
	}
	for _, relationship := range []domain.PartyRelationshipRegistration{
		rolledRelationshipFixture(t, "tenant-1", "SYN-REL-1", "SYN-PARTY-A", "SYN-PARTY-X", domain.CustomerRole, true),
		rolledRelationshipFixture(t, "tenant-1", "SYN-REL-2", "SYN-PARTY-B", "SYN-PARTY-A", domain.CustomerRole, false),
		rolledRelationshipFixture(t, "tenant-1", "SYN-REL-3", "SYN-PARTY-C", "SYN-PARTY-A", domain.SupplierRole, true),
	} {
		mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
			return registrations.SaveRelationship(txCtx, relationship)
		})
	}

	tenant := pcTenant(t, "tenant-1")
	assertFilteredPages(t, "参与方关系", ports.PartyRelationshipPages,
		func(query cataloguepage.Query, limit int) (cataloguepage.Result[ports.PartyRelationshipRow], error) {
			return catalogue.ListPartyRelationships(ctx, tenant, limit, query)
		},
		func(row ports.PartyRelationshipRow) string { return row.RelationshipID },
		[]pageCase{
			{"sort=relationshipId", "SYN-REL-1,SYN-REL-2,SYN-REL-3"},
			{"sort=relationshipId&status=EFFECTIVE", "SYN-REL-1,SYN-REL-3"},
			{"sort=relationshipId&status=CANDIDATE", "SYN-REL-2"},
			{"sort=relationshipId&role=SUPPLIER", "SYN-REL-3"},
			{"sort=relationshipId&role=CUSTOMER&status=EFFECTIVE", "SYN-REL-1"},
			{"sort=relationshipId&role=CUSTOMER&role=SUPPLIER&status=EFFECTIVE", "SYN-REL-1,SYN-REL-3"},
			{"sort=relationshipId&q=华南", "SYN-REL-2"},
			{"sort=relationshipId&q=代理", "SYN-REL-3"},
			{"sort=relationshipId&q=syn-party-x", "SYN-REL-1"},
			{"sort=relationshipId&role=SUPPLIER&q=华东", "SYN-REL-3"},
			{"sort=relationshipId&q=%25", ""},
		})
}
