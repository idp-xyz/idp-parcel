package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 证装载分配的分配量与预占上的分配关系（迁移 0031，票 demo-chain-gaps/21，ADR-0203 决定二与文末
// 补记）：分配量随版本落册、读回走构造门；预占上的分配关系取各分配当前那一版派生，撤回与改版之后旧版那一组不再算数；
// 已分配的那一段释放不出来，池换存不丢分配行；库面守住分配行的形状与「一版至多一个后继」。夹具全为合成值（S 级）：班次
// SYN-TPL-1/2026-10-21 的重量池 5000 KG 上一份预占 SYN-RES-1 1200 KG。

const allocationScheduleRef = "SYN-TPL-1/2026-10-21"

var allocationAssignedAt = registryReservedAt.Add(2 * time.Hour)

type allocationStores struct {
	scheduleRegistryStores
	tenant      domain.TenantID
	poolID      string
	assignments *adapter.LoadAssignments
}

func newAllocationStores(t *testing.T) allocationStores {
	t.Helper()
	stores := newScheduleRegistryStores(t)
	tenant := deliveryValue(t, domain.NewTenantID, "SYN-TENANT-01")
	pool := registeredWeightPool(t, stores, tenant)
	assignments, err := adapter.NewLoadAssignments(stores.db)
	if err != nil {
		t.Fatalf("装载分配册：%v", err)
	}
	wantPoolOutcome(t, stores.reserve(t, registryReserveCommand(tenant, pool, "SYN-RES-1", 1200)), application.CapacityReserved)
	return allocationStores{scheduleRegistryStores: stores, tenant: tenant, poolID: pool, assignments: assignments}
}

func (stores allocationStores) form(t *testing.T, command application.FormLoadAssignmentCommand) application.FormLoadAssignmentResult {
	t.Helper()
	handler := application.NewFormLoadAssignmentHandler(application.FormLoadAssignmentDeps{
		Assignments: stores.assignments,
		Schedules:   stores.schedules,
		Pools:       stores.pools,
		Clock:       fixedScheduleClock{at: allocationAssignedAt.Add(time.Minute)},
	})
	var result application.FormLoadAssignmentResult
	if err := stores.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		handled, err := handler.Form(txCtx, command)
		result = handled
		return err
	}); err != nil {
		t.Fatalf("形成分配：%v", err)
	}
	return result
}

func (stores allocationStores) assignmentCommand(assignment string, lines ...application.LoadAssignmentAllocationLine) application.FormLoadAssignmentCommand {
	return application.FormLoadAssignmentCommand{
		TenantID: stores.tenant, Assignment: assignment, Schedule: allocationScheduleRef,
		Members: []string{"SYN-PARCEL-1", "SYN-PARCEL-2"}, Version: "v1", AssignedAt: allocationAssignedAt,
		Allocations: lines,
	}
}

func (stores allocationStores) weightLine(quantity int64) application.LoadAssignmentAllocationLine {
	return application.LoadAssignmentAllocationLine{
		Pool: stores.poolID, Reservation: "SYN-RES-1", Quantity: quantity, Unit: "KG",
		SourceKind: "OPERATOR_DECLARATION", SourceReference: "SYN-DEMAND-DECL-01/v1",
	}
}

// reservation 读回池上那一份预占。
func (stores allocationStores) reservation(t *testing.T) domain.CapacityReservation {
	t.Helper()
	found, exists, err := stores.pools.FindByKey(t.Context(), ports.CapacityPoolKey{
		TenantID: stores.tenant, Pool: deliveryValue(t, domain.NewCapacityPoolReference, stores.poolID),
	})
	if err != nil || !exists {
		t.Fatalf("读回池：exists=%v err=%v", exists, err)
	}
	reservation, held := found.Pool.ReservationFor(deliveryValue(t, domain.NewCapacityReservationReference, "SYN-RES-1"))
	if !held {
		t.Fatal("池上没有 SYN-RES-1")
	}
	return reservation
}

func (stores allocationStores) save(t *testing.T, assignment domain.LoadAssignment) {
	t.Helper()
	key := ports.LoadAssignmentKey{TenantID: assignment.TenantID(), Assignment: assignment.Assignment(), Version: assignment.Version()}
	var saved ports.LoadAssignmentSaveOutcome
	if err := stores.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		outcome, err := stores.assignments.Save(txCtx, ports.LoadAssignmentRecord{Key: key, Assignment: assignment, RecordedAt: allocationAssignedAt.Add(time.Hour)})
		saved = outcome
		return err
	}); err != nil || saved != ports.LoadAssignmentSaved {
		t.Fatalf("登记 %s：%v %v", assignment.Version(), saved, err)
	}
}

func wantAllocated(t *testing.T, reservation domain.CapacityReservation, quantity int64, version string) {
	t.Helper()
	if reservation.Allocated() != quantity {
		t.Fatalf("预占上已分配 = %d, want %d", reservation.Allocated(), quantity)
	}
	allocations := reservation.Allocations()
	if version == "" {
		if len(allocations) != 0 {
			t.Fatalf("分配关系 = %+v, want 没有", allocations)
		}
		return
	}
	if len(allocations) != 1 || allocations[0].Version().String() != version {
		t.Fatalf("分配关系 = %+v, want 一条、取 %s", allocations, version)
	}
}

// Covers: 票 demo-chain-gaps/21（AT-TF-171、AT-TF-175、AT-TF-180）——在真实库上：带预占引用的分配形成，分配量随版本落册并原样读回；
// 预占上的分配关系取当前那一版；超出余量的第二个分配答`分配超出`带余量；已分配的那一段释放不出来、未分配的那一段照常释放，
// 换存池不丢分配行；改版之后取新版的量，撤回之后归零，归还的那一段随即释放得出来；各版的分配行都留着。
func TestAllocationsLandWithTheVersionAndTheReservationTakesTheCurrentOne(t *testing.T) {
	stores := newAllocationStores(t)

	formed := stores.form(t, stores.assignmentCommand("SYN-LA-1", stores.weightLine(1000)))
	if formed.Outcome() != application.LoadAssignmentFormed {
		t.Fatalf("形成：%q", formed.Outcome())
	}
	found, exists, err := stores.assignments.FindByKey(t.Context(), ports.LoadAssignmentKey{
		TenantID: stores.tenant, Assignment: deliveryValue(t, domain.NewLoadAssignmentReference, "SYN-LA-1"),
		Version: deliveryValue(t, domain.NewLoadAssignmentVersion, "v1"),
	})
	if err != nil || !exists {
		t.Fatalf("读回分配：exists=%v err=%v", exists, err)
	}
	allocations := found.Assignment.Allocations()
	if len(allocations) != 1 {
		t.Fatalf("分配量 %d 行, want 1", len(allocations))
	}
	allocation := allocations[0]
	if allocation.Pool().String() != stores.poolID || allocation.Reservation().String() != "SYN-RES-1" ||
		allocation.Demand().Quantity() != 1000 || allocation.Demand().Unit().String() != "KG" ||
		allocation.Demand().Source().Reference().String() != "SYN-DEMAND-DECL-01/v1" || allocation.Basis() != domain.CapacityAllocationDeclared {
		t.Fatalf("分配量读回 = %+v", allocation)
	}
	wantAllocated(t, stores.reservation(t), 1000, "v1")

	exceeded := stores.form(t, stores.assignmentCommand("SYN-LA-2", stores.weightLine(201)))
	if exceeded.Outcome() != application.LoadAssignmentAllocationExceeded || exceeded.AvailableQuantity() != 200 {
		t.Fatalf("超出余量：%q 余量 %d, want ALLOCATION_EXCEEDED 200", exceeded.Outcome(), exceeded.AvailableQuantity())
	}

	releasedAt := allocationAssignedAt.Add(time.Hour)
	wantPoolOutcome(t, stores.release(t, registryReleaseCommand(stores.tenant, stores.poolID, "SYN-RES-1", "SYN-REL-1", 201, releasedAt)),
		application.ReservationOverdrawnOutcome)
	wantPoolOutcome(t, stores.release(t, registryReleaseCommand(stores.tenant, stores.poolID, "SYN-RES-1", "SYN-REL-1", 200, releasedAt)),
		application.CapacityReleased)
	wantAllocated(t, stores.reservation(t), 1000, "v1")

	revised, err := found.Assignment.ReviseMembers(
		[]domain.CarriedObjectReference{deliveryValue(t, domain.NewCarriedObjectReference, "SYN-PARCEL-1")},
		[]domain.CapacityAllocationDeclaration{{
			Pool:        allocation.Pool(),
			Reservation: allocation.Reservation(),
			Quantity:    600,
			Unit:        allocation.Demand().Unit(),
			Source:      allocation.Demand().Source(),
		}},
		deliveryValue(t, domain.NewLoadAssignmentVersion, "v2"),
		allocationAssignedAt.Add(30*time.Minute),
	)
	if err != nil {
		t.Fatalf("改版：%v", err)
	}
	stores.save(t, revised)
	wantAllocated(t, stores.reservation(t), 600, "v2")

	withdrawn, err := revised.Withdraw(deliveryValue(t, domain.NewLoadAssignmentVersion, "v3"), allocationAssignedAt.Add(45*time.Minute))
	if err != nil {
		t.Fatalf("撤回：%v", err)
	}
	stores.save(t, withdrawn)
	wantAllocated(t, stores.reservation(t), 0, "")
	wantPoolOutcome(t, stores.release(t, registryReleaseCommand(stores.tenant, stores.poolID, "SYN-RES-1", "SYN-REL-2", 1000, releasedAt)),
		application.CapacityReleased)

	var rows int
	if err := stores.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM transport_fulfillment.load_assignment_allocation WHERE assignment_ref = 'SYN-LA-1'`).Scan(&rows); err != nil || rows != 2 {
		t.Fatalf("SYN-LA-1 的分配行 %d（%v），want v1、v2 各一行留着", rows, err)
	}
}

// Covers: 票 demo-chain-gaps/21（AT-TF-171）——两个池各一行：件数池取成员数，读回仍是成员数那一种得法、数等于成员数。
func TestAMemberCountAllocationRoundTripsOnAPiecesPool(t *testing.T) {
	stores := newAllocationStores(t)
	wantPoolOutcome(t, stores.establishPool(t, application.EstablishPoolCommand{
		TenantID: stores.tenant, Schedule: allocationScheduleRef, Dimension: "PIECES", Unit: "PCS", Capacity: 1000,
	}), application.PoolEstablished)
	pieces := allocationScheduleRef + "/PIECES"
	reserve := registryReserveCommand(stores.tenant, pieces, "SYN-RES-P", 10)
	reserve.Unit = "PCS"
	wantPoolOutcome(t, stores.reserve(t, reserve), application.CapacityReserved)

	members := application.LoadAssignmentAllocationLine{
		Pool: pieces, Reservation: "SYN-RES-P", FromMembers: true, Unit: "PCS",
		SourceKind: "PREPARATION_DEMAND_SNAPSHOT", SourceReference: "SYN-PREP-01/demand-v1",
	}
	formed := stores.form(t, stores.assignmentCommand("SYN-LA-1", stores.weightLine(100), members))
	if formed.Outcome() != application.LoadAssignmentFormed {
		t.Fatalf("形成：%q", formed.Outcome())
	}
	found, _, err := stores.assignments.FindByKey(t.Context(), ports.LoadAssignmentKey{
		TenantID: stores.tenant, Assignment: deliveryValue(t, domain.NewLoadAssignmentReference, "SYN-LA-1"),
		Version: deliveryValue(t, domain.NewLoadAssignmentVersion, "v1"),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, allocation := range found.Assignment.Allocations() {
		if allocation.Pool().String() == pieces &&
			(allocation.Basis() != domain.CapacityAllocationFromMembers || allocation.Demand().Quantity() != 2) {
			t.Fatalf("件数池那一行读回 = %s %d, want MEMBER_COUNT 2", allocation.Basis(), allocation.Demand().Quantity())
		}
	}
}

// Covers: 票 demo-chain-gaps/21——库面守住分配行的形状：成员数落在重量池上、指向不在池上的预占（外键提交时核）都写不进去；
// 同一版本第二个后继撞「一版至多一个后继」，登记口译成`已登记`。
func TestTheAllocationTableHoldsItsShape(t *testing.T) {
	stores := newAllocationStores(t)
	if formed := stores.form(t, stores.assignmentCommand("SYN-LA-1", stores.weightLine(10))); formed.Outcome() != application.LoadAssignmentFormed {
		t.Fatalf("形成：%q", formed.Outcome())
	}
	insert := func(pool, reservation, basis string) error {
		tx, err := stores.pool.Begin(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(context.Background()) }()
		if _, err := tx.Exec(t.Context(),
			`INSERT INTO transport_fulfillment.load_assignment_allocation
			     (tenant_id, assignment_ref, version, pool_id, reservation_id, quantity, unit_ref,
			      source_kind, source_ref, quantity_basis, recorded_at)
			 VALUES ('SYN-TENANT-01', 'SYN-LA-1', 'v1', $1, $2, 2, 'KG', 'OPERATOR_DECLARATION', 'D', $3, now())`,
			pool, reservation, basis); err != nil {
			return err
		}
		return tx.Commit(t.Context())
	}
	wantViolation := func(label string, err error, constraint string) {
		t.Helper()
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.ConstraintName != constraint {
			t.Fatalf("%s：err = %v, want 撞 %s", label, err, constraint)
		}
	}
	wantPoolOutcome(t, stores.establishPool(t, application.EstablishPoolCommand{
		TenantID: stores.tenant, Schedule: allocationScheduleRef, Dimension: "VOLUME", Unit: "CM3", Capacity: 1000,
	}), application.PoolEstablished)
	volume := allocationScheduleRef + "/VOLUME"
	volumeReserve := registryReserveCommand(stores.tenant, volume, "SYN-RES-V", 10)
	volumeReserve.Unit = "CM3"
	wantPoolOutcome(t, stores.reserve(t, volumeReserve), application.CapacityReserved)
	wantViolation("成员数落在体积池上", insert(volume, "SYN-RES-V", "MEMBER_COUNT"), "load_assignment_allocation_member_count_on_pieces")
	wantViolation("预占不在池上", insert(volume, "SYN-RES-NEVER", "DECLARED"), "load_assignment_allocation_reservation_fkey")

	first := loadAssignmentAt(t, stores, "SYN-LA-1", "v1")
	twin, err := first.Withdraw(deliveryValue(t, domain.NewLoadAssignmentVersion, "v2"), allocationAssignedAt.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	stores.save(t, twin)
	fork, err := first.Withdraw(deliveryValue(t, domain.NewLoadAssignmentVersion, "v2-fork"), allocationAssignedAt.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	var saved ports.LoadAssignmentSaveOutcome
	if err := stores.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		saved, err = stores.assignments.Save(txCtx, ports.LoadAssignmentRecord{
			Key:        ports.LoadAssignmentKey{TenantID: fork.TenantID(), Assignment: fork.Assignment(), Version: fork.Version()},
			Assignment: fork, RecordedAt: allocationAssignedAt.Add(2 * time.Hour),
		})
		return err
	}); err != nil || saved != ports.LoadAssignmentVersionAlreadyRegistered {
		t.Fatalf("同一版本第二个后继：%v %v, want 已登记", saved, err)
	}
}

func loadAssignmentAt(t *testing.T, stores allocationStores, assignment, version string) domain.LoadAssignment {
	t.Helper()
	found, exists, err := stores.assignments.FindByKey(t.Context(), ports.LoadAssignmentKey{
		TenantID: stores.tenant, Assignment: deliveryValue(t, domain.NewLoadAssignmentReference, assignment),
		Version: deliveryValue(t, domain.NewLoadAssignmentVersion, version),
	})
	if err != nil || !exists {
		t.Fatalf("读回 %s/%s：exists=%v err=%v", assignment, version, exists, err)
	}
	return found.Assignment
}
