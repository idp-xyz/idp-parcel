package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

func formedPickup(t *testing.T, object, attempt string) domain.OffsitePickup {
	t.Helper()
	spec := pickupSpec(t)
	spec.Object = mustValue(t, domain.NewCarriedObjectReference, object)
	spec.Attempt = mustValue(t, domain.NewAttemptReference, attempt)
	spec.Version = mustValue(t, domain.NewPickupResultVersion, "pickup-result/"+object+"/v1")
	pickup, err := domain.FormOffsitePickup(spec)
	if err != nil {
		t.Fatalf("form offsite pickup: %v", err)
	}
	return pickup
}

func formedHandover(t *testing.T, object string, verdict domain.HandoverVerdict) domain.TransportHandover {
	t.Helper()
	handover, err := domain.FormTransportHandover(handoverSpec(t, object, verdict))
	if err != nil {
		t.Fatalf("form transport handover: %v", err)
	}
	return handover
}

func formedDelivery(t *testing.T, object string) domain.EffectiveDelivery {
	t.Helper()
	spec := attemptSpec(t, "attempt-delivery-"+object)
	spec.Objects = []domain.CarriedObjectReference{mustValue(t, domain.NewCarriedObjectReference, object)}
	attempt, err := domain.FormFulfillmentAttempt(spec)
	if err != nil {
		t.Fatalf("form delivery attempt: %v", err)
	}
	delivery, err := domain.FormEffectiveDelivery(attempt, deliveredResult(t, attempt, object), deliverySpec(t, "delivery-result/"+object+"/v1"))
	if err != nil {
		t.Fatalf("form effective delivery: %v", err)
	}
	return delivery
}

// Covers: CONTEXT 生命周期①「有效收寄或权威交接确认首个载运对象进入共同运输控制范围
// → 实际履约段成立…没有该事实时只存在计划、委托、订舱、分配或尝试」——两种控制事实
// 各可成段；拒收与待确认的交接立不起段（它们不转出控制）。
func TestASegmentIsEstablishedOnlyByAControlFact(t *testing.T) {
	reference := mustValue(t, domain.NewFulfillmentSegmentReference, "segment-1")

	t.Run("an offsite pickup establishes the segment", func(t *testing.T) {
		segment, err := domain.EstablishSegmentWithPickup(reference, formedPickup(t, "parcel-1", "attempt-1"), domain.PlannedSegmentReference{})
		if err != nil {
			t.Fatalf("establish: %v", err)
		}
		if !segment.Established() || segment.ActiveParticipations() != 1 {
			t.Fatalf("participations = %d, want 1", segment.ActiveParticipations())
		}
		participation, found := segment.ParticipationFor(mustValue(t, domain.NewCarriedObjectReference, "parcel-1"))
		if !found {
			t.Fatal("首个对象没有形成参与关系")
		}
		if participation.EntryKind() != domain.EnteredByOffsitePickup {
			t.Fatalf("entry kind = %q, want OFFSITE_PICKUP", participation.EntryKind())
		}
		if !participation.EnteredAt().Equal(pickedUpAt) {
			t.Fatalf("entered at = %s（起点须锚在实际接货时间）", participation.EnteredAt())
		}
		if participation.EntryBasis().String() != "OFFSITE-PICKUP/pickup-result/parcel-1/v1" {
			t.Fatalf("entry basis = %q", participation.EntryBasis())
		}
	})

	t.Run("a handed-over handover establishes the segment", func(t *testing.T) {
		segment, err := domain.EstablishSegmentWithHandover(reference, formedHandover(t, "parcel-1", domain.ObjectHandedOver), domain.PlannedSegmentReference{})
		if err != nil {
			t.Fatalf("establish: %v", err)
		}
		participation, _ := segment.ParticipationFor(mustValue(t, domain.NewCarriedObjectReference, "parcel-1"))
		if participation.EntryKind() != domain.EnteredByTransportHandover {
			t.Fatalf("entry kind = %q, want TRANSPORT_HANDOVER", participation.EntryKind())
		}
		if !participation.EnteredAt().Equal(handoverJudgedAt) {
			t.Fatalf("entered at = %s", participation.EnteredAt())
		}
	})

	for name, verdict := range map[string]domain.HandoverVerdict{
		"a refused handover":      domain.HandoverRefused,
		"an unconfirmed handover": domain.HandoverPendingConfirmation,
	} {
		t.Run(name+" cannot establish a segment", func(t *testing.T) {
			if _, err := domain.EstablishSegmentWithHandover(reference, formedHandover(t, "parcel-1", verdict), domain.PlannedSegmentReference{}); !errors.Is(err, domain.ErrSegmentNeedsAControlFact) {
				t.Fatalf("error = %v, want ErrSegmentNeedsAControlFact", err)
			}
		})
	}
}

// Covers: CONTEXT 生命周期②「后续兼容载运对象取得同一运输控制 → 分别加入实际履约段
// 并形成自己的参与起点，不修改其他对象的起点」——重复加入被拒（同一控制范围不重复
// 建立参与），关段后不再接新对象。
func TestLaterObjectsJoinWithTheirOwnEntry(t *testing.T) {
	reference := mustValue(t, domain.NewFulfillmentSegmentReference, "segment-1")
	segment, err := domain.EstablishSegmentWithPickup(reference, formedPickup(t, "parcel-1", "attempt-1"), domain.PlannedSegmentReference{})
	if err != nil {
		t.Fatalf("establish: %v", err)
	}
	firstEnteredAt := pickedUpAt

	joined, err := segment.JoinWithHandover(formedHandover(t, "parcel-2", domain.ObjectHandedOver), domain.PlannedSegmentReference{})
	if err != nil {
		t.Fatalf("join: %v", err)
	}
	// 计数锚定本夹具：两对象（parcel-1 揽收于 2026-08-09T08:15Z，parcel-2 交接于
	// 2026-08-10T14:00Z）。
	if joined.ActiveParticipations() != 2 {
		t.Fatalf("active = %d, want 2", joined.ActiveParticipations())
	}
	first, _ := joined.ParticipationFor(mustValue(t, domain.NewCarriedObjectReference, "parcel-1"))
	if !first.EnteredAt().Equal(firstEnteredAt) {
		t.Fatal("后来者的加入改写了首个对象的起点")
	}
	second, _ := joined.ParticipationFor(mustValue(t, domain.NewCarriedObjectReference, "parcel-2"))
	if !second.EnteredAt().Equal(handoverJudgedAt) {
		t.Fatal("后来者没有形成自己的起点")
	}

	t.Run("a duplicate object cannot join twice", func(t *testing.T) {
		if _, err := joined.JoinWithPickup(formedPickup(t, "parcel-1", "attempt-9"), domain.PlannedSegmentReference{}); !errors.Is(err, domain.ErrObjectAlreadyParticipating) {
			t.Fatalf("error = %v, want ErrObjectAlreadyParticipating", err)
		}
	})

	t.Run("another tenant's fact cannot join", func(t *testing.T) {
		spec := handoverSpec(t, "parcel-3", domain.ObjectHandedOver)
		spec.TenantID = mustValue(t, domain.NewTenantID, "tenant-2")
		foreign, err := domain.FormTransportHandover(spec)
		if err != nil {
			t.Fatalf("form foreign handover: %v", err)
		}
		if _, err := joined.JoinWithHandover(foreign, domain.PlannedSegmentReference{}); !errors.Is(err, domain.ErrInvalidFulfillmentSegment) {
			t.Fatalf("error = %v; 跨租户事实加入了段", err)
		}
	})
}

// Covers: CONTEXT 生命周期③「对象形成有效交付、下一次权威交接或明确控制终止 → 该对象
// 参与结束」与④「全部有效参与关系已经结束且不再接受新对象 → 段结束；各对象可以具有
// 不同结果」——逐对象结束、结果各异；结束后不重复结束、关段后不再接新。
func TestParticipationEndsPerObjectWithItsFact(t *testing.T) {
	reference := mustValue(t, domain.NewFulfillmentSegmentReference, "segment-1")
	segment, err := domain.EstablishSegmentWithPickup(reference, formedPickup(t, "parcel-1", "attempt-1"), domain.PlannedSegmentReference{})
	if err != nil {
		t.Fatalf("establish: %v", err)
	}
	segment, err = segment.JoinWithPickup(formedPickup(t, "parcel-2", "attempt-1"), domain.PlannedSegmentReference{})
	if err != nil {
		t.Fatalf("join: %v", err)
	}

	afterDelivery, err := segment.EndParticipationWithDelivery(formedDelivery(t, "parcel-1"))
	if err != nil {
		t.Fatalf("end with delivery: %v", err)
	}
	if afterDelivery.ActiveParticipations() != 1 {
		t.Fatalf("active = %d, want 1（只结束交付对象的参与）", afterDelivery.ActiveParticipations())
	}
	deliveredParticipation, _ := afterDelivery.ParticipationFor(mustValue(t, domain.NewCarriedObjectReference, "parcel-1"))
	kind, basis, _, ended := deliveredParticipation.End()
	if !ended || kind != domain.EndedByEffectiveDelivery {
		t.Fatalf("end kind = %q ended=%v, want EFFECTIVE_DELIVERY", kind, ended)
	}
	if basis.String() != "EFFECTIVE-DELIVERY/delivery-result/parcel-1/v1" {
		t.Fatalf("end basis = %q", basis)
	}

	afterHandover, err := afterDelivery.EndParticipationWithNextHandover(formedHandover(t, "parcel-2", domain.ObjectHandedOver))
	if err != nil {
		t.Fatalf("end with next handover: %v", err)
	}
	if afterHandover.ActiveParticipations() != 0 {
		t.Fatalf("active = %d, want 0", afterHandover.ActiveParticipations())
	}
	handedParticipation, _ := afterHandover.ParticipationFor(mustValue(t, domain.NewCarriedObjectReference, "parcel-2"))
	handedKind, _, _, _ := handedParticipation.End()
	if handedKind != domain.EndedByNextHandover {
		t.Fatalf("end kind = %q, want NEXT_HANDOVER（两对象结果各异并存）", handedKind)
	}

	closed, err := afterHandover.CloseSegment(handoverJudgedAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("close segment: %v", err)
	}
	if !closed.Closed() {
		t.Fatal("段没有关上")
	}

	t.Run("an ended participation cannot be ended again", func(t *testing.T) {
		if _, err := afterHandover.EndParticipationWithDelivery(formedDelivery(t, "parcel-1")); !errors.Is(err, domain.ErrObjectNotParticipating) {
			t.Fatalf("error = %v, want ErrObjectNotParticipating（更正走新版本，不在这里覆盖）", err)
		}
	})

	t.Run("an unknown object cannot be ended", func(t *testing.T) {
		if _, err := afterHandover.EndParticipationWithDelivery(formedDelivery(t, "parcel-9")); !errors.Is(err, domain.ErrObjectNotParticipating) {
			t.Fatalf("error = %v, want ErrObjectNotParticipating", err)
		}
	})

	t.Run("a refused next handover ends nothing", func(t *testing.T) {
		if _, err := segment.EndParticipationWithNextHandover(formedHandover(t, "parcel-2", domain.HandoverRefused)); !errors.Is(err, domain.ErrSegmentNeedsAControlFact) {
			t.Fatalf("error = %v, want ErrSegmentNeedsAControlFact", err)
		}
	})

	t.Run("a closed segment accepts no new objects", func(t *testing.T) {
		if _, err := closed.JoinWithPickup(formedPickup(t, "parcel-3", "attempt-2"), domain.PlannedSegmentReference{}); !errors.Is(err, domain.ErrSegmentClosed) {
			t.Fatalf("error = %v, want ErrSegmentClosed", err)
		}
	})

	// 段结束在最后一个对象离开控制之后：parcel-2 在 handoverJudgedAt 才交出去，一个早于它的
	// 关闭时刻意味着段在那个对象仍受控时就结束了——与「一个仍在控制中的对象足以让段继续存在」
	// 同一条，只是换成时序面。等于允许（同一刻交出最后一个对象并关段）。
	t.Run("the segment cannot close before its last participation ended", func(t *testing.T) {
		if _, err := afterHandover.CloseSegment(handoverJudgedAt.Add(-time.Minute)); !errors.Is(err, domain.ErrInvalidFulfillmentSegment) {
			t.Fatalf("error = %v, want ErrInvalidFulfillmentSegment（关闭早于最后一条参与的终点）", err)
		}
		if _, err := afterHandover.CloseSegment(handoverJudgedAt); err != nil {
			t.Fatalf("与最后一条参与终点同刻的关闭应当收得下：%v", err)
		}
	})
}

// Covers: CONTEXT「车辆故障、运输中断、失联…不自动结束实际履约段。没有有效交付、权威
// 交接或明确控制终止依据时，当前运输方的控制责任不得因停止移动而消失」与「已成立的
// 实际履约段不存在『取消回未开始』转换」——仍有有效参与时段关不上；不带依据的终止
// 进不来；类型上没有任何取消或回退方法。
func TestInterruptionDoesNotEndTheSegment(t *testing.T) {
	reference := mustValue(t, domain.NewFulfillmentSegmentReference, "segment-1")
	segment, err := domain.EstablishSegmentWithPickup(reference, formedPickup(t, "parcel-1", "attempt-1"), domain.PlannedSegmentReference{})
	if err != nil {
		t.Fatalf("establish: %v", err)
	}

	if _, err := segment.CloseSegment(pickedUpAt.Add(time.Hour)); !errors.Is(err, domain.ErrSegmentStillActive) {
		t.Fatalf("error = %v, want ErrSegmentStillActive（中断不是控制结束事实）", err)
	}

	t.Run("a termination without a basis cannot end control", func(t *testing.T) {
		if _, err := segment.EndParticipationWithTermination(
			mustValue(t, domain.NewCarriedObjectReference, "parcel-1"),
			domain.ParticipationBasisReference{},
			pickedUpAt.Add(time.Hour),
		); !errors.Is(err, domain.ErrInvalidFulfillmentSegment) {
			t.Fatalf("error = %v; 没有依据的终止让控制责任凭空消失", err)
		}
	})

	t.Run("an explicit termination with its basis ends the participation", func(t *testing.T) {
		terminated, err := segment.EndParticipationWithTermination(
			mustValue(t, domain.NewCarriedObjectReference, "parcel-1"),
			mustValue(t, domain.NewParticipationBasisReference, "CONTROL-TERMINATION/case-1"),
			pickedUpAt.Add(2*time.Hour),
		)
		if err != nil {
			t.Fatalf("terminate: %v", err)
		}
		kind, _, _, ended := func() (domain.ParticipationEndKind, domain.ParticipationBasisReference, time.Time, bool) {
			participation, _ := terminated.ParticipationFor(mustValue(t, domain.NewCarriedObjectReference, "parcel-1"))
			return participation.End()
		}()
		if !ended || kind != domain.EndedByControlTermination {
			t.Fatalf("end kind = %q ended=%v, want CONTROL_TERMINATED", kind, ended)
		}
		if segment.ActiveParticipations() != 1 {
			t.Fatal("终止改写了原段值——值语义破了")
		}
	})

	t.Run("an end before the entry is refused", func(t *testing.T) {
		if _, err := segment.EndParticipationWithTermination(
			mustValue(t, domain.NewCarriedObjectReference, "parcel-1"),
			mustValue(t, domain.NewParticipationBasisReference, "CONTROL-TERMINATION/case-2"),
			pickedUpAt.Add(-time.Hour),
		); !errors.Is(err, domain.ErrInvalidFulfillmentSegment) {
			t.Fatalf("error = %v; 控制不可能在进入之前结束", err)
		}
	})

	t.Run("the entry and end kind sets are closed", func(t *testing.T) {
		if domain.ParticipationEntryKind(3).String() != "" {
			t.Fatal("第三个参与起点取值带了标签——封闭集合被悄悄放开")
		}
		if domain.ParticipationEndKind(4).String() != "" {
			t.Fatal("第四个参与终点取值带了标签——封闭集合被悄悄放开")
		}
	})
}
