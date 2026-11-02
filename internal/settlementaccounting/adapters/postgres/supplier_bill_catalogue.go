package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 供应商账单详情与审核应付册的读面（票 document-numbering/11），挂在 StatementCatalogue 上：读的是 0002、0016 两张表，
// 与接收册同一个适配器，装配点一处构造。

var (
	_ ports.SupplierBillLineRead        = (*StatementCatalogue)(nil)
	_ ports.AuditedPayableCatalogueRead = (*StatementCatalogue)(nil)
)

// linePayableDocument 是挂在主张上的应付在聚合里的临时词形。单据号两列与操作证据在库上可空，词形留指针格，理由同
// statementDisputeDocument。
type linePayableDocument struct {
	Payable           string    `json:"payable"`
	Line              string    `json:"line"`
	DocumentNumber    *string   `json:"documentNumber"`
	NumberAbsence     *string   `json:"documentNumberAbsence"`
	AmountMinor       int64     `json:"amountMinor"`
	Currency          string    `json:"currency"`
	Auditor           string    `json:"auditor"`
	OperationEvidence *string   `json:"operationEvidence"`
	AuditedAt         time.Time `json:"auditedAt"`
}

// FindSupplierBillLines 一条语句读回一份主张、它的逐行匹配与挂在它上面的审核应付（同一快照）。各行按主张原序排；
// 匹配与应付按行对上。主张不在本租户的册上答 false。
func (catalogue *StatementCatalogue) FindSupplierBillLines(
	ctx context.Context,
	tenant domain.TenantID,
	claim string,
	version string,
) (ports.SupplierBillLines, bool, error) {
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return ports.SupplierBillLines{}, false, fmt.Errorf("find supplier bill lines: %w", err)
	}
	var (
		found        ports.SupplierBillLines
		claimJSON    []byte
		matchesJSON  []byte
		payablesJSON []byte
		evidence     *string
		recordedAt   time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT r.supplier_ref, r.legal_entity, r.period_ref, r.currency, r.claim, r.matches,
		        r.audit_authority_configured, r.operation_evidence, r.recorded_at,
		        (SELECT COALESCE(
		                    json_agg(
		                        json_build_object(
		                            'payable',               p.payable_id,
		                            'line',                  p.line_ref,
		                            'documentNumber',        p.document_number,
		                            'documentNumberAbsence', p.document_number_absence,
		                            'amountMinor',           p.amount_minor,
		                            'currency',              p.currency,
		                            'auditor',               p.auditor_ref,
		                            'operationEvidence',     p.operation_evidence,
		                            'auditedAt',             p.audited_at
		                        )
		                        ORDER BY p.line_ref
		                    ),
		                    '[]'::json
		                )
		           FROM settlement_accounting.audited_payable AS p
		          WHERE p.tenant_id = r.tenant_id
		            AND p.claim_id = r.claim_id)
		   FROM settlement_accounting.supplier_bill_reception AS r
		  WHERE r.tenant_id = $1 AND r.claim_id = $2 AND r.claim_version = $3`,
		tenant.String(), claim, version,
	).Scan(&found.Supplier, &found.LegalEntity, &found.Period, &found.Currency, &claimJSON, &matchesJSON,
		&found.AuditAuthorityConfigured, &evidence, &recordedAt, &payablesJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.SupplierBillLines{}, false, nil
	}
	if err != nil {
		return ports.SupplierBillLines{}, false, fmt.Errorf("find supplier bill lines: %w", err)
	}
	found.Claim, found.ClaimVersion = claim, version
	found.OperationEvidence = catalogueText(evidence)
	found.RecordedAt = recordedAt.UTC()

	// 行模型与写侧同一份（supplier_bill_reception.go 的 claimDocument、matchRow）：写侧改键名，这里跟着改，不会静默读空。
	var document claimDocument
	if err := json.Unmarshal(claimJSON, &document); err != nil {
		return ports.SupplierBillLines{}, false, fmt.Errorf("find supplier bill lines: 主张解码：%w", err)
	}
	var matches []matchRow
	if err := json.Unmarshal(matchesJSON, &matches); err != nil {
		return ports.SupplierBillLines{}, false, fmt.Errorf("find supplier bill lines: 匹配解码：%w", err)
	}
	var payables []linePayableDocument
	if err := json.Unmarshal(payablesJSON, &payables); err != nil {
		return ports.SupplierBillLines{}, false, fmt.Errorf("find supplier bill lines: 应付解码：%w", err)
	}
	found.ReceivedAt = document.ReceivedAt.UTC()
	found.Supplements = document.Supplements

	matchByLine := make(map[string]matchRow, len(matches))
	for _, match := range matches {
		matchByLine[match.Line] = match
	}
	payableByLine := make(map[string]*ports.SupplierBillLinePayable, len(payables))
	for _, payable := range payables {
		numbering, err := numberingFrom(payable.DocumentNumber, payable.NumberAbsence)
		if err != nil {
			return ports.SupplierBillLines{}, false, fmt.Errorf("find supplier bill lines: 应付 %s 的应付单号：%w", payable.Payable, err)
		}
		number, absence := numbering.Words()
		payableByLine[payable.Line] = &ports.SupplierBillLinePayable{
			Payable: payable.Payable, DocumentNumber: number, DocumentNumberAbsence: absence,
			AmountMinor: payable.AmountMinor, Currency: payable.Currency, AuditorRef: payable.Auditor,
			OperationEvidence: catalogueText(payable.OperationEvidence), AuditedAt: payable.AuditedAt.UTC(),
		}
	}
	found.Lines = make([]ports.SupplierBillLineEntry, 0, len(document.Lines))
	for _, line := range document.Lines {
		entry := ports.SupplierBillLineEntry{
			Line: line.Line, FeeItem: line.FeeItem, ClaimedMinor: line.ClaimedMinor, Payable: payableByLine[line.Line],
		}
		if match, matched := matchByLine[line.Line]; matched {
			matchedAt := match.MatchedAt.UTC()
			entry.Classification, entry.ExpectedVersion, entry.ExpectedMinor = match.Classification, match.Expected, match.ExpectedMinor
			entry.Basis, entry.MatchedAt = match.Basis, &matchedAt
		}
		found.Lines = append(found.Lines, entry)
	}
	return found, true, nil
}

// ListAuditedPayables 上列审核应付，新形成的在前。供应商与账期取这份主张最早落库的那一版接收记录（见
// ports.AuditedPayableCatalogueRow）；一笔应付找不到它的接收记录就是册上矛盾——审核只从接收记录起步——答错误，不把它当作
// 「没有供应商」上列，也不悄悄从列表里漏掉。
func (catalogue *StatementCatalogue) ListAuditedPayables(
	ctx context.Context,
	tenant domain.TenantID,
	filter ports.AuditedPayableFilter,
	limit int,
) ([]ports.AuditedPayableCatalogueRow, error) {
	if limit < 1 {
		return nil, fmt.Errorf("list audited payables: limit must be positive, got %d", limit)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list audited payables: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT p.payable_id, p.document_number, p.document_number_absence, p.claim_id, p.line_ref, p.expected_version,
		        r.supplier_ref, r.period_ref, p.legal_entity, p.account_id, p.currency, p.amount_minor,
		        p.auditor_ref, p.operation_evidence, p.audited_at, p.recorded_at
		   FROM settlement_accounting.audited_payable AS p
		   LEFT JOIN LATERAL (
		        SELECT b.supplier_ref, b.period_ref
		          FROM settlement_accounting.supplier_bill_reception AS b
		         WHERE b.tenant_id = p.tenant_id
		           AND b.claim_id = p.claim_id
		         ORDER BY b.recorded_at, b.claim_version
		         LIMIT 1
		       ) AS r ON true
		  WHERE p.tenant_id = $1
		    AND ($2::text = '' OR p.document_number = $2)
		    AND ($3::text = '' OR p.claim_id = $3)
		    AND ($4::text = '' OR r.supplier_ref = $4)
		  ORDER BY p.audited_at DESC, p.payable_id
		  LIMIT $5`,
		tenant.String(), filter.DocumentNumber, filter.Claim, filter.Supplier, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list audited payables: %w", err)
	}
	defer rows.Close()

	entries := make([]ports.AuditedPayableCatalogueRow, 0, limit)
	for rows.Next() {
		var (
			row                 ports.AuditedPayableCatalogueRow
			number, absence     *string
			supplier, period    *string
			evidence            *string
			auditedAt, recorded time.Time
		)
		if err := rows.Scan(
			&row.Payable, &number, &absence, &row.Claim, &row.Line, &row.ExpectedVersion,
			&supplier, &period, &row.LegalEntity, &row.Account, &row.Currency, &row.AmountMinor,
			&row.AuditorRef, &evidence, &auditedAt, &recorded,
		); err != nil {
			return nil, fmt.Errorf("list audited payables: %w", err)
		}
		if supplier == nil || period == nil {
			return nil, fmt.Errorf("list audited payables: 应付 %s 找不到主张 %s 的接收记录", row.Payable, row.Claim)
		}
		numbering, err := numberingFrom(number, absence)
		if err != nil {
			return nil, fmt.Errorf("list audited payables: 应付 %s 的应付单号：%w", row.Payable, err)
		}
		row.DocumentNumber, row.DocumentNumberAbsence = numbering.Words()
		row.Supplier, row.Period = *supplier, *period
		row.OperationEvidence = catalogueText(evidence)
		row.AuditedAt, row.RecordedAt = auditedAt.UTC(), recorded.UTC()
		entries = append(entries, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list audited payables: %w", err)
	}
	return entries, nil
}
