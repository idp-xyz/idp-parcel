package postgres_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber/docnumberpg"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	whidentity "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/identity"
	adapter "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// 入库预报的真库用例：每个用例一个施加过全套迁移的全新库。取号编排照可见性异常的测试现装，类型声明只取这一类。

var preAdviceNumberingInstant = time.Date(2026, 11, 19, 18, 0, 0, 0, time.UTC) // 上海 2026-11-20 02:00

type preAdviceWorld struct {
	pool       *pgxpool.Pool
	db         *bentopg.DB
	transactor bentoapp.Transactor
	preAdvices *adapter.PreAdvices
	numbers    *adapter.PreAdviceNumbers
	warehouses *adapter.Warehouses
	goods      *adapter.Goods
	identities *whidentity.PreAdviceIdentities
	registrar  *docnumber.Registrar
	clock      fixedClock
}

func newPreAdviceWorld(t *testing.T, at time.Time) preAdviceWorld {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	catalogue, err := docnumber.NewCatalogue(domain.DocumentNumberTypes())
	if err != nil {
		t.Fatalf("汇总类型声明：%v", err)
	}
	book, err := docnumberpg.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	sequences, err := docnumberpg.NewSequences(db)
	if err != nil {
		t.Fatal(err)
	}
	clock := fixedClock{at: at}
	assigner, err := docnumber.NewAssigner(docnumber.AssignerDeps{
		Catalogue: catalogue, Book: book, Sequences: sequences, Transactions: book, Secrets: docnumber.SecretTable{}, Clock: clock,
	})
	if err != nil {
		t.Fatal(err)
	}
	registrar, err := docnumber.NewRegistrar(docnumber.RegistrarDeps{
		Catalogue: catalogue, Book: book, Secrets: docnumber.SecretTable{}, Clock: clock,
	})
	if err != nil {
		t.Fatal(err)
	}
	preAdvices, err := adapter.NewPreAdvices(db)
	if err != nil {
		t.Fatal(err)
	}
	numbers, err := adapter.NewPreAdviceNumbers(db, assigner)
	if err != nil {
		t.Fatal(err)
	}
	warehouses, err := adapter.NewWarehouses(db)
	if err != nil {
		t.Fatal(err)
	}
	goods, err := adapter.NewGoods(db)
	if err != nil {
		t.Fatal(err)
	}
	identities, err := whidentity.NewPreAdviceIdentities()
	if err != nil {
		t.Fatal(err)
	}
	return preAdviceWorld{
		pool: pool, db: db, transactor: db.Transactor(), preAdvices: preAdvices, numbers: numbers,
		warehouses: warehouses, goods: goods, identities: identities, registrar: registrar, clock: clock,
	}
}

func (world preAdviceWorld) handler() *application.AcceptPreAdviceHandler {
	return application.NewAcceptPreAdviceHandler(
		world.preAdvices, world.warehouses, world.warehouses, world.goods, world.numbers, world.identities,
		presentOwnerAccounts{}, world.clock,
	)
}

func (world preAdviceWorld) seedWarehouse(t *testing.T, edit func(*domain.WarehouseContentSpec)) {
	t.Helper()
	mustSave(t, world.transactor, world.warehouses, revisionFixture(t, "SYN-TENANT-01", "SYN-WH-01", 1, edit))
}

func (world preAdviceWorld) seedGoods(t *testing.T) {
	t.Helper()
	revision := goodsRevisionFixture(t, "SYN-OWNER-01", "SYN-GOODS-01", 1, "REQ-GOODS-1", nil)
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		saved, err := world.goods.SaveGoodsRevision(ctx, revision)
		if err != nil {
			return err
		}
		if saved.Outcome != ports.GoodsRevisionSaved {
			return fmt.Errorf("保存商品 %s", saved.Outcome)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
