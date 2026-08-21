package domain

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalidSuspension = errors.New("pilot governance: invalid suspension decision")
	ErrInvalidResumption = errors.New("pilot governance: invalid resumption decision")
	ErrInvalidInventory  = errors.New("pilot governance: invalid inventory entry")
	ErrInvalidTakeover   = errors.New("pilot governance: invalid takeover record")
)

// SuspensionID 是暂停决定的标识。
type SuspensionID struct{ requiredValue }

func NewSuspensionID(value string) (SuspensionID, error) {
	required, err := newRequiredValue("suspension ID", value)
	return SuspensionID{required}, err
}

// SuspensionDecisionSpec 是一次暂停决定所需的全部输入（交接治理记录表逐项）。
type SuspensionDecisionSpec struct {
	ID            SuspensionID
	TriggerSource string
	Basis         string
	Evidence      string
	Scope         ScopeVersionReference
	ExecutedBy    string
	OccurredAt    time.Time
	EffectiveAt   time.Time
	InTransitNote string
}

// SuspensionDecision 是「暂停指定范围的新委托纳入」的记录：它不取消、不迁移、不回退
// 已在途对象——类型上没有那些字段，暂停的语义边界是结构性的。
type SuspensionDecision struct {
	id            SuspensionID
	triggerSource string
	basis         string
	evidence      string
	scope         ScopeVersionReference
	executedBy    string
	occurredAt    time.Time
	effectiveAt   time.Time
	inTransitNote string
}

func RecordSuspension(spec SuspensionDecisionSpec) (SuspensionDecision, error) {
	if !spec.ID.valid() ||
		strings.TrimSpace(spec.TriggerSource) == "" ||
		strings.TrimSpace(spec.Basis) == "" ||
		strings.TrimSpace(spec.Evidence) == "" ||
		!spec.Scope.valid() ||
		strings.TrimSpace(spec.ExecutedBy) == "" ||
		spec.OccurredAt.IsZero() ||
		spec.EffectiveAt.IsZero() ||
		strings.TrimSpace(spec.InTransitNote) == "" {
		return SuspensionDecision{}, ErrInvalidSuspension
	}
	return SuspensionDecision{
		id:            spec.ID,
		triggerSource: spec.TriggerSource,
		basis:         spec.Basis,
		evidence:      spec.Evidence,
		scope:         spec.Scope,
		executedBy:    spec.ExecutedBy,
		occurredAt:    spec.OccurredAt.UTC(),
		effectiveAt:   spec.EffectiveAt.UTC(),
		inTransitNote: spec.InTransitNote,
	}, nil
}

func (suspension SuspensionDecision) ID() SuspensionID {
	return suspension.id
}

func (suspension SuspensionDecision) Scope() ScopeVersionReference {
	return suspension.scope
}

func (suspension SuspensionDecision) Evidence() string {
	return suspension.evidence
}

func (suspension SuspensionDecision) TriggerSource() string {
	return suspension.triggerSource
}

func (suspension SuspensionDecision) Basis() string {
	return suspension.basis
}

func (suspension SuspensionDecision) ExecutedBy() string {
	return suspension.executedBy
}

func (suspension SuspensionDecision) OccurredAt() time.Time {
	return suspension.occurredAt
}

func (suspension SuspensionDecision) InTransitNote() string {
	return suspension.inTransitNote
}

func (suspension SuspensionDecision) EffectiveAt() time.Time {
	return suspension.effectiveAt
}

// AdmissionSuspensionGround 说明「某个范围版本此刻还拦不拦新准入」这一答复凭什么成立。
//
// 三格而不是布尔，因为「拦」有两种来源，而两者的运维动作不同：命中那格该去走恢复决定，
// 保守那格该去把范围版本之间的覆盖关系登进登记册。折成同一格就把这个差别丢了，跟把
// 依赖故障读成「没暂停」是同一类错。
type AdmissionSuspensionGround uint8

const (
	AdmissionSuspensionGroundInvalid AdmissionSuspensionGround = iota
	// AdmissionNotSuspended：该时点没有任何已生效且尚未被恢复的暂停。
	AdmissionNotSuspended
	// AdmissionSuspendedByNamedScope：有一条已生效未恢复的暂停，且它写明的范围版本
	// 正是所问那一版。
	AdmissionSuspendedByNamedScope
	// AdmissionSuspendedByUnreadableScopeRelation：有已生效未恢复的暂停，但它写明的
	// 范围版本与所问那一版之间的覆盖关系在登记册里读不出来。
	//
	// ScopeVersionReference 是脱敏引用组合，前缀、子串、版本号解析或时间序都推不出谁
	// 覆盖谁——从不透明串里解析谱系等于发明实例事实。试点范围规则对这个处境已经给了
	// 动作：「如果共享依赖、共同原因或证据不足导致影响范围无法可靠隔离，必须保守暂停
	// 整个试点的新准入，不能仅拒绝当前报错的单个委托后继续放量。」
	AdmissionSuspendedByUnreadableScopeRelation
)

// Blocks 说这一格要不要拦住新准入。只有 AdmissionNotSuspended 放行，判断不出来的一律
// 算拦着，零值同此：漏填一处不能表现为一次默认放行。
func (ground AdmissionSuspensionGround) Blocks() bool {
	return ground != AdmissionNotSuspended
}

func (ground AdmissionSuspensionGround) String() string {
	switch ground {
	case AdmissionNotSuspended:
		return "NOT_SUSPENDED"
	case AdmissionSuspendedByNamedScope:
		return "SUSPENDED_BY_NAMED_SCOPE"
	case AdmissionSuspendedByUnreadableScopeRelation:
		return "SUSPENDED_BY_UNREADABLE_SCOPE_RELATION"
	default:
		return ""
	}
}

// InventoryEntry 是在途盘点的一条审计快照：逐项稳定身份、当前有效事实、当前权威方、
// 责任方、下一行动和预计复核时间（PAR-GOV-08 完成队列口径——未终局委托不要求被强行
// 关闭，但每项六件必须齐全）。
type InventoryEntry struct {
	ObjectIdentity   string
	CurrentFacts     string
	CurrentAuthority string
	ResponsibleParty string
	NextAction       string
	ReviewBy         time.Time
}

func (entry InventoryEntry) complete() bool {
	return strings.TrimSpace(entry.ObjectIdentity) != "" &&
		strings.TrimSpace(entry.CurrentFacts) != "" &&
		strings.TrimSpace(entry.CurrentAuthority) != "" &&
		strings.TrimSpace(entry.ResponsibleParty) != "" &&
		strings.TrimSpace(entry.NextAction) != "" &&
		!entry.ReviewBy.IsZero()
}

// InTransitInventory 是一次在途盘点：条目六件齐全、对象身份不重。它是审计快照不是
// 生命周期决定——「不对业务对象生命周期重新决定」，类型上没有任何改写对象的入口。
type InTransitInventory struct {
	entries []InventoryEntry
	takenAt time.Time
}

func TakeInventory(entries []InventoryEntry, takenAt time.Time) (InTransitInventory, error) {
	if len(entries) == 0 || takenAt.IsZero() {
		return InTransitInventory{}, ErrInvalidInventory
	}
	seen := make(map[string]bool, len(entries))
	for _, entry := range entries {
		if !entry.complete() || seen[entry.ObjectIdentity] {
			return InTransitInventory{}, ErrInvalidInventory
		}
		seen[entry.ObjectIdentity] = true
	}
	return InTransitInventory{
		entries: append([]InventoryEntry(nil), entries...),
		takenAt: takenAt.UTC(),
	}, nil
}

func (inventory InTransitInventory) Entries() []InventoryEntry {
	return append([]InventoryEntry(nil), inventory.entries...)
}

func (inventory InTransitInventory) TakenAt() time.Time {
	return inventory.takenAt
}

// ResumptionDecisionSpec 是一次恢复决定所需的全部输入。
type ResumptionDecisionSpec struct {
	Suspension       SuspensionID
	ReleaseEvidence  string
	ConsistencyCheck string
	Inventory        InTransitInventory
	DecidedBy        string
	DecidedAt        time.Time
	EffectiveAt      time.Time
}

// ResumptionDecision 是恢复新准入的记录：原暂停决定引用、原因解除证据、一致性核对
// 与在途盘点缺一不可（交接治理记录表逐项）；原暂停记录不可修改——这里只引用它的
// 标识，没有任何回写入口。
type ResumptionDecision struct {
	suspension       SuspensionID
	releaseEvidence  string
	consistencyCheck string
	inventory        InTransitInventory
	decidedBy        string
	decidedAt        time.Time
	effectiveAt      time.Time
}

func RecordResumption(spec ResumptionDecisionSpec) (ResumptionDecision, error) {
	if !spec.Suspension.valid() ||
		strings.TrimSpace(spec.ReleaseEvidence) == "" ||
		strings.TrimSpace(spec.ConsistencyCheck) == "" ||
		len(spec.Inventory.entries) == 0 ||
		strings.TrimSpace(spec.DecidedBy) == "" ||
		spec.DecidedAt.IsZero() ||
		spec.EffectiveAt.IsZero() {
		return ResumptionDecision{}, ErrInvalidResumption
	}
	return ResumptionDecision{
		suspension:       spec.Suspension,
		releaseEvidence:  spec.ReleaseEvidence,
		consistencyCheck: spec.ConsistencyCheck,
		inventory:        spec.Inventory,
		decidedBy:        spec.DecidedBy,
		decidedAt:        spec.DecidedAt.UTC(),
		effectiveAt:      spec.EffectiveAt.UTC(),
	}, nil
}

func (resumption ResumptionDecision) Suspension() SuspensionID {
	return resumption.suspension
}

func (resumption ResumptionDecision) Inventory() InTransitInventory {
	return resumption.inventory
}

func (resumption ResumptionDecision) ReleaseEvidence() string {
	return resumption.releaseEvidence
}

func (resumption ResumptionDecision) ConsistencyCheck() string {
	return resumption.consistencyCheck
}

func (resumption ResumptionDecision) DecidedBy() string {
	return resumption.decidedBy
}

func (resumption ResumptionDecision) DecidedAt() time.Time {
	return resumption.decidedAt
}

func (resumption ResumptionDecision) EffectiveAt() time.Time {
	return resumption.effectiveAt
}

// TakeoverRecordSpec 是一次对象级受控接管所需的全部输入。
type TakeoverRecordSpec struct {
	StopEvidence     string
	Interval         AuthorityInterval
	AcceptedFacts    string
	PendingExternals string
	ActualControl    string
	Responsibilities string
	NextAction       string
	Inventory        InTransitInventory
	EffectiveAt      time.Time
}

// TakeoverRecord 是「原权威确实无法继续时，对明确对象范围建立新权威区间」的记录：
// 原权威停止写入证据先行（先停原权威写入，逐对象盘点和受控接管——批量迁移、重复
// 指令与覆盖历史是被点名的错误结果）；新权威区间与在途盘点必备。
type TakeoverRecord struct {
	stopEvidence     string
	interval         AuthorityInterval
	acceptedFacts    string
	pendingExternals string
	actualControl    string
	responsibilities string
	nextAction       string
	inventory        InTransitInventory
	effectiveAt      time.Time
}

func RecordTakeover(spec TakeoverRecordSpec) (TakeoverRecord, error) {
	if strings.TrimSpace(spec.StopEvidence) == "" ||
		!spec.Interval.valid() ||
		strings.TrimSpace(spec.AcceptedFacts) == "" ||
		strings.TrimSpace(spec.PendingExternals) == "" ||
		strings.TrimSpace(spec.ActualControl) == "" ||
		strings.TrimSpace(spec.Responsibilities) == "" ||
		strings.TrimSpace(spec.NextAction) == "" ||
		len(spec.Inventory.entries) == 0 ||
		spec.EffectiveAt.IsZero() {
		return TakeoverRecord{}, ErrInvalidTakeover
	}
	return TakeoverRecord{
		stopEvidence:     spec.StopEvidence,
		interval:         spec.Interval,
		acceptedFacts:    spec.AcceptedFacts,
		pendingExternals: spec.PendingExternals,
		actualControl:    spec.ActualControl,
		responsibilities: spec.Responsibilities,
		nextAction:       spec.NextAction,
		inventory:        spec.Inventory,
		effectiveAt:      spec.EffectiveAt.UTC(),
	}, nil
}

func (takeover TakeoverRecord) StopEvidence() string {
	return takeover.stopEvidence
}

func (takeover TakeoverRecord) Interval() AuthorityInterval {
	return takeover.interval
}

func (takeover TakeoverRecord) Inventory() InTransitInventory {
	return takeover.inventory
}

func (takeover TakeoverRecord) AcceptedFacts() string {
	return takeover.acceptedFacts
}

func (takeover TakeoverRecord) PendingExternals() string {
	return takeover.pendingExternals
}

func (takeover TakeoverRecord) ActualControl() string {
	return takeover.actualControl
}

func (takeover TakeoverRecord) Responsibilities() string {
	return takeover.responsibilities
}

func (takeover TakeoverRecord) NextAction() string {
	return takeover.nextAction
}

func (takeover TakeoverRecord) EffectiveAt() time.Time {
	return takeover.effectiveAt
}
