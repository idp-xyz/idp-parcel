package main

import (
	"context"
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

// transactionalCarrierPickupJudge 把一次实际承运商首次有效收寄的显式判断包进一笔事务，理由随
// transactionalEffectiveTimeJudge：登记册写口、Outbox 入队、段与判断的写口按框架合同无事务即拒，事务边界归装配点；
// 收寄版本登记、意图入队、进段（或参与重派生）与实际承运商判断追加同笔落地，编排返回 error 时整笔回滚。
type transactionalCarrierPickupJudge struct {
	transactor bentoapp.Transactor
	inner      *tfapp.JudgeCarrierFirstEffectivePickupHandler
}

var _ tfhttp.CarrierPickupJudge = transactionalCarrierPickupJudge{}

func (judge transactionalCarrierPickupJudge) Judge(
	ctx context.Context,
	command tfapp.JudgeCarrierFirstEffectivePickupCommand,
) (tfapp.JudgeCarrierFirstEffectivePickupResult, error) {
	var result tfapp.JudgeCarrierFirstEffectivePickupResult
	err := judge.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := judge.inner.Judge(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return tfapp.JudgeCarrierFirstEffectivePickupResult{}, err
	}
	return result, nil
}

// buildCarrierPickupJudgment 装配 `/transport-fulfillment-carrier-first-effective-pickup-judgments` 背后的真编排
// （票 label-channel/31，ADR-0135 决定八）。缝全接真：收寄登记册、身份签发、Outbox 意图交付（已形成 / 替代 / 失效
// 交 parcel-shipment）、外部承运轨迹事实登记册（依据读回）、承运主体身份读口（TF 对 PC 参与方册与法人册的消费侧
// 适配器，ADR-0025）、段登记册与实际承运商判断登记册（进段 / 重派生 / 首版）、把收寄依据交给实际承运商判断的编排、
// 时钟。本笔没有「显式未配置」缝：判断引用的都是本上下文自己的事实与 PC 在册身份，没有等租户参数的实例半边——
// 规则那一路（决定八第二种读法来源）随第一家真源另立，不在这里。
//
// 第二个返回值是同一只收寄登记册：它也是查阅面 `/transport-fulfillment-carrier-first-effective-pickups` 的读口
// （链按对象上列），读面不是编排，main 直接取它交给查阅端点。
func buildCarrierPickupJudgment(db *bentopg.DB) (tfhttp.CarrierPickupJudge, *tfpostgres.CarrierFirstEffectivePickups, error) {
	pickups, err := tfpostgres.NewCarrierFirstEffectivePickups(db)
	if err != nil {
		return nil, nil, fmt.Errorf("parcel-api: carrier first effective pickups: %w", err)
	}
	versions, err := tfpostgres.NewResultVersions(db)
	if err != nil {
		return nil, nil, fmt.Errorf("parcel-api: carrier pickup identities: %w", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		return nil, nil, fmt.Errorf("parcel-api: outbox store: %w", err)
	}
	clock := systemClock{}
	downstream, err := tfpostgres.NewOutboxCarrierFirstEffectivePickupHandoff(db, store, clock)
	if err != nil {
		return nil, nil, fmt.Errorf("parcel-api: carrier pickup handoff: %w", err)
	}
	evidence, err := tfpostgres.NewExternalTrackingFacts(db)
	if err != nil {
		return nil, nil, fmt.Errorf("parcel-api: external tracking facts: %w", err)
	}
	parties, err := pcpostgres.NewPartyIdentityRegistrations(db)
	if err != nil {
		return nil, nil, fmt.Errorf("parcel-api: party identity registrations: %w", err)
	}
	directory, err := tfpartycommercial.NewCarrierIdentityDirectory(parties, parties)
	if err != nil {
		return nil, nil, fmt.Errorf("parcel-api: carrier identity directory: %w", err)
	}
	segments, err := tfpostgres.NewFulfillmentSegments(db)
	if err != nil {
		return nil, nil, fmt.Errorf("parcel-api: fulfillment segments: %w", err)
	}
	judgments, err := tfpostgres.NewActualCarrierJudgments(db)
	if err != nil {
		return nil, nil, fmt.Errorf("parcel-api: actual carrier judgments: %w", err)
	}
	carrierJudgments := tfapp.NewFormActualCarrierJudgmentHandler(tfapp.FormActualCarrierJudgmentDeps{
		Segments:   segments,
		Judgments:  judgments,
		Identities: directory,
		Clock:      clock,
	})
	handler := tfapp.NewJudgeCarrierFirstEffectivePickupHandler(tfapp.JudgeCarrierFirstEffectivePickupDeps{
		Pickups:          pickups,
		Identities:       versions,
		Downstream:       downstream,
		Evidence:         evidence,
		Directory:        directory,
		Segments:         segments,
		Judgments:        judgments,
		CarrierJudgments: carrierJudgments,
		Clock:            clock,
	})
	return transactionalCarrierPickupJudge{transactor: db.Transactor(), inner: handler}, pickups, nil
}
