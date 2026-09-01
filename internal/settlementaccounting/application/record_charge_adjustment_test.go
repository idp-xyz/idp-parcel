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

var adjustmentRecordedAt = time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)

type chargeAdjustmentStoreDouble struct {
	records  map[string]ports.ChargeAdjustmentRecord
	saveErr  error
	findErr  error
	lostRead bool
	saves    int
}

func newChargeAdjustmentStore() *chargeAdjustmentStoreDouble {
	return &chargeAdjustmentStoreDouble{records: map[string]ports.ChargeAdjustmentRecord{}}
}

func (double *chargeAdjustmentStoreDouble) FindByKey(
	_ context.Context,
	key ports.ChargeAdjustmentKey,
) (ports.ChargeAdjustmentRecord, bool, error) {
	if double.findErr != nil {
		return ports.ChargeAdjustmentRecord{}, false, double.findErr
	}
	if double.lostRead {
		return ports.ChargeAdjustmentRecord{}, false, nil
	}
	record, found := double.records[key.Adjustment.String()]
	return record, found, nil
}

func (double *chargeAdjustmentStoreDouble) ListByCharge(
	_ context.Context,
	_ domain.TenantID,
	charge domain.CustomerChargeID,
) ([]ports.ChargeAdjustmentRecord, error) {
	listed := []ports.ChargeAdjustmentRecord{}
	for _, record := range double.records {
		if record.Adjustment.Charge() == charge {
			listed = append(listed, record)
		}
	}
	return listed, nil
}

func (double *chargeAdjustmentStoreDouble) Save(
	_ context.Context,
	record ports.ChargeAdjustmentRecord,
) (ports.ChargeAdjustmentSaveOutcome, error) {
	double.saves++
	if double.saveErr != nil {
		return ports.ChargeAdjustmentSaveOutcomeInvalid, double.saveErr
	}
	if _, found := double.records[record.Key.Adjustment.String()]; found {
		return ports.ChargeAdjustmentAlreadyRecorded, nil
	}
	double.records[record.Key.Adjustment.String()] = record
	return ports.ChargeAdjustmentSaved, nil
}

type adjustmentFixture struct {
	charges     *chargeStoreDouble
	adjustments *chargeAdjustmentStoreDouble
	handler     *application.RecordChargeAdjustmentHandler
}

func newAdjustmentFixture(t *testing.T) *adjustmentFixture {
	t.Helper()
	fixture := &adjustmentFixture{
		charges:     newChargeStore(),
		adjustments: newChargeAdjustmentStore(),
	}
	fixture.handler = application.NewRecordChargeAdjustmentHandler(application.RecordChargeAdjustmentDeps{
		Charges:     fixture.charges,
		Adjustments: fixture.adjustments,
		Clock:       confirmClock{at: adjustmentRecordedAt},
	})
	confirmed, err := provisionalCharge(t).Confirm(
		registeredFacts(t),
		billValue(t, domain.NewConfirmationBasisReference, "delivery-confirmed-1"),
		chargeConfirmedAt)
	if err != nil {
		t.Fatalf("confirm charge: %v", err)
	}
	fixture.charges.charges["charge-1"] = confirmed
	return fixture
}

func correctionCommand(t *testing.T, id string) application.RecordChargeAdjustmentCommand {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	return application.RecordChargeAdjustmentCommand{
		TenantID:           tenant,
		AdjustmentID:       id,
		ChargeID:           "charge-1",
		Kind:               domain.PricingCorrection,
		Direction:          domain.AdjustmentDebit,
		Evaluation:         "sell-eval/re-1",
		OriginalCurrency:   "USD",
		OriginalMinor:      900,
		SettlementCurrency: "USD",
		SettlementMinor:    900,
	}
}

// Covers: ADR-0087 决定二与 SA CONTEXT「已确认费用……由对应唯一创建用例追加调整明细；
// 原确认费用仍保留，对账纳入和资金核销不能代替金额调整」——调整独立成册地形成，不碰
// 原费用、不需要任何对账单参与；重复触发返回原结果。
func TestAnAdjustmentIsAppendedWithoutTouchingTheChargeOrAStatement(t *testing.T) {
	fixture := newAdjustmentFixture(t)

	result, err := fixture.handler.Handle(context.Background(), correctionCommand(t, "adj-1"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.ChargeAdjustmentRecordedOutcome {
		t.Fatalf("outcome = %q, want ADJUSTMENT_RECORDED", result.Outcome())
	}
	adjustment, present := result.Adjustment()
	if !present || adjustment.Kind() != domain.PricingCorrection {
		t.Fatalf("adjustment = %#v present = %v", adjustment, present)
	}
	if !adjustment.FormedAt().Equal(adjustmentRecordedAt) {
		t.Fatalf("形成时点 = %s；它该由时钟给出，不由调用方声称", adjustment.FormedAt())
	}

	// 原费用一字未动：确认阶段、金额、确认依据都还是原来那份。
	stored := fixture.charges.charges["charge-1"]
	if stored.Stage() != domain.ChargeConfirmed {
		t.Fatalf("原费用阶段变成了 %q", stored.Stage())
	}
	if _, amount := stored.SettlementAmount(); amount != 12000 {
		t.Fatalf("原费用金额变成了 %d——调整是新对象，不改写费用本体", amount)
	}

	// 重复触发返回原结果，不重登。
	replay, err := fixture.handler.Handle(context.Background(), correctionCommand(t, "adj-1"))
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replay.Outcome() != application.ChargeAdjustmentReplayedOutcome {
		t.Fatalf("outcome = %q, want ALREADY_RECORDED", replay.Outcome())
	}
	if len(fixture.adjustments.records) != 1 {
		t.Fatalf("册上有 %d 笔——重复触发造出了第二笔", len(fixture.adjustments.records))
	}
}

// Covers: 调整只挂在已确认费用上（CONTEXT「已确认费用发现迟到事实……追加调整明细」）；
// 无语义种类、填错依据格由领域形成门拒；依赖故障与提交矛盾各归其格（ADR-0029）。
func TestAdjustmentGridsSplitByRecoveryAction(t *testing.T) {
	t.Run("an unconfirmed charge is its own grid", func(t *testing.T) {
		fixture := newAdjustmentFixture(t)
		fixture.charges.charges["charge-1"] = provisionalCharge(t)
		result, err := fixture.handler.Handle(context.Background(), correctionCommand(t, "adj-2"))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.AdjustmentChargeNotConfirmed {
			t.Fatalf("outcome = %q；调预估不是调整，是重新预估", result.Outcome())
		}
		if result.ContinuationReference() == "" {
			t.Fatal("专格没有留下续办引用")
		}
		if fixture.adjustments.saves != 0 {
			t.Fatal("费用还没确认就把调整登进去了")
		}
	})

	t.Run("a missing charge is not accepted", func(t *testing.T) {
		fixture := newAdjustmentFixture(t)
		command := correctionCommand(t, "adj-3")
		command.ChargeID = "charge-9"
		result, err := fixture.handler.Handle(context.Background(), command)
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.AdjustmentNotAccepted {
			t.Fatalf("outcome = %q, want SOURCE_NOT_ACCEPTED", result.Outcome())
		}
	})

	t.Run("a semanticless kind never reaches the register", func(t *testing.T) {
		fixture := newAdjustmentFixture(t)
		command := correctionCommand(t, "adj-4")
		command.Kind = domain.AdjustmentKindInvalid
		result, err := fixture.handler.Handle(context.Background(), command)
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.AdjustmentNotAccepted {
			t.Fatalf("outcome = %q；无语义的「冲销」进了册", result.Outcome())
		}
		if fixture.adjustments.saves != 0 {
			t.Fatal("形成门没拦住就写册了")
		}
	})

	t.Run("a correction carrying an authorization is refused", func(t *testing.T) {
		fixture := newAdjustmentFixture(t)
		command := correctionCommand(t, "adj-5")
		command.Authorization = "CONCESSION-APPROVAL/9"
		result, err := fixture.handler.Handle(context.Background(), command)
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.AdjustmentNotAccepted {
			t.Fatalf("outcome = %q；纠错挂着授权被收下了", result.Outcome())
		}
	})

	t.Run("a charge store failure is undecided with its reason", func(t *testing.T) {
		fixture := newAdjustmentFixture(t)
		fixture.charges.findErr = errors.New("charge store down")
		result, err := fixture.handler.Handle(context.Background(), correctionCommand(t, "adj-6"))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.AdjustmentUndecided ||
			result.UndecidedReason() != application.AdjustmentChargeStoreUnavailable {
			t.Fatalf("outcome = %q reason = %q", result.Outcome(), result.UndecidedReason())
		}
	})

	t.Run("a register failure is undecided with its own reason", func(t *testing.T) {
		fixture := newAdjustmentFixture(t)
		fixture.adjustments.saveErr = errors.New("register down")
		result, err := fixture.handler.Handle(context.Background(), correctionCommand(t, "adj-7"))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.AdjustmentUndecided ||
			result.UndecidedReason() != application.AdjustmentRegisterUnavailable {
			t.Fatalf("outcome = %q reason = %q", result.Outcome(), result.UndecidedReason())
		}
		if result.ContinuationReference() == "" {
			t.Fatal("依赖故障没有留下续办引用")
		}
	})

	t.Run("a replay whose original cannot be read back is undecided", func(t *testing.T) {
		fixture := newAdjustmentFixture(t)
		if _, err := fixture.handler.Handle(context.Background(), correctionCommand(t, "adj-8")); err != nil {
			t.Fatalf("first handle: %v", err)
		}
		// 册答`已登记`却读不回原件：这不是重放成功，是册此刻说不出话——答未决而不是
		// 拿刚形成的那一份冒充原件。
		fixture.adjustments.lostRead = true
		result, err := fixture.handler.Handle(context.Background(), correctionCommand(t, "adj-8"))
		if err != nil {
			t.Fatalf("replay: %v", err)
		}
		if result.Outcome() != application.AdjustmentUndecided ||
			result.UndecidedReason() != application.AdjustmentRegisterUnavailable {
			t.Fatalf("outcome = %q reason = %q", result.Outcome(), result.UndecidedReason())
		}
	})

	t.Run("the undecided reason set is closed", func(t *testing.T) {
		labels := map[string]struct{}{}
		for _, reason := range []application.AdjustmentUndecidedReason{
			application.AdjustmentChargeStoreUnavailable, application.AdjustmentRegisterUnavailable,
		} {
			label := reason.String()
			if label == "" {
				t.Fatalf("reason %d has no label", reason)
			}
			labels[label] = struct{}{}
		}
		if len(labels) != 2 {
			t.Fatalf("labels collapsed into %d", len(labels))
		}
		if application.AdjustmentUndecidedReason(len(labels)+1).String() != "" {
			t.Fatal("第三个未决原因带了标签——封闭集合被悄悄放开")
		}
	})
}
