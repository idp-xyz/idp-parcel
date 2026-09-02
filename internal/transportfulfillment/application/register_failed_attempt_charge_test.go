package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证失败尝试费发生项的登记用例（票 tf-unwired-seven/04，ADR-0098）：采购上下文由
// 调用方显式给出而不推导；**自营揽收失败不形成发生项，且那是正确答案不是缺席**；成功的
// 对象结果被拒（AT-TF-094）。夹具全部为合成登记（S 级）。

var chargeArrivedAt = time.Date(2026, 9, 11, 9, 0, 0, 0, time.UTC)

type chargeAttemptViewDouble struct {
	record ports.PickupAttemptRecord
	found  bool
	err    error
	calls  int
}

func (double *chargeAttemptViewDouble) FindByKey(
	_ context.Context,
	_ ports.PickupAttemptKey,
) (ports.PickupAttemptRecord, bool, error) {
	double.calls++
	if double.err != nil {
		return ports.PickupAttemptRecord{}, false, double.err
	}
	return double.record, double.found, nil
}

type chargeRegistryDouble struct {
	saved   []ports.ChargeOccurrenceRecord
	outcome ports.ChargeOccurrenceSaveOutcome
	err     error
}

func (double *chargeRegistryDouble) FindByKey(
	_ context.Context,
	_ ports.ChargeOccurrenceKey,
) (ports.ChargeOccurrenceRecord, bool, error) {
	return ports.ChargeOccurrenceRecord{}, false, nil
}

func (double *chargeRegistryDouble) Save(
	_ context.Context,
	record ports.ChargeOccurrenceRecord,
) (ports.ChargeOccurrenceSaveOutcome, error) {
	if double.err != nil {
		return ports.ChargeOccurrenceSaveOutcomeInvalid, double.err
	}
	double.saved = append(double.saved, record)
	if double.outcome != ports.ChargeOccurrenceSaveOutcomeInvalid {
		return double.outcome, nil
	}
	return ports.ChargeOccurrenceSaved, nil
}

type chargeClockDouble struct{ now time.Time }

func (clock chargeClockDouble) Now() time.Time { return clock.now }

func chargeValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}

// attemptRecordWith 造一份含单对象结果的揽收尝试记录。
func attemptRecordWith(t *testing.T, outcome domain.AttemptObjectOutcome) ports.PickupAttemptRecord {
	t.Helper()
	object := chargeValue(t, domain.NewCarriedObjectReference, "parcel-1")
	spec := domain.FulfillmentAttemptSpec{
		TenantID:    chargeValue(t, domain.NewTenantID, "tenant-1"),
		Attempt:     chargeValue(t, domain.NewAttemptReference, "attempt-1"),
		Task:        chargeValue(t, domain.NewDispatchTaskReference, "task-1"),
		ExecutedBy:  chargeValue(t, domain.NewExecutingPartyReference, "courier-1"),
		Place:       chargeValue(t, domain.NewAttemptPlaceReference, "place-1"),
		Evidence:    chargeValue(t, domain.NewAttemptEvidenceReference, "evidence-1"),
		Objects:     []domain.CarriedObjectReference{object},
		PlannedFrom: chargeArrivedAt.Add(-time.Hour),
		PlannedTo:   chargeArrivedAt.Add(time.Hour),
		ArrivedAt:   chargeArrivedAt,
	}
	attempt, err := domain.FormFulfillmentAttempt(spec)
	if err != nil {
		t.Fatalf("构造尝试：%v", err)
	}
	var basis domain.AttemptResultBasisReference
	if outcome.Failed() {
		basis = chargeValue(t, domain.NewAttemptResultBasisReference, "basis-absent")
	}
	result, err := domain.FormAttemptObjectResult(attempt, object, outcome, basis, chargeArrivedAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("构造对象结果：%v", err)
	}
	return ports.PickupAttemptRecord{
		Key:     ports.PickupAttemptKey{TenantID: spec.TenantID, SourceID: "source-1"},
		Attempt: attempt,
		Results: []domain.AttemptObjectResult{result},
	}
}

func chargeCommand(t *testing.T) application.RegisterFailedAttemptChargeCommand {
	t.Helper()
	return application.RegisterFailedAttemptChargeCommand{
		TenantID:    chargeValue(t, domain.NewTenantID, "tenant-1"),
		SourceID:    "source-1",
		Object:      "parcel-1",
		Occurrence:  "OCC-0001",
		Journey:     "journey-1",
		LegalEntity: "legal-1",
		Provider:    "partner-1",
		Agreement:   "agreement-1/v1",
		Scope:       "scope-1",
		Quantity:    1,
		Unit:        "attempt",
		Validity:    "v1",
	}
}

func newChargeHandler(
	view *chargeAttemptViewDouble,
	registry *chargeRegistryDouble,
) *application.RegisterFailedAttemptChargeHandler {
	return application.NewRegisterFailedAttemptChargeHandler(application.FailedAttemptChargeDeps{
		Attempts:    view,
		Occurrences: registry,
		Clock:       chargeClockDouble{now: chargeArrivedAt.Add(2 * time.Hour)},
	})
}

// Covers: AT-TF-094 与 ADR-0098 决定二——采购上下文由调用方给出，发生项原样带着它入册。
func TestAFailedAttemptFormsAnOccurrenceCarryingTheGivenProcurementContext(t *testing.T) {
	view := &chargeAttemptViewDouble{record: attemptRecordWith(t, domain.CustomerAbsent), found: true}
	registry := &chargeRegistryDouble{}

	result, err := newChargeHandler(view, registry).Register(t.Context(), chargeCommand(t))
	if err != nil {
		t.Fatalf("登记：%v", err)
	}
	if result.Outcome() != application.FailedAttemptChargeRegistered {
		t.Fatalf("outcome = %q, want CHARGE_REGISTERED", result.Outcome())
	}
	if len(registry.saved) != 1 {
		t.Fatalf("入册 %d 条", len(registry.saved))
	}
	occurrence := registry.saved[0].Occurrence
	if occurrence.Reason() != domain.FailedAttemptOccurrence {
		t.Fatalf("发生原因 = %q", occurrence.Reason())
	}
	if occurrence.Provider().String() != "partner-1" || occurrence.Agreement().String() != "agreement-1/v1" {
		t.Fatalf("采购上下文没有原样带上：%q %q", occurrence.Provider(), occurrence.Agreement())
	}
	// 事实依据与业务时间取自结果本身，不取自命令，也不取自时钟。
	if !occurrence.OccurredAt().Equal(chargeArrivedAt.Add(time.Hour)) {
		t.Fatalf("业务时间 = %v，应取自对象结果", occurrence.OccurredAt())
	}
	if occurrence.FactBasis().String() != "ATTEMPT-RESULT/attempt-1/parcel-1" {
		t.Fatalf("事实依据 = %q", occurrence.FactBasis())
	}
}

// Covers: ADR-0098 决定三——自营揽收（无协议快照）失败**不形成发生项，而这是正确答案**。
// 它既不是未决也不是错误：三者的恢复动作分别是「什么都不用做」「重试」「去修」。
func TestASelfOperatedFailedAttemptIsNotApplicableRatherThanUndecidedOrRejected(t *testing.T) {
	view := &chargeAttemptViewDouble{record: attemptRecordWith(t, domain.CustomerAbsent), found: true}
	registry := &chargeRegistryDouble{}

	command := chargeCommand(t)
	command.Agreement = "   "

	result, err := newChargeHandler(view, registry).Register(t.Context(), command)
	if err != nil {
		t.Fatalf("自营那一格不该上抛：%v", err)
	}
	if result.Outcome() != application.FailedAttemptChargeNotApplicable {
		t.Fatalf("outcome = %q, want NOT_APPLICABLE", result.Outcome())
	}
	if result.Reason() != application.SelfOperatedHasNoExternalCostSource {
		t.Fatalf("reason = %q", result.Reason())
	}
	if len(registry.saved) != 0 {
		t.Fatal("自营揽收凭空落了一条外部成本来源")
	}
}

// Covers: 领域 ErrNotAFailedAttempt——揽收到手的结果被拒，且这是业务答案不是技术错误。
func TestASucceededObjectResultDoesNotFormACharge(t *testing.T) {
	view := &chargeAttemptViewDouble{record: attemptRecordWith(t, domain.ObjectPickedUp), found: true}
	registry := &chargeRegistryDouble{}

	result, err := newChargeHandler(view, registry).Register(t.Context(), chargeCommand(t))
	if err != nil {
		t.Fatalf("成功结果不该上抛：%v", err)
	}
	if result.Outcome() != application.FailedAttemptChargeNotApplicable {
		t.Fatalf("outcome = %q, want NOT_APPLICABLE", result.Outcome())
	}
	if result.Reason() != application.AttemptObjectDidNotFail {
		t.Fatalf("reason = %q, want ATTEMPT_DID_NOT_FAIL", result.Reason())
	}
	if len(registry.saved) != 0 {
		t.Fatal("给一次成功的揽收开了失败尝试费")
	}
}

// Covers: 本仓编排纪律——依赖读不回形成本上下文自己的未决并带续办引用，不上抛技术错误。
// 而「读不回」与「这个对象没有结果」必须分得开：前者重试，后者回去查是不是问错了对象。
func TestUnreadableAttemptIsUndecidedWhileAMissingObjectIsNotAccepted(t *testing.T) {
	unreadable := &chargeAttemptViewDouble{err: errors.New("store down")}
	result, err := newChargeHandler(unreadable, &chargeRegistryDouble{}).Register(t.Context(), chargeCommand(t))
	if err != nil {
		t.Fatalf("读失败不该上抛：%v", err)
	}
	if result.Outcome() != application.FailedAttemptChargeUndecided ||
		result.Reason() != application.AttemptStoreUnavailable {
		t.Fatalf("outcome/reason = %q/%q", result.Outcome(), result.Reason())
	}
	if result.Continuation() == "" {
		t.Fatal("未决没有续办引用")
	}

	missing := &chargeAttemptViewDouble{record: attemptRecordWith(t, domain.CustomerAbsent), found: true}
	command := chargeCommand(t)
	command.Object = "parcel-not-in-attempt"
	other, err := newChargeHandler(missing, &chargeRegistryDouble{}).Register(t.Context(), command)
	if err != nil {
		t.Fatalf("对象不在尝试内不该上抛：%v", err)
	}
	if other.Outcome() != application.FailedAttemptChargeInputNotAccepted {
		t.Fatalf("outcome = %q, want INPUT_NOT_ACCEPTED", other.Outcome())
	}
}

// Covers: 最小身份先于任何权威读取——身份立不住时不去读库。
func TestBadIdentityIsRefusedBeforeReadingTheAttempt(t *testing.T) {
	view := &chargeAttemptViewDouble{}
	command := chargeCommand(t)
	command.SourceID = "  "

	result, err := newChargeHandler(view, &chargeRegistryDouble{}).Register(t.Context(), command)
	if err != nil {
		t.Fatalf("身份不受理不该上抛：%v", err)
	}
	if result.Outcome() != application.FailedAttemptChargeInputNotAccepted {
		t.Fatalf("outcome = %q", result.Outcome())
	}
	if view.calls != 0 {
		t.Fatalf("身份立不住却读了 %d 次库", view.calls)
	}
}

// Covers: 撞键是业务答案（ADR-0031），重放交回`已登记`而不是再落一条。
func TestReplayingTheSameOccurrenceAnswersAlreadyRegistered(t *testing.T) {
	view := &chargeAttemptViewDouble{record: attemptRecordWith(t, domain.CustomerAbsent), found: true}
	registry := &chargeRegistryDouble{outcome: ports.ChargeOccurrenceAlreadyRegistered}

	result, err := newChargeHandler(view, registry).Register(t.Context(), chargeCommand(t))
	if err != nil {
		t.Fatalf("重放：%v", err)
	}
	if result.Outcome() != application.FailedAttemptChargeExisting {
		t.Fatalf("outcome = %q, want CHARGE_EXISTING", result.Outcome())
	}
}
