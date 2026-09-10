package main

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	psidentity "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/identity"
	psparty "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	shipmentports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	pcapplication "go.idp.xyz/idp-parcel/internal/partycommercial/application"
)

// 本文件是`面单继续尝试决定`写面的组合根（票 label-channel/30 做法 4）：`/shipment-requests/continued-attempt-closures`
// 与 `/shipment-requests/continued-attempt-reopenings` 两个受控编排端点背后的同一只编排，形照 assemble_withdrawal.go。
//
// 机制半边全部接真：登记册、当前有效终局、判断意图 outbox、决定标识走 PS 真库口；关闭 / 重开授权经 partycommercial
// 适配器接 PC 裁定编排与真授权册。实例半边留空、各归各口的「显式未配置」形状：授权请求映射（运营角色 / 法人 /
// 等级 / 范围 / 证据，`PAR-COM-13` / `PAR-COM-14`）nil——适配器答未形成，编排如实停在`未决 · 授权口不可用`，不代拟坐标、
// 不冒充`授权规则未配置`，也不默认任何角色有关闭权或重开权（CONTEXT「系统不得自动形成」在这一格的形）。
// 接线前后的区别不在结果在来源：恢复动作从「写代码」变成「登记参数」（ADR-0063）。

// transactionalContinuedAttemptDecision 把两条命令各包进一笔事务：PS 的写口按框架合同无事务即拒，事务边界归装配点
// （ADR-0134 决定三）。编排交回业务答案（含未决、拒绝、冲突）时事务提交——那几格什么都没写，提交等于无事；
// 编排返回错误（Insert / Save / 入队失败）时整笔回滚，不留「决定已落、判断意图丢了」的中间态。
type transactionalContinuedAttemptDecision struct {
	transactor bentoapp.Transactor
	inner      *shipmentapp.FormContinuedAttemptDecisionHandler
}

var _ shipmenthttp.ContinuedAttemptDecisionHandler = transactionalContinuedAttemptDecision{}

func (decision transactionalContinuedAttemptDecision) FormControlledClosure(
	ctx context.Context,
	command shipmentapp.FormControlledClosureCommand,
) (shipmentapp.ContinuedAttemptDecisionResult, error) {
	return decision.within(ctx, func(txCtx context.Context) (shipmentapp.ContinuedAttemptDecisionResult, error) {
		return decision.inner.FormControlledClosure(txCtx, command)
	})
}

func (decision transactionalContinuedAttemptDecision) FormReopening(
	ctx context.Context,
	command shipmentapp.FormReopeningCommand,
) (shipmentapp.ContinuedAttemptDecisionResult, error) {
	return decision.within(ctx, func(txCtx context.Context) (shipmentapp.ContinuedAttemptDecisionResult, error) {
		return decision.inner.FormReopening(txCtx, command)
	})
}

func (decision transactionalContinuedAttemptDecision) within(
	ctx context.Context,
	step func(context.Context) (shipmentapp.ContinuedAttemptDecisionResult, error),
) (shipmentapp.ContinuedAttemptDecisionResult, error) {
	var result shipmentapp.ContinuedAttemptDecisionResult
	err := decision.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, stepErr := step(txCtx)
		if stepErr != nil {
			return stepErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return shipmentapp.ContinuedAttemptDecisionResult{}, err
	}
	return result, nil
}

// continuedAttemptDecisionSeams 是 buildContinuedAttemptDecisionOrchestrationWith 收的可替换处，**只给测试用**：
// 授权请求映射是实例半边、仓里没有种子，真库用例要走到落册之后只能在这一层换合成映射；判断意图口可换成会失败的
// 替身，证入队失败整步回滚（票面完成判据 4）。生产装配从不填它们（buildContinuedAttemptDecisionOrchestration 交零值）。
type continuedAttemptDecisionSeams struct {
	Requests psparty.ContinuedAttemptDecisionAuthorizationRequestSource
	Handoff  shipmentports.ContinuedAttemptDecisionHandoff
}

// continuedAttemptDecisionOrchestration 是接线的产物：事务壳里的编排（端点调它），加裸编排本体——只给真库用例证
// 「不在事务里调用 → 写口拒」（票面完成判据 5），端点表不接它。
type continuedAttemptDecisionOrchestration struct {
	Shell shipmenthttp.ContinuedAttemptDecisionHandler
	Inner *shipmentapp.FormContinuedAttemptDecisionHandler
}

// buildContinuedAttemptDecisionOrchestration 是生产装配：映射显式未配置，判断意图口是真 outbox。
func buildContinuedAttemptDecisionOrchestration(db *bentopg.DB) (shipmenthttp.ContinuedAttemptDecisionHandler, error) {
	orchestration, err := buildContinuedAttemptDecisionOrchestrationWith(db, continuedAttemptDecisionSeams{})
	if err != nil {
		return nil, err
	}
	return orchestration.Shell, nil
}

// buildContinuedAttemptDecisionOrchestrationWith 让测试把缝配上合成串走通链的后半段；生产不调它。
func buildContinuedAttemptDecisionOrchestrationWith(db *bentopg.DB, seams continuedAttemptDecisionSeams) (continuedAttemptDecisionOrchestration, error) {
	none := continuedAttemptDecisionOrchestration{}
	if db == nil {
		return none, fmt.Errorf("parcel-api: continued attempt decision: db is nil")
	}
	clock := systemClock{}

	registers, err := pspostgres.NewContinuedAttemptRegisters(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: continued attempt registers: %w", err)
	}
	finals, err := pspostgres.NewFinalOutcomes(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: final outcomes: %w", err)
	}
	grants, err := pcpostgres.NewAuthorityGrants(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: authority grants: %w", err)
	}
	identities, err := psidentity.NewContinuedAttemptDecisions()
	if err != nil {
		return none, fmt.Errorf("parcel-api: continued attempt decision identities: %w", err)
	}
	var handoff shipmentports.ContinuedAttemptDecisionHandoff
	if seams.Handoff != nil {
		handoff = seams.Handoff
	} else {
		store, err := outbox.NewStore(db)
		if err != nil {
			return none, fmt.Errorf("parcel-api: outbox store: %w", err)
		}
		handoff, err = pspostgres.NewOutboxContinuedAttemptDecisionHandoff(db, store, clock)
		if err != nil {
			return none, fmt.Errorf("parcel-api: continued attempt decision handoff: %w", err)
		}
	}

	// 授权适配器接 PC 裁定编排的旧构造器即可：关闭与重开是运营侧自己的决定，决定方不经委派解出，委派读口在
	// 这两格不会被问到（适配器构造器头注）。RequestSource 按缝：生产为 nil，见文件头注。
	authorizer := psparty.NewContinuedAttemptDecisionAuthorizationAdapter(
		pcapplication.NewAdjudicateCommercialAuthorizationHandler(grants),
		seams.Requests,
	)

	handler, err := shipmentapp.NewFormContinuedAttemptDecisionHandler(shipmentapp.FormContinuedAttemptDecisionDeps{
		Authorizer: authorizer,
		Finals:     finals,
		Registers:  registers,
		Identities: identities,
		Handoff:    handoff,
		Clock:      clock,
	})
	if err != nil {
		return none, fmt.Errorf("parcel-api: continued attempt decision handler: %w", err)
	}
	return continuedAttemptDecisionOrchestration{
		Shell: transactionalContinuedAttemptDecision{transactor: db.Transactor(), inner: handler},
		Inner: handler,
	}, nil
}
