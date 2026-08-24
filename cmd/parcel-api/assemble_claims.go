package main

import (
	"context"
	"errors"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
	veidentity "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/identity"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	visibilityapp "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	visibilitydomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// errEligibilityRuleViewNotConfigured 见 unconfiguredEligibilityRules 的注释。
var errEligibilityRuleViewNotConfigured = errors.New("parcel-api: claim eligibility rule view is not configured")

// unconfiguredEligibilityRules 是资格规则缝的「显式未配置」。
//
// 真适配器（VE postgres 的 ClaimEligibilityRules）是存在的，但它把租户钉在装配期
// ——那个形状为受控登记口与将来的单租户装配而设，而本进程是多租户入口，今天没有
// 任何租户可钉。钉一个空租户不行：视图会对一切查询答「声明不在场」，那是「查过了，
// 没人声明」的业务答案，顶「没接」用会把恢复动作指向登记参数；而租户真登记之后这里
// 的答案也不会变——接错看着像接对。真正缺的是这条缝带租户维的读法，那是机制半边，
// 不是等租户参数。
//
// 端口合同明写「依赖调不通作为错误返回」，据此对每次查询如实报错，编排停在指名到
// 缝的未决（ELIGIBILITY_RULES_UNAVAILABLE）并保持索赔项一字不动。绝不默认「一律
// 有资格」或「一律无资格」（接线票 04）：前者把没审过的索赔放行，后者是 ADR-0051
// 禁止的默认拒赔。
type unconfiguredEligibilityRules struct{}

var _ veports.EligibilityRuleView = unconfiguredEligibilityRules{}

func (unconfiguredEligibilityRules) RulesForClaim(
	context.Context,
	veports.EligibilityQuery,
) (veports.EligibilityRules, bool, error) {
	return veports.EligibilityRules{}, false, errEligibilityRuleViewNotConfigured
}

// unconfiguredClaimEvidence 是证据缝的「显式未配置」：VE 今天没有材料归集的读适配
// 器（归集面机制未建），这项事实无从查起。端口合同给这一格留了自己的答案——第二个
// 返回值为 false 即「材料归集无从查起，核不了，只能停在未决」，据此如实答 false，
// 不报错。
//
// 绝不交回空清单顶替：known 为真的零件是「归集查过了，一件都没收到」的有效事实，
// 差集会等于整份清单、把索赔推进限期补充——那是替一个不存在的归集面替客户立下补充
// 义务。归集面落地后这里换真读适配器。
type unconfiguredClaimEvidence struct{}

var _ veports.ClaimEvidenceView = unconfiguredClaimEvidence{}

func (unconfiguredClaimEvidence) ReceivedMaterials(
	context.Context,
	visibilitydomain.TenantID,
	visibilitydomain.ClaimBatchReference,
	visibilitydomain.ClaimItemID,
) ([]visibilitydomain.MaterialRequirementReference, bool, error) {
	return nil, false, nil
}

// transactionalClaims 把索赔编排的入口各包进一笔事务，理由随 transactionalSubmission：
// 索赔库的写口（UPSERT 与只增历史）按框架合同无事务即拒，事务边界归装配点。编排交回
// 业务答案（含未决与已有项）时事务提交；返回错误时整笔回滚。
//
// HTTP 端点只见 ClaimReceiver（受理）：资格审核与责任结论是另外两个判断，由内部授权
// 角色走各自入口，客户提交面连形状都看不见（CONTEXT 三判分步）。ScreenClaim 在这里
// 仍然包装，是给装配测试钉资格未配置缝用的——审核面今天没有 HTTP 端点，但它审的是
// 生产装配的同一个处理器，不另造一份。
type transactionalClaims struct {
	transactor bentoapp.Transactor
	inner      *visibilityapp.HandleClaimHandler
}

var _ visibilityhttp.ClaimReceiver = transactionalClaims{}

func (claims transactionalClaims) ReceiveClaim(
	ctx context.Context,
	command visibilityapp.ReceiveClaimCommand,
) (visibilityapp.HandleClaimResult, error) {
	var result visibilityapp.HandleClaimResult
	err := claims.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := claims.inner.ReceiveClaim(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return visibilityapp.HandleClaimResult{}, err
	}
	return result, nil
}

func (claims transactionalClaims) ScreenClaim(
	ctx context.Context,
	command visibilityapp.ScreenClaimCommand,
) (visibilityapp.HandleClaimResult, error) {
	var result visibilityapp.HandleClaimResult
	err := claims.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := claims.inner.ScreenClaim(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return visibilityapp.HandleClaimResult{}, err
	}
	return result, nil
}

// buildClaimsOrchestration 装配 `/claims` 的真编排（UC-VE-007；接线票
// `.scratch/parcel-api-remaining-endpoint-wiring/issues/04`）。
//
// 七条缝里五条接真：索赔库、追偿库、追偿标识签发、责任结论的 Outbox 意图交付与生产
// 时钟——全是本上下文自己的机制半边。资格规则与材料证据两条是「显式未配置」，各自的
// 理由与形状见 unconfiguredEligibilityRules 与 unconfiguredClaimEvidence——受理入口
// 本就不碰这两条缝（受理只保全提交事实，资格审核是下一个判断），它们只让审核入口
// 如实停在指名到缝的未决，不拦受理。
func buildClaimsOrchestration(db *bentopg.DB) (transactionalClaims, error) {
	claimStore, err := vepostgres.NewClaims(db)
	if err != nil {
		return transactionalClaims{}, fmt.Errorf("parcel-api: claim store: %w", err)
	}
	recoveries, err := vepostgres.NewRecoveries(db)
	if err != nil {
		return transactionalClaims{}, fmt.Errorf("parcel-api: recovery store: %w", err)
	}
	identities, err := veidentity.NewRecoveryMatters()
	if err != nil {
		return transactionalClaims{}, fmt.Errorf("parcel-api: recovery identities: %w", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		return transactionalClaims{}, fmt.Errorf("parcel-api: outbox store: %w", err)
	}
	clock := systemClock{}
	settlement, err := vepostgres.NewOutboxLiabilityHandoff(db, store, clock)
	if err != nil {
		return transactionalClaims{}, fmt.Errorf("parcel-api: liability handoff: %w", err)
	}
	handler := visibilityapp.NewHandleClaimHandler(visibilityapp.HandleClaimDeps{
		Claims:      claimStore,
		Eligibility: unconfiguredEligibilityRules{},
		Evidence:    unconfiguredClaimEvidence{},
		Recoveries:  recoveries,
		Identities:  identities,
		Settlement:  settlement,
		Clock:       clock,
	})
	return transactionalClaims{transactor: db.Transactor(), inner: handler}, nil
}
