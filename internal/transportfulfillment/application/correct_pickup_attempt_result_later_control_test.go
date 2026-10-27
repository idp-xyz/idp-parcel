package application_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证更正成揽收到手晚到时不问更晚的控制事实会让对象两段同时在场（ADR-0307 决定三的同一问，票 docs-consistency/code/90 评审）：
// 待确认那一版之后，对象已凭别的尝试的对象级揽收登记进了另一个段，这时更正命令再声明一个段，不拦就让对象落进两个段。

const otherAttemptSegment = "segment-of-attempt-2"

// enterTheOtherSegmentThroughALaterAttempt 让 parcel-1 凭另一次尝试的对象级揽收登记进 otherAttemptSegment，发生时刻 at。
func (fixture *pickupCorrectionFixture) enterTheOtherSegmentThroughALaterAttempt(t *testing.T, at time.Time) {
	t.Helper()
	registrar := application.NewRegisterOffsitePickupHandler(application.RegisterOffsitePickupDeps{
		Pickups: fixture.registry, Segments: fixture.segments, Versions: &pickupRegVersionFactory{}, Downstream: fixture.regHandoff,
		Clock: pickupRegClock{at: recordedAt},
	})
	result, err := registrar.Register(context.Background(), application.RegisterOffsitePickupCommand{
		TenantID: value(t, domain.NewTenantID, "tenant-1"), Object: "parcel-1", Task: "pickup-task-2", Attempt: "attempt-2",
		Place: "customer-warehouse-1", Control: "TRANSPORT-CONTROL/TF-2", ExecutedBy: "courier-1", OccurredAt: at,
		Segment: otherAttemptSegment,
	})
	if err != nil || result.Outcome() != application.PickupRegistered || result.SegmentEntryRefusal() != application.SegmentEntryRefusalNone ||
		result.SegmentContinuationReference() != "" {
		t.Fatalf("另一次尝试的登记 = %v %s refusal=%s debt=%q err=%v", result.Outcome(), result.Outcome(), result.SegmentEntryRefusal(),
			result.SegmentContinuationReference(), err)
	}
}

func (fixture *pickupCorrectionFixture) activeSegmentsOf(t *testing.T, object string) []string {
	t.Helper()
	keys, err := fixture.segments.FindActiveSegments(context.Background(), value(t, domain.NewTenantID, "tenant-1"),
		value(t, domain.NewCarriedObjectReference, object))
	if err != nil {
		t.Fatal(err)
	}
	var segments []string
	for _, key := range keys {
		segments = append(segments, key.Segment.String())
	}
	return segments
}

// Covers: `AT-TF-363`（编排半边）——ADR-0307 决定三的同一问用在更正成揽收到手上——对象在待确认之后已凭另一次尝试的登记进了 segment-of-attempt-2（发生时刻晚于
// 这一次更正说的发生时刻），晚到的更正命令声明 segment-1：更正照落、场外揽收照登、意图照交（揽收事实是真的），但进段那一半一笔不写——不立 segment-1，答
// `LATER_CONTROL_FACTS`、不留续办引用，对象仍只在原来的那一段。
func TestACorrectionToAPickupAfterLaterControlFactsRegistersThePickupWithoutDeclaringTheSegment(t *testing.T) {
	fixture := newPickupCorrectionFixture(t)
	fixture.enterTheOtherSegmentThroughALaterAttempt(t, arrivedAt.Add(time.Hour))
	intentsBefore := len(fixture.regHandoff.intents)

	result := fixture.correct(t, fixture.pickedUpCommand(t))

	if result.Outcome() != application.AttemptResultCorrected {
		t.Fatalf("outcome = %q, want RESULT_CORRECTED", result.Outcome())
	}
	registration := result.Registration()
	if registration.SegmentEntryRefusal() != application.SegmentEntryRefusedLaterControlFacts ||
		registration.SegmentEntryRefusal().String() != "LATER_CONTROL_FACTS" || registration.SegmentContinuationReference() != "" {
		t.Fatalf("refusal=%s debt=%q, want LATER_CONTROL_FACTS、无欠账", registration.SegmentEntryRefusal(), registration.SegmentContinuationReference())
	}
	if _, found, err := fixture.registry.FindByKey(context.Background(), ports.OffsitePickupKey{
		TenantID: value(t, domain.NewTenantID, "tenant-1"),
		Object:   value(t, domain.NewCarriedObjectReference, "parcel-1"),
		Attempt:  value(t, domain.NewAttemptReference, "attempt-1"),
	}); err != nil || !found {
		t.Fatalf("场外揽收 found=%v err=%v；揽收事实是真的，段那一半拦下不该连它一起拦", found, err)
	}
	if len(fixture.regHandoff.intents) != intentsBefore+1 {
		t.Fatalf("intents = %d, want %d（更正成的场外揽收照交 parcel-shipment）", len(fixture.regHandoff.intents), intentsBefore+1)
	}
	if fixture.corrections.saves != 1 {
		t.Fatalf("correction saves = %d, want 1（更正照落）", fixture.corrections.saves)
	}
	for _, row := range fixture.segments.rows {
		if row.key.Segment.String() == "segment-1" {
			t.Fatal("声明的 segment-1 被立起来了：对象会两段同时在场")
		}
	}
	if active := fixture.activeSegmentsOf(t, "parcel-1"); len(active) != 1 || active[0] != otherAttemptSegment {
		t.Fatalf("在场的段 = %v, want 仍只在 %s", active, otherAttemptSegment)
	}
}

// Covers: `AT-TF-363`（编排半边，对照）——没有更晚的控制事实时更正成揽收到手照常进声明的段：对象在另一个段的入场早于这一次更正说的发生时刻（同一刻也不算更晚），
// 这条拦截不碰它；一件对象在段里没有任何参与的常规路径见 TestAPendingPickupResultCorrectedToAPickupFormsTheOffsitePickupAndEntersTheSegment。
func TestACorrectionToAPickupWithoutLaterControlFactsStillEntersTheDeclaredSegment(t *testing.T) {
	fixture := newPickupCorrectionFixture(t)
	command := fixture.pickedUpCommand(t)
	fixture.enterTheOtherSegmentThroughALaterAttempt(t, command.OccurredAt)

	result := fixture.correct(t, command)

	if result.Outcome() != application.AttemptResultCorrected {
		t.Fatalf("outcome = %q, want RESULT_CORRECTED", result.Outcome())
	}
	if refusal := result.Registration().SegmentEntryRefusal(); refusal != application.SegmentEntryRefusalNone {
		t.Fatalf("refusal = %s；同一刻的入场不算更晚，不该拦", refusal)
	}
	segment := fixture.segments.saved(t, "tenant-1", "segment-1")
	if _, joined := segment.Segment.ParticipationFor(value(t, domain.NewCarriedObjectReference, "parcel-1")); !joined {
		t.Fatal("没有更晚的控制事实，更正成揽收到手的对象没有进声明的段")
	}
}

// Covers: `AT-TF-363`（编排半边）——命令没有声明段时没有进段这一半可拦：对象此后已有更晚的控制事实，更正照落、揽收照登，答复里没有拒绝格（拒绝格说的是声明的段没进成）。
func TestACorrectionToAPickupThatDeclaresNoSegmentCarriesNoRefusalEvenAfterLaterControlFacts(t *testing.T) {
	fixture := newPickupCorrectionFixture(t)
	fixture.enterTheOtherSegmentThroughALaterAttempt(t, arrivedAt.Add(time.Hour))
	command := fixture.pickedUpCommand(t)
	command.Segment = ""

	result := fixture.correct(t, command)

	if result.Outcome() != application.AttemptResultCorrected ||
		result.Registration().SegmentEntryRefusal() != application.SegmentEntryRefusalNone {
		t.Fatalf("outcome=%s refusal=%s, want 更正成立且没有拒绝格", result.Outcome(), result.Registration().SegmentEntryRefusal())
	}
}

// Covers: `AT-TF-363`（编排半边）——同（对象，尝试）的对象级揽收登记早已登下（没带段声明）：晚到的更正读回在册那一份，这一次没碰段，所以答复不带拒绝格——拒绝格说的是这一次登记里
// 声明的段没进成，不是对象的参与链上有没有更晚的事实。
func TestACorrectionAdoptingAnExistingRegistrationCarriesNoRefusalForTheSegmentItNeverTouched(t *testing.T) {
	fixture := newPickupCorrectionFixture(t)
	command := fixture.pickedUpCommand(t)
	registrar := application.NewRegisterOffsitePickupHandler(application.RegisterOffsitePickupDeps{
		Pickups: fixture.registry, Versions: &pickupRegVersionFactory{}, Downstream: fixture.regHandoff, Clock: pickupRegClock{at: recordedAt},
	})
	if first, err := registrar.Register(context.Background(), application.RegisterOffsitePickupCommand{
		TenantID: value(t, domain.NewTenantID, "tenant-1"), Object: "parcel-1", Task: "pickup-task-1", Attempt: "attempt-1",
		Place: "customer-warehouse-1", Control: command.Control.String(), ExecutedBy: "courier-1", OccurredAt: command.OccurredAt,
	}); err != nil || first.Outcome() != application.PickupRegistered {
		t.Fatalf("先登的对象级揽收 = %v err=%v", first.Outcome(), err)
	}
	fixture.enterTheOtherSegmentThroughALaterAttempt(t, arrivedAt.Add(time.Hour))

	result := fixture.correct(t, command)

	if result.Outcome() != application.AttemptResultCorrected ||
		result.Registration().Outcome() != application.PickupExistingVersion ||
		result.Registration().SegmentEntryRefusal() != application.SegmentEntryRefusalNone {
		t.Fatalf("outcome=%s registration=%s refusal=%s, want 更正成立、读回在册那份、没有拒绝格",
			result.Outcome(), result.Registration().Outcome(), result.Registration().SegmentEntryRefusal())
	}
}
