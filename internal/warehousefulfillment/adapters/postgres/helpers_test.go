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

// 本包的用例都跑在真实 PostgreSQL 上：pgtest.Pool 给每个用例一个施加过全套迁移计划的全新库，所以固定的租户与仓编码
// 不会在用例之间串。租户与仓编码一律是合成值；国家取内置目录里的真码，业务时区取时区库里的真名。

func valueOf[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}

// newWarehouseStores 交回写口、读口、事务器与连接池（直连用来读库、造非法行与观察锁）。
func newWarehouseStores(t *testing.T) (*adapter.Warehouses, *adapter.WarehouseCatalogue, bentoapp.Transactor, *pgxpool.Pool) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	warehouses, err := adapter.NewWarehouses(db)
	if err != nil {
		t.Fatalf("构造仓登记册：%v", err)
	}
	catalogue, err := adapter.NewWarehouseCatalogue(db)
	if err != nil {
		t.Fatalf("构造仓读口：%v", err)
	}
	return warehouses, catalogue, db.Transactor(), pool
}

var baseInstant = time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)

func contentSpec(t *testing.T) domain.WarehouseContentSpec {
	t.Helper()
	return domain.WarehouseContentSpec{
		Name: valueOf(t, domain.NewWarehouseName, "合成一号仓"),
		Kind: domain.WarehouseKindSelfOperated,
		Address: domain.NewWarehouseAddress(
			valueOf(t, domain.NewCountryCode, "CN"),
			valueOf(t, domain.NewAddressText, "合成路 1 号"),
		),
		BusinessTimeZone: valueOf(t, domain.NewBusinessTimeZone, "Asia/Shanghai"),
		ColocatedNode:    valueOf(t, domain.NewColocatedNode, "SYN-NODE-01"),
	}
}

func contentWith(t *testing.T, edit func(*domain.WarehouseContentSpec)) domain.WarehouseContent {
	t.Helper()
	spec := contentSpec(t)
	if edit != nil {
		edit(&spec)
	}
	content, err := domain.NewWarehouseContent(spec)
	if err != nil {
		t.Fatalf("NewWarehouseContent: %v", err)
	}
	return content
}

// revisionFixture 造一笔修订：登记责任方 operator-1，登记时刻随修订号往后推一分钟。
func revisionFixture(
	t *testing.T,
	tenant, code string,
	revision int,
	edit func(*domain.WarehouseContentSpec),
) domain.WarehouseRevision {
	t.Helper()
	built, err := domain.NewWarehouseRevision(
		valueOf(t, domain.NewTenantID, tenant),
		valueOf(t, domain.NewWarehouseCode, code),
		revision,
		contentWith(t, edit),
		valueOf(t, domain.NewRegistrant, "operator-1"),
		baseInstant.Add(time.Duration(revision)*time.Minute),
	)
	if err != nil {
		t.Fatalf("NewWarehouseRevision: %v", err)
	}
	return built
}

// registeredBy 把同一份内容换一个登记责任方与登记时刻再造一笔：重放的样子。
func registeredBy(t *testing.T, base domain.WarehouseRevision, registrant string, at time.Time) domain.WarehouseRevision {
	t.Helper()
	built, err := domain.NewWarehouseRevision(
		base.Tenant(), base.Code(), base.Revision(), base.Content(), valueOf(t, domain.NewRegistrant, registrant), at)
	if err != nil {
		t.Fatalf("NewWarehouseRevision: %v", err)
	}
	return built
}

// saveRevision 在自己的事务里保存一笔修订。断言在事务回调之外：t.Fatal 走 Goexit，会让回调永不返回、事务悬在半空。
func saveRevision(
	t *testing.T,
	transactor bentoapp.Transactor,
	warehouses *adapter.Warehouses,
	revision domain.WarehouseRevision,
) ports.WarehouseSaveOutcome {
	t.Helper()
	var outcome ports.WarehouseSaveOutcome
	err := transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var saveErr error
		outcome, saveErr = warehouses.SaveWarehouseRevision(txCtx, revision)
		return saveErr
	})
	if err != nil {
		t.Fatalf("保存 %s 修订 %d：%v", revision.Code(), revision.Revision(), err)
	}
	return outcome
}

func mustSave(
	t *testing.T,
	transactor bentoapp.Transactor,
	warehouses *adapter.Warehouses,
	revisions ...domain.WarehouseRevision,
) {
	t.Helper()
	for _, revision := range revisions {
		if outcome := saveRevision(t, transactor, warehouses, revision); outcome != ports.WarehouseRevisionSaved {
			t.Fatalf("保存 %s 修订 %d = %s，want SAVED", revision.Code(), revision.Revision(), outcome)
		}
	}
}

func firstPage(t *testing.T, values url.Values) cataloguepage.Query {
	t.Helper()
	query, err := ports.WarehousePages.Decode(values)
	if err != nil {
		t.Fatalf("解出查询 %v：%v", values, err)
	}
	return query
}

func codesOf(rows []ports.WarehouseRow) []string {
	codes := make([]string, len(rows))
	for index, row := range rows {
		codes[index] = row.WarehouseCode
	}
	return codes
}
