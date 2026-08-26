package main

import (
	"context"
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
// 仍然包装，是给装配测试钉资格缝两态（未登记租户答待登记、已登记租户按册答）用的
// ——审核面今天没有 HTTP 端点，但它审的是生产装配的同一个处理器，不另造一份。
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
// 七条缝里六条接真：索赔库、追偿库、追偿标识签发、责任结论的 Outbox 意图交付、生产
// 时钟与资格规则目录。资格缝走多租户读适配器（租户从查询来，票 ve-claims-read-seams/01
// 把这条缝补上）：未登记租户答「声明不在场」的业务格，编排停在
// ELIGIBILITY_CATALOGUE_NOT_CONFIGURED——那是待登记的实例参数，不再是接线未决。材料
// 证据一条仍是「显式未配置」，理由与形状见 unconfiguredClaimEvidence（归集面机制未建，
// 票 ve-claims-read-seams/02）——受理入口本就不碰这条缝（受理只保全提交事实，资格审核
// 是下一个判断），它只让审核入口如实停在指名到缝的未决，不拦受理。
func buildClaimsOrchestration(db *bentopg.DB) (transactionalClaims, error) {
	claimStore, err := vepostgres.NewClaims(db)
	if err != nil {
		return transactionalClaims{}, fmt.Errorf("parcel-api: claim store: %w", err)
	}
	eligibility, err := vepostgres.NewMultiTenantClaimEligibilityRules(db)
	if err != nil {
		return transactionalClaims{}, fmt.Errorf("parcel-api: claim eligibility rules: %w", err)
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
		Eligibility: eligibility,
		Evidence:    unconfiguredClaimEvidence{},
		Recoveries:  recoveries,
		Identities:  identities,
		Settlement:  settlement,
		Clock:       clock,
	})
	return transactionalClaims{transactor: db.Transactor(), inner: handler}, nil
}
