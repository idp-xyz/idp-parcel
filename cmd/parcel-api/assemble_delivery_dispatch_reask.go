package main

import (
	"context"
	"fmt"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// buildDeliveryDispatchReask 装配 `/transport-fulfillment-delivery-dispatch-reasks` 背后的按册重问（ADR-0230 决定四、五）。执行器
// 就是补拍口那一只（buildDeliveryDispatchTrigger）：同一组派送要求缝、同一本派送待形成册，每问一件自成一笔事务——一件写不动只回滚那
// 一件，已问过的各件留在册上，再按一次接着问。列册与读重问间隔不在事务里：它们只读，读到的是此刻已提交的那几件。
func buildDeliveryDispatchReask(db *bentopg.DB) (*tfapp.ReaskPendingDeliveryDispatchesHandler, error) {
	trigger, err := buildDeliveryDispatchTrigger(db)
	if err != nil {
		return nil, err
	}
	return deliveryDispatchReaskOver(db, trigger)
}

// deliveryDispatchReaskOver 把派送待形成册与重问间隔册接到给定的执行器上。执行器之外的接线只有这一处：生产装配交补拍口那一只，
// 真库回归交接了替身缝的那一只。
func deliveryDispatchReaskOver(db *bentopg.DB, trigger tfapp.DeliveryDispatchTrigger) (*tfapp.ReaskPendingDeliveryDispatchesHandler, error) {
	pendings, err := tfpostgres.NewDeliveryDispatchPendings(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: delivery dispatch pendings: %w", err)
	}
	cadences, err := tfpostgres.NewDeliveryDispatchReaskCadences(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: delivery dispatch reask cadences: %w", err)
	}
	asks, err := tfpostgres.NewDeliveryDispatchAsks(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: delivery dispatch asks: %w", err)
	}
	return tfapp.NewReaskPendingDeliveryDispatchesHandler(tfapp.ReaskPendingDeliveryDispatchesDeps{
		Asks:     transactionalAskLedger{transactor: db.Transactor(), asks: asks},
		Pending:  pendings,
		Cadences: cadences,
		Trigger:  trigger,
		Failed:   transactionalReaskFailureMarker{transactor: db.Transactor(), pendings: pendings},
		Clock:    systemClock{},
	}), nil
}

// transactionalReaskFailureMarker 给执行器给不出答复的那一件另起一笔事务记下「这一次问过」：那一拍自己的事务已整笔回滚，这一笔
// 与它无关，提交了那一件就排到队尾。
type transactionalReaskFailureMarker struct {
	transactor bentoapp.Transactor
	pendings   *tfpostgres.DeliveryDispatchPendings
}

var _ tfapp.ReaskFailureMarker = transactionalReaskFailureMarker{}

func (marker transactionalReaskFailureMarker) MarkAsked(ctx context.Context, key tfports.DeliveryDispatchPendingKey, at time.Time) error {
	return marker.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return marker.pendings.MarkAsked(txCtx, key, at)
	})
}

// transactionalAskLedger 给按册重问记按下的那一次另起一笔事务：先于逐件去问，提交了才开始问，一件问不动也不让这一次按下无处可查
// （ADR-0303 决定五与复核记录）。补拍口不用它：补拍的记录随执行器那一拍同笔。
type transactionalAskLedger struct {
	transactor bentoapp.Transactor
	asks       *tfpostgres.DeliveryDispatchAsks
}

var _ tfports.DeliveryDispatchAskLedger = transactionalAskLedger{}

func (ledger transactionalAskLedger) RecordAsk(ctx context.Context, record tfports.DeliveryDispatchAskRecord) error {
	return ledger.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return ledger.asks.RecordAsk(txCtx, record)
	})
}

func (ledger transactionalAskLedger) ListAsks(ctx context.Context, tenant tfdomain.TenantID, limit int) ([]tfports.DeliveryDispatchAskRecord, error) {
	return ledger.asks.ListAsks(ctx, tenant, limit)
}

// transactionalReaskCadenceRegistrar 把每次重问间隔登记包进一笔事务，理由随 transactionalPreAdviceWindowRegistrar：登记册的写口按
// 框架合同无事务即拒，事务边界归装配点。
type transactionalReaskCadenceRegistrar struct {
	transactor bentoapp.Transactor
	inner      *tfapp.RegisterReaskCadenceHandler
}

var _ tfhttp.ReaskCadenceRegistrar = transactionalReaskCadenceRegistrar{}

func (registrar transactionalReaskCadenceRegistrar) Register(
	ctx context.Context,
	command tfapp.RegisterReaskCadenceCommand,
) (tfapp.RegisterReaskCadenceResult, error) {
	var result tfapp.RegisterReaskCadenceResult
	err := registrar.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := registrar.inner.Register(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return tfapp.RegisterReaskCadenceResult{}, err
	}
	return result, nil
}

// buildReaskCadenceRegistration 装配 `/transport-fulfillment-delivery-dispatch-reask-cadence-registrations` 背后的登记编排（ADR-0230
// 决定五）。间隔是租户取值：产品不种任何一行，没登过的租户到期重问答未配置。
func buildReaskCadenceRegistration(db *bentopg.DB) (transactionalReaskCadenceRegistrar, error) {
	cadences, err := tfpostgres.NewDeliveryDispatchReaskCadences(db)
	if err != nil {
		return transactionalReaskCadenceRegistrar{}, fmt.Errorf("parcel-api: delivery dispatch reask cadences: %w", err)
	}
	return transactionalReaskCadenceRegistrar{
		transactor: db.Transactor(),
		inner:      tfapp.NewRegisterReaskCadenceHandler(tfapp.RegisterReaskCadenceDeps{Cadences: cadences, Clock: systemClock{}}),
	}, nil
}
