package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// PlanApplicabilities 实现 ports.PlanApplicabilityStore。主键取计划版本：计划本体
// 不可变，适用性另立一行，Save 换态不另开行。
type PlanApplicabilities struct {
	db *bentopg.DB
}

func NewPlanApplicabilities(db *bentopg.DB) (*PlanApplicabilities, error) {
	if db == nil {
		return nil, fmt.Errorf("network routing postgres: db is nil")
	}
	return &PlanApplicabilities{db: db}, nil
}

var _ ports.PlanApplicabilityStore = (*PlanApplicabilities)(nil)

// FindByPlan 按计划版本取回适用性。否定结果只回 false。读回经重建门复验四态形状，
// 不重放离场转换。
func (repository *PlanApplicabilities) FindByPlan(
	ctx context.Context,
	plan domain.RoutePlanVersionID,
) (domain.PlanApplicability, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.PlanApplicability{}, false, fmt.Errorf("find plan applicability: %w", err)
	}

	var stateName string
	var transitionedAt time.Time
	var basis, successor *string
	err = querier.QueryRow(ctx,
		`SELECT state, transitioned_at, basis, successor
		   FROM network_routing.plan_applicability
		  WHERE plan_id = $1`,
		plan.String(),
	).Scan(&stateName, &transitionedAt, &basis, &successor)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PlanApplicability{}, false, nil
	}
	if err != nil {
		return domain.PlanApplicability{}, false, fmt.Errorf("find plan applicability: %w", err)
	}

	applicability, err := rebuildPlanApplicability(plan, stateName, transitionedAt, basis, successor)
	if err != nil {
		return domain.PlanApplicability{}, false, fmt.Errorf("find plan applicability: %w", err)
	}
	return applicability, true, nil
}

// Save 写下或换写一份适用性。同一计划版本一行：首次登记当前有效，随后换离场态。
func (repository *PlanApplicabilities) Save(
	ctx context.Context,
	applicability domain.PlanApplicability,
) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save plan applicability: %w", err)
	}

	basis, hasBasis := applicability.Basis()
	successor, hasSuccessor := applicability.Successor()
	_, err = executor.Exec(ctx,
		`INSERT INTO network_routing.plan_applicability
			(plan_id, state, transitioned_at, basis, successor)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (plan_id) DO UPDATE SET
			state = EXCLUDED.state,
			transitioned_at = EXCLUDED.transitioned_at,
			basis = EXCLUDED.basis,
			successor = EXCLUDED.successor`,
		applicability.Plan().String(),
		applicability.State().String(),
		applicability.TransitionedAt().UTC(),
		optionalText(basis, hasBasis),
		optionalText(successor, hasSuccessor),
	)
	if err != nil {
		return fmt.Errorf("save plan applicability: %w", err)
	}
	return nil
}

func rebuildPlanApplicability(
	plan domain.RoutePlanVersionID,
	stateName string,
	transitionedAt time.Time,
	basis, successor *string,
) (domain.PlanApplicability, error) {
	state, err := planApplicabilityStateFrom(stateName)
	if err != nil {
		return domain.PlanApplicability{}, err
	}
	spec := domain.RehydratePlanApplicabilitySpec{
		Plan:           plan,
		State:          state,
		TransitionedAt: transitionedAt,
	}
	if basis != nil {
		spec.Basis, err = domain.NewApplicabilityBasisReference(*basis)
		if err != nil {
			return domain.PlanApplicability{}, err
		}
	}
	if successor != nil {
		spec.Successor, err = domain.NewRoutePlanVersionID(*successor)
		if err != nil {
			return domain.PlanApplicability{}, err
		}
	}
	return domain.RehydratePlanApplicability(spec)
}

func planApplicabilityStateFrom(raw string) (domain.PlanApplicabilityState, error) {
	switch raw {
	case domain.PlanCurrentlyEffective.String():
		return domain.PlanCurrentlyEffective, nil
	case domain.PlanSuperseded.String():
		return domain.PlanSuperseded, nil
	case domain.PlanLapsed.String():
		return domain.PlanLapsed, nil
	case domain.PlanConcluded.String():
		return domain.PlanConcluded, nil
	default:
		return 0, fmt.Errorf("unknown plan applicability state %q", raw)
	}
}

func optionalText[T interface{ String() string }](value T, present bool) *string {
	if !present {
		return nil
	}
	raw := value.String()
	return &raw
}
