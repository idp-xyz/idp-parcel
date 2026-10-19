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

// 揽收更正口的装配（票 tf-segment-lifecycle-closure/08）：`/transport-fulfillment/offsite-pickup-corrections`
// 背后的真编排。形照揽收登记那一格（assemble_control_facts.go）：一笔事务包住读回前版、落新版本行、意图
// 入队与段上重派生参与；意图交付或重派生失败不翻结果，编排吞成续办引用（领域正当拒绝单开答格）后照常作答；
// 编排返回 error 整笔回滚。
//
// **交入段登记册，是为了让更正在段上重派生参与**（ADR-0112 决定一、二；票 tf-segment-lifecycle-closure/10）：
// 该对象凭前版入场的那条参与由回指它的替代版本接替，起点随更正后的发生时刻，与更正同一笔事务。Deps 允许
// Segments 缺席，而 rederiveFulfillmentParticipation 对缺席既不重派生也不报失败——答复与重派生成功逐格同形，
// 装配点漏接时没有任何答复会红，只有读回段才看得出来，所以这里偏不让它缺席。
//
// **不交判断登记册**：Correct 不碰承运主体判断（票 tf-segment-lifecycle-closure/10 红线「不改承运主体判断的
// 现有版本」）——判断随更正重新派生是 ActualCarrierJudgment 自己的下一版，走它自己的口；交进来也无人读。
// 同一条 RegisterOffsitePickupHandler 在控制事实那一格另有一只带段与判断登记册的实例服务首登口——两只共用
// 同一册、同一签发器、同一 Outbox 口，编排本身无状态，各接各的口。

type transactionalPickupCorrection struct {
	transactor bentoapp.Transactor
	inner      *tfapp.RegisterOffsitePickupHandler
}

var _ tfhttp.PickupCorrectionHandler = transactionalPickupCorrection{}

func (pickup transactionalPickupCorrection) Correct(
	ctx context.Context,
	command tfapp.CorrectOffsitePickupCommand,
) (tfapp.RegisterOffsitePickupResult, error) {
	var result tfapp.RegisterOffsitePickupResult
	err := pickup.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := pickup.inner.Correct(txCtx, command)
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

// buildOffsitePickupCorrectionOrchestration 装配揽收更正口背后的真编排：揽收登记册、段登记册、结果版本签发、
// 揽收登记意图交付（与首登共用同一口——更正版本在信封 ID 上加版本段，两代各自入队）、时钟。
// 没有「显式未配置」缝：更正引用的都是本上下文自己的事实。
func buildOffsitePickupCorrectionOrchestration(db *bentopg.DB) (tfhttp.PickupCorrectionHandler, error) {
	pickups, err := tfpostgres.NewOffsitePickupRegistrations(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: offsite pickup registrations: %w", err)
	}
	segments, err := tfpostgres.NewFulfillmentSegments(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: fulfillment segments: %w", err)
	}
	versions, err := tfpostgres.NewResultVersions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: pickup result versions: %w", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: outbox store: %w", err)
	}
	clock := systemClock{}
	downstream, err := tfpostgres.NewOutboxOffsitePickupRegistrationHandoff(db, store, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: offsite pickup registration handoff: %w", err)
	}
	return transactionalPickupCorrection{
		transactor: db.Transactor(),
		inner: tfapp.NewRegisterOffsitePickupHandler(tfapp.RegisterOffsitePickupDeps{
			Pickups:    pickups,
			Segments:   segments,
			Versions:   versions,
			Downstream: downstream,
			Clock:      clock,
		}),
	}, nil
}
