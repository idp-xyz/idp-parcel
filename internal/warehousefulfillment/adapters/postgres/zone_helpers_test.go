package postgres_test

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// 库区这一册的真库用例沿用仓那一册的约定：pgtest.Pool 给每个用例一个施加过全套迁移计划的全新库，租户、仓与库区编码一律是合成值。

// zoneStores 是一个用例要用的全部：库区写口、库区读口、仓写口（核「仓已登记」与造仓用）、事务器与连接池。
type zoneStores struct {
	zones      *adapter.Zones
	catalogue  *adapter.ZoneCatalogue
	warehouses *adapter.Warehouses
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newZoneStores(t *testing.T) zoneStores {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	zones, err := adapter.NewZones(db)
	if err != nil {
		t.Fatalf("构造库区登记册：%v", err)
	}
	catalogue, err := adapter.NewZoneCatalogue(db)
	if err != nil {
		t.Fatalf("构造库区读口：%v", err)
	}
	warehouses, err := adapter.NewWarehouses(db)
	if err != nil {
		t.Fatalf("构造仓登记册：%v", err)
	}
	return zoneStores{zones: zones, catalogue: catalogue, warehouses: warehouses, transactor: db.Transactor(), pool: pool}
}

func zoneContentWith(t *testing.T, edit func(*domain.ZoneContentSpec)) domain.ZoneContent {
	t.Helper()
	spec := domain.ZoneContentSpec{
		Name:  valueOf(t, domain.NewZoneName, "合成存储区"),
		Usage: domain.ZoneUsageStorage,
	}
	if edit != nil {
		edit(&spec)
	}
	content, err := domain.NewZoneContent(spec)
	if err != nil {
		t.Fatalf("NewZoneContent: %v", err)
	}
	return content
}

// zoneFixture 造一笔库区修订：登记责任方 operator-1，登记时刻随修订号往后推一分钟。
func zoneFixture(
	t *testing.T,
	tenant, warehouse, code string,
	revision int,
	edit func(*domain.ZoneContentSpec),
) domain.ZoneRevision {
	t.Helper()
	built, err := domain.NewZoneRevision(
		valueOf(t, domain.NewTenantID, tenant),
		valueOf(t, domain.NewWarehouseCode, warehouse),
		valueOf(t, domain.NewZoneCode, code),
		revision,
		zoneContentWith(t, edit),
		valueOf(t, domain.NewRegistrant, "operator-1"),
		baseInstant.Add(time.Duration(revision)*time.Minute),
	)
	if err != nil {
		t.Fatalf("NewZoneRevision: %v", err)
	}
	return built
}

// zoneRegisteredBy 把同一份内容换一个登记责任方与登记时刻再造一笔：重放的样子。
func zoneRegisteredBy(t *testing.T, base domain.ZoneRevision, registrant string, at time.Time) domain.ZoneRevision {
	t.Helper()
	built, err := domain.NewZoneRevision(
		base.Tenant(), base.Warehouse(), base.Code(), base.Revision(), base.Content(),
		valueOf(t, domain.NewRegistrant, registrant), at)
	if err != nil {
		t.Fatalf("NewZoneRevision: %v", err)
	}
	return built
}

// saveZone 在自己的事务里保存一笔库区修订。断言在事务回调之外：t.Fatal 走 Goexit，会让回调永不返回、事务悬在半空。
func saveZone(t *testing.T, stores zoneStores, revision domain.ZoneRevision) ports.RegistrySaveOutcome {
	t.Helper()
	var outcome ports.RegistrySaveOutcome
	err := stores.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var saveErr error
		outcome, saveErr = stores.zones.SaveZoneRevision(txCtx, revision)
		return saveErr
	})
	if err != nil {
		t.Fatalf("保存库区 %s 修订 %d：%v", revision.Code(), revision.Revision(), err)
	}
	return outcome
}

func mustSaveZones(t *testing.T, stores zoneStores, revisions ...domain.ZoneRevision) {
	t.Helper()
	for _, revision := range revisions {
		if outcome := saveZone(t, stores, revision); outcome != ports.RegistryRevisionSaved {
			t.Fatalf("保存库区 %s 修订 %d = %s，want SAVED", revision.Code(), revision.Revision(), outcome)
		}
	}
}

func zoneFirstPage(t *testing.T, values url.Values) cataloguepage.Query {
	t.Helper()
	query, err := ports.ZonePages.Decode(values)
	if err != nil {
		t.Fatalf("解出查询 %v：%v", values, err)
	}
	return query
}

// zoneKeysOf 把读面的行摘成「仓编码/库区编码」，列表的次序断言用它。
func zoneKeysOf(rows []ports.ZoneRow) []string {
	keys := make([]string, len(rows))
	for index, row := range rows {
		keys[index] = row.WarehouseCode + "/" + row.ZoneCode
	}
	return keys
}
