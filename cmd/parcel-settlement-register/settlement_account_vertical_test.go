package main

import (
	"strings"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证 buildRegistrar 装的结算账户登记（隔离合成 S；票 routing-first-cut/19）：账户行与向
// parcel-shipment 交的「结算账户已登记」信封在同一笔里同生同灭——首登一行一封、重放不翻倍、冲突不交、信封被
// 确定性拒收时行也不落且归用法格。

func settlementAccountDocument(accountID, counterparty, basis string) string {
	return `{
		"tenantId": "SYN-T1", "accountId": "` + accountID + `", "legalEntityId": "SYN-LE-1",
		"counterpartyId": "` + counterparty + `", "direction": "RECEIVABLE", "currency": "XTS",
		"settlementPolicyId": "SYN-SETTLE-1", "responsibilityBasis": "` + basis + `"
	}`
}

func TestSettlementAccountRegistrationHandsOffItsEnvelopeInTheSameTransaction(t *testing.T) {
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

	mustExecute := func(raw string, wantCode int, wantAnswer string) {
		t.Helper()
		message, code := execute(ctx, commandSettlementAccount, []byte(raw), registrar)
		if code != wantCode {
			t.Fatalf("退出码 = %d（%s），要 %d", code, message, wantCode)
		}
		if !strings.Contains(message, wantAnswer) {
			t.Fatalf("答复 = %q，要含 %s", message, wantAnswer)
		}
	}
	accountRows := func() int {
		return countRows(t, pool,
			`SELECT count(*) FROM settlement_accounting.settlement_account WHERE tenant_id = $1`, "SYN-T1")
	}
	envelopes := func() int {
		return countRows(t, pool,
			`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE event_type = $1`,
			"settlement-accounting.settlement-account.registered")
	}

	mustExecute(settlementAccountDocument("SYN-ACCT-1", "SYN-CP-1", "SYN-CONTRACT-1"), exitRegistered, "已登记")
	if accountRows() != 1 || envelopes() != 1 {
		t.Fatalf("首登后账户行 = %d、信封 = %d，各要 1", accountRows(), envelopes())
	}
	eventID := string(outboxintent.FingerprintEventID("settlement-account", "SYN-T1", "SYN-ACCT-1"))
	if n := countRows(t, pool,
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE event_id = $1 AND partition_key = $2`,
		eventID, "SYN-T1"); n != 1 {
		t.Fatalf("按（租户 / 账户）重算的信封 ID 与租户分区读到 %d 封，要 1", n)
	}

	mustExecute(settlementAccountDocument("SYN-ACCT-1", "SYN-CP-1", "SYN-CONTRACT-1"), exitRegistered, "已存在")
	if accountRows() != 1 || envelopes() != 1 {
		t.Fatalf("重放后账户行 = %d、信封 = %d，各要仍是 1", accountRows(), envelopes())
	}

	mustExecute(settlementAccountDocument("SYN-ACCT-1", "SYN-CP-1", "SYN-CONTRACT-2"), exitConflict, "冲突")
	if accountRows() != 1 || envelopes() != 1 {
		t.Fatalf("冲突后账户行 = %d、信封 = %d，各要仍是 1", accountRows(), envelopes())
	}

	// 换一个相对方：同一组固定属性挂到另一个标识上是冲突格，会在信封之前就答掉。
	overlong := "SYN-ACCT-" + strings.Repeat("x", 600)
	mustExecute(settlementAccountDocument(overlong, "SYN-CP-2", "SYN-CONTRACT-1"), exitUsage, "确定性拒收")
	if accountRows() != 1 || envelopes() != 1 {
		t.Fatalf("拒收后账户行 = %d、信封 = %d，各要仍是 1——行与信封同生同灭", accountRows(), envelopes())
	}
}
