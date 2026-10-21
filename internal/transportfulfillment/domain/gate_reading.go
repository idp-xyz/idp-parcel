package domain

import (
	"errors"
	"strings"
)

// 本文件是门禁读数的共用内核：一件对象上关务怎么答（五格之一）、凭什么答（依据原文）、关务判断标识。各门禁读数共用
// 同一套封闭集、同一种逐对象那一格（五格与压制次序的权威在 ADR-0205）；各自另有的只是「问之前本上下文这一侧停在了哪」
// 与「问的是哪一份意图」，那两样留在各自的文件里，哪一种读数用了本内核也由那一种自己的文件说，这里不列。
//
// 读数里逐对象的答复与依据都是 customs-compliance 的判断（ADR-0206）。本上下文只把它译成自己的五格并保存依据原文，
// 不重折任何一源：三源怎么折、跨不跨区、门禁满足为什么不等于放行，只在关务那边定义一处。

// ErrInvalidGateFinding：逐对象那一格立不起来。各门禁读数把它包进自己的读数错误里交出。
var ErrInvalidGateFinding = errors.New("transport fulfillment: invalid gate finding")

// GateAnswer 是门禁读数的封闭各格。常量的取值次序就是多个对象（或停点）压成一格时的压制次序（大者压小者），次序与
// 理由只在 ADR-0205 决定二定义：改次序要先改那里，再挪这里的常量。关务的`未配置`不单成一格，同在决定二。
type GateAnswer uint8

const (
	GateAnswerInvalid GateAnswer = iota
	GateNotGuarded
	GateAdmitted
	GateUndecided
	GateUnreadable
	GateBlocked
)

func (answer GateAnswer) valid() bool {
	return answer >= GateNotGuarded && answer <= GateBlocked
}

func (answer GateAnswer) String() string {
	switch answer {
	case GateNotGuarded:
		return "NOT_GUARDED"
	case GateAdmitted:
		return "ADMITTED"
	case GateUndecided:
		return "UNDECIDED"
	case GateUnreadable:
		return "UNREADABLE"
	case GateBlocked:
		return "BLOCKED"
	default:
		return ""
	}
}

// GateBasis 是关务交回的一条依据：适用的受门禁动作、依据种类与来源，原文保存——词汇属 customs-compliance，
// 本上下文不解释、不重排。读不动时没有动作与来源，种类指名读不动的是哪一口。
type GateBasis struct {
	Action  string
	Cause   string
	Sources []string
}

// GateFinding 是一件载运对象的那一格：关务对它这一次动作意图的答复译成的五格之一、关务判断标识与同格依据。
// 标识只在关务形成了判断时有——读不动时关务那边什么也没记下，指不回去。
//
// 集运单元自己那一格（unit 为真）不是问关务问来的：关务按包裹认对象，问它只会答出一格假的干净读数。那一格由同一份读数里
// 各直接成员那几格与本侧问不到的成员推出，构造与推法在 consolidation_gate_finding.go。
type GateFinding struct {
	object    CarriedObjectReference
	answer    GateAnswer
	judgement string
	bases     []GateBasis
	unit      bool
	members   []CarriedObjectReference
}

// NewGateFinding 立一件对象的那一格。依据至少一条：没有依据的答复读者无从核对，与关务「答复只列同格依据」那条同一
// 纪律。原文里不许有 NUL——库面 jsonb 存不下，存不下的值在这里拒，不等到落库那一刻。
func NewGateFinding(
	object CarriedObjectReference,
	answer GateAnswer,
	judgement string,
	bases []GateBasis,
) (GateFinding, error) {
	if !object.valid() || !answer.valid() || len(bases) == 0 {
		return GateFinding{}, ErrInvalidGateFinding
	}
	judged := strings.TrimSpace(judgement) != ""
	if judged == (answer == GateUnreadable) || strings.IndexByte(judgement, 0) >= 0 {
		return GateFinding{}, ErrInvalidGateFinding
	}
	copied := make([]GateBasis, 0, len(bases))
	for _, basis := range bases {
		if !storableBasisText(basis.Cause, true) || !storableBasisText(basis.Action, false) {
			return GateFinding{}, ErrInvalidGateFinding
		}
		for _, source := range basis.Sources {
			if !storableBasisText(source, true) {
				return GateFinding{}, ErrInvalidGateFinding
			}
		}
		basis.Sources = append([]string(nil), basis.Sources...)
		copied = append(copied, basis)
	}
	return GateFinding{object: object, answer: answer, judgement: judgement, bases: copied}, nil
}

func storableBasisText(value string, required bool) bool {
	if strings.IndexByte(value, 0) >= 0 {
		return false
	}
	return !required || strings.TrimSpace(value) != ""
}

// formed 说这是不是一格；零值不是——读数里「没有逐对象那一格」与「那一格答了某格」必须分得开。集运单元那一格的依据可以只是
// 它的成员（成员那几格就是它的依据），所以依据与成员不能都空。
func (finding GateFinding) formed() bool {
	return finding.object.valid() && finding.answer.valid() && len(finding.bases)+len(finding.members) > 0
}

func (finding GateFinding) Object() CarriedObjectReference { return finding.object }
func (finding GateFinding) Answer() GateAnswer             { return finding.answer }

// Judgement 交回关务判断标识；读不动时答 false。
func (finding GateFinding) Judgement() (string, bool) {
	return finding.judgement, finding.judgement != ""
}

// ConsolidationUnit 说这一格是不是集运单元自己那一格；是的话交回它的直接成员里在同一份读数里各有一格的那几件（定序）。
func (finding GateFinding) ConsolidationUnit() ([]CarriedObjectReference, bool) {
	return append([]CarriedObjectReference(nil), finding.members...), finding.unit
}

func (finding GateFinding) Bases() []GateBasis {
	bases := make([]GateBasis, 0, len(finding.bases))
	for _, basis := range finding.bases {
		basis.Sources = append([]string(nil), basis.Sources...)
		bases = append(bases, basis)
	}
	return bases
}

// ExecutorGateCell 是门禁读数回给执行方的逐对象那一格（ADR-0237）：对象与它的格；集运单元那一格另带它的直接成员里同样回给执行方
// 的那几件。它是读数的投影，不是一格读数：关务判断标识与依据原文不在里面——那些只随事实落册（ADR-0205 决定一）——单元的成员
// 也只剩同样回给执行方的那几件，所以不能拿它推回单元那一格。
type ExecutorGateCell struct {
	Object  CarriedObjectReference
	Answer  GateAnswer
	Unit    bool
	Members []CarriedObjectReference
}

// ExecutorGateCells 是「执行方看得到哪几格」的唯一判法（ADR-0237）：各门禁读数回给设备的逐对象那一格都只从这里取，读数本身
// 与落册的那一份照旧记全。
//
// 只列没得出放行的对象：阻断、读不动、未决。放行与不受门禁的那几件执行方不必动它们，列出来只会把同车别的客户的包裹交给一台
// 未必运着它们的设备——问询不按作业范围限段（ADR-0219 越权风险点 5）。读不动与未决同列，是因为执行方对它们要做的事与阻断
// 一样是先把那一件拿下或转人工，整格是读不动时说不出是哪一件，设备就只能整车不走。
//
// 集运单元那一格的格是成员里最重的一格（ADR-0225 决定三），成员没得出放行，单元也就没得出放行：列出的成员总能沿单元一路找到
// 段里在控的那一件，执行方据此知道被挡的那一件装在哪一袋里。单元那一格只指名同样列出的成员，放行的成员不随单元带出去；
// 单元因本侧问不到的成员（身份不成立、读不动、成环）或空袋而没得出放行时，它在列、成员为空。
//
// 次序照读数里逐对象那几格的次序，不另排。
func ExecutorGateCells(findings []GateFinding) []ExecutorGateCell {
	shown := make(map[CarriedObjectReference]bool, len(findings))
	for _, finding := range findings {
		if !finding.answer.passes() {
			shown[finding.object] = true
		}
	}
	cells := make([]ExecutorGateCell, 0, len(shown))
	for _, finding := range findings {
		if !shown[finding.object] {
			continue
		}
		cell := ExecutorGateCell{Object: finding.object, Answer: finding.answer, Unit: finding.unit}
		for _, member := range finding.members {
			if shown[member] {
				cell.Members = append(cell.Members, member)
			}
		}
		cells = append(cells, cell)
	}
	return cells
}

// passes 说这一格是不是得出了放行：放行与不受门禁。显式列出两格而不按压制次序比大小——次序是压格用的（ADR-0205 决定二），
// 哪几格算得出放行是另一个问题，次序日后若挪，这一问不该跟着变。
func (answer GateAnswer) passes() bool {
	return answer == GateAdmitted || answer == GateNotGuarded
}
