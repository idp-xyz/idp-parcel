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
			tenant_id, occurrence_reason, moment, registered_at
		) VALUES ($1, $2, $3, $4)`,
		tenant.String(),
		registration.Reason().String(),
		registration.Moment().String(),
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
		SELECT moment
		  FROM settlement_accounting.buy_evaluation_trigger
		 WHERE tenant_id = $1 AND occurrence_reason = $2`,
		tenant.String(), registration.Reason().String()))
	if err != nil {
		return 0, fmt.Errorf("save buy evaluation trigger: %w", err)
	}
	if !found || loaded != registration.Moment() {
		return ports.CatalogueConflict, nil
	}
	return ports.CatalogueReplay, nil
}

func (repository *BuyEvaluationTriggers) LoadBuyEvaluationTrigger(
	ctx context.Context,
	tenant domain.TenantID,
	reason domain.OccurrenceReasonReference,
) (domain.BuyEvaluationTriggerMoment, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.BuyEvaluationTriggerMomentInvalid, false, fmt.Errorf("load buy evaluation trigger: %w", err)
	}
	moment, found, err := scanBuyEvaluationTrigger(querier.QueryRow(ctx, `
		SELECT moment
		  FROM settlement_accounting.buy_evaluation_trigger
		 WHERE tenant_id = $1 AND occurrence_reason = $2`,
		tenant.String(), reason.String()))
	if err != nil {
		return domain.BuyEvaluationTriggerMomentInvalid, false, fmt.Errorf("load buy evaluation trigger: %w", err)
	}
	return moment, found, nil
}

func scanBuyEvaluationTrigger(row pgx.Row) (domain.BuyEvaluationTriggerMoment, bool, error) {
	var name string
	err := row.Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.BuyEvaluationTriggerMomentInvalid, false, nil
	}
	if err != nil {
		return domain.BuyEvaluationTriggerMomentInvalid, false, err
	}
	moment, err := domain.BuyEvaluationTriggerMomentFromName(name)
	if err != nil {
		return domain.BuyEvaluationTriggerMomentInvalid, false, err
	}
	return moment, true, nil
}
