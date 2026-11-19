package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 钉复磅后新评价合计为零的那条路在真库上的排队与落点：链上前一笔未确认的费用被记为替代、没有后继，确认与它排成一列；
// 之后一代形成新预估时补上后继。夹具与 charge_stage_lock_test.go 共用，非零额的那一对用例在那里。夹具全是合成串。

// makeTotalZero 让第二代评价在结算币上合计为零：评价替身与形成编排收到的采用快照一起改。
func (fixture *stageLockFixture) makeTotalZero() {
	fixture.adoption.OriginalMinor, fixture.adoption.SettlementMinor = 0, 0
	fixture.evaluations.adoption = fixture.adoption
}

// resumeGeneration 在此刻最新那一代之后再提交一代请求，评价合计 minor；之后 reweighCommand 与 successor 指向这一代。
func (fixture *stageLockFixture) resumeGeneration(t *testing.T, request, basis, evaluation string, minor int64) {
	t.Helper()
	resumeBasis, err := domain.NewMeasurementChangeResumeBasis(basis)
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := fixture.next.Request.Resume(saValue(t, domain.NewSellEvaluationRequestID, request), resumeBasis)
	if err != nil {
		t.Fatal(err)
	}
	record := ports.SellEvaluationRequestRecord{
		Key:        ports.SellEvaluationRequestKey{TenantID: fixture.tenant, Request: resumed.ID()},
		Request:    resumed,
		RecordedAt: fixture.next.RecordedAt.Add(time.Hour),
		State:      domain.SellRequestSubmitted,
	}
	saWithin(t, fixture.db.Transactor(), t.Context(), func(ctx context.Context) error {
		_, err := fixture.requests.Save(ctx, record)
		return err
	})
	fixture.next = record
	fixture.adoption.Request = resumed.ID()
	fixture.adoption.Evaluation = saValue(t, domain.NewSellEvaluationReference, evaluation)
	fixture.adoption.OriginalMinor, fixture.adoption.SettlementMinor = minor, minor
	fixture.evaluations.adoption = fixture.adoption
}

// supersessionRow 是被替代那一笔在替代册上的原样：行在不在、后继（空串即没有后继）、最初记下的时刻。
type supersessionRow struct {
	found      bool
	successor  string
	recordedAt time.Time
}

func (fixture *stageLockFixture) supersessionRow(t *testing.T, charge domain.CustomerChargeID) supersessionRow {
	t.Helper()
	querier, err := fixture.db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var (
		successor  *string
		recordedAt time.Time
	)
	err = querier.QueryRow(t.Context(), `
		SELECT superseded_by_charge_id, recorded_at
		  FROM settlement_accounting.customer_charge_supersession
		 WHERE tenant_id = $1 AND charge_id = $2`,
		fixture.tenant.String(), charge.String(),
	).Scan(&successor, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return supersessionRow{}
	}
	if err != nil {
		t.Fatal(err)
	}
	row := supersessionRow{found: true, recordedAt: recordedAt}
	if successor != nil {
		row.successor = *successor
	}
	return row
}

// chargeRows 数这个租户在客户费用册上一共有几行：「没有多出费用行」拿它断言。
func (fixture *stageLockFixture) chargeRows(t *testing.T) int {
	t.Helper()
	querier, err := fixture.db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err := querier.QueryRow(t.Context(),
		`SELECT count(*) FROM settlement_accounting.customer_charge WHERE tenant_id = $1`,
		fixture.tenant.String(),
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

// confirmNow 在自己的事务里确认指名的那笔费用，提交后交回答复。
func (fixture *stageLockFixture) confirmNow(t *testing.T, charge domain.CustomerChargeID) application.ConfirmChargeOutcome {
	t.Helper()
	var outcome application.ConfirmChargeOutcome
	saWithin(t, fixture.db.Transactor(), t.Context(), func(ctx context.Context) error {
		result, err := fixture.confirm.Handle(ctx, application.ConfirmChargeCommand{TenantID: fixture.tenant, ChargeID: charge.String()})
		outcome = result.Outcome()
		return err
	})
	return outcome
}

// Covers: 零额复磅先到、确认后到——零额那一头先取到费用的行锁、记下没有后继的替代、提交之前确认来办：确认必须等着，不能在对方
// 提交前返回；对方提交之后确认读到的是已被替代，答 CHARGE_SUPERSEDED。费用在库上仍不是已确认，替代行在、后继为空，没有多出
// 费用行（零额不建新预估）。
func TestAManualConfirmationWaitsForAZeroTotalReweighHoldingTheChargeThenAnswersSuperseded(t *testing.T) {
	fixture := newStageLockFixture(t)
	fixture.makeTotalZero()

	reweigh := holdOpen(t, fixture.db, func(ctx context.Context) error {
		result, err := fixture.formation.Handle(ctx, fixture.reweighCommand())
		if err != nil {
			return err
		}
		if result.Outcome() != application.EstimatedCustomerChargeConcluded || result.State() != domain.SellRequestZeroTotal {
			return fmt.Errorf("复磅形成 = %s / %s，want 记下负向结束（零额）", result.Outcome(), result.State())
		}
		return nil
	})

	confirmation := fixture.confirmAsync()
	select {
	case early := <-confirmation:
		reweigh.release()
		<-reweigh.done
		t.Fatalf("确认没等零额复磅提交就答了 %s；两头都提交之后：%s", early.outcome, fixture.describe(t))
	case <-time.After(blockedFor):
	}
	reweigh.release()
	if err := <-reweigh.done; err != nil {
		t.Fatalf("零额复磅提交：%v", err)
	}

	answer := <-confirmation
	if answer.err != nil || answer.outcome != application.ChargeSuperseded {
		t.Fatalf("确认 = %s err = %v，want CHARGE_SUPERSEDED", answer.outcome, answer.err)
	}
	if stage, found := fixture.stageOf(t, fixture.earlier); !found || stage != domain.ChargeEstimated {
		t.Fatalf("被替代的费用阶段 = %s found=%v，want 仍是预估", stage, found)
	}
	if row := fixture.supersessionRow(t, fixture.earlier); !row.found || row.successor != "" {
		t.Fatalf("替代行 found=%v 后继=%q，want 行在、后继为空", row.found, row.successor)
	}
	if rows := fixture.chargeRows(t); rows != 1 {
		t.Fatalf("费用行 %d 行，want 只有原来那一行", rows)
	}
	if stored, found, err := fixture.requests.FindByID(t.Context(), fixture.next.Key); err != nil || !found ||
		stored.State != domain.SellRequestZeroTotal || stored.FormedCharge.String() != "" {
		t.Fatalf("这一代的结论 = %s / %q found=%v err=%v，want 负向结束（零额）、不带费用", stored.State, stored.FormedCharge, found, err)
	}
}

// Covers: 确认先到、零额复磅后到——确认先把费用确认、提交之前零额复磅来办：复磅必须等着；确认提交之后复磅读到的是已确认，走
// settleConfirmedReweigh——不记替代、不建费用，按已确认那条路办：新评价结算币金额为零，与费用的差额是 −12000，记一笔 12000 的
// 计价纠错贷项（费用连同纠错净额为零，不为负，门放行），这一代的结论是已形成、指回原来那笔已确认费用，不是负向结束（零额）。
func TestAZeroTotalReweighWaitsForAConfirmationHoldingTheChargeThenSettlesTheDifferenceAsACredit(t *testing.T) {
	fixture := newStageLockFixture(t)
	fixture.makeTotalZero()

	confirmation := holdOpen(t, fixture.db, func(ctx context.Context) error {
		result, err := fixture.confirm.Handle(ctx, fixture.confirmCommand())
		if err != nil {
			return err
		}
		if result.Outcome() != application.ChargeConfirmedOutcome {
			return fmt.Errorf("确认 = %s，want CHARGE_CONFIRMED", result.Outcome())
		}
		return nil
	})

	reweigh := fixture.reweighAsync()
	select {
	case early := <-reweigh:
		confirmation.release()
		<-confirmation.done
		t.Fatalf("零额复磅没等确认提交就答了 %s / %s；两头都提交之后：%s", early.state, early.charge, fixture.describe(t))
	case <-time.After(blockedFor):
	}
	confirmation.release()
	if err := <-confirmation.done; err != nil {
		t.Fatalf("确认提交：%v", err)
	}

	answer := <-reweigh
	if answer.err != nil || answer.state != domain.SellRequestFormed || answer.charge != fixture.earlier {
		t.Fatalf("复磅 = %s / %q err = %v，want 已形成、指向原来那笔 %s", answer.state, answer.charge, answer.err, fixture.earlier)
	}
	if stage, found := fixture.stageOf(t, fixture.earlier); !found || stage != domain.ChargeConfirmed {
		t.Fatalf("原费用阶段 = %s found=%v，want 已确认", stage, found)
	}
	if row := fixture.supersessionRow(t, fixture.earlier); row.found {
		t.Fatalf("已确认的费用被记了替代 后继=%q", row.successor)
	}
	if rows := fixture.chargeRows(t); rows != 1 {
		t.Fatalf("费用行 %d 行，want 不另建费用、只有原来那一行", rows)
	}
	recorded, err := fixture.adjustments.ListByCharge(t.Context(), fixture.tenant, fixture.earlier)
	if err != nil || len(recorded) != 1 {
		t.Fatalf("调整 %d 笔 err = %v，want 一笔差额", len(recorded), err)
	}
	if _, minor := recorded[0].Adjustment.SettlementAmount(); minor != 12000 ||
		recorded[0].Adjustment.Direction() != domain.AdjustmentCredit || recorded[0].Adjustment.Kind() != domain.PricingCorrection {
		_, minor := recorded[0].Adjustment.SettlementAmount()
		t.Fatalf("差额 = %d %s %s，want 计价纠错贷项 12000", minor, recorded[0].Adjustment.Kind(), recorded[0].Adjustment.Direction())
	}
	if stored, found, err := fixture.requests.FindByID(t.Context(), fixture.next.Key); err != nil || !found ||
		stored.State != domain.SellRequestFormed || stored.FormedCharge != fixture.earlier {
		t.Fatalf("这一代的结论 = %s / %q found=%v err=%v", stored.State, stored.FormedCharge, found, err)
	}
}

// Covers: 零额那一代记下「被替代、没有后继」，再来一代非零评价形成新预估：那条替代行的后继被补上，recorded_at 仍是最初被
// 替代的那一刻；确认旧的那笔此前此后都答 CHARGE_SUPERSEDED、阶段不变，确认新预估能成。
func TestAZeroTotalGenerationFollowedByANonZeroOneGetsItsSupersessionCompletedWithTheSuccessor(t *testing.T) {
	fixture := newStageLockFixture(t)
	fixture.makeTotalZero()

	zeroed := <-fixture.reweighAsync()
	if zeroed.err != nil || zeroed.state != domain.SellRequestZeroTotal {
		t.Fatalf("零额那一代 = %s err = %v，want 负向结束（零额）", zeroed.state, zeroed.err)
	}
	first := fixture.supersessionRow(t, fixture.earlier)
	if !first.found || first.successor != "" || !first.recordedAt.Equal(stageLockAt) {
		t.Fatalf("零额之后的替代行 found=%v 后继=%q 时刻=%s，want 行在、后继为空、时刻 %s",
			first.found, first.successor, first.recordedAt, stageLockAt)
	}
	if outcome := fixture.confirmNow(t, fixture.earlier); outcome != application.ChargeSuperseded {
		t.Fatalf("没有后继时确认旧的那笔 = %s，want CHARGE_SUPERSEDED", outcome)
	}

	fixture.clock.advance(time.Hour)
	fixture.resumeGeneration(t, "SELLREQ-SYN-3", "measure-2", "SYN-EVAL-SELL-3", 15000)
	formed := <-fixture.reweighAsync()
	if formed.err != nil || formed.state != domain.SellRequestFormed || formed.charge != fixture.successor() {
		t.Fatalf("非零那一代 = %s / %q err = %v，want 已形成新预估 %s", formed.state, formed.charge, formed.err, fixture.successor())
	}

	completed := fixture.supersessionRow(t, fixture.earlier)
	if !completed.found || completed.successor != fixture.successor().String() {
		t.Fatalf("补后继之后的替代行 found=%v 后继=%q，want 后继是 %s", completed.found, completed.successor, fixture.successor())
	}
	if !completed.recordedAt.Equal(stageLockAt) {
		t.Fatalf("补后继之后 recorded_at = %s，want 仍是最初被替代的那一刻 %s", completed.recordedAt, stageLockAt)
	}
	if rows := fixture.chargeRows(t); rows != 2 {
		t.Fatalf("费用行 %d 行，want 旧的与新预估两行", rows)
	}

	if outcome := fixture.confirmNow(t, fixture.earlier); outcome != application.ChargeSuperseded {
		t.Fatalf("确认旧的那笔 = %s，want CHARGE_SUPERSEDED", outcome)
	}
	if stage, found := fixture.stageOf(t, fixture.earlier); !found || stage != domain.ChargeEstimated {
		t.Fatalf("被替代的费用阶段 = %s found=%v，want 仍是预估", stage, found)
	}
	if outcome := fixture.confirmNow(t, fixture.successor()); outcome != application.ChargeConfirmedOutcome {
		t.Fatalf("确认新预估 = %s，want CHARGE_CONFIRMED", outcome)
	}
	if stage, found := fixture.stageOf(t, fixture.successor()); !found || stage != domain.ChargeConfirmed {
		t.Fatalf("新预估阶段 = %s found=%v，want 已确认", stage, found)
	}
}
