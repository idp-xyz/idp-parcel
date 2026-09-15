package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 本文件对真实 PostgreSQL 16 证 buildRegistrar 装配的整条采用链（隔离合成 S；票 sa-cc/27 判据 2）：两条命令各自
// 贯通「译装 → 用例 → 真库 → 真 Outbox」，且版本行与向 CC 交的采用信封在同一笔里同生同灭——首版一行一封、
// 重放不翻倍、同版本异内容冲突不覆盖、更正回指链头第二行第二封且载荷回指前版、回指非链头顺序到达零行零封
// （28 写进 Save 头注的那条「顺序未受理 / 并发已采用」不对称，在这里成断言）。sa-cc/20 评审记的「CorrectFact ×
// 真库 × 真 Outbox 零用例」由此收口。

const fundsFactPartition = "SYN-T1/funds-fact/SYN-FACT-1"

func adoptDocumentFor(version string, amountMinor string) string {
	return `{
		"tenantId": "SYN-T1", "factRef": "SYN-FACT-1", "sourceRef": "SYN-SOURCE-BANK-1",
		"payerRef": "SYN-PAYER-1", "kind": "RECEIPT_CONFIRMED", "currency": "EUR",
		"amountMinor": ` + amountMinor + `, "version": "` + version + `",
		"occurredAt": "2026-09-01T08:00:00Z"
	}`
}

func correctionDocumentFor(fact, corrects, version, amountMinor string) string {
	return `{
		"tenantId": "SYN-T1", "factRef": "` + fact + `", "corrects": "` + corrects + `",
		"version": "` + version + `", "amountMinor": ` + amountMinor + `,
		"correctedAt": "2026-09-02T08:00:00Z"
	}`
}

func countRows(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(t.Context(), query, args...).Scan(&count); err != nil {
		t.Fatalf("数行：%v", err)
	}
	return count
}

func TestSettlementRegisterVerticalOnRealPostgres(t *testing.T) {
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

	mustExecute := func(command, raw string, wantCode int, wantAnswer string) string {
		t.Helper()
		message, code := execute(ctx, command, []byte(raw), registrar)
		if code != wantCode {
			t.Fatalf("%s 退出码 = %d（%s），要 %d", command, code, message, wantCode)
		}
		if !strings.Contains(message, wantAnswer) {
			t.Fatalf("%s 答复 = %q，要含 %s", command, message, wantAnswer)
		}
		return message
	}
	identityRows := func() int {
		return countRows(t, pool,
			`SELECT count(*) FROM settlement_accounting.external_funds_fact WHERE tenant_id = $1 AND fact_id = $2`,
			"SYN-T1", "SYN-FACT-1")
	}
	versionRows := func() int {
		return countRows(t, pool,
			`SELECT count(*) FROM settlement_accounting.external_funds_fact_version WHERE tenant_id = $1 AND fact_id = $2`,
			"SYN-T1", "SYN-FACT-1")
	}
	envelopes := func() int {
		return countRows(t, pool,
			`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE partition_key = $1`, fundsFactPartition)
	}

	// 正：首版采用 → 身份行 + 版本行 + 一封，事件类型与分区主体是 sa-cc/02 钉的那个形，信封 ID 带版本维。
	message := mustExecute(commandExternalFundsFact, adoptDocumentFor("SYN-FACT-1/v1", "8000"), exitRegistered, "FUNDS_FACT_ADOPTED")
	if !strings.Contains(message, "SYN-FACT-1/v1") {
		t.Fatalf("已采用答复 %q 没带版本字面", message)
	}
	if identityRows() != 1 || versionRows() != 1 {
		t.Fatalf("首版后身份行 = %d、版本行 = %d，各要 1", identityRows(), versionRows())
	}
	if envelopes() != 1 {
		t.Fatalf("首版后信封 = %d，要 1", envelopes())
	}
	var eventType string
	if err := pool.QueryRow(ctx,
		`SELECT event_type FROM `+migrate.SchemaBento+`.outbox WHERE event_id = $1`,
		fundsFactPartition+"/SYN-FACT-1/v1",
	).Scan(&eventType); err != nil {
		t.Fatalf("按带版本维的信封 ID 读不到那一封：%v", err)
	}
	if eventType != "settlement-accounting.external-funds-fact.adopted" {
		t.Fatalf("事件类型 = %q", eventType)
	}

	// 重放同载荷 → 已存在，无第二封（同 ID 被 Outbox 认领键吞掉——「重放不交」在真库上就是这样成立的）。
	mustExecute(commandExternalFundsFact, adoptDocumentFor("SYN-FACT-1/v1", "8000"), exitRegistered, "EXISTING_FUNDS_FACT")
	if versionRows() != 1 || envelopes() != 1 {
		t.Fatalf("重放后版本行 = %d、信封 = %d，各要仍是 1", versionRows(), envelopes())
	}

	// 同版本字面换金额 → 冲突，绝不覆盖；同事实换一个首版字面也是冲突（一条事实只有一个首版）。
	mustExecute(commandExternalFundsFact, adoptDocumentFor("SYN-FACT-1/v1", "8001"), exitConflict, "FUNDS_FACT_CONFLICT")
	mustExecute(commandExternalFundsFact, adoptDocumentFor("SYN-FACT-1/v1b", "8000"), exitConflict, "FUNDS_FACT_CONFLICT")
	if versionRows() != 1 || envelopes() != 1 {
		t.Fatalf("冲突后版本行 = %d、信封 = %d，各要仍是 1", versionRows(), envelopes())
	}

	// 正：更正回指链头 → 第二行 + 第二封，载荷回指前版；链头随之换成 v2、金额是更正后的。
	mustExecute(commandExternalFundsFactCorrection,
		correctionDocumentFor("SYN-FACT-1", "SYN-FACT-1/v1", "SYN-FACT-1/v2", "9000"), exitRegistered, "FUNDS_FACT_ADOPTED")
	if identityRows() != 1 || versionRows() != 2 || envelopes() != 2 {
		t.Fatalf("更正后身份行 = %d、版本行 = %d、信封 = %d，要 1 / 2 / 2", identityRows(), versionRows(), envelopes())
	}
	var rawPayload []byte
	if err := pool.QueryRow(ctx,
		`SELECT payload FROM `+migrate.SchemaBento+`.outbox WHERE event_id = $1`,
		fundsFactPartition+"/SYN-FACT-1/v2",
	).Scan(&rawPayload); err != nil {
		t.Fatalf("读更正版信封载荷：%v", err)
	}
	var payload struct {
		Corrects string `json:"corrects"`
		Version  string `json:"version"`
	}
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		t.Fatalf("解载荷：%v", err)
	}
	if payload.Version != "SYN-FACT-1/v2" || payload.Corrects != "SYN-FACT-1/v1" {
		t.Fatalf("更正版载荷 = %+v，要 version v2 回指 v1", payload)
	}
	facts, err := sapostgres.NewExternalFundsFacts(db)
	if err != nil {
		t.Fatalf("构造读口：%v", err)
	}
	tenant, err := domain.NewTenantID("SYN-T1")
	if err != nil {
		t.Fatalf("构造租户：%v", err)
	}
	factRef, err := domain.NewFundsFactReference("SYN-FACT-1")
	if err != nil {
		t.Fatalf("构造事实引用：%v", err)
	}
	head, found, err := facts.FindByKey(ctx, ports.FundsFactKey{TenantID: tenant, Fact: factRef})
	if err != nil || !found {
		t.Fatalf("读链头：found=%v err=%v", found, err)
	}
	if _, amount := head.Fact.Amount(); head.Fact.Version().String() != "SYN-FACT-1/v2" || amount != 9000 {
		t.Fatalf("链头 = %s / %d，要 v2 / 9000", head.Fact.Version(), amount)
	}

	// 反：回指非链头顺序到达 → 未受理，零行零封——铸造方不容忍乱序，与 CC 作为接收方容忍乱序是两侧各自的纪律。
	mustExecute(commandExternalFundsFactCorrection,
		correctionDocumentFor("SYN-FACT-1", "SYN-FACT-1/v1", "SYN-FACT-1/v3", "9500"), exitUsage, "SOURCE_NOT_ACCEPTED")
	// 反：更正一条未采用的事实 → 未受理，一行不落、一封不出。
	mustExecute(commandExternalFundsFactCorrection,
		correctionDocumentFor("SYN-FACT-NEVER", "SYN-FACT-NEVER/v1", "SYN-FACT-NEVER/v2", "100"), exitUsage, "SOURCE_NOT_ACCEPTED")
	if versionRows() != 2 || envelopes() != 2 {
		t.Fatalf("未受理后版本行 = %d、信封 = %d，各要仍是 2", versionRows(), envelopes())
	}
	if never := countRows(t, pool,
		`SELECT count(*) FROM settlement_accounting.external_funds_fact WHERE fact_id = $1`, "SYN-FACT-NEVER"); never != 0 {
		t.Fatalf("更正未采用的事实落了 %d 行身份", never)
	}

	// 译装拒在入库前：kind 词表外答用法格，库面一格未动。
	mustExecute(commandExternalFundsFact,
		strings.Replace(adoptDocumentFor("SYN-FACT-1/v9", "100"), `"RECEIPT_CONFIRMED"`, `"RECEIVED"`, 1), exitUsage, "译装被拒")
	if identityRows() != 1 || versionRows() != 2 {
		t.Fatalf("被拒的输入动了库面：身份行 = %d、版本行 = %d", identityRows(), versionRows())
	}
}
