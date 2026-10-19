package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件证装载分配的分配量与预占上的分配关系（票 demo-chain-gaps/21，ADR-0203 决定二）。夹具池是 establishedPool 那一个
// （schedule-1/WEIGHT，kg）；件数池另立。全为合成值（S 级）。

func allocationDeclaration(t *testing.T, pool, reservation string, quantity int64, unit string) domain.CapacityAllocationDeclaration {
	t.Helper()
	source, err := domain.NewCapacityDemandSource(domain.CapacityDemandOperatorDeclaration,
		mustValue(t, domain.NewCapacityDemandSourceReference, "SYN-DEMAND-DECL-1/v1"))
	if err != nil {
		t.Fatal(err)
	}
	return domain.CapacityAllocationDeclaration{
		Pool:        mustValue(t, domain.NewCapacityPoolReference, pool),
		Reservation: reservationRef(t, reservation),
		Quantity:    quantity,
		Unit:        mustValue(t, domain.NewQuantityUnitReference, unit),
		Source:      source,
	}
}

func membersOn(t *testing.T, pool, reservation string) domain.CapacityAllocationDeclaration {
	t.Helper()
	declaration := allocationDeclaration(t, pool, reservation, 0, "pcs")
	declaration.FromMembers = true
	return declaration
}

func allocatedAssignment(t *testing.T, declarations ...domain.CapacityAllocationDeclaration) domain.LoadAssignment {
	t.Helper()
	spec := assignmentSpec(t)
	spec.Allocations = declarations
	assignment, err := domain.FormLoadAssignment(spec)
	if err != nil {
		t.Fatalf("形成带分配量的分配：%v", err)
	}
	return assignment
}

// reservedPool 在夹具池上预占 reservation-1（数量 quantity），判断时点 reservedAt、有效到 reservedUntil。
func reservedPool(t *testing.T, quantity int64, others ...domain.CapacityAllocationSnapshot) domain.CapacityPool {
	t.Helper()
	pool := establishedPool(t, 100)
	source, err := domain.NewCapacityDemandSource(domain.CapacityDemandOperatorDeclaration,
		mustValue(t, domain.NewCapacityDemandSourceReference, "SYN-DEMAND-DECL-1/v1"))
	if err != nil {
		t.Fatal(err)
	}
	rehydrated, err := domain.RehydrateCapacityPool(domain.CapacityPoolSpec{
		TenantID:  pool.TenantID(),
		Schedule:  pool.Schedule(),
		Dimension: pool.Dimension(),
		Unit:      pool.Unit(),
		Capacity:  pool.Capacity(),
	}, []domain.CapacityReservationSnapshot{{
		Reference:   reservationRef(t, "reservation-1"),
		Quantity:    quantity,
		ValidUntil:  reservedUntil,
		Source:      source,
		ReservedAt:  reservedAt,
		Allocations: others,
	}})
	if err != nil {
		t.Fatalf("预占夹具：%v", err)
	}
	return rehydrated
}

func heldBy(t *testing.T, assignment string, quantity int64) domain.CapacityAllocationSnapshot {
	t.Helper()
	return domain.CapacityAllocationSnapshot{
		Assignment: mustValue(t, domain.NewLoadAssignmentReference, assignment),
		Version:    mustValue(t, domain.NewLoadAssignmentVersion, assignment+"/v1"),
		Quantity:   quantity,
	}
}

func weightAllocation(t *testing.T, quantity int64, unit string) domain.CapacityAllocation {
	t.Helper()
	return allocatedAssignment(t, allocationDeclaration(t, "schedule-1/WEIGHT", "reservation-1", quantity, unit)).Allocations()[0]
}

// Covers: 票 demo-chain-gaps/21（AT-TF-177 的领域半边）——分配量逐池一行、带来源；成员数只许落在件数维的池上，数就是成员数；
// 成员数与声明数量同给、声明数量不为正、同一个池两行、池身份认不出维度都不成立。
func TestAllocationLinesAreDeclaredPerPoolAndOnlyPiecesTakeTheMemberCount(t *testing.T) {
	assignment := allocatedAssignment(t,
		membersOn(t, "schedule-1/PIECES", "reservation-p"),
		allocationDeclaration(t, "schedule-1/WEIGHT", "reservation-1", 40, "kg"),
	)
	allocations := assignment.Allocations()
	if len(allocations) != 2 || allocations[0].Pool().String() != "schedule-1/PIECES" {
		t.Fatalf("分配量没按池身份排：%+v", allocations)
	}
	if allocations[0].Demand().Quantity() != 2 || allocations[0].Basis() != domain.CapacityAllocationFromMembers {
		t.Fatalf("件数池取成员数 = %d %s, want 2 MEMBER_COUNT", allocations[0].Demand().Quantity(), allocations[0].Basis())
	}
	if allocations[1].Demand().Quantity() != 40 || allocations[1].Basis() != domain.CapacityAllocationDeclared {
		t.Fatalf("重量池 = %d %s", allocations[1].Demand().Quantity(), allocations[1].Basis())
	}

	membersOnWeight := membersOn(t, "schedule-1/WEIGHT", "reservation-1")
	both := membersOn(t, "schedule-1/PIECES", "reservation-p")
	both.Quantity = 2
	broken := map[string][]domain.CapacityAllocationDeclaration{
		"成员数落在重量池上":     {membersOnWeight},
		"成员数落在认不出维度的池上": {membersOn(t, "schedule-1-PIECES", "reservation-p")},
		"成员数与声明数量同给":    {both},
		"声明数量不为正":       {allocationDeclaration(t, "schedule-1/WEIGHT", "reservation-1", 0, "kg")},
		"同一个池两行": {
			allocationDeclaration(t, "schedule-1/WEIGHT", "reservation-1", 10, "kg"),
			allocationDeclaration(t, "schedule-1/WEIGHT", "reservation-2", 10, "kg"),
		},
	}
	for name, declarations := range broken {
		spec := assignmentSpec(t)
		spec.Allocations = declarations
		if _, err := domain.FormLoadAssignment(spec); !errors.Is(err, domain.ErrInvalidLoadAssignment) {
			t.Fatalf("%s：err = %v, want ErrInvalidLoadAssignment", name, err)
		}
	}
}

// Covers: 票 demo-chain-gaps/21（AT-TF-180 的领域半边）——撤回版不带分配量，改版带自己重新声明的一组、成员数按新成员重数；前一版
// 原样保留。预占上的分配关系取当前那一版，归还因此是版本本身的样子。
func TestRevisingOrWithdrawingAnAssignmentCarriesItsOwnAllocations(t *testing.T) {
	first := allocatedAssignment(t,
		membersOn(t, "schedule-1/PIECES", "reservation-p"),
		allocationDeclaration(t, "schedule-1/WEIGHT", "reservation-1", 40, "kg"),
	)
	revised, err := first.ReviseMembers(
		[]domain.CarriedObjectReference{mustValue(t, domain.NewCarriedObjectReference, "parcel-1")},
		[]domain.CapacityAllocationDeclaration{membersOn(t, "schedule-1/PIECES", "reservation-p")},
		mustValue(t, domain.NewLoadAssignmentVersion, "assignment/v2"),
		assignedAt.Add(time.Hour),
	)
	if err != nil {
		t.Fatalf("改版：%v", err)
	}
	if got := revised.Allocations(); len(got) != 1 || got[0].Demand().Quantity() != 1 {
		t.Fatalf("改版的分配量 = %+v, want 件数池一行、按新成员数 1", got)
	}
	if len(first.Allocations()) != 2 {
		t.Fatal("改版动了前一版的分配量")
	}

	withdrawn, err := revised.Withdraw(mustValue(t, domain.NewLoadAssignmentVersion, "assignment/v3"), assignedAt.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("撤回：%v", err)
	}
	if len(withdrawn.Allocations()) != 0 {
		t.Fatalf("撤回版带着分配量：%+v", withdrawn.Allocations())
	}
	if len(revised.Allocations()) != 1 {
		t.Fatal("撤回动了前一版的分配量")
	}
}

// Covers: 票 demo-chain-gaps/21——重建门核分配量：成员数那一行存的数与读回的成员数对不上、撤回版带着分配量、得法词认不出，都是坏行。
func TestRehydratingAnAssignmentChecksItsStoredAllocations(t *testing.T) {
	first := allocatedAssignment(t, membersOn(t, "schedule-1/PIECES", "reservation-p"))
	stored := func(quantity int64, basis domain.CapacityAllocationBasis) domain.RehydrateCapacityAllocationSpec {
		allocation := first.Allocations()[0]
		return domain.RehydrateCapacityAllocationSpec{
			Pool: allocation.Pool(), Reservation: allocation.Reservation(), Quantity: quantity,
			Unit: allocation.Demand().Unit(), Source: allocation.Demand().Source(), Basis: basis,
		}
	}
	spec := domain.RehydrateLoadAssignmentSpec{
		TenantID: first.TenantID(), Assignment: first.Assignment(), Schedule: first.Schedule(),
		Members: first.Members(), Version: first.Version(), AssignedAt: first.AssignedAt(),
		Allocations: []domain.RehydrateCapacityAllocationSpec{stored(2, domain.CapacityAllocationFromMembers)},
	}
	if rehydrated, err := domain.RehydrateLoadAssignment(spec); err != nil || len(rehydrated.Allocations()) != 1 {
		t.Fatalf("好行读不回：%v", err)
	}

	miscounted := spec
	miscounted.Allocations = []domain.RehydrateCapacityAllocationSpec{stored(3, domain.CapacityAllocationFromMembers)}
	withdrawnWithAllocations := spec
	withdrawnWithAllocations.Corrects = mustValue(t, domain.NewLoadAssignmentVersion, "assignment/v0")
	withdrawnWithAllocations.Withdrawn = true
	withdrawnWithAllocations.WithdrawnAt = assignedAt.Add(time.Hour)
	unknownBasis := spec
	unknownBasis.Allocations = []domain.RehydrateCapacityAllocationSpec{stored(2, domain.CapacityAllocationBasisInvalid)}
	for name, broken := range map[string]domain.RehydrateLoadAssignmentSpec{
		"成员数对不上":   miscounted,
		"撤回版带着分配量": withdrawnWithAllocations,
		"得法词认不出":   unknownBasis,
	} {
		if _, err := domain.RehydrateLoadAssignment(broken); !errors.Is(err, domain.ErrInvalidLoadAssignment) {
			t.Fatalf("%s：err = %v, want ErrInvalidLoadAssignment", name, err)
		}
	}
	if _, err := domain.ParseCapacityAllocationBasis("member_count"); err == nil {
		t.Fatal("得法词按大小写折了")
	}
}

// Covers: 票 demo-chain-gaps/21（AT-TF-175 的领域半边）——一行分配量对着池判：预占不在池上、单位不是池的单位、分配时点在预占有效
// 区间之外、超出那份预占剩下的量各成一种拒绝；剩下的量是预占减释放、减消耗、减别的分配，同一分配原有的那一份不算（它被新的一版
// 接替）。分配不改池：可用容量不随它变。
func TestAnAllocationIsAdmittedAgainstWhatItsReservationHasLeft(t *testing.T) {
	assignment := mustValue(t, domain.NewLoadAssignmentReference, "assignment-1")
	pool := reservedPool(t, 60, heldBy(t, "assignment-other", 20), heldBy(t, "assignment-1", 30))
	within := reservedAt.Add(time.Hour)

	if headroom, found := pool.AllocationHeadroom(reservationRef(t, "reservation-1"), assignment); !found || headroom != 40 {
		t.Fatalf("剩下的量 = %d %v, want 40（不算本分配原有的 30）", headroom, found)
	}
	if err := pool.AdmitAllocation(weightAllocation(t, 40, "kg"), assignment, within); err != nil {
		t.Fatalf("恰好分完：%v", err)
	}
	cases := map[string]struct {
		allocation domain.CapacityAllocation
		at         time.Time
		want       error
	}{
		"超出剩下的量": {weightAllocation(t, 41, "kg"), within, domain.ErrAllocationExceeded},
		"单位不是池的": {weightAllocation(t, 10, "KG"), within, domain.ErrCapacityUnitMismatch},
		"判断时点之前": {weightAllocation(t, 10, "kg"), reservedAt.Add(-time.Second), domain.ErrReservationNotYetEffective},
		"有效期之后":  {weightAllocation(t, 10, "kg"), reservedUntil.Add(time.Second), domain.ErrReservationExpired},
	}
	for name, testCase := range cases {
		if err := pool.AdmitAllocation(testCase.allocation, assignment, testCase.at); !errors.Is(err, testCase.want) {
			t.Fatalf("%s：err = %v, want %v", name, err, testCase.want)
		}
	}
	missing := allocatedAssignment(t, allocationDeclaration(t, "schedule-1/WEIGHT", "reservation-9", 1, "kg")).Allocations()[0]
	if err := pool.AdmitAllocation(missing, assignment, within); !errors.Is(err, domain.ErrReservationNotFound) {
		t.Fatalf("预占不在池上：err = %v", err)
	}
	if pool.AvailableAt(within) != 40 {
		t.Fatalf("可用容量 = %d, want 40：分配不该改池的占用", pool.AvailableAt(within))
	}
}

// Covers: 票 demo-chain-gaps/21（AT-TF-180 的领域半边）——守恒把已分配算在内：分给装载分配的那一段既不能释放回池，也不能被别的依据
// 消耗；余下未分配的照常可释放。读回的池里已分配、已释放、已消耗之和超过预占量，或同一分配在一份预占上两条，都读不回。
func TestAllocatedRangeCannotBeReleasedOrConsumedAndIsCheckedOnRehydration(t *testing.T) {
	pool := reservedPool(t, 60, heldBy(t, "assignment-other", 50))
	within := reservedAt.Add(time.Hour)

	if _, err := pool.Release(releaseRef(t, "release-1"), reservationRef(t, "reservation-1"), declaredDemand(t, 11), within); !errors.Is(err, domain.ErrReservationOverdrawn) {
		t.Fatalf("释放进已分配的那一段：err = %v, want ErrReservationOverdrawn", err)
	}
	if _, err := pool.Release(releaseRef(t, "release-1"), reservationRef(t, "reservation-1"), declaredDemand(t, 10), within); err != nil {
		t.Fatalf("释放未分配的那一段：%v", err)
	}
	if _, err := pool.Consume(reservationRef(t, "reservation-1"), 11, mustValue(t, domain.NewLoadAssignmentReference, "assignment-1"), within); !errors.Is(err, domain.ErrReservationOverdrawn) {
		t.Fatalf("消耗进已分配的那一段：err = %v, want ErrReservationOverdrawn", err)
	}

	spec := domain.CapacityPoolSpec{
		TenantID: pool.TenantID(), Schedule: pool.Schedule(), Dimension: pool.Dimension(), Unit: pool.Unit(), Capacity: pool.Capacity(),
	}
	overdrawn := pool.ReservationSnapshots()
	overdrawn[0].Consumed = 11
	if _, err := domain.RehydrateCapacityPool(spec, overdrawn); !errors.Is(err, domain.ErrReservationOverdrawn) {
		t.Fatalf("超分的预占读回来了：err = %v", err)
	}
	twice := pool.ReservationSnapshots()
	twice[0].Allocations = append(twice[0].Allocations, heldBy(t, "assignment-other", 1))
	if _, err := domain.RehydrateCapacityPool(spec, twice); !errors.Is(err, domain.ErrInvalidCapacityPool) {
		t.Fatalf("同一分配两条读回来了：err = %v", err)
	}
	if roundTrip, err := domain.RehydrateCapacityPool(spec, pool.ReservationSnapshots()); err != nil {
		t.Fatalf("原样读回：%v", err)
	} else if held, _ := roundTrip.ReservationFor(reservationRef(t, "reservation-1")); held.Allocated() != 50 {
		t.Fatalf("读回的已分配 = %d, want 50", held.Allocated())
	}
}
