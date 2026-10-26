package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// RerouteSuggestionCatalogue 实现改路建议的列表读端口（ADR-0284 决定六）：管理台据它看哪几份建议待决定、各有哪几条候选。
// 建议本体住在复核行的 suggestion 列，这里照判断口的行模型解开转写，不另立一套读法；状态经决定册与同判断键的复核联查得出，
// 口径与决定编排的过时判据同一条，但只是此刻的大概——决定成不成立仍由决定口在判断键锁里判。租户条件进每条语句（ADR-0003）。
type RerouteSuggestionCatalogue struct {
	db *bentopg.DB
}

func NewRerouteSuggestionCatalogue(db *bentopg.DB) (*RerouteSuggestionCatalogue, error) {
	if db == nil {
		return nil, fmt.Errorf("network routing postgres: db is nil")
	}
	return &RerouteSuggestionCatalogue{db: db}, nil
}

var _ ports.RerouteSuggestionCatalogueRead = (*RerouteSuggestionCatalogue)(nil)

// ListRerouteSuggestions 上列本租户带建议的复核，按落册时间倒序、同刻按关联正序收尾。没有业务时间的旧行比不出先后，与决定口
// 同一处置，读成已过时。
func (catalogue *RerouteSuggestionCatalogue) ListRerouteSuggestions(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.RerouteSuggestionCatalogueRow, error) {
	if err := routePlanListLimit("list reroute suggestions", limit); err != nil {
		return nil, err
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list reroute suggestions: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT r.correlation_id, r.customer_account_id, r.shipment_request_id, r.acceptance_baseline,
		        r.declared_parcel_id, r.service_purpose, r.reviewed_plan, r.lapse_basis, r.suggestion,
		        r.trigger_occurred_at,
		        d.chosen_candidate, d.new_plan_version, d.decided_by, d.decision_basis, d.decided_at,
		        CASE WHEN r.trigger_occurred_at IS NULL THEN true
		             ELSE EXISTS (
		                 SELECT 1
		                   FROM network_routing.route_reassessment o
		                  WHERE o.tenant_id = r.tenant_id
		                    AND o.customer_account_id = r.customer_account_id
		                    AND o.shipment_request_id = r.shipment_request_id
		                    AND o.acceptance_baseline = r.acceptance_baseline
		                    AND o.declared_parcel_id = r.declared_parcel_id
		                    AND o.service_purpose = r.service_purpose
		                    AND o.correlation_id <> r.correlation_id
		                    AND o.trigger_occurred_at >= r.trigger_occurred_at)
		        END AS overtaken
		   FROM network_routing.route_reassessment r
		   LEFT JOIN network_routing.authorized_reroute_decision d
		          ON d.tenant_id = r.tenant_id
		         AND d.suggestion_correlation_id = r.correlation_id
		  WHERE r.tenant_id = $1
		    AND r.suggestion IS NOT NULL
		  ORDER BY r.recorded_at DESC, r.correlation_id
		  LIMIT $2`,
		tenant.String(), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list reroute suggestions: %w", err)
	}
	defer rows.Close()

	listed := make([]ports.RerouteSuggestionCatalogueRow, 0)
	for rows.Next() {
		var (
			row                               ports.RerouteSuggestionCatalogueRow
			reviewedPlan, lapseBasis          *string
			suggestionJSON                    []byte
			triggerAt                         *time.Time
			chosen, newPlan, decidedBy, basis *string
			decidedAt                         *time.Time
			overtaken                         bool
		)
		if err := rows.Scan(&row.CorrelationID, &row.CustomerAccountID, &row.ShipmentRequestID, &row.AcceptanceBaseline,
			&row.DeclaredParcelID, &row.ServicePurpose, &reviewedPlan, &lapseBasis, &suggestionJSON, &triggerAt,
			&chosen, &newPlan, &decidedBy, &basis, &decidedAt, &overtaken); err != nil {
			return nil, fmt.Errorf("list reroute suggestions: %w", err)
		}
		var suggestion suggestionRow
		if err := json.Unmarshal(suggestionJSON, &suggestion); err != nil {
			return nil, fmt.Errorf("list reroute suggestions: suggestion of %s: %w", row.CorrelationID, err)
		}
		if reviewedPlan != nil {
			row.ReviewedPlan = *reviewedPlan
		}
		if lapseBasis != nil {
			row.LapseBasis = *lapseBasis
		}
		if triggerAt != nil {
			row.TriggerOccurredAt = triggerAt.UTC()
		}
		row.Blockers = append([]string{}, suggestion.Blockers...)
		row.SuggestedAt = suggestion.SuggestedAt.UTC()
		for _, candidate := range suggestion.Candidates {
			row.Candidates = append(row.Candidates, ports.RerouteSuggestionCandidateRow{
				ID: candidate.ID, Outcome: candidate.Outcome, Reason: candidate.Reason,
			})
		}
		switch {
		case chosen != nil:
			row.State = ports.RerouteSuggestionDecided
			row.HasDecision = true
			row.Decision = ports.RerouteSuggestionDecisionRow{
				Candidate:      *chosen,
				NewPlanVersion: *newPlan,
				DecidedBy:      *decidedBy,
				Basis:          *basis,
				DecidedAt:      decidedAt.UTC(),
			}
		case overtaken:
			row.State = ports.RerouteSuggestionSuperseded
		default:
			row.State = ports.RerouteSuggestionPending
		}
		listed = append(listed, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list reroute suggestions: %w", err)
	}
	return listed, nil
}
