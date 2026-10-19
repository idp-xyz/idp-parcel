package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// adoptionSegment 从库面重建一个段：每条参与给（对象、入场依据、入场时刻），supersedes 非空即替代版本，voided 即失效版本。
type adoptionParticipation struct {
	object, basis, supersedes string
	enteredAt                 time.Time
	voided                    bool
}

func adoptionSegment(t *testing.T, tenant string, participations ...adoptionParticipation) domain.ActualFulfillmentSegment {
	t.Helper()
	spec := domain.RehydrateActualFulfillmentSegmentSpec{
		TenantID: mustValue(t, domain.NewTenantID, tenant),
		Segment:  mustValue(t, domain.NewFulfillmentSegmentReference, "SEG-TRUNK-1"),
	}
	for _, each := range participations {
		row := domain.RehydrateParticipationSpec{
			Object:     mustValue(t, domain.NewCarriedObjectReference, each.object),
			EntryKind:  domain.EnteredByTransportHandover,
			EntryBasis: mustValue(t, domain.NewParticipationBasisReference, each.basis),
			EnteredAt:  each.enteredAt,
			Voided:     each.voided,
		}
		if each.supersedes != "" {
			row.Supersedes = mustValue(t, domain.NewParticipationBasisReference, each.supersedes)
		}
		spec.Participations = append(spec.Participations, row)
	}
	segment, err := domain.RehydrateActualFulfillmentSegment(spec)
	if err != nil {
		t.Fatalf("重建段：%v", err)
	}
	return segment
}

func submittedCommission(t *testing.T) domain.TransportCommission {
	t.Helper()
	commission, err := submitCommission(t, commissionSpec(t))
	if err != nil {
		t.Fatalf("提交委托：%v", err)
	}
	return commission
}

// Covers: ADR-0204 决定一、二——采用声明（段、委托、服务方）：段的首个当前参与作开始依据，委托未开始时随之标为已开始，开始
// 的依据与时刻取首个参与而不取登记时刻；委托的开始是已采用各段首个参与里最早的那一条：后到的段开始得更晚，委托原样交回，
// 开始得更早，开始前移到它；连线自带这一段的开始依据。
func TestAdoptingACommissionStartsItFromTheSegmentsFirstParticipation(t *testing.T) {
	commission := submittedCommission(t)
	provider := mustValue(t, domain.NewServiceProviderReference, "partner-1")
	firstAt := commissionSubmittedAt.Add(2 * time.Hour)
	segment := adoptionSegment(t, "tenant-1",
		adoptionParticipation{object: "parcel-2", basis: "TRANSPORT-HANDOVER/h-2/v1", enteredAt: firstAt.Add(time.Hour)},
		adoptionParticipation{object: "parcel-1", basis: "TRANSPORT-HANDOVER/h-1/v1", enteredAt: firstAt},
	)

	adoption, adopted, change, err := domain.AdoptCommissionForSegment(segment, commission, provider)
	if err != nil {
		t.Fatalf("采用：%v", err)
	}
	if change != domain.CommissionStartMarked {
		t.Fatalf("未开始的委托采用之后 change = %s，want MARKED", change)
	}
	basis, at, started := adopted.TransportStarted()
	if !started || basis.String() != "TRANSPORT-HANDOVER/h-1/v1" || !at.Equal(firstAt) {
		t.Fatalf("委托的开始 = (%s, %s, %v)，want 段的首个参与", basis, at, started)
	}
	if _, _, started := commission.TransportStarted(); started {
		t.Fatal("值语义：原委托被改了")
	}
	if adoption.Segment().String() != "SEG-TRUNK-1" || adoption.Commission().String() != "commission-1" || adoption.Provider() != provider {
		t.Fatalf("连线 = %+v", adoption)
	}
	if startBasis, startAt := adoption.Started(); startBasis.String() != "TRANSPORT-HANDOVER/h-1/v1" || !startAt.Equal(firstAt) {
		t.Fatalf("连线上的开始 = (%s, %s)", startBasis, startAt)
	}

	t.Run("a segment that started later keeps the commission's start", func(t *testing.T) {
		later := adoptionSegment(t, "tenant-1",
			adoptionParticipation{object: "parcel-3", basis: "TRANSPORT-HANDOVER/h-3/v1", enteredAt: firstAt.Add(time.Hour)},
		)
		second, kept, change, err := domain.AdoptCommissionForSegment(later, adopted, provider)
		if err != nil {
			t.Fatalf("已开始的委托被另一段采用：%v", err)
		}
		if change != domain.CommissionStartKept {
			t.Fatalf("change = %s，want KEPT", change)
		}
		if basis, at, _ := kept.TransportStarted(); basis.String() != "TRANSPORT-HANDOVER/h-1/v1" || !at.Equal(firstAt) {
			t.Fatalf("委托的开始被开始得更晚的段改写成 (%s, %s)", basis, at)
		}
		if startBasis, _ := second.Started(); startBasis.String() != "TRANSPORT-HANDOVER/h-3/v1" {
			t.Fatalf("后到那一段的连线没带自己的开始依据：%s", startBasis)
		}
	})

	t.Run("a segment adopted later but started earlier advances the commission's start", func(t *testing.T) {
		earlier := adoptionSegment(t, "tenant-1",
			adoptionParticipation{object: "parcel-3", basis: "TRANSPORT-HANDOVER/h-3/v1", enteredAt: firstAt.Add(-time.Hour)},
		)
		second, advanced, change, err := domain.AdoptCommissionForSegment(earlier, adopted, provider)
		if err != nil {
			t.Fatalf("已开始的委托被开始得更早的段采用：%v", err)
		}
		if change != domain.CommissionStartAdvanced {
			t.Fatalf("change = %s，want ADVANCED", change)
		}
		if basis, at, _ := advanced.TransportStarted(); basis.String() != "TRANSPORT-HANDOVER/h-3/v1" || !at.Equal(firstAt.Add(-time.Hour)) {
			t.Fatalf("委托的开始没有前移到更早的那一段：(%s, %s)", basis, at)
		}
		if basis, at, _ := adopted.TransportStarted(); basis.String() != "TRANSPORT-HANDOVER/h-1/v1" || !at.Equal(firstAt) {
			t.Fatal("值语义：前移改了原委托")
		}
		if startBasis, _ := second.Started(); startBasis.String() != "TRANSPORT-HANDOVER/h-3/v1" {
			t.Fatalf("连线没带这一段自己的开始依据：%s", startBasis)
		}
		if _, err := advanced.Cancel(firstAt.Add(-30 * time.Minute)); !errors.Is(err, domain.ErrTransportAlreadyStarted) {
			t.Fatalf("前移之后取消：err = %v，want ErrTransportAlreadyStarted", err)
		}
	})
}

// Covers: ADR-0204 决定二「依据取该段首个参与」的读法——被替代的版本与失效版本不算当前参与，已离场的照算；同一时刻按入场
// 依据字面序取，读几次都同一条；每条参与都已失效的段没有开始依据可取。
func TestTheFirstEffectiveParticipationSkipsSupersededAndVoidedVersions(t *testing.T) {
	base := commissionSubmittedAt.Add(time.Hour)
	segment := adoptionSegment(t, "tenant-1",
		adoptionParticipation{object: "parcel-1", basis: "TRANSPORT-HANDOVER/h-1/v1", enteredAt: base},
		adoptionParticipation{object: "parcel-1", basis: "TRANSPORT-HANDOVER/h-1/v2", supersedes: "TRANSPORT-HANDOVER/h-1/v1", enteredAt: base.Add(2 * time.Hour)},
		adoptionParticipation{object: "parcel-2", basis: "TRANSPORT-HANDOVER/h-2/v1", enteredAt: base.Add(30 * time.Minute)},
		adoptionParticipation{object: "parcel-2", basis: "TRANSPORT-HANDOVER/h-2/v2", supersedes: "TRANSPORT-HANDOVER/h-2/v1", enteredAt: base.Add(30 * time.Minute), voided: true},
		adoptionParticipation{object: "parcel-4", basis: "TRANSPORT-HANDOVER/h-4/v1", enteredAt: base.Add(time.Hour)},
		adoptionParticipation{object: "parcel-3", basis: "TRANSPORT-HANDOVER/h-3/v1", enteredAt: base.Add(time.Hour)},
	)
	first, present := segment.FirstEffectiveParticipation()
	if !present || first.EntryBasis().String() != "TRANSPORT-HANDOVER/h-3/v1" {
		t.Fatalf("首个当前参与 = %s %v，want 被替代与失效之外最早的那条、同刻取字面序第一条", first.EntryBasis(), present)
	}

	allVoided := adoptionSegment(t, "tenant-1",
		adoptionParticipation{object: "parcel-1", basis: "TRANSPORT-HANDOVER/h-1/v1", enteredAt: base},
		adoptionParticipation{object: "parcel-1", basis: "TRANSPORT-HANDOVER/h-1/v2", supersedes: "TRANSPORT-HANDOVER/h-1/v1", enteredAt: base, voided: true},
	)
	if _, present := allVoided.FirstEffectiveParticipation(); present {
		t.Fatal("每条参与都已失效的段交出了一条开始依据")
	}
	_, _, _, err := domain.AdoptCommissionForSegment(allVoided, submittedCommission(t), mustValue(t, domain.NewServiceProviderReference, "partner-1"))
	if !errors.Is(err, domain.ErrSegmentWithoutEffectiveParticipation) {
		t.Fatalf("没有当前参与的段：err = %v", err)
	}
}

// Covers: ADR-0204 决定一、三的前置条件——服务方不符、委托在段开始之前已取消、委托在段开始之后才取消、段早于委托提交就已
// 开始各自成格，不并成一格不成形；服务方排在取消之前；取消按业务时间比段的首个参与，同一刻算开始在先；段没有当前参与或
// 早于委托提交时没有可比的开始，已取消的委托答不可采用；租户不同的段与委托连不上。
func TestAnAdoptionRefusesEachUnmetPrecondition(t *testing.T) {
	provider := mustValue(t, domain.NewServiceProviderReference, "partner-1")
	segment := adoptionSegment(t, "tenant-1",
		adoptionParticipation{object: "parcel-1", basis: "TRANSPORT-HANDOVER/h-1/v1", enteredAt: commissionSubmittedAt.Add(time.Hour)},
	)
	cancelled, err := submittedCommission(t).Cancel(commissionSubmittedAt.Add(30 * time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	early := adoptionSegment(t, "tenant-1",
		adoptionParticipation{object: "parcel-1", basis: "TRANSPORT-HANDOVER/h-1/v1", enteredAt: commissionSubmittedAt.Add(-time.Minute)},
	)
	cancelledLater, err := submittedCommission(t).Cancel(commissionSubmittedAt.Add(2 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	cancelledAtTheStart, err := submittedCommission(t).Cancel(commissionSubmittedAt.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	voided := adoptionSegment(t, "tenant-1",
		adoptionParticipation{object: "parcel-1", basis: "TRANSPORT-HANDOVER/h-1/v1", enteredAt: commissionSubmittedAt.Add(time.Hour)},
		adoptionParticipation{object: "parcel-1", basis: "TRANSPORT-HANDOVER/h-1/v2", supersedes: "TRANSPORT-HANDOVER/h-1/v1", enteredAt: commissionSubmittedAt.Add(time.Hour), voided: true},
	)
	for name, tc := range map[string]struct {
		segment    domain.ActualFulfillmentSegment
		commission domain.TransportCommission
		provider   domain.ServiceProviderReference
		want       error
	}{
		"another provider":            {segment, submittedCommission(t), mustValue(t, domain.NewServiceProviderReference, "partner-9"), domain.ErrAdoptionProviderMismatch},
		"another provider, cancelled": {segment, cancelled, mustValue(t, domain.NewServiceProviderReference, "partner-9"), domain.ErrAdoptionProviderMismatch},
		"cancelled":                   {segment, cancelled, provider, domain.ErrCommissionNotAdoptable},
		"cancelled after the start":   {segment, cancelledLater, provider, domain.ErrCommissionCancelledAfterStart},
		"cancelled at the start":      {segment, cancelledAtTheStart, provider, domain.ErrCommissionCancelledAfterStart},
		"cancelled, no participation": {voided, cancelledLater, provider, domain.ErrCommissionNotAdoptable},
		"cancelled, started early":    {early, cancelledLater, provider, domain.ErrCommissionNotAdoptable},
		"started before submission":   {early, submittedCommission(t), provider, domain.ErrSegmentStartedBeforeCommission},
		"blank provider":              {segment, submittedCommission(t), domain.ServiceProviderReference{}, domain.ErrInvalidCommissionAdoption},
		"another tenant":              {adoptionSegment(t, "tenant-2", adoptionParticipation{object: "parcel-1", basis: "TRANSPORT-HANDOVER/h-1/v1", enteredAt: commissionSubmittedAt.Add(time.Hour)}), submittedCommission(t), provider, domain.ErrInvalidCommissionAdoption},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, _, err := domain.AdoptCommissionForSegment(tc.segment, tc.commission, tc.provider); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

// Covers: ADR-0204 决定三与 `AT-TF-092`——采用标了开始之后，委托取消答运输已开始；采用摘要只收声明的那一对，换委托或换服务方
// 都改串，开始依据不进串。
func TestAnAdoptedCommissionCannotBeCancelledAndTheDigestCoversTheDeclaredPair(t *testing.T) {
	segment := adoptionSegment(t, "tenant-1",
		adoptionParticipation{object: "parcel-1", basis: "TRANSPORT-HANDOVER/h-1/v1", enteredAt: commissionSubmittedAt.Add(time.Hour)},
	)
	_, adopted, _, err := domain.AdoptCommissionForSegment(segment, submittedCommission(t), mustValue(t, domain.NewServiceProviderReference, "partner-1"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adopted.Cancel(commissionSubmittedAt.Add(3 * time.Hour)); !errors.Is(err, domain.ErrTransportAlreadyStarted) {
		t.Fatalf("取消已采用的委托：err = %v，want ErrTransportAlreadyStarted", err)
	}

	digest := func(commission, provider string) string {
		t.Helper()
		value, err := domain.CanonicalizeSegmentCommissionAdoptionPayload(commission, provider)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	same := digest("commission-1", "partner-1")
	if same != digest("commission-1", "partner-1") || same == digest("commission-2", "partner-1") || same == digest("commission-1", "partner-2") {
		t.Fatal("采用摘要没有恰好盖住声明的那一对")
	}
	if domain.CompareStoredDigest(same, digest("commission-1", "partner-1"), "") != domain.SamePayload {
		t.Fatal("采用摘要不是 TFC-1 形状")
	}
}

// Covers: 采用连线的重建门只验各格成形（ADR-0028），缺一格即拒。
func TestARehydratedAdoptionNeedsEveryColumn(t *testing.T) {
	spec := domain.RehydrateSegmentCommissionAdoptionSpec{
		TenantID:   mustValue(t, domain.NewTenantID, "tenant-1"),
		Segment:    mustValue(t, domain.NewFulfillmentSegmentReference, "SEG-TRUNK-1"),
		Commission: mustValue(t, domain.NewTransportCommissionReference, "commission-1"),
		Provider:   mustValue(t, domain.NewServiceProviderReference, "partner-1"),
		StartBasis: mustValue(t, domain.NewParticipationBasisReference, "TRANSPORT-HANDOVER/h-1/v1"),
		StartedAt:  commissionSubmittedAt,
	}
	if _, err := domain.RehydrateSegmentCommissionAdoption(spec); err != nil {
		t.Fatalf("成形的行被拒：%v", err)
	}
	broken := spec
	broken.StartBasis = domain.ParticipationBasisReference{}
	if _, err := domain.RehydrateSegmentCommissionAdoption(broken); !errors.Is(err, domain.ErrInvalidCommissionAdoption) {
		t.Fatalf("缺开始依据：err = %v", err)
	}
	broken = spec
	broken.StartedAt = time.Time{}
	if _, err := domain.RehydrateSegmentCommissionAdoption(broken); !errors.Is(err, domain.ErrInvalidCommissionAdoption) {
		t.Fatalf("缺开始时刻：err = %v", err)
	}
}
