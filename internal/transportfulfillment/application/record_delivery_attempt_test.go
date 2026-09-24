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

type deliveryAttemptStoreDouble struct {
	records map[string]ports.DeliveryAttemptRecord
	findErr error
	saveErr error
	// winner 非 nil 时，Save 先把它放进册里再答已有记录：本方查时还没有、存时已被别人抢先——并发落败那一格。
	winner *ports.DeliveryAttemptRecord
	saves  int
}

func newDeliveryAttemptStore() *deliveryAttemptStoreDouble {
	return &deliveryAttemptStoreDouble{records: map[string]ports.DeliveryAttemptRecord{}}
}

func deliveryAttemptID(key ports.DeliveryAttemptKey) string {
	return key.TenantID.String() + "/" + key.Attempt.String()
}

func (double *deliveryAttemptStoreDouble) FindByKey(
	_ context.Context,
	key ports.DeliveryAttemptKey,
) (ports.DeliveryAttemptRecord, bool, error) {
	if double.findErr != nil {
		return ports.DeliveryAttemptRecord{}, false, double.findErr
	}
	record, found := double.records[deliveryAttemptID(key)]
	return record, found, nil
}

func (double *deliveryAttemptStoreDouble) Save(
	_ context.Context,
	record ports.DeliveryAttemptRecord,
) (ports.DeliveryAttemptSaveOutcome, error) {
	double.saves++
	if double.saveErr != nil {
		return ports.DeliveryAttemptSaveOutcomeInvalid, double.saveErr
	}
	id := deliveryAttemptID(record.Key)
	if double.winner != nil {
		double.records[id] = *double.winner
		double.winner = nil
		return ports.DeliveryAttemptAlreadyRecorded, nil
	}
	if _, exists := double.records[id]; exists {
		return ports.DeliveryAttemptAlreadyRecorded, nil
	}
	double.records[id] = record
	return ports.DeliveryAttemptSaved, nil
}

type deliveryTaskReaderDouble struct {
	records map[string]ports.DispatchTaskRecord
	err     error
}

func (double *deliveryTaskReaderDouble) FindByKey(
	_ context.Context,
	key ports.DispatchTaskKey,
) (ports.DispatchTaskRecord, bool, error) {
	if double.err != nil {
		return ports.DispatchTaskRecord{}, false, double.err
	}
	record, found := double.records[key.Task.String()]
	return record, found, nil
}

type deliveryAttemptFixture struct {
	t       *testing.T
	tenant  domain.TenantID
	store   *deliveryAttemptStoreDouble
	tasks   *deliveryTaskReaderDouble
	handler *application.RecordDeliveryAttemptHandler
}

func newDeliveryAttemptFixture(t *testing.T) *deliveryAttemptFixture {
	t.Helper()
	fixture := &deliveryAttemptFixture{
		t:      t,
		tenant: value(t, domain.NewTenantID, "tenant-1"),
		store:  newDeliveryAttemptStore(),
		tasks:  &deliveryTaskReaderDouble{records: map[string]ports.DispatchTaskRecord{}},
	}
	fixture.openTask("task-d1", domain.DeliveryDispatch, "parcel-1", "parcel-2", "parcel-3")
	fixture.handler = application.NewRecordDeliveryAttemptHandler(application.RecordDeliveryAttemptDeps{
		Attempts: fixture.store,
		Tasks:    fixture.tasks,
		Clock:    fixedClock{at: recordedAt},
	})
	return fixture
}

func (fixture *deliveryAttemptFixture) openTask(task string, kind domain.DispatchTaskKind, objects ...string) domain.DispatchTask {
	fixture.t.Helper()
	spec := domain.DispatchTaskSpec{
		TenantID:   fixture.tenant,
		Task:       value(fixture.t, domain.NewDispatchTaskReference, task),
		Kind:       kind,
		Place:      value(fixture.t, domain.NewAttemptPlaceReference, "place-1"),
		WindowFrom: plannedFrom,
		WindowTo:   plannedTo,
		Conditions: value(fixture.t, domain.NewServiceConditionReference, "conditions-1"),
		OpenedAt:   plannedFrom.Add(-time.Hour),
	}
	for _, object := range objects {
		spec.Objects = append(spec.Objects, value(fixture.t, domain.NewCarriedObjectReference, object))
	}
	opened, err := domain.OpenDispatchTask(spec)
	if err != nil {
		fixture.t.Fatalf("open dispatch task: %v", err)
	}
	fixture.tasks.records[task] = ports.DispatchTaskRecord{
		Key:        ports.DispatchTaskKey{TenantID: fixture.tenant, Task: opened.Task()},
		Task:       opened,
		RecordedAt: spec.OpenedAt,
	}
	return opened
}

func (fixture *deliveryAttemptFixture) delivered(object string) application.ObjectDeliverySubmission {
	fixture.t.Helper()
	return application.ObjectDeliverySubmission{
		Object:     value(fixture.t, domain.NewCarriedObjectReference, object),
		Outcome:    domain.ObjectDelivered,
		OccurredAt: arrivedAt.Add(5 * time.Minute),
	}
}

func (fixture *deliveryAttemptFixture) failed(
	object string,
	outcome domain.DeliveryObjectOutcome,
	basis string,
) application.ObjectDeliverySubmission {
	fixture.t.Helper()
	return application.ObjectDeliverySubmission{
		Object:     value(fixture.t, domain.NewCarriedObjectReference, object),
		Outcome:    outcome,
		Basis:      value(fixture.t, domain.NewAttemptResultBasisReference, basis),
		OccurredAt: arrivedAt.Add(7 * time.Minute),
	}
}

func (fixture *deliveryAttemptFixture) command(objects ...application.ObjectDeliverySubmission) application.RecordDeliveryAttemptCommand {
	return application.RecordDeliveryAttemptCommand{
		TenantID:    fixture.tenant,
		Task:        "task-d1",
		Attempt:     "attempt-1",
		ExecutedBy:  "courier-1",
		Place:       "place-1",
		PlannedFrom: plannedFrom,
		PlannedTo:   plannedTo,
		ArrivedAt:   arrivedAt,
		Evidence:    "evidence-1",
		Objects:     objects,
	}
}

func (fixture *deliveryAttemptFixture) handle(command application.RecordDeliveryAttemptCommand) application.RecordDeliveryAttemptResult {
	fixture.t.Helper()
	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		fixture.t.Fatalf("handle: %v", err)
	}
	return result
}

func TestAFirstDeliveryAttemptIsRecordedWithEachObjectResult(t *testing.T) {
	fixture := newDeliveryAttemptFixture(t)

	result := fixture.handle(fixture.command(
		fixture.delivered("parcel-1"),
		fixture.failed("parcel-2", domain.NoOneToReceive, "basis/no-one-home"),
	))

	if got := result.Outcome().String(); got != "ATTEMPT_RECORDED" {
		t.Fatalf("outcome = %q, want ATTEMPT_RECORDED", got)
	}
	record, present := result.Record()
	if !present {
		t.Fatal("recorded attempt must carry its record")
	}
	if record.Attempt.Attempt().String() != "attempt-1" || record.Attempt.Task().String() != "task-d1" {
		t.Fatalf("attempt = %s on %s", record.Attempt.Attempt(), record.Attempt.Task())
	}
	if !record.RecordedAt.Equal(recordedAt) {
		t.Fatalf("recorded at = %s, want the clock's %s", record.RecordedAt, recordedAt)
	}
	outcomes := map[string]string{}
	for _, objectResult := range record.Results {
		outcomes[objectResult.Object().String()] = objectResult.Outcome().String()
	}
	if len(outcomes) != 2 || outcomes["parcel-1"] != "DELIVERED" || outcomes["parcel-2"] != "NO_ONE_TO_RECEIVE" {
		t.Fatalf("object results = %v, want each object's own outcome", outcomes)
	}
	if fixture.store.saves != 1 {
		t.Fatalf("saves = %d, want exactly one", fixture.store.saves)
	}
}

func TestReplayingTheSameAttemptWithTheSameContentReturnsTheExistingResult(t *testing.T) {
	fixture := newDeliveryAttemptFixture(t)
	command := fixture.command(
		fixture.delivered("parcel-1"),
		fixture.failed("parcel-2", domain.DeliveryRefused, "basis/refused-at-door"),
	)
	fixture.handle(command)

	reordered := command
	reordered.Objects = []application.ObjectDeliverySubmission{command.Objects[1], command.Objects[0]}
	result := fixture.handle(reordered)

	if got := result.Outcome().String(); got != "EXISTING_RESULT" {
		t.Fatalf("outcome = %q, want EXISTING_RESULT (object order is not content)", got)
	}
	if _, present := result.Record(); !present {
		t.Fatal("a replay answers with the record it already holds")
	}
	if len(fixture.store.records) != 1 {
		t.Fatalf("records = %d, want the one original", len(fixture.store.records))
	}
}

func TestTheSameAttemptWithDifferentContentIsAConflictAndKeepsTheOriginal(t *testing.T) {
	fixture := newDeliveryAttemptFixture(t)
	original := fixture.command(fixture.failed("parcel-1", domain.NoOneToReceive, "basis/no-one-home"))
	fixture.handle(original)

	changed := fixture.command(fixture.delivered("parcel-1"))
	result := fixture.handle(changed)

	if got := result.Outcome().String(); got != "SOURCE_CONFLICT" {
		t.Fatalf("outcome = %q, want SOURCE_CONFLICT", got)
	}
	kept := fixture.store.records["tenant-1/attempt-1"]
	if len(kept.Results) != 1 || kept.Results[0].Outcome() != domain.NoOneToReceive {
		t.Fatalf("stored results = %v, want the first report untouched", kept.Results)
	}
}

func TestAnAttemptOnATaskThatIsNotAnOpenDeliveryTaskIsNotRecorded(t *testing.T) {
	cases := map[string]func(fixture *deliveryAttemptFixture){
		"task never opened": func(fixture *deliveryAttemptFixture) {
			delete(fixture.tasks.records, "task-d1")
		},
		"task is a pickup task": func(fixture *deliveryAttemptFixture) {
			fixture.openTask("task-d1", domain.PickupDispatch, "parcel-1", "parcel-2")
		},
		"task already terminated": func(fixture *deliveryAttemptFixture) {
			opened := fixture.openTask("task-d1", domain.DeliveryDispatch, "parcel-1", "parcel-2")
			closed, err := opened.Terminate(
				value(t, domain.NewTaskClosureBasisReference, "basis/route-cancelled"), plannedFrom)
			if err != nil {
				t.Fatalf("terminate: %v", err)
			}
			record := fixture.tasks.records["task-d1"]
			record.Task = closed
			fixture.tasks.records["task-d1"] = record
		},
	}
	for name, arrange := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newDeliveryAttemptFixture(t)
			arrange(fixture)

			result := fixture.handle(fixture.command(fixture.delivered("parcel-1")))

			if got := result.Outcome().String(); got != "DELIVERY_TASK_NOT_OPEN" {
				t.Fatalf("outcome = %q, want DELIVERY_TASK_NOT_OPEN", got)
			}
			if fixture.store.saves != 0 {
				t.Fatalf("saves = %d, an attempt without an open delivery task must not be stored", fixture.store.saves)
			}
		})
	}
}

func TestAnAttemptReportingAnObjectOutsideTheTaskIsNotRecorded(t *testing.T) {
	fixture := newDeliveryAttemptFixture(t)

	result := fixture.handle(fixture.command(
		fixture.delivered("parcel-1"),
		fixture.delivered("parcel-9"),
	))

	if got := result.Outcome().String(); got != "OBJECT_OUTSIDE_TASK" {
		t.Fatalf("outcome = %q, want OBJECT_OUTSIDE_TASK", got)
	}
	if fixture.store.saves != 0 {
		t.Fatalf("saves = %d, want none", fixture.store.saves)
	}
}

func TestUnavailableDependenciesLeaveTheAttemptUndecidedWithAContinuation(t *testing.T) {
	unavailable := errors.New("connection refused")
	cases := map[string]struct {
		arrange func(fixture *deliveryAttemptFixture)
		reason  string
	}{
		"attempt store cannot be read": {
			arrange: func(fixture *deliveryAttemptFixture) { fixture.store.findErr = unavailable },
			reason:  "DELIVERY_ATTEMPT_STORE_UNAVAILABLE",
		},
		"task registry cannot be read": {
			arrange: func(fixture *deliveryAttemptFixture) { fixture.tasks.err = unavailable },
			reason:  "DISPATCH_TASK_REGISTRY_UNAVAILABLE",
		},
		"attempt store cannot be written": {
			arrange: func(fixture *deliveryAttemptFixture) { fixture.store.saveErr = unavailable },
			reason:  "DELIVERY_ATTEMPT_STORE_UNAVAILABLE",
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newDeliveryAttemptFixture(t)
			testCase.arrange(fixture)

			result := fixture.handle(fixture.command(fixture.delivered("parcel-1")))

			if got := result.Outcome().String(); got != "DELIVERY_ATTEMPT_UNDECIDED" {
				t.Fatalf("outcome = %q, want DELIVERY_ATTEMPT_UNDECIDED", got)
			}
			if got := result.UndecidedReason().String(); got != testCase.reason {
				t.Fatalf("reason = %q, want %s", got, testCase.reason)
			}
			if result.ContinuationReference() == "" {
				t.Fatal("an undecided attempt must leave a continuation to retry the same submission")
			}
		})
	}
}

func TestLosingARaceReadsTheWinnerBack(t *testing.T) {
	t.Run("winner carries the same content", func(t *testing.T) {
		fixture := newDeliveryAttemptFixture(t)
		command := fixture.command(fixture.delivered("parcel-1"))
		winner := fixture.recordOf(command)
		fixture.store.winner = &winner

		result := fixture.handle(command)

		if got := result.Outcome().String(); got != "EXISTING_RESULT" {
			t.Fatalf("outcome = %q, want EXISTING_RESULT", got)
		}
	})
	t.Run("winner carries different content", func(t *testing.T) {
		fixture := newDeliveryAttemptFixture(t)
		winner := fixture.recordOf(fixture.command(fixture.failed("parcel-1", domain.WrongAddress, "basis/wrong-address")))
		fixture.store.winner = &winner

		result := fixture.handle(fixture.command(fixture.delivered("parcel-1")))

		if got := result.Outcome().String(); got != "SOURCE_CONFLICT" {
			t.Fatalf("outcome = %q, want SOURCE_CONFLICT", got)
		}
	})
}

// recordOf 用另一只处理器把命令登进一份空册，取回它会存下的记录——并发的赢家就是这样一份记录。
func (fixture *deliveryAttemptFixture) recordOf(command application.RecordDeliveryAttemptCommand) ports.DeliveryAttemptRecord {
	fixture.t.Helper()
	store := newDeliveryAttemptStore()
	handler := application.NewRecordDeliveryAttemptHandler(application.RecordDeliveryAttemptDeps{
		Attempts: store,
		Tasks:    fixture.tasks,
		Clock:    fixedClock{at: recordedAt.Add(-time.Second)},
	})
	result, err := handler.Handle(context.Background(), command)
	if err != nil {
		fixture.t.Fatalf("record winner: %v", err)
	}
	record, present := result.Record()
	if !present {
		fixture.t.Fatalf("winner not recorded: %s", result.Outcome())
	}
	return record
}

func TestASelfContradictorySubmissionIsNotAcceptedRatherThanFailing(t *testing.T) {
	cases := map[string]func(fixture *deliveryAttemptFixture) application.RecordDeliveryAttemptCommand{
		"no tenant": func(fixture *deliveryAttemptFixture) application.RecordDeliveryAttemptCommand {
			command := fixture.command(fixture.delivered("parcel-1"))
			command.TenantID = domain.TenantID{}
			return command
		},
		"no objects": func(fixture *deliveryAttemptFixture) application.RecordDeliveryAttemptCommand {
			return fixture.command()
		},
		"no attempt identity": func(fixture *deliveryAttemptFixture) application.RecordDeliveryAttemptCommand {
			command := fixture.command(fixture.delivered("parcel-1"))
			command.Attempt = ""
			return command
		},
		"delivered carrying a failure basis": func(fixture *deliveryAttemptFixture) application.RecordDeliveryAttemptCommand {
			submission := fixture.delivered("parcel-1")
			submission.Basis = value(fixture.t, domain.NewAttemptResultBasisReference, "basis/whatever")
			return fixture.command(submission)
		},
		"refusal without a basis": func(fixture *deliveryAttemptFixture) application.RecordDeliveryAttemptCommand {
			submission := fixture.delivered("parcel-1")
			submission.Outcome = domain.DeliveryRefused
			return fixture.command(submission)
		},
		"result before arrival": func(fixture *deliveryAttemptFixture) application.RecordDeliveryAttemptCommand {
			submission := fixture.delivered("parcel-1")
			submission.OccurredAt = arrivedAt.Add(-time.Minute)
			return fixture.command(submission)
		},
		"the same object twice": func(fixture *deliveryAttemptFixture) application.RecordDeliveryAttemptCommand {
			return fixture.command(fixture.delivered("parcel-1"), fixture.delivered("parcel-1"))
		},
	}
	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newDeliveryAttemptFixture(t)

			result := fixture.handle(build(fixture))

			if got := result.Outcome().String(); got != "SOURCE_NOT_ACCEPTED" {
				t.Fatalf("outcome = %q, want SOURCE_NOT_ACCEPTED", got)
			}
			if fixture.store.saves != 0 {
				t.Fatalf("saves = %d, want none", fixture.store.saves)
			}
		})
	}
}
