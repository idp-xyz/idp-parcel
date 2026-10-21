package domain

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// 本文件是门禁读数里集运单元自己那一格（ADR-0225）。段里在控的若是一袋，关务按包裹认对象、解析不到它，问它只会答出一格
// 假的干净读数；所以不问它，按成员逐件问，它自己那一格取成员里最重的一格。成员各自那一格与它同在一份读数里，它只指名它们。
//
// 本上下文这一侧问不到的成员不另立一格——它们没有可问关务的对象引用——而记成单元那一格的依据，每种缘由只属一格，与读数停点
// 同一纪律：答复由成员与缘由推出，不另存一份可与它们对不上的结论。

var ErrInvalidConsolidationUnitFinding = errors.New("transport fulfillment: invalid consolidation unit gate finding")

// ConsolidationMemberStop 是集运单元那一格里，本上下文这一侧没走到「问关务」的那部分成员的缘由。
type ConsolidationMemberStop uint8

const (
	ConsolidationMemberStopInvalid ConsolidationMemberStop = iota

	// 未决：缺什么
	// ConsolidationUnitWithoutMembers：出发那一刻成员关系册说这件单元不装任何东西。与段里没有在控对象同格：车上这一袋装了什么
	// 说不出来，不是「什么都不受门禁」。只在单元既没有成员那几格、也没有别的缘由时出现。
	ConsolidationUnitWithoutMembers
	// ConsolidationMemberIdentityUnresolved：成员的正式包裹身份此刻不成立（待识别、身份冲突、没有收寄记录），依据来源是那几件
	// 作业实物标识——没有包裹引用就没有可问关务的对象。
	ConsolidationMemberIdentityUnresolved

	// 读不动：哪一口
	// ConsolidationMemberIdentityUnreadable：成员的身份那一问读不动，依据来源是那几件作业实物标识。
	ConsolidationMemberIdentityUnreadable
	// ConsolidationMembershipCycle：嵌套成员已在这一条展开路径上——成员关系册在出发那一刻成了环（NO CONTEXT 不许）。依据来源是
	// 那件嵌套单元；它自己那一格在读数里照常有，这里只不再沿环回头展开。
	ConsolidationMembershipCycle
)

func (stop ConsolidationMemberStop) valid() bool {
	return stop >= ConsolidationUnitWithoutMembers && stop <= ConsolidationMembershipCycle
}

// Answer 是这种缘由所属的那一格。
func (stop ConsolidationMemberStop) Answer() GateAnswer {
	switch stop {
	case ConsolidationUnitWithoutMembers, ConsolidationMemberIdentityUnresolved:
		return GateUndecided
	case ConsolidationMemberIdentityUnreadable, ConsolidationMembershipCycle:
		return GateUnreadable
	default:
		return GateAnswerInvalid
	}
}

func (stop ConsolidationMemberStop) String() string {
	switch stop {
	case ConsolidationUnitWithoutMembers:
		return "CONSOLIDATION_UNIT_WITHOUT_MEMBERS"
	case ConsolidationMemberIdentityUnresolved:
		return "CONSOLIDATION_MEMBER_IDENTITY_UNRESOLVED"
	case ConsolidationMemberIdentityUnreadable:
		return "CONSOLIDATION_MEMBER_IDENTITY_UNREADABLE"
	case ConsolidationMembershipCycle:
		return "CONSOLIDATION_MEMBERSHIP_CYCLE"
	default:
		return ""
	}
}

func consolidationMemberStopNamed(cause string) ConsolidationMemberStop {
	for stop := ConsolidationUnitWithoutMembers; stop <= ConsolidationMembershipCycle; stop++ {
		if stop.String() == cause {
			return stop
		}
	}
	return ConsolidationMemberStopInvalid
}

// ConsolidationMemberGap 是一种缘由连同它涉及的那几件。
type ConsolidationMemberGap struct {
	Stop    ConsolidationMemberStop
	Sources []string
}

// FormConsolidationUnitGateFinding 立集运单元自己那一格：members 是它各直接成员在同一份读数里的那一格（包裹问来的、嵌套单元推出的，
// 或那一件读不动），gaps 是本侧问不到的成员。答复由领域推出：成员与缘由里最重的一格；两样都没有，即单元在那一刻不装任何东西，
// 由这里补上「单元无成员」那一条。
func FormConsolidationUnitGateFinding(
	unit CarriedObjectReference,
	members []GateFinding,
	gaps []ConsolidationMemberGap,
) (GateFinding, error) {
	references := make([]CarriedObjectReference, 0, len(members))
	answers := make([]GateAnswer, 0, len(members))
	for _, member := range members {
		if !member.formed() {
			return GateFinding{}, ErrInvalidConsolidationUnitFinding
		}
		references = append(references, member.object)
		answers = append(answers, member.answer)
	}
	bases := make([]GateBasis, 0, len(gaps)+1)
	for _, gap := range gaps {
		bases = append(bases, GateBasis{Cause: gap.Stop.String(), Sources: append([]string(nil), gap.Sources...)})
	}
	if len(members) == 0 && len(gaps) == 0 {
		bases = append(bases, GateBasis{Cause: ConsolidationUnitWithoutMembers.String()})
	}
	finding, err := consolidationUnitFinding(unit, references, bases)
	if err != nil {
		return GateFinding{}, err
	}
	finding.answer = consolidationUnitAnswer(answers, finding.bases)
	return finding, nil
}

// RehydrateConsolidationUnitGateFinding 从库面重建单元那一格，答复照存下的。它与成员那几格对不对得上，要连同成员才核得了，由各读数
// 的重建门在整份读数上核（checkConsolidationUnitFindings），这里只守形状。
func RehydrateConsolidationUnitGateFinding(
	unit CarriedObjectReference,
	recorded GateAnswer,
	members []CarriedObjectReference,
	bases []GateBasis,
) (GateFinding, error) {
	if !recorded.valid() {
		return GateFinding{}, ErrInvalidConsolidationUnitFinding
	}
	finding, err := consolidationUnitFinding(unit, members, bases)
	if err != nil {
		return GateFinding{}, err
	}
	finding.answer = recorded
	return finding, nil
}

// consolidationUnitFinding 守单元那一格的形状：成员定序、不重、不是它自己；依据只取本文件的缘由，每种至多一条、不带动作；
// 「单元无成员」只单独出现，其余缘由都要指名涉及的那几件。答复由调用方补。
func consolidationUnitFinding(
	unit CarriedObjectReference,
	members []CarriedObjectReference,
	bases []GateBasis,
) (GateFinding, error) {
	if !unit.valid() {
		return GateFinding{}, ErrInvalidConsolidationUnitFinding
	}
	sorted := append([]CarriedObjectReference(nil), members...)
	slices.SortFunc(sorted, func(left, right CarriedObjectReference) int {
		return strings.Compare(left.String(), right.String())
	})
	for index, member := range sorted {
		if !member.valid() || member == unit || (index > 0 && sorted[index-1] == member) {
			return GateFinding{}, ErrInvalidConsolidationUnitFinding
		}
	}
	copied := make([]GateBasis, 0, len(bases))
	seen := map[ConsolidationMemberStop]bool{}
	for _, basis := range bases {
		stop := consolidationMemberStopNamed(basis.Cause)
		if !stop.valid() || seen[stop] || basis.Action != "" {
			return GateFinding{}, ErrInvalidConsolidationUnitFinding
		}
		seen[stop] = true
		if (stop == ConsolidationUnitWithoutMembers) != (len(basis.Sources) == 0) {
			return GateFinding{}, ErrInvalidConsolidationUnitFinding
		}
		for _, source := range basis.Sources {
			if !storableBasisText(source, true) {
				return GateFinding{}, ErrInvalidConsolidationUnitFinding
			}
		}
		copied = append(copied, GateBasis{Cause: basis.Cause, Sources: append([]string(nil), basis.Sources...)})
	}
	if seen[ConsolidationUnitWithoutMembers] && (len(sorted) > 0 || len(copied) > 1) {
		return GateFinding{}, ErrInvalidConsolidationUnitFinding
	}
	if len(sorted) == 0 && len(copied) == 0 {
		return GateFinding{}, ErrInvalidConsolidationUnitFinding
	}
	slices.SortFunc(copied, func(left, right GateBasis) int { return strings.Compare(left.Cause, right.Cause) })
	return GateFinding{object: unit, unit: true, members: sorted, bases: copied}, nil
}

// consolidationUnitAnswer 是单元那一格的推法，只此一处：成员那几格与各缘由所属的格里取最重。次序就是 GateAnswer 的压制次序
// （ADR-0205 决定二），单元那一格不另立次序——它与整条出发取最重是同一件事，只是范围小一层。
func consolidationUnitAnswer(members []GateAnswer, bases []GateBasis) GateAnswer {
	answer := GateAnswerInvalid
	for _, member := range members {
		answer = max(answer, member)
	}
	for _, basis := range bases {
		answer = max(answer, consolidationMemberStopNamed(basis.Cause).Answer())
	}
	return answer
}

// checkConsolidationUnitFindings 在整份读数上核单元那几格：每件成员都在同一份读数里有一格，单元那一格正是成员与缘由推出的那一格，
// 成员关系连起来不成环。成环的读数说不清哪一格从哪一格推来，一条不收。
func checkConsolidationUnitFindings(findings []GateFinding) error {
	byObject := make(map[CarriedObjectReference]GateFinding, len(findings))
	for _, finding := range findings {
		byObject[finding.object] = finding
	}
	for _, finding := range findings {
		if !finding.unit {
			continue
		}
		answers := make([]GateAnswer, 0, len(finding.members))
		for _, member := range finding.members {
			cell, present := byObject[member]
			if !present {
				return fmt.Errorf("%w: unit %q names member %q that has no finding", ErrInvalidConsolidationUnitFinding, finding.object, member)
			}
			answers = append(answers, cell.answer)
		}
		if derived := consolidationUnitAnswer(answers, finding.bases); derived != finding.answer {
			return fmt.Errorf("%w: unit %q recorded %q, its members say %q", ErrInvalidConsolidationUnitFinding,
				finding.object, finding.answer, derived)
		}
	}
	const (
		unvisited = iota
		visiting
		done
	)
	state := make(map[CarriedObjectReference]int, len(findings))
	var visit func(object CarriedObjectReference) bool
	visit = func(object CarriedObjectReference) bool {
		switch state[object] {
		case visiting:
			return false
		case done:
			return true
		}
		state[object] = visiting
		for _, member := range byObject[object].members {
			if !visit(member) {
				return false
			}
		}
		state[object] = done
		return true
	}
	for _, finding := range findings {
		if !visit(finding.object) {
			return fmt.Errorf("%w: the units around %q contain each other", ErrInvalidConsolidationUnitFinding, finding.object)
		}
	}
	return nil
}
