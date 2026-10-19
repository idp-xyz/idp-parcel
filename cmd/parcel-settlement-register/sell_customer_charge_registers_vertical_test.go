package main

import (
	"strings"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证 buildRegistrar 装的 ADR-0190 两本租户取值册（隔离合成 S）：SELL 计费时点采用——账户
// 不在册上归用法格、行不落；首登、重放、换时点冲突不覆盖；词表外的时点在事务之前就拒。费用项目映射——首登、重放、换
// 费用项目冲突不覆盖。两本都不交信封：没有停着等它们的东西可重驱。

func sellBillingMomentDocument(accountID, moment, scope string) string {
	return `{"tenantId": "SYN-T1", "accountId": "` + accountID + `", "billingMoment": "` + moment + `", "pricingScopeId": "` + scope + `"}`
}

func feeItemMappingDocument(code, feeItem string) string {
	return `{"tenantId": "SYN-T1", "pricingChargeCode": "` + code + `", "feeItem": "` + feeItem + `", "basisRef": "SYN-CLAUSE-1"}`
}

func TestTheSellCustomerChargeRegistersRegisterOnRealPostgres(t *testing.T) {
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
	rows := func(table string) int {
		return countRows(t, pool, `SELECT count(*) FROM settlement_accounting.`+table+` WHERE tenant_id = $1`, "SYN-T1")
	}
	outboxRows := func() int {
		return countRows(t, pool, `SELECT count(*) FROM `+migrate.SchemaBento+`.outbox`)
	}
	const intake, delivery = "NETWORK_INTAKE_ADOPTED", "EFFECTIVE_DELIVERY_FINALIZED"

	mustExecute(commandSellBillingMomentAdoption, sellBillingMomentDocument("SYN-ACCT-1", intake, "SYN-SCOPE-SELL-1"), exitUsage, "还不在账户册上")
	if rows("sell_billing_moment_adoption") != 0 {
		t.Fatal("账户不在册上，采用行仍落了")
	}
	mustExecute(commandSellBillingMomentAdoption, sellBillingMomentDocument("SYN-ACCT-1", "COMMISSION_ACCEPTED", "SYN-SCOPE-SELL-1"), exitUsage, "译装被拒")

	mustExecute(commandSettlementAccount, settlementAccountDocument("SYN-ACCT-1", "SYN-CP-1", "SYN-CONTRACT-1"), exitRegistered, "已登记")
	envelopesBefore := outboxRows()

	mustExecute(commandSellBillingMomentAdoption, sellBillingMomentDocument("SYN-ACCT-1", intake, "SYN-SCOPE-SELL-1"), exitRegistered, "已登记")
	mustExecute(commandSellBillingMomentAdoption, sellBillingMomentDocument("SYN-ACCT-1", intake, "SYN-SCOPE-SELL-1"), exitRegistered, "已存在")
	mustExecute(commandSellBillingMomentAdoption, sellBillingMomentDocument("SYN-ACCT-1", delivery, "SYN-SCOPE-SELL-1"), exitConflict, "冲突")
	if n := countRows(t, pool,
		`SELECT count(*) FROM settlement_accounting.sell_billing_moment_adoption WHERE tenant_id = $1 AND billing_moment = $2`,
		"SYN-T1", intake); n != 1 || rows("sell_billing_moment_adoption") != 1 {
		t.Fatalf("采用行 = %d（首登那一份 %d），要只有首登那一行", rows("sell_billing_moment_adoption"), n)
	}

	mustExecute(commandFeeItemMapping, feeItemMappingDocument("BASE_FREIGHT", "SYN-FEE-FREIGHT"), exitRegistered, "已登记")
	mustExecute(commandFeeItemMapping, feeItemMappingDocument("BASE_FREIGHT", "SYN-FEE-FREIGHT"), exitRegistered, "已存在")
	mustExecute(commandFeeItemMapping, feeItemMappingDocument("BASE_FREIGHT", "SYN-FEE-SURCHARGE"), exitConflict, "冲突")
	if n := countRows(t, pool,
		`SELECT count(*) FROM settlement_accounting.fee_item_mapping WHERE tenant_id = $1 AND fee_item = $2`,
		"SYN-T1", "SYN-FEE-FREIGHT"); n != 1 || rows("fee_item_mapping") != 1 {
		t.Fatalf("映射行 = %d（首登那一份 %d），要只有首登那一行", rows("fee_item_mapping"), n)
	}

	if after := outboxRows(); after != envelopesBefore {
		t.Fatalf("两本册登记后 Outbox 从 %d 封变成 %d 封，这两本不交信封", envelopesBefore, after)
	}
}
