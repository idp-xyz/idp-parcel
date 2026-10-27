package domain

import "fmt"

// 本文件是取消先后冲突的解开决定（ADR-0295）：有权主体对册上一份冲突（ADR-0254 决定二第 3 格）形成的明确业务决定。决定不改写冲突行，也
// 不改写已提交的取消；它的作用由读它的各方各自落地——有效取消的读排除被认定不成立的取消，边界事实被交来源方更正的那一代在收寄采用与面单
// 终局判断处不停在冲突上。

// CancellationOrderConflictDecisionKind 是解开一份冲突的两种去向（封闭集）。
//
//   - 认定取消依据不成立：边界事实成立，取消提出时包裹其实已越过取消边界，那份取消不是一份有效取消。取消记录保留作历史，不再被任何
//     判断当作有效取消读到；包裹按越过边界的包裹走收寄后服务处置。
//   - 认定边界事实有误，交来源方更正：取消保持有效；这一代边界事实不被采用为责任起点或非取消终局，等来源方更正出新的一代，新的一代
//     按 ADR-0254 决定二重新比先后。本上下文不改任何来源事实。
type CancellationOrderConflictDecisionKind uint8

const (
	CancellationOrderConflictDecisionKindInvalid CancellationOrderConflictDecisionKind = iota
	CancellationNotUpheld
	BoundaryFactReferred
)

func (kind CancellationOrderConflictDecisionKind) String() string {
	switch kind {
	case CancellationNotUpheld:
		return "CANCELLATION_NOT_UPHELD"
	case BoundaryFactReferred:
		return "BOUNDARY_FACT_REFERRED"
	default:
		return ""
	}
}

// ParseCancellationOrderConflictDecisionKind 按字面认决定去向，词不在封闭集内即拒，不猜。
func ParseCancellationOrderConflictDecisionKind(raw string) (CancellationOrderConflictDecisionKind, error) {
	for _, kind := range []CancellationOrderConflictDecisionKind{CancellationNotUpheld, BoundaryFactReferred} {
		if kind.String() == raw {
			return kind, nil
		}
	}
	return CancellationOrderConflictDecisionKindInvalid, fmt.Errorf("cancellation order conflict decision kind %q is not a kind", raw)
}

// CancellationOrderConflictDeciderReference 是决定人的引用，取（发行方，sub）这一对：sub 只在它的发行方之内唯一。本上下文只记引用，
// 不拥有操作者目录；有没有权决定由「运营决定」的授予把守。
type CancellationOrderConflictDeciderReference struct{ requiredValue }

func NewCancellationOrderConflictDeciderReference(value string) (CancellationOrderConflictDeciderReference, error) {
	required, err := newRequiredValue("cancellation order conflict decider reference", value)
	return CancellationOrderConflictDeciderReference{required}, err
}

// CancellationOrderConflictDecisionBasis 是决定依据：决定人核对了什么、凭什么认定取消依据不成立或边界事实有误。两种去向都必填——事后读册
// 的人要分得出这一次是依物流证据裁断，还是出过差错。本上下文不判依据够不够。
type CancellationOrderConflictDecisionBasis struct{ requiredValue }

func NewCancellationOrderConflictDecisionBasis(value string) (CancellationOrderConflictDecisionBasis, error) {
	required, err := newRequiredValue("cancellation order conflict decision basis", value)
	return CancellationOrderConflictDecisionBasis{required}, err
}
