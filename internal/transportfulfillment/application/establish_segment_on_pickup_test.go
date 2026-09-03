package application_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

type pickupSegmentFixture struct {
	pickups  *pickupRegistryDouble
	segments *segmentRegistryDouble
	handler  *application.RegisterOffsitePickupHandler
}

func newPickupSegmentFixture(t *testing.T) *pickupSegmentFixture {
	t.Helper()
	fixture := &pickupSegmentFixture{
		pickups:  newPickupRegistry(),
		segments: newSegmentRegistry(),
	}
	fixture.handler = application.NewRegisterOffsitePickupHandler(application.RegisterOffsitePickupDeps{
		Pickups:    fixture.pickups,
		Segments:   fixture.segments,
		Versions:   &pickupRegVersionFactory{},
		Downstream: &pickupRegHandoffDouble{},
		Clock:      pickupRegClock{at: pickupRegisteredAt},
	})
	return fixture
}

// 首个对象凭有效收寄立段：CONTEXT 生命周期①的另一半来源。与交接那一半的分别只在
// 参与起点的种类与时刻——收寄取业务发生时间，不取登记时刻。
func TestPickedUpFirstObjectEstablishesSegment(t *testing.T) {
	fixture := newPickupSegmentFixture(t)
	command := pickupRegistrationCommand(t)
	command.Segment = "segment-1"
	command.PlannedSegment = "planned-segment-1"

	result, err := fixture.handler.Register(t.Context(), command)
	if err != nil {
		t.Fatalf("登记：%v", err)
	}
	if result.Outcome() != application.PickupRegistered {
		t.Fatalf("outcome = %q, want PICKUP_REGISTERED", result.Outcome())
	}

	record := fixture.segments.saved(t, "tenant-1", "segment-1")
	object, err := domain.NewCarriedObjectReference("parcel-1")
	if err != nil {
		t.Fatalf("对象引用：%v", err)
	}
	participation, joined := record.Segment.ParticipationFor(object)
	if !joined {
		t.Fatal("段成立了，首个对象却没有履约参与关系")
	}
	if participation.EntryKind() != domain.EnteredByOffsitePickup {
		t.Fatalf("参与起点种类 = %q, want OFFSITE_PICKUP", participation.EntryKind())
	}
	if !participation.EnteredAt().Equal(pickupOccurredAt) {
		t.Fatalf("参与起点时刻 = %s, want %s", participation.EnteredAt(), pickupOccurredAt)
	}
}

// 段引用缺席时照登不误，一次不碰段登记册——今天全部调用方都不给段号，这条保住它们。
func TestPickupWithoutASegmentReferenceStillRegisters(t *testing.T) {
	fixture := newPickupSegmentFixture(t)

	result, err := fixture.handler.Register(t.Context(), pickupRegistrationCommand(t))
	if err != nil {
		t.Fatalf("登记：%v", err)
	}
	if result.Outcome() != application.PickupRegistered {
		t.Fatalf("outcome = %q, want PICKUP_REGISTERED", result.Outcome())
	}
	if fixture.segments.saves != 0 || fixture.segments.joins != 0 {
		t.Fatalf("没给段引用却动了段登记册：saves=%d joins=%d", fixture.segments.saves, fixture.segments.joins)
	}
	if reference := result.SegmentContinuationReference(); reference != "" {
		t.Fatalf("没给段引用却留了续办引用 %q——那会让调用方去续办一件它从没要求过的事", reference)
	}
}
