package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// ErrObjectActiveInSeveralSegments 说明登记册按对象找回了多于一条在场参与。一对象同时只能在一个共同控制
// 范围里（CONTEXT），多于一条是库面不一致——不挑一个去结束，响亮报错让人来看（票 06 裁决 (a)）。
var ErrObjectActiveInSeveralSegments = errors.New("transport fulfillment: the object is active in several fulfillment segments")

// ParticipationEndSource 是结束一条履约参与关系的控制事实来源。
//
// 封闭集合而不是一个自由字符串：CONTEXT 把参与终点限定为有效交付、下一次权威交接或明确控制
// 终止三来源——中断、折返、异常案件不在其中，它们不结束控制。多开一格就等于给「说不清为什么
// 结束」开一条路。
//
// **交付那一格本刀未做**（它要读交付登记册，另起一刀），所以这里暂时只有两格；加它时补的是
// 一格，不是改这个集合的性质。
type ParticipationEndSource uint8

const (
	ParticipationEndSourceInvalid ParticipationEndSource = iota
	ParticipationEndedByDelivery
	ParticipationEndedByNextHandover
	ParticipationEndedByTermination
)

// carriesControlOnward 只有下一次权威交接为真：控制移入下一段，本段对该对象的责任到此为止。
//
// 有效交付把控制转给收件方、明确终止是控制结束——**两者之后都没有「下一段」**。这个谓词存在
// 就是为了让「已交付但又进了下一段」在编排里表达不出来。
func (source ParticipationEndSource) carriesControlOnward() bool {
	return source == ParticipationEndedByNextHandover
}

// ParticipationEndOutcome 是结束一条参与关系的结果代数。
//
// `已离场`、`对象不在段内`、`段不在册`、`控制事实不在册`各自成格而不并成一个「不行」：四者的
// 续办动作两两不同——重放、去查对象进没进段、去查段成没成立、去登记那条控制事实。
type ParticipationEndOutcome uint8

const (
	ParticipationEndOutcomeInvalid ParticipationEndOutcome = iota
	ParticipationEndedNow
	ParticipationAlreadyEnded
	ParticipationObjectNotInSegment
	ParticipationSegmentNotFound
	ParticipationControlFactNotFound
	ParticipationEndNotAccepted
	ParticipationEndUndecided
	// ParticipationNoActiveParticipation 只在命令不带段、由登记册按对象找段的两路出现：对象此刻不在任何
	// 段里在场。它不是失败（重试不会变）也不是`对象不在段内`（那一格说的是指名的某个段），单开一格让
	// 调用方看得见——交付登上了、但没有任何参与被它结束（票 06 裁决 (a)）。
	ParticipationNoActiveParticipation
	// ParticipationEndNotWired 给「来源编排（交付、交接）在装配点没被交入 ParticipationEnds」这一格一个名字。
	// 它**不出现在任何结果里**：缺席时来源编排整笔不落，名字坐在 ErrParticipationEndsNotWired 的错误信息里
	// ——与 End 失败同格，因为 CONTEXT 写的是有效交付**同时**结束参与，让登记落库而参与没结就是应用层写出
	// 一份违反它的库面状态；NO_ACTIVE 是库面真相，这一格是装配缺陷，后者不该以 201 的样子出现。留在这个集合里
	// 是让它与其余各格同一处命名、同一个 String，而不是散成一个自由字符串。
	ParticipationEndNotWired
)

func (outcome ParticipationEndOutcome) String() string {
	switch outcome {
	case ParticipationEndedNow:
		return "PARTICIPATION_ENDED"
	case ParticipationAlreadyEnded:
		return "PARTICIPATION_ALREADY_ENDED"
	case ParticipationObjectNotInSegment:
		return "OBJECT_NOT_IN_SEGMENT"
	case ParticipationNoActiveParticipation:
		return "NO_ACTIVE_PARTICIPATION"
	case ParticipationEndNotWired:
		return "PARTICIPATION_END_NOT_WIRED"
	case ParticipationSegmentNotFound:
		return "SEGMENT_NOT_FOUND"
	case ParticipationControlFactNotFound:
		return "CONTROL_FACT_NOT_FOUND"
	case ParticipationEndNotAccepted:
		return "INPUT_NOT_ACCEPTED"
	case ParticipationEndUndecided:
		return "PARTICIPATION_END_UNDECIDED"
	default:
		return ""
	}
}

// EndFulfillmentParticipationCommand 携带结束一条参与关系的全部输入。
//
// **交接那一路只收指名一条已登记交接的键，不收依据串。** 依据由那条交接自己交出
// （`TransferOutBasis`）——收调用方自报的引用就等于让任何人凭一次并未发生的交接把对象移出本段。
// 终止那一路的依据只能由调用方给（它指向本上下文之外的处置决定），那是两条路真正的不对称，
// 不是疏漏。
type EndFulfillmentParticipationCommand struct {
	TenantID domain.TenantID
	// Segment 在终止那一路必填（运营决定，操作者面对的是具体某个段）；在交付与下一次交接两路**可缺席**
	// ——缺席时由登记册按对象找它此刻在场的段（票 06 裁决 (a)）：那两路是关于对象的事实，回传方不知道段。
	Segment string
	Object  string
	Source  ParticipationEndSource

	// 下一次权威交接那一路：指名已登记的那一条。
	Scope   string
	Version string

	// 有效交付那一路：指名已登记的那一次（交付的幂等键是租户+对象+尝试）。
	Attempt string

	// 明确控制终止那一路：依据必备，时刻由调用方给（终止是何时发生的不由写库那一刻决定）。
	Basis   string
	EndedAt time.Time

	// NextSegment 指名控制边界变化后对象进入的下一段（CONTEXT「结束原载运对象的履约参与关系
	// 并形成下一实际履约段」），NextPlannedSegment 是它在新段里关联的计划段，可缺席。
	//
	// **段引用仍由调用方显式给，缺席就只结束不立新段**——与票 02 同一条裁定：段身份由谁铸出
	// 至今没有裁决，编排不拿手边任一引用顶替。**只有交接那一路收得下它**，理由见
	// carriesControlOnward。
	NextSegment        string
	NextPlannedSegment string
}

type EndFulfillmentParticipationResult struct {
	outcome      ParticipationEndOutcome
	continuation string
	nextSegment  string
	segment      string
}

func (result EndFulfillmentParticipationResult) Outcome() ParticipationEndOutcome {
	return result.outcome
}

// Segment 是这次结束落在哪个段上；命令不带段时它是登记册按对象找到的那一个，调用方据以知道结束的是哪段。
// 没结束任何参与时为空。
func (result EndFulfillmentParticipationResult) Segment() string {
	return result.segment
}

// ContinuationReference 只在`未决`时非空。
func (result EndFulfillmentParticipationResult) ContinuationReference() string {
	return result.continuation
}

// NextSegmentContinuationReference 非空说明本段这一条已结束、进下一段那一半还欠着。
//
// 它与 Outcome 分开是因为**两半的失败后果不同**：结束是来源保全那一侧（对象在本段的责任到此
// 为止，这一点已经成立），进下一段是派生的一侧；后者失败不该把前者翻回去。
func (result EndFulfillmentParticipationResult) NextSegmentContinuationReference() string {
	return result.nextSegment
}

type EndFulfillmentParticipationDeps struct {
	Segments ports.ActualFulfillmentSegmentRegistry
	// Judgments 让「进下一段」那一半在立起新段时同笔铸实际承运商判断的首版（票 tf-segment-lifecycle-
	// closure/02）。可缺席，判据与形状同 RegisterOffsitePickupDeps.Judgments。
	Judgments  ports.ActualCarrierJudgmentRegistry
	Handovers  ports.TransportHandoverRegistry
	Deliveries ports.EffectiveDeliveryStore
	Clock      ports.Clock
}

type EndFulfillmentParticipationHandler struct {
	deps EndFulfillmentParticipationDeps
}

func NewEndFulfillmentParticipationHandler(
	deps EndFulfillmentParticipationDeps,
) *EndFulfillmentParticipationHandler {
	return &EndFulfillmentParticipationHandler{deps: deps}
}

// End 结束一条履约参与关系。
//
// **整段读回、领域判定、只写那一条。** 先把整段读回来交给领域的 EndParticipationWith* 判——
// 「已结束的不重复结束也不改写」「终点不得早于起点」都在那里；然后只把**那一条**参与关系交给
// 窄写口。**没有任何一步按段批量更新成员**，那是本票的头号红线：CONTEXT 要求共享段中每个对象
// 分别成立、结束和更正，不能由整段结果覆盖成员差异——哪怕当下所有成员结果确实相同。
//
// **结束一条参与不顺手关段。** CONTEXT 生命周期④要求「全部有效参与关系已经结束**且不再接受
// 新对象**」才结束段，后半句是一个决定不是一个可推导的状态；这里自动关段就等于替人做了那个决定。
func (handler *EndFulfillmentParticipationHandler) End(
	ctx context.Context,
	command EndFulfillmentParticipationCommand,
) (EndFulfillmentParticipationResult, error) {
	// 交付与终止之后没有下一段；带着它来就是一条自相矛盾的命令，在动库之前就拒。
	if strings.TrimSpace(command.NextSegment) != "" && !command.Source.carriesControlOnward() {
		return EndFulfillmentParticipationResult{outcome: ParticipationEndNotAccepted}, nil
	}
	if strings.TrimSpace(command.Segment) == "" && command.Source != ParticipationEndedByTermination {
		resolved, outcome, err := handler.resolveSegmentByObject(ctx, command)
		if err != nil {
			return EndFulfillmentParticipationResult{}, err
		}
		if outcome != ParticipationEndOutcomeInvalid {
			return EndFulfillmentParticipationResult{outcome: outcome, continuation: continuationFor(outcome, command)}, nil
		}
		command.Segment = resolved
	}
	segment, object, accepted := participationEndTargetFrom(command)
	if !accepted {
		return EndFulfillmentParticipationResult{outcome: ParticipationEndNotAccepted}, nil
	}

	key := ports.FulfillmentSegmentKey{TenantID: command.TenantID, Segment: segment}
	record, found, err := handler.deps.Segments.FindByKey(ctx, key)
	if err != nil {
		return participationEndUndecided(command), nil
	}
	if !found {
		return EndFulfillmentParticipationResult{outcome: ParticipationSegmentNotFound}, nil
	}
	current, joined := record.Segment.ParticipationFor(object)
	// 链尾失效即该对象在本段当前无有效参与（票 tf-segment-lifecycle-closure/11 裁决 4）：它从未离场，答`已离场`
	// 会让调用方以为册上有一个终点；`对象不在段内`说的正是「此刻没有有效参与可结束」。
	if !joined || current.Voided() {
		return EndFulfillmentParticipationResult{outcome: ParticipationObjectNotInSegment}, nil
	}
	if !current.Active() {
		// 已离场的不重复结束也不改写——更正走新的判断版本，不在这里覆盖。
		return EndFulfillmentParticipationResult{outcome: ParticipationAlreadyEnded}, nil
	}

	ended, fact, outcome := handler.applyEnd(ctx, command, record.Segment, object)
	if outcome != ParticipationEndOutcomeInvalid {
		return EndFulfillmentParticipationResult{outcome: outcome, continuation: continuationFor(outcome, command)}, nil
	}

	participation, present := ended.ParticipationFor(object)
	if !present {
		return participationEndUndecided(command), nil
	}
	saved, err := handler.deps.Segments.EndParticipation(ctx, key, participation)
	if err != nil {
		return participationEndUndecided(command), nil
	}
	switch saved {
	case ports.ParticipationEnded:
		return EndFulfillmentParticipationResult{
			outcome:     ParticipationEndedNow,
			segment:     command.Segment,
			nextSegment: handler.enterNextSegment(ctx, command, fact),
		}, nil
	case ports.ParticipationAlreadyEnded:
		// 并发下另一方先结束：那是业务答案，不是本次失败。
		return EndFulfillmentParticipationResult{outcome: ParticipationAlreadyEnded}, nil
	default:
		return participationEndUndecided(command), nil
	}
}

// resolveSegmentByObject 在命令不带段时问登记册「这个对象此刻在哪个段里在场」（票 06 裁决 (a)）。
//
// 三种回答三种走向：恰一条 → 交回段引用继续；零条 → `NO_ACTIVE_PARTICIPATION`（形成了的答案，不静默）；
// 多于一条 → error——库面不一致，不挑一个。登记册读不回是欠账（`未决`），与 FindByKey 读不回同格。
func (handler *EndFulfillmentParticipationHandler) resolveSegmentByObject(
	ctx context.Context,
	command EndFulfillmentParticipationCommand,
) (string, ParticipationEndOutcome, error) {
	if strings.TrimSpace(command.TenantID.String()) == "" {
		return "", ParticipationEndNotAccepted, nil
	}
	object, err := domain.NewCarriedObjectReference(command.Object)
	if err != nil {
		return "", ParticipationEndNotAccepted, nil
	}
	active, err := handler.deps.Segments.FindActiveSegments(ctx, command.TenantID, object)
	if err != nil {
		return "", ParticipationEndUndecided, nil
	}
	switch len(active) {
	case 0:
		return "", ParticipationNoActiveParticipation, nil
	case 1:
		return active[0].Segment.String(), ParticipationEndOutcomeInvalid, nil
	default:
		return "", ParticipationEndOutcomeInvalid, ErrObjectActiveInSeveralSegments
	}
}

// enterNextSegment 让控制边界变化后的对象进入下一段，交回续办引用；空串表示这一半没有欠账。
//
// 走的是与两条立段入口同一道门（enterFulfillmentSegment），不另写一份——同一形状两个口径正是
// 那道门存在的理由。**这一半失败不把「本段这一条已结束」翻回去**：责任到此为止已经成立，进下
// 一段是派生的一侧。
//
// 那道门交回的`段已关闭`拒绝格这里**暂不透出**，只取续办引用：下一段已关闭要不要单开一格答给
// 调用方，与「下一段由谁指名」是同一次裁决的两半，归票 tf-segment-lifecycle-closure/06。
func (handler *EndFulfillmentParticipationHandler) enterNextSegment(
	ctx context.Context,
	command EndFulfillmentParticipationCommand,
	handover domain.TransportHandover,
) string {
	if !command.Source.carriesControlOnward() || strings.TrimSpace(command.NextSegment) == "" {
		return ""
	}
	return enterNextSegmentEntry(ctx, handler, command, handover).continuation
}

func enterNextSegmentEntry(
	ctx context.Context,
	handler *EndFulfillmentParticipationHandler,
	command EndFulfillmentParticipationCommand,
	handover domain.TransportHandover,
) segmentEntry {
	return enterFulfillmentSegment(
		ctx, handler.deps.Segments, handler.deps.Judgments, handler.deps.Clock,
		command.TenantID, command.NextSegment, command.NextPlannedSegment,
		segmentEntryDoors{
			object: handover.Object(),
			establish: func(
				segment domain.FulfillmentSegmentReference,
				planned domain.PlannedSegmentReference,
			) (domain.ActualFulfillmentSegment, error) {
				return domain.EstablishSegmentWithHandover(segment, handover, planned)
			},
			join: func(
				existing domain.ActualFulfillmentSegment,
				planned domain.PlannedSegmentReference,
			) (domain.ActualFulfillmentSegment, error) {
				return existing.JoinWithHandover(handover, planned)
			},
		},
	)
}

// applyEnd 按来源走对应的领域门。第三个返回值非零表示这一步已经有了答案，段不必再写；
// 第二个返回值只在交接那一路有意义（下一段要凭同一条交接进）。
func (handler *EndFulfillmentParticipationHandler) applyEnd(
	ctx context.Context,
	command EndFulfillmentParticipationCommand,
	segment domain.ActualFulfillmentSegment,
	object domain.CarriedObjectReference,
) (domain.ActualFulfillmentSegment, domain.TransportHandover, ParticipationEndOutcome) {
	none := domain.TransportHandover{}
	switch command.Source {
	case ParticipationEndedByTermination:
		basis, err := domain.NewParticipationBasisReference(command.Basis)
		if err != nil || command.EndedAt.IsZero() {
			return domain.ActualFulfillmentSegment{}, none, ParticipationEndNotAccepted
		}
		ended, err := segment.EndParticipationWithTermination(object, basis, command.EndedAt)
		if err != nil {
			return domain.ActualFulfillmentSegment{}, none, ParticipationEndNotAccepted
		}
		return ended, none, ParticipationEndOutcomeInvalid

	case ParticipationEndedByNextHandover:
		handover, outcome := handler.handoverFact(ctx, command, object)
		if outcome != ParticipationEndOutcomeInvalid {
			return domain.ActualFulfillmentSegment{}, none, outcome
		}
		ended, err := segment.EndParticipationWithNextHandover(handover)
		if err != nil {
			// 拒收与待确认转不出控制，结束不了参与——那是正当结果，由领域把门。
			return domain.ActualFulfillmentSegment{}, none, ParticipationEndNotAccepted
		}
		return ended, handover, ParticipationEndOutcomeInvalid

	case ParticipationEndedByDelivery:
		delivery, outcome := handler.deliveryFact(ctx, command, object)
		if outcome != ParticipationEndOutcomeInvalid {
			return domain.ActualFulfillmentSegment{}, none, outcome
		}
		ended, err := segment.EndParticipationWithDelivery(delivery)
		if err != nil {
			return domain.ActualFulfillmentSegment{}, none, ParticipationEndNotAccepted
		}
		return ended, none, ParticipationEndOutcomeInvalid

	default:
		return domain.ActualFulfillmentSegment{}, none, ParticipationEndNotAccepted
	}
}

// deliveryFact 取回指名的那一次交付。**读不回来就不结束**——与交接那一路同一条判据：依据必须
// 是一条真实存在的控制事实，不是调用方自报的引用串。
func (handler *EndFulfillmentParticipationHandler) deliveryFact(
	ctx context.Context,
	command EndFulfillmentParticipationCommand,
	object domain.CarriedObjectReference,
) (domain.EffectiveDelivery, ParticipationEndOutcome) {
	if handler.deps.Deliveries == nil {
		return domain.EffectiveDelivery{}, ParticipationControlFactNotFound
	}
	attempt, err := domain.NewAttemptReference(command.Attempt)
	if err != nil {
		return domain.EffectiveDelivery{}, ParticipationEndNotAccepted
	}
	record, found, err := handler.deps.Deliveries.FindByKey(ctx, ports.EffectiveDeliveryKey{
		TenantID: command.TenantID,
		Object:   object,
		Attempt:  attempt,
	})
	if err != nil {
		return domain.EffectiveDelivery{}, ParticipationEndUndecided
	}
	if !found {
		return domain.EffectiveDelivery{}, ParticipationControlFactNotFound
	}
	return record.Delivery, ParticipationEndOutcomeInvalid
}

// handoverFact 取回指名的那一条交接。**读不回来就不结束**——依据必须是一条真实存在的控制事实。
func (handler *EndFulfillmentParticipationHandler) handoverFact(
	ctx context.Context,
	command EndFulfillmentParticipationCommand,
	object domain.CarriedObjectReference,
) (domain.TransportHandover, ParticipationEndOutcome) {
	if handler.deps.Handovers == nil {
		return domain.TransportHandover{}, ParticipationControlFactNotFound
	}
	scope, err := domain.NewHandoverScopeReference(command.Scope)
	if err != nil {
		return domain.TransportHandover{}, ParticipationEndNotAccepted
	}
	version, err := domain.NewHandoverResultVersion(command.Version)
	if err != nil {
		return domain.TransportHandover{}, ParticipationEndNotAccepted
	}
	record, found, err := handler.deps.Handovers.FindByKey(ctx, ports.TransportHandoverKey{
		TenantID: command.TenantID,
		Object:   object,
		Scope:    scope,
		Version:  version,
	})
	if err != nil {
		return domain.TransportHandover{}, ParticipationEndUndecided
	}
	if !found {
		return domain.TransportHandover{}, ParticipationControlFactNotFound
	}
	return record.Handover, ParticipationEndOutcomeInvalid
}

func participationEndTargetFrom(
	command EndFulfillmentParticipationCommand,
) (domain.FulfillmentSegmentReference, domain.CarriedObjectReference, bool) {
	if strings.TrimSpace(command.TenantID.String()) == "" {
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

func continuationFor(
	outcome ParticipationEndOutcome,
	command EndFulfillmentParticipationCommand,
) string {
	if outcome != ParticipationEndUndecided {
		return ""
	}
	return participationEndUndecided(command).continuation
}

func participationEndUndecided(command EndFulfillmentParticipationCommand) EndFulfillmentParticipationResult {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		"PARTICIPATION_END_UNAVAILABLE",
		command.TenantID.String(),
		command.Segment,
		command.Object,
	}, "\x00")))
	return EndFulfillmentParticipationResult{
		outcome:      ParticipationEndUndecided,
		continuation: "CONT-" + hex.EncodeToString(digest[:8]),
	}
}
