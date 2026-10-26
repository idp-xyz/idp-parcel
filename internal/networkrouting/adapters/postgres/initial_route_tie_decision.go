package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// InitialRouteTieDecisions 实现 ports.InitialRouteTieDecisions 与并列读面（ADR-0309 决定四、六；迁移 0030）。
type InitialRouteTieDecisions struct {
	db *bentopg.DB
}

func NewInitialRouteTieDecisions(db *bentopg.DB) (*InitialRouteTieDecisions, error) {
	if db == nil {
		return nil, fmt.Errorf("network routing postgres: db is nil")
	}
	return &InitialRouteTieDecisions{db: db}, nil
}

var (
	_ ports.InitialRouteTieDecisions     = (*InitialRouteTieDecisions)(nil)
	_ ports.InitialRouteTieCatalogueRead = (*InitialRouteTieDecisions)(nil)
)

// Record 写下一行裁决证据。走 RequireExecutor：它与同键的 initial_route 行、发布意图同笔，脱开事务写会让三者各说各话。
// 撞键不翻成业务答案——initial_route 先成才走到这里，同一判断键再有一行是编排缺陷，主键或外键拒了就响亮报错。
func (repository *InitialRouteTieDecisions) Record(ctx context.Context, decision ports.InitialRouteTieDecision) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("record initial route tie decision: %w", err)
	}
	switch {
	case decision.DecidedAt.IsZero():
		return errors.New("record initial route tie decision: the decision time is missing")
	case decision.Basis.String() == "":
		return errors.New("record initial route tie decision: the decision basis is missing")
	case decision.DecidedBy.String() == "":
		return errors.New("record initial route tie decision: the decider is missing")
	}
	key := decision.Key
	if _, err := executor.Exec(ctx,
		`INSERT INTO network_routing.initial_route_tie_decision
			(tenant_id, customer_account_id, shipment_request_id, acceptance_baseline, declared_parcel_id, service_purpose,
			 chosen_candidate, plan_version, decided_by, decision_basis, decided_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		key.TenantID.String(), key.CustomerAccountID.String(), key.ShipmentRequestID.String(),
		key.AcceptanceBaseline.String(), key.DeclaredParcelID.String(), key.ServicePurpose.String(),
		decision.Candidate.String(), decision.PlanVersion.String(), decision.DecidedBy.String(),
		decision.Basis.String(), decision.DecidedAt.UTC(),
	); err != nil {
		return fmt.Errorf("record initial route tie decision: %w", err)
	}
	return nil
}

// ListInitialRouteTies 上列本租户未形成登记最近一次停在并列的包裹：全部候选、并列的那几家与打平的金额，以及决定状态。状态
// 经判断库与裁决证据联查得出，口径只是此刻的大概，裁决成不成立仍由决定口在判断键锁里判。租户条件进每条语句（ADR-0003）。
func (repository *InitialRouteTieDecisions) ListInitialRouteTies(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.InitialRouteTieCatalogueRow, error) {
	if err := routePlanListLimit("list initial route ties", limit); err != nil {
		return nil, err
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list initial route ties: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT n.customer_account_id, n.shipment_request_id, n.acceptance_baseline, n.declared_parcel_id, n.service_purpose,
		        n.candidates, n.tied_costs, n.accepted_at, n.settled_at,
		        r.declared_parcel_id IS NOT NULL AS formed,
		        d.chosen_candidate, d.plan_version, d.decided_by, d.decision_basis, d.decided_at
		   FROM network_routing.initial_route_not_formed n
		   LEFT JOIN network_routing.initial_route r
		          ON r.tenant_id = n.tenant_id AND r.customer_account_id = n.customer_account_id
		         AND r.shipment_request_id = n.shipment_request_id AND r.acceptance_baseline = n.acceptance_baseline
		         AND r.declared_parcel_id = n.declared_parcel_id AND r.service_purpose = n.service_purpose
		   LEFT JOIN network_routing.initial_route_tie_decision d
		          ON d.tenant_id = n.tenant_id AND d.customer_account_id = n.customer_account_id
		         AND d.shipment_request_id = n.shipment_request_id AND d.acceptance_baseline = n.acceptance_baseline
		         AND d.declared_parcel_id = n.declared_parcel_id AND d.service_purpose = n.service_purpose
		  WHERE n.tenant_id = $1
		    AND n.reason = 'CANDIDATES_TIED'
		  ORDER BY n.settled_at DESC, n.declared_parcel_id, n.customer_account_id, n.shipment_request_id,
		           n.acceptance_baseline, n.service_purpose
		  LIMIT $2`,
		tenant.String(), limit)
	if err != nil {
		return nil, fmt.Errorf("list initial route ties: %w", err)
	}
	defer rows.Close()

	listed := make([]ports.InitialRouteTieCatalogueRow, 0)
	for rows.Next() {
		var (
			row                                   ports.InitialRouteTieCatalogueRow
			candidatesJSON, costsJSON             []byte
			acceptedAt, settledAt                 time.Time
			formed                                bool
			chosen, planVersion, decidedBy, basis *string
			decidedAt                             *time.Time
		)
		if err := rows.Scan(&row.CustomerAccountID, &row.ShipmentRequestID, &row.AcceptanceBaseline, &row.DeclaredParcelID,
			&row.ServicePurpose, &candidatesJSON, &costsJSON, &acceptedAt, &settledAt, &formed,
			&chosen, &planVersion, &decidedBy, &basis, &decidedAt); err != nil {
			return nil, fmt.Errorf("list initial route ties: %w", err)
		}
		var candidates []candidateRow
		if err := json.Unmarshal(candidatesJSON, &candidates); err != nil {
			return nil, fmt.Errorf("list initial route ties: candidates of %s: %w", row.DeclaredParcelID, err)
		}
		var costs []tiedCostRow
		if err := json.Unmarshal(costsJSON, &costs); err != nil {
			return nil, fmt.Errorf("list initial route ties: tied costs of %s: %w", row.DeclaredParcelID, err)
		}
		for _, candidate := range candidates {
			row.Candidates = append(row.Candidates, ports.RerouteSuggestionCandidateRow{
				ID: candidate.ID, Outcome: candidate.Outcome, Reason: candidate.Reason,
			})
		}
		for _, cost := range costs {
			row.Tied = append(row.Tied, ports.InitialRouteTieCostRow{
				Candidate: cost.Candidate, AmountMinor: cost.AmountMinor, Currency: cost.Currency,
			})
		}
		row.AcceptedAt, row.SettledAt = acceptedAt.UTC(), settledAt.UTC()
		switch {
		case chosen != nil:
			row.State = ports.InitialRouteTieDecided
			row.HasDecision = true
			row.Decision = ports.InitialRouteTieDecisionRow{
				Candidate: *chosen, PlanVersion: *planVersion, DecidedBy: *decidedBy, Basis: *basis, DecidedAt: decidedAt.UTC(),
			}
		case formed:
			row.State = ports.InitialRouteTieFormedOtherwise
		default:
			row.State = ports.InitialRouteTiePending
		}
		listed = append(listed, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list initial route ties: %w", err)
	}
	return listed, nil
}
