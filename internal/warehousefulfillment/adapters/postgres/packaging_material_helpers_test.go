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

// 包材这一册的真库用例沿用别的登记册的约定：pgtest.Pool 给每个用例一个施加过全套迁移计划的全新库，租户与包材编码一律是合成值。

// packagingMaterialStores 是一个用例要用的全部：包材写口、包材读口、仓写口（核「本租户已登记过仓」与造仓用）、事务器与连接池。
type packagingMaterialStores struct {
	packagingMaterials *adapter.PackagingMaterials
	catalogue          *adapter.PackagingMaterialCatalogue
	warehouses         *adapter.Warehouses
	transactor         bentoapp.Transactor
	pool               *pgxpool.Pool
}

func newPackagingMaterialStores(t *testing.T) packagingMaterialStores {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	packagingMaterials, err := adapter.NewPackagingMaterials(db)
	if err != nil {
		t.Fatalf("构造包材登记册：%v", err)
	}
	catalogue, err := adapter.NewPackagingMaterialCatalogue(db)
	if err != nil {
		t.Fatalf("构造包材读口：%v", err)
	}
	warehouses, err := adapter.NewWarehouses(db)
	if err != nil {
		t.Fatalf("构造仓登记册：%v", err)
	}
	return packagingMaterialStores{
		packagingMaterials: packagingMaterials, catalogue: catalogue, warehouses: warehouses, transactor: db.Transactor(), pool: pool,
	}
}

func measureValue(t *testing.T, text string) domain.Measure {
	t.Helper()
	return valueOf(t, domain.NewMeasure, text)
}

func packagingMaterialContentWith(t *testing.T, edit func(*domain.PackagingMaterialContentSpec)) domain.PackagingMaterialContent {
	t.Helper()
	spec := domain.PackagingMaterialContentSpec{
		Name:           valueOf(t, domain.NewPackagingMaterialName, "合成一号纸箱"),
		Length:         measureValue(t, "30"),
		Width:          measureValue(t, "20"),
		Height:         measureValue(t, "15.5"),
		DimensionsUnit: valueOf(t, domain.NewUnitCode, "CM"),
		Weight:         measureValue(t, "0.25"),
		WeightUnit:     valueOf(t, domain.NewUnitCode, "KG"),
	}
	if edit != nil {
		edit(&spec)
	}
	content, err := domain.NewPackagingMaterialContent(spec)
	if err != nil {
		t.Fatalf("NewPackagingMaterialContent: %v", err)
	}
	return content
}

// packagingMaterialFixture 造一笔包材修订：登记责任方 operator-1，登记时刻随修订号往后推一分钟。
func packagingMaterialFixture(
	t *testing.T,
	tenant, code string,
	revision int,
	edit func(*domain.PackagingMaterialContentSpec),
) domain.PackagingMaterialRevision {
	t.Helper()
	built, err := domain.NewPackagingMaterialRevision(
		valueOf(t, domain.NewTenantID, tenant),
		valueOf(t, domain.NewPackagingMaterialCode, code),
		revision,
		packagingMaterialContentWith(t, edit),
		valueOf(t, domain.NewRegistrant, "operator-1"),
		baseInstant.Add(time.Duration(revision)*time.Minute),
	)
	if err != nil {
		t.Fatalf("NewPackagingMaterialRevision: %v", err)
	}
	return built
}

// packagingMaterialRegisteredBy 把同一份内容换一个登记责任方与登记时刻再造一笔：重放的样子。
func packagingMaterialRegisteredBy(t *testing.T, base domain.PackagingMaterialRevision, registrant string, at time.Time) domain.PackagingMaterialRevision {
	t.Helper()
	built, err := domain.NewPackagingMaterialRevision(
		base.Tenant(), base.Code(), base.Revision(), base.Content(), valueOf(t, domain.NewRegistrant, registrant), at)
	if err != nil {
		t.Fatalf("NewPackagingMaterialRevision: %v", err)
	}
	return built
}

// savePackagingMaterial 在自己的事务里保存一笔包材修订。断言在事务回调之外：t.Fatal 走 Goexit，会让回调永不返回、事务悬在半空。
func savePackagingMaterial(t *testing.T, stores packagingMaterialStores, revision domain.PackagingMaterialRevision) ports.RegistrySaveOutcome {
	t.Helper()
	var outcome ports.RegistrySaveOutcome
	err := stores.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var saveErr error
		outcome, saveErr = stores.packagingMaterials.SavePackagingMaterialRevision(txCtx, revision)
		return saveErr
	})
	if err != nil {
		t.Fatalf("保存包材 %s 修订 %d：%v", revision.Code(), revision.Revision(), err)
	}
	return outcome
}

func mustSavePackagingMaterials(t *testing.T, stores packagingMaterialStores, revisions ...domain.PackagingMaterialRevision) {
	t.Helper()
	for _, revision := range revisions {
		if outcome := savePackagingMaterial(t, stores, revision); outcome != ports.RegistryRevisionSaved {
			t.Fatalf("保存包材 %s 修订 %d = %s，want SAVED", revision.Code(), revision.Revision(), outcome)
		}
	}
}

func packagingMaterialFirstPage(t *testing.T, values url.Values) cataloguepage.Query {
	t.Helper()
	query, err := ports.PackagingMaterialPages.Decode(values)
	if err != nil {
		t.Fatalf("解出查询 %v：%v", values, err)
	}
	return query
}

// packagingMaterialCodesOf 把读面的行摘成包材编码，列表的次序断言用它。
func packagingMaterialCodesOf(rows []ports.PackagingMaterialRow) []string {
	codes := make([]string, len(rows))
	for index, row := range rows {
		codes[index] = row.PackagingMaterialCode
	}
	return codes
}

func countPackagingMaterialRows(t *testing.T, stores packagingMaterialStores) int {
	t.Helper()
	var rows int
	if err := stores.pool.QueryRow(t.Context(), `SELECT count(*) FROM warehouse_fulfillment.packaging_material_revision`).Scan(&rows); err != nil {
		t.Fatalf("数册里的行：%v", err)
	}
	return rows
}
