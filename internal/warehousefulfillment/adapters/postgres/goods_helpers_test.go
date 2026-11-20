package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// 商品这一册的真库用例沿用别的登记册的约定：每个用例一个施加过全套迁移的全新库。租户、货主与商品都是合成值。

type goodsStores struct {
	goods      *adapter.Goods
	catalogue  *adapter.GoodsCatalogue
	warehouses *adapter.Warehouses
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newGoodsStores(t *testing.T) goodsStores {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	goods, err := adapter.NewGoods(db)
	if err != nil {
		t.Fatalf("构造商品登记册：%v", err)
	}
	catalogue, err := adapter.NewGoodsCatalogue(db)
	if err != nil {
		t.Fatalf("构造商品读口：%v", err)
	}
	warehouses, err := adapter.NewWarehouses(db)
	if err != nil {
		t.Fatalf("构造仓登记册：%v", err)
	}
	return goodsStores{goods: goods, catalogue: catalogue, warehouses: warehouses, transactor: db.Transactor(), pool: pool}
}

func goodsLevelFixture(t *testing.T, name string, factor int64) domain.PackagingLevel {
	t.Helper()
	return domain.NewPackagingLevel(valueOf(t, domain.NewGoodsUnitName, name), factor)
}

func goodsContentFixture(t *testing.T, edit func(*domain.GoodsContentSpec)) domain.GoodsContent {
	t.Helper()
	weight, err := domain.NewDeclaredWeight(measureValue(t, "0.50"), valueOf(t, domain.NewUnitCode, "KG"))
	if err != nil {
		t.Fatal(err)
	}
	measurement, err := domain.NewDeclaredMeasurement(domain.DeclaredMeasurementSpec{
		Level: valueOf(t, domain.NewGoodsUnitName, "件"), Weight: &weight,
	})
	if err != nil {
		t.Fatal(err)
	}
	declaration, err := domain.NewGoodsDeclaration(domain.GoodsDeclarationSpec{
		Name: valueOf(t, domain.NewDeclaredGoodsName, " T-shirt "), UnitPrice: measureValue(t, "5.50"),
		Currency: valueOf(t, domain.NewCurrencyCode, "USD"),
	})
	if err != nil {
		t.Fatal(err)
	}
	spec := domain.GoodsContentSpec{
		Name:     valueOf(t, domain.NewGoodsName, "合成商品"),
		BaseUnit: valueOf(t, domain.NewGoodsUnitName, "件"),
		PackagingLevels: []domain.PackagingLevel{
			goodsLevelFixture(t, "件", 1), goodsLevelFixture(t, "箱", 24),
		},
		Aliases: []domain.GoodsAlias{domain.NewGoodsAlias(
			valueOf(t, domain.NewGoodsAliasCode, "SYN-BAR-01"), domain.GoodsAliasBarcode, domain.PlatformName{},
			domain.NewAliasValidity(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)),
		)},
		Declaration:          declaration,
		DeclaredMeasurements: []domain.DeclaredMeasurement{measurement},
	}
	if edit != nil {
		edit(&spec)
	}
	content, err := domain.NewGoodsContent(spec)
	if err != nil {
		t.Fatalf("NewGoodsContent: %v", err)
	}
	return content
}

func goodsRevisionFixture(t *testing.T, owner, code string, revision int, key string, edit func(*domain.GoodsContentSpec)) domain.GoodsRevision {
	t.Helper()
	submission, err := domain.NewOwnerSubmission(
		valueOf(t, domain.NewSubmissionSource, "SYN-CHANNEL-01"), valueOf(t, domain.NewRequestKey, key))
	if err != nil {
		t.Fatal(err)
	}
	built, err := domain.NewGoodsRevision(
		valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		valueOf(t, domain.NewOwnerAccountID, owner),
		valueOf(t, domain.NewGoodsCode, code),
		revision,
		goodsContentFixture(t, edit),
		submission,
		valueOf(t, domain.NewRegistrant, "operator-1"),
		baseInstant.Add(time.Duration(revision)*time.Minute),
	)
	if err != nil {
		t.Fatalf("NewGoodsRevision: %v", err)
	}
	return built
}

func saveGoods(t *testing.T, stores goodsStores, revision domain.GoodsRevision) ports.GoodsSaveResult {
	t.Helper()
	var result ports.GoodsSaveResult
	err := stores.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var saveErr error
		result, saveErr = stores.goods.SaveGoodsRevision(txCtx, revision)
		return saveErr
	})
	if err != nil {
		t.Fatalf("保存 %s 修订 %d：%v", revision.Code(), revision.Revision(), err)
	}
	return result
}
