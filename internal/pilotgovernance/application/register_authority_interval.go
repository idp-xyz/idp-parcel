package application

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
	"go.idp.xyz/idp-parcel/internal/pilotgovernance/ports"
)

// RegisterAuthorityIntervalOutcome 是权威区间独立登记的应用处理结果。
type RegisterAuthorityIntervalOutcome uint8

const (
	RegisterAuthorityIntervalOutcomeInvalid RegisterAuthorityIntervalOutcome = iota
	IntervalRegistered
	IntervalAlreadyRegistered
	IntervalConflictBlocked
	IntervalNotAccepted
	IntervalUndecided
)

func (outcome RegisterAuthorityIntervalOutcome) String() string {
	switch outcome {
	case IntervalRegistered:
		return "REGISTERED"
	case IntervalAlreadyRegistered:
		return "ALREADY_REGISTERED"
	case IntervalConflictBlocked:
		return "AUTHORITY_CONFLICT"
	case IntervalNotAccepted:
		return "NOT_ACCEPTED"
	case IntervalUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

type RegisterAuthorityIntervalResult struct {
	outcome   RegisterAuthorityIntervalOutcome
	conflicts []domain.AuthorityConflict
}

func (result RegisterAuthorityIntervalResult) Outcome() RegisterAuthorityIntervalOutcome {
	return result.outcome
}

// Conflicts 只在冲突阻断时给出——处置者要知道撞上了哪些区间，一对都不能少。
func (result RegisterAuthorityIntervalResult) Conflicts() []domain.AuthorityConflict {
	return append([]domain.AuthorityConflict(nil), result.conflicts...)
}

type RegisterAuthorityIntervalDeps struct {
	Intervals ports.AuthorityIntervalStore
}

// RegisterAuthorityIntervalHandler 是权威区间的独立登记编排。评审 Go 与接管各有
// 附带追加，但「登记一条权威区间」作为归属未决的恢复动作此前无路可走（票 12 首批
// 第一类）；这里沿用同一套纪律：冲突预检先于任何落库，全等重放答已在册。
type RegisterAuthorityIntervalHandler struct {
	deps RegisterAuthorityIntervalDeps
}

func NewRegisterAuthorityIntervalHandler(deps RegisterAuthorityIntervalDeps) *RegisterAuthorityIntervalHandler {
	return &RegisterAuthorityIntervalHandler{deps: deps}
}

// Handle 登记一条生产权威区间：形状门（判据在域，非法形状不问库）→ 全等重放答
// 已在册（先于冲突预检——全等区间与自己必然重叠，重放不是冲突）→ 冲突预检（重叠
// 即阻断带全部冲突对，「双写后人工对账」是被点名的错误结果）→ 追加。
func (handler *RegisterAuthorityIntervalHandler) Handle(
	ctx context.Context,
	interval domain.AuthorityInterval,
) (RegisterAuthorityIntervalResult, error) {
	if _, err := domain.DetectAuthorityConflicts([]domain.AuthorityInterval{interval}); err != nil {
		return RegisterAuthorityIntervalResult{outcome: IntervalNotAccepted}, nil
	}

	current, err := handler.deps.Intervals.ListCurrent(ctx)
	if err != nil {
		return RegisterAuthorityIntervalResult{outcome: IntervalUndecided}, nil
	}
	for _, existing := range current {
		if existing == interval {
			return RegisterAuthorityIntervalResult{outcome: IntervalAlreadyRegistered}, nil
		}
	}

	conflicts, err := domain.DetectAuthorityConflicts(append(current, interval))
	if err != nil {
		return RegisterAuthorityIntervalResult{outcome: IntervalNotAccepted}, nil
	}
	if len(conflicts) > 0 {
		return RegisterAuthorityIntervalResult{outcome: IntervalConflictBlocked, conflicts: conflicts}, nil
	}

	if err := handler.deps.Intervals.Append(ctx, interval); err != nil {
		return RegisterAuthorityIntervalResult{outcome: IntervalUndecided}, nil
	}
	return RegisterAuthorityIntervalResult{outcome: IntervalRegistered}, nil
}
