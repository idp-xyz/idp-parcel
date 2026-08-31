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

// FundsApplicationCatalogue 实现 ports.FundsApplicationCatalogueRead：已采用外部资金
// 事实册的列表读面，映射与核销挂在事实行上（ADR-0077，票
// admin-skeleton-closure-batch/04）。读的就是 0004 的三张本表。
//
// 事实面、两笔核销求和与两个挂册**一条语句取回**：ReadExecutor 不保证两条语句同一
// 快照，分次取会拼出从未同时存在的账面状态——与核销并发登记时尤甚（判据同
// ChargeCatalogue）。求和走 LATERAL 一次扫过该事实的全部核销，映射与核销明细各走
// 一个相关子查询 json_agg，各聚各的。
//
// **已核销与已撤销分两笔求和，不给净额。** 「从未核销过」与「核销过又撤销了」在一个
// 净额上长着同一张脸，而两态的续办相反：前者要人去分配，后者要人去看当初为什么撤。
// 撤销不删历史，那一截金额因此必须仍然看得见。
type FundsApplicationCatalogue struct {
	db *bentopg.DB
}

func NewFundsApplicationCatalogue(db *bentopg.DB) (*FundsApplicationCatalogue, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &FundsApplicationCatalogue{db: db}, nil
}

var _ ports.FundsApplicationCatalogueRead = (*FundsApplicationCatalogue)(nil)

// fundsMappingDocument 与 settlementApplicationDocument 是两个挂册的临时词形。
// 撤销两件在库上可空，词形留指针格（理由同 statementDisputeDocument）。
type fundsMappingDocument struct {
	Mapping    string    `json:"mapping"`
	TargetKind string    `json:"targetKind"`
	Target     string    `json:"target"`
	Basis      string    `json:"basis"`
	MappedAt   time.Time `json:"mappedAt"`
}

type settlementApplicationDocument struct {
	Application     string     `json:"application"`
	AppliedMinor    int64      `json:"appliedMinor"`
	Basis           string     `json:"basis"`
	AppliedAt       time.Time  `json:"appliedAt"`
	AllocationCount int64      `json:"allocationCount"`
	ReversalBasis   *string    `json:"reversalBasis"`
	ReversedAt      *time.Time `json:"reversedAt"`
}

func (catalogue *FundsApplicationCatalogue) ListExternalFundsFacts(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.ExternalFundsFactCatalogueRow, error) {
	if limit < 1 {
		return nil, fmt.Errorf("list external funds facts: limit must be positive, got %d", limit)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list external funds facts: %w", err)
	}

	// SUM(bigint) 在 PG 里是 numeric，显式收窄回 bigint——金额语义就是最小单位整数，
	// 让类型在语句里说这句话，超界在库内报错而不是静默进小数。
	rows, err := querier.Query(ctx,
		`SELECT f.fact_id, f.source_ref, f.kind, f.currency, f.amount_minor, f.version,
		        f.occurred_at, f.corrects, f.corrected_at,
		        applied.applied_minor, applied.reversed_minor, applied.application_count,
		        (SELECT COALESCE(
		                    json_agg(
		                        json_build_object(
		                            'mapping',    m.mapping_id,
		                            'targetKind', m.target_kind,
		                            'target',     m.target_ref,
		                            'basis',      m.basis,
		                            'mappedAt',   m.mapped_at
		                        )
		                        ORDER BY m.mapping_id
		                    ),
		                    '[]'::json
		                )
		           FROM settlement_accounting.funds_mapping AS m
		          WHERE m.tenant_id = f.tenant_id
		            AND m.fact_id = f.fact_id),
		        (SELECT COALESCE(
		                    json_agg(
		                        json_build_object(
		                            'application',     a.application_id,
		                            'appliedMinor',    a.applied_minor,
		                            'basis',           a.basis,
		                            'appliedAt',       a.applied_at,
		                            'allocationCount', jsonb_array_length(a.allocations),
		                            'reversalBasis',   a.reversal_basis,
		                            'reversedAt',      a.reversed_at
		                        )
		                        ORDER BY a.application_id
		                    ),
		                    '[]'::json
		                )
		           FROM settlement_accounting.settlement_application AS a
		          WHERE a.tenant_id = f.tenant_id
		            AND a.fact_id = f.fact_id)
		   FROM settlement_accounting.external_funds_fact AS f
		   LEFT JOIN LATERAL (
		        SELECT
		            COALESCE(SUM(a.applied_minor) FILTER (WHERE a.reversed_at IS NULL),
		                     0)::bigint AS applied_minor,
		            COALESCE(SUM(a.applied_minor) FILTER (WHERE a.reversed_at IS NOT NULL),
		                     0)::bigint AS reversed_minor,
		            COUNT(*)::bigint AS application_count
		          FROM settlement_accounting.settlement_application AS a
		         WHERE a.tenant_id = f.tenant_id
		           AND a.fact_id = f.fact_id
		   ) AS applied ON true
		  WHERE f.tenant_id = $1
		  ORDER BY f.fact_id
		  LIMIT $2`,
		tenant.String(), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list external funds facts: %w", err)
	}
	defer rows.Close()

	entries := make([]ports.ExternalFundsFactCatalogueRow, 0, limit)
	for rows.Next() {
		var (
			row              ports.ExternalFundsFactCatalogueRow
			occurredAt       time.Time
			corrects         *string
			correctedAt      *time.Time
			mappingsJSON     []byte
			applicationsJSON []byte
		)
		if err := rows.Scan(
			&row.Fact, &row.Source, &row.Kind, &row.Currency, &row.AmountMinor, &row.Version,
			&occurredAt, &corrects, &correctedAt,
			&row.AppliedMinor, &row.ReversedMinor, &row.ApplicationCount,
			&mappingsJSON, &applicationsJSON,
		); err != nil {
			return nil, fmt.Errorf("list external funds facts: %w", err)
		}
		row.OccurredAt = occurredAt.UTC()
		row.Corrects = catalogueText(corrects)
		row.CorrectedAt = catalogueInstant(correctedAt)
		// 未核销余额只扣未撤销那一笔：撤销把金额退回未结，它不该继续算作已分配。
		row.UnappliedMinor = row.AmountMinor - row.AppliedMinor

		var mappingDocuments []fundsMappingDocument
		if err := json.Unmarshal(mappingsJSON, &mappingDocuments); err != nil {
			return nil, fmt.Errorf("list external funds facts: 映射集解码：%w", err)
		}
		mappings := make([]ports.FundsMappingEntry, 0, len(mappingDocuments))
		for _, document := range mappingDocuments {
			mappings = append(mappings, ports.FundsMappingEntry{
				Mapping:    document.Mapping,
				TargetKind: document.TargetKind,
				Target:     document.Target,
				Basis:      document.Basis,
				MappedAt:   document.MappedAt.UTC(),
			})
		}
		row.Mappings = mappings

		var applicationDocuments []settlementApplicationDocument
		if err := json.Unmarshal(applicationsJSON, &applicationDocuments); err != nil {
			return nil, fmt.Errorf("list external funds facts: 核销集解码：%w", err)
		}
		applications := make([]ports.SettlementApplicationEntry, 0, len(applicationDocuments))
		for _, document := range applicationDocuments {
			applications = append(applications, ports.SettlementApplicationEntry{
				Application:     document.Application,
				AppliedMinor:    document.AppliedMinor,
				Basis:           document.Basis,
				AppliedAt:       document.AppliedAt.UTC(),
				AllocationCount: document.AllocationCount,
				ReversalBasis:   catalogueText(document.ReversalBasis),
				ReversedAt:      catalogueInstant(document.ReversedAt),
			})
		}
		row.Applications = applications

		entries = append(entries, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list external funds facts: %w", err)
	}
	return entries, nil
}
