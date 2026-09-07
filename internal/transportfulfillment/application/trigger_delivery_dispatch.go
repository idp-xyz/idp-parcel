package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// ErrUnexpectedDispatchTaskOutcome 说明既有任务口交回了封闭集合以外的结果。
var ErrUnexpectedDispatchTaskOutcome = errors.New("transport fulfillment: unexpected dispatch task outcome")

// DeliveryDispatchTriggerOutcome 是「对象凭`已交接`进入派送段 → 形成末端派送任务」这一拍的结果代数（ADR-0114 决定二）。
//
// `已形成`与`已在册`分开：后者是同一拍重跑，任务引用确定性铸出撞上既有任务，交回它而不重建。`不是触发事实`是领域答案
// ——段不是派送段、对象不在段里或已不在场、进段凭的不是`已交接`，重跑不会变，哪一格由 DeliveryTriggerRefusal 点名。
// `要求缺失`是所有者答「没有」：任务保持待形成，不填默认（ADR-0114 决定三）。`未决`是缝没接或读不到：原样重跑本拍就
// 可能过，理由里点名是哪一条缝。
type DeliveryDispatchTriggerOutcome uint8

const (
	DeliveryDispatchTriggerOutcomeInvalid DeliveryDispatchTriggerOutcome = iota
	DeliveryDispatchTaskFormed
	DeliveryDispatchTaskAlreadyFormed
	DeliveryDispatchNotTriggered
	DeliveryDispatchRequirementMissing
	DeliveryDispatchNotAccepted
	DeliveryDispatchUndecided
)

func (outcome DeliveryDispatchTriggerOutcome) String() string {
	switch outcome {
	case DeliveryDispatchTaskFormed:
		return "DISPATCH_TASK_FORMED"
	case DeliveryDispatchTaskAlreadyFormed:
		return "DISPATCH_TASK_ALREADY_FORMED"
	case DeliveryDispatchNotTriggered:
		return "NOT_A_DELIVERY_TRIGGER"
	case DeliveryDispatchRequirementMissing:
		return "REQUIREMENT_MISSING"
	case DeliveryDispatchNotAccepted:
		return "INPUT_NOT_ACCEPTED"
	case DeliveryDispatchUndecided:
		return "DISPATCH_UNDECIDED"
	default:
		return ""
	}
}

// DeliveryTriggerRefusal 点名`不是触发事实`是哪一格。五格各自成格而不并成一个「不触发」，因为续办动作两两不同：
// 去查段立没立起来、去查对象进没进段、去看对象是不是已经交出去了、去看登记方有没有声明服务动作、去看进段凭的是
// 哪种控制事实。与 SegmentEntryRefusal 同一条规则：领域正当拒绝单开答格、不留续办引用。
type DeliveryTriggerRefusal uint8

const (
	DeliveryTriggerRefusalNone DeliveryTriggerRefusal = iota
	DeliveryTriggerRefusedSegmentNotFound
	DeliveryTriggerRefusedObjectNotInSegment
	// DeliveryTriggerRefusedParticipationNotActive：对象在段里有参与，但链尾已离场——这一拍来晚了，对象已经交出去或
	// 控制已终止，没有可派送的对象（ADR-0114 决定二「凭链尾判，被替代或已离场的参与不触发」）。
	DeliveryTriggerRefusedParticipationNotActive
	// DeliveryTriggerRefusedSegmentNotDelivery：段未声明为末端派送（未声明、节点间运输、场外揽收三格都算）。未声明是
	// 一种答案不是缺陷——这样的段照常成立与结束，只是不触发依赖服务动作的派生（CONTEXT「段服务动作」）。
	DeliveryTriggerRefusedSegmentNotDelivery
	// DeliveryTriggerRefusedEntryNotByHandover：对象凭有效收寄而不是`已交接`进的段。CONTEXT 把内部触发限定为「载运对象凭
	// `已交接`的权威交接进入派送段」这一种事实；揽收成立的段被声明为派送段时（ADR-0114 越权风险点 2 的那一格），凭收寄
	// 进入的对象照字面不触发，后续凭交接加入的对象才触发。
	DeliveryTriggerRefusedEntryNotByHandover
)

func (refusal DeliveryTriggerRefusal) String() string {
	switch refusal {
	case DeliveryTriggerRefusedSegmentNotFound:
		return "SEGMENT_NOT_FOUND"
	case DeliveryTriggerRefusedObjectNotInSegment:
		return "OBJECT_NOT_IN_SEGMENT"
	case DeliveryTriggerRefusedParticipationNotActive:
		return "PARTICIPATION_NOT_ACTIVE"
	case DeliveryTriggerRefusedSegmentNotDelivery:
		return "SEGMENT_NOT_DELIVERY"
	case DeliveryTriggerRefusedEntryNotByHandover:
		return "ENTRY_NOT_BY_HANDOVER"
	default:
		return ""
	}
}

// DeliveryRequirement 是派送要求的三件（CONTEXT「派送要求」）：收件地点引用归 parcel-shipment、计划履约段时间窗口归
// network-routing、交付条件引用归 party-commercial。`要求缺失`逐件点名，缺几件报几件——所有者各不相同，追哪一家去要
// 取决于缺的是哪一件。
type DeliveryRequirement uint8

const (
	DeliveryRequirementInvalid DeliveryRequirement = iota
	DeliveryPlaceRequirement
	DeliveryWindowRequirement
	DeliveryConditionRequirement
)

func (requirement DeliveryRequirement) String() string {
	switch requirement {
	case DeliveryPlaceRequirement:
		return "DELIVERY_PLACE"
	case DeliveryWindowRequirement:
		return "DELIVERY_WINDOW"
	case DeliveryConditionRequirement:
		return "DELIVERY_CONDITION"
	default:
		return ""
	}
}

// DeliveryDispatchUndecidedReason 指名`未决`停在哪一步等谁。三条缝各两格（未接线 / 读不到），加段登记册与任务口各一格。
// 未接线单独成格是 ADR-0114 决定三的要求：缝没接时答未决并点名哪条缝，不造替身、不填默认——今天三条缝一条都没接
// （票 tf-segment-lifecycle-closure/12–14），生产装配里这一格就是执行器的诚实停点。
type DeliveryDispatchUndecidedReason uint8

const (
	DeliveryDispatchUndecidedReasonNone DeliveryDispatchUndecidedReason = iota
	DeliverySegmentRegistryUnavailable
	DeliveryPlaceSourceNotWired
	DeliveryPlaceSourceUnavailable
	DeliveryWindowSourceNotWired
	DeliveryWindowSourceUnavailable
	DeliveryConditionSourceNotWired
	DeliveryConditionSourceUnavailable
	DeliveryDispatchTaskUndecided
)

func (reason DeliveryDispatchUndecidedReason) String() string {
	switch reason {
	case DeliverySegmentRegistryUnavailable:
		return "SEGMENT_REGISTRY_UNAVAILABLE"
	case DeliveryPlaceSourceNotWired:
		return "DELIVERY_PLACE_SOURCE_NOT_WIRED"
	case DeliveryPlaceSourceUnavailable:
		return "DELIVERY_PLACE_SOURCE_UNAVAILABLE"
	case DeliveryWindowSourceNotWired:
		return "DELIVERY_WINDOW_SOURCE_NOT_WIRED"
	case DeliveryWindowSourceUnavailable:
		return "DELIVERY_WINDOW_SOURCE_UNAVAILABLE"
	case DeliveryConditionSourceNotWired:
		return "DELIVERY_CONDITION_SOURCE_NOT_WIRED"
	case DeliveryConditionSourceUnavailable:
		return "DELIVERY_CONDITION_SOURCE_UNAVAILABLE"
	case DeliveryDispatchTaskUndecided:
		return "DISPATCH_TASK_UNDECIDED"
	default:
		return ""
	}
}

// TriggerDeliveryDispatchCommand 是一拍的输入：哪个租户的哪个对象进了哪个段，以及这一拍的业务时间。
//
// 任务的成立时间取 OccurredAt 而不取对象进段的时刻：CONTEXT 写的是「控制事实先如实落库，任务在下一拍形成」，任务是
// 在这一拍成立的，进段是它的触发事实不是它的成立时刻。它由调用方给而不是这里取时钟，与 OpenDispatchTaskCommand 同一
// 条通例——任务是何时成立的，不由写库那一刻决定。谁按拍调本编排、拍频多大，随第一条派送要求缝的实施票立（ADR-0114
// 决定二末句），这里不预设。
type TriggerDeliveryDispatchCommand struct {
	TenantID   domain.TenantID
	Segment    string
	Object     string
	OccurredAt time.Time
}

type TriggerDeliveryDispatchResult struct {
	outcome      DeliveryDispatchTriggerOutcome
	refusal      DeliveryTriggerRefusal
	missing      []DeliveryRequirement
	reason       DeliveryDispatchUndecidedReason
	record       ports.DispatchTaskRecord
	hasRecord    bool
	continuation string
}

func (result TriggerDeliveryDispatchResult) Outcome() DeliveryDispatchTriggerOutcome {
	return result.outcome
}

// Refusal 只在`不是触发事实`时非零。
func (result TriggerDeliveryDispatchResult) Refusal() DeliveryTriggerRefusal {
	return result.refusal
}

// Missing 只在`要求缺失`时非空，按地点、时间窗、条件的固定顺序列出所有者答「没有」的每一件。
func (result TriggerDeliveryDispatchResult) Missing() []DeliveryRequirement {
	return append([]DeliveryRequirement(nil), result.missing...)
}

// UndecidedReason 只在`未决`时非零。
func (result TriggerDeliveryDispatchResult) UndecidedReason() DeliveryDispatchUndecidedReason {
	return result.reason
}

// Record 在`已形成`与`已在册`时给出那项任务。
func (result TriggerDeliveryDispatchResult) Record() (ports.DispatchTaskRecord, bool) {
	return result.record, result.hasRecord
}

// ContinuationReference 只在`未决`时非空，供按拍调本编排的一方重跑同一拍。
func (result TriggerDeliveryDispatchResult) ContinuationReference() string {
	return result.continuation
}

// DispatchTaskOpener 是本编排调的既有任务口：OpenDispatchTask 签名不动（票 09 边界），这里只依赖它的形状。
type DispatchTaskOpener interface {
	Open(ctx context.Context, command OpenDispatchTaskCommand) (OpenDispatchTaskResult, error)
}

// TriggerDeliveryDispatchDeps 的三条派送要求端口**可缺席**：缝没接时编排答`未决`并点名那条缝，不造替身、不填默认
// （ADR-0114 决定三）。段登记册与任务口必备。没有时钟：本编排不铸任何时间，成立时间由命令带、登记时刻由任务口取。
type TriggerDeliveryDispatchDeps struct {
	Segments   ports.ActualFulfillmentSegmentRegistry
	Places     ports.DeliveryPlaceSource
	Windows    ports.DeliveryWindowSource
	Conditions ports.DeliveryConditionSource
	Dispatch   DispatchTaskOpener
}

// TriggerDeliveryDispatchHandler 是末端派送任务的内部触发执行器：以「对象凭`已交接`进入派送段」为输入，按派送要求
// 取齐七件，调既有 OpenDispatchTask。它是异步的一拍不是同事务：控制事实早已落库，这里形成失败只重跑本拍，不翻交接
// （CONTEXT「揽收、派送与交付」；ADR-0114 决定二）。本编排对段登记册**只读**——它没有任何一条路径写段、写参与或写交接，
// 「不回滚交接」在依赖形状上就成立。一拍一对象一任务：同段对象目的地各异，合并规则是运营政策，不代拟。
type TriggerDeliveryDispatchHandler struct {
	deps TriggerDeliveryDispatchDeps
}

func NewTriggerDeliveryDispatchHandler(deps TriggerDeliveryDispatchDeps) *TriggerDeliveryDispatchHandler {
	return &TriggerDeliveryDispatchHandler{deps: deps}
}

// Trigger 跑一拍：受理形状 → 读回段（必须是派送段、对象必须是凭`已交接`在场的当前参与）→ 三条缝都接了才去问 →
// 三件各取一次、缺的逐件点名 → 调任务口。
func (handler *TriggerDeliveryDispatchHandler) Trigger(
	ctx context.Context,
	command TriggerDeliveryDispatchCommand,
) (TriggerDeliveryDispatchResult, error) {
	segmentReference, object, accepted := deliveryTriggerTargetFrom(command)
	if !accepted {
		return TriggerDeliveryDispatchResult{outcome: DeliveryDispatchNotAccepted}, nil
	}

	key := ports.FulfillmentSegmentKey{TenantID: command.TenantID, Segment: segmentReference}
	record, found, err := handler.deps.Segments.FindByKey(ctx, key)
	if err != nil {
		return deliveryDispatchUndecided(DeliverySegmentRegistryUnavailable, command), nil
	}
	if !found {
		return deliveryDispatchRefused(DeliveryTriggerRefusedSegmentNotFound), nil
	}
	// 段先答、对象后答：段是不是派送段与对象无关，先判它，不去派送段的对象们就不必逐个问。
	if !record.Segment.IsDeliverySegment() {
		return deliveryDispatchRefused(DeliveryTriggerRefusedSegmentNotDelivery), nil
	}
	participation, present := record.Segment.ParticipationFor(object)
	if !present {
		return deliveryDispatchRefused(DeliveryTriggerRefusedObjectNotInSegment), nil
	}
	if !participation.Active() {
		return deliveryDispatchRefused(DeliveryTriggerRefusedParticipationNotActive), nil
	}
	if participation.EntryKind() != domain.EnteredByTransportHandover {
		return deliveryDispatchRefused(DeliveryTriggerRefusedEntryNotByHandover), nil
	}

	// 缝没接是装配事实不是这一拍的事实：三条先一并核过，第一条没接的点名出去；不为没接的缝问已接的缝——问了也
	// 形成不了任务，白读一次所有者。
	if unwired, any := handler.unwiredSeam(); any {
		return deliveryDispatchUndecided(unwired, command), nil
	}
	requirements, undecided := handler.pullRequirements(ctx, command, object)
	if undecided != nil {
		return *undecided, nil
	}
	if len(requirements.missing) > 0 {
		return TriggerDeliveryDispatchResult{outcome: DeliveryDispatchRequirementMissing, missing: requirements.missing}, nil
	}

	// 任务引用按（段，对象，入场依据）确定性铸出：同一拍重跑撞`已在册`而不重建（ADR-0114 决定二）。
	opened, err := handler.deps.Dispatch.Open(ctx, OpenDispatchTaskCommand{
		TenantID:   command.TenantID,
		Task:       deliveryDispatchTaskReference(segmentReference, object, participation.EntryBasis()),
		Kind:       domain.DeliveryDispatch,
		Objects:    []string{object.String()},
		Place:      requirements.place,
		WindowFrom: requirements.windowFrom,
		WindowTo:   requirements.windowTo,
		Conditions: requirements.conditions,
		OpenedAt:   command.OccurredAt,
	})
	if err != nil {
		return TriggerDeliveryDispatchResult{}, err
	}
	switch opened.Outcome() {
	case DispatchTaskOpened:
		task, _ := opened.Record()
		return TriggerDeliveryDispatchResult{outcome: DeliveryDispatchTaskFormed, record: task, hasRecord: true}, nil
	case DispatchTaskAlreadyRegistered:
		task, _ := opened.Record()
		return TriggerDeliveryDispatchResult{outcome: DeliveryDispatchTaskAlreadyFormed, record: task, hasRecord: true}, nil
	case DispatchTaskUndecided:
		return TriggerDeliveryDispatchResult{
			outcome:      DeliveryDispatchUndecided,
			reason:       DeliveryDispatchTaskUndecided,
			continuation: opened.ContinuationReference(),
		}, nil
	case DispatchTaskNotAccepted:
		// 三件都是所有者给的、对象与引用都过了构造门，任务口仍不受理只剩一种可能：某条缝交回的东西过不了任务的
		// 构造门（如时间窗首尾颠倒）。那是适配器翻译的缺陷，重试不会变，如实答不受理，不替所有者修。
		return TriggerDeliveryDispatchResult{outcome: DeliveryDispatchNotAccepted}, nil
	default:
		return TriggerDeliveryDispatchResult{}, fmt.Errorf("%w: %d", ErrUnexpectedDispatchTaskOutcome, opened.Outcome())
	}
}

// deliveryRequirements 是三条缝各答一次之后手里的东西：给了的三件与答「没有」的名单。
type deliveryRequirements struct {
	place      string
	windowFrom time.Time
	windowTo   time.Time
	conditions string
	missing    []DeliveryRequirement
}

// unwiredSeam 按地点、时间窗、条件的顺序答第一条没接线的缝。
func (handler *TriggerDeliveryDispatchHandler) unwiredSeam() (DeliveryDispatchUndecidedReason, bool) {
	switch {
	case handler.deps.Places == nil:
		return DeliveryPlaceSourceNotWired, true
	case handler.deps.Windows == nil:
		return DeliveryWindowSourceNotWired, true
	case handler.deps.Conditions == nil:
		return DeliveryConditionSourceNotWired, true
	default:
		return DeliveryDispatchUndecidedReasonNone, false
	}
}

// pullRequirements 三条缝各问一次。读不到是欠账，第一条读不到的即答未决（后面的不必再问，这一拍反正要重跑）；
// 所有者答「没有」不是欠账，记进名单继续问下一条——缺几件要一次说清。答法不在封闭集合里（零值）是适配器的缺陷，
// 与读不到同格：不把它读成「没有」，那会让一次适配器故障变成一个业务答案。
func (handler *TriggerDeliveryDispatchHandler) pullRequirements(
	ctx context.Context,
	command TriggerDeliveryDispatchCommand,
	object domain.CarriedObjectReference,
) (deliveryRequirements, *TriggerDeliveryDispatchResult) {
	var requirements deliveryRequirements

	place, resolution, err := handler.deps.Places.LoadDeliveryPlace(ctx, command.TenantID, object)
	if faulted := requirementFault(resolution, err); faulted {
		undecided := deliveryDispatchUndecided(DeliveryPlaceSourceUnavailable, command)
		return deliveryRequirements{}, &undecided
	}
	if resolution == ports.RequirementMissing {
		requirements.missing = append(requirements.missing, DeliveryPlaceRequirement)
	} else {
		requirements.place = place
	}

	windowFrom, windowTo, resolution, err := handler.deps.Windows.LoadDeliveryWindow(ctx, command.TenantID, object)
	if faulted := requirementFault(resolution, err); faulted {
		undecided := deliveryDispatchUndecided(DeliveryWindowSourceUnavailable, command)
		return deliveryRequirements{}, &undecided
	}
	if resolution == ports.RequirementMissing {
		requirements.missing = append(requirements.missing, DeliveryWindowRequirement)
	} else {
		requirements.windowFrom, requirements.windowTo = windowFrom, windowTo
	}

	conditions, resolution, err := handler.deps.Conditions.LoadDeliveryConditions(ctx, command.TenantID, object)
	if faulted := requirementFault(resolution, err); faulted {
		undecided := deliveryDispatchUndecided(DeliveryConditionSourceUnavailable, command)
		return deliveryRequirements{}, &undecided
	}
	if resolution == ports.RequirementMissing {
		requirements.missing = append(requirements.missing, DeliveryConditionRequirement)
	} else {
		requirements.conditions = conditions
	}
	return requirements, nil
}

// requirementFault 报告一条缝的回答算不算读不到：出错，或答法不在封闭二格（给了 / 没有）里。
func requirementFault(resolution ports.RequirementResolution, err error) bool {
	if err != nil {
		return true
	}
	return resolution != ports.RequirementResolved && resolution != ports.RequirementMissing
}

// deliveryDispatchTaskReference 是任务身份的铸法：段、对象、入场依据三件定一拍（ADR-0114 决定二）。入场依据在键里
// 是因为同一对象经来源更正后替代入场（ADR-0112）是另一版参与——它的任务要不要另立由那一拍自己判，这里不让它撞上
// 前一版的任务；照 ADR 字面铸，是否该按（段，对象）铸以免更正后重开任务列为越权风险点供 owner 复核。
func deliveryDispatchTaskReference(
	segment domain.FulfillmentSegmentReference,
	object domain.CarriedObjectReference,
	entryBasis domain.ParticipationBasisReference,
) string {
	return "DELIVERY-DISPATCH/" + segment.String() + "/" + object.String() + "/" + entryBasis.String()
}

func deliveryTriggerTargetFrom(
	command TriggerDeliveryDispatchCommand,
) (domain.FulfillmentSegmentReference, domain.CarriedObjectReference, bool) {
	if strings.TrimSpace(command.TenantID.String()) == "" || command.OccurredAt.IsZero() {
		return domain.FulfillmentSegmentReference{}, domain.CarriedObjectReference{}, false
	}
	segment, err := domain.NewFulfillmentSegmentReference(command.Segment)
	if err != nil {
		return domain.FulfillmentSegmentReference{}, domain.CarriedObjectReference{}, false
	}
	object, err := domain.NewCarriedObjectReference(command.Object)
	if err != nil {
		return domain.FulfillmentSegmentReference{}, domain.CarriedObjectReference{}, false
	}
	return segment, object, true
}

func deliveryDispatchRefused(refusal DeliveryTriggerRefusal) TriggerDeliveryDispatchResult {
	return TriggerDeliveryDispatchResult{outcome: DeliveryDispatchNotTriggered, refusal: refusal}
}

func deliveryDispatchUndecided(reason DeliveryDispatchUndecidedReason, command TriggerDeliveryDispatchCommand) TriggerDeliveryDispatchResult {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		reason.String(), command.TenantID.String(), command.Segment, command.Object,
	}, "\x00")))
	return TriggerDeliveryDispatchResult{
		outcome:      DeliveryDispatchUndecided,
		reason:       reason,
		continuation: "CONT-" + hex.EncodeToString(digest[:8]),
	}
}
