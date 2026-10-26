package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件钉 ADR-0303 在段聚合上的领域半边：关段与明确终止把作出决定的操作者记在段与参与的终点上，只作操作证据——
// 转换门的任何一道判断都不读它，带与不带走出同一个状态；它跟着终点走，来源更正长出的替代版本承继终点时一并承继；
// 重建门照收存量里没有它的段，拒它出现在不能属于的位置。夹具全为合成引用（S 级）。

func operatorsForEvidence(t *testing.T) (closing, terminating domain.RegistrantReference) {
	t.Helper()
	return mustValue(t, domain.NewRegistrantReference, "https://id.syn.example/dex#SYN-OPERATOR-A"),
		mustValue(t, domain.NewRegistrantReference, "https://id.syn.example/dex#SYN-OPERATOR-B")
}

// Covers: `AT-TF-337`、`AT-TF-338` 领域半边——操作者随关闭与终止留痕，不改变转换门的任何一格：同一条参与带不带终止人
// 结束出同一个终点；同一个段带不带关段人关出同一个时刻；以交付结束的参与没有终止人；更正之后的替代版本承继终止人。
func TestTheClosingAndTerminatingOperatorsAreKeptWithoutGatingTheSegment(t *testing.T) {
	closingOperator, terminatingOperator := operatorsForEvidence(t)
	pickup := formedPickup(t, "parcel-1", "attempt-1")
	object := pickup.Object()
	basis := mustValue(t, domain.NewParticipationBasisReference, "CONTROL-TERMINATION/case-1")
	endedAt := pickedUpAt.Add(2 * time.Hour)
	closedAt := endedAt.Add(time.Hour)
	establish := func(t *testing.T) domain.ActualFulfillmentSegment {
		t.Helper()
		segment, err := domain.EstablishSegmentWithPickup(
			mustValue(t, domain.NewFulfillmentSegmentReference, "SEG-1"), pickup, domain.PlannedSegmentReference{})
		if err != nil {
			t.Fatalf("establish: %v", err)
		}
		return segment
	}

	attributed, err := establish(t).EndParticipationWithTermination(object, basis, endedAt, terminatingOperator)
	if err != nil {
		t.Fatalf("带终止人终止：%v", err)
	}
	unattributed, err := establish(t).EndParticipationWithTermination(object, basis, endedAt, domain.RegistrantReference{})
	if err != nil {
		t.Fatalf("不带终止人终止：%v", err)
	}
	withBy, _ := attributed.ParticipationFor(object)
	withoutBy, _ := unattributed.ParticipationFor(object)
	kind, endBasis, at, ended := withBy.End()
	otherKind, otherBasis, otherAt, otherEnded := withoutBy.End()
	if !ended || !otherEnded || kind != otherKind || endBasis != otherBasis || !at.Equal(otherAt) {
		t.Fatalf("终止人改变了终点：%v %v %v / %v %v %v", kind, endBasis, at, otherKind, otherBasis, otherAt)
	}
	if by, recorded := withBy.TerminatedBy(); !recorded || by != terminatingOperator {
		t.Fatalf("终止人 = %q (%v)，want %q", by.String(), recorded, terminatingOperator.String())
	}
	if _, recorded := withoutBy.TerminatedBy(); recorded {
		t.Fatal("不带终止人的终止不该凭空记一个人")
	}

	closed, err := attributed.CloseSegment(closedAt, closingOperator)
	if err != nil {
		t.Fatalf("带关段人关段：%v", err)
	}
	closedWithout, err := unattributed.CloseSegment(closedAt, domain.RegistrantReference{})
	if err != nil {
		t.Fatalf("不带关段人关段：%v", err)
	}
	at, _ = closed.ClosedAt()
	otherAt, _ = closedWithout.ClosedAt()
	if !closed.Closed() || !closedWithout.Closed() || !at.Equal(otherAt) {
		t.Fatalf("关段人改变了关闭：%v %v / %v %v", closed.Closed(), at, closedWithout.Closed(), otherAt)
	}
	if by, recorded := closed.ClosedBy(); !recorded || by != closingOperator {
		t.Fatalf("关段人 = %q (%v)，want %q", by.String(), recorded, closingOperator.String())
	}
	if _, recorded := closedWithout.ClosedBy(); recorded {
		t.Fatal("不带关段人的关段不该凭空记一个人")
	}
	if _, err := closed.CloseSegment(closedAt.Add(time.Hour), terminatingOperator); !errors.Is(err, domain.ErrSegmentClosed) {
		t.Fatalf("另一位操作者再关：err = %v，want ErrSegmentClosed", err)
	}

	corrected, err := pickup.Correct(pickupCorrection(t, "pickup-result/parcel-1/v2", pickedUpAt.Add(-time.Minute)))
	if err != nil {
		t.Fatalf("correct: %v", err)
	}
	rederived, err := attributed.RederiveParticipationWithPickup(corrected)
	if err != nil {
		t.Fatalf("rederive: %v", err)
	}
	tail, _ := rederived.ParticipationFor(object)
	if _, chained := tail.Supersedes(); !chained {
		t.Fatal("更正没有长出替代版本")
	}
	if by, recorded := tail.TerminatedBy(); !recorded || by != terminatingOperator {
		t.Fatalf("替代版本承继的终止人 = %q (%v)，want %q——那仍是同一次终止决定", by.String(), recorded, terminatingOperator.String())
	}

	delivered, err := establish(t).EndParticipationWithDelivery(formedDelivery(t, "parcel-1"))
	if err != nil {
		t.Fatalf("交付结束：%v", err)
	}
	byDelivery, _ := delivered.ParticipationFor(object)
	if _, recorded := byDelivery.TerminatedBy(); recorded {
		t.Fatal("以交付结束的参与不是运营决定，没有终止人")
	}
}

// Covers: `AT-TF-337`、`AT-TF-338` 重建半边——重建门照收存量里没有操作者的段（规则生效之前落下的），带回有操作者的段；
// 关段人出现在没关的段上、终止人出现在仍在场或以交付结束的参与上，是转换门产不出的形状，拒。
func TestTheRehydrationDoorKeepsTheOperatorsWhereTheyBelongAndRefusesThemElsewhere(t *testing.T) {
	closingOperator, terminatingOperator := operatorsForEvidence(t)
	terminated := func(t *testing.T, object string) domain.RehydrateParticipationSpec {
		t.Helper()
		spec := activeParticipationSpec(t, object)
		spec.EndKind = domain.EndedByControlTermination
		spec.EndBasis = segmentValue(t, domain.NewParticipationBasisReference, "CONTROL-TERMINATION/case-"+object)
		spec.EndedAt = segmentEndedAt
		return spec
	}

	legacy := segmentSpec(t, terminated(t, "parcel-1"))
	legacy.Closed, legacy.ClosedAt = true, segmentClosedAt
	rebuilt, err := domain.RehydrateActualFulfillmentSegment(legacy)
	if err != nil {
		t.Fatalf("存量（没有操作者）应照收：%v", err)
	}
	if _, recorded := rebuilt.ClosedBy(); recorded {
		t.Fatal("存量段读回不该有关段人")
	}

	attributedMember := terminated(t, "parcel-1")
	attributedMember.TerminatedBy = terminatingOperator
	attributed := segmentSpec(t, attributedMember)
	attributed.Closed, attributed.ClosedAt, attributed.ClosedBy = true, segmentClosedAt, closingOperator
	rebuilt, err = domain.RehydrateActualFulfillmentSegment(attributed)
	if err != nil {
		t.Fatalf("带操作者的段应读得回：%v", err)
	}
	member, _ := rebuilt.ParticipationFor(attributedMember.Object)
	if by, recorded := rebuilt.ClosedBy(); !recorded || by != closingOperator {
		t.Fatalf("关段人读回 = %q (%v)", by.String(), recorded)
	}
	if by, recorded := member.TerminatedBy(); !recorded || by != terminatingOperator {
		t.Fatalf("终止人读回 = %q (%v)", by.String(), recorded)
	}

	openWithCloser := segmentSpec(t, activeParticipationSpec(t, "parcel-1"))
	openWithCloser.ClosedBy = closingOperator
	activeWithTerminator := activeParticipationSpec(t, "parcel-1")
	activeWithTerminator.TerminatedBy = terminatingOperator
	deliveredWithTerminator := endedParticipationSpec(t, "parcel-1")
	deliveredWithTerminator.TerminatedBy = terminatingOperator
	for name, spec := range map[string]domain.RehydrateActualFulfillmentSegmentSpec{
		"a closer on an open segment":                    openWithCloser,
		"a terminator on an active participation":        segmentSpec(t, activeWithTerminator),
		"a terminator on a participation ended by a POD": segmentSpec(t, deliveredWithTerminator),
	} {
		if _, err := domain.RehydrateActualFulfillmentSegment(spec); !errors.Is(err, domain.ErrInvalidFulfillmentSegment) {
			t.Errorf("%s：err = %v，want ErrInvalidFulfillmentSegment", name, err)
		}
	}
}

// Covers: ADR-0303「可以缺」——空白读作缺席，带 NUL 的存不进登记册，在构造期拒。
func TestAnOptionalOperatorReadsBlankAsAbsentAndRefusesWhatCannotBeStored(t *testing.T) {
	for _, blank := range []string{"", "   "} {
		reference, err := domain.OptionalRegistrantReference(blank)
		if err != nil || reference.Present() {
			t.Fatalf("%q：present=%v err=%v，want 缺席", blank, reference.Present(), err)
		}
	}
	reference, err := domain.OptionalRegistrantReference("issuer#sub")
	if err != nil || !reference.Present() || reference.String() != "issuer#sub" {
		t.Fatalf("在场的操作者：%q present=%v err=%v", reference.String(), reference.Present(), err)
	}
	if _, err := domain.OptionalRegistrantReference("issuer#sub\x00"); !errors.Is(err, domain.ErrValueContainsNUL) {
		t.Fatalf("带 NUL：err = %v，want ErrValueContainsNUL", err)
	}
}
