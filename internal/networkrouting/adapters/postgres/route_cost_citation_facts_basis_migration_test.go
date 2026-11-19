package postgres_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	"go.idp.xyz/idp-parcel/migrations"
)

// Covers: 逐段成本出处的包裹事实依据在库里收封闭集（迁移 0036）。任何一处落出处的 jsonb 列里要是真有封闭集之外的字面（这里取
// 已退役的 PHYSICAL_WITH_DECLARATION），加约束时失败、整份迁移回滚——别处的约束也不留下，行原样不动；这是该停下来看的信号，
// 迁移不用 NOT VALID 绕过，也不替人改写那些行。没有这样的行（缺席即客户声明的存量出处、封闭集里的字面）时照常施加，行一件不动，
// 之后越界字面被拒。
// 从空库按序施加本上下文的迁移，停在 0036 之前往被测的那一列落一行，再施加 0036；每一处落出处的列各跑一遍。
func TestMigration0036ClosesTheCostCitationFactsBasisOnlyWhenNoRowCarriesAnOutsideLiteral(t *testing.T) {
	for _, column := range costCitationColumns {
		t.Run(column.name, func(t *testing.T) {
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
			const target = "0036_route_cost_citation_facts_basis_closed"
			var closing *migrations.Asset
			for index := range assets {
				if assets[index].Name == target {
					closing = &assets[index]
					break
				}
				if _, err := conn.Exec(ctx, assets[index].SQL); err != nil {
					t.Fatalf("施加 %s：%v", assets[index].Name, err)
				}
			}
			if closing == nil {
				t.Fatalf("没有找到迁移 %s", target)
			}

			constraintNames := make([]string, 0, len(costCitationColumns))
			for _, each := range costCitationColumns {
				constraintNames = append(constraintNames, each.constraint)
			}
			constraintsPresent := func() int {
				var count int
				if err := conn.QueryRow(ctx,
					`SELECT count(*) FROM pg_constraint
					  WHERE connamespace = 'network_routing'::regnamespace AND conname = ANY($1)`,
					constraintNames).Scan(&count); err != nil {
					t.Fatalf("数封闭集约束：%v", err)
				}
				return count
			}
			rowsOfTable := func() int {
				var count int
				if err := conn.QueryRow(ctx, `SELECT count(*) FROM network_routing.`+column.table).Scan(&count); err != nil {
					t.Fatalf("数 %s 的行：%v", column.table, err)
				}
				return count
			}

			retired := column.document(citationArray(costCitation(`"PHYSICAL_WITH_DECLARATION"`)))
			if err := column.insert(ctx, conn, "legacy-retired", &retired); err != nil {
				t.Fatalf("落 0036 之前带 PHYSICAL_WITH_DECLARATION 的行：%v", err)
			}
			rowsBefore := rowsOfTable()
			if _, err := conn.Exec(ctx, closing.SQL); err == nil {
				t.Fatalf("%s 里有行带 PHYSICAL_WITH_DECLARATION，0036 照样施加了", column.name)
			}
			if present := constraintsPresent(); present != 0 {
				t.Fatalf("拒绝施加之后留下了 %d 条封闭集约束，整份迁移没有回滚", present)
			}
			if rowsAfter := rowsOfTable(); rowsAfter != rowsBefore {
				t.Fatalf("拒绝施加之后 %s 的行数 %d → %d，行被动了", column.table, rowsBefore, rowsAfter)
			}

			// 清掉越界的行，换成存量里真实存在的形状：出处缺依据那一格，与封闭集里的字面。
			for _, table := range []string{"authorized_reroute_decision", "route_reassessment", "initial_route"} {
				if _, err := conn.Exec(ctx, `DELETE FROM network_routing.`+table); err != nil {
					t.Fatalf("清 %s：%v", table, err)
				}
			}
			legacy := column.document(citationArray(costCitation("")))
			declared := column.document(citationArray(costCitation(`"DECLARATION"`)))
			for label, document := range map[string]*string{"legacy-no-basis": &legacy, "legacy-declaration": &declared} {
				if err := column.insert(ctx, conn, label, document); err != nil {
					t.Fatalf("落存量形状的行 %s：%v", label, err)
				}
			}
			rowsBefore = rowsOfTable()
			if _, err := conn.Exec(ctx, closing.SQL); err != nil {
				t.Fatalf("没有越界的行，施加 %s：%v", target, err)
			}
			if present, want := constraintsPresent(), len(costCitationColumns); present != want {
				t.Fatalf("施加之后封闭集约束 %d 条，want %d", present, want)
			}
			if rowsAfter := rowsOfTable(); rowsAfter != rowsBefore {
				t.Fatalf("施加之后 %s 的行数 %d → %d，迁移动了行", column.table, rowsBefore, rowsAfter)
			}

			err = column.insert(ctx, conn, "after-closing-retired", &retired)
			requireCheckViolation(t, err, column.constraint, "0036 之后退役的字面")
			physical := column.document(citationArray(costCitation(`"PHYSICAL"`)))
			if err := column.insert(ctx, conn, "after-closing-physical", &physical); err != nil {
				t.Fatalf("0036 之后封闭集里的字面应照收：%v", err)
			}
		})
	}
}
