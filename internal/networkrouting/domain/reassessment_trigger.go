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

// ControlEvidenceKind 是触发所携控制依据的封闭三值。前两格是权威控制依据（节点收寄结果、
// transport-fulfillment 的权威运输交接结果），第三格是来源线索——普通扫描、位置消息、
// 装卸或车辆到场。分类维度是「能不能据以认定控制权」，不是消息从哪个系统来。
type ControlEvidenceKind uint8

const (
	ControlEvidenceKindInvalid ControlEvidenceKind = iota
	NodeIntakeControl
	TransportHandoverControl
	SourceHint
)

func (kind ControlEvidenceKind) valid() bool {
	return kind >= NodeIntakeControl && kind <= SourceHint
}

func (kind ControlEvidenceKind) String() string {
	switch kind {
	case NodeIntakeControl:
		return "NODE_INTAKE_CONTROL"
	case TransportHandoverControl:
		return "TRANSPORT_HANDOVER_CONTROL"
	case SourceHint:
		return "SOURCE_HINT"
	default:
		return ""
	}
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
