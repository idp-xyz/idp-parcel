// cut_off_statements_on_cycle.go 是到点截单（ADR-0339 决定三）：登了对账周期的一户，每个已经关上的截止日截一期——收拢已确认、
// 归属日不晚于截止日、尚未出账的费用，合计照它们的结算金额加总，交给截单发布那一只编排。截单仍先过截单时点门；本期没有费用的
// 截止日只记一笔、不发单。周期、对账安排、截单时点任一没登都不截：那是租户取值缺席，不代登。
package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// ErrCycleCutUnconfigured 说明这一户的租户取值缺了一格（对账安排、或某笔已确认费用的归属日形态），到点截单在这一户停住、
// 不记截止日。与等依赖分开：补登之前每拍都会停在同一格，调用方据它记成未配置而不是故障。
var ErrCycleCutUnconfigured = errors.New("settlement accounting: cycle cut stopped on a missing tenant value")

// ErrCycleCutUndecided 说明这一户这一拍停在依赖上（读写不动、截单未决、对账单信封没交出去）：调用方回滚这一户的事务，下一拍
// 再截。
var ErrCycleCutUndecided = errors.New("settlement accounting: cycle cut stalled on a dependency")

// StatementCutter 是截单发布那一口：先问截单时点，再截单发布。到点截单与运营发布口进的是同一道门、同一只编排。调用方给环境事务。
type StatementCutter interface {
	CutOff(ctx context.Context, command PublishStatementCommand) (SettlementMomentOutcome, StatementResult, error)
}

// CycleCutOutcome 是一户这一拍的答案。
type CycleCutOutcome uint8

const (
	CycleCutOutcomeInvalid CycleCutOutcome = iota
	// CycleCutArrangementUnconfigured：这一户没登对账安排，判不出截止日什么时候关上。
	CycleCutArrangementUnconfigured
	// CycleCutMomentNotAdmitted：这一户没登截单时点（或时点门不受理），不截，也不记截止日。
	CycleCutMomentNotAdmitted
	// CycleCutAttributionUnconfigured：有已确认费用的归属日判不出（缺对账安排或归属日形态），不截，也不记截止日。
	CycleCutAttributionUnconfigured
	// CycleCutDone：已关上的截止日都截过了（含这一拍一个都没有到点的）。
	CycleCutDone
)

func (outcome CycleCutOutcome) String() string {
	switch outcome {
	case CycleCutArrangementUnconfigured:
		return "CYCLE_ARRANGEMENT_UNCONFIGURED"
	case CycleCutMomentNotAdmitted:
		return "CYCLE_MOMENT_NOT_ADMITTED"
	case CycleCutAttributionUnconfigured:
		return "CYCLE_ATTRIBUTION_UNCONFIGURED"
	case CycleCutDone:
		return "CYCLE_CUT_DONE"
	default:
		return ""
	}
}

// cycleCutNoCharge 是截止日那一期一笔能收的费用都没有时记下的结果。
const cycleCutNoCharge = "NO_CHARGE"

type CycleCutResult struct {
	outcome CycleCutOutcome
	cuts    []ports.StatementCycleCut
}

func (result CycleCutResult) Outcome() CycleCutOutcome { return result.outcome }

// Cuts 是这一拍记下的各个截止日。
func (result CycleCutResult) Cuts() []ports.StatementCycleCut { return result.cuts }

type CutOffStatementsOnCycleDeps struct {
	Ledger       ports.StatementCycleLedger
	Candidates   ports.CycleCutCandidateView
	Arrangements ports.StatementArrangementView
	Accounts     ports.SettlementAccountByIDView
	// Moments 是截单时点门：一户没登截单时点，什么都不做、不记截止日。
	Moments *AdmitSettlementMomentHandler
	// Charges、Attributions、AttributionDates 给确认时判不出归属日的费用当场补判（CONTEXT「截单时当场判、判出即落库」），与截单
	// 编排判同一只、落同一格。
	Charges          ports.CustomerChargeStore
	Attributions     *JudgeChargeAttributionHandler
	AttributionDates ports.ChargeAttributionDateRecorder
	Cutter           StatementCutter
	Clock            ports.Clock
}

type CutOffStatementsOnCycleHandler struct {
	deps CutOffStatementsOnCycleDeps
}

func NewCutOffStatementsOnCycleHandler(deps CutOffStatementsOnCycleDeps) (*CutOffStatementsOnCycleHandler, error) {
	if deps.Ledger == nil || deps.Candidates == nil || deps.Arrangements == nil || deps.Accounts == nil ||
		deps.Moments == nil || deps.Charges == nil || deps.Attributions == nil || deps.AttributionDates == nil ||
		deps.Cutter == nil || deps.Clock == nil {
		return nil, fmt.Errorf("%w: cut off statements on cycle", ErrNilDependency)
	}
	return &CutOffStatementsOnCycleHandler{deps: deps}, nil
}

// Cycles 交回册上各户的周期，给调用方逐户开事务。
func (handler *CutOffStatementsOnCycleHandler) Cycles(ctx context.Context) ([]ports.StatementCycleRecord, error) {
	records, err := handler.deps.Ledger.ListStatementCycles(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: statement cycle register: %w", ErrCycleCutUndecided, err)
	}
	return records, nil
}

// CutAccount 截一户：此刻已关上、还没截过的截止日按先后各截一期。截单停在依赖上就交回 ErrCycleCutUndecided，这一户整笔回滚，
// 已截的那几期也一起撤回、下一拍重截——截单编排对同一份命令重放答已存在，不会多出单。
func (handler *CutOffStatementsOnCycleHandler) CutAccount(
	ctx context.Context,
	record ports.StatementCycleRecord,
) (CycleCutResult, error) {
	tenant, cycle := record.Tenant, record.Cycle
	moment, err := handler.deps.Moments.Admit(ctx, tenant, domain.SettlementMomentCutOff)
	if err != nil {
		return CycleCutResult{}, err
	}
	switch moment {
	case SettlementMomentAdmitted:
	case SettlementMomentViewUnavailable:
		return CycleCutResult{}, fmt.Errorf("%w: settlement moment register unavailable", ErrCycleCutUndecided)
	default:
		return CycleCutResult{outcome: CycleCutMomentNotAdmitted}, nil
	}
	arrangement, found, err := handler.deps.Arrangements.LoadStatementArrangement(ctx, tenant, cycle.Account())
	if err != nil {
		return CycleCutResult{}, fmt.Errorf("%w: statement arrangement: %w", ErrCycleCutUndecided, err)
	}
	if !found {
		return CycleCutResult{outcome: CycleCutArrangementUnconfigured},
			fmt.Errorf("%w: account %s has a statement cycle but no statement arrangement", ErrCycleCutUnconfigured, cycle.Account())
	}
	if stopped, err := handler.attributePending(ctx, tenant, cycle.Account()); err != nil || stopped {
		return CycleCutResult{outcome: CycleCutAttributionUnconfigured}, err
	}
	account, found, err := handler.deps.Accounts.FindByID(ctx, tenant, cycle.Account())
	if err != nil {
		return CycleCutResult{}, fmt.Errorf("%w: settlement account register: %w", ErrCycleCutUndecided, err)
	}
	if !found {
		// 周期行对账户有外键：读不到是两侧说法矛盾。
		return CycleCutResult{}, fmt.Errorf("settlement accounting: statement cycle names account %s absent from the register", cycle.Account())
	}
	now := handler.deps.Clock.Now()
	latest, err := arrangement.LatestClosableDate(now)
	if err != nil {
		return CycleCutResult{}, err
	}
	last, cutBefore, err := handler.deps.Ledger.LastCycleCut(ctx, tenant, cycle.Account())
	if err != nil {
		return CycleCutResult{}, fmt.Errorf("%w: statement cycle cut ledger: %w", ErrCycleCutUndecided, err)
	}
	if !cutBefore {
		last = domain.AttributionDate{}
	}
	result := CycleCutResult{outcome: CycleCutDone}
	for _, closing := range cycle.ClosingsAfter(last, latest) {
		cut, admitted, err := handler.cutClosing(ctx, tenant, account, arrangement, closing, now)
		if err != nil {
			return CycleCutResult{}, err
		}
		if !admitted {
			return CycleCutResult{outcome: CycleCutMomentNotAdmitted, cuts: result.cuts}, nil
		}
		if err := handler.deps.Ledger.RecordCycleCut(ctx, tenant, cycle.Account(), cut); err != nil {
			return CycleCutResult{}, fmt.Errorf("%w: statement cycle cut ledger: %w", ErrCycleCutUndecided, err)
		}
		result.cuts = append(result.cuts, cut)
	}
	return result, nil
}

func (handler *CutOffStatementsOnCycleHandler) cutClosing(
	ctx context.Context,
	tenant domain.TenantID,
	account domain.SettlementAccount,
	arrangement domain.StatementArrangement,
	closing domain.AttributionDate,
	now time.Time,
) (ports.StatementCycleCut, bool, error) {
	currency := account.Key().Currency()
	candidates, err := handler.deps.Candidates.ListCycleCutCandidates(ctx, tenant, account.ID(), currency, closing)
	if err != nil {
		return ports.StatementCycleCut{}, false, fmt.Errorf("%w: cycle cut candidates: %w", ErrCycleCutUndecided, err)
	}
	if len(candidates) == 0 {
		return ports.StatementCycleCut{Closing: closing, Outcome: cycleCutNoCharge, CutAt: now}, true, nil
	}
	cutOffAt, err := arrangement.ClosingInstant(closing)
	if err != nil {
		return ports.StatementCycleCut{}, false, err
	}
	command := PublishStatementCommand{
		TenantID: tenant,
		// 单号与账期都从截止日派生：同一户同一个截止日只会有一张单，重截一次是同一份命令的重放。
		Number:              cycleStatementNumber(account.ID(), closing),
		Account:             account.ID().String(),
		Period:              "CYCLE/" + closing.String(),
		Version:             "v1",
		Currency:            currency.String(),
		LastAttributionDate: closing.String(),
		CutOffAt:            cutOffAt,
		PublishedAt:         now,
	}
	for _, candidate := range candidates {
		command.ChargeIDs = append(command.ChargeIDs, candidate.Charge.String())
		command.DeclaredTotalMinor += candidate.Minor
	}
	moment, published, err := handler.deps.Cutter.CutOff(ctx, command)
	if err != nil {
		return ports.StatementCycleCut{}, false, err
	}
	switch moment {
	case SettlementMomentAdmitted:
	case SettlementMomentViewUnavailable:
		return ports.StatementCycleCut{}, false, fmt.Errorf("%w: settlement moment register unavailable", ErrCycleCutUndecided)
	default:
		return ports.StatementCycleCut{}, false, nil
	}
	if published.Outcome().String() == "" {
		return ports.StatementCycleCut{}, false, fmt.Errorf("settlement accounting: statement cut answered an unnamed outcome %d", published.Outcome())
	}
	if published.Outcome() == StatementUndecided {
		return ports.StatementCycleCut{}, false, fmt.Errorf("%w: statement cut %s", ErrCycleCutUndecided, published.UndecidedReason())
	}
	if published.StatementHandoffReference() != "" {
		return ports.StatementCycleCut{}, false, fmt.Errorf("%w: statement handoff pending %s", ErrCycleCutUndecided,
			published.StatementHandoffReference())
	}
	// 发布、已存在之外的格（勾稽不平、单号冲突、本期无费用等）照记：截止日记下了就不再每拍重问，人从截止记录与对账单读口看到它。
	cut := ports.StatementCycleCut{Closing: closing, Outcome: published.Outcome().String(), CutAt: now}
	if record, has := published.Statement(); has {
		cut.Statement = record.Statement.Number().String()
	}
	return cut, true, nil
}

func cycleStatementNumber(account domain.SettlementAccountID, closing domain.AttributionDate) string {
	return "STMT/" + account.String() + "/" + strings.ReplaceAll(closing.String(), "-", "")
}

// attributePending 给这一户确认时判不出归属日的费用当场补判，判出即落库，此后照常进候选。判不出是缺册：这一户这一拍停住、不记
// 截止日，交回 ErrCycleCutUnconfigured；形态要的时点费用上没有，补登哪一册都补不上，跳过它（截单编排对它也只逐件排除）。
func (handler *CutOffStatementsOnCycleHandler) attributePending(
	ctx context.Context,
	tenant domain.TenantID,
	account domain.SettlementAccountID,
) (bool, error) {
	pending, err := handler.deps.Candidates.ListUnattributedConfirmedCharges(ctx, tenant, account)
	if err != nil {
		return false, fmt.Errorf("%w: unattributed charges: %w", ErrCycleCutUndecided, err)
	}
	for _, id := range pending {
		charge, found, err := handler.deps.Charges.FindByID(ctx, tenant, id)
		if err != nil {
			return false, fmt.Errorf("%w: customer charge store: %w", ErrCycleCutUndecided, err)
		}
		if !found {
			continue
		}
		judged := handler.deps.Attributions.JudgeConfirmedCharge(ctx, tenant, charge)
		switch judged.Outcome() {
		case ChargeAttributionJudged:
		case ChargeAttributionUnconfigured:
			return true, fmt.Errorf("%w: charge %s attribution date: missing %v", ErrCycleCutUnconfigured, id, judged.Missing())
		case ChargeAttributionViewUnavailable:
			return false, fmt.Errorf("%w: attribution view unavailable", ErrCycleCutUndecided)
		default:
			continue
		}
		date, _ := judged.Date()
		if _, err := handler.deps.AttributionDates.RecordAttributionDate(ctx, tenant, id, date); err != nil {
			return false, fmt.Errorf("%w: attribution date: %w", ErrCycleCutUndecided, err)
		}
	}
	return false, nil
}
