package postgres

import (
	"context"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// SupplierBillAuditDecisions 实现 ports.AuditDecisionStore，读写 0054 的审核决定册。主键（租户，决定身份）与两道范围排他约束都由
// ON CONFLICT DO NOTHING 加零行判定翻译成`已有记录`（ADR-0031）：不带冲突目标的 DO NOTHING 连排他约束也接住，撞哪一道都不把
// 事务打进中止态——编排还要同事务读回赢家作答。
type SupplierBillAuditDecisions struct {
	db *bentopg.DB
}

func NewSupplierBillAuditDecisions(db *bentopg.DB) (*SupplierBillAuditDecisions, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &SupplierBillAuditDecisions{db: db}, nil
}

var _ ports.AuditDecisionStore = (*SupplierBillAuditDecisions)(nil)

const auditDecisionColumns = `decision_id, claim_id, claim_version, line_ref, verdict, range_from_minor, range_to_minor,
	        basis_ref, resolves_decision, auditor_ref, operation_evidence, audited_at, content_digest, recorded_at`

// LockLine 以两把事务级咨询锁把同一决定身份、同一行主张的审核排成一队（见 ports.AuditDecisionStore）。先身份后行，次序固定：每笔
// 审核各只拿一个身份与一行，按同一次序取就不会互等成环。
func (repository *SupplierBillAuditDecisions) LockLine(
	ctx context.Context,
	key ports.AuditDecisionKey,
	claim domain.BillClaimID,
	line domain.BillLineReference,
) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("lock supplier bill audit: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext('settlement_accounting.supplier_bill_audit_decision.identity'), hashtext($1 || chr(31) || $2))`,
		key.TenantID.String(), key.Decision.String()); err != nil {
		return fmt.Errorf("lock supplier bill audit: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext('settlement_accounting.supplier_bill_audit_decision.line'), hashtext($1 || chr(31) || $2 || chr(31) || $3))`,
		key.TenantID.String(), claim.String(), line.String()); err != nil {
		return fmt.Errorf("lock supplier bill audit: %w", err)
	}
	return nil
}

// FindByKey 按幂等键取回一份审核决定。否定结果只回 false，不区分「不存在」与「属于另一个租户」。
func (repository *SupplierBillAuditDecisions) FindByKey(
	ctx context.Context,
	key ports.AuditDecisionKey,
) (ports.AuditDecisionRecord, bool, error) {
	records, err := repository.query(ctx, key.TenantID,
		`WHERE tenant_id = $1 AND decision_id = $2`, key.TenantID.String(), key.Decision.String())
	if err != nil {
		return ports.AuditDecisionRecord{}, false, fmt.Errorf("find supplier bill audit decision: %w", err)
	}
	if len(records) == 0 {
		return ports.AuditDecisionRecord{}, false, nil
	}
	return records[0], true, nil
}

// ListByLine 读同一主张同一行上的全部审核决定，按落库先后。
func (repository *SupplierBillAuditDecisions) ListByLine(
	ctx context.Context,
	tenant domain.TenantID,
	claim domain.BillClaimID,
	line domain.BillLineReference,
) ([]ports.AuditDecisionRecord, error) {
	records, err := repository.query(ctx, tenant,
		`WHERE tenant_id = $1 AND claim_id = $2 AND line_ref = $3 ORDER BY recorded_at, decision_id`,
		tenant.String(), claim.String(), line.String())
	if err != nil {
		return nil, fmt.Errorf("list supplier bill audit decisions: %w", err)
	}
	return records, nil
}

func (repository *SupplierBillAuditDecisions) query(
	ctx context.Context,
	tenant domain.TenantID,
	where string,
	args ...any,
) ([]ports.AuditDecisionRecord, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := querier.Query(ctx,
		`SELECT `+auditDecisionColumns+` FROM settlement_accounting.supplier_bill_audit_decision `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []ports.AuditDecisionRecord
	for rows.Next() {
		var row auditDecisionRow
		if err := rows.Scan(row.scanTargets()...); err != nil {
			return nil, err
		}
		decision, err := rebuildAuditDecision(row)
		if err != nil {
			return nil, err
		}
		records = append(records, ports.AuditDecisionRecord{
			Key:               ports.AuditDecisionKey{TenantID: tenant, Decision: decision.Decision()},
			ContentDigest:     row.digest,
			Decision:          decision,
			RecordedAt:        row.recordedAt.UTC(),
			OperationEvidence: catalogueText(row.evidence),
		})
	}
	return records, rows.Err()
}

// Save 写下一份审核决定。同身份已有、或这一段撞上同一行（同一争议）已有的一段，都答`已有记录`——业务答案不是错误（ADR-0031）。
func (repository *SupplierBillAuditDecisions) Save(
	ctx context.Context,
	record ports.AuditDecisionRecord,
) (ports.AuditDecisionSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.AuditDecisionSaveOutcomeInvalid, fmt.Errorf("save supplier bill audit decision: %w", err)
	}
	decision := record.Decision
	if decision.Decision() != record.Key.Decision {
		return ports.AuditDecisionSaveOutcomeInvalid, fmt.Errorf(
			"save supplier bill audit decision: record key disagrees with the decision's identity")
	}
	var version, basis, resolves *string
	if value, has := decision.Version(); has {
		version = optionalText(value.String())
	}
	if value, has := decision.Basis(); has {
		basis = optionalText(value.String())
	}
	if value, has := decision.Resolves(); has {
		resolves = optionalText(value.String())
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO settlement_accounting.supplier_bill_audit_decision
			(tenant_id, decision_id, claim_id, claim_version, line_ref, verdict, range_from_minor, range_to_minor,
			 basis_ref, resolves_decision, auditor_ref, operation_evidence, audited_at, content_digest, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		decision.Decision().String(),
		decision.Claim().String(),
		version,
		decision.Line().String(),
		decision.Verdict().String(),
		decision.Span().From(),
		decision.Span().To(),
		basis,
		resolves,
		decision.Auditor().String(),
		optionalText(record.OperationEvidence),
		decision.AuditedAt().UTC(),
		record.ContentDigest,
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.AuditDecisionSaveOutcomeInvalid, fmt.Errorf("save supplier bill audit decision: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.AuditDecisionAlreadyRecorded, nil
	}
	return ports.AuditDecisionSaved, nil
}

// auditDecisionRow 是 supplier_bill_audit_decision 一行的原始列值，交给重建门复验。
type auditDecisionRow struct {
	decision, claim string
	version         *string
	line, verdict   string
	from, to        int64
	basis, resolves *string
	auditor         string
	evidence        *string
	auditedAt       time.Time
	digest          string
	recordedAt      time.Time
}

func (row *auditDecisionRow) scanTargets() []any {
	return []any{
		&row.decision, &row.claim, &row.version, &row.line, &row.verdict, &row.from, &row.to,
		&row.basis, &row.resolves, &row.auditor, &row.evidence, &row.auditedAt, &row.digest, &row.recordedAt,
	}
}

// rebuildAuditDecision 让行过 RehydrateBillLineAuditDecision 回来（ADR-0030 的纪律）：每个引用各自过自己的构造函数。
func rebuildAuditDecision(row auditDecisionRow) (domain.BillLineAuditDecision, error) {
	spec := domain.RehydrateBillLineAuditDecisionSpec{AuditedAt: row.auditedAt}
	var err error
	if spec.Decision, err = domain.NewAuditDecisionID(row.decision); err != nil {
		return domain.BillLineAuditDecision{}, err
	}
	if spec.Claim, err = domain.NewBillClaimID(row.claim); err != nil {
		return domain.BillLineAuditDecision{}, err
	}
	if row.version != nil {
		if spec.Version, err = domain.NewBillClaimVersion(*row.version); err != nil {
			return domain.BillLineAuditDecision{}, err
		}
	}
	if spec.Line, err = domain.NewBillLineReference(row.line); err != nil {
		return domain.BillLineAuditDecision{}, err
	}
	if spec.Verdict, err = domain.AuditVerdictFromName(row.verdict); err != nil {
		return domain.BillLineAuditDecision{}, err
	}
	if spec.Span, err = domain.NewAmountRange(row.from, row.to); err != nil {
		return domain.BillLineAuditDecision{}, err
	}
	if row.basis != nil {
		if spec.Basis, err = domain.NewAuditBasisReference(*row.basis); err != nil {
			return domain.BillLineAuditDecision{}, err
		}
	}
	if row.resolves != nil {
		if spec.Resolves, err = domain.NewAuditDecisionID(*row.resolves); err != nil {
			return domain.BillLineAuditDecision{}, err
		}
	}
	if spec.Auditor, err = domain.NewAuditorReference(row.auditor); err != nil {
		return domain.BillLineAuditDecision{}, err
	}
	return domain.RehydrateBillLineAuditDecision(spec)
}
