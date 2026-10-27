package main

import (
	"context"
	"errors"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	ccpostgres "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	ccapplication "go.idp.xyz/idp-parcel/internal/customscompliance/application"
	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
	tfcustoms "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/customscompliance"
	tfinbox "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/inbox"
	tfnode "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/nodeoperations"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapplication "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	vetf "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/transportfulfillment"
)

// 出发门禁补问的派发装配（ADR-0305）：TF 消费自己的交接与对象级揽收信封，替那件对象按原出发时点再问一次关务、追加一版读数并交一封
// 补问信封；全程追踪消费那封补问信封，为补问读数里新判阻断的那几件形成出发门禁阻断信号。
//
// 读数那一半与 cmd/parcel-api 的 buildDepartureGateReader 接的是同一批真适配器、同一个库：段册、班次册、交接册、节点作业的集运成员
// 关系与收寄身份、关务的动作准入判断。两个进程各自是组合根，不跨 main 包复用，理由同 triggerDeliveryDispatchOnHandoverConsumer；
// 两份接法要一致，补问问出来的才是登记那一刻那条路径会问出来的。

// transactionalActionAdmissionJudge 给关务的准入判断一笔自己的事务，与 cmd/parcel-api 那一份同形：判断册写口无环境事务即拒，判断要在
// 关务自己的事务里记下、不随调用方回滚（ADR-0206 决定六）。补问那一拍在任何写事务之外问它（tfinbox 那两扇门的头注），这道门让
// 接错的那一次在第一条用例里就红，而不是把每一件都记成读不动。
type transactionalActionAdmissionJudge struct {
	db         *bentopg.DB
	transactor bentoapp.Transactor
	inner      *ccapplication.JudgeActionAdmissionHandler
}

var (
	errActionAdmissionInsideCallerTransaction = errors.New(
		"parcel-dispatch: action admission runs in its own transaction; ask before opening the caller's write transaction")
	// errActionAdmissionUnreadable 只在本壳里流转：编排答读不动时逼事务回滚而不提交，回滚之后交回编排那一份答复。理由同
	// cmd/parcel-api 那一份：读口在事务里报了 SQL 错误，整笔就进了中止态，提交只换回提交错误。
	errActionAdmissionUnreadable = errors.New("parcel-dispatch: action admission unreadable; transaction rolled back")
)

var _ tfcustoms.ActionAdmissionJudge = transactionalActionAdmissionJudge{}

func (judge transactionalActionAdmissionJudge) Handle(
	ctx context.Context,
	command ccapplication.JudgeActionAdmissionCommand,
) (ccapplication.JudgeActionAdmissionResult, error) {
	if _, err := judge.db.RequireExecutor(ctx); !errors.Is(err, bentopg.ErrTransactionRequired) {
		return ccapplication.JudgeActionAdmissionResult{}, errActionAdmissionInsideCallerTransaction
	}
	var result ccapplication.JudgeActionAdmissionResult
	err := judge.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		judged, judgeErr := judge.inner.Handle(txCtx, command)
		if judgeErr != nil {
			return judgeErr
		}
		result = judged
		if judged.Outcome() == ccapplication.ActionAdmissionUnreadable {
			return errActionAdmissionUnreadable
		}
		return nil
	})
	if errors.Is(err, errActionAdmissionUnreadable) {
		return result, nil
	}
	if err != nil {
		return ccapplication.JudgeActionAdmissionResult{}, err
	}
	return result, nil
}

// departureGateCustomsView 装补问问关务的那一口：关务的动作准入判断编排（与 cmd/parcel-api 的 buildCustomsAdmissionOrchestration
// 判断那一半同一批只读册与判断册），包进自带事务的壳，再经 TF 的消费侧适配器翻译。
func departureGateCustomsView(db *bentopg.DB, clock systemClock) (*tfcustoms.DepartureGateView, error) {
	zoneView, err := ccpostgres.NewNodeCustomsZonePointView(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: reask customs node zone view: %w", err)
	}
	scopes, err := ccpostgres.NewObjectCustomsScopeView(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: reask customs object scope view: %w", err)
	}
	restrictions, err := ccpostgres.NewRestrictions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: reask customs restrictions: %w", err)
	}
	gates, err := ccpostgres.NewGateConditionView(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: reask customs gate condition view: %w", err)
	}
	dutyVerifications, err := ccpostgres.NewDutyPaymentReconciliation(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: reask customs duty payment reconciliation: %w", err)
	}
	releases, err := ccpostgres.NewReleaseOutcomeView(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: reask customs release outcome view: %w", err)
	}
	judgements, err := ccpostgres.NewActionAdmissionJudgements(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: reask customs action admission judgements: %w", err)
	}
	judge, err := ccapplication.NewJudgeActionAdmissionHandler(ccapplication.JudgeActionAdmissionDeps{
		Zones:             zoneView,
		Scopes:            scopes,
		Restrictions:      restrictions,
		Conditions:        gates,
		DutyRules:         gates,
		DutyVerifications: dutyVerifications,
		Releases:          releases,
		Store:             judgements,
		Clock:             clock,
	})
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: reask customs action admission: %w", err)
	}
	view, err := tfcustoms.NewDepartureGateView(transactionalActionAdmissionJudge{db: db, transactor: db.Transactor(), inner: judge})
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: reask customs view: %w", err)
	}
	return view, nil
}

// departureGateReader 装补问走的读数路径：段册、班次册、交接册、节点作业的集运成员关系与收寄身份，加给定的关务那一口。交接与揽收上的
// 补问（departureGateReasks）和节点收寄形成信封上的成员补问（departureGateReaskOnNodeIntakeConsumerOver）都经它问，两条路因此是登记那
// 一刻同一条读数路径、同一份接法。
func departureGateReader(db *bentopg.DB, customs tfports.DepartureGateView) (*tfapplication.DepartureGateReader, error) {
	segments, err := tfpostgres.NewFulfillmentSegments(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: reask segments: %w", err)
	}
	schedules, err := tfpostgres.NewTransportSchedules(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: reask schedules: %w", err)
	}
	handovers, err := tfpostgres.NewTransportHandovers(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: reask handovers: %w", err)
	}
	memberships, err := nopostgres.NewConsolidationMembershipsAt(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: reask consolidation memberships: %w", err)
	}
	identities, err := nopostgres.NewIntakeIdentities(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: reask intake identities: %w", err)
	}
	members, err := tfnode.NewConsolidationMembershipView(memberships, identities)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: reask consolidation members: %w", err)
	}
	reader, err := tfapplication.NewDepartureGateReader(tfapplication.DepartureGateReadDeps{
		Segments:  segments,
		Schedules: schedules,
		Handovers: handovers,
		Members:   members,
		Customs:   customs,
	})
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: reask gate reader: %w", err)
	}
	return reader, nil
}

// departureGateReasks 把补问编排接到给定的关务那一口上。这一口之外的接线只有这一处：生产装配交真关务（departureGateCustomsView），
// 真库用例交按对象答格的替身，两边的段册、事实册、补问册、交出与读数路径是同一份接法。
func departureGateReasks(
	db *bentopg.DB,
	outboxStore *outbox.Store,
	clock systemClock,
	customs tfports.DepartureGateView,
) (*tfapplication.ReaskDepartureGatesHandler, error) {
	segments, err := tfpostgres.NewFulfillmentSegments(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: reask segments: %w", err)
	}
	handovers, err := tfpostgres.NewTransportHandovers(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: reask handovers: %w", err)
	}
	pickups, err := tfpostgres.NewOffsitePickupRegistrations(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: reask pickups: %w", err)
	}
	facts, err := tfpostgres.NewMovementFacts(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: reask movement facts: %w", err)
	}
	reasks, err := tfpostgres.NewDepartureGateReasks(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: reask registry: %w", err)
	}
	handoff, err := tfpostgres.NewOutboxDepartureGateReaskHandoff(db, outboxStore, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: reask handoff: %w", err)
	}
	reader, err := departureGateReader(db, customs)
	if err != nil {
		return nil, err
	}
	handler, err := tfapplication.NewReaskDepartureGatesHandler(tfapplication.ReaskDepartureGatesDeps{
		Handovers: handovers,
		Pickups:   pickups,
		Segments:  segments,
		Facts:     facts,
		Reasks:    reasks,
		Gate:      reader,
		Handoff:   handoff,
		Clock:     clock,
	})
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: reask departure gates: %w", err)
	}
	return handler, nil
}

// departureGateReaskConsumersOver 装交接与揽收两扇补问门，接到给定的关务那一口上。
func departureGateReaskConsumersOver(
	db *bentopg.DB,
	inboxStore *inbox.Store,
	outboxStore *outbox.Store,
	clock systemClock,
	customs tfports.DepartureGateView,
) (onHandover, onPickup dispatch.Consumer, err error) {
	handler, err := departureGateReasks(db, outboxStore, clock, customs)
	if err != nil {
		return nil, nil, err
	}
	handoverConsumer, err := tfinbox.NewDepartureGateReaskOnHandoverConsumer(db.Transactor(), inboxStore, handler)
	if err != nil {
		return nil, nil, fmt.Errorf("parcel-dispatch: reask departure gates on handover consumer: %w", err)
	}
	pickupConsumer, err := tfinbox.NewDepartureGateReaskOnPickupConsumer(db.Transactor(), inboxStore, handler)
	if err != nil {
		return nil, nil, fmt.Errorf("parcel-dispatch: reask departure gates on pickup consumer: %w", err)
	}
	return handoverConsumer, pickupConsumer, nil
}

// departureGateReaskConsumers 是生产装配：关务那一口接真判断。
func departureGateReaskConsumers(
	db *bentopg.DB,
	inboxStore *inbox.Store,
	outboxStore *outbox.Store,
	clock systemClock,
) (onHandover, onPickup dispatch.Consumer, err error) {
	customs, err := departureGateCustomsView(db, clock)
	if err != nil {
		return nil, nil, err
	}
	return departureGateReaskConsumersOver(db, inboxStore, outboxStore, clock, customs)
}

// raiseDepartureGateSignalOnReaskConsumer 接 TF 出发门禁补问 → VE 出发门禁阻断信号（ADR-0305）。按键回读补问与它补的那条出发，
// 信号借投影那一份租户现绑的派生（发作期库、身份签发与分诊意图交出），与移动事实那一路同一个写入方。
func raiseDepartureGateSignalOnReaskConsumer(
	db *bentopg.DB,
	inboxStore *inbox.Store,
	derive *tenantBoundProjectionDerive,
) (dispatch.Consumer, error) {
	reasks, err := tfpostgres.NewDepartureGateReasks(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: signal departure gate reasks: %w", err)
	}
	facts, err := tfpostgres.NewMovementFacts(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: signal movement facts: %w", err)
	}
	processing, err := vetf.NewRaiseOnDepartureGateReaskAdapter(reasks, facts, tenantBoundDepartureGateSignals{derive: derive})
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: raise on departure gate reask: %w", err)
	}
	consumer, err := veinbox.NewDepartureGateReaskConsumer(db.Transactor(), inboxStore, processing)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: departure gate reask signal consumer: %w", err)
	}
	return consumer, nil
}
