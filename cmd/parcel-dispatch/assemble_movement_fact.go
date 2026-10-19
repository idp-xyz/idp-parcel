package main

import (
	"context"
	"errors"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	vetf "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/transportfulfillment"
	veapplication "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
)

// veMovementFactUndecidedSentinels 只给 VE 移动事实投影这一路（ADR-0205 决定五）。本路不 FanOut：NR 的路由偏离消费移动事实
// 另立票 demo-chain-gaps/35，接得住之前不挂（ADR-0049：接得住才登记）。与交接那份分开列：本路多两站——事实声明的段，以及阻断
// 读数那一步的信号规则与分诊。
//
// 不在名单里：ErrMovementFactRecordInconsistent（仓储不变量已破，含段册答非所问）、ErrMovementFactUntranslatableAnswer（引用
// 坏了）、ErrDepartureGateSignalHandoffPending 与 ErrProjectionHandoffPending（要查 outbox 下游）、
// veconsume.ErrUnexpectedProjectionOutcome 与 veapplication.ErrUnexpectedDepartureGateSignalOutcome（封闭集合外）。
var veMovementFactUndecidedSentinels = []error{
	vetf.ErrMovementFactNotVisible,
	vetf.ErrMovementSegmentNotVisible,
	vetf.ErrDepartureGateSignalUndecided,
	vetf.ErrProjectionUndecided,
}

// deriveMovementFactConsumer 接 TF 移动事实已形成 → VE 投影与阻断信号（ADR-0205 决定五）。信封只带事实键三维；处理适配器按键
// 重读移动事实，沿它声明的段取那一刻在控的对象逐个派生（ADR-0066），读数里阻断的那几件交分诊——权威事实留在
// transport-fulfillment。交回的 Consumer 已包好本路的未决哨兵，路由表上直接挂。
func deriveMovementFactConsumer(
	db *bentopg.DB,
	inboxStore *inbox.Store,
	derive *tenantBoundProjectionDerive,
) (dispatch.Consumer, error) {
	facts, err := tfpostgres.NewMovementFacts(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: projection movement facts: %w", err)
	}
	segments, err := tfpostgres.NewFulfillmentSegments(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: projection movement segments: %w", err)
	}
	processing, err := vetf.NewDeriveOnMovementFactAdapter(facts, segments, derive, tenantBoundDepartureGateSignals{derive: derive})
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: derive on movement fact: %w", err)
	}
	consumer, err := veinbox.NewMovementFactConsumer(db.Transactor(), inboxStore, processing)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: derive movement fact consumer: %w", err)
	}
	routed, err := dispatch.WithUndecidedSentinels(consumer, veMovementFactUndecidedSentinels...)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: movement fact projection undecided translation: %w", err)
	}
	return routed, nil
}

var _ vetf.MovementProjectionHandler = (*tenantBoundProjectionDerive)(nil)

// tenantBoundDepartureGateSignals 在每次 Handle 用命令上的租户构造出发门禁阻断信号规则视图与分诊规则视图，理由同
// tenantBoundProjectionDerive：两本册都在构造期绑租户，派发进程是多租户。发作期库、身份签发与分诊意图交出借投影派生那一份，
// 信号与投影同一笔事务落库。
type tenantBoundDepartureGateSignals struct {
	derive *tenantBoundProjectionDerive
}

var _ vetf.DepartureGateSignalRaiser = tenantBoundDepartureGateSignals{}

func (signals tenantBoundDepartureGateSignals) Handle(
	ctx context.Context,
	command veapplication.RaiseDepartureGateSignalCommand,
) (veapplication.RaiseDepartureGateSignalResult, error) {
	if command.TenantID.String() == "" {
		return veapplication.RaiseDepartureGateSignalResult{}, errors.New("parcel-dispatch: departure gate signal tenant is empty")
	}
	derive := signals.derive
	rules, err := vepostgres.NewDepartureGateSignalRules(derive.db, command.TenantID)
	if err != nil {
		return veapplication.RaiseDepartureGateSignalResult{}, err
	}
	triage, err := vepostgres.NewTriageRules(derive.db, command.TenantID)
	if err != nil {
		return veapplication.RaiseDepartureGateSignalResult{}, err
	}
	handler, err := veapplication.NewRaiseDepartureGateSignalHandler(veapplication.RaiseDepartureGateSignalDeps{
		Rules: rules,
		Signals: veapplication.NewRaiseSignalHandler(veapplication.RaiseSignalDeps{
			Episodes:   derive.episodes,
			Triage:     triage,
			Identities: derive.episodeIdentities,
			Cases:      derive.caseIdentities,
			Downstream: derive.triageHandoff,
			Clock:      derive.clock,
		}),
		Clock: derive.clock,
	})
	if err != nil {
		return veapplication.RaiseDepartureGateSignalResult{}, err
	}
	return handler.Handle(ctx, command)
}
