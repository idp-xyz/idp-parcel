package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
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
//
// 各册按 ports 上各自的声明取页：总数另数一句，与取页不在同一快照里，是此刻的大概；一行的
// 各格仍在一条语句里。
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

// statementDisputeNumbers 是单上各项争议的争议号连成的一格，只供 q 检索：客户报来的常是争议号，争议挂在单上、不另成一册。
// 以换行相隔，检索词里没有换行就不会跨两个号命中；单上没有争议、或争议都没有号时为 NULL，这一格不命中。
const statementDisputeNumbers = `(SELECT string_agg(d.document_number, E'\n')
		   FROM settlement_accounting.statement_dispute AS d
		  WHERE d.tenant_id = s.tenant_id
		    AND d.statement_number = s.statement_number)`

// customerStatementPageColumns 是对账单册与挂着未了结争议的单两册共用的落列：两册同列同解码，次序、状态维与 q 也同一份。
//
// voided 落在作废时刻在场的文本形上（'true' / 'false'，与词表同词）。作废两格同生同灭（约束 customer_statement_void_trace_coherent），
// 只看时刻一列，与答复里 voided 读的是同一格。不建索引：这一维只在本租户的行上再滤一遍。
var customerStatementPageColumns = cataloguepage.Columns{
	Sorts:    map[string]string{"statementNumber": "s.statement_number", "publishedAt": "s.published_at"},
	Identity: []string{"s.statement_number"},
	Filters:  map[string]string{"voided": "(s.voided_at IS NOT NULL)::text"},
	Keyword:  []string{"s.statement_number", "s.account_id", "s.period_ref", statementDisputeNumbers},
}

var (
	customerStatementPages         = ports.CustomerStatementPages.MustBind(customerStatementPageColumns)
	disputedCustomerStatementPages = ports.DisputedCustomerStatementPages.MustBind(customerStatementPageColumns)
)

const customerStatementTable = `settlement_accounting.customer_statement AS s`

// customerStatementPosition 给出对账单一行的位置，两册共用。
func customerStatementPosition(row ports.CustomerStatementCatalogueRow, sortField string) cataloguepage.Position {
	position := cataloguepage.Position{Value: row.StatementNumber, Identity: []string{row.StatementNumber}}
	if sortField == "publishedAt" {
		position.Value = cataloguepage.FormatInstant(row.PublishedAt)
	}
	return position
}

func (catalogue *StatementCatalogue) ListCustomerStatements(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.CustomerStatementCatalogueRow], error) {
	fail := func(err error) (cataloguepage.Result[ports.CustomerStatementCatalogueRow], error) {
		return cataloguepage.Result[ports.CustomerStatementCatalogueRow]{}, fmt.Errorf("list customer statements: %w", err)
	}
	if limit < 1 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	plan, err := customerStatementPages.Plan(query, "s.tenant_id = $1", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	var total int64
	if err := querier.QueryRow(ctx, plan.CountSQL(customerStatementTable), plan.Args...).Scan(&total); err != nil {
		return fail(fmt.Errorf("count: %w", err))
	}
	rows, err := querier.Query(ctx, plan.PageSQL(customerStatementColumns, customerStatementTable), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	entries := make([]ports.CustomerStatementCatalogueRow, 0, limit)
	for rows.Next() {
		row, err := scanCustomerStatement(rows.Scan)
		if err != nil {
			return fail(err)
		}
		entries = append(entries, row)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	page, err := cataloguepage.Finish(query, entries, limit, total, customerStatementPosition)
	if err != nil {
		return fail(err)
	}
	return page, nil
}

// disputedCustomerStatementCondition 是「挂着未了结争议」的那条固定条件：至少一项争议没有终局结果（$2 是终局结果的词）。
const disputedCustomerStatementCondition = `s.tenant_id = $1
		    AND EXISTS (
		        SELECT 1
		          FROM settlement_accounting.statement_dispute AS o
		         WHERE o.tenant_id = s.tenant_id
		           AND o.statement_number = s.statement_number
		           AND (o.resolved_at IS NULL OR o.resolution <> ALL($2)))`

// ListDisputedCustomerStatements 上列至少挂着一项没有终局结果的争议的对账单的一页（见 ports.DisputedCustomerStatements）。
// 张数是整个待对账队列的积压数，不随 q 变（票 catalogue-read-pagination-rollout spec「已带计数与 truncated 的待办队列保留这两格」）：
// 本次没有任何条件时它就是
// total，不多数一次；带了 q 时另数一次只带固定条件的。几句不在同一快照里，张数与清单是此刻的大概；一张都没有时张数如实是 0。
func (catalogue *StatementCatalogue) ListDisputedCustomerStatements(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (ports.DisputedCustomerStatements, error) {
	fail := func(err error) (ports.DisputedCustomerStatements, error) {
		return ports.DisputedCustomerStatements{}, fmt.Errorf("list disputed customer statements: %w", err)
	}
	if limit < 1 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	conditionArgs := []any{tenant.String(), finalDisputeResolutions}
	plan, err := disputedCustomerStatementPages.Plan(query, disputedCustomerStatementCondition, conditionArgs, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	var total int64
	if err := querier.QueryRow(ctx, plan.CountSQL(customerStatementTable), plan.Args...).Scan(&total); err != nil {
		return fail(fmt.Errorf("count: %w", err))
	}
	backlog := total
	if query.Keyword != "" || len(query.Filters) > 0 {
		if err := querier.QueryRow(ctx,
			"SELECT count(*) FROM "+customerStatementTable+" WHERE "+disputedCustomerStatementCondition,
			conditionArgs...).Scan(&backlog); err != nil {
			return fail(fmt.Errorf("count backlog: %w", err))
		}
	}
	rows, err := querier.Query(ctx, plan.PageSQL(customerStatementColumns, customerStatementTable), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	entries := make([]ports.CustomerStatementCatalogueRow, 0, limit)
	for rows.Next() {
		row, err := scanCustomerStatement(rows.Scan)
		if err != nil {
			return fail(err)
		}
		entries = append(entries, row)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	page, err := cataloguepage.Finish(query, entries, limit, total, customerStatementPosition)
	if err != nil {
		return fail(err)
	}
	return ports.DisputedCustomerStatements{
		Count:      int(backlog),
		Next:       page.Next,
		Total:      page.Total,
		Statements: page.Rows,
	}, nil
}

// scanCustomerStatement 按 customerStatementColumns 的列序解一行。
func scanCustomerStatement(scan func(dest ...any) error) (ports.CustomerStatementCatalogueRow, error) {
	var (
		row            ports.CustomerStatementCatalogueRow
		publishedAt    time.Time
		voidBasis      *string
		voidedAt       *time.Time
		disputesJSON   []byte
		inclusionsJSON []byte
	)
	if err := scan(
		&row.StatementNumber, &row.Account, &row.Period, &row.Currency, &row.TotalMinor,
		&row.LineCount, &row.AdjustmentCount,
		&publishedAt, &voidBasis, &voidedAt,
		&disputesJSON, &inclusionsJSON,
	); err != nil {
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

// supplierBillReceptionPages 把账单接收册的声明落到列上，列名都是 supplierBillReceptionWith 导出的。
var supplierBillReceptionPages = ports.SupplierBillReceptionPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"receivedAt": "r.received_at", "period": "r.period_ref"},
	Identity: []string{"r.claim_id", "r.claim_version"},
	Keyword:  []string{"r.claim_id", "r.bill_number", "r.supplier_ref", "r.period_ref", "r.legal_entity"},
})

// supplierBillReceptionWith 是账单接收册的取数来源：接收行连同账单号册上的号与分配方，收到时刻经迁移 0056 的取值函数导出成
// 一列。读面调的必须就是那个函数而不是展开写的同义表达式：函数标了 IMMUTABLE、体内的文本转时刻却不是，规划器因此不内联它，
// 只有调同一个函数，那条表达式索引才认得出来。
const supplierBillReceptionWith = `WITH r AS (
		SELECT b.tenant_id, b.claim_id, b.claim_version, b.supplier_ref, b.legal_entity, b.period_ref, b.currency,
		       b.claim, b.matches, b.audit_authority_configured, b.operation_evidence, b.recorded_at,
		       settlement_accounting.supplier_bill_received_at(b.claim) AS received_at,
		       n.bill_number, n.supplier_ref AS bill_supplier_ref
		  FROM settlement_accounting.supplier_bill_reception AS b
		  LEFT JOIN settlement_accounting.supplier_bill_number AS n
		    ON n.tenant_id = b.tenant_id AND n.claim_id = b.claim_id
	) `

// supplierBillReceptionCondition 是读面的固定条件：租户与 ports.SupplierBillReceptionFilter 的三维（$2 主张身份、$3 供应商、
// $4 账单号，空串即不筛）。按账单号检索时，供应商比的是号册上的分配方（$3 对 bill_supplier_ref）：号是那家供应商分配的；
// 存量里各版本供应商不一的主张，按接收行的供应商比就会让另一家的检索命中这个号。
const supplierBillReceptionCondition = `r.tenant_id = $1
		    AND ($2::text = '' OR r.claim_id = $2)
		    AND ($3::text = '' OR ($4::text = '' AND r.supplier_ref = $3) OR ($4::text <> '' AND r.bill_supplier_ref = $3))
		    AND ($4::text = '' OR r.bill_number = $4)`

// supplierBillReceptionSelection 与 supplierBillReceptionLaterals 是取页那一句的列与来源。三个占位由 fmt 填：已匹配那个分类与
// 两种审核结果的字面只出现在取页那一句里，总数那一句不带，编号因此接在取页参数之后（见 ListSupplierBillReceptions）。
const (
	supplierBillReceptionSelection = `r.claim_id, r.claim_version, r.supplier_ref, r.legal_entity, r.period_ref, r.currency,
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
		        r.bill_number,
		        (SELECT count(*)::bigint
		           FROM jsonb_array_elements(r.matches) AS m
		          WHERE m ->> 'classification' = $%[1]d
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
		        r.received_at,
		        r.claim ->> 'supplements',
		        r.operation_evidence,
		        r.recorded_at`
	supplierBillReceptionLaterals = `r
		  CROSS JOIN LATERAL (
		        SELECT count(DISTINCT p.line_ref)::bigint AS lines, COALESCE(sum(p.amount_minor), 0)::bigint AS minor
		          FROM settlement_accounting.audited_payable AS p
		         WHERE p.tenant_id = r.tenant_id
		           AND p.claim_id = r.claim_id
		           AND p.line_ref IN (SELECT l ->> 'line' FROM jsonb_array_elements(r.claim -> 'lines') AS l)
		       ) AS payables
		  CROSS JOIN LATERAL (
		        SELECT COALESCE(sum(d.range_to_minor - d.range_from_minor) FILTER (WHERE d.verdict = $%[2]d), 0)::bigint AS rejected,
		               GREATEST(
		                   COALESCE(sum(d.range_to_minor - d.range_from_minor)
		                            FILTER (WHERE d.verdict = $%[1]d AND d.resolves_decision IS NULL), 0)
		                 - COALESCE(sum(d.range_to_minor - d.range_from_minor)
		                            FILTER (WHERE d.resolves_decision IS NOT NULL), 0),
		                 0)::bigint AS open_dispute
		          FROM settlement_accounting.supplier_bill_audit_decision AS d
		         WHERE d.tenant_id = r.tenant_id
		           AND d.claim_id = r.claim_id
		           AND d.line_ref IN (SELECT l ->> 'line' FROM jsonb_array_elements(r.claim -> 'lines') AS l)
		       ) AS audits`
)

func (catalogue *StatementCatalogue) ListSupplierBillReceptions(
	ctx context.Context,
	tenant domain.TenantID,
	filter ports.SupplierBillReceptionFilter,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.SupplierBillReceptionCatalogueRow], error) {
	fail := func(err error) (cataloguepage.Result[ports.SupplierBillReceptionCatalogueRow], error) {
		return cataloguepage.Result[ports.SupplierBillReceptionCatalogueRow]{}, fmt.Errorf("list supplier bill receptions: %w", err)
	}
	if limit < 1 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	plan, err := supplierBillReceptionPages.Plan(query, supplierBillReceptionCondition,
		[]any{tenant.String(), filter.Claim, filter.Supplier, filter.SupplierBillNumber}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	var total int64
	if err := querier.QueryRow(ctx, supplierBillReceptionWith+plan.CountSQL("r"), plan.Args...).Scan(&total); err != nil {
		return fail(fmt.Errorf("count: %w", err))
	}

	// 缺省按到达的业务时刻倒序（同刻按主张身份与版本，同向）：最新收到的账单总在第一页上。按主张身份排，超过一页之后排在
	// 后面的是主张身份靠后的账单，最新收到的那一份可能根本不在第一页上。
	//
	// 审核授权有两格：audit_authority_configured 是接收时写下的快照，另一格读这一刻审核授权册上有没有这一组——审核步骤在
	// 审核那一刻读的正是这本册（理由见 ports.SupplierBillReceptionCatalogueRow）。
	//
	// 汇总都在库内一条语句里求（同一快照）：行金额之和与分类计数各展开一次 jsonb；应付只数落在这一版各行上的那几笔——
	// 应付不带主张版本，同一主张的另一版若多出一行，那一行的应付不算进这一版。「已匹配却没有应付」按行逐条看应付在不在，
	// 已匹配那个分类的字面取自领域，不在 SQL 里另写一份。一行可以有几份应付（一段一份，ADR-0357 决定三），应付行数数的是
	// 有应付的行，不是应付的份数。
	//
	// 审核决定的三个和（已拒绝、仍在争议中、还没有任何结果）也只数这一版各行上的决定，求法与 domain.TallyLineDecisions 一致：
	// 仍在争议中 = 直接给的争议之和减去全部再审之和，还没有结果 = 逐行主张金额减去直接给的各段之和、不小于零。结果的字面取自
	// 领域。账单号取账单号册，不论哪一版带来。
	next := len(plan.PageArgs)
	args := append(append([]any(nil), plan.PageArgs...),
		domain.LineMatched.String(), domain.AuditDisputed.String(), domain.AuditRejected.String())
	rows, err := querier.Query(ctx, supplierBillReceptionWith+plan.PageSQL(
		fmt.Sprintf(supplierBillReceptionSelection, next+1),
		fmt.Sprintf(supplierBillReceptionLaterals, next+2, next+3),
	), args...)
	if err != nil {
		return fail(err)
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
			return fail(err)
		}
		if err := json.Unmarshal(classificationsJS, &row.ClassificationCounts); err != nil {
			return fail(fmt.Errorf("分类计数解码：%w", err))
		}
		row.ReceivedAt = receivedAt.UTC()
		row.SupplierBillNumber = catalogueText(billNumber)
		row.Supplements = catalogueText(supplements)
		row.OperationEvidence = catalogueText(evidence)
		row.RecordedAt = recordedAt.UTC()
		entries = append(entries, row)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	page, err := cataloguepage.Finish(query, entries, limit, total,
		func(row ports.SupplierBillReceptionCatalogueRow, sortField string) cataloguepage.Position {
			position := cataloguepage.Position{
				Value:    cataloguepage.FormatInstant(row.ReceivedAt),
				Identity: []string{row.Claim, row.ClaimVersion},
			}
			if sortField == "period" {
				position.Value = row.Period
			}
			return position
		})
	if err != nil {
		return fail(err)
	}
	return page, nil
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
