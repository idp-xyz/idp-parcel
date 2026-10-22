package main

import (
	"strings"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证 buildRegistrar 装的对账安排登记（ADR-0269 决定一，隔离合成 S）：账户不在册上归用法格、
// 行不落；首登、重放；换时区或截单时刻是追加新一版，不答冲突；费用项目一册只收形态。

func statementArrangementDocument(accountID, zone, cutoff string) string {
	return `{"tenantId": "SYN-T1", "accountId": "` + accountID + `", "timeZone": "` + zone + `", "cutoffMinute": ` + cutoff + `}`
}

func TestTheStatementArrangementRegistersRevisionsOnRealPostgres(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registrar, err := buildRegistrar(db)
	if err != nil {
		t.Fatalf("装配登记口：%v", err)
	}
	ctx := t.Context()
	mustExecute := func(command, raw string, wantCode int, wantAnswer string) {
		t.Helper()
		message, code := execute(ctx, command, []byte(raw), registrar)
		if code != wantCode {
			t.Fatalf("%s 退出码 = %d（%s），要 %d", command, code, message, wantCode)
		}
		if !strings.Contains(message, wantAnswer) {
			t.Fatalf("%s 答复 = %q，要含 %s", command, message, wantAnswer)
		}
	}
	rows := func() int {
		return countRows(t, pool, `SELECT count(*) FROM settlement_accounting.statement_arrangement WHERE tenant_id = $1`, "SYN-T1")
	}

	mustExecute(commandStatementArrangement, statementArrangementDocument("SYN-ACCT-1", "Asia/Shanghai", "1080"), exitUsage, "还不在账户册上")
	if rows() != 0 {
		t.Fatal("账户不在册上，对账安排仍落了")
	}
	mustExecute(commandSettlementAccount, settlementAccountDocument("SYN-ACCT-1", "SYN-CP-1", "SYN-CONTRACT-1"), exitRegistered, "已登记")
	mustExecute(commandStatementArrangement, statementArrangementDocument("SYN-ACCT-1", "Asia/Shanghai", "1080"), exitRegistered, "已登记")
	mustExecute(commandStatementArrangement, statementArrangementDocument("SYN-ACCT-1", "Asia/Shanghai", "1080"), exitRegistered, "已存在")
	mustExecute(commandStatementArrangement, statementArrangementDocument("SYN-ACCT-1", "Asia/Shanghai", "0"), exitRegistered, "已登记")
	if rows() != 2 {
		t.Fatalf("对账安排 = %d 版，要 2（换截单时刻是新一版，旧版留着）", rows())
	}

	mustExecute(commandChargeAttribution, `{"tenantId": "SYN-T1", "feeItem": "SYN-FEE-FREIGHT", "form": "CHARGE_CONFIRMED"}`, exitRegistered, "已登记")
	mustExecute(commandChargeAttribution, `{"tenantId": "SYN-T1", "feeItem": "SYN-FEE-FREIGHT", "form": "SOURCE_OCCURRED"}`, exitConflict, "冲突")
}
