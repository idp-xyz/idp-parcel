package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/migrations"
)

// 本文件对真实 PostgreSQL 16 证 ADR-0358 的持久化半边：供应商随成本落库读回、旧行读作未记、读口带出供应商；补记只写仍为空的
// 那一格、只动点名的租户；迁移 0055 施加在有成本行的库上。

type supplierCostStack struct {
	costs     *adapter.SupplierExpectedCosts
	catalogue *adapter.ChargeCatalogue
	db        *bentopg.DB
}

func newSupplierCostStack(t *testing.T) supplierCostStack {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	costs, err := adapter.NewSupplierExpectedCosts(db)
	if err != nil {
		t.Fatal(err)
	}
	catalogue, err := adapter.NewChargeCatalogue(db)
	if err != nil {
		t.Fatal(err)
	}
	return supplierCostStack{costs: costs, catalogue: catalogue, db: db}
}

// insertLegacyCost 照本记录之前的样子直写一行：没有供应商一格。
func insertLegacyCost(t *testing.T, stack supplierCostStack, tenant, version, agreement string) {
	t.Helper()
	saWithin(t, stack.db.Transactor(), t.Context(), func(ctx context.Context) error {
		executor, err := stack.db.RequireExecutor(ctx)
		if err != nil {
			return err
		}
		_, err = executor.Exec(ctx, `
			INSERT INTO settlement_accounting.supplier_expected_cost
				(tenant_id, version, occurrence_id, occurrence_reason, occurrence_version, occurred_at, fee_item, purchase_rule_version,
				 agreement_ref, evaluation_ref, original_currency, original_minor, settlement_currency, settlement_minor, recorded_at)
			VALUES ($1, $2, 'occurrence-'||$2, 'ACTUAL_FULFILLMENT', 'V1', $4, 'LINEHAUL', 'purchase-rule-1',
			        $3, 'buy-eval-'||$2, 'USD', 4200, 'USD', 4200, $4)`,
			tenant, version, agreement, expectedCostOccurredAt)
		return err
	})
}

// Covers: `AT-SA-236`、`AT-SA-237`（真库半边：存取与读口）——形成的成本连供应商落库、按版本读回带着它；旧行读回读作未记；预期成本册
// 每行带供应商，旧行这一格为空。
func TestTheExpectedCostSupplierIsStoredAndLegacyRowsReadAsUnrecorded(t *testing.T) {
	stack := newSupplierCostStack(t)
	ctx := t.Context()
	tenant := saTenant(t, "tenant-1")
	formed := formedExpectedCost(t, "cost-v1", "occurrence-1")
	mustSaveExpectedCost(t, stack.db.Transactor(), stack.costs, tenant, formed)
	insertLegacyCost(t, stack, "tenant-1", "cost-legacy", "agreement-1")

	loaded, found, err := stack.costs.LoadExpectedCost(ctx, tenant, formed.Version())
	if supplier, recorded := loaded.Supplier(); err != nil || !found || !recorded || supplier.String() != "supplier-1" {
		t.Fatalf("读回供应商 = %q recorded=%v found=%v err=%v", supplier, recorded, found, err)
	}
	legacy, found, err := stack.costs.LoadExpectedCost(ctx, tenant, saValue(t, domain.NewSupplierCostVersionID, "cost-legacy"))
	if supplier, recorded := legacy.Supplier(); err != nil || !found || recorded || supplier.String() != "" {
		t.Fatalf("旧行供应商 = %q recorded=%v found=%v err=%v，want 未记", supplier, recorded, found, err)
	}

	rows, err := stack.catalogue.ListSupplierExpectedCosts(ctx, tenant, 10)
	if err != nil || len(rows) != 2 {
		t.Fatalf("读口 %d 行 err=%v", len(rows), err)
	}
	bySupplier := map[string]string{}
	for _, row := range rows {
		bySupplier[row.Version] = row.Supplier
	}
	if bySupplier["cost-v1"] != "supplier-1" || bySupplier["cost-legacy"] != "" {
		t.Fatalf("读口供应商 = %v", bySupplier)
	}
}

// Covers: `AT-SA-237`（真库半边：补记）——只列点名租户里没记供应商的；补记只写仍为空的那一格，第二次写同一版答没写下、不改已记的
// （换一家也不改）；别的租户的旧行不动；形成时就记了的那一版不在列表里。
func TestBackfillRecordsOnlyEmptySupplierCellsOfTheNamedTenant(t *testing.T) {
	stack := newSupplierCostStack(t)
	ctx := t.Context()
	tenant := saTenant(t, "tenant-1")
	mustSaveExpectedCost(t, stack.db.Transactor(), stack.costs, tenant, formedExpectedCost(t, "cost-v1", "occurrence-1"))
	insertLegacyCost(t, stack, "tenant-1", "cost-legacy-a", "agreement-1")
	insertLegacyCost(t, stack, "tenant-1", "cost-legacy-b", "agreement-2")
	insertLegacyCost(t, stack, "tenant-2", "cost-legacy-a", "agreement-1")

	awaiting, err := stack.costs.ListExpectedCostsAwaitingSupplier(ctx, tenant)
	if err != nil || len(awaiting) != 2 || awaiting[0].Version.String() != "cost-legacy-a" || awaiting[0].Agreement.String() != "agreement-1" ||
		awaiting[1].Version.String() != "cost-legacy-b" {
		t.Fatalf("待补记 = %+v err=%v", awaiting, err)
	}

	record := func(version, supplier string) bool {
		t.Helper()
		var recorded bool
		saWithin(t, stack.db.Transactor(), ctx, func(txCtx context.Context) error {
			var err error
			recorded, err = stack.costs.RecordExpectedCostSupplier(txCtx, tenant,
				saValue(t, domain.NewSupplierCostVersionID, version), saValue(t, domain.NewSupplierPartyReference, supplier))
			return err
		})
		return recorded
	}
	if !record("cost-legacy-a", "supplier-1") {
		t.Fatal("空着的那一格没写下")
	}
	if record("cost-legacy-a", "supplier-9") {
		t.Fatal("已记的一格又被写了")
	}
	if record("cost-v1", "supplier-9") {
		t.Fatal("形成时就记了的那一版被补记改写")
	}
	if record("cost-missing", "supplier-1") {
		t.Fatal("不在册的版本答写下了")
	}
	loaded, _, _ := stack.costs.LoadExpectedCost(ctx, tenant, saValue(t, domain.NewSupplierCostVersionID, "cost-legacy-a"))
	if supplier, _ := loaded.Supplier(); supplier.String() != "supplier-1" {
		t.Fatalf("补记之后读回 = %q", supplier)
	}
	formed, _, _ := stack.costs.LoadExpectedCost(ctx, tenant, saValue(t, domain.NewSupplierCostVersionID, "cost-v1"))
	if supplier, _ := formed.Supplier(); supplier.String() != "supplier-1" {
		t.Fatalf("形成时记的供应商被改成 %q", supplier)
	}
	other, _, _ := stack.costs.LoadExpectedCost(ctx, saTenant(t, "tenant-2"), saValue(t, domain.NewSupplierCostVersionID, "cost-legacy-a"))
	if _, recorded := other.Supplier(); recorded {
		t.Fatal("补记 tenant-1 动了 tenant-2 的同号旧行")
	}
	if left, err := stack.costs.ListExpectedCostsAwaitingSupplier(ctx, tenant); err != nil || len(left) != 1 || left[0].Version.String() != "cost-legacy-b" {
		t.Fatalf("补记之后待补记 = %+v err=%v", left, err)
	}
	if _, err := stack.costs.RecordExpectedCostSupplier(ctx, tenant, saValue(t, domain.NewSupplierCostVersionID, "cost-legacy-b"),
		saValue(t, domain.NewSupplierPartyReference, "supplier-1")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外补记 err = %v, want ErrTransactionRequired", err)
	}
}

// Covers: 迁移 settlement_accounting/0055 施加在有成本行的库上——旧行保留、供应商一格为空；此后空白供应商写不进。
func TestTheSupplierMigrationAppliesOnARegisterWithRows(t *testing.T) {
	ctx := t.Context()
	conn, err := pgx.Connect(ctx, pgtest.FreshDatabase(t))
	if err != nil {
		t.Fatalf("连空库：%v", err)
	}
	defer conn.Close(context.WithoutCancel(ctx))
	if _, err := conn.Exec(ctx, `CREATE SCHEMA settlement_accounting`); err != nil {
		t.Fatalf("建 schema：%v", err)
	}
	assets, err := migrations.SettlementAccounting()
	if err != nil {
		t.Fatalf("读迁移：%v", err)
	}
	const target = "0055_supplier_expected_cost_supplier"
	var supplierColumn *migrations.Asset
	for index := range assets {
		if assets[index].Name == target {
			supplierColumn = &assets[index]
			break
		}
		if _, err := conn.Exec(ctx, assets[index].SQL); err != nil {
			t.Fatalf("施加 %s：%v", assets[index].Name, err)
		}
	}
	if supplierColumn == nil {
		t.Fatalf("没有找到迁移 %s", target)
	}
	const legacyRow = `
		INSERT INTO settlement_accounting.supplier_expected_cost
			(tenant_id, version, occurrence_id, occurrence_reason, occurrence_version, occurred_at, fee_item, purchase_rule_version,
			 agreement_ref, evaluation_ref, original_currency, original_minor, settlement_currency, settlement_minor, recorded_at)
		VALUES ('SYN-TENANT-01', $1, 'SYN-OCC-1', 'ACTUAL_FULFILLMENT', 'V1', now(), 'SYN-FEE-1', 'SYN-CARD/v1',
		        'SYN-SUPPLIER-TRUNK-CN-SG-01/v1', 'SYN-EVAL-1', 'CNY', 8500, 'CNY', 8500, now())`
	if _, err := conn.Exec(ctx, legacyRow, "SYN-COST-OLD"); err != nil {
		t.Fatalf("落旧形状的行：%v", err)
	}
	if _, err := conn.Exec(ctx, supplierColumn.SQL); err != nil {
		t.Fatalf("施加 %s：%v", target, err)
	}
	var supplier *string
	if err := conn.QueryRow(ctx, `SELECT supplier_party_ref FROM settlement_accounting.supplier_expected_cost WHERE version = 'SYN-COST-OLD'`).
		Scan(&supplier); err != nil || supplier != nil {
		t.Fatalf("旧行供应商 = %v err=%v，want 空", supplier, err)
	}
	if _, err := conn.Exec(ctx, `UPDATE settlement_accounting.supplier_expected_cost SET supplier_party_ref = '  ' WHERE version = 'SYN-COST-OLD'`); err == nil {
		t.Fatal("空白供应商写进去了")
	}
}
