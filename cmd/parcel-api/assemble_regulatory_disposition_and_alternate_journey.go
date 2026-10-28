package main

import (
	"context"
	"errors"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// 本文件是监管运输处置承接与替代/退运旅程启动两口的组合根（票 docs-consistency/code/43，ADR-0337）。

// errDispositionAcceptanceUndecided、errAlternateJourneyUndecided 只在各自的壳里流转：编排答`未决`时回调交它，逼事务回滚；
// 回滚之后认出它，交回编排那一份答复。承接册或旅程册在事务里报了 SQL 错误，PostgreSQL 就把整笔置为 aborted，照常提交只换回
// 提交错误，线上落成 500 `NO_ANSWER_FORMED`，与 UC-TF-001、UC-TF-007 把`未决`写成「原样重交即可」的答复对不上；`未决`各格也没有
// 任何东西要落库。其余各格（待补充、纯节点动作、冲突、未受理）本就没有写入，提交与回滚无别。
var (
	errDispositionAcceptanceUndecided = errors.New("parcel-api: regulatory disposition acceptance undecided; transaction rolled back")
	errAlternateJourneyUndecided      = errors.New("parcel-api: alternate journey start undecided; transaction rolled back")
)

// transactionalRegulatoryDispositionAcceptor 把一次承接包进一笔事务：承接册的写口与回执的 Outbox 入队按框架合同无事务即拒，
// 事务边界归装配点；承接决定与「承接已记录」回执意图同笔落地（ADR-0043）。
type transactionalRegulatoryDispositionAcceptor struct {
	transactor bentoapp.Transactor
	inner      *tfapp.AcceptRegulatoryDispositionHandler
}

var _ tfhttp.RegulatoryDispositionAcceptor = transactionalRegulatoryDispositionAcceptor{}

func (acceptor transactionalRegulatoryDispositionAcceptor) Handle(
	ctx context.Context,
	command tfapp.AcceptRegulatoryDispositionCommand,
) (tfapp.AcceptDispositionResult, error) {
	var result tfapp.AcceptDispositionResult
	err := acceptor.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := acceptor.inner.Handle(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		if handled.Outcome() == tfapp.AcceptDispositionUndecided {
			return errDispositionAcceptanceUndecided
		}
		return nil
	})
	if errors.Is(err, errDispositionAcceptanceUndecided) {
		return result, nil
	}
	if err != nil {
		return tfapp.AcceptDispositionResult{}, err
	}
	return result, nil
}

// buildRegulatoryDispositionAcceptance 装配 `/transport-fulfillment-regulatory-disposition-acceptances` 背后的真编排。缝全接真：
// 承接册、承接回执的 Outbox 意图交付与时钟。协作事项本体不经读口：关务一侧今天没有按引用读回的提供方，事项的载运对象、移动动作、
// 起止责任由提交的运营人员抄录进载荷（ADR-0337 决定三）。
func buildRegulatoryDispositionAcceptance(db *bentopg.DB) (tfhttp.RegulatoryDispositionAcceptor, error) {
	acceptances, err := tfpostgres.NewDispositionAcceptances(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: disposition acceptances: %w", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: outbox store: %w", err)
	}
	clock := systemClock{}
	receipt, err := tfpostgres.NewOutboxRegulatoryAcceptanceHandoff(db, store, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: regulatory acceptance handoff: %w", err)
	}
	handler := tfapp.NewAcceptRegulatoryDispositionHandler(tfapp.AcceptRegulatoryDispositionDeps{
		Acceptances: acceptances,
		Receipt:     receipt,
		Clock:       clock,
	})
	return transactionalRegulatoryDispositionAcceptor{transactor: db.Transactor(), inner: handler}, nil
}

// transactionalAlternateJourneyStarter 把一次旅程启动包进一笔事务：旅程册的写口与两条意图链（监管来路的处置执行、异常链）的
// Outbox 入队按框架合同无事务即拒；旅程与意图同笔落地（ADR-0043）。
type transactionalAlternateJourneyStarter struct {
	transactor bentoapp.Transactor
	inner      *tfapp.StartAlternateJourneyHandler
}

var _ tfhttp.AlternateJourneyStarter = transactionalAlternateJourneyStarter{}

func (starter transactionalAlternateJourneyStarter) Handle(
	ctx context.Context,
	command tfapp.StartAlternateJourneyCommand,
) (tfapp.StartAlternateJourneyResult, error) {
	var result tfapp.StartAlternateJourneyResult
	err := starter.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := starter.inner.Handle(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		if handled.Outcome() == tfapp.JourneyUndecided {
			return errAlternateJourneyUndecided
		}
		return nil
	})
	if errors.Is(err, errAlternateJourneyUndecided) {
		return result, nil
	}
	if err != nil {
		return tfapp.StartAlternateJourneyResult{}, err
	}
	return result, nil
}

// buildAlternateJourneyStart 装配 `/transport-fulfillment-alternate-journey-starts` 背后的真编排。缝全接真：旅程册、监管来路的
// 处置执行意图交付、异常链意图交付与时钟。异常链信封由派发进程投给全程追踪投影（读一侧已接，UC-TF-005 `AT-TF-252`）。
func buildAlternateJourneyStart(db *bentopg.DB) (tfhttp.AlternateJourneyStarter, error) {
	journeys, err := tfpostgres.NewAlternateJourneys(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: alternate journeys: %w", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: outbox store: %w", err)
	}
	clock := systemClock{}
	disposition, err := tfpostgres.NewOutboxDispositionExecutionHandoff(db, store, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: disposition execution handoff: %w", err)
	}
	exception, err := tfpostgres.NewOutboxExceptionJourneyHandoff(db, store, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: exception journey handoff: %w", err)
	}
	handler := tfapp.NewStartAlternateJourneyHandler(tfapp.StartAlternateJourneyDeps{
		Journeys:    journeys,
		Disposition: disposition,
		Exception:   exception,
		Clock:       clock,
	})
	return transactionalAlternateJourneyStarter{transactor: db.Transactor(), inner: handler}, nil
}
