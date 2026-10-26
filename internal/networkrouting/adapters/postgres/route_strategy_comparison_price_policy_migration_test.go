package postgres_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	"go.idp.xyz/idp-parcel/migrations"
)

// Covers: ADR-0260 决定三在路由策略版本上的那一半——比较币种换算用各段价卡绑定的汇率序列，策略版本所引的价格政策不参与
// 换算，迁移 0027 删掉那一列。登过非空值的库拒绝施加、行原样不动（删列会抹掉一份当真写下的声明，去留由登记方定）；
// 置空之后照常施加，列随之消失，比较币种一列照旧在。
// 从空库按序施加本上下文的迁移，停在 0027 之前落一行带比较价格政策的策略版本，再施加 0027。
func TestMigration0027DropsTheComparisonPricePolicyOnlyWhenNothingWasDeclared(t *testing.T) {
	ctx := t.Context()
	conn, err := pgx.Connect(ctx, pgtest.FreshDatabase(t))
	if err != nil {
		t.Fatalf("连空库：%v", err)
	}
	defer conn.Close(context.WithoutCancel(ctx))
	if _, err := conn.Exec(ctx, `CREATE SCHEMA network_routing`); err != nil {
		t.Fatalf("建 schema：%v", err)
	}
	assets, err := migrations.NetworkRouting()
	if err != nil {
		t.Fatalf("读迁移：%v", err)
	}
	const target = "0027_route_strategy_comparison_price_policy_retired"
	var retirement *migrations.Asset
	for index := range assets {
		if assets[index].Name == target {
			retirement = &assets[index]
			break
		}
		if _, err := conn.Exec(ctx, assets[index].SQL); err != nil {
			t.Fatalf("施加 %s：%v", assets[index].Name, err)
		}
	}
	if retirement == nil {
		t.Fatalf("没有找到迁移 %s", target)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO network_routing.route_strategy_version
		(tenant_id, strategy_code, version, applicable_scope, effective_from, comparison_currency, comparison_price_policy)
		VALUES ('SYN-TENANT-NR27', 'SYN-RS-NR27', 1, 'NETWORK_SERVICE', '2026-01-01T00:00:00Z', 'CNY', 'SYN-PRICE-RULE@v1')`); err != nil {
		t.Fatalf("落 0027 之前带比较价格政策的行：%v", err)
	}

	if _, err := conn.Exec(ctx, retirement.SQL); err == nil {
		t.Fatal("登过比较价格政策的库上，0027 照样删了列")
	}
	var declared string
	if err := conn.QueryRow(ctx, `SELECT comparison_price_policy FROM network_routing.route_strategy_version
		WHERE strategy_code = 'SYN-RS-NR27'`).Scan(&declared); err != nil || declared != "SYN-PRICE-RULE@v1" {
		t.Fatalf("拒绝施加之后行变了：%q %v", declared, err)
	}

	if _, err := conn.Exec(ctx, `UPDATE network_routing.route_strategy_version SET comparison_price_policy = NULL`); err != nil {
		t.Fatalf("置空：%v", err)
	}
	if _, err := conn.Exec(ctx, retirement.SQL); err != nil {
		t.Fatalf("置空之后施加 %s：%v", target, err)
	}
	columns := map[string]bool{}
	rows, err := conn.Query(ctx, `SELECT column_name FROM information_schema.columns
		WHERE table_schema = 'network_routing' AND table_name = 'route_strategy_version'`)
	if err != nil {
		t.Fatalf("读列：%v", err)
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("读列：%v", err)
		}
		columns[name] = true
	}
	rows.Close()
	if columns["comparison_price_policy"] || !columns["comparison_currency"] {
		t.Fatalf("0027 之后的列 = %v；比较价格政策应已删、比较币种应仍在", columns)
	}
}
