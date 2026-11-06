package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
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

// rerouteSuggestionSource 是总数与取页共用的前半句。state 在 CTE 里导出：筛选维与本页行读的是同一列，各格的判据只写这一处。
// 没有业务时间的旧行比不出先后，与决定口同一处置，读成已过时。租户条件在 CTE 里，占 $1。
var rerouteSuggestionSource = fmt.Sprintf(`WITH suggestions AS (
	SELECT r.correlation_id, r.customer_account_id, r.shipment_request_id, r.acceptance_baseline,
	       r.declared_parcel_id, r.service_purpose, r.reviewed_plan, r.lapse_basis, r.suggestion,
	       r.trigger_occurred_at, r.recorded_at,
	       d.chosen_candidate, d.new_plan_version, d.decided_by, d.decision_basis, d.decided_at,
	       CASE WHEN d.chosen_candidate IS NOT NULL THEN '%s'
	            WHEN r.trigger_occurred_at IS NULL OR EXISTS (
	                SELECT 1
	                  FROM network_routing.route_reassessment o
	                 WHERE o.tenant_id = r.tenant_id
	                   AND o.customer_account_id = r.customer_account_id
	                   AND o.shipment_request_id = r.shipment_request_id
	                   AND o.acceptance_baseline = r.acceptance_baseline
	                   AND o.declared_parcel_id = r.declared_parcel_id
	                   AND o.service_purpose = r.service_purpose
	                   AND o.correlation_id <> r.correlation_id
	                   AND o.trigger_occurred_at >= r.trigger_occurred_at) THEN '%s'
	            ELSE '%s'
	       END AS state
	  FROM network_routing.route_reassessment r
	  LEFT JOIN network_routing.authorized_reroute_decision d
	         ON d.tenant_id = r.tenant_id
	        AND d.suggestion_correlation_id = r.correlation_id
	 WHERE r.tenant_id = $1
	   AND r.suggestion IS NOT NULL
) `, ports.RerouteSuggestionDecided, ports.RerouteSuggestionSuperseded, ports.RerouteSuggestionPending)

var rerouteSuggestionPages = ports.RerouteSuggestionPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"recordedAt": "recorded_at"},
	Identity: []string{"correlation_id"},
	Filters:  map[string]string{"state": "state"},
	Keyword: []string{
		"correlation_id", "customer_account_id", "shipment_request_id", "acceptance_baseline", "declared_parcel_id",
		"reviewed_plan",
	},
})

// scannedRerouteSuggestion 是取页读回的一行。建议 jsonb 先留着，只解本页的行：多取来判下一页的那一行解不开，不该让本页答不出。
type scannedRerouteSuggestion struct {
	row        ports.RerouteSuggestionCatalogueRow
	suggestion []byte
}

// ListRerouteSuggestions 上列本租户带建议的复核，按查询的筛选、q 与排序取游标之后的一页；缺省是落册时刻倒序、同刻按关联倒序。
func (catalogue *RerouteSuggestionCatalogue) ListRerouteSuggestions(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.RerouteSuggestionCatalogueRow], error) {
	fail := func(err error) (cataloguepage.Result[ports.RerouteSuggestionCatalogueRow], error) {
		return cataloguepage.Result[ports.RerouteSuggestionCatalogueRow]{}, fmt.Errorf("list reroute suggestions: %w", err)
	}
	if err := routePlanListLimit("list reroute suggestions", limit); err != nil {
		return cataloguepage.Result[ports.RerouteSuggestionCatalogueRow]{}, err
	}
	plan, err := rerouteSuggestionPages.Plan(query, "", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}

	var total int64
	if err := querier.QueryRow(ctx, rerouteSuggestionSource+plan.CountSQL("suggestions"), plan.Args...).Scan(&total); err != nil {
		return fail(err)
	}
	rows, err := querier.Query(ctx, rerouteSuggestionSource+plan.PageSQL(
		`correlation_id, customer_account_id, shipment_request_id, acceptance_baseline,
		 declared_parcel_id, service_purpose, reviewed_plan, lapse_basis, suggestion,
		 trigger_occurred_at, recorded_at,
		 chosen_candidate, new_plan_version, decided_by, decision_basis, decided_at, state`,
		"suggestions",
	), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	var fetched []scannedRerouteSuggestion
	for rows.Next() {
		var (
			scanned                           scannedRerouteSuggestion
			reviewedPlan, lapseBasis          *string
			triggerAt                         *time.Time
			chosen, newPlan, decidedBy, basis *string
			decidedAt                         *time.Time
			state                             string
		)
		row := &scanned.row
		if err := rows.Scan(&row.CorrelationID, &row.CustomerAccountID, &row.ShipmentRequestID, &row.AcceptanceBaseline,
			&row.DeclaredParcelID, &row.ServicePurpose, &reviewedPlan, &lapseBasis, &scanned.suggestion, &triggerAt,
			&row.RecordedAt, &chosen, &newPlan, &decidedBy, &basis, &decidedAt, &state); err != nil {
			return fail(err)
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
		row.State = ports.RerouteSuggestionState(state)
		if chosen != nil {
			row.HasDecision = true
			row.Decision = ports.RerouteSuggestionDecisionRow{
				Candidate:      *chosen,
				NewPlanVersion: *newPlan,
				DecidedBy:      *decidedBy,
				Basis:          *basis,
				DecidedAt:      decidedAt.UTC(),
			}
		}
		fetched = append(fetched, scanned)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	rows.Close()

	page, err := cataloguepage.Finish(query, fetched, limit, total, rerouteSuggestionPosition)
	if err != nil {
		return fail(err)
	}
	listed := make([]ports.RerouteSuggestionCatalogueRow, 0, len(page.Rows))
	for _, scanned := range page.Rows {
		row := scanned.row
		var suggestion suggestionRow
		if err := json.Unmarshal(scanned.suggestion, &suggestion); err != nil {
			return fail(fmt.Errorf("suggestion of %s: %w", row.CorrelationID, err))
		}
		row.Blockers = append([]string{}, suggestion.Blockers...)
		row.SuggestedAt = suggestion.SuggestedAt.UTC()
		for _, candidate := range suggestion.Candidates {
			row.Candidates = append(row.Candidates, ports.RerouteSuggestionCandidateRow{
				ID: candidate.ID, Outcome: candidate.Outcome, Reason: candidate.Reason,
			})
		}
		listed = append(listed, row)
	}
	return cataloguepage.Result[ports.RerouteSuggestionCatalogueRow]{Rows: listed, Next: page.Next, Total: page.Total}, nil
}

// rerouteSuggestionPosition 取一行在缺省序上的位置。本册只放出 recordedAt 一维；另放可排维时要按 sortField 取那一维的列值。
func rerouteSuggestionPosition(scanned scannedRerouteSuggestion, _ string) cataloguepage.Position {
	return cataloguepage.Position{
		Value:    cataloguepage.FormatInstant(scanned.row.RecordedAt),
		Identity: []string{scanned.row.CorrelationID},
	}
}
