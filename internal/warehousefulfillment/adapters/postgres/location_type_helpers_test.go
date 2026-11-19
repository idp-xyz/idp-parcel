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

// 库位类型这一册的真库用例沿用仓、库区两册的约定：pgtest.Pool 给每个用例一个施加过全套迁移计划的全新库，租户与库位类型编码一律是合成值。

// locationTypeStores 是一个用例要用的全部：库位类型写口、库位类型读口、仓写口（核「本租户已登记过仓」与造仓用）、事务器与连接池。
type locationTypeStores struct {
	locationTypes *adapter.LocationTypes
	catalogue     *adapter.LocationTypeCatalogue
	warehouses    *adapter.Warehouses
	transactor    bentoapp.Transactor
	pool          *pgxpool.Pool
}

func newLocationTypeStores(t *testing.T) locationTypeStores {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	locationTypes, err := adapter.NewLocationTypes(db)
	if err != nil {
		t.Fatalf("构造库位类型登记册：%v", err)
	}
	catalogue, err := adapter.NewLocationTypeCatalogue(db)
	if err != nil {
		t.Fatalf("构造库位类型读口：%v", err)
	}
	warehouses, err := adapter.NewWarehouses(db)
	if err != nil {
		t.Fatalf("构造仓登记册：%v", err)
	}
	return locationTypeStores{
		locationTypes: locationTypes, catalogue: catalogue, warehouses: warehouses, transactor: db.Transactor(), pool: pool,
	}
}

func locationTypeContentWith(t *testing.T, edit func(*domain.LocationTypeContentSpec)) domain.LocationTypeContent {
	t.Helper()
	spec := domain.LocationTypeContentSpec{Name: valueOf(t, domain.NewLocationTypeName, "合成货架位")}
	if edit != nil {
		edit(&spec)
	}
	content, err := domain.NewLocationTypeContent(spec)
	if err != nil {
		t.Fatalf("NewLocationTypeContent: %v", err)
	}
	return content
}

// locationTypeFixture 造一笔库位类型修订：登记责任方 operator-1，登记时刻随修订号往后推一分钟。
func locationTypeFixture(
	t *testing.T,
	tenant, code string,
	revision int,
	edit func(*domain.LocationTypeContentSpec),
) domain.LocationTypeRevision {
	t.Helper()
	built, err := domain.NewLocationTypeRevision(
		valueOf(t, domain.NewTenantID, tenant),
		valueOf(t, domain.NewLocationTypeCode, code),
		revision,
		locationTypeContentWith(t, edit),
		valueOf(t, domain.NewRegistrant, "operator-1"),
		baseInstant.Add(time.Duration(revision)*time.Minute),
	)
	if err != nil {
		t.Fatalf("NewLocationTypeRevision: %v", err)
	}
	return built
}

// locationTypeRegisteredBy 把同一份内容换一个登记责任方与登记时刻再造一笔：重放的样子。
func locationTypeRegisteredBy(t *testing.T, base domain.LocationTypeRevision, registrant string, at time.Time) domain.LocationTypeRevision {
	t.Helper()
	built, err := domain.NewLocationTypeRevision(
		base.Tenant(), base.Code(), base.Revision(), base.Content(), valueOf(t, domain.NewRegistrant, registrant), at)
	if err != nil {
		t.Fatalf("NewLocationTypeRevision: %v", err)
	}
	return built
}

// saveLocationType 在自己的事务里保存一笔库位类型修订。断言在事务回调之外：t.Fatal 走 Goexit，会让回调永不返回、事务悬在半空。
func saveLocationType(t *testing.T, stores locationTypeStores, revision domain.LocationTypeRevision) ports.RegistrySaveOutcome {
	t.Helper()
	var outcome ports.RegistrySaveOutcome
	err := stores.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var saveErr error
		outcome, saveErr = stores.locationTypes.SaveLocationTypeRevision(txCtx, revision)
		return saveErr
	})
	if err != nil {
		t.Fatalf("保存库位类型 %s 修订 %d：%v", revision.Code(), revision.Revision(), err)
	}
	return outcome
}

func mustSaveLocationTypes(t *testing.T, stores locationTypeStores, revisions ...domain.LocationTypeRevision) {
	t.Helper()
	for _, revision := range revisions {
		if outcome := saveLocationType(t, stores, revision); outcome != ports.RegistryRevisionSaved {
			t.Fatalf("保存库位类型 %s 修订 %d = %s，want SAVED", revision.Code(), revision.Revision(), outcome)
		}
	}
}

func locationTypeFirstPage(t *testing.T, values url.Values) cataloguepage.Query {
	t.Helper()
	query, err := ports.LocationTypePages.Decode(values)
	if err != nil {
		t.Fatalf("解出查询 %v：%v", values, err)
	}
	return query
}

// locationTypeCodesOf 把读面的行摘成库位类型编码，列表的次序断言用它。
func locationTypeCodesOf(rows []ports.LocationTypeRow) []string {
	codes := make([]string, len(rows))
	for index, row := range rows {
		codes[index] = row.LocationTypeCode
	}
	return codes
}

func countLocationTypeRows(t *testing.T, stores locationTypeStores) int {
	t.Helper()
	var rows int
	if err := stores.pool.QueryRow(t.Context(), `SELECT count(*) FROM warehouse_fulfillment.location_type_revision`).Scan(&rows); err != nil {
		t.Fatalf("数册里的行：%v", err)
	}
	return rows
}
