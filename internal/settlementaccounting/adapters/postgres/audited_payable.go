package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// AuditedPayables 实现 ports.AuditedPayableStore。主键（租户+应付身份）与唯一约束（租户+
// 主张+行）都由 ON CONFLICT DO NOTHING 加零行判定翻译成`已有记录`（ADR-0031），撞哪一个都
// 不把事务打进中止态——编排还要同事务按行读回赢家作答。
type AuditedPayables struct {
	db *bentopg.DB
}

func NewAuditedPayables(db *bentopg.DB) (*AuditedPayables, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &AuditedPayables{db: db}, nil
}

var _ ports.AuditedPayableStore = (*AuditedPayables)(nil)

const auditedPayableTable = "settlement_accounting.audited_payable"

const auditedPayableColumns = `payable_id, claim_id, line_ref, expected_version, legal_entity,
	        account_id, currency, amount_minor, auditor_ref, audited_at, content_digest, recorded_at,
	        document_number, document_number_absence`

// FindByKey 按幂等键取回一份应付。否定结果只回 false，不区分「不存在」与「属于另一个租户」。
func (repository *AuditedPayables) FindByKey(
	ctx context.Context,
	key ports.AuditedPayableKey,
) (ports.AuditedPayableRecord, bool, error) {
	return repository.findOne(ctx, key.TenantID,
		`WHERE tenant_id = $1 AND payable_id = $2`, key.TenantID.String(), key.Payable.String())
}

// FindByLine 按（主张+行）取回那一行的应付——一行只成立一份，所以最多一条。
func (repository *AuditedPayables) FindByLine(
	ctx context.Context,
	tenant domain.TenantID,
	claim domain.BillClaimID,
	line domain.BillLineReference,
) (ports.AuditedPayableRecord, bool, error) {
	return repository.findOne(ctx, tenant,
		`WHERE tenant_id = $1 AND claim_id = $2 AND line_ref = $3`, tenant.String(), claim.String(), line.String())
}

func (repository *AuditedPayables) findOne(
	ctx context.Context,
	tenant domain.TenantID,
	where string,
	args ...any,
) (ports.AuditedPayableRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.AuditedPayableRecord{}, false, fmt.Errorf("find audited payable: %w", err)
	}

	var row payableRow
	err = querier.QueryRow(ctx,
		`SELECT `+auditedPayableColumns+`
		   FROM settlement_accounting.audited_payable `+where,
		args...,
	).Scan(row.scanTargets()...)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.AuditedPayableRecord{}, false, nil
	}
	if err != nil {
		return ports.AuditedPayableRecord{}, false, fmt.Errorf("find audited payable: %w", err)
	}

	payable, err := rebuildAuditedPayable(row)
	if err != nil {
		return ports.AuditedPayableRecord{}, false, fmt.Errorf("find audited payable: %w", err)
	}
	numbering, err := numberingFrom(row.documentNumber, row.numberAbsence)
	if err != nil {
		return ports.AuditedPayableRecord{}, false, fmt.Errorf("find audited payable: %w", err)
	}
	return ports.AuditedPayableRecord{
		Key:           ports.AuditedPayableKey{TenantID: tenant, Payable: payable.Payable()},
		ContentDigest: row.digest,
		Payable:       payable,
		RecordedAt:    row.recordedAt.UTC(),
		Numbering:     numbering,
	}, true, nil
}

// LockIssuance 以两把事务级咨询锁把同一应付身份、同一行主张的审核排成一队（见 ports.AuditedPayableStore）。先身份后行，
// 次序固定：每笔审核各只拿一个身份与一行，按同一次序取就不会互等成环。
func (repository *AuditedPayables) LockIssuance(
	ctx context.Context,
	key ports.AuditedPayableKey,
	claim domain.BillClaimID,
	line domain.BillLineReference,
) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("lock audited payable issuance: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext('settlement_accounting.audited_payable.identity'), hashtext($1 || chr(31) || $2))`,
		key.TenantID.String(), key.Payable.String()); err != nil {
		return fmt.Errorf("lock audited payable issuance: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext('settlement_accounting.audited_payable.line'), hashtext($1 || chr(31) || $2 || chr(31) || $3))`,
		key.TenantID.String(), claim.String(), line.String()); err != nil {
		return fmt.Errorf("lock audited payable issuance: %w", err)
	}
	return nil
}

// DocumentNumberUsed 答一个应付号是否已在本租户的审核应付册上。
func (repository *AuditedPayables) DocumentNumberUsed(ctx context.Context, tenant domain.TenantID, number string) (bool, error) {
	used, err := documentNumberUsed(ctx, repository.db, auditedPayableTable, tenant, number)
	if err != nil {
		return false, fmt.Errorf("audited payable number used: %w", err)
	}
	return used, nil
}

// Save 写下一份审核应付。同身份或同一行已有应付时答`已有记录`——业务答案不是错误
// （ADR-0031），编排据此按行读回赢家。
func (repository *AuditedPayables) Save(
	ctx context.Context,
	record ports.AuditedPayableRecord,
) (ports.AuditedPayableSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.AuditedPayableSaveOutcomeInvalid, fmt.Errorf("save audited payable: %w", err)
	}

	payable := record.Payable
	if payable.Payable() != record.Key.Payable {
		return ports.AuditedPayableSaveOutcomeInvalid, fmt.Errorf(
			"save audited payable: record key disagrees with the payable's identity")
	}
	number, absence, err := numberingColumns(record.Numbering)
	if err != nil {
		return ports.AuditedPayableSaveOutcomeInvalid, fmt.Errorf("save audited payable: %w", err)
	}
	currency, minor := payable.Amount()
	tag, err := executor.Exec(ctx,
		`INSERT INTO settlement_accounting.audited_payable
			(tenant_id, payable_id, claim_id, line_ref, expected_version, legal_entity,
			 account_id, currency, amount_minor, auditor_ref, audited_at, content_digest, recorded_at, operation_evidence,
			 document_number, document_number_absence)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		payable.Payable().String(),
		payable.Claim().String(),
		payable.Line().String(),
		payable.Expected().String(),
		payable.LegalEntity().String(),
		payable.Account().String(),
		currency.String(),
		minor,
		payable.Auditor().String(),
		payable.AuditedAt().UTC(),
		record.ContentDigest,
		record.RecordedAt.UTC(),
		optionalText(record.OperationEvidence),
		number,
		absence,
	)
	if err != nil {
		return ports.AuditedPayableSaveOutcomeInvalid, fmt.Errorf("save audited payable: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.AuditedPayableAlreadyRecorded, nil
	}
	return ports.AuditedPayableSaved, nil
}

// payableRow 是 audited_payable 一行的原始列值，交给重建门复验。
type payableRow struct {
	payable, claim, line, expected string
	legalEntity, account, currency string
	amountMinor                    int64
	auditor                        string
	auditedAt                      time.Time
	digest                         string
	recordedAt                     time.Time
	documentNumber, numberAbsence  *string
}

func (row *payableRow) scanTargets() []any {
	return []any{
		&row.payable, &row.claim, &row.line, &row.expected,
		&row.legalEntity, &row.account, &row.currency, &row.amountMinor,
		&row.auditor, &row.auditedAt, &row.digest, &row.recordedAt,
		&row.documentNumber, &row.numberAbsence,
	}
}

// rebuildAuditedPayable 让行过 RehydrateAuditedPayable 回来（ADR-0030 的纪律）：每个引用
// 各自过自己的构造函数，空白值在这里就报错。
func rebuildAuditedPayable(row payableRow) (domain.AuditedPayable, error) {
	spec := domain.RehydrateAuditedPayableSpec{
		AmountMinor: row.amountMinor,
		AuditedAt:   row.auditedAt,
	}
	var err error
	if spec.Payable, err = domain.NewPayableID(row.payable); err != nil {
		return domain.AuditedPayable{}, err
	}
	if spec.Claim, err = domain.NewBillClaimID(row.claim); err != nil {
		return domain.AuditedPayable{}, err
	}
	if spec.Line, err = domain.NewBillLineReference(row.line); err != nil {
		return domain.AuditedPayable{}, err
	}
	if spec.Expected, err = domain.NewSupplierCostVersionID(row.expected); err != nil {
		return domain.AuditedPayable{}, err
	}
	if spec.LegalEntity, err = domain.NewLegalEntityReference(row.legalEntity); err != nil {
		return domain.AuditedPayable{}, err
	}
	if spec.Account, err = domain.NewSettlementAccountID(row.account); err != nil {
		return domain.AuditedPayable{}, err
	}
	if spec.Currency, err = domain.NewCurrencyCode(row.currency); err != nil {
		return domain.AuditedPayable{}, err
	}
	if spec.Auditor, err = domain.NewAuditorReference(row.auditor); err != nil {
		return domain.AuditedPayable{}, err
	}
	return domain.RehydrateAuditedPayable(spec)
}
