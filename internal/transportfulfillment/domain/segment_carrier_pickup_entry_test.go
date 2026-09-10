package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件证第三格参与起点（ADR-0135 决定五 / 六）：已形成的收寄立段 / 加入段，入场依据 `CARRIER-FIRST-EFFECTIVE-
// PICKUP/<版本>`、起点取业务发生时间；待确认与失效版本立不起参与；收寄替代版本 → 替代参与版本，失效版本 →
// 失效参与版本；重建门认第三格并许它失效。

func carrierPickupSegmentRef(t *testing.T) domain.FulfillmentSegmentReference {
	t.Helper()
	return mustValue(t, domain.NewFulfillmentSegmentReference, "SEG-CARRIER-X")
}

// Covers: CONTEXT 生命周期①「有效收寄……确认首个载运对象进入共同运输控制范围 → 实际履约段成立」对第三格成立。
func TestAFormedCarrierPickupEstablishesTheSegmentWithTheThirdEntryKind(t *testing.T) {
	pickup := formedCarrierPickup(t)
	segment, err := domain.EstablishSegmentWithCarrierPickup(carrierPickupSegmentRef(t), pickup, domain.PlannedSegmentReference{})
	if err != nil {
		t.Fatalf("立段：%v", err)
	}
	participation, present := segment.ParticipationFor(pickup.Object())
	if !present {
		t.Fatalf("对象没有进段")
	}
	if participation.EntryKind() != domain.EnteredByCarrierFirstEffectivePickup {
		t.Fatalf("入场种类 = %s", participation.EntryKind())
	}
	if participation.EntryBasis().String() != "CARRIER-FIRST-EFFECTIVE-PICKUP/CFEV-1" {
		t.Fatalf("入场依据 = %s", participation.EntryBasis())
	}
	if !participation.EnteredAt().Equal(carrierPickupOccurredAt) {
		t.Fatalf("起点应取业务发生时间：%s", participation.EnteredAt())
	}
	if participation.EntryKind().String() != "CARRIER_FIRST_EFFECTIVE_PICKUP" {
		t.Fatalf("第三格字串 = %q", participation.EntryKind().String())
	}
}

// Covers: CONTEXT 词条「待确认……不构成收寄、不进入段」——待确认与失效版本立不起参与。
func TestPendingAndVoidedPickupVersionsCannotEnterASegment(t *testing.T) {
	pending, err := domain.HoldCarrierFirstEffectivePickupPending(domain.PendingCarrierFirstEffectivePickupSpec{
		TenantID: mustValue(t, domain.NewTenantID, "tenant-1"),
		Object:   mustValue(t, domain.NewCarriedObjectReference, "PCL-1"),
		Fact:     mustValue(t, domain.NewCarrierFirstEffectivePickupReference, "CFEP-1"),
		Version:  mustValue(t, domain.NewCarrierFirstEffectivePickupVersion, "CFEV-1"),
		Reason:   domain.PickupCarrierIdentityNotRegistered,
		JudgedAt: carrierPickupJudgedAt,
		Bases:    []domain.CarrierPickupBasis{carrierPickupBasis(t, "EXTF-1", "EXTV-1")},
	})
	if err != nil {
		t.Fatalf("形成待确认：%v", err)
	}
	if _, err := domain.EstablishSegmentWithCarrierPickup(carrierPickupSegmentRef(t), pending, domain.PlannedSegmentReference{}); !errors.Is(err, domain.ErrSegmentNeedsAControlFact) {
		t.Fatalf("待确认立段应拒 ErrSegmentNeedsAControlFact：%v", err)
	}
	voided, err := formedCarrierPickup(t).Void(domain.CarrierPickupVoiding{
		Version:  mustValue(t, domain.NewCarrierFirstEffectivePickupVersion, "CFEV-2"),
		JudgedAt: carrierPickupJudgedAt.Add(time.Hour),
		Bases:    []domain.CarrierPickupBasis{carrierPickupBasis(t, "EXTF-1", "EXTV-2")},
	})
	if err != nil {
		t.Fatalf("失效：%v", err)
	}
	if _, err := domain.EstablishSegmentWithCarrierPickup(carrierPickupSegmentRef(t), voided, domain.PlannedSegmentReference{}); !errors.Is(err, domain.ErrSegmentNeedsAControlFact) {
		t.Fatalf("失效版本立段应拒 ErrSegmentNeedsAControlFact：%v", err)
	}
}

// Covers: ADR-0135 决定五 / 六——收寄替代版本在同段形成替代参与版本（起点随新业务时间、回指前版入场依据）；
// 失效版本形成失效参与版本（起点沿用、标失效、对象在本段当前无有效参与）；更正的不是当前依据时无可替代。
func TestACarrierPickupCorrectionRederivesTheParticipation(t *testing.T) {
	first := formedCarrierPickup(t)
	segment, err := domain.EstablishSegmentWithCarrierPickup(carrierPickupSegmentRef(t), first, domain.PlannedSegmentReference{})
	if err != nil {
		t.Fatalf("立段：%v", err)
	}
	later := carrierPickupOccurredAt.Add(20 * time.Minute)
	superseding, err := first.Supersede(domain.CarrierPickupSupersession{
		Version:    mustValue(t, domain.NewCarrierFirstEffectivePickupVersion, "CFEV-2"),
		Carrier:    externalCarrierSubject(t, "party-x"),
		OccurredAt: later,
		JudgedAt:   carrierPickupJudgedAt.Add(time.Hour),
		Bases:      []domain.CarrierPickupBasis{carrierPickupBasis(t, "EXTF-1", "EXTV-2")},
	})
	if err != nil {
		t.Fatalf("替代：%v", err)
	}

	t.Run("a superseding pickup version grows a superseding participation", func(t *testing.T) {
		rederived, err := segment.RederiveParticipationWithCarrierPickup(superseding)
		if err != nil {
			t.Fatalf("重派生：%v", err)
		}
		current, present := rederived.ParticipationFor(first.Object())
		if !present || current.Voided() || !current.Active() {
			t.Fatalf("链尾应是在场的替代版本：%+v %v", current, present)
		}
		if current.EntryBasis().String() != "CARRIER-FIRST-EFFECTIVE-PICKUP/CFEV-2" || !current.EnteredAt().Equal(later) {
			t.Fatalf("替代版本依据 / 起点不对：%s %s", current.EntryBasis(), current.EnteredAt())
		}
		if prior, has := current.Supersedes(); !has || prior.String() != "CARRIER-FIRST-EFFECTIVE-PICKUP/CFEV-1" {
			t.Fatalf("替代版本没有回指前版入场依据：%v %v", prior, has)
		}
		if len(rederived.ParticipationHistory(first.Object())) != 2 || rederived.ActiveParticipations() != 1 {
			t.Fatalf("链长 / 在场数不对")
		}
	})

	t.Run("a voided pickup version grows a voided participation", func(t *testing.T) {
		voided, err := first.Void(domain.CarrierPickupVoiding{
			Version:  mustValue(t, domain.NewCarrierFirstEffectivePickupVersion, "CFEV-2"),
			JudgedAt: carrierPickupJudgedAt.Add(time.Hour),
			Bases:    []domain.CarrierPickupBasis{carrierPickupBasis(t, "EXTF-1", "EXTV-2")},
		})
		if err != nil {
			t.Fatalf("失效：%v", err)
		}
		rederived, err := segment.RederiveParticipationWithCarrierPickup(voided)
		if err != nil {
			t.Fatalf("重派生失效：%v", err)
		}
		current, present := rederived.ParticipationFor(first.Object())
		if !present || !current.Voided() || current.Active() {
			t.Fatalf("链尾应是失效版本：%+v", current)
		}
		if !current.EnteredAt().Equal(carrierPickupOccurredAt) {
			t.Fatalf("失效版本起点应沿用前版：%s", current.EnteredAt())
		}
		if rederived.ActiveParticipations() != 0 {
			t.Fatalf("失效后该对象在本段应无有效参与")
		}
	})

	t.Run("a correction of a version that is not the current basis has nothing to rederive", func(t *testing.T) {
		other, err := domain.FormCarrierFirstEffectivePickup(func() domain.CarrierFirstEffectivePickupSpec {
			spec := formedCarrierPickupSpec(t)
			spec.Version = mustValue(t, domain.NewCarrierFirstEffectivePickupVersion, "CFEV-7")
			return spec
		}())
		if err != nil {
			t.Fatalf("形成：%v", err)
		}
		stranger, err := other.Supersede(domain.CarrierPickupSupersession{
			Version:    mustValue(t, domain.NewCarrierFirstEffectivePickupVersion, "CFEV-8"),
			Carrier:    externalCarrierSubject(t, "party-x"),
			OccurredAt: later,
			JudgedAt:   carrierPickupJudgedAt.Add(time.Hour),
			Bases:      []domain.CarrierPickupBasis{carrierPickupBasis(t, "EXTF-1", "EXTV-2")},
		})
		if err != nil {
			t.Fatalf("替代：%v", err)
		}
		if _, err := segment.RederiveParticipationWithCarrierPickup(stranger); !errors.Is(err, domain.ErrNoParticipationToRederive) {
			t.Fatalf("回指的不是当前依据应答无可替代：%v", err)
		}
		if _, err := segment.RederiveParticipationWithCarrierPickup(first); !errors.Is(err, domain.ErrNoParticipationToRederive) {
			t.Fatalf("首登没有前版，应答无可替代：%v", err)
		}
	})
}

// Covers: 重建门认第三格并许它失效（迁移 0020 的 CHECK 同形）；第四格仍拒。
func TestTheRehydrationGateAcceptsTheThirdEntryKindAndItsVoidedVersion(t *testing.T) {
	tenant := mustValue(t, domain.NewTenantID, "tenant-1")
	object := mustValue(t, domain.NewCarriedObjectReference, "PCL-1")
	rows := []domain.RehydrateParticipationSpec{
		{
			Object:     object,
			EntryKind:  domain.EnteredByCarrierFirstEffectivePickup,
			EntryBasis: mustValue(t, domain.NewParticipationBasisReference, "CARRIER-FIRST-EFFECTIVE-PICKUP/CFEV-1"),
			EnteredAt:  carrierPickupOccurredAt,
		},
		{
			Object:     object,
			EntryKind:  domain.EnteredByCarrierFirstEffectivePickup,
			EntryBasis: mustValue(t, domain.NewParticipationBasisReference, "CARRIER-FIRST-EFFECTIVE-PICKUP/CFEV-2"),
			EnteredAt:  carrierPickupOccurredAt,
			Supersedes: mustValue(t, domain.NewParticipationBasisReference, "CARRIER-FIRST-EFFECTIVE-PICKUP/CFEV-1"),
			Voided:     true,
		},
	}
	segment, err := domain.RehydrateActualFulfillmentSegment(domain.RehydrateActualFulfillmentSegmentSpec{
		TenantID:       tenant,
		Segment:        carrierPickupSegmentRef(t),
		Participations: rows,
	})
	if err != nil {
		t.Fatalf("重建：%v", err)
	}
	if segment.ActiveParticipations() != 0 {
		t.Fatalf("失效链尾重建后不应在场")
	}
	rows[1].EntryKind = domain.ParticipationEntryKind(4)
	rows[0].EntryKind = domain.ParticipationEntryKind(4)
	if _, err := domain.RehydrateActualFulfillmentSegment(domain.RehydrateActualFulfillmentSegmentSpec{
		TenantID:       tenant,
		Segment:        carrierPickupSegmentRef(t),
		Participations: rows,
	}); !errors.Is(err, domain.ErrInvalidFulfillmentSegment) {
		t.Fatalf("第四格应拒：%v", err)
	}
}
