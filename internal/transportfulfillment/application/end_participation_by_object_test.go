package application_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// Covers: 票 tf-segment-lifecycle-closure/06 裁决 (a)——交付与下一次交接两路结束参与时**命令不带段**，
// 段由登记册按对象找：交付是关于对象的事实，回传方不知道段。找到一条 → 照常结束；零条 → 单开一格
// NO_ACTIVE_PARTICIPATION（可观察，不静默）；多于一条 → 响亮 error（库面不一致，不挑一个）。
func TestEndingByDeliveryResolvesTheSegmentByObject(t *testing.T) {
	fixture := newParticipationFixture(t)
	fixture.twoMemberSegment(t)
	fixture.joinEarlyMember(t, "parcel-3")
	fixture.registerDelivery(t, "parcel-3", "attempt-1")

	command := terminationCommand(t, "parcel-3")
	command.Segment = ""
	command.Source = application.ParticipationEndedByDelivery
	command.Basis = ""
	command.Attempt = "attempt-1"

	result, err := fixture.handler.End(t.Context(), command)
	if err != nil {
		t.Fatalf("交付结束参与：%v", err)
	}
	if result.Outcome() != application.ParticipationEndedNow {
		t.Fatalf("outcome = %s, want PARTICIPATION_ENDED——段该由登记册按对象找到", result.Outcome())
	}
	if fixture.participation(t, "parcel-3").Active() {
		t.Fatal("parcel-3 的参与没有结束")
	}
	if result.Segment() != "segment-1" {
		t.Fatalf("result.Segment() = %q，结束在哪个段要答给调用方", result.Segment())
	}
}

func TestEndingAnObjectWithNoActiveParticipationIsItsOwnAnswer(t *testing.T) {
	fixture := newParticipationFixture(t)
	fixture.twoMemberSegment(t)
	fixture.registerDelivery(t, "parcel-9", "attempt-9")

	command := terminationCommand(t, "parcel-9")
	command.Segment = ""
	command.Source = application.ParticipationEndedByDelivery
	command.Basis = ""
	command.Attempt = "attempt-9"

	result, err := fixture.handler.End(t.Context(), command)
	if err != nil {
		t.Fatalf("无在场参与：%v", err)
	}
	if result.Outcome() != application.ParticipationNoActiveParticipation {
		t.Fatalf("outcome = %s, want NO_ACTIVE_PARTICIPATION", result.Outcome())
	}
	if result.Outcome().String() != "NO_ACTIVE_PARTICIPATION" {
		t.Fatalf("String() = %q", result.Outcome().String())
	}
}

func TestEndingAnObjectActiveInTwoSegmentsIsALoudError(t *testing.T) {
	fixture := newParticipationFixture(t)
	fixture.twoMemberSegment(t)
	// 同一对象再进第二个段：库面不一致的形状，替身如实存两条。
	handler := application.NewRegisterTransportHandoverHandler(application.RegisterTransportHandoverDeps{
		Handovers:  fixture.handovers,
		Segments:   fixture.segments,
		Downstream: &handoverHandoffDouble{},
		Clock:      handoverClock{at: handoverRegisteredAt},
	})
	second := registerHandoverCommand(t)
	second.Object = "parcel-1"
	second.Scope = "handover-scope-2"
	second.Version = "handover-result/parcel-1/v2"
	second.Segment = "segment-2"
	if _, err := handler.Register(t.Context(), second); err != nil {
		t.Fatalf("进第二段：%v", err)
	}
	fixture.registerDelivery(t, "parcel-1", "attempt-1")

	command := terminationCommand(t, "parcel-1")
	command.Segment = ""
	command.Source = application.ParticipationEndedByDelivery
	command.Basis = ""
	command.Attempt = "attempt-1"

	_, err := fixture.handler.End(t.Context(), command)
	if !errors.Is(err, application.ErrObjectActiveInSeveralSegments) {
		t.Fatalf("err = %v, want ErrObjectActiveInSeveralSegments——一对象同时只能在一个共同控制范围里", err)
	}
	if !fixture.participation(t, "parcel-1").Active() {
		t.Fatal("响亮报错时不该挑一个段去结束")
	}
}

// 终止那一路仍要显式段：它是运营决定，操作者面对的是具体某个段，不由登记册替他找。
func TestTerminationStillRequiresAnExplicitSegment(t *testing.T) {
	fixture := newParticipationFixture(t)
	fixture.twoMemberSegment(t)

	command := terminationCommand(t, "parcel-1")
	command.Segment = ""

	result, err := fixture.handler.End(t.Context(), command)
	if err != nil {
		t.Fatalf("终止不带段：%v", err)
	}
	if result.Outcome() != application.ParticipationEndNotAccepted {
		t.Fatalf("outcome = %s, want INPUT_NOT_ACCEPTED", result.Outcome())
	}
}

// 登记册按对象找不回来是欠账不是答案。
func TestResolvingTheSegmentByObjectFailingIsUndecided(t *testing.T) {
	fixture := newParticipationFixture(t)
	fixture.twoMemberSegment(t)
	fixture.segments.findErr = errors.New("registry down")

	command := terminationCommand(t, "parcel-1")
	command.Segment = ""
	command.Source = application.ParticipationEndedByDelivery
	command.Basis = ""
	command.Attempt = "attempt-1"

	result, err := fixture.handler.End(t.Context(), command)
	if err != nil {
		t.Fatalf("登记册故障：%v", err)
	}
	if result.Outcome() != application.ParticipationEndUndecided || result.ContinuationReference() == "" {
		t.Fatalf("outcome = %s continuation = %q", result.Outcome(), result.ContinuationReference())
	}
}

var _ = domain.EndedByEffectiveDelivery
