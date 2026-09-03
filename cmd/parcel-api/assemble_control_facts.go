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

// 控制事实入口三条编排的装配（票 tf-segment-lifecycle-closure/04）：交接判断登记（含更正）、
// 单对象揽收登记、一次到访多对象揽收执行。三条各包一笔事务，理由随 transactionalDelivery：
// 登记库与段登记册的写口按框架合同无事务即拒，事务边界归装配点；意图交付失败不翻结果——
// 编排把它吞成续办引用后照常交回业务答案，事务照常提交；编排返回 error 时整笔回滚。
//
// **段登记册在这里接真，而且是这三条第一次在生产上接真。** 三条编排的 Deps 都允许 Segments
// 缺席（缺席时交接 / 收寄照登、不立段），这份装配偏不让它缺席：进段那道门
// （enterFulfillmentSegment）在生产上只从这三条编排走得到，装配点若把 Segments 留空，票 04
// 接上的就只是来源保全那一半，CONTEXT 生命周期①②仍然没有任何路走到。

type transactionalHandover struct {
	transactor bentoapp.Transactor
	inner      *tfapp.RegisterTransportHandoverHandler
}

var _ tfhttp.HandoverHandler = transactionalHandover{}

func (handover transactionalHandover) Register(
	ctx context.Context,
	command tfapp.RegisterTransportHandoverCommand,
) (tfapp.RegisterTransportHandoverResult, error) {
	var result tfapp.RegisterTransportHandoverResult
	err := handover.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := handover.inner.Register(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return tfapp.RegisterTransportHandoverResult{}, err
	}
	return result, nil
}

func (handover transactionalHandover) Correct(
	ctx context.Context,
	command tfapp.CorrectTransportHandoverCommand,
) (tfapp.RegisterTransportHandoverResult, error) {
	var result tfapp.RegisterTransportHandoverResult
	err := handover.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := handover.inner.Correct(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return tfapp.RegisterTransportHandoverResult{}, err
	}
	return result, nil
}

type transactionalPickupRegistration struct {
	transactor bentoapp.Transactor
	inner      *tfapp.RegisterOffsitePickupHandler
}

var _ tfhttp.PickupRegistrationHandler = transactionalPickupRegistration{}

func (pickup transactionalPickupRegistration) Register(
	ctx context.Context,
	command tfapp.RegisterOffsitePickupCommand,
) (tfapp.RegisterOffsitePickupResult, error) {
	var result tfapp.RegisterOffsitePickupResult
	err := pickup.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := pickup.inner.Register(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return tfapp.RegisterOffsitePickupResult{}, err
	}
	return result, nil
}

type transactionalPickupAttempt struct {
	transactor bentoapp.Transactor
	inner      *tfapp.PerformOffsitePickupHandler
}

var _ tfhttp.PickupAttemptHandler = transactionalPickupAttempt{}

func (pickup transactionalPickupAttempt) Handle(
	ctx context.Context,
	command tfapp.PerformOffsitePickupCommand,
) (tfapp.PerformOffsitePickupResult, error) {
	var result tfapp.PerformOffsitePickupResult
	err := pickup.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := pickup.inner.Handle(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return tfapp.PerformOffsitePickupResult{}, err
	}
	return result, nil
}

// controlFactOrchestrations 是三条编排一次装配的产物；三个字段分收而不合成一个类型，装配点
// 与装配测试才盖得住「某一口接错了编排」。
type controlFactOrchestrations struct {
	handover           tfhttp.HandoverHandler
	pickupRegistration tfhttp.PickupRegistrationHandler
	pickupAttempt      tfhttp.PickupAttemptHandler
}

// buildControlFactOrchestrations 装配 `/transport-fulfillment/handovers`、
// `/transport-fulfillment/handover-corrections`、`/transport-fulfillment/offsite-pickups` 与
// `/transport-fulfillment/offsite-pickup-attempts` 背后的真编排。
//
// 缝全接真：交接登记册、揽收登记册、揽收尝试库、段登记册、结果版本签发（一只 ResultVersions
// 担揽收与交付两个签发端口，按端口各自注入）、三条 Outbox 意图交付、时钟。本笔没有「显式未
// 配置」缝：控制事实引用的都是本上下文自己的事实，没有等租户参数的实例半边。
func buildControlFactOrchestrations(db *bentopg.DB) (controlFactOrchestrations, error) {
	segments, err := tfpostgres.NewFulfillmentSegments(db)
	if err != nil {
		return controlFactOrchestrations{}, fmt.Errorf("parcel-api: fulfillment segments: %w", err)
	}
	versions, err := tfpostgres.NewResultVersions(db)
	if err != nil {
		return controlFactOrchestrations{}, fmt.Errorf("parcel-api: pickup result versions: %w", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		return controlFactOrchestrations{}, fmt.Errorf("parcel-api: outbox store: %w", err)
	}
	clock := systemClock{}

	handovers, err := tfpostgres.NewTransportHandovers(db)
	if err != nil {
		return controlFactOrchestrations{}, fmt.Errorf("parcel-api: transport handovers: %w", err)
	}
	handoverDownstream, err := tfpostgres.NewOutboxTransportHandoverRegistrationHandoff(db, store, clock)
	if err != nil {
		return controlFactOrchestrations{}, fmt.Errorf("parcel-api: transport handover handoff: %w", err)
	}

	pickups, err := tfpostgres.NewOffsitePickupRegistrations(db)
	if err != nil {
		return controlFactOrchestrations{}, fmt.Errorf("parcel-api: offsite pickup registrations: %w", err)
	}
	pickupRegistrationDownstream, err := tfpostgres.NewOutboxOffsitePickupRegistrationHandoff(db, store, clock)
	if err != nil {
		return controlFactOrchestrations{}, fmt.Errorf("parcel-api: offsite pickup registration handoff: %w", err)
	}

	attempts, err := tfpostgres.NewPickupAttempts(db)
	if err != nil {
		return controlFactOrchestrations{}, fmt.Errorf("parcel-api: pickup attempts: %w", err)
	}
	pickupAttemptDownstream, err := tfpostgres.NewOutboxOffsitePickupHandoff(db, store, clock)
	if err != nil {
		return controlFactOrchestrations{}, fmt.Errorf("parcel-api: offsite pickup handoff: %w", err)
	}

	transactor := db.Transactor()
	return controlFactOrchestrations{
		handover: transactionalHandover{
			transactor: transactor,
			inner: tfapp.NewRegisterTransportHandoverHandler(tfapp.RegisterTransportHandoverDeps{
				Handovers:  handovers,
				Segments:   segments,
				Downstream: handoverDownstream,
				Clock:      clock,
			}),
		},
		pickupRegistration: transactionalPickupRegistration{
			transactor: transactor,
			inner: tfapp.NewRegisterOffsitePickupHandler(tfapp.RegisterOffsitePickupDeps{
				Pickups:    pickups,
				Segments:   segments,
				Versions:   versions,
				Downstream: pickupRegistrationDownstream,
				Clock:      clock,
			}),
		},
		pickupAttempt: transactionalPickupAttempt{
			transactor: transactor,
			inner: tfapp.NewPerformOffsitePickupHandler(tfapp.PerformOffsitePickupDeps{
				Attempts:   attempts,
				Segments:   segments,
				Versions:   versions,
				Downstream: pickupAttemptDownstream,
				Clock:      clock,
			}),
		},
	}, nil
}
