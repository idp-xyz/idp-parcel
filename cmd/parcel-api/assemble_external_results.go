package main

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	ccpostgres "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	customsapp "go.idp.xyz/idp-parcel/internal/customscompliance/application"
)

// transactionalResults 把外部结果接收编排包进一笔事务，理由随 transactionalSubmission：
// 结果登记册的写口按框架合同无事务即拒，事务边界归装配点。编排交回业务答案（含未决、
// 留存不猜与同层冲突留存双方）时事务提交；意图投递失败不翻结果——编排吞成续办引用后
// 照常作答，重放重发同一份；编排返回错误时整笔回滚。
type transactionalResults struct {
	transactor bentoapp.Transactor
	inner      *customsapp.ReceiveExternalResultHandler
}

var _ customshttp.ResultHandler = transactionalResults{}

func (results transactionalResults) Handle(
	ctx context.Context,
	command customsapp.ReceiveExternalResultCommand,
) (customsapp.ReceiveExternalResultResult, error) {
	var result customsapp.ReceiveExternalResultResult
	err := results.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := results.inner.Handle(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return customsapp.ReceiveExternalResultResult{}, err
	}
	return result, nil
}

// buildExternalResultsOrchestration 装配 `/customs/external-results` 的真编排
// （UC-CC-006；接线票 `.scratch/parcel-api-remaining-endpoint-wiring/issues/05`）。
//
// 七条缝全接真，本票没有「显式未配置」缝：结果登记册（留存不猜形状入 CHECK）、提交
// 索引（读提交链自己的权威表）、版本化解释规则读口（租户随调用到达，按辖区与评估
// 时点解半开区间）、辖区回指链的单元与案件两读口（ADR-0070 问三甲：适用辖区从结果
// 范围→单元→案件取，不留「当前唯一辖区」兜底缝）、Outbox 意图交付与生产时钟。
//
// 实例半边的留白不在缝上而在登记册内容里：某（租户,层,辖区,时点）无已登记规则版本
// 时，真读口如实答未配置，编排停在指名的未决——恢复动作是经 parcel-customs-register
// 登记参数，不是改代码（ADR-0063 的分界）。绝不用默认口径猜监管语义。
func buildExternalResultsOrchestration(db *bentopg.DB) (customshttp.ResultHandler, error) {
	resultStore, err := ccpostgres.NewExternalResults(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: external results: %w", err)
	}
	submissions, err := ccpostgres.NewSubmissionIndex(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: submission index: %w", err)
	}
	rules, err := ccpostgres.NewInterpretationRuleView(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: interpretation rule view: %w", err)
	}
	units, err := ccpostgres.NewDeclarationUnits(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: declaration units: %w", err)
	}
	cases, err := ccpostgres.NewCustomsCases(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: customs cases: %w", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: outbox store: %w", err)
	}
	clock := systemClock{}
	downstream, err := ccpostgres.NewOutboxExternalResultHandoff(db, store, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: external result handoff: %w", err)
	}
	handler := customsapp.NewReceiveExternalResultHandler(customsapp.ReceiveExternalResultDeps{
		Results:     resultStore,
		Submissions: submissions,
		Rules:       rules,
		Units:       units,
		Cases:       cases,
		Downstream:  downstream,
		Clock:       clock,
	})
	return transactionalResults{transactor: db.Transactor(), inner: handler}, nil
}
