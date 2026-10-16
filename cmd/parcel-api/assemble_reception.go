package main

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	noidentity "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/identity"
	noparcel "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/parcelshipment"
	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	nodeopsapp "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
)

// transactionalReception 把收寄编排包进一笔事务，理由随 transactionalCancellation：收寄
// 库的写口按框架合同无事务即拒（收寄判断落库与同一步的意图发布必须同生共死），事务
// 边界归装配点。意图交付失败不翻结果——编排把它吞成续办引用后照常交回业务答案，事务
// 因此照常提交，重放重发同一份（AT-NO-027 的语义靠的正是「结果已提交而意图没跟上」
// 这个可观察状态）；编排返回错误时整笔回滚，半截写入不落库。
type transactionalReception struct {
	transactor bentoapp.Transactor
	inner      *nodeopsapp.ReceiveDeliveredUnitHandler
}

var _ nodeopshttp.ReceptionHandler = transactionalReception{}

func (reception transactionalReception) Handle(
	ctx context.Context,
	command nodeopsapp.ReceiveDeliveredUnitCommand,
) (nodeopsapp.ReceiveDeliveredUnitResult, error) {
	var result nodeopsapp.ReceiveDeliveredUnitResult
	err := reception.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := reception.inner.Handle(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return nodeopsapp.ReceiveDeliveredUnitResult{}, err
	}
	return result, nil
}

// buildReceptionOrchestration 装配 `/node-operations/receptions` 的真编排（UC-NO-002；
// 接线票 `.scratch/parcel-api-remaining-endpoint-wiring/issues/01`）。
//
// 机制半边全部接真：收寄库、收寄结果版本签发、Outbox 意图交付走 NO 真库口；身份核对读 PS 的外部标识关联册
// （ADR-0184），服务结果读口读 PS 的取消与终局两册（票 operator-channel/25），都与收寄同一笔事务读。
func buildReceptionOrchestration(db *bentopg.DB) (nodeopshttp.ReceptionHandler, error) {
	receptions, err := nopostgres.NewReceptions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: receptions: %w", err)
	}
	versions, err := noidentity.NewIntakeResultVersions()
	if err != nil {
		return nil, fmt.Errorf("parcel-api: intake result versions: %w", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: outbox store: %w", err)
	}
	clock := systemClock{}
	downstream, err := nopostgres.NewOutboxNodeIntakeHandoff(db, store, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: node intake handoff: %w", err)
	}
	identity, err := buildParcelIdentityView(db)
	if err != nil {
		return nil, err
	}
	serviceResults, err := buildParcelServiceResultView(db)
	if err != nil {
		return nil, err
	}
	handler := nodeopsapp.NewReceiveDeliveredUnitHandler(nodeopsapp.ReceiveDeliveredUnitDeps{
		Identity:       identity,
		ServiceResults: serviceResults,
		Receptions:     receptions,
		Versions:       versions,
		Downstream:     downstream,
		Clock:          clock,
	})
	return transactionalReception{transactor: db.Transactor(), inner: handler}, nil
}

// buildParcelIdentityView 装收寄编排的身份核对读口：PS 的外部标识关联册经 NO 的防腐适配器读（ADR-0184 决定五）。
func buildParcelIdentityView(db *bentopg.DB) (noports.ParcelIdentityView, error) {
	identifiers, err := pspostgres.NewExternalIdentifiers(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: external identifiers: %w", err)
	}
	view, err := noparcel.NewParcelIdentityView(identifiers)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: parcel identity view: %w", err)
	}
	return view, nil
}

// buildParcelServiceResultView 装收寄编排的服务结果读口：PS 的取消决定册与终局册经 NO 的防腐适配器读（票 operator-channel/25）。
func buildParcelServiceResultView(db *bentopg.DB) (noports.ParcelServiceResultView, error) {
	cancellations, err := pspostgres.NewParcelCancellations(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: parcel cancellations: %w", err)
	}
	finals, err := pspostgres.NewFinalOutcomes(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: final outcomes: %w", err)
	}
	view, err := noparcel.NewServiceResultView(cancellations, finals)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: parcel service result view: %w", err)
	}
	return view, nil
}
