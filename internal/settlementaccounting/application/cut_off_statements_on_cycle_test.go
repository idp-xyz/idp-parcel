package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 钉到点截单的编排分格（ADR-0339 决定三）：对账安排没登不截；截止日按周期与「此刻已关上的最后一天」推，从截止记录之后起；本期
// 没有费用只记截止日；有费用就照候选组一份截单命令——单号与账期从截止日派生、合计是候选的结算金额之和、截单时刻是截止日的截单
// 时刻；截单时点没登不截也不记；时点册读不动与截单未名都交回错误。发布那一格由 cmd/parcel-dispatch 的真库用例钉。

type cycleLedgerDouble struct {
	last     domain.AttributionDate
	hasLast  bool
	recorded []ports.StatementCycleCut
}

func (double *cycleLedgerDouble) ListStatementCycles(context.Context) ([]ports.StatementCycleRecord, error) {
	return nil, nil
}

func (double *cycleLedgerDouble) LastCycleCut(context.Context, domain.TenantID, domain.SettlementAccountID) (domain.AttributionDate, bool, error) {
	return double.last, double.hasLast, nil
}

func (double *cycleLedgerDouble) RecordCycleCut(_ context.Context, _ domain.TenantID, _ domain.SettlementAccountID, cut ports.StatementCycleCut) error {
	double.recorded = append(double.recorded, cut)
	return nil
}

type cycleCandidatesDouble struct {
	byClosing map[string][]ports.CycleCutCandidate
}

func (double cycleCandidatesDouble) ListUnattributedConfirmedCharges(
	context.Context, domain.TenantID, domain.SettlementAccountID,
) ([]domain.CustomerChargeID, error) {
	return nil, nil
}

func (double cycleCandidatesDouble) ListCycleCutCandidates(
	_ context.Context, _ domain.TenantID, _ domain.SettlementAccountID, _ domain.CurrencyCode, through domain.AttributionDate,
) ([]ports.CycleCutCandidate, error) {
	return double.byClosing[through.String()], nil
}

type cycleCutterDouble struct {
	moment   application.SettlementMomentOutcome
	commands []application.PublishStatementCommand
}

func (double *cycleCutterDouble) CutOff(
	_ context.Context, command application.PublishStatementCommand,
) (application.SettlementMomentOutcome, application.StatementResult, error) {
	double.commands = append(double.commands, command)
	return double.moment, application.StatementResult{}, nil
}

type cycleFixture struct {
	ledger       *cycleLedgerDouble
	arrangements *arrangementsDouble
	cutter       *cycleCutterDouble
	candidates   cycleCandidatesDouble
	handler      *application.CutOffStatementsOnCycleHandler
	record       ports.StatementCycleRecord
}

// newCycleFixture：日截、起算 10-27，上海 18:00 截单，此刻 10-31 11:00Z（上海 19:00），所以已关上的最后一天是 10-31。
func newCycleFixture(t *testing.T, candidates map[string][]ports.CycleCutCandidate, moment application.SettlementMomentOutcome) *cycleFixture {
	t.Helper()
	account := billValue(t, domain.NewSettlementAccountID, "SYN-ACCT-1")
	since, _ := domain.ParseAttributionDate("2026-10-27")
	cycle, err := domain.NewStatementCycle(account, domain.DailyCycle, nil, nil, since)
	if err != nil {
		t.Fatal(err)
	}
	arrangement, err := domain.NewStatementArrangement(account, "Asia/Shanghai", 18*60)
	if err != nil {
		t.Fatal(err)
	}
	fixture := &cycleFixture{
		ledger:       &cycleLedgerDouble{},
		arrangements: newArrangementsDouble(),
		cutter:       &cycleCutterDouble{moment: moment},
		candidates:   cycleCandidatesDouble{byClosing: candidates},
		record:       ports.StatementCycleRecord{Tenant: confirmCommand(t).TenantID, Cycle: cycle},
	}
	fixture.arrangements.arrangements[account.String()] = arrangement
	handler, err := application.NewCutOffStatementsOnCycleHandler(application.CutOffStatementsOnCycleDeps{
		Ledger: fixture.ledger, Candidates: fixture.candidates, Arrangements: fixture.arrangements,
		Accounts: accountByIDDouble{account: cycleAccount(t, account), found: true},
		Moments:  newMomentHandler(t, momentViewOf(moment)), Charges: newChargeStore(),
		Attributions: newAttributionJudge(t, newAttributionFormsDouble(), fixture.arrangements), AttributionDates: newChargeStore(),
		Cutter: fixture.cutter, Clock: confirmClock{at: time.Date(2026, 10, 31, 11, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.handler = handler
	return fixture
}

func cycleAccount(t *testing.T, id domain.SettlementAccountID) domain.SettlementAccount {
	t.Helper()
	key, err := domain.NewSettlementAccountKey(billValue(t, domain.NewLegalEntityReference, "SYN-LE-1"),
		billValue(t, domain.NewSettlementCounterpartyReference, "SYN-SHIPPER-1"), domain.ChargeReceivable,
		billValue(t, domain.NewCurrencyCode, "CNY"), billValue(t, domain.NewSettlementPolicyReference, "SYN-POLICY-1"))
	if err != nil {
		t.Fatal(err)
	}
	account, err := domain.NewSettlementAccount(id, key, domain.SettlementCounterpartyReference{}, false,
		billValue(t, domain.NewResponsibilityBasis, "SYN-CONTRACT-1"))
	if err != nil {
		t.Fatal(err)
	}
	return account
}

// Covers: AT-SA-224——没有费用的截止日只记一笔；有费用的那一天组一份截单命令，各格从截止日与候选派生；截止记录之后才起截。
func TestACycleCutRecordsEmptyClosingsAndComposesTheCommandFromTheCandidates(t *testing.T) {
	charge := billValue(t, domain.NewCustomerChargeID, "SYN-CHARGE-1")
	other := billValue(t, domain.NewCustomerChargeID, "SYN-CHARGE-2")
	fixture := newCycleFixture(t, map[string][]ports.CycleCutCandidate{
		"2026-10-30": {{Charge: charge, Minor: 9600}, {Charge: other, Minor: 400}},
	}, application.SettlementMomentAdmitted)
	fixture.ledger.last, _ = domain.ParseAttributionDate("2026-10-28")
	fixture.ledger.hasLast = true

	// 截单器替身交回零值结果：这一格答未名，交回错误；在那之前 10-29 已照无费用记下。
	_, err := fixture.handler.CutAccount(t.Context(), fixture.record)
	if err == nil {
		t.Fatal("截单答了未名的结果却没交回错误")
	}
	if len(fixture.ledger.recorded) != 1 || fixture.ledger.recorded[0].Closing.String() != "2026-10-29" ||
		fixture.ledger.recorded[0].Outcome != "NO_CHARGE" {
		t.Fatalf("截止记录 = %+v，want 只记 10-29 无费用", fixture.ledger.recorded)
	}
	if len(fixture.cutter.commands) != 1 {
		t.Fatalf("截单命令 = %d 份", len(fixture.cutter.commands))
	}
	command := fixture.cutter.commands[0]
	if command.Number != "STMT/SYN-ACCT-1/20261030" || command.Period != "CYCLE/2026-10-30" || command.Version != "v1" ||
		command.Currency != "CNY" || command.LastAttributionDate != "2026-10-30" || command.DeclaredTotalMinor != 10000 ||
		len(command.ChargeIDs) != 2 || !command.CutOffAt.Equal(time.Date(2026, 10, 30, 10, 0, 0, 0, time.UTC)) ||
		!command.PublishedAt.Equal(time.Date(2026, 10, 31, 11, 0, 0, 0, time.UTC)) {
		t.Fatalf("截单命令 = %+v", command)
	}
}

// Covers: AT-SA-224——截单时点没登：什么都不做、不记截止日；对账安排没登：报未配置；时点册读不动交回等依赖。
func TestACycleCutStopsWhereATenantValueIsMissing(t *testing.T) {
	charge := billValue(t, domain.NewCustomerChargeID, "SYN-CHARGE-1")
	withCharge := map[string][]ports.CycleCutCandidate{"2026-10-28": {{Charge: charge, Minor: 9600}}}

	unadopted := newCycleFixture(t, withCharge, application.SettlementMomentUnconfigured)
	result, err := unadopted.handler.CutAccount(t.Context(), unadopted.record)
	if err != nil || result.Outcome() != application.CycleCutMomentNotAdmitted ||
		len(unadopted.ledger.recorded) != 0 || len(unadopted.cutter.commands) != 0 {
		t.Fatalf("没登截单时点 = %s err = %v，记了 %v、截了 %d 份，want 什么都不做", result.Outcome(), err,
			unadopted.ledger.recorded, len(unadopted.cutter.commands))
	}

	unarranged := newCycleFixture(t, withCharge, application.SettlementMomentAdmitted)
	unarranged.arrangements.arrangements = map[string]domain.StatementArrangement{}
	if result, err := unarranged.handler.CutAccount(t.Context(), unarranged.record); !errors.Is(err, application.ErrCycleCutUnconfigured) ||
		result.Outcome() != application.CycleCutArrangementUnconfigured || len(unarranged.cutter.commands) != 0 {
		t.Fatalf("没登对账安排 = %s err = %v", result.Outcome(), err)
	}

	stalled := newCycleFixture(t, withCharge, application.SettlementMomentViewUnavailable)
	if _, err := stalled.handler.CutAccount(t.Context(), stalled.record); !errors.Is(err, application.ErrCycleCutUndecided) {
		t.Fatalf("时点册读不动 err = %v", err)
	}
}

// momentViewOf 把截单时点门要答的那一格换成时点册的样子：受理即册上有，读不动即册读不动，其余即册上没有。
func momentViewOf(moment application.SettlementMomentOutcome) settlementMomentViewDouble {
	switch moment {
	case application.SettlementMomentAdmitted:
		return settlementMomentViewDouble{found: true}
	case application.SettlementMomentViewUnavailable:
		return settlementMomentViewDouble{err: errors.New("synthetic moment register down")}
	default:
		return settlementMomentViewDouble{}
	}
}
