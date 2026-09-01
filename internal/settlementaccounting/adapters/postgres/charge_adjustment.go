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

// ChargeAdjustments 实现 ports.ChargeAdjustmentStore。册只追加：写口没有 UPDATE 分支，
// 同标识重放答`已登记`（ADR-0031），原调整不被改写。
type ChargeAdjustments struct {
	db *bentopg.DB
}

func NewChargeAdjustments(db *bentopg.DB) (*ChargeAdjustments, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &ChargeAdjustments{db: db}, nil
}

var _ ports.ChargeAdjustmentStore = (*ChargeAdjustments)(nil)

const chargeAdjustmentColumns = `charge_id, kind, direction, evaluation_ref, authorization_ref,
	        original_currency, original_minor, settlement_currency, settlement_minor,
	        conversion_ref, formed_at, recorded_at`

func (repository *ChargeAdjustments) FindByKey(
	ctx context.Context,
	key ports.ChargeAdjustmentKey,
) (ports.ChargeAdjustmentRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ChargeAdjustmentRecord{}, false, fmt.Errorf("find charge adjustment: %w", err)
	}

	var row adjustmentRow
	err = querier.QueryRow(ctx,
		`SELECT `+chargeAdjustmentColumns+`
		   FROM settlement_accounting.charge_adjustment
		  WHERE tenant_id = $1
		    AND adjustment_id = $2`,
		key.TenantID.String(),
		key.Adjustment.String(),
	).Scan(row.scanTargets()...)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.ChargeAdjustmentRecord{}, false, nil
	}
	if err != nil {
		return ports.ChargeAdjustmentRecord{}, false, fmt.Errorf("find charge adjustment: %w", err)
	}

	adjustment, err := rebuildChargeAdjustment(key.Adjustment, row)
	if err != nil {
		return ports.ChargeAdjustmentRecord{}, false, fmt.Errorf("find charge adjustment: %w", err)
	}
	return ports.ChargeAdjustmentRecord{
		Key:        key,
		Adjustment: adjustment,
		RecordedAt: row.recordedAt.UTC(),
	}, true, nil
}

// ListByCharge 交回一笔费用上的调整，按形成时点升序。空集是诚实答案：一笔费用没有被
// 调整过，与费用不存在不是同一回事，后者由费用库回答。
func (repository *ChargeAdjustments) ListByCharge(
	ctx context.Context,
	tenant domain.TenantID,
	charge domain.CustomerChargeID,
) ([]ports.ChargeAdjustmentRecord, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list charge adjustments: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT adjustment_id, `+chargeAdjustmentColumns+`
		   FROM settlement_accounting.charge_adjustment
		  WHERE tenant_id = $1
		    AND charge_id = $2
		  ORDER BY formed_at, adjustment_id`,
		tenant.String(),
		charge.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("list charge adjustments: %w", err)
	}
	defer rows.Close()

	records := []ports.ChargeAdjustmentRecord{}
	for rows.Next() {
		var id string
		var row adjustmentRow
		if err := rows.Scan(append([]any{&id}, row.scanTargets()...)...); err != nil {
			return nil, fmt.Errorf("list charge adjustments: %w", err)
		}
		adjustmentID, err := domain.NewChargeAdjustmentID(id)
		if err != nil {
			return nil, fmt.Errorf("list charge adjustments: %w", err)
		}
		adjustment, err := rebuildChargeAdjustment(adjustmentID, row)
		if err != nil {
			return nil, fmt.Errorf("list charge adjustments: %w", err)
		}
		records = append(records, ports.ChargeAdjustmentRecord{
			Key:        ports.ChargeAdjustmentKey{TenantID: tenant, Adjustment: adjustmentID},
			Adjustment: adjustment,
			RecordedAt: row.recordedAt.UTC(),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list charge adjustments: %w", err)
	}
	return records, nil
}

// Save 写下一笔调整。同标识已有记录时答`已登记`，不覆盖先到者——调整只追加。
func (repository *ChargeAdjustments) Save(
	ctx context.Context,
	record ports.ChargeAdjustmentRecord,
) (ports.ChargeAdjustmentSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ChargeAdjustmentSaveOutcomeInvalid, fmt.Errorf("save charge adjustment: %w", err)
	}

	adjustment := record.Adjustment
	evaluation, hasEvaluation := adjustment.Evaluation()
	authorization, hasAuthorization := adjustment.Authorization()
	conversion, hasConversion := adjustment.Conversion()
	originalCurrency, originalMinor := adjustment.OriginalAmount()
	settlementCurrency, settlementMinor := adjustment.SettlementAmount()

	tag, err := executor.Exec(ctx,
		`INSERT INTO settlement_accounting.charge_adjustment
			(tenant_id, adjustment_id, charge_id, kind, direction,
			 evaluation_ref, authorization_ref, original_currency, original_minor,
			 settlement_currency, settlement_minor, conversion_ref, formed_at, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Adjustment.String(),
		adjustment.Charge().String(),
		adjustment.Kind().String(),
		adjustment.Direction().String(),
		optionalRef(evaluation, hasEvaluation),
		optionalRef(authorization, hasAuthorization),
		originalCurrency.String(),
		originalMinor,
		settlementCurrency.String(),
		settlementMinor,
		optionalRef(conversion, hasConversion),
		adjustment.FormedAt().UTC(),
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.ChargeAdjustmentSaveOutcomeInvalid, fmt.Errorf("save charge adjustment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ChargeAdjustmentAlreadyRecorded, nil
	}
	return ports.ChargeAdjustmentSaved, nil
}

// adjustmentRow 是 charge_adjustment 一行的原始列值，交给重建走形成门。
type adjustmentRow struct {
	charge             string
	kindName           string
	directionName      string
	evaluation         *string
	authorization      *string
	originalCurrency   string
	originalMinor      int64
	settlementCurrency string
	settlementMinor    int64
	conversion         *string
	formedAt           time.Time
	recordedAt         time.Time
}

func (row *adjustmentRow) scanTargets() []any {
	return []any{
		&row.charge, &row.kindName, &row.directionName, &row.evaluation, &row.authorization,
		&row.originalCurrency, &row.originalMinor, &row.settlementCurrency, &row.settlementMinor,
		&row.conversion, &row.formedAt, &row.recordedAt,
	}
}

// rebuildChargeAdjustment 让行走 FormChargeAdjustment 回来，不旁路形成门：库上的 CHECK
// 与领域的形成门是同一组判据的两份，读回时再过一次，任一份先松掉都会当场露出来。
func rebuildChargeAdjustment(
	id domain.ChargeAdjustmentID,
	row adjustmentRow,
) (domain.ChargeAdjustment, error) {
	charge, err := domain.NewCustomerChargeID(row.charge)
	if err != nil {
		return domain.ChargeAdjustment{}, err
	}
	kind, err := adjustmentKindFrom(row.kindName)
	if err != nil {
		return domain.ChargeAdjustment{}, err
	}
	direction, err := adjustmentDirectionFrom(row.directionName)
	if err != nil {
		return domain.ChargeAdjustment{}, err
	}
	spec := domain.ChargeAdjustmentSpec{
		ID:        id,
		Charge:    charge,
		Kind:      kind,
		Direction: direction,
		FormedAt:  row.formedAt,
	}
	if row.evaluation != nil {
		if spec.Evaluation, err = domain.NewSellEvaluationReference(*row.evaluation); err != nil {
			return domain.ChargeAdjustment{}, err
		}
	}
	if row.authorization != nil {
		if spec.Authorization, err = domain.NewCommercialAuthorizationReference(*row.authorization); err != nil {
			return domain.ChargeAdjustment{}, err
		}
	}
	if row.conversion != nil {
		if spec.Conversion, err = domain.NewConversionStepReference(*row.conversion); err != nil {
			return domain.ChargeAdjustment{}, err
		}
	}
	if spec.OriginalCurrency, err = domain.NewCurrencyCode(row.originalCurrency); err != nil {
		return domain.ChargeAdjustment{}, err
	}
	if spec.SettlementCurrency, err = domain.NewCurrencyCode(row.settlementCurrency); err != nil {
		return domain.ChargeAdjustment{}, err
	}
	spec.OriginalMinor = row.originalMinor
	spec.SettlementMinor = row.settlementMinor
	return domain.FormChargeAdjustment(spec)
}

func adjustmentKindFrom(raw string) (domain.AdjustmentKind, error) {
	switch raw {
	case domain.PricingCorrection.String():
		return domain.PricingCorrection, nil
	case domain.CommercialConcession.String():
		return domain.CommercialConcession, nil
	default:
		return 0, fmt.Errorf("unknown charge adjustment kind %q", raw)
	}
}
