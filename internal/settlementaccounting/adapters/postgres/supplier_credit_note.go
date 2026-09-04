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

// SupplierCreditNotes 实现 ports.SupplierCreditNoteStore。册只追加：写口没有 UPDATE 分支，
// 同（身份+版本）重放答`已登记`（ADR-0031），原贷项与它回指的原应付都不被改写。
type SupplierCreditNotes struct {
	db *bentopg.DB
}

func NewSupplierCreditNotes(db *bentopg.DB) (*SupplierCreditNotes, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &SupplierCreditNotes{db: db}, nil
}

var _ ports.SupplierCreditNoteStore = (*SupplierCreditNotes)(nil)

// FindByKey 按幂等键取回一份贷项。否定结果只回 false，不区分「不存在」与「属于另一个租户」。
// 读回走 FormSupplierCreditNote 而不旁路形成门：贷项规格里没有聚合本体，形成门本身就是
// 单一权威，不必另开一扇重建门。
func (repository *SupplierCreditNotes) FindByKey(
	ctx context.Context,
	key ports.SupplierCreditNoteKey,
) (ports.SupplierCreditNoteRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.SupplierCreditNoteRecord{}, false, fmt.Errorf("find supplier credit note: %w", err)
	}

	var (
		payable, claim, account, currency, reason, digest string
		amountMinor                                       int64
		issuedAt, recordedAt                              time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT payable_id, claim_id, account_id, currency, amount_minor, reason_ref,
		        issued_at, content_digest, recorded_at
		   FROM settlement_accounting.supplier_credit_note
		  WHERE tenant_id = $1
		    AND note_id = $2
		    AND note_version = $3`,
		key.TenantID.String(),
		key.Note.String(),
		key.Version.String(),
	).Scan(&payable, &claim, &account, &currency, &amountMinor, &reason, &issuedAt, &digest, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.SupplierCreditNoteRecord{}, false, nil
	}
	if err != nil {
		return ports.SupplierCreditNoteRecord{}, false, fmt.Errorf("find supplier credit note: %w", err)
	}

	note, err := rebuildCreditNote(key, creditNoteColumns{
		payable: payable, claim: claim, account: account, currency: currency,
		amountMinor: amountMinor, reason: reason, issuedAt: issuedAt,
	})
	if err != nil {
		return ports.SupplierCreditNoteRecord{}, false, fmt.Errorf("find supplier credit note: %w", err)
	}
	return ports.SupplierCreditNoteRecord{
		Key:           key,
		ContentDigest: digest,
		Note:          note,
		RecordedAt:    recordedAt.UTC(),
	}, true, nil
}

// Save 写下一份贷项。同（身份+版本）已有记录时答`已登记`，不覆盖先到者。
func (repository *SupplierCreditNotes) Save(
	ctx context.Context,
	record ports.SupplierCreditNoteRecord,
) (ports.SupplierCreditNoteSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.SupplierCreditNoteSaveOutcomeInvalid, fmt.Errorf("save supplier credit note: %w", err)
	}

	note := record.Note
	if note.Note() != record.Key.Note || note.Version() != record.Key.Version {
		return ports.SupplierCreditNoteSaveOutcomeInvalid, fmt.Errorf(
			"save supplier credit note: record key disagrees with the note's identity")
	}
	currency, minor := note.Amount()
	tag, err := executor.Exec(ctx,
		`INSERT INTO settlement_accounting.supplier_credit_note
			(tenant_id, note_id, note_version, payable_id, claim_id, account_id,
			 currency, amount_minor, reason_ref, issued_at, content_digest, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		note.Note().String(),
		note.Version().String(),
		note.Payable().String(),
		note.Claim().String(),
		note.Account().String(),
		currency.String(),
		minor,
		note.Reason().String(),
		note.IssuedAt().UTC(),
		record.ContentDigest,
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.SupplierCreditNoteSaveOutcomeInvalid, fmt.Errorf("save supplier credit note: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.SupplierCreditNoteAlreadyRecorded, nil
	}
	return ports.SupplierCreditNoteSaved, nil
}

type creditNoteColumns struct {
	payable, claim, account, currency, reason string
	amountMinor                               int64
	issuedAt                                  time.Time
}

func rebuildCreditNote(key ports.SupplierCreditNoteKey, columns creditNoteColumns) (domain.SupplierCreditNote, error) {
	spec := domain.SupplierCreditNoteSpec{
		Note:        key.Note,
		Version:     key.Version,
		AmountMinor: columns.amountMinor,
		IssuedAt:    columns.issuedAt,
	}
	var err error
	if spec.Payable, err = domain.NewPayableID(columns.payable); err != nil {
		return domain.SupplierCreditNote{}, err
	}
	if spec.Claim, err = domain.NewBillClaimID(columns.claim); err != nil {
		return domain.SupplierCreditNote{}, err
	}
	if spec.Account, err = domain.NewSettlementAccountID(columns.account); err != nil {
		return domain.SupplierCreditNote{}, err
	}
	if spec.Currency, err = domain.NewCurrencyCode(columns.currency); err != nil {
		return domain.SupplierCreditNote{}, err
	}
	if spec.Reason, err = domain.NewCreditReasonReference(columns.reason); err != nil {
		return domain.SupplierCreditNote{}, err
	}
	return domain.FormSupplierCreditNote(spec)
}
