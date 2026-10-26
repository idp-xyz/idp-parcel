package main

import (
	"context"
	"errors"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	networkhttp "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/http"
	nrparcelshipment "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/parcelshipment"
	nrpostgres "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	nrapplication "go.idp.xyz/idp-parcel/internal/networkrouting/application"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
)

// errTieNotDecided 只在本壳里流转：编排答裁决成立以外的格时回调交它，逼事务回滚；回滚之后认出它，交回编排那一份答复。
var errTieNotDecided = errors.New("parcel-api: initial route tie not decided; transaction rolled back")

// transactionalInitialRouteTieDecider 把一次并列裁决包进一笔事务（ADR-0309 决定三）：判断键锁、读已有结果、重判、落判断库、落裁决
// 证据与交发布意图同笔，锁随事务释放。裁决成立以外的各格整笔回滚：它们本不该留下写入，未决那几格可能已写到一半。
type transactionalInitialRouteTieDecider struct {
	transactor bentoapp.Transactor
	inner      *nrapplication.DecideInitialRouteHandler
}

var _ networkhttp.InitialRouteTieDecider = transactionalInitialRouteTieDecider{}

func (decider transactionalInitialRouteTieDecider) Decide(
	ctx context.Context,
	command nrapplication.DecideInitialRouteCommand,
) (nrapplication.DecideInitialRouteResult, error) {
	var result nrapplication.DecideInitialRouteResult
	err := decider.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		decided, decideErr := decider.inner.Decide(txCtx, command)
		if decideErr != nil {
			return decideErr
		}
		result = decided
		if decided.Outcome() != nrapplication.InitialRouteTieDecided {
			return errTieNotDecided
		}
		return nil
	})
	if errors.Is(err, errTieNotDecided) {
		return result, nil
	}
	if err != nil {
		return nrapplication.DecideInitialRouteResult{}, err
	}
	return result, nil
}

// buildInitialRouteTieDecision 装配 `/route-initial-route-decisions` 背后的真编排：证据视图、关务来源、候选成本取数侧与受理基线
// 投影来源同授权改路决定那一份（buildRerouteDecision），发布意图走真 outbox，与 parcel-dispatch 初始路由那一份同一个口。
func buildInitialRouteTieDecision(db *bentopg.DB) (transactionalInitialRouteTieDecider, error) {
	clock := systemClock{}
	catalog, err := nrpostgres.NewNetworkCatalog(db)
	if err != nil {
		return transactionalInitialRouteTieDecider{}, fmt.Errorf("parcel-api: initial route tie network catalog: %w", err)
	}
	evidence, err := nrapplication.NewCatalogInitialRouteEvidence(catalog, rerouteCustomsSource(db), clock)
	if err != nil {
		return transactionalInitialRouteTieDecider{}, fmt.Errorf("parcel-api: initial route tie evidence: %w", err)
	}
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		return transactionalInitialRouteTieDecider{}, fmt.Errorf("parcel-api: initial route tie shipment requests: %w", err)
	}
	baselines, err := nrparcelshipment.NewAcceptedCarriedGeoSource(requests)
	if err != nil {
		return transactionalInitialRouteTieDecider{}, fmt.Errorf("parcel-api: initial route tie acceptance baseline geo: %w", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		return transactionalInitialRouteTieDecider{}, fmt.Errorf("parcel-api: outbox store: %w", err)
	}
	downstream, err := nrpostgres.NewOutboxInitialRouteHandoff(db, store, clock)
	if err != nil {
		return transactionalInitialRouteTieDecider{}, fmt.Errorf("parcel-api: initial route tie handoff: %w", err)
	}
	return initialRouteTieDeciderOver(db, clock, evidence, rerouteCosts(db, clock), baselines, downstream)
}

// initialRouteTieDeciderOver 把判断库各册接到给定的证据视图、候选成本取数侧、受理基线投影来源与发布意图口上。真库用例换掉这
// 四样，判断库、未形成登记、裁决证据、取号与事务壳与生产同一份。
func initialRouteTieDeciderOver(
	db *bentopg.DB,
	clock nrports.Clock,
	evidence nrports.InitialRouteEvidenceView,
	costs nrports.RouteCandidateCostSource,
	baselines nrports.AcceptanceBaselineGeoSource,
	downstream nrports.InitialRouteHandoff,
) (transactionalInitialRouteTieDecider, error) {
	none := transactionalInitialRouteTieDecider{}
	isolatedCosts, err := nrpostgres.NewSavepointedRouteCandidateCosts(db, costs)
	if err != nil {
		return none, fmt.Errorf("parcel-api: initial route tie candidate costs savepoint: %w", err)
	}
	locks, err := nrpostgres.NewRouteReassessments(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: initial route tie judgment lock: %w", err)
	}
	routes, err := nrpostgres.NewInitialRoutes(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: initial route tie routes: %w", err)
	}
	decisions, err := nrpostgres.NewInitialRouteTieDecisions(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: initial route tie decisions: %w", err)
	}
	identities, err := nrpostgres.NewRouteIdentities(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: initial route tie route identities: %w", err)
	}
	return transactionalInitialRouteTieDecider{
		transactor: db.Transactor(),
		inner: nrapplication.NewDecideInitialRouteHandler(nrapplication.DecideInitialRouteDeps{
			Locks:      locks,
			NotFormed:  routes,
			Routes:     routes,
			Decisions:  decisions,
			Baselines:  baselines,
			Evidence:   evidence,
			Costs:      isolatedCosts,
			Downstream: downstream,
			Identities: identities,
			Clock:      clock,
		}),
	}, nil
}
