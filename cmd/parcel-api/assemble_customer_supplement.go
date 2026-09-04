package main

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	psidentity "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/identity"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// supplementBoundary 是 ADR-0106 Decision 三的事务边界壳：新提交版本落库与「新提交版本已形成」
// 信封在同一个事务里成立或一起消失，形状照 reviewCompletionBoundary。版本落了库而信封没入队，
// 停在`等待受控补充`并已入账的委托就再也没有投递来续办——入账反而成了更安静的永久停滞，那正是
// 这层壳要焊死的缝。
//
// 交接挂在 Save 而不是包整段 Handle：受控补充编排只在`已记录`那一格走到 Save（重放、冲突、基准
// 过期、成员变更、决定已形成都在保存之前交回），Save 即新版本落库。挂错地方有护栏：交接适配器
// 对没有前版的聚合响亮报错，整笔回滚。
type supplementBoundary struct {
	transactor bentoapp.Transactor
	inner      ports.ShipmentRequestRepository
	handoff    ports.SubmissionVersionFormedHandoff
}

var _ ports.ShipmentRequestRepository = supplementBoundary{}

func (boundary supplementBoundary) FindBySourceIdentity(
	ctx context.Context,
	identity domain.SourceIdentity,
) (domain.ShipmentRequest, bool, error) {
	return boundary.inner.FindBySourceIdentity(ctx, identity)
}

// Insert 只切事务、不发信封：受控补充编排走不到这一口，它在这里是因为端口带着它。
func (boundary supplementBoundary) Insert(
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

func (boundary supplementBoundary) Save(
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
			// 版本冲突是业务答案：本事务没写下任何东西，不发信封，交回编排按 ADR-0031 重读重放。
			return nil
		}
		return boundary.handoff.HandOffSubmissionVersionFormed(txCtx, ports.SubmissionVersionFormedHandoffIntent{
			Identity: identity,
			Request:  request,
		})
	})
	if err != nil {
		return ports.ShipmentRequestSaveOutcomeInvalid, err
	}
	return outcome, nil
}

// buildCustomerSupplementOrchestration 装配受控补充（ADR-0045 的编排半边，ADR-0106 Decision 四把它
// 接进生产）的真编排。来源保全走 preservationBoundary（与首次提交同一层壳），新版本落库走
// supplementBoundary 携 OutboxSubmissionVersionFormedHandoff，事件类型
// `parcel-shipment.shipment-request.submission-version-formed`，消费门在 cmd/parcel-dispatch
// （同一条接受判断链的第四扇门）。
//
// 客户渠道的采信身份属 `PAR-INT-01`（实例半边）：端点表那一行以 `UnconfiguredIntake{}` 起步，
// 谁能替哪个客户账户补充由渠道认证答，编排收到的命令已带核验过的两份来源身份。本函数不带
// 任何默认身份。
func buildCustomerSupplementOrchestration(db *bentopg.DB) (*shipmentapp.FormNewSubmissionVersionHandler, error) {
	sources, err := pspostgres.NewSourceSubmissions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: source submissions: %w", err)
	}
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: shipment requests: %w", err)
	}
	identities, err := psidentity.NewSubmissionIdentities()
	if err != nil {
		return nil, fmt.Errorf("parcel-api: submission identities: %w", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: outbox store: %w", err)
	}
	clock := systemClock{}
	handoff, err := pspostgres.NewOutboxSubmissionVersionFormedHandoff(db, store, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: submission version formed handoff: %w", err)
	}
	return shipmentapp.NewFormNewSubmissionVersionHandler(shipmentapp.FormNewSubmissionVersionDeps{
		Sources: preservationBoundary{transactor: db.Transactor(), inner: sources},
		Requests: supplementBoundary{
			transactor: db.Transactor(),
			inner:      requests,
			handoff:    handoff,
		},
		Identities: identities,
		Clock:      clock,
	}), nil
}
