package main

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	saidentity "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/identity"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
)

// 本文件是 UC-SA-002 步 2「请求评价」SA 半边的组合根（票 sa-cc/08）：登记册、铸造口、Outbox 交接口与时钟
// 全接真适配器，编排的一次 Handle 包进一笔事务——登记与信封同事务是裁决 1 的前提，事务边界归装配点。
//
// `main` 的 `run` 在启动时装配它，**只为 fail-fast**（照面单渠道写链 label-channel/34 的同一条纪律）：组合根里
// 任一适配器构造失败，进程带原因退出。进程内触发面是 `Trigger`：发生项原因登记了「发生项形成」才发起请求，
// 没登记答未配置，不发起请求，也不预列发生项。三件合格来源引用仍由调用方交进来，不从 TF 登记册推。
//
// 不为此种任何行：请求的三件引用是实例半边（`PAR-SET-03`），仓里没有种子，组合根不造合成串顶上。

// evaluationRequestOrchestration 是接线的产物：请求评价编排的事务壳。
type evaluationRequestOrchestration struct {
	transactor bentoapp.Transactor
	inner      *saapplication.RequestBuyEvaluationHandler
	trigger    *saapplication.TriggerBuyEvaluationHandler
}

// buildEvaluationRequestOrchestration 是生产装配：每一口都是真适配器，没有替身位——本编排没有实例半边的缝
// 要留（三件引用由命令交进来，不在装配点取数），测试要走通只需一只真库。
func buildEvaluationRequestOrchestration(db *bentopg.DB) (evaluationRequestOrchestration, error) {
	none := evaluationRequestOrchestration{}
	if db == nil {
		return none, fmt.Errorf("parcel-api: evaluation request: db is nil")
	}
	clock := systemClock{}

	registry, err := sapostgres.NewEvaluationRequests(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: evaluation requests: %w", err)
	}
	identities, err := saidentity.NewEvaluationRequestIdentities()
	if err != nil {
		return none, fmt.Errorf("parcel-api: evaluation request identities: %w", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: outbox store: %w", err)
	}
	handoff, err := sapostgres.NewOutboxEvaluationRequestHandoff(db, store, clock)
	if err != nil {
		return none, fmt.Errorf("parcel-api: evaluation request handoff: %w", err)
	}
	handler, err := saapplication.NewRequestBuyEvaluationHandler(saapplication.RequestBuyEvaluationDeps{
		Registry:   registry,
		Identity:   identities,
		Downstream: handoff,
		Clock:      clock,
	})
	if err != nil {
		return none, fmt.Errorf("parcel-api: request buy evaluation handler: %w", err)
	}
	triggers, err := sapostgres.NewBuyEvaluationTriggers(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: buy evaluation triggers: %w", err)
	}
	trigger, err := saapplication.NewTriggerBuyEvaluationHandler(triggers, handler)
	if err != nil {
		return none, fmt.Errorf("parcel-api: buy evaluation trigger: %w", err)
	}
	return evaluationRequestOrchestration{transactor: db.Transactor(), inner: handler, trigger: trigger}, nil
}

// Request 把一次请求包进一笔事务：登记面写口按框架合同无事务即拒，信封与登记同笔落地或同笔回滚。编排交回
// error 时整笔回滚，调用方重放；业务答案（已请求 /`已存在`/ 未受理 / 未决）不是 error，照常提交。
func (orchestration evaluationRequestOrchestration) Request(
	ctx context.Context,
	command saapplication.RequestBuyEvaluationCommand,
) (saapplication.RequestBuyEvaluationResult, error) {
	var result saapplication.RequestBuyEvaluationResult
	err := orchestration.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := orchestration.inner.Handle(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return saapplication.RequestBuyEvaluationResult{}, err
	}
	return result, nil
}

// Trigger 是 BUY 评价请求的进程内触发面。发生项原因没登记「发生项形成」时答未配置，不调用 Request。
func (orchestration evaluationRequestOrchestration) Trigger(
	ctx context.Context,
	command saapplication.RequestBuyEvaluationCommand,
) (saapplication.BuyEvaluationTriggerResult, error) {
	var result saapplication.BuyEvaluationTriggerResult
	err := orchestration.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := orchestration.trigger.Trigger(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return saapplication.BuyEvaluationTriggerResult{}, err
	}
	return result, nil
}
