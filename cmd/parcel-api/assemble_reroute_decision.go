package main

import (
	"context"
	"errors"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	ccpostgres "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	ccapplication "go.idp.xyz/idp-parcel/internal/customscompliance/application"
	nrcustoms "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/customscompliance"
	networkhttp "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/http"
	nrpricing "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/parcelpricing"
	nrparcelshipment "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/parcelshipment"
	nrpartycommercial "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/partycommercial"
	nrpostgres "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	nrapplication "go.idp.xyz/idp-parcel/internal/networkrouting/application"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	pppostgres "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	ppapplication "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
)

// errRerouteNotDecided 只在本壳里流转：编排答决定成立以外的格时回调交它，逼事务回滚；回滚之后认出它，交回编排那一份答复。
var errRerouteNotDecided = errors.New("parcel-api: reroute not decided; transaction rolled back")

// transactionalRerouteDecider 把一次授权改路决定包进一笔事务（ADR-0284 决定五）：判断键锁、读已有决定、判过时、重判与落册同笔，
// 锁随事务释放。决定成立以外的各格整笔回滚：它们本不该留下写入，未决那几格可能已写到一半。
type transactionalRerouteDecider struct {
	transactor bentoapp.Transactor
	inner      *nrapplication.DecideRerouteHandler
}

var _ networkhttp.RerouteDecider = transactionalRerouteDecider{}

func (decider transactionalRerouteDecider) Decide(
	ctx context.Context,
	command nrapplication.DecideRerouteCommand,
) (nrapplication.DecideRerouteResult, error) {
	var result nrapplication.DecideRerouteResult
	err := decider.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		decided, decideErr := decider.inner.Decide(txCtx, command)
		if decideErr != nil {
			return decideErr
		}
		result = decided
		if decided.Outcome() != nrapplication.RerouteDecided {
			return errRerouteNotDecided
		}
		return nil
	})
	if errors.Is(err, errRerouteNotDecided) {
		return result, nil
	}
	if err != nil {
		return nrapplication.DecideRerouteResult{}, err
	}
	return result, nil
}

// buildRerouteDecision 装配 `/route-reroute-decisions` 背后的真编排：证据视图、关务来源与候选成本取数侧与 parcel-dispatch 复核那一份
// 同一种装法（parcel-dispatch 的 initialRouteEvidence、customsApplicabilitySource 与 routeCosts），所携地理投影经消费侧适配器问 PS 的
// 受理基线地理投影读口（ADR-0284 决定三）。两个进程各是一个 package main，装法不共用代码；改一边时对照另一边。
func buildRerouteDecision(db *bentopg.DB) (transactionalRerouteDecider, error) {
	clock := systemClock{}
	catalog, err := nrpostgres.NewNetworkCatalog(db)
	if err != nil {
		return transactionalRerouteDecider{}, fmt.Errorf("parcel-api: reroute decision network catalog: %w", err)
	}
	evidence, err := nrapplication.NewCatalogInitialRouteEvidence(catalog, rerouteCustomsSource(db), clock)
	if err != nil {
		return transactionalRerouteDecider{}, fmt.Errorf("parcel-api: reroute decision evidence: %w", err)
	}
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		return transactionalRerouteDecider{}, fmt.Errorf("parcel-api: reroute decision shipment requests: %w", err)
	}
	baselines, err := nrparcelshipment.NewAcceptedCarriedGeoSource(requests)
	if err != nil {
		return transactionalRerouteDecider{}, fmt.Errorf("parcel-api: reroute decision acceptance baseline geo: %w", err)
	}
	return rerouteDeciderOver(db, clock, evidence, rerouteCosts(db, clock), baselines)
}

// rerouteDeciderOver 把判断库各册接到给定的证据视图、候选成本取数侧与受理基线投影来源上。真库用例换掉这三样，判断库、决定册、
// 取号与事务壳与生产同一份。取数侧在这里包 savepoint：它的读失败不连累决定那一笔事务，与复核同一层隔离。
func rerouteDeciderOver(
	db *bentopg.DB,
	clock nrports.Clock,
	evidence nrports.InitialRouteEvidenceView,
	costs nrports.RouteCandidateCostSource,
	baselines nrports.AcceptanceBaselineGeoSource,
) (transactionalRerouteDecider, error) {
	none := transactionalRerouteDecider{}
	isolatedCosts, err := nrpostgres.NewSavepointedRouteCandidateCosts(db, costs)
	if err != nil {
		return none, fmt.Errorf("parcel-api: reroute decision candidate costs savepoint: %w", err)
	}
	reassessments, err := nrpostgres.NewRouteReassessments(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: reroute decision reassessments: %w", err)
	}
	decisions, err := nrpostgres.NewAuthorizedRerouteDecisions(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: reroute decision register: %w", err)
	}
	applicabilities, err := nrpostgres.NewPlanApplicabilities(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: reroute decision plan applicabilities: %w", err)
	}
	identities, err := nrpostgres.NewRouteIdentities(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: reroute decision route identities: %w", err)
	}
	return transactionalRerouteDecider{
		transactor: db.Transactor(),
		inner: nrapplication.NewDecideRerouteHandler(nrapplication.DecideRerouteDeps{
			Reassessments: reassessments,
			Decisions:     decisions,
			Applicability: applicabilities,
			Baselines:     baselines,
			Evidence:      evidence,
			Costs:         isolatedCosts,
			Identities:    identities,
			Clock:         clock,
		}),
	}, nil
}

// rerouteCustomsSource 装配关务适用性取数侧：CC 的判断服务读两本目录的全量快照即时作答，NR 侧适配器翻译投影与答案。装配失败
// 留到判断时如实报出，编排答证据读不动（未决），不代拟一条满足。
func rerouteCustomsSource(db *bentopg.DB) nrports.CustomsApplicabilitySource {
	snapshots, err := ccpostgres.NewPortsPathsSnapshots(db)
	if err != nil {
		return brokenRerouteCustomsSource{err: fmt.Errorf("parcel-api: ports paths snapshots: %w", err)}
	}
	assessor, err := nrcustoms.NewCustomsApplicabilityAssessor(
		ccapplication.NewCustomsApplicabilityHandler(ccapplication.CustomsApplicabilityDeps{Catalog: snapshots}))
	if err != nil {
		return brokenRerouteCustomsSource{err: fmt.Errorf("parcel-api: customs applicability assessor: %w", err)}
	}
	return assessor
}

type brokenRerouteCustomsSource struct{ err error }

func (broken brokenRerouteCustomsSource) AssessCustomsApplicability(
	context.Context, nrports.CustomsApplicabilityQuery,
) (nrports.CustomsApplicabilityAssessment, error) {
	return nrports.CustomsApplicabilityAssessment{}, broken.err
}

// rerouteCosts 装配候选成本取数侧：目录依据、内部政策解析与方案装载全接库，包裹事实按判断点选来源（客户声明取自 PS，物理事实
// 的来源与复核那一份同一种装法：node-operations 还没有实际测量登记册，装「一项都没取得」，授权改路决定因此退回客户声明出价并
// 标明），逐段区域取租户登的段计价区域，评价走 PP「对给定价卡形成试算」的入口。
func rerouteCosts(db *bentopg.DB, clock systemClock) nrports.RouteCandidateCostSource {
	catalog, err := nrpostgres.NewNetworkCatalog(db)
	if err != nil {
		return brokenRerouteCostSource{err: fmt.Errorf("parcel-api: network catalog: %w", err)}
	}
	publications, err := pcpostgres.NewCommercialPublications(db)
	if err != nil {
		return brokenRerouteCostSource{err: fmt.Errorf("parcel-api: commercial publications: %w", err)}
	}
	priceCards, err := pppostgres.NewPriceCards(db)
	if err != nil {
		return brokenRerouteCostSource{err: fmt.Errorf("parcel-api: price cards: %w", err)}
	}
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		return brokenRerouteCostSource{err: fmt.Errorf("parcel-api: route parcel facts shipment requests: %w", err)}
	}
	declared, err := nrpricing.NewDeclaredParcelFacts(requests)
	if err != nil {
		return brokenRerouteCostSource{err: fmt.Errorf("parcel-api: route declared parcel facts: %w", err)}
	}
	facts, err := nrpricing.NewPointedParcelFacts(declared, nrpricing.NoPhysicalParcelFacts{}, clock)
	if err != nil {
		return brokenRerouteCostSource{err: fmt.Errorf("parcel-api: route parcel facts: %w", err)}
	}
	zones, err := nrpricing.NewRegisteredLegZones(catalog)
	if err != nil {
		return brokenRerouteCostSource{err: fmt.Errorf("parcel-api: leg pricing zones: %w", err)}
	}
	estimates, err := reroutePlanGivenEstimates(db, clock)
	if err != nil {
		return brokenRerouteCostSource{err: err}
	}
	return nrpricing.NewRouteCandidateCostAdapter(nrpricing.RouteCandidateCostDeps{
		Bases:     catalog,
		Internal:  nrpartycommercial.NewInternalPlanResolver(publications),
		Facts:     facts,
		Zones:     zones,
		Estimates: estimates,
		Plans:     priceCards,
		Clock:     clock,
	})
}

func reroutePlanGivenEstimates(db *bentopg.DB, clock systemClock) (*ppapplication.FormPlanGivenEstimateEvaluationsHandler, error) {
	seriesVersions, err := pppostgres.NewReferenceSeriesVersions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: plan-given estimate reference series register: %w", err)
	}
	seriesReviews, err := pppostgres.NewReferenceSeriesReviews(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: plan-given estimate reference series in-force resolver: %w", err)
	}
	catalogueVersions, err := pppostgres.NewReferenceCatalogueVersions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: plan-given estimate reference catalogue register: %w", err)
	}
	catalogueReviews, err := pppostgres.NewReferenceCatalogueReviews(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: plan-given estimate reference catalogue in-force resolver: %w", err)
	}
	planGiven, err := ppapplication.NewFormPlanGivenEstimateEvaluationsHandler(ppapplication.FormPlanGivenEstimateEvaluationsDeps{
		Clock:            clock,
		SeriesInForce:    seriesReviews,
		SeriesVersions:   seriesVersions,
		CatalogueInForce: catalogueReviews,
		Catalogues:       catalogueVersions,
	})
	if err != nil {
		return nil, fmt.Errorf("parcel-api: plan-given estimate evaluations: %w", err)
	}
	return planGiven, nil
}

type brokenRerouteCostSource struct{ err error }

func (broken brokenRerouteCostSource) LoadCandidateCosts(
	context.Context, nrdomain.InitialRouteJudgmentKey, nrdomain.ParcelFactsPoint, nrports.InitialRouteEvidence, nrports.RequestCarriedContent,
) (nrports.RouteCandidateCosts, error) {
	return nrports.RouteCandidateCosts{}, broken.err
}
