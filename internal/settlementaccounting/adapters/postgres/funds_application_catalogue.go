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

// FundsApplicationCatalogue 实现 ports.FundsApplicationCatalogueRead：已采用外部资金
// 事实册的列表读面，映射与核销挂在事实行上（ADR-0077，票
// admin-skeleton-closure-batch/04）。读的是 0004 的映射与核销两表加 0021 的版本子表。
//
// **一事实一行，行上是链头那一版的内容。** 0021 起一事实多版各占一行（票 sa-cc/20），而映射与核销挂
// 在事实身份上，不挂在某一版上——按事实身份关联的聚合若逐版本行各出一遍，两笔求和与两个挂册就会
// 按版本数翻倍；所以事实面只取没有后继的那一版，Version / Corrects / CorrectedAt 三列就是它在链上的
// 位置。前版的内容不在这张列表上：它是历史，按版本读走 AdoptedFundsFactView。
//
// 事实面、两笔核销求和与两个挂册**一条语句取回**：ReadExecutor 不保证两条语句同一
// 快照，分次取会拼出从未同时存在的账面状态——与核销并发登记时尤甚（判据同
// ChargeCatalogue）。求和走 LATERAL 一次扫过该事实的全部核销，映射与核销明细各走
// 一个相关子查询 json_agg，各聚各的。按 ports.ExternalFundsFactPages 取页时总数另数
// 一句，与取页不在同一快照里，是此刻的大概。
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

// externalFundsFactPages 把资金事实册的声明落到列上。固定条件只留链头那一版，总数与取页同一个条件。
var externalFundsFactPages = ports.ExternalFundsFactPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"fact": "f.fact_id", "occurredAt": "f.occurred_at"},
	Identity: []string{"f.fact_id", "f.version"},
	Filters:  map[string]string{"kind": "f.kind"},
	Keyword:  []string{"f.fact_id", "f.source_ref"},
})

const (
	externalFundsFactTable     = `settlement_accounting.external_funds_fact_version AS f`
	externalFundsFactCondition = `f.tenant_id = $1
		    AND NOT EXISTS (
		        SELECT 1
		          FROM settlement_accounting.external_funds_fact_version AS successor
		         WHERE successor.tenant_id = f.tenant_id
		           AND successor.fact_id = f.fact_id
		           AND successor.corrects = f.version)`
)

func (catalogue *FundsApplicationCatalogue) ListExternalFundsFacts(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.ExternalFundsFactCatalogueRow], error) {
	fail := func(err error) (cataloguepage.Result[ports.ExternalFundsFactCatalogueRow], error) {
		return cataloguepage.Result[ports.ExternalFundsFactCatalogueRow]{}, fmt.Errorf("list external funds facts: %w", err)
	}
	if limit < 1 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	plan, err := externalFundsFactPages.Plan(query, externalFundsFactCondition, []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	var total int64
	if err := querier.QueryRow(ctx, plan.CountSQL(externalFundsFactTable), plan.Args...).Scan(&total); err != nil {
		return fail(fmt.Errorf("count: %w", err))
	}

	// SUM(bigint) 在 PG 里是 numeric，显式收窄回 bigint——金额语义就是最小单位整数，
	// 让类型在语句里说这句话，超界在库内报错而不是静默进小数。
	rows, err := querier.Query(ctx, plan.PageSQL(
		`f.fact_id, f.source_ref, f.kind, f.currency, f.amount_minor, f.version,
		        f.occurred_at, f.corrects, f.corrected_at, f.operation_evidence,
		        applied.applied_minor, applied.reversed_minor, applied.application_count,
		        (SELECT COUNT(DISTINCT mapped.account_id)
		           FROM (`+mappingReceivableScopesSQL+`) AS mapped
		          WHERE mapped.fact_id = f.fact_id),
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
		            AND a.fact_id = f.fact_id)`,
		externalFundsFactTable+`
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
		   ) AS applied ON true`), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	entries := make([]ports.ExternalFundsFactCatalogueRow, 0, limit)
	for rows.Next() {
		var (
			row              ports.ExternalFundsFactCatalogueRow
			occurredAt       time.Time
			corrects         *string
			correctedAt      *time.Time
			evidence         *string
			mappingsJSON     []byte
			applicationsJSON []byte
		)
		if err := rows.Scan(
			&row.Fact, &row.Source, &row.Kind, &row.Currency, &row.AmountMinor, &row.Version,
			&occurredAt, &corrects, &correctedAt, &evidence,
			&row.AppliedMinor, &row.ReversedMinor, &row.ApplicationCount, &row.CustomerScopeCount,
			&mappingsJSON, &applicationsJSON,
		); err != nil {
			return fail(err)
		}
		row.OccurredAt = occurredAt.UTC()
		row.Corrects = catalogueText(corrects)
		row.CorrectedAt = catalogueInstant(correctedAt)
		row.OperationEvidence = catalogueText(evidence)
		// 未核销余额只扣未撤销那一笔：撤销把金额退回未结，它不该继续算作已分配。外部更正把金额改到已核销合计以下时
		// 它为负，读面上超核就是这个负数，不截成 0。
		row.UnappliedMinor = row.AmountMinor - row.AppliedMinor
		// 落到几个客户应收作用域的事实按核销记入，没核销的那一截不归任何一个作用域（ADR-0283）；判法与
		// CustomerReceiptPostings.CustomerDirectionNets 同一个作用域口径。超核时那一截是 0，超核本身已由上一格的负数说出。
		if row.CustomerScopeCount > 1 && row.UnappliedMinor > 0 {
			row.UnscopedUnappliedMinor = row.UnappliedMinor
		}

		var mappingDocuments []fundsMappingDocument
		if err := json.Unmarshal(mappingsJSON, &mappingDocuments); err != nil {
			return fail(fmt.Errorf("映射集解码：%w", err))
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
			return fail(fmt.Errorf("核销集解码：%w", err))
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
		return fail(err)
	}
	page, err := cataloguepage.Finish(query, entries, limit, total,
		func(row ports.ExternalFundsFactCatalogueRow, sortField string) cataloguepage.Position {
			position := cataloguepage.Position{Value: row.Fact, Identity: []string{row.Fact, row.Version}}
			if sortField == "occurredAt" {
				position.Value = cataloguepage.FormatInstant(row.OccurredAt)
			}
			return position
		})
	if err != nil {
		return fail(err)
	}
	return page, nil
}
