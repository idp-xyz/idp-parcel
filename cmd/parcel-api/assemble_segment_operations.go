package main

import (
	"context"
	"errors"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// TF 四个 admin 写面的装配（票 tf-segment-lifecycle-closure/07）：关段、建派送任务、装载分配、明确终止
// 参与。四条各包一笔事务，理由随 transactionalDelivery；四个包装类型分立，装配测试才盖得住「某一口接错了
// 编排」。没有意图交付：四条都是本上下文内的运营决定，不向任何下游交意图。

type transactionalSegmentCloser struct {
	transactor bentoapp.Transactor
	inner      *tfapp.CloseFulfillmentSegmentHandler
}

var _ tfhttp.SegmentCloser = transactionalSegmentCloser{}

func (closer transactionalSegmentCloser) Close(
	ctx context.Context,
	command tfapp.CloseFulfillmentSegmentCommand,
) (tfapp.CloseFulfillmentSegmentResult, error) {
	var result tfapp.CloseFulfillmentSegmentResult
	err := closer.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := closer.inner.Close(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return tfapp.CloseFulfillmentSegmentResult{}, err
	}
	return result, nil
}

type transactionalDispatchTaskOpener struct {
	transactor bentoapp.Transactor
	inner      *tfapp.OpenDispatchTaskHandler
}

var _ tfhttp.DispatchTaskOpener = transactionalDispatchTaskOpener{}

func (opener transactionalDispatchTaskOpener) Open(
	ctx context.Context,
	command tfapp.OpenDispatchTaskCommand,
) (tfapp.OpenDispatchTaskResult, error) {
	var result tfapp.OpenDispatchTaskResult
	err := opener.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := opener.inner.Open(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return tfapp.OpenDispatchTaskResult{}, err
	}
	return result, nil
}

// transactionalLoadAssigner 把装载分配包进一笔事务：容量保障在这笔里锁住班次上的各池行、读预占与分配关系、判守恒，
// 再落版本与分配量（ADR-0203 决定二），同一个池上并发的分配、预占与释放因此串行判。
//
// 编排答`未决`时整笔回滚再交回那一份答复，理由随 transactionalCapacityDecisions：读池在事务里报了 SQL 错误，整笔事务
// 就已置为 aborted，照常提交只换回提交错误、落成 500；`未决`也没有任何东西要落库。
type transactionalLoadAssigner struct {
	transactor bentoapp.Transactor
	inner      *tfapp.FormLoadAssignmentHandler
}

// errLoadAssignmentUndecided 只在本壳里流转：编排答`未决`时回调交它，逼事务回滚；回滚之后认出它，交回编排那一份答复。
var errLoadAssignmentUndecided = errors.New("parcel-api: load assignment undecided; transaction rolled back")

var _ tfhttp.LoadAssigner = transactionalLoadAssigner{}

func (assigner transactionalLoadAssigner) Form(
	ctx context.Context,
	command tfapp.FormLoadAssignmentCommand,
) (tfapp.FormLoadAssignmentResult, error) {
	var result tfapp.FormLoadAssignmentResult
	err := assigner.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := assigner.inner.Form(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		if handled.Outcome() == tfapp.LoadAssignmentUndecided {
			return errLoadAssignmentUndecided
		}
		return nil
	})
	if errors.Is(err, errLoadAssignmentUndecided) {
		return result, nil
	}
	if err != nil {
		return tfapp.FormLoadAssignmentResult{}, err
	}
	return result, nil
}

type transactionalParticipationEnder struct {
	transactor bentoapp.Transactor
	inner      *tfapp.EndFulfillmentParticipationHandler
}

var _ tfhttp.ParticipationEnder = transactionalParticipationEnder{}

func (ender transactionalParticipationEnder) End(
	ctx context.Context,
	command tfapp.EndFulfillmentParticipationCommand,
) (tfapp.EndFulfillmentParticipationResult, error) {
	var result tfapp.EndFulfillmentParticipationResult
	err := ender.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := ender.inner.End(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return tfapp.EndFulfillmentParticipationResult{}, err
	}
	return result, nil
}

// segmentOperations 是四条写面编排一次装配的产物；四个字段分收，理由同 controlFactOrchestrations。
type segmentOperations struct {
	closer  tfhttp.SegmentCloser
	opener  tfhttp.DispatchTaskOpener
	assign  tfhttp.LoadAssigner
	enderOf tfhttp.ParticipationEnder
}

// buildSegmentOperations 装配 `/transport-fulfillment-segment-closures`、`/transport-fulfillment-dispatch-task-registrations`、
// `/transport-fulfillment-load-assignment-registrations` 与 `/transport-fulfillment-participation-terminations` 背后的真编排。
//
// 缝全接真：段登记册、实际承运商判断登记册（终止编排「进下一段」立新段时同笔铸首版）、派送任务登记册、
// 装载分配登记册、班次登记（装载分配核班次在册，只读）与容量池册（装载分配按班次读池、锁池行判容量保障，不换存池）、
// 交接登记册与交付登记库（终止那一路不读后两个，但 EndFulfillmentParticipationDeps 是一份，装配点不为一个口做半份
// Deps）、时钟。
func buildSegmentOperations(db *bentopg.DB) (segmentOperations, error) {
	segments, err := tfpostgres.NewFulfillmentSegments(db)
	if err != nil {
		return segmentOperations{}, fmt.Errorf("parcel-api: fulfillment segments: %w", err)
	}
	judgments, err := tfpostgres.NewActualCarrierJudgments(db)
	if err != nil {
		return segmentOperations{}, fmt.Errorf("parcel-api: actual carrier judgments: %w", err)
	}
	tasks, err := tfpostgres.NewDispatchTasks(db)
	if err != nil {
		return segmentOperations{}, fmt.Errorf("parcel-api: dispatch tasks: %w", err)
	}
	assignments, err := tfpostgres.NewLoadAssignments(db)
	if err != nil {
		return segmentOperations{}, fmt.Errorf("parcel-api: load assignments: %w", err)
	}
	schedules, err := tfpostgres.NewTransportSchedules(db)
	if err != nil {
		return segmentOperations{}, fmt.Errorf("parcel-api: transport schedules: %w", err)
	}
	pools, err := tfpostgres.NewCapacityPools(db)
	if err != nil {
		return segmentOperations{}, fmt.Errorf("parcel-api: capacity pools: %w", err)
	}
	handovers, err := tfpostgres.NewTransportHandovers(db)
	if err != nil {
		return segmentOperations{}, fmt.Errorf("parcel-api: transport handovers: %w", err)
	}
	deliveries, err := tfpostgres.NewEffectiveDeliveries(db)
	if err != nil {
		return segmentOperations{}, fmt.Errorf("parcel-api: effective deliveries: %w", err)
	}
	clock := systemClock{}
	transactor := db.Transactor()
	return segmentOperations{
		closer: transactionalSegmentCloser{
			transactor: transactor,
			inner:      tfapp.NewCloseFulfillmentSegmentHandler(tfapp.CloseFulfillmentSegmentDeps{Segments: segments}),
		},
		opener: transactionalDispatchTaskOpener{
			transactor: transactor,
			inner:      tfapp.NewOpenDispatchTaskHandler(tfapp.OpenDispatchTaskDeps{Tasks: tasks, Clock: clock}),
		},
		assign: transactionalLoadAssigner{
			transactor: transactor,
			inner: tfapp.NewFormLoadAssignmentHandler(tfapp.FormLoadAssignmentDeps{
				Assignments: assignments,
				Schedules:   schedules,
				Pools:       pools,
				Clock:       clock,
			}),
		},
		enderOf: transactionalParticipationEnder{
			transactor: transactor,
			inner: tfapp.NewEndFulfillmentParticipationHandler(tfapp.EndFulfillmentParticipationDeps{
				Segments:   segments,
				Judgments:  judgments,
				Handovers:  handovers,
				Deliveries: deliveries,
				Clock:      clock,
			}),
		},
	}, nil
}
