package main

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// transactionalMovementFact 把移动事实登记包进一笔事务，理由随 transactionalDelivery：登记册的写口按
// 框架合同无事务即拒，事务边界归装配点；编排返回 error 时整笔回滚。
type transactionalMovementFact struct {
	transactor bentoapp.Transactor
	inner      *tfapp.RecordMovementFactHandler
}

var _ tfhttp.MovementFactHandler = transactionalMovementFact{}

func (movement transactionalMovementFact) Record(
	ctx context.Context,
	command tfapp.RecordMovementFactCommand,
) (tfapp.RecordMovementFactResult, error) {
	var result tfapp.RecordMovementFactResult
	err := movement.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := movement.inner.Record(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return tfapp.RecordMovementFactResult{}, err
	}
	return result, nil
}

// buildMovementFactOrchestration 装配 `/transport-fulfillment/movement-facts` 背后的真编排（票
// tf-segment-lifecycle-closure/05）。缝只有两条——移动事实登记册与时钟——全接真；没有意图交付：
// 移动是段内的事实，不是段的成立或结束依据，编排既不读段也不写段，也不向任何下游交意图。
func buildMovementFactOrchestration(db *bentopg.DB) (tfhttp.MovementFactHandler, error) {
	facts, err := tfpostgres.NewMovementFacts(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: movement facts: %w", err)
	}
	handler := tfapp.NewRecordMovementFactHandler(tfapp.RecordMovementFactDeps{
		Facts: facts,
		Clock: systemClock{},
	})
	return transactionalMovementFact{transactor: db.Transactor(), inner: handler}, nil
}
