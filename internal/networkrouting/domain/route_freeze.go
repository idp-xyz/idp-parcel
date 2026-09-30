package domain

import "fmt"

// ErrUnknownFreezeForm 说明形态词不在封闭族里，或已声明形态的取值不成立。
var ErrUnknownFreezeForm = fmt.Errorf("network routing: unknown freeze form")

// FreezeForm 是路由策略版本声明的冻结边界形态。未声明不是「未冻结」。
type FreezeForm uint8

const (
	FreezeFormUndeclared FreezeForm = iota
	// RemainingSegmentCountFreeze 在剩余计划段数不超过已登记的段数时越过冻结边界。
	// 段数是租户取值，不在这里预填。
	RemainingSegmentCountFreeze
)

func (form FreezeForm) String() string {
	switch form {
	case RemainingSegmentCountFreeze:
		return "REMAINING_SEGMENT_COUNT"
	default:
		return ""
	}
}

func FreezeFormFrom(raw string) (FreezeForm, error) {
	switch raw {
	case "REMAINING_SEGMENT_COUNT":
		return RemainingSegmentCountFreeze, nil
	default:
		return FreezeFormUndeclared, fmt.Errorf("%w: %q", ErrUnknownFreezeForm, raw)
	}
}

// FreezeJudgment 是一次冻结判断。未配置时 Frozen 没有意义，不能读成未冻结。
type FreezeJudgment struct {
	outcome FreezeOutcome
}

type FreezeOutcome uint8

const (
	FreezeOutcomeInvalid FreezeOutcome = iota
	FreezeUnconfigured
	FreezeBoundaryCrossed
	FreezeBoundaryNotCrossed
)

func (judgment FreezeJudgment) Outcome() FreezeOutcome { return judgment.outcome }

func (judgment FreezeJudgment) Unconfigured() bool { return judgment.outcome == FreezeUnconfigured }

func (judgment FreezeJudgment) Crossed() bool { return judgment.outcome == FreezeBoundaryCrossed }

// JudgeFreezeBoundary 按已声明形态判断是否越过冻结边界。形态或取值缺席答未配置。
func JudgeFreezeBoundary(form FreezeForm, limit *int, remainingSegments int) (FreezeJudgment, error) {
	if form == FreezeFormUndeclared || limit == nil {
		return FreezeJudgment{outcome: FreezeUnconfigured}, nil
	}
	if form != RemainingSegmentCountFreeze || *limit < 0 || remainingSegments < 0 {
		return FreezeJudgment{}, fmt.Errorf("%w: freeze facts", ErrUnknownFreezeForm)
	}
	if remainingSegments <= *limit {
		return FreezeJudgment{outcome: FreezeBoundaryCrossed}, nil
	}
	return FreezeJudgment{outcome: FreezeBoundaryNotCrossed}, nil
}

// ExecutedPrefixGrade 是已执行前缀的分格。
type ExecutedPrefixGrade uint8

const (
	ExecutedPrefixGradeInvalid ExecutedPrefixGrade = iota
	ExecutedPrefixNoControllableNode
	ExecutedPrefixNodeNotOnPlan
	ExecutedPrefixEstablished
)

// ExecutedPrefix 是计划段链上已经执行的节点前缀和剩余节点。
type ExecutedPrefix struct {
	grade     ExecutedPrefixGrade
	prefix    []string
	remaining []string
}

func (prefix ExecutedPrefix) Grade() ExecutedPrefixGrade { return prefix.grade }

func (prefix ExecutedPrefix) Prefix() []string { return append([]string(nil), prefix.prefix...) }

func (prefix ExecutedPrefix) Remaining() []string {
	return append([]string(nil), prefix.remaining...)
}

// RemainingSegments 是剩余节点之间的段数。尚无可控节点时剩余段数是整条计划。
func (prefix ExecutedPrefix) RemainingSegments() int {
	if len(prefix.remaining) == 0 {
		return 0
	}
	return len(prefix.remaining) - 1
}

// JudgeExecutedPrefix 用计划节点链和当前可控节点切出已执行前缀。可控节点只认节点收寄或
// 权威运输交接。扫描、位置或消息顺序不建立可控节点。
func JudgeExecutedPrefix(planNodes []string, control ControlEvidenceKind, location string) ExecutedPrefix {
	nodes := append([]string(nil), planNodes...)
	if control != NodeIntakeControl && control != TransportHandoverControl {
		return ExecutedPrefix{grade: ExecutedPrefixNoControllableNode, remaining: nodes}
	}
	index := -1
	for i, node := range nodes {
		if node == location {
			index = i
			break
		}
	}
	if index < 0 {
		return ExecutedPrefix{grade: ExecutedPrefixNodeNotOnPlan, remaining: nodes}
	}
	return ExecutedPrefix{
		grade:     ExecutedPrefixEstablished,
		prefix:    append([]string(nil), nodes[:index+1]...),
		remaining: append([]string(nil), nodes[index:]...),
	}
}

// PlanNodes 从连续段链取出计划节点。断链时第二返回值是假。
func PlanNodes(legs []PlannedLeg) ([]string, bool) {
	if len(legs) == 0 {
		return nil, false
	}
	nodes := []string{legs[0].From().String()}
	for _, leg := range legs {
		if nodes[len(nodes)-1] != leg.From().String() {
			return nil, false
		}
		nodes = append(nodes, leg.To().String())
	}
	return nodes, true
}
