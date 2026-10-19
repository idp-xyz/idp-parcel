package main

import (
	"context"
	"errors"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfpartycommercial "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/partycommercial"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// 本文件是运输收费发生项登记口的组合根（票 demo-chain-gaps/27，ADR-0207）：`/transport-fulfillment-charge-occurrence-
// registrations` 背后的真编排。缝全接真：揽收尝试与派送尝试两册（失败尝试的事实）、段登记册与实际承运商判断册（实际
// 履约的事实与自营判断）、TF 对 PC 协议版本点读口的消费侧适配器（ADR-0025）、发生项登记册、发生项已形成信封的 Outbox
// 意图、时钟。没有「显式未配置」缝：登记引用的事实都是本上下文自己的，协议在 PC 册上；没登的协议由核对口如实答
// 未在册，不在这里代拟。

// errChargeOccurrenceUndecided 只在本壳里流转：编排答未决时回调交它，逼事务回滚而不提交。未决的几格要么什么都没写，
// 要么写到一半——落了册而意图没交上，提交它就留下一份结算永远等不到的发生项；读口在事务里报了 SQL 错误时整笔事务
// 已置为 aborted，提交只换回提交错误、编排指名的那一格随之丢掉。回滚之后认出它，交回编排那一份答复。
var errChargeOccurrenceUndecided = errors.New("parcel-api: charge occurrence registration undecided; transaction rolled back")

// transactionalChargeOccurrenceRegistrar 把一次登记包进一笔事务：发生项册与 Outbox 的写口按框架合同无事务即拒，事务
// 边界归装配点。发生项与它的已形成信封同笔落地（ADR-0207 决定五）；编排答未决或返回 error 时整笔回滚。
type transactionalChargeOccurrenceRegistrar struct {
	transactor bentoapp.Transactor
	inner      *tfapp.RegisterChargeOccurrenceHandler
}

var _ tfhttp.ChargeOccurrenceRegistrar = transactionalChargeOccurrenceRegistrar{}

func (registrar transactionalChargeOccurrenceRegistrar) Register(
	ctx context.Context,
	command tfapp.RegisterChargeOccurrenceCommand,
) (tfapp.RegisterChargeOccurrenceResult, error) {
	var result tfapp.RegisterChargeOccurrenceResult
	err := registrar.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		registered, registerErr := registrar.inner.Register(txCtx, command)
		if registerErr != nil {
			return registerErr
		}
		result = registered
		if registered.Outcome() == tfapp.ChargeOccurrenceUndecided {
			return errChargeOccurrenceUndecided
		}
		return nil
	})
	if errors.Is(err, errChargeOccurrenceUndecided) {
		return result, nil
	}
	if err != nil {
		return tfapp.RegisterChargeOccurrenceResult{}, err
	}
	return result, nil
}

func buildChargeOccurrenceRegistration(db *bentopg.DB) (tfhttp.ChargeOccurrenceRegistrar, error) {
	deps, err := chargeOccurrenceDeps(db)
	if err != nil {
		return nil, err
	}
	return transactionalChargeOccurrenceRegistrar{
		transactor: db.Transactor(),
		inner:      tfapp.NewRegisterChargeOccurrenceHandler(deps),
	}, nil
}

// chargeOccurrenceDeps 交回登记编排的真依赖。与事务壳分开交出：并发用例要在同一套真依赖外面套一层挂起，才挂得住
// 一笔握着串行锁的登记。
func chargeOccurrenceDeps(db *bentopg.DB) (tfapp.RegisterChargeOccurrenceDeps, error) {
	none := tfapp.RegisterChargeOccurrenceDeps{}
	pickups, err := tfpostgres.NewPickupAttempts(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: pickup attempts for charge occurrences: %w", err)
	}
	deliveries, err := tfpostgres.NewDeliveryAttempts(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: delivery attempts for charge occurrences: %w", err)
	}
	segments, err := tfpostgres.NewFulfillmentSegments(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: fulfillment segments for charge occurrences: %w", err)
	}
	judgments, err := tfpostgres.NewActualCarrierJudgments(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: actual carrier judgments for charge occurrences: %w", err)
	}
	agreementVersions, err := pcpostgres.NewSupplierAgreementContents(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: supplier agreement versions: %w", err)
	}
	agreements, err := tfpartycommercial.NewSupplierAgreementSource(agreementVersions)
	if err != nil {
		return none, fmt.Errorf("parcel-api: supplier agreement check: %w", err)
	}
	occurrences, err := tfpostgres.NewChargeOccurrences(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: charge occurrences: %w", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: outbox store: %w", err)
	}
	clock := systemClock{}
	handoff, err := tfpostgres.NewOutboxChargeOccurrenceHandoff(db, store, clock)
	if err != nil {
		return none, fmt.Errorf("parcel-api: charge occurrence handoff: %w", err)
	}
	return tfapp.RegisterChargeOccurrenceDeps{
		PickupAttempts:   pickups,
		DeliveryAttempts: deliveries,
		Segments:         segments,
		CarrierJudgments: judgments,
		Agreements:       agreements,
		Occurrences:      occurrences,
		Handoff:          handoff,
		Clock:            clock,
	}, nil
}
