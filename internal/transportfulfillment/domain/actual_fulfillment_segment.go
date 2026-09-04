package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidFulfillmentSegment = errors.New("transport fulfillment: invalid fulfillment segment")
	// ErrSegmentNeedsAControlFact：段与参与只由有效收寄或`已交接`的权威交接成立/结束。
	// 拒收、待确认、计划、委托、订舱、分配或尝试都不是控制事实——没有该事实时它们各自
	// 存在，但立不起段（CONTEXT 生命周期节）。
	ErrSegmentNeedsAControlFact   = errors.New("transport fulfillment: a segment needs a control fact")
	ErrObjectAlreadyParticipating = errors.New("transport fulfillment: object already participates in the segment")
	ErrObjectNotParticipating     = errors.New("transport fulfillment: object has no active participation in the segment")
	// ErrSegmentStillActive：仍有有效参与关系时段结束不了——车辆故障、中断、失联或异常
	// 案件都不是控制结束事实，控制责任不因停止移动而消失（CONTEXT）。
	ErrSegmentStillActive = errors.New("transport fulfillment: the segment still has active participations")
	ErrSegmentClosed      = errors.New("transport fulfillment: the segment no longer accepts objects")
	// ErrNoParticipationToRederive：来源更正只能替代该对象在段内的**当前**参与，且被更正的版本
	// 必须恰是当前参与的入场依据（ADR-0112 决定二）。不是更正、更正的是前前版、或对象根本不在
	// 段内，都没有可替代的东西。
	ErrNoParticipationToRederive = errors.New("transport fulfillment: no current participation matches the corrected source version")
	// ErrCorrectionWithdrawsControl：更正后的来源不再表达控制转移（交接改判拒收/待确认），没有
	// 入场依据可立替代版本——那是参与失效，不是替代（ADR-0112 决定四，机制另票）。
	ErrCorrectionWithdrawsControl = errors.New("transport fulfillment: the correction withdraws the control fact and cannot rederive a participation")
)

// FulfillmentSegmentReference 指名一个实际履约段。
type FulfillmentSegmentReference struct{ requiredValue }

func NewFulfillmentSegmentReference(value string) (FulfillmentSegmentReference, error) {
	required, err := newRequiredValue("fulfillment segment reference", value)
	return FulfillmentSegmentReference{required}, err
}

// PlannedSegmentReference 指名对象自己关联的计划履约段。它可缺席——明确允许待路由的
// 产品在没有可行候选时照样实际揽收，计划段此刻不存在；任何关联都不得用实际事实覆盖
// 原计划（CONTEXT）。
type PlannedSegmentReference struct{ requiredValue }

func NewPlannedSegmentReference(value string) (PlannedSegmentReference, error) {
	required, err := newRequiredValue("planned segment reference", value)
	return PlannedSegmentReference{required}, err
}

// ParticipationBasisReference 指名参与起点或终点所依据的控制事实。
type ParticipationBasisReference struct{ requiredValue }

func NewParticipationBasisReference(value string) (ParticipationBasisReference, error) {
	required, err := newRequiredValue("participation basis reference", value)
	return ParticipationBasisReference{required}, err
}

// ParticipationEntryKind 是参与起点的封闭二来源：有效收寄（场外揽收）或`已交接`的
// 权威交接。刻意没有第三格——扫描、装载、订舱确认都立不起参与（CONTEXT 79）。
type ParticipationEntryKind uint8

const (
	ParticipationEntryKindInvalid ParticipationEntryKind = iota
	EnteredByOffsitePickup
	EnteredByTransportHandover
)

func (kind ParticipationEntryKind) String() string {
	switch kind {
	case EnteredByOffsitePickup:
		return "OFFSITE_PICKUP"
	case EnteredByTransportHandover:
		return "TRANSPORT_HANDOVER"
	default:
		return ""
	}
}

// ParticipationEndKind 是参与终点的封闭三来源：有效交付、下一次权威交接或明确控制
// 终止（CONTEXT 生命周期节③）。中断、折返、异常案件不在其中——它们不结束控制。
type ParticipationEndKind uint8

const (
	ParticipationEndKindInvalid ParticipationEndKind = iota
	EndedByEffectiveDelivery
	EndedByNextHandover
	EndedByControlTermination
)

func (kind ParticipationEndKind) String() string {
	switch kind {
	case EndedByEffectiveDelivery:
		return "EFFECTIVE_DELIVERY"
	case EndedByNextHandover:
		return "NEXT_HANDOVER"
	case EndedByControlTermination:
		return "CONTROL_TERMINATED"
	default:
		return ""
	}
}

// FulfillmentParticipation 是一个载运对象参与某实际履约段的对象级关系：保存关联的
// 计划履约段、实际控制起止、结果及依据（CONTEXT「履约参与关系」）。逐对象成立与结束，
// 整段结果覆盖不了成员差异——本类型就是那个「分别」。
//
// 来源更正引起的重派生在同段内形成替代参与版本（ADR-0112 决定一）：新版本以 supersedes
// 回指被替代参与的入场依据，原参与一字不动；同一对象在一段内的参与由此成一条链。superseded
// 是派生态不落列——聚合在重派生与重建时按回指关系标出，被回指的版本不再表达当前控制。
type FulfillmentParticipation struct {
	object     CarriedObjectReference
	planned    PlannedSegmentReference
	entryKind  ParticipationEntryKind
	entryBasis ParticipationBasisReference
	enteredAt  time.Time
	endKind    ParticipationEndKind
	endBasis   ParticipationBasisReference
	endedAt    time.Time
	supersedes ParticipationBasisReference
	superseded bool
}

// Supersedes 只在替代参与版本上给出：被替代参与的入场依据。首次入场答 false。
func (participation FulfillmentParticipation) Supersedes() (ParticipationBasisReference, bool) {
	return participation.supersedes, participation.supersedes.valid()
}

// Superseded 报告本版本是否已被后一版替代——它保留在历史里，但不再是该对象在段内的当前参与。
func (participation FulfillmentParticipation) Superseded() bool {
	return participation.superseded
}

func (participation FulfillmentParticipation) Object() CarriedObjectReference {
	return participation.object
}

// PlannedSegment 交回对象自己关联的计划段（若有）。
func (participation FulfillmentParticipation) PlannedSegment() (PlannedSegmentReference, bool) {
	if !participation.planned.valid() {
		return PlannedSegmentReference{}, false
	}
	return participation.planned, true
}

func (participation FulfillmentParticipation) EntryKind() ParticipationEntryKind {
	return participation.entryKind
}

func (participation FulfillmentParticipation) EntryBasis() ParticipationBasisReference {
	return participation.entryBasis
}

func (participation FulfillmentParticipation) EnteredAt() time.Time {
	return participation.enteredAt
}

// Active 报告本参与是否仍表达当前控制：没有离场，也没有被后一版替代。
func (participation FulfillmentParticipation) Active() bool {
	return participation.endedAt.IsZero() && !participation.superseded
}

// End 报告终点三件（种类、依据、时刻），只在已结束的参与上给出。被替代而未离场的版本不算
// 已结束——它没有终点，只是不再是当前。
func (participation FulfillmentParticipation) End() (ParticipationEndKind, ParticipationBasisReference, time.Time, bool) {
	if participation.endedAt.IsZero() {
		return ParticipationEndKindInvalid, ParticipationBasisReference{}, time.Time{}, false
	}
	return participation.endKind, participation.endBasis, participation.endedAt, true
}

// ActualFulfillmentSegment 是由有效收寄或权威交接事实形成的共同运输执行与控制责任
// 范围（CONTEXT「实际履约段」）。值类型：每次转换返回新值——已成立的段不存在「取消回
// 未开始」，类型上也没有任何取消或回退方法。
type ActualFulfillmentSegment struct {
	tenantID       TenantID
	segment        FulfillmentSegmentReference
	closed         bool
	closedAt       time.Time
	participations []FulfillmentParticipation
}

// EstablishSegmentWithPickup 由首个对象的有效收寄成立段（CONTEXT 生命周期①）。
func EstablishSegmentWithPickup(
	segment FulfillmentSegmentReference,
	pickup OffsitePickup,
	planned PlannedSegmentReference,
) (ActualFulfillmentSegment, error) {
	established := ActualFulfillmentSegment{tenantID: pickup.TenantID(), segment: segment}
	if !segment.valid() || !pickup.TenantID().valid() {
		return ActualFulfillmentSegment{}, ErrInvalidFulfillmentSegment
	}
	return established.JoinWithPickup(pickup, planned)
}

// EstablishSegmentWithHandover 由首个对象的`已交接`权威交接成立段。
func EstablishSegmentWithHandover(
	segment FulfillmentSegmentReference,
	handover TransportHandover,
	planned PlannedSegmentReference,
) (ActualFulfillmentSegment, error) {
	established := ActualFulfillmentSegment{tenantID: handover.TenantID(), segment: segment}
	if !segment.valid() || !handover.TenantID().valid() {
		return ActualFulfillmentSegment{}, ErrInvalidFulfillmentSegment
	}
	return established.JoinWithHandover(handover, planned)
}

func (segment ActualFulfillmentSegment) TenantID() TenantID {
	return segment.tenantID
}

func (segment ActualFulfillmentSegment) Segment() FulfillmentSegmentReference {
	return segment.segment
}

func (segment ActualFulfillmentSegment) Established() bool {
	return len(segment.participations) > 0
}

func (segment ActualFulfillmentSegment) Closed() bool {
	return segment.closed
}

func (segment ActualFulfillmentSegment) ClosedAt() (time.Time, bool) {
	if segment.closedAt.IsZero() {
		return time.Time{}, false
	}
	return segment.closedAt, true
}

func (segment ActualFulfillmentSegment) Participations() []FulfillmentParticipation {
	return append([]FulfillmentParticipation(nil), segment.participations...)
}

// ParticipationFor 答该对象在段内的**当前**参与：替代链的链尾（没有被后一版替代的那一条）。
// 历史各版由 ParticipationHistory 给。
func (segment ActualFulfillmentSegment) ParticipationFor(object CarriedObjectReference) (FulfillmentParticipation, bool) {
	for _, participation := range segment.participations {
		if participation.object == object && !participation.superseded {
			return participation, true
		}
	}
	return FulfillmentParticipation{}, false
}

// ParticipationHistory 交回该对象在段内的全部参与版本，按入场依据的替代顺序从首次入场到当前。
func (segment ActualFulfillmentSegment) ParticipationHistory(object CarriedObjectReference) []FulfillmentParticipation {
	byBasis := make(map[ParticipationBasisReference]FulfillmentParticipation)
	var root *FulfillmentParticipation
	for index := range segment.participations {
		participation := segment.participations[index]
		if participation.object != object {
			continue
		}
		byBasis[participation.entryBasis] = participation
		if !participation.supersedes.valid() {
			root = &segment.participations[index]
		}
	}
	if root == nil {
		return nil
	}
	successorOf := make(map[ParticipationBasisReference]FulfillmentParticipation, len(byBasis))
	for _, participation := range byBasis {
		if participation.supersedes.valid() {
			successorOf[participation.supersedes] = participation
		}
	}
	history := []FulfillmentParticipation{*root}
	for current := *root; ; {
		next, chained := successorOf[current.entryBasis]
		if !chained {
			return history
		}
		history = append(history, next)
		current = next
	}
}

func (segment ActualFulfillmentSegment) ActiveParticipations() int {
	active := 0
	for _, participation := range segment.participations {
		if participation.Active() {
			active++
		}
	}
	return active
}

// JoinWithPickup 让对象凭有效收寄加入段并形成自己的参与起点，不修改其他对象的起点
// （CONTEXT 生命周期②）。
func (segment ActualFulfillmentSegment) JoinWithPickup(
	pickup OffsitePickup,
	planned PlannedSegmentReference,
) (ActualFulfillmentSegment, error) {
	basis, err := NewParticipationBasisReference("OFFSITE-PICKUP/" + pickup.Version().String())
	if err != nil {
		return ActualFulfillmentSegment{}, ErrInvalidFulfillmentSegment
	}
	return segment.join(FulfillmentParticipation{
		object:     pickup.Object(),
		planned:    planned,
		entryKind:  EnteredByOffsitePickup,
		entryBasis: basis,
		enteredAt:  pickup.OccurredAt(),
	}, pickup.TenantID())
}

// JoinWithHandover 让对象凭`已交接`的权威交接加入段。拒收与待确认不转出控制，自然也
// 立不起参与——那正是 TransferOutBasis 只在已交接给出的原因。
func (segment ActualFulfillmentSegment) JoinWithHandover(
	handover TransportHandover,
	planned PlannedSegmentReference,
) (ActualFulfillmentSegment, error) {
	reference, transfers := handover.TransferOutBasis()
	if !transfers {
		return ActualFulfillmentSegment{}, ErrSegmentNeedsAControlFact
	}
	basis, err := NewParticipationBasisReference(reference)
	if err != nil {
		return ActualFulfillmentSegment{}, ErrInvalidFulfillmentSegment
	}
	return segment.join(FulfillmentParticipation{
		object:     handover.Object(),
		planned:    planned,
		entryKind:  EnteredByTransportHandover,
		entryBasis: basis,
		enteredAt:  handover.JudgedAt(),
	}, handover.TenantID())
}

func (segment ActualFulfillmentSegment) join(
	participation FulfillmentParticipation,
	tenant TenantID,
) (ActualFulfillmentSegment, error) {
	if !segment.segment.valid() || !segment.tenantID.valid() {
		return ActualFulfillmentSegment{}, ErrInvalidFulfillmentSegment
	}
	if segment.closed {
		return ActualFulfillmentSegment{}, ErrSegmentClosed
	}
	if tenant != segment.tenantID {
		return ActualFulfillmentSegment{}, ErrInvalidFulfillmentSegment
	}
	if participation.enteredAt.IsZero() || !participation.object.valid() {
		return ActualFulfillmentSegment{}, ErrInvalidFulfillmentSegment
	}
	// 同一实际控制范围不能因伙伴重投、任务重建或批量重试重复建立履约参与（UC-TF-002
	// 一致性）。已结束的参与也不重开：再次进入是新的段。
	if _, exists := segment.ParticipationFor(participation.object); exists {
		return ActualFulfillmentSegment{}, ErrObjectAlreadyParticipating
	}
	joined := segment
	joined.participations = append(append([]FulfillmentParticipation(nil), segment.participations...), participation)
	return joined, nil
}

// RederiveParticipationWithPickup 以更正后的揽收在同段内形成替代参与版本（ADR-0112 决定一、三）：
// 新版本回指被替代参与的入场依据，起点随更正后的发生时刻，计划段沿用；原参与一字不动但不再是
// 当前。段已关闭照样长版本——CONTEXT 封存例外格「除来源事实更正引起的重新派生」——段不重开。
func (segment ActualFulfillmentSegment) RederiveParticipationWithPickup(pickup OffsitePickup) (ActualFulfillmentSegment, error) {
	corrects, corrected := pickup.Corrects()
	if !corrected {
		return ActualFulfillmentSegment{}, ErrNoParticipationToRederive
	}
	replaced, err := NewParticipationBasisReference("OFFSITE-PICKUP/" + corrects.String())
	if err != nil {
		return ActualFulfillmentSegment{}, ErrInvalidFulfillmentSegment
	}
	basis, err := NewParticipationBasisReference("OFFSITE-PICKUP/" + pickup.Version().String())
	if err != nil {
		return ActualFulfillmentSegment{}, ErrInvalidFulfillmentSegment
	}
	return segment.rederive(pickup.TenantID(), pickup.Object(), EnteredByOffsitePickup, replaced, basis, pickup.OccurredAt())
}

// RederiveParticipationWithHandover 以更正后的`已交接`交接在同段内形成替代参与版本。更正若撤回了
// 控制转移（改成拒收或待确认），没有入场依据可立——那是失效格（ADR-0112 决定四），本方法如实拒，
// 不猜也不把它当替代。
func (segment ActualFulfillmentSegment) RederiveParticipationWithHandover(handover TransportHandover) (ActualFulfillmentSegment, error) {
	corrects, corrected := handover.Corrects()
	if !corrected {
		return ActualFulfillmentSegment{}, ErrNoParticipationToRederive
	}
	reference, transfers := handover.TransferOutBasis()
	if !transfers {
		return ActualFulfillmentSegment{}, ErrCorrectionWithdrawsControl
	}
	replaced, err := NewParticipationBasisReference("TRANSPORT-HANDOVER/" + corrects.String())
	if err != nil {
		return ActualFulfillmentSegment{}, ErrInvalidFulfillmentSegment
	}
	basis, err := NewParticipationBasisReference(reference)
	if err != nil {
		return ActualFulfillmentSegment{}, ErrInvalidFulfillmentSegment
	}
	return segment.rederive(handover.TenantID(), handover.Object(), EnteredByTransportHandover, replaced, basis, handover.JudgedAt())
}

// rederive 是两种来源共用的替代门。被替代的必须是该对象**当前**参与（链尾）且入场依据恰是被更正的
// 那一版——更正一个已被替代的前版是分叉，更正别的来源种类是另一件事，都拒。替代版本继承原参与的
// 离场三件：对象的控制终点是它自己的事实，更正入场不改它；更正后的起点晚于继承的终点即先结束再
// 进入，拒。
func (segment ActualFulfillmentSegment) rederive(
	tenant TenantID,
	object CarriedObjectReference,
	kind ParticipationEntryKind,
	replaced ParticipationBasisReference,
	basis ParticipationBasisReference,
	enteredAt time.Time,
) (ActualFulfillmentSegment, error) {
	if !segment.segment.valid() || !segment.tenantID.valid() || tenant != segment.tenantID || enteredAt.IsZero() {
		return ActualFulfillmentSegment{}, ErrInvalidFulfillmentSegment
	}
	current, present := segment.ParticipationFor(object)
	if !present || current.entryKind != kind || current.entryBasis != replaced {
		return ActualFulfillmentSegment{}, ErrNoParticipationToRederive
	}
	replacement := FulfillmentParticipation{
		object:     object,
		planned:    current.planned,
		entryKind:  kind,
		entryBasis: basis,
		enteredAt:  enteredAt.UTC(),
		supersedes: current.entryBasis,
	}
	if endKind, endBasis, endedAt, ended := current.End(); ended {
		if enteredAt.After(endedAt) {
			return ActualFulfillmentSegment{}, ErrInvalidFulfillmentSegment
		}
		replacement.endKind, replacement.endBasis, replacement.endedAt = endKind, endBasis, endedAt
	}
	rederived := segment
	rederived.participations = append([]FulfillmentParticipation(nil), segment.participations...)
	for index := range rederived.participations {
		if rederived.participations[index].object == object && rederived.participations[index].entryBasis == replaced {
			rederived.participations[index].superseded = true
		}
	}
	rederived.participations = append(rederived.participations, replacement)
	return rederived, nil
}

// EndParticipationWithDelivery 以有效交付结束该对象的参与（CONTEXT 生命周期③；有效
// 交付同时形成向收件方的控制转移）。
func (segment ActualFulfillmentSegment) EndParticipationWithDelivery(
	delivery EffectiveDelivery,
) (ActualFulfillmentSegment, error) {
	basis, err := NewParticipationBasisReference("EFFECTIVE-DELIVERY/" + delivery.Version().String())
	if err != nil {
		return ActualFulfillmentSegment{}, ErrInvalidFulfillmentSegment
	}
	return segment.end(delivery.Object(), EndedByEffectiveDelivery, basis, delivery.OccurredAt())
}

// EndParticipationWithNextHandover 以下一次`已交接`的权威交接结束该对象的参与——控制
// 移入下一段，本段对该对象的责任到此为止。
func (segment ActualFulfillmentSegment) EndParticipationWithNextHandover(
	handover TransportHandover,
) (ActualFulfillmentSegment, error) {
	reference, transfers := handover.TransferOutBasis()
	if !transfers {
		return ActualFulfillmentSegment{}, ErrSegmentNeedsAControlFact
	}
	basis, err := NewParticipationBasisReference(reference)
	if err != nil {
		return ActualFulfillmentSegment{}, ErrInvalidFulfillmentSegment
	}
	return segment.end(handover.Object(), EndedByNextHandover, basis, handover.JudgedAt())
}

// EndParticipationWithTermination 以明确控制终止结束参与。依据必备：没有有效控制结束
// 事实时，停止移动、异常案件或计划取消均不结束控制（CONTEXT）——不带依据的终止在
// 构造期就进不来。
func (segment ActualFulfillmentSegment) EndParticipationWithTermination(
	object CarriedObjectReference,
	basis ParticipationBasisReference,
	at time.Time,
) (ActualFulfillmentSegment, error) {
	if !basis.valid() {
		return ActualFulfillmentSegment{}, ErrInvalidFulfillmentSegment
	}
	return segment.end(object, EndedByControlTermination, basis, at)
}

func (segment ActualFulfillmentSegment) end(
	object CarriedObjectReference,
	kind ParticipationEndKind,
	basis ParticipationBasisReference,
	at time.Time,
) (ActualFulfillmentSegment, error) {
	if !object.valid() || at.IsZero() {
		return ActualFulfillmentSegment{}, ErrInvalidFulfillmentSegment
	}
	ended := segment
	ended.participations = append([]FulfillmentParticipation(nil), segment.participations...)
	for index := range ended.participations {
		participation := &ended.participations[index]
		// 只有当前参与（链尾）会被结束：被替代的版本留在历史里，它没有终点也不再表达控制。
		if participation.object != object || participation.superseded {
			continue
		}
		if !participation.Active() {
			// 已结束的参与不重复结束也不改写：更正走新的判断版本，不在这里覆盖。
			return ActualFulfillmentSegment{}, ErrObjectNotParticipating
		}
		if at.Before(participation.enteredAt) {
			return ActualFulfillmentSegment{}, ErrInvalidFulfillmentSegment
		}
		participation.endKind = kind
		participation.endBasis = basis
		participation.endedAt = at.UTC()
		return ended, nil
	}
	return ActualFulfillmentSegment{}, ErrObjectNotParticipating
}

// CloseSegment 声明段不再接受新对象并结束段。只有全部有效参与关系已经结束才关得上
// ——各对象可以带着不同结果收尾（CONTEXT 生命周期④），但一个仍在控制中的对象足以
// 让段继续存在。
//
// 同一条规则的时序面：关闭时刻不得早于任何参与的终点。早于它就等于说段在那个对象仍受控时
// 已经结束——与上一句矛盾。同刻允许：最后一个对象交出去的那一刻关段是正当的。
func (segment ActualFulfillmentSegment) CloseSegment(at time.Time) (ActualFulfillmentSegment, error) {
	if !segment.Established() || at.IsZero() {
		return ActualFulfillmentSegment{}, ErrInvalidFulfillmentSegment
	}
	if segment.closed {
		return ActualFulfillmentSegment{}, ErrSegmentClosed
	}
	if segment.ActiveParticipations() > 0 {
		return ActualFulfillmentSegment{}, ErrSegmentStillActive
	}
	if segment.closesBeforeAParticipationEnded(at) {
		return ActualFulfillmentSegment{}, ErrInvalidFulfillmentSegment
	}
	closed := segment
	closed.closed = true
	closed.closedAt = at.UTC()
	return closed, nil
}

// closesBeforeAParticipationEnded 只比对已结束参与的终点；在场的参与由 ActiveParticipations
// 那一道另判，这里不重复。
func (segment ActualFulfillmentSegment) closesBeforeAParticipationEnded(at time.Time) bool {
	for _, participation := range segment.participations {
		if _, _, endedAt, ended := participation.End(); ended && at.Before(endedAt) {
			return true
		}
	}
	return false
}
