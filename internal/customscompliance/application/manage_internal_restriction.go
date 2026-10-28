package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 内部合规限制的建立与决定（UC-CC-011，ADR-0334 决定七）。与 ManageRestrictionHandler 是两个编排：监管限制整条凭监管结果解除，
// 内部合规限制逐覆盖项由限制责任来源追加确认或解除决定，两者没有共用的写路径。
//
// 建立与决定都跑在调用方的事务里：决定先对限制头行取行锁，在锁下读回限制与它全部已有决定再判再追加，同一限制上的
// 两个决定串行。不发布意图——今天没有订阅者（ADR-0049），有订阅者时随它登记。

// ErrUnexpectedInternalRestrictionSave 说内部合规限制册交回了封闭集合以外的写入结果。
var ErrUnexpectedInternalRestrictionSave = errors.New("customs compliance: unexpected internal restriction save outcome")

// InternalRestrictionOutcome 是内部合规限制请求的应用处理结果。
type InternalRestrictionOutcome uint8

const (
	InternalRestrictionOutcomeInvalid InternalRestrictionOutcome = iota
	// InternalRestrictionEstablished 等三格是建立请求的结果。
	InternalRestrictionEstablished
	InternalRestrictionExisting
	InternalRestrictionContentConflict
	// InternalRestrictionDecided 起是决定请求的结果。
	InternalRestrictionDecided
	InternalRestrictionDecisionExisting
	InternalRestrictionDecisionContentConflict
	// InternalRestrictionNotResponsibleSource 说出具决定的来源不是限制的责任来源：确定性拒绝，决定不成立。
	InternalRestrictionNotResponsibleSource
	// InternalRestrictionDecisionRejected 说决定的其余要素不成立：覆盖项不在册、已解除、不需确认、证据缺、时间不对。
	InternalRestrictionDecisionRejected
	InternalRestrictionNotFound
	InternalRestrictionNotAccepted
	// InternalRestrictionUndecided 是册读写不动，没有形成结论。
	InternalRestrictionUndecided
)

func (outcome InternalRestrictionOutcome) String() string {
	switch outcome {
	case InternalRestrictionEstablished:
		return "ESTABLISHED"
	case InternalRestrictionExisting:
		return "EXISTING"
	case InternalRestrictionContentConflict:
		return "CONTENT_CONFLICT"
	case InternalRestrictionDecided:
		return "DECIDED"
	case InternalRestrictionDecisionExisting:
		return "DECISION_EXISTING"
	case InternalRestrictionDecisionContentConflict:
		return "DECISION_CONTENT_CONFLICT"
	case InternalRestrictionNotResponsibleSource:
		return "NOT_RESPONSIBLE_SOURCE"
	case InternalRestrictionDecisionRejected:
		return "DECISION_REJECTED"
	case InternalRestrictionNotFound:
		return "NOT_FOUND"
	case InternalRestrictionNotAccepted:
		return "NOT_ACCEPTED"
	case InternalRestrictionUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

// EstablishInternalRestrictionCommand 携带一次内部合规限制建立请求。
type EstablishInternalRestrictionCommand struct {
	TenantID domain.TenantID
	Spec     domain.InternalRestrictionSpec
}

// DecideInternalRestrictionCommand 携带一次确认或解除决定。决定标识由请求方给出。
type DecideInternalRestrictionCommand struct {
	TenantID      domain.TenantID
	RestrictionID domain.InternalRestrictionID
	Spec          domain.RestrictionDecisionSpec
}

type InternalRestrictionResult struct {
	outcome     InternalRestrictionOutcome
	restriction domain.InternalRestriction
	hasRecord   bool
}

func (result InternalRestrictionResult) Outcome() InternalRestrictionOutcome { return result.outcome }

// Restriction 只在已形成、已有与已决定的结果上给出，带已追加的全部决定。
func (result InternalRestrictionResult) Restriction() (domain.InternalRestriction, bool) {
	return result.restriction, result.hasRecord
}

type ManageInternalRestrictionDeps struct {
	Store ports.InternalRestrictionStore
}

type ManageInternalRestrictionHandler struct {
	deps ManageInternalRestrictionDeps
}

func NewManageInternalRestrictionHandler(deps ManageInternalRestrictionDeps) (*ManageInternalRestrictionHandler, error) {
	if deps.Store == nil {
		return nil, fmt.Errorf("%w: internal restriction store", ErrNilDependency)
	}
	return &ManageInternalRestrictionHandler{deps: deps}, nil
}

// Establish 建立内部合规限制：同标识同内容重放返已有，不重立；同标识异内容答内容冲突，不顶替——限制头建立后不改写，
// 改内容要新限制。
func (handler *ManageInternalRestrictionHandler) Establish(
	ctx context.Context,
	command EstablishInternalRestrictionCommand,
) (InternalRestrictionResult, error) {
	if blankTenant(command.TenantID) {
		return InternalRestrictionResult{outcome: InternalRestrictionNotAccepted}, nil
	}
	restriction, err := domain.EstablishInternalRestriction(command.Spec)
	if err != nil {
		return InternalRestrictionResult{outcome: InternalRestrictionNotAccepted}, nil
	}
	saved, err := handler.deps.Store.Save(ctx, command.TenantID, restriction)
	if err != nil {
		return InternalRestrictionResult{outcome: InternalRestrictionUndecided}, nil
	}
	switch saved {
	case ports.InternalRestrictionSaved:
		return InternalRestrictionResult{outcome: InternalRestrictionEstablished, restriction: restriction, hasRecord: true}, nil
	case ports.InternalRestrictionAlreadyRecorded:
		existing, found, err := handler.deps.Store.FindByID(ctx, command.TenantID, restriction.ID())
		if err != nil || !found {
			return InternalRestrictionResult{outcome: InternalRestrictionUndecided}, nil
		}
		if !existing.SameHeadAs(restriction) {
			return InternalRestrictionResult{outcome: InternalRestrictionContentConflict}, nil
		}
		return InternalRestrictionResult{outcome: InternalRestrictionExisting, restriction: existing, hasRecord: true}, nil
	default:
		return InternalRestrictionResult{}, fmt.Errorf("%w: %d", ErrUnexpectedInternalRestrictionSave, saved)
	}
}

// Decide 追加限制责任来源的确认或解除决定。重放先于来源与覆盖项的核对：同标识同内容总是答已有决定，哪怕覆盖项此后
// 已被别的决定改变；同标识异内容答决定内容冲突，不覆盖。
func (handler *ManageInternalRestrictionHandler) Decide(
	ctx context.Context,
	command DecideInternalRestrictionCommand,
) (InternalRestrictionResult, error) {
	if blankTenant(command.TenantID) || strings.TrimSpace(command.RestrictionID.String()) == "" {
		return InternalRestrictionResult{outcome: InternalRestrictionNotAccepted}, nil
	}
	decision, err := domain.NewRestrictionDecision(command.Spec)
	switch {
	case errors.Is(err, domain.ErrConditionNotEvidenced):
		return InternalRestrictionResult{outcome: InternalRestrictionDecisionRejected}, nil
	case err != nil:
		return InternalRestrictionResult{outcome: InternalRestrictionNotAccepted}, nil
	}

	restriction, found, err := handler.deps.Store.FindForDecision(ctx, command.TenantID, command.RestrictionID)
	if err != nil {
		return InternalRestrictionResult{outcome: InternalRestrictionUndecided}, nil
	}
	if !found {
		return InternalRestrictionResult{outcome: InternalRestrictionNotFound}, nil
	}
	if recorded, replay := restriction.Decision(decision.ID()); replay {
		if recorded.SameContentAs(decision) {
			return InternalRestrictionResult{outcome: InternalRestrictionDecisionExisting, restriction: restriction, hasRecord: true}, nil
		}
		return InternalRestrictionResult{outcome: InternalRestrictionDecisionContentConflict}, nil
	}

	decided, err := restriction.Decide(decision)
	switch {
	case errors.Is(err, domain.ErrNotTheResponsibleSource):
		return InternalRestrictionResult{outcome: InternalRestrictionNotResponsibleSource}, nil
	case errors.Is(err, domain.ErrCoverageNotInRestriction),
		errors.Is(err, domain.ErrCoverageNotDecidable),
		errors.Is(err, domain.ErrInvalidRestrictionDecision):
		return InternalRestrictionResult{outcome: InternalRestrictionDecisionRejected}, nil
	case err != nil:
		return InternalRestrictionResult{}, err
	}
	if err := handler.deps.Store.AppendDecision(ctx, command.TenantID, command.RestrictionID, decision); err != nil {
		return InternalRestrictionResult{outcome: InternalRestrictionUndecided}, nil
	}
	return InternalRestrictionResult{outcome: InternalRestrictionDecided, restriction: decided, hasRecord: true}, nil
}
