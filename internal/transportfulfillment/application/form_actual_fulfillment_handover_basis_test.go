package application_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证自动形成找「这次交接结束了哪一条参与」时认得出交接（ADR-0221，票 demo-chain-gaps/111）：同一包裹在不同范围的几次
// 交接都写 `v1`，按交接键铸的依据各指各的；ADR-0221 之前只带版本的存量依据按版本加终点时刻认，仍分不开就答一格、入账。
// 段与参与都按读回的形状摆，交接按登记的形状摆；夹具全是合成串，证据只记 S。

const selfSegment = "SYN-SEG-SHA-SZX-111"

var (
	selfEnteredAt = occurrenceEnteredAt.Add(-3 * time.Hour)
	selfEndedAt   = occurrenceEnteredAt.Add(-time.Hour)
)

// handedOverIn 登一版`已交接`：对象在 scope 下以 version 交出，业务时间 at。
func (rig *formationRig) handedOverIn(t *testing.T, object, scope, version string, at time.Time) application.FormActualFulfillmentOnHandoverCommand {
	t.Helper()
	spec := domain.TransportHandoverSpec{
		TenantID:          occurrenceValue(t, domain.NewTenantID, occurrenceTenant),
		Object:            occurrenceValue(t, domain.NewCarriedObjectReference, object),
		Scope:             occurrenceValue(t, domain.NewHandoverScopeReference, scope),
		ReleasedBy:        occurrenceValue(t, domain.NewHandoverPartyReference, "SYN-RELEASED-BY-"+scope),
		ReceivedBy:        occurrenceValue(t, domain.NewHandoverPartyReference, "SYN-RECEIVED-BY-"+scope),
		Verdict:           domain.ObjectHandedOver,
		ReleasingEvidence: occurrenceValue(t, domain.NewHandoverEvidenceReference, "SYN-EVIDENCE-RELEASE-"+scope),
		ReceivingEvidence: occurrenceValue(t, domain.NewHandoverEvidenceReference, "SYN-EVIDENCE-RECEIVE-"+scope),
		Rule:              occurrenceValue(t, domain.NewHandoverRuleReference, "SYN-HANDOVER-RULE/v1"),
		Version:           occurrenceValue(t, domain.NewHandoverResultVersion, version),
		JudgedAt:          at,
	}
	handover, err := domain.FormTransportHandover(spec)
	if err != nil {
		t.Fatalf("交接夹具：%v", err)
	}
	key := ports.TransportHandoverKey{TenantID: spec.TenantID, Object: spec.Object, Scope: spec.Scope, Version: spec.Version}
	rig.handovers.records[key] = ports.TransportHandoverRecord{Key: key, Handover: handover}
	return application.FormActualFulfillmentOnHandoverCommand{
		TenantID: spec.TenantID, Object: object, Scope: scope, Version: version,
	}
}

// endedRow 是一条凭交接入场、以下一次交接离场的参与，入场与终点依据由调用方给（新铸法或旧拼法）。
func endedRow(t *testing.T, object string, entry domain.ParticipationBasisReference, enteredAt time.Time,
	end domain.ParticipationBasisReference, endedAt time.Time) domain.RehydrateParticipationSpec {
	t.Helper()
	return domain.RehydrateParticipationSpec{
		Object:     occurrenceValue(t, domain.NewCarriedObjectReference, object),
		EntryKind:  domain.EnteredByTransportHandover,
		EntryBasis: entry,
		EnteredAt:  enteredAt,
		EndKind:    domain.EndedByNextHandover,
		EndBasis:   end,
		EndedAt:    endedAt,
	}
}

func legacyBasis(t *testing.T, version string) domain.ParticipationBasisReference {
	t.Helper()
	return occurrenceValue(t, domain.NewParticipationBasisReference, "TRANSPORT-HANDOVER/"+version)
}

// placeSelfSegment 在段登记册上另摆一段：对象先在这里（自营段，没有采用连线），离场之后才进干线段。
func placeSelfSegment(t *testing.T, rig *formationRig, row domain.RehydrateParticipationSpec) {
	t.Helper()
	tenant := occurrenceValue(t, domain.NewTenantID, occurrenceTenant)
	segment, err := domain.RehydrateActualFulfillmentSegment(domain.RehydrateActualFulfillmentSegmentSpec{
		TenantID:       tenant,
		Segment:        occurrenceValue(t, domain.NewFulfillmentSegmentReference, selfSegment),
		Participations: []domain.RehydrateParticipationSpec{row},
	})
	if err != nil {
		t.Fatalf("自营段夹具：%v", err)
	}
	rig.segments.records = append([]ports.FulfillmentSegmentRecord{{
		Key:     ports.FulfillmentSegmentKey{TenantID: tenant, Segment: segment.Segment()},
		Segment: segment,
	}}, rig.segments.records...)
}

// Covers: AT-TF-267（S，票 demo-chain-gaps/111 演示判据的编排半边）——同一包裹照演示动线登各次交接，范围各异、版本都写
// `v1`（深圳口岸交进与交出同用范围 SZX，版本 v1、v2）：自营段的终点是 SZX/v1，干线段的终点是 SIN/v1。SIN/v1 只认出
// 干线段、形成发生项；SZX/v1 认出自营段、段没有采用连线入账；SINLM/v1 没结束任何参与，入账。三封信都不响亮失败。
func TestHandoversReusingAVersionAcrossScopesEachFindTheirOwnParticipation(t *testing.T) {
	occurrences := adoptedTrunkRig(t, adoptedCommission(t))
	occurrences.segments = trunkSegmentWith(t, endedRow(t, trunkParcel,
		handoverParticipationBasis(t, trunkParcel, "SZX", "v2"), occurrenceEnteredAt,
		handoverParticipationBasis(t, trunkParcel, "SIN", "v1"), occurrenceEndedAt))
	rig := newFormationRig(t, occurrences)
	placeSelfSegment(t, rig, endedRow(t, trunkParcel,
		handoverParticipationBasis(t, trunkParcel, "SHA", "v1"), selfEnteredAt,
		handoverParticipationBasis(t, trunkParcel, "SZX", "v1"), selfEndedAt))

	atSIN := rig.onHandover(t, rig.handedOverIn(t, trunkParcel, "SIN", "v1", occurrenceEndedAt))
	if atSIN.Outcome() != application.FormationRegistrationAnswered || atSIN.Segment() != trunkSegment {
		t.Fatalf("SIN/v1 = %s 段 %s, want 交给登记用例、干线段", atSIN.Outcome(), atSIN.Segment())
	}
	registeredOccurrence(t, atSIN.Registration())
	assertFormationDisposition(t, atSIN, application.FormationSettled)

	atSZX := rig.onHandover(t, rig.handedOverIn(t, trunkParcel, "SZX", "v1", selfEndedAt))
	if atSZX.Outcome() != application.FormationSegmentNotAdopted || atSZX.Segment() != selfSegment {
		t.Fatalf("SZX/v1 = %s 段 %s, want 自营段没有采用连线", atSZX.Outcome(), atSZX.Segment())
	}
	assertFormationDisposition(t, atSZX, application.FormationSettled)

	atSINLM := rig.onHandover(t, rig.handedOverIn(t, trunkParcel, "SINLM", "v1", occurrenceEndedAt.Add(time.Hour)))
	if atSINLM.Outcome() != application.FormationNoParticipationEnded {
		t.Fatalf("SINLM/v1 = %s, want 没有参与被它结束", atSINLM.Outcome())
	}
	assertFormationDisposition(t, atSINLM, application.FormationSettled)
	if len(rig.spy.commands) != 1 {
		t.Fatalf("交给登记用例 %d 次, want 只有 SIN/v1 那一次", len(rig.spy.commands))
	}
}

// Covers: AT-TF-268（ADR-0221）——ADR-0221 之前的存量：两段参与的终点依据都是旧拼法 `TRANSPORT-HANDOVER/v1`。按版本加终点
// 时刻认：SIN/v1 只认出终点时刻是它业务时间的干线段，照常形成；业务时间对不上任何终点的 SINLM/v1 答没有参与被它结束。
func TestLegacyEndBasesAreToldApartByTheEndInstant(t *testing.T) {
	occurrences := adoptedTrunkRig(t, adoptedCommission(t))
	occurrences.segments = trunkSegmentWith(t, endedRow(t, trunkParcel,
		legacyBasis(t, "v2"), occurrenceEnteredAt, legacyBasis(t, "v1"), occurrenceEndedAt))
	rig := newFormationRig(t, occurrences)
	placeSelfSegment(t, rig, endedRow(t, trunkParcel, legacyBasis(t, "v1"), selfEnteredAt, legacyBasis(t, "v1"), selfEndedAt))

	atSIN := rig.onHandover(t, rig.handedOverIn(t, trunkParcel, "SIN", "v1", occurrenceEndedAt))
	if atSIN.Outcome() != application.FormationRegistrationAnswered || atSIN.Segment() != trunkSegment {
		t.Fatalf("SIN/v1 = %s 段 %s, want 交给登记用例、干线段", atSIN.Outcome(), atSIN.Segment())
	}
	registeredOccurrence(t, atSIN.Registration())

	atSINLM := rig.onHandover(t, rig.handedOverIn(t, trunkParcel, "SINLM", "v1", occurrenceEndedAt.Add(time.Hour)))
	if atSINLM.Outcome() != application.FormationNoParticipationEnded {
		t.Fatalf("SINLM/v1 = %s, want 没有参与被它结束", atSINLM.Outcome())
	}
}

// Covers: AT-TF-269（ADR-0221）——旧拼法存量里两段的终点版本与时刻都对得上同一次交接（登记方在两个范围用了同一个版本号、
// 业务时间也相同）：认不出结束的是哪一条，答 ENDED_PARTICIPATION_AMBIGUOUS、入账，不挑一段交给登记用例。
func TestLegacyEndBasesThatStillCollideAreSettledAsAmbiguous(t *testing.T) {
	occurrences := adoptedTrunkRig(t, adoptedCommission(t))
	occurrences.segments = trunkSegmentWith(t, endedRow(t, trunkParcel,
		legacyBasis(t, "v2"), occurrenceEnteredAt, legacyBasis(t, "v1"), occurrenceEndedAt))
	rig := newFormationRig(t, occurrences)
	placeSelfSegment(t, rig, endedRow(t, trunkParcel, legacyBasis(t, "v1"), selfEnteredAt, legacyBasis(t, "v1"), occurrenceEndedAt))

	result := rig.onHandover(t, rig.handedOverIn(t, trunkParcel, "SIN", "v1", occurrenceEndedAt))
	if result.Outcome() != application.FormationEndedParticipationAmbiguous || result.Outcome().String() != "ENDED_PARTICIPATION_AMBIGUOUS" {
		t.Fatalf("outcome = %s, want ENDED_PARTICIPATION_AMBIGUOUS", result.Outcome())
	}
	assertFormationDisposition(t, result, application.FormationSettled)
	if len(rig.spy.commands) != 0 {
		t.Fatalf("认不出是哪一段却交给了登记用例：%+v", rig.spy.commands)
	}
}

// Covers: AT-TF-270（ADR-0221）——逐字对上的优先：干线段的终点是这次交接按键铸的依据，自营段是一条旧拼法存量、版本与时刻
// 恰好也对得上。按键铸的那一条一定是这次交接，取它；旧拼法那一条不让它变成歧义。
func TestAnExactEndBasisWinsOverALegacyOneThatAlsoMatches(t *testing.T) {
	occurrences := adoptedTrunkRig(t, adoptedCommission(t))
	occurrences.segments = trunkSegmentWith(t, endedRow(t, trunkParcel,
		handoverParticipationBasis(t, trunkParcel, "SZX", "v2"), occurrenceEnteredAt,
		handoverParticipationBasis(t, trunkParcel, "SIN", "v1"), occurrenceEndedAt))
	rig := newFormationRig(t, occurrences)
	placeSelfSegment(t, rig, endedRow(t, trunkParcel, legacyBasis(t, "v1"), selfEnteredAt, legacyBasis(t, "v1"), occurrenceEndedAt))

	result := rig.onHandover(t, rig.handedOverIn(t, trunkParcel, "SIN", "v1", occurrenceEndedAt))
	if result.Outcome() != application.FormationRegistrationAnswered || result.Segment() != trunkSegment {
		t.Fatalf("outcome = %s 段 %s, want 交给登记用例、干线段", result.Outcome(), result.Segment())
	}
}
