package domain

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// ErrAllocationExceeded：分配量超出那一份预占剩下的范围——预占量减去已释放、已消耗，再减去别的装载分配此刻占着的分配量
// （ADR-0203 决定二的守恒）。它与`容量超出`不是一格：后者是池的硬上限挡住新预占，这一格是预占自己的范围不够分，续办是
// 改量、另占一份或先撤别的分配，不是等池。
var ErrAllocationExceeded = errors.New("transport fulfillment: the allocation exceeds what the reservation has left")

// CapacityAllocationBasis 说一行分配量是怎么得出的：运营逐池声明的数，或取分配的成员数（ADR-0203 决定二）。
//
// 成员数单列一种而不当作声明值存：只有件数维的池可以拿成员数作分配量（UC-TF-003 禁止用对象数顶替别的维度），而存成
// 声明值之后，读的人就分不出这个数是数出来的还是报上来的，变化版也没法按新的成员数重算它。
type CapacityAllocationBasis uint8

const (
	CapacityAllocationBasisInvalid CapacityAllocationBasis = iota
	CapacityAllocationDeclared
	CapacityAllocationFromMembers
)

func (basis CapacityAllocationBasis) String() string {
	switch basis {
	case CapacityAllocationDeclared:
		return "DECLARED"
	case CapacityAllocationFromMembers:
		return "MEMBER_COUNT"
	default:
		return ""
	}
}

// ParseCapacityAllocationBasis 把 String 写出的词认回来，供读回走构造门。大小写不折：同一种得法只有一种写法。
func ParseCapacityAllocationBasis(word string) (CapacityAllocationBasis, error) {
	for basis := CapacityAllocationDeclared; basis <= CapacityAllocationFromMembers; basis++ {
		if basis.String() == word {
			return basis, nil
		}
	}
	return CapacityAllocationBasisInvalid, fmt.Errorf("%w: allocation basis %q", ErrInvalidLoadAssignment, word)
}

// CapacityAllocationDeclaration 是装载分配在一个池上的一行分配量声明：引用哪个池上的哪一份预占、分多少、什么单位、
// 依据什么来源。数量要么声明（Quantity），要么取成员数（FromMembers，Quantity 留零）——两者都给或都不给都不成立，
// 不替声明方挑一个。来源与预占的需求来源同一组封闭词（ADR-0203 决定二）。
type CapacityAllocationDeclaration struct {
	Pool        CapacityPoolReference
	Reservation CapacityReservationReference
	Quantity    int64
	FromMembers bool
	Unit        QuantityUnitReference
	Source      CapacityDemandSource
}

// CapacityAllocation 是装载分配的一个版本在一个池上落定的分配量。它随版本存、只插不改：变化版带自己的一组，撤回版
// 一行也不带——预占上的分配关系取各分配当前那一版的这几行，所以撤回与改版的归还不靠任何人记得去还（ADR-0203 决定二
// 与文末补记）。
type CapacityAllocation struct {
	pool        CapacityPoolReference
	reservation CapacityReservationReference
	demand      CapacityDemand
	basis       CapacityAllocationBasis
}

func (allocation CapacityAllocation) Pool() CapacityPoolReference {
	return allocation.pool
}

func (allocation CapacityAllocation) Reservation() CapacityReservationReference {
	return allocation.reservation
}

// Demand 是这一行的数量、单位与来源。成员数得来的那一行，数量就是这一版的成员数。
func (allocation CapacityAllocation) Demand() CapacityDemand {
	return allocation.demand
}

func (allocation CapacityAllocation) Basis() CapacityAllocationBasis {
	return allocation.basis
}

// resolveCapacityAllocations 逐行过构造门，把成员数那几行落成数，按池身份排成一种写法。
//
// 一池只认一行：判据是「对每个池引用一份预占」，同一个池两行就是在一个池上分两份，哪一行算数都是替声明方选。成员数
// 只许落在件数维的池上——维度由池身份派生，单看引用就判得出，不必读池。
func resolveCapacityAllocations(declarations []CapacityAllocationDeclaration, members int) ([]CapacityAllocation, error) {
	if len(declarations) == 0 {
		return nil, nil
	}
	seen := make(map[CapacityPoolReference]bool, len(declarations))
	resolved := make([]CapacityAllocation, 0, len(declarations))
	for _, declaration := range declarations {
		if !declaration.Pool.valid() || !declaration.Reservation.valid() || seen[declaration.Pool] {
			return nil, ErrInvalidLoadAssignment
		}
		seen[declaration.Pool] = true
		basis := CapacityAllocationDeclared
		quantity := declaration.Quantity
		if declaration.FromMembers {
			if declaration.Quantity != 0 {
				return nil, ErrInvalidLoadAssignment
			}
			if dimension, derived := declaration.Pool.Dimension(); !derived || dimension != CapacityPieces {
				return nil, ErrInvalidLoadAssignment
			}
			basis = CapacityAllocationFromMembers
			quantity = int64(members)
		}
		demand, err := NewCapacityDemand(quantity, declaration.Unit, declaration.Source)
		if err != nil {
			return nil, ErrInvalidLoadAssignment
		}
		resolved = append(resolved, CapacityAllocation{
			pool:        declaration.Pool,
			reservation: declaration.Reservation,
			demand:      demand,
			basis:       basis,
		})
	}
	slices.SortFunc(resolved, func(left, right CapacityAllocation) int {
		return strings.Compare(left.pool.String(), right.pool.String())
	})
	return resolved, nil
}

// Dimension 从派生的池身份认回维度。池身份由（班次、维度）派生、按最后一个分隔符切开总能认回两半（见
// capacityPoolReference）；认不回说明这不是一个派生出来的池身份。
func (reference CapacityPoolReference) Dimension() (CapacityDimension, bool) {
	value := reference.String()
	cut := strings.LastIndex(value, capacityPoolSeparator)
	if cut <= 0 {
		return CapacityDimensionInvalid, false
	}
	dimension, err := ParseCapacityDimension(value[cut+len(capacityPoolSeparator):])
	if err != nil {
		return CapacityDimensionInvalid, false
	}
	return dimension, true
}

// ReservationAllocation 是预占上的一条分配关系：哪一次装载分配，以它当前那一版，在这份预占上分走了多少。
//
// 它从各分配的当前版本派生，不在预占上另存：分配量只在装载分配的版本上存一份，预占读的是它。两处各存一份，撤回或改版
// 时就得有人记得去改预占那一份，漏改一次，那段范围就永远还不回来，而没有任何东西会报（ADR-0203 文末补记）。
type ReservationAllocation struct {
	assignment LoadAssignmentReference
	version    LoadAssignmentVersion
	quantity   int64
}

func (allocation ReservationAllocation) Assignment() LoadAssignmentReference {
	return allocation.assignment
}

func (allocation ReservationAllocation) Version() LoadAssignmentVersion {
	return allocation.version
}

func (allocation ReservationAllocation) Quantity() int64 {
	return allocation.quantity
}

// CapacityAllocationSnapshot 是预占上一条分配关系在读回时的样子。
type CapacityAllocationSnapshot struct {
	Assignment LoadAssignmentReference
	Version    LoadAssignmentVersion
	Quantity   int64
}

// Allocated 是这份预占此刻分出去的总量。
func (reservation CapacityReservation) Allocated() int64 {
	var allocated int64
	for _, allocation := range reservation.allocations {
		allocated += allocation.quantity
	}
	return allocated
}

// Allocations 交出这份预占上的全部分配关系，按分配引用排。
func (reservation CapacityReservation) Allocations() []ReservationAllocation {
	return append([]ReservationAllocation(nil), reservation.allocations...)
}

// allocatedApartFrom 是别的装载分配在这份预占上占着的量。同一分配原有的那一份不算：它的新一版接替旧的一版，旧的
// 那一份随之归还，不能让一次分配在同一份预占上跟自己抢。
func (reservation CapacityReservation) allocatedApartFrom(assignment LoadAssignmentReference) int64 {
	var allocated int64
	for _, allocation := range reservation.allocations {
		if allocation.assignment != assignment {
			allocated += allocation.quantity
		}
	}
	return allocated
}

// AllocationHeadroom 答这份预占还能分给 assignment 多少：预占量减去已释放、已消耗，再减去别的装载分配占着的分配量
// （ADR-0203 决定二「已分配 ≤ 预占 − 释放 − 消耗」）。释放按全部已落的算，不按时点：守恒是数量的事，倒填一次释放不该
// 让同一范围既还回池里又分给分配。预占不在池上答 false。
func (pool CapacityPool) AllocationHeadroom(
	reservation CapacityReservationReference,
	assignment LoadAssignmentReference,
) (int64, bool) {
	held, found := pool.ReservationFor(reservation)
	if !found {
		return 0, false
	}
	return held.quantity - held.released - held.consumed - held.allocatedApartFrom(assignment), true
}

// AdmitAllocation 判一行分配量能不能落在这个池的那一份预占上（ADR-0203 决定二）。次序：预占在池上 → 单位是池的单位
// → 分配时点落在预占的有效区间里 → 守恒。
//
// 本方法不改池、不调 Consume：分配是执行意图，占的是预占里已经扣过池的那一段，可用容量不随它变；消耗是永久占用，撤回时
// 还不回来。分配量随装载分配的版本落库，预占读回时从各分配当前那一版派生出分配关系，所以这里只判、不交回新池。
func (pool CapacityPool) AdmitAllocation(
	allocation CapacityAllocation,
	assignment LoadAssignmentReference,
	at time.Time,
) error {
	at = AtRegistryPrecision(at)
	if allocation.pool != pool.pool || !assignment.valid() || at.IsZero() {
		return ErrInvalidCapacityPool
	}
	held, found := pool.ReservationFor(allocation.reservation)
	if !found {
		return ErrReservationNotFound
	}
	if allocation.demand.unit != pool.unit {
		return ErrCapacityUnitMismatch
	}
	if at.Before(held.reservedAt) {
		return ErrReservationNotYetEffective
	}
	if at.After(held.validUntil) {
		return ErrReservationExpired
	}
	headroom, _ := pool.AllocationHeadroom(allocation.reservation, assignment)
	if allocation.demand.quantity > headroom {
		return ErrAllocationExceeded
	}
	return nil
}

// rehydrateReservationAllocations 核读回的分配关系：引用与版本成立、量为正、一次分配在一份预占上只有一条（各分配只有
// 当前那一版算数）。
func rehydrateReservationAllocations(snapshots []CapacityAllocationSnapshot) ([]ReservationAllocation, error) {
	if len(snapshots) == 0 {
		return nil, nil
	}
	seen := make(map[LoadAssignmentReference]bool, len(snapshots))
	allocations := make([]ReservationAllocation, 0, len(snapshots))
	for _, snapshot := range snapshots {
		if !snapshot.Assignment.valid() || !snapshot.Version.valid() || snapshot.Quantity <= 0 || seen[snapshot.Assignment] {
			return nil, ErrInvalidCapacityPool
		}
		seen[snapshot.Assignment] = true
		allocations = append(allocations, ReservationAllocation{
			assignment: snapshot.Assignment,
			version:    snapshot.Version,
			quantity:   snapshot.Quantity,
		})
	}
	slices.SortFunc(allocations, func(left, right ReservationAllocation) int {
		return strings.Compare(left.assignment.String(), right.assignment.String())
	})
	return allocations, nil
}
