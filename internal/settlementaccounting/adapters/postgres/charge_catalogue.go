package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// ChargeCatalogue 实现 ports.ChargeCatalogueRead：客户费用册与供应商预期成本版本册的
// 列表读面（ADR-0077，票 admin-skeleton-closure-batch/04）。读的就是 0003/0009/0013 与
// 0008 的本表，不是第二份数据。
//
// 客户费用一行**一条语句取回**开立面、确认条件要求与已到达依据：ReadExecutor 不保证
// 两条语句同一快照，分次取会拼出从未同时存在的账面状态——与确认依据并发登记时尤甚
// （判据同 collectionremittance CodSubledgerCatalogue 文件头那句）。确认条件目录走
// LEFT JOIN：目录没有那一行时该费用照样上列，只是要求依据种类为空，「目录未配置」不
// 折成「费用不存在」。已到达依据走相关子查询 json_agg，与目录连接各聚各的。
//
// 两册按各自的声明取页（ports.CustomerChargePages、ports.SupplierExpectedCostPages）：
// 总数另数一句，与取页不在同一快照里，是此刻的大概；一行的各格仍在一条语句里。limit 非正
// 是调用方编程错误。
type ChargeCatalogue struct {
	db *bentopg.DB
}

func NewChargeCatalogue(db *bentopg.DB) (*ChargeCatalogue, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &ChargeCatalogue{db: db}, nil
}

var _ ports.ChargeCatalogueRead = (*ChargeCatalogue)(nil)

// confirmationBasisDocument 是确认依据在 json_agg 里的临时词形。依据表三列全部
// NOT NULL，不设指针格。
type confirmationBasisDocument struct {
	BasisKind  string    `json:"basisKind"`
	Basis      string    `json:"basis"`
	RecordedAt time.Time `json:"recordedAt"`
}

// customerChargePages 把客户费用册的声明落到列上。筛选与 q 只碰费用行自己的列，总数因此只数费用表，不带确认条件目录的连接。
var customerChargePages = ports.CustomerChargePages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"charge": "c.charge_id", "formedAt": "c.formed_at"},
	Identity: []string{"c.charge_id"},
	Filters:  map[string]string{"stage": "c.stage"},
	Keyword: []string{
		"c.charge_id", "c.fee_item", "c.evaluation_ref", "c.counterparty_ref",
		"c.source_fact_ref", "c.confirmation_basis",
	},
})

const (
	customerChargeTable  = `settlement_accounting.customer_charge AS c`
	customerChargeSource = customerChargeTable + `
		   LEFT JOIN settlement_accounting.charge_confirmation_condition AS req
		          ON req.tenant_id = c.tenant_id
		         AND req.fee_item = c.fee_item`
	customerChargeColumns = `c.charge_id, c.fee_item, c.evaluation_ref, c.stage,
		        c.original_currency, c.original_minor,
		        c.settlement_currency, c.settlement_minor,
		        c.conversion_ref, c.confirmation_basis, c.formed_at, c.confirmed_at,
		        c.responsible_entity, c.counterparty_ref, c.charge_direction,
		        c.settlement_account_id, c.contract_basis, c.primary_charging_scope,
		        c.source_fact_ref,
		        to_char(c.attribution_date, 'YYYY-MM-DD'),
		        (SELECT s.statement_number
		           FROM settlement_accounting.customer_statement AS s
		          WHERE s.tenant_id = c.tenant_id
		            AND s.voided_at IS NULL
		            AND s.lines @> jsonb_build_array(jsonb_build_object('charge', c.charge_id))
		          ORDER BY s.published_at, s.statement_number
		          LIMIT 1),
		        req.required_basis_kind,
		        (SELECT COALESCE(
		                    json_agg(
		                        json_build_object(
		                            'basisKind',  b.basis_kind,
		                            'basis',      b.basis_ref,
		                            'recordedAt', b.recorded_at
		                        )
		                        ORDER BY b.basis_kind
		                    ),
		                    '[]'::json
		                )
		           FROM settlement_accounting.charge_confirmation_basis AS b
		          WHERE b.tenant_id = c.tenant_id
		            AND b.charge_id = c.charge_id
		            AND b.fee_item = c.fee_item)`
)

func (catalogue *ChargeCatalogue) ListCustomerCharges(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.CustomerChargeCatalogueRow], error) {
	fail := func(err error) (cataloguepage.Result[ports.CustomerChargeCatalogueRow], error) {
		return cataloguepage.Result[ports.CustomerChargeCatalogueRow]{}, fmt.Errorf("list customer charges: %w", err)
	}
	if limit < 1 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	plan, err := customerChargePages.Plan(query, "c.tenant_id = $1", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	var total int64
	if err := querier.QueryRow(ctx, plan.CountSQL(customerChargeTable), plan.Args...).Scan(&total); err != nil {
		return fail(fmt.Errorf("count: %w", err))
	}
	rows, err := querier.Query(ctx, plan.PageSQL(customerChargeColumns, customerChargeSource), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	entries := make([]ports.CustomerChargeCatalogueRow, 0, limit)
	for rows.Next() {
		var (
			row               ports.CustomerChargeCatalogueRow
			conversionStep    *string
			confirmationBasis *string
			requiredBasisKind *string
			attributionDate   *string
			billedOn          *string
			formedAt          time.Time
			confirmedAt       *time.Time
			basesJSON         []byte
			facts             confirmationFactColumns
		)
		if err := rows.Scan(
			&row.Charge, &row.FeeItem, &row.Evaluation, &row.Stage,
			&row.OriginalCurrency, &row.OriginalMinor,
			&row.SettlementCurrency, &row.SettlementMinor,
			&conversionStep, &confirmationBasis, &formedAt, &confirmedAt,
			&facts.responsibleEntity, &facts.counterparty, &facts.direction,
			&facts.settlementAccount, &facts.contractBasis, &facts.primaryChargingScope,
			&facts.sourceFact,
			&attributionDate, &billedOn,
			&requiredBasisKind,
			&basesJSON,
		); err != nil {
			return fail(err)
		}
		row.ConversionStep = catalogueText(conversionStep)
		row.ConfirmationBasis = catalogueText(confirmationBasis)
		// 七项在非确认行上一律为空，读面照实交空串——库上那条同在或同缺的 CHECK 保证
		// 这不是缺数据而是「这一行还没确认」，读面不代填也不合并成一句话。
		row.ResponsibleEntity = catalogueText(facts.responsibleEntity)
		row.Counterparty = catalogueText(facts.counterparty)
		row.ChargeDirection = catalogueText(facts.direction)
		row.SettlementAccount = catalogueText(facts.settlementAccount)
		row.ContractBasis = catalogueText(facts.contractBasis)
		row.PrimaryChargingScope = catalogueText(facts.primaryChargingScope)
		row.SourceFact = catalogueText(facts.sourceFact)
		row.RequiredBasisKind = catalogueText(requiredBasisKind)
		// 归属日与所在对账单给运营列截单清单用（ADR-0339 决定三）。所在对账单的判据与截单的出账锁（LockChargeBilling）
		// 同一条：未作废的已发布单的行里有这笔费用，取法也同，按发布时刻与单号取第一张。截单不读这两格，照旧当场判。
		row.AttributionDate = catalogueText(attributionDate)
		row.BilledOn = catalogueText(billedOn)
		row.FormedAt = formedAt.UTC()
		row.ConfirmedAt = catalogueInstant(confirmedAt)

		var documents []confirmationBasisDocument
		if err := json.Unmarshal(basesJSON, &documents); err != nil {
			return fail(fmt.Errorf("确认依据集解码：%w", err))
		}
		bases := make([]ports.ChargeConfirmationBasisEntry, 0, len(documents))
		for _, document := range documents {
			bases = append(bases, ports.ChargeConfirmationBasisEntry{
				BasisKind:  document.BasisKind,
				Basis:      document.Basis,
				RecordedAt: document.RecordedAt.UTC(),
			})
		}
		row.ConfirmationBases = bases
		entries = append(entries, row)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	page, err := cataloguepage.Finish(query, entries, limit, total,
		func(row ports.CustomerChargeCatalogueRow, sortField string) cataloguepage.Position {
			position := cataloguepage.Position{Value: row.Charge, Identity: []string{row.Charge}}
			if sortField == "formedAt" {
				position.Value = cataloguepage.FormatInstant(row.FormedAt)
			}
			return position
		})
	if err != nil {
		return fail(err)
	}
	return page, nil
}

// supplierExpectedCostPages 把预期成本版本册的声明落到列上。缺省序把同一（发生项＋费用项目）的版本链排在一起、链内按版本
// 升序：纠错版本与被它纠正的那一版隔着别的链会让「这是重述不是新成本」在页面上看不出来。行标识因此带上费用项目一段
// （见 ports.SupplierExpectedCostPages）。
var supplierExpectedCostPages = ports.SupplierExpectedCostPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"occurrence": "occurrence_id", "recordedAt": "recorded_at"},
	Identity: []string{"fee_item", "version"},
	Keyword:  []string{"version", "occurrence_id", "fee_item", "agreement_ref", "supplier_party_ref", "evaluation_ref"},
})

func (catalogue *ChargeCatalogue) ListSupplierExpectedCosts(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.SupplierExpectedCostCatalogueRow], error) {
	fail := func(err error) (cataloguepage.Result[ports.SupplierExpectedCostCatalogueRow], error) {
		return cataloguepage.Result[ports.SupplierExpectedCostCatalogueRow]{}, fmt.Errorf("list supplier expected costs: %w", err)
	}
	if limit < 1 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	plan, err := supplierExpectedCostPages.Plan(query, "tenant_id = $1", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	const source = `settlement_accounting.supplier_expected_cost`
	var total int64
	if err := querier.QueryRow(ctx, plan.CountSQL(source), plan.Args...).Scan(&total); err != nil {
		return fail(fmt.Errorf("count: %w", err))
	}
	rows, err := querier.Query(ctx, plan.PageSQL(
		`version, occurrence_id, occurrence_reason, occurrence_version, occurred_at,
		        fee_item, purchase_rule_version, agreement_ref, supplier_party_ref, evaluation_ref,
		        original_currency, original_minor,
		        settlement_currency, settlement_minor,
		        conversion_ref, prior_version, correction_reason, withdrawn, recorded_at`, source), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	entries := make([]ports.SupplierExpectedCostCatalogueRow, 0, limit)
	for rows.Next() {
		var (
			row              ports.SupplierExpectedCostCatalogueRow
			supplier         *string
			conversionStep   *string
			priorVersion     *string
			correctionReason *string
			occurredAt       time.Time
			recordedAt       time.Time
		)
		if err := rows.Scan(
			&row.Version, &row.Occurrence, &row.OccurrenceReason, &row.OccurrenceVersion, &occurredAt,
			&row.FeeItem, &row.PurchaseRuleVersion, &row.Agreement, &supplier, &row.Evaluation,
			&row.OriginalCurrency, &row.OriginalMinor,
			&row.SettlementCurrency, &row.SettlementMinor,
			&conversionStep, &priorVersion, &correctionReason, &row.Withdrawn, &recordedAt,
		); err != nil {
			return fail(err)
		}
		row.Supplier = catalogueText(supplier)
		row.ConversionStep = catalogueText(conversionStep)
		row.PriorVersion = catalogueText(priorVersion)
		row.CorrectionReason = catalogueText(correctionReason)
		row.OccurredAt = occurredAt.UTC()
		row.RecordedAt = recordedAt.UTC()
		entries = append(entries, row)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	page, err := cataloguepage.Finish(query, entries, limit, total,
		func(row ports.SupplierExpectedCostCatalogueRow, sortField string) cataloguepage.Position {
			position := cataloguepage.Position{Value: row.Occurrence, Identity: []string{row.FeeItem, row.Version}}
			if sortField == "recordedAt" {
				position.Value = cataloguepage.FormatInstant(row.RecordedAt)
			}
			return position
		})
	if err != nil {
		return fail(err)
	}
	return page, nil
}
