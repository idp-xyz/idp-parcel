package domain

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// 本文件是出发门禁读数（ADR-0205）：一条出发落册时与它同一笔保存的「当时关务怎么答」。读数不挡事实——出发已经
// 发生，挡下是动作之前那一问的事；读数只把那一刻问来的答复如实记下，阻断也照记。
//
// 五格与逐对象那一格是门禁读数的共用内核（gate_reading.go），交出门禁读数也用它。本文件自己定义的是：问之前本上下文
// 这一侧停在了哪（段、班次、对象），以及一条出发上多个对象的答复怎样压成整条出发的一格。段里在控的集运单元按成员展开，
// 单元与成员各一格同在逐对象那几格里（consolidation_gate_finding.go，ADR-0225）；只有出发读数装得下它们。

var (
	ErrInvalidDepartureGateReading = errors.New("transport fulfillment: invalid departure gate reading")
	// ErrDepartureGateReadingMisplaced：出发没带读数、非出发带了读数，或读数与事实声明的段对不上。这是编排接错，
	// 不是登记方的输入有误——答「未受理」会让离线队列把一次实际发生的出发出队丢掉（ADR-0022），所以单独具名，
	// 编排据此上抛而不是答一格。
	ErrDepartureGateReadingMisplaced = errors.New("transport fulfillment: the departure gate reading does not belong to this movement fact")
)

// DepartureGateAnswer 与以下几个出发一侧的名字是共用内核（gate_reading.go）的别名：出发读数先落、名字先占，消费方
// （持久化、线格式、全程追踪的派生）照旧按这些名字读；同一封闭集只定义一处，出发的答复因此逐字不变。
type DepartureGateAnswer = GateAnswer

const (
	DepartureGateAnswerInvalid = GateAnswerInvalid
	DepartureGateNotGuarded    = GateNotGuarded
	DepartureGateAdmitted      = GateAdmitted
	DepartureGateUndecided     = GateUndecided
	DepartureGateUnreadable    = GateUnreadable
	DepartureGateBlocked       = GateBlocked
)

// DepartureGateStop 是本上下文这一侧没走到「问关务」那一步的原因。每种只属于一格（Answer）：答复由停点推出，
// 不另存一份可与停点对不上的结论。
type DepartureGateStop uint8

const (
	DepartureGateStopInvalid DepartureGateStop = iota

	// 未决：缺什么
	DepartureSegmentNotDeclared
	DepartureSegmentNotRegistered
	DepartureNoObjectUnderControl
	DepartureScheduleNotRegistered
	// DepartureRecordedBeforeGateReading 只出现在迁移前就已落册的出发上（迁移 0026 的存量回填）：那时没问过关务，
	// 如实答未决而不是猜一格。新登记的出发不会停在这里，构造门拒它。
	DepartureRecordedBeforeGateReading
	// DepartureSegmentEntryNodesUnknown：出发那一刻段里在控的某一件，说不出它是在哪个节点交进这一段、往哪个节点去——凭收寄进段、
	// 进段那次`已交接`没声明交出或去向节点，或那次交接在册上认不出。说不出就核不了班次是不是载着这一段（DepartureEntryStops）。
	DepartureSegmentEntryNodesUnknown
	// DepartureScheduleNotServingSegment：班次的起讫与段里在控各件交进来时声明的交出、去向节点对不上，这一班载的不是这一段。
	DepartureScheduleNotServingSegment

	// 读不动：哪一口
	DepartureSegmentRegistryUnavailable
	DepartureScheduleRegistryUnavailable
	// DepartureHandoverRegistryUnavailable：找段里各件进段那次交接时交接登记册读不动。
	DepartureHandoverRegistryUnavailable
)

func (stop DepartureGateStop) valid() bool {
	return stop >= DepartureSegmentNotDeclared && stop <= DepartureHandoverRegistryUnavailable
}

// Answer 是这种停点所属的那一格。
func (stop DepartureGateStop) Answer() DepartureGateAnswer {
	switch stop {
	case DepartureSegmentNotDeclared, DepartureSegmentNotRegistered, DepartureNoObjectUnderControl,
		DepartureScheduleNotRegistered, DepartureRecordedBeforeGateReading,
		DepartureSegmentEntryNodesUnknown, DepartureScheduleNotServingSegment:
		return DepartureGateUndecided
	case DepartureSegmentRegistryUnavailable, DepartureScheduleRegistryUnavailable, DepartureHandoverRegistryUnavailable:
		return DepartureGateUnreadable
	default:
		return DepartureGateAnswerInvalid
	}
}

func (stop DepartureGateStop) String() string {
	switch stop {
	case DepartureSegmentNotDeclared:
		return "SEGMENT_NOT_DECLARED"
	case DepartureSegmentNotRegistered:
		return "SEGMENT_NOT_REGISTERED"
	case DepartureNoObjectUnderControl:
		return "NO_OBJECT_UNDER_CONTROL"
	case DepartureScheduleNotRegistered:
		return "SCHEDULE_NOT_REGISTERED"
	case DepartureRecordedBeforeGateReading:
		return "RECORDED_BEFORE_GATE_READING"
	case DepartureSegmentEntryNodesUnknown:
		return "SEGMENT_ENTRY_NODES_UNKNOWN"
	case DepartureScheduleNotServingSegment:
		return "SCHEDULE_NOT_SERVING_SEGMENT"
	case DepartureSegmentRegistryUnavailable:
		return "SEGMENT_REGISTRY_UNAVAILABLE"
	case DepartureScheduleRegistryUnavailable:
		return "SCHEDULE_REGISTRY_UNAVAILABLE"
	case DepartureHandoverRegistryUnavailable:
		return "HANDOVER_REGISTRY_UNAVAILABLE"
	default:
		return ""
	}
}

// DepartureGateBasis、DepartureGateFinding 同为共用内核的别名，理由同 DepartureGateAnswer。
type (
	DepartureGateBasis   = GateBasis
	DepartureGateFinding = GateFinding
)

// NewDepartureGateFinding 立出发读数里一件对象的那一格；构造门在共用内核（NewGateFinding），立不起来的那一格在出发这一侧
// 认作读数不成形。
func NewDepartureGateFinding(
	object CarriedObjectReference,
	answer DepartureGateAnswer,
	judgement string,
	bases []DepartureGateBasis,
) (DepartureGateFinding, error) {
	finding, err := NewGateFinding(object, answer, judgement, bases)
	if err != nil {
		return DepartureGateFinding{}, fmt.Errorf("%w: %w", ErrInvalidDepartureGateReading, err)
	}
	return finding, nil
}

// DepartureGateReadingSpec 是一次读数的全部内容。停点与逐对象答复二者恰有其一：停在本上下文这一侧就没问关务，
// 问了关务就没有停点。起讫是问关务时那一份出发意图，取自班次方向；班次读不到时两格皆空。
type DepartureGateReadingSpec struct {
	From     NetworkNodeReference
	To       NetworkNodeReference
	Stops    []DepartureGateStop
	Findings []DepartureGateFinding
}

// DepartureGateReading 是一条出发的门禁读数。
type DepartureGateReading struct {
	answer   DepartureGateAnswer
	from     NetworkNodeReference
	to       NetworkNodeReference
	stops    []DepartureGateStop
	findings []DepartureGateFinding
}

// FormDepartureGateReading 形成一次新读数。存量回填那一种停点只由重建门收：新登记的出发都问过，不能声称没问。
func FormDepartureGateReading(spec DepartureGateReadingSpec) (DepartureGateReading, error) {
	if slices.Contains(spec.Stops, DepartureRecordedBeforeGateReading) {
		return DepartureGateReading{}, ErrInvalidDepartureGateReading
	}
	return formDepartureGateReading(spec)
}

// RehydrateDepartureGateReading 从库面重建读数，并核对存下的那一格正是停点与逐对象答复推出的那一格——整格另成
// 一列是为了按格查不必解 jsonb，代价是它可能与依据对不上；对不上的那一行不装回。
func RehydrateDepartureGateReading(spec DepartureGateReadingSpec, recorded DepartureGateAnswer) (DepartureGateReading, error) {
	reading, err := formDepartureGateReading(spec)
	if err != nil {
		return DepartureGateReading{}, err
	}
	if reading.answer != recorded {
		return DepartureGateReading{}, fmt.Errorf("%w: recorded %q, the stops and findings say %q",
			ErrInvalidDepartureGateReading, recorded, reading.answer)
	}
	return reading, nil
}

func formDepartureGateReading(spec DepartureGateReadingSpec) (DepartureGateReading, error) {
	if (len(spec.Stops) == 0) == (len(spec.Findings) == 0) {
		return DepartureGateReading{}, ErrInvalidDepartureGateReading
	}
	directed := spec.From.valid() || spec.To.valid()
	if directed && (!spec.From.valid() || !spec.To.valid() || spec.From == spec.To) {
		return DepartureGateReading{}, ErrInvalidDepartureGateReading
	}
	if len(spec.Findings) > 0 && !directed {
		// 问了关务就一定有起讫：关务的出发意图起讫必填，没有起讫的逐对象答复不是这次出发问来的。
		return DepartureGateReading{}, ErrInvalidDepartureGateReading
	}

	answer := DepartureGateAnswerInvalid
	stops := append([]DepartureGateStop(nil), spec.Stops...)
	slices.Sort(stops)
	for index, stop := range stops {
		if !stop.valid() || (index > 0 && stops[index-1] == stop) {
			return DepartureGateReading{}, ErrInvalidDepartureGateReading
		}
		answer = max(answer, stop.Answer())
	}
	if slices.Contains(stops, DepartureRecordedBeforeGateReading) && (len(stops) > 1 || directed) {
		// 存量回填只有这一个停点：那时什么也没读，不会同时知道段或班次怎样。
		return DepartureGateReading{}, ErrInvalidDepartureGateReading
	}

	findings := append([]DepartureGateFinding(nil), spec.Findings...)
	slices.SortFunc(findings, func(left, right DepartureGateFinding) int {
		return strings.Compare(left.object.String(), right.object.String())
	})
	for index, finding := range findings {
		if !finding.formed() {
			return DepartureGateReading{}, ErrInvalidDepartureGateReading
		}
		if index > 0 && findings[index-1].object == finding.object {
			return DepartureGateReading{}, ErrInvalidDepartureGateReading
		}
		answer = max(answer, finding.answer)
	}
	if err := checkConsolidationUnitFindings(findings); err != nil {
		return DepartureGateReading{}, fmt.Errorf("%w: %w", ErrInvalidDepartureGateReading, err)
	}
	return DepartureGateReading{
		answer:   answer,
		from:     spec.From,
		to:       spec.To,
		stops:    stops,
		findings: findings,
	}, nil
}

// formed 说这是不是一份读数；零值不是——出发缺读数与「读数答了某一格」必须分得开。
func (reading DepartureGateReading) formed() bool {
	return reading.answer.valid()
}

func (reading DepartureGateReading) Answer() DepartureGateAnswer { return reading.answer }

// Direction 交回问关务时那一份出发意图的起讫；班次没读到时答 false。
func (reading DepartureGateReading) Direction() (NetworkNodeReference, NetworkNodeReference, bool) {
	return reading.from, reading.to, reading.from.valid()
}

func (reading DepartureGateReading) Stops() []DepartureGateStop {
	return append([]DepartureGateStop(nil), reading.stops...)
}

func (reading DepartureGateReading) Findings() []DepartureGateFinding {
	return append([]DepartureGateFinding(nil), reading.findings...)
}

func (reading DepartureGateReading) recordedBeforeGateReading() bool {
	return slices.Contains(reading.stops, DepartureRecordedBeforeGateReading)
}

func (reading DepartureGateReading) segmentNotDeclared() bool {
	return slices.Contains(reading.stops, DepartureSegmentNotDeclared)
}

// ObjectsUnderControlAt 答出发那一刻在本段控制之下的对象：每个对象取链尾那一版参与（未被替代、未失效），且出发
// 时点落在它的控制起止之内。按出发时点而不按此刻取，是因为出发可以迟到——离线补登的那一条到达时，段里的对象可能
// 已经在下一站交接离场，按此刻取就问不到真正随车出发的那几件。不经装载分配推：分配是意图，拿它覆盖段里的实际
// 控制，就是 CONTEXT 禁止的「用计划覆盖事实」。
func (segment ActualFulfillmentSegment) ObjectsUnderControlAt(at time.Time) []CarriedObjectReference {
	participations := segment.ParticipationsUnderControlAt(at)
	objects := make([]CarriedObjectReference, 0, len(participations))
	for _, participation := range participations {
		objects = append(objects, participation.object)
	}
	return objects
}

// ParticipationsUnderControlAt 交回 ObjectsUnderControlAt 那几件各自在控的那一版参与，口径与次序都同它：要知道那几件是凭什么
// 进的这一段时用（出发读数核班次是不是载着这一段，DepartureEntryStops）。
func (segment ActualFulfillmentSegment) ParticipationsUnderControlAt(at time.Time) []FulfillmentParticipation {
	moment := AtRegistryPrecision(at)
	var inControl []FulfillmentParticipation
	for _, participation := range segment.participations {
		if participation.superseded || participation.voided {
			continue
		}
		if AtRegistryPrecision(participation.enteredAt).After(moment) {
			continue
		}
		if !participation.endedAt.IsZero() && !AtRegistryPrecision(participation.endedAt).After(moment) {
			continue
		}
		if !slices.ContainsFunc(inControl, func(kept FulfillmentParticipation) bool { return kept.object == participation.object }) {
			inControl = append(inControl, participation)
		}
	}
	slices.SortFunc(inControl, func(left, right FulfillmentParticipation) int {
		return strings.Compare(left.object.String(), right.object.String())
	})
	return inControl
}

// DepartureEntryStops 答这一班能不能替段里出发那一刻在控的这几件问出发意图（ADR-0237 决定五）。班次与段都是登记方或问询方
// 声明的：两样不互相核对，配一班别的方向的班次去问，关务就会按一份不属于这一车的意图作答——例如把一段境内段配上一班跨关务区域的
// 班次，段里不在案卷的每件都答未决、都回给设备。所以核的是段自己的事实：每件交进这一段时那次`已交接`声明的交出节点就是班次的起点、
// 去向节点就是班次的终点。
//
// entries 是认出来的那几次进段交接；unresolved 说有没有一件认不出（凭收寄进段，或那次交接在册上认不出）。说不出那两格答
// DepartureSegmentEntryNodesUnknown，声明了却与班次起讫对不上答 DepartureScheduleNotServingSegment，两样可以同在；都停在本侧、
// 不问关务。只核起讫节点、不核班次身份：同一方向换一班（改搭快班、顺延下一班）照样载得了这一段。
func DepartureEntryStops(direction ScheduleDirection, entries []TransportHandover, unresolved bool) []DepartureGateStop {
	unknown, mismatched := unresolved, false
	for _, entry := range entries {
		releasing, releasingDeclared := entry.ReleasingNode()
		onward, onwardDeclared := entry.OnwardNode()
		if !releasingDeclared || !onwardDeclared {
			unknown = true
			continue
		}
		if releasing != direction.From() || onward != direction.To() {
			mismatched = true
		}
	}
	var stops []DepartureGateStop
	if unknown {
		stops = append(stops, DepartureSegmentEntryNodesUnknown)
	}
	if mismatched {
		stops = append(stops, DepartureScheduleNotServingSegment)
	}
	return stops
}
