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
)

// transactionalCancellation 把取消编排包进一笔事务，理由随 transactionalSubmission：
// 取消库的写口按框架合同无事务即拒，事务边界归装配点。编排交回业务答案（含未决、
// 拒绝、待处置与已有结果）时事务提交；发布意图交不出不翻决定——编排吞成重发引用后
// 照常作答，重放重发同一份；编排返回错误时整笔回滚。
type transactionalCancellation struct {
	transactor bentoapp.Transactor
	inner      *shipmentapp.CancelParcelHandler
}

var _ shipmenthttp.CancellationHandler = transactionalCancellation{}

func (cancellation transactionalCancellation) Handle(
	ctx context.Context,
	command shipmentapp.CancelParcelCommand,
) (shipmentapp.CancelParcelResult, error) {
	var result shipmentapp.CancelParcelResult
	err := cancellation.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := cancellation.inner.Handle(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return shipmentapp.CancelParcelResult{}, err
	}
	return result, nil
}

// buildCancellationOrchestration 装配 `/shipment-requests/parcel-cancellations` 的
// 真编排（UC-PS-006 接受后取消；传输层随 391f30a 落库，本笔只做组合根挂载）。
//
// 七条缝六真一「诚实未配置」：委托仓储、收寄采认读口、取消决定库、PCXL 取消标识
// 签发、发布意图的 Outbox 交付与生产时钟接真——全是本上下文自己的机制半边。
//
// 授权缝走 ServiceStageRulesAdapter + DeclaredStageContent 整条真翻译链，未配置的
// 位置在链的最里端：AdoptedStageOwner 缺席（SourceIdentity 尚未固定采用哪个授权规则
// 版本，PAR-COM-17 实例半边），DeclaredStageContent 据此答「目录未配置」，适配器
// 原样交出 found=false，编排停在 AUTHORITY_UNCONFIGURED——恢复动作是租户登记授权
// 规则，不是修授权服务（AUTHORITY_UNAVAILABLE 那格），更不是默认任何角色可取消或
// 不可取消（UC-PS-006 明禁两个方向的默认）。三个 PC 内容读口与请求方映射一并留空：
// 采用版本未配置时链在它们之前就停了，填上任何一个都改变不了答案的来源。
func buildCancellationOrchestration(db *bentopg.DB) (shipmenthttp.CancellationHandler, error) {
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: shipment requests: %w", err)
	}
	adoptions, err := pspostgres.NewIntakeAdoptions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: intake adoptions: %w", err)
	}
	cancellations, err := pspostgres.NewParcelCancellations(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: parcel cancellations: %w", err)
	}
	identities, err := psidentity.NewParcelCancellations()
	if err != nil {
		return nil, fmt.Errorf("parcel-api: cancellation identities: %w", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: outbox store: %w", err)
	}
	clock := systemClock{}
	downstream, err := pspostgres.NewOutboxParcelCancellationHandoff(db, store, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: parcel cancellation handoff: %w", err)
	}

	// 采用版本源传 nil 即 UnconfiguredAdoptedStageOwner：见函数注释「授权缝」段。
	stageContent := psparty.NewDeclaredStageContent(nil, nil, nil, nil)
	authority := psparty.NewServiceStageRulesAdapter(stageContent, stageContent, stageContent, nil, nil)

	handler := shipmentapp.NewCancelParcelHandler(shipmentapp.CancelParcelDeps{
		Requests:   requests,
		Authority:  authority,
		Adoptions:  adoptions,
		Store:      cancellations,
		Identities: identities,
		Downstream: downstream,
		Clock:      clock,
	})
	return transactionalCancellation{transactor: db.Transactor(), inner: handler}, nil
}
