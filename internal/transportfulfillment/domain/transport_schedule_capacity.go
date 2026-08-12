package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidTransportSchedule = errors.New("transport fulfillment: invalid transport schedule")
	ErrInvalidCapacityPool      = errors.New("transport fulfillment: invalid capacity pool")
	// ErrCapacityExceeded：池容量是硬上限，普通业务授权不能突破（CONTEXT：触及硬限制时
	// 拒绝新的预占，不能以审批绕过）。软容量超配走版本化阈值与缺口处置，另票。
	ErrCapacityExceeded = errors.New("transport fulfillment: the reservation exceeds pool capacity")
	// ErrReservationOverdrawn：释放量+消耗量不得超过预占量——同一范围不得重复扣减或
	// 重复归还（CONTEXT 数量守恒）。
	ErrReservationOverdrawn = errors.New("transport fulfillment: release and consumption exceed the reserved quantity")
	ErrReservationExpired   = errors.New("transport fulfillment: the reservation validity has expired")
	ErrReservationNotFound  = errors.New("transport fulfillment: no such reservation in the pool")
	ErrDuplicateReservation = errors.New("transport fulfillment: the reservation reference is already taken")
)

// ScheduleReference 指名一个具体班次。
type ScheduleReference struct{ requiredValue }

func NewScheduleReference(value string) (ScheduleReference, error) {
	required, err := newRequiredValue("schedule reference", value)
	return ScheduleReference{required}, err
}

// TransportSchedule 是一次运输机会的执行体：明确日期、时间范围与方向的独立班次身份
// （周期模板不是班次）。班次存在不等于容量可用，也不等于已订舱——本类型上没有任何
// 容量或订舱字段可以承载那两个结论，它们各自成对象（CONTEXT 119）。
type TransportSchedule struct {
	tenantID  TenantID
	schedule  ScheduleReference
	direction requiredValue
	departsAt time.Time
}

// TransportScheduleSpec 固定班次身份三件：方向、发运时间与引用。
type TransportScheduleSpec struct {
	TenantID  TenantID
	Schedule  ScheduleReference
	Direction string
	DepartsAt time.Time
}

func FormTransportSchedule(spec TransportScheduleSpec) (TransportSchedule, error) {
	direction, err := newRequiredValue("schedule direction", spec.Direction)
	if err != nil {
		return TransportSchedule{}, ErrInvalidTransportSchedule
	}
	if !spec.TenantID.valid() || !spec.Schedule.valid() || spec.DepartsAt.IsZero() {
		return TransportSchedule{}, ErrInvalidTransportSchedule
	}
	return TransportSchedule{
		tenantID:  spec.TenantID,
		schedule:  spec.Schedule,
		direction: direction,
		departsAt: spec.DepartsAt.UTC(),
	}, nil
}

func (schedule TransportSchedule) TenantID() TenantID {
	return schedule.tenantID
}

func (schedule TransportSchedule) Schedule() ScheduleReference {
	return schedule.schedule
}

func (schedule TransportSchedule) Direction() string {
	return schedule.direction.String()
}

func (schedule TransportSchedule) DepartsAt() time.Time {
	return schedule.departsAt
}

// CapacityPoolReference 指名一个容量池。
type CapacityPoolReference struct{ requiredValue }

func NewCapacityPoolReference(value string) (CapacityPoolReference, error) {
	required, err := newRequiredValue("capacity pool reference", value)
	return CapacityPoolReference{required}, err
}

// CapacityReservationReference 指名一次容量预占。
type CapacityReservationReference struct{ requiredValue }

func NewCapacityReservationReference(value string) (CapacityReservationReference, error) {
	required, err := newRequiredValue("capacity reservation reference", value)
	return CapacityReservationReference{required}, err
}

// LoadAssignmentReference 指名确认消耗的装载分配。消耗只由它确认——预占不是运力承诺
// 也不是订舱，没有装载分配的「消耗」无从谈起。
type LoadAssignmentReference struct{ requiredValue }

func NewLoadAssignmentReference(value string) (LoadAssignmentReference, error) {
	required, err := newRequiredValue("load assignment reference", value)
	return LoadAssignmentReference{required}, err
}

// CapacityReservation 是在容量池的明确维度上为一个业务范围保留可用容量的关系
// （CONTEXT「容量预占」）。预占量、释放量与消耗量分别保存并守恒；有效期届满后未消耗
// 的剩余自动归还可用容量（到期是释放的一种，不需要显式动作）。
type CapacityReservation struct {
	reference  CapacityReservationReference
	quantity   int64
	validUntil time.Time
	released   int64
	consumed   int64
}

func (reservation CapacityReservation) Reference() CapacityReservationReference {
	return reservation.reference
}

// Quantities 报告预占/已释放/已消耗三个守恒量。
func (reservation CapacityReservation) Quantities() (reserved, released, consumed int64) {
	return reservation.quantity, reservation.released, reservation.consumed
}

func (reservation CapacityReservation) ValidUntil() time.Time {
	return reservation.validUntil
}

// occupiedAt 是该预占在某时点仍占用池的量：已消耗的永远占用（实物已在承载范围内），
// 未消耗的剩余只在有效期内占用——过期即归还。
func (reservation CapacityReservation) occupiedAt(at time.Time) int64 {
	occupied := reservation.consumed
	if !at.After(reservation.validUntil) {
		occupied += reservation.quantity - reservation.released - reservation.consumed
	}
	return occupied
}

// CapacityPool 是班次或运输资源在明确范围与维度上可供预占与分配的运力范围（CONTEXT
// 「容量池」）。单维度：多维各自建池，不强制换算（CONTEXT 124）。值类型：每次转换返回
// 新值；释放与消耗都没有反向方法——归还与扣减只能沿正向关系再转换。
type CapacityPool struct {
	tenantID     TenantID
	pool         CapacityPoolReference
	schedule     ScheduleReference
	unit         QuantityUnitReference
	capacity     int64
	reservations []CapacityReservation
}

// CapacityPoolSpec 固定池身份、维度单位与有效容量（硬上限）。
type CapacityPoolSpec struct {
	TenantID TenantID
	Pool     CapacityPoolReference
	Schedule ScheduleReference
	Unit     QuantityUnitReference
	Capacity int64
}

func EstablishCapacityPool(spec CapacityPoolSpec) (CapacityPool, error) {
	if !spec.TenantID.valid() ||
		!spec.Pool.valid() ||
		!spec.Schedule.valid() ||
		!spec.Unit.valid() ||
		spec.Capacity <= 0 {
		return CapacityPool{}, ErrInvalidCapacityPool
	}
	return CapacityPool{
		tenantID: spec.TenantID,
		pool:     spec.Pool,
		schedule: spec.Schedule,
		unit:     spec.Unit,
		capacity: spec.Capacity,
	}, nil
}

func (pool CapacityPool) Pool() CapacityPoolReference {
	return pool.pool
}

func (pool CapacityPool) Schedule() ScheduleReference {
	return pool.schedule
}

func (pool CapacityPool) Unit() QuantityUnitReference {
	return pool.unit
}

func (pool CapacityPool) Capacity() int64 {
	return pool.capacity
}

func (pool CapacityPool) ReservationFor(reference CapacityReservationReference) (CapacityReservation, bool) {
	for _, reservation := range pool.reservations {
		if reservation.reference == reference {
			return reservation, true
		}
	}
	return CapacityReservation{}, false
}

// AvailableAt 报告某时点仍可预占的量：容量减去全部预占在该时点的占用。
func (pool CapacityPool) AvailableAt(at time.Time) int64 {
	available := pool.capacity
	for _, reservation := range pool.reservations {
		available -= reservation.occupiedAt(at)
	}
	return available
}

// Reserve 建立有效预占并扣减可用容量。只有当前有效的预占扣减容量——已过期预占的
// 未消耗剩余在计算可用量时已经归还。超出硬上限拒绝，不能以审批绕过（AT-TF-031）。
func (pool CapacityPool) Reserve(
	reference CapacityReservationReference,
	quantity int64,
	validUntil time.Time,
	at time.Time,
) (CapacityPool, error) {
	if !reference.valid() || quantity <= 0 || validUntil.IsZero() || at.IsZero() || !validUntil.After(at) {
		return CapacityPool{}, ErrInvalidCapacityPool
	}
	if _, exists := pool.ReservationFor(reference); exists {
		return CapacityPool{}, ErrDuplicateReservation
	}
	if pool.AvailableAt(at) < quantity {
		return CapacityPool{}, ErrCapacityExceeded
	}
	reserved := pool
	reserved.reservations = append(append([]CapacityReservation(nil), pool.reservations...), CapacityReservation{
		reference:  reference,
		quantity:   quantity,
		validUntil: validUntil.UTC(),
	})
	return reserved, nil
}

// Release 显式释放部分或全部未消耗范围，归还可用容量。释放不可逆也不可重复：释放量
// 与消耗量之和不得超过预占量（同一范围不得重复归还或扣减）。
func (pool CapacityPool) Release(
	reference CapacityReservationReference,
	quantity int64,
	at time.Time,
) (CapacityPool, error) {
	return pool.convert(reference, at, func(reservation *CapacityReservation) error {
		reservation.released += quantity
		return nil
	}, quantity)
}

// Consume 依据装载分配确认实际消耗。消耗后的量永远占用池——实物已经进入承载范围，
// 释放不了也收不回；没有装载分配依据的消耗无从谈起。
func (pool CapacityPool) Consume(
	reference CapacityReservationReference,
	quantity int64,
	assignment LoadAssignmentReference,
	at time.Time,
) (CapacityPool, error) {
	if !assignment.valid() {
		return CapacityPool{}, ErrInvalidCapacityPool
	}
	return pool.convert(reference, at, func(reservation *CapacityReservation) error {
		reservation.consumed += quantity
		return nil
	}, quantity)
}

// convert 沿正向关系转换预占范围。有效期届满后既不能释放也不能消耗——到期已经把
// 未消耗剩余归还了，再动它就是对同一范围二次转换。
func (pool CapacityPool) convert(
	reference CapacityReservationReference,
	at time.Time,
	apply func(*CapacityReservation) error,
	quantity int64,
) (CapacityPool, error) {
	if quantity <= 0 || at.IsZero() {
		return CapacityPool{}, ErrInvalidCapacityPool
	}
	converted := pool
	converted.reservations = append([]CapacityReservation(nil), pool.reservations...)
	for index := range converted.reservations {
		reservation := &converted.reservations[index]
		if reservation.reference != reference {
			continue
		}
		if at.After(reservation.validUntil) {
			return CapacityPool{}, ErrReservationExpired
		}
		if reservation.released+reservation.consumed+quantity > reservation.quantity {
			return CapacityPool{}, ErrReservationOverdrawn
		}
		if err := apply(reservation); err != nil {
			return CapacityPool{}, err
		}
		return converted, nil
	}
	return CapacityPool{}, ErrReservationNotFound
}
