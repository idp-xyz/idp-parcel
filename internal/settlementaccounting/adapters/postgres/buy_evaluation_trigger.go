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

// BuyEvaluationTriggers 是发生项原因选用哪个触发时点的登记册，按（租户、原因）排版次，读口取最新一版。空册答
// found=false。
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

// SaveBuyEvaluationTrigger 登一版。册按（租户、原因）排版次：没有这一原因就落第一版；与某一版同内容是重放；最新一版没登
// 费用项目、这一份登了，追加下一版（domain.BuyEvaluationTriggerRegistration.Revises，ADR-0229 决定七）；其余答冲突，
// 都不改已落的行。两拍并发追加同一版次，后到者撞主键，回滚到保存点后按提交后的册再判一次。
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
	for attempt := 0; attempt < 2; attempt++ {
		effect, retry, err := repository.saveOnce(ctx, executor, tenant, registration, at)
		if err != nil {
			return 0, fmt.Errorf("save buy evaluation trigger: %w", err)
		}
		if !retry {
			return effect, nil
		}
	}
	return 0, fmt.Errorf("save buy evaluation trigger: concurrent revisions of reason %s kept colliding", registration.Reason())
}

func (repository *BuyEvaluationTriggers) saveOnce(
	ctx context.Context,
	executor bentopg.Executor,
	tenant domain.TenantID,
	registration domain.BuyEvaluationTriggerRegistration,
	at time.Time,
) (ports.CatalogueRegistrationEffect, bool, error) {
	rows, err := executor.Query(ctx, `
		SELECT occurrence_reason, moment, fee_item_ref, revision
		  FROM settlement_accounting.buy_evaluation_trigger
		 WHERE tenant_id = $1 AND occurrence_reason = $2
		 ORDER BY revision`,
		tenant.String(), registration.Reason().String())
	if err != nil {
		return 0, false, err
	}
	var (
		latest       domain.BuyEvaluationTriggerRegistration
		latestNumber int
	)
	for rows.Next() {
		stored, revision, err := scanBuyEvaluationTriggerRevision(rows)
		if err != nil {
			rows.Close()
			return 0, false, err
		}
		if stored.SameRegistration(registration) {
			rows.Close()
			return ports.CatalogueReplay, false, nil
		}
		latest, latestNumber = stored, revision
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, false, err
	}
	if latestNumber > 0 && !registration.Revises(latest) {
		return ports.CatalogueConflict, false, nil
	}

	if _, err := executor.Exec(ctx, "SAVEPOINT buy_evaluation_trigger"); err != nil {
		return 0, false, err
	}
	_, err = executor.Exec(ctx, `
		INSERT INTO settlement_accounting.buy_evaluation_trigger (
			tenant_id, occurrence_reason, moment, fee_item_ref, registered_at, revision
		) VALUES ($1, $2, $3, $4, $5, $6)`,
		tenant.String(),
		registration.Reason().String(),
		registration.Moment().String(),
		feeItemColumn(registration),
		at.UTC(),
		latestNumber+1,
	)
	if err == nil {
		return ports.CatalogueRegistered, false, nil
	}
	if !uniqueViolation(err) {
		return 0, false, err
	}
	if _, err := executor.Exec(ctx, "ROLLBACK TO SAVEPOINT buy_evaluation_trigger"); err != nil {
		return 0, false, err
	}
	return 0, true, nil
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

// LoadBuyEvaluationTrigger 交回这一原因最新的那一版。
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
		 WHERE tenant_id = $1 AND occurrence_reason = $2
		 ORDER BY revision DESC LIMIT 1`,
		tenant.String(), reason.String()))
	if err != nil {
		return domain.BuyEvaluationTriggerRegistration{}, false, fmt.Errorf("load buy evaluation trigger: %w", err)
	}
	return registration, found, nil
}

// scanBuyEvaluationTriggerRevision 读一行连同它的版次。
func scanBuyEvaluationTriggerRevision(rows pgx.Rows) (domain.BuyEvaluationTriggerRegistration, int, error) {
	var (
		reasonText, momentName string
		feeItemText            *string
		revision               int
	)
	if err := rows.Scan(&reasonText, &momentName, &feeItemText, &revision); err != nil {
		return domain.BuyEvaluationTriggerRegistration{}, 0, err
	}
	registration, err := rebuildBuyEvaluationTrigger(reasonText, momentName, feeItemText)
	return registration, revision, err
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
	registration, err := rebuildBuyEvaluationTrigger(reasonText, momentName, feeItemText)
	if err != nil {
		return domain.BuyEvaluationTriggerRegistration{}, false, err
	}
	return registration, true, nil
}

func rebuildBuyEvaluationTrigger(
	reasonText, momentName string,
	feeItemText *string,
) (domain.BuyEvaluationTriggerRegistration, error) {
	reason, err := domain.NewOccurrenceReasonReference(reasonText)
	if err != nil {
		return domain.BuyEvaluationTriggerRegistration{}, err
	}
	moment, err := domain.BuyEvaluationTriggerMomentFromName(momentName)
	if err != nil {
		return domain.BuyEvaluationTriggerRegistration{}, err
	}
	registration, err := domain.NewBuyEvaluationTriggerRegistration(reason, moment)
	if err != nil {
		return domain.BuyEvaluationTriggerRegistration{}, err
	}
	if feeItemText != nil {
		feeItem, err := domain.NewFeeItemReference(*feeItemText)
		if err != nil {
			return domain.BuyEvaluationTriggerRegistration{}, err
		}
		if registration, err = registration.WithFeeItem(feeItem); err != nil {
			return domain.BuyEvaluationTriggerRegistration{}, err
		}
	}
	return registration, nil
}
