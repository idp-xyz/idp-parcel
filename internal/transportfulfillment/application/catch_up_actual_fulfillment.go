package application

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 实际履约补形成与实际履约未形成册（ADR-0231 决定一、二、五、六）。
//
// 自动形成随交接与交付信封判（ADR-0218），判的是派发处理那封信的那一刻：段那时采没采用委托，委托那一版协议那时怎么答。采用晚于那一刻、
// 或委托此后换了版，那一刻答的「不形成」就没有东西回头再问。补形成就是那个回头：采用成立、委托换版各在自己那一笔交一封门铃，派发进程
// 收到门铃，把那一段（换版时是采用了这份委托的各段）上已结束的参与逐件交回自动形成的同一个后半（formEnded）——同一个登记用例、同一个
// 旅程取法、同一个系统登记者。所以补形成与自动形成落在同一个身份上，已形成的答已存在，重投与重放都不多出一项。
//
// 不在采用或换版那一笔里内联（ADR-0207 决定七前半句不变）：发生项登记要回问 party-commercial，读不动时采用与换版不该跟着失败；门铃
// 跑在派发进程的 inbox 事务里，读不动整笔回滚、门铃重投。
//
// 只补以下一次权威交接或有效交付结束的参与：自动形成只对这两种终点形成，补了明确终止结束的，形不形成又随采用早晚而异，正是补形成要消掉的
// 那种不确定。

// CatchUpActualFulfillmentOutcome 是一封门铃的结果代数。
type CatchUpActualFulfillmentOutcome uint8

const (
	CatchUpActualFulfillmentOutcomeInvalid CatchUpActualFulfillmentOutcome = iota
	// CatchUpAnswered：各段上已结束的参与都交过一遍，逐件的答复在 Formations 里。
	CatchUpAnswered
	// CatchUpSegmentNotAdopted：门铃指名的段此刻没有采用连线。采用之后不解除的今天走不到；日后有了解除，那一段不再按委托补。
	CatchUpSegmentNotAdopted
	// CatchUpNotAccepted：本上下文自己交出的门铃键过不了自己的门，是缺陷。
	CatchUpNotAccepted
	// CatchUpUndecided：某一口此刻读不动，整笔回滚、门铃重投；停在哪一口在 UndecidedReason 里。
	CatchUpUndecided
)

func (outcome CatchUpActualFulfillmentOutcome) String() string {
	switch outcome {
	case CatchUpAnswered:
		return "CATCH_UP_ANSWERED"
	case CatchUpSegmentNotAdopted:
		return "SEGMENT_NOT_ADOPTED"
	case CatchUpNotAccepted:
		return "INPUT_NOT_ACCEPTED"
	case CatchUpUndecided:
		return "CATCH_UP_UNDECIDED"
	default:
		return ""
	}
}

// CatchUpActualFulfillmentOnAdoptionCommand 是采用门铃上的键：哪个租户的哪一段采用了委托。
type CatchUpActualFulfillmentOnAdoptionCommand struct {
	TenantID domain.TenantID
	Segment  string
}

// CatchUpActualFulfillmentOnRevisionCommand 是换版门铃上的键：哪个租户的哪一份委托换到了第几版。号只进运维读的那句话：补形成按委托此刻
// 的各次换版判，不按门铃上的号。
type CatchUpActualFulfillmentOnRevisionCommand struct {
	TenantID   domain.TenantID
	Commission string
	Revision   int
}

type CatchUpActualFulfillmentResult struct {
	outcome    CatchUpActualFulfillmentOutcome
	reason     ActualFulfillmentFormationUndecidedReason
	formations []FormActualFulfillmentResult
}

func (result CatchUpActualFulfillmentResult) Outcome() CatchUpActualFulfillmentOutcome {
	return result.outcome
}

// UndecidedReason 只在`未决`时非零：补形成自己停在哪一口，或某一件的形成停在哪一口。
func (result CatchUpActualFulfillmentResult) UndecidedReason() ActualFulfillmentFormationUndecidedReason {
	return result.reason
}

// Formations 交回逐件的答复，按段、再按对象的次序。
func (result CatchUpActualFulfillmentResult) Formations() []FormActualFulfillmentResult {
	return slices.Clone(result.formations)
}

// Disposition 把一封门铃的答复折成对信封的处置，判据同 FormActualFulfillmentResult.Disposition：逐件里有一件是缺陷，整封是缺陷；有一件
// 要重投，整封重投（整笔回滚，已答的各件下一次再答一遍，同一身份落回同一项）；都入账才入账。
func (result CatchUpActualFulfillmentResult) Disposition() ActualFulfillmentFormationDisposition {
	switch result.outcome {
	case CatchUpSegmentNotAdopted:
		return FormationSettled
	case CatchUpUndecided:
		return FormationRetry
	case CatchUpAnswered:
		disposition := FormationSettled
		for _, formed := range result.formations {
			switch formed.Disposition() {
			case FormationSettled:
			case FormationRetry:
				disposition = FormationRetry
			default:
				return FormationDefect
			}
		}
		return disposition
	default:
		return FormationDefect
	}
}

// CatchUpOnAdoption 跑采用门铃：受理键 → 复核那一段此刻仍采用着委托 → 对段上已结束的参与逐件补形成。
func (handler *FormActualFulfillmentOnParticipationEndHandler) CatchUpOnAdoption(
	ctx context.Context,
	command CatchUpActualFulfillmentOnAdoptionCommand,
) (CatchUpActualFulfillmentResult, error) {
	segment, err := domain.NewFulfillmentSegmentReference(command.Segment)
	if err != nil || strings.TrimSpace(command.TenantID.String()) == "" {
		return CatchUpActualFulfillmentResult{outcome: CatchUpNotAccepted}, nil
	}
	adoption, adopted, err := handler.deps.Adoptions.FindBySegment(ctx, ports.SegmentCommissionAdoptionKey{TenantID: command.TenantID, Segment: segment})
	if err != nil {
		return catchUpUndecided(FormationAdoptionRegistryUnavailable), nil
	}
	if !adopted {
		return CatchUpActualFulfillmentResult{outcome: CatchUpSegmentNotAdopted}, nil
	}
	var result CatchUpActualFulfillmentResult
	if stop, err := handler.catchUpSegment(ctx, command.TenantID, adoption, &result); err != nil || stop != FormationUndecidedReasonNone {
		return catchUpStopped(stop, err)
	}
	result.outcome = CatchUpAnswered
	return result, nil
}

// CatchUpOnRevision 跑换版门铃：受理键 → 此刻列出采用了这份委托的各段 → 逐段补形成。在门铃消费的这一刻才列：换版落下之后才采用这份委托
// 的段，由它自己的采用门铃补，那一封到时委托已经换了版。
func (handler *FormActualFulfillmentOnParticipationEndHandler) CatchUpOnRevision(
	ctx context.Context,
	command CatchUpActualFulfillmentOnRevisionCommand,
) (CatchUpActualFulfillmentResult, error) {
	commission, err := domain.NewTransportCommissionReference(command.Commission)
	if err != nil || strings.TrimSpace(command.TenantID.String()) == "" || command.Revision <= 1 {
		return CatchUpActualFulfillmentResult{outcome: CatchUpNotAccepted}, nil
	}
	adoptions, err := handler.deps.AdoptionsByCommission.ListByCommission(ctx, ports.TransportCommissionKey{TenantID: command.TenantID, Commission: commission})
	if err != nil {
		return catchUpUndecided(FormationAdoptionRegistryUnavailable), nil
	}
	var result CatchUpActualFulfillmentResult
	for _, adoption := range adoptions {
		if stop, err := handler.catchUpSegment(ctx, command.TenantID, adoption, &result); err != nil || stop != FormationUndecidedReasonNone {
			return catchUpStopped(stop, err)
		}
	}
	result.outcome = CatchUpAnswered
	return result, nil
}

// catchUpSegment 对一段上已结束的参与逐件交 formEnded，答复追加进 result。哪一件未决就停在那一件：整封会回滚重投，往下问只是白问。
func (handler *FormActualFulfillmentOnParticipationEndHandler) catchUpSegment(
	ctx context.Context,
	tenant domain.TenantID,
	adoption ports.SegmentCommissionAdoptionRecord,
	result *CatchUpActualFulfillmentResult,
) (ActualFulfillmentFormationUndecidedReason, error) {
	segmentRef := adoption.Key.Segment
	record, found, err := handler.deps.Segments.FindByKey(ctx, ports.FulfillmentSegmentKey{TenantID: tenant, Segment: segmentRef})
	if err != nil {
		return FormationSegmentRegistryUnavailable, nil
	}
	if !found {
		// 采用连线落册时段在册（迁移 0035 的外键），段又不删；读不回是库里的数据违约。
		return FormationUndecidedReasonNone, fmt.Errorf("catch up actual fulfillment: adopted segment %s is not registered", segmentRef)
	}
	for _, participation := range endedByHandoverOrDelivery(record.Segment) {
		formed, err := handler.formEnded(ctx, tenant, segmentRef, adoption, participation)
		if err != nil {
			return FormationUndecidedReasonNone, err
		}
		result.formations = append(result.formations, formed)
		if formed.Outcome() == FormationUndecided {
			return formed.UndecidedReason(), nil
		}
	}
	return FormationUndecidedReasonNone, nil
}

// endedByHandoverOrDelivery 取一段上各对象的当前参与（替代链链尾、不是失效版本）里，以下一次权威交接或有效交付结束的那几条，按对象排序。
func endedByHandoverOrDelivery(segment domain.ActualFulfillmentSegment) []domain.FulfillmentParticipation {
	var ended []domain.FulfillmentParticipation
	for _, participation := range segment.Participations() {
		if participation.Superseded() || participation.Voided() {
			continue
		}
		kind, _, _, done := participation.End()
		if done && (kind == domain.EndedByNextHandover || kind == domain.EndedByEffectiveDelivery) {
			ended = append(ended, participation)
		}
	}
	slices.SortFunc(ended, func(left, right domain.FulfillmentParticipation) int {
		return strings.Compare(left.Object().String(), right.Object().String())
	})
	return ended
}

func catchUpUndecided(reason ActualFulfillmentFormationUndecidedReason) CatchUpActualFulfillmentResult {
	return CatchUpActualFulfillmentResult{outcome: CatchUpUndecided, reason: reason}
}

func catchUpStopped(reason ActualFulfillmentFormationUndecidedReason, err error) (CatchUpActualFulfillmentResult, error) {
	if err != nil {
		return CatchUpActualFulfillmentResult{}, err
	}
	return catchUpUndecided(reason), nil
}

// keepUnformedRegister 照一次入账的登记答复写实际履约未形成册（ADR-0231 决定六），交回未决停在哪一口（零值即写成了）。
//
//   - 形成、已存在、冲突（同一身份异内容、成员或范围已被计过）：有一项发生项盖住这件参与，册上若有它就了结为已形成，记下那一项。
//   - 未配置、协议未生效、已终止、缺采购上下文、不计费：先问一次「是不是已有一项仍有效的发生项盖住它」。委托换版之后，参与终点所凭的那
//     一版可能已不是当初形成它的那一版，那一版答的「不形成」说的不是这件参与没有成本——盖住了就照已形成了结。没盖住时不计费了结为不
//     计费（只在册上已有它时），其余立或刷新为未形成。
//
// 问「盖住」先取发生项登记册的范围锁（FindCovering），再按身份读：同一段上并发的另一拍若正在形成同一件，后到者等它提交之后再判，
// 不会一笔记已形成、一笔记未形成。
func (handler *FormActualFulfillmentOnParticipationEndHandler) keepUnformedRegister(
	ctx context.Context,
	tenant domain.TenantID,
	segment domain.FulfillmentSegmentReference,
	adoption ports.SegmentCommissionAdoptionRecord,
	participation domain.FulfillmentParticipation,
	registered RegisterChargeOccurrenceResult,
) (ActualFulfillmentFormationUndecidedReason, error) {
	key := ports.ActualFulfillmentUnformedKey{TenantID: tenant, Segment: segment, Object: participation.Object()}
	now := handler.deps.Clock.Now().UTC()
	var reason ports.ActualFulfillmentUnformedReason
	switch registered.Outcome() {
	case ChargeOccurrenceRegistered, ChargeOccurrenceExisting, ChargeOccurrenceConflict:
		record, present := registered.Record()
		if !present {
			return FormationUndecidedReasonNone, fmt.Errorf("keep actual fulfillment unformed: %s without the occurrence it names", registered.Outcome())
		}
		return unformedWrite(handler.deps.Unformed.SettleFormed(ctx, key, record.Key, now))
	case ChargeOccurrenceNotConfigured:
		reason = ports.UnformedNotConfigured
	case ChargeOccurrenceAgreementNotEffective:
		reason = ports.UnformedAgreementNotEffective
	case ChargeOccurrenceAgreementTerminated:
		reason = ports.UnformedAgreementTerminated
	case ChargeOccurrenceInputNotAccepted:
		reason = ports.UnformedProcurementContextMissing
	case ChargeOccurrenceNotCharged:
	default:
		return FormationUndecidedReasonNone, fmt.Errorf("keep actual fulfillment unformed: settled answer %s outside the register's set", registered.Outcome())
	}

	covering, covered, err := handler.coveringOccurrence(ctx, tenant, segment, participation.Object())
	if err != nil {
		return FormationOccurrenceRegistryUnavailable, nil
	}
	if covered {
		return unformedWrite(handler.deps.Unformed.SettleFormed(ctx, key, covering.Key, now))
	}
	if registered.Outcome() == ChargeOccurrenceNotCharged {
		agreement, err := domain.NewAgreementSnapshotReference(registered.Agreement())
		if err != nil {
			return FormationUndecidedReasonNone, fmt.Errorf("keep actual fulfillment unformed: not charged without its agreement: %w", err)
		}
		return unformedWrite(handler.deps.Unformed.SettleNotCharged(ctx, key, agreement, now))
	}
	terms, named := registered.CommissionTerms()
	if !named {
		return FormationUndecidedReasonNone, fmt.Errorf("keep actual fulfillment unformed: %s on an adopted segment without the commission's terms", registered.Outcome())
	}
	_, _, endedAt, _ := participation.End()
	entry := ports.ActualFulfillmentUnformedEntry{
		Key:        key,
		Commission: adoption.Adoption.Commission(),
		Revision:   terms.Revision(),
		EndedAt:    endedAt,
		Reason:     reason,
	}
	if asked, present := registered.AskedAgreement(); present {
		entry.Agreement = asked
	}
	return unformedWrite(handler.deps.Unformed.RecordUnformed(ctx, entry, now))
}

// coveringOccurrence 问这件参与是不是已有一项发生项盖住：同一段上同一件事实已被另一项计过（成员交集或按段计过），或同一身份那一项
// 已在册。身份照登记用例的派生；按身份读只读首版，与登记用例的重放同一个读法。
func (handler *FormActualFulfillmentOnParticipationEndHandler) coveringOccurrence(
	ctx context.Context,
	tenant domain.TenantID,
	segment domain.FulfillmentSegmentReference,
	object domain.CarriedObjectReference,
) (ports.ChargeOccurrenceRecord, bool, error) {
	basis, err := domain.ActualFulfillmentBasis(segment)
	if err != nil {
		return ports.ChargeOccurrenceRecord{}, false, err
	}
	scope, err := domain.SegmentOccurrenceScope(segment)
	if err != nil {
		return ports.ChargeOccurrenceRecord{}, false, err
	}
	members := []domain.CarriedObjectReference{object}
	identity, err := domain.DeriveChargeOccurrenceReference(tenant, domain.ActualFulfillmentOccurrence, basis, members)
	if err != nil {
		return ports.ChargeOccurrenceRecord{}, false, err
	}
	covering, covered, err := handler.deps.Coverage.FindCovering(ctx, ports.ChargeOccurrenceCoverageQuery{
		TenantID:   tenant,
		Reason:     domain.ActualFulfillmentOccurrence,
		Scope:      scope,
		FactBasis:  basis,
		Members:    members,
		Except:     identity,
		ScopeUnits: domain.ScopeCountingUnits(),
	})
	if err != nil || covered {
		return covering, covered, err
	}
	return handler.deps.Coverage.FindByKey(ctx, ports.ChargeOccurrenceKey{
		TenantID: tenant, Occurrence: identity, Validity: domain.FirstOccurrenceValidity(),
	})
}

// unformedWrite 把册写口的失败分开：确定性拒绝是缺陷、上抛；其余是此刻写不动，答未决重投。
func unformedWrite(err error) (ActualFulfillmentFormationUndecidedReason, error) {
	switch {
	case err == nil:
		return FormationUndecidedReasonNone, nil
	case errors.Is(err, ports.ErrActualFulfillmentUnformedWriteRefused):
		return FormationUndecidedReasonNone, err
	default:
		return FormationUnformedRegisterUnavailable, nil
	}
}
