package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// ErrUnexpectedFollowUpSave 说明后续动作库交回了封闭集合以外的写入结果。
var ErrUnexpectedFollowUpSave = errors.New("customs compliance: unexpected follow-up save outcome")

// FollowUpOutcome 是后续申报动作管理请求的应用处理结果。
type FollowUpOutcome uint8

const (
	FollowUpOutcomeInvalid FollowUpOutcome = iota
	FollowUpTargetFormed
	FollowUpTargetExisting
	ReplacementProposed
	ReplacementExisting
	ReplacementUnitConflict
	ReplacementEffective
	ReplacementAlreadyEffective
	FollowUpTargetNotFound
	FollowUpNotAccepted
	FollowUpUndecided
)

func (outcome FollowUpOutcome) String() string {
	switch outcome {
	case FollowUpTargetFormed:
		return "TARGET_FORMED"
	case FollowUpTargetExisting:
		return "TARGET_EXISTING"
	case ReplacementProposed:
		return "REPLACEMENT_PROPOSED"
	case ReplacementExisting:
		return "REPLACEMENT_EXISTING"
	case ReplacementUnitConflict:
		return "REPLACEMENT_UNIT_CONFLICT"
	case ReplacementEffective:
		return "REPLACEMENT_EFFECTIVE"
	case ReplacementAlreadyEffective:
		return "REPLACEMENT_ALREADY_EFFECTIVE"
	case FollowUpTargetNotFound:
		return "TARGET_NOT_FOUND"
	case FollowUpNotAccepted:
		return "NOT_ACCEPTED"
	case FollowUpUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

// FormFollowUpTargetCommand 携带一次目标形成请求。
type FormFollowUpTargetCommand struct {
	TenantID domain.TenantID
	Spec     domain.FollowUpTargetSpec
}

// ProposeReplacementCommand 携带一次拟替代建立请求。
type ProposeReplacementCommand struct {
	TenantID        domain.TenantID
	Key             ports.FollowUpTargetKey
	ReplacementUnit domain.DeclarationUnitID
}

// RecordReplacementEffectCommand 携带一次替代生效登记：外部结果必备——内部决定、
// 请求发出或技术成功都不等于替代成立（领域封死，这里只透传）。
type RecordReplacementEffectCommand struct {
	TenantID       domain.TenantID
	Key            ports.FollowUpTargetKey
	ExternalResult string
	At             time.Time
}

type FollowUpResult struct {
	outcome    FollowUpOutcome
	target     domain.FollowUpTarget
	relation   domain.ReplacementRelation
	hasTarget  bool
	hasRelated bool
	handoffRef string
}

func (result FollowUpResult) Outcome() FollowUpOutcome {
	return result.outcome
}

func (result FollowUpResult) Target() (domain.FollowUpTarget, bool) {
	return result.target, result.hasTarget
}

func (result FollowUpResult) Relation() (domain.ReplacementRelation, bool) {
	return result.relation, result.hasRelated
}

// HandoffReference 非空说明结果已入册但意图还没交出去，重放会重发同一份。
func (result FollowUpResult) HandoffReference() string {
	return result.handoffRef
}

type ManageFollowUpDeps struct {
	Store      ports.FollowUpStore
	Downstream ports.FollowUpHandoff
	Clock      ports.Clock
}

type ManageFollowUpHandler struct {
	deps ManageFollowUpDeps
}

func NewManageFollowUpHandler(deps ManageFollowUpDeps) *ManageFollowUpHandler {
	return &ManageFollowUpHandler{deps: deps}
}

// FormTarget 立后续动作目标：同一触发依据对同一版本的同类动作只立一个目标，重放返
// 原目标不重立。目标形成不等于资料已准备、已经提交或监管结果已经成立——那半句在
// 领域类型上，编排不复述。
func (handler *ManageFollowUpHandler) FormTarget(
	ctx context.Context,
	command FormFollowUpTargetCommand,
) (FollowUpResult, error) {
	if strings.TrimSpace(command.TenantID.String()) == "" {
		return FollowUpResult{outcome: FollowUpNotAccepted}, nil
	}
	target, err := domain.FormFollowUpTarget(command.Spec)
	if err != nil {
		return FollowUpResult{outcome: FollowUpNotAccepted}, nil
	}

	key := ports.FollowUpTargetKey{
		TenantID: command.TenantID,
		Trigger:  command.Spec.Trigger,
		Version:  command.Spec.Version,
		Kind:     command.Spec.Kind,
	}
	saved, err := handler.deps.Store.SaveTarget(ctx, key, target)
	if err != nil {
		return FollowUpResult{outcome: FollowUpUndecided}, nil
	}
	switch saved {
	case ports.FollowUpSaved:
		result := FollowUpResult{outcome: FollowUpTargetFormed, target: target, hasTarget: true}
		result.handoffRef = handler.handOffFollowUp(ctx, key, target, nil)
		return result, nil
	case ports.FollowUpAlreadyRecorded:
		existing, found, err := handler.deps.Store.FindTarget(ctx, key)
		if err != nil || !found {
			return FollowUpResult{outcome: FollowUpUndecided}, nil
		}
		return FollowUpResult{outcome: FollowUpTargetExisting, target: existing, hasTarget: true}, nil
	default:
		return FollowUpResult{}, fmt.Errorf("%w: %d", ErrUnexpectedFollowUpSave, saved)
	}
}

// Propose 依据重报目标建立拟替代关系：一个目标至多一份关系——同替代单元重放返原，
// 异替代单元是冲突不顶替（换单元要先处置原拟替代）。
func (handler *ManageFollowUpHandler) Propose(
	ctx context.Context,
	command ProposeReplacementCommand,
) (FollowUpResult, error) {
	target, found, err := handler.deps.Store.FindTarget(ctx, command.Key)
	if err != nil {
		return FollowUpResult{outcome: FollowUpUndecided}, nil
	}
	if !found {
		return FollowUpResult{outcome: FollowUpTargetNotFound}, nil
	}

	relation, err := domain.ProposeReplacement(target, command.ReplacementUnit)
	if err != nil {
		return FollowUpResult{outcome: FollowUpNotAccepted}, nil
	}
	saved, err := handler.deps.Store.SaveRelation(ctx, command.Key, relation)
	if err != nil {
		return FollowUpResult{outcome: FollowUpUndecided}, nil
	}
	switch saved {
	case ports.FollowUpSaved:
		result := FollowUpResult{
			outcome:    ReplacementProposed,
			target:     target,
			relation:   relation,
			hasTarget:  true,
			hasRelated: true,
		}
		result.handoffRef = handler.handOffFollowUp(ctx, command.Key, target, &relation)
		return result, nil
	case ports.FollowUpAlreadyRecorded:
		existing, found, err := handler.deps.Store.FindRelation(ctx, command.Key)
		if err != nil || !found {
			return FollowUpResult{outcome: FollowUpUndecided}, nil
		}
		if existing.ReplacementUnit() != command.ReplacementUnit {
			return FollowUpResult{outcome: ReplacementUnitConflict}, nil
		}
		return FollowUpResult{
			outcome:    ReplacementExisting,
			target:     target,
			relation:   existing,
			hasTarget:  true,
			hasRelated: true,
		}, nil
	default:
		return FollowUpResult{}, fmt.Errorf("%w: %d", ErrUnexpectedFollowUpSave, saved)
	}
}

// RecordEffect 依据权威外部结果把拟替代升为有效替代：已生效重放按已生效作答（幂等
// 不是错误）；没有拟替代无从生效。
func (handler *ManageFollowUpHandler) RecordEffect(
	ctx context.Context,
	command RecordReplacementEffectCommand,
) (FollowUpResult, error) {
	relation, found, err := handler.deps.Store.FindRelation(ctx, command.Key)
	if err != nil {
		return FollowUpResult{outcome: FollowUpUndecided}, nil
	}
	if !found {
		return FollowUpResult{outcome: FollowUpTargetNotFound}, nil
	}
	if relation.Effective() {
		return FollowUpResult{
			outcome:    ReplacementAlreadyEffective,
			relation:   relation,
			hasRelated: true,
		}, nil
	}

	effective, err := relation.TakeEffect(command.ExternalResult, command.At)
	if err != nil {
		return FollowUpResult{outcome: FollowUpNotAccepted}, nil
	}
	if err := handler.deps.Store.UpdateRelation(ctx, command.Key, effective); err != nil {
		return FollowUpResult{outcome: FollowUpUndecided}, nil
	}
	target, foundTarget, err := handler.deps.Store.FindTarget(ctx, command.Key)
	if err != nil || !foundTarget {
		return FollowUpResult{outcome: FollowUpUndecided}, nil
	}
	result := FollowUpResult{
		outcome:    ReplacementEffective,
		target:     target,
		relation:   effective,
		hasTarget:  true,
		hasRelated: true,
	}
	result.handoffRef = handler.handOffFollowUp(ctx, command.Key, target, &effective)
	return result, nil
}

// handOffFollowUp 交发布意图。失败不翻结果，留续办引用重发同一份。
func (handler *ManageFollowUpHandler) handOffFollowUp(
	ctx context.Context,
	key ports.FollowUpTargetKey,
	target domain.FollowUpTarget,
	relation *domain.ReplacementRelation,
) string {
	if err := handler.deps.Downstream.HandOffFollowUp(ctx, ports.FollowUpHandoffIntent{
		Key:      key,
		Target:   target,
		Relation: relation,
	}); err == nil {
		return ""
	}
	return "CONT-FOLLOWUP/" + key.Version.String() + "/" + key.Kind.String()
}
