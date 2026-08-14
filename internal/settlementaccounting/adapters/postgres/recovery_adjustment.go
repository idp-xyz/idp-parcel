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

// RecoveryAdjustments 实现 ports.RecoveryAdjustmentStore（写入代数同 ADR-0031）。
// 同一调整标识只形成一次。
type RecoveryAdjustments struct {
	db *bentopg.DB
}

func NewRecoveryAdjustments(db *bentopg.DB) (*RecoveryAdjustments, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &RecoveryAdjustments{db: db}, nil
}

// FindByKey 按（租户+调整）取回。否定结果只回 false。读回经重建门复验封闭三因，
// 不重审原回收是否仍在场。
func (repository *RecoveryAdjustments) FindByKey(
	ctx context.Context,
	key ports.RecoveryAdjustmentKey,
) (ports.RecoveryAdjustmentRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.RecoveryAdjustmentRecord{}, false, fmt.Errorf("find recovery adjustment: %w", err)
	}

	var recovery, reasonName, basis, directionName, currency, period, digest string
	var amount int64
	var formedAt, recordedAt time.Time
	err = querier.QueryRow(ctx,
		`SELECT recovery_id, reason, new_basis, direction, currency, amount_minor,
		        period_ref, formed_at, content_digest, recorded_at
		   FROM settlement_accounting.recovery_adjustment
		  WHERE tenant_id = $1
		    AND adjustment_id = $2`,
		key.TenantID.String(),
		key.Adjustment.String(),
	).Scan(&recovery, &reasonName, &basis, &directionName, &currency, &amount,
		&period, &formedAt, &digest, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.RecoveryAdjustmentRecord{}, false, nil
	}
	if err != nil {
		return ports.RecoveryAdjustmentRecord{}, false, fmt.Errorf("find recovery adjustment: %w", err)
	}

	adjustment, err := rebuildRecoveryAdjustment(key.Adjustment, recovery, reasonName, basis, directionName, currency, amount, period, formedAt)
	if err != nil {
		return ports.RecoveryAdjustmentRecord{}, false, fmt.Errorf("find recovery adjustment: %w", err)
	}
	return ports.RecoveryAdjustmentRecord{
		Key:           key,
		ContentDigest: digest,
		Adjustment:    adjustment,
		RecordedAt:    recordedAt.UTC(),
	}, true, nil
}

// Save 写下一笔回收调整。同标识已有记录时答`已形成`，不覆盖先到者。
func (repository *RecoveryAdjustments) Save(
	ctx context.Context,
	record ports.RecoveryAdjustmentRecord,
) (ports.RecoveryAdjustmentSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.RecoveryAdjustmentSaveOutcomeInvalid, fmt.Errorf("save recovery adjustment: %w", err)
	}

	currency, amount := record.Adjustment.Amount()
	tag, err := executor.Exec(ctx,
		`INSERT INTO settlement_accounting.recovery_adjustment
			(tenant_id, adjustment_id, recovery_id, reason, new_basis, direction,
			 currency, amount_minor, period_ref, formed_at, content_digest, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Adjustment.String(),
		record.Adjustment.Recovery().String(),
		record.Adjustment.Reason().String(),
		record.Adjustment.NewBasis().String(),
		record.Adjustment.Direction().String(),
		currency.String(),
		amount,
		record.Adjustment.Period().String(),
		record.Adjustment.FormedAt().UTC(),
		record.ContentDigest,
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.RecoveryAdjustmentSaveOutcomeInvalid, fmt.Errorf("save recovery adjustment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.RecoveryAdjustmentAlreadyFormed, nil
	}
	return ports.RecoveryAdjustmentSaved, nil
}

func rebuildRecoveryAdjustment(
	id domain.RecoveryAdjustmentID,
	recovery, reasonName, basis, directionName, currency string,
	amount int64,
	period string,
	formedAt time.Time,
) (domain.RecoveryAdjustment, error) {
	recoveryID, err := domain.NewAdvanceRecoveryID(recovery)
	if err != nil {
		return domain.RecoveryAdjustment{}, err
	}
	reason, err := recoveryAdjustmentReasonFrom(reasonName)
	if err != nil {
		return domain.RecoveryAdjustment{}, err
	}
	basisRef, err := domain.NewAssessmentBasisReference(basis)
	if err != nil {
		return domain.RecoveryAdjustment{}, err
	}
	direction, err := adjustmentDirectionFrom(directionName)
	if err != nil {
		return domain.RecoveryAdjustment{}, err
	}
	currencyCode, err := domain.NewCurrencyCode(currency)
	if err != nil {
		return domain.RecoveryAdjustment{}, err
	}
	periodRef, err := domain.NewBillingPeriodReference(period)
	if err != nil {
		return domain.RecoveryAdjustment{}, err
	}
	return domain.RehydrateRecoveryAdjustment(domain.RehydrateRecoveryAdjustmentSpec{
		ID:          id,
		Recovery:    recoveryID,
		Reason:      reason,
		NewBasis:    basisRef,
		Direction:   direction,
		Currency:    currencyCode,
		AmountMinor: amount,
		Period:      periodRef,
		FormedAt:    formedAt,
	})
}

func recoveryAdjustmentReasonFrom(raw string) (domain.RecoveryAdjustmentReason, error) {
	switch raw {
	case domain.TaxAssessmentCorrected.String():
		return domain.TaxAssessmentCorrected, nil
	case domain.FundsFactRevised.String():
		return domain.FundsFactRevised, nil
	case domain.CustomerResponsibilityChanged.String():
		return domain.CustomerResponsibilityChanged, nil
	default:
		return 0, fmt.Errorf("unknown recovery adjustment reason %q", raw)
	}
}
