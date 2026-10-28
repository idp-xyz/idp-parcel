package main

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	ccpostgres "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	customsapp "go.idp.xyz/idp-parcel/internal/customscompliance/application"
)

// 本文件是内部合规限制两口的组合根（票 launch-wiring/01，UC-CC-011，ADR-0334 决定七）。

// internalRestrictionFaces 把建立与决定各包进一笔事务：限制册的写口按框架合同无事务即拒，事务边界归装配点；决定先对限制头行取行锁、
// 再读回判再追加，同一限制上的两个决定靠这一笔事务串行（manage_internal_restriction.go 的头注）。壳复用关务三口那一只：
// 未决整笔回滚、再交回编排那一份答复；不发布意图——今天没有订阅者（ADR-0049）。
type internalRestrictionFaces struct {
	establishment customsDecisionTransaction[customsapp.EstablishInternalRestrictionCommand, customsapp.InternalRestrictionResult]
	decision      customsDecisionTransaction[customsapp.DecideInternalRestrictionCommand, customsapp.InternalRestrictionResult]
}

func (faces internalRestrictionFaces) Establish(
	ctx context.Context, command customsapp.EstablishInternalRestrictionCommand,
) (customsapp.InternalRestrictionResult, error) {
	return faces.establishment.Handle(ctx, command)
}

func (faces internalRestrictionFaces) Decide(
	ctx context.Context, command customsapp.DecideInternalRestrictionCommand,
) (customsapp.InternalRestrictionResult, error) {
	return faces.decision.Handle(ctx, command)
}

// buildInternalRestrictionFaces 装配 `/customs-internal-restrictions` 与 `/customs-internal-restriction-decisions` 背后的真编排：
// 缝只有内部合规限制册一只。哪些来源、依据、覆盖项与解除条件是租户取值，产品不代填；册空着时动作准入读到的就是「没有内部限制」。
func buildInternalRestrictionFaces(db *bentopg.DB) (internalRestrictionFaces, error) {
	store, err := ccpostgres.NewInternalRestrictions(db)
	if err != nil {
		return internalRestrictionFaces{}, fmt.Errorf("parcel-api: internal restrictions: %w", err)
	}
	handler, err := customsapp.NewManageInternalRestrictionHandler(customsapp.ManageInternalRestrictionDeps{Store: store})
	if err != nil {
		return internalRestrictionFaces{}, fmt.Errorf("parcel-api: manage internal restriction: %w", err)
	}
	undecided := func(result customsapp.InternalRestrictionResult) bool {
		return result.Outcome() == customsapp.InternalRestrictionUndecided
	}
	transactor := db.Transactor()
	return internalRestrictionFaces{
		establishment: customsDecisionTransaction[customsapp.EstablishInternalRestrictionCommand, customsapp.InternalRestrictionResult]{
			transactor: transactor, handle: handler.Establish, undecided: undecided,
		},
		decision: customsDecisionTransaction[customsapp.DecideInternalRestrictionCommand, customsapp.InternalRestrictionResult]{
			transactor: transactor, handle: handler.Decide, undecided: undecided,
		},
	}, nil
}
