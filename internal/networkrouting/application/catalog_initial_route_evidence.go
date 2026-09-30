package application

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// CatalogInitialRouteEvidence 是初始路由与复核所用证据视图的目录实现。除成本外的事实族从目录折出
// （ADR-0148 决定一、ADR-0175）。成本缺席不在这里补。
//
// 初始路由判断键没有 asOf：路由判断时点归本上下文（UC-NR-001「网络判断基线」一行），取时钟。
type CatalogInitialRouteEvidence struct {
	catalog ports.NetworkCatalogRead
	clock   ports.Clock
}

var _ ports.InitialRouteEvidenceView = (*CatalogInitialRouteEvidence)(nil)

func NewCatalogInitialRouteEvidence(
	catalog ports.NetworkCatalogRead,
	clock ports.Clock,
) (*CatalogInitialRouteEvidence, error) {
	if catalog == nil {
		return nil, fmt.Errorf("network routing application: network catalog read is required")
	}
	if clock == nil {
		return nil, fmt.Errorf("network routing application: clock is required")
	}
	return &CatalogInitialRouteEvidence{catalog: catalog, clock: clock}, nil
}

// LoadInitialRouteEvidence 三格见 ports.InitialRouteEvidenceView。成本不在这里折（归 routing-first-cut/10）。
func (view *CatalogInitialRouteEvidence) LoadInitialRouteEvidence(
	ctx context.Context,
	key domain.InitialRouteJudgmentKey,
	carried ports.RequestCarriedContent,
) (ports.InitialRouteEvidence, bool, error) {
	none := ports.InitialRouteEvidence{}
	asOf := view.clock.Now()
	snapshot, configured, err := view.catalog.LoadDefinitionsAt(ctx, key.TenantID, asOf)
	if err != nil {
		return none, false, fmt.Errorf("load initial route evidence: %w", err)
	}
	if !catalogConfiguredFor(snapshot, configured, key.ServicePurpose) {
		return none, false, nil
	}
	strategy, err := applicableStrategy(snapshot.Strategies, key.ServicePurpose)
	if err != nil {
		return none, false, err
	}
	strategyRef, err := domain.NewRouteStrategyReference(versionReference(strategy.Code, strategy.Version))
	if err != nil {
		return none, false, err
	}
	candidates, err := generateCatalogCandidates(snapshot, key.ServicePurpose)
	if err != nil {
		return none, false, err
	}
	areas, err := resolveCandidateServiceAreas(candidates, carried.Geo)
	if err != nil {
		return none, false, err
	}
	executability, err := foldPathExecutability(candidates, snapshot.Adjustments)
	if err != nil {
		return none, false, err
	}
	projections, paths, executability, err := foldTimeAndPaths(candidates, asOf, snapshot, executability)
	if err != nil {
		return none, false, err
	}
	return ports.InitialRouteEvidence{
		ServiceAreas:                         areas,
		PathExecutability:                    executability,
		Projections:                          projections,
		CommittedBound:                       carried.Commitment,
		RankingForm:                          strategy.RankingForm,
		FreezeForm:                           strategy.FreezeForm,
		FreezeRemainingSegmentLimit:          strategy.FreezeRemainingSegmentLimit,
		AutoRerouteForm:                      strategy.AutoRerouteForm,
		AutoRerouteImprovementThresholdMinor: strategy.AutoRerouteImprovementThresholdMinor,
		Paths:                                paths,
		Strategy:                             strategyRef,
		ViewRevision:                         snapshot.Revision,
	}, true, nil
}

func applicableStrategy(
	strategies []ports.RouteStrategyDefinitionVersion,
	purpose domain.ServicePurpose,
) (ports.RouteStrategyDefinitionVersion, error) {
	var matched []ports.RouteStrategyDefinitionVersion
	for _, strategy := range strategies {
		if strategy.ApplicableScope == purpose.String() {
			matched = append(matched, strategy)
		}
	}
	if len(matched) != 1 {
		return ports.RouteStrategyDefinitionVersion{}, fmt.Errorf("%w: %d route strategy versions apply to %s",
			ErrCatalogUnresolvable, len(matched), purpose)
	}
	return matched[0], nil
}

func foldTimeAndPaths(
	candidates []catalogCandidate,
	asOf time.Time,
	snapshot ports.NetworkCatalogSnapshot,
	executability []domain.PathExecutability,
) ([]domain.CandidateTimeProjection, []ports.CandidatePath, []domain.PathExecutability, error) {
	nodes := map[string]ports.NodeDefinitionVersion{}
	for _, node := range snapshot.Nodes {
		nodes[node.Code] = node
	}
	projections := make([]domain.CandidateTimeProjection, 0, len(candidates))
	paths := make([]ports.CandidatePath, 0, len(candidates))
	for index, candidate := range candidates {
		projection, legs, missing, err := projectCandidateTime(candidate, asOf, nodes, snapshot.Calendars)
		if err != nil {
			return nil, nil, nil, err
		}
		if missing != "" {
			updated, err := calendarMissing(candidate, missing, executability[index])
			if err != nil {
				return nil, nil, nil, err
			}
			executability[index] = updated
			continue
		}
		projections = append(projections, projection)
		paths = append(paths, ports.CandidatePath{Candidate: candidate.id, Legs: legs})
	}
	return projections, paths, executability, nil
}

func calendarMissing(candidate catalogCandidate, missing string, current domain.PathExecutability) (domain.PathExecutability, error) {
	if current.Outcome() == domain.PathNotExecutable {
		return current, nil
	}
	schedule, err := domain.NewScheduleVersionReference("CALENDAR_NOT_REGISTERED/" + missing)
	if err != nil {
		return domain.PathExecutability{}, err
	}
	return domain.NewPathExecutability(domain.PathExecutabilitySpec{
		Candidate: candidate.id, Outcome: domain.PathNotExecutable, Schedule: schedule,
	})
}
