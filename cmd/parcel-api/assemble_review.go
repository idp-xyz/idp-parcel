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
	pssettlement "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/settlementaccounting"
	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	pcapplication "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
)

// reviewCompletionBoundary 是 ADR-0086 Decision 二的事务边界壳：复核完成落库与「复核
// 已完成」信封在同一个事务里成立或一起消失，形状照 submissionBoundary + Outbox 交接
// 先例。完成落了库而信封没入队，停等复核的委托就再也没有投递来续办——那正是这层壳
// 要焊死的缝。
//
// 交接挂在 Save 而不是包整段 Handle：复核完成编排只在`已记录`那一格走到 Save（重复
// 完成、任务完结、版本换代都在保存之前交回），Save 即完成落库。挂错地方有护栏：交接
// 适配器对不带完成留痕的聚合响亮报错，整笔回滚。
type reviewCompletionBoundary struct {
	transactor bentoapp.Transactor
	inner      ports.ShipmentRequestRepository
	handoff    ports.ManualReviewCompletedHandoff
}

var _ ports.ShipmentRequestRepository = reviewCompletionBoundary{}

func (boundary reviewCompletionBoundary) FindBySourceIdentity(
	ctx context.Context,
	identity domain.SourceIdentity,
) (domain.ShipmentRequest, bool, error) {
	return boundary.inner.FindBySourceIdentity(ctx, identity)
}

// Insert 只切事务、不发信封：复核完成编排走不到这一口，它在这里是因为端口带着它——
// 同规格切事务，谁日后复用这个壳都不会撞 RequireExecutor。
func (boundary reviewCompletionBoundary) Insert(
	ctx context.Context,
	identity domain.SourceIdentity,
	request domain.ShipmentRequest,
) (ports.ShipmentRequestInsertOutcome, error) {
	var outcome ports.ShipmentRequestInsertOutcome
	err := boundary.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		inserted, err := boundary.inner.Insert(txCtx, identity, request)
		if err != nil {
			return err
		}
		outcome = inserted
		return nil
	})
	if err != nil {
		return ports.ShipmentRequestInsertOutcomeInvalid, err
	}
	return outcome, nil
}

func (boundary reviewCompletionBoundary) Save(
	ctx context.Context,
	identity domain.SourceIdentity,
	request domain.ShipmentRequest,
) (ports.ShipmentRequestSaveOutcome, error) {
	var outcome ports.ShipmentRequestSaveOutcome
	err := boundary.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		saved, err := boundary.inner.Save(txCtx, identity, request)
		if err != nil {
			return err
		}
		outcome = saved
		if saved != ports.ShipmentRequestSaved {
			// 版本冲突是业务答案：本事务没写下任何东西，不发信封，交回编排按 ADR-0031
			// 重读重放。
			return nil
		}
		return boundary.handoff.HandOffManualReviewCompleted(txCtx, ports.ManualReviewCompletedHandoffIntent{
			Identity: identity,
			Request:  request,
		})
	})
	if err != nil {
		return ports.ShipmentRequestSaveOutcomeInvalid, err
	}
	return outcome, nil
}

// buildManualReviewOrchestration 装配 `/shipment-requests/manual-review-completions`
// 的真编排（票 09；ADR-0086）。
//
// 编排只记录完成不驱链（CompleteManualReviewHandler 的注释记着理由）；续办由信封驱动：
// reviewCompletionBoundary 携 OutboxManualReviewCompletedHandoff，事件类型
// `parcel-shipment.shipment-request.manual-review-completed`，消费门在 cmd/parcel-dispatch
// （同一条接受判断链的第二扇门）。
//
// 「谁在复核」与「谁有权复核」两问的权威不同源，各归各口：前者是接入身份，由 Intake 半边
// （PAR-INT-01 实例参数）核验后随命令交来复核人与证据引用；后者是商业授权，编排拿到委托就去问
// party-commercial（UC-PC-003 带 ManualReviewAction，票 wiring-baseline-remainder/04），授权引用
// 由那边签发进复核留痕——与主动拒绝同一条路。此前授权引用也由 Intake 整组注入，等于采信自报。
//
// 复核授权的请求映射（法人/权限等级/商业范围/结构化原因/时点，`PAR-COM-14`）是实例半边，生产
// 装配留 nil——适配器答未形成，编排如实停在 error（HTTP 5xx NO_ANSWER_FORMED），不代拟坐标也不
// 冒充`授权规则未配置`；与拒绝授权那只适配器的处置同款。
func buildManualReviewOrchestration(db *bentopg.DB) (shipmenthttp.ManualReviewCompletionHandler, error) {
	return manualReviewOrchestrationWith(db, nil)
}

// manualReviewOrchestrationWith 是 buildManualReviewOrchestration 的全部实现，请求映射作入参：
// 装配测试要在真授权册上证「已授权时留痕里的授权引用是 PC 签发的那一版」，只能从这一格把合成
// 映射递进来——映射之外的每一件（委托仓储、边界壳、Outbox 交接、PC 裁定编排与真授权册）都是
// 生产实现。生产入口只传 nil。
func manualReviewOrchestrationWith(
	db *bentopg.DB,
	reviewRequests psparty.ManualReviewAuthorizationRequestSource,
) (shipmenthttp.ManualReviewCompletionHandler, error) {
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: shipment requests: %w", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: outbox store: %w", err)
	}
	grants, err := pcpostgres.NewAuthorityGrants(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: authority grants: %w", err)
	}
	clock := systemClock{}
	handoff, err := pspostgres.NewOutboxManualReviewCompletedHandoff(db, store, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: manual review completed handoff: %w", err)
	}
	return shipmentapp.NewCompleteManualReviewHandler(shipmentapp.CompleteManualReviewDeps{
		Requests: reviewCompletionBoundary{
			transactor: db.Transactor(),
			inner:      requests,
			handoff:    handoff,
		},
		Authorizer: psparty.NewManualReviewAuthorizationAdapter(
			pcapplication.NewAdjudicateCommercialAuthorizationHandler(grants),
			reviewRequests,
		),
		Clock: clock,
	}), nil
}

// transactionalRejection 把主动拒绝编排包进一笔事务，理由随 transactionalWithdrawal：
// PS 的写口无事务即拒，事务边界归装配点；拒绝越过提交边界后的冻结释放与决定落库同一
// 事务，释放失败交回补偿续办而不回滚决定（编排自己分的格）。
type transactionalRejection struct {
	transactor bentoapp.Transactor
	inner      *shipmentapp.RejectShipmentRequestHandler
}

var _ shipmenthttp.ActiveRejectionHandler = transactionalRejection{}

func (rejection transactionalRejection) Handle(
	ctx context.Context,
	command shipmentapp.RejectShipmentRequestCommand,
) (shipmentapp.RejectShipmentRequestResult, error) {
	var result shipmentapp.RejectShipmentRequestResult
	err := rejection.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := rejection.inner.Handle(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return shipmentapp.RejectShipmentRequestResult{}, err
	}
	return result, nil
}

// buildRejectionOrchestration 装配 `/shipment-requests/rejections` 的真编排（票 09；
// ADR-0081 的命令面保留条款；ADR-0086 Decision 三——主动拒绝在自己的命令事务里直接
// 形成决定，不发续办信封）。
//
// 机制半边全部接真：委托仓储、判断读写与决定标识走 PS 真库口，拒绝授权经 partycommercial
// 适配器接 PC 裁定编排与真授权册，冻结释放经 settlementaccounting 适配器接 SA 释放编排
// 与真两账本。实例半边照旧留空，各自的「显式未配置」形状各归各口：
//
//   - 拒绝授权的请求映射（法人/权限等级/商业范围/时点，`PAR-COM-14`）nil——适配器答
//     未形成，编排如实停在`拒绝授权不可用`，不代拟坐标也不冒充`授权规则未配置`；
//   - 释放的作用域与金额映射（账户/估价缝）nil——真有冻结要释放时它答未配置留补偿
//     续办，不凭空认领别人的钱。
func buildRejectionOrchestration(db *bentopg.DB) (shipmenthttp.ActiveRejectionHandler, error) {
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

	authorizer := psparty.NewActiveRejectionAdapter(
		pcapplication.NewAdjudicateCommercialAuthorizationHandler(grants),
		// RequestSource 留空：实例半边，见函数注释。
		nil,
	)
	release := pssettlement.NewPreAcceptanceControlAdapter(pssettlement.PreAcceptanceControlAdapterDeps{
		Release: saapplication.NewReleasePreAcceptanceControlHandler(freezes, exposures, clock),
		// Apply 留空：主动拒绝只消费释放半边；Scopes/Amounts 留空：实例半边，见函数注释。
	})

	handler := shipmentapp.NewRejectShipmentRequestHandler(shipmentapp.RejectShipmentRequestDeps{
		Requests:   requests,
		Authorizer: authorizer,
		Judgments:  judgments,
		Recorder:   judgments,
		Release:    release,
		Identities: identities,
		Clock:      clock,
	})
	return transactionalRejection{transactor: db.Transactor(), inner: handler}, nil
}
