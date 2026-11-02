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

// 供应商账单详情与审核应付册的读面（票 document-numbering/11、13），挂在 StatementCatalogue 上：读的是 0002、0016 与 0054 的
// 几张表，与接收册同一个适配器，装配点一处构造。

var (
	_ ports.SupplierBillLineRead        = (*StatementCatalogue)(nil)
	_ ports.AuditedPayableCatalogueRead = (*StatementCatalogue)(nil)
)

// lineDecisionDocument 是挂在主张上的一份审核决定连同它形成的应付（通过才有）在聚合里的临时词形。可空的列留指针格，理由同
// statementDisputeDocument。
type lineDecisionDocument struct {
	Decision          string    `json:"decision"`
	Line              string    `json:"line"`
	Verdict           string    `json:"verdict"`
	RangeFrom         int64     `json:"rangeFrom"`
	RangeTo           int64     `json:"rangeTo"`
	Version           *string   `json:"version"`
	Basis             *string   `json:"basis"`
	Resolves          *string   `json:"resolves"`
	Auditor           string    `json:"auditor"`
	OperationEvidence *string   `json:"operationEvidence"`
	AuditedAt         time.Time `json:"auditedAt"`
	Payable           *string   `json:"payable"`
	DocumentNumber    *string   `json:"documentNumber"`
	NumberAbsence     *string   `json:"documentNumberAbsence"`
	AmountMinor       *int64    `json:"amountMinor"`
	Currency          *string   `json:"currency"`
}

// FindSupplierBillLines 一条语句读回一份主张、它的逐行匹配、挂在它上面的审核决定连同各自形成的应付，与它的供应商账单号（同一
// 快照）。各行按主张原序排；匹配与决定按行对上，决定按落库先后。主张不在本租户的册上答 false。
//
// 已匹配金额与四个和取领域那一处定义：决定经重建门回来交给 domain.TallyLineDecisions，已匹配金额经 domain.MatchedAmountFor——
// 读面另写一份求法，就会和审核门各说一套。
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
		found         ports.SupplierBillLines
		billNumber    *string
		claimJSON     []byte
		matchesJSON   []byte
		decisionsJSON []byte
		evidence      *string
		recordedAt    time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT r.supplier_ref, n.bill_number, r.legal_entity, r.period_ref, r.currency, r.claim, r.matches,
		        r.audit_authority_configured, r.operation_evidence, r.recorded_at,
		        (SELECT COALESCE(
		                    json_agg(
		                        json_build_object(
		                            'decision',              d.decision_id,
		                            'line',                  d.line_ref,
		                            'verdict',               d.verdict,
		                            'rangeFrom',             d.range_from_minor,
		                            'rangeTo',               d.range_to_minor,
		                            'version',               d.claim_version,
		                            'basis',                 d.basis_ref,
		                            'resolves',              d.resolves_decision,
		                            'auditor',               d.auditor_ref,
		                            'operationEvidence',     d.operation_evidence,
		                            'auditedAt',             d.audited_at,
		                            'payable',               p.payable_id,
		                            'documentNumber',        p.document_number,
		                            'documentNumberAbsence', p.document_number_absence,
		                            'amountMinor',           p.amount_minor,
		                            'currency',              p.currency
		                        )
		                        ORDER BY d.recorded_at, d.decision_id
		                    ),
		                    '[]'::json
		                )
		           FROM settlement_accounting.supplier_bill_audit_decision AS d
		           LEFT JOIN settlement_accounting.audited_payable AS p
		             ON p.tenant_id = d.tenant_id AND p.payable_id = d.decision_id
		          WHERE d.tenant_id = r.tenant_id
		            AND d.claim_id = r.claim_id)
		   FROM settlement_accounting.supplier_bill_reception AS r
		   LEFT JOIN settlement_accounting.supplier_bill_number AS n
		     ON n.tenant_id = r.tenant_id AND n.claim_id = r.claim_id
		  WHERE r.tenant_id = $1 AND r.claim_id = $2 AND r.claim_version = $3`,
		tenant.String(), claim, version,
	).Scan(&found.Supplier, &billNumber, &found.LegalEntity, &found.Period, &found.Currency, &claimJSON, &matchesJSON,
		&found.AuditAuthorityConfigured, &evidence, &recordedAt, &decisionsJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.SupplierBillLines{}, false, nil
	}
	if err != nil {
		return ports.SupplierBillLines{}, false, fmt.Errorf("find supplier bill lines: %w", err)
	}
	found.Claim, found.ClaimVersion = claim, version
	found.SupplierBillNumber = catalogueText(billNumber)
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
	var decisions []lineDecisionDocument
	if err := json.Unmarshal(decisionsJSON, &decisions); err != nil {
		return ports.SupplierBillLines{}, false, fmt.Errorf("find supplier bill lines: 审核决定解码：%w", err)
	}
	found.ReceivedAt = document.ReceivedAt.UTC()
	found.Supplements = document.Supplements

	matchByLine := make(map[string]matchRow, len(matches))
	for _, match := range matches {
		matchByLine[match.Line] = match
	}
	entriesByLine := make(map[string][]ports.SupplierBillLineDecision, len(decisions))
	domainByLine := make(map[string][]domain.BillLineAuditDecision, len(decisions))
	for _, decision := range decisions {
		entry, rebuilt, err := lineDecisionOf(claim, decision)
		if err != nil {
			return ports.SupplierBillLines{}, false, fmt.Errorf("find supplier bill lines: 审核决定 %s：%w", decision.Decision, err)
		}
		entriesByLine[decision.Line] = append(entriesByLine[decision.Line], entry)
		domainByLine[decision.Line] = append(domainByLine[decision.Line], rebuilt)
	}
	found.Lines = make([]ports.SupplierBillLineEntry, 0, len(document.Lines))
	for _, line := range document.Lines {
		tally := domain.TallyLineDecisions(line.ClaimedMinor, domainByLine[line.Line])
		entry := ports.SupplierBillLineEntry{
			Line: line.Line, FeeItem: line.FeeItem, ClaimedMinor: line.ClaimedMinor, Decisions: entriesByLine[line.Line],
			PassedMinor: tally.PassedMinor, RejectedMinor: tally.RejectedMinor,
			OpenDisputeMinor: tally.OpenDisputeMinor, UndecidedMinor: tally.UndecidedMinor,
		}
		if match, matched := matchByLine[line.Line]; matched {
			classification, err := domain.MatchClassificationFromName(match.Classification)
			if err != nil {
				return ports.SupplierBillLines{}, false, fmt.Errorf("find supplier bill lines: 第 %s 行：%w", line.Line, err)
			}
			matchedAt := match.MatchedAt.UTC()
			entry.Classification, entry.ExpectedVersion, entry.ExpectedMinor = match.Classification, match.Expected, match.ExpectedMinor
			entry.Basis, entry.MatchedAt = match.Basis, &matchedAt
			entry.MatchedMinor = domain.MatchedAmountFor(classification, line.ClaimedMinor, match.ExpectedMinor)
		}
		found.Lines = append(found.Lines, entry)
	}
	return found, true, nil
}

// lineDecisionOf 把聚合里的一份决定转写成读口的词形，同时经重建门（RehydrateBillLineAuditDecision）回成领域决定给求和用——一份
// 坏写入在读回处暴露，不被求成一个看似合理的和。通过的那一份带它的应付；应付的号与无号原因经 numberingFrom 复验。
func lineDecisionOf(claim string, document lineDecisionDocument) (ports.SupplierBillLineDecision, domain.BillLineAuditDecision, error) {
	entry := ports.SupplierBillLineDecision{
		Decision: document.Decision, Verdict: document.Verdict, RangeFromMinor: document.RangeFrom, RangeToMinor: document.RangeTo,
		Basis: catalogueText(document.Basis), ResolvesDispute: catalogueText(document.Resolves), ClaimVersion: catalogueText(document.Version),
		AuditorRef: document.Auditor, OperationEvidence: catalogueText(document.OperationEvidence), AuditedAt: document.AuditedAt.UTC(),
	}
	row := auditDecisionRow{
		decision: document.Decision, claim: claim, version: document.Version, line: document.Line, verdict: document.Verdict,
		from: document.RangeFrom, to: document.RangeTo, basis: document.Basis, resolves: document.Resolves,
		auditor: document.Auditor, auditedAt: document.AuditedAt,
	}
	rebuilt, err := rebuildAuditDecision(row)
	if err != nil {
		return ports.SupplierBillLineDecision{}, domain.BillLineAuditDecision{}, err
	}
	if document.Payable != nil {
		numbering, err := numberingFrom(document.DocumentNumber, document.NumberAbsence)
		if err != nil {
			return ports.SupplierBillLineDecision{}, domain.BillLineAuditDecision{}, fmt.Errorf("应付单号：%w", err)
		}
		number, absence := numbering.Words()
		entry.Payable = &ports.SupplierBillLinePayable{
			Payable: *document.Payable, DocumentNumber: number, DocumentNumberAbsence: absence,
			AmountMinor: valueOr(document.AmountMinor), Currency: catalogueText(document.Currency),
		}
	}
	return entry, rebuilt, nil
}

func valueOr(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

// ListAuditedPayables 上列审核应付，新形成的在前。供应商与账期取这份主张最早落库的那一版接收记录（见
// ports.AuditedPayableCatalogueRow）；一笔应付找不到它的接收记录就是册上矛盾——审核只从接收记录起步——答错误，不把它当作
// 「没有供应商」上列，也不悄悄从列表里漏掉。
//
// 供应商检索维放过找不到接收记录的那几笔（r.supplier_ref IS NULL），让它们照样走到扫描时的矛盾检查：供应商只能从接收记录
// 取，用 join 出来的供应商去比，孤儿在比较这一步就被滤掉了，带不带供应商检索就成了两种答法。
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
		        p.auditor_ref, p.operation_evidence, p.audited_at, p.recorded_at,
		        d.range_from_minor, d.range_to_minor, d.resolves_decision, n.bill_number
		   FROM settlement_accounting.audited_payable AS p
		   LEFT JOIN settlement_accounting.supplier_bill_audit_decision AS d
		     ON d.tenant_id = p.tenant_id AND d.decision_id = p.payable_id
		   LEFT JOIN settlement_accounting.supplier_bill_number AS n
		     ON n.tenant_id = p.tenant_id AND n.claim_id = p.claim_id
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
		    AND ($4::text = '' OR r.supplier_ref IS NULL OR r.supplier_ref = $4)
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
			resolves, billNo    *string
		)
		if err := rows.Scan(
			&row.Payable, &number, &absence, &row.Claim, &row.Line, &row.ExpectedVersion,
			&supplier, &period, &row.LegalEntity, &row.Account, &row.Currency, &row.AmountMinor,
			&row.AuditorRef, &evidence, &auditedAt, &recorded,
			&row.RangeFromMinor, &row.RangeToMinor, &resolves, &billNo,
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
		row.ResolvesDispute, row.SupplierBillNumber = catalogueText(resolves), catalogueText(billNo)
		row.AuditedAt, row.RecordedAt = auditedAt.UTC(), recorded.UTC()
		entries = append(entries, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list audited payables: %w", err)
	}
	return entries, nil
}
