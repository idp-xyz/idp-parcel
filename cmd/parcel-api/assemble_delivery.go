package main

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	tfcustoms "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/customscompliance"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// transactionalDelivery 把交付编排的两个入口各包进一笔事务，理由随 transactionalCancellation：
// 交付登记库的写口（Save 与 Supersede）按框架合同无事务即拒，事务边界归装配点。意图
// 交付失败不翻结果——编排把它吞成续办引用后照常交回业务答案，事务照常提交，重放重发
// 同一份；编排返回错误时整笔回滚。一个类型顶两个端点：首登与更正共用 DeliveryHandler，
// 与 unwiredDelivery 同一个理由。
//
// 首登先在写事务之外问交付门禁读数，读数随交付同一笔落册（ADR-0213 决定五）；同内容重放不重问，取在册那一版的读数
// （RecordedDeliveryGateReading 说明为什么）。读数不进写事务的理由同 transactionalHandover：关务的判断在它自己的事务里
// 记下，嵌进来即拒；读段册、交接册在写事务里出一次 SQL 级错误，整笔进中止态，读不动反而挡住了交付落册。更正不问：
// POD 更正照前版承继读数。
type transactionalDelivery struct {
	transactor bentoapp.Transactor
	gate       *tfapp.DeliveryGateReader
	inner      *tfapp.RegisterEffectiveDeliveryHandler
}

var _ tfhttp.DeliveryHandler = transactionalDelivery{}

func (delivery transactionalDelivery) Register(
	ctx context.Context,
	command tfapp.RegisterEffectiveDeliveryCommand,
) (tfapp.RegisterEffectiveDeliveryResult, error) {
	reading, replayed := delivery.inner.RecordedDeliveryGateReading(ctx, command)
	if !replayed {
		var err error
		if reading, err = delivery.gate.ReadRegistration(ctx, command); err != nil {
			return tfapp.RegisterEffectiveDeliveryResult{}, err
		}
	}
	var result tfapp.RegisterEffectiveDeliveryResult
	err := delivery.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := delivery.inner.RegisterWithGateReading(txCtx, command, reading)
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
// 都是本上下文自己的事实，没有等租户参数的实例半边。交付门禁读数那一侧另装（buildDeliveryGateReader）。
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
	gate, err := buildDeliveryGateReader(db, attempts)
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
	return transactionalDelivery{transactor: db.Transactor(), gate: gate, inner: handler}, nil
}

// buildDeliveryGateReader 装交付门禁读数那一侧：派送尝试读面（交付的业务时间）、段登记册与交接登记册（at 取哪个节点），
// 以及关务动作准入判断的消费侧适配器。关务那一口另装一份，理由同 buildHandoverGateReader。另成一个函数，让与生产装配同形的
// 测试装配取的是同一份。
func buildDeliveryGateReader(db *bentopg.DB, attempts *tfpostgres.DeliveryAttempts) (*tfapp.DeliveryGateReader, error) {
	segments, err := tfpostgres.NewFulfillmentSegments(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: delivery gate: fulfillment segments: %w", err)
	}
	handovers, err := tfpostgres.NewTransportHandovers(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: delivery gate: transport handovers: %w", err)
	}
	customsAdmission, err := buildCustomsAdmissionOrchestration(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: delivery gate: %w", err)
	}
	customs, err := tfcustoms.NewDeliveryGateView(customsAdmission.actionAdmission)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: delivery gate: %w", err)
	}
	gate, err := tfapp.NewDeliveryGateReader(tfapp.DeliveryGateReadDeps{
		Attempts:  attempts,
		Segments:  segments,
		Handovers: handovers,
		Customs:   customs,
	})
	if err != nil {
		return nil, fmt.Errorf("parcel-api: delivery gate: %w", err)
	}
	return gate, nil
}

// transactionalDeliveryAttempt 把派送尝试登记包进一笔事务：尝试父行与逐对象结果子行要一起成立，写口按框架合同
// 无事务即拒，事务边界归装配点（同 transactionalDelivery）。编排返回错误时整笔回滚。
type transactionalDeliveryAttempt struct {
	transactor bentoapp.Transactor
	inner      *tfapp.RecordDeliveryAttemptHandler
}

var _ tfhttp.DeliveryAttemptHandler = transactionalDeliveryAttempt{}

func (attempts transactionalDeliveryAttempt) Handle(
	ctx context.Context,
	command tfapp.RecordDeliveryAttemptCommand,
) (tfapp.RecordDeliveryAttemptResult, error) {
	var result tfapp.RecordDeliveryAttemptResult
	err := attempts.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := attempts.inner.Handle(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return tfapp.RecordDeliveryAttemptResult{}, err
	}
	return result, nil
}

// buildDeliveryAttemptOrchestration 装配 `/transport-fulfillment/delivery-attempts` 背后的真编排（票
// product-strategy-boundary/19）。三条缝全接真：派送尝试登记册（与交付生效读的是同一张表）、揽派任务登记册的只读口、
// 时钟。没有「显式未配置」缝，也不交发布意图：登记派送尝试只保全到场事实，由它形成运输收费发生项是另一步。
func buildDeliveryAttemptOrchestration(db *bentopg.DB) (tfhttp.DeliveryAttemptHandler, error) {
	attempts, err := tfpostgres.NewDeliveryAttempts(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: delivery attempts: %w", err)
	}
	tasks, err := tfpostgres.NewDispatchTasks(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: dispatch tasks: %w", err)
	}
	handler := tfapp.NewRecordDeliveryAttemptHandler(tfapp.RecordDeliveryAttemptDeps{
		Attempts: attempts,
		Tasks:    tasks,
		Clock:    systemClock{},
	})
	return transactionalDeliveryAttempt{transactor: db.Transactor(), inner: handler}, nil
}

// buildParticipationEnder 装配结束参与那条编排，供交付与交接两条来源编排在各自事务里同步调用（票
// tf-segment-lifecycle-closure/06 裁决 (i)）。它不包事务：调用它的编排已经在事务里，同一 ctx 带着同一笔。
// 缝全接真——段登记册（按对象找段的读口也在这只上）、实际承运商判断登记册（「进下一段」立新段时
// 同笔铸首版，理由见 assemble_control_facts.go 顶部）、交接登记册、交付登记库、时钟。判断登记册在
// 这里自己再构造一只而不从调用方传入：适配器无状态，三个装配点各自持有，比把它穿过签名更少缝。
func buildParticipationEnder(db *bentopg.DB, clock systemClock) (*tfapp.EndFulfillmentParticipationHandler, error) {
	segments, err := tfpostgres.NewFulfillmentSegments(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: fulfillment segments: %w", err)
	}
	judgments, err := tfpostgres.NewActualCarrierJudgments(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: actual carrier judgments: %w", err)
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
		Judgments:  judgments,
		Handovers:  handovers,
		Deliveries: deliveries,
		Clock:      clock,
	}), nil
}
