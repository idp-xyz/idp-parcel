package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidTransportSchedule = errors.New("transport fulfillment: invalid transport schedule")
	ErrInvalidScheduleDirection = errors.New("transport fulfillment: invalid schedule direction")
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

// NetworkConnectionReference 指名 network-routing 目录上的一条连接，不带版本。拼写归 NR，本上下文原样保存、不解析，
// 也不核它在 NR 目录里在不在：登班次与登网络是两本册子，各有各的登记方与节奏（ADR-0201 决定二）。
type NetworkConnectionReference struct{ requiredValue }

func NewNetworkConnectionReference(value string) (NetworkConnectionReference, error) {
	required, err := newRequiredValue("network connection reference", value)
	return NetworkConnectionReference{required}, err
}

// NetworkNodeReference 指名 network-routing 目录上的一个节点，拼写同样归 NR。
type NetworkNodeReference struct{ requiredValue }

func NewNetworkNodeReference(value string) (NetworkNodeReference, error) {
	required, err := newRequiredValue("network node reference", value)
	return NetworkNodeReference{required}, err
}

// ScheduleServiceScopeReference 指名班次适用的服务范围（CONTEXT「建立具体班次」要固定的那一格）。它只被引用，
// 本上下文不据以挑选候选——候选按方向与时刻判（ADR-0201 决定六）。
type ScheduleServiceScopeReference struct{ requiredValue }

func NewScheduleServiceScopeReference(value string) (ScheduleServiceScopeReference, error) {
	required, err := newRequiredValue("schedule service scope reference", value)
	return ScheduleServiceScopeReference{required}, err
}

// ScheduleDirection 是班次与周期安排的方向身份：NR 连接代码加起讫节点（ADR-0201 决定二）。只认节点对会在平行连接
// 上撞成同一个方向；只认连接代码又挡不住起讫被登反——三格缺一不可。与计划履约段怎么比对归消费侧适配器，本类型
// 不认识 NR 的任何结构。
type ScheduleDirection struct {
	connection NetworkConnectionReference
	from       NetworkNodeReference
	to         NetworkNodeReference
}

func NewScheduleDirection(connection, from, to string) (ScheduleDirection, error) {
	connectionRef, err := NewNetworkConnectionReference(connection)
	if err != nil {
		return ScheduleDirection{}, ErrInvalidScheduleDirection
	}
	fromRef, err := NewNetworkNodeReference(from)
	if err != nil {
		return ScheduleDirection{}, ErrInvalidScheduleDirection
	}
	toRef, err := NewNetworkNodeReference(to)
	if err != nil {
		return ScheduleDirection{}, ErrInvalidScheduleDirection
	}
	if fromRef == toRef {
		return ScheduleDirection{}, ErrInvalidScheduleDirection
	}
	return ScheduleDirection{connection: connectionRef, from: fromRef, to: toRef}, nil
}

func (direction ScheduleDirection) valid() bool {
	return direction.connection.valid() && direction.from.valid() && direction.to.valid() && direction.from != direction.to
}

func (direction ScheduleDirection) Connection() NetworkConnectionReference {
	return direction.connection
}

func (direction ScheduleDirection) From() NetworkNodeReference {
	return direction.from
}

func (direction ScheduleDirection) To() NetworkNodeReference {
	return direction.to
}

// ScheduleVersion 是具体班次在登记册上的版本序号，从 1 起。同一身份异内容再登就是下一版（ADR-0201 决定四）；
// 序号由登记册顺次给，不由登记方指名——登记方登的是「这一班现在是什么样」，不是「第几版」。
type ScheduleVersion int

func (version ScheduleVersion) valid() bool {
	return version >= 1
}

// ScheduleOriginKind 说一个具体班次从哪来：由周期安排按日期物化，或是不出自任何周期安排的临时班次。
type ScheduleOriginKind uint8

const (
	ScheduleOriginInvalid ScheduleOriginKind = iota
	ScheduleMaterializedFromTemplate
	ScheduleAdHoc
)

func (kind ScheduleOriginKind) String() string {
	switch kind {
	case ScheduleMaterializedFromTemplate:
		return "TEMPLATE"
	case ScheduleAdHoc:
		return "AD_HOC"
	default:
		return ""
	}
}

// ParseScheduleOriginKind 把 String 写出的词认回来，供读回走构造门。
func ParseScheduleOriginKind(value string) (ScheduleOriginKind, error) {
	switch value {
	case ScheduleMaterializedFromTemplate.String():
		return ScheduleMaterializedFromTemplate, nil
	case ScheduleAdHoc.String():
		return ScheduleAdHoc, nil
	default:
		return ScheduleOriginInvalid, ErrInvalidTransportSchedule
	}
}

// ScheduleOrigin 是班次的来历。物化来的班次记下周期安排代码、物化时那一版与运行日期；运行日期连同代码就是班次
// 身份的来源，版本只是出处——同一日期按更新的一版再物化，若班次本身没变，就不算班次换了内容。
type ScheduleOrigin struct {
	kind            ScheduleOriginKind
	template        ScheduleTemplateCode
	templateVersion ScheduleTemplateVersion
	runningDate     RunningDate
}

func (origin ScheduleOrigin) Kind() ScheduleOriginKind {
	return origin.kind
}

// Template 只在物化来的班次上交回出处；临时班次交回 false。
func (origin ScheduleOrigin) Template() (ScheduleTemplateCode, ScheduleTemplateVersion, RunningDate, bool) {
	if origin.kind != ScheduleMaterializedFromTemplate {
		return ScheduleTemplateCode{}, 0, RunningDate{}, false
	}
	return origin.template, origin.templateVersion, origin.runningDate, true
}

func (origin ScheduleOrigin) valid() bool {
	switch origin.kind {
	case ScheduleMaterializedFromTemplate:
		return origin.template.valid() && origin.templateVersion.valid() && origin.runningDate.valid()
	case ScheduleAdHoc:
		return !origin.template.valid() && origin.templateVersion == 0 && !origin.runningDate.valid()
	default:
		return false
	}
}

// TransportSchedule 是一次运输机会的执行体：明确运行方向、出发与到达时刻、适用服务范围的独立班次身份（CONTEXT
// 「建立具体班次」；周期安排不是班次）。班次存在不等于容量可用，也不等于已订舱——本类型上没有任何容量或订舱字段
// 可以承载那两个结论，它们各自成对象。
type TransportSchedule struct {
	tenantID     TenantID
	schedule     ScheduleReference
	version      ScheduleVersion
	direction    ScheduleDirection
	departsAt    time.Time
	arrivesAt    time.Time
	serviceScope ScheduleServiceScopeReference
	origin       ScheduleOrigin
}

// TransportScheduleSpec 是一个班次的身份与正文：方向、出发与到达时刻、适用服务范围。
type TransportScheduleSpec struct {
	TenantID     TenantID
	Schedule     ScheduleReference
	Direction    ScheduleDirection
	DepartsAt    time.Time
	ArrivesAt    time.Time
	ServiceScope ScheduleServiceScopeReference
}

// FormTransportSchedule 立一个不出自周期安排的临时班次的首版（ADR-0201 决定五）。临时班次不得取物化班次那种
// 「代码/日期」形的身份：那一形归周期安排，按（代码、日期）物化时会撞上一份来历不同的班次，同一身份两种来历，
// 读的人分不出这一班是谁定的。
func FormTransportSchedule(spec TransportScheduleSpec) (TransportSchedule, error) {
	if _, _, derived := parseMaterializedScheduleReference(spec.Schedule.String()); derived {
		return TransportSchedule{}, ErrInvalidTransportSchedule
	}
	return formSchedule(spec, 1, ScheduleOrigin{kind: ScheduleAdHoc})
}

// RehydrateTransportScheduleSpec 是读回一版班次所需的全部：正文、版本与来历。
type RehydrateTransportScheduleSpec struct {
	Schedule        TransportScheduleSpec
	Version         ScheduleVersion
	OriginKind      ScheduleOriginKind
	Template        ScheduleTemplateCode
	TemplateVersion ScheduleTemplateVersion
	RunningDate     RunningDate
}

// RehydrateTransportSchedule 是持久化适配器读回一版班次的门：逐格过构造门，并核身份与来历对得上——物化来的
// 班次身份必须正是由它的（代码、日期）派生的那一个，临时班次身份必须不是那一形。
func RehydrateTransportSchedule(spec RehydrateTransportScheduleSpec) (TransportSchedule, error) {
	origin := ScheduleOrigin{
		kind:            spec.OriginKind,
		template:        spec.Template,
		templateVersion: spec.TemplateVersion,
		runningDate:     spec.RunningDate,
	}
	switch spec.OriginKind {
	case ScheduleMaterializedFromTemplate:
		if !origin.valid() || spec.Schedule.Schedule != materializedScheduleReference(spec.Template, spec.RunningDate) {
			return TransportSchedule{}, ErrInvalidTransportSchedule
		}
	case ScheduleAdHoc:
		if _, _, derived := parseMaterializedScheduleReference(spec.Schedule.Schedule.String()); derived {
			return TransportSchedule{}, ErrInvalidTransportSchedule
		}
	}
	return formSchedule(spec.Schedule, spec.Version, origin)
}

func formSchedule(spec TransportScheduleSpec, version ScheduleVersion, origin ScheduleOrigin) (TransportSchedule, error) {
	departsAt := AtRegistryPrecision(spec.DepartsAt)
	arrivesAt := AtRegistryPrecision(spec.ArrivesAt)
	if !spec.TenantID.valid() || !spec.Schedule.valid() || !version.valid() || !origin.valid() ||
		!spec.Direction.valid() || !spec.ServiceScope.valid() ||
		departsAt.IsZero() || arrivesAt.IsZero() || !arrivesAt.After(departsAt) {
		return TransportSchedule{}, ErrInvalidTransportSchedule
	}
	return TransportSchedule{
		tenantID:     spec.TenantID,
		schedule:     spec.Schedule,
		version:      version,
		direction:    spec.Direction,
		departsAt:    spec.DepartsAt.UTC(),
		arrivesAt:    spec.ArrivesAt.UTC(),
		serviceScope: spec.ServiceScope,
		origin:       origin,
	}, nil
}

// Revise 以 next 的正文形成本班次的下一版（ADR-0201 决定四）。身份与来历种类不变：临时班次不会变成物化来的，
// 物化来的也只能由同一（代码、日期）再物化——身份本来就由它们派生。内容没变就不该有新版，那由编排比摘要答
// `已存在`，不走到这里。
func (schedule TransportSchedule) Revise(next TransportSchedule) (TransportSchedule, error) {
	if next.tenantID != schedule.tenantID || next.schedule != schedule.schedule ||
		next.origin.kind != schedule.origin.kind {
		return TransportSchedule{}, ErrInvalidTransportSchedule
	}
	revised := next
	revised.version = schedule.version + 1
	return revised, nil
}

func (schedule TransportSchedule) TenantID() TenantID {
	return schedule.tenantID
}

func (schedule TransportSchedule) Schedule() ScheduleReference {
	return schedule.schedule
}

func (schedule TransportSchedule) Version() ScheduleVersion {
	return schedule.version
}

func (schedule TransportSchedule) Direction() ScheduleDirection {
	return schedule.direction
}

func (schedule TransportSchedule) DepartsAt() time.Time {
	return schedule.departsAt
}

func (schedule TransportSchedule) ArrivesAt() time.Time {
	return schedule.arrivesAt
}

func (schedule TransportSchedule) ServiceScope() ScheduleServiceScopeReference {
	return schedule.serviceScope
}

func (schedule TransportSchedule) Origin() ScheduleOrigin {
	return schedule.origin
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
// 「容量池」）。单维度：多维各自建池，不强制换算（AT-TF-030）——CONTEXT 说的多维运力范围，在这里是同一班次上
// 逐维的一组池。值类型：每次转换返回新值；释放与消耗都没有反向方法——归还与扣减只能沿正向关系再转换。
type CapacityPool struct {
	tenantID     TenantID
	pool         CapacityPoolReference
	schedule     ScheduleReference
	dimension    CapacityDimension
	unit         QuantityUnitReference
	capacity     int64
	reservations []CapacityReservation
}

// CapacityPoolSpec 固定池所在班次、维度、单位与有效容量（硬上限）。池身份不在这里：它由班次与维度派生。
type CapacityPoolSpec struct {
	TenantID  TenantID
	Schedule  ScheduleReference
	Dimension CapacityDimension
	Unit      QuantityUnitReference
	Capacity  int64
}

// EstablishCapacityPool 立一个空池。池身份由（班次、维度）派生：一个班次在一个维度上只有一个池，周期安排物化与显式
// 登记两处立的同一维度是同一个池，不会因为两处各起一个名字而在同一班次上并存两个重量池——准备评估要问「这一班的
// 重量够不够」，两个重量池让这一问没有唯一答案（ADR-0201 补记「池身份」）。
func EstablishCapacityPool(spec CapacityPoolSpec) (CapacityPool, error) {
	if !spec.TenantID.valid() ||
		!spec.Schedule.valid() ||
		!spec.Dimension.valid() ||
		!spec.Unit.valid() ||
		spec.Capacity <= 0 {
		return CapacityPool{}, ErrInvalidCapacityPool
	}
	return CapacityPool{
		tenantID:  spec.TenantID,
		pool:      capacityPoolReference(spec.Schedule, spec.Dimension),
		schedule:  spec.Schedule,
		dimension: spec.Dimension,
		unit:      spec.Unit,
		capacity:  spec.Capacity,
	}, nil
}

// capacityPoolSeparator 隔开班次身份与维度词。取 `/` 与物化班次身份同一个分隔符：派生池身份读作「这一班/这一维」。
// 维度词里没有 `/`，按最后一个 `/` 切开总能认回班次与维度，派生因此是单射。
const capacityPoolSeparator = "/"

func capacityPoolReference(schedule ScheduleReference, dimension CapacityDimension) CapacityPoolReference {
	return CapacityPoolReference{requiredValue{value: schedule.String() + capacityPoolSeparator + dimension.String()}}
}

func (pool CapacityPool) TenantID() TenantID {
	return pool.tenantID
}

func (pool CapacityPool) Pool() CapacityPoolReference {
	return pool.pool
}

func (pool CapacityPool) Schedule() ScheduleReference {
	return pool.schedule
}

func (pool CapacityPool) Dimension() CapacityDimension {
	return pool.dimension
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

// CapacityReservationSnapshot 是一次预占在库里的样子：三量加有效期。装载分配引用
// 不在预占对象上——Consume 只把它当调用期依据，读回不必、也不能重放那次确认。
type CapacityReservationSnapshot struct {
	Reference  CapacityReservationReference
	Quantity   int64
	Released   int64
	Consumed   int64
	ValidUntil time.Time
}

// ReservationSnapshots 交出当前全部预占，供适配器按行落库。顺序与内部切片一致。
func (pool CapacityPool) ReservationSnapshots() []CapacityReservationSnapshot {
	snapshots := make([]CapacityReservationSnapshot, 0, len(pool.reservations))
	for _, reservation := range pool.reservations {
		snapshots = append(snapshots, CapacityReservationSnapshot{
			Reference:  reservation.reference,
			Quantity:   reservation.quantity,
			Released:   reservation.released,
			Consumed:   reservation.consumed,
			ValidUntil: reservation.validUntil,
		})
	}
	return snapshots
}

// RehydrateCapacityPool 从身份加预占快照重建池。EstablishCapacityPool 只建空池，
// 读回已有预占不能重放 Reserve/Release/Consume：那三条是转换门，有效期与装载分配
// 是调用期依据，不是行上的事实。
func RehydrateCapacityPool(spec CapacityPoolSpec, snapshots []CapacityReservationSnapshot) (CapacityPool, error) {
	pool, err := EstablishCapacityPool(spec)
	if err != nil {
		return CapacityPool{}, err
	}
	if len(snapshots) == 0 {
		return pool, nil
	}
	seen := make(map[string]struct{}, len(snapshots))
	reservations := make([]CapacityReservation, 0, len(snapshots))
	for _, snapshot := range snapshots {
		if !snapshot.Reference.valid() || snapshot.Quantity <= 0 || snapshot.ValidUntil.IsZero() {
			return CapacityPool{}, ErrInvalidCapacityPool
		}
		if snapshot.Released < 0 || snapshot.Consumed < 0 || snapshot.Released+snapshot.Consumed > snapshot.Quantity {
			return CapacityPool{}, ErrReservationOverdrawn
		}
		if _, exists := seen[snapshot.Reference.String()]; exists {
			return CapacityPool{}, ErrDuplicateReservation
		}
		seen[snapshot.Reference.String()] = struct{}{}
		reservations = append(reservations, CapacityReservation{
			reference:  snapshot.Reference,
			quantity:   snapshot.Quantity,
			validUntil: snapshot.ValidUntil.UTC(),
			released:   snapshot.Released,
			consumed:   snapshot.Consumed,
		})
	}
	pool.reservations = reservations
	return pool, nil
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
	if !reference.valid() || quantity <= 0 || AtRegistryPrecision(validUntil).IsZero() || at.IsZero() || !validUntil.After(at) {
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
