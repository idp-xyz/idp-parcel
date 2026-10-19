package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// ErrUnexpectedDispatchTaskOutcome 说明既有任务口交回了封闭集合以外的结果。
var ErrUnexpectedDispatchTaskOutcome = errors.New("transport fulfillment: unexpected dispatch task outcome")

// DeliveryDispatchTriggerOutcome 是「对象凭`已交接`进入派送段 → 形成末端派送任务」这一拍的结果代数（ADR-0114 决定二）。
//
// `已形成`与`已在册`分开：后者是本拍要的那一项任务已经在册——同一拍重跑，任务引用确定性铸出撞上它——交回它而不重建。
// `引用被占`是本拍的任务引用上立着的不是本拍那一项（种类不是末端派送，或对象集不恰为本对象）：交回占用者，不写、不另铸；
// 它不并进`已在册`，因为那一格对调用方说的是「这个对象已经有派送任务了」，而占用者并不是（票 tf-face-algebra/09 裁定
// 第 2 条）。`不是触发事实`是领域答案——段不是派送段、对象不在段里或已不在场、进段凭的不是`已交接`，重跑不会变，哪一格由
// DeliveryTriggerRefusal 点名。`要求缺失`是所有者答「没有」：任务保持待形成，不填默认（ADR-0114 决定三）。`未决`是缝没接
// 或读不到：原样重跑本拍就可能过，理由里点名是哪一条缝。
type DeliveryDispatchTriggerOutcome uint8

const (
	DeliveryDispatchTriggerOutcomeInvalid DeliveryDispatchTriggerOutcome = iota
	DeliveryDispatchTaskFormed
	DeliveryDispatchTaskAlreadyFormed
	DeliveryDispatchNotTriggered
	DeliveryDispatchRequirementMissing
	DeliveryDispatchNotAccepted
	DeliveryDispatchUndecided
	DeliveryDispatchTaskReferenceTaken
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
	case DeliveryDispatchTaskReferenceTaken:
		return "DISPATCH_TASK_REFERENCE_TAKEN"
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
// 未接线单独成格是 ADR-0114 决定三的要求：缝没接时答未决并点名哪条缝，不造替身、不填默认。生产装配
// buildDeliveryDispatchTrigger 三条缝都接了真适配器；这一格守的是缺件的装配——缺哪条照实点名，不拿替身顶上。
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

// Record 在`已形成`与`已在册`时给出那项任务，在`引用被占`时给出占用者。
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
// （ADR-0114 决定三）。段登记册、任务登记册读口与任务口必备。读口与任务口分开给：查在册只读，在册了就停在这里，走不到
// 任务口落库那一步；装配点交的必须是任务口写的那一张登记册。没有时钟：本编排不铸任何时间，成立时间由命令带、登记时刻
// 由任务口取。
type TriggerDeliveryDispatchDeps struct {
	Segments   ports.ActualFulfillmentSegmentRegistry
	Tasks      ports.DispatchTaskReader
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

// Trigger 跑一拍：受理形状 → 读回段（必须是派送段、对象在段里有未失效的参与、链尾凭`已交接`入场）→ 按本拍的键查在册
// → 参与在场 → 三条缝都接了才去问 → 三件各取一次、缺的逐件点名 → 调任务口。
//
// 查在册排在参与在场之前、读回段与确认链尾之后（票 tf-face-algebra/09 裁定第 3 条）。键由段、对象与链尾的入场依据定，
// 前两步省不掉；在册了就是同一拍的重放：答复不随所有者后来的改答漂移，也不白读所有者。参与离场之后的重放同样交回在册
// 那一项，它不形成新任务，ADR-0114 决定二「已离场的参与不触发」照样成立。
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
	// 链尾失效即该对象在本段当前无有效参与（票 tf-segment-lifecycle-closure/11 裁决 4）——它从未离场，「来晚了」
	// 那一格说的不是它。
	if !present || participation.Voided() {
		return deliveryDispatchRefused(DeliveryTriggerRefusedObjectNotInSegment), nil
	}
	if participation.EntryKind() != domain.EnteredByTransportHandover {
		return deliveryDispatchRefused(DeliveryTriggerRefusedEntryNotByHandover), nil
	}
	reference := deliveryDispatchTaskReference(segmentReference, object, participation.EntryBasis())
	if registered, answered := handler.registeredTask(ctx, command, object, reference,
		legacyDeliveryDispatchTaskReference(segmentReference, object, participation.EntryBasis())); answered {
		return registered, nil
	}
	if !participation.Active() {
		return deliveryDispatchRefused(DeliveryTriggerRefusedParticipationNotActive), nil
	}

	// 缝没接是装配事实不是这一拍的事实：三条先一并核过，第一条没接的点名出去；不为没接的缝问已接的缝——问了也
	// 形成不了任务，白读一次所有者。
	if unwired, any := handler.unwiredSeam(); any {
		return deliveryDispatchUndecided(unwired, command), nil
	}
	requirements, undecided := handler.pullRequirements(ctx, command, object, participation)
	if undecided != nil {
		return *undecided, nil
	}
	if len(requirements.missing) > 0 {
		return TriggerDeliveryDispatchResult{outcome: DeliveryDispatchRequirementMissing, missing: requirements.missing}, nil
	}

	opened, err := handler.deps.Dispatch.Open(ctx, OpenDispatchTaskCommand{
		TenantID:   command.TenantID,
		Task:       reference,
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
		// 查在册之后、落库之前另一笔先提交了这个键（典型是并发的同一拍）：读回的赢家照样要核，占键的未必是本拍那一项。
		task, _ := opened.Record()
		return deliveryDispatchRegisteredAnswer(task, object), nil
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
//
// 时间窗那一缝按参与关系上登记方关联的计划履约段引用问，不按对象问（ADR-0131 决定一）：对象在执行哪条段是登记方
// 声明的事实，这里原样转交，连「引用缺席」也交给适配器答（决定三）——执行器不重复判缺席，判了就是两处各说一遍
// 同一条规则。
func (handler *TriggerDeliveryDispatchHandler) pullRequirements(
	ctx context.Context,
	command TriggerDeliveryDispatchCommand,
	object domain.CarriedObjectReference,
	participation domain.FulfillmentParticipation,
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

	planned, present := participation.PlannedSegment()
	windowFrom, windowTo, resolution, err := handler.deps.Windows.LoadDeliveryWindow(ctx, command.TenantID, planned, present)
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

// registeredTask 按本拍的键查在册，answered 为真时 result 就是这一拍的答复。
//
// 先查新铸法的键，查不到再查旧拼法的键（票 tf-face-algebra/09 裁定第 1 条）：旧拼法铸下的任务留在原键上，已落库的身份
// 不追溯改写（ADR-0014 同一纪律），演示租户的存量也照此（ADR-0150 决定一）。新键上有任务就以它为准，核得过答`已在册`，
// 核不过答`引用被占`。旧键上的任务只有核得过才算本拍那一项；核不过说明它是别的任务（手工建任务口指名过这一串，或别的段
// 与对象在斜杠直拼下撞上了它），本拍不算在册，照新键往下走——旧键只是兼认的去处，不是本拍的身份。读不回答未决：在不在册
// 还没定，不去问所有者。
func (handler *TriggerDeliveryDispatchHandler) registeredTask(
	ctx context.Context,
	command TriggerDeliveryDispatchCommand,
	object domain.CarriedObjectReference,
	reference, legacy string,
) (result TriggerDeliveryDispatchResult, answered bool) {
	task, found, err := handler.findTask(ctx, command.TenantID, reference)
	if err != nil {
		return deliveryDispatchUndecided(DeliveryDispatchTaskUndecided, command), true
	}
	if found {
		return deliveryDispatchRegisteredAnswer(task, object), true
	}
	// 段与入场依据是调用方给的不透明串，旧拼法串因此可能恰好合新形状；那样的串上若有任务，是新铸法下另一组（段，对象，
	// 入场依据）的，同一对象跨段时种类与对象集都对得上，核对拦不住。新旧两种拼法字面上不得相同（裁定第 1 条），在这里守。
	if _, newShape := parseDeliveryDispatchTaskReference(legacy); newShape {
		return TriggerDeliveryDispatchResult{}, false
	}
	task, found, err = handler.findTask(ctx, command.TenantID, legacy)
	if err != nil {
		return deliveryDispatchUndecided(DeliveryDispatchTaskUndecided, command), true
	}
	if found && isThisObjectsDeliveryTask(task, object) {
		return TriggerDeliveryDispatchResult{outcome: DeliveryDispatchTaskAlreadyFormed, record: task, hasRecord: true}, true
	}
	return TriggerDeliveryDispatchResult{}, false
}

// findTask 按租户与引用串取任务。串过不了引用的构造门答「不在册」：段、对象与入场依据都过了各自的构造门，拼出来的串
// 不会过不了；真过不了，任务口会对同一串答`未受理`，那才是该给的答复。
func (handler *TriggerDeliveryDispatchHandler) findTask(
	ctx context.Context,
	tenant domain.TenantID,
	raw string,
) (ports.DispatchTaskRecord, bool, error) {
	task, err := domain.NewDispatchTaskReference(raw)
	if err != nil {
		return ports.DispatchTaskRecord{}, false, nil
	}
	return handler.deps.Tasks.FindByKey(ctx, ports.DispatchTaskKey{TenantID: tenant, Task: task})
}

// deliveryDispatchRegisteredAnswer 核本拍键上那一项是不是本拍要的任务（裁定第 2 条）：种类须是末端派送，对象集须恰为
// 本对象。单靠这道核对拦不住同一对象跨段撞键，主修是单射铸法；它是第二道防线，兜住存量里指名执行器引用的手工任务。
func deliveryDispatchRegisteredAnswer(task ports.DispatchTaskRecord, object domain.CarriedObjectReference) TriggerDeliveryDispatchResult {
	if isThisObjectsDeliveryTask(task, object) {
		return TriggerDeliveryDispatchResult{outcome: DeliveryDispatchTaskAlreadyFormed, record: task, hasRecord: true}
	}
	return TriggerDeliveryDispatchResult{outcome: DeliveryDispatchTaskReferenceTaken, record: task, hasRecord: true}
}

func isThisObjectsDeliveryTask(task ports.DispatchTaskRecord, object domain.CarriedObjectReference) bool {
	objects := task.Task.Objects()
	return task.Task.Kind() == domain.DeliveryDispatch && len(objects) == 1 && objects[0] == object
}

const (
	// deliveryDispatchTaskReferenceRoot 起头的任务引用归本执行器铸，手工建任务口拒指名它（裁定第 4 条）。新旧两种拼法
	// 都以它起头，所以挡它一处就把两种都挡住了。
	deliveryDispatchTaskReferenceRoot = "DELIVERY-DISPATCH/"
	// deliveryDispatchTaskReferenceShape 是形状标记。旧拼法没有标记，段紧跟在起头之后。
	deliveryDispatchTaskReferenceShape = "v2"
)

// IsReservedDeliveryDispatchTaskReference 答一个任务引用是不是落在派送发起执行器的保留起头下。手工建任务口据此拒指名：
// 那一口能写任意引用，而执行器认在册只认键——手工任务一旦占了执行器要铸的那一串，就会被读成别的对象的派送任务。
func IsReservedDeliveryDispatchTaskReference(task string) bool {
	return strings.HasPrefix(task, deliveryDispatchTaskReferenceRoot)
}

// deliveryDispatchTaskReference 是任务身份的铸法：段、对象、入场依据三件定一拍（ADR-0114 决定二）。起头之后是形状标记，
// 再把三件依次写成「/字节长度:原串」，例如
// `DELIVERY-DISPATCH/v2/9:segment-1/8:parcel-1/46:TRANSPORT-HANDOVER/handover-result/parcel-1/v1`。
//
// 三件都是只拒空白的不透明串，可以含斜杠，交接版本跨对象还可以相同；以斜杠直拼时不同的三件能拼出同一串，后到的对象首拍
// 就被答成「已形成」。写上长度之后每一件在哪里结束是定的，读回唯一，铸法单射（parseDeliveryDispatchTaskReference 就是那个
// 读回）。不取摘要：引用出现在答复、日志与管理台上，人要读得出它指的是哪个段哪个对象（票 tf-face-algebra/09 裁定第 1 条）。
//
// 入场依据在键里是因为同一对象经来源更正后替代入场（ADR-0112）是另一版参与——它的任务要不要另立由那一拍自己判，这里
// 不让它撞上前一版的任务；照 ADR 字面铸，是否该按（段，对象）铸以免更正后重开任务列为越权风险点供 owner 复核。
func deliveryDispatchTaskReference(
	segment domain.FulfillmentSegmentReference,
	object domain.CarriedObjectReference,
	entryBasis domain.ParticipationBasisReference,
) string {
	var reference strings.Builder
	reference.WriteString(deliveryDispatchTaskReferenceRoot)
	reference.WriteString(deliveryDispatchTaskReferenceShape)
	for _, part := range []string{segment.String(), object.String(), entryBasis.String()} {
		reference.WriteByte('/')
		reference.WriteString(strconv.Itoa(len(part)))
		reference.WriteByte(':')
		reference.WriteString(part)
	}
	return reference.String()
}

// parseDeliveryDispatchTaskReference 按新铸法把引用读回段、对象、入场依据三件；串不合新形状答 false。长度只认不带前导
// 零的十进制：一组三件只有一种写法，读回才与铸法一一对应。
func parseDeliveryDispatchTaskReference(reference string) ([3]string, bool) {
	var parts [3]string
	rest, ok := strings.CutPrefix(reference, deliveryDispatchTaskReferenceRoot+deliveryDispatchTaskReferenceShape)
	if !ok {
		return parts, false
	}
	for index := range parts {
		if rest, ok = strings.CutPrefix(rest, "/"); !ok {
			return parts, false
		}
		digits, after, found := strings.Cut(rest, ":")
		if !found || digits == "" || (len(digits) > 1 && digits[0] == '0') || strings.Trim(digits, "0123456789") != "" {
			return parts, false
		}
		length, err := strconv.Atoi(digits)
		if err != nil || length > len(after) {
			return parts, false
		}
		parts[index], rest = after[:length], after[length:]
	}
	return parts, rest == ""
}

// legacyDeliveryDispatchTaskReference 是旧拼法：三件以斜杠直拼，不单射。只用来兼认停在它上面的存量，不拿它铸新任务。
func legacyDeliveryDispatchTaskReference(
	segment domain.FulfillmentSegmentReference,
	object domain.CarriedObjectReference,
	entryBasis domain.ParticipationBasisReference,
) string {
	return deliveryDispatchTaskReferenceRoot + segment.String() + "/" + object.String() + "/" + entryBasis.String()
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
