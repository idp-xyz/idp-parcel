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

// transactionalPostIntakeDisposition 把收寄后服务处置决定编排包进一笔事务：取消边界锁、决定留痕与交给各对象所有者的信封同生共死（ADR-0094 决定四）。
// 编排返回错误即整笔回滚——留痕落了信封没入队，所有者就永远收不到请求；业务上的确定答案（已决定过、前一份已变等）不是错误，事务照常提交。
type transactionalPostIntakeDisposition struct {
	transactor bentoapp.Transactor
	inner      *shipmentapp.DecidePostIntakeDispositionHandler
}

var _ shipmenthttp.PostIntakeDispositionDecisionHandler = transactionalPostIntakeDisposition{}

func (decision transactionalPostIntakeDisposition) Handle(
	ctx context.Context,
	command shipmentapp.DecidePostIntakeDispositionCommand,
) (shipmentapp.DecidePostIntakeDispositionResult, error) {
	var result shipmentapp.DecidePostIntakeDispositionResult
	err := decision.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		decided, decideErr := decision.inner.Handle(txCtx, command)
		if decideErr != nil {
			return decideErr
		}
		result = decided
		return nil
	})
	if err != nil {
		return shipmentapp.DecidePostIntakeDispositionResult{}, err
	}
	return result, nil
}

// buildPostIntakeDisposition 装配 `/post-intake-disposition-decisions` 的真编排（ADR-0332）。
//
// 取消边界锁与取消编排、收寄采用、终局判断、取消先后冲突解开同一把：决定的前提（已越界、尚无终局或有效取消）正是它们在锁里写的东西。信封入 Outbox，
// 各对象所有者各自承接；系统不替所有者执行的几项与决定同笔落成待人工处理项（ADR-0341）。本进程不执行任何所有者的动作。
func buildPostIntakeDisposition(db *bentopg.DB) (shipmenthttp.PostIntakeDispositionDecisionHandler, error) {
	dispositions, err := pspostgres.NewPostIntakeDispositions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: post-intake dispositions: %w", err)
	}
	requests, err := pspostgres.NewPostIntakeDispositionRequests(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: post-intake disposition requests: %w", err)
	}
	adoptions, err := pspostgres.NewIntakeAdoptions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: intake adoptions: %w", err)
	}
	finals, err := pspostgres.NewFinalOutcomes(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: final outcomes: %w", err)
	}
	cancellations, err := pspostgres.NewParcelCancellations(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: parcel cancellations: %w", err)
	}
	boundaryLocks, err := pspostgres.NewCancellationBoundaryLocks(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: cancellation boundary locks: %w", err)
	}
	manualItems, err := pspostgres.NewPostIntakeManualItems(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: post-intake manual items: %w", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: outbox store: %w", err)
	}
	clock := systemClock{}
	handoff, err := pspostgres.NewOutboxPostIntakeDispositionHandoff(db, store, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: post-intake disposition handoff: %w", err)
	}
	return transactionalPostIntakeDisposition{
		transactor: db.Transactor(),
		inner: shipmentapp.NewDecidePostIntakeDispositionHandler(shipmentapp.DecidePostIntakeDispositionDeps{
			Dispositions:  dispositions,
			Requests:      requests,
			Adoptions:     adoptions,
			Finals:        finals,
			Cancellations: cancellations,
			Handoff:       handoff,
			ManualItems:   manualItems,
			BoundaryLock:  boundaryLocks,
			Clock:         clock,
		}),
	}, nil
}
