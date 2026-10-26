package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证参与方关系三种结束的落库（票 ps-external-mark-relations/17）：终止修订经结束口落册、读回
// 同一份终止事实、目录如实上列；0042 的 CHECK 拒领域拒的两种终止行；授权交付关系撤销之后判断即时答否，替代之后改由
// 接替的交付方答是。

var endingRegisteredAt = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

type endingOnRegister struct {
	db         *bentopg.DB
	transactor bentoapp.Transactor
	register   *application.RegisterPartyIdentityHandler
	ending     *application.PartyRelationshipEndingHandler
	registry   *adapter.PartyIdentityRegistrations
	catalogue  *adapter.OperationsCatalogue
}

func newEndingOnRegister(t *testing.T) endingOnRegister {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registry, err := adapter.NewPartyIdentityRegistrations(db)
	if err != nil {
		t.Fatalf("构造身份登记册：%v", err)
	}
	catalogue, err := adapter.NewOperationsCatalogue(db)
	if err != nil {
		t.Fatalf("构造目录读口：%v", err)
	}
	return endingOnRegister{
		db:         db,
		transactor: db.Transactor(),
		register:   application.NewRegisterPartyIdentityHandler(registry, nil),
		ending:     application.NewPartyRelationshipEndingHandler(registry, fixedClock{at: endingRegisteredAt}),
		registry:   registry,
		catalogue:  catalogue,
	}
}

// within 把一次用例调用包进一笔事务，形照受控批量口与在线口的装配：用例交回 error 即整笔回滚。
func (fixture endingOnRegister) within(
	t *testing.T,
	call func(context.Context) (application.PartyRegistryResult, error),
) (application.PartyRegistryResult, error) {
	t.Helper()
	var result application.PartyRegistryResult
	err := fixture.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var callErr error
		result, callErr = call(txCtx)
		return callErr
	})
	return result, err
}

func (fixture endingOnRegister) mustLand(
	t *testing.T,
	label string,
	want application.PartyRegistryOutcome,
	call func(context.Context) (application.PartyRegistryResult, error),
) {
	t.Helper()
	result, err := fixture.within(t, call)
	if err != nil || result.Outcome() != want {
		t.Fatalf("%s = (%v, cause %v, err %v), want %v", label, result.Outcome(), result.Cause(), err, want)
	}
}

func (fixture endingOnRegister) registerParties(t *testing.T, tenant string, ids ...string) {
	t.Helper()
	for _, id := range ids {
		party, err := domain.NewBusinessParty(pcTenant(t, tenant), pcValue(t, domain.NewPartyID, id), pcValue(t, domain.NewPartyName, "SYN 合成参与方 "+id))
		if err != nil {
			t.Fatalf("party %s: %v", id, err)
		}
		fixture.mustLand(t, "参与方 "+id, application.PartyIdentityRegistered, func(ctx context.Context) (application.PartyRegistryResult, error) {
			return fixture.register.RegisterBusinessParty(ctx, application.RegisterBusinessPartyCommand{
				Party:         party,
				Revision:      1,
				Basis:         pcValue(t, domain.NewIdentityBasisReference, "SYN-BASIS/"+id),
				EffectiveFrom: identityEffectiveFrom,
			})
		})
	}
}

func (fixture endingOnRegister) registerRelationship(t *testing.T, tenant, id string, spec domain.PartyRelationshipSpec) {
	t.Helper()
	fixture.mustLand(t, "关系 "+id, application.PartyIdentityRegistered, func(ctx context.Context) (application.PartyRegistryResult, error) {
		return fixture.register.RegisterRelationship(ctx, application.RegisterPartyRelationshipCommand{
			Tenant:   pcTenant(t, tenant),
			ID:       pcValue(t, domain.NewRelationshipID, id),
			Revision: 1,
			Spec:     spec,
			Approval: &application.RelationshipApproval{
				Reference:  pcValue(t, domain.NewApprovalReference, "SYN-APPROVAL/"+id),
				ApprovedAt: spec.Effective.StartsAt(),
			},
		})
	})
}

func customerSpec(t *testing.T, holder, counterparty string, startsAt, endsAt time.Time) domain.PartyRelationshipSpec {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(startsAt, endsAt)
	if err != nil {
		t.Fatalf("interval: %v", err)
	}
	return domain.PartyRelationshipSpec{
		Holder:       pcValue(t, domain.NewPartyID, holder),
		Counterparty: pcValue(t, domain.NewPartyID, counterparty),
		Role:         domain.CustomerRole,
		Scope:        pcScope(t),
		Basis:        pcValue(t, domain.NewRelationshipBasisReference, "SYN-CONTRACT/"+holder),
		Effective:    interval,
	}
}

// Covers: 票 17 完成判据「领域、编排、PG……各层用例」的 PG 一格——撤销、到期、替代经结束口落下终止修订，读回的最新修订
// 带着同一份终止事实（时刻、依据、接替方），目录上列的那一行也是；替代的接替关系在同一事务里落册；同一笔重放答重复。
func TestEndedRelationshipsLandAndReadBackFromTheRegister(t *testing.T) {
	fixture := newEndingOnRegister(t)
	const tenant = "SYN-TENANT-01"
	fixture.registerParties(t, tenant, "SYN-PARTY-H1", "SYN-PARTY-C1", "SYN-PARTY-H2")
	revokedAt := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	boundedEnd := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	fixture.registerRelationship(t, tenant, "SYN-REL-REVOKE", customerSpec(t, "SYN-PARTY-H1", "SYN-PARTY-C1", relationshipStartsAt, time.Time{}))
	fixture.registerRelationship(t, tenant, "SYN-REL-EXPIRE", customerSpec(t, "SYN-PARTY-H1", "SYN-PARTY-C1", relationshipStartsAt, boundedEnd))
	fixture.registerRelationship(t, tenant, "SYN-REL-SUPERSEDE", customerSpec(t, "SYN-PARTY-H1", "SYN-PARTY-C1", relationshipStartsAt, time.Time{}))

	revoke := application.RevokePartyRelationshipCommand{
		Tenant: pcTenant(t, tenant), ID: pcValue(t, domain.NewRelationshipID, "SYN-REL-REVOKE"), Revision: 2,
		Basis: pcValue(t, domain.NewRelationshipBasisReference, "SYN-REVOCATION/01"), At: revokedAt,
	}
	expire := application.ExpirePartyRelationshipCommand{
		Tenant: pcTenant(t, tenant), ID: pcValue(t, domain.NewRelationshipID, "SYN-REL-EXPIRE"), Revision: 2,
		Basis: pcValue(t, domain.NewRelationshipBasisReference, "SYN-TERM-ENDED/01"), At: boundedEnd,
	}
	supersede := application.SupersedePartyRelationshipCommand{
		Tenant: pcTenant(t, tenant), ID: pcValue(t, domain.NewRelationshipID, "SYN-REL-SUPERSEDE"), Revision: 2,
		Successor: pcValue(t, domain.NewPartyID, "SYN-PARTY-H2"), At: revokedAt,
		SuccessorRelationship: pcValue(t, domain.NewRelationshipID, "SYN-REL-SUPERSEDE-NEXT"),
		Basis:                 pcValue(t, domain.NewRelationshipBasisReference, "SYN-HANDOVER/01"),
		Approval: application.RelationshipApproval{
			Reference: pcValue(t, domain.NewApprovalReference, "SYN-APPROVAL/handover-01"), ApprovedAt: revokedAt,
		},
	}
	for _, replayed := range []bool{false, true} {
		want := application.PartyRelationshipEnded
		if replayed {
			want = application.PartyIdentityAlreadyRegistered
		}
		fixture.mustLand(t, "撤销", want, func(ctx context.Context) (application.PartyRegistryResult, error) {
			return fixture.ending.Revoke(ctx, revoke)
		})
		fixture.mustLand(t, "到期", want, func(ctx context.Context) (application.PartyRegistryResult, error) {
			return fixture.ending.Expire(ctx, expire)
		})
		fixture.mustLand(t, "替代", want, func(ctx context.Context) (application.PartyRegistryResult, error) {
			return fixture.ending.Supersede(ctx, supersede)
		})
	}

	load := func(id string) domain.PartyRelationshipRegistration {
		t.Helper()
		latest, found, err := fixture.registry.LoadLatestRelationship(t.Context(), pcTenant(t, tenant), pcValue(t, domain.NewRelationshipID, id))
		if err != nil || !found {
			t.Fatalf("load %s = (%v, %v)", id, found, err)
		}
		return latest
	}
	for id, want := range map[string]struct {
		status    domain.RelationshipStatus
		endedAt   time.Time
		endBasis  string
		successor string
	}{
		"SYN-REL-REVOKE":    {domain.RelationshipRevoked, revokedAt, "SYN-REVOCATION/01", ""},
		"SYN-REL-EXPIRE":    {domain.RelationshipExpired, boundedEnd, "SYN-TERM-ENDED/01", ""},
		"SYN-REL-SUPERSEDE": {domain.RelationshipSuperseded, revokedAt, "", "SYN-PARTY-H2"},
	} {
		latest := load(id)
		relationship := latest.Relationship()
		endedAt, _ := relationship.EndedAt()
		endBasis, _ := relationship.EndBasis()
		successor, _ := relationship.Successor()
		if latest.Revision() != 2 || relationship.Status() != want.status || !endedAt.Equal(want.endedAt) ||
			endBasis.String() != want.endBasis || successor.String() != want.successor {
			t.Fatalf("%s 读回 r%d %s ended %v basis %q successor %q, want %+v",
				id, latest.Revision(), relationship.Status(), endedAt, endBasis.String(), successor.String(), want)
		}
	}
	next := load("SYN-REL-SUPERSEDE-NEXT")
	if next.Revision() != 1 || next.Relationship().Status() != domain.RelationshipEffective ||
		next.Relationship().Holder().String() != "SYN-PARTY-H2" || !next.Relationship().Effective().StartsAt().Equal(revokedAt) {
		t.Fatalf("接替关系读回 r%d %s holder %s starts %v", next.Revision(), next.Relationship().Status(),
			next.Relationship().Holder(), next.Relationship().Effective().StartsAt())
	}

	rows, err := fixture.catalogue.ListPartyRelationships(t.Context(), pcTenant(t, tenant), 50)
	if err != nil {
		t.Fatalf("list relationships: %v", err)
	}
	listed := map[string]bool{}
	for _, row := range rows {
		listed[row.RelationshipID] = true
		switch row.RelationshipID {
		case "SYN-REL-EXPIRE":
			if row.Status != "EXPIRED" || !row.HasEnd || row.EndBasis != "SYN-TERM-ENDED/01" || row.Revision != 2 {
				t.Fatalf("目录上列的到期行 = %+v", row)
			}
		case "SYN-REL-SUPERSEDE":
			if row.Status != "SUPERSEDED" || row.SuccessorID != "SYN-PARTY-H2" || row.EndBasis != "" {
				t.Fatalf("目录上列的替代行 = %+v", row)
			}
		}
	}
	if !listed["SYN-REL-EXPIRE"] || !listed["SYN-REL-SUPERSEDE"] || !listed["SYN-REL-SUPERSEDE-NEXT"] {
		t.Fatalf("目录少了行：%v", listed)
	}
}

// Covers: 0042 的两处收紧——到期行缺终止依据、替代行的接替方是原持有方或相对方，库都拒，拒的是 end_coherent。领域产不出
// 这样的行，这里绕过适配器直插。
func TestTheRegisterRefusesEndingsTheDomainRefuses(t *testing.T) {
	pool := pgtest.Pool(t)
	const insert = `INSERT INTO party_commercial.party_relationship_registration
		(tenant_id, relationship_id, revision, holder_party_id, counterparty_party_id, role, scope_ref, basis_ref, status,
		 effective_starts_at, effective_ends_at, approval_ref, approved_at, ended_at, end_basis, successor_party_id,
		 content_digest, snapshot)
	 VALUES ('SYN-TENANT-01', $1, 2, 'SYN-PARTY-H1', 'SYN-PARTY-C1', 'CUSTOMER', 'SYN-SCOPE', 'SYN-CONTRACT', $2,
		 '2026-02-01T00:00:00Z', '2026-06-01T00:00:00Z', 'SYN-APPROVAL', '2026-02-01T00:00:00Z', '2026-06-01T00:00:00Z', $3, $4,
		 'sha256:probe', '{}'::jsonb)`
	for _, probe := range []struct {
		id        string
		status    string
		endBasis  any
		successor any
	}{
		{"SYN-REL-EXPIRED-NO-BASIS", "EXPIRED", nil, nil},
		{"SYN-REL-SUPERSEDED-BY-HOLDER", "SUPERSEDED", nil, "SYN-PARTY-H1"},
		{"SYN-REL-SUPERSEDED-BY-COUNTERPARTY", "SUPERSEDED", nil, "SYN-PARTY-C1"},
	} {
		_, err := pool.Exec(t.Context(), insert, probe.id, probe.status, probe.endBasis, probe.successor)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.ConstraintName != "party_relationship_registration_end_coherent" {
			t.Fatalf("%s：库没有按 end_coherent 拒：%v", probe.id, err)
		}
	}
	if _, err := pool.Exec(t.Context(), insert, "SYN-REL-EXPIRED-WITH-BASIS", "EXPIRED", "SYN-TERM-ENDED", nil); err != nil {
		t.Fatalf("带依据的到期行被拒：%v", err)
	}
}

// Covers: 票 17 完成判据「撤销后授权交付判断即时答否（真库）」——授权交付关系经登记口落册、判断答是；经撤销口落下终止
// 修订，同一时刻再问即答否，不等任何时钟。另证替代：原交付方答否，接替的交付方答是；判断读口没有改，读的就是最新修订。
func TestRevokingAnAuthorizedDeliveryRelationshipAnswersNoAtOnce(t *testing.T) {
	fixture := newEndingOnRegister(t)
	judgments := newAuthorizedDeliveryJudgments(t, fixture.db)
	const (
		tenant  = "SYN-TENANT-01"
		account = "SYN-ACCOUNT-01"
		owner   = "SYN-PARTY-SHIPPER-01"
		hub     = "SYN-NODE-SHA-HUB"
	)
	fixture.registerParties(t, tenant, owner, "SYN-PARTY-DELIVERER-01", "SYN-PARTY-DELIVERER-02", "SYN-PARTY-DELIVERER-03")
	fixture.mustLand(t, "客户账户", application.PartyIdentityRegistered, func(ctx context.Context) (application.PartyRegistryResult, error) {
		return fixture.register.RegisterCustomerAccount(ctx, application.RegisterCustomerAccountCommand{
			Tenant: pcTenant(t, tenant), Account: pcValue(t, domain.NewCustomerAccountID, account),
			CustomerParty: pcValue(t, domain.NewPartyID, owner), Revision: 1,
			Basis: pcValue(t, domain.NewIdentityBasisReference, "SYN-BASIS/account"), EffectiveFrom: identityEffectiveFrom,
		})
	})
	scope, err := domain.NewAuthorizedDeliveryScope(pcValue(t, domain.NewCustomerAccountID, account), nil)
	if err != nil {
		t.Fatalf("scope: %v", err)
	}
	deliverySpec := func(holder string) domain.PartyRelationshipSpec {
		interval, err := domain.NewEffectiveInterval(deliveryStartsAt, time.Time{})
		if err != nil {
			t.Fatalf("interval: %v", err)
		}
		return domain.PartyRelationshipSpec{
			Holder: pcValue(t, domain.NewPartyID, holder), Counterparty: pcValue(t, domain.NewPartyID, owner),
			Role: domain.AuthorizedDeliveringPartyRole, Scope: scope.Reference(),
			Basis: pcValue(t, domain.NewRelationshipBasisReference, "SYN-MANDATE/"+holder), Effective: interval,
		}
	}
	fixture.registerRelationship(t, tenant, "SYN-REL-DELIVERY-REVOKE", deliverySpec("SYN-PARTY-DELIVERER-01"))
	fixture.registerRelationship(t, tenant, "SYN-REL-DELIVERY-HANDOVER", deliverySpec("SYN-PARTY-DELIVERER-02"))

	asked := endingRegisteredAt.Add(time.Hour)
	judge := func(party string) domain.AuthorizedDeliveryAnswer {
		t.Helper()
		answer, err := judgments.JudgeAuthorizedDelivery(t.Context(), authorizedDeliveryQuery(t, tenant, party, account, hub, asked))
		if err != nil {
			t.Fatalf("judge %s: %v", party, err)
		}
		return answer
	}
	if got := judge("SYN-PARTY-DELIVERER-01"); got != domain.DeliveryAuthorized {
		t.Fatalf("撤销之前 = %s, want AUTHORIZED", got)
	}

	revokedAt := endingRegisteredAt.Add(-time.Minute)
	fixture.mustLand(t, "撤销授权交付关系", application.PartyRelationshipEnded, func(ctx context.Context) (application.PartyRegistryResult, error) {
		return fixture.ending.Revoke(ctx, application.RevokePartyRelationshipCommand{
			Tenant: pcTenant(t, tenant), ID: pcValue(t, domain.NewRelationshipID, "SYN-REL-DELIVERY-REVOKE"), Revision: 2,
			Basis: pcValue(t, domain.NewRelationshipBasisReference, "SYN-REVOCATION/delivery-01"), At: revokedAt,
		})
	})
	if got := judge("SYN-PARTY-DELIVERER-01"); got != domain.DeliveryNotAuthorized {
		t.Fatalf("撤销之后同一时刻再问 = %s, want NOT_AUTHORIZED", got)
	}

	if got := judge("SYN-PARTY-DELIVERER-03"); got != domain.DeliveryNotAuthorized {
		t.Fatalf("替代之前接替方 = %s, want NOT_AUTHORIZED", got)
	}
	fixture.mustLand(t, "替代授权交付关系", application.PartyRelationshipEnded, func(ctx context.Context) (application.PartyRegistryResult, error) {
		return fixture.ending.Supersede(ctx, application.SupersedePartyRelationshipCommand{
			Tenant: pcTenant(t, tenant), ID: pcValue(t, domain.NewRelationshipID, "SYN-REL-DELIVERY-HANDOVER"), Revision: 2,
			Successor: pcValue(t, domain.NewPartyID, "SYN-PARTY-DELIVERER-03"), At: revokedAt,
			SuccessorRelationship: pcValue(t, domain.NewRelationshipID, "SYN-REL-DELIVERY-HANDOVER-NEXT"),
			Basis:                 pcValue(t, domain.NewRelationshipBasisReference, "SYN-HANDOVER/delivery-02"),
			Approval: application.RelationshipApproval{
				Reference: pcValue(t, domain.NewApprovalReference, "SYN-APPROVAL/handover-delivery-02"), ApprovedAt: revokedAt,
			},
		})
	})
	if got := judge("SYN-PARTY-DELIVERER-02"); got != domain.DeliveryNotAuthorized {
		t.Fatalf("替代之后原交付方 = %s, want NOT_AUTHORIZED", got)
	}
	if got := judge("SYN-PARTY-DELIVERER-03"); got != domain.DeliveryAuthorized {
		t.Fatalf("替代之后接替的交付方 = %s, want AUTHORIZED", got)
	}
}
