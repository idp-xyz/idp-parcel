package main

import (
	"context"
	"errors"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// transactionalCapacityDecisions 把每次预占或释放包进一笔事务：池册写口按框架合同无事务即拒，事务边界归装配点。读池
// 在写事务里锁住池那一行，判余量与换存同笔，两笔并发的预占因此串行判、不会各按旧池放行（ADR-0203 决定四）。
//
// 编排答`未决`时整笔回滚再交回那一份答复，理由随 transactionalTransportCommissionSubmitter：读口在事务里报了 SQL 错误，
// PostgreSQL 就把整笔事务置为 aborted，照常提交只换回提交错误，线上落成 500；`未决`各格也没有任何东西要落库。
type transactionalCapacityDecisions struct {
	transactor bentoapp.Transactor
	inner      *tfapp.PrepareTransportOpportunityHandler
}

// errCapacityDecisionUndecided 只在本壳里流转：编排答`未决`时回调交它，逼事务回滚；回滚之后认出它，交回编排那一份答复。
var errCapacityDecisionUndecided = errors.New("parcel-api: capacity decision undecided; transaction rolled back")

var (
	_ tfhttp.CapacityReserver = transactionalCapacityDecisions{}
	_ tfhttp.CapacityReleaser = transactionalCapacityDecisions{}
)

func (decisions transactionalCapacityDecisions) Reserve(
	ctx context.Context,
	command tfapp.ReserveCapacityCommand,
) (tfapp.OpportunityResult, error) {
	return decisions.within(ctx, func(txCtx context.Context) (tfapp.OpportunityResult, error) {
		return decisions.inner.Reserve(txCtx, command)
	})
}

func (decisions transactionalCapacityDecisions) Release(
	ctx context.Context,
	command tfapp.ReleaseCapacityCommand,
) (tfapp.OpportunityResult, error) {
	return decisions.within(ctx, func(txCtx context.Context) (tfapp.OpportunityResult, error) {
		return decisions.inner.Release(txCtx, command)
	})
}

func (decisions transactionalCapacityDecisions) within(
	ctx context.Context,
	decide func(context.Context) (tfapp.OpportunityResult, error),
) (tfapp.OpportunityResult, error) {
	var result tfapp.OpportunityResult
	err := decisions.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := decide(txCtx)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		if handled.Outcome() == tfapp.OpportunityUndecided {
			return errCapacityDecisionUndecided
		}
		return nil
	})
	if errors.Is(err, errCapacityDecisionUndecided) {
		return result, nil
	}
	if err != nil {
		return tfapp.OpportunityResult{}, err
	}
	return result, nil
}

// buildCapacityDecisions 装配 `/transport-fulfillment-capacity-reservations` 与 `/transport-fulfillment-capacity-releases`
// 背后的真编排（票 demo-chain-gaps/20，ADR-0203）。两口是同一只编排的两个方法，端点表仍各收一参：装配测试才盖得住
// 「预占口接了释放编排」一类错接。缝只接池册与时钟：预占与释放不核班次（池建立时已核过，池身份由班次派生），也不交
// 消耗意图——消耗交接那条缝不接，交出去的类型上也就没有 Consume。
func buildCapacityDecisions(db *bentopg.DB) (transactionalCapacityDecisions, error) {
	pools, err := tfpostgres.NewCapacityPools(db)
	if err != nil {
		return transactionalCapacityDecisions{}, fmt.Errorf("parcel-api: capacity pools: %w", err)
	}
	handler := tfapp.NewPrepareTransportOpportunityHandler(tfapp.PrepareTransportOpportunityDeps{
		Pools: pools,
		Clock: systemClock{},
	})
	return transactionalCapacityDecisions{transactor: db.Transactor(), inner: handler}, nil
}
