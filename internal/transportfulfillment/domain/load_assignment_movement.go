package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidLoadAssignment = errors.New("transport fulfillment: invalid load assignment")
	// ErrLoadAssignmentWithdrawn：已撤回的分配没有可变化的意图；再动它只能是新的分配。
	ErrLoadAssignmentWithdrawn = errors.New("transport fulfillment: the load assignment is withdrawn")
	ErrInvalidMovementFact     = errors.New("transport fulfillment: invalid movement fact")
	// ErrDepartureGateBlocked：受监管门禁约束的装载出发动作在没有放行依据时立不成出发
	// ——门禁判断本身属 customs-compliance（GuardedAction 的 LOADING_DEPARTURE 格），
	// 这里只认它的放行引用，不重建门禁机制。
	ErrDepartureGateBlocked = errors.New("transport fulfillment: the gated departure has no clearance basis")
)

// LoadAssignmentVersion 是装载分配的版本标识：变化或撤回形成新版本，不修改分配历史
// （CONTEXT「装载分配形成、变化或撤回时保存版本和对象范围」）。
type LoadAssignmentVersion struct{ requiredValue }

func NewLoadAssignmentVersion(value string) (LoadAssignmentVersion, error) {
	required, err := newRequiredValue("load assignment version", value)
	return LoadAssignmentVersion{required}, err
}

// LoadAssignmentSpec 是形成一次装载分配所需的全部输入。
type LoadAssignmentSpec struct {
	TenantID   TenantID
	Assignment LoadAssignmentReference
	Schedule   ScheduleReference
	Members    []CarriedObjectReference
	Version    LoadAssignmentVersion
	AssignedAt time.Time
}

// LoadAssignment 是把明确载运对象分配到具体班次的执行意图（CONTEXT「装载分配」）。
// 它不证明物理装载完成，也不证明控制转移——实际装载、短装、多装或错装作为独立事实
// 与分配版本比较，不修改分配历史；本类型上没有任何已装载或控制字段可以冒充那些事实。
type LoadAssignment struct {
	tenantID    TenantID
	assignment  LoadAssignmentReference
	schedule    ScheduleReference
	members     []CarriedObjectReference
	version     LoadAssignmentVersion
	assignedAt  time.Time
	corrects    LoadAssignmentVersion
	revisedAt   time.Time
	withdrawn   bool
	withdrawnAt time.Time
}

func FormLoadAssignment(spec LoadAssignmentSpec) (LoadAssignment, error) {
	if !spec.TenantID.valid() ||
		!spec.Assignment.valid() ||
		!spec.Schedule.valid() ||
		len(spec.Members) == 0 ||
		!spec.Version.valid() ||
		spec.AssignedAt.IsZero() {
		return LoadAssignment{}, ErrInvalidLoadAssignment
	}
	members, err := copiedUniqueMembers(spec.Members)
	if err != nil {
		return LoadAssignment{}, ErrInvalidLoadAssignment
	}
	return LoadAssignment{
		tenantID:   spec.TenantID,
		assignment: spec.Assignment,
		schedule:   spec.Schedule,
		members:    members,
		version:    spec.Version,
		assignedAt: spec.AssignedAt.UTC(),
	}, nil
}

func copiedUniqueMembers(members []CarriedObjectReference) ([]CarriedObjectReference, error) {
	seen := make(map[CarriedObjectReference]struct{}, len(members))
	for _, member := range members {
		if !member.valid() {
			return nil, ErrInvalidLoadAssignment
		}
		if _, exists := seen[member]; exists {
			return nil, ErrInvalidLoadAssignment
		}
		seen[member] = struct{}{}
	}
	return append([]CarriedObjectReference(nil), members...), nil
}

func (assignment LoadAssignment) TenantID() TenantID {
	return assignment.tenantID
}

func (assignment LoadAssignment) Assignment() LoadAssignmentReference {
	return assignment.assignment
}

func (assignment LoadAssignment) Schedule() ScheduleReference {
	return assignment.schedule
}

func (assignment LoadAssignment) Members() []CarriedObjectReference {
	return append([]CarriedObjectReference(nil), assignment.members...)
}

func (assignment LoadAssignment) Version() LoadAssignmentVersion {
	return assignment.version
}

func (assignment LoadAssignment) AssignedAt() time.Time {
	return assignment.assignedAt
}

// Corrects 交回本版本变化/撤回所接续的前一版本（若有）。
func (assignment LoadAssignment) Corrects() (LoadAssignmentVersion, bool) {
	if !assignment.corrects.valid() {
		return LoadAssignmentVersion{}, false
	}
	return assignment.corrects, true
}

func (assignment LoadAssignment) Withdrawn() (time.Time, bool) {
	if !assignment.withdrawn {
		return time.Time{}, false
	}
	return assignment.withdrawnAt, true
}

// ReviseMembers 以新对象范围形成新分配版本：原版本原样保留（值语义），新版本回指前身；
// 沿用原版本号就是覆盖，构造期拒绝。
func (assignment LoadAssignment) ReviseMembers(
	members []CarriedObjectReference,
	version LoadAssignmentVersion,
	revisedAt time.Time,
) (LoadAssignment, error) {
	if assignment.withdrawn {
		return LoadAssignment{}, ErrLoadAssignmentWithdrawn
	}
	if len(members) == 0 || !version.valid() || revisedAt.IsZero() || revisedAt.Before(assignment.assignedAt) {
		return LoadAssignment{}, ErrInvalidLoadAssignment
	}
	if version == assignment.version {
		return LoadAssignment{}, ErrInvalidLoadAssignment
	}
	copied, err := copiedUniqueMembers(members)
	if err != nil {
		return LoadAssignment{}, ErrInvalidLoadAssignment
	}
	revised := assignment
	revised.members = copied
	revised.corrects = assignment.version
	revised.version = version
	revised.revisedAt = revisedAt.UTC()
	return revised, nil
}

// Withdraw 撤回分配并形成新版本；已撤回的分配不再变化——后续要装载只能是新的分配。
func (assignment LoadAssignment) Withdraw(
	version LoadAssignmentVersion,
	withdrawnAt time.Time,
) (LoadAssignment, error) {
	if assignment.withdrawn {
		return LoadAssignment{}, ErrLoadAssignmentWithdrawn
	}
	if !version.valid() || withdrawnAt.IsZero() || withdrawnAt.Before(assignment.assignedAt) {
		return LoadAssignment{}, ErrInvalidLoadAssignment
	}
	if version == assignment.version {
		return LoadAssignment{}, ErrInvalidLoadAssignment
	}
	withdrawn := assignment
	withdrawn.members = append([]CarriedObjectReference(nil), assignment.members...)
	withdrawn.corrects = assignment.version
	withdrawn.version = version
	withdrawn.withdrawn = true
	withdrawn.withdrawnAt = withdrawnAt.UTC()
	return withdrawn, nil
}

// MovementFactKind 是实际运输事实的封闭三值：出发、移动、到达。中断、折返、改降属
// 执行结果家族，随履约段与班次结果另行表达。
type MovementFactKind uint8

const (
	MovementFactKindInvalid MovementFactKind = iota
	DepartureFact
	InTransitFact
	ArrivalFact
)

func (kind MovementFactKind) valid() bool {
	return kind >= DepartureFact && kind <= ArrivalFact
}

func (kind MovementFactKind) String() string {
	switch kind {
	case DepartureFact:
		return "DEPARTURE"
	case InTransitFact:
		return "IN_TRANSIT"
	case ArrivalFact:
		return "ARRIVAL"
	default:
		return ""
	}
}

// MovementFactReference 指名一条实际运输事实。
type MovementFactReference struct{ requiredValue }

func NewMovementFactReference(value string) (MovementFactReference, error) {
	required, err := newRequiredValue("movement fact reference", value)
	return MovementFactReference{required}, err
}

// MovementLocationReference 指名事实发生的位置。
type MovementLocationReference struct{ requiredValue }

func NewMovementLocationReference(value string) (MovementLocationReference, error) {
	required, err := newRequiredValue("movement location reference", value)
	return MovementLocationReference{required}, err
}

// MovementSourceReference 指名事实的原始来源（执行方回传、车载设备、伙伴接入……）。
type MovementSourceReference struct{ requiredValue }

func NewMovementSourceReference(value string) (MovementSourceReference, error) {
	required, err := newRequiredValue("movement source reference", value)
	return MovementSourceReference{required}, err
}

// MovementFactVersion 是事实记录的版本标识：迟到与更正形成新版本，不改写历史。
type MovementFactVersion struct{ requiredValue }

func NewMovementFactVersion(value string) (MovementFactVersion, error) {
	required, err := newRequiredValue("movement fact version", value)
	return MovementFactVersion{required}, err
}

// GateClearanceReference 指名受监管门禁约束的装载出发所依据的放行结果（CC 侧
// GuardedAction 的 LOADING_DEPARTURE 格）。
type GateClearanceReference struct{ requiredValue }

func NewGateClearanceReference(value string) (GateClearanceReference, error) {
	required, err := newRequiredValue("gate clearance reference", value)
	return GateClearanceReference{required}, err
}

// MovementFactSpec 是记录一条实际运输事实所需的全部输入。GateRequired 只对出发有意义：
// 门禁约束的是装载出发动作，移动与到达不受它管。
type MovementFactSpec struct {
	TenantID      TenantID
	Fact          MovementFactReference
	Schedule      ScheduleReference
	Kind          MovementFactKind
	Location      MovementLocationReference
	Source        MovementSourceReference
	Version       MovementFactVersion
	OccurredAt    time.Time
	GateRequired  bool
	GateClearance GateClearanceReference
}

// TransportMovementFact 是出发、移动或到达的事实记录（CONTEXT：「出发、移动、到达、
// 中断、折返和结束属于实际事实」，与执行准备判断「不得共用一个可覆盖状态」）。
//
// 它不等于交接、不结束控制——控制转移只随权威交接或有效交付成立，本类型上没有任何
// 控制或交接字段；迟到与更正形成新版本，原记录与其派生历史不被改写。
type TransportMovementFact struct {
	tenantID      TenantID
	fact          MovementFactReference
	schedule      ScheduleReference
	kind          MovementFactKind
	location      MovementLocationReference
	source        MovementSourceReference
	version       MovementFactVersion
	occurredAt    time.Time
	gateClearance GateClearanceReference
	corrects      MovementFactVersion
	correctedAt   time.Time
}

func RecordMovementFact(spec MovementFactSpec) (TransportMovementFact, error) {
	if !spec.TenantID.valid() ||
		!spec.Fact.valid() ||
		!spec.Schedule.valid() ||
		!spec.Kind.valid() ||
		!spec.Location.valid() ||
		!spec.Source.valid() ||
		!spec.Version.valid() ||
		spec.OccurredAt.IsZero() {
		return TransportMovementFact{}, ErrInvalidMovementFact
	}
	if spec.Kind != DepartureFact && (spec.GateRequired || spec.GateClearance.valid()) {
		// 门禁只约束装载出发；把放行依据挂到移动或到达上，等于造了一个不存在的门。
		return TransportMovementFact{}, ErrInvalidMovementFact
	}
	if spec.Kind == DepartureFact && spec.GateRequired && !spec.GateClearance.valid() {
		return TransportMovementFact{}, ErrDepartureGateBlocked
	}
	return TransportMovementFact{
		tenantID:      spec.TenantID,
		fact:          spec.Fact,
		schedule:      spec.Schedule,
		kind:          spec.Kind,
		location:      spec.Location,
		source:        spec.Source,
		version:       spec.Version,
		occurredAt:    spec.OccurredAt.UTC(),
		gateClearance: spec.GateClearance,
	}, nil
}

func (fact TransportMovementFact) TenantID() TenantID {
	return fact.tenantID
}

func (fact TransportMovementFact) Fact() MovementFactReference {
	return fact.fact
}

func (fact TransportMovementFact) Schedule() ScheduleReference {
	return fact.schedule
}

func (fact TransportMovementFact) Kind() MovementFactKind {
	return fact.kind
}

func (fact TransportMovementFact) Location() MovementLocationReference {
	return fact.location
}

func (fact TransportMovementFact) Source() MovementSourceReference {
	return fact.source
}

func (fact TransportMovementFact) Version() MovementFactVersion {
	return fact.version
}

// OccurredAt 是事实发生的业务时间——迟到消息按它采用，不按到达顺序。
func (fact TransportMovementFact) OccurredAt() time.Time {
	return fact.occurredAt
}

// GateClearance 交回出发所依据的门禁放行（若该出发受门禁约束）。
func (fact TransportMovementFact) GateClearance() (GateClearanceReference, bool) {
	if !fact.gateClearance.valid() {
		return GateClearanceReference{}, false
	}
	return fact.gateClearance, true
}

// Corrects 交回本版本更正的前一版本（若本版本由更正产生）。
func (fact TransportMovementFact) Corrects() (MovementFactVersion, bool) {
	if !fact.corrects.valid() {
		return MovementFactVersion{}, false
	}
	return fact.corrects, true
}

func (fact TransportMovementFact) CorrectedAt() (time.Time, bool) {
	if fact.correctedAt.IsZero() {
		return time.Time{}, false
	}
	return fact.correctedAt, true
}

// Correct 依据更正来源形成新版本：保留原事实和原判断（值语义，接收者不动），新版本
// 回指被更正版本；沿用原版本号就是覆盖，构造期拒绝。位置或业务时间可被更正——被更正
// 的是记录，不是让实物倒回去重新出发。
func (fact TransportMovementFact) Correct(
	location MovementLocationReference,
	occurredAt time.Time,
	source MovementSourceReference,
	version MovementFactVersion,
	correctedAt time.Time,
) (TransportMovementFact, error) {
	if !fact.version.valid() {
		return TransportMovementFact{}, ErrInvalidMovementFact
	}
	if !location.valid() || occurredAt.IsZero() || !source.valid() || !version.valid() || correctedAt.IsZero() {
		return TransportMovementFact{}, ErrInvalidMovementFact
	}
	if version == fact.version {
		return TransportMovementFact{}, ErrInvalidMovementFact
	}
	corrected := fact
	corrected.location = location
	corrected.occurredAt = occurredAt.UTC()
	corrected.source = source
	corrected.corrects = fact.version
	corrected.version = version
	corrected.correctedAt = correctedAt.UTC()
	return corrected, nil
}
