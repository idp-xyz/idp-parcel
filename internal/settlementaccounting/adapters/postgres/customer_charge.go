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

// CustomerCharges 实现 ports.CustomerChargeStore。SaveConfirmed 落确认；已确认行
// 不会被第二次确认覆盖（ADR-0031）。
type CustomerCharges struct {
	db *bentopg.DB
}

func NewCustomerCharges(db *bentopg.DB) (*CustomerCharges, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &CustomerCharges{db: db}, nil
}

// FindByID 按（租户+费用）取回。否定结果只回 false。已确认的读回经 Form 再精确重放
// 一次 Confirm——确认是转换门，行上存的是定格后的两半。
func (repository *CustomerCharges) FindByID(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.CustomerChargeID,
) (domain.CustomerCharge, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.CustomerCharge{}, false, fmt.Errorf("find customer charge: %w", err)
	}

	var feeItem, evaluation, originalCurrency, settlementCurrency, stageName string
	var originalMinor, settlementMinor int64
	var conversion *string
	var formedAt, recordedAt time.Time
	var confirmation *string
	var confirmedAt *time.Time
	err = querier.QueryRow(ctx,
		`SELECT fee_item, evaluation_ref, original_currency, original_minor,
		        settlement_currency, settlement_minor, conversion_ref, stage,
		        confirmation_basis, formed_at, confirmed_at, recorded_at
		   FROM settlement_accounting.customer_charge
		  WHERE tenant_id = $1
		    AND charge_id = $2`,
		tenant.String(),
		id.String(),
	).Scan(&feeItem, &evaluation, &originalCurrency, &originalMinor,
		&settlementCurrency, &settlementMinor, &conversion, &stageName,
		&confirmation, &formedAt, &confirmedAt, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.CustomerCharge{}, false, nil
	}
	if err != nil {
		return domain.CustomerCharge{}, false, fmt.Errorf("find customer charge: %w", err)
	}

	charge, err := rebuildCustomerCharge(chargeRow{
		id:                 id,
		feeItem:            feeItem,
		evaluation:         evaluation,
		originalCurrency:   originalCurrency,
		originalMinor:      originalMinor,
		settlementCurrency: settlementCurrency,
		settlementMinor:    settlementMinor,
		conversion:         conversion,
		stageName:          stageName,
		confirmation:       confirmation,
		formedAt:           formedAt,
		confirmedAt:        confirmedAt,
	})
	if err != nil {
		return domain.CustomerCharge{}, false, fmt.Errorf("find customer charge: %w", err)
	}
	return charge, true, nil
}

// SaveConfirmed 写下一次确认。同标识已确认时答`已确认`，不覆盖先到者的依据。
func (repository *CustomerCharges) SaveConfirmed(
	ctx context.Context,
	tenant domain.TenantID,
	charge domain.CustomerCharge,
) (ports.ChargeSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ChargeSaveOutcomeInvalid, fmt.Errorf("save confirmed charge: %w", err)
	}
	if charge.Stage() != domain.ChargeConfirmed {
		return ports.ChargeSaveOutcomeInvalid, fmt.Errorf("save confirmed charge: charge is not confirmed")
	}
	basis, ok := charge.Confirmation()
	if !ok {
		return ports.ChargeSaveOutcomeInvalid, fmt.Errorf("save confirmed charge: confirmation basis missing")
	}
	confirmedAt, ok := charge.ConfirmedAt()
	if !ok {
		return ports.ChargeSaveOutcomeInvalid, fmt.Errorf("save confirmed charge: confirmed at missing")
	}
	originalCurrency, originalMinor := charge.OriginalAmount()
	settlementCurrency, settlementMinor := charge.SettlementAmount()
	var conversion *string
	if reference, present := charge.Conversion(); present {
		value := reference.String()
		conversion = &value
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO settlement_accounting.customer_charge
			(tenant_id, charge_id, fee_item, evaluation_ref, original_currency,
			 original_minor, settlement_currency, settlement_minor, conversion_ref,
			 stage, confirmation_basis, formed_at, confirmed_at, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'CONFIRMED', $10, $11, $12, $12)
		 ON CONFLICT (tenant_id, charge_id) DO UPDATE
		    SET stage = 'CONFIRMED',
		        confirmation_basis = EXCLUDED.confirmation_basis,
		        confirmed_at = EXCLUDED.confirmed_at,
		        recorded_at = EXCLUDED.recorded_at
		  WHERE settlement_accounting.customer_charge.stage IS DISTINCT FROM 'CONFIRMED'`,
		tenant.String(),
		charge.ID().String(),
		charge.FeeItem().String(),
		charge.Evaluation().String(),
		originalCurrency.String(),
		originalMinor,
		settlementCurrency.String(),
		settlementMinor,
		conversion,
		basis.String(),
		charge.FormedAt().UTC(),
		confirmedAt.UTC(),
	)
	if err != nil {
		return ports.ChargeSaveOutcomeInvalid, fmt.Errorf("save confirmed charge: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ChargeAlreadyConfirmed, nil
	}
	return ports.ChargeSaved, nil
}

// chargeRow 是 customer_charge 一行的原始列值，交给重建走形成门。
type chargeRow struct {
	id                 domain.CustomerChargeID
	feeItem            string
	evaluation         string
	originalCurrency   string
	originalMinor      int64
	settlementCurrency string
	settlementMinor    int64
	conversion         *string
	stageName          string
	confirmation       *string
	formedAt           time.Time
	confirmedAt        *time.Time
}

func rebuildCustomerCharge(row chargeRow) (domain.CustomerCharge, error) {
	feeRef, err := domain.NewFeeItemReference(row.feeItem)
	if err != nil {
		return domain.CustomerCharge{}, err
	}
	evalRef, err := domain.NewSellEvaluationReference(row.evaluation)
	if err != nil {
		return domain.CustomerCharge{}, err
	}
	originalCurrency, err := domain.NewCurrencyCode(row.originalCurrency)
	if err != nil {
		return domain.CustomerCharge{}, err
	}
	settlementCurrency, err := domain.NewCurrencyCode(row.settlementCurrency)
	if err != nil {
		return domain.CustomerCharge{}, err
	}
	var conversion domain.ConversionStepReference
	if row.conversion != nil {
		conversion, err = domain.NewConversionStepReference(*row.conversion)
		if err != nil {
			return domain.CustomerCharge{}, err
		}
	}
	stage, err := chargeStageFrom(row.stageName)
	if err != nil {
		return domain.CustomerCharge{}, err
	}
	formStage := stage
	if stage == domain.ChargeConfirmed {
		formStage = domain.ChargeEstimated
	}
	charge, err := domain.FormCustomerCharge(domain.CustomerChargeSpec{
		ID:                 row.id,
		FeeItem:            feeRef,
		Evaluation:         evalRef,
		OriginalCurrency:   originalCurrency,
		OriginalMinor:      row.originalMinor,
		SettlementCurrency: settlementCurrency,
		SettlementMinor:    row.settlementMinor,
		Conversion:         conversion,
		Stage:              formStage,
		FormedAt:           row.formedAt,
	})
	if err != nil {
		return domain.CustomerCharge{}, err
	}
	if stage != domain.ChargeConfirmed {
		return charge, nil
	}
	if row.confirmation == nil || row.confirmedAt == nil {
		return domain.CustomerCharge{}, fmt.Errorf("confirmed charge missing confirmation columns")
	}
	basis, err := domain.NewConfirmationBasisReference(*row.confirmation)
	if err != nil {
		return domain.CustomerCharge{}, err
	}
	return charge.Confirm(basis, *row.confirmedAt)
}

func chargeStageFrom(raw string) (domain.ChargeStage, error) {
	switch raw {
	case domain.ChargeEstimated.String():
		return domain.ChargeEstimated, nil
	case domain.ChargeProvisional.String():
		return domain.ChargeProvisional, nil
	case domain.ChargeConfirmed.String():
		return domain.ChargeConfirmed, nil
	default:
		return 0, fmt.Errorf("unknown charge stage %q", raw)
	}
}
