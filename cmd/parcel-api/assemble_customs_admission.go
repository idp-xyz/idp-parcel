package main

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	ccpostgres "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	customsapp "go.idp.xyz/idp-parcel/internal/customscompliance/application"
)

// 关务动作准入判断与节点关务区域登记的生产装配（ADR-0206）。另立文件而不并进 assemble_customs_registration.go：
// 那边是案件配置族各册的登记写面，这边多一只请求驱动的判断编排，两只的依赖面不同，合在一处装配点就看不出谁
// 接了谁。

type transactionalNodeCustomsZoneRegistration struct {
	transactor bentoapp.Transactor
	inner      *customsapp.RegisterNodeCustomsZoneHandler
}

var _ customshttp.NodeCustomsZoneRegistrar = transactionalNodeCustomsZoneRegistration{}

func (registration transactionalNodeCustomsZoneRegistration) Handle(
	ctx context.Context,
	command customsapp.RegisterNodeCustomsZoneCommand,
) (customsapp.CaseConfigurationOutcome, error) {
	return caseConfigurationInTransaction(ctx, registration.transactor,
		func(txCtx context.Context) (customsapp.CaseConfigurationOutcome, error) {
			return registration.inner.Handle(txCtx, command)
		})
}

// transactionalActionAdmissionJudge 给准入判断一笔自己的事务：判断册写口无环境事务即拒，而消费方（运输履约的出发
// 登记，ADR-0205）在开自己的写事务之前同步问——两笔事务各管各的，准入判断记下了而消费方那笔回滚，留下的是一次
// 如实发生过的询问，不是一条悬空事实。
type transactionalActionAdmissionJudge struct {
	transactor bentoapp.Transactor
	inner      *customsapp.JudgeActionAdmissionHandler
}

func (judge transactionalActionAdmissionJudge) Handle(
	ctx context.Context,
	command customsapp.JudgeActionAdmissionCommand,
) (customsapp.JudgeActionAdmissionResult, error) {
	var result customsapp.JudgeActionAdmissionResult
	err := judge.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		judged, judgeErr := judge.inner.Handle(txCtx, command)
		if judgeErr != nil {
			return judgeErr
		}
		result = judged
		return nil
	})
	if err != nil {
		return customsapp.JudgeActionAdmissionResult{}, err
	}
	return result, nil
}

// customsAdmissionOrchestration 收拢本文件装的两格。actionAdmission 今天只有装配测试在用：它的生产消费者是运输履约
// 出发登记的消费侧端口（票 demo-chain-gaps/32），那一格接上之前，这里先把判断编排连同真库依赖装齐、在进程启动时
// 炸出装配疏漏。
type customsAdmissionOrchestration struct {
	nodeZoneRegistration transactionalNodeCustomsZoneRegistration
	actionAdmission      transactionalActionAdmissionJudge
}

func buildCustomsAdmissionOrchestration(db *bentopg.DB) (customsAdmissionOrchestration, error) {
	none := customsAdmissionOrchestration{}
	zoneRegistry, err := ccpostgres.NewNodeCustomsZoneRegistrations(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: customs node zone registry: %w", err)
	}
	zoneView, err := ccpostgres.NewNodeCustomsZonePointView(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: customs node zone view: %w", err)
	}
	scopes, err := ccpostgres.NewObjectCustomsScopeView(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: customs object scope view: %w", err)
	}
	restrictions, err := ccpostgres.NewRestrictions(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: customs restrictions: %w", err)
	}
	// 门禁目录与「税费付款」规则行同一只读适配器，付款核对当前版另一只——与放行门禁核对接的是同一批（读法也是同一份，
	// assessReleaseGate）。
	gates, err := ccpostgres.NewGateConditionView(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: customs gate condition view: %w", err)
	}
	dutyVerifications, err := ccpostgres.NewDutyPaymentReconciliation(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: customs duty payment reconciliation store: %w", err)
	}
	releases, err := ccpostgres.NewReleaseOutcomeView(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: customs release outcome view: %w", err)
	}
	judgements, err := ccpostgres.NewActionAdmissionJudgements(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: customs action admission judgements: %w", err)
	}
	judge, err := customsapp.NewJudgeActionAdmissionHandler(customsapp.JudgeActionAdmissionDeps{
		Zones:             zoneView,
		Scopes:            scopes,
		Restrictions:      restrictions,
		Conditions:        gates,
		DutyRules:         gates,
		DutyVerifications: dutyVerifications,
		Releases:          releases,
		Store:             judgements,
		Clock:             systemClock{},
	})
	if err != nil {
		return none, fmt.Errorf("parcel-api: customs action admission orchestration: %w", err)
	}

	transactor := db.Transactor()
	return customsAdmissionOrchestration{
		nodeZoneRegistration: transactionalNodeCustomsZoneRegistration{
			transactor: transactor,
			inner: customsapp.NewRegisterNodeCustomsZoneHandler(
				customsapp.RegisterNodeCustomsZoneDeps{Registry: zoneRegistry, View: zoneView}),
		},
		actionAdmission: transactionalActionAdmissionJudge{transactor: transactor, inner: judge},
	}, nil
}
