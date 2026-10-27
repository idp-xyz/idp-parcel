package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证待确认的逐对象尝试结果凭同一次上门的后补来源更正成确定结果在编排面上的走法（票 docs-consistency/code/90）：
// 更正只回指待确认那一版、按内容分重放与冲突、更正成揽收到手经对象级揽收登记形成场外揽收并进段。夹具全是合成串，证据只记 S。

type pickupCorrectionsDouble struct {
	attempts *pickupStoreDouble
	stored   map[string]ports.PickupResultCorrectionRecord
	loads    int
	saves    int
	loadErr  error
	saveErr  error
}

func newPickupCorrections(attempts *pickupStoreDouble) *pickupCorrectionsDouble {
	return &pickupCorrectionsDouble{attempts: attempts, stored: map[string]ports.PickupResultCorrectionRecord{}}
}

func (double *pickupCorrectionsDouble) LoadForCorrection(
	_ context.Context,
	key ports.PickupAttemptKey,
	object domain.CarriedObjectReference,
) (ports.PickupResultCorrectionTarget, bool, error) {
	double.loads++
	if double.loadErr != nil {
		return ports.PickupResultCorrectionTarget{}, false, double.loadErr
	}
	record, found := double.attempts.records[key.SourceID]
	if !found {
		return ports.PickupResultCorrectionTarget{}, false, nil
	}
	for _, result := range record.Results {
		if result.Object() != object {
			continue
		}
		target := ports.PickupResultCorrectionTarget{Attempt: record.Attempt, Result: result}
		target.Correction, target.HasCorrection = double.stored[key.SourceID+"|"+object.String()]
		return target, true, nil
	}
	return ports.PickupResultCorrectionTarget{}, false, nil
}

func (double *pickupCorrectionsDouble) Save(
	_ context.Context,
	record ports.PickupResultCorrectionRecord,
) (ports.PickupResultCorrectionSaveOutcome, error) {
	double.saves++
	if double.saveErr != nil {
		return ports.PickupResultCorrectionSaveOutcomeInvalid, double.saveErr
	}
	id := record.Key.SourceID + "|" + record.Correction.Pending().Object().String()
	if _, exists := double.stored[id]; exists {
		return ports.PickupResultCorrectionAlreadyRecorded, nil
	}
	double.stored[id] = record
	return ports.PickupResultCorrectionSaved, nil
}

type pickupCorrectionFixture struct {
	pending     *pendingPickupFixture
	registry    *pickupRegistryDouble
	segments    *segmentRegistryDouble
	regHandoff  *pickupRegHandoffDouble
	corrections *pickupCorrectionsDouble
	handler     *application.CorrectPickupAttemptResultHandler
}

// newPickupCorrectionFixture 先经尝试登记口登下一次到访：parcel-1 待确认、parcel-2 揽收到手、parcel-3 客户不在。
func newPickupCorrectionFixture(t *testing.T) *pickupCorrectionFixture {
	t.Helper()
	pending := newPendingPickupFixture(t)
	command := pickupCommand(t, "source-1", "attempt-1",
		pendingPickupSubmission(t, "parcel-1"),
		successSubmission(t, "parcel-2", "TRANSPORT-CONTROL/TF-2"),
		failureSubmission(t, "parcel-3", domain.CustomerAbsent, "reason-absent-1"))
	if result := pending.handle(t, command); result.Outcome() != application.PickupAttemptRecorded {
		t.Fatalf("recording the visit = %q", result.Outcome())
	}

	fixture := &pickupCorrectionFixture{
		pending:    pending,
		registry:   newPickupRegistry(),
		segments:   newSegmentRegistry(),
		regHandoff: &pickupRegHandoffDouble{},
	}
	fixture.corrections = newPickupCorrections(pending.store)
	registrar := application.NewRegisterOffsitePickupHandler(application.RegisterOffsitePickupDeps{
		Pickups:    fixture.registry,
		Segments:   fixture.segments,
		Versions:   &pickupRegVersionFactory{},
		Downstream: fixture.regHandoff,
		Clock:      pickupRegClock{at: recordedAt},
	})
	fixture.handler = application.NewCorrectPickupAttemptResultHandler(application.CorrectPickupAttemptResultDeps{
		Corrections: fixture.corrections,
		Pickups:     registrar,
		Clock:       fixedClock{at: recordedAt.Add(time.Hour)},
	})
	return fixture
}

func (fixture *pickupCorrectionFixture) command(t *testing.T) application.CorrectPickupAttemptResultCommand {
	t.Helper()
	return application.CorrectPickupAttemptResultCommand{
		TenantID:    value(t, domain.NewTenantID, "tenant-1"),
		SourceID:    "source-1",
		Object:      "parcel-1",
		Outcome:     domain.CustomerAbsent,
		Basis:       value(t, domain.NewAttemptResultBasisReference, "reason/partner-confirms-absent"),
		Source:      value(t, domain.NewAttemptResultSourceReference, "source/partner-late-status"),
		OccurredAt:  arrivedAt.Add(2 * time.Minute),
		CorrectedAt: arrivedAt.Add(3 * time.Hour),
	}
}

func (fixture *pickupCorrectionFixture) pickedUpCommand(t *testing.T) application.CorrectPickupAttemptResultCommand {
	t.Helper()
	command := fixture.command(t)
	command.Outcome = domain.ObjectPickedUp
	command.Basis = domain.AttemptResultBasisReference{}
	command.Control = value(t, domain.NewTransportControlReference, "TRANSPORT-CONTROL/TF-1")
	command.Segment = "segment-1"
	return command
}

func (fixture *pickupCorrectionFixture) correct(t *testing.T, command application.CorrectPickupAttemptResultCommand) application.CorrectPickupAttemptResultResult {
	t.Helper()
	result, err := fixture.handler.Correct(context.Background(), command)
	if err != nil {
		t.Fatalf("correct: %v", err)
	}
	return result
}

// Covers: TF CONTEXT「揽收、派送与交付」「之后同一对象的确定结果，凭同一次上门的后补来源以更正形成新判断版本……待确认那一版留在册上
// 不改写」（揽收，失败一族）——待确认更正成失败：更正落册、带着所凭来源，首次提交的那一格一个字不动，也不形成场外揽收、不进段。
func TestAPendingPickupResultIsCorrectedToAFailureWithoutTouchingThePendingVersion(t *testing.T) {
	fixture := newPickupCorrectionFixture(t)

	result := fixture.correct(t, fixture.command(t))
	if result.Outcome() != application.AttemptResultCorrected {
		t.Fatalf("outcome = %q, want RESULT_CORRECTED", result.Outcome())
	}
	record, _ := result.Record()
	if record.Correction.Effective().Outcome() != domain.CustomerAbsent {
		t.Fatalf("effective outcome = %q", record.Correction.Effective().Outcome())
	}
	if record.Correction.Source().String() != "source/partner-late-status" {
		t.Fatalf("correction source = %q；更正丢了所凭的后补来源", record.Correction.Source())
	}
	if kept := record.Correction.Pending(); !kept.Outcome().PendingConfirmation() {
		t.Fatalf("pending version = %q；待确认那一版被改写", kept.Outcome())
	}
	stored := fixture.pending.store.records["source-1"]
	for _, original := range stored.Results {
		if original.Object().String() == "parcel-1" && !original.Outcome().PendingConfirmation() {
			t.Fatal("首次提交的结果行被更正改写了")
		}
	}
	if fixture.registry.saves != 0 || len(fixture.regHandoff.intents) != 0 {
		t.Fatalf("registry saves = %d intents = %d；失败结果不形成场外揽收，也没有东西交 parcel-shipment",
			fixture.registry.saves, len(fixture.regHandoff.intents))
	}
}

// Covers: 同上（揽收，揽收到手）与 UC-TF-002 步骤 6A——更正成揽收到手经对象级揽收登记形成场外揽收：控制依据随揽收保全、
// 地点与执行方取自那次尝试、同笔进声明的段、交 parcel-shipment 采认；待确认那一版仍留在册上。
func TestAPendingPickupResultCorrectedToAPickupFormsTheOffsitePickupAndEntersTheSegment(t *testing.T) {
	fixture := newPickupCorrectionFixture(t)

	result := fixture.correct(t, fixture.pickedUpCommand(t))
	if result.Outcome() != application.AttemptResultCorrected {
		t.Fatalf("outcome = %q, want RESULT_CORRECTED", result.Outcome())
	}
	registered, found, err := fixture.registry.FindByKey(context.Background(), ports.OffsitePickupKey{
		TenantID: value(t, domain.NewTenantID, "tenant-1"),
		Object:   value(t, domain.NewCarriedObjectReference, "parcel-1"),
		Attempt:  value(t, domain.NewAttemptReference, "attempt-1"),
	})
	if err != nil || !found {
		t.Fatalf("registered pickup found=%v err=%v；更正成揽收到手没有形成场外揽收", found, err)
	}
	pickup := registered.Pickup
	if pickup.Control().String() != "TRANSPORT-CONTROL/TF-1" || pickup.Task().String() != "pickup-task-1" ||
		pickup.Place().String() != "customer-warehouse-1" || pickup.ExecutedBy().String() != "courier-1" {
		t.Fatalf("pickup = control %q task %q place %q executedBy %q；控制取自更正，其余取自那次尝试",
			pickup.Control(), pickup.Task(), pickup.Place(), pickup.ExecutedBy())
	}
	if !pickup.OccurredAt().Equal(arrivedAt.Add(2 * time.Minute)) {
		t.Fatalf("pickup occurredAt = %v；责任起点取更正所说的业务时间", pickup.OccurredAt())
	}
	if len(fixture.regHandoff.intents) != 1 {
		t.Fatalf("intents = %d, want 1（交 parcel-shipment 采认）", len(fixture.regHandoff.intents))
	}
	segment := fixture.segments.saved(t, "tenant-1", "segment-1")
	if _, joined := segment.Segment.ParticipationFor(value(t, domain.NewCarriedObjectReference, "parcel-1")); !joined {
		t.Fatal("更正成揽收到手的对象没有进声明的段")
	}
	record, _ := result.Record()
	if record.Control.String() != "TRANSPORT-CONTROL/TF-1" {
		t.Fatalf("stored control = %q；重放比对要用", record.Control)
	}
	if !record.Correction.Pending().Outcome().PendingConfirmation() {
		t.Fatal("待确认那一版被改写")
	}
}

// Covers: UC-TF-002「同一尝试来源身份和内容返回已有结果；同一身份不同内容形成冲突」与「同一对象在同一尝试中只能有一个当前有效
// 结果」——同一格的同内容重放答已有更正、不再登记、不再进段；换了说法答冲突，在册的更正不被顶替。
func TestACorrectionReplayAnswersTheExistingOneAndADifferentStatementConflicts(t *testing.T) {
	fixture := newPickupCorrectionFixture(t)
	first := fixture.correct(t, fixture.pickedUpCommand(t))
	if first.Outcome() != application.AttemptResultCorrected {
		t.Fatalf("first = %q", first.Outcome())
	}

	replay := fixture.correct(t, fixture.pickedUpCommand(t))
	if replay.Outcome() != application.AttemptResultCorrectionExisting {
		t.Fatalf("replay = %q, want EXISTING_CORRECTION", replay.Outcome())
	}
	if fixture.registry.saves != 1 || fixture.corrections.saves != 1 || len(fixture.regHandoff.intents) != 1 {
		t.Fatalf("registry saves = %d correction saves = %d intents = %d；重放登记了第二份", fixture.registry.saves,
			fixture.corrections.saves, len(fixture.regHandoff.intents))
	}

	for name, change := range map[string]func(*application.CorrectPickupAttemptResultCommand){
		"another outcome": func(command *application.CorrectPickupAttemptResultCommand) {
			command.Outcome = domain.GoodsNotReady
			command.Basis = value(t, domain.NewAttemptResultBasisReference, "reason/other")
			command.Control = domain.TransportControlReference{}
			command.Segment = ""
		},
		"another control": func(command *application.CorrectPickupAttemptResultCommand) {
			command.Control = value(t, domain.NewTransportControlReference, "TRANSPORT-CONTROL/TF-OTHER")
		},
		"another source": func(command *application.CorrectPickupAttemptResultCommand) {
			command.Source = value(t, domain.NewAttemptResultSourceReference, "source/other")
		},
		"another business time": func(command *application.CorrectPickupAttemptResultCommand) {
			command.OccurredAt = command.OccurredAt.Add(time.Minute)
		},
	} {
		t.Run(name, func(t *testing.T) {
			command := fixture.pickedUpCommand(t)
			change(&command)
			if conflict := fixture.correct(t, command); conflict.Outcome() != application.AttemptResultCorrectionConflict {
				t.Fatalf("outcome = %q, want SOURCE_CONFLICT", conflict.Outcome())
			}
		})
	}
	record, _ := fixture.corrections.stored["source-1|parcel-1"], true
	if record.Correction.Effective().Outcome() != domain.ObjectPickedUp {
		t.Fatalf("stored outcome = %q；在册的更正被后来的说法顶替了", record.Correction.Effective().Outcome())
	}
}

// Covers: ADR-0029 按恢复动作分格——更正只回指待确认那一版：已是确定结果的那一格答`并非待确认`（不再更正，也不写任何东西）；
// 指错尝试或对象、形状不成的提交答`来源未受理`，后者连库都不读。
func TestOnlyAPendingVersionIsCorrectedAndMalformedCorrectionsAreNotAccepted(t *testing.T) {
	fixture := newPickupCorrectionFixture(t)

	for _, object := range []string{"parcel-2", "parcel-3"} {
		command := fixture.command(t)
		command.Object = object
		if result := fixture.correct(t, command); result.Outcome() != application.AttemptResultNotPending {
			t.Fatalf("%s outcome = %q, want RESULT_NOT_PENDING", object, result.Outcome())
		}
	}
	unknown := fixture.command(t)
	unknown.SourceID = "source-unknown"
	if result := fixture.correct(t, unknown); result.Outcome() != application.AttemptResultCorrectionNotAccepted {
		t.Fatalf("unknown source outcome = %q", result.Outcome())
	}
	outside := fixture.command(t)
	outside.Object = "parcel-9"
	if result := fixture.correct(t, outside); result.Outcome() != application.AttemptResultCorrectionNotAccepted {
		t.Fatalf("outside object outcome = %q", result.Outcome())
	}
	if fixture.corrections.saves != 0 || fixture.registry.saves != 0 {
		t.Fatalf("correction saves = %d registry saves = %d；拒绝的更正写了东西", fixture.corrections.saves, fixture.registry.saves)
	}

	loadsBefore := fixture.corrections.loads
	malformed := map[string]func(*application.CorrectPickupAttemptResultCommand){
		"into pending again": func(c *application.CorrectPickupAttemptResultCommand) { c.Outcome = domain.PickupPendingConfirmation },
		"no outcome":         func(c *application.CorrectPickupAttemptResultCommand) { c.Outcome = domain.AttemptObjectOutcomeInvalid },
		"no correcting source": func(c *application.CorrectPickupAttemptResultCommand) {
			c.Source = domain.AttemptResultSourceReference{}
		},
		"a failure carrying a control basis": func(c *application.CorrectPickupAttemptResultCommand) {
			c.Control = value(t, domain.NewTransportControlReference, "TRANSPORT-CONTROL/TF-1")
		},
		"a failure declaring a segment": func(c *application.CorrectPickupAttemptResultCommand) { c.Segment = "segment-1" },
		"a pickup without a control basis": func(c *application.CorrectPickupAttemptResultCommand) {
			c.Outcome = domain.ObjectPickedUp
			c.Basis = domain.AttemptResultBasisReference{}
		},
		"a failure without a basis": func(c *application.CorrectPickupAttemptResultCommand) { c.Basis = domain.AttemptResultBasisReference{} },
		"a blank decider":           func(c *application.CorrectPickupAttemptResultCommand) { c.DecidedBy = "  " },
		"a decider with NUL":        func(c *application.CorrectPickupAttemptResultCommand) { c.DecidedBy = "issuer#sub\x00" },
		"no tenant":                 func(c *application.CorrectPickupAttemptResultCommand) { c.TenantID = domain.TenantID{} },
		"no source id":              func(c *application.CorrectPickupAttemptResultCommand) { c.SourceID = " " },
		"before the arrival":        func(c *application.CorrectPickupAttemptResultCommand) { c.OccurredAt = arrivedAt.Add(-time.Minute) },
		"corrected before pending":  func(c *application.CorrectPickupAttemptResultCommand) { c.CorrectedAt = arrivedAt.Add(3 * time.Minute) },
	}
	for name, change := range malformed {
		t.Run(name, func(t *testing.T) {
			command := fixture.command(t)
			change(&command)
			if result := fixture.correct(t, command); result.Outcome() != application.AttemptResultCorrectionNotAccepted {
				t.Fatalf("outcome = %q, want SOURCE_NOT_ACCEPTED", result.Outcome())
			}
		})
	}
	if fixture.corrections.saves != 0 {
		t.Fatalf("correction saves = %d", fixture.corrections.saves)
	}
	_ = loadsBefore
}

// Covers: 要裁的「揽收一侧更正成揽收到手，与对象级登记口同（对象，尝试）的那一版谁是当前有效」——同（对象，尝试）已经经对象级登记
// 口登记了同内容的揽收（此前的绕法）：更正收口它，不登记第二份、不再进一次段；登记的是另一回事（控制依据不同）则更正答冲突、
// 不写更正——要改那份揽收走揽收更正口，更正不顶替它。
func TestACorrectionToAPickupAdoptsTheObjectLevelRegistrationInsteadOfFormingASecond(t *testing.T) {
	registeredFirst := func(t *testing.T, fixture *pickupCorrectionFixture, control string) {
		t.Helper()
		registrar := application.NewRegisterOffsitePickupHandler(application.RegisterOffsitePickupDeps{
			Pickups: fixture.registry, Versions: &pickupRegVersionFactory{}, Downstream: fixture.regHandoff,
			Clock: pickupRegClock{at: recordedAt},
		})
		result, err := registrar.Register(context.Background(), application.RegisterOffsitePickupCommand{
			TenantID: value(t, domain.NewTenantID, "tenant-1"), Object: "parcel-1", Task: "pickup-task-1", Attempt: "attempt-1",
			Place: "customer-warehouse-1", Control: control, ExecutedBy: "courier-1", OccurredAt: arrivedAt.Add(2 * time.Minute),
		})
		if err != nil || result.Outcome() != application.PickupRegistered {
			t.Fatalf("prior registration = %v err=%v", result.Outcome(), err)
		}
	}

	t.Run("the same content closes the gap", func(t *testing.T) {
		fixture := newPickupCorrectionFixture(t)
		registeredFirst(t, fixture, "TRANSPORT-CONTROL/TF-1")
		result := fixture.correct(t, fixture.pickedUpCommand(t))
		if result.Outcome() != application.AttemptResultCorrected {
			t.Fatalf("outcome = %q, want RESULT_CORRECTED", result.Outcome())
		}
		if fixture.registry.saves != 1 {
			t.Fatalf("registry saves = %d, want 1；同一对象同一尝试长出了两份揽收", fixture.registry.saves)
		}
	})
	t.Run("another control conflicts and writes no correction", func(t *testing.T) {
		fixture := newPickupCorrectionFixture(t)
		registeredFirst(t, fixture, "TRANSPORT-CONTROL/TF-OTHER")
		result := fixture.correct(t, fixture.pickedUpCommand(t))
		if result.Outcome() != application.AttemptResultCorrectionConflict {
			t.Fatalf("outcome = %q, want SOURCE_CONFLICT", result.Outcome())
		}
		if fixture.corrections.saves != 0 {
			t.Fatal("冲突的更正落了册")
		}
	})
}

// Covers: ADR-0029——读不动才是未决：登记册读不动答`未决`（点名是对象级揽收登记那一条路，等它恢复重试同一份）、更正册读写不动答`未决`
// （点名更正册）；未决不落更正。更正成揽收到手而装配没交入对象级揽收登记，一律不受理，不会悄悄只改结果。
func TestCorrectionDependenciesThatCannotAnswerAreUndecidedNotRefused(t *testing.T) {
	t.Run("the registry cannot be read", func(t *testing.T) {
		fixture := newPickupCorrectionFixture(t)
		fixture.registry.findErr = errors.New("registry down")
		result := fixture.correct(t, fixture.pickedUpCommand(t))
		if result.Outcome() != application.AttemptResultCorrectionUndecided ||
			result.UndecidedReason() != application.AttemptResultCorrectionPickupRegistrationUnsettled ||
			!strings.HasPrefix(result.ContinuationReference(), "CONT-") {
			t.Fatalf("outcome = %q reason = %q continuation = %q", result.Outcome(), result.UndecidedReason(), result.ContinuationReference())
		}
		if fixture.corrections.saves != 0 {
			t.Fatal("未决的更正落了册")
		}
	})
	t.Run("the correction store cannot be read", func(t *testing.T) {
		fixture := newPickupCorrectionFixture(t)
		fixture.corrections.loadErr = errors.New("store down")
		result := fixture.correct(t, fixture.command(t))
		if result.Outcome() != application.AttemptResultCorrectionUndecided ||
			result.UndecidedReason() != application.AttemptResultCorrectionStoreUnavailable {
			t.Fatalf("outcome = %q reason = %q", result.Outcome(), result.UndecidedReason())
		}
	})
	t.Run("the correction cannot be written", func(t *testing.T) {
		fixture := newPickupCorrectionFixture(t)
		fixture.corrections.saveErr = errors.New("store down")
		result := fixture.correct(t, fixture.command(t))
		if result.Outcome() != application.AttemptResultCorrectionUndecided ||
			result.UndecidedReason() != application.AttemptResultCorrectionStoreUnavailable {
			t.Fatalf("outcome = %q reason = %q", result.Outcome(), result.UndecidedReason())
		}
	})
	t.Run("a pickup without a registrar is not accepted", func(t *testing.T) {
		fixture := newPickupCorrectionFixture(t)
		handler := application.NewCorrectPickupAttemptResultHandler(application.CorrectPickupAttemptResultDeps{
			Corrections: fixture.corrections, Clock: fixedClock{at: recordedAt},
		})
		result, err := handler.Correct(context.Background(), fixture.pickedUpCommand(t))
		if err != nil || result.Outcome() != application.AttemptResultCorrectionNotAccepted {
			t.Fatalf("outcome = %q err = %v", result.Outcome(), err)
		}
	})
}

// Covers: ADR-0303 的待遇（沿用于这条更正）——运营决定面提交的操作者作为操作证据记在更正上，只进审计：同一更正换一位操作者重放
// 仍答已有、在册的不被顶替。
func TestTheSubmittingOperatorIsKeptAsEvidenceAndNeverCompared(t *testing.T) {
	fixture := newPickupCorrectionFixture(t)
	command := fixture.command(t)
	command.DecidedBy = "issuer#reviewer-1"
	first := fixture.correct(t, command)
	record, _ := first.Record()
	if record.DecidedBy.String() != "issuer#reviewer-1" {
		t.Fatalf("decidedBy = %q", record.DecidedBy)
	}

	command.DecidedBy = "issuer#reviewer-2"
	replay := fixture.correct(t, command)
	if replay.Outcome() != application.AttemptResultCorrectionExisting {
		t.Fatalf("replay by another operator = %q, want EXISTING_CORRECTION", replay.Outcome())
	}
	if kept, _ := replay.Record(); kept.DecidedBy.String() != "issuer#reviewer-1" {
		t.Fatalf("kept decidedBy = %q；先到的操作者被顶替", kept.DecidedBy)
	}

	command = fixture.command(t)
	command.Object = "parcel-2"
	if result := fixture.correct(t, command); result.Outcome() != application.AttemptResultNotPending {
		t.Fatalf("outcome = %q", result.Outcome())
	}
}

// --- 派送一侧 ---

type deliveryCorrectionsDouble struct {
	attempts *deliveryAttemptStoreDouble
	stored   map[string]ports.DeliveryResultCorrectionRecord
	saves    int
	loadErr  error
	saveErr  error
}

func (double *deliveryCorrectionsDouble) LoadForCorrection(
	_ context.Context,
	key ports.DeliveryAttemptKey,
	object domain.CarriedObjectReference,
) (ports.DeliveryResultCorrectionTarget, bool, error) {
	if double.loadErr != nil {
		return ports.DeliveryResultCorrectionTarget{}, false, double.loadErr
	}
	record, found := double.attempts.records[deliveryAttemptID(key)]
	if !found {
		return ports.DeliveryResultCorrectionTarget{}, false, nil
	}
	for _, result := range record.Results {
		if result.Object() != object {
			continue
		}
		target := ports.DeliveryResultCorrectionTarget{Attempt: record.Attempt, Result: result}
		target.Correction, target.HasCorrection = double.stored[deliveryAttemptID(key)+"|"+object.String()]
		return target, true, nil
	}
	return ports.DeliveryResultCorrectionTarget{}, false, nil
}

func (double *deliveryCorrectionsDouble) Save(
	_ context.Context,
	record ports.DeliveryResultCorrectionRecord,
) (ports.DeliveryResultCorrectionSaveOutcome, error) {
	double.saves++
	if double.saveErr != nil {
		return ports.DeliveryResultCorrectionSaveOutcomeInvalid, double.saveErr
	}
	id := deliveryAttemptID(record.Key) + "|" + record.Correction.Pending().Object().String()
	if _, exists := double.stored[id]; exists {
		return ports.DeliveryResultCorrectionAlreadyRecorded, nil
	}
	double.stored[id] = record
	return ports.DeliveryResultCorrectionSaved, nil
}

type deliveryCorrectionFixture struct {
	attempts    *deliveryAttemptFixture
	corrections *deliveryCorrectionsDouble
	handler     *application.CorrectDeliveryAttemptResultHandler
}

func newDeliveryCorrectionFixture(t *testing.T) *deliveryCorrectionFixture {
	t.Helper()
	attempts := newDeliveryAttemptFixture(t)
	if result := attempts.handle(attempts.command(attempts.pending("parcel-1"), attempts.delivered("parcel-2"))); result.Outcome() != application.DeliveryAttemptRecorded {
		t.Fatalf("recording the delivery attempt = %q", result.Outcome())
	}
	corrections := &deliveryCorrectionsDouble{attempts: attempts.store, stored: map[string]ports.DeliveryResultCorrectionRecord{}}
	return &deliveryCorrectionFixture{
		attempts:    attempts,
		corrections: corrections,
		handler: application.NewCorrectDeliveryAttemptResultHandler(application.CorrectDeliveryAttemptResultDeps{
			Corrections: corrections, Clock: fixedClock{at: recordedAt.Add(time.Hour)},
		}),
	}
}

func (fixture *deliveryCorrectionFixture) command(t *testing.T) application.CorrectDeliveryAttemptResultCommand {
	t.Helper()
	return application.CorrectDeliveryAttemptResultCommand{
		TenantID:    value(t, domain.NewTenantID, "tenant-1"),
		Attempt:     "attempt-1",
		Object:      "parcel-1",
		Outcome:     domain.ObjectDelivered,
		Source:      value(t, domain.NewAttemptResultSourceReference, "source/address-review-confirmed"),
		OccurredAt:  arrivedAt.Add(9 * time.Minute),
		CorrectedAt: arrivedAt.Add(3 * time.Hour),
	}
}

func (fixture *deliveryCorrectionFixture) correct(t *testing.T, command application.CorrectDeliveryAttemptResultCommand) application.CorrectDeliveryAttemptResultResult {
	t.Helper()
	result, err := fixture.handler.Correct(context.Background(), command)
	if err != nil {
		t.Fatalf("correct: %v", err)
	}
	return result
}

// Covers: `AT-TF-070` 之后的确定（UC-TF-006「尝试待确认」）——待确认的派送结果凭同一次上门的后补来源更正成妥投：更正落册并回指
// 待确认那一版；同内容重放答已有，换了说法答冲突；已是确定结果的那一格答`并非待确认`；形状不成与指错答`来源未受理`；
// 读不动答`未决`；操作证据只进审计。
func TestAPendingDeliveryResultIsCorrectedByTheSameRulesAsThePickupSide(t *testing.T) {
	fixture := newDeliveryCorrectionFixture(t)

	first := fixture.correct(t, fixture.command(t))
	if first.Outcome() != application.AttemptResultCorrected {
		t.Fatalf("outcome = %q, want RESULT_CORRECTED", first.Outcome())
	}
	record, _ := first.Record()
	if record.Correction.Effective().Outcome() != domain.ObjectDelivered ||
		!record.Correction.Pending().Outcome().PendingConfirmation() {
		t.Fatalf("effective = %q pending = %q", record.Correction.Effective().Outcome(), record.Correction.Pending().Outcome())
	}

	command := fixture.command(t)
	command.DecidedBy = "issuer#reviewer-2"
	if replay := fixture.correct(t, command); replay.Outcome() != application.AttemptResultCorrectionExisting {
		t.Fatalf("replay = %q, want EXISTING_CORRECTION", replay.Outcome())
	}
	changed := fixture.command(t)
	changed.Outcome = domain.WrongAddress
	changed.Basis = value(t, domain.NewAttemptResultBasisReference, "reason/other")
	if conflict := fixture.correct(t, changed); conflict.Outcome() != application.AttemptResultCorrectionConflict {
		t.Fatalf("different statement = %q, want SOURCE_CONFLICT", conflict.Outcome())
	}
	if fixture.corrections.saves != 1 {
		t.Fatalf("saves = %d, want 1", fixture.corrections.saves)
	}

	t.Run("a determinate result is not corrected here", func(t *testing.T) {
		fixture := newDeliveryCorrectionFixture(t)
		command := fixture.command(t)
		command.Object = "parcel-2"
		if result := fixture.correct(t, command); result.Outcome() != application.AttemptResultNotPending {
			t.Fatalf("outcome = %q, want RESULT_NOT_PENDING", result.Outcome())
		}
	})
	t.Run("a missing attempt or object is not accepted", func(t *testing.T) {
		fixture := newDeliveryCorrectionFixture(t)
		unknown := fixture.command(t)
		unknown.Attempt = "attempt-unknown"
		outside := fixture.command(t)
		outside.Object = "parcel-9"
		for _, command := range []application.CorrectDeliveryAttemptResultCommand{unknown, outside} {
			if result := fixture.correct(t, command); result.Outcome() != application.AttemptResultCorrectionNotAccepted {
				t.Fatalf("outcome = %q", result.Outcome())
			}
		}
	})
	t.Run("malformed", func(t *testing.T) {
		fixture := newDeliveryCorrectionFixture(t)
		for name, change := range map[string]func(*application.CorrectDeliveryAttemptResultCommand){
			"into pending again": func(c *application.CorrectDeliveryAttemptResultCommand) {
				c.Outcome = domain.DeliveryPendingConfirmation
			},
			"no correcting source": func(c *application.CorrectDeliveryAttemptResultCommand) {
				c.Source = domain.AttemptResultSourceReference{}
			},
			"a failure without a basis": func(c *application.CorrectDeliveryAttemptResultCommand) {
				c.Outcome = domain.NoOneToReceive
			},
			"a delivery with a basis": func(c *application.CorrectDeliveryAttemptResultCommand) {
				c.Basis = value(t, domain.NewAttemptResultBasisReference, "reason/x")
			},
			"a blank decider":    func(c *application.CorrectDeliveryAttemptResultCommand) { c.DecidedBy = " " },
			"before the arrival": func(c *application.CorrectDeliveryAttemptResultCommand) { c.OccurredAt = arrivedAt.Add(-time.Minute) },
			"no tenant":          func(c *application.CorrectDeliveryAttemptResultCommand) { c.TenantID = domain.TenantID{} },
		} {
			command := fixture.command(t)
			change(&command)
			if result := fixture.correct(t, command); result.Outcome() != application.AttemptResultCorrectionNotAccepted {
				t.Fatalf("%s: outcome = %q, want SOURCE_NOT_ACCEPTED", name, result.Outcome())
			}
		}
		if fixture.corrections.saves != 0 {
			t.Fatal("拒绝的更正落了册")
		}
	})
	t.Run("unreadable and unwritable stores are undecided", func(t *testing.T) {
		fixture := newDeliveryCorrectionFixture(t)
		fixture.corrections.loadErr = errors.New("down")
		if result := fixture.correct(t, fixture.command(t)); result.Outcome() != application.AttemptResultCorrectionUndecided ||
			result.UndecidedReason() != application.AttemptResultCorrectionStoreUnavailable {
			t.Fatalf("load: outcome = %q reason = %q", result.Outcome(), result.UndecidedReason())
		}
		fixture.corrections.loadErr = nil
		fixture.corrections.saveErr = errors.New("down")
		if result := fixture.correct(t, fixture.command(t)); result.Outcome() != application.AttemptResultCorrectionUndecided {
			t.Fatalf("save: outcome = %q", result.Outcome())
		}
	})
}
