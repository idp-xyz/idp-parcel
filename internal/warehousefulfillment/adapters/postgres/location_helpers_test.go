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

// 库位这一册的真库用例沿用别的登记册的约定：pgtest.Pool 给每个用例一个施加过全套迁移计划的全新库，租户、仓、库区、库位类型与库位编码
// 一律是合成值。

// locationStores 是一个用例要用的全部：库位写口、库位读口、仓、库区与库位类型的写口（核所指的对象与造它们用）、事务器与连接池。
type locationStores struct {
	locations     *adapter.Locations
	catalogue     *adapter.LocationCatalogue
	warehouses    *adapter.Warehouses
	zones         *adapter.Zones
	locationTypes *adapter.LocationTypes
	transactor    bentoapp.Transactor
	pool          *pgxpool.Pool
}

func newLocationStores(t *testing.T) locationStores {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	locations, err := adapter.NewLocations(db)
	if err != nil {
		t.Fatalf("构造库位登记册：%v", err)
	}
	catalogue, err := adapter.NewLocationCatalogue(db)
	if err != nil {
		t.Fatalf("构造库位读口：%v", err)
	}
	warehouses, err := adapter.NewWarehouses(db)
	if err != nil {
		t.Fatalf("构造仓登记册：%v", err)
	}
	zones, err := adapter.NewZones(db)
	if err != nil {
		t.Fatalf("构造库区登记册：%v", err)
	}
	locationTypes, err := adapter.NewLocationTypes(db)
	if err != nil {
		t.Fatalf("构造库位类型登记册：%v", err)
	}
	return locationStores{
		locations: locations, catalogue: catalogue, warehouses: warehouses, zones: zones, locationTypes: locationTypes,
		transactor: db.Transactor(), pool: pool,
	}
}

func locationContentWith(t *testing.T, edit func(*domain.LocationContentSpec)) domain.LocationContent {
	t.Helper()
	spec := domain.LocationContentSpec{ZoneCode: valueOf(t, domain.NewZoneCode, "SYN-ZONE-01")}
	if edit != nil {
		edit(&spec)
	}
	content, err := domain.NewLocationContent(spec)
	if err != nil {
		t.Fatalf("NewLocationContent: %v", err)
	}
	return content
}

// 下面几个是内容的写法：用例只改它关心的格，缺省是只带所属库区 SYN-ZONE-01、不带库位类型与优先级、不停用。

func inZoneOf(t *testing.T, code string) func(*domain.LocationContentSpec) {
	return func(spec *domain.LocationContentSpec) { spec.ZoneCode = valueOf(t, domain.NewZoneCode, code) }
}

func typedAs(t *testing.T, code string) func(*domain.LocationContentSpec) {
	return func(spec *domain.LocationContentSpec) {
		spec.LocationType = valueOf(t, domain.NewLocationTypeCode, code)
	}
}

func prioritisedAs(value int32) func(*domain.LocationContentSpec) {
	return func(spec *domain.LocationContentSpec) { spec.Priority = domain.NewLocationPriority(value) }
}

func deactivatedLocation(spec *domain.LocationContentSpec) { spec.Deactivated = true }

func combined(edits ...func(*domain.LocationContentSpec)) func(*domain.LocationContentSpec) {
	return func(spec *domain.LocationContentSpec) {
		for _, edit := range edits {
			edit(spec)
		}
	}
}

// locationFixture 造一笔库位修订：登记责任方 operator-1，登记时刻随修订号往后推一分钟。
func locationFixture(
	t *testing.T,
	tenant, warehouse, code string,
	revision int,
	edit func(*domain.LocationContentSpec),
) domain.LocationRevision {
	t.Helper()
	built, err := domain.NewLocationRevision(
		valueOf(t, domain.NewTenantID, tenant),
		valueOf(t, domain.NewWarehouseCode, warehouse),
		valueOf(t, domain.NewLocationCode, code),
		revision,
		locationContentWith(t, edit),
		valueOf(t, domain.NewRegistrant, "operator-1"),
		baseInstant.Add(time.Duration(revision)*time.Minute),
	)
	if err != nil {
		t.Fatalf("NewLocationRevision: %v", err)
	}
	return built
}

// locationRegisteredBy 把同一份内容换一个登记责任方与登记时刻再造一笔：重放的样子。
func locationRegisteredBy(t *testing.T, base domain.LocationRevision, registrant string, at time.Time) domain.LocationRevision {
	t.Helper()
	built, err := domain.NewLocationRevision(
		base.Tenant(), base.Warehouse(), base.Code(), base.Revision(), base.Content(),
		valueOf(t, domain.NewRegistrant, registrant), at)
	if err != nil {
		t.Fatalf("NewLocationRevision: %v", err)
	}
	return built
}

// saveLocation 在自己的事务里保存一笔库位修订。断言在事务回调之外：t.Fatal 走 Goexit，会让回调永不返回、事务悬在半空。
func saveLocation(t *testing.T, stores locationStores, revision domain.LocationRevision) ports.RegistrySaveOutcome {
	t.Helper()
	var outcome ports.RegistrySaveOutcome
	err := stores.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var saveErr error
		outcome, saveErr = stores.locations.SaveLocationRevision(txCtx, revision)
		return saveErr
	})
	if err != nil {
		t.Fatalf("保存库位 %s 修订 %d：%v", revision.Code(), revision.Revision(), err)
	}
	return outcome
}

func mustSaveLocations(t *testing.T, stores locationStores, revisions ...domain.LocationRevision) {
	t.Helper()
	for _, revision := range revisions {
		if outcome := saveLocation(t, stores, revision); outcome != ports.RegistryRevisionSaved {
			t.Fatalf("保存库位 %s 修订 %d = %s，want SAVED", revision.Code(), revision.Revision(), outcome)
		}
	}
}

// 下面这些方法造登记用例要核的别的册——仓、库区与库位类型，各在自己的事务里落册。

func (stores locationStores) mustSaveWarehouse(t *testing.T, tenant, code string, revision int, deactivated bool) {
	t.Helper()
	mustSave(t, stores.transactor, stores.warehouses, revisionFixture(t, tenant, code, revision, func(spec *domain.WarehouseContentSpec) {
		spec.Deactivated = deactivated
	}))
}

func (stores locationStores) mustSaveZone(t *testing.T, tenant, warehouse, code string, deactivated bool) {
	t.Helper()
	revision := zoneFixture(t, tenant, warehouse, code, 1, func(spec *domain.ZoneContentSpec) { spec.Deactivated = deactivated })
	err := stores.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		_, saveErr := stores.zones.SaveZoneRevision(txCtx, revision)
		return saveErr
	})
	if err != nil {
		t.Fatalf("保存库区 %s：%v", code, err)
	}
}

func (stores locationStores) mustSaveLocationType(t *testing.T, tenant, code string, revision int, deactivated bool) {
	t.Helper()
	built := locationTypeFixture(t, tenant, code, revision, func(spec *domain.LocationTypeContentSpec) { spec.Deactivated = deactivated })
	err := stores.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		_, saveErr := stores.locationTypes.SaveLocationTypeRevision(txCtx, built)
		return saveErr
	})
	if err != nil {
		t.Fatalf("保存库位类型 %s 修订 %d：%v", code, revision, err)
	}
}

func locationFirstPage(t *testing.T, values url.Values) cataloguepage.Query {
	t.Helper()
	query, err := ports.LocationPages.Decode(values)
	if err != nil {
		t.Fatalf("解出查询 %v：%v", values, err)
	}
	return query
}

// locationKeysOf 把读面的行摘成「仓编码/库位编码」，列表的次序断言用它。
func locationKeysOf(rows []ports.LocationRow) []string {
	keys := make([]string, len(rows))
	for index, row := range rows {
		keys[index] = row.WarehouseCode + "/" + row.LocationCode
	}
	return keys
}

func countLocationRows(t *testing.T, stores locationStores) int {
	t.Helper()
	var rows int
	if err := stores.pool.QueryRow(t.Context(), `SELECT count(*) FROM warehouse_fulfillment.location_revision`).Scan(&rows); err != nil {
		t.Fatalf("数册里的行：%v", err)
	}
	return rows
}
