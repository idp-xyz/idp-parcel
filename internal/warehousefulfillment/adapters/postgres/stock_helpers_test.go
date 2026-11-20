package postgres_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	whidentity "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/identity"
	adapter "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// 库存账的真库用例沿用别的册的约定：每个用例一个施加过全套迁移的全新库。租户、货主、仓与商品都是合成值。

type stockStores struct {
	ledger     *adapter.StockLedger
	goods      *adapter.Goods
	warehouses *adapter.Warehouses
	zones      *adapter.Zones
	locations  *adapter.Locations
	identities *whidentity.StockIdentities
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newStockStores(t *testing.T) stockStores {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	ledger, err := adapter.NewStockLedger(db)
	if err != nil {
		t.Fatalf("构造库存账：%v", err)
	}
	goods, err := adapter.NewGoods(db)
	if err != nil {
		t.Fatalf("构造商品登记册：%v", err)
	}
	warehouses, err := adapter.NewWarehouses(db)
	if err != nil {
		t.Fatalf("构造仓登记册：%v", err)
	}
	zones, err := adapter.NewZones(db)
	if err != nil {
		t.Fatalf("构造库区登记册：%v", err)
	}
	locations, err := adapter.NewLocations(db)
	if err != nil {
		t.Fatalf("构造库位登记册：%v", err)
	}
	identities, err := whidentity.NewStockIdentities()
	if err != nil {
		t.Fatalf("构造标识签发器：%v", err)
	}
	return stockStores{
		ledger: ledger, goods: goods, warehouses: warehouses, zones: zones, locations: locations,
		identities: identities, transactor: db.Transactor(), pool: pool,
	}
}

func (stores stockStores) poster() *application.PostStockMovementHandler {
	return application.NewPostStockMovementHandler(
		stores.ledger, stores.warehouses, stores.locations, stores.zones, stores.goods, stores.identities,
		fixedClock{at: baseInstant},
	)
}

func (stores stockStores) registrar() *application.RegisterGoodsHandler {
	return application.NewRegisterGoodsHandler(
		stores.goods, stores.warehouses, stores.ledger, presentOwnerAccounts{}, fixedClock{at: baseInstant},
	)
}

// seedShelf 落一个自营仓、一个存储库区和一个库位，记数才有处可放。
func (stores stockStores) seedShelf(t *testing.T) {
	t.Helper()
	mustSave(t, stores.transactor, stores.warehouses, revisionFixture(t, "SYN-TENANT-01", "SYN-WH-01", 1, nil))
	zone := zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-01", 1, nil)
	location := locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-A", 1, nil)
	err := stores.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		if _, saveErr := stores.zones.SaveZoneRevision(txCtx, zone); saveErr != nil {
			return saveErr
		}
		_, saveErr := stores.locations.SaveLocationRevision(txCtx, location)
		return saveErr
	})
	if err != nil {
		t.Fatalf("落库区与库位：%v", err)
	}
}

func (stores stockStores) saveGoods(t *testing.T, code, key string, edit func(*domain.GoodsContentSpec)) {
	t.Helper()
	revision := goodsRevisionFixture(t, "SYN-OWNER-01", code, 1, key, edit)
	err := stores.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		result, saveErr := stores.goods.SaveGoodsRevision(txCtx, revision)
		if saveErr != nil {
			return saveErr
		}
		if result.Outcome != ports.GoodsRevisionSaved {
			return fmt.Errorf("保存商品 %s = %s", code, result.Outcome)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("保存商品 %s：%v", code, err)
	}
}

func (stores stockStores) post(t *testing.T, request domain.StockMovementRequest) application.PostStockMovementResult {
	t.Helper()
	var result application.PostStockMovementResult
	err := stores.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var postErr error
		result, postErr = stores.poster().Post(txCtx, application.PostStockMovementCommand{
			Tenant:  valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
			Request: request,
		})
		return postErr
	})
	if err != nil {
		t.Fatalf("Post：%v", err)
	}
	return result
}

func (stores stockStores) register(t *testing.T, command application.RegisterGoodsCommand) application.RegisterGoodsResult {
	t.Helper()
	var result application.RegisterGoodsResult
	err := stores.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var registerErr error
		result, registerErr = stores.registrar().Register(txCtx, command)
		return registerErr
	})
	if err != nil {
		t.Fatalf("Register：%v", err)
	}
	return result
}

func stockCell(t *testing.T, goods string, edit func(*domain.StockCellSpec)) domain.StockCell {
	t.Helper()
	spec := domain.StockCellSpec{
		Location: valueOf(t, domain.NewLocationCode, "SYN-LOC-A"),
		Goods:    valueOf(t, domain.NewGoodsCode, goods),
		Unit:     valueOf(t, domain.NewGoodsUnitName, "件"),
		State:    domain.StockStateGood,
	}
	if edit != nil {
		edit(&spec)
	}
	cell, err := domain.NewStockCell(spec)
	if err != nil {
		t.Fatal(err)
	}
	return cell
}

func stockRequest(t *testing.T, reason domain.StockMovementReason, source string, lines ...domain.StockLineRequest) domain.StockMovementRequest {
	t.Helper()
	return domain.NewStockMovementRequest(
		valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
		valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
		reason,
		valueOf(t, domain.NewSourceFactReference, source),
		baseInstant,
		lines,
	)
}

func goodsCommand(t *testing.T, code, key string, basis domain.BasisRevision, edit func(*domain.GoodsContentSpec)) application.RegisterGoodsCommand {
	t.Helper()
	submission, err := domain.NewOwnerSubmission(
		valueOf(t, domain.NewSubmissionSource, "SYN-CHANNEL-01"), valueOf(t, domain.NewRequestKey, key))
	if err != nil {
		t.Fatal(err)
	}
	return application.RegisterGoodsCommand{
		Tenant:     valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		Owner:      valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
		Code:       valueOf(t, domain.NewGoodsCode, code),
		Basis:      basis,
		Content:    goodsContentFixture(t, edit),
		Submission: submission,
		Registrant: valueOf(t, domain.NewRegistrant, "operator-1"),
	}
}

func countStockRows(t *testing.T, pool *pgxpool.Pool, table string) int {
	t.Helper()
	var count int
	query := "SELECT count(*) FROM warehouse_fulfillment." + table
	if err := pool.QueryRow(t.Context(), query).Scan(&count); err != nil {
		t.Fatalf("数 %s：%v", table, err)
	}
	return count
}

func stockRowCounts(t *testing.T, pool *pgxpool.Pool) (int, int, int) {
	t.Helper()
	return countStockRows(t, pool, "stock_movement"), countStockRows(t, pool, "inbound_batch"), countStockRows(t, pool, "stock_movement_line")
}
