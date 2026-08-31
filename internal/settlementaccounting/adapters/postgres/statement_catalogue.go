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

// StatementCatalogue 实现 ports.StatementCatalogueRead：已发布对账单册与供应商账单
// 接收册的列表读面（ADR-0077，票 admin-skeleton-closure-batch/04）。读的就是 0002 与
// 0007 的本表。
//
// 对账单一行**一条语句取回**单面、异议与后续纳入（同一快照，理由同 ChargeCatalogue）。
// 异议与纳入各走一个相关子查询 json_agg，各聚各的，不互相做笛卡尔积。
//
// 费用行与调整行只取长度不取内容：明细属详情面，目录行要的是「有几行」与「一行都
// 没有」分得开。长度在库内用 jsonb_array_length 求，不把整列 jsonb 搬到进程里再数
// ——搬回来的那一份除了被数一次没有别的用处。
type StatementCatalogue struct {
	db *bentopg.DB
}

func NewStatementCatalogue(db *bentopg.DB) (*StatementCatalogue, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &StatementCatalogue{db: db}, nil
}

var _ ports.StatementCatalogueRead = (*StatementCatalogue)(nil)

// statementDisputeDocument 与 subsequentInclusionDocument 是两个聚合的临时词形。
// 裁定三件与纳入的调整回指在库上可空，词形跟着留指针格——把它们折成空串会在解码
// 这一层就把「未裁定」与「裁定为空串」混掉，而库上的非空白门只管非 NULL 的那一侧。
type statementDisputeDocument struct {
	Dispute       string     `json:"dispute"`
	Charge        string     `json:"charge"`
	DisputedMinor int64      `json:"disputedMinor"`
	Reason        string     `json:"reason"`
	OpenedAt      time.Time  `json:"openedAt"`
	Resolution    *string    `json:"resolution"`
	ResolutionRef *string    `json:"resolutionRef"`
	ResolvedAt    *time.Time `json:"resolvedAt"`
}

type subsequentInclusionDocument struct {
	Inclusion        string    `json:"inclusion"`
	Kind             string    `json:"kind"`
	OriginalPeriod   string    `json:"originalPeriod"`
	SubsequentPeriod string    `json:"subsequentPeriod"`
	Charge           string    `json:"charge"`
	Adjustment       *string   `json:"adjustment"`
	IncludedAt       time.Time `json:"includedAt"`
}

func (catalogue *StatementCatalogue) ListCustomerStatements(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.CustomerStatementCatalogueRow, error) {
	if limit < 1 {
		return nil, fmt.Errorf("list customer statements: limit must be positive, got %d", limit)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list customer statements: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT s.statement_number, s.account_id, s.period_ref, s.currency, s.total_minor,
		        jsonb_array_length(s.lines)::bigint,
		        jsonb_array_length(s.adjustment_lines)::bigint,
		        s.published_at, s.void_basis, s.voided_at,
		        (SELECT COALESCE(
		                    json_agg(
		                        json_build_object(
		                            'dispute',       d.dispute_id,
		                            'charge',        d.charge_id,
		                            'disputedMinor', d.disputed_minor,
		                            'reason',        d.reason_ref,
		                            'openedAt',      d.opened_at,
		                            'resolution',    d.resolution,
		                            'resolutionRef', d.resolution_ref,
		                            'resolvedAt',    d.resolved_at
		                        )
		                        ORDER BY d.dispute_id
		                    ),
		                    '[]'::json
		                )
		           FROM settlement_accounting.statement_dispute AS d
		          WHERE d.tenant_id = s.tenant_id
		            AND d.statement_number = s.statement_number),
		        (SELECT COALESCE(
		                    json_agg(
		                        json_build_object(
		                            'inclusion',        i.inclusion_id,
		                            'kind',             i.kind,
		                            'originalPeriod',   i.original_period,
		                            'subsequentPeriod', i.subsequent_period,
		                            'charge',           i.charge_id,
		                            'adjustment',       i.adjustment_id,
		                            'includedAt',       i.included_at
		                        )
		                        ORDER BY i.inclusion_id
		                    ),
		                    '[]'::json
		                )
		           FROM settlement_accounting.subsequent_inclusion AS i
		          WHERE i.tenant_id = s.tenant_id
		            AND i.statement_number = s.statement_number)
		   FROM settlement_accounting.customer_statement AS s
		  WHERE s.tenant_id = $1
		  ORDER BY s.statement_number
		  LIMIT $2`,
		tenant.String(), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list customer statements: %w", err)
	}
	defer rows.Close()

	entries := make([]ports.CustomerStatementCatalogueRow, 0, limit)
	for rows.Next() {
		var (
			row            ports.CustomerStatementCatalogueRow
			publishedAt    time.Time
			voidBasis      *string
			voidedAt       *time.Time
			disputesJSON   []byte
			inclusionsJSON []byte
		)
		if err := rows.Scan(
			&row.StatementNumber, &row.Account, &row.Period, &row.Currency, &row.TotalMinor,
			&row.LineCount, &row.AdjustmentCount,
			&publishedAt, &voidBasis, &voidedAt,
			&disputesJSON, &inclusionsJSON,
		); err != nil {
			return nil, fmt.Errorf("list customer statements: %w", err)
		}
		row.PublishedAt = publishedAt.UTC()
		row.VoidBasis = catalogueText(voidBasis)
		row.VoidedAt = catalogueInstant(voidedAt)

		var disputeDocuments []statementDisputeDocument
		if err := json.Unmarshal(disputesJSON, &disputeDocuments); err != nil {
			return nil, fmt.Errorf("list customer statements: 异议集解码：%w", err)
		}
		disputes := make([]ports.StatementDisputeEntry, 0, len(disputeDocuments))
		for _, document := range disputeDocuments {
			disputes = append(disputes, ports.StatementDisputeEntry{
				Dispute:       document.Dispute,
				Charge:        document.Charge,
				DisputedMinor: document.DisputedMinor,
				Reason:        document.Reason,
				OpenedAt:      document.OpenedAt.UTC(),
				Resolution:    catalogueText(document.Resolution),
				ResolutionRef: catalogueText(document.ResolutionRef),
				ResolvedAt:    catalogueInstant(document.ResolvedAt),
			})
		}
		row.Disputes = disputes

		var inclusionDocuments []subsequentInclusionDocument
		if err := json.Unmarshal(inclusionsJSON, &inclusionDocuments); err != nil {
			return nil, fmt.Errorf("list customer statements: 后续纳入集解码：%w", err)
		}
		inclusions := make([]ports.SubsequentInclusionEntry, 0, len(inclusionDocuments))
		for _, document := range inclusionDocuments {
			inclusions = append(inclusions, ports.SubsequentInclusionEntry{
				Inclusion:        document.Inclusion,
				Kind:             document.Kind,
				OriginalPeriod:   document.OriginalPeriod,
				SubsequentPeriod: document.SubsequentPeriod,
				Charge:           document.Charge,
				Adjustment:       catalogueText(document.Adjustment),
				IncludedAt:       document.IncludedAt.UTC(),
			})
		}
		row.SubsequentInclusions = inclusions

		entries = append(entries, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list customer statements: %w", err)
	}
	return entries, nil
}

func (catalogue *StatementCatalogue) ListSupplierBillReceptions(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.SupplierBillReceptionCatalogueRow, error) {
	if limit < 1 {
		return nil, fmt.Errorf("list supplier bill receptions: limit must be positive, got %d", limit)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list supplier bill receptions: %w", err)
	}

	// 主张身份与版本同键排序：同一主张的多个版本相邻，版本冲突在页面上才看得出来。
	rows, err := querier.Query(ctx,
		`SELECT claim_id, claim_version, supplier_ref, legal_entity, period_ref, currency,
		        jsonb_array_length(claim -> 'lines')::bigint,
		        jsonb_array_length(matches)::bigint,
		        audit_authority_configured, recorded_at
		   FROM settlement_accounting.supplier_bill_reception
		  WHERE tenant_id = $1
		  ORDER BY claim_id, claim_version
		  LIMIT $2`,
		tenant.String(), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list supplier bill receptions: %w", err)
	}
	defer rows.Close()

	entries := make([]ports.SupplierBillReceptionCatalogueRow, 0, limit)
	for rows.Next() {
		var (
			row        ports.SupplierBillReceptionCatalogueRow
			recordedAt time.Time
		)
		if err := rows.Scan(
			&row.Claim, &row.ClaimVersion, &row.Supplier, &row.LegalEntity,
			&row.Period, &row.Currency,
			&row.LineCount, &row.MatchCount,
			&row.AuditAuthorityConfigured, &recordedAt,
		); err != nil {
			return nil, fmt.Errorf("list supplier bill receptions: %w", err)
		}
		row.RecordedAt = recordedAt.UTC()
		entries = append(entries, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list supplier bill receptions: %w", err)
	}
	return entries, nil
}
