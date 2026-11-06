package postgres_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// newIdentificationRegistrations 同 newPartyIdentityRegistrations，另交回连接池：要直接读快照原文。
func newIdentificationRegistrations(t *testing.T) (*adapter.PartyIdentityRegistrations, *adapter.OperationsCatalogue, bentoapp.Transactor, *pgxpool.Pool) {
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
	return registrations, catalogue, db.Transactor(), pool
}

// 本文件对真实 PostgreSQL 证往来单位识别信息（ADR-0367，迁移 0045）：带识别信息的修订往返重建、目录与修订历史如实交出、
// 没带识别信息的快照字节不变；查重读口只认各身份最新修订且在用的那一笔、往来单位与公司主体一并查、跨租户不见、
// 无事务拒；同号两笔并发时后到的那一笔等前一笔提交、读得到它。

func identityLayerFixture(t *testing.T, country, typeCode, number string) domain.LegalEntityIdentityLayer {
	t.Helper()
	lifetime, err := domain.NewLifetimeRegistrationNumber(
		pcValue(t, domain.NewRegistrationNumberTypeCode, typeCode),
		pcValue(t, domain.NewRegistrationNumber, number),
	)
	if err != nil {
		t.Fatalf("lifetime number: %v", err)
	}
	layer, err := domain.NewLegalEntityIdentityLayer(pcValue(t, domain.NewRegistrationCountryCode, country), []domain.LifetimeRegistrationNumber{lifetime})
	if err != nil {
		t.Fatalf("identity layer: %v", err)
	}
	return layer
}

func identifiedPartyFixture(
	t *testing.T,
	tenant, id string,
	layer *domain.LegalEntityIdentityLayer,
	references ...domain.ExternalReference,
) domain.BusinessPartyRegistration {
	t.Helper()
	registration, err := partyRegistrationFixture(t, tenant, id, "合成单位 "+id).WithIdentification(layer, nil, references)
	if err != nil {
		t.Fatalf("with identification: %v", err)
	}
	return registration
}

func referenceFixture(t *testing.T, system, purpose, code string) domain.ExternalReference {
	t.Helper()
	reference, err := domain.ParseExternalReference(system, purpose, code)
	if err != nil {
		t.Fatalf("external reference: %v", err)
	}
	return reference
}

// Covers: 带识别信息的修订往返重建；目录与修订历史交出四格；没带识别信息的修订快照里没有任何新键（重放摘要不变）。
func TestBusinessPartyIdentificationRoundTrip(t *testing.T) {
	registrations, catalogue, transactor, pool := newIdentificationRegistrations(t)
	ctx := t.Context()

	plain := partyRegistrationFixture(t, "tenant-1", "party-plain", "合成单位 plain")
	mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
		return registrations.SaveBusinessParty(txCtx, plain)
	})
	layer := identityLayerFixture(t, "CN", "SYN-CN-REG", "SYN-CN-REG-0001")
	identified := identifiedPartyFixture(t, "tenant-1", "party-id", &layer,
		referenceFixture(t, "SYN-ERP", "SUPPLIER", "V-0001"), referenceFixture(t, "SYN-ERP", "CUSTOMER", "C-0001"))
	mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
		return registrations.SaveBusinessParty(txCtx, identified)
	})

	loaded, found, err := registrations.LoadLatestBusinessParty(ctx, pcTenant(t, "tenant-1"), pcValue(t, domain.NewPartyID, "party-id"))
	if err != nil || !found {
		t.Fatalf("load: %v found=%v", err, found)
	}
	if got, has := loaded.IdentityLayer(); !has || !got.Equal(layer) {
		t.Fatalf("identity layer = %+v has=%v", got, has)
	}
	if references := loaded.ExternalReferences(); len(references) != 2 || references[0].Code() != "C-0001" || references[1].Code() != "V-0001" {
		t.Fatalf("external references = %+v", references)
	}

	var snapshot string
	if err := pool.QueryRow(ctx,
		`SELECT snapshot::text FROM party_commercial.business_party_registration WHERE party_id = 'party-plain'`).Scan(&snapshot); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"registrationCountry", "lifetimeRegistrationNumbers", "identityCorrectionBasis", "externalReferences"} {
		if strings.Contains(snapshot, key) {
			t.Fatalf("没带识别信息的快照不该出现 %s：%s", key, snapshot)
		}
	}

	rows, err := rowsOf(catalogue.ListBusinessParties(ctx, pcTenant(t, "tenant-1"), 50, firstPage(t, ports.BusinessPartyPages)))
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]ports.BusinessPartyRow{}
	for _, row := range rows {
		byID[row.PartyID] = row
	}
	if row := byID["party-plain"]; row.HasIdentityLayer || row.ExternalReferences != nil {
		t.Fatalf("plain row = %+v", row.BusinessPartyIdentificationRow)
	}
	if row := byID["party-id"]; !row.HasIdentityLayer || row.RegistrationCountry != "CN" ||
		len(row.LifetimeNumbers) != 1 || row.LifetimeNumbers[0].Number != "SYN-CN-REG-0001" ||
		len(row.ExternalReferences) != 2 || row.ExternalReferences[0].Purpose != "CUSTOMER" {
		t.Fatalf("identified row = %+v", row.BusinessPartyIdentificationRow)
	}
	revisions, err := catalogue.ListBusinessPartyRevisions(ctx, pcTenant(t, "tenant-1"), pcValue(t, domain.NewPartyID, "party-id"))
	if err != nil || len(revisions) != 1 || !revisions[0].HasIdentityLayer || len(revisions[0].ExternalReferences) != 2 {
		t.Fatalf("revisions = %+v err=%v", revisions, err)
	}
}

// Covers: 查重只认最新修订且在用；往来单位与公司主体一并查，公司主体带上它钉的参与方；已停用的不算；跨租户不见；
// 外部编码按系统、用途与码三者一起认；钉着参与方的公司主体按最新修订找。
func TestIdentificationHolderLookups(t *testing.T) {
	registrations, _, transactor := newPartyIdentityRegistrations(t)
	layer := identityLayerFixture(t, "CN", "SYN-CN-REG", "SYN-CN-REG-0001")
	customer := referenceFixture(t, "SYN-ERP", "CUSTOMER", "C-0001")

	live := identifiedPartyFixture(t, "tenant-1", "party-live", &layer, customer)
	stopped := identifiedPartyFixture(t, "tenant-1", "party-stopped", &layer, customer)
	stoppedNext, err := stopped.Deactivate(pcValue(t, domain.NewIdentityBasisReference, "basis-stop"), identityDeactivateAt)
	if err != nil {
		t.Fatal(err)
	}
	elsewhere := identifiedPartyFixture(t, "tenant-2", "party-elsewhere", &layer, customer)
	entity, err := legalEntityRegistrationFixture(t, "tenant-1", "le-1", "party-le").WithIdentityLayer(layer, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, registration := range []domain.BusinessPartyRegistration{live, stopped, stoppedNext, elsewhere, partyRegistrationFixture(t, "tenant-1", "party-le", "法人参与方")} {
		mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
			return registrations.SaveBusinessParty(txCtx, registration)
		})
	}
	mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
		return registrations.SaveLegalEntity(txCtx, entity)
	})

	mustWithinPublicationTransaction(t, transactor, t.Context(), func(txCtx context.Context) error {
		holders, err := registrations.FindRegistrationNumberHolders(txCtx, pcTenant(t, "tenant-1"), layer)
		if err != nil {
			return err
		}
		if len(holders) != 2 ||
			holders[0].Kind != ports.IdentificationHeldByBusinessParty || holders[0].ID != "party-live" ||
			holders[1].Kind != ports.IdentificationHeldByLegalEntity || holders[1].ID != "le-1" || holders[1].Party != "party-le" {
			t.Errorf("number holders = %+v", holders)
		}
		other, err := registrations.FindRegistrationNumberHolders(txCtx, pcTenant(t, "tenant-1"),
			identityLayerFixture(t, "HK", "SYN-CN-REG", "SYN-CN-REG-0001"))
		if err != nil {
			return err
		}
		if len(other) != 0 {
			t.Errorf("同号不同国家不是同一个号：%+v", other)
		}
		references, err := registrations.FindExternalReferenceHolders(txCtx, pcTenant(t, "tenant-1"),
			[]domain.ExternalReference{customer, referenceFixture(t, "SYN-ERP", "SUPPLIER", "C-0001")})
		if err != nil {
			return err
		}
		if len(references) != 1 || references[0].Party != "party-live" || references[0].Purpose != "CUSTOMER" {
			t.Errorf("reference holders = %+v", references)
		}
		return nil
	})
	mustWithinPublicationTransaction(t, transactor, t.Context(), func(txCtx context.Context) error {
		pinning, err := registrations.FindLegalEntitiesPinning(txCtx, pcTenant(t, "tenant-1"), pcValue(t, domain.NewPartyID, "party-le"))
		if err != nil || len(pinning) != 1 || pinning[0] != "le-1" {
			t.Errorf("pinning = %v err=%v", pinning, err)
		}
		return nil
	})
	if _, err := registrations.FindLegalEntitiesPinning(t.Context(), pcTenant(t, "tenant-1"), pcValue(t, domain.NewPartyID, "party-le")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务查钉住应返回 ErrTransactionRequired，实得：%v", err)
	}

	if _, err := registrations.FindRegistrationNumberHolders(t.Context(), pcTenant(t, "tenant-1"), layer); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务查重应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// Covers: 同号两笔并发——前一笔在事务内查过、落了册、尚未提交时，后一笔的查重等着；前一笔提交后它读得到持有方。
func TestRegistrationNumberLookupWaitsForAConcurrentHolder(t *testing.T) {
	registrations, _, transactor := newPartyIdentityRegistrations(t)
	layer := identityLayerFixture(t, "CN", "SYN-CN-REG", "SYN-CN-REG-0042")
	first := identifiedPartyFixture(t, "tenant-1", "party-first", &layer)

	locked := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- transactor.WithinTransaction(context.Background(), func(txCtx context.Context) error {
			if _, err := registrations.FindRegistrationNumberHolders(txCtx, pcTenant(t, "tenant-1"), layer); err != nil {
				return err
			}
			if _, err := registrations.SaveBusinessParty(txCtx, first); err != nil {
				return err
			}
			close(locked)
			<-release
			return nil
		})
	}()
	<-locked

	type answer struct {
		holders []ports.RegistrationNumberHolder
		at      time.Time
		err     error
	}
	secondDone := make(chan answer, 1)
	go func() {
		var got answer
		got.err = transactor.WithinTransaction(context.Background(), func(txCtx context.Context) error {
			holders, err := registrations.FindRegistrationNumberHolders(txCtx, pcTenant(t, "tenant-1"), layer)
			got.holders = holders
			return err
		})
		got.at = time.Now()
		secondDone <- got
	}()

	time.Sleep(300 * time.Millisecond)
	releasedAt := time.Now()
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first transaction: %v", err)
	}
	second := <-secondDone
	if second.err != nil {
		t.Fatalf("second transaction: %v", second.err)
	}
	if second.at.Before(releasedAt) {
		t.Fatal("后一笔的查重没有等前一笔提交")
	}
	if len(second.holders) != 1 || second.holders[0].ID != "party-first" {
		t.Fatalf("后一笔应读到前一笔：%+v", second.holders)
	}
}

// Covers: 「给参与方登号」与「拿公司主体钉这个参与方」互斥——前一笔取了钉住锁、落了公司主体、尚未提交时，后一笔查钉住
// 等着；前一笔提交后它读得到那个公司主体。
func TestPinningLookupWaitsForAConcurrentLegalEntity(t *testing.T) {
	registrations, _, transactor := newPartyIdentityRegistrations(t)
	mustSavePartyIdentity(t, transactor, func(txCtx context.Context) (ports.PartyRegistrySaveOutcome, error) {
		return registrations.SaveBusinessParty(txCtx, partyRegistrationFixture(t, "tenant-1", "party-pinned", "被钉的参与方"))
	})
	layer := identityLayerFixture(t, "CN", "SYN-CN-REG", "SYN-CN-REG-0077")
	entity, err := legalEntityRegistrationFixture(t, "tenant-1", "le-pin", "party-pinned").WithIdentityLayer(layer, nil)
	if err != nil {
		t.Fatal(err)
	}
	party := pcValue(t, domain.NewPartyID, "party-pinned")

	locked := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- transactor.WithinTransaction(context.Background(), func(txCtx context.Context) error {
			if _, err := registrations.FindLegalEntitiesPinning(txCtx, pcTenant(t, "tenant-1"), party); err != nil {
				return err
			}
			if _, err := registrations.SaveLegalEntity(txCtx, entity); err != nil {
				return err
			}
			close(locked)
			<-release
			return nil
		})
	}()
	<-locked

	type answer struct {
		entities []string
		at       time.Time
		err      error
	}
	secondDone := make(chan answer, 1)
	go func() {
		var got answer
		got.err = transactor.WithinTransaction(context.Background(), func(txCtx context.Context) error {
			entities, err := registrations.FindLegalEntitiesPinning(txCtx, pcTenant(t, "tenant-1"), party)
			got.entities = entities
			return err
		})
		got.at = time.Now()
		secondDone <- got
	}()

	time.Sleep(300 * time.Millisecond)
	releasedAt := time.Now()
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first transaction: %v", err)
	}
	second := <-secondDone
	if second.err != nil {
		t.Fatalf("second transaction: %v", second.err)
	}
	if second.at.Before(releasedAt) {
		t.Fatal("后一笔查钉住没有等前一笔提交")
	}
	if len(second.entities) != 1 || second.entities[0] != "le-pin" {
		t.Fatalf("后一笔应读到前一笔的公司主体：%v", second.entities)
	}
}
