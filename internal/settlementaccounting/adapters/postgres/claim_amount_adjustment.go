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

// ClaimAmountAdjustments 实现 ports.ClaimAmountAdjustmentStore（写入代数同
// ADR-0031）。同一调整标识只形成一次。
type ClaimAmountAdjustments struct {
	db *bentopg.DB
}

func NewClaimAmountAdjustments(db *bentopg.DB) (*ClaimAmountAdjustments, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &ClaimAmountAdjustments{db: db}, nil
}

// FindByKey 按（租户+调整）取回。否定结果只回 false。读回经重建门复验封闭三因，
// 不重审目标金额是否仍在场。
func (repository *ClaimAmountAdjustments) FindByKey(
	ctx context.Context,
	key ports.ClaimAdjustmentKey,
) (ports.ClaimAdjustmentRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ClaimAdjustmentRecord{}, false, fmt.Errorf("find claim amount adjustment: %w", err)
	}

	var targetKindName, target, reasonName, basis, directionName, currency, period, digest string
	var amount int64
	var formedAt, recordedAt time.Time
	err = querier.QueryRow(ctx,
		`SELECT target_kind, target_ref, reason, basis_ref, direction, currency,
		        amount_minor, period_ref, formed_at, content_digest, recorded_at
		   FROM settlement_accounting.claim_amount_adjustment
		  WHERE tenant_id = $1
		    AND adjustment_id = $2`,
		key.TenantID.String(),
		key.Adjustment.String(),
	).Scan(&targetKindName, &target, &reasonName, &basis, &directionName, &currency,
		&amount, &period, &formedAt, &digest, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.ClaimAdjustmentRecord{}, false, nil
	}
	if err != nil {
		return ports.ClaimAdjustmentRecord{}, false, fmt.Errorf("find claim amount adjustment: %w", err)
	}

	adjustment, err := rebuildClaimAmountAdjustment(key.Adjustment, targetKindName, target, reasonName, basis, directionName, currency, amount, period, formedAt)
	if err != nil {
		return ports.ClaimAdjustmentRecord{}, false, fmt.Errorf("find claim amount adjustment: %w", err)
	}
	return ports.ClaimAdjustmentRecord{
		Key:           key,
		ContentDigest: digest,
		Adjustment:    adjustment,
		RecordedAt:    recordedAt.UTC(),
	}, true, nil
}

// Save 写下一笔索赔金额调整。同标识已有记录时答`已形成`，不覆盖先到者。
func (repository *ClaimAmountAdjustments) Save(
	ctx context.Context,
	record ports.ClaimAdjustmentRecord,
) (ports.ClaimAdjustmentSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ClaimAdjustmentSaveOutcomeInvalid, fmt.Errorf("save claim amount adjustment: %w", err)
	}

	currency, amount := record.Adjustment.Amount()
	tag, err := executor.Exec(ctx,
		`INSERT INTO settlement_accounting.claim_amount_adjustment
			(tenant_id, adjustment_id, target_kind, target_ref, reason, basis_ref,
			 direction, currency, amount_minor, period_ref, formed_at,
			 content_digest, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Adjustment.String(),
		record.Adjustment.TargetKind().String(),
		record.Adjustment.Target().String(),
		record.Adjustment.Reason().String(),
		record.Adjustment.Basis().String(),
		record.Adjustment.Direction().String(),
		currency.String(),
		amount,
		record.Adjustment.Period().String(),
		record.Adjustment.FormedAt().UTC(),
		record.ContentDigest,
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.ClaimAdjustmentSaveOutcomeInvalid, fmt.Errorf("save claim amount adjustment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ClaimAdjustmentAlreadyFormed, nil
	}
	return ports.ClaimAdjustmentSaved, nil
}

func rebuildClaimAmountAdjustment(
	id domain.ClaimAmountAdjustmentID,
	targetKindName, target, reasonName, basis, directionName, currency string,
	amount int64,
	period string,
	formedAt time.Time,
) (domain.ClaimAmountAdjustment, error) {
	targetKind, err := adjustedAmountKindFrom(targetKindName)
	if err != nil {
		return domain.ClaimAmountAdjustment{}, err
	}
	targetRef, err := domain.NewAdjustedAmountReference(target)
	if err != nil {
		return domain.ClaimAmountAdjustment{}, err
	}
	reason, err := claimAdjustmentReasonFrom(reasonName)
	if err != nil {
		return domain.ClaimAmountAdjustment{}, err
	}
	basisRef, err := domain.NewResponsibilityConclusionReference(basis)
	if err != nil {
		return domain.ClaimAmountAdjustment{}, err
	}
	direction, err := adjustmentDirectionFrom(directionName)
	if err != nil {
		return domain.ClaimAmountAdjustment{}, err
	}
	currencyCode, err := domain.NewCurrencyCode(currency)
	if err != nil {
		return domain.ClaimAmountAdjustment{}, err
	}
	periodRef, err := domain.NewBillingPeriodReference(period)
	if err != nil {
		return domain.ClaimAmountAdjustment{}, err
	}
	return domain.RehydrateClaimAmountAdjustment(domain.RehydrateClaimAmountAdjustmentSpec{
		ID:          id,
		TargetKind:  targetKind,
		Target:      targetRef,
		Reason:      reason,
		Basis:       basisRef,
		Direction:   direction,
		Currency:    currencyCode,
		AmountMinor: amount,
		Period:      periodRef,
		FormedAt:    formedAt,
	})
}

func adjustedAmountKindFrom(raw string) (domain.AdjustedAmountKind, error) {
	switch raw {
	case domain.AdjustsCustomerClaimAmount.String():
		return domain.AdjustsCustomerClaimAmount, nil
	case domain.AdjustsRecoveryReceivable.String():
		return domain.AdjustsRecoveryReceivable, nil
	case domain.AdjustsAcknowledgement.String():
		return domain.AdjustsAcknowledgement, nil
	default:
		return 0, fmt.Errorf("unknown adjusted amount kind %q", raw)
	}
}

func claimAdjustmentReasonFrom(raw string) (domain.ClaimAdjustmentReason, error) {
	switch raw {
	case domain.ResponsibilityRevised.String():
		return domain.ResponsibilityRevised, nil
	case domain.AmountRuleCorrected.String():
		return domain.AmountRuleCorrected, nil
	case domain.AcknowledgementChanged.String():
		return domain.AcknowledgementChanged, nil
	default:
		return 0, fmt.Errorf("unknown claim adjustment reason %q", raw)
	}
}
