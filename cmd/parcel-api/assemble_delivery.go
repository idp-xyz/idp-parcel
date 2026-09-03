package main

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// transactionalDelivery 把交付编排的两个入口各包进一笔事务，理由随 transactionalCancellation：
// 交付登记库的写口（Save 与 Supersede）按框架合同无事务即拒，事务边界归装配点。意图
// 交付失败不翻结果——编排把它吞成续办引用后照常交回业务答案，事务照常提交，重放重发
// 同一份；编排返回错误时整笔回滚。一个类型顶两个端点：首登与更正共用 DeliveryHandler，
// 与 unwiredDelivery 同一个理由。
type transactionalDelivery struct {
	transactor bentoapp.Transactor
	inner      *tfapp.RegisterEffectiveDeliveryHandler
}

var _ tfhttp.DeliveryHandler = transactionalDelivery{}

func (delivery transactionalDelivery) Register(
	ctx context.Context,
	command tfapp.RegisterEffectiveDeliveryCommand,
) (tfapp.RegisterEffectiveDeliveryResult, error) {
	var result tfapp.RegisterEffectiveDeliveryResult
	err := delivery.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := delivery.inner.Register(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return tfapp.RegisterEffectiveDeliveryResult{}, err
	}
	return result, nil
}

func (delivery transactionalDelivery) Correct(
	ctx context.Context,
	command tfapp.CorrectDeliveryProofCommand,
) (tfapp.RegisterEffectiveDeliveryResult, error) {
	var result tfapp.RegisterEffectiveDeliveryResult
	err := delivery.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := delivery.inner.Correct(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return tfapp.RegisterEffectiveDeliveryResult{}, err
	}
	return result, nil
}

// buildDeliveryOrchestration 装配 `/transport-fulfillment/deliveries` 与
// `/transport-fulfillment/delivery-proof-corrections` 共用的真编排（接线票
// `.scratch/parcel-api-remaining-endpoint-wiring/issues/02`）。
//
// 四条缝全接真：派送尝试读面（只读，尝试由执行侧登记——交付入口不造尝试）、交付
// 登记库、结果版本签发、Outbox 意图交付。本笔没有「显式未配置」缝：交付生效引用的
// 都是本上下文自己的事实，没有等租户参数的实例半边。
func buildDeliveryOrchestration(db *bentopg.DB) (tfhttp.DeliveryHandler, error) {
	attempts, err := tfpostgres.NewDeliveryAttempts(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: delivery attempts: %w", err)
	}
	deliveries, err := tfpostgres.NewEffectiveDeliveries(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: effective deliveries: %w", err)
	}
	versions, err := tfpostgres.NewResultVersions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: delivery result versions: %w", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: outbox store: %w", err)
	}
	clock := systemClock{}
	downstream, err := tfpostgres.NewOutboxEffectiveDeliveryHandoff(db, store, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: effective delivery handoff: %w", err)
	}
	participationEnds, err := buildParticipationEnder(db, clock)
	if err != nil {
		return nil, err
	}
	handler := tfapp.NewRegisterEffectiveDeliveryHandler(tfapp.RegisterEffectiveDeliveryDeps{
		Attempts:          attempts,
		Deliveries:        deliveries,
		Versions:          versions,
		Downstream:        downstream,
		Clock:             clock,
		ParticipationEnds: participationEnds,
	})
	return transactionalDelivery{transactor: db.Transactor(), inner: handler}, nil
}

// buildParticipationEnder 装配结束参与那条编排，供交付与交接两条来源编排在各自事务里同步调用（票
// tf-segment-lifecycle-closure/06 裁决 (i)）。它不包事务：调用它的编排已经在事务里，同一 ctx 带着同一笔。
// 缝全接真——段登记册（按对象找段的读口也在这只上）、交接登记册、交付登记库、时钟。
func buildParticipationEnder(db *bentopg.DB, clock systemClock) (*tfapp.EndFulfillmentParticipationHandler, error) {
	segments, err := tfpostgres.NewFulfillmentSegments(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: fulfillment segments: %w", err)
	}
	handovers, err := tfpostgres.NewTransportHandovers(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: transport handovers: %w", err)
	}
	deliveries, err := tfpostgres.NewEffectiveDeliveries(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: effective deliveries: %w", err)
	}
	return tfapp.NewEndFulfillmentParticipationHandler(tfapp.EndFulfillmentParticipationDeps{
		Segments:   segments,
		Handovers:  handovers,
		Deliveries: deliveries,
		Clock:      clock,
	}), nil
}
