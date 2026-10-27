package postgres_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	"go.idp.xyz/idp-parcel/migrations"
)

// 迁移 0003 加「一笔记账至多被冲正一次」的唯一索引。存量里若已有同一笔被冲正了不止一次，
// 就拒绝施加并在报文里列出（租户、被冲正的记账、各冲正记账），整笔迁移回滚、行原样不动，
// 不替谁挑留哪一笔冲正；冲正那笔冲正（链）与别的租户同名的被冲正记账不挡。移走多余的
// 冲正之后照常施加，施加之后第二笔冲正撞的是这条索引。
// 从空库按序施加本上下文的迁移，停在 0003 之前落存量行，再施加 0003。
// Covers: AT-CR-025（库约束半边）
func TestMigration0003RefusesToApplyOverAPostingCorrectedMoreThanOnce(t *testing.T) {
	ctx := t.Context()
	conn, err := pgx.Connect(ctx, pgtest.FreshDatabase(t))
	if err != nil {
		t.Fatalf("连空库：%v", err)
	}
	defer conn.Close(context.WithoutCancel(ctx))
	if _, err := conn.Exec(ctx, `CREATE SCHEMA collection_remittance`); err != nil {
		t.Fatalf("建 schema：%v", err)
	}
	assets, err := migrations.CollectionRemittance()
	if err != nil {
		t.Fatalf("读迁移：%v", err)
	}
	const target = "0003_agreed_offset_has_its_own_position_and_basis"
	var guard *migrations.Asset
	for index := range assets {
		if assets[index].Name == target {
			guard = &assets[index]
			break
		}
		if _, err := conn.Exec(ctx, assets[index].SQL); err != nil {
			t.Fatalf("施加 %s：%v", assets[index].Name, err)
		}
	}
	if guard == nil {
		t.Fatalf("没有找到迁移 %s", target)
	}

	for _, tenant := range []string{"tenant-a", "tenant-b"} {
		if _, err := conn.Exec(ctx, `INSERT INTO collection_remittance.subledger
			(tenant_id, customer_ref, legal_entity_ref, currency, channel_ref, custody_basis_ref, opened_at)
			VALUES ($1, 'SYN-CUST-01', 'SYN-LE-01', 'SYN-CUR-01', 'SYN-CHAN-01', 'SYN-CUSTODY-01', now())`,
			tenant); err != nil {
			t.Fatalf("开立 %s 的分户账：%v", tenant, err)
		}
	}
	insert := func(tenant, posting, from, to, kind, basis string) error {
		_, err := conn.Exec(ctx, `INSERT INTO collection_remittance.subledger_posting
			(tenant_id, posting_ref, customer_ref, legal_entity_ref, currency, channel_ref,
			 from_position, to_position, amount_minor, basis_kind, basis_ref, posted_at)
			VALUES ($1, $2, 'SYN-CUST-01', 'SYN-LE-01', 'SYN-CUR-01', 'SYN-CHAN-01', $3, $4, 100, $5, $6, now())`,
			tenant, posting, from, to, kind, basis)
		return err
	}
	const inTransit, awaiting = "IN_TRANSIT_AT_CHANNEL", "AWAITING_ALLOCATION"
	for _, row := range []struct{ tenant, posting, from, to, kind, basis string }{
		{"tenant-a", "SYN-POST-01", "EXTERNAL_SOURCE", inTransit, "COLLECTION_FACT", "SYN-FACT-01"},
		{"tenant-a", "SYN-POST-02", inTransit, awaiting, "COLLECTION_FACT", "SYN-FACT-02"},
		{"tenant-a", "SYN-POST-FIX-A", awaiting, inTransit, "CORRECTION", "SYN-POST-02"},
		{"tenant-a", "SYN-POST-FIX-B", awaiting, inTransit, "CORRECTION", "SYN-POST-02"},
		// 冲正那笔冲正：被冲正的是 FIX-A，它自己只被冲正一次，不挡。
		{"tenant-a", "SYN-POST-FIX-OF-FIX", inTransit, awaiting, "CORRECTION", "SYN-POST-FIX-A"},
		// 别的租户里同名的被冲正记账只冲正一次，不挡，也不进报文。
		{"tenant-b", "SYN-POST-02", inTransit, awaiting, "COLLECTION_FACT", "SYN-FACT-02"},
		{"tenant-b", "SYN-POST-FIX-A", awaiting, inTransit, "CORRECTION", "SYN-POST-02"},
	} {
		if err := insert(row.tenant, row.posting, row.from, row.to, row.kind, row.basis); err != nil {
			t.Fatalf("落 0003 之前的记账 %s/%s：%v", row.tenant, row.posting, err)
		}
	}

	_, err = conn.Exec(ctx, guard.SQL)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "P0001" ||
		!strings.Contains(pgErr.Message, "tenant-a/SYN-POST-02") ||
		!strings.Contains(pgErr.Message, "SYN-POST-FIX-A") ||
		!strings.Contains(pgErr.Message, "SYN-POST-FIX-B") {
		t.Fatalf("有记账被冲正了两次，0003 应以可读的错误拒绝施加并列出被冲正的记账与各冲正记账，实得：%v", err)
	}
	for _, unrelated := range []string{"tenant-b", "SYN-POST-FIX-OF-FIX", "SYN-POST-01"} {
		if strings.Contains(pgErr.Message, unrelated) {
			t.Fatalf("报文列出了不该列的 %q：%s", unrelated, pgErr.Message)
		}
	}
	var applied *string
	if err := conn.QueryRow(ctx, `SELECT to_regclass('collection_remittance.agreed_offset')::text`).Scan(&applied); err != nil ||
		applied != nil {
		t.Fatalf("拒绝施加之后 agreed_offset 表还在（%v，%v）——整笔迁移该回滚", applied, err)
	}
	var rows int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM collection_remittance.subledger_posting`).Scan(&rows); err != nil ||
		rows != 7 {
		t.Fatalf("拒绝施加之后记账 %d 笔（%v），want 7 原样不动", rows, err)
	}

	if _, err := conn.Exec(ctx, `DELETE FROM collection_remittance.subledger_posting
		WHERE tenant_id = 'tenant-a' AND posting_ref = 'SYN-POST-FIX-B'`); err != nil {
		t.Fatalf("移走多余的冲正：%v", err)
	}
	if _, err := conn.Exec(ctx, guard.SQL); err != nil {
		t.Fatalf("多余的冲正移走之后施加 %s：%v", target, err)
	}

	err = insert("tenant-a", "SYN-POST-FIX-C", awaiting, inTransit, "CORRECTION", "SYN-POST-02")
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" ||
		pgErr.ConstraintName != "subledger_posting_one_correction_per_original" {
		t.Fatalf("0003 之后库面放行了第二笔冲正：%v", err)
	}
}
