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
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
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

// initialRouteTieSource 是总数与取页共用的前半句。state 在 CTE 里导出：筛选维与本页行读的是同一列，各格的判据只写这一处。
// 租户条件在 CTE 里，占 $1。
var initialRouteTieSource = fmt.Sprintf(`WITH ties AS (
	SELECT n.customer_account_id, n.shipment_request_id, n.acceptance_baseline, n.declared_parcel_id, n.service_purpose,
	       n.candidates, n.tied_costs, n.accepted_at, n.settled_at,
	       d.chosen_candidate, d.plan_version, d.decided_by, d.decision_basis, d.decided_at,
	       CASE WHEN d.chosen_candidate IS NOT NULL THEN '%s'
	            WHEN r.declared_parcel_id IS NOT NULL THEN '%s'
	            ELSE '%s'
	       END AS state
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
) `, ports.InitialRouteTieDecided, ports.InitialRouteTieFormedOtherwise, ports.InitialRouteTiePending)

var initialRouteTiePages = ports.InitialRouteTiePages.MustBind(cataloguepage.Columns{
	Sorts: map[string]string{"settledAt": "settled_at"},
	Identity: []string{
		"declared_parcel_id", "customer_account_id", "shipment_request_id", "acceptance_baseline", "service_purpose",
	},
	Filters: map[string]string{"state": "state"},
	Keyword: []string{"customer_account_id", "shipment_request_id", "acceptance_baseline", "declared_parcel_id"},
})

// scannedInitialRouteTie 是取页读回的一行。候选与金额的 jsonb 先留着，只解本页的行。
type scannedInitialRouteTie struct {
	row               ports.InitialRouteTieCatalogueRow
	candidates, costs []byte
}

// ListInitialRouteTies 上列本租户未形成登记最近一次停在并列的包裹：全部候选、并列的那几家与打平的金额，以及决定状态。状态
// 经判断库与裁决证据联查得出，口径只是此刻的大概，裁决成不成立仍由决定口在判断键锁里判。租户条件进每条语句（ADR-0003）。
// 按查询的筛选、q 与排序取游标之后的一页；缺省是最近停住的时刻倒序、同刻按包裹打头的判断键各段倒序。
func (repository *InitialRouteTieDecisions) ListInitialRouteTies(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.InitialRouteTieCatalogueRow], error) {
	fail := func(err error) (cataloguepage.Result[ports.InitialRouteTieCatalogueRow], error) {
		return cataloguepage.Result[ports.InitialRouteTieCatalogueRow]{}, fmt.Errorf("list initial route ties: %w", err)
	}
	if err := routePlanListLimit("list initial route ties", limit); err != nil {
		return cataloguepage.Result[ports.InitialRouteTieCatalogueRow]{}, err
	}
	plan, err := initialRouteTiePages.Plan(query, "", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}

	var total int64
	if err := querier.QueryRow(ctx, initialRouteTieSource+plan.CountSQL("ties"), plan.Args...).Scan(&total); err != nil {
		return fail(err)
	}
	rows, err := querier.Query(ctx, initialRouteTieSource+plan.PageSQL(
		`customer_account_id, shipment_request_id, acceptance_baseline, declared_parcel_id, service_purpose,
		 candidates, tied_costs, accepted_at, settled_at,
		 chosen_candidate, plan_version, decided_by, decision_basis, decided_at, state`,
		"ties",
	), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	var fetched []scannedInitialRouteTie
	for rows.Next() {
		var (
			scanned                               scannedInitialRouteTie
			acceptedAt, settledAt                 time.Time
			chosen, planVersion, decidedBy, basis *string
			decidedAt                             *time.Time
			state                                 string
		)
		row := &scanned.row
		if err := rows.Scan(&row.CustomerAccountID, &row.ShipmentRequestID, &row.AcceptanceBaseline, &row.DeclaredParcelID,
			&row.ServicePurpose, &scanned.candidates, &scanned.costs, &acceptedAt, &settledAt,
			&chosen, &planVersion, &decidedBy, &basis, &decidedAt, &state); err != nil {
			return fail(err)
		}
		row.AcceptedAt, row.SettledAt = acceptedAt.UTC(), settledAt.UTC()
		row.State = ports.InitialRouteTieState(state)
		if chosen != nil {
			row.HasDecision = true
			row.Decision = ports.InitialRouteTieDecisionRow{
				Candidate: *chosen, PlanVersion: *planVersion, DecidedBy: *decidedBy, Basis: *basis, DecidedAt: decidedAt.UTC(),
			}
		}
		fetched = append(fetched, scanned)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	rows.Close()

	page, err := cataloguepage.Finish(query, fetched, limit, total, initialRouteTiePosition)
	if err != nil {
		return fail(err)
	}
	listed := make([]ports.InitialRouteTieCatalogueRow, 0, len(page.Rows))
	for _, scanned := range page.Rows {
		row := scanned.row
		var candidates []candidateRow
		if err := json.Unmarshal(scanned.candidates, &candidates); err != nil {
			return fail(fmt.Errorf("candidates of %s: %w", row.DeclaredParcelID, err))
		}
		var costs []tiedCostRow
		if err := json.Unmarshal(scanned.costs, &costs); err != nil {
			return fail(fmt.Errorf("tied costs of %s: %w", row.DeclaredParcelID, err))
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
		listed = append(listed, row)
	}
	return cataloguepage.Result[ports.InitialRouteTieCatalogueRow]{Rows: listed, Next: page.Next, Total: page.Total}, nil
}

// initialRouteTiePosition 取一行在缺省序上的位置，标识段序与 initialRouteTiePages 的落列一致。本册只放出 settledAt 一维；另放
// 可排维时要按 sortField 取那一维的列值。
func initialRouteTiePosition(scanned scannedInitialRouteTie, _ string) cataloguepage.Position {
	row := scanned.row
	return cataloguepage.Position{
		Value: cataloguepage.FormatInstant(row.SettledAt),
		Identity: []string{
			row.DeclaredParcelID, row.CustomerAccountID, row.ShipmentRequestID, row.AcceptanceBaseline, row.ServicePurpose,
		},
	}
}
