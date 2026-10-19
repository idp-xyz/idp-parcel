package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件证权威交接作参与依据时由交接键铸（ADR-0221）：拼法、单射、进段与结束各指回自己那一次交接，以及只带版本的旧拼法
// 存量照读、按版本加时刻认。夹具全是合成串，证据只记 S。

func handoverIn(t *testing.T, object, scope, version string, at time.Time) domain.TransportHandover {
	t.Helper()
	spec := handoverSpec(t, object, domain.ObjectHandedOver)
	spec.Scope = mustValue(t, domain.NewHandoverScopeReference, scope)
	spec.Version = mustValue(t, domain.NewHandoverResultVersion, version)
	spec.JudgedAt = at
	handover, err := domain.FormTransportHandover(spec)
	if err != nil {
		t.Fatalf("form handover %s/%s: %v", scope, version, err)
	}
	return handover
}

func keyedBasis(t *testing.T, handover domain.TransportHandover) domain.ParticipationBasisReference {
	t.Helper()
	basis, formed := handover.ParticipationBasis()
	if !formed {
		t.Fatalf("`已交接`给不出参与依据：%s/%s", handover.Scope(), handover.Version())
	}
	return basis
}

// stockSegment 摆一段 ADR-0221 之前的存量：对象凭旧拼法 `TRANSPORT-HANDOVER/<版本>` 入场，起点 enteredAt；ended 为真时以
// 同样只带版本的终点离场。
func stockSegment(t *testing.T, object, entryVersion string, enteredAt time.Time, endVersion string, endedAt time.Time) domain.ActualFulfillmentSegment {
	t.Helper()
	row := domain.RehydrateParticipationSpec{
		Object:     mustValue(t, domain.NewCarriedObjectReference, object),
		EntryKind:  domain.EnteredByTransportHandover,
		EntryBasis: mustValue(t, domain.NewParticipationBasisReference, "TRANSPORT-HANDOVER/"+entryVersion),
		EnteredAt:  enteredAt,
	}
	if endVersion != "" {
		row.EndKind = domain.EndedByNextHandover
		row.EndBasis = mustValue(t, domain.NewParticipationBasisReference, "TRANSPORT-HANDOVER/"+endVersion)
		row.EndedAt = endedAt
	}
	segment, err := domain.RehydrateActualFulfillmentSegment(domain.RehydrateActualFulfillmentSegmentSpec{
		TenantID:       mustValue(t, domain.NewTenantID, "tenant-1"),
		Segment:        mustValue(t, domain.NewFulfillmentSegmentReference, "SEG-STOCK"),
		Participations: []domain.RehydrateParticipationSpec{row},
	})
	if err != nil {
		t.Fatalf("重建存量段：%v", err)
	}
	return segment
}

// Covers: AT-TF-266（ADR-0221）——`已交接`的参与依据由交接键铸：`TRANSPORT-HANDOVER-KEY/` 后接对象、范围、版本各一节
// 「字节长度:原串」。拒收与待确认给不出。同一对象在两个范围同用一个版本号铸出两串；三件以斜杠直拼会撞的输入，写上长度
// 之后也不撞。NO 的转出引用不随之换拼法，仍是只带版本的旧串。
func TestTheParticipationBasisIsMintedFromTheHandoverKey(t *testing.T) {
	handover := formedHandover(t, "parcel-1", domain.ObjectHandedOver)
	const want = "TRANSPORT-HANDOVER-KEY/8:parcel-1/16:handover-scope-1/27:handover-result/parcel-1/v1"
	if got := keyedBasis(t, handover).String(); got != want {
		t.Fatalf("参与依据 = %q, want %q", got, want)
	}
	if reference, _ := handover.TransferOutBasis(); reference != "TRANSPORT-HANDOVER/handover-result/parcel-1/v1" {
		t.Fatalf("NO 的转出引用 = %q, want 仍是只带版本的旧串", reference)
	}
	for _, verdict := range []domain.HandoverVerdict{domain.HandoverRefused, domain.HandoverPendingConfirmation} {
		if _, formed := formedHandover(t, "parcel-1", verdict).ParticipationBasis(); formed {
			t.Fatalf("%s 给出了参与依据", verdict)
		}
	}

	at := handoverJudgedAt
	sameVersion := []domain.TransportHandover{handoverIn(t, "parcel-1", "SZX", "v1", at), handoverIn(t, "parcel-1", "SIN", "v1", at)}
	if keyedBasis(t, sameVersion[0]) == keyedBasis(t, sameVersion[1]) {
		t.Fatal("两个范围同用 v1 铸出同一串")
	}
	// 范围 `a/1:b` 加版本 `c` 与范围 `a` 加版本 `1:b/c`：斜杠直拼都是 `a/1:b/c`。
	concatenated := []domain.TransportHandover{handoverIn(t, "parcel-1", "a/1:b", "c", at), handoverIn(t, "parcel-1", "a", "1:b/c", at)}
	if keyedBasis(t, concatenated[0]) == keyedBasis(t, concatenated[1]) {
		t.Fatal("斜杠直拼会撞的两组键铸出同一串")
	}
}

// Covers: AT-TF-266（ADR-0221）——进段、结束都记按键铸的依据，各自只认得出自己那一次交接：同一对象同一版本号、同一业务
// 时间、不同范围的另一次交接，EnteredByHandover 与 EndedByHandover 都答不是。入场依据读得回版本；新铸法里的对象不是这条
// 参与的对象时不认。
func TestKeyedBasesTellTwoScopesReusingAVersionApart(t *testing.T) {
	at := handoverJudgedAt
	intoSZX := handoverIn(t, "parcel-1", "SZX", "v1", at)
	sameVersionElsewhere := handoverIn(t, "parcel-1", "SHA", "v1", at)
	outOfSIN := handoverIn(t, "parcel-1", "SIN", "v1", at.Add(time.Hour))

	segment, err := domain.EstablishSegmentWithHandover(mustValue(t, domain.NewFulfillmentSegmentReference, "SEG-1"), intoSZX, domain.PlannedSegmentReference{})
	if err != nil {
		t.Fatalf("立段：%v", err)
	}
	segment, err = segment.EndParticipationWithNextHandover(outOfSIN)
	if err != nil {
		t.Fatalf("结束参与：%v", err)
	}
	participation, _ := segment.ParticipationFor(intoSZX.Object())
	if !participation.EnteredByHandover(intoSZX) || participation.EnteredByHandover(sameVersionElsewhere) {
		t.Fatalf("入场认法：自己 %v，同版本另一范围 %v", participation.EnteredByHandover(intoSZX), participation.EnteredByHandover(sameVersionElsewhere))
	}
	if match := participation.EndedByHandover(outOfSIN); match != domain.ControlFactMatchedExactly {
		t.Fatalf("终点认法 = %d, want 逐字对上", match)
	}
	if _, match := segment.ParticipationEndedByHandover(handoverIn(t, "parcel-1", "SINLM", "v1", at.Add(time.Hour))); match != domain.ControlFactUnmatched {
		t.Fatalf("同版本另一范围认成了终点：%d", match)
	}
	if version, named := participation.EntryHandoverVersion(); !named || version.String() != "v1" {
		t.Fatalf("入场版本 = %q %v", version, named)
	}

	foreign, err := domain.RehydrateActualFulfillmentSegment(domain.RehydrateActualFulfillmentSegmentSpec{
		TenantID: mustValue(t, domain.NewTenantID, "tenant-1"),
		Segment:  mustValue(t, domain.NewFulfillmentSegmentReference, "SEG-FOREIGN"),
		Participations: []domain.RehydrateParticipationSpec{{
			Object:     mustValue(t, domain.NewCarriedObjectReference, "parcel-2"),
			EntryKind:  domain.EnteredByTransportHandover,
			EntryBasis: keyedBasis(t, intoSZX),
			EnteredAt:  at,
		}},
	})
	if err != nil {
		t.Fatalf("重建：%v", err)
	}
	stray, _ := foreign.ParticipationFor(mustValue(t, domain.NewCarriedObjectReference, "parcel-2"))
	if _, named := stray.EntryHandoverVersion(); named {
		t.Fatal("依据里的对象不是这条参与的对象，却读出了入场版本")
	}
}

// Covers: AT-TF-268（ADR-0221）——旧拼法存量照读：只带版本的终点，版本与终点时刻都对上这一版交接才算，答「按版本与时刻对上」；
// 时刻对不上答不是。入场一侧同理，读得回版本。
func TestLegacyBasesAreReadByVersionAndInstant(t *testing.T) {
	at := handoverJudgedAt
	stock := stockSegment(t, "parcel-1", "v2", at, "v1", at.Add(time.Hour))
	participation, _ := stock.ParticipationFor(mustValue(t, domain.NewCarriedObjectReference, "parcel-1"))

	if match := participation.EndedByHandover(handoverIn(t, "parcel-1", "SIN", "v1", at.Add(time.Hour))); match != domain.ControlFactMatchedByVersionAndTime {
		t.Fatalf("旧拼法终点 = %d, want 按版本与时刻对上", match)
	}
	if match := participation.EndedByHandover(handoverIn(t, "parcel-1", "SINLM", "v1", at.Add(2*time.Hour))); match != domain.ControlFactUnmatched {
		t.Fatalf("业务时间对不上却认成了终点：%d", match)
	}
	if !participation.EnteredByHandover(handoverIn(t, "parcel-1", "SZX", "v2", at)) ||
		participation.EnteredByHandover(handoverIn(t, "parcel-1", "SZX", "v2", at.Add(time.Minute))) {
		t.Fatal("旧拼法入场没按版本加起点认")
	}
	if version, named := participation.EntryHandoverVersion(); !named || version.String() != "v2" {
		t.Fatalf("旧拼法入场版本 = %q %v", version, named)
	}
}

// Covers: AT-TF-272（ADR-0221）——存量参与凭旧拼法入场，同一（对象，范围）的更正照样重派生：新版本记按键铸的依据，回指写
// 在册的旧串原样；另一个范围同用那个版本号、业务时间不同的更正不牵连这一段。
func TestAHandoverCorrectionRederivesAStockParticipationOnTheLegacyBasis(t *testing.T) {
	at := handoverJudgedAt
	original := handoverIn(t, "parcel-1", "SZX", "v1", at)
	stock := stockSegment(t, "parcel-1", "v1", at, "", time.Time{})
	releasing, _ := original.ReleasingEvidence()
	receiving, _ := original.ReceivingEvidence()
	rule, _ := original.Rule()
	correction := domain.HandoverCorrection{
		Verdict: domain.ObjectHandedOver, ReleasingEvidence: releasing, ReceivingEvidence: receiving, Rule: rule,
		Version: mustValue(t, domain.NewHandoverResultVersion, "v2"), CorrectedAt: at.Add(3 * time.Hour),
	}

	corrected, err := original.Correct(correction)
	if err != nil {
		t.Fatalf("更正：%v", err)
	}
	rederived, err := stock.RederiveParticipationWithHandover(corrected)
	if err != nil {
		t.Fatalf("存量参与没被重派生：%v", err)
	}
	current, _ := rederived.ParticipationFor(original.Object())
	if current.EntryBasis() != keyedBasis(t, corrected) {
		t.Fatalf("替代版本的入场依据 = %s, want 按键铸的那一串", current.EntryBasis())
	}
	if supersedes, chained := current.Supersedes(); !chained || supersedes.String() != "TRANSPORT-HANDOVER/v1" {
		t.Fatalf("回指 = %q %v, want 在册的旧串原样", supersedes, chained)
	}

	elsewhere, err := handoverIn(t, "parcel-1", "SHA", "v1", at.Add(-2*time.Hour)).Correct(correction)
	if err != nil {
		t.Fatalf("另一范围的更正：%v", err)
	}
	if _, err := stock.RederiveParticipationWithHandover(elsewhere); !errors.Is(err, domain.ErrNoParticipationToRederive) {
		t.Fatalf("另一范围同版本号的更正牵连了这一段：err = %v", err)
	}
}
