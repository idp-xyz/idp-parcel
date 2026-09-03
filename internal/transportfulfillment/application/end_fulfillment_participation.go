package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

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
	ParticipationEndedByNextHandover
	ParticipationEndedByTermination
)

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
)

func (outcome ParticipationEndOutcome) String() string {
	switch outcome {
	case ParticipationEndedNow:
		return "PARTICIPATION_ENDED"
	case ParticipationAlreadyEnded:
		return "PARTICIPATION_ALREADY_ENDED"
	case ParticipationObjectNotInSegment:
		return "OBJECT_NOT_IN_SEGMENT"
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
	Segment  string
	Object   string
	Source   ParticipationEndSource

	// 下一次权威交接那一路：指名已登记的那一条。
	Scope   string
	Version string

	// 明确控制终止那一路：依据必备，时刻由调用方给（终止是何时发生的不由写库那一刻决定）。
	Basis   string
	EndedAt time.Time
}

type EndFulfillmentParticipationResult struct {
	outcome      ParticipationEndOutcome
	continuation string
}

func (result EndFulfillmentParticipationResult) Outcome() ParticipationEndOutcome {
	return result.outcome
}

// ContinuationReference 只在`未决`时非空。
func (result EndFulfillmentParticipationResult) ContinuationReference() string {
	return result.continuation
}

type EndFulfillmentParticipationDeps struct {
	Segments  ports.ActualFulfillmentSegmentRegistry
	Handovers ports.TransportHandoverRegistry
	Clock     ports.Clock
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
	if !joined {
		return EndFulfillmentParticipationResult{outcome: ParticipationObjectNotInSegment}, nil
	}
	if !current.Active() {
		// 已离场的不重复结束也不改写——更正走新的判断版本，不在这里覆盖。
		return EndFulfillmentParticipationResult{outcome: ParticipationAlreadyEnded}, nil
	}

	ended, outcome := handler.applyEnd(ctx, command, record.Segment, object)
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
		return EndFulfillmentParticipationResult{outcome: ParticipationEndedNow}, nil
	case ports.ParticipationAlreadyEnded:
		// 并发下另一方先结束：那是业务答案，不是本次失败。
		return EndFulfillmentParticipationResult{outcome: ParticipationAlreadyEnded}, nil
	default:
		return participationEndUndecided(command), nil
	}
}

// applyEnd 按来源走对应的领域门。第二个返回值非零表示这一步已经有了答案，段不必再写。
func (handler *EndFulfillmentParticipationHandler) applyEnd(
	ctx context.Context,
	command EndFulfillmentParticipationCommand,
	segment domain.ActualFulfillmentSegment,
	object domain.CarriedObjectReference,
) (domain.ActualFulfillmentSegment, ParticipationEndOutcome) {
	switch command.Source {
	case ParticipationEndedByTermination:
		basis, err := domain.NewParticipationBasisReference(command.Basis)
		if err != nil || command.EndedAt.IsZero() {
			return domain.ActualFulfillmentSegment{}, ParticipationEndNotAccepted
		}
		ended, err := segment.EndParticipationWithTermination(object, basis, command.EndedAt)
		if err != nil {
			return domain.ActualFulfillmentSegment{}, ParticipationEndNotAccepted
		}
		return ended, ParticipationEndOutcomeInvalid

	case ParticipationEndedByNextHandover:
		handover, outcome := handler.handoverFact(ctx, command, object)
		if outcome != ParticipationEndOutcomeInvalid {
			return domain.ActualFulfillmentSegment{}, outcome
		}
		ended, err := segment.EndParticipationWithNextHandover(handover)
		if err != nil {
			// 拒收与待确认转不出控制，结束不了参与——那是正当结果，由领域把门。
			return domain.ActualFulfillmentSegment{}, ParticipationEndNotAccepted
		}
		return ended, ParticipationEndOutcomeInvalid

	default:
		return domain.ActualFulfillmentSegment{}, ParticipationEndNotAccepted
	}
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
