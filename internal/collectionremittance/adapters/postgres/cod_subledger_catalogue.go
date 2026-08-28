package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/collectionremittance/domain"
	"go.idp.xyz/idp-parcel/internal/collectionremittance/ports"
)

// CodSubledgerCatalogue 实现 ports.CodSubledgerCatalogueRead:代收分户账册的列表读面
// (ADR-0077,票 admin-remainder-mechanism-batch/04)。读的就是 0001 的开立面本表加
// 记账派生与批次册,不是第二份数据。
//
// 开立面、余额派生与批次**一条语句取回**:ReadExecutor 不保证两条语句同一快照,分次
// 取会拼出从未同时存在的账面状态(记账与读并发时尤甚)——判据同 customscompliance
// CaseRegisterCatalogue 文件头那句。余额走 LATERAL 聚合,一次扫过该账全部记账、六个
// 位置各自「去向侧和−来源侧和」;批次走相关子查询 json_agg,各聚各的,不与余额聚合
// 做笛卡尔积。
//
// 排序按分户账键四维升序保证分页可重复;批次在行内按批次标识升序。开立而无记账的账
// 以全零余额在场——LEFT JOIN LATERAL 对空记账集交回零,不把「无记账」折成「不在册」
// (迁移 0001 开立面自注:两格含义相反)。limit 非正是调用方编程错误(判据同
// PortsPathsCatalogue 那句)。
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

func (catalogue *CodSubledgerCatalogue) ListCodSubledgers(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.CodSubledgerCatalogueRow, error) {
	if limit < 1 {
		return nil, fmt.Errorf("list cod subledgers: limit must be positive, got %d", limit)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list cod subledgers: %w", err)
	}

	// SUM(bigint) 在 PG 里是 numeric,显式收窄回 bigint——余额语义就是最小单位整数,
	// 让类型在语句里说这句话,超界在库内报错而不是静默进小数。
	rows, err := querier.Query(ctx,
		`SELECT s.customer_ref, s.legal_entity_ref, s.currency, s.channel_ref,
		        s.custody_basis_ref, s.opened_at,
		        balance.in_transit, balance.awaiting, balance.payable,
		        balance.remitted, balance.shortfall, balance.surplus,
		        balance.posting_count,
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
		   FROM collection_remittance.subledger AS s
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
		            COUNT(*)::bigint AS posting_count
		          FROM collection_remittance.subledger_posting AS p
		         WHERE p.tenant_id = s.tenant_id
		           AND p.customer_ref = s.customer_ref
		           AND p.legal_entity_ref = s.legal_entity_ref
		           AND p.currency = s.currency
		           AND p.channel_ref = s.channel_ref
		   ) AS balance ON true
		  WHERE s.tenant_id = $1
		  ORDER BY s.customer_ref, s.legal_entity_ref, s.currency, s.channel_ref
		  LIMIT $2`,
		tenant.String(), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list cod subledgers: %w", err)
	}
	defer rows.Close()

	entries := make([]ports.CodSubledgerCatalogueRow, 0, limit)
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
			&row.PostingCount,
			&batchesJSON,
		); err != nil {
			return nil, fmt.Errorf("list cod subledgers: %w", err)
		}
		row.OpenedAt = openedAt.UTC()

		var documents []remittanceBatchDocument
		if err := json.Unmarshal(batchesJSON, &documents); err != nil {
			return nil, fmt.Errorf("list cod subledgers: 批次集解码:%w", err)
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
		entries = append(entries, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list cod subledgers: %w", err)
	}
	return entries, nil
}
