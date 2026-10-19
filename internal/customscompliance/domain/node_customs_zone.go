package domain

import (
	"errors"
	"fmt"
)

// 本文件是「节点关务区域」登记册与「动作意图 → 适用受门禁动作」的领域词形（ADR-0206）。所有权取 CONTEXT
// 所有权句：本上下文拥有合规候选区域与关务适用性判断；节点是谁归 network-routing，节点落在哪个关务区域、
// 两个节点之间算不算跨关务区域归本上下文。区域取值是租户取值（PAR-CUS-08），这里只有引用词形，不内置任何
// 国家或区域默认。

var (
	ErrInvalidActionIntent     = errors.New("customs compliance: invalid action intent")
	ErrNodeZoneReadingMissing  = errors.New("customs compliance: a node named by the intent has no zone reading")
	ErrNodeZoneReadingMismatch = errors.New("customs compliance: a zone reading names no node of the intent")
	ErrInvalidNodeZoneReading  = errors.New("customs compliance: invalid node zone reading")
)

// NetworkNodeReference 指名 network-routing 目录里的一个节点，按 NR 的节点代码拼写不透明保存。本上下文
// 不读节点目录、不判节点在不在册：节点不在 NR 册上与节点没登关务区域是两件事，前者不归这里答。
type NetworkNodeReference struct{ requiredValue }

func NewNetworkNodeReference(value string) (NetworkNodeReference, error) {
	required, err := newRequiredValue("network node reference", value)
	return NetworkNodeReference{required}, err
}

// CustomsZoneReference 指名一个关务区域。区域是引用不是封闭枚举：哪些区域、按什么粒度划，是租户按其
// 监管实情登的（PAR-CUS-08）；把区域写成 Go 常量就是替租户拟默认值。
type CustomsZoneReference struct{ requiredValue }

func NewCustomsZoneReference(value string) (CustomsZoneReference, error) {
	required, err := newRequiredValue("customs zone reference", value)
	return CustomsZoneReference{required}, err
}

// NodeZoneReading 是某节点在判断时点的关务区域读数。Registered=false 即那一刻没有在册版本——它既不是
// 「同区」也不是「异区」，跨区与否因此判不出（ADR-0206 决定二）。
type NodeZoneReading struct {
	Node       NetworkNodeReference
	Zone       CustomsZoneReference
	Registered bool
}

func (reading NodeZoneReading) valid() bool {
	if !reading.Node.valid() {
		return false
	}
	// 未登记的读数不得带区域：带了就说不清它到底登没登。
	return reading.Registered == reading.Zone.valid()
}

// ActionIntentKind 是消费方交来的动作意图种类。消费方只说「要做什么」，不说「该问哪几道门」：适用哪些
// 受门禁动作由本上下文按意图与节点区域定（ADR-0206 决定二）——让消费方逐动作问，就是把「是否跨关务
// 区域」的判断交给了不拥有它的一方。
type ActionIntentKind uint8

const (
	ActionIntentKindInvalid ActionIntentKind = iota
	DepartureIntent
	HandoverIntent
	DeliveryIntent
)

func (kind ActionIntentKind) valid() bool {
	return kind >= DepartureIntent && kind <= DeliveryIntent
}

func (kind ActionIntentKind) String() string {
	switch kind {
	case DepartureIntent:
		return "DEPARTURE"
	case HandoverIntent:
		return "HANDOVER"
	case DeliveryIntent:
		return "DELIVERY"
	default:
		return ""
	}
}

// ActionIntent 是一次动作意图：出发（自 A 到 B）、交出（在 A 交给他方，可带后续去向 B）、交付（在 A）。
// 交出只指交出方向——接收方那一侧的接收不受门禁约束（CONTEXT「不因此阻止接收」），所以没有「接收」意图。
type ActionIntent struct {
	kind   ActionIntentKind
	node   NetworkNodeReference
	onward NetworkNodeReference
}

// NewDepartureIntent 起讫两端都要：出发问不问跨区，取决于两端区域，缺一端就判不出。起讫同一节点不是
// 出发。
func NewDepartureIntent(from, to NetworkNodeReference) (ActionIntent, error) {
	if !from.valid() || !to.valid() || from == to {
		return ActionIntent{}, ErrInvalidActionIntent
	}
	return ActionIntent{kind: DepartureIntent, node: from, onward: to}, nil
}

// NewHandoverIntent 交出所在节点必填；后续去向可缺（零值即未给）——交给承运商时交出方常知道货去哪，
// 但「不知道」也必须说得出来，不能拿所在节点顶替去向（那会把跨境交出判成同区）。
func NewHandoverIntent(at, onward NetworkNodeReference) (ActionIntent, error) {
	if !at.valid() || onward == at {
		return ActionIntent{}, ErrInvalidActionIntent
	}
	if onward != (NetworkNodeReference{}) && !onward.valid() {
		return ActionIntent{}, ErrInvalidActionIntent
	}
	return ActionIntent{kind: HandoverIntent, node: at, onward: onward}, nil
}

func NewDeliveryIntent(at NetworkNodeReference) (ActionIntent, error) {
	if !at.valid() {
		return ActionIntent{}, ErrInvalidActionIntent
	}
	return ActionIntent{kind: DeliveryIntent, node: at}, nil
}

func (intent ActionIntent) Kind() ActionIntentKind { return intent.kind }

// Node 是出发起点、交出所在或交付所在。
func (intent ActionIntent) Node() NetworkNodeReference { return intent.node }

// Onward 是出发终点或交出后续去向；交付与未给去向的交出答 false。
func (intent ActionIntent) Onward() (NetworkNodeReference, bool) {
	return intent.onward, intent.onward.valid()
}

func (intent ActionIntent) valid() bool {
	if !intent.kind.valid() || !intent.node.valid() {
		return false
	}
	if intent.kind == DepartureIntent && !intent.onward.valid() {
		return false
	}
	return true
}

// ZoneNodes 是判断这一意图要读区域的节点：有起讫才问跨区，交付与未给去向的交出不读区域。
func (intent ActionIntent) ZoneNodes() []NetworkNodeReference {
	onward, ok := intent.Onward()
	if !ok || intent.kind == DeliveryIntent {
		return nil
	}
	return []NetworkNodeReference{intent.node, onward}
}

// CrossZoneState 是起讫两端是否跨关务区域的判断。
type CrossZoneState uint8

const (
	CrossZoneStateInvalid CrossZoneState = iota
	// CrossZoneNotAsked：意图本身没有起讫（交付；未给去向的交出）。
	CrossZoneNotAsked
	CrossZoneSame
	CrossZoneCrossing
	// CrossZoneUndetermined：起讫至少一端在判断时点未登记区域。
	CrossZoneUndetermined
)

func (state CrossZoneState) String() string {
	switch state {
	case CrossZoneNotAsked:
		return "NOT_ASKED"
	case CrossZoneSame:
		return "SAME_ZONE"
	case CrossZoneCrossing:
		return "CROSSING"
	case CrossZoneUndetermined:
		return "UNDETERMINED"
	default:
		return ""
	}
}

// ActionApplicability 是一次意图适用哪些受门禁动作。跨区待定时 CROSS_CUSTOMS_MOVEMENT 不在 Guarded 里——
// 它适不适用要等准入折叠看对象有没有跨区的门（ADR-0206 决定二），这里只如实交出「判不出」与缺哪几个节点。
type ActionApplicability struct {
	guarded      []GuardedAction
	crossZone    CrossZoneState
	unregistered []NetworkNodeReference
}

// Guarded 是肯定适用的动作。
func (applicability ActionApplicability) Guarded() []GuardedAction {
	return append([]GuardedAction(nil), applicability.guarded...)
}

func (applicability ActionApplicability) CrossZone() CrossZoneState { return applicability.crossZone }

// UnregisteredNodes 只在跨区待定时非空。
func (applicability ActionApplicability) UnregisteredNodes() []NetworkNodeReference {
	return append([]NetworkNodeReference(nil), applicability.unregistered...)
}

// DetermineGuardedActions 按意图定适用动作：出发对应 LOADING_DEPARTURE，交出对应 OUTBOUND_RELEASE，交付对应
// FINAL_DELIVERY；有起讫且两端区域不同时另加 CROSS_CUSTOMS_MOVEMENT。不采纳「每次出发都问两种」：那会让
// 同区段被只为跨境登的门挡住（ADR-0206 决定二）。
//
// readings 必须覆盖 intent.ZoneNodes() 的每个节点，且不得指名意图以外的节点——缺了或多了都是调用方编程
// 错误，不折成「未登记」：未登记是读回来的业务答案，没去读不是。
func DetermineGuardedActions(intent ActionIntent, readings []NodeZoneReading) (ActionApplicability, error) {
	if !intent.valid() {
		return ActionApplicability{}, ErrInvalidActionIntent
	}
	byNode := make(map[NetworkNodeReference]NodeZoneReading, len(readings))
	wanted := intent.ZoneNodes()
	for _, reading := range readings {
		if !reading.valid() {
			return ActionApplicability{}, ErrInvalidNodeZoneReading
		}
		named := false
		for _, node := range wanted {
			named = named || node == reading.Node
		}
		if !named {
			return ActionApplicability{}, fmt.Errorf("%w: %s", ErrNodeZoneReadingMismatch, reading.Node)
		}
		byNode[reading.Node] = reading
	}

	var primary GuardedAction
	switch intent.kind {
	case DepartureIntent:
		primary = LoadingDeparture
	case HandoverIntent:
		primary = OutboundRelease
	case DeliveryIntent:
		primary = FinalDelivery
	}
	applicability := ActionApplicability{guarded: []GuardedAction{primary}, crossZone: CrossZoneNotAsked}
	if len(wanted) == 0 {
		return applicability, nil
	}

	for _, node := range wanted {
		reading, ok := byNode[node]
		if !ok {
			return ActionApplicability{}, fmt.Errorf("%w: %s", ErrNodeZoneReadingMissing, node)
		}
		if !reading.Registered {
			applicability.unregistered = append(applicability.unregistered, node)
		}
	}
	switch {
	case len(applicability.unregistered) > 0:
		applicability.crossZone = CrossZoneUndetermined
	case byNode[wanted[0]].Zone == byNode[wanted[1]].Zone:
		applicability.crossZone = CrossZoneSame
	default:
		applicability.crossZone = CrossZoneCrossing
		applicability.guarded = append(applicability.guarded, CrossCustomsMovement)
	}
	return applicability, nil
}
