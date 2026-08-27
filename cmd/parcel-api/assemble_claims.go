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
)

// transactionalClaims 把索赔编排的入口各包进一笔事务，理由随 transactionalCancellation：
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
// 七条缝全部接真：索赔库、追偿库、追偿标识签发、责任结论的 Outbox 意图交付、生产
// 时钟、资格规则目录与材料证据。资格缝走多租户读适配器（租户从查询来，票
// ve-claims-read-seams/01）：未登记租户答「声明不在场」的业务格，编排停在
// ELIGIBILITY_CATALOGUE_NOT_CONFIGURED——那是待登记的实例参数，不再是接线未决。
// 材料证据缝读收讫减撤销的现存集（归集面由票 ve-claims-read-seams/02 建起，登记走
// 受控 CLI `parcel-ve-register`）：零行是「查过了，一件都没收到」的有效事实，差集
// 成立、限期补充可以推进——「无从查起」那一格随显式未配置桩退役。
func buildClaimsOrchestration(db *bentopg.DB) (transactionalClaims, error) {
	claimStore, err := vepostgres.NewClaims(db)
	if err != nil {
		return transactionalClaims{}, fmt.Errorf("parcel-api: claim store: %w", err)
	}
	eligibility, err := vepostgres.NewMultiTenantClaimEligibilityRules(db)
	if err != nil {
		return transactionalClaims{}, fmt.Errorf("parcel-api: claim eligibility rules: %w", err)
	}
	evidence, err := vepostgres.NewClaimMaterialReceipts(db)
	if err != nil {
		return transactionalClaims{}, fmt.Errorf("parcel-api: claim material receipts: %w", err)
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
		Evidence:    evidence,
		Recoveries:  recoveries,
		Identities:  identities,
		Settlement:  settlement,
		Clock:       clock,
	})
	return transactionalClaims{transactor: db.Transactor(), inner: handler}, nil
}
