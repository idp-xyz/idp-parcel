package postgres_test

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 结算与核算四张页读面的真库用例（ADR-0077 Decision 四/五那几句在这四本册上的样子，
// 票 admin-skeleton-closure-batch/04）。
//
// 播种走 pool.Exec 的显式 SQL，不借道写适配器：读口用例必须能独立于写口播种，否则
// 写口一有 bug 就会同时染红两侧，再也分不出是谁错（判据同 collectionremittance
// catalogueFixture 注释）。
//
// 助手一律带 catalogue 前缀：本包与写侧各聚合的测试共享 postgres_test 包名。

var catalogueBaseAt = time.Date(2026, 8, 30, 9, 0, 0, 0, time.UTC)

const catalogueTenant = "tenant-a"

type catalogueFixture struct {
	pool      *pgxpool.Pool
	charges   *adapter.ChargeCatalogue
	statement *adapter.StatementCatalogue
	funds     *adapter.FundsApplicationCatalogue
	operating *adapter.OperatingCatalogue
}

func newCatalogueFixture(t *testing.T) *catalogueFixture {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	charges, err := adapter.NewChargeCatalogue(db)
	if err != nil {
		t.Fatalf("构造费用读口：%v", err)
	}
	statement, err := adapter.NewStatementCatalogue(db)
	if err != nil {
		t.Fatalf("构造对账读口：%v", err)
	}
	funds, err := adapter.NewFundsApplicationCatalogue(db)
	if err != nil {
		t.Fatalf("构造核销读口：%v", err)
	}
	operating, err := adapter.NewOperatingCatalogue(db)
	if err != nil {
		t.Fatalf("构造经营读口：%v", err)
	}
	return &catalogueFixture{
		pool: pool, charges: charges, statement: statement,
		funds: funds, operating: operating,
	}
}

// seed 播种一行登记内容，失败即测试失败——播不进去的夹具证不了任何读口行为。
func (fixture *catalogueFixture) seed(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := fixture.pool.Exec(t.Context(), sql, args...); err != nil {
		t.Fatalf("播种登记内容：%v", err)
	}
}

func catalogueTenantID(t *testing.T, raw string) domain.TenantID {
	t.Helper()
	tenant, err := domain.NewTenantID(raw)
	if err != nil {
		t.Fatalf("构造租户 %q：%v", raw, err)
	}
	return tenant
}

// Covers: ADR-0077 Decision 四 — 七本册全空时如实答空列表，不是错误也不折成未配置。
func TestEmptySettlementRegistersAnswerEmptyLists(t *testing.T) {
	fixture := newCatalogueFixture(t)
	ctx, tenant := t.Context(), catalogueTenantID(t, catalogueTenant)

	charges, err := fixture.charges.ListCustomerCharges(ctx, tenant, 10)
	if err != nil || len(charges) != 0 {
		t.Fatalf("空客户费用册走样：err=%v rows=%+v", err, charges)
	}
	costs, err := fixture.charges.ListSupplierExpectedCosts(ctx, tenant, 10)
	if err != nil || len(costs) != 0 {
		t.Fatalf("空预期成本册走样：err=%v rows=%+v", err, costs)
	}
	statements, err := fixture.statement.ListCustomerStatements(ctx, tenant, 10)
	if err != nil || len(statements) != 0 {
		t.Fatalf("空对账单册走样：err=%v rows=%+v", err, statements)
	}
	receptions, err := fixture.statement.ListSupplierBillReceptions(ctx, tenant, 10)
	if err != nil || len(receptions) != 0 {
		t.Fatalf("空账单接收册走样：err=%v rows=%+v", err, receptions)
	}
	facts, err := fixture.funds.ListExternalFundsFacts(ctx, tenant, 10)
	if err != nil || len(facts) != 0 {
		t.Fatalf("空资金事实册走样：err=%v rows=%+v", err, facts)
	}
	results, err := fixture.operating.ListOperatingResults(ctx, tenant, 10)
	if err != nil || len(results) != 0 {
		t.Fatalf("空经营结果册走样：err=%v rows=%+v", err, results)
	}
	allocations, err := fixture.operating.ListCostAllocations(ctx, tenant, 10)
	if err != nil || len(allocations) != 0 {
		t.Fatalf("空分摊册走样：err=%v rows=%+v", err, allocations)
	}
}

// Covers: ADR-0077 Decision 五 — limit 非正拒，七个读口一个不漏。
func TestSettlementCataloguesGuardTheirLimit(t *testing.T) {
	fixture := newCatalogueFixture(t)
	ctx, tenant := t.Context(), catalogueTenantID(t, catalogueTenant)

	for name, read := range map[string]func(int) error{
		"客户费用": func(limit int) error {
			_, err := fixture.charges.ListCustomerCharges(ctx, tenant, limit)
			return err
		},
		"预期成本": func(limit int) error {
			_, err := fixture.charges.ListSupplierExpectedCosts(ctx, tenant, limit)
			return err
		},
		"对账单": func(limit int) error {
			_, err := fixture.statement.ListCustomerStatements(ctx, tenant, limit)
			return err
		},
		"账单接收": func(limit int) error {
			_, err := fixture.statement.ListSupplierBillReceptions(ctx, tenant, limit)
			return err
		},
		"资金事实": func(limit int) error {
			_, err := fixture.funds.ListExternalFundsFacts(ctx, tenant, limit)
			return err
		},
		"经营结果": func(limit int) error {
			_, err := fixture.operating.ListOperatingResults(ctx, tenant, limit)
			return err
		},
		"成本分摊": func(limit int) error {
			_, err := fixture.operating.ListCostAllocations(ctx, tenant, limit)
			return err
		},
	} {
		if err := read(0); err == nil {
			t.Fatalf("%s：limit=0 的上列被接受了", name)
		}
		if err := read(-1); err == nil {
			t.Fatalf("%s：limit=-1 的上列被接受了", name)
		}
	}
}

// seedCustomerCharge 落一笔客户费用。同币种时 conversion 传空串（库上同币种禁带
// 换算步骤的反面：带了也没有依据），跨币种时必带。
func (fixture *catalogueFixture) seedCustomerCharge(
	t *testing.T,
	tenant, charge, feeItem, stage string,
	originalCurrency string, originalMinor int64,
	settlementCurrency string, settlementMinor int64,
	conversion, confirmationBasis string,
) {
	t.Helper()
	var confirmedAt *time.Time
	var basis *string
	// 七项与确认留痕同进同出：customer_charge_confirmation_facts_coupled 要求确认行
	// 俱全、非确认行全缺，播种绕不过它（ADR-0087 决定一）。
	var facts [7]*string
	if stage == "CONFIRMED" {
		confirmed := catalogueBaseAt.Add(time.Hour)
		confirmedAt = &confirmed
		basis = &confirmationBasis
		values := []string{
			"LEGAL-ENTITY/SYN-01", "COUNTERPARTY/SYN-01", "RECEIVABLE",
			"ACCOUNT/SYN-01", "CONTRACT/SYN-01", "SCOPE/SYN-01", "SOURCE-FACT/SYN-01",
		}
		for index := range values {
			facts[index] = &values[index]
		}
	}
	var conversionRef *string
	if conversion != "" {
		conversionRef = &conversion
	}
	fixture.seed(t,
		`INSERT INTO settlement_accounting.customer_charge
			(tenant_id, charge_id, fee_item, evaluation_ref,
			 settlement_currency, settlement_minor, stage, confirmation_basis,
			 formed_at, confirmed_at, recorded_at,
			 original_currency, original_minor, conversion_ref,
			 responsible_entity, counterparty_ref, charge_direction,
			 settlement_account_id, contract_basis, primary_charging_scope,
			 source_fact_ref)
		 VALUES ($1, $2, $3, 'SYN-EVAL-01', $4, $5, $6, $7, $8, $9, $8, $10, $11, $12,
		         $13, $14, $15, $16, $17, $18, $19)`,
		tenant, charge, feeItem, settlementCurrency, settlementMinor, stage, basis,
		catalogueBaseAt, confirmedAt, originalCurrency, originalMinor, conversionRef,
		facts[0], facts[1], facts[2], facts[3], facts[4], facts[5], facts[6])
}

// Covers: 客户费用逐格转写 — 币种三件组整组在场、确认留痕成对、已到达依据按种类
// 升序挂在行上；跨租户不可见；确认条件目录用 LEFT JOIN 接，目录没有那一行时费用
// 照样上列（「目录未配置」不折成「费用不存在」）。
func TestCustomerChargeCatalogueTranscribesChargesAndTheirBases(t *testing.T) {
	fixture := newCatalogueFixture(t)

	fixture.seedCustomerCharge(t, catalogueTenant, "SYN-CHG-01", "SYN-FEE-01", "CONFIRMED",
		"SYN-CUR-01", 120000, "SYN-CUR-01", 120000, "", "SYN-CBASIS-01")
	fixture.seedCustomerCharge(t, "tenant-b", "SYN-CHG-09", "SYN-FEE-01", "ESTIMATED",
		"SYN-CUR-01", 1, "SYN-CUR-01", 1, "", "")

	fixture.seed(t,
		`INSERT INTO settlement_accounting.charge_confirmation_condition
			(tenant_id, fee_item, required_basis_kind, registered_at)
		 VALUES ($1, 'SYN-FEE-01', 'DELIVERY_CONFIRMED', $2)`,
		catalogueTenant, catalogueBaseAt)

	// 两种依据到达，按种类升序上列（MILESTONE 在 DELIVERY 之后）。
	fixture.seed(t,
		`INSERT INTO settlement_accounting.charge_confirmation_basis
			(tenant_id, charge_id, fee_item, basis_kind, basis_ref, recorded_at)
		 VALUES
			($1, 'SYN-CHG-01', 'SYN-FEE-01', 'MILESTONE_REACHED', 'SYN-MS-01', $2),
			($1, 'SYN-CHG-01', 'SYN-FEE-01', 'DELIVERY_CONFIRMED', 'SYN-DLV-01', $2)`,
		catalogueTenant, catalogueBaseAt)

	rows, err := fixture.charges.ListCustomerCharges(
		t.Context(), catalogueTenantID(t, catalogueTenant), 10)
	if err != nil {
		t.Fatalf("上列客户费用：%v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("上列了 %d 行，要 1 行（另一行属他租户）：%+v", len(rows), rows)
	}
	row := rows[0]
	if row.Charge != "SYN-CHG-01" || row.FeeItem != "SYN-FEE-01" ||
		row.Evaluation != "SYN-EVAL-01" || row.Stage != "CONFIRMED" {
		t.Fatalf("费用身份转写走样：%+v", row)
	}
	if row.OriginalCurrency != "SYN-CUR-01" || row.OriginalMinor != 120000 ||
		row.SettlementCurrency != "SYN-CUR-01" || row.SettlementMinor != 120000 ||
		row.ConversionStep != "" {
		t.Fatalf("币种三件组走样（同币种两额须相等且无换算步骤）：%+v", row)
	}
	if row.ConfirmationBasis != "SYN-CBASIS-01" || row.ConfirmedAt == nil ||
		!row.ConfirmedAt.Equal(catalogueBaseAt.Add(time.Hour)) ||
		!row.FormedAt.Equal(catalogueBaseAt) {
		t.Fatalf("确认留痕走样：%+v", row)
	}
	// 确认时固定的七项逐格转写（ADR-0087 决定一）。逐格点名而不是只判非空：七列同型
	// 同种，SQL 里错位一列照样每格都有值，只有对上各自的播种值才认得出来。
	for name, pair := range map[string][2]string{
		"责任法人":    {row.ResponsibleEntity, "LEGAL-ENTITY/SYN-01"},
		"结算相对方":   {row.Counterparty, "COUNTERPARTY/SYN-01"},
		"收付方向":    {row.ChargeDirection, "RECEIVABLE"},
		"结算账户":    {row.SettlementAccount, "ACCOUNT/SYN-01"},
		"合同或责任依据": {row.ContractBasis, "CONTRACT/SYN-01"},
		"主要计费范围":  {row.PrimaryChargingScope, "SCOPE/SYN-01"},
		"来源事实":    {row.SourceFact, "SOURCE-FACT/SYN-01"},
	} {
		if pair[0] != pair[1] {
			t.Fatalf("「%s」转写成了 %q，要 %q——七列同型，错位一列每格照样有值", name, pair[0], pair[1])
		}
	}

	t.Run("an unconfirmed row carries none of the seven", func(t *testing.T) {
		fixture.seedCustomerCharge(t, catalogueTenant, "SYN-CHG-02", "SYN-FEE-01", "ESTIMATED",
			"SYN-CUR-01", 500, "SYN-CUR-01", 500, "", "")
		rows, err := fixture.charges.ListCustomerCharges(
			t.Context(), catalogueTenantID(t, catalogueTenant), 10)
		if err != nil {
			t.Fatalf("上列客户费用：%v", err)
		}
		var estimated *ports.CustomerChargeCatalogueRow
		for index := range rows {
			if rows[index].Charge == "SYN-CHG-02" {
				estimated = &rows[index]
			}
		}
		if estimated == nil {
			t.Fatal("预估行没上列")
		}
		// 七格皆空是「这一行还没确认」的正面形状，由库上那条同在或同缺守着；读面不代填。
		if estimated.ResponsibleEntity != "" || estimated.Counterparty != "" ||
			estimated.ChargeDirection != "" || estimated.SettlementAccount != "" ||
			estimated.ContractBasis != "" || estimated.PrimaryChargingScope != "" ||
			estimated.SourceFact != "" {
			t.Fatalf("预估行带了确认才该固定的事实：%+v", estimated)
		}
	})
	if row.RequiredBasisKind != "DELIVERY_CONFIRMED" {
		t.Fatalf("确认条件目录要求的依据种类走样：%q", row.RequiredBasisKind)
	}
	if len(row.ConfirmationBases) != 2 ||
		row.ConfirmationBases[0].BasisKind != "DELIVERY_CONFIRMED" ||
		row.ConfirmationBases[0].Basis != "SYN-DLV-01" ||
		!row.ConfirmationBases[0].RecordedAt.Equal(catalogueBaseAt) ||
		row.ConfirmationBases[1].BasisKind != "MILESTONE_REACHED" {
		t.Fatalf("已到达依据走样（要按种类升序两项）：%+v", row.ConfirmationBases)
	}
}

// Covers: 「确认条件目录未配置」与「配了但依据没到」是两个答案，读口分两格摆开
// 不合并（迁移 0009 分两张表的同一条理由）；跨币种费用必带换算步骤，照实转写。
func TestCustomerChargeCatalogueKeepsUnconfiguredConditionApartFromMissingBasis(t *testing.T) {
	fixture := newCatalogueFixture(t)

	// SYN-CHG-01：目录里没有 SYN-FEE-01 这一行——未配置。
	fixture.seedCustomerCharge(t, catalogueTenant, "SYN-CHG-01", "SYN-FEE-01", "ESTIMATED",
		"SYN-CUR-02", 88000, "SYN-CUR-01", 90000, "SYN-CONV-01", "")
	// SYN-CHG-02：目录配了 SYN-FEE-02 要哪种依据，但一条依据都没到。
	fixture.seedCustomerCharge(t, catalogueTenant, "SYN-CHG-02", "SYN-FEE-02", "PROVISIONAL",
		"SYN-CUR-01", 5000, "SYN-CUR-01", 5000, "", "")
	fixture.seed(t,
		`INSERT INTO settlement_accounting.charge_confirmation_condition
			(tenant_id, fee_item, required_basis_kind, registered_at)
		 VALUES ($1, 'SYN-FEE-02', 'DELIVERY_CONFIRMED', $2)`,
		catalogueTenant, catalogueBaseAt)

	rows, err := fixture.charges.ListCustomerCharges(
		t.Context(), catalogueTenantID(t, catalogueTenant), 10)
	if err != nil || len(rows) != 2 {
		t.Fatalf("上列客户费用走样：err=%v rows=%+v", err, rows)
	}

	unconfigured := rows[0]
	if unconfigured.Charge != "SYN-CHG-01" {
		t.Fatalf("键序走样（要按费用标识升序）：%+v", rows)
	}
	if unconfigured.RequiredBasisKind != "" || len(unconfigured.ConfirmationBases) != 0 {
		t.Fatalf("目录未配置该费用项目时应两格皆空：%+v", unconfigured)
	}
	if unconfigured.OriginalCurrency != "SYN-CUR-02" || unconfigured.OriginalMinor != 88000 ||
		unconfigured.SettlementMinor != 90000 || unconfigured.ConversionStep != "SYN-CONV-01" {
		t.Fatalf("跨币种三件组走样：%+v", unconfigured)
	}
	if unconfigured.ConfirmedAt != nil || unconfigured.ConfirmationBasis != "" {
		t.Fatalf("未确认费用不该带确认留痕：%+v", unconfigured)
	}

	awaiting := rows[1]
	if awaiting.RequiredBasisKind != "DELIVERY_CONFIRMED" || len(awaiting.ConfirmationBases) != 0 {
		t.Fatalf("「配了但依据没到」应是有要求、无依据：%+v", awaiting)
	}
}

// Covers: 供应商预期成本版本链 — 首版与纠错版本同为多行（纠错换版本、原版本保留），
// 排序把同一（发生项＋费用项目）的版本排在一起；纠错两件成对上列。
func TestSupplierExpectedCostCatalogueKeepsTheVersionChain(t *testing.T) {
	fixture := newCatalogueFixture(t)

	// 同币种两额必须相等，对所有版本成立（迁移 0012 依 ADR-0067 撤掉了 0008 给纠错
	// 版本留的例外支）：计价纠错整组重述一个新评价的计价结果，原币金额随之改，不是
	// 只动结算额。所以纠错版本传的是同一个新数字，不是一新一旧。
	seedCost := func(version, prior, reason string, amountMinor int64) {
		var priorRef, reasonRef *string
		if prior != "" {
			priorRef, reasonRef = &prior, &reason
		}
		fixture.seed(t,
			`INSERT INTO settlement_accounting.supplier_expected_cost
				(tenant_id, version, occurrence_id, occurrence_reason, occurrence_version,
				 occurred_at, fee_item, purchase_rule_version, agreement_ref, evaluation_ref,
				 original_currency, original_minor, settlement_currency, settlement_minor,
				 conversion_ref, prior_version, correction_reason, recorded_at)
			 VALUES ($1, $2, 'SYN-OCC-01', 'BOOKING', 'V1', $3, 'SYN-FEE-01',
			         'SYN-RULE-01', 'SYN-AGR-01', 'SYN-BUY-01',
			         'SYN-CUR-01', $4, 'SYN-CUR-01', $4, NULL, $5, $6, $3)`,
			catalogueTenant, version, catalogueBaseAt, amountMinor, priorRef, reasonRef)
	}
	seedCost("SYN-COST-V1", "", "", 40000)
	seedCost("SYN-COST-V2", "SYN-COST-V1", "PRICING_CORRECTION", 41000)

	rows, err := fixture.charges.ListSupplierExpectedCosts(
		t.Context(), catalogueTenantID(t, catalogueTenant), 10)
	if err != nil || len(rows) != 2 {
		t.Fatalf("上列预期成本走样：err=%v rows=%+v", err, rows)
	}
	first, correction := rows[0], rows[1]
	if first.Version != "SYN-COST-V1" || first.PriorVersion != "" || first.CorrectionReason != "" {
		t.Fatalf("首版不该带纠错两件：%+v", first)
	}
	if first.Occurrence != "SYN-OCC-01" || first.OccurrenceReason != "BOOKING" ||
		first.OccurrenceVersion != "V1" || first.FeeItem != "SYN-FEE-01" ||
		first.PurchaseRuleVersion != "SYN-RULE-01" || first.Agreement != "SYN-AGR-01" ||
		first.Evaluation != "SYN-BUY-01" || first.ConversionStep != "" ||
		first.SettlementMinor != 40000 || !first.OccurredAt.Equal(catalogueBaseAt) {
		t.Fatalf("首版逐格转写走样：%+v", first)
	}
	if correction.PriorVersion != "SYN-COST-V1" ||
		correction.CorrectionReason != "PRICING_CORRECTION" ||
		correction.SettlementMinor != 41000 || correction.OriginalMinor != 41000 {
		t.Fatalf("纠错版本走样（整组重述，两额同为新评价的数）：%+v", correction)
	}
}

// Covers: 对账单行 — 明细只取行数不取内容、作废留痕成对、异议与后续纳入各自按
// 标识升序挂在本单上；未裁定异议三件皆空，LATE_CHARGE 纳入不指名调整。
func TestCustomerStatementCatalogueAttachesDisputesAndInclusions(t *testing.T) {
	fixture := newCatalogueFixture(t)

	fixture.seed(t,
		`INSERT INTO settlement_accounting.customer_statement
			(tenant_id, statement_number, account_id, period_ref, currency, total_minor,
			 content_digest, lines, adjustment_lines, published_at, recorded_at)
		 VALUES ($1, 'SYN-STMT-01', 'SYN-ACC-01', 'SYN-PER-01', 'SYN-CUR-01', 150000,
		         'SYN-DIGEST-01',
		         '[{"charge":"SYN-CHG-01","amount_minor":100000},
		           {"charge":"SYN-CHG-02","amount_minor":50000}]'::jsonb,
		         '[]'::jsonb, $2, $2)`,
		catalogueTenant, catalogueBaseAt)

	resolvedAt := catalogueBaseAt.Add(48 * time.Hour)
	fixture.seed(t,
		`INSERT INTO settlement_accounting.statement_dispute
			(tenant_id, dispute_id, statement_number, charge_id, disputed_minor,
			 reason_ref, opened_at, resolution, resolution_ref, resolved_at,
			 content_digest, recorded_at)
		 VALUES
			($1, 'SYN-DSP-02', 'SYN-STMT-01', 'SYN-CHG-02', 20000, 'SYN-RSN-02', $2,
			 NULL, NULL, NULL, 'SYN-DIGEST-D2', $2),
			($1, 'SYN-DSP-01', 'SYN-STMT-01', 'SYN-CHG-01', 30000, 'SYN-RSN-01', $2,
			 'PARTIALLY_ACCEPTED', 'SYN-RES-01', $3, 'SYN-DIGEST-D1', $2)`,
		catalogueTenant, catalogueBaseAt, resolvedAt)

	fixture.seed(t,
		`INSERT INTO settlement_accounting.subsequent_inclusion
			(tenant_id, inclusion_id, kind, statement_number, original_period,
			 subsequent_period, charge_id, adjustment_id, included_at,
			 content_digest, recorded_at)
		 VALUES
			($1, 'SYN-INC-01', 'ADJUSTMENT', 'SYN-STMT-01', 'SYN-PER-01', 'SYN-PER-02',
			 'SYN-CHG-01', 'SYN-ADJ-01', $2, 'SYN-DIGEST-I1', $2),
			($1, 'SYN-INC-02', 'LATE_CHARGE', 'SYN-STMT-01', 'SYN-PER-01', 'SYN-PER-02',
			 'SYN-CHG-03', NULL, $2, 'SYN-DIGEST-I2', $2)`,
		catalogueTenant, catalogueBaseAt)

	rows, err := fixture.statement.ListCustomerStatements(
		t.Context(), catalogueTenantID(t, catalogueTenant), 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("上列对账单走样：err=%v rows=%+v", err, rows)
	}
	row := rows[0]
	if row.StatementNumber != "SYN-STMT-01" || row.Account != "SYN-ACC-01" ||
		row.Period != "SYN-PER-01" || row.Currency != "SYN-CUR-01" ||
		row.TotalMinor != 150000 || !row.PublishedAt.Equal(catalogueBaseAt) {
		t.Fatalf("单面转写走样：%+v", row)
	}
	if row.LineCount != 2 || row.AdjustmentCount != 0 {
		t.Fatalf("行数走样（要 2 条费用行、0 条调整行）：%+v", row)
	}
	if row.VoidBasis != "" || row.VoidedAt != nil {
		t.Fatalf("未作废单不该带作废留痕：%+v", row)
	}
	if len(row.Disputes) != 2 {
		t.Fatalf("异议数走样：%+v", row.Disputes)
	}
	resolved, pending := row.Disputes[0], row.Disputes[1]
	if resolved.Dispute != "SYN-DSP-01" || resolved.Charge != "SYN-CHG-01" ||
		resolved.DisputedMinor != 30000 || resolved.Reason != "SYN-RSN-01" ||
		resolved.Resolution != "PARTIALLY_ACCEPTED" || resolved.ResolutionRef != "SYN-RES-01" ||
		resolved.ResolvedAt == nil || !resolved.ResolvedAt.Equal(resolvedAt) {
		t.Fatalf("已裁定异议走样（要按标识升序在前）：%+v", resolved)
	}
	if pending.Dispute != "SYN-DSP-02" || pending.Resolution != "" ||
		pending.ResolutionRef != "" || pending.ResolvedAt != nil {
		t.Fatalf("未裁定异议应三件皆空，不代填结论：%+v", pending)
	}
	if len(row.SubsequentInclusions) != 2 ||
		row.SubsequentInclusions[0].Kind != "ADJUSTMENT" ||
		row.SubsequentInclusions[0].Adjustment != "SYN-ADJ-01" ||
		row.SubsequentInclusions[0].SubsequentPeriod != "SYN-PER-02" ||
		row.SubsequentInclusions[1].Kind != "LATE_CHARGE" ||
		row.SubsequentInclusions[1].Adjustment != "" {
		t.Fatalf("后续纳入走样（迟到费用不得指名调整）：%+v", row.SubsequentInclusions)
	}
}

// Covers: 作废留痕成对上列 — 作废不删行不改总额，`已作废`是这一行上的留痕而不是
// 它的消失。
func TestCustomerStatementCatalogueKeepsVoidedStatementsInPlace(t *testing.T) {
	fixture := newCatalogueFixture(t)
	voidedAt := catalogueBaseAt.Add(24 * time.Hour)
	fixture.seed(t,
		`INSERT INTO settlement_accounting.customer_statement
			(tenant_id, statement_number, account_id, period_ref, currency, total_minor,
			 content_digest, lines, adjustment_lines, published_at,
			 void_basis, voided_at, recorded_at)
		 VALUES ($1, 'SYN-STMT-01', 'SYN-ACC-01', 'SYN-PER-01', 'SYN-CUR-01', 150000,
		         'SYN-DIGEST-01',
		         '[{"charge":"SYN-CHG-01","amount_minor":150000}]'::jsonb,
		         '[]'::jsonb, $2, 'SYN-VOID-01', $3, $2)`,
		catalogueTenant, catalogueBaseAt, voidedAt)

	rows, err := fixture.statement.ListCustomerStatements(
		t.Context(), catalogueTenantID(t, catalogueTenant), 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("上列对账单走样：err=%v rows=%+v", err, rows)
	}
	row := rows[0]
	if row.VoidBasis != "SYN-VOID-01" || row.VoidedAt == nil || !row.VoidedAt.Equal(voidedAt) {
		t.Fatalf("作废留痕走样：%+v", row)
	}
	if row.TotalMinor != 150000 || row.LineCount != 1 {
		t.Fatalf("作废不该改总额或抹掉行数：%+v", row)
	}
}

// Covers: 供应商账单接收 — 主张行数与匹配数只取长度；审核授权是否已配置照实转写，
// 不折成一个动作可用性。
func TestSupplierBillReceptionCatalogueTranscribesCountsAndAuthority(t *testing.T) {
	fixture := newCatalogueFixture(t)
	fixture.seed(t,
		`INSERT INTO settlement_accounting.supplier_bill_reception
			(tenant_id, claim_id, claim_version, supplier_ref, legal_entity, period_ref,
			 currency, content_digest, claim, matches, audit_authority_configured, recorded_at)
		 VALUES ($1, 'SYN-CLM-01', 'V1', 'SYN-SUP-01', 'SYN-LE-01', 'SYN-PER-01',
		         'SYN-CUR-01', 'SYN-DIGEST-B1',
		         '{"lines":[{"line":"L1","fee_item":"SYN-FEE-01","claimed_minor":1000},
		                    {"line":"L2","fee_item":"SYN-FEE-02","claimed_minor":2000}],
		           "received_at":"2026-08-30T09:00:00Z"}'::jsonb,
		         '[{"line":"L1","classification":"MATCHED","claimed_minor":1000,
		            "expected_minor":1000,"matched_at":"2026-08-30T09:00:00Z"}]'::jsonb,
		         false, $2)`,
		catalogueTenant, catalogueBaseAt)

	rows, err := fixture.statement.ListSupplierBillReceptions(
		t.Context(), catalogueTenantID(t, catalogueTenant), 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("上列账单接收走样：err=%v rows=%+v", err, rows)
	}
	row := rows[0]
	if row.Claim != "SYN-CLM-01" || row.ClaimVersion != "V1" ||
		row.Supplier != "SYN-SUP-01" || row.LegalEntity != "SYN-LE-01" ||
		row.Period != "SYN-PER-01" || row.Currency != "SYN-CUR-01" {
		t.Fatalf("主张身份转写走样：%+v", row)
	}
	if row.LineCount != 2 || row.MatchCount != 1 {
		t.Fatalf("行数与匹配数走样（要 2 与 1）：%+v", row)
	}
	if row.AuditAuthorityConfigured {
		t.Fatalf("审核授权未配置却报成已配置：%+v", row)
	}
}

// Covers: 已核销与已撤销两笔分别求和，不给净额 —— 「从未核销过」与「核销过又撤销
// 了」在一个净额上长着同一张脸，而两态续办相反；未核销余额只扣未撤销那一笔。
// 映射与核销各按标识升序挂在事实行上。
func TestExternalFundsFactCatalogueSplitsAppliedFromReversed(t *testing.T) {
	fixture := newCatalogueFixture(t)

	seedExternalFundsFactVersion(t, fixture, "SYN-FACT-01", "RECEIPT_CONFIRMED", 100000, "V1", "", catalogueBaseAt)

	fixture.seed(t,
		`INSERT INTO settlement_accounting.funds_mapping
			(tenant_id, mapping_id, fact_id, target_kind, target_ref, basis,
			 mapped_at, content_digest, recorded_at)
		 VALUES ($1, 'SYN-MAP-01', 'SYN-FACT-01', 'STATEMENT', 'SYN-STMT-01',
		         'SYN-MBASIS-01', $2, 'SYN-DIGEST-M1', $2)`,
		catalogueTenant, catalogueBaseAt)

	reversedAt := catalogueBaseAt.Add(72 * time.Hour)
	fixture.seed(t,
		`INSERT INTO settlement_accounting.settlement_application
			(tenant_id, application_id, fact_id, currency, fact_minor, applied_minor,
			 allocations, basis, applied_at, reversal_basis, reversed_at,
			 content_digest, recorded_at)
		 VALUES
			($1, 'SYN-APP-01', 'SYN-FACT-01', 'SYN-CUR-01', 100000, 30000,
			 '[{"mapping":"SYN-MAP-01","targetKind":"STATEMENT","target":"SYN-STMT-01",
			    "direction":"CREDIT","amountMinor":30000}]'::jsonb,
			 'SYN-ABASIS-01', $2, NULL, NULL, 'SYN-DIGEST-A1', $2),
			($1, 'SYN-APP-02', 'SYN-FACT-01', 'SYN-CUR-01', 100000, 25000,
			 '[{"mapping":"SYN-MAP-01","targetKind":"STATEMENT","target":"SYN-STMT-01",
			    "direction":"CREDIT","amountMinor":25000}]'::jsonb,
			 'SYN-ABASIS-02', $2, 'SYN-REV-01', $3, 'SYN-DIGEST-A2', $2)`,
		catalogueTenant, catalogueBaseAt, reversedAt)

	rows, err := fixture.funds.ListExternalFundsFacts(
		t.Context(), catalogueTenantID(t, catalogueTenant), 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("上列资金事实走样：err=%v rows=%+v", err, rows)
	}
	row := rows[0]
	if row.Fact != "SYN-FACT-01" || row.Source != "SYN-SRC-01" ||
		row.Kind != "RECEIPT_CONFIRMED" || row.Currency != "SYN-CUR-01" ||
		row.AmountMinor != 100000 || row.Version != "V1" ||
		!row.OccurredAt.Equal(catalogueBaseAt) {
		t.Fatalf("事实面转写走样：%+v", row)
	}
	if row.Corrects != "" || row.CorrectedAt != nil {
		t.Fatalf("未更正事实不该带更正留痕：%+v", row)
	}
	if row.AppliedMinor != 30000 || row.ReversedMinor != 25000 ||
		row.UnappliedMinor != 70000 || row.ApplicationCount != 2 {
		t.Fatalf("两笔求和走样（要 已核销 30000、已撤销 25000、未核销 70000、共 2 笔）：%+v", row)
	}
	if len(row.Mappings) != 1 || row.Mappings[0].Mapping != "SYN-MAP-01" ||
		row.Mappings[0].TargetKind != "STATEMENT" || row.Mappings[0].Target != "SYN-STMT-01" ||
		row.Mappings[0].Basis != "SYN-MBASIS-01" ||
		!row.Mappings[0].MappedAt.Equal(catalogueBaseAt) {
		t.Fatalf("映射转写走样：%+v", row.Mappings)
	}
	if len(row.Applications) != 2 {
		t.Fatalf("核销数走样：%+v", row.Applications)
	}
	live, reversed := row.Applications[0], row.Applications[1]
	if live.Application != "SYN-APP-01" || live.AppliedMinor != 30000 ||
		live.Basis != "SYN-ABASIS-01" || live.AllocationCount != 1 ||
		live.ReversalBasis != "" || live.ReversedAt != nil {
		t.Fatalf("未撤销核销走样：%+v", live)
	}
	if reversed.Application != "SYN-APP-02" || reversed.ReversalBasis != "SYN-REV-01" ||
		reversed.ReversedAt == nil || !reversed.ReversedAt.Equal(reversedAt) {
		t.Fatalf("已撤销核销走样（撤销不删历史，那一截金额要仍然看得见）：%+v", reversed)
	}
}

// Covers: 从未核销过的事实以三个零与空挂册在场 —— 与「核销过又撤销了」（上一个
// 用例）是两个相反的答案，不共用一格。
func TestExternalFundsFactCatalogueKeepsNeverAppliedFactsDistinct(t *testing.T) {
	fixture := newCatalogueFixture(t)
	seedExternalFundsFactVersion(t, fixture, "SYN-FACT-01", "FUNDS_RETURNED", 100000, "V1", "", catalogueBaseAt)

	rows, err := fixture.funds.ListExternalFundsFacts(
		t.Context(), catalogueTenantID(t, catalogueTenant), 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("上列资金事实走样：err=%v rows=%+v", err, rows)
	}
	row := rows[0]
	if row.AppliedMinor != 0 || row.ReversedMinor != 0 ||
		row.UnappliedMinor != 100000 || row.ApplicationCount != 0 {
		t.Fatalf("从未核销的事实走样：%+v", row)
	}
	if len(row.Mappings) != 0 || len(row.Applications) != 0 {
		t.Fatalf("从未核销的事实不该带挂册：%+v", row)
	}
}

// Covers: 票 sa-cc/20 完成判据 (2)「ListExternalFundsFacts 一事实一行、聚合不重复」——同一事实两版
// （V2 回指 V1、金额变）在库上两行，列表只出一行、内容是链头 V2 的、带回指；挂在事实身份上的映射与
// 核销只算一遍，未核销余额按链头金额算。另一条只有首版的事实照旧一行，排序仍按事实。
func TestExternalFundsFactCatalogueListsOneRowPerFactWithTheChainHead(t *testing.T) {
	fixture := newCatalogueFixture(t)
	correctedAt := catalogueBaseAt.Add(24 * time.Hour)
	seedExternalFundsFactVersion(t, fixture, "SYN-FACT-01", "RECEIPT_CONFIRMED", 100000, "V1", "", catalogueBaseAt)
	seedExternalFundsFactVersion(t, fixture, "SYN-FACT-01", "RECEIPT_CONFIRMED", 90000, "V2", "V1", correctedAt)
	seedExternalFundsFactVersion(t, fixture, "SYN-FACT-02", "RECEIPT_CONFIRMED", 5000, "V1", "", catalogueBaseAt)

	fixture.seed(t,
		`INSERT INTO settlement_accounting.funds_mapping
			(tenant_id, mapping_id, fact_id, target_kind, target_ref, basis,
			 mapped_at, content_digest, recorded_at)
		 VALUES ($1, 'SYN-MAP-01', 'SYN-FACT-01', 'STATEMENT', 'SYN-STMT-01',
		         'SYN-MBASIS-01', $2, 'SYN-DIGEST-M1', $2)`,
		catalogueTenant, catalogueBaseAt)
	fixture.seed(t,
		`INSERT INTO settlement_accounting.settlement_application
			(tenant_id, application_id, fact_id, currency, fact_minor, applied_minor,
			 allocations, basis, applied_at, content_digest, recorded_at)
		 VALUES ($1, 'SYN-APP-01', 'SYN-FACT-01', 'SYN-CUR-01', 100000, 30000,
			 '[{"mapping":"SYN-MAP-01","targetKind":"STATEMENT","target":"SYN-STMT-01",
			    "direction":"CREDIT","amountMinor":30000}]'::jsonb,
			 'SYN-ABASIS-01', $2, 'SYN-DIGEST-A1', $2)`,
		catalogueTenant, catalogueBaseAt)

	rows, err := fixture.funds.ListExternalFundsFacts(
		t.Context(), catalogueTenantID(t, catalogueTenant), 10)
	if err != nil || len(rows) != 2 {
		t.Fatalf("两条事实、其一两版，该列两行：err=%v rows=%+v", err, rows)
	}
	head := rows[0]
	if head.Fact != "SYN-FACT-01" || head.Version != "V2" || head.Corrects != "V1" ||
		head.CorrectedAt == nil || !head.CorrectedAt.Equal(correctedAt) || head.AmountMinor != 90000 {
		t.Fatalf("事实行该是链头 V2 的内容并回指 V1：%+v", head)
	}
	if head.AppliedMinor != 30000 || head.ReversedMinor != 0 ||
		head.UnappliedMinor != 60000 || head.ApplicationCount != 1 ||
		len(head.Mappings) != 1 || len(head.Applications) != 1 {
		t.Fatalf("挂在事实身份上的映射与核销只该算一遍、余额按链头金额：%+v", head)
	}
	if rows[1].Fact != "SYN-FACT-02" || rows[1].Version != "V1" || rows[1].Corrects != "" {
		t.Fatalf("只有首版的事实走样：%+v", rows[1])
	}
}

// seedExternalFundsFactVersion 直写 0021 起的两张表：身份行 DO NOTHING（同一事实的第二版撞见它），版本行
// 一版一行；corrects 为空即首版。付款人留 NULL（来源未提供），列表用例不看它。
func seedExternalFundsFactVersion(
	t *testing.T,
	fixture *catalogueFixture,
	fact, kind string,
	amountMinor int64,
	version, corrects string,
	recordedAt time.Time,
) {
	t.Helper()
	fixture.seed(t,
		`INSERT INTO settlement_accounting.external_funds_fact (tenant_id, fact_id, recorded_at)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (tenant_id, fact_id) DO NOTHING`,
		catalogueTenant, fact, recordedAt)
	var correctsColumn *string
	var correctedAt *time.Time
	if corrects != "" {
		correctsColumn = &corrects
		correctedAt = &recordedAt
	}
	fixture.seed(t,
		`INSERT INTO settlement_accounting.external_funds_fact_version
			(tenant_id, fact_id, version, source_ref, kind, currency, amount_minor,
			 occurred_at, corrects, corrected_at, content_digest, recorded_at)
		 VALUES ($1, $2, $3, 'SYN-SRC-01', $4, 'SYN-CUR-01', $5, $6, $7, $8, $9, $6)`,
		catalogueTenant, fact, version, kind, amountMinor, catalogueBaseAt, correctsColumn, correctedAt,
		"SYN-DIGEST-"+fact+"-"+version)
}

// Covers: 经营结果组成逐项上列（审核应付与贷项按各自借贷方向各计一次，不净额）；
// 毛利照库上那一列转写，读口不重算。
func TestOperatingResultCatalogueTranscribesComponentsWithoutNetting(t *testing.T) {
	fixture := newCatalogueFixture(t)
	fixture.seed(t,
		`INSERT INTO settlement_accounting.operating_result
			(tenant_id, scope_ref, period_ref, basis, currency, components, margin_minor,
			 version, as_of, content_digest, recorded_at)
		 VALUES ($1, 'SYN-SCOPE-01', 'SYN-PER-01', 'CONFIRMED', 'SYN-CUR-01',
		         '[{"source":"SYN-RECEIVABLE-01","role":"CUSTOMER_OPERATING_RECEIVABLE","effect":"INCREASES","amountMinor":150000},
		           {"source":"SYN-PAYABLE-01","role":"AUDITED_PAYABLE","effect":"DECREASES","amountMinor":90000},
		           {"source":"SYN-CREDIT-01","role":"SUPPLIER_CREDIT_NOTE","effect":"INCREASES","amountMinor":5000}]'::jsonb,
		         65000, 'SYN-RESV-01', $2, 'SYN-DIGEST-O1', $2)`,
		catalogueTenant, catalogueBaseAt)

	rows, err := fixture.operating.ListOperatingResults(
		t.Context(), catalogueTenantID(t, catalogueTenant), 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("上列经营结果走样：err=%v rows=%+v", err, rows)
	}
	row := rows[0]
	if row.Scope != "SYN-SCOPE-01" || row.Period != "SYN-PER-01" ||
		row.Basis != "CONFIRMED" || row.Currency != "SYN-CUR-01" ||
		row.MarginMinor != 65000 || row.Version != "SYN-RESV-01" ||
		!row.AsOf.Equal(catalogueBaseAt) || row.Corrects != "" {
		t.Fatalf("口径与毛利转写走样：%+v", row)
	}
	if len(row.Components) != 3 {
		t.Fatalf("组成项被折叠了（应付与贷项须各计一次）：%+v", row.Components)
	}
	if row.Components[0].Source != "SYN-RECEIVABLE-01" ||
		row.Components[0].Effect != "INCREASES" || row.Components[0].AmountMinor != 150000 ||
		row.Components[1].Effect != "DECREASES" || row.Components[1].AmountMinor != 90000 ||
		row.Components[2].Source != "SYN-CREDIT-01" {
		t.Fatalf("组成项逐格转写走样：%+v", row.Components)
	}
	// 角色照册原样转写（ADR-0087 决定三）：审核应付与贷项从此在读面上分得开，不必由
	// 读的人按 source 去猜——票 admin-skeleton-closure-batch/04 撤掉经营页四栏正是因为
	// 那时只能猜。
	if row.Components[0].Role != "CUSTOMER_OPERATING_RECEIVABLE" ||
		row.Components[1].Role != "AUDITED_PAYABLE" ||
		row.Components[2].Role != "SUPPLIER_CREDIT_NOTE" {
		t.Fatalf("组成项角色转写走样：%+v", row.Components)
	}
}

// Covers: 成本分摊 — 份额与未分摊余额各自照实转写；全额未分摊以空份额加满额未分摊
// 在场（没有合格对象时来源金额整笔留下等新依据，不折进份额里凑平）。
func TestCostAllocationCatalogueTranscribesPortionsAndUnallocated(t *testing.T) {
	fixture := newCatalogueFixture(t)
	fixture.seed(t,
		`INSERT INTO settlement_accounting.cost_allocation
			(tenant_id, allocation_id, source_ref, source_minor, currency, rule_ref,
			 portions, unallocated_minor, version, allocated_at, content_digest, recorded_at)
		 VALUES
			($1, 'SYN-ALC-01', 'SYN-SRC-01', 100000, 'SYN-CUR-01', 'SYN-ARULE-01',
			 '[{"target":"SYN-TGT-01","amountMinor":60000},
			   {"target":"SYN-TGT-02","amountMinor":40000}]'::jsonb,
			 0, 'SYN-ALCV-01', $2, 'SYN-DIGEST-L1', $2),
			($1, 'SYN-ALC-02', 'SYN-SRC-02', 70000, 'SYN-CUR-01', 'SYN-ARULE-01',
			 '[]'::jsonb, 70000, 'SYN-ALCV-02', $2, 'SYN-DIGEST-L2', $2)`,
		catalogueTenant, catalogueBaseAt)

	rows, err := fixture.operating.ListCostAllocations(
		t.Context(), catalogueTenantID(t, catalogueTenant), 10)
	if err != nil || len(rows) != 2 {
		t.Fatalf("上列成本分摊走样：err=%v rows=%+v", err, rows)
	}
	allocated, unallocated := rows[0], rows[1]
	if allocated.Allocation != "SYN-ALC-01" || allocated.Source != "SYN-SRC-01" ||
		allocated.SourceMinor != 100000 || allocated.Rule != "SYN-ARULE-01" ||
		allocated.UnallocatedMinor != 0 || allocated.Version != "SYN-ALCV-01" ||
		!allocated.AllocatedAt.Equal(catalogueBaseAt) {
		t.Fatalf("分摊面转写走样：%+v", allocated)
	}
	if len(allocated.Portions) != 2 ||
		allocated.Portions[0].Target != "SYN-TGT-01" || allocated.Portions[0].AmountMinor != 60000 ||
		allocated.Portions[1].Target != "SYN-TGT-02" || allocated.Portions[1].AmountMinor != 40000 {
		t.Fatalf("份额转写走样：%+v", allocated.Portions)
	}
	if len(unallocated.Portions) != 0 || unallocated.UnallocatedMinor != 70000 {
		t.Fatalf("全额未分摊应是空份额加满额未分摊：%+v", unallocated)
	}
}
