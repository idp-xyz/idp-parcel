package main

import (
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	ccpostgres "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	customsapp "go.idp.xyz/idp-parcel/internal/customscompliance/application"
)

// 本文件是关务案件关闭口的组合根（票 launch-wiring/02，UC-CC-010 关闭入口，code/03 的冲突项）。

// buildCaseClosure 装配 `/customs-case-closures` 背后的真编排。缝全接真：义务盘点读口（目录未登记答未决，不当成「没有义务所以可关」）、
// 关闭册、关闭意图的 Outbox 交付与时钟。壳复用关务三口那一只：关闭记录与它的发布意图同笔落地，未决整笔回滚。
// 义务目录与逐项依据是租户取值（PAR-CUS-06），没登时本口如实答未决并指名`OBLIGATION_INVENTORY_UNCONFIGURED`，不为此种任何行。
func buildCaseClosure(db *bentopg.DB) (customsDecisionTransaction[customsapp.CloseCustomsCaseCommand, customsapp.CloseCustomsCaseResult], error) {
	none := customsDecisionTransaction[customsapp.CloseCustomsCaseCommand, customsapp.CloseCustomsCaseResult]{}
	inventory, err := ccpostgres.NewObligationInventoryView(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: obligation inventory view: %w", err)
	}
	closures, err := ccpostgres.NewCaseClosures(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: case closures: %w", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: outbox store: %w", err)
	}
	clock := systemClock{}
	handoff, err := ccpostgres.NewOutboxCaseClosureHandoff(db, store, clock)
	if err != nil {
		return none, fmt.Errorf("parcel-api: case closure handoff: %w", err)
	}
	handler := customsapp.NewCloseCustomsCaseHandler(customsapp.CloseCustomsCaseDeps{
		Inventory: inventory, Store: closures, Downstream: handoff, Clock: clock,
	})
	return customsDecisionTransaction[customsapp.CloseCustomsCaseCommand, customsapp.CloseCustomsCaseResult]{
		transactor: db.Transactor(), handle: handler.Handle,
		undecided: func(result customsapp.CloseCustomsCaseResult) bool {
			return result.Outcome() == customsapp.CloseCaseUndecided
		},
	}, nil
}
