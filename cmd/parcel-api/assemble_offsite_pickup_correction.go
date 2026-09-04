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
// 背后的真编排。形照揽收登记那一格（assemble_control_facts.go）：一笔事务包住读回前版、落新版本行与意图
// 入队；意图交付失败不翻结果，编排吞成续办引用后照常作答；编排返回 error 整笔回滚。
//
// 不交入段登记册与判断登记册：更正不进段（裁决附问——段侧「来源更正 → 参与关系重派生」另立一票覆盖
// 交接与揽收两种来源），编排的 Correct 也不会去碰它们；这里留空是如实，不是漏接。同一条
// RegisterOffsitePickupHandler 在控制事实那一格另有一只带段登记册的实例服务首登口——两只共用同一册、
// 同一签发器、同一 Outbox 口，编排本身无状态，各接各的口。

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

// buildOffsitePickupCorrectionOrchestration 装配揽收更正口背后的真编排：揽收登记册、结果版本签发、
// 揽收登记意图交付（与首登共用同一口——更正版本在信封 ID 上加版本段，两代各自入队）、时钟。
// 没有「显式未配置」缝：更正引用的都是本上下文自己的事实。
func buildOffsitePickupCorrectionOrchestration(db *bentopg.DB) (tfhttp.PickupCorrectionHandler, error) {
	pickups, err := tfpostgres.NewOffsitePickupRegistrations(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: offsite pickup registrations: %w", err)
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
			Versions:   versions,
			Downstream: downstream,
			Clock:      clock,
		}),
	}, nil
}
