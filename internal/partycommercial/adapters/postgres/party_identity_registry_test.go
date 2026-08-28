package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证参与方身份与关系登记册（0015）：往返重建、撞键
// 重放/冲突、停用修订落册后目录如实显示状态、双方名称左连接、跨租户读不到、无事务拒。

var (
	identityEffectiveFrom = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	identityDeactivateAt  = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	relationshipStartsAt  = time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
)

func newPartyIdentityRegistrations(t *testing.T) (*adapter.PartyIdentityRegistrations, *adapter.OperationsCatalogue, bentoapp.Transactor) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registrations, err := adapter.NewPartyIdentityRegistrations(db)
	if err != nil {
		t.Fatalf("构造身份登记册：%v", err)
	}
	catalogue, err := adapter.NewOperationsCatalogue(db)
	if err != nil {
		t.Fatalf("构造目录读口：%v", err)
	}
	return registrations, catalogue, db.Transactor()
}

func partyRegistrationFixture(t *testing.T, tenant, id, name string) domain.BusinessPartyRegistration {
	t.Helper()
	party, err := domain.NewBusinessParty(
		pcTenant(t, tenant),
		pcValue(t, domain.NewPartyID, id),
		pcValue(t, domain.NewPartyName, name),
	)
	if err != nil {
		t.Fatalf("new business party: %v", err)
	}
	lifecycle, err := domain.NewIdentityLifecycle(identityEffectiveFrom)
	if err != nil {
		t.Fatalf("new identity lifecycle: %v", err)
	}
	registration, err := domain.NewBusinessPartyRegistration(
		party, 1, pcValue(t, domain.NewIdentityBasisReference, "basis-"+id), lifecycle,
	)
	if err != nil {
		t.Fatalf("new party registration: %v", err)
	}
	return registration
}

func legalEntityRegistrationFixture(t *testing.T, tenant, entityID, partyID string) domain.LegalEntityRegistration {
	t.Helper()
	entity, err := domain.RehydrateResponsibleLegalEntity(
		pcTenant(t, tenant),
		pcValue(t, domain.NewLegalEntityReference, entityID),
		pcValue(t, domain.NewPartyID, partyID),
	)
	if err != nil {
		t.Fatalf("rehydrate legal entity: %v", err)
	}
	lifecycle, err := domain.NewIdentityLifecycle(identityEffectiveFrom)
	if err != nil {
		t.Fatalf("new identity lifecycle: %v", err)
	}
	registration, err := domain.NewLegalEntityRegistration(
		entity, 1, pcValue(t, domain.NewIdentityBasisReference, "basis-"+entityID), lifecycle,
	)
	if err != nil {
		t.Fatalf("new legal entity registration: %v", err)
	}
	return registration
}

func relationshipRegistrationFixture(
	t *testing.T,
	tenant, id, holder, counterparty string,
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
		Role:         domain.CustomerRole,
		Scope:        pcScope(t),
		Basis:        pcValue(t, domain.NewRelationshipBasisReference, "contract-"+id),
		Effective:    interval,
	})
	if err != nil {
		t.Fatalf("new candidate relationship: %v", err)
	}
	if approved {
		relationship, err = relationship.Approve(
			pcValue(t, domain.NewApprovalReference, "approval-"+id), relationshipStartsAt,
		)
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

func mustSavePartyIdentity(
	t *testing.T,
	transactor bentoapp.Transactor,
	save func(context.Context) (ports.PartyRegistrySaveOutcome, error),
) {
	t.Helper()
	mustWithinPublicationTransaction(t, transactor, t.Context(), func(txCtx context.Context) error {
		outcome, err := save(txCtx)
		if err != nil {
			return err
		}
		if outcome != ports.PartyRegistrySaved {
			return fmt.Errorf("save outcome = %s", outcome)
		}
		return nil
	})
}

// Covers: 票 01 完成判据「登记 CLI 灌 SYN- 种子后两页非空册；停用后目录如实显示状态」
// 的库上半边——身份与关系落册、目录上列名称与状态、停用修订让状态翻成 DEACTIVATED。
func TestPartyIdentityRegistryRoundTripAndCatalogue(t *testing.T) {
	registrations, catalogue, transactor := newPartyIdentityRegistrations(t)
	ctx := t.Context()

	partyLE := partyRegistrationFixture(t, "tenant-1", "party-le", "运营法人参与方")
	partyCust := partyRegistrationFixture(t, "tenant-1", "party-cust", "货主客户参与方")
	mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
		return registrations.SaveBusinessParty(txCtx, partyLE)
	})
	mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
		return registrations.SaveBusinessParty(txCtx, partyCust)
	})

	entity := legalEntityRegistrationFixture(t, "tenant-1", "le-1", "party-le")
	mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
		return registrations.SaveLegalEntity(txCtx, entity)
	})

	account, err := domain.RehydrateCustomerAccount(
		pcTenant(t, "tenant-1"),
		pcValue(t, domain.NewCustomerAccountID, "account-1"),
		pcValue(t, domain.NewPartyID, "party-cust"),
	)
	if err != nil {
		t.Fatalf("rehydrate account: %v", err)
	}
	accountLifecycle, err := domain.NewIdentityLifecycle(identityEffectiveFrom)
	if err != nil {
		t.Fatalf("account lifecycle: %v", err)
	}
	accountRegistration, err := domain.NewCustomerAccountRegistration(
		account, 1, pcValue(t, domain.NewIdentityBasisReference, "basis-account-1"), accountLifecycle,
	)
	if err != nil {
		t.Fatalf("account registration: %v", err)
	}
	mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
		return registrations.SaveCustomerAccount(txCtx, accountRegistration)
	})

	relationship := relationshipRegistrationFixture(t, "tenant-1", "rel-1", "party-cust", "party-le", true)
	mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
		return registrations.SaveRelationship(txCtx, relationship)
	})

	// 往返重建：最新修订原样回来（含批准事实与生命周期）。
	loadedParty, found, err := registrations.LoadLatestBusinessParty(
		ctx, pcTenant(t, "tenant-1"), pcValue(t, domain.NewPartyID, "party-le"))
	if err != nil || !found {
		t.Fatalf("load party = (%v, %v)", found, err)
	}
	if loadedParty.Party().Name().String() != "运营法人参与方" || loadedParty.Revision() != 1 {
		t.Fatalf("loaded party = %+v", loadedParty)
	}
	loadedRelationship, found, err := registrations.LoadLatestRelationship(
		ctx, pcTenant(t, "tenant-1"), pcValue(t, domain.NewRelationshipID, "rel-1"))
	if err != nil || !found {
		t.Fatalf("load relationship = (%v, %v)", found, err)
	}
	if loadedRelationship.Relationship().Status() != domain.RelationshipEffective {
		t.Fatalf("loaded relationship status = %v", loadedRelationship.Relationship().Status())
	}
	if _, _, has := loadedRelationship.Relationship().Approval(); !has {
		t.Fatal("loaded relationship lost its approval facts")
	}

	// 目录：法人行带参与方名称、状态 EFFECTIVE；关系行双方名称齐、状态 EFFECTIVE。
	entities, err := catalogue.ListGroupLegalEntities(ctx, pcTenant(t, "tenant-1"), 10)
	if err != nil {
		t.Fatalf("list entities: %v", err)
	}
	if len(entities) != 1 {
		t.Fatalf("entities = %d, want 1", len(entities))
	}
	if entities[0].Status != "EFFECTIVE" || !entities[0].HasPartyName ||
		entities[0].PartyName != "运营法人参与方" {
		t.Fatalf("entity row = %+v", entities[0])
	}

	relations, err := catalogue.ListPartyRelationships(ctx, pcTenant(t, "tenant-1"), 10)
	if err != nil {
		t.Fatalf("list relationships: %v", err)
	}
	if len(relations) != 1 {
		t.Fatalf("relationships = %d, want 1", len(relations))
	}
	relationRow := relations[0]
	if relationRow.Status != "EFFECTIVE" || relationRow.Role != "CUSTOMER" ||
		!relationRow.HasHolderName || relationRow.HolderName != "货主客户参与方" ||
		!relationRow.HasCounterpartyName || relationRow.CounterpartyName != "运营法人参与方" {
		t.Fatalf("relationship row = %+v", relationRow)
	}

	// 停用修订落册后：LoadLatest 回停用修订，目录状态如实翻成 DEACTIVATED。
	loadedEntity, found, err := registrations.LoadLatestLegalEntity(
		ctx, pcTenant(t, "tenant-1"), pcValue(t, domain.NewLegalEntityReference, "le-1"))
	if err != nil || !found {
		t.Fatalf("load entity = (%v, %v)", found, err)
	}
	deactivatedEntity, err := loadedEntity.Deactivate(
		pcValue(t, domain.NewIdentityBasisReference, "basis-deact"), identityDeactivateAt)
	if err != nil {
		t.Fatalf("deactivate entity: %v", err)
	}
	mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
		return registrations.SaveLegalEntity(txCtx, deactivatedEntity)
	})

	latestEntity, _, err := registrations.LoadLatestLegalEntity(
		ctx, pcTenant(t, "tenant-1"), pcValue(t, domain.NewLegalEntityReference, "le-1"))
	if err != nil {
		t.Fatalf("reload entity: %v", err)
	}
	if latestEntity.Revision() != 2 {
		t.Fatalf("latest entity revision = %d, want 2", latestEntity.Revision())
	}
	entitiesAfter, err := catalogue.ListGroupLegalEntities(ctx, pcTenant(t, "tenant-1"), 10)
	if err != nil {
		t.Fatalf("list entities after deactivation: %v", err)
	}
	if len(entitiesAfter) != 1 || entitiesAfter[0].Status != "DEACTIVATED" ||
		!entitiesAfter[0].HasDeactivation ||
		entitiesAfter[0].DeactivationBasis != "basis-deact" {
		t.Fatalf("entity row after deactivation = %+v", entitiesAfter[0])
	}

	// 跨租户读不到：目录与点读都与从未登记同形。
	foreignEntities, err := catalogue.ListGroupLegalEntities(ctx, pcTenant(t, "tenant-b"), 10)
	if err != nil || len(foreignEntities) != 0 {
		t.Fatalf("foreign tenant entities = (%d, %v)", len(foreignEntities), err)
	}
	_, found, err = registrations.LoadLatestBusinessParty(
		ctx, pcTenant(t, "tenant-b"), pcValue(t, domain.NewPartyID, "party-le"))
	if err != nil || found {
		t.Fatalf("foreign tenant load = (%v, %v)", found, err)
	}
}

// Covers: ADR-0031 登记面代数在四册上的镜像——同键同内容重放，同键异内容冲突。
func TestPartyIdentitySavesReplayAndConflict(t *testing.T) {
	registrations, _, transactor := newPartyIdentityRegistrations(t)

	registration := partyRegistrationFixture(t, "tenant-1", "party-1", "参与方一号")
	mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
		return registrations.SaveBusinessParty(txCtx, registration)
	})

	var outcome ports.PartyRegistrySaveOutcome
	mustWithinPublicationTransaction(t, transactor, t.Context(), func(txCtx context.Context) error {
		var err error
		outcome, err = registrations.SaveBusinessParty(txCtx, registration)
		return err
	})
	if outcome != ports.PartyRegistryAlreadyRegistered {
		t.Fatalf("replay outcome = %s, want ALREADY_REGISTERED", outcome)
	}

	renamed := partyRegistrationFixture(t, "tenant-1", "party-1", "参与方一号改名")
	mustWithinPublicationTransaction(t, transactor, t.Context(), func(txCtx context.Context) error {
		var err error
		outcome, err = registrations.SaveBusinessParty(txCtx, renamed)
		return err
	})
	if outcome != ports.PartyRegistryContentConflict {
		t.Fatalf("conflict outcome = %s, want CONTENT_CONFLICT", outcome)
	}
}

// Covers: 候选关系照候选存、照候选回——批准事实缺席不是坏数据。
func TestACandidateRelationshipRoundTripsWithoutApproval(t *testing.T) {
	registrations, catalogue, transactor := newPartyIdentityRegistrations(t)

	candidate := relationshipRegistrationFixture(t, "tenant-1", "rel-cand", "party-a", "party-b", false)
	mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
		return registrations.SaveRelationship(txCtx, candidate)
	})

	loaded, found, err := registrations.LoadLatestRelationship(
		t.Context(), pcTenant(t, "tenant-1"), pcValue(t, domain.NewRelationshipID, "rel-cand"))
	if err != nil || !found {
		t.Fatalf("load candidate = (%v, %v)", found, err)
	}
	if loaded.Relationship().Status() != domain.RelationshipCandidate {
		t.Fatalf("candidate status = %v", loaded.Relationship().Status())
	}

	rows, err := catalogue.ListPartyRelationships(t.Context(), pcTenant(t, "tenant-1"), 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("list = (%d, %v)", len(rows), err)
	}
	// 双方参与方没登记：名称栏如实缺席，不补占位文本。
	if rows[0].Status != "CANDIDATE" || rows[0].HasHolderName || rows[0].HasCounterpartyName {
		t.Fatalf("candidate row = %+v", rows[0])
	}
}

func TestPartyIdentityWritesRefuseToRunOutsideATransaction(t *testing.T) {
	registrations, _, _ := newPartyIdentityRegistrations(t)
	_, err := registrations.SaveBusinessParty(t.Context(),
		partyRegistrationFixture(t, "tenant-1", "party-1", "参与方一号"))
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记应返回 ErrTransactionRequired，实得：%v", err)
	}
}
