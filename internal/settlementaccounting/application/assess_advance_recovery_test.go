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

var (
	advanceJudgedAt = time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC)
	advanceFormedAt = time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)
	advanceNowAt    = time.Date(2026, 8, 13, 11, 0, 0, 0, time.UTC)
)

type assessmentStoreDouble struct {
	records map[string]ports.AdvanceAssessmentRecord
	findErr error
	saves   int
}

func newAssessmentStore() *assessmentStoreDouble {
	return &assessmentStoreDouble{records: map[string]ports.AdvanceAssessmentRecord{}}
}

func assessmentKey(key ports.AdvanceAssessmentKey) string {
	return key.TenantID.String() + "|" + key.Assessment.String()
}

func (double *assessmentStoreDouble) FindByKey(
	_ context.Context,
	key ports.AdvanceAssessmentKey,
) (ports.AdvanceAssessmentRecord, bool, error) {
	if double.findErr != nil {
		return ports.AdvanceAssessmentRecord{}, false, double.findErr
	}
	record, found := double.records[assessmentKey(key)]
	return record, found, nil
}

func (double *assessmentStoreDouble) Save(
	_ context.Context,
	record ports.AdvanceAssessmentRecord,
) (ports.AdvanceAssessmentSaveOutcome, error) {
	double.saves++
	if _, exists := double.records[assessmentKey(record.Key)]; exists {
		return ports.AdvanceAssessmentAlreadyRecorded, nil
	}
	double.records[assessmentKey(record.Key)] = record
	return ports.AdvanceAssessmentSaved, nil
}

type recoveryStoreDouble struct {
	records map[string]ports.AdvanceRecoveryRecord
	findErr error
}

func newRecoveryStore() *recoveryStoreDouble {
	return &recoveryStoreDouble{records: map[string]ports.AdvanceRecoveryRecord{}}
}

func recoveryKey(key ports.AdvanceRecoveryKey) string {
	return key.TenantID.String() + "|" + key.Recovery.String()
}

func (double *recoveryStoreDouble) FindByKey(
	_ context.Context,
	key ports.AdvanceRecoveryKey,
) (ports.AdvanceRecoveryRecord, bool, error) {
	if double.findErr != nil {
		return ports.AdvanceRecoveryRecord{}, false, double.findErr
	}
	record, found := double.records[recoveryKey(key)]
	return record, found, nil
}

func (double *recoveryStoreDouble) Save(
	_ context.Context,
	record ports.AdvanceRecoveryRecord,
) (ports.AdvanceRecoverySaveOutcome, error) {
	if _, exists := double.records[recoveryKey(record.Key)]; exists {
		return ports.AdvanceRecoveryAlreadyFormed, nil
	}
	double.records[recoveryKey(record.Key)] = record
	return ports.AdvanceRecoverySaved, nil
}

type adjustmentStoreDouble struct {
	records map[string]ports.RecoveryAdjustmentRecord
	findErr error
}

func newAdjustmentStore() *adjustmentStoreDouble {
	return &adjustmentStoreDouble{records: map[string]ports.RecoveryAdjustmentRecord{}}
}

func adjustmentKey(key ports.RecoveryAdjustmentKey) string {
	return key.TenantID.String() + "|" + key.Adjustment.String()
}

func (double *adjustmentStoreDouble) FindByKey(
	_ context.Context,
	key ports.RecoveryAdjustmentKey,
) (ports.RecoveryAdjustmentRecord, bool, error) {
	if double.findErr != nil {
		return ports.RecoveryAdjustmentRecord{}, false, double.findErr
	}
	record, found := double.records[adjustmentKey(key)]
	return record, found, nil
}

func (double *adjustmentStoreDouble) Save(
	_ context.Context,
	record ports.RecoveryAdjustmentRecord,
) (ports.RecoveryAdjustmentSaveOutcome, error) {
	if _, exists := double.records[adjustmentKey(record.Key)]; exists {
		return ports.RecoveryAdjustmentAlreadyFormed, nil
	}
	double.records[adjustmentKey(record.Key)] = record
	return ports.RecoveryAdjustmentSaved, nil
}

type contractViewDouble struct {
	configured bool
	err        error
}

func (double *contractViewDouble) LoadContractResponsibility(
	_ context.Context,
	_ domain.TenantID,
	_ domain.RecoveryCustomerReference,
	_ domain.TaxObligationReference,
) (domain.ContractResponsibilityReference, bool, error) {
	if double.err != nil {
		return domain.ContractResponsibilityReference{}, false, double.err
	}
	if !double.configured {
		return domain.ContractResponsibilityReference{}, false, nil
	}
	contract, err := domain.NewContractResponsibilityReference("contract-clause-7")
	return contract, true, err
}

type advanceHandoffDouble struct {
	intents []ports.AdvanceRecoveryIntent
	err     error
}

func (double *advanceHandoffDouble) HandOffAdvanceRecovery(
	_ context.Context,
	intent ports.AdvanceRecoveryIntent,
) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

type advanceClock struct{ at time.Time }

func (clock advanceClock) Now() time.Time { return clock.at }

type advanceFixture struct {
	assessments *assessmentStoreDouble
	recoveries  *recoveryStoreDouble
	adjustments *adjustmentStoreDouble
	contracts   *contractViewDouble
	handoff     *advanceHandoffDouble
	handler     *application.AssessAdvanceRecoveryHandler
}

func newAdvanceFixture(t *testing.T) *advanceFixture {
	t.Helper()
	fixture := &advanceFixture{
		assessments: newAssessmentStore(),
		recoveries:  newRecoveryStore(),
		adjustments: newAdjustmentStore(),
		contracts:   &contractViewDouble{configured: true},
		handoff:     &advanceHandoffDouble{},
	}
	fixture.handler = application.NewAssessAdvanceRecoveryHandler(application.AssessAdvanceRecoveryDeps{
		Assessments: fixture.assessments,
		Recoveries:  fixture.recoveries,
		Adjustments: fixture.adjustments,
		Contracts:   fixture.contracts,
		Downstream:  fixture.handoff,
		Clock:       advanceClock{at: advanceNowAt},
	})
	return fixture
}

func assessCommand(t *testing.T, verdict domain.AdvanceVerdict) application.AssessAdvanceCommand {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	command := application.AssessAdvanceCommand{
		TenantID:    tenant,
		Assessment:  "assessment-1",
		Obligation:  "tax-obligation-1",
		Verdict:     verdict,
		Currency:    "USD",
		AmountMinor: 5000,
		Version:     "assessment-1/v1",
		JudgedAt:    advanceJudgedAt,
	}
	if verdict == domain.AdvanceEstablished {
		command.FundsFact = "funds-fact-1"
		command.Payer = "payer-legal-entity-1"
		command.Responsibility = "responsibility-conclusion-1"
	} else {
		command.Basis = "assessment-basis-" + verdict.String()
	}
	return command
}

func formRecoveryCommand(t *testing.T) application.FormRecoveryCommand {
	t.Helper()
	tenant, _ := domain.NewTenantID("tenant-1")
	return application.FormRecoveryCommand{
		TenantID:    tenant,
		Assessment:  "assessment-1",
		Recovery:    "recovery-1",
		Customer:    "customer-1",
		Account:     "settlement-account-1",
		AmountMinor: 5000,
		FormedAt:    advanceFormedAt,
	}
}

// Covers: `AT-SA-002/003/004` 的编排面（成立要资金事实+付款方+责任三引用，其余裁决
// 带依据——AssessActualAdvance 领域把门，SA 不重算税费不造付款）——同一评估标识只登
// 一次，重放返原、异内容冲突（重评换新标识）。
func TestAnAssessmentRegistersOncePerIdentity(t *testing.T) {
	fixture := newAdvanceFixture(t)
	command := assessCommand(t, domain.AdvanceEstablished)

	first, err := fixture.handler.Assess(context.Background(), command)
	if err != nil {
		t.Fatalf("assess: %v", err)
	}
	if first.Outcome() != application.AdvanceAssessed {
		t.Fatalf("outcome = %q, want ADVANCE_ASSESSED", first.Outcome())
	}

	replay, err := fixture.handler.Assess(context.Background(), command)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replay.Outcome() != application.AssessmentExistingResult || fixture.assessments.saves != 1 {
		t.Fatalf("outcome = %q saves = %d", replay.Outcome(), fixture.assessments.saves)
	}

	t.Run("a different verdict under the same identity is a conflict", func(t *testing.T) {
		flipped := assessCommand(t, domain.AdvanceNotEstablishedVerdict)
		result, err := fixture.handler.Assess(context.Background(), flipped)
		if err != nil {
			t.Fatalf("conflict assess: %v", err)
		}
		if result.Outcome() != application.AssessmentConflict {
			t.Fatalf("outcome = %q, want ASSESSMENT_CONFLICT（重评换新评估标识）", result.Outcome())
		}
	})

	t.Run("an established verdict without its three references is not accepted", func(t *testing.T) {
		broken := assessCommand(t, domain.AdvanceEstablished)
		broken.Assessment = "assessment-2"
		broken.FundsFact = ""
		result, err := fixture.handler.Assess(context.Background(), broken)
		if err != nil {
			t.Fatalf("assess: %v", err)
		}
		if result.Outcome() != application.AdvanceNotAccepted {
			t.Fatalf("outcome = %q（单独的核定进不了成立格）", result.Outcome())
		}
	})
}

// Covers: UC-SA-001「客户回收与代垫分层（回收需合同依据）」的编排面——合同责任由视图
// 核对且未配置→未决不默认可回收（实例半边）；非成立评估 → ADVANCE_NOT_ESTABLISHED
// 业务负向（ErrAdvanceNotEstablished 哨兵分格）；回收金额限于代垫金额（领域把门）；
// 意图交对账单纳入。
func TestRecoveryNeedsEstablishmentAndContract(t *testing.T) {
	fixture := newAdvanceFixture(t)
	if _, err := fixture.handler.Assess(context.Background(), assessCommand(t, domain.AdvanceEstablished)); err != nil {
		t.Fatalf("assess: %v", err)
	}

	formed, err := fixture.handler.FormRecovery(context.Background(), formRecoveryCommand(t))
	if err != nil {
		t.Fatalf("form recovery: %v", err)
	}
	if formed.Outcome() != application.RecoveryFormed {
		t.Fatalf("outcome = %q, want RECOVERY_FORMED", formed.Outcome())
	}
	record, _ := formed.Recovery()
	if record.Recovery.ContractBasis().String() != "contract-clause-7" {
		t.Fatal("合同依据没有来自视图核对")
	}
	if len(fixture.handoff.intents) != 1 {
		t.Fatalf("intents = %d, want 1（对账单纳入上游）", len(fixture.handoff.intents))
	}

	t.Run("a non-established assessment cannot form a recovery", func(t *testing.T) {
		notEstablished := newAdvanceFixture(t)
		if _, err := notEstablished.handler.Assess(context.Background(), assessCommand(t, domain.AdvanceNotEstablishedVerdict)); err != nil {
			t.Fatalf("assess: %v", err)
		}
		result, err := notEstablished.handler.FormRecovery(context.Background(), formRecoveryCommand(t))
		if err != nil {
			t.Fatalf("form: %v", err)
		}
		if result.Outcome() != application.RecoveryNotEstablished {
			t.Fatalf("outcome = %q, want ADVANCE_NOT_ESTABLISHED（等新证据重评，不是改单）", result.Outcome())
		}
		if len(notEstablished.recoveries.records) != 0 {
			t.Fatal("未成立还形成了回收")
		}
	})

	t.Run("an unconfigured contract catalogue is undecided", func(t *testing.T) {
		unconfigured := newAdvanceFixture(t)
		if _, err := unconfigured.handler.Assess(context.Background(), assessCommand(t, domain.AdvanceEstablished)); err != nil {
			t.Fatalf("assess: %v", err)
		}
		unconfigured.contracts.configured = false
		result, err := unconfigured.handler.FormRecovery(context.Background(), formRecoveryCommand(t))
		if err != nil {
			t.Fatalf("form: %v", err)
		}
		if result.Outcome() != application.AdvanceUndecidedOutcome ||
			result.UndecidedReason() != application.ContractUnconfigured {
			t.Fatalf("outcome = %q reason = %q（不默认可回收）", result.Outcome(), result.UndecidedReason())
		}
	})

	t.Run("an over-advance amount is not accepted", func(t *testing.T) {
		over := formRecoveryCommand(t)
		over.Recovery = "recovery-2"
		over.AmountMinor = 5001
		result, err := fixture.handler.FormRecovery(context.Background(), over)
		if err != nil {
			t.Fatalf("form: %v", err)
		}
		if result.Outcome() != application.AdvanceNotAccepted {
			t.Fatalf("outcome = %q（回收金额限于代垫金额）", result.Outcome())
		}
	})

	t.Run("an absent assessment is not accepted", func(t *testing.T) {
		missing := formRecoveryCommand(t)
		missing.Assessment = "assessment-9"
		result, err := fixture.handler.FormRecovery(context.Background(), missing)
		if err != nil {
			t.Fatalf("form: %v", err)
		}
		if result.Outcome() != application.AdvanceNotAccepted {
			t.Fatalf("outcome = %q", result.Outcome())
		}
	})

	t.Run("a replay returns the original recovery", func(t *testing.T) {
		replay, err := fixture.handler.FormRecovery(context.Background(), formRecoveryCommand(t))
		if err != nil {
			t.Fatalf("replay: %v", err)
		}
		if replay.Outcome() != application.RecoveryExistingResult {
			t.Fatalf("outcome = %q", replay.Outcome())
		}
		if len(fixture.handoff.intents) != 2 {
			t.Fatalf("intents = %d（重放重发同一份）", len(fixture.handoff.intents))
		}
	})
}

// Covers: UC-SA-001「调整带原因不改写」的编排面——调整依附已形成的回收（无中生有拒）、
// 带原因与新依据（FormRecoveryAdjustment 领域把门）、幂等分重放/冲突、意图随调整重发。
func TestAdjustmentsAppendWithoutRewriting(t *testing.T) {
	fixture := newAdvanceFixture(t)
	if _, err := fixture.handler.Assess(context.Background(), assessCommand(t, domain.AdvanceEstablished)); err != nil {
		t.Fatalf("assess: %v", err)
	}
	if _, err := fixture.handler.FormRecovery(context.Background(), formRecoveryCommand(t)); err != nil {
		t.Fatalf("form recovery: %v", err)
	}
	tenant, _ := domain.NewTenantID("tenant-1")
	adjust := application.AdjustRecoveryCommand{
		TenantID:    tenant,
		Adjustment:  "adjustment-1",
		Recovery:    "recovery-1",
		Reason:      domain.TaxAssessmentCorrected,
		NewBasis:    "assessment-basis-corrected",
		Direction:   domain.AdjustmentCredit,
		Currency:    "USD",
		AmountMinor: 700,
		Period:      "2026-09",
		FormedAt:    advanceFormedAt.Add(time.Hour),
	}

	formed, err := fixture.handler.Adjust(context.Background(), adjust)
	if err != nil {
		t.Fatalf("adjust: %v", err)
	}
	if formed.Outcome() != application.AdjustmentFormed {
		t.Fatalf("outcome = %q, want ADJUSTMENT_FORMED", formed.Outcome())
	}
	if len(fixture.handoff.intents) != 2 {
		t.Fatalf("intents = %d, want 2（调整也是对账单纳入的上游）", len(fixture.handoff.intents))
	}

	t.Run("adjusting an absent recovery is refused", func(t *testing.T) {
		missing := adjust
		missing.Adjustment = "adjustment-2"
		missing.Recovery = "recovery-9"
		result, err := fixture.handler.Adjust(context.Background(), missing)
		if err != nil {
			t.Fatalf("adjust: %v", err)
		}
		if result.Outcome() != application.AdvanceNotAccepted {
			t.Fatalf("outcome = %q; 调整出了无中生有的金额", result.Outcome())
		}
	})

	t.Run("a replay returns the original adjustment", func(t *testing.T) {
		replay, err := fixture.handler.Adjust(context.Background(), adjust)
		if err != nil {
			t.Fatalf("replay: %v", err)
		}
		if replay.Outcome() != application.AdjustmentExistingResult {
			t.Fatalf("outcome = %q", replay.Outcome())
		}
	})

	t.Run("a different content under the same adjustment identity is a conflict", func(t *testing.T) {
		flipped := adjust
		flipped.AmountMinor = 900
		result, err := fixture.handler.Adjust(context.Background(), flipped)
		if err != nil {
			t.Fatalf("conflict adjust: %v", err)
		}
		if result.Outcome() != application.AdjustmentConflict {
			t.Fatalf("outcome = %q", result.Outcome())
		}
	})
}

// Covers: ADR-0029（依赖故障归未决且指名等谁）、ADR-0031（写入代数封闭）与 ADR-0043
// （投递失败不翻结果、重放重发同一份）在本编排的恢复面；未决原因集封闭。
func TestAdvanceRecoveryDiscipline(t *testing.T) {
	t.Run("store and view failures are undecided with their reasons", func(t *testing.T) {
		fixture := newAdvanceFixture(t)
		fixture.assessments.findErr = errors.New("store down")
		result, err := fixture.handler.Assess(context.Background(), assessCommand(t, domain.AdvanceEstablished))
		if err != nil {
			t.Fatalf("assess: %v", err)
		}
		if result.UndecidedReason() != application.AssessmentStoreUnavailable {
			t.Fatalf("reason = %q", result.UndecidedReason())
		}

		viewFixture := newAdvanceFixture(t)
		if _, err := viewFixture.handler.Assess(context.Background(), assessCommand(t, domain.AdvanceEstablished)); err != nil {
			t.Fatalf("assess: %v", err)
		}
		viewFixture.contracts.err = errors.New("view down")
		result, err = viewFixture.handler.FormRecovery(context.Background(), formRecoveryCommand(t))
		if err != nil {
			t.Fatalf("form: %v", err)
		}
		if result.UndecidedReason() != application.ContractViewUnavailable {
			t.Fatalf("reason = %q", result.UndecidedReason())
		}
	})

	t.Run("a handoff failure keeps the outcome and is resent on replay", func(t *testing.T) {
		fixture := newAdvanceFixture(t)
		if _, err := fixture.handler.Assess(context.Background(), assessCommand(t, domain.AdvanceEstablished)); err != nil {
			t.Fatalf("assess: %v", err)
		}
		fixture.handoff.err = errors.New("downstream unavailable")
		first, err := fixture.handler.FormRecovery(context.Background(), formRecoveryCommand(t))
		if err != nil {
			t.Fatalf("form: %v", err)
		}
		if first.Outcome() != application.RecoveryFormed || first.RecoveryHandoffReference() == "" {
			t.Fatalf("outcome = %q handoff = %q（投递失败不翻结果）", first.Outcome(), first.RecoveryHandoffReference())
		}
		fixture.handoff.err = nil
		replay, err := fixture.handler.FormRecovery(context.Background(), formRecoveryCommand(t))
		if err != nil {
			t.Fatalf("replay: %v", err)
		}
		if replay.RecoveryHandoffReference() != "" || len(fixture.handoff.intents) != 1 {
			t.Fatalf("intents = %d handoff = %q", len(fixture.handoff.intents), replay.RecoveryHandoffReference())
		}
	})

	t.Run("the undecided reason set is closed", func(t *testing.T) {
		labels := map[string]struct{}{}
		for _, reason := range []application.AdvanceUndecidedReason{
			application.AssessmentStoreUnavailable, application.RecoveryStoreUnavailable,
			application.AdjustmentStoreUnavailable, application.ContractViewUnavailable,
			application.ContractUnconfigured,
		} {
			label := reason.String()
			if label == "" {
				t.Fatalf("reason %d has no label", reason)
			}
			labels[label] = struct{}{}
		}
		if len(labels) != 5 {
			t.Fatalf("labels collapsed into %d", len(labels))
		}
		if application.AdvanceUndecidedReason(len(labels)+1).String() != "" {
			t.Fatal("第六个未决原因带了标签——封闭集合被悄悄放开")
		}
	})
}
