package application

import (
	"context"
	"errors"
	"slices"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件是随袋走过出发的成员后来才识别成功时的出发门禁补问（ADR-0331）：出发那一刻袋里这件实物的身份未决，出发读数没有它自己的一格
// （袋那一格答未决，缘由指名这件作业实物），关务没有被问过它；它识别成功、有了正式包裹身份之后，按原出发时点替这件包裹再问一次关务，
// 问来的读数追加一版、首版不改。落册、交信封与阻断信号都走 ADR-0305 同一条路，只是键是这件成员包裹，唤醒是节点收寄形成信封。

// MemberCarrier 说一件成员在某一时点是不是随某件集运单元走，并按原出发时点只替这一件成员再问一次，真实装配接 DepartureGateReader。
type MemberCarrier interface {
	CarriesMember(ctx context.Context, tenant domain.TenantID, unit, member domain.CarriedObjectReference, at time.Time) (bool, error)
	ReaskMember(ctx context.Context, fact domain.TransportMovementFact, unit, member domain.CarriedObjectReference) (domain.DepartureGateReading, error)
}

// ReaskOnNodeIntakeCommand 是节点收寄形成信封上的判断键，两维同节点作业收寄册的键。
type ReaskOnNodeIntakeCommand struct {
	TenantID domain.TenantID
	Source   string
}

// ReaskDepartureGatesOnIdentifiedMemberDeps 全部必备。
type ReaskDepartureGatesOnIdentifiedMemberDeps struct {
	Identified ports.IdentifiedMemberView
	Segments   ObjectSegmentView
	Facts      ports.MovementFactsOfSegment
	Reasks     ports.DepartureGateReaskRegistry
	Gate       MemberCarrier
	Handoff    ports.DepartureGateReaskHandoff
	Clock      ports.Clock
}

// ReaskDepartureGatesOnIdentifiedMemberHandler 是成员补问的进程内入口：本上下文消费节点作业的节点收寄形成信封，替那件新认出的包裹把
// 它随袋走过的出发补问一遍。
//
// 一拍分两步，理由同 ReaskDepartureGatesHandler：关务的判断在它自己的事务里记下、在调用方事务里即拒（ADR-0206 决定六），Plan 在任何
// 写事务之外读、问关务；Record 在写事务里把问来的各份追加落册、各交一封信。
type ReaskDepartureGatesOnIdentifiedMemberHandler struct {
	deps ReaskDepartureGatesOnIdentifiedMemberDeps
}

var errReaskDepartureGatesOnIdentifiedMemberDepsMissing = errors.New(
	"transport fulfillment: the member departure gate reask needs the identified member view, segments, facts, reasks, the gate, a handoff and a clock")

// NewReaskDepartureGatesOnIdentifiedMemberHandler 构造期拒缺件：漏装的一口到第一封信才暴露，会被当成读不动反复重投。
func NewReaskDepartureGatesOnIdentifiedMemberHandler(
	deps ReaskDepartureGatesOnIdentifiedMemberDeps,
) (*ReaskDepartureGatesOnIdentifiedMemberHandler, error) {
	if deps.Identified == nil || deps.Segments == nil || deps.Facts == nil || deps.Reasks == nil || deps.Gate == nil ||
		deps.Handoff == nil || deps.Clock == nil {
		return nil, errReaskDepartureGatesOnIdentifiedMemberDepsMissing
	}
	return &ReaskDepartureGatesOnIdentifiedMemberHandler{deps: deps}, nil
}

// Plan 按信封键回读收寄；它没有认出包裹就收工。认出了，就向节点作业问这件实物曾被装进哪几件单元（逐层向上，候选只放宽），在每件候选
// 单元有参与的段上按段列出声明了它的在册出发，凡原版出发、出发那一刻单元在控、这件包裹正随单元走（逐层往下验证，收紧的只此一处）、
// 且这条出发对它还没有定论、补问次数没到上限的，按原出发时点只替这一件包裹问一次。
//
// 一条出发不论经几件候选单元到达，只问一次：同一条出发替同一件成员的下一次补问次序只有一个，问两次会在落册时撞键。
func (handler *ReaskDepartureGatesOnIdentifiedMemberHandler) Plan(
	ctx context.Context,
	command ReaskOnNodeIntakeCommand,
) (DepartureGateReaskResult, error) {
	identity, err := handler.deps.Identified.FormedIntakeIdentity(ctx, command.TenantID, command.Source)
	switch {
	case errors.Is(err, ports.ErrFormedIntakeUntranslatable):
		return departureGateReaskStopped(DepartureGateReaskDefect, DepartureGateReaskIntakeInconsistent), nil
	case err != nil:
		return departureGateReaskStopped(DepartureGateReaskUndecided, DepartureGateReaskIntakeNotVisible), nil
	case !identity.Identified:
		return DepartureGateReaskResult{outcome: DepartureGateReaskMemberNotIdentified}, nil
	}
	units, stopped := handler.unitsAbove(ctx, command.TenantID, identity.Handling)
	if stopped.outcome != DepartureGateReaskOutcomeInvalid {
		return stopped, nil
	}

	trigger := "node-intake/" + command.Source
	result := DepartureGateReaskResult{outcome: DepartureGateReaskScanned}
	settled := map[ports.MovementFactKey]bool{}
	for _, unit := range units {
		keys, err := handler.deps.Segments.FindSegmentsForObject(ctx, command.TenantID, unit)
		if err != nil {
			return departureGateReaskStopped(DepartureGateReaskUndecided, DepartureGateReaskSegmentRegistryUnavailable), nil
		}
		for _, key := range keys {
			segment, stopped := segmentOfKey(ctx, handler.deps.Segments, key, command.TenantID)
			if stopped.outcome != DepartureGateReaskOutcomeInvalid {
				return stopped, nil
			}
			records, stopped := factsOfSegment(ctx, handler.deps.Facts, key)
			if stopped.outcome != DepartureGateReaskOutcomeInvalid {
				return stopped, nil
			}
			for _, record := range records {
				if settled[record.Key] {
					continue
				}
				step, err := handler.reaskMember(ctx, key, segment, record, unit, identity.Parcel, trigger)
				if err != nil {
					return DepartureGateReaskResult{}, err
				}
				if step.stopped.outcome != DepartureGateReaskOutcomeInvalid {
					return step.stopped, nil
				}
				if step.exhausted {
					result.exhausted++
					settled[record.Key] = true
				}
				if step.draft != nil {
					result.drafts = append(result.drafts, *step.draft)
					settled[record.Key] = true
				}
			}
		}
	}
	return result, nil
}

// unitsAbove 由这件实物逐层向上列出它曾被装进的每一件单元，宽度优先、每件只问一次：嵌套的袋同样可以被别的袋装过，成环的册也不会
// 绕不出来。单元标识立不成载运对象引用是节点作业的答复不可译，重问也是同一个结果。
func (handler *ReaskDepartureGatesOnIdentifiedMemberHandler) unitsAbove(
	ctx context.Context,
	tenant domain.TenantID,
	handling string,
) ([]domain.CarriedObjectReference, DepartureGateReaskResult) {
	seen := map[string]bool{handling: true}
	queue := []string{handling}
	var units []domain.CarriedObjectReference
	for len(queue) > 0 {
		member := queue[0]
		queue = queue[1:]
		holders, err := handler.deps.Identified.UnitsEverHolding(ctx, tenant, member)
		if errors.Is(err, ports.ErrConsolidationMembershipUntranslatable) {
			return nil, departureGateReaskStopped(DepartureGateReaskDefect, DepartureGateReaskMembershipUntranslatable)
		}
		if err != nil {
			return nil, departureGateReaskStopped(DepartureGateReaskUndecided, DepartureGateReaskConsolidationUnitsUnavailable)
		}
		for _, holder := range holders {
			if seen[holder] {
				continue
			}
			seen[holder] = true
			unit, err := domain.NewCarriedObjectReference(holder)
			if err != nil {
				return nil, departureGateReaskStopped(DepartureGateReaskDefect, DepartureGateReaskMembershipUntranslatable)
			}
			units = append(units, unit)
			queue = append(queue, holder)
		}
	}
	return units, DepartureGateReaskResult{}
}

// reaskMember 判一条按段列出的事实要不要替 member 补问，要就问。先看补问册（便宜），确定还没有定论才去验证成员是不是随单元走（要读
// 整袋的成员与每件的身份）。
func (handler *ReaskDepartureGatesOnIdentifiedMemberHandler) reaskMember(
	ctx context.Context,
	key ports.FulfillmentSegmentKey,
	segment domain.ActualFulfillmentSegment,
	record ports.MovementFactRecord,
	unit, member domain.CarriedObjectReference,
	trigger string,
) (departureGateReaskStep, error) {
	fact := record.Fact
	first, original, stopped := originalDepartureOf(key, record)
	if stopped.outcome != DepartureGateReaskOutcomeInvalid {
		return departureGateReaskStep{stopped: stopped}, nil
	}
	if !original {
		return departureGateReaskStep{}, nil
	}
	if !slices.Contains(segment.ObjectsUnderControlAt(fact.OccurredAt()), unit) {
		return departureGateReaskStep{}, nil
	}

	chain, stopped := reaskChainOf(ctx, handler.deps.Reasks, record.Key, member)
	if stopped.outcome != DepartureGateReaskOutcomeInvalid {
		return departureGateReaskStep{stopped: stopped}, nil
	}
	if departureGateVerdictOnAnotherChain(first, chain.others, member) || !departureGateReaskDue(first, chain.earlier, member) {
		return departureGateReaskStep{}, nil
	}
	if chain.last >= MaxDepartureGateReasks {
		return departureGateReaskStep{exhausted: true}, nil
	}

	carried, err := handler.deps.Gate.CarriesMember(ctx, record.Key.TenantID, unit, member, fact.OccurredAt())
	switch {
	case errors.Is(err, ports.ErrConsolidationMembershipUntranslatable):
		return departureGateReaskStep{stopped: departureGateReaskStopped(DepartureGateReaskDefect, DepartureGateReaskMembershipUntranslatable)}, nil
	case err != nil:
		return departureGateReaskStep{stopped: departureGateReaskStopped(DepartureGateReaskUndecided, DepartureGateReaskMembershipUnavailable)}, nil
	case !carried:
		return departureGateReaskStep{}, nil
	}

	reading, err := handler.deps.Gate.ReaskMember(ctx, fact, unit, member)
	if err != nil {
		return departureGateReaskStep{}, err
	}
	return departureGateReaskStep{draft: &ports.DepartureGateReaskRecord{
		Key:     reaskKeyAfter(record.Key, member, chain),
		Reading: reading,
		Trigger: trigger,
	}}, nil
}

// Record 在调用方的写事务里把 Plan 问来的各份追加落册，每份同一笔交一封信；撞键、交不出去的处置同 ReaskDepartureGatesHandler.Record。
func (handler *ReaskDepartureGatesOnIdentifiedMemberHandler) Record(
	ctx context.Context,
	planned DepartureGateReaskResult,
) (DepartureGateReaskResult, error) {
	return recordDepartureGateReasks(ctx, handler.deps.Reasks, handler.deps.Handoff, handler.deps.Clock, planned)
}

// departureGateVerdictOnAnotherChain 说这条出发的首版读数，或替别的对象（常是装着它的那件单元，ADR-0305）补问的某一份读数里，member
// 这件包裹已有定论。成员经袋展开时在那几份里也有自己的一格：已有定论就是关务给过的答复，按原出发时点再问不会变，不重复问。
func departureGateVerdictOnAnotherChain(
	first domain.DepartureGateReading,
	others []ports.DepartureGateReaskRecord,
	member domain.CarriedObjectReference,
) bool {
	readings := make([]domain.DepartureGateReading, 0, len(others)+1)
	readings = append(readings, first)
	for _, other := range others {
		readings = append(readings, other.Reading)
	}
	for _, reading := range readings {
		findings := reading.Findings()
		byObject := make(map[domain.CarriedObjectReference]domain.GateFinding, len(findings))
		for _, finding := range findings {
			byObject[finding.Object()] = finding
		}
		if finding, found := byObject[member]; found && !reaskableFinding(byObject, finding, map[domain.CarriedObjectReference]bool{member: true}) {
			return true
		}
	}
	return false
}
