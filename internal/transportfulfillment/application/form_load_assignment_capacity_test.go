package application_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证装载分配的容量保障（票 demo-chain-gaps/21，ADR-0203 决定一、二）在编排上的分格与次序。夹具班次 schedule-1 挂两池：
// 重量池 100 kg、件数池 10 pcs，各有一份预占 reservation-w（60 kg）与 reservation-p（5 pcs），判断时点与有效期都框住夹具分配
// 时刻。计数都锚定这几个数；夹具全为合成值（S 级）。

const (
	weightPool = "schedule-1/WEIGHT"
	piecesPool = "schedule-1/PIECES"
)

var (
	allocationReservedAt = assignedAtFixture.Add(-2 * time.Hour)
	allocationValidUntil = assignedAtFixture.Add(6 * time.Hour)
)

// assignmentPoolsDouble 是按班次读池的替身：键是（租户，班次），与库面同键，他租户的同名池天然读不回。
type assignmentPoolsDouble struct {
	records []ports.CapacityPoolRecord
	err     error
	reads   int
}

func assignmentPools(records ...ports.CapacityPoolRecord) *assignmentPoolsDouble {
	return &assignmentPoolsDouble{records: records}
}

func (double *assignmentPoolsDouble) ListPoolsOnSchedules(
	_ context.Context,
	tenant domain.TenantID,
	schedules []domain.ScheduleReference,
) ([]ports.CapacityPoolRecord, error) {
	double.reads++
	if double.err != nil {
		return nil, double.err
	}
	var found []ports.CapacityPoolRecord
	for _, record := range double.records {
		for _, schedule := range schedules {
			if record.Key.TenantID == tenant && record.Pool.Schedule() == schedule {
				found = append(found, record)
			}
		}
	}
	return found, nil
}

// allocationPool 立一个挂在 schedule 上的池，带一份预占；others 是别的装载分配已经在这份预占上分走的量。
func allocationPool(
	t *testing.T,
	schedule string,
	dimension domain.CapacityDimension,
	unit string,
	capacity int64,
	reservation string,
	reserved, released int64,
	others ...domain.CapacityAllocationSnapshot,
) ports.CapacityPoolRecord {
	t.Helper()
	tenant := mustLoadTenant(t, "tenant-1")
	scheduleRef, err := domain.NewScheduleReference(schedule)
	if err != nil {
		t.Fatal(err)
	}
	unitRef, err := domain.NewQuantityUnitReference(unit)
	if err != nil {
		t.Fatal(err)
	}
	reservationRef, err := domain.NewCapacityReservationReference(reservation)
	if err != nil {
		t.Fatal(err)
	}
	var releases []domain.CapacityReleaseSnapshot
	if released > 0 {
		releaseRef, err := domain.NewCapacityReleaseReference("release-" + reservation)
		if err != nil {
			t.Fatal(err)
		}
		releases = append(releases, domain.CapacityReleaseSnapshot{
			Reference: releaseRef, Quantity: released, Source: allocationSource(t), ReleasedAt: allocationReservedAt.Add(time.Minute),
		})
	}
	pool, err := domain.RehydrateCapacityPool(domain.CapacityPoolSpec{
		TenantID: tenant, Schedule: scheduleRef, Dimension: dimension, Unit: unitRef, Capacity: capacity,
	}, []domain.CapacityReservationSnapshot{{
		Reference:   reservationRef,
		Quantity:    reserved,
		Released:    released,
		ValidUntil:  allocationValidUntil,
		Source:      allocationSource(t),
		ReservedAt:  allocationReservedAt,
		Releases:    releases,
		Allocations: others,
	}})
	if err != nil {
		t.Fatalf("池夹具：%v", err)
	}
	return ports.CapacityPoolRecord{Key: ports.CapacityPoolKey{TenantID: tenant, Pool: pool.Pool()}, Pool: pool}
}

func allocationSource(t *testing.T) domain.CapacityDemandSource {
	t.Helper()
	reference, err := domain.NewCapacityDemandSourceReference("SYN-DEMAND-DECL-1/v1")
	if err != nil {
		t.Fatal(err)
	}
	source, err := domain.NewCapacityDemandSource(domain.CapacityDemandOperatorDeclaration, reference)
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func mustLoadTenant(t *testing.T, value string) domain.TenantID {
	t.Helper()
	tenant, err := domain.NewTenantID(value)
	if err != nil {
		t.Fatal(err)
	}
	return tenant
}

func otherAllocation(t *testing.T, assignment string, quantity int64) domain.CapacityAllocationSnapshot {
	t.Helper()
	ref, err := domain.NewLoadAssignmentReference(assignment)
	if err != nil {
		t.Fatal(err)
	}
	version, err := domain.NewLoadAssignmentVersion(assignment + "/v1")
	if err != nil {
		t.Fatal(err)
	}
	return domain.CapacityAllocationSnapshot{Assignment: ref, Version: version, Quantity: quantity}
}

// twoPoolSchedule 是夹具班次上的两池。
func twoPoolSchedule(t *testing.T, weightOthers ...domain.CapacityAllocationSnapshot) *assignmentPoolsDouble {
	t.Helper()
	return assignmentPools(
		allocationPool(t, "schedule-1", domain.CapacityWeight, "kg", 100, "reservation-w", 60, 0, weightOthers...),
		allocationPool(t, "schedule-1", domain.CapacityPieces, "pcs", 10, "reservation-p", 5, 0),
	)
}

func weightLine(quantity int64) application.LoadAssignmentAllocationLine {
	return application.LoadAssignmentAllocationLine{
		Pool: weightPool, Reservation: "reservation-w", Quantity: quantity, Unit: "kg",
		SourceKind: "OPERATOR_DECLARATION", SourceReference: "SYN-DEMAND-DECL-1/v1",
	}
}

func piecesFromMembers() application.LoadAssignmentAllocationLine {
	return application.LoadAssignmentAllocationLine{
		Pool: piecesPool, Reservation: "reservation-p", FromMembers: true, Unit: "pcs",
		SourceKind: "OPERATOR_DECLARATION", SourceReference: "SYN-DEMAND-DECL-1/v1",
	}
}

func coveredCommand(t *testing.T, lines ...application.LoadAssignmentAllocationLine) application.FormLoadAssignmentCommand {
	t.Helper()
	command := formAssignmentCommand(t)
	command.Allocations = lines
	return command
}

// Covers: 票 demo-chain-gaps/21（AT-TF-171）——班次挂了池，两个池各有一行、各引用池上一份当前有效预占：形成分配，逐池的分配量
// 随版本落册；件数池取成员数（两个成员即 2 pcs），重量池取声明值。编排不换存池——分配不调 Consume，可用容量不随它变。
func TestALoadAssignmentOnAPooledScheduleFormsWithAReservationOnEveryPool(t *testing.T) {
	registry := newLoadAssignmentRegistry()
	pools := twoPoolSchedule(t)

	result, err := newLoadAssignmentHandlerWith(registry, registeredSchedules([2]string{"tenant-1", "schedule-1"}), pools).
		Form(t.Context(), coveredCommand(t, weightLine(40), piecesFromMembers()))
	if err != nil || result.Outcome() != application.LoadAssignmentFormed {
		t.Fatalf("outcome = %q err = %v, want LOAD_ASSIGNMENT_FORMED", result.Outcome(), err)
	}
	record, _ := result.Record()
	allocations := record.Assignment.Allocations()
	if len(allocations) != 2 {
		t.Fatalf("分配量 %d 行, want 2", len(allocations))
	}
	byPool := map[string]domain.CapacityAllocation{}
	for _, allocation := range allocations {
		byPool[allocation.Pool().String()] = allocation
	}
	if got := byPool[weightPool]; got.Demand().Quantity() != 40 || got.Basis() != domain.CapacityAllocationDeclared {
		t.Fatalf("重量池那一行 = %d %s", got.Demand().Quantity(), got.Basis())
	}
	if got := byPool[piecesPool]; got.Demand().Quantity() != 2 || got.Basis() != domain.CapacityAllocationFromMembers {
		t.Fatalf("件数池那一行 = %d %s, want 成员数 2", got.Demand().Quantity(), got.Basis())
	}
	if registry.saves != 1 {
		t.Fatalf("登记 %d 次, want 1", registry.saves)
	}
}

// Covers: 票 demo-chain-gaps/21（AT-TF-172）——形成分配的编排够不着消耗：它的缝里没有池册写口，也没有消耗交接。分配占的是预占
// 里已经扣过池的那一段，消耗是永久占用，撤回时还不回来（ADR-0203 决定二）。
func TestFormingALoadAssignmentCannotReachConsume(t *testing.T) {
	deps := reflect.TypeOf(application.FormLoadAssignmentDeps{})
	forbidden := []reflect.Type{
		reflect.TypeOf((*ports.CapacityPoolStore)(nil)).Elem(),
		reflect.TypeOf((*ports.CapacityConsumptionHandoff)(nil)).Elem(),
	}
	for index := 0; index < deps.NumField(); index++ {
		field := deps.Field(index).Type
		for _, port := range forbidden {
			if field == port || (field.Kind() == reflect.Interface && field.Implements(port)) {
				t.Fatalf("FormLoadAssignmentDeps.%s 是 %s，形成分配就够得着消耗", deps.Field(index).Name, port)
			}
		}
	}
}

// Covers: 票 demo-chain-gaps/21（AT-TF-173）——班次挂了池而有池没有分配行，答`须引用预占`并交回缺行的池，不登记：一行都没有时两个
// 池都缺，只有重量那一行时只缺件数池。
func TestAPooledScheduleRequiresAReservationOnEveryPool(t *testing.T) {
	cases := map[string]struct {
		lines []application.LoadAssignmentAllocationLine
		want  []string
	}{
		"没有任何分配行": {lines: nil, want: []string{piecesPool, weightPool}},
		"只有重量池一行": {lines: []application.LoadAssignmentAllocationLine{weightLine(10)}, want: []string{piecesPool}},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			registry := newLoadAssignmentRegistry()
			pools := assignmentPools(
				allocationPool(t, "schedule-1", domain.CapacityPieces, "pcs", 10, "reservation-p", 5, 0),
				allocationPool(t, "schedule-1", domain.CapacityWeight, "kg", 100, "reservation-w", 60, 0),
			)
			result, err := newLoadAssignmentHandlerWith(registry, registeredSchedules([2]string{"tenant-1", "schedule-1"}), pools).
				Form(t.Context(), coveredCommand(t, testCase.lines...))
			if err != nil || result.Outcome() != application.LoadAssignmentCapacityReservationRequired {
				t.Fatalf("outcome = %q err = %v, want CAPACITY_RESERVATION_REQUIRED", result.Outcome(), err)
			}
			var got []string
			for _, pool := range result.PoolsWithoutReservation() {
				got = append(got, pool.Pool.Pool().String())
			}
			if !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("缺行的池 = %v, want %v", got, testCase.want)
			}
			if registry.saves != 0 {
				t.Fatalf("须引用预占却登记了 %d 次", registry.saves)
			}
		})
	}
}

// Covers: 票 demo-chain-gaps/21（AT-TF-174）——一行指的池不是本班次的池（别的班次的池、本班次没登过的维度、没挂池的班次上带了行），
// 或那份预占不在那个池上，答`预占不在本班次`并交回指错的行，不登记；先于`须引用预占`答。
func TestAnAllocationMustReferenceAReservationOnThisSchedule(t *testing.T) {
	elsewhere := weightLine(10)
	elsewhere.Pool = "schedule-9/WEIGHT"
	undeclaredDimension := weightLine(10)
	undeclaredDimension.Pool = "schedule-1/VOLUME"
	missingReservation := weightLine(10)
	missingReservation.Reservation = "reservation-never"

	cases := map[string]struct {
		pools *assignmentPoolsDouble
		line  application.LoadAssignmentAllocationLine
	}{
		"别的班次的池": {
			pools: assignmentPools(
				allocationPool(t, "schedule-1", domain.CapacityWeight, "kg", 100, "reservation-w", 60, 0),
				allocationPool(t, "schedule-9", domain.CapacityWeight, "kg", 100, "reservation-w", 60, 0),
			),
			line: elsewhere,
		},
		"本班次没登过的维度": {pools: twoPoolSchedule(t), line: undeclaredDimension},
		"预占不在那个池上":  {pools: twoPoolSchedule(t), line: missingReservation},
		"班次没挂池却带了行": {pools: assignmentPools(), line: weightLine(10)},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			registry := newLoadAssignmentRegistry()
			result, err := newLoadAssignmentHandlerWith(registry, registeredSchedules([2]string{"tenant-1", "schedule-1"}), testCase.pools).
				Form(t.Context(), coveredCommand(t, testCase.line))
			if err != nil || result.Outcome() != application.LoadAssignmentReservationNotOnSchedule {
				t.Fatalf("outcome = %q err = %v, want RESERVATION_NOT_ON_SCHEDULE", result.Outcome(), err)
			}
			misplaced := result.AllocationsNotOnSchedule()
			if len(misplaced) != 1 || misplaced[0].Pool().String() != testCase.line.Pool ||
				misplaced[0].Reservation().String() != testCase.line.Reservation {
				t.Fatalf("指错的行 = %+v", misplaced)
			}
			if registry.saves != 0 {
				t.Fatalf("预占不在本班次却登记了 %d 次", registry.saves)
			}
		})
	}
}

// Covers: 票 demo-chain-gaps/21（AT-TF-175）——引用的预占剩下的不够分：答`分配超出`，带那份预占此刻还能分给这次分配的量（预占 60
// 减已释放 15、减别的分配占着的 20，余 25）、挡下的那一行与池，不登记。恰好分完余量照常形成。
func TestAnAllocationBeyondWhatTheReservationHasLeftIsRefusedWithTheHeadroom(t *testing.T) {
	pools := func(t *testing.T) *assignmentPoolsDouble {
		return assignmentPools(
			allocationPool(t, "schedule-1", domain.CapacityWeight, "kg", 100, "reservation-w", 60, 15,
				otherAllocation(t, "load-assignment-other", 20)),
		)
	}
	registry := newLoadAssignmentRegistry()
	result, err := newLoadAssignmentHandlerWith(registry, registeredSchedules([2]string{"tenant-1", "schedule-1"}), pools(t)).
		Form(t.Context(), coveredCommand(t, weightLine(26)))
	if err != nil || result.Outcome() != application.LoadAssignmentAllocationExceeded {
		t.Fatalf("outcome = %q err = %v, want ALLOCATION_EXCEEDED", result.Outcome(), err)
	}
	if result.AvailableQuantity() != 25 {
		t.Fatalf("余量 = %d, want 25", result.AvailableQuantity())
	}
	allocation, pool, refused := result.RefusedAllocation()
	if !refused || allocation.Pool().String() != weightPool || pool.Pool.Pool().String() != weightPool {
		t.Fatalf("挡下的那一行没有带回来：%+v", allocation)
	}
	if registry.saves != 0 {
		t.Fatalf("分配超出却登记了 %d 次", registry.saves)
	}

	exact, err := newLoadAssignmentHandlerWith(newLoadAssignmentRegistry(), registeredSchedules([2]string{"tenant-1", "schedule-1"}), pools(t)).
		Form(t.Context(), coveredCommand(t, weightLine(25)))
	if err != nil || exact.Outcome() != application.LoadAssignmentFormed {
		t.Fatalf("恰好分完余量：outcome = %q err = %v", exact.Outcome(), err)
	}
}

// Covers: 票 demo-chain-gaps/21（AT-TF-176）——单位不是池的单位、分配时刻早于预占的判断时点、晚于预占的有效期，分别答单位不符、预占
// 尚未生效、预占过期，带挡下的那一行与池，不登记。与预占口同名同义。
func TestAnAllocationIsJudgedAgainstThePoolUnitAndTheReservationWindow(t *testing.T) {
	wrongUnit := weightLine(10)
	wrongUnit.Unit = "KG"
	cases := map[string]struct {
		line       application.LoadAssignmentAllocationLine
		assignedAt time.Time
		want       application.LoadAssignmentOutcome
	}{
		"单位不符":   {line: wrongUnit, assignedAt: assignedAtFixture, want: application.LoadAssignmentUnitMismatch},
		"预占尚未生效": {line: weightLine(10), assignedAt: allocationReservedAt.Add(-time.Second), want: application.LoadAssignmentReservationNotYetEffective},
		"预占过期":   {line: weightLine(10), assignedAt: allocationValidUntil.Add(time.Second), want: application.LoadAssignmentReservationExpired},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			registry := newLoadAssignmentRegistry()
			command := coveredCommand(t, testCase.line)
			command.AssignedAt = testCase.assignedAt
			result, err := newLoadAssignmentHandlerWith(registry, registeredSchedules([2]string{"tenant-1", "schedule-1"}),
				assignmentPools(allocationPool(t, "schedule-1", domain.CapacityWeight, "kg", 100, "reservation-w", 60, 0))).
				Form(t.Context(), command)
			if err != nil || result.Outcome() != testCase.want {
				t.Fatalf("outcome = %q err = %v, want %q", result.Outcome(), err, testCase.want)
			}
			if _, _, refused := result.RefusedAllocation(); !refused {
				t.Fatal("没有带回挡下的那一行与池")
			}
			if registry.saves != 0 {
				t.Fatalf("登记了 %d 次", registry.saves)
			}
		})
	}
}

// Covers: 票 demo-chain-gaps/21（AT-TF-177）——分配行单看不成立答未受理，一次池都不读：成员数落在重量池上（UC-TF-003 不许拿对象数
// 顶替别的维度）、成员数与声明数量同给、声明数量不为正、同一个池两行、来源种类不在封闭词里、缺单位。
func TestAnIncompleteAllocationLineIsNotAcceptedWithoutReadingPools(t *testing.T) {
	membersOnWeight := piecesFromMembers()
	membersOnWeight.Pool = weightPool
	membersOnWeight.Reservation = "reservation-w"
	both := piecesFromMembers()
	both.Quantity = 2
	unknownSource := weightLine(10)
	unknownSource.SourceKind = "MEMBER_COUNT"
	noUnit := weightLine(10)
	noUnit.Unit = ""
	cases := map[string][]application.LoadAssignmentAllocationLine{
		"成员数落在重量池上":  {membersOnWeight},
		"成员数与声明数量同给": {both},
		"声明数量为零":     {weightLine(0)},
		"同一个池两行":     {weightLine(10), weightLine(20)},
		"来源种类不在封闭词里": {unknownSource},
		"缺单位":        {noUnit},
	}
	for name, lines := range cases {
		t.Run(name, func(t *testing.T) {
			registry := newLoadAssignmentRegistry()
			pools := twoPoolSchedule(t)
			result, err := newLoadAssignmentHandlerWith(registry, registeredSchedules([2]string{"tenant-1", "schedule-1"}), pools).
				Form(t.Context(), coveredCommand(t, lines...))
			if err != nil || result.Outcome() != application.LoadAssignmentNotAccepted {
				t.Fatalf("outcome = %q err = %v, want INPUT_NOT_ACCEPTED", result.Outcome(), err)
			}
			if pools.reads != 0 || registry.saves != 0 {
				t.Fatalf("未受理却读了 %d 次池、登记了 %d 次", pools.reads, registry.saves)
			}
		})
	}
}

// Covers: 票 demo-chain-gaps/21（AT-TF-178）——池册读不动答`未决`带续办引用、不登记；同一版本重放与已形成的分配排在容量保障之前，
// 池册读不动也照答原格，一次池都不读。
func TestCapacityAssuranceComesAfterReplayAndIsUndecidedWhenPoolsAreUnreadable(t *testing.T) {
	registry := newLoadAssignmentRegistry()
	unreadable := twoPoolSchedule(t)
	unreadable.err = errors.New("池册不可用")
	schedules := registeredSchedules([2]string{"tenant-1", "schedule-1"})

	result, err := newLoadAssignmentHandlerWith(registry, schedules, unreadable).
		Form(t.Context(), coveredCommand(t, weightLine(10), piecesFromMembers()))
	if err != nil || result.Outcome() != application.LoadAssignmentUndecided || result.ContinuationReference() == "" {
		t.Fatalf("outcome = %q continuation = %q err = %v, want 未决带续办引用", result.Outcome(), result.ContinuationReference(), err)
	}
	if registry.saves != 0 {
		t.Fatalf("池册读不动却登记了 %d 次", registry.saves)
	}

	if formed, err := newLoadAssignmentHandlerWith(registry, schedules, twoPoolSchedule(t)).
		Form(t.Context(), coveredCommand(t, weightLine(10), piecesFromMembers())); err != nil || formed.Outcome() != application.LoadAssignmentFormed {
		t.Fatalf("首版：%q %v", formed.Outcome(), err)
	}
	unreadable.reads = 0
	replay, err := newLoadAssignmentHandlerWith(registry, schedules, unreadable).Form(t.Context(), coveredCommand(t))
	if err != nil || replay.Outcome() != application.LoadAssignmentVersionExists {
		t.Fatalf("同版本重放：%q %v", replay.Outcome(), err)
	}
	if record, _ := replay.Record(); len(record.Assignment.Allocations()) != 2 {
		t.Fatal("重放没有回显原版本的分配量")
	}
	another := coveredCommand(t)
	another.Version = "LAV-000000000002"
	if again, err := newLoadAssignmentHandlerWith(registry, schedules, unreadable).Form(t.Context(), another); err != nil ||
		again.Outcome() != application.LoadAssignmentAlreadyFormed {
		t.Fatalf("换版本号：%q %v", again.Outcome(), err)
	}
	if unreadable.reads != 0 {
		t.Fatalf("重放与链头之前读了 %d 次池", unreadable.reads)
	}
}

// Covers: 票 demo-chain-gaps/21（AT-TF-179）——没挂池的班次照旧形成：不带分配行，读一次池、答已形成，版本上没有分配量。
func TestALoadAssignmentOnAScheduleWithoutPoolsFormsAsBefore(t *testing.T) {
	registry := newLoadAssignmentRegistry()
	pools := assignmentPools(allocationPool(t, "schedule-9", domain.CapacityWeight, "kg", 100, "reservation-w", 60, 0))

	result, err := newLoadAssignmentHandlerWith(registry, registeredSchedules([2]string{"tenant-1", "schedule-1"}), pools).
		Form(t.Context(), formAssignmentCommand(t))
	if err != nil || result.Outcome() != application.LoadAssignmentFormed {
		t.Fatalf("outcome = %q err = %v, want LOAD_ASSIGNMENT_FORMED", result.Outcome(), err)
	}
	if record, _ := result.Record(); len(record.Assignment.Allocations()) != 0 {
		t.Fatal("没挂池的班次上形成的分配带了分配量")
	}
	if pools.reads != 1 {
		t.Fatalf("读了 %d 次池, want 1", pools.reads)
	}
}
