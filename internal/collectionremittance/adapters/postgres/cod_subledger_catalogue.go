package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/collectionremittance/domain"
	"go.idp.xyz/idp-parcel/internal/collectionremittance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// CodSubledgerCatalogue 实现 ports.CodSubledgerCatalogueRead:代收分户账册的列表读面
// (ADR-0077,票 admin-remainder-mechanism-batch/04)。读的就是 0001 的开立面本表加
// 记账派生与批次册,不是第二份数据。
//
// 本页的开立面、余额派生与批次**一条语句取回**:ReadExecutor 不保证两条语句同一快照,
// 分次取会拼出从未同时存在的账面状态(记账与读并发时尤甚)——判据同 customscompliance
// CaseRegisterCatalogue 文件头那句。总数另起一句:与本页同一组条件,不保证同一快照——
// 要同快照的是一行的开立面、余额与批次,不是页与总数。余额走 LATERAL 聚合,一次扫过该账
// 全部记账、各个位置各自「去向侧和−来源侧和」;批次走相关子查询 json_agg,各聚各的,不与
// 余额聚合做笛卡尔积。
//
// 取页按 ports.CodSubledgerPages:缺省分户账键四维升序,键集落在四维键上。本页先在开立
// 面上取出(带 LIMIT 的子查询,规划器不把它拍平),余额与批次只随本页行算——不为翻不到
// 的行扫记账。批次在行内按批次标识升序。开立而无记账的账以全零余额在场——LEFT JOIN
// LATERAL 对空记账集交回零,不把「无记账」折成「不在册」(迁移 0001 开立面自注:两格
// 含义相反)。limit 非正是调用方编程错误(判据同 PortsPathsCatalogue 那句)。
type CodSubledgerCatalogue struct {
	db *bentopg.DB
}

func NewCodSubledgerCatalogue(db *bentopg.DB) (*CodSubledgerCatalogue, error) {
	if db == nil {
		return nil, fmt.Errorf("collection remittance postgres: db is nil")
	}
	return &CodSubledgerCatalogue{db: db}, nil
}

var _ ports.CodSubledgerCatalogueRead = (*CodSubledgerCatalogue)(nil)

// remittanceBatchDocument 是批次在 json_agg 里的临时词形。批次表列全部 NOT NULL,
// 不设指针格。
type remittanceBatchDocument struct {
	Batch            string    `json:"batch"`
	State            string    `json:"state"`
	CollectedThrough time.Time `json:"collectedThrough"`
	FormedAt         time.Time `json:"formedAt"`
}

// codSubledgerPages 把 ports.CodSubledgerPages 落到开立面 s 的列上;q 在四维键与保管
// 依据上做字面包含。取页语句里 s 先是开立面本表、再是取出的本页(子查询同名),两处列名
// 同形,同一份落列在两处都成立。
var codSubledgerPages = ports.CodSubledgerPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"customer": "s.customer_ref"},
	Identity: []string{"s.customer_ref", "s.legal_entity_ref", "s.currency", "s.channel_ref"},
	Keyword: []string{
		"s.customer_ref", "s.legal_entity_ref", "s.currency", "s.channel_ref", "s.custody_basis_ref",
	},
})

func (catalogue *CodSubledgerCatalogue) ListCodSubledgers(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.CodSubledgerCatalogueRow], error) {
	fail := func(err error) (cataloguepage.Result[ports.CodSubledgerCatalogueRow], error) {
		return cataloguepage.Result[ports.CodSubledgerCatalogueRow]{}, fmt.Errorf("list cod subledgers: %w", err)
	}
	if limit < 1 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	plan, err := codSubledgerPages.Plan(query, "s.tenant_id = $1", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}

	const source = "collection_remittance.subledger AS s"
	var total int64
	if err := querier.QueryRow(ctx, plan.CountSQL(source), plan.Args...).Scan(&total); err != nil {
		return fail(err)
	}
	pageSQL := plan.PageSQL(
		`s.tenant_id, s.customer_ref, s.legal_entity_ref, s.currency, s.channel_ref,
		 s.custody_basis_ref, s.opened_at`, source)

	// SUM(bigint) 在 PG 里是 numeric,显式收窄回 bigint——余额语义就是最小单位整数,
	// 让类型在语句里说这句话,超界在库内报错而不是静默进小数。
	rows, err := querier.Query(ctx,
		`SELECT s.customer_ref, s.legal_entity_ref, s.currency, s.channel_ref,
		        s.custody_basis_ref, s.opened_at,
		        balance.in_transit, balance.awaiting, balance.payable,
		        balance.remitted, balance.shortfall, balance.surplus,
		        balance.offset_applied, balance.posting_count,
		        (SELECT COALESCE(
		                    json_agg(
		                        json_build_object(
		                            'batch',            b.batch_ref,
		                            'state',            b.state,
		                            'collectedThrough', b.collected_through,
		                            'formedAt',         b.formed_at
		                        )
		                        ORDER BY b.batch_ref
		                    ),
		                    '[]'::json
		                )
		           FROM collection_remittance.remittance_batch AS b
		          WHERE b.tenant_id = s.tenant_id
		            AND b.customer_ref = s.customer_ref
		            AND b.legal_entity_ref = s.legal_entity_ref
		            AND b.currency = s.currency
		            AND b.channel_ref = s.channel_ref)
		   FROM (`+pageSQL+`) AS s
		   LEFT JOIN LATERAL (
		        SELECT
		            COALESCE(SUM(CASE WHEN p.to_position   = 'IN_TRANSIT_AT_CHANNEL' THEN p.amount_minor ELSE 0 END)
		                   - SUM(CASE WHEN p.from_position = 'IN_TRANSIT_AT_CHANNEL' THEN p.amount_minor ELSE 0 END),
		                     0)::bigint AS in_transit,
		            COALESCE(SUM(CASE WHEN p.to_position   = 'AWAITING_ALLOCATION' THEN p.amount_minor ELSE 0 END)
		                   - SUM(CASE WHEN p.from_position = 'AWAITING_ALLOCATION' THEN p.amount_minor ELSE 0 END),
		                     0)::bigint AS awaiting,
		            COALESCE(SUM(CASE WHEN p.to_position   = 'PAYABLE_TO_CUSTOMER' THEN p.amount_minor ELSE 0 END)
		                   - SUM(CASE WHEN p.from_position = 'PAYABLE_TO_CUSTOMER' THEN p.amount_minor ELSE 0 END),
		                     0)::bigint AS payable,
		            COALESCE(SUM(CASE WHEN p.to_position   = 'REMITTED' THEN p.amount_minor ELSE 0 END)
		                   - SUM(CASE WHEN p.from_position = 'REMITTED' THEN p.amount_minor ELSE 0 END),
		                     0)::bigint AS remitted,
		            COALESCE(SUM(CASE WHEN p.to_position   = 'SHORTFALL' THEN p.amount_minor ELSE 0 END)
		                   - SUM(CASE WHEN p.from_position = 'SHORTFALL' THEN p.amount_minor ELSE 0 END),
		                     0)::bigint AS shortfall,
		            COALESCE(SUM(CASE WHEN p.to_position   = 'SURPLUS' THEN p.amount_minor ELSE 0 END)
		                   - SUM(CASE WHEN p.from_position = 'SURPLUS' THEN p.amount_minor ELSE 0 END),
		                     0)::bigint AS surplus,
		            COALESCE(SUM(CASE WHEN p.to_position   = 'OFFSET_APPLIED' THEN p.amount_minor ELSE 0 END)
		                   - SUM(CASE WHEN p.from_position = 'OFFSET_APPLIED' THEN p.amount_minor ELSE 0 END),
		                     0)::bigint AS offset_applied,
		            COUNT(*)::bigint AS posting_count
		          FROM collection_remittance.subledger_posting AS p
		         WHERE p.tenant_id = s.tenant_id
		           AND p.customer_ref = s.customer_ref
		           AND p.legal_entity_ref = s.legal_entity_ref
		           AND p.currency = s.currency
		           AND p.channel_ref = s.channel_ref
		   ) AS balance ON true
		  ORDER BY `+plan.OrderBy,
		plan.PageArgs...,
	)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	var fetched []ports.CodSubledgerCatalogueRow
	for rows.Next() {
		var (
			row         ports.CodSubledgerCatalogueRow
			openedAt    time.Time
			batchesJSON []byte
		)
		if err := rows.Scan(
			&row.Customer, &row.LegalEntity, &row.Currency, &row.Channel,
			&row.CustodyBasis, &openedAt,
			&row.Balances.InTransitAtChannelMinor,
			&row.Balances.AwaitingAllocationMinor,
			&row.Balances.PayableToCustomerMinor,
			&row.Balances.RemittedMinor,
			&row.Balances.ShortfallMinor,
			&row.Balances.SurplusMinor,
			&row.Balances.OffsetAppliedMinor,
			&row.PostingCount,
			&batchesJSON,
		); err != nil {
			return fail(err)
		}
		row.OpenedAt = openedAt.UTC()

		var documents []remittanceBatchDocument
		if err := json.Unmarshal(batchesJSON, &documents); err != nil {
			return fail(fmt.Errorf("批次集解码:%w", err))
		}
		batches := make([]ports.CodSubledgerRemittanceBatch, 0, len(documents))
		for _, document := range documents {
			batches = append(batches, ports.CodSubledgerRemittanceBatch{
				Batch:            document.Batch,
				State:            document.State,
				CollectedThrough: document.CollectedThrough.UTC(),
				FormedAt:         document.FormedAt.UTC(),
			})
		}
		row.Batches = batches
		fetched = append(fetched, row)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	page, err := cataloguepage.Finish(query, fetched, limit, total, codSubledgerPosition)
	if err != nil {
		return fail(err)
	}
	return page, nil
}

// codSubledgerPosition 取本页末行扫描回来的四维键:都从 s 的列上原样读回,与 ORDER BY 用的是
// 同一份。本册只按客户排,排序维的值就是行标识的第一段。
func codSubledgerPosition(row ports.CodSubledgerCatalogueRow, _ string) cataloguepage.Position {
	return cataloguepage.Position{
		Value:    row.Customer,
		Identity: []string{row.Customer, row.LegalEntity, row.Currency, row.Channel},
	}
}
