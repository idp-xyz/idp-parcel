package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// WarehousePlans 记下仓储计费方案与评价。方案文档不含金额以外的租户默认。
type WarehousePlans struct {
	db *bentopg.DB
}

func NewWarehousePlans(db *bentopg.DB) (*WarehousePlans, error) {
	if db == nil {
		return nil, errors.New("parcel pricing postgres: warehouse plans need a db")
	}
	return &WarehousePlans{db: db}, nil
}

var _ application.WarehousePlanStore = (*WarehousePlans)(nil)

func (store *WarehousePlans) LoadPlan(ctx context.Context, tenant, id, version string) (domain.WarehousePlan, bool, error) {
	return store.scanPlan(ctx, `
		SELECT document FROM parcel_pricing.warehouse_plan
		 WHERE tenant_id = $1 AND plan_id = $2 AND version = $3`, tenant, id, version)
}

func (store *WarehousePlans) SelectPlan(ctx context.Context, tenant, scope string, at time.Time) (domain.WarehousePlan, bool, error) {
	return store.scanPlan(ctx, `
		SELECT document FROM parcel_pricing.warehouse_plan
		 WHERE tenant_id = $1 AND scope = $2 AND effective_from <= $3
		 ORDER BY effective_from DESC, version DESC
		 LIMIT 1`, tenant, scope, at)
}

func (store *WarehousePlans) scanPlan(ctx context.Context, query string, args ...any) (domain.WarehousePlan, bool, error) {
	executor, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return domain.WarehousePlan{}, false, fmt.Errorf("load warehouse plan: %w", err)
	}
	var raw []byte
	err = executor.QueryRow(ctx, query, args...).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.WarehousePlan{}, false, nil
	}
	if err != nil {
		return domain.WarehousePlan{}, false, fmt.Errorf("load warehouse plan: %w", err)
	}
	var plan domain.WarehousePlan
	if err := json.Unmarshal(raw, &plan); err != nil {
		return domain.WarehousePlan{}, false, err
	}
	built, issue := domain.NewWarehousePlan(plan)
	if issue != "" {
		return domain.WarehousePlan{}, false, fmt.Errorf("load warehouse plan: %s", issue)
	}
	return built, true, nil
}

func (store *WarehousePlans) SavePlan(ctx context.Context, tenant string, plan domain.WarehousePlan) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save warehouse plan: %w", err)
	}
	raw, err := json.Marshal(plan)
	if err != nil {
		return err
	}
	if _, err := executor.Exec(ctx, `
		INSERT INTO parcel_pricing.warehouse_plan (
		    tenant_id, plan_id, version, scope, currency, effective_from, content_digest, document)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		tenant, plan.ID, plan.Version, plan.Scope, plan.Currency, plan.EffectiveFrom, plan.Digest(), raw); err != nil {
		return fmt.Errorf("save warehouse plan: %w", err)
	}
	return nil
}

func (store *WarehousePlans) SaveEvaluation(ctx context.Context, tenant, id, occurrence string, version int, planID string, evaluation domain.WarehouseEvaluation) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save warehouse evaluation: %w", err)
	}
	raw, err := json.Marshal(evaluation)
	if err != nil {
		return err
	}
	if _, err := executor.Exec(ctx, `
		INSERT INTO parcel_pricing.warehouse_evaluation (
		    tenant_id, evaluation_id, occurrence_id, version, plan_id, status, total_minor, currency, document)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		tenant, id, occurrence, version, planID, evaluation.Status, evaluation.TotalMinor, evaluation.Currency, raw); err != nil {
		return fmt.Errorf("save warehouse evaluation: %w", err)
	}
	return nil
}
