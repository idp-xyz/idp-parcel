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

// SellEvaluationTriggers 是 SELL 评价触发时点的登记册。空册答 found=false。与 BUY 触发册不是同一张表。
type SellEvaluationTriggers struct {
	db *bentopg.DB
}

func NewSellEvaluationTriggers(db *bentopg.DB) (*SellEvaluationTriggers, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &SellEvaluationTriggers{db: db}, nil
}

func (repository *SellEvaluationTriggers) SaveSellEvaluationTrigger(
	ctx context.Context,
	tenant domain.TenantID,
	registration domain.SellEvaluationTriggerRegistration,
	at time.Time,
) (ports.CatalogueRegistrationEffect, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return 0, fmt.Errorf("save sell evaluation trigger: %w", err)
	}
	if _, err := executor.Exec(ctx, "SAVEPOINT sell_evaluation_trigger"); err != nil {
		return 0, fmt.Errorf("save sell evaluation trigger: %w", err)
	}
	_, err = executor.Exec(ctx, `
		INSERT INTO settlement_accounting.sell_evaluation_trigger (
			tenant_id, occurrence_reason, moment, registered_at
		) VALUES ($1, $2, $3, $4)`,
		tenant.String(), registration.Reason().String(), registration.Moment().String(), at.UTC())
	if err == nil {
		return ports.CatalogueRegistered, nil
	}
	if !uniqueViolation(err) {
		return 0, fmt.Errorf("save sell evaluation trigger: %w", err)
	}
	if _, err := executor.Exec(ctx, "ROLLBACK TO SAVEPOINT sell_evaluation_trigger"); err != nil {
		return 0, fmt.Errorf("save sell evaluation trigger: %w", err)
	}
	loaded, found, err := scanSellEvaluationTrigger(executor.QueryRow(ctx, `
		SELECT moment
		  FROM settlement_accounting.sell_evaluation_trigger
		 WHERE tenant_id = $1 AND occurrence_reason = $2`,
		tenant.String(), registration.Reason().String()))
	if err != nil {
		return 0, fmt.Errorf("save sell evaluation trigger: %w", err)
	}
	if !found || loaded != registration.Moment() {
		return ports.CatalogueConflict, nil
	}
	return ports.CatalogueReplay, nil
}

func (repository *SellEvaluationTriggers) LoadSellEvaluationTrigger(
	ctx context.Context,
	tenant domain.TenantID,
	reason domain.OccurrenceReasonReference,
) (domain.SellEvaluationTriggerMoment, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.SellEvaluationTriggerMomentInvalid, false, fmt.Errorf("load sell evaluation trigger: %w", err)
	}
	moment, found, err := scanSellEvaluationTrigger(querier.QueryRow(ctx, `
		SELECT moment
		  FROM settlement_accounting.sell_evaluation_trigger
		 WHERE tenant_id = $1 AND occurrence_reason = $2`,
		tenant.String(), reason.String()))
	if err != nil {
		return domain.SellEvaluationTriggerMomentInvalid, false, fmt.Errorf("load sell evaluation trigger: %w", err)
	}
	return moment, found, nil
}

func scanSellEvaluationTrigger(row pgx.Row) (domain.SellEvaluationTriggerMoment, bool, error) {
	var name string
	err := row.Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.SellEvaluationTriggerMomentInvalid, false, nil
	}
	if err != nil {
		return domain.SellEvaluationTriggerMomentInvalid, false, err
	}
	moment, err := domain.SellEvaluationTriggerMomentFromName(name)
	if err != nil {
		return domain.SellEvaluationTriggerMomentInvalid, false, err
	}
	return moment, true, nil
}
