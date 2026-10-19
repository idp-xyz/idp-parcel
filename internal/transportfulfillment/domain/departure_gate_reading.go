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
// 读数里逐对象的答复与依据都是 customs-compliance 的判断（ADR-0206）。本上下文只把它译成自己的五格并保存依据原文，
// 不重折任何一源：三源怎么折、跨不跨区、门禁满足为什么不等于放行，只在关务那边定义一处。本文件自己定义的是：问之前
// 本上下文这一侧停在了哪（段、班次、对象），以及一条出发上多个对象的答复怎样压成整条出发的一格。

var (
	ErrInvalidDepartureGateReading = errors.New("transport fulfillment: invalid departure gate reading")
	// ErrDepartureGateReadingMisplaced：出发没带读数、非出发带了读数，或读数与事实声明的段对不上。这是编排接错，
	// 不是登记方的输入有误——答「未受理」会让离线队列把一次实际发生的出发出队丢掉（ADR-0022），所以单独具名，
	// 编排据此上抛而不是答一格。
	ErrDepartureGateReadingMisplaced = errors.New("transport fulfillment: the departure gate reading does not belong to this movement fact")
)

// DepartureGateAnswer 是出发门禁读数的封闭各格。常量的取值次序就是一条出发上多个对象压成一格时的压制次序（大者压
// 小者），次序与理由只在 ADR-0205 决定二定义：改次序要先改那里，再挪这里的常量。关务的`未配置`不单成一格，同在决定二。
type DepartureGateAnswer uint8

const (
	DepartureGateAnswerInvalid DepartureGateAnswer = iota
	DepartureGateNotGuarded
	DepartureGateAdmitted
	DepartureGateUndecided
	DepartureGateUnreadable
	DepartureGateBlocked
)

func (answer DepartureGateAnswer) valid() bool {
	return answer >= DepartureGateNotGuarded && answer <= DepartureGateBlocked
}

func (answer DepartureGateAnswer) String() string {
	switch answer {
	case DepartureGateNotGuarded:
		return "NOT_GUARDED"
	case DepartureGateAdmitted:
		return "ADMITTED"
	case DepartureGateUndecided:
		return "UNDECIDED"
	case DepartureGateUnreadable:
		return "UNREADABLE"
	case DepartureGateBlocked:
		return "BLOCKED"
	default:
		return ""
	}
}

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

	// 读不动：哪一口
	DepartureSegmentRegistryUnavailable
	DepartureScheduleRegistryUnavailable
)

func (stop DepartureGateStop) valid() bool {
	return stop >= DepartureSegmentNotDeclared && stop <= DepartureScheduleRegistryUnavailable
}

// Answer 是这种停点所属的那一格。
func (stop DepartureGateStop) Answer() DepartureGateAnswer {
	switch stop {
	case DepartureSegmentNotDeclared, DepartureSegmentNotRegistered, DepartureNoObjectUnderControl,
		DepartureScheduleNotRegistered, DepartureRecordedBeforeGateReading:
		return DepartureGateUndecided
	case DepartureSegmentRegistryUnavailable, DepartureScheduleRegistryUnavailable:
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
	case DepartureSegmentRegistryUnavailable:
		return "SEGMENT_REGISTRY_UNAVAILABLE"
	case DepartureScheduleRegistryUnavailable:
		return "SCHEDULE_REGISTRY_UNAVAILABLE"
	default:
		return ""
	}
}

// DepartureGateBasis 是关务交回的一条依据：适用的受门禁动作、依据种类与来源，原文保存——词汇属 customs-compliance，
// 本上下文不解释、不重排。读不动时没有动作与来源，种类指名读不动的是哪一口。
type DepartureGateBasis struct {
	Action  string
	Cause   string
	Sources []string
}

// DepartureGateFinding 是一件载运对象的那一格：关务对它这次出发的答复译成的五格之一、关务判断标识与同格依据。
// 标识只在关务形成了判断时有——读不动时关务那边什么也没记下，指不回去。
type DepartureGateFinding struct {
	object    CarriedObjectReference
	answer    DepartureGateAnswer
	judgement string
	bases     []DepartureGateBasis
}

// NewDepartureGateFinding 立一件对象的那一格。依据至少一条：没有依据的答复读者无从核对，与关务「答复只列同格
// 依据」那条同一纪律。原文里不许有 NUL——库面 jsonb 存不下，存不下的值在这里拒，不等到落库那一刻。
func NewDepartureGateFinding(
	object CarriedObjectReference,
	answer DepartureGateAnswer,
	judgement string,
	bases []DepartureGateBasis,
) (DepartureGateFinding, error) {
	if !object.valid() || !answer.valid() || len(bases) == 0 {
		return DepartureGateFinding{}, ErrInvalidDepartureGateReading
	}
	judged := strings.TrimSpace(judgement) != ""
	if judged == (answer == DepartureGateUnreadable) || strings.IndexByte(judgement, 0) >= 0 {
		return DepartureGateFinding{}, ErrInvalidDepartureGateReading
	}
	copied := make([]DepartureGateBasis, 0, len(bases))
	for _, basis := range bases {
		if !storableBasisText(basis.Cause, true) || !storableBasisText(basis.Action, false) {
			return DepartureGateFinding{}, ErrInvalidDepartureGateReading
		}
		for _, source := range basis.Sources {
			if !storableBasisText(source, true) {
				return DepartureGateFinding{}, ErrInvalidDepartureGateReading
			}
		}
		basis.Sources = append([]string(nil), basis.Sources...)
		copied = append(copied, basis)
	}
	return DepartureGateFinding{object: object, answer: answer, judgement: judgement, bases: copied}, nil
}

func storableBasisText(value string, required bool) bool {
	if strings.IndexByte(value, 0) >= 0 {
		return false
	}
	return !required || strings.TrimSpace(value) != ""
}

func (finding DepartureGateFinding) Object() CarriedObjectReference { return finding.object }
func (finding DepartureGateFinding) Answer() DepartureGateAnswer    { return finding.answer }

// Judgement 交回关务判断标识；读不动时答 false。
func (finding DepartureGateFinding) Judgement() (string, bool) {
	return finding.judgement, finding.judgement != ""
}

func (finding DepartureGateFinding) Bases() []DepartureGateBasis {
	bases := make([]DepartureGateBasis, 0, len(finding.bases))
	for _, basis := range finding.bases {
		basis.Sources = append([]string(nil), basis.Sources...)
		bases = append(bases, basis)
	}
	return bases
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
		if !finding.object.valid() || !finding.answer.valid() || len(finding.bases) == 0 {
			return DepartureGateReading{}, ErrInvalidDepartureGateReading
		}
		if index > 0 && findings[index-1].object == finding.object {
			return DepartureGateReading{}, ErrInvalidDepartureGateReading
		}
		answer = max(answer, finding.answer)
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
	moment := AtRegistryPrecision(at)
	var objects []CarriedObjectReference
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
		if !slices.Contains(objects, participation.object) {
			objects = append(objects, participation.object)
		}
	}
	slices.SortFunc(objects, func(left, right CarriedObjectReference) int {
		return strings.Compare(left.String(), right.String())
	})
	return objects
}
