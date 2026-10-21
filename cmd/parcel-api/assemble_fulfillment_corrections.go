package main

import (
	"context"
	"errors"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// 本文件是更正两口的组合根（票 demo-chain-gaps/57，ADR-0232）：运输收费发生项失效与解除履约段采用背后的真编排。

// errChargeOccurrenceInvalidationUndecided 只在本壳里流转：失效编排答未决时逼事务回滚。未决的几格要么什么都没写，要么落了失效版本
// 而信封没交上——提交它，结算就永远不知道那笔成本要纠错。回滚之后认出它，交回编排那一份答复。
var errChargeOccurrenceInvalidationUndecided = errors.New("parcel-api: charge occurrence invalidation undecided; transaction rolled back")

// transactionalChargeOccurrenceInvalidator 把一次失效包进一笔事务：范围锁、发生项册与 Outbox 的写口按框架合同无事务即拒。失效版本
// 与它的已形成信封同笔落地；未决或报错整笔回滚。其余各格只读过、或只重交了同一封信（按信封 ID 认领），提交与回滚无别。
type transactionalChargeOccurrenceInvalidator struct {
	transactor bentoapp.Transactor
	inner      *tfapp.InvalidateChargeOccurrenceHandler
}

var _ tfhttp.ChargeOccurrenceInvalidator = transactionalChargeOccurrenceInvalidator{}

func (invalidator transactionalChargeOccurrenceInvalidator) Invalidate(
	ctx context.Context,
	command tfapp.InvalidateChargeOccurrenceCommand,
) (tfapp.ChargeOccurrenceInvalidationResult, error) {
	var result tfapp.ChargeOccurrenceInvalidationResult
	err := invalidator.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := invalidator.inner.Invalidate(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		if handled.Outcome() == tfapp.ChargeOccurrenceInvalidationUndecided {
			return errChargeOccurrenceInvalidationUndecided
		}
		return nil
	})
	if errors.Is(err, errChargeOccurrenceInvalidationUndecided) {
		return result, nil
	}
	if err != nil {
		return tfapp.ChargeOccurrenceInvalidationResult{}, err
	}
	return result, nil
}

// buildChargeOccurrenceInvalidation 装配 `/transport-fulfillment-charge-occurrence-invalidations` 背后的失效编排。缝全接真：发生项
// 登记册（沿链读、落失效版本、范围锁是同一个适配器）、已形成信封的 Outbox 意图与时钟。信封与登记口交的是同一类，结算那一侧按
// 键回读，认得出这一版是失效版本。
func buildChargeOccurrenceInvalidation(db *bentopg.DB) (tfhttp.ChargeOccurrenceInvalidator, error) {
	occurrences, err := tfpostgres.NewChargeOccurrences(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: charge occurrences for invalidation: %w", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: outbox store for charge occurrence invalidation: %w", err)
	}
	clock := systemClock{}
	handoff, err := tfpostgres.NewOutboxChargeOccurrenceHandoff(db, store, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: charge occurrence handoff for invalidation: %w", err)
	}
	handler := tfapp.NewInvalidateChargeOccurrenceHandler(tfapp.InvalidateChargeOccurrenceDeps{
		Occurrences: occurrences,
		Scopes:      occurrences,
		Handoff:     handoff,
		Clock:       clock,
	})
	return transactionalChargeOccurrenceInvalidator{transactor: db.Transactor(), inner: handler}, nil
}
