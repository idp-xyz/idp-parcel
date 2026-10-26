package main

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	psaccess "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/accessidentity"
	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	psidentity "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/identity"
	psparty "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	pssettlement "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/settlementaccounting"
	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	pcapplication "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
)

// transactionalWithdrawal 把撤回编排包进一笔事务，理由随 transactionalCancellation：PS 的
// 写口按框架合同无事务即拒，事务边界归装配点。编排交回业务答案（含未决与已有决定）时
// 事务提交——被拦下的撤回也已保全来源；编排返回错误时整笔回滚（指名查不到的委托正是
// 这一格：调用方的错不留半截写入）。
type transactionalWithdrawal struct {
	transactor bentoapp.Transactor
	inner      *shipmentapp.WithdrawShipmentRequestHandler
}

var _ shipmenthttp.WithdrawalHandler = transactionalWithdrawal{}

func (withdrawal transactionalWithdrawal) Handle(
	ctx context.Context,
	command shipmentapp.WithdrawShipmentRequestCommand,
) (shipmentapp.WithdrawShipmentRequestResult, error) {
	var result shipmentapp.WithdrawShipmentRequestResult
	err := withdrawal.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := withdrawal.inner.Handle(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return shipmentapp.WithdrawShipmentRequestResult{}, err
	}
	return result, nil
}

// buildWithdrawalOrchestration 装配撤回编排（UC-PS-005）的 PC 裁定授权形态——生产的首方撤回口不走它（走
// buildCustomerWithdrawalOrchestration），它留给代理人 1:N 与运营侧撤回将来挂 PC 授权的那一天，现在只有用例证这条链。
//
// 机制半边全部接真：来源保全、委托仓储、判断读写与决定标识走 PS 真库口，撤回授权经
// partycommercial 适配器接 PC 裁定编排与真授权册，冻结释放经 settlementaccounting 适配器
// 接 SA 释放编排与真两账本。实例半边照旧留空，各自的「显式未配置」形状各归各口：
//
//   - 撤回授权的请求映射（法人/权限等级/商业范围/时点，`PAR-COM-14`）nil——适配器答
//     未形成，编排如实停在`撤回授权不可用`，不代拟坐标也不冒充`授权规则未配置`；
//   - 释放不接作用域源：释放按施加时随控制结果存下的那一份作用域认领（ADR-0282 决定四），
//     不在这里重新推导；存量结果没存作用域时 SA 答未受理，编排记进释放未受理册待人处理。
//
// 接线前后的区别不在结果在来源：恢复动作从「写代码」变成「登记参数」（ADR-0063）。
func buildWithdrawalOrchestration(db *bentopg.DB) (shipmenthttp.WithdrawalHandler, error) {
	grants, err := pcpostgres.NewAuthorityGrants(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: authority grants: %w", err)
	}
	return assembleWithdrawalOrchestration(db, psparty.NewWithdrawalAuthorizationAdapter(
		pcapplication.NewAdjudicateCommercialAuthorizationHandler(grants),
		// RequestSource 留空：实例半边，见函数注释。
		nil,
	))
}

// buildCustomerWithdrawalOrchestration 装配首方撤回口（`/v1/shipment-requests/withdrawals`）的编排：与上面同一条链，只把撤回授权
// 换成直客 1:1 的渠道绑定授权（ADR-0310 取舍 B：撤回无 PC 裁定，授权 = 账户绑定 + 授予 + 准入范围，已在铸造信封时判过），
// 再在前面套 NewCustomerWithdrawalHandler 补齐并核对委托号与提交版本。
func buildCustomerWithdrawalOrchestration(db *bentopg.DB) (shipmenthttp.WithdrawalHandler, error) {
	inner, err := assembleWithdrawalOrchestration(db, psaccess.DirectCustomerWithdrawalAuthorizer{})
	if err != nil {
		return nil, err
	}
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: shipment requests: %w", err)
	}
	return shipmenthttp.NewCustomerWithdrawalHandler(requests, inner), nil
}

func assembleWithdrawalOrchestration(db *bentopg.DB, authorizer ports.WithdrawalAuthorizer) (shipmenthttp.WithdrawalHandler, error) {
	sources, err := pspostgres.NewSourceSubmissions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: source submissions: %w", err)
	}
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: shipment requests: %w", err)
	}
	judgments, err := pspostgres.NewAcceptanceJudgments(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: acceptance judgments: %w", err)
	}
	freezes, err := sapostgres.NewFreezeLedgers(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: freeze ledgers: %w", err)
	}
	exposures, err := sapostgres.NewCreditExposureLedgers(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: credit exposure ledgers: %w", err)
	}
	scopeLocks, err := sapostgres.NewPreAcceptanceControlScopeLocks(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: pre-acceptance control scope locks: %w", err)
	}
	refusals, err := pspostgres.NewControlReleaseRefusals(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: control release refusals: %w", err)
	}
	identities, err := psidentity.NewAcceptanceDecisions()
	if err != nil {
		return nil, fmt.Errorf("parcel-api: acceptance decision identities: %w", err)
	}
	clock := systemClock{}

	release := pssettlement.NewPreAcceptanceControlAdapter(pssettlement.PreAcceptanceControlAdapterDeps{
		Release: saapplication.NewReleasePreAcceptanceControlHandler(freezes, exposures, scopeLocks, clock),
		// Apply 留空：撤回只消费释放半边，本装配不把施加口交给任何人；
		// Scopes 留空：释放按存下的作用域认领，用不到它，见函数注释。
	})

	handler := shipmentapp.NewWithdrawShipmentRequestHandler(shipmentapp.WithdrawShipmentRequestDeps{
		Sources:    sources,
		Requests:   requests,
		Authorizer: authorizer,
		Judgments:  judgments,
		Recorder:   judgments,
		Release:    pspostgres.NewSavepointedControlRelease(db, release),
		Refusals:   refusals,
		ScopeLock:  pssettlement.NewControlScopeLocks(scopeLocks),
		Identities: identities,
		Clock:      clock,
	})
	return transactionalWithdrawal{transactor: db.Transactor(), inner: handler}, nil
}
