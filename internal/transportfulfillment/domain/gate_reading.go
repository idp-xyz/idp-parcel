package domain

import (
	"errors"
	"strings"
)

// 本文件是门禁读数的共用内核：一件对象上关务怎么答（五格之一）、凭什么答（依据原文）、关务判断标识。出发门禁读数
// （ADR-0205）与交出门禁读数（ADR-0213 决定四）同一套封闭集、同一种逐对象那一格；各自另有的只是「问之前本上下文这一
// 侧停在了哪」与「问的是哪一份意图」，那两样留在各自的文件里。
//
// 读数里逐对象的答复与依据都是 customs-compliance 的判断（ADR-0206）。本上下文只把它译成自己的五格并保存依据原文，
// 不重折任何一源：三源怎么折、跨不跨区、门禁满足为什么不等于放行，只在关务那边定义一处。

// ErrInvalidGateFinding：逐对象那一格立不起来。出发与交出两侧各把它包进自己的读数错误里交出。
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
type GateFinding struct {
	object    CarriedObjectReference
	answer    GateAnswer
	judgement string
	bases     []GateBasis
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

// formed 说这是不是一格；零值不是——读数里「没有逐对象那一格」与「那一格答了某格」必须分得开。
func (finding GateFinding) formed() bool {
	return finding.object.valid() && finding.answer.valid() && len(finding.bases) > 0
}

func (finding GateFinding) Object() CarriedObjectReference { return finding.object }
func (finding GateFinding) Answer() GateAnswer             { return finding.answer }

// Judgement 交回关务判断标识；读不动时答 false。
func (finding GateFinding) Judgement() (string, bool) {
	return finding.judgement, finding.judgement != ""
}

func (finding GateFinding) Bases() []GateBasis {
	bases := make([]GateBasis, 0, len(finding.bases))
	for _, basis := range finding.bases {
		basis.Sources = append([]string(nil), basis.Sources...)
		bases = append(bases, basis)
	}
	return bases
}
