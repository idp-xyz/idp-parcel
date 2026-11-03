package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// StatementCatalogue 实现 ports.StatementCatalogueRead：已发布对账单册与供应商账单
// 接收册的列表读面（ADR-0077，票 admin-skeleton-closure-batch/04）。读的就是 0002 与
// 0007 的本表；供应商账单详情与审核应付册（0016）也挂在它上面，见 supplier_bill_catalogue.go。
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

var (
	_ ports.StatementCatalogueRead = (*StatementCatalogue)(nil)
	_ ports.DisputedStatementRead  = (*StatementCatalogue)(nil)
	_ ports.StatementLineRead      = (*StatementCatalogue)(nil)
)

// statementDisputeDocument 与 subsequentInclusionDocument 是两个聚合的临时词形。
// 裁定三件与纳入的调整回指在库上可空，词形跟着留指针格——把它们折成空串会在解码
// 这一层就把「未裁定」与「裁定为空串」混掉，而库上的非空白门只管非 NULL 的那一侧。
type statementDisputeDocument struct {
	Dispute        string     `json:"dispute"`
	DocumentNumber *string    `json:"documentNumber"`
	NumberAbsence  *string    `json:"documentNumberAbsence"`
	Charge         string     `json:"charge"`
	DisputedMinor  int64      `json:"disputedMinor"`
	Reason         string     `json:"reason"`
	OpenedAt       time.Time  `json:"openedAt"`
	Resolution     *string    `json:"resolution"`
	ResolutionRef  *string    `json:"resolutionRef"`
	ResolvedAt     *time.Time `json:"resolvedAt"`
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

// customerStatementColumns 是对账单册一行的列：单面、两个行数、发布与作废留痕、异议集与后续纳入集。对账单册与
// 挂着未了结争议的单两口同列同解码，同一张单两口读出来必须一样。
const customerStatementColumns = `s.statement_number, s.account_id, s.period_ref, s.currency, s.total_minor,
		        jsonb_array_length(s.lines)::bigint,
		        jsonb_array_length(s.adjustment_lines)::bigint,
		        s.published_at, s.void_basis, s.voided_at,
		        (SELECT COALESCE(
		                    json_agg(
		                        json_build_object(
		                            'dispute',       d.dispute_id,
		                            'documentNumber', d.document_number,
		                            'documentNumberAbsence', d.document_number_absence,
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
		            AND i.statement_number = s.statement_number)`

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
		`SELECT `+customerStatementColumns+`
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
		row, err := scanCustomerStatement(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("list customer statements: %w", err)
		}
		entries = append(entries, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list customer statements: %w", err)
	}
	return entries, nil
}

// ListDisputedCustomerStatements 上列至少挂着一项没有终局结果的争议的对账单（见 ports.DisputedCustomerStatements）。张数用窗口计数在同一条语句里求：窗口在 LIMIT 之前算，张数是全部而不是这一页，
// 也不会与这一页分属两个快照；一张都没有时一行都不回，张数如实是 0。
func (catalogue *StatementCatalogue) ListDisputedCustomerStatements(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) (ports.DisputedCustomerStatements, error) {
	if limit < 1 {
		return ports.DisputedCustomerStatements{}, fmt.Errorf("list disputed customer statements: limit must be positive, got %d", limit)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return ports.DisputedCustomerStatements{}, fmt.Errorf("list disputed customer statements: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT `+customerStatementColumns+`, count(*) OVER ()
		   FROM settlement_accounting.customer_statement AS s
		  WHERE s.tenant_id = $1
		    AND EXISTS (
		        SELECT 1
		          FROM settlement_accounting.statement_dispute AS o
		         WHERE o.tenant_id = s.tenant_id
		           AND o.statement_number = s.statement_number
		           AND (o.resolved_at IS NULL OR o.resolution <> ALL($2)))
		  ORDER BY s.statement_number
		  LIMIT $3`,
		tenant.String(), finalDisputeResolutions, limit,
	)
	if err != nil {
		return ports.DisputedCustomerStatements{}, fmt.Errorf("list disputed customer statements: %w", err)
	}
	defer rows.Close()

	listed := ports.DisputedCustomerStatements{Statements: make([]ports.CustomerStatementCatalogueRow, 0, limit)}
	for rows.Next() {
		var count int64
		row, err := scanCustomerStatement(rows.Scan, &count)
		if err != nil {
			return ports.DisputedCustomerStatements{}, fmt.Errorf("list disputed customer statements: %w", err)
		}
		listed.Count = int(count)
		listed.Statements = append(listed.Statements, row)
	}
	if err := rows.Err(); err != nil {
		return ports.DisputedCustomerStatements{}, fmt.Errorf("list disputed customer statements: %w", err)
	}
	return listed, nil
}

// scanCustomerStatement 按 customerStatementColumns 的列序解一行；extra 接在这些列之后（如窗口计数）。
func scanCustomerStatement(scan func(dest ...any) error, extra ...any) (ports.CustomerStatementCatalogueRow, error) {
	var (
		row            ports.CustomerStatementCatalogueRow
		publishedAt    time.Time
		voidBasis      *string
		voidedAt       *time.Time
		disputesJSON   []byte
		inclusionsJSON []byte
	)
	dest := append([]any{
		&row.StatementNumber, &row.Account, &row.Period, &row.Currency, &row.TotalMinor,
		&row.LineCount, &row.AdjustmentCount,
		&publishedAt, &voidBasis, &voidedAt,
		&disputesJSON, &inclusionsJSON,
	}, extra...)
	if err := scan(dest...); err != nil {
		return ports.CustomerStatementCatalogueRow{}, err
	}
	row.PublishedAt = publishedAt.UTC()
	row.VoidBasis = catalogueText(voidBasis)
	row.VoidedAt = catalogueInstant(voidedAt)

	var disputeDocuments []statementDisputeDocument
	if err := json.Unmarshal(disputesJSON, &disputeDocuments); err != nil {
		return ports.CustomerStatementCatalogueRow{}, fmt.Errorf("异议集解码：%w", err)
	}
	disputes := make([]ports.StatementDisputeEntry, 0, len(disputeDocuments))
	for _, document := range disputeDocuments {
		numbering, err := numberingFrom(document.DocumentNumber, document.NumberAbsence)
		if err != nil {
			return ports.CustomerStatementCatalogueRow{}, fmt.Errorf("异议 %s 的争议号：%w", document.Dispute, err)
		}
		number, absence := numbering.Words()
		disputes = append(disputes, ports.StatementDisputeEntry{
			Dispute:               document.Dispute,
			DocumentNumber:        number,
			DocumentNumberAbsence: absence,
			Charge:                document.Charge,
			DisputedMinor:         document.DisputedMinor,
			Reason:                document.Reason,
			OpenedAt:              document.OpenedAt.UTC(),
			Resolution:            catalogueText(document.Resolution),
			ResolutionRef:         catalogueText(document.ResolutionRef),
			ResolvedAt:            catalogueInstant(document.ResolvedAt),
		})
	}
	row.Disputes = disputes

	var inclusionDocuments []subsequentInclusionDocument
	if err := json.Unmarshal(inclusionsJSON, &inclusionDocuments); err != nil {
		return ports.CustomerStatementCatalogueRow{}, fmt.Errorf("后续纳入集解码：%w", err)
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
	return row, nil
}

func (catalogue *StatementCatalogue) ListSupplierBillReceptions(
	ctx context.Context,
	tenant domain.TenantID,
	filter ports.SupplierBillReceptionFilter,
	limit int,
) ([]ports.SupplierBillReceptionCatalogueRow, error) {
	if limit < 1 {
		return nil, fmt.Errorf("list supplier bill receptions: limit must be positive, got %d", limit)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list supplier bill receptions: %w", err)
	}

	// 按到达的业务时刻倒序截页（同刻按主张身份与版本，排序才确定）：页大小截掉的必须是最早收到的那些。按主张身份排再截，
	// 超过一页之后截掉的是主张身份排在后面的账单，最新收到的那一份可能根本不在页上，页面再怎么按收到时间排也补不回来。
	//
	// 审核授权有两格：audit_authority_configured 是接收时写下的快照，另一格读这一刻审核授权册上有没有这一组——审核步骤在
	// 审核那一刻读的正是这本册（理由见 ports.SupplierBillReceptionCatalogueRow）。
	//
	// 汇总都在库内一条语句里求（同一快照）：行金额之和与分类计数各展开一次 jsonb；应付只数落在这一版各行上的那几笔——
	// 应付不带主张版本，同一主张的另一版若多出一行，那一行的应付不算进这一版。「已匹配却没有应付」按行逐条看应付在不在，
	// 已匹配那个分类的字面取自领域（$3），不在 SQL 里另写一份。一行可以有几份应付（一段一份，ADR-0357 决定三），应付行数数的是
	// 有应付的行，不是应付的份数。
	//
	// 审核决定的三个和（已拒绝、仍在争议中、还没有任何结果）也只数这一版各行上的决定，求法与 domain.TallyLineDecisions 一致：
	// 仍在争议中 = 直接给的争议之和减去全部再审之和，还没有结果 = 逐行主张金额减去直接给的各段之和、不小于零。结果的字面取自
	// 领域（$4、$5）。账单号取账单号册，不论哪一版带来。按账单号检索时，供应商比的是号册上的分配方（$7 对 n.supplier_ref）：
	// 号是那家供应商分配的；存量里各版本供应商不一的主张，按接收行的供应商比就会让另一家的检索命中这个号。
	rows, err := querier.Query(ctx,
		`SELECT r.claim_id, r.claim_version, r.supplier_ref, r.legal_entity, r.period_ref, r.currency,
		        jsonb_array_length(r.claim -> 'lines')::bigint,
		        jsonb_array_length(r.matches)::bigint,
		        (SELECT COALESCE(sum((l ->> 'claimed_minor')::bigint), 0)::bigint
		           FROM jsonb_array_elements(r.claim -> 'lines') AS l),
		        (SELECT COALESCE(jsonb_object_agg(c.classification, c.lines), '{}'::jsonb)
		           FROM (SELECT m ->> 'classification' AS classification, count(*)::bigint AS lines
		                   FROM jsonb_array_elements(r.matches) AS m
		                  GROUP BY 1) AS c),
		        payables.lines, payables.minor,
		        audits.rejected, audits.open_dispute,
		        (SELECT COALESCE(sum(GREATEST((l ->> 'claimed_minor')::bigint - COALESCE((
		                    SELECT sum(d.range_to_minor - d.range_from_minor)
		                      FROM settlement_accounting.supplier_bill_audit_decision AS d
		                     WHERE d.tenant_id = r.tenant_id
		                       AND d.claim_id = r.claim_id
		                       AND d.line_ref = l ->> 'line'
		                       AND d.resolves_decision IS NULL), 0), 0)), 0)::bigint
		           FROM jsonb_array_elements(r.claim -> 'lines') AS l),
		        n.bill_number,
		        (SELECT count(*)::bigint
		           FROM jsonb_array_elements(r.matches) AS m
		          WHERE m ->> 'classification' = $3
		            AND NOT EXISTS (
		                SELECT 1
		                  FROM settlement_accounting.audited_payable AS p
		                 WHERE p.tenant_id = r.tenant_id
		                   AND p.claim_id = r.claim_id
		                   AND p.line_ref = m ->> 'line')),
		        r.audit_authority_configured,
		        EXISTS (
		            SELECT 1
		              FROM settlement_accounting.supplier_audit_authority AS a
		             WHERE a.tenant_id = r.tenant_id
		               AND a.supplier_id = r.supplier_ref
		               AND a.legal_entity_id = r.legal_entity),
		        (r.claim ->> 'received_at')::timestamptz,
		        r.claim ->> 'supplements',
		        r.operation_evidence,
		        r.recorded_at
		   FROM settlement_accounting.supplier_bill_reception AS r
		   LEFT JOIN settlement_accounting.supplier_bill_number AS n
		     ON n.tenant_id = r.tenant_id AND n.claim_id = r.claim_id
		  CROSS JOIN LATERAL (
		        SELECT count(DISTINCT p.line_ref)::bigint AS lines, COALESCE(sum(p.amount_minor), 0)::bigint AS minor
		          FROM settlement_accounting.audited_payable AS p
		         WHERE p.tenant_id = r.tenant_id
		           AND p.claim_id = r.claim_id
		           AND p.line_ref IN (SELECT l ->> 'line' FROM jsonb_array_elements(r.claim -> 'lines') AS l)
		       ) AS payables
		  CROSS JOIN LATERAL (
		        SELECT COALESCE(sum(d.range_to_minor - d.range_from_minor) FILTER (WHERE d.verdict = $5), 0)::bigint AS rejected,
		               GREATEST(
		                   COALESCE(sum(d.range_to_minor - d.range_from_minor)
		                            FILTER (WHERE d.verdict = $4 AND d.resolves_decision IS NULL), 0)
		                 - COALESCE(sum(d.range_to_minor - d.range_from_minor)
		                            FILTER (WHERE d.resolves_decision IS NOT NULL), 0),
		                 0)::bigint AS open_dispute
		          FROM settlement_accounting.supplier_bill_audit_decision AS d
		         WHERE d.tenant_id = r.tenant_id
		           AND d.claim_id = r.claim_id
		           AND d.line_ref IN (SELECT l ->> 'line' FROM jsonb_array_elements(r.claim -> 'lines') AS l)
		       ) AS audits
		  WHERE r.tenant_id = $1
		    AND ($6::text = '' OR r.claim_id = $6)
		    AND ($7::text = '' OR ($8::text = '' AND r.supplier_ref = $7) OR ($8::text <> '' AND n.supplier_ref = $7))
		    AND ($8::text = '' OR n.bill_number = $8)
		  ORDER BY (r.claim ->> 'received_at')::timestamptz DESC, r.claim_id, r.claim_version
		  LIMIT $2`,
		tenant.String(), limit, domain.LineMatched.String(), domain.AuditDisputed.String(), domain.AuditRejected.String(),
		filter.Claim, filter.Supplier, filter.SupplierBillNumber,
	)
	if err != nil {
		return nil, fmt.Errorf("list supplier bill receptions: %w", err)
	}
	defer rows.Close()

	entries := make([]ports.SupplierBillReceptionCatalogueRow, 0, limit)
	for rows.Next() {
		var (
			row               ports.SupplierBillReceptionCatalogueRow
			classificationsJS []byte
			receivedAt        time.Time
			supplements       *string
			evidence          *string
			billNumber        *string
			recordedAt        time.Time
		)
		if err := rows.Scan(
			&row.Claim, &row.ClaimVersion, &row.Supplier, &row.LegalEntity,
			&row.Period, &row.Currency,
			&row.LineCount, &row.MatchCount,
			&row.TotalClaimedMinor, &classificationsJS,
			&row.PayableLineCount, &row.PayableMinor,
			&row.RejectedMinor, &row.OpenDisputeMinor, &row.UndecidedMinor, &billNumber,
			&row.MatchedWithoutPayableCount,
			&row.AuditAuthorityConfigured, &row.AuditAuthorityConfiguredNow, &receivedAt, &supplements, &evidence, &recordedAt,
		); err != nil {
			return nil, fmt.Errorf("list supplier bill receptions: %w", err)
		}
		if err := json.Unmarshal(classificationsJS, &row.ClassificationCounts); err != nil {
			return nil, fmt.Errorf("list supplier bill receptions: 分类计数解码：%w", err)
		}
		row.ReceivedAt = receivedAt.UTC()
		row.SupplierBillNumber = catalogueText(billNumber)
		row.Supplements = catalogueText(supplements)
		row.OperationEvidence = catalogueText(evidence)
		row.RecordedAt = recordedAt.UTC()
		entries = append(entries, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list supplier bill receptions: %w", err)
	}
	return entries, nil
}

// FindStatementLines 读一张已发布对账单自己的费用行（单上的不可覆盖快照），连同币种、发布与作废时刻；单不在册答 false。
func (catalogue *StatementCatalogue) FindStatementLines(
	ctx context.Context,
	tenant domain.TenantID,
	number string,
) (ports.StatementLines, bool, error) {
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return ports.StatementLines{}, false, fmt.Errorf("find statement lines: %w", err)
	}
	var (
		found       ports.StatementLines
		publishedAt time.Time
		voidedAt    *time.Time
		linesJSON   []byte
	)
	err = querier.QueryRow(ctx,
		`SELECT statement_number, currency, published_at, voided_at, lines
		   FROM settlement_accounting.customer_statement
		  WHERE tenant_id = $1 AND statement_number = $2`,
		tenant.String(), number,
	).Scan(&found.StatementNumber, &found.Currency, &publishedAt, &voidedAt, &linesJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.StatementLines{}, false, nil
	}
	if err != nil {
		return ports.StatementLines{}, false, fmt.Errorf("find statement lines: %w", err)
	}
	found.PublishedAt = publishedAt.UTC()
	found.VoidedAt = catalogueInstant(voidedAt)
	// 行模型与写侧同一份（customer_statement.go 的 statementLineRow）：写侧改键名，这里跟着改，不会静默读空。
	var documents []statementLineRow
	if err := json.Unmarshal(linesJSON, &documents); err != nil {
		return ports.StatementLines{}, false, fmt.Errorf("find statement lines: 费用行解码：%w", err)
	}
	found.Lines = make([]ports.StatementLineEntry, 0, len(documents))
	for _, document := range documents {
		found.Lines = append(found.Lines, ports.StatementLineEntry{Charge: document.Charge, AmountMinor: document.AmountMinor})
	}
	return found, true, nil
}
