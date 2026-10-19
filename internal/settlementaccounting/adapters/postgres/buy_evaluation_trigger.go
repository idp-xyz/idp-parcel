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

// BuyEvaluationTriggers 是发生项原因选用哪个触发时点的登记册。空册答 found=false。
type BuyEvaluationTriggers struct {
	db *bentopg.DB
}

func NewBuyEvaluationTriggers(db *bentopg.DB) (*BuyEvaluationTriggers, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &BuyEvaluationTriggers{db: db}, nil
}

var (
	_ ports.BuyEvaluationTriggerRegister = (*BuyEvaluationTriggers)(nil)
	_ ports.BuyEvaluationTriggerView     = (*BuyEvaluationTriggers)(nil)
)

func (repository *BuyEvaluationTriggers) SaveBuyEvaluationTrigger(
	ctx context.Context,
	tenant domain.TenantID,
	registration domain.BuyEvaluationTriggerRegistration,
	at time.Time,
) (ports.CatalogueRegistrationEffect, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return 0, fmt.Errorf("save buy evaluation trigger: %w", err)
	}
	if _, err := executor.Exec(ctx, "SAVEPOINT buy_evaluation_trigger"); err != nil {
		return 0, fmt.Errorf("save buy evaluation trigger: %w", err)
	}
	_, err = executor.Exec(ctx, `
		INSERT INTO settlement_accounting.buy_evaluation_trigger (
			tenant_id, occurrence_reason, moment, fee_item_ref, registered_at
		) VALUES ($1, $2, $3, $4, $5)`,
		tenant.String(),
		registration.Reason().String(),
		registration.Moment().String(),
		feeItemColumn(registration),
		at.UTC(),
	)
	if err == nil {
		return ports.CatalogueRegistered, nil
	}
	if !uniqueViolation(err) {
		return 0, fmt.Errorf("save buy evaluation trigger: %w", err)
	}
	if _, err := executor.Exec(ctx, "ROLLBACK TO SAVEPOINT buy_evaluation_trigger"); err != nil {
		return 0, fmt.Errorf("save buy evaluation trigger: %w", err)
	}
	loaded, found, err := scanBuyEvaluationTrigger(executor.QueryRow(ctx, `
		SELECT occurrence_reason, moment, fee_item_ref
		  FROM settlement_accounting.buy_evaluation_trigger
		 WHERE tenant_id = $1 AND occurrence_reason = $2`,
		tenant.String(), registration.Reason().String()))
	if err != nil {
		return 0, fmt.Errorf("save buy evaluation trigger: %w", err)
	}
	// 本册不设修订：先登了没有费用项目的行、再来补登一格，也是异内容，答冲突而不是悄悄补上（ADR-0208 越权风险点 4）。
	if !found || !loaded.SameRegistration(registration) {
		return ports.CatalogueConflict, nil
	}
	return ports.CatalogueReplay, nil
}

// feeItemColumn 把没登的费用项目落成 NULL：空串会被列约束拒，而 NULL 才说得出「这一格没登」。
func feeItemColumn(registration domain.BuyEvaluationTriggerRegistration) *string {
	feeItem, registered := registration.FeeItem()
	if !registered {
		return nil
	}
	value := feeItem.String()
	return &value
}

func (repository *BuyEvaluationTriggers) LoadBuyEvaluationTrigger(
	ctx context.Context,
	tenant domain.TenantID,
	reason domain.OccurrenceReasonReference,
) (domain.BuyEvaluationTriggerRegistration, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.BuyEvaluationTriggerRegistration{}, false, fmt.Errorf("load buy evaluation trigger: %w", err)
	}
	registration, found, err := scanBuyEvaluationTrigger(querier.QueryRow(ctx, `
		SELECT occurrence_reason, moment, fee_item_ref
		  FROM settlement_accounting.buy_evaluation_trigger
		 WHERE tenant_id = $1 AND occurrence_reason = $2`,
		tenant.String(), reason.String()))
	if err != nil {
		return domain.BuyEvaluationTriggerRegistration{}, false, fmt.Errorf("load buy evaluation trigger: %w", err)
	}
	return registration, found, nil
}

// scanBuyEvaluationTrigger 把一行重建成登记：读回过同一个构造门，库里一格坏值报错，不折成「没有这一行」。
func scanBuyEvaluationTrigger(row pgx.Row) (domain.BuyEvaluationTriggerRegistration, bool, error) {
	var reasonText, momentName string
	var feeItemText *string
	err := row.Scan(&reasonText, &momentName, &feeItemText)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.BuyEvaluationTriggerRegistration{}, false, nil
	}
	if err != nil {
		return domain.BuyEvaluationTriggerRegistration{}, false, err
	}
	reason, err := domain.NewOccurrenceReasonReference(reasonText)
	if err != nil {
		return domain.BuyEvaluationTriggerRegistration{}, false, err
	}
	moment, err := domain.BuyEvaluationTriggerMomentFromName(momentName)
	if err != nil {
		return domain.BuyEvaluationTriggerRegistration{}, false, err
	}
	registration, err := domain.NewBuyEvaluationTriggerRegistration(reason, moment)
	if err != nil {
		return domain.BuyEvaluationTriggerRegistration{}, false, err
	}
	if feeItemText != nil {
		feeItem, err := domain.NewFeeItemReference(*feeItemText)
		if err != nil {
			return domain.BuyEvaluationTriggerRegistration{}, false, err
		}
		if registration, err = registration.WithFeeItem(feeItem); err != nil {
			return domain.BuyEvaluationTriggerRegistration{}, false, err
		}
	}
	return registration, true, nil
}
