package application

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// WarehousePlanOutcome 是登记或评价一张仓储计费方案的结果。
type WarehousePlanOutcome string

const (
	WarehousePlanRecorded      WarehousePlanOutcome = "RECORDED"
	WarehousePlanExisting      WarehousePlanOutcome = "ALREADY_REGISTERED"
	WarehousePlanConflict      WarehousePlanOutcome = "CONTENT_CONFLICT"
	WarehousePlanNotEnabled    WarehousePlanOutcome = "NEWER_CANONICALIZATION_NOT_ENABLED"
	WarehousePlanNotAccepted   WarehousePlanOutcome = "NOT_ACCEPTED"
	WarehousePlanNotConfigured WarehousePlanOutcome = "NOT_CONFIGURED"
	WarehousePlanPriced        WarehousePlanOutcome = "PRICED"
	WarehousePlanPending       WarehousePlanOutcome = "PENDING"
	WarehousePlanZero          WarehousePlanOutcome = "ZERO"
)

// WarehousePlanStore 记下仓储计费方案与它的评价。写入在没有环境事务时拒绝。
type WarehousePlanStore interface {
	LoadPlan(ctx context.Context, tenant, id, version string) (domain.WarehousePlan, bool, error)
	SavePlan(ctx context.Context, tenant string, plan domain.WarehousePlan) error
	SelectPlan(ctx context.Context, tenant, scope string, at time.Time) (domain.WarehousePlan, bool, error)
	SaveEvaluation(ctx context.Context, tenant, id, occurrence string, version int, planID string, evaluation domain.WarehouseEvaluation) error
}

// WarehousePlanHandler 登记仓储计费方案并按它评价合成发生项。较新规范化版本没开开关时不入册。
type WarehousePlanHandler struct {
	plans      WarehousePlanStore
	enabled    bool
	identities interface {
		Next() (string, error)
	}
}

func NewWarehousePlanHandler(plans WarehousePlanStore, enabled bool, identities interface{ Next() (string, error) }) *WarehousePlanHandler {
	return &WarehousePlanHandler{plans: plans, enabled: enabled, identities: identities}
}

func (handler *WarehousePlanHandler) Register(ctx context.Context, tenant string, plan domain.WarehousePlan) (WarehousePlanOutcome, error) {
	built, issue := domain.NewWarehousePlan(plan)
	if issue != "" {
		return WarehousePlanNotAccepted, nil
	}
	if domain.WarehousePlanCanonicalization != domain.CurrentCanonicalizationVersion() && !handler.enabled {
		return WarehousePlanNotEnabled, nil
	}
	existing, found, err := handler.plans.LoadPlan(ctx, tenant, built.ID, built.Version)
	if err != nil {
		return "", fmt.Errorf("register warehouse plan: %w", err)
	}
	if found {
		if existing.Digest() == built.Digest() {
			return WarehousePlanExisting, nil
		}
		return WarehousePlanConflict, nil
	}
	if err := handler.plans.SavePlan(ctx, tenant, built); err != nil {
		return "", fmt.Errorf("register warehouse plan: %w", err)
	}
	return WarehousePlanRecorded, nil
}

// Price 按业务时间选这一范围在用的仓储计费方案。没有卡答未配置，不拿别的范围顶。
func (handler *WarehousePlanHandler) Price(ctx context.Context, tenant, scope string, at time.Time, occurrence domain.WarehouseOccurrenceSnapshot) (WarehousePlanOutcome, domain.WarehouseEvaluation, string, error) {
	plan, found, err := handler.plans.SelectPlan(ctx, tenant, scope, at)
	if err != nil {
		return "", domain.WarehouseEvaluation{}, "", fmt.Errorf("price warehouse occurrence: %w", err)
	}
	if !found {
		return WarehousePlanNotConfigured, domain.WarehouseEvaluation{}, "", nil
	}
	evaluation := domain.EvaluateWarehousePlan(plan, occurrence)
	switch evaluation.Status {
	case "PENDING":
		return WarehousePlanPending, evaluation, "", nil
	case "CONFLICT":
		return WarehousePlanConflict, evaluation, "", nil
	}
	id, err := handler.identities.Next()
	if err != nil {
		return "", domain.WarehouseEvaluation{}, "", fmt.Errorf("price warehouse occurrence: %w", err)
	}
	if err := handler.plans.SaveEvaluation(ctx, tenant, id, occurrence.ID, occurrence.Version, plan.ID, evaluation); err != nil {
		return "", domain.WarehouseEvaluation{}, "", fmt.Errorf("price warehouse occurrence: %w", err)
	}
	if evaluation.Status == "ZERO" {
		return WarehousePlanZero, evaluation, id, nil
	}
	return WarehousePlanPriced, evaluation, id, nil
}
