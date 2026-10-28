package main

import (
	"context"
	"errors"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 到点截单的驱动（ADR-0339 决定三）：派发器每一拍之后逐户截一遍。逐户一笔事务：这一户截了几期、记了几个截止日同生同灭，一户停在
// 依赖上不拖累别户。一拍里别的户照截，出错的那几户合进这一拍的错误，进程循环照记、下一拍再来。

// cycleCutBeat 是派发器外面的那一层。
type cycleCutBeat struct {
	dispatch   Beat
	cycles     *saapplication.CutOffStatementsOnCycleHandler
	transactor bentoapp.Transactor
}

func (beat cycleCutBeat) DispatchOnce(ctx context.Context) (int, error) {
	published, err := beat.dispatch.DispatchOnce(ctx)
	return published, errors.Join(err, beat.cutDueStatements(ctx))
}

func (beat cycleCutBeat) cutDueStatements(ctx context.Context) error {
	records, err := beat.cycles.Cycles(ctx)
	if err != nil {
		return err
	}
	var failures []error
	for _, record := range records {
		if err := beat.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			_, cutErr := beat.cycles.CutAccount(txCtx, record)
			return cutErr
		}); err != nil {
			failures = append(failures, fmt.Errorf("cycle cut %s/%s %s: %w", record.Tenant, record.Cycle.Account(), cycleCutFailureKind(err), err))
		}
	}
	return errors.Join(failures...)
}

// cycleCutFailureKind 给一户的失败分个类写进日志：等依赖的下一拍多半自愈，未配置要租户补登，其余是两侧说法矛盾、要人查。三类都
// 每拍再来一次，这一格只为让看日志的人分得开。
func cycleCutFailureKind(err error) string {
	switch {
	case errors.Is(err, saapplication.ErrCycleCutUndecided):
		return "stalled on a dependency"
	case errors.Is(err, saapplication.ErrCycleCutUnconfigured):
		return "tenant value missing"
	default:
		return "contradiction"
	}
}

// cycleStatementCutter 先问截单时点再截单发布，跑在调用方的事务里——与 parcel-api 的 settlementOrchestrations.CutOff 是同一道门、
// 同一只编排，只是不另开事务。
type cycleStatementCutter struct {
	moments *saapplication.AdmitSettlementMomentHandler
	cutoff  *saapplication.CutOffPublishStatementHandler
}

func (cutter cycleStatementCutter) CutOff(
	ctx context.Context,
	command saapplication.PublishStatementCommand,
) (saapplication.SettlementMomentOutcome, saapplication.StatementResult, error) {
	admitted, err := cutter.moments.Admit(ctx, command.TenantID, sadomain.SettlementMomentCutOff)
	if err != nil || admitted != saapplication.SettlementMomentAdmitted {
		return admitted, saapplication.StatementResult{}, err
	}
	result, err := cutter.cutoff.Publish(ctx, command)
	return admitted, result, err
}

// cutOffStatementsOnCycle 装到点截单。截单编排的依赖照 parcel-api 的 buildSettlementOrchestrations 那一段接，册与信封交接同一套。
func cutOffStatementsOnCycle(db *bentopg.DB, outboxStore *outbox.Store, clock systemClock) (*saapplication.CutOffStatementsOnCycleHandler, error) {
	charges, err := sapostgres.NewCustomerCharges(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: customer charges for cycle cut: %w", err)
	}
	statements, err := sapostgres.NewCustomerStatements(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: customer statements: %w", err)
	}
	adjustments, err := sapostgres.NewChargeAdjustments(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: charge adjustments: %w", err)
	}
	inclusions, err := sapostgres.NewSubsequentInclusions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: subsequent inclusions: %w", err)
	}
	disputes, err := sapostgres.NewStatementDisputes(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: statement disputes: %w", err)
	}
	statementHandoff, err := sapostgres.NewOutboxStatementHandoff(db, outboxStore, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: statement handoff: %w", err)
	}
	attributionForms, err := sapostgres.NewChargeAttributions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: charge attributions: %w", err)
	}
	arrangements, err := sapostgres.NewStatementArrangements(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: statement arrangements: %w", err)
	}
	attributions, err := saapplication.NewJudgeChargeAttributionHandler(attributionForms, arrangements)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: judge charge attribution: %w", err)
	}
	cutoff, err := saapplication.NewCutOffPublishStatementHandler(saapplication.CutOffPublishStatementDeps{
		Statements: statements, Charges: charges, Billing: statements, Attributions: attributions,
		AttributionDates: charges, Adjustments: adjustments, Inclusions: inclusions,
		Disputes: disputes, Downstream: statementHandoff, Clock: clock,
	})
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: cut-off publish statement: %w", err)
	}
	momentStore, err := sapostgres.NewSettlementMoments(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: settlement moments: %w", err)
	}
	moments, err := saapplication.NewAdmitSettlementMomentHandler(momentStore)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: admit settlement moment: %w", err)
	}
	cycles, err := sapostgres.NewStatementCycles(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: statement cycles: %w", err)
	}
	accounts, err := sapostgres.NewSettlementAccounts(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: settlement accounts for cycle cut: %w", err)
	}
	handler, err := saapplication.NewCutOffStatementsOnCycleHandler(saapplication.CutOffStatementsOnCycleDeps{
		Ledger: cycles, Candidates: cycles, Arrangements: arrangements, Accounts: accounts, Moments: moments,
		Charges: charges, Attributions: attributions, AttributionDates: charges,
		Cutter: cycleStatementCutter{moments: moments, cutoff: cutoff}, Clock: clock,
	})
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: cut off statements on cycle: %w", err)
	}
	return handler, nil
}
