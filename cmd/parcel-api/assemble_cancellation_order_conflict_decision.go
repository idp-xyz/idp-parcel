package main

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
)

// transactionalCancellationOrderConflictDecision 把冲突解开决定编排包进一笔事务：取消边界锁、决定留痕、终局不采用记录的作废与续办信封同生共死
// （ADR-0094 决定四）。编排返回错误即整笔回滚——留痕落了信封没入队，或作废落了留痕没写，都会让被这份取消挡住的来源既不被放行又没有人重驱；业务上的确定答案
// （已被决定）不是错误，事务照常提交。
type transactionalCancellationOrderConflictDecision struct {
	transactor bentoapp.Transactor
	inner      *shipmentapp.DecideCancellationOrderConflictHandler
}

var _ shipmenthttp.CancellationOrderConflictDecisionHandler = transactionalCancellationOrderConflictDecision{}

func (decision transactionalCancellationOrderConflictDecision) Handle(
	ctx context.Context,
	command shipmentapp.DecideCancellationOrderConflictCommand,
) (shipmentapp.DecideCancellationOrderConflictResult, error) {
	var result shipmentapp.DecideCancellationOrderConflictResult
	err := decision.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		decided, decideErr := decision.inner.Handle(txCtx, command)
		if decideErr != nil {
			return decideErr
		}
		result = decided
		return nil
	})
	if err != nil {
		return shipmentapp.DecideCancellationOrderConflictResult{}, err
	}
	return result, nil
}

// buildCancellationOrderConflictDecision 装配 `/cancellation-order-conflict-decisions` 的真编排（ADR-0295）。
//
// 取消边界锁与取消编排、本进程之外 parcel-dispatch 的收寄采用、面单终局判断、终局采用同一把：决定改的是「这件包裹有没有有效取消」，那几处正是在锁里读它。
// 作废用终局库（同一张 final_outcome），冲突与决定留痕用各自的真册；续办信封入 Outbox，由 parcel-dispatch 的续办门消费。
func buildCancellationOrderConflictDecision(db *bentopg.DB) (shipmenthttp.CancellationOrderConflictDecisionHandler, error) {
	conflicts, err := pspostgres.NewCancellationOrderConflicts(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: cancellation order conflicts: %w", err)
	}
	decisions, err := pspostgres.NewCancellationOrderConflictDecisions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: cancellation order conflict decisions: %w", err)
	}
	finals, err := pspostgres.NewFinalOutcomes(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: final outcomes: %w", err)
	}
	boundaryLocks, err := pspostgres.NewCancellationBoundaryLocks(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: cancellation boundary locks: %w", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: outbox store: %w", err)
	}
	clock := systemClock{}
	handoff, err := pspostgres.NewOutboxCancellationNotUpheldHandoff(db, store, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: cancellation not upheld handoff: %w", err)
	}
	return transactionalCancellationOrderConflictDecision{
		transactor: db.Transactor(),
		inner: shipmentapp.NewDecideCancellationOrderConflictHandler(shipmentapp.DecideCancellationOrderConflictDeps{
			Conflicts:    conflicts,
			Decisions:    decisions,
			Voider:       finals,
			Handoff:      handoff,
			BoundaryLock: boundaryLocks,
			Clock:        clock,
		}),
	}, nil
}
