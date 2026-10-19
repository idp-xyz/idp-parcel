package application

import (
	"context"
	"strings"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// DispatchOnHandoverOutcome 是「一封交接信封到了 → 按对象此刻在场的段发起一拍派送」的结果代数（ADR-0216 决定一）。
//
// 只有`已交给执行器`一格要往下看执行器的答复（Trigger）；其余各格在交给执行器之前就答了。交接不是`已交接`、对象此刻
// 不在任何段里在场，都是这封信不构成触发事实——CONTEXT 把内部触发限定为「凭`已交接`进入派送段」，拒收与待确认没有让
// 控制移入接收方，对象也就没进新段。两格分开，因为人去查的地方不同：一个看交接裁决，一个看段登记册。
type DispatchOnHandoverOutcome uint8

const (
	DispatchOnHandoverOutcomeInvalid DispatchOnHandoverOutcome = iota
	DispatchOnHandoverTriggered
	DispatchOnHandoverNotHandedOver
	DispatchOnHandoverNoActiveSegment
	DispatchOnHandoverNotAccepted
	DispatchOnHandoverUndecided
)

func (outcome DispatchOnHandoverOutcome) String() string {
	switch outcome {
	case DispatchOnHandoverTriggered:
		return "DISPATCH_TRIGGERED"
	case DispatchOnHandoverNotHandedOver:
		return "HANDOVER_NOT_HANDED_OVER"
	case DispatchOnHandoverNoActiveSegment:
		return "NO_ACTIVE_SEGMENT"
	case DispatchOnHandoverNotAccepted:
		return "INPUT_NOT_ACCEPTED"
	case DispatchOnHandoverUndecided:
		return "DISPATCH_ON_HANDOVER_UNDECIDED"
	default:
		return ""
	}
}

// DispatchOnHandoverUndecidedReason 指名交给执行器之前停在哪一口。交接读不回单列一格：交接与信封在登记那一笔同时
// 落库，读不回只剩可见性滞后，与登记册本身读不动的续办不同。
type DispatchOnHandoverUndecidedReason uint8

const (
	DispatchOnHandoverUndecidedReasonNone DispatchOnHandoverUndecidedReason = iota
	DispatchOnHandoverHandoverRegistryUnavailable
	DispatchOnHandoverHandoverNotVisible
	DispatchOnHandoverSegmentRegistryUnavailable
)

func (reason DispatchOnHandoverUndecidedReason) String() string {
	switch reason {
	case DispatchOnHandoverHandoverRegistryUnavailable:
		return "HANDOVER_REGISTRY_UNAVAILABLE"
	case DispatchOnHandoverHandoverNotVisible:
		return "HANDOVER_NOT_VISIBLE"
	case DispatchOnHandoverSegmentRegistryUnavailable:
		return "SEGMENT_REGISTRY_UNAVAILABLE"
	default:
		return ""
	}
}

// DispatchOnHandoverDisposition 是这一拍的答复对那封交接信封意味着什么（ADR-0216 决定三）。判据是「原样重投会不会改变
// 结果」：会变的才重投；不会变的分两种——业务答案入账，本上下文自己的缺陷响亮失败而不入账。
//
// 放在应用层而不放在消费门：哪一格是业务答案、哪一格是等得来的依赖、哪一格是装配缺件，要认得执行器的每一格才分得出，
// 那是本上下文的知识；消费门只把三格译成 inbox 的三种动作。
type DispatchOnHandoverDisposition uint8

const (
	DispatchOnHandoverDispositionInvalid DispatchOnHandoverDisposition = iota
	// DispatchOnHandoverSettled：入账不重投。含`要求缺失`——所有者答「没有」不会因重投而变，任务保持待形成，补齐之后经
	// 补拍口再发起（ADR-0216 决定三；越权风险点「要求缺失不会自愈」）。
	DispatchOnHandoverSettled
	// DispatchOnHandoverRetry：等一个读得到的依赖，重投可能就过。
	DispatchOnHandoverRetry
	// DispatchOnHandoverDefect：本上下文自己交出的东西过不了自己的门，或装配缺一条缝；重投一万次都一样，不入账，让它响。
	DispatchOnHandoverDefect
)

func (disposition DispatchOnHandoverDisposition) String() string {
	switch disposition {
	case DispatchOnHandoverSettled:
		return "SETTLED"
	case DispatchOnHandoverRetry:
		return "RETRY"
	case DispatchOnHandoverDefect:
		return "DEFECT"
	default:
		return ""
	}
}

// TriggerDispatchOnHandoverCommand 是交接信封上的判断键：哪个租户的哪个对象，在哪个交接范围的哪一版。版本是键的一维，
// 更正是新版本新登记，各有自己的信封。
type TriggerDispatchOnHandoverCommand struct {
	TenantID domain.TenantID
	Object   string
	Scope    string
	Version  string
}

type TriggerDispatchOnHandoverResult struct {
	outcome DispatchOnHandoverOutcome
	reason  DispatchOnHandoverUndecidedReason
	trigger TriggerDeliveryDispatchResult
}

func (result TriggerDispatchOnHandoverResult) Outcome() DispatchOnHandoverOutcome {
	return result.outcome
}

// UndecidedReason 只在本编排自己`未决`时非零；执行器的未决理由在 Trigger 里。
func (result TriggerDispatchOnHandoverResult) UndecidedReason() DispatchOnHandoverUndecidedReason {
	return result.reason
}

// Trigger 只在`已交给执行器`时有意义，交回执行器这一拍的完整答复。
func (result TriggerDispatchOnHandoverResult) Trigger() TriggerDeliveryDispatchResult {
	return result.trigger
}

// Disposition 把本编排与执行器两层答复折成对信封的处置（ADR-0216 决定三）。
//
// 执行器各格逐一列出、不留「其余一律入账」的默认：执行器日后多一格时落进 Defect 响起来，而不是悄悄入账吞掉——那正是
// 「要求缺失」这种业务答案与真缺陷要分开的理由。执行器的`未决`再分一层：三条缝没接线是装配缺件，原样重投不会接上，
// 按缺陷处置；其余未决理由都是某一口此刻读不到，重投。
func (result TriggerDispatchOnHandoverResult) Disposition() DispatchOnHandoverDisposition {
	switch result.outcome {
	case DispatchOnHandoverNotHandedOver, DispatchOnHandoverNoActiveSegment:
		return DispatchOnHandoverSettled
	case DispatchOnHandoverUndecided:
		return DispatchOnHandoverRetry
	case DispatchOnHandoverTriggered:
		return triggeredDisposition(result.trigger)
	default:
		return DispatchOnHandoverDefect
	}
}

func triggeredDisposition(triggered TriggerDeliveryDispatchResult) DispatchOnHandoverDisposition {
	switch triggered.Outcome() {
	case DeliveryDispatchTaskFormed,
		DeliveryDispatchTaskAlreadyFormed,
		DeliveryDispatchNotTriggered,
		DeliveryDispatchRequirementMissing,
		DeliveryDispatchTaskReferenceTaken:
		return DispatchOnHandoverSettled
	case DeliveryDispatchUndecided:
		switch triggered.UndecidedReason() {
		case DeliveryPlaceSourceNotWired, DeliveryWindowSourceNotWired, DeliveryConditionSourceNotWired:
			return DispatchOnHandoverDefect
		default:
			return DispatchOnHandoverRetry
		}
	default:
		return DispatchOnHandoverDefect
	}
}

// TransportHandoverFinder 是交接登记册按键取一版的那一口。本编排对交接只读，类型上就只给读口。
type TransportHandoverFinder interface {
	FindByKey(ctx context.Context, key ports.TransportHandoverKey) (ports.TransportHandoverRecord, bool, error)
}

// ActiveSegmentFinder 是段登记册按对象问「此刻在哪些段里在场」的那一口。
type ActiveSegmentFinder interface {
	FindActiveSegments(ctx context.Context, tenant domain.TenantID, object domain.CarriedObjectReference) ([]ports.FulfillmentSegmentKey, error)
}

// DeliveryDispatchTrigger 是触发执行器那一口，真实装配接 TriggerDeliveryDispatchHandler。
type DeliveryDispatchTrigger interface {
	Trigger(ctx context.Context, command TriggerDeliveryDispatchCommand) (TriggerDeliveryDispatchResult, error)
}

// TriggerDispatchOnHandoverDeps 全部必备。Clock 给的是任务的成立时间：这一拍在派发进程里形成任务，任务就是这一刻成立的
// （ADR-0216 决定二）。
type TriggerDispatchOnHandoverDeps struct {
	Handovers TransportHandoverFinder
	Segments  ActiveSegmentFinder
	Trigger   DeliveryDispatchTrigger
	Clock     ports.Clock
}

// TriggerDispatchOnHandoverHandler 是派送发起的进程内入口（ADR-0216）：TF 自己消费自己的交接信封，逐对象调既有触发执行器。
// 它不另判派送段、在场与入场依据——那些都是执行器的门，在这里再判一遍就是一条规则两处定义；本编排只答执行器问不到的两件：
// 这封信是不是`已交接`，对象此刻在哪个段里。
type TriggerDispatchOnHandoverHandler struct {
	deps TriggerDispatchOnHandoverDeps
}

func NewTriggerDispatchOnHandoverHandler(deps TriggerDispatchOnHandoverDeps) *TriggerDispatchOnHandoverHandler {
	return &TriggerDispatchOnHandoverHandler{deps: deps}
}

// Trigger 跑一拍：受理键 → 按键读回那一版交接（须是`已交接`）→ 按对象找此刻在场的段 → 交执行器。
//
// 段取「此刻在场的段」而不是这次交接立进的段：交接记录不带段，段号只在段登记册的参与上；信封晚到、对象已经又被下一次
// 交接带走时，此刻在场的段才是该派送的那一段，前一段的参与已离场、照 ADR-0114 决定二本就不触发。多于一个在场段是
// 库面不一致，不挑一个，与结束参与那一侧同一个错误。
func (handler *TriggerDispatchOnHandoverHandler) Trigger(
	ctx context.Context,
	command TriggerDispatchOnHandoverCommand,
) (TriggerDispatchOnHandoverResult, error) {
	key, accepted := handoverKeyOf(command)
	if !accepted {
		return TriggerDispatchOnHandoverResult{outcome: DispatchOnHandoverNotAccepted}, nil
	}
	record, found, err := handler.deps.Handovers.FindByKey(ctx, key)
	if err != nil {
		return dispatchOnHandoverUndecided(DispatchOnHandoverHandoverRegistryUnavailable), nil
	}
	if !found {
		return dispatchOnHandoverUndecided(DispatchOnHandoverHandoverNotVisible), nil
	}
	if !record.Handover.TransfersControl() {
		return TriggerDispatchOnHandoverResult{outcome: DispatchOnHandoverNotHandedOver}, nil
	}

	active, err := handler.deps.Segments.FindActiveSegments(ctx, key.TenantID, key.Object)
	if err != nil {
		return dispatchOnHandoverUndecided(DispatchOnHandoverSegmentRegistryUnavailable), nil
	}
	switch len(active) {
	case 0:
		return TriggerDispatchOnHandoverResult{outcome: DispatchOnHandoverNoActiveSegment}, nil
	case 1:
	default:
		return TriggerDispatchOnHandoverResult{}, ErrObjectActiveInSeveralSegments
	}

	triggered, err := handler.deps.Trigger.Trigger(ctx, TriggerDeliveryDispatchCommand{
		TenantID:   key.TenantID,
		Segment:    active[0].Segment.String(),
		Object:     key.Object.String(),
		OccurredAt: handler.deps.Clock.Now(),
	})
	if err != nil {
		return TriggerDispatchOnHandoverResult{}, err
	}
	return TriggerDispatchOnHandoverResult{outcome: DispatchOnHandoverTriggered, trigger: triggered}, nil
}

func handoverKeyOf(command TriggerDispatchOnHandoverCommand) (ports.TransportHandoverKey, bool) {
	if strings.TrimSpace(command.TenantID.String()) == "" {
		return ports.TransportHandoverKey{}, false
	}
	object, err := domain.NewCarriedObjectReference(command.Object)
	if err != nil {
		return ports.TransportHandoverKey{}, false
	}
	scope, err := domain.NewHandoverScopeReference(command.Scope)
	if err != nil {
		return ports.TransportHandoverKey{}, false
	}
	version, err := domain.NewHandoverResultVersion(command.Version)
	if err != nil {
		return ports.TransportHandoverKey{}, false
	}
	return ports.TransportHandoverKey{TenantID: command.TenantID, Object: object, Scope: scope, Version: version}, true
}

func dispatchOnHandoverUndecided(reason DispatchOnHandoverUndecidedReason) TriggerDispatchOnHandoverResult {
	return TriggerDispatchOnHandoverResult{outcome: DispatchOnHandoverUndecided, reason: reason}
}
