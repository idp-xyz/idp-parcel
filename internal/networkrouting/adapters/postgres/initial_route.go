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

// initialRouteConclusion 是 initial_route.conclusion 列的封闭二值。它只属行模型：
// ports.InitialRouteRecord 用两对 (value, has) 表达同一件事，列上折成一格便于 CHECK。
const (
	conclusionRouteFormed    = "ROUTE_FORMED"
	conclusionNoCurrentRoute = "NO_CURRENT_ROUTE"
)

// InitialRoutes 实现 ports.InitialRouteStore。幂等硬句钉在主键选维上（同一接受基线、
// 同一包裹和同一初始路由目的恰一个结果）；`已有记录`由 ON CONFLICT DO NOTHING 加零行
// 判定翻译（ADR-0031），撞键不把事务打进中止态——Save 之后编排还要同事务读回赢家。
type InitialRoutes struct {
	db *bentopg.DB
}

func NewInitialRoutes(db *bentopg.DB) (*InitialRoutes, error) {
	if db == nil {
		return nil, fmt.Errorf("network routing postgres: db is nil")
	}
	return &InitialRoutes{db: db}, nil
}

// planRow 与 legRow 是计划 jsonb 列的行模型；noRouteRow 是无路可走判断的。判断键
// 不进 jsonb：键由平铺列独家拥有，存两份迟早各说各话。
type planRow struct {
	Version       string         `json:"version"`
	Selected      string         `json:"selected"`
	Candidates    []candidateRow `json:"candidates"`
	Legs          []legRow       `json:"legs"`
	Strategy      string         `json:"strategy"`
	ViewRevision  string         `json:"view_revision"`
	JudgedAt      time.Time      `json:"judged_at"`
	EffectiveFrom time.Time      `json:"effective_from"`
}

type legRow struct {
	From        string    `json:"from"`
	To          string    `json:"to"`
	Responsible string    `json:"responsible"`
	Earliest    time.Time `json:"earliest"`
	Latest      time.Time `json:"latest"`
	WindowBasis string    `json:"window_basis"`
	Opaque      bool      `json:"opaque,omitempty"`
}

type noRouteRow struct {
	Candidates   []candidateRow `json:"candidates"`
	Strategy     string         `json:"strategy"`
	ViewRevision string         `json:"view_revision"`
	JudgedAt     time.Time      `json:"judged_at"`
}

// FindByKey 按完整判断键取回已提交结果。否定结果只回 false，不区分「不存在」与
// 「属于另一个租户」。读回的一切经领域构造门重建（FormInitialRoutePlan /
// FormNoCurrentRouteJudgment）：候选空间、段链连续与被选候选合格在读回处复验，
// 一次坏写入在这里暴露，而不是变成一个看起来合法的计划。
func (repository *InitialRoutes) FindByKey(
	ctx context.Context,
	key domain.InitialRouteJudgmentKey,
) (ports.InitialRouteRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.InitialRouteRecord{}, false, fmt.Errorf("find initial route: %w", err)
	}

	var (
		conclusion            string
		planJSON, noRouteJSON []byte
		recordedAt            time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT conclusion, plan, no_route, recorded_at
		   FROM network_routing.initial_route
		  WHERE tenant_id = $1
		    AND customer_account_id = $2
		    AND shipment_request_id = $3
		    AND acceptance_baseline = $4
		    AND declared_parcel_id = $5
		    AND service_purpose = $6`,
		key.TenantID.String(),
		key.CustomerAccountID.String(),
		key.ShipmentRequestID.String(),
		key.AcceptanceBaseline.String(),
		key.DeclaredParcelID.String(),
		key.ServicePurpose.String(),
	).Scan(&conclusion, &planJSON, &noRouteJSON, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.InitialRouteRecord{}, false, nil
	}
	if err != nil {
		return ports.InitialRouteRecord{}, false, fmt.Errorf("find initial route: %w", err)
	}

	switch conclusion {
	case conclusionRouteFormed:
		plan, err := rebuildPlan(key, planJSON)
		if err != nil {
			return ports.InitialRouteRecord{}, false, fmt.Errorf("find initial route: %w", err)
		}
		return ports.InitialRouteRecord{
			Key: key, Plan: plan, HasPlan: true, RecordedAt: recordedAt.UTC(),
		}, true, nil
	case conclusionNoCurrentRoute:
		judgment, err := rebuildNoRoute(key, noRouteJSON)
		if err != nil {
			return ports.InitialRouteRecord{}, false, fmt.Errorf("find initial route: %w", err)
		}
		return ports.InitialRouteRecord{
			Key: key, NoRoute: judgment, HasNoRoute: true, RecordedAt: recordedAt.UTC(),
		}, true, nil
	default:
		return ports.InitialRouteRecord{}, false, fmt.Errorf(
			"find initial route: unknown conclusion %q", conclusion)
	}
}

// Save 写下一次包裹级结果。同键已有记录时答`已有记录`——业务答案不是错误
// （ADR-0031），第二个写入方据此读回赢家（AT-NR-004 并发裁决）。
//
// 走 RequireExecutor：结果落库与同一步的交接发布必须同生共死，无事务时框架直接拒绝。
func (repository *InitialRoutes) Save(
	ctx context.Context,
	record ports.InitialRouteRecord,
) (ports.InitialRouteSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.InitialRouteSaveOutcomeInvalid, fmt.Errorf("save initial route: %w", err)
	}

	conclusion, planVersion, planJSON, noRouteJSON, err := initialRouteColumns(record)
	if err != nil {
		return ports.InitialRouteSaveOutcomeInvalid, fmt.Errorf("save initial route: %w", err)
	}

	key := record.Key
	tag, err := executor.Exec(ctx,
		`INSERT INTO network_routing.initial_route
			(tenant_id, customer_account_id, shipment_request_id,
			 acceptance_baseline, declared_parcel_id, service_purpose,
			 conclusion, plan_version, plan, no_route)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 ON CONFLICT DO NOTHING`,
		key.TenantID.String(),
		key.CustomerAccountID.String(),
		key.ShipmentRequestID.String(),
		key.AcceptanceBaseline.String(),
		key.DeclaredParcelID.String(),
		key.ServicePurpose.String(),
		conclusion,
		planVersion,
		planJSON,
		noRouteJSON,
	)
	if err != nil {
		return ports.InitialRouteSaveOutcomeInvalid, fmt.Errorf("save initial route: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.InitialRouteAlreadyRecorded, nil
	}
	return ports.InitialRouteSaved, nil
}

// initialRouteColumns 把记录折成行。计划与无路可走二居其一、记录键与判断本体的键
// 一致，都是装配契约：违反者是写入方缺陷，响亮报错不落库。
func initialRouteColumns(
	record ports.InitialRouteRecord,
) (conclusion string, planVersion *string, planJSON, noRouteJSON []byte, err error) {
	switch {
	case record.HasPlan && !record.HasNoRoute:
		if record.Plan.Key() != record.Key {
			return "", nil, nil, nil, errors.New("record key disagrees with the plan's judgment key")
		}
		version := record.Plan.Version().String()
		data, err := json.Marshal(rowOfPlan(record.Plan))
		if err != nil {
			return "", nil, nil, nil, err
		}
		return conclusionRouteFormed, &version, data, nil, nil
	case record.HasNoRoute && !record.HasPlan:
		if record.NoRoute.Key() != record.Key {
			return "", nil, nil, nil, errors.New("record key disagrees with the judgment's key")
		}
		data, err := json.Marshal(rowOfNoRoute(record.NoRoute))
		if err != nil {
			return "", nil, nil, nil, err
		}
		return conclusionNoCurrentRoute, nil, nil, data, nil
	default:
		return "", nil, nil, nil, errors.New(
			"an initial route record carries exactly one of plan and no-route")
	}
}

func rowOfPlan(plan domain.InitialRoutePlan) planRow {
	legs := plan.Legs()
	legRows := make([]legRow, 0, len(legs))
	for _, leg := range legs {
		window := leg.Window()
		legRows = append(legRows, legRow{
			From:        leg.From().String(),
			To:          leg.To().String(),
			Responsible: leg.Responsible().String(),
			Earliest:    window.Earliest(),
			Latest:      window.Latest(),
			WindowBasis: window.Basis().String(),
			Opaque:      leg.Opaque(),
		})
	}
	return planRow{
		Version:       plan.Version().String(),
		Selected:      plan.SelectedCandidate().String(),
		Candidates:    rowsOfCandidates(plan.Candidates()),
		Legs:          legRows,
		Strategy:      plan.Strategy().String(),
		ViewRevision:  plan.ViewRevision().String(),
		JudgedAt:      plan.JudgedAt(),
		EffectiveFrom: plan.EffectiveFrom(),
	}
}

func rowOfNoRoute(judgment domain.NoCurrentRouteJudgment) noRouteRow {
	return noRouteRow{
		Candidates:   rowsOfCandidates(judgment.Candidates()),
		Strategy:     judgment.Strategy().String(),
		ViewRevision: judgment.ViewRevision().String(),
		JudgedAt:     judgment.JudgedAt(),
	}
}

func rebuildPlan(key domain.InitialRouteJudgmentKey, data []byte) (domain.InitialRoutePlan, error) {
	var row planRow
	if err := json.Unmarshal(data, &row); err != nil {
		return domain.InitialRoutePlan{}, err
	}
	version, err := domain.NewRoutePlanVersionID(row.Version)
	if err != nil {
		return domain.InitialRoutePlan{}, err
	}
	selected, err := domain.NewCandidateID(row.Selected)
	if err != nil {
		return domain.InitialRoutePlan{}, err
	}
	candidates, err := candidatesOfRows(row.Candidates)
	if err != nil {
		return domain.InitialRoutePlan{}, err
	}
	legs := make([]domain.PlannedLeg, 0, len(row.Legs))
	for _, stored := range row.Legs {
		leg, err := rebuildLeg(stored)
		if err != nil {
			return domain.InitialRoutePlan{}, err
		}
		legs = append(legs, leg)
	}
	strategy, err := domain.NewRouteStrategyReference(row.Strategy)
	if err != nil {
		return domain.InitialRoutePlan{}, err
	}
	revision, err := domain.NewNetworkViewRevision(row.ViewRevision)
	if err != nil {
		return domain.InitialRoutePlan{}, err
	}
	return domain.FormInitialRoutePlan(domain.InitialRoutePlanSpec{
		Key:           key,
		Version:       version,
		Selected:      selected,
		Candidates:    candidates,
		Legs:          legs,
		Strategy:      strategy,
		ViewRevision:  revision,
		JudgedAt:      row.JudgedAt,
		EffectiveFrom: row.EffectiveFrom,
	})
}

func rebuildLeg(stored legRow) (domain.PlannedLeg, error) {
	from, err := domain.NewPlanNodeReference(stored.From)
	if err != nil {
		return domain.PlannedLeg{}, err
	}
	to, err := domain.NewPlanNodeReference(stored.To)
	if err != nil {
		return domain.PlannedLeg{}, err
	}
	responsible, err := domain.NewResponsiblePartyReference(stored.Responsible)
	if err != nil {
		return domain.PlannedLeg{}, err
	}
	basis, err := domain.NewWindowBasisReference(stored.WindowBasis)
	if err != nil {
		return domain.PlannedLeg{}, err
	}
	window, err := domain.NewPlannedTimeWindow(stored.Earliest, stored.Latest, basis)
	if err != nil {
		return domain.PlannedLeg{}, err
	}
	return domain.NewPlannedLeg(domain.PlannedLegSpec{
		From:        from,
		To:          to,
		Responsible: responsible,
		Window:      window,
		Opaque:      stored.Opaque,
	})
}

func rebuildNoRoute(
	key domain.InitialRouteJudgmentKey,
	data []byte,
) (domain.NoCurrentRouteJudgment, error) {
	var row noRouteRow
	if err := json.Unmarshal(data, &row); err != nil {
		return domain.NoCurrentRouteJudgment{}, err
	}
	candidates, err := candidatesOfRows(row.Candidates)
	if err != nil {
		return domain.NoCurrentRouteJudgment{}, err
	}
	strategy, err := domain.NewRouteStrategyReference(row.Strategy)
	if err != nil {
		return domain.NoCurrentRouteJudgment{}, err
	}
	revision, err := domain.NewNetworkViewRevision(row.ViewRevision)
	if err != nil {
		return domain.NoCurrentRouteJudgment{}, err
	}
	return domain.FormNoCurrentRouteJudgment(domain.NoCurrentRouteJudgmentSpec{
		Key:          key,
		Candidates:   candidates,
		Strategy:     strategy,
		ViewRevision: revision,
		JudgedAt:     row.JudgedAt,
	})
}

// rowsOfCandidates 与 candidatesOfRows 复用可达性判断的 candidateRow 行模型：两张
// 判断库的候选是同一个领域对象，行模型分家会让同一格证据两处两个形状。
func rowsOfCandidates(candidates []domain.RouteCandidate) []candidateRow {
	rows := make([]candidateRow, 0, len(candidates))
	for _, candidate := range candidates {
		rows = append(rows, candidateRow{
			ID:      candidate.ID().String(),
			Outcome: candidate.Outcome().String(),
			Reason:  candidate.Reason().String(),
		})
	}
	return rows
}

func candidatesOfRows(rows []candidateRow) ([]domain.RouteCandidate, error) {
	candidates := make([]domain.RouteCandidate, 0, len(rows))
	for _, row := range rows {
		id, err := domain.NewCandidateID(row.ID)
		if err != nil {
			return nil, err
		}
		outcome, err := candidateOutcomeFrom(row.Outcome)
		if err != nil {
			return nil, err
		}
		var reason domain.CandidateReason
		if row.Reason != "" {
			reason, err = domain.NewCandidateReason(row.Reason)
			if err != nil {
				return nil, err
			}
		}
		candidate, err := domain.NewRouteCandidate(id, outcome, reason)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate)
	}
	return candidates, nil
}
