package main

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	tfcustoms "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/customscompliance"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// transactionalMovementFact 先在写事务之外问出发门禁读数，再把移动事实登记包进一笔事务，读数随事实同一笔落册（ADR-0205）。
// 同内容重放的出发不重问关务，取在册那一版的读数（RecordedDepartureReading 说明为什么）；那一问也在池上。
//
// 读数不进写事务：关务的动作准入判断在它自己的事务里记下，嵌进来即拒（transactionalActionAdmissionJudge）；读段与班次也在
// 池上读——跨上下文读在写事务里出一次 SQL 级错误，整笔就进了中止态，读不动于是挡住了落册，而已经发生的出发必须照记。
// 写那一半的理由随 transactionalDelivery：登记册的写口按框架合同无事务即拒，事务边界归装配点；编排返回 error 时整笔回滚。
type transactionalMovementFact struct {
	transactor bentoapp.Transactor
	gate       *tfapp.DepartureGateReader
	inner      *tfapp.RecordMovementFactHandler
}

var _ tfhttp.MovementFactHandler = transactionalMovementFact{}

func (movement transactionalMovementFact) Record(
	ctx context.Context,
	command tfapp.RecordMovementFactCommand,
) (tfapp.RecordMovementFactResult, error) {
	reading, replayed := movement.inner.RecordedDepartureReading(ctx, command)
	if !replayed {
		var err error
		if reading, err = movement.gate.Read(ctx, command); err != nil {
			return tfapp.RecordMovementFactResult{}, err
		}
	}
	var result tfapp.RecordMovementFactResult
	err := movement.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := movement.inner.Record(txCtx, command, reading)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return tfapp.RecordMovementFactResult{}, err
	}
	return result, nil
}

// buildMovementFactOrchestration 装配 `/transport-fulfillment/movement-facts` 背后的真编排（票 tf-segment-lifecycle-closure/05、
// demo-chain-gaps/32）。登记那一半的缝是移动事实登记册与时钟；出发读数那一半问本上下文的段册与班次册，再经消费侧适配器问
// 关务的动作准入判断——全接真。仍不向任何下游交意图：移动事实信封与它的消费者同一个变更上线（ADR-0205 决定五）。
//
// 关务那一口在这里另装一份，不从 main 的 customsAdmission 传进来：两份接的是同一批真适配器、同一个库，无状态；传进来就要
// 改 main 里既有几行的次序。
func buildMovementFactOrchestration(db *bentopg.DB) (tfhttp.MovementFactHandler, error) {
	facts, err := tfpostgres.NewMovementFacts(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: movement facts: %w", err)
	}
	segments, err := tfpostgres.NewFulfillmentSegments(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: movement facts: fulfillment segments: %w", err)
	}
	schedules, err := tfpostgres.NewTransportSchedules(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: movement facts: transport schedules: %w", err)
	}
	customsAdmission, err := buildCustomsAdmissionOrchestration(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: movement facts: %w", err)
	}
	customs, err := tfcustoms.NewDepartureGateView(customsAdmission.actionAdmission)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: movement facts: %w", err)
	}
	gate, err := tfapp.NewDepartureGateReader(tfapp.DepartureGateReadDeps{
		Segments:  segments,
		Schedules: schedules,
		Customs:   customs,
	})
	if err != nil {
		return nil, fmt.Errorf("parcel-api: movement facts: %w", err)
	}
	handler := tfapp.NewRecordMovementFactHandler(tfapp.RecordMovementFactDeps{
		Facts: facts,
		Clock: systemClock{},
	})
	return transactionalMovementFact{transactor: db.Transactor(), gate: gate, inner: handler}, nil
}
