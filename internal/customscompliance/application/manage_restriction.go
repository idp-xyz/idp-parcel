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

// ErrUnexpectedRestrictionSave 说明限制库交回了封闭集合以外的写入结果。
var ErrUnexpectedRestrictionSave = errors.New("customs compliance: unexpected restriction save outcome")

// RestrictionOutcome 是限制管理请求的应用处理结果。
type RestrictionOutcome uint8

const (
	RestrictionOutcomeInvalid RestrictionOutcome = iota
	RestrictionEstablished
	RestrictionExisting
	RestrictionContentConflict
	RestrictionReleased
	RestrictionAlreadyReleased
	RestrictionNotFound
	RestrictionNotAccepted
	RestrictionUndecided
)

func (outcome RestrictionOutcome) String() string {
	switch outcome {
	case RestrictionEstablished:
		return "ESTABLISHED"
	case RestrictionExisting:
		return "EXISTING"
	case RestrictionContentConflict:
		return "CONTENT_CONFLICT"
	case RestrictionReleased:
		return "RELEASED"
	case RestrictionAlreadyReleased:
		return "ALREADY_RELEASED"
	case RestrictionNotFound:
		return "NOT_FOUND"
	case RestrictionNotAccepted:
		return "NOT_ACCEPTED"
	case RestrictionUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

// EstablishRestrictionCommand 携带一次限制建立请求。
type EstablishRestrictionCommand struct {
	TenantID domain.TenantID
	Spec     domain.RegulatoryRestrictionSpec
}

// ReleaseRestrictionCommand 携带一次限制解除请求：解除依据只能是责任来源接受的监管
// 结果——引用类型在领域上封死，这里不再复述。
type ReleaseRestrictionCommand struct {
	TenantID domain.TenantID
	ID       domain.RestrictionID
	Release  domain.RegulatoryReleaseReference
	At       time.Time
}

type RestrictionResult struct {
	outcome     RestrictionOutcome
	restriction domain.RegulatoryRestriction
	hasRecord   bool
	handoffRef  string
}

func (result RestrictionResult) Outcome() RestrictionOutcome {
	return result.outcome
}

func (result RestrictionResult) Restriction() (domain.RegulatoryRestriction, bool) {
	return result.restriction, result.hasRecord
}

// HandoffReference 非空说明结果已入册但意图还没交出去，重放会重发同一份。
func (result RestrictionResult) HandoffReference() string {
	return result.handoffRef
}

type ManageRestrictionDeps struct {
	Store      ports.RestrictionStore
	Downstream ports.RestrictionHandoff
	Clock      ports.Clock
}

type ManageRestrictionHandler struct {
	deps ManageRestrictionDeps
}

func NewManageRestrictionHandler(deps ManageRestrictionDeps) *ManageRestrictionHandler {
	return &ManageRestrictionHandler{deps: deps}
}

// Establish 建立监管限制：同 ID 重放返原限制不重立；同 ID 异内容（决定/范围/约束集
// 不同）是冒名冲突不顶替——限制的身份由建立它的监管决定给出，改内容要新限制。
func (handler *ManageRestrictionHandler) Establish(
	ctx context.Context,
	command EstablishRestrictionCommand,
) (RestrictionResult, error) {
	if strings.TrimSpace(command.TenantID.String()) == "" {
		return RestrictionResult{outcome: RestrictionNotAccepted}, nil
	}
	restriction, err := domain.EstablishRestriction(command.Spec)
	if err != nil {
		return RestrictionResult{outcome: RestrictionNotAccepted}, nil
	}

	saved, err := handler.deps.Store.Save(ctx, command.TenantID, restriction)
	if err != nil {
		return RestrictionResult{outcome: RestrictionUndecided}, nil
	}
	switch saved {
	case ports.RestrictionSaved:
		result := RestrictionResult{
			outcome:     RestrictionEstablished,
			restriction: restriction,
			hasRecord:   true,
		}
		result.handoffRef = handler.handOffRestriction(ctx, command.TenantID, restriction)
		return result, nil
	case ports.RestrictionAlreadyRecorded:
		existing, found, err := handler.deps.Store.FindByID(ctx, command.TenantID, restriction.ID())
		if err != nil || !found {
			return RestrictionResult{outcome: RestrictionUndecided}, nil
		}
		if !sameRestrictionContent(existing, restriction) {
			return RestrictionResult{outcome: RestrictionContentConflict}, nil
		}
		result := RestrictionResult{
			outcome:     RestrictionExisting,
			restriction: existing,
			hasRecord:   true,
		}
		result.handoffRef = handler.handOffRestriction(ctx, command.TenantID, existing)
		return result, nil
	default:
		return RestrictionResult{}, fmt.Errorf("%w: %d", ErrUnexpectedRestrictionSave, saved)
	}
}

// Release 依据监管结果解除限制：已解除的限制重复解除按已解除作答（幂等重放不是
// 错误）；不存在的限制如实不存在。
func (handler *ManageRestrictionHandler) Release(
	ctx context.Context,
	command ReleaseRestrictionCommand,
) (RestrictionResult, error) {
	if strings.TrimSpace(command.TenantID.String()) == "" {
		return RestrictionResult{outcome: RestrictionNotAccepted}, nil
	}
	restriction, found, err := handler.deps.Store.FindByID(ctx, command.TenantID, command.ID)
	if err != nil {
		return RestrictionResult{outcome: RestrictionUndecided}, nil
	}
	if !found {
		return RestrictionResult{outcome: RestrictionNotFound}, nil
	}
	if !restriction.Current() {
		return RestrictionResult{
			outcome:     RestrictionAlreadyReleased,
			restriction: restriction,
			hasRecord:   true,
		}, nil
	}

	released, err := restriction.ReleaseByRegulatoryOutcome(command.Release, command.At)
	if err != nil {
		return RestrictionResult{outcome: RestrictionNotAccepted}, nil
	}
	if err := handler.deps.Store.Update(ctx, command.TenantID, released); err != nil {
		return RestrictionResult{outcome: RestrictionUndecided}, nil
	}
	result := RestrictionResult{
		outcome:     RestrictionReleased,
		restriction: released,
		hasRecord:   true,
	}
	result.handoffRef = handler.handOffRestriction(ctx, command.TenantID, released)
	return result, nil
}

// JudgeAction 判断一个方向性动作对明确对象是否可继续。纯读：范围内全部限制参与领域
// 判断，阻断清单一个不少；限制盘不出来就不判——半份清单判出的放行是假放行。
func (handler *ManageRestrictionHandler) JudgeAction(
	ctx context.Context,
	tenant domain.TenantID,
	action domain.GuardedAction,
	scope domain.DecisionScopeReference,
) (domain.ActionAdmissibility, error) {
	restrictions, err := handler.deps.Store.ListByScope(ctx, tenant, scope)
	if err != nil {
		return domain.ActionAdmissibility{}, fmt.Errorf("list restrictions: %w", err)
	}
	return domain.JudgeActionAdmissibility(action, scope, restrictions)
}

// sameRestrictionContent 比较两份限制的建立内容（身份外的全部语义）。
func sameRestrictionContent(a, b domain.RegulatoryRestriction) bool {
	if a.Decision() != b.Decision() || a.Scope() != b.Scope() || !a.EffectiveAt().Equal(b.EffectiveAt()) {
		return false
	}
	constraintsA, constraintsB := a.Constrains(), b.Constrains()
	if len(constraintsA) != len(constraintsB) {
		return false
	}
	declared := make(map[domain.GuardedAction]bool, len(constraintsA))
	for _, action := range constraintsA {
		declared[action] = true
	}
	for _, action := range constraintsB {
		if !declared[action] {
			return false
		}
	}
	return true
}

// handOffRestriction 交发布意图。失败不翻结果，留续办引用重发同一份。
func (handler *ManageRestrictionHandler) handOffRestriction(
	ctx context.Context,
	tenant domain.TenantID,
	restriction domain.RegulatoryRestriction,
) string {
	if err := handler.deps.Downstream.HandOffRestriction(ctx, ports.RestrictionHandoffIntent{
		TenantID:    tenant,
		Restriction: restriction,
	}); err == nil {
		return ""
	}
	return "CONT-RESTRICTION/" + restriction.ID().String()
}
