package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 16 证票 tf-face-algebra/01：编排接真登记册、每次调用一笔事务（同生产装配的事务包装）。
// 零时刻之后不足一微秒的业务时间、含 NUL 的引用都答`未受理`，事务照常提交，册上零行；零点后恰一微秒首登后读得回，
// 原样重放答`已在册`。夹具全为合成值（S 级）。

var registryZeroWindow = map[string]time.Time{
	"零点后 1ns":            time.Time{}.Add(time.Nanosecond),
	"零点后 999ns":          time.Time{}.Add(999 * time.Nanosecond),
	"+14:00 写法零点后 500ns": time.Date(1, 1, 1, 14, 0, 0, 500, time.FixedZone("SYN+14", 14*3600)),
}

type storableFixture struct {
	t           *testing.T
	pool        *pgxpool.Pool
	transactor  bentoapp.Transactor
	tenant      domain.TenantID
	assignments *adapter.LoadAssignments
	facts       *adapter.MovementFacts
	form        *application.FormLoadAssignmentHandler
	record      *application.RecordMovementFactHandler
	delivery    *deliveryPrecisionFixture
}

func newStorableFixture(t *testing.T) *storableFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	transactor := db.Transactor()
	assignments, err := adapter.NewLoadAssignments(db)
	if err != nil {
		t.Fatalf("装载分配册：%v", err)
	}
	facts, err := adapter.NewMovementFacts(db)
	if err != nil {
		t.Fatalf("移动事实册：%v", err)
	}
	tasks, err := adapter.NewDispatchTasks(db)
	if err != nil {
		t.Fatalf("揽派任务登记册：%v", err)
	}
	attempts, err := adapter.NewDeliveryAttempts(db)
	if err != nil {
		t.Fatalf("派送尝试册：%v", err)
	}
	schedules, err := adapter.NewTransportSchedules(db)
	if err != nil {
		t.Fatalf("班次登记：%v", err)
	}
	// 装载分配核班次在册：先登下夹具引用的那个班次，本文件要证的那几格才走得到登记册。
	// 夹具没登进去时，后面的用例会红在 SCHEDULE_NOT_FOUND 上、看不出病根在夹具，所以在这里先拦。
	if saved := inTransaction(t, transactor, func(ctx context.Context) (ports.ScheduleSaveOutcome, error) {
		return schedules.Save(ctx, formedSchedule(t, "SYN-TENANT-01", "SYN-SCHEDULE-1"))
	}); saved != ports.ScheduleSaved {
		t.Fatalf("夹具班次没有登进去：%v", saved)
	}
	clock := credentialClock{at: credentialNow}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("Outbox Store：%v", err)
	}
	handoff, err := adapter.NewOutboxMovementFactHandoff(db, store, clock)
	if err != nil {
		t.Fatalf("移动事实信封：%v", err)
	}
	tenant := segmentRef(t, domain.NewTenantID, "SYN-TENANT-01")
	return &storableFixture{
		t: t, pool: pool, transactor: transactor, tenant: tenant, assignments: assignments, facts: facts,
		form: application.NewFormLoadAssignmentHandler(application.FormLoadAssignmentDeps{
			Assignments: assignments, Schedules: schedules, Clock: clock,
		}),
		record: application.NewRecordMovementFactHandler(application.RecordMovementFactDeps{Facts: facts, Handoff: handoff, Clock: clock}),
		delivery: &deliveryPrecisionFixture{
			t: t, transactor: transactor, tenant: tenant,
			tasks:    application.NewOpenDispatchTaskHandler(application.OpenDispatchTaskDeps{Tasks: tasks, Clock: clock}),
			attempts: application.NewRecordDeliveryAttemptHandler(application.RecordDeliveryAttemptDeps{Attempts: attempts, Tasks: tasks, Clock: clock}),
		},
	}
}

func (fixture *storableFixture) rows(table string) int {
	fixture.t.Helper()
	var count int
	if err := fixture.pool.QueryRow(fixture.t.Context(), `SELECT count(*) FROM transport_fulfillment.`+table).Scan(&count); err != nil {
		fixture.t.Fatalf("数 %s：%v", table, err)
	}
	return count
}

func (fixture *storableFixture) assignmentCommand(assignment, version string, members []string, at time.Time) application.FormLoadAssignmentCommand {
	return application.FormLoadAssignmentCommand{
		TenantID: fixture.tenant, Assignment: assignment, Schedule: "SYN-SCHEDULE-1",
		Members: members, Version: version, AssignedAt: at,
	}
}

func (fixture *storableFixture) formAssignment(command application.FormLoadAssignmentCommand) application.LoadAssignmentOutcome {
	fixture.t.Helper()
	return inTransaction(fixture.t, fixture.transactor, func(ctx context.Context) (application.FormLoadAssignmentResult, error) {
		return fixture.form.Form(ctx, command)
	}).Outcome()
}

func (fixture *storableFixture) factCommand(fact, location string, at time.Time) application.RecordMovementFactCommand {
	return application.RecordMovementFactCommand{
		TenantID: fixture.tenant, Fact: fact, Schedule: "SYN-SCHEDULE-1", Kind: domain.ArrivalFact,
		Location: location, Source: "SYN-SOURCE-1", Version: "SYN-MF-V1", OccurredAt: at,
	}
}

func (fixture *storableFixture) recordFact(command application.RecordMovementFactCommand) application.MovementFactOutcome {
	fixture.t.Helper()
	return inTransaction(fixture.t, fixture.transactor, func(ctx context.Context) (application.RecordMovementFactResult, error) {
		return fixture.record.Record(ctx, command, domain.DepartureGateReading{})
	}).Outcome()
}

// Covers: 票 tf-face-algebra/01 完成判据「真库回归」——装载分配时刻在登记册上是零点答`未受理`，版本行与成员行零行；
// 零点后一微秒首登后 FindByKey 读得回，原样重放答`已在册`。
func TestALoadAssignmentAtTheRegistryZeroIsNotAcceptedAgainstARealRegistry(t *testing.T) {
	fixture := newStorableFixture(t)
	for name, at := range registryZeroWindow {
		command := fixture.assignmentCommand("SYN-LA-ZERO", "SYN-LA-V1", []string{"SYN-PARCEL-1"}, at)
		if got := fixture.formAssignment(command); got != application.LoadAssignmentNotAccepted {
			t.Fatalf("%s = %s，期望 INPUT_NOT_ACCEPTED", name, got)
		}
	}
	if versions, members := fixture.rows("load_assignment"), fixture.rows("load_assignment_member"); versions != 0 || members != 0 {
		t.Fatalf("零点格落了库：版本行 %d、成员行 %d", versions, members)
	}

	atMicro := time.Time{}.Add(time.Microsecond)
	command := fixture.assignmentCommand("SYN-LA-MICRO", "SYN-LA-V1", []string{"SYN-PARCEL-1"}, atMicro)
	if got := fixture.formAssignment(command); got != application.LoadAssignmentFormed {
		t.Fatalf("零点后 1µs 首登 = %s，期望 LOAD_ASSIGNMENT_FORMED", got)
	}
	key := ports.LoadAssignmentKey{
		TenantID:   fixture.tenant,
		Assignment: segmentRef(t, domain.NewLoadAssignmentReference, "SYN-LA-MICRO"),
		Version:    segmentRef(t, domain.NewLoadAssignmentVersion, "SYN-LA-V1"),
	}
	stored := inTransaction(t, fixture.transactor, func(ctx context.Context) (ports.LoadAssignmentRecord, error) {
		record, found, err := fixture.assignments.FindByKey(ctx, key)
		if err == nil && !found {
			t.Fatal("零点后 1µs 首登后读不回")
		}
		return record, err
	})
	pinRoundTrip(t, "分配时刻", stored.Assignment.AssignedAt(), atMicro)
	if got := fixture.formAssignment(command); got != application.LoadAssignmentVersionExists {
		t.Fatalf("零点后 1µs 原样重放 = %s，期望 LOAD_ASSIGNMENT_VERSION_EXISTS", got)
	}
}

// Covers: 票 tf-face-algebra/01 完成判据「真库回归」——移动事实发生时刻在登记册上是零点答`未受理`，事实行零行；
// 零点后一微秒首登后 FindByKey 读得回，原样重放答`已有版本`。
func TestAMovementFactAtTheRegistryZeroIsNotAcceptedAgainstARealRegistry(t *testing.T) {
	fixture := newStorableFixture(t)
	for name, at := range registryZeroWindow {
		if got := fixture.recordFact(fixture.factCommand("SYN-MF-ZERO", "SYN-LOC-1", at)); got != application.MovementFactNotAccepted {
			t.Fatalf("%s = %s，期望 INPUT_NOT_ACCEPTED", name, got)
		}
	}
	if facts := fixture.rows("transport_movement_fact"); facts != 0 {
		t.Fatalf("零点格落了库：事实行 %d", facts)
	}

	atMicro := time.Time{}.Add(time.Microsecond)
	command := fixture.factCommand("SYN-MF-MICRO", "SYN-LOC-1", atMicro)
	if got := fixture.recordFact(command); got != application.MovementFactRecorded {
		t.Fatalf("零点后 1µs 首登 = %s，期望 MOVEMENT_FACT_RECORDED", got)
	}
	key := ports.MovementFactKey{
		TenantID: fixture.tenant,
		Fact:     segmentRef(t, domain.NewMovementFactReference, "SYN-MF-MICRO"),
		Version:  segmentRef(t, domain.NewMovementFactVersion, "SYN-MF-V1"),
	}
	stored := inTransaction(t, fixture.transactor, func(ctx context.Context) (ports.MovementFactRecord, error) {
		record, found, err := fixture.facts.FindByKey(ctx, key)
		if err == nil && !found {
			t.Fatal("零点后 1µs 首登后读不回")
		}
		return record, err
	})
	pinRoundTrip(t, "发生时刻", stored.Fact.OccurredAt(), atMicro)
	if got := fixture.recordFact(command); got != application.MovementFactVersionExists {
		t.Fatalf("零点后 1µs 原样重放 = %s，期望 MOVEMENT_FACT_VERSION_EXISTS", got)
	}
}

// Covers: 票 tf-face-algebra/01 完成判据「真库回归」——装载分配版本、新身份的成员、移动事实地点、派送尝试证据含 NUL 都答
// `未受理`，事务照常提交（inTransaction 要求提交成功），册上零行。
func TestValuesWithNULAreNotAcceptedAgainstARealRegistry(t *testing.T) {
	fixture := newStorableFixture(t)
	at := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	for name, command := range map[string]application.FormLoadAssignmentCommand{
		"版本": fixture.assignmentCommand("SYN-LA-NUL-1", "SYN-LA-V1\x00", []string{"SYN-PARCEL-1"}, at),
		"成员": fixture.assignmentCommand("SYN-LA-NUL-2", "SYN-LA-V1", []string{"SYN-PARCEL-\x001"}, at),
	} {
		if got := fixture.formAssignment(command); got != application.LoadAssignmentNotAccepted {
			t.Fatalf("装载分配%s含 NUL = %s，期望 INPUT_NOT_ACCEPTED", name, got)
		}
	}
	if got := fixture.recordFact(fixture.factCommand("SYN-MF-NUL", "SYN-LOC-\x001", at)); got != application.MovementFactNotAccepted {
		t.Fatalf("移动事实地点含 NUL = %s，期望 INPUT_NOT_ACCEPTED", got)
	}

	delivery := fixture.delivery
	if got := delivery.open("SYN-TASK-D1", nsFrom.Add(-time.Hour), nsFrom.Add(12*time.Hour)).Outcome(); got != application.DispatchTaskOpened {
		t.Fatalf("开立派送任务 = %s", got)
	}
	attempt := delivery.nsDeliveryCommand("SYN-ATTEMPT-NUL")
	attempt.Evidence = "SYN-EVIDENCE-\x001"
	if got := delivery.record(attempt).Outcome(); got != application.DeliveryAttemptNotAccepted {
		t.Fatalf("派送尝试证据含 NUL = %s，期望 INPUT_NOT_ACCEPTED", got)
	}

	for _, table := range []string{"load_assignment", "load_assignment_member", "transport_movement_fact", "delivery_attempt", "delivery_attempt_result"} {
		if rows := fixture.rows(table); rows != 0 {
			t.Fatalf("%s 有 %d 行，期望零行", table, rows)
		}
	}
}
