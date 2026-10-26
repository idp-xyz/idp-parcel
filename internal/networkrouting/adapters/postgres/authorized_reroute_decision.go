package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// AuthorizedRerouteDecisions 实现 ports.AuthorizedRerouteDecisionStore（ADR-0284 决定五，迁移 0029）：主键=（租户，建议所在
// 复核的触发关联），一份建议至多一份决定，撞键由 ON CONFLICT DO NOTHING 加零行判定翻成`已有记录`，事务不进中止态。读回时新
// 计划经 FormInitialRoutePlan、决定经 FormRerouteDecision 重走构造门。
type AuthorizedRerouteDecisions struct {
	db *bentopg.DB
}

func NewAuthorizedRerouteDecisions(db *bentopg.DB) (*AuthorizedRerouteDecisions, error) {
	if db == nil {
		return nil, fmt.Errorf("network routing postgres: db is nil")
	}
	return &AuthorizedRerouteDecisions{db: db}, nil
}

var _ ports.AuthorizedRerouteDecisionStore = (*AuthorizedRerouteDecisions)(nil)

// FindBySuggestion 按（租户，建议关联）取回决定。否定结果只回 false，不区分不存在与属于另一个租户。
func (repository *AuthorizedRerouteDecisions) FindBySuggestion(
	ctx context.Context,
	tenant domain.TenantID,
	suggestion domain.RequestCorrelationID,
) (ports.AuthorizedRerouteDecision, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.AuthorizedRerouteDecision{}, false, fmt.Errorf("find authorized reroute decision: %w", err)
	}
	var (
		columns                      routeKeyColumns
		candidate, original, version string
		decidedBy, basis             string
		planJSON                     []byte
		decidedAt                    time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT customer_account_id, shipment_request_id, acceptance_baseline, declared_parcel_id, service_purpose,
		        chosen_candidate, original_plan, new_plan_version, new_plan, decided_by, decision_basis, decided_at
		   FROM network_routing.authorized_reroute_decision
		  WHERE tenant_id = $1
		    AND suggestion_correlation_id = $2`,
		tenant.String(), suggestion.String(),
	).Scan(&columns.customerAccount, &columns.shipmentRequest, &columns.acceptanceBaseline, &columns.declaredParcel,
		&columns.servicePurpose, &candidate, &original, &version, &planJSON, &decidedBy, &basis, &decidedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.AuthorizedRerouteDecision{}, false, nil
	}
	if err != nil {
		return ports.AuthorizedRerouteDecision{}, false, fmt.Errorf("find authorized reroute decision: %w", err)
	}
	decision, err := rebuildAuthorizedRerouteDecision(tenant, suggestion, columns, candidate, original, version,
		planJSON, decidedBy, basis, decidedAt)
	if err != nil {
		return ports.AuthorizedRerouteDecision{}, false, fmt.Errorf("find authorized reroute decision: %w: %v",
			ports.ErrStoredRouteRecordUnreadable, err)
	}
	return decision, true, nil
}

// Save 写下一份决定。同一份建议已有决定时答`已有记录`——业务答案不是错误，编排据此读回赢家。走 RequireExecutor：决定与它
// 取的判断键锁同一笔，脱开事务写，锁在写之前就放掉了。
func (repository *AuthorizedRerouteDecisions) Save(
	ctx context.Context,
	decision ports.AuthorizedRerouteDecision,
) (ports.AuthorizedRerouteDecisionSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.AuthorizedRerouteDecisionSaveOutcomeInvalid, fmt.Errorf("save authorized reroute decision: %w", err)
	}
	formed := decision.Decision
	plan := formed.NewPlan()
	decidedBy, byRole := formed.DecidedBy()
	// 行形状在写之前核一遍：这一册只收授权角色方式的决定；触发就是建议所在的那次复核，所择候选就是新计划选中的那一条，
	// 判断键与新计划同一把。哪一样对不上都是写入方的缺陷，落进去读回时才暴露就晚了。
	switch {
	case formed.Mode() != domain.AuthorizedRoleReroute || !byRole:
		return ports.AuthorizedRerouteDecisionSaveOutcomeInvalid, fmt.Errorf(
			"save authorized reroute decision: mode %q is not an authorized role decision", formed.Mode())
	case formed.Trigger().String() != decision.Suggestion.String():
		return ports.AuthorizedRerouteDecisionSaveOutcomeInvalid, fmt.Errorf(
			"save authorized reroute decision: trigger %q is not the suggestion %q", formed.Trigger(), decision.Suggestion)
	case plan.SelectedCandidate() != decision.Candidate:
		return ports.AuthorizedRerouteDecisionSaveOutcomeInvalid, fmt.Errorf(
			"save authorized reroute decision: the new plan selects %q, not %q", plan.SelectedCandidate(), decision.Candidate)
	case plan.Key() != decision.Key:
		return ports.AuthorizedRerouteDecisionSaveOutcomeInvalid, errors.New(
			"save authorized reroute decision: record key disagrees with the new plan's judgment key")
	case decision.Basis.String() == "":
		return ports.AuthorizedRerouteDecisionSaveOutcomeInvalid, errors.New(
			"save authorized reroute decision: the decision basis is missing")
	}
	planJSON, err := json.Marshal(rowOfPlan(plan))
	if err != nil {
		return ports.AuthorizedRerouteDecisionSaveOutcomeInvalid, fmt.Errorf("save authorized reroute decision: %w", err)
	}
	key := decision.Key
	tag, err := executor.Exec(ctx,
		`INSERT INTO network_routing.authorized_reroute_decision
			(tenant_id, suggestion_correlation_id,
			 customer_account_id, shipment_request_id, acceptance_baseline, declared_parcel_id, service_purpose,
			 chosen_candidate, original_plan, new_plan_version, new_plan, decided_by, decision_basis, decided_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		 ON CONFLICT DO NOTHING`,
		key.TenantID.String(),
		decision.Suggestion.String(),
		key.CustomerAccountID.String(),
		key.ShipmentRequestID.String(),
		key.AcceptanceBaseline.String(),
		key.DeclaredParcelID.String(),
		key.ServicePurpose.String(),
		decision.Candidate.String(),
		formed.OriginalPlan().String(),
		plan.Version().String(),
		planJSON,
		decidedBy.String(),
		decision.Basis.String(),
		formed.DecidedAt().UTC(),
	)
	if err != nil {
		return ports.AuthorizedRerouteDecisionSaveOutcomeInvalid, fmt.Errorf("save authorized reroute decision: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.AuthorizedRerouteDecisionAlreadyRecorded, nil
	}
	return ports.AuthorizedRerouteDecisionSaved, nil
}

// SuggestionOvertaken 答同一判断键上有没有建议所在那次以外的复核，其触发业务时间不早于 at（ADR-0284 决定二）。迁移 0022
// 之前没有业务时间的复核比不出先后，不算。在判断键锁里调，读得到锁前刚提交的复核。
func (repository *AuthorizedRerouteDecisions) SuggestionOvertaken(
	ctx context.Context,
	key domain.InitialRouteJudgmentKey,
	suggestion domain.RequestCorrelationID,
	at time.Time,
) (bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return false, fmt.Errorf("reroute suggestion overtaken: %w", err)
	}
	var overtaken bool
	if err := querier.QueryRow(ctx,
		`SELECT EXISTS (
		    SELECT 1
		      FROM network_routing.route_reassessment
		     WHERE tenant_id = $1
		       AND customer_account_id = $2
		       AND shipment_request_id = $3
		       AND acceptance_baseline = $4
		       AND declared_parcel_id = $5
		       AND service_purpose = $6
		       AND correlation_id <> $7
		       AND trigger_occurred_at >= $8)`,
		key.TenantID.String(),
		key.CustomerAccountID.String(),
		key.ShipmentRequestID.String(),
		key.AcceptanceBaseline.String(),
		key.DeclaredParcelID.String(),
		key.ServicePurpose.String(),
		suggestion.String(),
		at.UTC(),
	).Scan(&overtaken); err != nil {
		return false, fmt.Errorf("reroute suggestion overtaken: %w", err)
	}
	return overtaken, nil
}

// rebuildAuthorizedRerouteDecision 把一行决定经领域构造门重建：新计划过 FormInitialRoutePlan，决定过 FormRerouteDecision
// （仅建议之下、授权角色方式，触发取建议关联）。
func rebuildAuthorizedRerouteDecision(
	tenant domain.TenantID,
	suggestion domain.RequestCorrelationID,
	columns routeKeyColumns,
	candidate, original, version string,
	planJSON []byte,
	decidedBy, basis string,
	decidedAt time.Time,
) (ports.AuthorizedRerouteDecision, error) {
	none := ports.AuthorizedRerouteDecision{}
	key, err := rebuildRouteKey(tenant, columns)
	if err != nil {
		return none, err
	}
	plan, err := rebuildPlan(key, planJSON)
	if err != nil {
		return none, err
	}
	if plan.Version().String() != version {
		return none, fmt.Errorf("plan version %q disagrees with the row's %q", plan.Version(), version)
	}
	chosen, err := domain.NewCandidateID(candidate)
	if err != nil {
		return none, err
	}
	if plan.SelectedCandidate() != chosen {
		return none, fmt.Errorf("plan selects %q, the row records %q", plan.SelectedCandidate(), candidate)
	}
	originalPlan, err := domain.NewRoutePlanVersionID(original)
	if err != nil {
		return none, err
	}
	role, err := domain.NewAuthorizedRoleReference(decidedBy)
	if err != nil {
		return none, err
	}
	reference, err := domain.NewRerouteDecisionBasisReference(basis)
	if err != nil {
		return none, err
	}
	trigger, err := domain.NewRerouteTriggerReference(suggestion.String())
	if err != nil {
		return none, err
	}
	decision, err := domain.FormRerouteDecision(domain.RerouteDecisionSpec{
		Authority:    domain.SuggestionOnly,
		Mode:         domain.AuthorizedRoleReroute,
		DecidedBy:    role,
		Trigger:      trigger,
		OriginalPlan: originalPlan,
		NewPlan:      plan,
		DecidedAt:    decidedAt,
	})
	if err != nil {
		return none, err
	}
	return ports.AuthorizedRerouteDecision{
		Suggestion: suggestion,
		Key:        key,
		Candidate:  chosen,
		Basis:      reference,
		Decision:   decision,
	}, nil
}
