package main

import (
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
)

// directOperatingNotifier 把面单渠道包裹的直接成本经营结果接上既有的派生编排。
// 没登记采用时它自己不调 Derive。分摊编排不从这里进生产。
func directOperatingNotifier(db *bentopg.DB, clock systemClock) (saapplication.DirectOperatingNotifier, error) {
	adoptions, err := sapostgres.NewOperatingBasisAdoptions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: operating basis adoptions: %w", err)
	}
	facts, err := sapostgres.NewParcelDirectFacts(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: parcel direct facts: %w", err)
	}
	dispositions, err := sapostgres.NewDirectOperatingDispositions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: direct operating dispositions: %w", err)
	}
	results, err := sapostgres.NewOperatingResults(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: operating results for direct margin: %w", err)
	}
	allocations, err := sapostgres.NewCostAllocations(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: cost allocations for direct margin: %w", err)
	}
	rules, err := sapostgres.NewAllocationRuleApplicability(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: allocation rules for direct margin: %w", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: operating handoff outbox: %w", err)
	}
	handoff, err := sapostgres.NewOutboxOperatingHandoff(db, store, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: operating handoff for direct margin: %w", err)
	}
	handler, err := saapplication.NewDeriveDirectOperatingResultsHandler(saapplication.DeriveDirectOperatingResultsDeps{
		Adoptions:    adoptions,
		Facts:        facts,
		Dispositions: dispositions,
		Results:      results,
		Allocate: saapplication.NewAllocateCostsHandler(saapplication.AllocateCostsDeps{
			Allocations: allocations, Results: results, Rules: rules, Downstream: handoff, Clock: clock,
		}),
		Clock: clock,
	})
	if err != nil {
		return nil, fmt.Errorf("parcel-api: direct operating results: %w", err)
	}
	return handler, nil
}
