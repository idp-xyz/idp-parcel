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

// 迁移 0004 给真实到账加采用事实的版本列、「一条已采用事实对一条指令至多一条到账」的唯一索引，以及新登记必有版本的
// CHECK（NOT VALID：只管之后的写入）。存量里按旧口径登记的真实到账没有版本，照常留在册上；同一（证据引用，指令）下旧口径
// 登了不止一条的，唯一索引建不起来，预检以可读的错误拒绝施加并列出各条，整笔迁移回滚、行原样不动，不替谁挑留哪一条。
// 从空库按序施加本上下文的迁移，停在 0004 之前落存量行，再施加 0004。
// Covers: AT-CR-020（库约束半边）
func TestMigration0004KeepsLegacyArrivalsAndRefusesToApplyOverDoubledOnes(t *testing.T) {
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
	const target = "0004_arrival_references_an_adopted_funds_fact"
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
		for _, instruction := range []string{"SYN-INSTR-1", "SYN-INSTR-2"} {
			if _, err := conn.Exec(ctx, `INSERT INTO collection_remittance.collection_instruction
				(tenant_id, instruction_ref, parcel_ref, requirement_ref, customer_ref, legal_entity_ref,
				 currency, channel_ref, amount_minor, instructed_at)
				VALUES ($1, $2, 'SYN-PARCEL-1', 'SYN-REQ-1', 'SYN-CUST-01', 'SYN-LE-01',
				        'SYN-CUR-01', 'SYN-CHAN-01', 100, now())`, tenant, instruction); err != nil {
				t.Fatalf("登记 %s 的指令 %s：%v", tenant, instruction, err)
			}
		}
	}
	insert := func(tenant, fact, instruction, layer, evidence string) error {
		_, err := conn.Exec(ctx, `INSERT INTO collection_remittance.collection_fact
			(tenant_id, fact_ref, instruction_ref, source_layer, evidence_ref, currency, amount_minor, occurred_at)
			VALUES ($1, $2, $3, $4, $5, 'SYN-CUR-01', 100, now())`, tenant, fact, instruction, layer, evidence)
		return err
	}
	for _, row := range []struct{ tenant, fact, instruction, layer, evidence string }{
		{"tenant-a", "SYN-FACT-OK", "SYN-INSTR-1", "OPERATOR_BANK_CREDIT", "SYN-EVIDENCE-OK"},
		// 同一证据引用、同一指令下登了两条：要拦。
		{"tenant-a", "SYN-FACT-DUP-1", "SYN-INSTR-2", "OPERATOR_BANK_CREDIT", "SYN-EVIDENCE-DUP"},
		{"tenant-a", "SYN-FACT-DUP-2", "SYN-INSTR-2", "OPERATOR_BANK_CREDIT", "SYN-EVIDENCE-DUP"},
		// 同一证据引用、另一条指令：不挡。别的层级同一串证据：不挡。别的租户同引用同指令：不挡。
		{"tenant-a", "SYN-FACT-OTHER-INSTR", "SYN-INSTR-1", "OPERATOR_BANK_CREDIT", "SYN-EVIDENCE-DUP"},
		{"tenant-a", "SYN-FACT-REPORT", "SYN-INSTR-2", "CHANNEL_COLLECTION_REPORT", "SYN-EVIDENCE-DUP"},
		{"tenant-b", "SYN-FACT-DUP-1", "SYN-INSTR-2", "OPERATOR_BANK_CREDIT", "SYN-EVIDENCE-DUP"},
	} {
		if err := insert(row.tenant, row.fact, row.instruction, row.layer, row.evidence); err != nil {
			t.Fatalf("落 0004 之前的事实 %s/%s：%v", row.tenant, row.fact, err)
		}
	}

	_, err = conn.Exec(ctx, guard.SQL)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "P0001" ||
		!strings.Contains(pgErr.Message, "tenant-a/SYN-EVIDENCE-DUP/SYN-INSTR-2") ||
		!strings.Contains(pgErr.Message, "SYN-FACT-DUP-1") || !strings.Contains(pgErr.Message, "SYN-FACT-DUP-2") {
		t.Fatalf("同一证据同一指令登了两条，0004 应以可读的错误拒绝施加并列出各条，实得：%v", err)
	}
	for _, unrelated := range []string{"tenant-b", "SYN-FACT-OK", "SYN-FACT-OTHER-INSTR", "SYN-FACT-REPORT"} {
		if strings.Contains(pgErr.Message, unrelated) {
			t.Fatalf("报文列出了不该列的 %q：%s", unrelated, pgErr.Message)
		}
	}
	var column *string
	if err := conn.QueryRow(ctx, `SELECT column_name FROM information_schema.columns
		WHERE table_schema = 'collection_remittance' AND table_name = 'collection_fact' AND column_name = 'funds_fact_version'`).
		Scan(&column); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("拒绝施加之后版本列还在（%v，%v）——整笔迁移该回滚", column, err)
	}

	if _, err := conn.Exec(ctx, `DELETE FROM collection_remittance.collection_fact
		WHERE tenant_id = 'tenant-a' AND fact_ref = 'SYN-FACT-DUP-2'`); err != nil {
		t.Fatalf("移走重复的一条：%v", err)
	}
	if _, err := conn.Exec(ctx, guard.SQL); err != nil {
		t.Fatalf("重复移走之后施加 %s：%v", target, err)
	}

	// 存量行留在册上、没有版本；新登记的真实到账必须带版本，同一（证据，指令）不能再登第二条。
	var legacy, unpinned int
	if err := conn.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE funds_fact_version IS NULL)
		FROM collection_remittance.collection_fact WHERE source_layer = 'OPERATOR_BANK_CREDIT'`).Scan(&legacy, &unpinned); err != nil ||
		legacy != 4 || unpinned != 4 {
		t.Fatalf("存量真实到账 %d 条、无版本 %d 条（%v），want 4/4 原样不动", legacy, unpinned, err)
	}
	err = insert("tenant-a", "SYN-FACT-NEW", "SYN-INSTR-1", "OPERATOR_BANK_CREDIT", "SYN-EVIDENCE-NEW")
	if !errors.As(err, &pgErr) || pgErr.ConstraintName != "collection_fact_arrival_pins_funds_version" {
		t.Fatalf("0004 之后库面放行了不带版本的新真实到账：%v", err)
	}
	_, err = conn.Exec(ctx, `INSERT INTO collection_remittance.collection_fact
		(tenant_id, fact_ref, instruction_ref, source_layer, evidence_ref, currency, amount_minor, occurred_at, funds_fact_version)
		VALUES ('tenant-a', 'SYN-FACT-AGAIN', 'SYN-INSTR-2', 'OPERATOR_BANK_CREDIT', 'SYN-EVIDENCE-DUP', 'SYN-CUR-01', 100, now(), 'v1')`)
	if !errors.As(err, &pgErr) || pgErr.ConstraintName != "collection_fact_one_arrival_per_funds_and_instruction" {
		t.Fatalf("0004 之后库面放行了同一（证据，指令）的第二条到账：%v", err)
	}
}
