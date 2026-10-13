package main

import (
	"strings"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证 buildRegistrar 装的受理前估价采用登记（隔离合成 S；票 product-strategy-boundary/25 第 3 条）：采用行
// 与向 parcel-shipment 交的「受理前估价采用已登记」信封在同一笔里同生同灭——首登一行一封、重放不翻倍、冲突不交；账户不在册上
// 归用法格、行与信封都不落；词表外的形态在事务之前就拒。

func estimateAdoptionDocument(accountID, form, scope string) string {
	return `{"tenantId": "SYN-T1", "accountId": "` + accountID + `", "form": "` + form + `", "pricingScopeId": "` + scope + `"}`
}

func TestPreAcceptanceEstimateAdoptionHandsOffItsEnvelopeInTheSameTransaction(t *testing.T) {
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
			t.Fatalf("退出码 = %d（%s），要 %d", code, message, wantCode)
		}
		if !strings.Contains(message, wantAnswer) {
			t.Fatalf("答复 = %q，要含 %s", message, wantAnswer)
		}
	}
	adoptionRows := func() int {
		return countRows(t, pool,
			`SELECT count(*) FROM settlement_accounting.pre_acceptance_estimate_adoption WHERE tenant_id = $1`, "SYN-T1")
	}
	envelopes := func() int {
		return countRows(t, pool,
			`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE event_type = $1`,
			"settlement-accounting.pre-acceptance-estimate-adoption.registered")
	}
	const form = "PER_PARCEL_SELL_TRIAL_SUM"

	mustExecute(commandPreAcceptanceEstimate, estimateAdoptionDocument("SYN-ACCT-1", form, "SYN-SCOPE-SELL-1"), exitUsage, "还不在账户册上")
	if adoptionRows() != 0 || envelopes() != 0 {
		t.Fatalf("账户不在册上后采用行 = %d、信封 = %d，各要 0", adoptionRows(), envelopes())
	}
	mustExecute(commandPreAcceptanceEstimate, estimateAdoptionDocument("SYN-ACCT-1", "PER_PARCEL_MARKUP", "SYN-SCOPE-SELL-1"), exitUsage, "译装被拒")

	mustExecute(commandSettlementAccount, settlementAccountDocument("SYN-ACCT-1", "SYN-CP-1", "SYN-CONTRACT-1"), exitRegistered, "已登记")

	mustExecute(commandPreAcceptanceEstimate, estimateAdoptionDocument("SYN-ACCT-1", form, "SYN-SCOPE-SELL-1"), exitRegistered, "已登记")
	if adoptionRows() != 1 || envelopes() != 1 {
		t.Fatalf("首登后采用行 = %d、信封 = %d，各要 1", adoptionRows(), envelopes())
	}
	eventID := string(outboxintent.FingerprintEventID("pre-acceptance-estimate-adoption", "SYN-T1", "SYN-ACCT-1"))
	if n := countRows(t, pool,
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE event_id = $1 AND partition_key = $2`,
		eventID, "SYN-T1"); n != 1 {
		t.Fatalf("按（租户 / 账户）重算的信封 ID 与租户分区读到 %d 封，要 1", n)
	}

	mustExecute(commandPreAcceptanceEstimate, estimateAdoptionDocument("SYN-ACCT-1", form, "SYN-SCOPE-SELL-1"), exitRegistered, "已存在")
	if adoptionRows() != 1 || envelopes() != 1 {
		t.Fatalf("重放后采用行 = %d、信封 = %d，各要仍是 1", adoptionRows(), envelopes())
	}
	mustExecute(commandPreAcceptanceEstimate, estimateAdoptionDocument("SYN-ACCT-1", form, "SYN-SCOPE-SELL-2"), exitConflict, "冲突")
	if adoptionRows() != 1 || envelopes() != 1 {
		t.Fatalf("冲突后采用行 = %d、信封 = %d，各要仍是 1", adoptionRows(), envelopes())
	}
}
