package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件证实际履约发生项自动形成时领域给出的两件判断（ADR-0218）：这件控制事实结束的是哪一条参与，发生项属于哪一趟旅程。
// 夹具全是合成串，证据只记 S。

var (
	formationEnteredAt = time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	formationEndedAt   = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
)

func formationJourney(t *testing.T, journey string, startedAt time.Time, members ...string) domain.AlternateJourney {
	t.Helper()
	objects := make([]domain.CarriedObjectReference, 0, len(members))
	for _, member := range members {
		objects = append(objects, mustValue(t, domain.NewCarriedObjectReference, member))
	}
	formed, err := domain.FormAlternateJourney(domain.AlternateJourneySpec{
		TenantID:        mustValue(t, domain.NewTenantID, "SYN-TENANT-01"),
		Journey:         mustValue(t, domain.NewJourneyReference, journey),
		Purpose:         domain.ReturnJourneyPurpose,
		OriginalJourney: mustValue(t, domain.NewJourneyReference, "JRN/SYN-PARCEL-67"),
		BasisKind:       domain.ServiceDispositionDecision,
		Basis:           mustValue(t, domain.NewDispositionBasisReference, "SYN-DISPOSITION-"+journey),
		Members:         objects,
		StartedAt:       startedAt,
	})
	if err != nil {
		t.Fatalf("旅程夹具：%v", err)
	}
	return formed
}

// Covers: AT-TF-252——没有替代 / 退运旅程时按正式包裹身份派生 `JRN/<对象>`；有一趟在参与入场时已开始就取它，几趟里取开始
// 最晚、且不晚于入场的那一趟；入场之后才开始的、不含这个对象的都不算；同一刻开始两趟不挑。
func TestTheActualFulfillmentJourneyIsTheAlternateStartedByEntryOrTheDerivedOne(t *testing.T) {
	parcel := mustValue(t, domain.NewCarriedObjectReference, "SYN-PARCEL-67")

	derived, err := domain.ActualFulfillmentJourney(parcel, formationEnteredAt, nil)
	if err != nil || derived.String() != "JRN/SYN-PARCEL-67" {
		t.Fatalf("没有新旅程 = %q %v, want JRN/SYN-PARCEL-67", derived, err)
	}

	earlier := formationJourney(t, "SYN-JRN-RETURN-1", formationEnteredAt.Add(-2*time.Hour), "SYN-PARCEL-67")
	later := formationJourney(t, "SYN-JRN-RETURN-2", formationEnteredAt.Add(-time.Hour), "SYN-PARCEL-67", "SYN-PARCEL-OTHER")
	afterEntry := formationJourney(t, "SYN-JRN-RETURN-3", formationEnteredAt.Add(time.Minute), "SYN-PARCEL-67")
	otherObject := formationJourney(t, "SYN-JRN-RETURN-4", formationEnteredAt.Add(-time.Minute), "SYN-PARCEL-OTHER")
	atEntry := formationJourney(t, "SYN-JRN-RETURN-5", formationEnteredAt, "SYN-PARCEL-67")

	cases := []struct {
		name       string
		alternates []domain.AlternateJourney
		want       string
	}{
		{"入场之前开始的那一趟", []domain.AlternateJourney{earlier}, "SYN-JRN-RETURN-1"},
		{"几趟里开始最晚的", []domain.AlternateJourney{earlier, later}, "SYN-JRN-RETURN-2"},
		{"入场之后才开始的不算", []domain.AlternateJourney{afterEntry}, "JRN/SYN-PARCEL-67"},
		{"不含这个对象的不算", []domain.AlternateJourney{otherObject}, "JRN/SYN-PARCEL-67"},
		{"恰在入场那一刻开始的算", []domain.AlternateJourney{earlier, atEntry, afterEntry}, "SYN-JRN-RETURN-5"},
	}
	for _, tc := range cases {
		journey, err := domain.ActualFulfillmentJourney(parcel, formationEnteredAt, tc.alternates)
		if err != nil || journey.String() != tc.want {
			t.Errorf("%s = %q %v, want %s", tc.name, journey, err, tc.want)
		}
	}

	tied := formationJourney(t, "SYN-JRN-RETURN-6", later.StartedAt(), "SYN-PARCEL-67")
	if _, err := domain.ActualFulfillmentJourney(parcel, formationEnteredAt, []domain.AlternateJourney{later, tied}); !errors.Is(err, domain.ErrAmbiguousJourney) {
		t.Fatalf("同一刻开始两趟 = %v, want ErrAmbiguousJourney", err)
	}
	if _, err := domain.ActualFulfillmentJourney(domain.CarriedObjectReference{}, formationEnteredAt, nil); err == nil {
		t.Fatal("空对象不该派生出旅程")
	}
}

func formationSegment(t *testing.T, rows ...domain.RehydrateParticipationSpec) domain.ActualFulfillmentSegment {
	t.Helper()
	segment, err := domain.RehydrateActualFulfillmentSegment(domain.RehydrateActualFulfillmentSegmentSpec{
		TenantID:       mustValue(t, domain.NewTenantID, "SYN-TENANT-01"),
		Segment:        mustValue(t, domain.NewFulfillmentSegmentReference, "SYN-SEG-SZX-SIN-67"),
		Participations: rows,
	})
	if err != nil {
		t.Fatalf("段夹具：%v", err)
	}
	return segment
}

func handoverParticipation(t *testing.T, object, entry, end string) domain.RehydrateParticipationSpec {
	t.Helper()
	row := domain.RehydrateParticipationSpec{
		Object:     mustValue(t, domain.NewCarriedObjectReference, object),
		EntryKind:  domain.EnteredByTransportHandover,
		EntryBasis: mustValue(t, domain.NewParticipationBasisReference, entry),
		EnteredAt:  formationEnteredAt,
	}
	if end != "" {
		row.EndKind = domain.EndedByNextHandover
		row.EndBasis = mustValue(t, domain.NewParticipationBasisReference, end)
		row.EndedAt = formationEndedAt
	}
	return row
}

// Covers: AT-TF-246、AT-TF-251 的领域半边——按终点的种类与依据认这件事实结束的那一条参与：对得上才算；仍在场、被别的事实
// 结束、种类不同、失效版本都不算；更正入场之后链尾继承前版终点，照样认得出。
func TestTheParticipationEndedByAControlFactIsFoundByItsEndBasis(t *testing.T) {
	parcel := mustValue(t, domain.NewCarriedObjectReference, "SYN-PARCEL-67")
	byHandover := mustValue(t, domain.NewParticipationBasisReference, "TRANSPORT-HANDOVER/SYN-HO-SIN-67")

	ended := formationSegment(t, handoverParticipation(t, "SYN-PARCEL-67", "TRANSPORT-HANDOVER/SYN-HO-SZX-67", byHandover.String()))
	participation, found := ended.ParticipationEndedBy(parcel, domain.EndedByNextHandover, byHandover)
	if !found || !participation.EnteredAt().Equal(formationEnteredAt) {
		t.Fatalf("恰由这次交接结束的参与没认出：found=%v", found)
	}
	if _, found := ended.ParticipationEndedBy(parcel, domain.EndedByEffectiveDelivery, byHandover); found {
		t.Fatal("终点种类不同也认了")
	}
	other := mustValue(t, domain.NewParticipationBasisReference, "TRANSPORT-HANDOVER/SYN-HO-OTHER")
	if _, found := ended.ParticipationEndedBy(parcel, domain.EndedByNextHandover, other); found {
		t.Fatal("被别的交接结束的也认了")
	}
	active := formationSegment(t, handoverParticipation(t, "SYN-PARCEL-67", "TRANSPORT-HANDOVER/SYN-HO-SZX-67", ""))
	if _, found := active.ParticipationEndedBy(parcel, domain.EndedByNextHandover, byHandover); found {
		t.Fatal("仍在场的参与也认了")
	}

	superseding := handoverParticipation(t, "SYN-PARCEL-67", "TRANSPORT-HANDOVER/SYN-HO-SZX-67-V2", byHandover.String())
	superseding.Supersedes = mustValue(t, domain.NewParticipationBasisReference, "TRANSPORT-HANDOVER/SYN-HO-SZX-67")
	corrected := formationSegment(t,
		handoverParticipation(t, "SYN-PARCEL-67", "TRANSPORT-HANDOVER/SYN-HO-SZX-67", byHandover.String()), superseding)
	if current, found := corrected.ParticipationEndedBy(parcel, domain.EndedByNextHandover, byHandover); !found ||
		current.EntryBasis().String() != "TRANSPORT-HANDOVER/SYN-HO-SZX-67-V2" {
		t.Fatalf("更正入场之后应认链尾：found=%v", found)
	}

	voiding := superseding
	voiding.EntryBasis = mustValue(t, domain.NewParticipationBasisReference, "TRANSPORT-HANDOVER/SYN-HO-SZX-67-VOID")
	voiding.Voided = true
	voided := formationSegment(t,
		handoverParticipation(t, "SYN-PARCEL-67", "TRANSPORT-HANDOVER/SYN-HO-SZX-67", byHandover.String()), voiding)
	if _, found := voided.ParticipationEndedBy(parcel, domain.EndedByNextHandover, byHandover); found {
		t.Fatal("失效版本也认了：入场被撤回的参与没有可计费的履约范围")
	}
}

// Covers: AT-TF-247 的领域半边——有效交付结束参与时记下的终点依据，就是事后按那一版交付结果去找的那一串：两处同一个出处。
func TestTheDeliveryEndBasisIsTheOneTheDeliveryEndsTheParticipationWith(t *testing.T) {
	version := mustValue(t, domain.NewDeliveryResultVersion, "SYN-DRV-67")
	basis, err := domain.EffectiveDeliveryEndBasis(version)
	if err != nil || basis.String() != "EFFECTIVE-DELIVERY/SYN-DRV-67" {
		t.Fatalf("终点依据 = %q %v", basis, err)
	}
	if _, err := domain.EffectiveDeliveryEndBasis(domain.DeliveryResultVersion{}); err == nil {
		t.Fatal("空版本不该给出依据")
	}
}
