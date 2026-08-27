package main

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	psidentity "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/identity"
	psparty "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	pssettlement "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/settlementaccounting"
	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
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

// buildWithdrawalOrchestration 装配 `/shipment-requests/withdrawals` 的真编排（UC-PS-005）。
//
// 机制半边全部接真：来源保全、委托仓储、判断读写与决定标识走 PS 真库口，撤回授权经
// partycommercial 适配器接 PC 裁定编排与真授权册，冻结释放经 settlementaccounting 适配器
// 接 SA 释放编排与真两账本。实例半边照旧留空，各自的「显式未配置」形状各归各口：
//
//   - 撤回授权的请求映射（法人/权限等级/商业范围/时点，`PAR-COM-14`）nil——适配器答
//     未形成，编排如实停在`撤回授权不可用`，不代拟坐标也不冒充`授权规则未配置`；
//   - 释放的作用域与金额映射（账户/估价缝）nil——今天判断册上不存在`已冻结`结果，释放
//     根本不会被发起；真有旧冻结要释放时它答未配置留补偿续办，不凭空认领别人的钱。
//
// 接线前后的区别不在结果在来源：恢复动作从「写代码」变成「登记参数」（ADR-0063）。
func buildWithdrawalOrchestration(db *bentopg.DB) (shipmenthttp.WithdrawalHandler, error) {
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
	grants, err := pcpostgres.NewAuthorityGrants(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: authority grants: %w", err)
	}
	freezes, err := sapostgres.NewFreezeLedgers(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: freeze ledgers: %w", err)
	}
	exposures, err := sapostgres.NewCreditExposureLedgers(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: credit exposure ledgers: %w", err)
	}
	identities, err := psidentity.NewAcceptanceDecisions()
	if err != nil {
		return nil, fmt.Errorf("parcel-api: acceptance decision identities: %w", err)
	}
	clock := systemClock{}

	authorizer := psparty.NewWithdrawalAuthorizationAdapter(
		pcapplication.NewAdjudicateCommercialAuthorizationHandler(grants),
		// RequestSource 留空：实例半边，见函数注释。
		nil,
	)
	release := pssettlement.NewPreAcceptanceControlAdapter(pssettlement.PreAcceptanceControlAdapterDeps{
		Release: saapplication.NewReleasePreAcceptanceControlHandler(freezes, exposures, clock),
		// Apply 留空：撤回只消费释放半边，本装配不把施加口交给任何人；
		// Scopes/Amounts 留空：实例半边，见函数注释。
	})

	handler := shipmentapp.NewWithdrawShipmentRequestHandler(shipmentapp.WithdrawShipmentRequestDeps{
		Sources:    sources,
		Requests:   requests,
		Authorizer: authorizer,
		Judgments:  judgments,
		Recorder:   judgments,
		Release:    release,
		Identities: identities,
		Clock:      clock,
	})
	return transactionalWithdrawal{transactor: db.Transactor(), inner: handler}, nil
}
