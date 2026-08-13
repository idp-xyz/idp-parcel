package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
	"go.idp.xyz/idp-parcel/internal/pilotgovernance/ports"
)

// ErrUnexpectedGovernanceSave 说明治理库交回了封闭集合以外的写入结果。
var ErrUnexpectedGovernanceSave = errors.New("pilot governance: unexpected governance save outcome")

// GovernIncidentOutcome 是治理事件请求的应用处理结果。
type GovernIncidentOutcome uint8

const (
	GovernIncidentOutcomeInvalid GovernIncidentOutcome = iota
	SuspensionRecorded
	SuspensionExisting
	ResumptionRecorded
	ResumptionExisting
	SuspensionNotFound
	TakeoverRecorded
	TakeoverExisting
	TakeoverConflictBlocked
	GovernIncidentNotAccepted
	GovernIncidentUndecided
)

func (outcome GovernIncidentOutcome) String() string {
	switch outcome {
	case SuspensionRecorded:
		return "SUSPENSION_RECORDED"
	case SuspensionExisting:
		return "SUSPENSION_EXISTING"
	case ResumptionRecorded:
		return "RESUMPTION_RECORDED"
	case ResumptionExisting:
		return "RESUMPTION_EXISTING"
	case SuspensionNotFound:
		return "SUSPENSION_NOT_FOUND"
	case TakeoverRecorded:
		return "TAKEOVER_RECORDED"
	case TakeoverExisting:
		return "TAKEOVER_EXISTING"
	case TakeoverConflictBlocked:
		return "TAKEOVER_CONFLICT"
	case GovernIncidentNotAccepted:
		return "NOT_ACCEPTED"
	case GovernIncidentUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

type GovernIncidentResult struct {
	outcome    GovernIncidentOutcome
	suspension domain.SuspensionDecision
	resumption domain.ResumptionDecision
	takeover   domain.TakeoverRecord
	conflicts  []domain.AuthorityConflict
	handoffRef string
}

func (result GovernIncidentResult) Outcome() GovernIncidentOutcome {
	return result.outcome
}

func (result GovernIncidentResult) Suspension() domain.SuspensionDecision { return result.suspension }

func (result GovernIncidentResult) Resumption() domain.ResumptionDecision { return result.resumption }

func (result GovernIncidentResult) Takeover() domain.TakeoverRecord { return result.takeover }

// Conflicts 只在接管被权威冲突阻断时给出——先关原区间再接管，一对都不能少。
func (result GovernIncidentResult) Conflicts() []domain.AuthorityConflict {
	return append([]domain.AuthorityConflict(nil), result.conflicts...)
}

// HandoffReference 非空说明记录已入册但意图还没交出去，重放会重发同一份。
func (result GovernIncidentResult) HandoffReference() string {
	return result.handoffRef
}

type GovernIncidentDeps struct {
	Suspensions ports.SuspensionStore
	Resumptions ports.ResumptionStore
	Takeovers   ports.TakeoverStore
	Intervals   ports.AuthorityIntervalStore
	Downstream  ports.GovernanceHandoff
	Clock       ports.Clock
}

type GovernIncidentHandler struct {
	deps GovernIncidentDeps
}

func NewGovernIncidentHandler(deps GovernIncidentDeps) *GovernIncidentHandler {
	return &GovernIncidentHandler{deps: deps}
}

// Suspend 记录暂停新准入的决定：同暂停标识重放返原（治理记录不可覆盖）；领域把门
// （触发来源/依据/证据/在途处置说明缺一不可——暂停不取消不迁移不回退在途，那半句在
// 领域类型上）。
func (handler *GovernIncidentHandler) Suspend(
	ctx context.Context,
	spec domain.SuspensionDecisionSpec,
) (GovernIncidentResult, error) {
	decision, err := domain.RecordSuspension(spec)
	if err != nil {
		return GovernIncidentResult{outcome: GovernIncidentNotAccepted}, nil
	}
	saved, err := handler.deps.Suspensions.Save(ctx, decision)
	if err != nil {
		return GovernIncidentResult{outcome: GovernIncidentUndecided}, nil
	}
	switch saved {
	case ports.GovernanceSaved:
		result := GovernIncidentResult{outcome: SuspensionRecorded, suspension: decision}
		result.handoffRef = handler.handOff(ctx, ports.GovernanceHandoffIntent{Suspension: &decision})
		return result, nil
	case ports.GovernanceAlreadyRecorded:
		existing, found, err := handler.deps.Suspensions.FindByID(ctx, spec.ID)
		if err != nil || !found {
			return GovernIncidentResult{outcome: GovernIncidentUndecided}, nil
		}
		return GovernIncidentResult{outcome: SuspensionExisting, suspension: existing}, nil
	default:
		return GovernIncidentResult{}, fmt.Errorf("%w: %d", ErrUnexpectedGovernanceSave, saved)
	}
}

// Resume 记录恢复新准入的决定：被恢复的暂停必须在场（引用不存在的暂停未受理——
// 恢复的身份挂在它要解除的暂停上）；一个暂停至多一次恢复，重复恢复按已恢复作答；
// 解除证据、一致性核对与在途盘点由领域把门。
func (handler *GovernIncidentHandler) Resume(
	ctx context.Context,
	spec domain.ResumptionDecisionSpec,
) (GovernIncidentResult, error) {
	_, found, err := handler.deps.Suspensions.FindByID(ctx, spec.Suspension)
	if err != nil {
		return GovernIncidentResult{outcome: GovernIncidentUndecided}, nil
	}
	if !found {
		return GovernIncidentResult{outcome: SuspensionNotFound}, nil
	}
	if existing, found, err := handler.deps.Resumptions.FindBySuspension(ctx, spec.Suspension); err != nil {
		return GovernIncidentResult{outcome: GovernIncidentUndecided}, nil
	} else if found {
		return GovernIncidentResult{outcome: ResumptionExisting, resumption: existing}, nil
	}

	decision, err := domain.RecordResumption(spec)
	if err != nil {
		return GovernIncidentResult{outcome: GovernIncidentNotAccepted}, nil
	}
	saved, err := handler.deps.Resumptions.Save(ctx, decision)
	if err != nil {
		return GovernIncidentResult{outcome: GovernIncidentUndecided}, nil
	}
	switch saved {
	case ports.GovernanceSaved:
		result := GovernIncidentResult{outcome: ResumptionRecorded, resumption: decision}
		result.handoffRef = handler.handOff(ctx, ports.GovernanceHandoffIntent{Resumption: &decision})
		return result, nil
	case ports.GovernanceAlreadyRecorded:
		winner, found, err := handler.deps.Resumptions.FindBySuspension(ctx, spec.Suspension)
		if err != nil || !found {
			return GovernIncidentResult{outcome: GovernIncidentUndecided}, nil
		}
		return GovernIncidentResult{outcome: ResumptionExisting, resumption: winner}, nil
	default:
		return GovernIncidentResult{}, fmt.Errorf("%w: %d", ErrUnexpectedGovernanceSave, saved)
	}
}

// TakeOver 记录对象级接管：新权威区间先过冲突预检——撞上仍开着的既有区间即阻断带
// 全部冲突对（先关原区间再接管，原权威停下的证据由领域把门）；同区间身份重放返原；
// 入册后追加区间沿用评审 handler 的续办纪律。
func (handler *GovernIncidentHandler) TakeOver(
	ctx context.Context,
	spec domain.TakeoverRecordSpec,
) (GovernIncidentResult, error) {
	record, err := domain.RecordTakeover(spec)
	if err != nil {
		return GovernIncidentResult{outcome: GovernIncidentNotAccepted}, nil
	}

	if existing, found, err := handler.deps.Takeovers.FindByInterval(ctx, spec.Interval); err != nil {
		return GovernIncidentResult{outcome: GovernIncidentUndecided}, nil
	} else if found {
		// 重放路补追加：上次区间追加失败留下的分岔在这里收口——接管不翻，只重试
		// 同一份追加（镜像阶段评审 handler 的续办纪律，已在册不重追）。
		result := GovernIncidentResult{outcome: TakeoverExisting, takeover: existing}
		result.handoffRef = handler.appendTakeoverInterval(ctx, spec.Interval)
		return result, nil
	}

	current, err := handler.deps.Intervals.ListCurrent(ctx)
	if err != nil {
		return GovernIncidentResult{outcome: GovernIncidentUndecided}, nil
	}
	conflicts, err := domain.DetectAuthorityConflicts(append(current, spec.Interval))
	if err != nil {
		return GovernIncidentResult{outcome: GovernIncidentNotAccepted}, nil
	}
	if len(conflicts) > 0 {
		return GovernIncidentResult{outcome: TakeoverConflictBlocked, conflicts: conflicts}, nil
	}

	saved, err := handler.deps.Takeovers.Save(ctx, record)
	if err != nil {
		return GovernIncidentResult{outcome: GovernIncidentUndecided}, nil
	}
	switch saved {
	case ports.GovernanceSaved:
		result := GovernIncidentResult{outcome: TakeoverRecorded, takeover: record}
		if ref := handler.appendTakeoverInterval(ctx, spec.Interval); ref != "" {
			result.handoffRef = ref
			return result, nil
		}
		result.handoffRef = handler.handOff(ctx, ports.GovernanceHandoffIntent{Takeover: &record})
		return result, nil
	case ports.GovernanceAlreadyRecorded:
		winner, found, err := handler.deps.Takeovers.FindByInterval(ctx, spec.Interval)
		if err != nil || !found {
			return GovernIncidentResult{outcome: GovernIncidentUndecided}, nil
		}
		result := GovernIncidentResult{outcome: TakeoverExisting, takeover: winner}
		result.handoffRef = handler.appendTakeoverInterval(ctx, spec.Interval)
		return result, nil
	default:
		return GovernIncidentResult{}, fmt.Errorf("%w: %d", ErrUnexpectedGovernanceSave, saved)
	}
}

// appendTakeoverInterval 追加接管授予的权威区间。失败不翻接管但交回续办引用（接管
// 在册而区间缺失且无处可知，正是权威不明的双帐分岔）；已在册的区间不重复追加。
func (handler *GovernIncidentHandler) appendTakeoverInterval(
	ctx context.Context,
	interval domain.AuthorityInterval,
) string {
	current, err := handler.deps.Intervals.ListCurrent(ctx)
	if err == nil {
		for _, existing := range current {
			if existing == interval {
				return ""
			}
		}
	}
	if err := handler.deps.Intervals.Append(ctx, interval); err != nil {
		return "CONT-TAKEOVER-INTERVAL/" + interval.ObjectScope
	}
	return ""
}

// handOff 交发布意图。失败不翻记录，留续办引用重发同一份。
func (handler *GovernIncidentHandler) handOff(
	ctx context.Context,
	intent ports.GovernanceHandoffIntent,
) string {
	if err := handler.deps.Downstream.HandOffGovernance(ctx, intent); err == nil {
		return ""
	}
	switch {
	case intent.Suspension != nil:
		return "CONT-GOV-SUSPENSION"
	case intent.Resumption != nil:
		return "CONT-GOV-RESUMPTION"
	default:
		return "CONT-GOV-TAKEOVER"
	}
}
