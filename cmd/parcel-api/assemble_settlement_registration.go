package main

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	settlementhttp "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/http"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	settlementapp "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
)

// 外部资金事实采用 / 更正在线登记面的生产装配（票 sa-cc/31，27 裁决 2 第二步；ADR-0085 决定一：与受控 CLI
// `parcel-settlement-register` 消费同一登记用例）。两口在生产上是同一只 MapExternalFundsHandler 的两个方法，
// 端点表仍各收一参、包装类型因此是两个：合并就得把两组方法挤进一个类型再按命令分派，装配测试会盖不住
// 「某一格接错了编排」（判据同 assemble_customs_registration 的协作 / 核对两格）。
//
// 事务边界与 dutyReconciliationInTransaction 同源：登记册写口按框架合同无事务即拒，一次调用一笔事务，采用与
// 它的重放 / 冲突判定读回、以及向 CC 交的采用信封因此看同一份快照；用例交回业务答案（含重放与治理答案）时
// 提交，返回错误时整笔回滚，端点按 ADR-0022 答「没形成答案」。

// fundsRegistrationInTransaction 是采用与更正两格共用的事务壳。
func fundsRegistrationInTransaction(
	ctx context.Context,
	transactor bentoapp.Transactor,
	register func(context.Context) (settlementapp.FundsResult, error),
) (settlementapp.FundsResult, error) {
	var result settlementapp.FundsResult
	err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		registered, registerErr := register(txCtx)
		if registerErr != nil {
			return registerErr
		}
		result = registered
		return nil
	})
	if err != nil {
		return settlementapp.FundsResult{}, err
	}
	return result, nil
}

// 两个包装类型各自只暴露端点要的那一个用例方法：内层是同一个 MapExternalFundsHandler，它本身就同时满足两个
// Registrar 契约，但直接把它接进端点表就绕过了事务壳——登记写口无环境事务即拒，形照 parcel-settlement-register
// 的 execute。
type transactionalExternalFundsFactRegistration struct {
	transactor bentoapp.Transactor
	inner      *settlementapp.MapExternalFundsHandler
}

var _ settlementhttp.ExternalFundsFactRegistrar = transactionalExternalFundsFactRegistration{}

func (registration transactionalExternalFundsFactRegistration) AdoptFact(
	ctx context.Context,
	command settlementapp.AdoptFundsFactCommand,
) (settlementapp.FundsResult, error) {
	return fundsRegistrationInTransaction(ctx, registration.transactor,
		func(txCtx context.Context) (settlementapp.FundsResult, error) {
			return registration.inner.AdoptFact(txCtx, command)
		})
}

type transactionalExternalFundsFactCorrectionRegistration struct {
	transactor bentoapp.Transactor
	inner      *settlementapp.MapExternalFundsHandler
}

var _ settlementhttp.ExternalFundsFactCorrectionRegistrar = transactionalExternalFundsFactCorrectionRegistration{}

func (registration transactionalExternalFundsFactCorrectionRegistration) CorrectFact(
	ctx context.Context,
	command settlementapp.CorrectFundsFactCommand,
) (settlementapp.FundsResult, error) {
	return fundsRegistrationInTransaction(ctx, registration.transactor,
		func(txCtx context.Context) (settlementapp.FundsResult, error) {
			return registration.inner.CorrectFact(txCtx, command)
		})
}

// settlementRegistrationOrchestration 收拢两格，供装配点一次取回。分字段而不是一个 handler：端点表按类各接
// 一格，收成一格就得在装配行上现取字段，那正是要避免的「谁接谁在装配点看不出来」。
type settlementRegistrationOrchestration struct {
	externalFundsFact           transactionalExternalFundsFactRegistration
	externalFundsFactCorrection transactionalExternalFundsFactCorrectionRegistration
}

// buildSettlementRegistrationOrchestration 装配 `/settlement-external-funds-fact-registrations` 与
// `/settlement-external-funds-fact-correction-registrations` 的真编排，与 parcel-settlement-register 的
// buildRegistrar 同一套：编排的每一口都接真，一只都不留 nil、不放替身。映射写口、核销写口与已结视图交接口不在
// 采用 / 更正路径上也接真——「生产装配里不放任何替身」是 SA 装配处的既有纪律，且 NewMapExternalFundsHandler 的
// 构造门拒 nil，半装根本装不进去。资金事实交接口与登记册同一只 db、同一只 Outbox Store：事务壳给的环境事务因此
// 同时罩住版本行与向 CC 交的采用信封——两者同生共死，形照 buildCustomsRegistrationOrchestration 的核对交接口。
func buildSettlementRegistrationOrchestration(db *bentopg.DB) (settlementRegistrationOrchestration, error) {
	none := settlementRegistrationOrchestration{}
	facts, err := sapostgres.NewExternalFundsFacts(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: settlement external funds fact store: %w", err)
	}
	mappings, err := sapostgres.NewFundsMappings(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: settlement funds mapping store: %w", err)
	}
	applications, err := sapostgres.NewSettlementApplications(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: settlement application store: %w", err)
	}
	outboxStore, err := outbox.NewStore(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: settlement outbox store: %w", err)
	}
	factHandoff, err := sapostgres.NewOutboxExternalFundsFactHandoff(db, outboxStore, systemClock{})
	if err != nil {
		return none, fmt.Errorf("parcel-api: settlement external funds fact handoff: %w", err)
	}
	downstream, err := sapostgres.NewOutboxSettlementApplicationHandoff(db, outboxStore, systemClock{})
	if err != nil {
		return none, fmt.Errorf("parcel-api: settlement application handoff: %w", err)
	}
	// 构造门在构造期拒 nil 依赖（票 sa-cc/27 裁决 4）：装配疏漏在进程启动那一刻炸出来，不等第一条事实到达。
	funds, err := settlementapp.NewMapExternalFundsHandler(settlementapp.MapExternalFundsDeps{
		Facts:        facts,
		Mappings:     mappings,
		Applications: applications,
		Downstream:   downstream,
		FactHandoff:  factHandoff,
		Clock:        systemClock{},
	})
	if err != nil {
		return none, fmt.Errorf("parcel-api: settlement external funds orchestration: %w", err)
	}

	transactor := db.Transactor()
	return settlementRegistrationOrchestration{
		externalFundsFact:           transactionalExternalFundsFactRegistration{transactor: transactor, inner: funds},
		externalFundsFactCorrection: transactionalExternalFundsFactCorrectionRegistration{transactor: transactor, inner: funds},
	}, nil
}
