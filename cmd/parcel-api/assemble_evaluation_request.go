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

// 本文件是 UC-SA-002 步 2「请求评价」SA 半边的组合根（票 sa-cc/08）：登记册、铸造口、Outbox 交接口与时钟四口
// 全接真适配器，编排的一次 Handle 包进一笔事务——登记与信封同事务是裁决 1 的前提，事务边界归装配点。
//
// `main` 的 `run` 在启动时装配它，**只为 fail-fast**（照面单渠道写链 label-channel/34 的同一条纪律）：组合根里
// 任一适配器构造失败，进程带原因退出，产物构造即丢。**今天没有运营端点、也没有进程内触发面调它**——三件合格
// 来源引用由「结算作业或上游编排」交进来（票面红线：不从 TF 登记册推），而那个调用方今天不存在；谁在什么
// 业务时点为哪些发生项发起请求是产品题，触发面另票。装配函数存在的意义是让「已装配、未触发」有一处可核：
// 接触发面的那张票只需要调 evaluationRequestOrchestration.Request。
//
// 不为此种任何行：请求的三件引用是实例半边（`PAR-SET-03`），仓里没有种子，组合根不造合成串顶上。

// evaluationRequestOrchestration 是接线的产物：请求评价编排的事务壳。
type evaluationRequestOrchestration struct {
	transactor bentoapp.Transactor
	inner      *saapplication.RequestBuyEvaluationHandler
}

// buildEvaluationRequestOrchestration 是生产装配：四口全是真适配器。
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
	return evaluationRequestOrchestration{transactor: db.Transactor(), inner: handler}, nil
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
