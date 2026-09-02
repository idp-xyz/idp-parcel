package domain

import (
	"errors"
	"sort"
	"time"
)

var (
	ErrInvalidConsolidation  = errors.New("node operations: invalid consolidation unit operation")
	ErrUnitSealed            = errors.New("node operations: the unit is sealed")
	ErrUnitClosed            = errors.New("node operations: the unit is permanently closed")
	ErrUnitNotSealed         = errors.New("node operations: the unit is not sealed")
	ErrMembersStillContained = errors.New("node operations: members are still contained")
)

// ConsolidationUnitID 是集运单元实例的标识。实例贯穿一次完整使用周期；可复用载具
// 再次投入使用时创建新实例并关联同一载具身份——载具身份与实例标识是两样东西。
type ConsolidationUnitID struct{ requiredValue }

func NewConsolidationUnitID(value string) (ConsolidationUnitID, error) {
	required, err := newRequiredValue("consolidation unit ID", value)
	return ConsolidationUnitID{required}, err
}

// CarrierAssetReference 指名可复用载具（笼车、袋、箱）的作业引用。
type CarrierAssetReference struct{ requiredValue }

func NewCarrierAssetReference(value string) (CarrierAssetReference, error) {
	required, err := newRequiredValue("carrier asset reference", value)
	return CarrierAssetReference{required}, err
}

// SealReference 指名一次封签记录。重新封装创建新封签，历史封签不被覆盖。
type SealReference struct{ requiredValue }

func NewSealReference(value string) (SealReference, error) {
	required, err := newRequiredValue("seal reference", value)
	return SealReference{required}, err
}

// WorkBasisReference 指名封装、开封或处置转移的作业依据。
type WorkBasisReference struct{ requiredValue }

func NewWorkBasisReference(value string) (WorkBasisReference, error) {
	required, err := newRequiredValue("work basis reference", value)
	return WorkBasisReference{required}, err
}

// SealedSnapshot 是封装时冻结的成员关系版本（CONTEXT「封装成员快照」），同时承载这一次
// 的「封签记录」：成员集、封签、作业依据与来源一次进入，值类型无改写入口——历史快照不被
// 新版本覆盖是结构性的。
//
// source 是这次封装的来源那一层。CONTEXT「封签记录」要保存的`施封依据`是 basis，`执行方、
// 来源和证据`（UC-NO-003 结果契约）则全在 source 里；封装时刻也取自它自带的业务时间，
// 不再是处理时的系统时钟。
type SealedSnapshot struct {
	members  []HandlingUnitID
	seal     SealReference
	basis    WorkBasisReference
	source   WorkFactSource
	sealedAt time.Time
}

func (snapshot SealedSnapshot) Members() []HandlingUnitID {
	return append([]HandlingUnitID(nil), snapshot.members...)
}

func (snapshot SealedSnapshot) Seal() SealReference {
	return snapshot.seal
}

func (snapshot SealedSnapshot) Basis() WorkBasisReference {
	return snapshot.basis
}

// Source 交回这次封装的来源、执行方与证据。
func (snapshot SealedSnapshot) Source() WorkFactSource {
	return snapshot.source
}

func (snapshot SealedSnapshot) SealedAt() time.Time {
	return snapshot.sealedAt
}

// unitPhase 是集运单元实例的三相：开放（可变更成员）、已封装（成员冻结）、已关闭
// （永久终局）。
type unitPhase uint8

const (
	unitOpen unitPhase = iota
	unitSealed
	unitClosed
)

// ConsolidationUnit 是一个集运单元实例。成员是作业实物（`HandlingUnitID`）；嵌套
// 集运（单元装单元）经直接父级链路派生间接包含，本对象先只承载实物成员，嵌套层
// 落地时按同一条「同一时点最多一个直接物理父级」硬句扩展。跨单元的单父级唯一性
// 需要仓储视野，由收纳编排在加入前核对；本对象守住自己这一侧：重复加入拒。
type ConsolidationUnit struct {
	id        ConsolidationUnitID
	asset     CarrierAssetReference
	openedBy  WorkFactSource
	phase     unitPhase
	members   map[HandlingUnitID]bool
	snapshots []SealedSnapshot
	closedAt  time.Time
}

// OpenConsolidationUnit 开启一个新实例。来源必备：一个说不出谁开的、依据什么开的实例，
// 后面挂在它下面的成员与封签都没有可追溯的起点。
func OpenConsolidationUnit(
	id ConsolidationUnitID,
	asset CarrierAssetReference,
	source WorkFactSource,
) (*ConsolidationUnit, error) {
	if !id.valid() || !asset.valid() {
		return nil, ErrInvalidConsolidation
	}
	if !source.valid() {
		return nil, ErrInvalidWorkFactSource
	}
	return &ConsolidationUnit{
		id:       id,
		asset:    asset,
		openedBy: source,
		phase:    unitOpen,
		members:  map[HandlingUnitID]bool{},
	}, nil
}

func (unit *ConsolidationUnit) ID() ConsolidationUnitID {
	return unit.id
}

func (unit *ConsolidationUnit) Asset() CarrierAssetReference {
	return unit.asset
}

// OpenedBy 交回开启这个实例的来源、执行方与证据。
func (unit *ConsolidationUnit) OpenedBy() WorkFactSource {
	return unit.openedBy
}

// Members 给出当前直接成员（稳定排序的副本）。
func (unit *ConsolidationUnit) Members() []HandlingUnitID {
	members := make([]HandlingUnitID, 0, len(unit.members))
	for member := range unit.members {
		members = append(members, member)
	}
	sort.Slice(members, func(left, right int) bool {
		return members[left].String() < members[right].String()
	})
	return members
}

// Snapshots 给出全部历史快照（副本）——重新封装创建新快照，一份都不覆盖。
func (unit *ConsolidationUnit) Snapshots() []SealedSnapshot {
	return append([]SealedSnapshot(nil), unit.snapshots...)
}

func (unit *ConsolidationUnit) Sealed() bool {
	return unit.phase == unitSealed
}

func (unit *ConsolidationUnit) Closed() bool {
	return unit.phase == unitClosed
}

// AddMember 移入一件实物。封装态拒绝——「封装后改变实物成员必须先形成开封事实」；
// 已在本单元的成员重复移入拒绝。
func (unit *ConsolidationUnit) AddMember(member HandlingUnitID) error {
	if unit.phase == unitClosed {
		return ErrUnitClosed
	}
	if unit.phase == unitSealed {
		return ErrUnitSealed
	}
	if !member.valid() || unit.members[member] {
		return ErrInvalidConsolidation
	}
	unit.members[member] = true
	return nil
}

// RemoveMember 移出一件实物，同样只在开放态允许。
func (unit *ConsolidationUnit) RemoveMember(member HandlingUnitID) error {
	if unit.phase == unitClosed {
		return ErrUnitClosed
	}
	if unit.phase == unitSealed {
		return ErrUnitSealed
	}
	if !member.valid() || !unit.members[member] {
		return ErrInvalidConsolidation
	}
	delete(unit.members, member)
	return nil
}

// Seal 封装：冻结当时成员快照并记封签。空单元封不了——没有成员的封装冻结不出任何
// 关系；已封装再封必须先开封。
//
// 封装时刻取自 source 自带的业务时间，不再由调用方另给一个 at：两处时间并存时，快照会
// 记下与来源事实不一致的那一个，而现场只报了一个时间。
func (unit *ConsolidationUnit) Seal(
	seal SealReference,
	basis WorkBasisReference,
	source WorkFactSource,
) error {
	if unit.phase == unitClosed {
		return ErrUnitClosed
	}
	if unit.phase == unitSealed {
		return ErrUnitSealed
	}
	if !source.valid() {
		return ErrInvalidWorkFactSource
	}
	if !seal.valid() || !basis.valid() || len(unit.members) == 0 {
		return ErrInvalidConsolidation
	}
	unit.snapshots = append(unit.snapshots, SealedSnapshot{
		members:  unit.Members(),
		seal:     seal,
		basis:    basis,
		source:   source,
		sealedAt: source.OccurredAt(),
	})
	unit.phase = unitSealed
	return nil
}

// Unseal 开封：回到开放态，历史快照原样保留。未封装开不了封。
func (unit *ConsolidationUnit) Unseal(basis WorkBasisReference, at time.Time) error {
	if unit.phase == unitClosed {
		return ErrUnitClosed
	}
	if unit.phase != unitSealed {
		return ErrUnitNotSealed
	}
	if !basis.valid() || at.IsZero() {
		return ErrInvalidConsolidation
	}
	unit.phase = unitOpen
	return nil
}

// Close 显式终局关闭：只在全部成员已经移出，或剩余成员已通过明确处置转移后成立
// （CONTEXT 硬句）。已关闭实例永久终局——不得重开、清空后复用或承载新成员；可复用
// 载具再投入使用创建新实例，不动这一个。
func (unit *ConsolidationUnit) Close(disposition WorkBasisReference, at time.Time) error {
	if unit.phase == unitClosed {
		return ErrUnitClosed
	}
	if unit.phase == unitSealed {
		return ErrUnitSealed
	}
	if at.IsZero() {
		return ErrInvalidConsolidation
	}
	if len(unit.members) > 0 && !disposition.valid() {
		return ErrMembersStillContained
	}
	unit.phase = unitClosed
	unit.closedAt = at.UTC()
	return nil
}

// ClosedAt 只在已关闭实例上非零。
func (unit *ConsolidationUnit) ClosedAt() time.Time {
	return unit.closedAt
}
