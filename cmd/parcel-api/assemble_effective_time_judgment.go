package main

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// transactionalEffectiveTimeJudge 把一次有效时间显式判断包进一笔事务，理由随 transactionalDelivery：
// 登记册的写口与 Outbox 入队按框架合同无事务即拒，事务边界归装配点；新版本登记与意图入队同笔落地，
// 编排返回 error 时整笔回滚。
type transactionalEffectiveTimeJudge struct {
	transactor bentoapp.Transactor
	inner      *tfapp.JudgeEffectiveTimeHandler
}

var _ tfhttp.EffectiveTimeJudge = transactionalEffectiveTimeJudge{}

func (judge transactionalEffectiveTimeJudge) Judge(
	ctx context.Context,
	command tfapp.JudgeEffectiveTimeCommand,
) (tfapp.JudgeEffectiveTimeResult, error) {
	var result tfapp.JudgeEffectiveTimeResult
	err := judge.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := judge.inner.Judge(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return tfapp.JudgeEffectiveTimeResult{}, err
	}
	return result, nil
}

// buildEffectiveTimeJudgment 装配 `/transport-fulfillment-effective-time-judgments` 背后的真编排（票
// label-channel/21）。四条缝全接真：事实登记册（读当前版、登记判断版本）、结果版本签发、Outbox 意图交付
// （判断过的版本交 visibility-exception）、时钟。本笔没有「显式未配置」缝：判断引用的都是本上下文自己的
// 事实，没有等租户参数的实例半边——规则那半边（ADR-0102 决定三第二种来源）在收编执行器上，不在这里。
//
// 同一只 ExternalTrackingFacts 也是 ports.ExternalTrackingFactReviewRead 的生产实现；判断面的读口在 main
// 直接取它交给查阅端点，不经本函数——读面不是编排。
func buildEffectiveTimeJudgment(db *bentopg.DB) (tfhttp.EffectiveTimeJudge, error) {
	facts, err := tfpostgres.NewExternalTrackingFacts(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: external tracking facts: %w", err)
	}
	versions, err := tfpostgres.NewResultVersions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: external tracking fact versions: %w", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: outbox store: %w", err)
	}
	clock := systemClock{}
	downstream, err := tfpostgres.NewOutboxExternalTrackingFactHandoff(db, store, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: external tracking fact handoff: %w", err)
	}
	handler := tfapp.NewJudgeEffectiveTimeHandler(tfapp.JudgeEffectiveTimeDeps{
		Facts:      facts,
		Identities: versions,
		Downstream: downstream,
		Clock:      clock,
	})
	return transactionalEffectiveTimeJudge{transactor: db.Transactor(), inner: handler}, nil
}
