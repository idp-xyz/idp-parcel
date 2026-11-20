package postgres_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

const quantitiesMigration = "0032_actual_measurement_quantities_must_be_positive_decimal"

// Covers: 数量约束的迁移遇到不合口径的存量行就停：加约束时失败、约束不留下、行原样不动——这是该停下来看的信号，迁移不用 NOT VALID
// 绕过，也不替人改写那一行。没有这样的行时照常施加，行一件不动，之后零被拒、合法数量仍写得进。
// 从空库按序施加本上下文的迁移，停在目标那一份之前，由用例落旧形状的行（之前重量为 0 的行写得进），再施加目标。
// 本上下文的迁移只引用自己 schema 里的对象；schema 不由迁移自己建，夹具在施加之前建一次。夹具为隔离合成，只记 `S`。
func TestTheQuantityConstraintMigrationStopsOnAnExistingRowOutsideTheRule(t *testing.T) {
	ctx := t.Context()
	conn, migration := nodeOperationsConnectionBefore(t, quantitiesMigration)

	rows := func() int {
		t.Helper()
		var count int
		if err := conn.QueryRow(ctx, `SELECT count(*) FROM node_operations.actual_measurement`).Scan(&count); err != nil {
			t.Fatalf("数册上的行：%v", err)
		}
		return count
	}
	constraintPresent := func() bool {
		t.Helper()
		var present bool
		if err := conn.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM pg_constraint
			                 WHERE conrelid = 'node_operations.actual_measurement'::regclass AND conname = $1)`,
			quantityConstraint).Scan(&present); err != nil {
			t.Fatalf("查约束 %s：%v", quantityConstraint, err)
		}
		return present
	}
	insert := func(sourceID string, quantities quantityColumns) error {
		if _, err := conn.Exec(ctx, claimSourceIdentitySQL, sourceIdentityTenant, sourceID, "MEASUREMENT"); err != nil {
			return err
		}
		_, err := conn.Exec(ctx, quantityRowSQL, sourceIdentityTenant, sourceID,
			quantities.weight, quantities.length, quantities.width, quantities.height)
		return err
	}

	if constraintPresent() {
		t.Fatalf("施加 %s 之前约束 %s 已经在了", quantitiesMigration, quantityConstraint)
	}
	if err := insert("SYN-MIG/legacy-zero", quantityColumns{weight: "0", length: "20", width: "15", height: "10"}); err != nil {
		t.Fatalf("落迁移之前重量为 0 的行：%v", err)
	}
	rowsBefore := rows()
	_, err := conn.Exec(ctx, migration.SQL)
	wantPgError(t, "存量里有重量为 0 的行", err, "23514", quantityConstraint)
	if constraintPresent() {
		t.Fatalf("拒绝施加之后约束 %s 还在，整份迁移没有回滚", quantityConstraint)
	}
	if rowsAfter := rows(); rowsAfter != rowsBefore {
		t.Fatalf("拒绝施加之后行数 %d → %d，行被动了", rowsBefore, rowsAfter)
	}

	// 清掉越界的行，换成存量里真实存在的形状：经构造器写下的行，数量合口径。
	if _, err := conn.Exec(ctx, `DELETE FROM node_operations.actual_measurement`); err != nil {
		t.Fatalf("清掉越界的行：%v", err)
	}
	if err := insert("SYN-MIG/legacy-legal", legalQuantities); err != nil {
		t.Fatalf("落迁移之前数量合法的行：%v", err)
	}
	rowsBefore = rows()
	if _, err := conn.Exec(ctx, migration.SQL); err != nil {
		t.Fatalf("存量都合口径，施加 %s：%v", quantitiesMigration, err)
	}
	if !constraintPresent() {
		t.Fatalf("施加之后约束 %s 不在", quantityConstraint)
	}
	if rowsAfter := rows(); rowsAfter != rowsBefore {
		t.Fatalf("施加之后行数 %d → %d，迁移动了行", rowsBefore, rowsAfter)
	}
	assertStoredQuantities(ctx, t, conn, "SYN-MIG/legacy-legal", legalQuantities)

	wantPgError(t, "迁移之后重量为 0", insert("SYN-MIG/after-zero", quantityColumns{weight: "0", length: "20", width: "15", height: "10"}),
		"23514", quantityConstraint)
	if err := insert("SYN-MIG/after-legal", legalQuantities); err != nil {
		t.Fatalf("迁移之后合法数量应照收：%v", err)
	}
	if got := rows(); got != rowsBefore+1 {
		t.Fatalf("迁移之后行数 = %d，want %d：被拒的那一行不该落下，合法的那一行该落下", got, rowsBefore+1)
	}
}

// assertStoredQuantities 要求册上这一行的四个数量逐字等于 want：迁移不改写已有的行。
func assertStoredQuantities(ctx context.Context, t *testing.T, conn *pgx.Conn, sourceID string, want quantityColumns) {
	t.Helper()
	var got quantityColumns
	if err := conn.QueryRow(ctx,
		`SELECT weight_value, length_value, width_value, height_value
		   FROM node_operations.actual_measurement WHERE tenant_id = $1 AND source_id = $2`,
		sourceIdentityTenant, sourceID,
	).Scan(&got.weight, &got.length, &got.width, &got.height); err != nil {
		t.Fatalf("读 %s 的数量：%v", sourceID, err)
	}
	if got != want {
		t.Fatalf("迁移之后 %s 的数量 = %+v，want %+v", sourceID, got, want)
	}
}
