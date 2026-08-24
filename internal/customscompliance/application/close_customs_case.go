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

// ErrUnexpectedClosureSave 说明关闭库交回了封闭集合以外的写入结果。
var ErrUnexpectedClosureSave = errors.New("customs compliance: unexpected case closure save outcome")

// CloseCaseOutcome 是关务案件关闭请求的应用处理结果。
type CloseCaseOutcome uint8

const (
	CloseCaseOutcomeInvalid CloseCaseOutcome = iota
	CaseClosed
	CaseAlreadyClosed
	ClosureBlocked
	CloseCaseNotAccepted
	CloseCaseUndecided
)

func (outcome CloseCaseOutcome) String() string {
	switch outcome {
	case CaseClosed:
		return "CLOSED"
	case CaseAlreadyClosed:
		return "ALREADY_CLOSED"
	case ClosureBlocked:
		return "BLOCKED"
	case CloseCaseNotAccepted:
		return "NOT_ACCEPTED"
	case CloseCaseUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

// CloseCaseReason 指名关闭停在哪一步。
type CloseCaseReason uint8

const (
	CloseCaseReasonNone CloseCaseReason = iota
	ObligationInventoryUnconfigured
	ObligationInventoryUnavailable
	ClosureStoreUnavailable
)

func (reason CloseCaseReason) String() string {
	switch reason {
	case ObligationInventoryUnconfigured:
		return "OBLIGATION_INVENTORY_UNCONFIGURED"
	case ObligationInventoryUnavailable:
		return "OBLIGATION_INVENTORY_UNAVAILABLE"
	case ClosureStoreUnavailable:
		return "CLOSURE_STORE_UNAVAILABLE"
	default:
		return ""
	}
}

// CloseCustomsCaseCommand 携带一次关闭请求：案件、业务截点与作出决定的责任角色。
type CloseCustomsCaseCommand struct {
	TenantID  domain.TenantID
	CaseRef   domain.CustomsCaseID
	CutoffAt  time.Time
	DecidedBy string
}

type CloseCustomsCaseResult struct {
	outcome    CloseCaseOutcome
	closure    *domain.CustomsCaseClosure
	unresolved []domain.ClosureObligationItem
	reason     CloseCaseReason
	handoffRef string
}

func (result CloseCustomsCaseResult) Outcome() CloseCaseOutcome {
	return result.outcome
}

func (result CloseCustomsCaseResult) Closure() (*domain.CustomsCaseClosure, bool) {
	return result.closure, result.closure != nil
}

// Unresolved 只在关闭被阻止时给出——被哪些未解决义务挡着一目了然，恢复动作是逐项
// 终结或有效承接，不是重试。
func (result CloseCustomsCaseResult) Unresolved() []domain.ClosureObligationItem {
	return append([]domain.ClosureObligationItem(nil), result.unresolved...)
}

func (result CloseCustomsCaseResult) UndecidedReason() CloseCaseReason {
	return result.reason
}

// HandoffReference 非空说明关闭已入册但意图还没交出去，重放会重发同一份。
func (result CloseCustomsCaseResult) HandoffReference() string {
	return result.handoffRef
}

type CloseCustomsCaseDeps struct {
	Inventory  ports.ObligationInventoryView
	Store      ports.CaseClosureStore
	Downstream ports.CaseClosureHandoff
	Clock      ports.Clock
}

type CloseCustomsCaseHandler struct {
	deps CloseCustomsCaseDeps
}

func NewCloseCustomsCaseHandler(deps CloseCustomsCaseDeps) *CloseCustomsCaseHandler {
	return &CloseCustomsCaseHandler{deps: deps}
}

// Handle 把一次关闭请求推进到整案一次的关闭决定：已关案件重放返原关闭（一案至多一份
// 关闭记录，重开走 Reopen 不走这里）→ 义务清单读取（目录未登记未决，不是「没有义务
// 所以可关」）→ VerifyClosure 逐项核对 → 任一未解决阻止关闭带清单（业务负向，恢复
// 动作是逐项处置不是重试）→ CloseCase 决定 → 提交与意图。
func (handler *CloseCustomsCaseHandler) Handle(
	ctx context.Context,
	command CloseCustomsCaseCommand,
) (CloseCustomsCaseResult, error) {
	if strings.TrimSpace(command.TenantID.String()) == "" ||
		strings.TrimSpace(command.CaseRef.String()) == "" ||
		command.CutoffAt.IsZero() ||
		strings.TrimSpace(command.DecidedBy) == "" {
		return CloseCustomsCaseResult{outcome: CloseCaseNotAccepted}, nil
	}

	existing, found, err := handler.deps.Store.FindByCase(ctx, command.TenantID, command.CaseRef)
	if err != nil {
		return CloseCustomsCaseResult{outcome: CloseCaseUndecided, reason: ClosureStoreUnavailable}, nil
	}
	if found {
		return handler.alreadyClosed(ctx, command.TenantID, existing), nil
	}

	items, configured, err := handler.deps.Inventory.LoadObligationItems(
		ctx, command.TenantID, command.CaseRef, command.CutoffAt)
	if err != nil {
		return CloseCustomsCaseResult{outcome: CloseCaseUndecided, reason: ObligationInventoryUnavailable}, nil
	}
	if !configured {
		return CloseCustomsCaseResult{outcome: CloseCaseUndecided, reason: ObligationInventoryUnconfigured}, nil
	}

	verification, err := domain.VerifyClosure(
		command.CaseRef, command.CutoffAt, items, handler.deps.Clock.Now())
	if err != nil {
		return CloseCustomsCaseResult{outcome: CloseCaseNotAccepted}, nil
	}
	if !verification.Closable() {
		return CloseCustomsCaseResult{
			outcome:    ClosureBlocked,
			unresolved: verification.UnresolvedItems(),
		}, nil
	}

	closure, err := domain.CloseCase(verification, command.DecidedBy, handler.deps.Clock.Now())
	if err != nil {
		return CloseCustomsCaseResult{outcome: CloseCaseNotAccepted}, nil
	}

	saved, err := handler.deps.Store.Save(ctx, command.TenantID, closure)
	if err != nil {
		return CloseCustomsCaseResult{outcome: CloseCaseUndecided, reason: ClosureStoreUnavailable}, nil
	}
	switch saved {
	case ports.CaseClosureSaved:
		result := CloseCustomsCaseResult{outcome: CaseClosed, closure: closure}
		result.handoffRef = handler.handOffClosure(ctx, command.TenantID, closure)
		return result, nil
	case ports.CaseClosureAlreadyRecorded:
		winner, found, err := handler.deps.Store.FindByCase(ctx, command.TenantID, command.CaseRef)
		if err != nil || !found {
			return CloseCustomsCaseResult{outcome: CloseCaseUndecided, reason: ClosureStoreUnavailable}, nil
		}
		return handler.alreadyClosed(ctx, command.TenantID, winner), nil
	default:
		return CloseCustomsCaseResult{}, fmt.Errorf("%w: %d", ErrUnexpectedClosureSave, saved)
	}
}

// alreadyClosed 按已有关闭作答并重发同一份意图。
func (handler *CloseCustomsCaseHandler) alreadyClosed(
	ctx context.Context,
	tenant domain.TenantID,
	closure *domain.CustomsCaseClosure,
) CloseCustomsCaseResult {
	result := CloseCustomsCaseResult{outcome: CaseAlreadyClosed, closure: closure}
	result.handoffRef = handler.handOffClosure(ctx, tenant, closure)
	return result
}

// handOffClosure 交发布意图。失败不翻关闭，留续办引用重发同一份。
func (handler *CloseCustomsCaseHandler) handOffClosure(
	ctx context.Context,
	tenant domain.TenantID,
	closure *domain.CustomsCaseClosure,
) string {
	if err := handler.deps.Downstream.HandOffClosure(ctx, ports.CaseClosureHandoffIntent{
		TenantID: tenant,
		Closure:  closure,
	}); err == nil {
		return ""
	}
	return "CONT-CLOSURE/" + closure.CaseRef().String()
}
