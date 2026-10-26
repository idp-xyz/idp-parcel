package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// MaxReaskBatch 是一次按册重问最多问几件。它约束的是一次请求的耗时与所有者那一侧的读压，不是业务取值：册上待形成的多于这个数，
// 运营人员再按一次，重问从最近一次问最早的那几件接着问；答复里的 Remaining 说这一次没问到的还有几件。
const MaxReaskBatch = 200

// ReaskScope 是按册重问问哪几件（ADR-0230 决定四、五）。
type ReaskScope uint8

const (
	ReaskScopeInvalid ReaskScope = iota
	// ReaskAllPending：册上待形成的全部（按最近一次问最早的在前，截到 limit）。运营人员此刻就要问，不看重问间隔。
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
//
// ReaskedBy 是按下这一次重问的操作者（「发行方#sub」，ADR-0303 决定五与复核记录），可缺：在场时先于逐件去问在问询册记一次按下
// （一次一行，不是逐件一行），逐件交执行器的命令不带它。它不进任何一格判断，重问的结果与答复形状不变，也不进任务。
type ReaskPendingDeliveryDispatchesCommand struct {
	TenantID  domain.TenantID
	Scope     ReaskScope
	Limit     int
	ReaskedBy string
}

// ReaskedDeliveryDispatch 是册上一件与它这一次的答复。
type ReaskedDeliveryDispatch struct {
	Segment domain.FulfillmentSegmentReference
	Object  domain.CarriedObjectReference
	Answer  TriggerDeliveryDispatchResult
}

type ReaskPendingDeliveryDispatchesResult struct {
	outcome   ReaskOutcome
	reason    ReaskUndecidedReason
	askedAt   time.Time
	cadence   ports.DeliveryDispatchReaskCadenceRecord
	hasDueBy  bool
	dueBy     time.Time
	items     []ReaskedDeliveryDispatch
	remaining int
}

func (result ReaskPendingDeliveryDispatchesResult) Outcome() ReaskOutcome { return result.outcome }

func (result ReaskPendingDeliveryDispatchesResult) UndecidedReason() ReaskUndecidedReason {
	return result.reason
}

// AskedAt 是这一次重问的时刻，也是各件那一拍的业务时间。
func (result ReaskPendingDeliveryDispatchesResult) AskedAt() time.Time { return result.askedAt }

// DueBy 只在问到期的、且间隔登记了时给出：最近一次问不晚于这一刻的算到期；Cadence 给出算它所凭的那一版。
func (result ReaskPendingDeliveryDispatchesResult) DueBy() (time.Time, ports.DeliveryDispatchReaskCadenceRecord, bool) {
	return result.dueBy, result.cadence, result.hasDueBy
}

func (result ReaskPendingDeliveryDispatchesResult) Items() []ReaskedDeliveryDispatch {
	return append([]ReaskedDeliveryDispatch(nil), result.items...)
}

// Remaining 是这一次没问到的件数：问之前按同一个筛法数出的待形成件数，减去这一次问到的。大于零就是件数上限截住了，再按一次接着问。
func (result ReaskPendingDeliveryDispatchesResult) Remaining() int { return result.remaining }

// ReaskFailureMarker 给一件记下「这一次问过」，自成一笔事务。按册重问只在执行器对那一件给不出答复时调它：那一拍的事务已整笔回滚，
// 执行器里的那一笔「问过」也跟着没了，不另记一次，那一件就一直停在队头。
type ReaskFailureMarker interface {
	MarkAsked(ctx context.Context, key ports.DeliveryDispatchPendingKey, at time.Time) error
}

// ErrReaskFailureMarkerNotWired 说明装配没交 Failed：缺了它，一件持续给不出答复的派送会让每一次重问都停在它身上。
var ErrReaskFailureMarkerNotWired = errors.New("transport fulfillment: reask failure marker is not wired")

// ReaskPendingDeliveryDispatchesDeps 全部必备。Trigger 是派送发起执行器，装配点交的那一只每问一件自成一笔事务：一件写不动只回滚
// 那一件，已问过的各件留在册上。Failed 是另起一笔事务的「问过」记录口，见 ReaskFailureMarker。
type ReaskPendingDeliveryDispatchesDeps struct {
	Pending  ports.DeliveryDispatchPendingQueue
	Cadences ports.DeliveryDispatchReaskCadenceReader
	Trigger  DeliveryDispatchTrigger
	Failed   ReaskFailureMarker
	Clock    ports.Clock
	// Asks 是问询册的写口，只在命令带了 ReaskedBy 时用到，装配点交的那一只每记一次自成一笔事务（先于逐件去问，一件问不动也不让这一次
	// 按下无处可查）。缺了它而命令带了操作者，不问（ErrDeliveryDispatchAskLedgerNotWired）。
	Asks ports.DeliveryDispatchAskLedger
}

// ReaskPendingDeliveryDispatchesHandler 是按册重问（ADR-0230 决定四）：把派送待形成册上的那几件逐件交回派送发起执行器，再问一次
// 所有者。它不另判任何一格——派送段、在场、入场依据、查在册、取件都是执行器的门，册怎么了结也由执行器那一拍写。所以重问是幂等的：
// 已形成的答`已在册`，从不另立第二项；仍缺的刷新最近一次答复时刻；对象离场的作罢；答复动不了册的那几件也记下这一次问过，排到
// 队尾，后面的那几件下一次轮得到。
//
// 不带期望交接：重问问的是对象此刻在这个段里的那条参与，不替哪一次交接说话。不是 ADR-0216 否掉的「按拍批量发起」：问的只是册上
// 已知待形成的那几件，不扫段找对象。
type ReaskPendingDeliveryDispatchesHandler struct {
	deps ReaskPendingDeliveryDispatchesDeps
}

func NewReaskPendingDeliveryDispatchesHandler(deps ReaskPendingDeliveryDispatchesDeps) *ReaskPendingDeliveryDispatchesHandler {
	return &ReaskPendingDeliveryDispatchesHandler{deps: deps}
}

// Reask 问一次：受理 → 问到期的先取间隔（没登记答未配置）→ 列册 → 逐件交执行器。某一件执行器给不出答复（错误）就停在那一件，交回
// 带段与对象的错误：前面各件已各自提交；那一件另起一笔事务记下这一次问过、排到队尾，再按一次先问后面的。那一笔也记不下（库面整个
// 读写不动）时，它仍停在查阅页队头，修好之前每一次重问都会先停在它身上。
func (handler *ReaskPendingDeliveryDispatchesHandler) Reask(
	ctx context.Context,
	command ReaskPendingDeliveryDispatchesCommand,
) (ReaskPendingDeliveryDispatchesResult, error) {
	if handler.deps.Failed == nil {
		return ReaskPendingDeliveryDispatchesResult{}, ErrReaskFailureMarkerNotWired
	}
	if strings.TrimSpace(command.TenantID.String()) == "" || command.Scope == ReaskScopeInvalid ||
		command.Limit < 1 || command.Limit > MaxReaskBatch {
		return ReaskPendingDeliveryDispatchesResult{outcome: ReaskNotAccepted}, nil
	}
	asker, err := domain.OptionalRegistrantReference(command.ReaskedBy)
	if err != nil {
		return ReaskPendingDeliveryDispatchesResult{outcome: ReaskNotAccepted}, nil
	}
	result := ReaskPendingDeliveryDispatchesResult{askedAt: handler.deps.Clock.Now()}
	if asker.Present() {
		if handler.deps.Asks == nil {
			return ReaskPendingDeliveryDispatchesResult{}, ErrDeliveryDispatchAskLedgerNotWired
		}
		if err := handler.deps.Asks.RecordAsk(ctx, ports.DeliveryDispatchAskRecord{
			TenantID: command.TenantID, Kind: ports.DeliveryDispatchAskReask, AskedBy: asker,
			AskedAt: result.askedAt, Scope: command.Scope.String(), Limit: command.Limit,
		}); err != nil {
			return ReaskPendingDeliveryDispatchesResult{}, fmt.Errorf("%w: %v", ErrDeliveryDispatchAskUnrecorded, err)
		}
	}
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
	candidates, err := handler.deps.Pending.CountPending(ctx, command.TenantID, result.dueBy)
	if err != nil {
		return ReaskPendingDeliveryDispatchesResult{outcome: ReaskUndecided, reason: ReaskPendingRegisterUnavailable}, nil
	}
	pending, err := handler.deps.Pending.ListPending(ctx, command.TenantID, result.dueBy, command.Limit)
	if err != nil {
		return ReaskPendingDeliveryDispatchesResult{outcome: ReaskUndecided, reason: ReaskPendingRegisterUnavailable}, nil
	}
	result.remaining = max(candidates-len(pending), 0)
	for _, entry := range pending {
		answer, err := handler.deps.Trigger.Trigger(ctx, TriggerDeliveryDispatchCommand{
			TenantID:   command.TenantID,
			Segment:    entry.Key.Segment.String(),
			Object:     entry.Key.Object.String(),
			OccurredAt: result.askedAt,
		})
		if err != nil {
			marked := handler.deps.Failed.MarkAsked(ctx, entry.Key, result.askedAt)
			return ReaskPendingDeliveryDispatchesResult{}, fmt.Errorf("reask delivery dispatch %s/%s: %w",
				entry.Key.Segment, entry.Key.Object, errors.Join(err, marked))
		}
		result.items = append(result.items, ReaskedDeliveryDispatch{Segment: entry.Key.Segment, Object: entry.Key.Object, Answer: answer})
	}
	result.outcome = ReaskAsked
	return result, nil
}
