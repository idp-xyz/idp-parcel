package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidReassessmentTrigger = errors.New("network routing: invalid reassessment trigger")
	// ErrSourceHintDoesNotTrigger 是 CONTEXT「当前可控节点」硬句的落点：不得仅凭消息到达
	// 顺序、扫描、位置、装载或卸载自行认定控制权——线索可以保全，但立不成一次复核触发。
	ErrSourceHintDoesNotTrigger = errors.New("network routing: a source hint does not establish control or trigger reassessment")
)

// ControlEvidenceKind 是触发所携控制依据的封闭集合。节点收寄、权威运输交接与场外揽收是权威控制依据（节点收寄结果、
// transport-fulfillment 的交接结果与揽收结果），SourceHint 是来源线索——普通扫描、位置消息、装卸或车辆到场。分类维度
// 是「能不能据以认定控制权」，不是消息从哪个系统来。
//
// 场外揽收与交进接收节点的权威运输交接同出 transport-fulfillment，却不能合成一格：揽收把包裹交到运输方手里，没有
// 节点能从那里施加下一步作业，揽收地点也是客户场所一类、不是节点码（ADR-0249）。合成一格，复核就会拿客户场所去比
// 计划节点，并把揽收当成可控节点去切前缀、判冻结、自动改路。
type ControlEvidenceKind uint8

const (
	ControlEvidenceKindInvalid ControlEvidenceKind = iota
	NodeIntakeControl
	TransportHandoverControl
	SourceHint
	OffsitePickupControl
)

func (kind ControlEvidenceKind) valid() bool {
	return kind >= NodeIntakeControl && kind <= OffsitePickupControl
}

func (kind ControlEvidenceKind) String() string {
	switch kind {
	case NodeIntakeControl:
		return "NODE_INTAKE_CONTROL"
	case TransportHandoverControl:
		return "TRANSPORT_HANDOVER_CONTROL"
	case SourceHint:
		return "SOURCE_HINT"
	case OffsitePickupControl:
		return "OFFSITE_PICKUP_CONTROL"
	default:
		return ""
	}
}

// EstablishesControllableNode 答这一格控制依据是否建立当前可控节点（CONTEXT「当前可控节点」）：只有节点收寄与把包裹
// 交进声明的接收节点的权威运输交接。也只有这两格的位置是节点码。凡问「包裹在不在节点上」的判断都问这一处，不另列名单。
// 场外揽收照复核却不建立（ADR-0249 决定二）；线索连触发都立不成。
func (kind ControlEvidenceKind) EstablishesControllableNode() bool {
	return kind == NodeIntakeControl || kind == TransportHandoverControl
}

// ActualLocationReference 指名实际接货位置或当前节点。位置事实属 node-operations 与
// transport-fulfillment，这里只引用。
type ActualLocationReference struct{ requiredValue }

func NewActualLocationReference(value string) (ActualLocationReference, error) {
	required, err := newRequiredValue("actual location reference", value)
	return ActualLocationReference{required}, err
}

// SourceFactVersionReference 指名触发所依据的来源事实版本。同一触发身份携带不同事实
// 版本是冲突（结果语义契约），比对锚就是它。
type SourceFactVersionReference struct{ requiredValue }

func NewSourceFactVersionReference(value string) (SourceFactVersionReference, error) {
	required, err := newRequiredValue("source fact version reference", value)
	return SourceFactVersionReference{required}, err
}

// ReassessmentTriggerSpec 是一次复核触发所需的全部输入。
type ReassessmentTriggerSpec struct {
	Correlation   RequestCorrelationID
	Key           InitialRouteJudgmentKey
	Control       ControlEvidenceKind
	Location      ActualLocationReference
	SourceVersion SourceFactVersionReference
	OccurredAt    time.Time
}

// ReassessmentTrigger 是 UC-NR-003 复核层次 1 已经验证过的触发：权威控制依据、实际位置、
// 来源版本与业务时间齐备。来源到达不等于来源已被业务接受——验证在构造期完成，编排拿到
// 的触发都已立得住。
type ReassessmentTrigger struct {
	correlation   RequestCorrelationID
	key           InitialRouteJudgmentKey
	control       ControlEvidenceKind
	location      ActualLocationReference
	sourceVersion SourceFactVersionReference
	occurredAt    time.Time
}

// NewReassessmentTrigger 拒绝两类输入：缺件的（位置、版本、时间齐备才可关联）与线索级
// 控制依据的——后者用自己的哨兵，让接入层能把「不是触发」与「触发坏了」分开处置：线索
// 该保全等权威依据，坏触发该修装配。
func NewReassessmentTrigger(spec ReassessmentTriggerSpec) (ReassessmentTrigger, error) {
	if !spec.Control.valid() {
		return ReassessmentTrigger{}, ErrInvalidReassessmentTrigger
	}
	if spec.Control == SourceHint {
		return ReassessmentTrigger{}, ErrSourceHintDoesNotTrigger
	}
	if !spec.Correlation.valid() ||
		!spec.Key.MinimumIdentityEstablished() ||
		!spec.Location.valid() ||
		!spec.SourceVersion.valid() ||
		spec.OccurredAt.IsZero() {
		return ReassessmentTrigger{}, ErrInvalidReassessmentTrigger
	}
	return ReassessmentTrigger{
		correlation:   spec.Correlation,
		key:           spec.Key,
		control:       spec.Control,
		location:      spec.Location,
		sourceVersion: spec.SourceVersion,
		occurredAt:    spec.OccurredAt.UTC(),
	}, nil
}

func (trigger ReassessmentTrigger) Correlation() RequestCorrelationID {
	return trigger.correlation
}

func (trigger ReassessmentTrigger) Key() InitialRouteJudgmentKey {
	return trigger.key
}

func (trigger ReassessmentTrigger) Control() ControlEvidenceKind {
	return trigger.control
}

func (trigger ReassessmentTrigger) Location() ActualLocationReference {
	return trigger.location
}

func (trigger ReassessmentTrigger) SourceVersion() SourceFactVersionReference {
	return trigger.sourceVersion
}

func (trigger ReassessmentTrigger) OccurredAt() time.Time {
	return trigger.occurredAt
}

// PrecedesPosition 答这次触发是否早于路由历史里最近一次已判控制触发的业务时间（ADR-0235）。CONTEXT 要求按业务时间
// 裁决、不按消息到达顺序：收寄采用与交接分属不同分区，较早的那次可能在较新的复核之后才到，再判一次就是据一个已经
// 过去的位置失效或改路。同一时刻不算更早：两次控制落在同一时刻时没有先后可比，照判，不替它们排一个没有业务依据
// 的次序。
func (trigger ReassessmentTrigger) PrecedesPosition(positionAt time.Time) bool {
	return trigger.occurredAt.Before(positionAt)
}
