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
	var confirmedAt, sourceOccurredAt, attributionDate *time.Time
	var facts confirmationFactColumns
	err = querier.QueryRow(ctx,
		`SELECT fee_item, evaluation_ref, original_currency, original_minor,
		        settlement_currency, settlement_minor, conversion_ref, stage,
		        confirmation_basis, formed_at, confirmed_at, recorded_at,
		        responsible_entity, counterparty_ref, charge_direction,
		        settlement_account_id, contract_basis, primary_charging_scope,
		        source_fact_ref, source_occurred_at, attribution_date
		   FROM settlement_accounting.customer_charge
		  WHERE tenant_id = $1
		    AND charge_id = $2`,
		tenant.String(),
		id.String(),
	).Scan(&feeItem, &evaluation, &originalCurrency, &originalMinor,
		&settlementCurrency, &settlementMinor, &conversion, &stageName,
		&confirmation, &formedAt, &confirmedAt, &recordedAt,
		&facts.responsibleEntity, &facts.counterparty, &facts.direction,
		&facts.settlementAccount, &facts.contractBasis, &facts.primaryChargingScope,
		&facts.sourceFact, &sourceOccurredAt, &attributionDate)
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
		facts:              facts,
		formedAt:           formedAt,
		confirmedAt:        confirmedAt,
		sourceOccurredAt:   sourceOccurredAt,
		attributionDate:    attributionDate,
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
	facts, ok := charge.ConfirmedFacts()
	if !ok {
		return ports.ChargeSaveOutcomeInvalid, fmt.Errorf("save confirmed charge: confirmation facts missing")
	}
	originalCurrency, originalMinor := charge.OriginalAmount()
	settlementCurrency, settlementMinor := charge.SettlementAmount()
	var conversion *string
	if reference, present := charge.Conversion(); present {
		value := reference.String()
		conversion = &value
	}
	sourceOccurredAt := sourceOccurredColumn(charge)
	attributionDate := attributionDateColumn(charge)

	// 七项随确认同笔落库、冲突分支同笔改写：分两笔写会让库上出现一个七项为空的已确认
	// 行，而 customer_charge_confirmation_facts_coupled 正是要挡住它。
	tag, err := executor.Exec(ctx,
		`INSERT INTO settlement_accounting.customer_charge
			(tenant_id, charge_id, fee_item, evaluation_ref, original_currency,
			 original_minor, settlement_currency, settlement_minor, conversion_ref,
			 stage, confirmation_basis, formed_at, confirmed_at, recorded_at,
			 responsible_entity, counterparty_ref, charge_direction,
			 settlement_account_id, contract_basis, primary_charging_scope,
			 source_fact_ref, source_occurred_at, attribution_date)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'CONFIRMED', $10, $11, $12, $12,
		         $13, $14, $15, $16, $17, $18, $19, $20, $21::date)
		 ON CONFLICT (tenant_id, charge_id) DO UPDATE
		    SET stage = 'CONFIRMED',
		        attribution_date = EXCLUDED.attribution_date,
		        confirmation_basis = EXCLUDED.confirmation_basis,
		        confirmed_at = EXCLUDED.confirmed_at,
		        recorded_at = EXCLUDED.recorded_at,
		        responsible_entity = EXCLUDED.responsible_entity,
		        counterparty_ref = EXCLUDED.counterparty_ref,
		        charge_direction = EXCLUDED.charge_direction,
		        settlement_account_id = EXCLUDED.settlement_account_id,
		        contract_basis = EXCLUDED.contract_basis,
		        primary_charging_scope = EXCLUDED.primary_charging_scope,
		        source_fact_ref = EXCLUDED.source_fact_ref
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
		facts.ResponsibleEntity.String(),
		facts.Counterparty.String(),
		facts.Direction.String(),
		facts.SettlementAccount.String(),
		facts.ContractBasis.String(),
		facts.PrimaryChargingScope.String(),
		facts.SourceFact.String(),
		sourceOccurredAt,
		attributionDate,
	)
	if err != nil {
		return ports.ChargeSaveOutcomeInvalid, fmt.Errorf("save confirmed charge: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ChargeAlreadyConfirmed, nil
	}
	return ports.ChargeSaved, nil
}

// SaveEstimated 写下尚未确认的客户费用。七项确认事实留空。已有同一标识则不覆盖。
func (repository *CustomerCharges) SaveEstimated(
	ctx context.Context,
	tenant domain.TenantID,
	charge domain.CustomerCharge,
) (ports.ChargeSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ChargeSaveOutcomeInvalid, fmt.Errorf("save estimated charge: %w", err)
	}
	if charge.Stage() != domain.ChargeEstimated && charge.Stage() != domain.ChargeProvisional {
		return ports.ChargeSaveOutcomeInvalid, fmt.Errorf("save estimated charge: charge is already confirmed")
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
			 stage, formed_at, recorded_at, source_occurred_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $11, $12)
		 ON CONFLICT DO NOTHING`,
		tenant.String(),
		charge.ID().String(),
		charge.FeeItem().String(),
		charge.Evaluation().String(),
		originalCurrency.String(),
		originalMinor,
		settlementCurrency.String(),
		settlementMinor,
		conversion,
		charge.Stage().String(),
		charge.FormedAt().UTC(),
		sourceOccurredColumn(charge),
	)
	if err != nil {
		return ports.ChargeSaveOutcomeInvalid, fmt.Errorf("save estimated charge: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ChargeAlreadyConfirmed, nil
	}
	return ports.ChargeSaved, nil
}

// RecordAttributionDate 把截单时当场判出的归属日落到已确认费用上。WHERE 带 attribution_date IS NULL：已有归属日的
// 行不改，读回册上那一天交回——归属日一经形成不变（ADR-0269 决定三）。行不在、或不是已确认，报错：截单只给
// 已确认费用判归属日，走到这里是编排与册说法矛盾。
func (repository *CustomerCharges) RecordAttributionDate(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.CustomerChargeID,
	date domain.AttributionDate,
) (domain.AttributionDate, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return domain.AttributionDate{}, fmt.Errorf("record attribution date: %w", err)
	}
	if !date.Valid() {
		return domain.AttributionDate{}, fmt.Errorf("record attribution date: invalid date")
	}
	var stored *time.Time
	var stageName string
	err = executor.QueryRow(ctx,
		`WITH formed AS (
		     UPDATE settlement_accounting.customer_charge
		        SET attribution_date = $3::date
		      WHERE tenant_id = $1
		        AND charge_id = $2
		        AND stage = 'CONFIRMED'
		        AND attribution_date IS NULL
		  RETURNING attribution_date, stage)
		 SELECT attribution_date, stage FROM formed
		 UNION ALL
		 SELECT attribution_date, stage
		   FROM settlement_accounting.customer_charge
		  WHERE tenant_id = $1
		    AND charge_id = $2
		    AND NOT EXISTS (SELECT 1 FROM formed)`,
		tenant.String(), id.String(), date.String(),
	).Scan(&stored, &stageName)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.AttributionDate{}, fmt.Errorf("record attribution date: charge %s is not in the register", id)
	}
	if err != nil {
		return domain.AttributionDate{}, fmt.Errorf("record attribution date: %w", err)
	}
	if stageName != domain.ChargeConfirmed.String() || stored == nil {
		return domain.AttributionDate{}, fmt.Errorf("record attribution date: charge %s is not confirmed", id)
	}
	return attributionDateFrom(*stored), nil
}

// sourceOccurredColumn 把没带上的来源发生时点落成 NULL：零值时刻会被读回成一个真实的公元元年时点。
func sourceOccurredColumn(charge domain.CustomerCharge) *time.Time {
	at, present := charge.SourceOccurredAt()
	if !present {
		return nil
	}
	utc := at.UTC()
	return &utc
}

func attributionDateColumn(charge domain.CustomerCharge) *string {
	date, attributed := charge.AttributionDate()
	if !attributed {
		return nil
	}
	value := date.String()
	return &value
}

// attributionDateFrom 取 date 列的公历日。pgx 把 date 读成 UTC 零点，取它的年月日不涉任何业务时区。
func attributionDateFrom(at time.Time) domain.AttributionDate {
	year, month, day := at.UTC().Date()
	return domain.AttributionDate{Year: year, Month: month, Day: day}
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
	facts              confirmationFactColumns
	formedAt           time.Time
	confirmedAt        *time.Time
	sourceOccurredAt   *time.Time
	attributionDate    *time.Time
}

// confirmationFactColumns 是确认时固定的七项在行上的原始列值。全部可空：非确认行按
// customer_charge_confirmation_facts_coupled 要求七项全缺。
type confirmationFactColumns struct {
	responsibleEntity    *string
	counterparty         *string
	direction            *string
	settlementAccount    *string
	contractBasis        *string
	primaryChargingScope *string
	sourceFact           *string
}

// rebuildConfirmedFacts 把七列走各自的构造门装回领域类型。任一列缺席即报错而不是留空
// 位：留空位造出的是一份领域拒收的事实，错会在 Confirm 那里以「提交矛盾」的面目出现，
// 而真正的病因是行本身不全。
func rebuildConfirmedFacts(columns confirmationFactColumns) (domain.ConfirmedChargeFacts, error) {
	var facts domain.ConfirmedChargeFacts
	for name, column := range map[string]*string{
		"responsible_entity":     columns.responsibleEntity,
		"counterparty_ref":       columns.counterparty,
		"charge_direction":       columns.direction,
		"settlement_account_id":  columns.settlementAccount,
		"contract_basis":         columns.contractBasis,
		"primary_charging_scope": columns.primaryChargingScope,
		"source_fact_ref":        columns.sourceFact,
	} {
		if column == nil {
			return facts, fmt.Errorf("confirmed charge missing %s", name)
		}
	}
	var err error
	if facts.ResponsibleEntity, err = domain.NewLegalEntityReference(*columns.responsibleEntity); err != nil {
		return facts, err
	}
	if facts.Counterparty, err = domain.NewSettlementCounterpartyReference(*columns.counterparty); err != nil {
		return facts, err
	}
	if facts.Direction, err = chargeDirectionFrom(*columns.direction); err != nil {
		return facts, err
	}
	if facts.SettlementAccount, err = domain.NewSettlementAccountID(*columns.settlementAccount); err != nil {
		return facts, err
	}
	if facts.ContractBasis, err = domain.NewContractBasisReference(*columns.contractBasis); err != nil {
		return facts, err
	}
	if facts.PrimaryChargingScope, err = domain.NewChargingScopeReference(*columns.primaryChargingScope); err != nil {
		return facts, err
	}
	if facts.SourceFact, err = domain.NewSourceFactReference(*columns.sourceFact); err != nil {
		return facts, err
	}
	return facts, nil
}

func chargeDirectionFrom(raw string) (domain.ChargeDirection, error) {
	switch raw {
	case domain.ChargeReceivable.String():
		return domain.ChargeReceivable, nil
	case domain.ChargePayable.String():
		return domain.ChargePayable, nil
	default:
		return 0, fmt.Errorf("unknown charge direction %q", raw)
	}
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
	var sourceOccurredAt time.Time
	if row.sourceOccurredAt != nil {
		sourceOccurredAt = *row.sourceOccurredAt
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
		SourceOccurredAt:   sourceOccurredAt,
	})
	if err != nil {
		return domain.CustomerCharge{}, err
	}
	if stage != domain.ChargeConfirmed {
		if row.attributionDate != nil {
			return domain.CustomerCharge{}, fmt.Errorf("unconfirmed charge carries an attribution date")
		}
		return charge, nil
	}
	if row.confirmation == nil || row.confirmedAt == nil {
		return domain.CustomerCharge{}, fmt.Errorf("confirmed charge missing confirmation columns")
	}
	basis, err := domain.NewConfirmationBasisReference(*row.confirmation)
	if err != nil {
		return domain.CustomerCharge{}, err
	}
	facts, err := rebuildConfirmedFacts(row.facts)
	if err != nil {
		return domain.CustomerCharge{}, err
	}
	confirmed, err := charge.Confirm(facts, basis, *row.confirmedAt)
	if err != nil || row.attributionDate == nil {
		return confirmed, err
	}
	return confirmed.WithAttributionDate(attributionDateFrom(*row.attributionDate))
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
