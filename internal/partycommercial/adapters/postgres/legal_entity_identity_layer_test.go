package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 票 legal-entity-profile/02 的真库用例：责任法人登记经应用层用例、按真注册号类型目录判身份层，落进 0034 加的
// 三列，再从目录上列、修订历史与最新修订装载三处读回。国家 / 地区取 ISO 3166 用户自定义码，类型、格式与号全为
// 合成值。

var legalEntityIdentityFrom = time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

type legalEntityIdentityFixture struct {
	pool       *pgxpool.Pool
	db         *bentopg.DB
	identities *adapter.PartyIdentityRegistrations
	types      *adapter.RegistrationNumberTypes
	catalogue  *adapter.OperationsCatalogue
	transactor bentoapp.Transactor
	handler    *application.RegisterPartyIdentityHandler
}

func newLegalEntityIdentityFixture(t *testing.T) legalEntityIdentityFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	identities, err := adapter.NewPartyIdentityRegistrations(db)
	if err != nil {
		t.Fatalf("构造身份登记册：%v", err)
	}
	types, err := adapter.NewRegistrationNumberTypes(db)
	if err != nil {
		t.Fatalf("构造注册号类型目录：%v", err)
	}
	catalogue, err := adapter.NewOperationsCatalogue(db)
	if err != nil {
		t.Fatalf("构造目录读口：%v", err)
	}
	fixture := legalEntityIdentityFixture{
		pool:       pool,
		db:         db,
		identities: identities,
		types:      types,
		catalogue:  catalogue,
		transactor: db.Transactor(),
		handler:    application.NewRegisterPartyIdentityHandler(identities, types),
	}
	for _, registration := range []domain.RegistrationNumberTypeRegistration{
		registrationTypeFixture(t, "tenant-1", "XA", "SYN-XA-LIFETIME", 1, domain.RegistrationNumberIdentityLayer, `SYN-XA-[0-9]{6}`),
		registrationTypeFixture(t, "tenant-1", "XA", "SYN-XA-TAX", 1, domain.RegistrationNumberProfileLayer, `SYN-XA-TAX-[0-9]{4}`),
	} {
		saveRegistrationType(t, fixture.transactor, types, registration)
	}
	mustSavePartyIdentity(t, fixture.transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
		return identities.SaveBusinessParty(txCtx, partyRegistrationFixture(t, "tenant-1", "party-le", "运营法人参与方"))
	})
	return fixture
}

func (fixture legalEntityIdentityFixture) register(
	t *testing.T,
	command application.RegisterLegalEntityCommand,
) application.PartyRegistryResult {
	t.Helper()
	var result application.PartyRegistryResult
	mustWithinPublicationTransaction(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		var err error
		result, err = fixture.handler.RegisterLegalEntity(txCtx, command)
		return err
	})
	return result
}

func identityCommand(t *testing.T, revision int, country, code, number string) application.RegisterLegalEntityCommand {
	t.Helper()
	command := application.RegisterLegalEntityCommand{
		Tenant:        pcTenant(t, "tenant-1"),
		Entity:        pcValue(t, domain.NewLegalEntityReference, "le-1"),
		Party:         pcValue(t, domain.NewPartyID, "party-le"),
		Revision:      revision,
		Basis:         pcValue(t, domain.NewIdentityBasisReference, "basis-le"),
		EffectiveFrom: legalEntityIdentityFrom,
	}
	if country != "" {
		value := pcValue(t, domain.NewRegistrationCountryCode, country)
		command.RegistrationCountry = &value
	}
	if code != "" {
		lifetime, err := domain.NewLifetimeRegistrationNumber(
			pcValue(t, domain.NewRegistrationNumberTypeCode, code),
			pcValue(t, domain.NewRegistrationNumber, number),
		)
		if err != nil {
			t.Fatalf("new lifetime registration number: %v", err)
		}
		command.LifetimeNumbers = []domain.LifetimeRegistrationNumber{lifetime}
	}
	return command
}

func expectRefused(t *testing.T, result application.PartyRegistryResult, mentions string) {
	t.Helper()
	if result.Outcome() != application.PartyIdentityNotAccepted {
		t.Fatalf("outcome = %s, want NOT_ACCEPTED", result.Outcome())
	}
	if result.Cause() == nil || !strings.Contains(result.Cause().Error(), mentions) {
		t.Fatalf("cause = %v, want it to mention %q", result.Cause(), mentions)
	}
}

// Covers: 票 legal-entity-profile/02 完成判据「真库用例：缺国家、缺号、号不属该国身份层类型、格式不符各拒一条；
// 更正修订改号须带依据，不带即拒」——拒绝的一个字节不写；合格即落册，身份层从目录上列与修订历史读回。
func TestLegalEntityIdentityLayerAgainstTheRealCatalogue(t *testing.T) {
	fixture := newLegalEntityIdentityFixture(t)
	ctx := t.Context()

	expectRefused(t, fixture.register(t, identityCommand(t, 1, "", "SYN-XA-LIFETIME", "SYN-XA-000001")), "缺注册国家 / 地区")
	expectRefused(t, fixture.register(t, identityCommand(t, 1, "XA", "", "")), "缺终身注册号")
	expectRefused(t, fixture.register(t, identityCommand(t, 1, "XA", "SYN-XA-TAX", "SYN-XA-TAX-0001")), "属资料层")
	expectRefused(t, fixture.register(t, identityCommand(t, 1, "XA", "SYN-XA-LIFETIME", "SYN-XA-12")), "不合类型")
	if rows, err := fixture.catalogue.ListGroupLegalEntities(ctx, pcTenant(t, "tenant-1"), 10); err != nil || len(rows) != 0 {
		t.Fatalf("refused registrations wrote rows: %d rows, err %v", len(rows), err)
	}

	if result := fixture.register(t, identityCommand(t, 1, "XA", "SYN-XA-LIFETIME", "SYN-XA-000001")); result.Outcome() != application.PartyIdentityRegistered {
		t.Fatalf("registration outcome = %s (%v)", result.Outcome(), result.Cause())
	}
	expectRefused(t, fixture.register(t, identityCommand(t, 2, "XA", "SYN-XA-LIFETIME", "SYN-XA-000009")), "不作变更")

	corrected := identityCommand(t, 2, "XA", "SYN-XA-LIFETIME", "SYN-XA-000009")
	basis := pcValue(t, domain.NewIdentityBasisReference, "SYN-CORRECTION-01")
	corrected.IdentityCorrectionBasis = &basis
	if result := fixture.register(t, corrected); result.Outcome() != application.PartyIdentityRegistered {
		t.Fatalf("correction outcome = %s (%v)", result.Outcome(), result.Cause())
	}

	rows, err := fixture.catalogue.ListGroupLegalEntities(ctx, pcTenant(t, "tenant-1"), 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("catalogue = %d rows, err %v", len(rows), err)
	}
	row := rows[0]
	if !row.HasIdentityLayer || row.RegistrationCountry != "XA" || len(row.LifetimeNumbers) != 1 ||
		row.LifetimeNumbers[0].TypeCode != "SYN-XA-LIFETIME" || row.LifetimeNumbers[0].Number != "SYN-XA-000009" {
		t.Fatalf("catalogue identity layer = %+v", row)
	}
	if !row.HasIdentityCorrection || row.IdentityCorrectionBasis != "SYN-CORRECTION-01" {
		t.Fatalf("catalogue correction = (%v, %q)", row.HasIdentityCorrection, row.IdentityCorrectionBasis)
	}

	revisions, err := fixture.catalogue.ListLegalEntityRevisions(ctx, pcTenant(t, "tenant-1"), pcValue(t, domain.NewLegalEntityReference, "le-1"))
	if err != nil || len(revisions) != 2 {
		t.Fatalf("revisions = %d, err %v", len(revisions), err)
	}
	if revisions[0].LifetimeNumbers[0].Number != "SYN-XA-000001" || revisions[0].HasIdentityCorrection {
		t.Fatalf("revision 1 = %+v", revisions[0])
	}
	if revisions[1].LifetimeNumbers[0].Number != "SYN-XA-000009" || !revisions[1].HasIdentityCorrection {
		t.Fatalf("revision 2 = %+v", revisions[1])
	}

	latest, found, err := fixture.identities.LoadLatestLegalEntity(ctx, pcTenant(t, "tenant-1"), pcValue(t, domain.NewLegalEntityReference, "le-1"))
	if err != nil || !found {
		t.Fatalf("load latest = (%v, %v)", found, err)
	}
	if layer, has := latest.IdentityLayer(); !has || layer.Numbers()[0].Number().String() != "SYN-XA-000009" {
		t.Fatalf("latest identity layer = %+v (has %v)", layer, has)
	}
}

// Covers: 票 legal-entity-profile/02 完成判据「历史修订读回时两格为空且不报错」——本格落地之前形状的修订落 NULL，
// 快照里没有新键（内容摘要与加格前同一串字节）；三处读回都答「没有身份层」；原样重放答重复；库拒只有一格的行。
func TestHistoricalLegalEntityRevisionReadsBackWithoutIdentityLayer(t *testing.T) {
	fixture := newLegalEntityIdentityFixture(t)
	ctx := t.Context()

	mustSavePartyIdentity(t, fixture.transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
		return fixture.identities.SaveLegalEntity(txCtx, legalEntityRegistrationFixture(t, "tenant-1", "le-legacy", "party-le"))
	})

	var hasNewKey bool
	if err := fixture.pool.QueryRow(ctx,
		`SELECT snapshot ? 'registrationCountry' OR snapshot ? 'lifetimeRegistrationNumbers' OR snapshot ? 'identityCorrectionBasis'
		   FROM party_commercial.legal_entity_registration WHERE legal_entity_id = 'le-legacy'`,
	).Scan(&hasNewKey); err != nil {
		t.Fatalf("read snapshot keys: %v", err)
	}
	if hasNewKey {
		t.Fatal("a historical-shape snapshot must not gain the new keys; its content digest would change")
	}

	latest, found, err := fixture.identities.LoadLatestLegalEntity(ctx, pcTenant(t, "tenant-1"), pcValue(t, domain.NewLegalEntityReference, "le-legacy"))
	if err != nil || !found {
		t.Fatalf("load latest = (%v, %v)", found, err)
	}
	if _, has := latest.IdentityLayer(); has {
		t.Fatal("historical revision must read back without an identity layer")
	}
	rows, err := fixture.catalogue.ListGroupLegalEntities(ctx, pcTenant(t, "tenant-1"), 10)
	if err != nil || len(rows) != 1 || rows[0].HasIdentityLayer || rows[0].HasIdentityCorrection {
		t.Fatalf("catalogue = %+v, err %v", rows, err)
	}
	revisions, err := fixture.catalogue.ListLegalEntityRevisions(ctx, pcTenant(t, "tenant-1"), pcValue(t, domain.NewLegalEntityReference, "le-legacy"))
	if err != nil || len(revisions) != 1 || revisions[0].HasIdentityLayer {
		t.Fatalf("revisions = %+v, err %v", revisions, err)
	}

	replay := application.RegisterLegalEntityCommand{
		Tenant:        pcTenant(t, "tenant-1"),
		Entity:        pcValue(t, domain.NewLegalEntityReference, "le-legacy"),
		Party:         pcValue(t, domain.NewPartyID, "party-le"),
		Revision:      1,
		Basis:         pcValue(t, domain.NewIdentityBasisReference, "basis-le-legacy"),
		EffectiveFrom: identityEffectiveFrom,
	}
	if result := fixture.register(t, replay); result.Outcome() != application.PartyIdentityAlreadyRegistered {
		t.Fatalf("legacy replay = %s (%v), want ALREADY_REGISTERED", result.Outcome(), result.Cause())
	}

	_, err = fixture.pool.Exec(ctx,
		`UPDATE party_commercial.legal_entity_registration SET registration_country = 'XA' WHERE legal_entity_id = 'le-legacy'`)
	if err == nil || !strings.Contains(err.Error(), "legal_entity_registration_identity_paired") {
		t.Fatalf("country without numbers must violate the paired constraint, got %v", err)
	}
}
