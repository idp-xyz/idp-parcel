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
// 空册答空页。状态维的口径与客户账户册同一条 CASE，词表每一格都有实例。夹具里的租户、标识、名称、依据与范围都是 SYN- 合成值，
// 国家 / 地区取 ISO 3166 用户自定义码。

const identityPageTenant = "SYN-TENANT-01"

// synPartyFixture 造一笔业务参与方登记；生效时点由用例给，`已登记`那一格要一个未来的时点。
func synPartyFixture(t *testing.T, id, name string, effectiveFrom time.Time) domain.BusinessPartyRegistration {
	t.Helper()
	party, err := domain.NewBusinessParty(
		pcTenant(t, identityPageTenant), pcValue(t, domain.NewPartyID, id), pcValue(t, domain.NewPartyName, name),
	)
	if err != nil {
		t.Fatalf("new business party: %v", err)
	}
	lifecycle, err := domain.NewIdentityLifecycle(effectiveFrom)
	if err != nil {
		t.Fatalf("new identity lifecycle: %v", err)
	}
	registration, err := domain.NewBusinessPartyRegistration(
		party, 1, pcValue(t, domain.NewIdentityBasisReference, "SYN-BASIS-"+id), lifecycle,
	)
	if err != nil {
		t.Fatalf("new party registration: %v", err)
	}
	return registration
}

func synLegalEntityFixture(t *testing.T, entityID, partyID string) domain.LegalEntityRegistration {
	t.Helper()
	entity, err := domain.RehydrateResponsibleLegalEntity(
		pcTenant(t, identityPageTenant), pcValue(t, domain.NewLegalEntityReference, entityID), pcValue(t, domain.NewPartyID, partyID),
	)
	if err != nil {
		t.Fatalf("rehydrate legal entity: %v", err)
	}
	lifecycle, err := domain.NewIdentityLifecycle(identityEffectiveFrom)
	if err != nil {
		t.Fatalf("new identity lifecycle: %v", err)
	}
	registration, err := domain.NewLegalEntityRegistration(
		entity, 1, pcValue(t, domain.NewIdentityBasisReference, "SYN-BASIS-"+entityID), lifecycle,
	)
	if err != nil {
		t.Fatalf("new legal entity registration: %v", err)
	}
	return registration
}

func rolledRelationshipFixture(
	t *testing.T,
	id, holder, counterparty string,
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
		Scope:        pcValue(t, domain.NewCommercialScopeReference, "SYN-SCOPE-1"),
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
		pcTenant(t, identityPageTenant), pcValue(t, domain.NewRelationshipID, id), 1, relationship,
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
	tenant := pcTenant(t, identityPageTenant)
	empty := pcTenant(t, "SYN-TENANT-EMPTY")

	parties := func(tenant domain.TenantID) pageLister[ports.BusinessPartyRow] {
		return func(query cataloguepage.Query, limit int) (cataloguepage.Result[ports.BusinessPartyRow], error) {
			return catalogue.ListBusinessParties(ctx, tenant, limit, query)
		}
	}
	assertPagesWithoutRepeatsOrGaps(t, ports.BusinessPartyPages, parties(tenant), func(id string) {
		party := synPartyFixture(t, id, "SYN 参与方 "+id, identityEffectiveFrom)
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
		entity := synLegalEntityFixture(t, id, "SYN-PARTY-1")
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
		relationship := rolledRelationshipFixture(t, id, "SYN-PARTY-1", "SYN-PARTY-2", domain.CustomerRole, true)
		mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
			return registrations.SaveRelationship(txCtx, relationship)
		})
	}, func(row ports.PartyRelationshipRow) string { return row.RelationshipID }, "SYN-REL-")
	assertEmptyCatalogue(t, "参与方关系", ports.PartyRelationshipPages, relations(empty))
}

// Covers: 完成判据「筛选维内取并、维间取交，total 随筛选与 q 变；q 里的 % 按字面」——业务参与方册。状态词表每一格都有实例；
// q 命中名称、识别号与外部编码，不命中快照里的键名与 JSON 的标点；一个外部编码的系统与编码连写命中，两个外部编码首尾
// 相接不命中。
func TestBusinessPartyCatalogueFiltersAndSearches(t *testing.T) {
	registrations, catalogue, transactor := newPartyIdentityRegistrations(t)
	ctx := t.Context()
	save := func(registration domain.BusinessPartyRegistration) {
		mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
			return registrations.SaveBusinessParty(txCtx, registration)
		})
	}

	layer := identityLayerFixture(t, "XA", "SYN-USCC", "SYN-91310000-A")
	identified, err := synPartyFixture(t, "SYN-PARTY-A", "SYN 合成单位 A", identityEffectiveFrom).WithIdentification(&layer, nil,
		[]domain.ExternalReference{
			referenceFixture(t, "SYN-ERP", "CUSTOMER", "SYN-ERP-777"),
			referenceFixture(t, "SYN-CRM", "SUPPLIER", "SYN-CRM-888"),
		})
	if err != nil {
		t.Fatalf("with identification: %v", err)
	}
	save(identified)
	save(synPartyFixture(t, "SYN-PARTY-B", "SYN 华南货主", time.Now().UTC().Add(72*time.Hour)))
	retiring := synPartyFixture(t, "SYN-PARTY-C", "SYN 停用单位", identityEffectiveFrom)
	save(retiring)
	retired, err := retiring.Deactivate(pcValue(t, domain.NewIdentityBasisReference, "SYN-BASIS-DEACT"), identityDeactivateAt)
	if err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	save(retired)
	save(synPartyFixture(t, "SYN-PARTY-D", "SYN 折扣 5%单位", identityEffectiveFrom))

	tenant := pcTenant(t, identityPageTenant)
	assertFilteredPages(t, "业务参与方", ports.BusinessPartyPages,
		func(query cataloguepage.Query, limit int) (cataloguepage.Result[ports.BusinessPartyRow], error) {
			return catalogue.ListBusinessParties(ctx, tenant, limit, query)
		},
		func(row ports.BusinessPartyRow) string { return row.PartyID },
		[]pageCase{
			{"", "SYN-PARTY-D,SYN-PARTY-C,SYN-PARTY-B,SYN-PARTY-A"},
			{"sort=partyId", "SYN-PARTY-A,SYN-PARTY-B,SYN-PARTY-C,SYN-PARTY-D"},
			{"sort=-partyId", "SYN-PARTY-D,SYN-PARTY-C,SYN-PARTY-B,SYN-PARTY-A"},
			{"sort=partyId&status=EFFECTIVE", "SYN-PARTY-A,SYN-PARTY-D"},
			{"sort=partyId&status=EFFECTIVE&status=REGISTERED", "SYN-PARTY-A,SYN-PARTY-B,SYN-PARTY-D"},
			{"sort=partyId&status=DEACTIVATED", "SYN-PARTY-C"},
			{"sort=partyId&q=华南", "SYN-PARTY-B"},
			{"sort=partyId&q=syn-91310000", "SYN-PARTY-A"},
			{"sort=partyId&q=SYN-ERP-777", "SYN-PARTY-A"},
			{"sort=partyId&q=SYN-CRM%20SYN-CRM-888", "SYN-PARTY-A"},
			{"sort=partyId&q=SYN-ERP-777%20SYN-CRM", ""},
			{"sort=partyId&q=单位", "SYN-PARTY-A,SYN-PARTY-C,SYN-PARTY-D"},
			{"sort=partyId&status=EFFECTIVE&q=单位", "SYN-PARTY-A,SYN-PARTY-D"},
			{"sort=partyId&q=%25", "SYN-PARTY-D"},
			{"sort=partyId&q=_", ""},
			{"sort=partyId&q=number", ""},
			{"sort=partyId&q=%22", ""},
			{"sort=partyId&q=%2C", ""},
		})
}

// Covers: 完成判据「筛选维内取并、维间取交，total 随筛选与 q 变；q 里的 % 按字面」——集团法人册。注册国家是开放取值的
// 筛选维；q 命中法人标识、所属参与方的标识与名称、注册国家与登记号。
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

	owner := synPartyFixture(t, "SYN-PARTY-A", "SYN 华东运营法人", identityEffectiveFrom)
	mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
		return registrations.SaveBusinessParty(txCtx, owner)
	})
	save(withLayer(synLegalEntityFixture(t, "SYN-LE-1", "SYN-PARTY-A"), "XA", "SYN-LE-NO-1"))
	retiring := withLayer(synLegalEntityFixture(t, "SYN-LE-2", "SYN-PARTY-X"), "XB", "SYN-LE-NO-2")
	save(retiring)
	retired, err := retiring.Deactivate(pcValue(t, domain.NewIdentityBasisReference, "SYN-BASIS-DEACT"), identityDeactivateAt)
	if err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	save(retired)
	save(synLegalEntityFixture(t, "SYN-LE-3", "SYN-PARTY-A"))

	tenant := pcTenant(t, identityPageTenant)
	assertFilteredPages(t, "集团法人", ports.GroupLegalEntityPages,
		func(query cataloguepage.Query, limit int) (cataloguepage.Result[ports.GroupLegalEntityRow], error) {
			return catalogue.ListGroupLegalEntities(ctx, tenant, limit, query)
		},
		func(row ports.GroupLegalEntityRow) string { return row.LegalEntityID },
		[]pageCase{
			{"", "SYN-LE-3,SYN-LE-2,SYN-LE-1"},
			{"sort=legalEntityId", "SYN-LE-1,SYN-LE-2,SYN-LE-3"},
			{"sort=legalEntityId&registrationCountry=XA", "SYN-LE-1"},
			{"sort=legalEntityId&registrationCountry=XA&registrationCountry=XB", "SYN-LE-1,SYN-LE-2"},
			{"sort=legalEntityId&status=EFFECTIVE", "SYN-LE-1,SYN-LE-3"},
			{"sort=legalEntityId&status=DEACTIVATED", "SYN-LE-2"},
			{"sort=legalEntityId&status=EFFECTIVE&registrationCountry=XB", ""},
			{"sort=legalEntityId&q=华东", "SYN-LE-1,SYN-LE-3"},
			{"sort=legalEntityId&q=syn-le-no-2", "SYN-LE-2"},
			{"sort=legalEntityId&q=SYN-PARTY-X", "SYN-LE-2"},
			{"sort=legalEntityId&q=%25", ""},
			{"sort=legalEntityId&q=%22", ""},
		})
}

// Covers: 完成判据「筛选维内取并、维间取交，total 随筛选与 q 变；q 里的 % 按字面」——参与方关系册。筛选维是状态与角色；
// q 命中关系标识、双方标识与双方名称（缺名的一方不挡另一方命中）。
func TestPartyRelationshipCatalogueFiltersAndSearches(t *testing.T) {
	registrations, catalogue, transactor := newPartyIdentityRegistrations(t)
	ctx := t.Context()
	for _, party := range []domain.BusinessPartyRegistration{
		synPartyFixture(t, "SYN-PARTY-A", "SYN 华东货主", identityEffectiveFrom),
		synPartyFixture(t, "SYN-PARTY-B", "SYN 华南货主", identityEffectiveFrom),
		synPartyFixture(t, "SYN-PARTY-C", "SYN 承运代理", identityEffectiveFrom),
	} {
		mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
			return registrations.SaveBusinessParty(txCtx, party)
		})
	}
	for _, relationship := range []domain.PartyRelationshipRegistration{
		rolledRelationshipFixture(t, "SYN-REL-1", "SYN-PARTY-A", "SYN-PARTY-X", domain.CustomerRole, true),
		rolledRelationshipFixture(t, "SYN-REL-2", "SYN-PARTY-B", "SYN-PARTY-A", domain.CustomerRole, false),
		rolledRelationshipFixture(t, "SYN-REL-3", "SYN-PARTY-C", "SYN-PARTY-A", domain.SupplierRole, true),
	} {
		mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
			return registrations.SaveRelationship(txCtx, relationship)
		})
	}

	tenant := pcTenant(t, identityPageTenant)
	assertFilteredPages(t, "参与方关系", ports.PartyRelationshipPages,
		func(query cataloguepage.Query, limit int) (cataloguepage.Result[ports.PartyRelationshipRow], error) {
			return catalogue.ListPartyRelationships(ctx, tenant, limit, query)
		},
		func(row ports.PartyRelationshipRow) string { return row.RelationshipID },
		[]pageCase{
			{"", "SYN-REL-3,SYN-REL-2,SYN-REL-1"},
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
