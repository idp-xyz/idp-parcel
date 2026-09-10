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
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
)

// transactionalDisposition 把授权处置编排包进一笔事务，理由随 transactionalRejection：PS 的写口无事务
// 即拒，事务边界归装配点；处置落库与随附的占用释放同一事务，释放失败交回补偿续办而不回滚处置
// （编排自己分的格）。两个去向都在这一笔里收口，不发续办信封（ADR-0132 决定二）：`拒绝`当场形成
// 决定，`交客户补充`转到`等待受控补充`、之后由既有的「新提交版本已形成」信封续办。
type transactionalDisposition struct {
	transactor bentoapp.Transactor
	inner      *shipmentapp.DisposeShipmentRequestHandler
}

var _ shipmenthttp.AuthorizedDispositionHandler = transactionalDisposition{}

func (disposition transactionalDisposition) Handle(
	ctx context.Context,
	command shipmentapp.DisposeShipmentRequestCommand,
) (shipmentapp.DisposeShipmentRequestResult, error) {
	var result shipmentapp.DisposeShipmentRequestResult
	err := disposition.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := disposition.inner.Handle(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return shipmentapp.DisposeShipmentRequestResult{}, err
	}
	return result, nil
}

// buildDispositionOrchestration 装配 `/shipment-requests/authorized-dispositions` 的真编排
// （票 sa-preacceptance-policy-view/04；ADR-0132）。
//
// 机制半边接真：委托仓储、判断读口与决定标识走 PS 真库口，占用释放经 settlementaccounting 适配器接 SA
// 释放编排与真两账本。处置授权那一格如实是**未配置**：party-commercial 的授权动作词汇今天没有「授权处置」
// 一格（ADR-0132 越权风险点 2，归 PC 另票），一份处置授权规则登记不出来，翻译适配器的动作守卫也无物可比
// 对——装配 UnconfiguredAuthorizedDispositionAuthorizer，编排对每一次处置答`授权规则未配置`、什么也不落库，
// 不判处置人越权也不放行。PC 那一格立起之后在这里换成翻译适配器，其余不动。释放的作用域与金额映射
// （账户/估价缝）照主动拒绝那一路留 nil：真有占用要释放时它答未配置留补偿续办，不凭空认领别人的钱。
func buildDispositionOrchestration(db *bentopg.DB) (shipmenthttp.AuthorizedDispositionHandler, error) {
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
	identities, err := psidentity.NewAcceptanceDecisions()
	if err != nil {
		return nil, fmt.Errorf("parcel-api: acceptance decision identities: %w", err)
	}
	clock := systemClock{}

	release := pssettlement.NewPreAcceptanceControlAdapter(pssettlement.PreAcceptanceControlAdapterDeps{
		Release: saapplication.NewReleasePreAcceptanceControlHandler(freezes, exposures, clock),
		// Apply 留空：处置只消费释放半边；Scopes/Amounts 留空：实例半边，见函数注释。
	})

	handler := shipmentapp.NewDisposeShipmentRequestHandler(shipmentapp.DisposeShipmentRequestDeps{
		Requests:   requests,
		Authorizer: psparty.UnconfiguredAuthorizedDispositionAuthorizer{},
		Judgments:  judgments,
		Release:    release,
		Identities: identities,
		Clock:      clock,
	})
	return transactionalDisposition{transactor: db.Transactor(), inner: handler}, nil
}
