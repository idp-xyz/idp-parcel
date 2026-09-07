package main

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	pppostgres "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	pricingapp "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
)

// transactionalEvaluationReplay 把评价回放编排包进一笔事务，理由随本目录其余事务包装：评价册的
// 写口按框架合同无事务即拒，事务边界归装配点。用例交回答复（含重复返原与「结构上重放不了」那
// 两格）时事务提交；返回错误时整笔回滚，端点按 ADR-0022 答「没形成答案」。
//
// 不并进 assemble_pricing_registration.go 的任何一个包装：回放是另一种命令、另一套答案代数，
// 且它**没有交付口**（ADR-0124 决定五）——合成一个类型之后装配点可以把登记那一格的编排接到
// 回放端点上而编译仍绿。
type transactionalEvaluationReplay struct {
	transactor bentoapp.Transactor
	inner      *pricingapp.ReplayPricingEvaluationHandler
}

var _ pricinghttp.EvaluationReplayer = transactionalEvaluationReplay{}

func (replay transactionalEvaluationReplay) Handle(
	ctx context.Context,
	command pricingapp.ReplayPricingEvaluationCommand,
) (pricingapp.ReplayPricingEvaluationResult, error) {
	var result pricingapp.ReplayPricingEvaluationResult
	err := replay.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := replay.inner.Handle(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return pricingapp.ReplayPricingEvaluationResult{}, err
	}
	return result, nil
}

// buildEvaluationReplayOrchestration 装配 `/pricing-evaluation-replays` 的真编排（ADR-0124；票
// wiring-baseline-remainder/06 件③）。评价册与价卡登记册是两只既有适配器：原评价从 Evaluations 读回，
// 原方案按原评价记录的版本引用从 PriceCards 读回——「原评价用的那一版」，不是此刻适用的那一版。
// 依赖里**没有 EvaluationHandoff**：回放结果不是新费用，装配点想接也没有那一格。
func buildEvaluationReplayOrchestration(db *bentopg.DB) (transactionalEvaluationReplay, error) {
	evaluations, err := pppostgres.NewEvaluations(db)
	if err != nil {
		return transactionalEvaluationReplay{}, fmt.Errorf("parcel-api: evaluation store: %w", err)
	}
	cards, err := pppostgres.NewPriceCards(db)
	if err != nil {
		return transactionalEvaluationReplay{}, fmt.Errorf("parcel-api: price card version loader: %w", err)
	}
	handler := pricingapp.NewReplayPricingEvaluationHandler(pricingapp.ReplayPricingEvaluationDeps{
		Store: evaluations,
		Plans: cards,
	})
	return transactionalEvaluationReplay{transactor: db.Transactor(), inner: handler}, nil
}
