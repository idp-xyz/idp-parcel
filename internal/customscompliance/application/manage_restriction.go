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
	// RestrictionBasisNotAccepted：所引的外部监管结果当不了这次建立或解除的依据——不在册、归属不上、同层冲突、层次不对、
	// 范围不是这份限制的范围，或发生在限制生效之前。
	RestrictionBasisNotAccepted
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
	case RestrictionBasisNotAccepted:
		return "BASIS_NOT_ACCEPTED"
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

// EstablishRestrictionOnResultCommand 是关务人员依据一条已接收的外部监管结果建立监管限制的请求。限制的决定身份、范围与
// 生效时刻都取自所引的那条结果，请求方只指名结果、给限制标识、声明它约束哪些方向性动作——哪种监管决定约束哪些动作是
// 关务人员读决定原文作出的判断，产品不替它推导。
type EstablishRestrictionOnResultCommand struct {
	TenantID   domain.TenantID
	ID         domain.RestrictionID
	Decision   string
	Constrains []domain.GuardedAction
}

// ReleaseRestrictionOnResultCommand 是依据一条已接收的外部监管结果解除监管限制的请求。解除时刻取所引结果的业务发生时间。
type ReleaseRestrictionOnResultCommand struct {
	TenantID domain.TenantID
	ID       domain.RestrictionID
	Result   string
}

type ManageRestrictionDeps struct {
	// Results 只供 EstablishOnResult 与 ReleaseOnResult 核依据；不经这两个入口时可以为空。
	Results    ports.AcceptedRegulatoryResults
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
	updated, err := handler.deps.Store.Update(ctx, command.TenantID, released)
	if err != nil {
		return RestrictionResult{outcome: RestrictionUndecided}, nil
	}
	if updated == ports.RestrictionAlreadySettled {
		// 读到「当前有效」之后、落解除之前，另一笔解除先落了：答已解除并交回在册那一份，不盖掉先落的解除依据。
		settled, found, err := handler.deps.Store.FindByID(ctx, command.TenantID, command.ID)
		if err != nil || !found {
			return RestrictionResult{outcome: RestrictionUndecided}, nil
		}
		return RestrictionResult{outcome: RestrictionAlreadyReleased, restriction: settled, hasRecord: true}, nil
	}
	if updated != ports.RestrictionReleaseRecorded {
		return RestrictionResult{}, fmt.Errorf("%w: update %d", ErrUnexpectedRestrictionSave, updated)
	}
	result := RestrictionResult{
		outcome:     RestrictionReleased,
		restriction: released,
		hasRecord:   true,
	}
	result.handoffRef = handler.handOffRestriction(ctx, command.TenantID, released)
	return result, nil
}

// EstablishOnResult 依据一条已接收的监管决定建立限制。所引结果必须在册、归属得上原提交、不处在同层冲突里，并且是过程
// 决定或处置决定这两层——接收、受理、核定税费与放行都不是能立限制的决定。限制的决定身份取结果的来源标识，范围与生效时刻
// 取结果自己的范围与业务发生时间：人只声明约束哪些动作，造不出一条没有监管决定撑着的限制。
func (handler *ManageRestrictionHandler) EstablishOnResult(
	ctx context.Context,
	command EstablishRestrictionOnResultCommand,
) (RestrictionResult, error) {
	if strings.TrimSpace(command.TenantID.String()) == "" || handler.deps.Results == nil {
		return RestrictionResult{outcome: RestrictionNotAccepted}, nil
	}
	record, outcome := handler.acceptedResult(ctx, command.TenantID, command.Decision,
		domain.ProcessDecisionLayer, domain.DispositionDecisionLayer)
	if outcome != RestrictionOutcomeInvalid {
		return RestrictionResult{outcome: outcome}, nil
	}
	decision, err := domain.NewRegulatoryDecisionID(record.Result.SourceID())
	if err != nil {
		return RestrictionResult{outcome: RestrictionBasisNotAccepted}, nil
	}
	return handler.Establish(ctx, EstablishRestrictionCommand{
		TenantID: command.TenantID,
		Spec: domain.RegulatoryRestrictionSpec{
			ID:          command.ID,
			Decision:    decision,
			Scope:       record.Result.Scope(),
			Constrains:  command.Constrains,
			EffectiveAt: record.Result.OccurredAt(),
		},
	})
}

// ReleaseOnResult 依据一条已接收的监管结果解除限制。所引结果必须在册、归属得上、不处在同层冲突里，是放行结果、过程决定
// 或处置决定这三层之一，范围就是这份限制的范围，且发生在限制生效之后。执行方的控制或隔离事实进不了外部结果册，也就当
// 不了这里的依据。
func (handler *ManageRestrictionHandler) ReleaseOnResult(
	ctx context.Context,
	command ReleaseRestrictionOnResultCommand,
) (RestrictionResult, error) {
	if strings.TrimSpace(command.TenantID.String()) == "" || handler.deps.Results == nil {
		return RestrictionResult{outcome: RestrictionNotAccepted}, nil
	}
	restriction, found, err := handler.deps.Store.FindByID(ctx, command.TenantID, command.ID)
	if err != nil {
		return RestrictionResult{outcome: RestrictionUndecided}, nil
	}
	if !found {
		return RestrictionResult{outcome: RestrictionNotFound}, nil
	}
	record, outcome := handler.acceptedResult(ctx, command.TenantID, command.Result,
		domain.ReleaseResultLayer, domain.ProcessDecisionLayer, domain.DispositionDecisionLayer)
	if outcome != RestrictionOutcomeInvalid {
		return RestrictionResult{outcome: outcome}, nil
	}
	if record.Result.Scope() != restriction.Scope() || record.Result.OccurredAt().Before(restriction.EffectiveAt()) ||
		record.Result.SourceID() == restriction.Decision().String() {
		return RestrictionResult{outcome: RestrictionBasisNotAccepted}, nil
	}
	release, err := domain.NewRegulatoryReleaseReference(record.Result.SourceID())
	if err != nil {
		return RestrictionResult{outcome: RestrictionBasisNotAccepted}, nil
	}
	return handler.Release(ctx, ReleaseRestrictionCommand{
		TenantID: command.TenantID,
		ID:       command.ID,
		Release:  release,
		At:       record.Result.OccurredAt(),
	})
}

// acceptedResult 读所引的外部监管结果并核它当不当得了依据。交回的 outcome 为零值表示可用；否则是该直接作答的那一格。
func (handler *ManageRestrictionHandler) acceptedResult(
	ctx context.Context,
	tenant domain.TenantID,
	source string,
	layers ...domain.ResultLayer,
) (ports.ExternalResultRecord, RestrictionOutcome) {
	source = strings.TrimSpace(source)
	if source == "" {
		return ports.ExternalResultRecord{}, RestrictionNotAccepted
	}
	record, found, err := handler.deps.Results.FindByKey(ctx, ports.ExternalResultKey{TenantID: tenant, SourceID: source})
	if err != nil {
		return ports.ExternalResultRecord{}, RestrictionUndecided
	}
	if !found || record.Unattributable || record.LayerConflict {
		return ports.ExternalResultRecord{}, RestrictionBasisNotAccepted
	}
	for _, layer := range layers {
		if record.Result.Layer() == layer {
			return record, RestrictionOutcomeInvalid
		}
	}
	return ports.ExternalResultRecord{}, RestrictionBasisNotAccepted
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
