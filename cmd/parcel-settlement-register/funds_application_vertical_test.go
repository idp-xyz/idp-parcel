package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 本文件对真实 PostgreSQL 16 证 buildRegistrar 装配的核销与核销撤销两条命令（隔离合成 S）：译装 → 用例 → 真库 → 真
// Outbox 贯通，客户赔付付款与追偿到账经资金映射落到各自那一条金额上，核销按目标各自算未核销；被指向的金额本体那一行
// 一字不动；撤销只写撤销两列、原分配原样；指向别的租户的金额与不存在的同答`未受理`；信封被框架确定性拒收整笔回滚、
// 退用法格。金额本体由 UC-SA-007 形成，这里经它的写口直接登两条合成金额，不走赔付编排。

const claimTenant = "SYN-T1"

func mustBuild[T any](t *testing.T, build func(string) (T, error), raw string) T {
	t.Helper()
	value, err := build(raw)
	if err != nil {
		t.Fatalf("构造 %q：%v", raw, err)
	}
	return value
}

// seedClaimAmounts 登一条客户赔付 5000 XTS（SYN-COMP-1，SYN-T1），一条只在 SYN-T2 的赔付（SYN-COMP-T2-ONLY），以及
// SYN-T1 名下一条应追偿 10000 与它的部分认可 6000（SYN-ACK-1）。
func seedClaimAmounts(t *testing.T, db *bentopg.DB) {
	t.Helper()
	amounts, err := sapostgres.NewCustomerClaimAmounts(db)
	if err != nil {
		t.Fatalf("构造索赔金额库：%v", err)
	}
	receivables, err := sapostgres.NewRecoveryReceivables(db)
	if err != nil {
		t.Fatalf("构造应追偿库：%v", err)
	}
	acknowledgements, err := sapostgres.NewRecoveryAcknowledgements(db)
	if err != nil {
		t.Fatalf("构造认可库：%v", err)
	}
	formedAt := time.Date(2026, 8, 20, 8, 0, 0, 0, time.UTC)
	compensation := func(tenant, id string) ports.ClaimAmountRecord {
		amount, err := domain.FormCustomerClaimAmount(domain.CustomerClaimAmountSpec{
			ID:             mustBuild(t, domain.NewCustomerClaimAmountID, id),
			Kind:           domain.CustomerCompensationPayable,
			ClaimItem:      mustBuild(t, domain.NewClaimItemReference, "SYN-CLAIM-ITEM-1"),
			Responsibility: mustBuild(t, domain.NewResponsibilityConclusionReference, "SYN-RESPONSIBILITY-1/v1"),
			RuleVersion:    mustBuild(t, domain.NewAmountRuleVersionReference, "SYN-CLAIM-RULE-1/v1"),
			LegalEntity:    mustBuild(t, domain.NewLegalEntityReference, "SYN-LE-01"),
			Currency:       mustBuild(t, domain.NewCurrencyCode, "XTS"),
			AmountMinor:    5000,
			Period:         mustBuild(t, domain.NewBillingPeriodReference, "SYN-PERIOD-2026-08"),
			FormedAt:       formedAt,
		})
		if err != nil {
			t.Fatalf("形成客户赔付：%v", err)
		}
		return ports.ClaimAmountRecord{
			Key:           ports.ClaimAmountKey{TenantID: mustBuild(t, domain.NewTenantID, tenant), Amount: amount.ID()},
			ContentDigest: "SYN-DIGEST-" + id, Amount: amount, RecordedAt: formedAt,
		}
	}
	receivable, err := domain.FormRecoveryReceivable(domain.RecoveryReceivableSpec{
		ID:             mustBuild(t, domain.NewRecoveryReceivableID, "SYN-RECOVERY-1"),
		Matter:         mustBuild(t, domain.NewRecoveryMatterReference, "SYN-RECOVERY-MATTER-1"),
		Responsibility: mustBuild(t, domain.NewResponsibilityConclusionReference, "SYN-RESPONSIBILITY-2/v1"),
		Counterparty:   mustBuild(t, domain.NewRecoveryCounterpartyReference, "SYN-PARTY-CARRIER-01"),
		RuleVersion:    mustBuild(t, domain.NewAmountRuleVersionReference, "SYN-CLAIM-RULE-1/v1"),
		LegalEntity:    mustBuild(t, domain.NewLegalEntityReference, "SYN-LE-01"),
		Currency:       mustBuild(t, domain.NewCurrencyCode, "XTS"),
		AmountMinor:    10000,
		FormedAt:       formedAt,
	})
	if err != nil {
		t.Fatalf("形成应追偿：%v", err)
	}
	acknowledgement, err := domain.AcknowledgeRecovery(receivable,
		mustBuild(t, domain.NewAcknowledgementID, "SYN-ACK-1"),
		mustBuild(t, domain.NewCounterpartyResponseReference, "SYN-RESPONSE-1/v1"),
		domain.ResponsePartiallyAccepted, 6000, formedAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("形成追偿认可：%v", err)
	}
	tenant := mustBuild(t, domain.NewTenantID, claimTenant)
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		for _, record := range []ports.ClaimAmountRecord{compensation(claimTenant, "SYN-COMP-1"), compensation("SYN-T2", "SYN-COMP-T2-ONLY")} {
			if _, err := amounts.Save(txCtx, record); err != nil {
				return err
			}
		}
		if _, err := receivables.Save(txCtx, ports.ReceivableRecord{
			Key: ports.ReceivableKey{TenantID: tenant, Receivable: receivable.ID()}, ContentDigest: "SYN-DIGEST-RECOVERY-1",
			Receivable: receivable, RecordedAt: formedAt,
		}); err != nil {
			return err
		}
		_, err := acknowledgements.Save(txCtx, ports.AcknowledgementRecord{
			Key: ports.AcknowledgementKey{TenantID: tenant, Acknowledgement: acknowledgement.ID()}, ContentDigest: "SYN-DIGEST-ACK-1",
			Acknowledgement: acknowledgement, RecordedAt: formedAt.Add(time.Hour),
		})
		return err
	}); err != nil {
		t.Fatalf("登合成金额：%v", err)
	}
}

func receiptDocument(fact string, amountMinor string) string {
	return `{
		"tenantId": "SYN-T1", "factRef": "` + fact + `", "sourceRef": "SYN-SOURCE-BANK-1",
		"kind": "RECEIPT_CONFIRMED", "currency": "XTS", "amountMinor": ` + amountMinor + `,
		"version": "` + fact + `/v1", "occurredAt": "2026-09-01T08:00:00Z"
	}`
}

func claimMappingDocument(mapping, fact, kind, target string) string {
	return `{
		"tenantId": "SYN-T1", "mappingRef": "` + mapping + `", "factRef": "` + fact + `",
		"targetKind": "` + kind + `", "targetRef": "` + target + `",
		"basis": "SYN-PAYMENT-INSTRUCTION-1", "mappedAt": "2026-09-01T09:00:00Z"
	}`
}

func applicationDocument(application, fact, mapping, kind, target, amountMinor string) string {
	return `{
		"tenantId": "SYN-T1", "applicationRef": "` + application + `", "factRef": "` + fact + `",
		"targetCurrency": "XTS", "mappingRefs": ["` + mapping + `"],
		"allocations": [{"mappingRef": "` + mapping + `", "targetKind": "` + kind + `", "targetRef": "` + target + `",
			"direction": "DEBIT", "amountMinor": ` + amountMinor + `}],
		"basis": "SYN-PAYMENT-INSTRUCTION-1", "appliedAt": "2026-09-01T10:00:00Z"
	}`
}

func reversalDocument(application string) string {
	return `{
		"tenantId": "SYN-T1", "applicationRef": "` + application + `",
		"basis": "SYN-FUNDS-RETURNED-1", "reversedAt": "2026-09-02T10:00:00Z"
	}`
}

type claimAmountRow struct {
	amount     int64
	digest     string
	recordedAt time.Time
}

func readClaimAmountRow(t *testing.T, pool *pgxpool.Pool) claimAmountRow {
	t.Helper()
	var row claimAmountRow
	if err := pool.QueryRow(t.Context(),
		`SELECT amount_minor, content_digest, recorded_at FROM settlement_accounting.customer_claim_amount
		  WHERE tenant_id = $1 AND amount_id = $2`, claimTenant, "SYN-COMP-1").Scan(&row.amount, &row.digest, &row.recordedAt); err != nil {
		t.Fatalf("直读客户赔付：%v", err)
	}
	return row
}

// Covers: `AT-SA-116`、`AT-SA-117`、`AT-SA-112`、`AT-SA-113` 与 UC-SA-007 `AT-SA-154`、`AT-SA-155` 的受控进程入口——赔付付款
// 映射到那一条客户赔付、分次核销，超出未核销的那一笔失衡不落；同键重放答已存在不翻倍、同键异内容冲突；撤销只写撤销两列、
// 原分配原样、未核销随之恢复；追偿到账映射到那一条追偿认可并部分核销；被指向的金额本体那一行自始至终一字不动。
func TestFundsApplicationCommandsRunThroughTheControlledEntryOnRealPostgres(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registrar, err := buildRegistrar(db)
	if err != nil {
		t.Fatalf("装配登记口：%v", err)
	}
	seedClaimAmounts(t, db)
	ctx := t.Context()
	run := func(command, raw string, wantCode int, wantAnswer string) string {
		t.Helper()
		message, code := execute(ctx, command, []byte(raw), registrar)
		if code != wantCode || !strings.Contains(message, wantAnswer) {
			t.Fatalf("%s 退出码 = %d、答复 = %q，要 %d 与 %s", command, code, message, wantCode, wantAnswer)
		}
		return message
	}
	applicationRows := func(application string) int {
		return countRows(t, pool,
			`SELECT count(*) FROM settlement_accounting.settlement_application WHERE tenant_id = $1 AND application_id = $2`,
			claimTenant, application)
	}
	envelopes := func(application string) int {
		return countRows(t, pool,
			`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE partition_key = $1`, claimTenant+"/application/"+application)
	}
	allocationsOf := func(application string) string {
		var allocations string
		if err := pool.QueryRow(ctx,
			`SELECT allocations::text FROM settlement_accounting.settlement_application WHERE tenant_id = $1 AND application_id = $2`,
			claimTenant, application).Scan(&allocations); err != nil {
			t.Fatalf("读分配：%v", err)
		}
		return allocations
	}
	amountBefore := readClaimAmountRow(t, pool)

	run(commandExternalFundsFact, receiptDocument("SYN-FACT-COMP-1", "8000"), exitRegistered, "FUNDS_FACT_ADOPTED")
	run(commandFundsMapping, claimMappingDocument("SYN-MAPPING-COMP-1", "SYN-FACT-COMP-1", "CUSTOMER_COMPENSATION", "SYN-COMP-1"),
		exitRegistered, "FUNDS_MAPPED")

	// 反：指向别的租户的同名赔付与指向不存在的赔付同答`未受理`，一行不落。
	run(commandFundsMapping, claimMappingDocument("SYN-MAPPING-FOREIGN", "SYN-FACT-COMP-1", "CUSTOMER_COMPENSATION", "SYN-COMP-T2-ONLY"),
		exitUsage, "SOURCE_NOT_ACCEPTED")
	run(commandFundsMapping, claimMappingDocument("SYN-MAPPING-RECOVERY-AS-COMP", "SYN-FACT-COMP-1", "CUSTOMER_COMPENSATION", "SYN-ACK-1"),
		exitUsage, "SOURCE_NOT_ACCEPTED")
	if rows := countRows(t, pool,
		`SELECT count(*) FROM settlement_accounting.funds_mapping WHERE tenant_id = $1`, claimTenant); rows != 1 {
		t.Fatalf("映射行 = %d，要只有那一条成立的", rows)
	}

	// 正：核销 3000；重放答已存在、不翻倍；同键异内容冲突。
	first := applicationDocument("SYN-APPLICATION-1", "SYN-FACT-COMP-1", "SYN-MAPPING-COMP-1", "CUSTOMER_COMPENSATION", "SYN-COMP-1", "3000")
	message := run(commandFundsApplication, first, exitRegistered, "SETTLEMENT_APPLIED")
	if !strings.Contains(message, "SYN-APPLICATION-1") {
		t.Fatalf("已核销答复 %q 没带核销引用", message)
	}
	if applicationRows("SYN-APPLICATION-1") != 1 || envelopes("SYN-APPLICATION-1") != 1 {
		t.Fatalf("核销后行 = %d、信封 = %d，各要 1", applicationRows("SYN-APPLICATION-1"), envelopes("SYN-APPLICATION-1"))
	}
	run(commandFundsApplication, first, exitRegistered, "EXISTING_APPLICATION")
	if envelopes("SYN-APPLICATION-1") != 1 {
		t.Fatalf("重放后信封 = %d，要仍是 1", envelopes("SYN-APPLICATION-1"))
	}
	run(commandFundsApplication, strings.Replace(first, `"amountMinor": 3000`, `"amountMinor": 2999`, 1), exitConflict, "APPLICATION_CONFLICT")
	originalAllocations := allocationsOf("SYN-APPLICATION-1")

	// 反：剩 2000 却核 2001——失衡，一行不落；超出的部分保持未分配。
	run(commandFundsApplication,
		applicationDocument("SYN-APPLICATION-2", "SYN-FACT-COMP-1", "SYN-MAPPING-COMP-1", "CUSTOMER_COMPENSATION", "SYN-COMP-1", "2001"),
		exitUsage, "APPLICATION_IMBALANCE")
	if applicationRows("SYN-APPLICATION-2") != 0 {
		t.Fatal("失衡的核销落了行")
	}

	// 撤销：只写撤销两列，原分配原样；重放答已撤销、不二撤；撤一笔不存在的核销答`未受理`。
	run(commandFundsApplicationReversal, reversalDocument("SYN-APPLICATION-1"), exitRegistered, "APPLICATION_REVERSED")
	if got := allocationsOf("SYN-APPLICATION-1"); got != originalAllocations {
		t.Fatalf("撤销动了原分配：%s → %s", originalAllocations, got)
	}
	var reversalBasis string
	if err := pool.QueryRow(ctx,
		`SELECT reversal_basis FROM settlement_accounting.settlement_application WHERE tenant_id = $1 AND application_id = $2`,
		claimTenant, "SYN-APPLICATION-1").Scan(&reversalBasis); err != nil || reversalBasis != "SYN-FUNDS-RETURNED-1" {
		t.Fatalf("撤销依据 = %q（%v）", reversalBasis, err)
	}
	if envelopes("SYN-APPLICATION-1") != 2 {
		t.Fatalf("撤销后信封 = %d，要 2（撤销拍自成一封、同分区）", envelopes("SYN-APPLICATION-1"))
	}
	run(commandFundsApplicationReversal, reversalDocument("SYN-APPLICATION-1"), exitRegistered, "ALREADY_REVERSED")
	run(commandFundsApplicationReversal, reversalDocument("SYN-APPLICATION-NEVER"), exitUsage, "SOURCE_NOT_ACCEPTED")

	// 撤销恢复的未核销可以再核：这一次核满 5000。
	run(commandFundsApplication,
		applicationDocument("SYN-APPLICATION-3", "SYN-FACT-COMP-1", "SYN-MAPPING-COMP-1", "CUSTOMER_COMPENSATION", "SYN-COMP-1", "5000"),
		exitRegistered, "SETTLEMENT_APPLIED")

	// 追偿到账映射到追偿认可并部分核销。
	run(commandExternalFundsFact, receiptDocument("SYN-FACT-RECOVERY-1", "4000"), exitRegistered, "FUNDS_FACT_ADOPTED")
	run(commandFundsMapping, claimMappingDocument("SYN-MAPPING-RECOVERY-1", "SYN-FACT-RECOVERY-1", "RECOVERY_ACKNOWLEDGEMENT", "SYN-ACK-1"),
		exitRegistered, "FUNDS_MAPPED")
	run(commandFundsApplication,
		applicationDocument("SYN-APPLICATION-R1", "SYN-FACT-RECOVERY-1", "SYN-MAPPING-RECOVERY-1", "RECOVERY_ACKNOWLEDGEMENT", "SYN-ACK-1", "4000"),
		exitRegistered, "SETTLEMENT_APPLIED")

	if after := readClaimAmountRow(t, pool); after != amountBefore {
		t.Fatalf("映射、核销与撤销改了客户赔付那一行：%+v → %+v", amountBefore, after)
	}
	var acknowledged int64
	if err := pool.QueryRow(ctx,
		`SELECT acknowledged_minor FROM settlement_accounting.recovery_acknowledgement WHERE tenant_id = $1 AND acknowledgement_id = $2`,
		claimTenant, "SYN-ACK-1").Scan(&acknowledged); err != nil || acknowledged != 6000 {
		t.Fatalf("追偿认可金额 = %d（%v），要原样 6000", acknowledged, err)
	}
}

// Covers: ADR-0029 在核销入口上的那一格——核销引用长到信封被框架确定性拒收，整笔回滚：核销行零、信封零；退用法格 1 并点名
// 原因、不叫人重跑。重放同一份同样退 1、同样零行，不答`已存在`。
func TestARejectedApplicationEnvelopeRollsBackTheApplicationAndExitsAsUsage(t *testing.T) {
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
	if message, code := execute(ctx, commandExternalFundsFact, []byte(receiptDocument("SYN-FACT-STMT-1", "8000")), registrar); code != exitRegistered {
		t.Fatalf("采用：%s", message)
	}
	if message, code := execute(ctx, commandFundsMapping,
		[]byte(claimMappingDocument("SYN-MAPPING-STMT-1", "SYN-FACT-STMT-1", "STATEMENT", "SYN-STMT-1")), registrar); code != exitRegistered {
		t.Fatalf("映射：%s", message)
	}
	overlong := "SYN-APPLICATION-" + strings.Repeat("x", eventing.MaxSubjectLength)
	document := applicationDocument(overlong, "SYN-FACT-STMT-1", "SYN-MAPPING-STMT-1", "STATEMENT", "SYN-STMT-1", "1000")
	for _, attempt := range []string{"首次", "重放同一份"} {
		message, code := execute(ctx, commandFundsApplication, []byte(document), registrar)
		if code != exitUsage || !strings.Contains(message, "拒收") || strings.Contains(message, "重跑") {
			t.Fatalf("%s：退出码 = %d、答复 = %q，要用法格 1、点名拒收、不叫人重跑", attempt, code, message)
		}
		if rows := countRows(t, pool,
			`SELECT count(*) FROM settlement_accounting.settlement_application WHERE tenant_id = $1`, claimTenant); rows != 0 {
			t.Fatalf("%s后核销行 = %d，要整笔回滚为零", attempt, rows)
		}
		if rows := countRows(t, pool,
			`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE partition_key = $1`, claimTenant+"/application/"+overlong); rows != 0 {
			t.Fatalf("%s后被拒的信封落了 %d 封", attempt, rows)
		}
	}
}
