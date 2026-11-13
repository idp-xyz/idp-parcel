package main

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	ccpostgres "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	customsapp "go.idp.xyz/idp-parcel/internal/customscompliance/application"
)

// regulatoryRestrictionFaces 把建立与解除各包进一笔事务：限制册的写口按框架合同无事务即拒，事务边界归装配点。壳复用关务
// 各决定口那一只：未决整笔回滚、再交回编排那一份答复。意图与限制在同一笔里入队：入队的 SQL 失败时这笔事务已经坏了，
// 提交不成、整笔回滚、答 5xx，操作者重发时限制与意图一起重来。
type regulatoryRestrictionFaces struct {
	establishment customsDecisionTransaction[customsapp.EstablishRestrictionOnResultCommand, customsapp.RestrictionResult]
	release       customsDecisionTransaction[customsapp.ReleaseRestrictionOnResultCommand, customsapp.RestrictionResult]
}

func (faces regulatoryRestrictionFaces) EstablishOnResult(
	ctx context.Context, command customsapp.EstablishRestrictionOnResultCommand,
) (customsapp.RestrictionResult, error) {
	return faces.establishment.Handle(ctx, command)
}

func (faces regulatoryRestrictionFaces) ReleaseOnResult(
	ctx context.Context, command customsapp.ReleaseRestrictionOnResultCommand,
) (customsapp.RestrictionResult, error) {
	return faces.release.Handle(ctx, command)
}

// buildRegulatoryRestrictionFaces 装配监管限制两口背后的真编排：限制册、外部结果册（只读，核依据）、Outbox 意图交付与
// 生产时钟。动作准入判断早就在读这本限制册；本装配补上的是往里写的那一半。哪条监管决定约束哪些动作由关务人员逐条声明，
// 产品不代填；册空着时动作准入读到的就是「没有监管限制」。
func buildRegulatoryRestrictionFaces(db *bentopg.DB) (regulatoryRestrictionFaces, error) {
	restrictions, err := ccpostgres.NewRestrictions(db)
	if err != nil {
		return regulatoryRestrictionFaces{}, fmt.Errorf("parcel-api: regulatory restrictions: %w", err)
	}
	results, err := ccpostgres.NewExternalResults(db)
	if err != nil {
		return regulatoryRestrictionFaces{}, fmt.Errorf("parcel-api: external results: %w", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		return regulatoryRestrictionFaces{}, fmt.Errorf("parcel-api: outbox store: %w", err)
	}
	clock := systemClock{}
	downstream, err := ccpostgres.NewOutboxRestrictionHandoff(db, store, clock)
	if err != nil {
		return regulatoryRestrictionFaces{}, fmt.Errorf("parcel-api: restriction handoff: %w", err)
	}
	handler := customsapp.NewManageRestrictionHandler(customsapp.ManageRestrictionDeps{
		Results:    results,
		Store:      restrictions,
		Downstream: downstream,
		Clock:      clock,
	})
	undecided := func(result customsapp.RestrictionResult) bool {
		return result.Outcome() == customsapp.RestrictionUndecided
	}
	transactor := db.Transactor()
	return regulatoryRestrictionFaces{
		establishment: customsDecisionTransaction[customsapp.EstablishRestrictionOnResultCommand, customsapp.RestrictionResult]{
			transactor: transactor, handle: handler.EstablishOnResult, rollBack: undecided,
		},
		release: customsDecisionTransaction[customsapp.ReleaseRestrictionOnResultCommand, customsapp.RestrictionResult]{
			transactor: transactor, handle: handler.ReleaseOnResult, rollBack: undecided,
		},
	}, nil
}
