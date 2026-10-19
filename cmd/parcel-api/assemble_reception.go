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
// 库的写口按框架合同无事务即拒，事务边界归装配点。原子范围只有 UC-NO-002 步骤 8「原子提交
// 收寄判断、控制、审计和对应发布意图」；编排返回错误时整笔回滚，半截写入不落库。
//
// 跨上下文读口须经 savepoint 隔离：它们跟着走这一笔事务，一条 SQL 级读错误会把事务打进中止态、
// 连带作废收寄的写。服务结果读口因此包在 SavepointedServiceResultView 里（ADR-0186）；身份核对
// 读口包在 SavepointedParcelIdentityView 里——它读不动也要落未核对的待识别、同一笔自投重核门铃
// （ADR-0199 决定五），没有这层隔离，那一行连同门铃跟着整笔回滚。
//
// 意图没交出去的收寄在这条路上不提交。编排把入队的 Go 层失败吞成续办引用、照常交回业务答案——前线导入靠它，报告把引用
// 写给内勤，重跑同一文件重发同一份（ADR-0043 决定三）；这里没有人读它：响应里没有续办这一格，设备收到 2xx 就出队，不会
// 再有人重投同一来源去触发重发。所以凭 IntakeHandoffError 整笔回滚、答 5xx，设备留在队里重发，重发时收寄与意图在同一笔
// 里落地（ADR-0049「发布意图与业务结果同一提交」）。入队报 SQL 级错误时事务已中止，本就整笔回滚、答 5xx。
//
// 信封被框架确定性拒收不另成一格：信封上随设备而变的只有来源标识（进 Subject），收寄口在接入时就按信封上限把越界的拒成
// 坏报文（nodeopshttp 的 receptionCommand）。剩下能让信封被拒的是服务端缺陷，答 5xx 让设备留着这次接收等修好；答 4xx
// 会让设备把一次如实记下的接收出队交给人，错却不在它。
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
		if handoffErr := handled.IntakeHandoffError(); handoffErr != nil {
			return fmt.Errorf("parcel-api: reception intent not handed off: %w", handoffErr)
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
// 机制半边全部接真：收寄库、收寄结果版本签发、Outbox 意图交付与自投的重核门铃走 NO 真库口；身份核对读 PS 的外部标识关联册
// （ADR-0184），服务结果读口读 PS 的取消与终局两册（票 operator-channel/25）。两个读口跟着收寄走同一笔事务只因
// ctx 是同一个：PS 读不加锁，READ COMMITTED 下同事务读既无一致性收益也无代价。
func buildReceptionOrchestration(db *bentopg.DB) (nodeopshttp.ReceptionHandler, error) {
	deps, err := buildReceptionDeps(db)
	if err != nil {
		return nil, err
	}
	return receptionWithin(db, deps)
}

// buildReceptionDeps 是 buildReceptionOrchestration 的依赖半边。单拆出来是为了真库用例只换一只依赖（让入队失败一次、
// 在读别的实物关联与落册之间挂起），其余照生产装配，不在测试里另抄一份装配。
func buildReceptionDeps(db *bentopg.DB) (nodeopsapp.ReceiveDeliveredUnitDeps, error) {
	none := nodeopsapp.ReceiveDeliveredUnitDeps{}
	receptions, err := nopostgres.NewReceptions(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: receptions: %w", err)
	}
	versions, err := noidentity.NewIntakeResultVersions()
	if err != nil {
		return none, fmt.Errorf("parcel-api: intake result versions: %w", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: outbox store: %w", err)
	}
	clock := systemClock{}
	downstream, err := nopostgres.NewOutboxNodeIntakeHandoff(db, store, clock)
	if err != nil {
		return none, fmt.Errorf("parcel-api: node intake handoff: %w", err)
	}
	doorbell, err := nopostgres.NewOutboxIdentityRecheckDoorbell(db, store, clock)
	if err != nil {
		return none, fmt.Errorf("parcel-api: identity recheck doorbell: %w", err)
	}
	identity, err := buildParcelIdentityView(db)
	if err != nil {
		return none, err
	}
	serviceResults, err := buildParcelServiceResultView(db)
	if err != nil {
		return none, err
	}
	// 身份判断先由送站带来的依据推导可验证交付范围（ADR-0212 决定四），证据读 NO 自己的预报册、交接单册与时窗册，跟着收寄走同一笔
	// 事务；本方册读不动不形成答案，不包 savepoint。
	scope, err := nopostgres.NewDeliveryScopeEvidence(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: delivery scope evidence: %w", err)
	}
	// 送站带着交接单标识时，同一笔把这件实物记成那张交接单的成员（ADR-0212 决定三），写的是交接单册本尊。
	handover, err := nopostgres.NewDeliveryHandoverNotes(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: delivery handover notes: %w", err)
	}
	return nodeopsapp.ReceiveDeliveredUnitDeps{
		Identity:       identity,
		ServiceResults: serviceResults,
		Receptions:     receptions,
		Versions:       versions,
		Downstream:     downstream,
		Doorbell:       doorbell,
		Scope:          scope,
		Handover:       handover,
		Clock:          clock,
	}, nil
}

// receptionWithin 把一组收寄依赖装成编排，包进 transactionalReception。
func receptionWithin(db *bentopg.DB, deps nodeopsapp.ReceiveDeliveredUnitDeps) (nodeopshttp.ReceptionHandler, error) {
	handler, err := nodeopsapp.NewReceiveDeliveredUnitHandler(deps)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: reception orchestration: %w", err)
	}
	return transactionalReception{transactor: db.Transactor(), inner: handler}, nil
}

// buildParcelIdentityView 装收寄编排的身份核对读口：PS 的外部标识关联册经 NO 的防腐适配器读（ADR-0184 决定五），外面包一层
// savepoint，读失败不连带收寄的写（ADR-0199 决定五，见 transactionalReception）。人工重核也经这里装：它读不动本就整笔回滚，
// 隔离对它不改变答案。
func buildParcelIdentityView(db *bentopg.DB) (noports.ParcelIdentityView, error) {
	identifiers, err := pspostgres.NewExternalIdentifiers(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: external identifiers: %w", err)
	}
	view, err := noparcel.NewParcelIdentityView(identifiers)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: parcel identity view: %w", err)
	}
	isolated, err := nopostgres.NewSavepointedParcelIdentityView(db, view)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: parcel identity savepoint: %w", err)
	}
	return isolated, nil
}

// buildParcelServiceResultView 装收寄编排的服务结果读口：PS 的取消决定册与终局册经 NO 的防腐适配器读（票 operator-channel/25），
// 外面包一层 savepoint，读失败不连带收寄的写（ADR-0186，见 transactionalReception）。
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
	isolated, err := nopostgres.NewSavepointedServiceResultView(db, view)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: parcel service result savepoint: %w", err)
	}
	return isolated, nil
}
