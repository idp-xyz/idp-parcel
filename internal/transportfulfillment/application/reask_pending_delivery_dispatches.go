package application

import (
	"context"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// MaxReaskBatch 是一次按册重问最多问几件。它约束的是一次请求的耗时与所有者那一侧的读压，不是业务取值：册上待形成的多于这个数，
// 运营人员再按一次，重问从最近一次答复最早的那几件接着问。
const MaxReaskBatch = 200

// ReaskScope 是按册重问问哪几件（ADR-0230 决定四、五）。
type ReaskScope uint8

const (
	ReaskScopeInvalid ReaskScope = iota
	// ReaskAllPending：册上待形成的全部（按最近一次答复最早的在前，截到 limit）。运营人员此刻就要问，不看重问间隔。
	ReaskAllPending
	// ReaskDuePending：只问按本租户重问间隔已到期的那几件。间隔没登记就没有到期这回事，答`未配置`。
	ReaskDuePending
)

func (scope ReaskScope) String() string {
	switch scope {
	case ReaskAllPending:
		return "ALL"
	case ReaskDuePending:
		return "DUE"
	default:
		return ""
	}
}

// ParseReaskScope 按字面认问法；不在封闭集里答零值。
func ParseReaskScope(raw string) ReaskScope {
	for _, scope := range []ReaskScope{ReaskAllPending, ReaskDuePending} {
		if scope.String() == strings.TrimSpace(raw) {
			return scope
		}
	}
	return ReaskScopeInvalid
}

// ReaskOutcome 是按册重问这一次的结果代数。逐件的答复在 Items 里，各是执行器那一拍的完整答复。
type ReaskOutcome uint8

const (
	ReaskOutcomeInvalid ReaskOutcome = iota
	// ReaskAsked：问过了（可能零件：册上没有待形成的、或没有到期的）。
	ReaskAsked
	// ReaskCadenceNotConfigured：问的是到期的，而本租户没登记重问间隔。一件也不问——不替租户选一个间隔。
	ReaskCadenceNotConfigured
	ReaskNotAccepted
	// ReaskUndecided：重问间隔册或派送待形成册读不动，还说不出该问哪几件。
	ReaskUndecided
)

func (outcome ReaskOutcome) String() string {
	switch outcome {
	case ReaskAsked:
		return "PENDING_DELIVERY_DISPATCHES_REASKED"
	case ReaskCadenceNotConfigured:
		return "REASK_CADENCE_NOT_CONFIGURED"
	case ReaskNotAccepted:
		return "INPUT_NOT_ACCEPTED"
	case ReaskUndecided:
		return "REASK_UNDECIDED"
	default:
		return ""
	}
}

// ReaskUndecidedReason 指名`未决`停在哪一本册上。
type ReaskUndecidedReason uint8

const (
	ReaskUndecidedReasonNone ReaskUndecidedReason = iota
	ReaskCadenceRegisterUnavailable
	ReaskPendingRegisterUnavailable
)

func (reason ReaskUndecidedReason) String() string {
	switch reason {
	case ReaskCadenceRegisterUnavailable:
		return "REASK_CADENCE_REGISTER_UNAVAILABLE"
	case ReaskPendingRegisterUnavailable:
		return "PENDING_REGISTER_UNAVAILABLE"
	default:
		return ""
	}
}

// ReaskPendingDeliveryDispatchesCommand：租户取认证结果；问法与一次最多问几件由请求给。
type ReaskPendingDeliveryDispatchesCommand struct {
	TenantID domain.TenantID
	Scope    ReaskScope
	Limit    int
}

// ReaskedDeliveryDispatch 是册上一件与它这一次的答复。
type ReaskedDeliveryDispatch struct {
	Segment domain.FulfillmentSegmentReference
	Object  domain.CarriedObjectReference
	Answer  TriggerDeliveryDispatchResult
}

type ReaskPendingDeliveryDispatchesResult struct {
	outcome  ReaskOutcome
	reason   ReaskUndecidedReason
	askedAt  time.Time
	cadence  ports.DeliveryDispatchReaskCadenceRecord
	hasDueBy bool
	dueBy    time.Time
	items    []ReaskedDeliveryDispatch
}

func (result ReaskPendingDeliveryDispatchesResult) Outcome() ReaskOutcome { return result.outcome }

func (result ReaskPendingDeliveryDispatchesResult) UndecidedReason() ReaskUndecidedReason {
	return result.reason
}

// AskedAt 是这一次重问的时刻，也是各件那一拍的业务时间。
func (result ReaskPendingDeliveryDispatchesResult) AskedAt() time.Time { return result.askedAt }

// DueBy 只在问到期的、且间隔登记了时给出：最近一次答复不晚于这一刻的算到期；Cadence 给出算它所凭的那一版。
func (result ReaskPendingDeliveryDispatchesResult) DueBy() (time.Time, ports.DeliveryDispatchReaskCadenceRecord, bool) {
	return result.dueBy, result.cadence, result.hasDueBy
}

func (result ReaskPendingDeliveryDispatchesResult) Items() []ReaskedDeliveryDispatch {
	return append([]ReaskedDeliveryDispatch(nil), result.items...)
}

// ReaskPendingDeliveryDispatchesDeps 全部必备。Trigger 是派送发起执行器，装配点交的那一只每问一件自成一笔事务：一件写不动只回滚
// 那一件，已问过的各件留在册上。
type ReaskPendingDeliveryDispatchesDeps struct {
	Pending  ports.DeliveryDispatchPendingQueue
	Cadences ports.DeliveryDispatchReaskCadenceReader
	Trigger  DeliveryDispatchTrigger
	Clock    ports.Clock
}

// ReaskPendingDeliveryDispatchesHandler 是按册重问（ADR-0230 决定四）：把派送待形成册上的那几件逐件交回派送发起执行器，再问一次
// 所有者。它不另判任何一格——派送段、在场、入场依据、查在册、取件都是执行器的门，册怎么了结也由执行器那一拍写。所以重问是幂等的：
// 已形成的答`已在册`，从不另立第二项；仍缺的刷新最近一次答复时刻；对象离场的作罢。
//
// 不带期望交接：重问问的是对象此刻在这个段里的那条参与，不替哪一次交接说话。不是 ADR-0216 否掉的「按拍批量发起」：问的只是册上
// 已知待形成的那几件，不扫段找对象。
type ReaskPendingDeliveryDispatchesHandler struct {
	deps ReaskPendingDeliveryDispatchesDeps
}

func NewReaskPendingDeliveryDispatchesHandler(deps ReaskPendingDeliveryDispatchesDeps) *ReaskPendingDeliveryDispatchesHandler {
	return &ReaskPendingDeliveryDispatchesHandler{deps: deps}
}

// Reask 问一次：受理 → 问到期的先取间隔（没登记答未配置）→ 列册 → 逐件交执行器。某一件执行器给不出答复（错误）就停在那一件交回
// 错误：前面各件已各自提交，原样再按一次从册上仍待形成的接着问。
func (handler *ReaskPendingDeliveryDispatchesHandler) Reask(
	ctx context.Context,
	command ReaskPendingDeliveryDispatchesCommand,
) (ReaskPendingDeliveryDispatchesResult, error) {
	if strings.TrimSpace(command.TenantID.String()) == "" || command.Scope == ReaskScopeInvalid ||
		command.Limit < 1 || command.Limit > MaxReaskBatch {
		return ReaskPendingDeliveryDispatchesResult{outcome: ReaskNotAccepted}, nil
	}
	result := ReaskPendingDeliveryDispatchesResult{askedAt: handler.deps.Clock.Now()}
	if command.Scope == ReaskDuePending {
		cadence, found, err := handler.deps.Cadences.CurrentCadence(ctx, command.TenantID)
		if err != nil {
			return ReaskPendingDeliveryDispatchesResult{outcome: ReaskUndecided, reason: ReaskCadenceRegisterUnavailable}, nil
		}
		if !found {
			return ReaskPendingDeliveryDispatchesResult{outcome: ReaskCadenceNotConfigured, askedAt: result.askedAt}, nil
		}
		result.cadence, result.hasDueBy, result.dueBy = cadence, true, cadence.Cadence.DueBy(result.askedAt)
	}
	pending, err := handler.deps.Pending.ListPending(ctx, command.TenantID, result.dueBy, command.Limit)
	if err != nil {
		return ReaskPendingDeliveryDispatchesResult{outcome: ReaskUndecided, reason: ReaskPendingRegisterUnavailable}, nil
	}
	for _, entry := range pending {
		answer, err := handler.deps.Trigger.Trigger(ctx, TriggerDeliveryDispatchCommand{
			TenantID:   command.TenantID,
			Segment:    entry.Key.Segment.String(),
			Object:     entry.Key.Object.String(),
			OccurredAt: result.askedAt,
		})
		if err != nil {
			return ReaskPendingDeliveryDispatchesResult{}, err
		}
		result.items = append(result.items, ReaskedDeliveryDispatch{Segment: entry.Key.Segment, Object: entry.Key.Object, Answer: answer})
	}
	result.outcome = ReaskAsked
	return result, nil
}
