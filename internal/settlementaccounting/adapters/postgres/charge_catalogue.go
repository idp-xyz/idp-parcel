package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

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
// 排序按登记身份升序保证分页可重复。limit 非正是调用方编程错误。
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

func (catalogue *ChargeCatalogue) ListCustomerCharges(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.CustomerChargeCatalogueRow, error) {
	if limit < 1 {
		return nil, fmt.Errorf("list customer charges: limit must be positive, got %d", limit)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list customer charges: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT c.charge_id, c.fee_item, c.evaluation_ref, c.stage,
		        c.original_currency, c.original_minor,
		        c.settlement_currency, c.settlement_minor,
		        c.conversion_ref, c.confirmation_basis, c.formed_at, c.confirmed_at,
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
		            AND b.fee_item = c.fee_item)
		   FROM settlement_accounting.customer_charge AS c
		   LEFT JOIN settlement_accounting.charge_confirmation_condition AS req
		          ON req.tenant_id = c.tenant_id
		         AND req.fee_item = c.fee_item
		  WHERE c.tenant_id = $1
		  ORDER BY c.charge_id
		  LIMIT $2`,
		tenant.String(), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list customer charges: %w", err)
	}
	defer rows.Close()

	entries := make([]ports.CustomerChargeCatalogueRow, 0, limit)
	for rows.Next() {
		var (
			row               ports.CustomerChargeCatalogueRow
			conversionStep    *string
			confirmationBasis *string
			requiredBasisKind *string
			formedAt          time.Time
			confirmedAt       *time.Time
			basesJSON         []byte
		)
		if err := rows.Scan(
			&row.Charge, &row.FeeItem, &row.Evaluation, &row.Stage,
			&row.OriginalCurrency, &row.OriginalMinor,
			&row.SettlementCurrency, &row.SettlementMinor,
			&conversionStep, &confirmationBasis, &formedAt, &confirmedAt,
			&requiredBasisKind,
			&basesJSON,
		); err != nil {
			return nil, fmt.Errorf("list customer charges: %w", err)
		}
		row.ConversionStep = catalogueText(conversionStep)
		row.ConfirmationBasis = catalogueText(confirmationBasis)
		row.RequiredBasisKind = catalogueText(requiredBasisKind)
		row.FormedAt = formedAt.UTC()
		row.ConfirmedAt = catalogueInstant(confirmedAt)

		var documents []confirmationBasisDocument
		if err := json.Unmarshal(basesJSON, &documents); err != nil {
			return nil, fmt.Errorf("list customer charges: 确认依据集解码：%w", err)
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
		return nil, fmt.Errorf("list customer charges: %w", err)
	}
	return entries, nil
}

func (catalogue *ChargeCatalogue) ListSupplierExpectedCosts(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.SupplierExpectedCostCatalogueRow, error) {
	if limit < 1 {
		return nil, fmt.Errorf("list supplier expected costs: limit must be positive, got %d", limit)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list supplier expected costs: %w", err)
	}

	// 排序把同一（发生项＋费用项目）的版本链排在一起、链内按版本升序：纠错版本与被它
	// 纠正的那一版分居两页会让「这是重述不是新成本」在页面上看不出来。
	rows, err := querier.Query(ctx,
		`SELECT version, occurrence_id, occurrence_reason, occurrence_version, occurred_at,
		        fee_item, purchase_rule_version, agreement_ref, evaluation_ref,
		        original_currency, original_minor,
		        settlement_currency, settlement_minor,
		        conversion_ref, prior_version, correction_reason, recorded_at
		   FROM settlement_accounting.supplier_expected_cost
		  WHERE tenant_id = $1
		  ORDER BY occurrence_id, fee_item, version
		  LIMIT $2`,
		tenant.String(), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list supplier expected costs: %w", err)
	}
	defer rows.Close()

	entries := make([]ports.SupplierExpectedCostCatalogueRow, 0, limit)
	for rows.Next() {
		var (
			row              ports.SupplierExpectedCostCatalogueRow
			conversionStep   *string
			priorVersion     *string
			correctionReason *string
			occurredAt       time.Time
			recordedAt       time.Time
		)
		if err := rows.Scan(
			&row.Version, &row.Occurrence, &row.OccurrenceReason, &row.OccurrenceVersion, &occurredAt,
			&row.FeeItem, &row.PurchaseRuleVersion, &row.Agreement, &row.Evaluation,
			&row.OriginalCurrency, &row.OriginalMinor,
			&row.SettlementCurrency, &row.SettlementMinor,
			&conversionStep, &priorVersion, &correctionReason, &recordedAt,
		); err != nil {
			return nil, fmt.Errorf("list supplier expected costs: %w", err)
		}
		row.ConversionStep = catalogueText(conversionStep)
		row.PriorVersion = catalogueText(priorVersion)
		row.CorrectionReason = catalogueText(correctionReason)
		row.OccurredAt = occurredAt.UTC()
		row.RecordedAt = recordedAt.UTC()
		entries = append(entries, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list supplier expected costs: %w", err)
	}
	return entries, nil
}
