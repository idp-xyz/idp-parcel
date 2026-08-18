package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 商业价格政策册的持久化面（ADR-0034 / ADR-0057）。装载不在这里而在
// CommercialPublications.LoadForScope——政策随整册一次取回。

type scannedPricePolicy struct {
	direction     *string
	planRef       *string
	planDirection *string
	conversion    *string
	scope         *string
	startsAt      *time.Time
	endsAt        *time.Time
}

func (row scannedPricePolicy) present() bool {
	return row.direction != nil
}

func (row scannedPricePolicy) complete() bool {
	return row.direction != nil && row.planRef != nil && row.planDirection != nil &&
		row.conversion != nil && row.scope != nil && row.startsAt != nil
}

// SavePricePolicy 登记一份价格规则版本的计价正文。撞键不覆盖：同内容是重放，异内容
// （含发布期保全的方案方向与转换）是需要商业责任方修正的冲突。
func (repository *CommercialPublications) SavePricePolicy(
	ctx context.Context,
	policy domain.CommercialPricePolicy,
	planDirection domain.PriceDirection,
	conversion domain.PlanBindingConversion,
) (ports.PricePolicySaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.PricePolicySaveOutcomeInvalid, fmt.Errorf("save price policy: %w", err)
	}

	version := policy.Version()
	reconstructed, err := domain.NewCommercialPricePolicy(
		version,
		policy.Direction(),
		policy.PricingPlan(),
		planDirection,
		conversion,
		policy.Scope(),
		policy.Effective(),
	)
	if err != nil {
		// 政策正文与传入的发布期答复对不上（含 SELL+BUY 却未声明转换）。拦在 INSERT 前，
		// 否则库里会多一行装载时过不了 NewCommercialPricePolicy 的记录。
		return ports.PricePolicySaveOutcomeInvalid, fmt.Errorf("save price policy: %w", err)
	}

	direction := reconstructed.Direction().String()
	planRef := reconstructed.PricingPlan().String()
	planDir := planDirection.String()
	conv := conversion.String()
	scope := reconstructed.Scope().String()

	var endsAt *time.Time
	if end, bounded := policy.Effective().EndsAt(); bounded {
		utc := end.UTC()
		endsAt = &utc
	}
	startsAt := policy.Effective().StartsAt().UTC()

	tag, err := executor.Exec(ctx,
		`INSERT INTO party_commercial.commercial_price_policy
			(tenant_id, object_kind, object_id, version_label,
			 direction, plan_ref, plan_direction, binding_conversion,
			 policy_scope_ref, effective_starts_at, effective_ends_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 ON CONFLICT DO NOTHING`,
		version.Tenant().String(),
		uint8(version.Kind()),
		version.ObjectID().String(),
		version.Version().String(),
		direction,
		planRef,
		planDir,
		conv,
		scope,
		startsAt,
		endsAt,
	)
	if err != nil {
		return ports.PricePolicySaveOutcomeInvalid, fmt.Errorf("save price policy: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return ports.PricePolicySaved, nil
	}

	var existingDirection, existingPlan, existingPlanDir, existingConv, existingScope string
	var existingStarts time.Time
	var existingEnds *time.Time
	err = executor.QueryRow(ctx,
		`SELECT direction, plan_ref, plan_direction, binding_conversion,
		        policy_scope_ref, effective_starts_at, effective_ends_at
		   FROM party_commercial.commercial_price_policy
		  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4`,
		version.Tenant().String(),
		uint8(version.Kind()),
		version.ObjectID().String(),
		version.Version().String(),
	).Scan(&existingDirection, &existingPlan, &existingPlanDir, &existingConv,
		&existingScope, &existingStarts, &existingEnds)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.PricePolicySaveOutcomeInvalid, fmt.Errorf("save price policy: 撞键后读不回既有行")
	}
	if err != nil {
		return ports.PricePolicySaveOutcomeInvalid, fmt.Errorf("save price policy: %w", err)
	}
	if existingDirection == direction &&
		existingPlan == planRef &&
		existingPlanDir == planDir &&
		existingConv == conv &&
		existingScope == scope &&
		existingStarts.Equal(startsAt) &&
		sameOptionalTime(existingEnds, endsAt) {
		return ports.PricePolicyAlreadyRegistered, nil
	}
	return ports.PricePolicyContentConflict, nil
}

func registerPricePolicy(
	registry *domain.CommercialRegistry,
	version domain.CommercialVersion,
	row scannedPricePolicy,
) error {
	if !row.present() {
		return nil
	}
	if !row.complete() {
		return fmt.Errorf("price policy row is incomplete")
	}
	direction, err := priceDirectionFrom(*row.direction)
	if err != nil {
		return err
	}
	planDirection, err := priceDirectionFrom(*row.planDirection)
	if err != nil {
		return err
	}
	conversion, err := planBindingConversionFrom(*row.conversion)
	if err != nil {
		return err
	}
	plan, err := domain.NewPricingPlanReference(*row.planRef)
	if err != nil {
		return err
	}
	scope, err := domain.NewCommercialScopeReference(*row.scope)
	if err != nil {
		return err
	}
	endsAt := time.Time{}
	if row.endsAt != nil {
		endsAt = *row.endsAt
	}
	interval, err := domain.NewEffectiveInterval(*row.startsAt, endsAt)
	if err != nil {
		return err
	}
	if version.Status() != domain.CommercialVersionEffective {
		return nil
	}
	policy, err := domain.NewCommercialPricePolicy(
		version, direction, plan, planDirection, conversion, scope, interval)
	if err != nil {
		return err
	}
	registry.RegisterPricePolicy(policy)
	return nil
}

func priceDirectionFrom(raw string) (domain.PriceDirection, error) {
	switch raw {
	case domain.BuyDirection.String():
		return domain.BuyDirection, nil
	case domain.SellDirection.String():
		return domain.SellDirection, nil
	case domain.InternalDirection.String():
		return domain.InternalDirection, nil
	default:
		return domain.PriceDirectionInvalid, fmt.Errorf("unknown price direction %q", raw)
	}
}

func planBindingConversionFrom(raw string) (domain.PlanBindingConversion, error) {
	switch raw {
	case domain.PlanBindingConversionNone.String():
		return domain.PlanBindingConversionNone, nil
	case domain.PlanBindingFrozenBuyEvaluation.String():
		return domain.PlanBindingFrozenBuyEvaluation, nil
	default:
		return domain.PlanBindingConversionNone, fmt.Errorf("unknown plan binding conversion %q", raw)
	}
}

func sameOptionalTime(left, right *time.Time) bool {
	return (left == nil && right == nil) ||
		(left != nil && right != nil && left.Equal(*right))
}
