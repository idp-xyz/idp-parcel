package main

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	pssettlement "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/settlementaccounting"
	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
)

// transactionalControlReleaseRefusalHandling 把释放拒收处理编排包进一笔事务：处理留痕、续办信封与结算侧的释放同生共死（ADR-0094 决定四）。
// 编排返回错误即整笔回滚——留痕落了信封没入队，或释放落了留痕没写，都会让这一笔占用既被「处理」又没有人重驱接受链；业务上的确定答案
// （释放未收口、再被拒收、已被处理）不是错误，事务照常提交，其中提供方已落库的那一半释放留着（ADR-0291）。
type transactionalControlReleaseRefusalHandling struct {
	transactor bentoapp.Transactor
	inner      *shipmentapp.HandleControlReleaseRefusalHandler
}

var _ shipmenthttp.ControlReleaseRefusalHandlingHandler = transactionalControlReleaseRefusalHandling{}

func (handling transactionalControlReleaseRefusalHandling) Handle(
	ctx context.Context,
	command shipmentapp.HandleControlReleaseRefusalCommand,
) (shipmentapp.HandleControlReleaseRefusalResult, error) {
	var result shipmentapp.HandleControlReleaseRefusalResult
	err := handling.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := handling.inner.Handle(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return shipmentapp.HandleControlReleaseRefusalResult{}, err
	}
	return result, nil
}

// buildControlReleaseRefusalHandling 装配 `/control-release-refusal-handlings` 的真编排（ADR-0293）。
//
// 「核对后在结算侧释放」不另立 SA 的人工释放入口：释放口就是接受链与终局各路用的那一只（SA 释放编排加两本账、外面一层 savepoint），
// 按控制结果上存下的作用域认领，不接作用域源。取锁用 SA 的作用域锁——释放前先取，与撤回类路径同一把同一序。
func buildControlReleaseRefusalHandling(db *bentopg.DB) (shipmenthttp.ControlReleaseRefusalHandlingHandler, error) {
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: shipment requests: %w", err)
	}
	refusals, err := pspostgres.NewControlReleaseRefusals(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: control release refusals: %w", err)
	}
	handlings, err := pspostgres.NewControlReleaseRefusalHandlings(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: control release refusal handlings: %w", err)
	}
	judgments, err := pspostgres.NewAcceptanceJudgments(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: acceptance judgments: %w", err)
	}
	freezes, err := sapostgres.NewFreezeLedgers(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: freeze ledgers: %w", err)
	}
	exposures, err := sapostgres.NewCreditExposureLedgers(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: credit exposure ledgers: %w", err)
	}
	scopeLocks, err := sapostgres.NewPreAcceptanceControlScopeLocks(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: pre-acceptance control scope locks: %w", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: outbox store: %w", err)
	}
	clock := systemClock{}
	handoff, err := pspostgres.NewOutboxControlReleaseRefusalHandledHandoff(db, store, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: control release refusal handled handoff: %w", err)
	}
	release := pssettlement.NewPreAcceptanceControlAdapter(pssettlement.PreAcceptanceControlAdapterDeps{
		Release: saapplication.NewReleasePreAcceptanceControlHandler(freezes, exposures, scopeLocks, clock),
		// Apply 与 Scopes 留空：处理只消费释放半边，作用域取控制结果上存下的那一份。
	})
	return transactionalControlReleaseRefusalHandling{
		transactor: db.Transactor(),
		inner: shipmentapp.NewHandleControlReleaseRefusalHandler(shipmentapp.HandleControlReleaseRefusalDeps{
			Requests:  requests,
			Refusals:  refusals,
			Handlings: handlings,
			Judgments: judgments,
			Release:   pspostgres.NewSavepointedControlRelease(db, release),
			ScopeLock: pssettlement.NewControlScopeLocks(scopeLocks),
			Handoff:   handoff,
			Clock:     clock,
		}),
	}, nil
}
