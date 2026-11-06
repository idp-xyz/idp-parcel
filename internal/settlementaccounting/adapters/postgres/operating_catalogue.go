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

// OperatingCatalogue 实现 ports.OperatingCatalogueRead：经营结果快照册与成本分摊册的
// 列表读面（ADR-0077，票 admin-skeleton-closure-batch/04）。读的就是 0005 的两张本表。
//
// 组成与份额两列 jsonb 原样取回再逐项转写，**不在库内求和**：毛利与未分摊余额都已经
// 是表上的列，读口重算一遍就成了第二处定义，且两处一旦不一致，页面上看到的会是读口
// 那个没人验过的数。写口重建时复验组成与毛利是否相符，那道门在写侧，读口不代守。
//
// 两册各一条语句：它们之间没有行级从属关系（分摊不挂在某个指标快照下），拼成一条
// 只会造出一个笛卡尔积。两册各按自己的声明取页（ports.OperatingResultPages、
// ports.CostAllocationPages），总数另数一句，是此刻的大概。
type OperatingCatalogue struct {
	db *bentopg.DB
}

func NewOperatingCatalogue(db *bentopg.DB) (*OperatingCatalogue, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &OperatingCatalogue{db: db}, nil
}

var _ ports.OperatingCatalogueRead = (*OperatingCatalogue)(nil)

// operatingResultPages 把经营结果册的声明落到列上。缺省序按口径三件（范围、账期、基准）升序：同一范围同一账期的三个口径
// 相邻，「预估与已确认差在哪」在页面上才对得起来。
var operatingResultPages = ports.OperatingResultPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"scope": "scope_ref", "recordedAt": "recorded_at"},
	Identity: []string{"scope_ref", "period_ref", "basis"},
	Filters:  map[string]string{"basis": "basis"},
	Keyword:  []string{"scope_ref", "period_ref", "version"},
})

func (catalogue *OperatingCatalogue) ListOperatingResults(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.OperatingResultCatalogueRow], error) {
	fail := func(err error) (cataloguepage.Result[ports.OperatingResultCatalogueRow], error) {
		return cataloguepage.Result[ports.OperatingResultCatalogueRow]{}, fmt.Errorf("list operating results: %w", err)
	}
	if limit < 1 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	plan, err := operatingResultPages.Plan(query, "tenant_id = $1", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	const source = `settlement_accounting.operating_result`
	var total int64
	if err := querier.QueryRow(ctx, plan.CountSQL(source), plan.Args...).Scan(&total); err != nil {
		return fail(fmt.Errorf("count: %w", err))
	}
	rows, err := querier.Query(ctx, plan.PageSQL(
		`scope_ref, period_ref, basis, currency, margin_minor, version, as_of,
		        corrects, recorded_at, components`, source), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	entries := make([]ports.OperatingResultCatalogueRow, 0, limit)
	for rows.Next() {
		var (
			row            ports.OperatingResultCatalogueRow
			asOf           time.Time
			corrects       *string
			recordedAt     time.Time
			componentsJSON []byte
		)
		if err := rows.Scan(
			&row.Scope, &row.Period, &row.Basis, &row.Currency, &row.MarginMinor,
			&row.Version, &asOf, &corrects, &recordedAt, &componentsJSON,
		); err != nil {
			return fail(err)
		}
		row.AsOf = asOf.UTC()
		row.Corrects = catalogueText(corrects)
		row.RecordedAt = recordedAt.UTC()

		// 词形复用写侧的 componentRow：组成列由它写下，读回换一套字段名就等于在同一
		// 列上摆了两份契约，其中一份没有任何东西在守。
		var documents []componentRow
		if err := json.Unmarshal(componentsJSON, &documents); err != nil {
			return fail(fmt.Errorf("组成集解码：%w", err))
		}
		components := make([]ports.OperatingComponentEntry, 0, len(documents))
		for _, document := range documents {
			components = append(components, ports.OperatingComponentEntry{
				Source:      document.Source,
				Role:        document.Role,
				Effect:      document.Effect,
				AmountMinor: document.AmountMinor,
			})
		}
		row.Components = components
		entries = append(entries, row)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	page, err := cataloguepage.Finish(query, entries, limit, total,
		func(row ports.OperatingResultCatalogueRow, sortField string) cataloguepage.Position {
			position := cataloguepage.Position{Value: row.Scope, Identity: []string{row.Scope, row.Period, row.Basis}}
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

// costAllocationPages 把成本分摊册的声明落到列上。
var costAllocationPages = ports.CostAllocationPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"allocation": "allocation_id", "recordedAt": "recorded_at"},
	Identity: []string{"allocation_id"},
	Keyword:  []string{"allocation_id", "source_ref", "rule_ref"},
})

func (catalogue *OperatingCatalogue) ListCostAllocations(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.CostAllocationCatalogueRow], error) {
	fail := func(err error) (cataloguepage.Result[ports.CostAllocationCatalogueRow], error) {
		return cataloguepage.Result[ports.CostAllocationCatalogueRow]{}, fmt.Errorf("list cost allocations: %w", err)
	}
	if limit < 1 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	plan, err := costAllocationPages.Plan(query, "tenant_id = $1", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	const source = `settlement_accounting.cost_allocation`
	var total int64
	if err := querier.QueryRow(ctx, plan.CountSQL(source), plan.Args...).Scan(&total); err != nil {
		return fail(fmt.Errorf("count: %w", err))
	}
	rows, err := querier.Query(ctx, plan.PageSQL(
		`allocation_id, source_ref, source_minor, currency, rule_ref,
		        unallocated_minor, version, allocated_at, corrects, recorded_at, portions`, source), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	entries := make([]ports.CostAllocationCatalogueRow, 0, limit)
	for rows.Next() {
		var (
			row          ports.CostAllocationCatalogueRow
			allocatedAt  time.Time
			corrects     *string
			recordedAt   time.Time
			portionsJSON []byte
		)
		if err := rows.Scan(
			&row.Allocation, &row.Source, &row.SourceMinor, &row.Currency, &row.Rule,
			&row.UnallocatedMinor, &row.Version, &allocatedAt, &corrects, &recordedAt,
			&portionsJSON,
		); err != nil {
			return fail(err)
		}
		row.AllocatedAt = allocatedAt.UTC()
		row.Corrects = catalogueText(corrects)
		row.RecordedAt = recordedAt.UTC()

		// 词形复用写侧的 portionRow，理由同组成集那句。
		var documents []portionRow
		if err := json.Unmarshal(portionsJSON, &documents); err != nil {
			return fail(fmt.Errorf("份额集解码：%w", err))
		}
		portions := make([]ports.AllocationPortionEntry, 0, len(documents))
		for _, document := range documents {
			portions = append(portions, ports.AllocationPortionEntry{
				Target:      document.Target,
				AmountMinor: document.AmountMinor,
			})
		}
		row.Portions = portions
		entries = append(entries, row)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	page, err := cataloguepage.Finish(query, entries, limit, total,
		func(row ports.CostAllocationCatalogueRow, sortField string) cataloguepage.Position {
			position := cataloguepage.Position{Value: row.Allocation, Identity: []string{row.Allocation}}
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
