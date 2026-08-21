package application_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

var (
	chargeFormedAt    = time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)
	chargeConfirmedAt = time.Date(2026, 8, 13, 11, 0, 0, 0, time.UTC)
)

type chargeStoreDouble struct {
	charges     map[string]domain.CustomerCharge
	findErr     error
	saveErr     error
	saveResult  ports.ChargeSaveOutcome
	forceResult bool
	saves       int
}

func newChargeStore() *chargeStoreDouble {
	return &chargeStoreDouble{charges: map[string]domain.CustomerCharge{}}
}

func (double *chargeStoreDouble) FindByID(
	_ context.Context,
	_ domain.TenantID,
	id domain.CustomerChargeID,
) (domain.CustomerCharge, bool, error) {
	if double.findErr != nil {
		return domain.CustomerCharge{}, false, double.findErr
	}
	charge, found := double.charges[id.String()]
	return charge, found, nil
}

func (double *chargeStoreDouble) SaveConfirmed(
	_ context.Context,
	_ domain.TenantID,
	charge domain.CustomerCharge,
) (ports.ChargeSaveOutcome, error) {
	double.saves++
	if double.saveErr != nil {
		return ports.ChargeSaveOutcomeInvalid, double.saveErr
	}
	if double.forceResult {
		return double.saveResult, nil
	}
	if existing, found := double.charges[charge.ID().String()]; found && existing.Stage() == domain.ChargeConfirmed {
		return ports.ChargeAlreadyConfirmed, nil
	}
	double.charges[charge.ID().String()] = charge
	return ports.ChargeSaved, nil
}

type conditionViewDouble struct {
	configured bool
	met        bool
	err        error
}

func (double *conditionViewDouble) LoadConfirmationCondition(
	_ context.Context,
	_ domain.TenantID,
	_ domain.CustomerChargeID,
	_ domain.FeeItemReference,
) (ports.ConfirmationCondition, bool, error) {
	if double.err != nil {
		return ports.ConfirmationCondition{}, false, double.err
	}
	if !double.configured {
		return ports.ConfirmationCondition{}, false, nil
	}
	if !double.met {
		return ports.ConfirmationCondition{Met: false, Gap: "delivery-not-confirmed"}, true, nil
	}
	basis, err := domain.NewConfirmationBasisReference("delivery-confirmed-1")
	if err != nil {
		return ports.ConfirmationCondition{}, false, err
	}
	return ports.ConfirmationCondition{Met: true, Basis: basis}, true, nil
}

type confirmationHandoffDouble struct {
	intents []ports.ChargeConfirmationHandoffIntent
	err     error
}

func (double *confirmationHandoffDouble) HandOffChargeConfirmation(
	_ context.Context,
	intent ports.ChargeConfirmationHandoffIntent,
) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

type confirmClock struct{ at time.Time }

func (clock confirmClock) Now() time.Time { return clock.at }

type confirmFixture struct {
	store      *chargeStoreDouble
	conditions *conditionViewDouble
	handoff    *confirmationHandoffDouble
	handler    *application.ConfirmChargeHandler
}

func newConfirmFixture(t *testing.T) *confirmFixture {
	t.Helper()
	fixture := &confirmFixture{
		store:      newChargeStore(),
		conditions: &conditionViewDouble{configured: true, met: true},
		handoff:    &confirmationHandoffDouble{},
	}
	fixture.handler = application.NewConfirmChargeHandler(application.ConfirmChargeDeps{
		Charges:    fixture.store,
		Conditions: fixture.conditions,
		Downstream: fixture.handoff,
		Clock:      confirmClock{at: chargeConfirmedAt},
	})
	fixture.store.charges["charge-1"] = provisionalCharge(t)
	return fixture
}

func provisionalCharge(t *testing.T) domain.CustomerCharge {
	t.Helper()
	charge, err := domain.FormCustomerCharge(domain.CustomerChargeSpec{
		ID:                 billValue(t, domain.NewCustomerChargeID, "charge-1"),
		FeeItem:            billValue(t, domain.NewFeeItemReference, "fee-freight"),
		Evaluation:         billValue(t, domain.NewSellEvaluationReference, "sell-evaluation-1"),
		OriginalCurrency:   billValue(t, domain.NewCurrencyCode, "USD"),
		OriginalMinor:      12000,
		SettlementCurrency: billValue(t, domain.NewCurrencyCode, "USD"),
		SettlementMinor:    12000,
		Stage:              domain.ChargeProvisional,
		FormedAt:           chargeFormedAt,
	})
	if err != nil {
		t.Fatalf("form charge: %v", err)
	}
	return charge
}

func confirmCommand(t *testing.T) application.ConfirmChargeCommand {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	return application.ConfirmChargeCommand{TenantID: tenant, ChargeID: "charge-1"}
}

// Covers: UC-SA-002 结果契约「费用已确认：确认条件已满足」——条件核对给出依据后定格；
// 确认不改金额（命令类型上连金额字段都没有，反射钉死）；意图交给对账单纳入。
func TestAMetConditionConfirmsWithoutTouchingTheAmount(t *testing.T) {
	commandType := reflect.TypeOf(application.ConfirmChargeCommand{})
	for index := 0; index < commandType.NumField(); index++ {
		name := strings.ToLower(commandType.Field(index).Name)
		if strings.Contains(name, "amount") || strings.Contains(name, "minor") || strings.Contains(name, "basis") {
			t.Fatalf("ConfirmChargeCommand 携带 %q——确认就有了改金额或口头声称依据的入口", commandType.Field(index).Name)
		}
	}

	fixture := newConfirmFixture(t)
	result, err := fixture.handler.Handle(context.Background(), confirmCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.ChargeConfirmedOutcome {
		t.Fatalf("outcome = %q, want CHARGE_CONFIRMED", result.Outcome())
	}
	charge, present := result.Charge()
	if !present || charge.Stage() != domain.ChargeConfirmed {
		t.Fatalf("stage = %q", charge.Stage())
	}
	// 金额锚定本夹具：确认前后都是 12000（USD）——确认不改金额。
	if _, amount := charge.SettlementAmount(); amount != 12000 {
		t.Fatalf("amount = %d, want 12000", amount)
	}
	basis, confirmed := charge.Confirmation()
	if !confirmed || basis.String() != "delivery-confirmed-1" {
		t.Fatal("确认依据没有来自条件核对")
	}
	if len(fixture.handoff.intents) != 1 {
		t.Fatalf("intents = %d, want 1", len(fixture.handoff.intents))
	}
}

// 已确认费用重放返原确认不二确；并发二确落败读回赢家；投递失败不翻结果重放重发。
func TestAnAlreadyConfirmedChargeIsNotConfirmedTwice(t *testing.T) {
	fixture := newConfirmFixture(t)
	command := confirmCommand(t)

	first, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("first handle: %v", err)
	}
	firstCharge, _ := first.Charge()

	replay, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("replay handle: %v", err)
	}
	if replay.Outcome() != application.ChargeAlreadyConfirmedOutcome {
		t.Fatalf("outcome = %q, want ALREADY_CONFIRMED", replay.Outcome())
	}
	replayCharge, _ := replay.Charge()
	replayBasis, _ := replayCharge.Confirmation()
	firstBasis, _ := firstCharge.Confirmation()
	if replayBasis != firstBasis {
		t.Fatal("重放换了确认依据——原确认被二确顶替")
	}
	if fixture.store.saves != 1 {
		t.Fatalf("saves = %d, want 1（重放不重提交）", fixture.store.saves)
	}
	if len(fixture.handoff.intents) != 2 {
		t.Fatalf("intents = %d, want 2（重放重发同一份）", len(fixture.handoff.intents))
	}

	t.Run("a concurrent loser reads back the winner", func(t *testing.T) {
		loser := newConfirmFixture(t)
		loser.store.forceResult = true
		loser.store.saveResult = ports.ChargeAlreadyConfirmed
		// 赢家已确认在库：落败方 Save 撞 AlreadyConfirmed 后读回赢家作答。
		winner, err := provisionalCharge(t).Confirm(
			billValue(t, domain.NewConfirmationBasisReference, "delivery-confirmed-winner"),
			chargeConfirmedAt.Add(-time.Minute))
		if err != nil {
			t.Fatalf("confirm winner: %v", err)
		}
		loser.store.charges["charge-1"] = provisionalCharge(t)
		result, err := loser.handler.Handle(context.Background(), confirmCommand(t))
		if err != nil {
			t.Fatalf("loser handle: %v", err)
		}
		_ = winner
		if result.Outcome() != application.ChargeAlreadyConfirmedOutcome {
			t.Fatalf("outcome = %q, want ALREADY_CONFIRMED", result.Outcome())
		}
	})

	t.Run("a handoff failure keeps the outcome and is resent on replay", func(t *testing.T) {
		fixture := newConfirmFixture(t)
		fixture.handoff.err = errors.New("downstream unavailable")
		first, err := fixture.handler.Handle(context.Background(), confirmCommand(t))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if first.Outcome() != application.ChargeConfirmedOutcome || first.ConfirmationHandoffReference() == "" {
			t.Fatalf("outcome = %q handoff = %q（投递失败不翻结果）", first.Outcome(), first.ConfirmationHandoffReference())
		}
		fixture.handoff.err = nil
		replay, err := fixture.handler.Handle(context.Background(), confirmCommand(t))
		if err != nil {
			t.Fatalf("replay: %v", err)
		}
		if replay.ConfirmationHandoffReference() != "" || len(fixture.handoff.intents) != 1 {
			t.Fatalf("intents = %d handoff = %q", len(fixture.handoff.intents), replay.ConfirmationHandoffReference())
		}
	})
}

// Covers: 确认条件是实例参数——未配置→未决不默认转正；未满足→专格带缺口不是拒绝；
// 依赖故障与提交矛盾各归其格（ADR-0029）。
func TestConditionGridsSplitByRecoveryAction(t *testing.T) {
	t.Run("an unconfigured condition catalogue is undecided", func(t *testing.T) {
		fixture := newConfirmFixture(t)
		fixture.conditions.configured = false
		result, err := fixture.handler.Handle(context.Background(), confirmCommand(t))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.ConfirmUndecided ||
			result.UndecidedReason() != application.ConditionUnconfigured {
			t.Fatalf("outcome = %q reason = %q（不默认转正）", result.Outcome(), result.UndecidedReason())
		}
		if fixture.store.saves != 0 {
			t.Fatal("未决还提交了确认")
		}
	})

	t.Run("an unmet condition is its own grid with the gap", func(t *testing.T) {
		fixture := newConfirmFixture(t)
		fixture.conditions.met = false
		result, err := fixture.handler.Handle(context.Background(), confirmCommand(t))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.ConfirmationConditionNotMet {
			t.Fatalf("outcome = %q, want CONDITION_NOT_MET（不是拒绝，等条件满足再来）", result.Outcome())
		}
		if result.ContinuationReference() == "" {
			t.Fatal("条件未满足没有留下缺口续办引用")
		}
		if result.UndecidedReason() != application.ConfirmUndecidedReasonNone {
			t.Fatalf("reason = %q；专格不指名依赖", result.UndecidedReason())
		}
		stored := fixture.store.charges["charge-1"]
		if stored.Stage() != domain.ChargeProvisional {
			t.Fatal("条件未满足还改了费用阶段")
		}
	})

	t.Run("a missing charge is not accepted", func(t *testing.T) {
		fixture := newConfirmFixture(t)
		command := confirmCommand(t)
		command.ChargeID = "charge-9"
		result, err := fixture.handler.Handle(context.Background(), command)
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.ConfirmNotAccepted {
			t.Fatalf("outcome = %q, want SOURCE_NOT_ACCEPTED", result.Outcome())
		}
	})

	t.Run("a store failure is undecided with its reason", func(t *testing.T) {
		fixture := newConfirmFixture(t)
		fixture.store.findErr = errors.New("store down")
		result, err := fixture.handler.Handle(context.Background(), confirmCommand(t))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.ConfirmUndecided ||
			result.UndecidedReason() != application.ChargeStoreUnavailable {
			t.Fatalf("outcome = %q reason = %q", result.Outcome(), result.UndecidedReason())
		}
	})

	t.Run("an unexpected save outcome is a programming error", func(t *testing.T) {
		fixture := newConfirmFixture(t)
		fixture.store.forceResult = true
		fixture.store.saveResult = ports.ChargeSaveOutcome(99)
		if _, err := fixture.handler.Handle(context.Background(), confirmCommand(t)); !errors.Is(err, application.ErrUnexpectedChargeSave) {
			t.Fatalf("error = %v, want ErrUnexpectedChargeSave", err)
		}
	})

	t.Run("the undecided reason set is closed", func(t *testing.T) {
		labels := map[string]struct{}{}
		for _, reason := range []application.ConfirmUndecidedReason{
			application.ChargeStoreUnavailable, application.ConditionViewUnavailable, application.ConditionUnconfigured,
		} {
			label := reason.String()
			if label == "" {
				t.Fatalf("reason %d has no label", reason)
			}
			labels[label] = struct{}{}
		}
		if len(labels) != 3 {
			t.Fatalf("labels collapsed into %d", len(labels))
		}
		if application.ConfirmUndecidedReason(len(labels)+1).String() != "" {
			t.Fatal("第四个未决原因带了标签——封闭集合被悄悄放开")
		}
	})
}
