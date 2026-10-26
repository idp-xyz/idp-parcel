package application_test

import (
	"errors"
	"strconv"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证票 demo-chain-gaps/130（ADR-0307）：前版待确认或拒收、本版`已交接`的交接更正就是对象的这一次交接——结束对象此刻
// 在场的参与、进首登时声明的段，与首登即为`已交接`逐格相同；对象此后已有更晚的入场或离场时一笔不写、单开答格；往回更正
// 仍走 ADR-0112 那一条链。结束参与走真编排（participationFixture），夹具全部合成。

const (
	enteringScope   = "handover-scope-1"
	enteringV1      = "handover-result/parcel-1/v1"
	enteringV2      = "handover-result/parcel-1/v2"
	previousSegment = "segment-0"
	declaredSegment = "segment-1"
)

// handoverKeyBasisOf 照 ADR-0221 的铸法拼出（对象，范围，版本）那一版交接作参与依据时的串。
func handoverKeyBasisOf(object, scope, version string) string {
	basis := "TRANSPORT-HANDOVER-KEY"
	for _, part := range []string{object, scope, version} {
		basis += "/" + strconv.Itoa(len(part)) + ":" + part
	}
	return basis
}

// confirmingAfter 是把 predecessor 更正成`已交接`的那一份，更正时刻取在前版业务时间之后——更正不早于被更正的判断。
func confirmingAfter(t *testing.T, predecessor, next string, judgedAt time.Time) application.CorrectTransportHandoverCommand {
	t.Helper()
	command := correctHandoverStillHandedOver(t, predecessor, next)
	command.CorrectedAt = judgedAt.Add(30 * time.Minute)
	return command
}

// enterPreviousSegment 让 object 凭一次`已交接`进 segment-0，业务时间 at。那一次是对象的第一次交接。
func (fixture *participationFixture) enterPreviousSegment(t *testing.T, handler *application.RegisterTransportHandoverHandler, object string, at time.Time) {
	t.Helper()
	command := registerHandoverCommand(t)
	command.Object = object
	command.Scope = "handover-scope-0"
	command.Version = "handover-result/" + object + "/h0"
	command.JudgedAt = at
	command.Segment = previousSegment
	result, err := handler.Register(t.Context(), command)
	if err != nil || result.Outcome() != application.HandoverRegistered || result.SegmentEntryRefusal() != application.SegmentEntryRefusalNone {
		t.Fatalf("%s 进 %s：%v %s %s", object, previousSegment, err, result.Outcome(), result.SegmentEntryRefusal())
	}
}

// firstJudgment 是 parcel-1 在 handover-scope-1 上的 v1：verdict 为`已交接`时带双方证据与规则，否则带依据。段声明三格照给。
func firstJudgment(t *testing.T, verdict domain.HandoverVerdict, at time.Time) application.RegisterTransportHandoverCommand {
	t.Helper()
	command := registerHandoverCommand(t)
	command.Scope, command.Version, command.JudgedAt = enteringScope, enteringV1, at
	command.Verdict = verdict
	if verdict != domain.ObjectHandedOver {
		command.Rule = ""
		command.Basis = "handover-basis/not-yet"
	}
	command.Segment = declaredSegment
	command.PlannedSegment = "planned-segment-1"
	command.SegmentServiceAction = domain.SegmentServesLinehaul.String()
	return command
}

func (fixture *participationFixture) tail(t *testing.T, segment, object string) (domain.FulfillmentParticipation, bool) {
	t.Helper()
	for _, row := range fixture.segments.rows {
		if row.key.Segment.String() != segment {
			continue
		}
		record := fixture.segments.saved(t, "tenant-1", segment)
		return record.Segment.ParticipationFor(value(t, domain.NewCarriedObjectReference, object))
	}
	return domain.FulfillmentParticipation{}, false
}

func (fixture *participationFixture) activeSegmentsOf(t *testing.T, object string) []string {
	t.Helper()
	keys, err := fixture.segments.FindActiveSegments(t.Context(), fixture.tenant, value(t, domain.NewCarriedObjectReference, object))
	if err != nil {
		t.Fatalf("按对象找在场段：%v", err)
	}
	segments := make([]string, 0, len(keys))
	for _, key := range keys {
		segments = append(segments, key.Segment.String())
	}
	return segments
}

// Covers: AT-TF-329（票 demo-chain-gaps/130；ADR-0307 决定一、二）——交接首登为待确认或已拒收（对象仍在先前的段里、控制没有
// 转出），之后更正成`已交接`：这一版结束对象在先前段里的参与（终点依据是这一版按交接键铸的串、时刻是交接业务时间），对象凭
// 这一版进首登时声明的那一段（段立起来、服务动作与计划段取那份声明），与首登即为`已交接`逐格相同；同段邻居不动；前版待确认
// 与拒收同路。此前这一版答`无可替代的参与`，对象挂在先前的段里、新段立不起来。
func TestAPendingOrRefusedHandoverCorrectedToHandedOverEntersTheDeclaredSegment(t *testing.T) {
	judgedAt := handoverJudgedTime.Add(3 * time.Hour)
	cases := []struct {
		name    string
		verdict domain.HandoverVerdict
	}{
		{"首登即为已交接（对照）", domain.ObjectHandedOver},
		{"前版待确认", domain.HandoverPendingConfirmation},
		{"前版已拒收", domain.HandoverRefused},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newParticipationFixture(t)
			handler := fixture.handoverHandlerEndingParticipations()
			fixture.enterPreviousSegment(t, handler, "parcel-1", handoverJudgedTime)
			fixture.enterPreviousSegment(t, handler, "parcel-2", handoverJudgedTime)

			first, err := handler.Register(t.Context(), firstJudgment(t, testCase.verdict, judgedAt))
			if err != nil || first.Outcome() != application.HandoverRegistered {
				t.Fatalf("首登：%v %s", err, first.Outcome())
			}
			result, entering := first, enteringV1
			if testCase.verdict != domain.ObjectHandedOver {
				if first.ParticipationEnd() != application.ParticipationEndOutcomeInvalid {
					t.Fatalf("没转出控制的首登结束了参与：%s", first.ParticipationEnd())
				}
				if active := fixture.activeSegmentsOf(t, "parcel-1"); len(active) != 1 || active[0] != previousSegment {
					t.Fatalf("首登之后 parcel-1 在场的段 = %v, want 仍只在 %s", active, previousSegment)
				}
				correction := correctHandoverStillHandedOver(t, enteringV1, enteringV2)
				correction.CorrectedAt = judgedAt.Add(time.Hour)
				result, err = handler.Correct(t.Context(), correction)
				if err != nil || result.Outcome() != application.HandoverCorrected {
					t.Fatalf("更正成已交接：%v %s", err, result.Outcome())
				}
				entering = enteringV2
			}

			if result.ParticipationEnd() != application.ParticipationEndedNow ||
				result.SegmentEntryRefusal() != application.SegmentEntryRefusalNone || result.SegmentContinuationReference() != "" {
				t.Fatalf("participationEnd=%s refusal=%s debt=%q, want PARTICIPATION_ENDED、两格皆空",
					result.ParticipationEnd(), result.SegmentEntryRefusal(), result.SegmentContinuationReference())
			}
			basis := handoverKeyBasisOf("parcel-1", enteringScope, entering)
			previous, _ := fixture.tail(t, previousSegment, "parcel-1")
			kind, endBasis, endedAt, ended := previous.End()
			if !ended || kind != domain.EndedByNextHandover || endBasis.String() != basis || !endedAt.Equal(judgedAt) {
				t.Fatalf("先前段的参与终点 = %v %s %s %v, want 下一次交接、%s、%s", kind, endBasis, endedAt, ended, basis, judgedAt)
			}
			if neighbour, _ := fixture.tail(t, previousSegment, "parcel-2"); !neighbour.Active() {
				t.Fatal("同段邻居 parcel-2 被一起结束了")
			}
			record := fixture.segments.saved(t, "tenant-1", declaredSegment)
			current, present := record.Segment.ParticipationFor(value(t, domain.NewCarriedObjectReference, "parcel-1"))
			if !present || !current.Active() || current.EntryKind() != domain.EnteredByTransportHandover ||
				current.EntryBasis().String() != basis || !current.EnteredAt().Equal(judgedAt) {
				t.Fatalf("声明的段里的参与 = %+v present=%v, want 在场、凭 %s、起于 %s", current, present, basis, judgedAt)
			}
			if _, chained := current.Supersedes(); chained {
				t.Fatal("进段立的是新参与，不该回指什么")
			}
			if planned, has := current.PlannedSegment(); !has || planned.String() != "planned-segment-1" {
				t.Fatalf("计划段 = %v %v, want 首登声明的 planned-segment-1", planned, has)
			}
			if action, _ := record.Segment.ServiceAction(); action != domain.SegmentServesLinehaul {
				t.Fatalf("段服务动作 = %s, want 首登声明的 LINEHAUL", action)
			}
			if active := fixture.activeSegmentsOf(t, "parcel-1"); len(active) != 1 || active[0] != declaredSegment {
				t.Fatalf("parcel-1 在场的段 = %v, want 只在 %s", active, declaredSegment)
			}
		})
	}
}

// Covers: AT-TF-329 的两种边（ADR-0307 决定一）——前版没声明段：更正成`已交接`照样结束先前段的参与、不进任何段，两格皆空，
// 与首登不声明段同形；对象此前不在任何段里：结束那一半答`NO_ACTIVE_PARTICIPATION`、照进声明的段。更正原样重投答已有版本，
// 不再结束、不再进段。
func TestACorrectionIntoTheSegmentAnswersLikeAFirstRegistration(t *testing.T) {
	judgedAt := handoverJudgedTime.Add(3 * time.Hour)
	t.Run("前版没声明段", func(t *testing.T) {
		fixture := newParticipationFixture(t)
		handler := fixture.handoverHandlerEndingParticipations()
		fixture.enterPreviousSegment(t, handler, "parcel-1", handoverJudgedTime)
		pending := firstJudgment(t, domain.HandoverPendingConfirmation, judgedAt)
		pending.Segment, pending.PlannedSegment, pending.SegmentServiceAction = "", "", ""
		if _, err := handler.Register(t.Context(), pending); err != nil {
			t.Fatalf("首登：%v", err)
		}
		result, err := handler.Correct(t.Context(), confirmingAfter(t, enteringV1, enteringV2, judgedAt))
		if err != nil || result.ParticipationEnd() != application.ParticipationEndedNow ||
			result.SegmentEntryRefusal() != application.SegmentEntryRefusalNone || result.SegmentContinuationReference() != "" {
			t.Fatalf("更正：%v end=%s refusal=%s debt=%q", err, result.ParticipationEnd(), result.SegmentEntryRefusal(), result.SegmentContinuationReference())
		}
		if active := fixture.activeSegmentsOf(t, "parcel-1"); len(active) != 0 {
			t.Fatalf("没声明段却在场于 %v", active)
		}
	})

	t.Run("对象此前不在任何段里", func(t *testing.T) {
		fixture := newParticipationFixture(t)
		handler := fixture.handoverHandlerEndingParticipations()
		if _, err := handler.Register(t.Context(), firstJudgment(t, domain.HandoverPendingConfirmation, judgedAt)); err != nil {
			t.Fatalf("首登：%v", err)
		}
		correction := correctHandoverStillHandedOver(t, enteringV1, enteringV2)
		correction.CorrectedAt = judgedAt.Add(time.Hour)
		result, err := handler.Correct(t.Context(), correction)
		if err != nil || result.ParticipationEnd() != application.ParticipationNoActiveParticipation ||
			result.SegmentEntryRefusal() != application.SegmentEntryRefusalNone {
			t.Fatalf("更正：%v end=%s refusal=%s", err, result.ParticipationEnd(), result.SegmentEntryRefusal())
		}
		if active := fixture.activeSegmentsOf(t, "parcel-1"); len(active) != 1 || active[0] != declaredSegment {
			t.Fatalf("parcel-1 在场的段 = %v, want %s", active, declaredSegment)
		}

		joins := fixture.segments.joins + fixture.segments.saves
		again, err := handler.Correct(t.Context(), correction)
		if err != nil || again.Outcome() != application.HandoverExistingVersion ||
			again.ParticipationEnd() != application.ParticipationEndOutcomeInvalid {
			t.Fatalf("原样重投：%v %s end=%s", err, again.Outcome(), again.ParticipationEnd())
		}
		if fixture.segments.joins+fixture.segments.saves != joins {
			t.Fatal("重投又进了一次段")
		}
	})
}

// Covers: AT-TF-347（票 demo-chain-gaps/130；ADR-0307 决定三）——待确认那一版之后，对象已凭更晚的控制事实挪动过：被下一次
// 交接带进了别的段，或已离场。这时更正成`已交接`照落、意图照交，段那一半一笔不写：不结束此刻的参与、不进声明的段，答
// `LATER_CONTROL_FACTS`、不留续办引用；后续事实一字不动，对象在场的段照旧。同一刻的入场不算更晚，照常进段。
func TestACorrectionIntoTheSegmentAfterLaterControlFactsWritesNoSegment(t *testing.T) {
	judgedAt := handoverJudgedTime.Add(3 * time.Hour)
	later := judgedAt.Add(2 * time.Hour)

	assertRefused := func(t *testing.T, fixture *participationFixture, result application.RegisterTransportHandoverResult, err error) {
		t.Helper()
		if err != nil || result.Outcome() != application.HandoverCorrected {
			t.Fatalf("更正没落：%v %s", err, result.Outcome())
		}
		if result.SegmentEntryRefusal() != application.SegmentEntryRefusedLaterControlFacts ||
			result.SegmentEntryRefusal().String() != "LATER_CONTROL_FACTS" ||
			result.SegmentContinuationReference() != "" || result.ParticipationEnd() != application.ParticipationEndOutcomeInvalid {
			t.Fatalf("refusal=%s debt=%q end=%s, want LATER_CONTROL_FACTS、无欠账、没结束任何参与",
				result.SegmentEntryRefusal(), result.SegmentContinuationReference(), result.ParticipationEnd())
		}
		for _, row := range fixture.segments.rows {
			if row.key.Segment.String() == declaredSegment {
				t.Fatal("声明的段被立起来了")
			}
		}
	}

	t.Run("被下一次交接带进了别的段", func(t *testing.T) {
		fixture := newParticipationFixture(t)
		handler := fixture.handoverHandlerEndingParticipations()
		fixture.enterPreviousSegment(t, handler, "parcel-1", handoverJudgedTime)
		if _, err := handler.Register(t.Context(), firstJudgment(t, domain.HandoverPendingConfirmation, judgedAt)); err != nil {
			t.Fatalf("首登：%v", err)
		}
		onward := registerHandoverCommand(t)
		onward.Scope, onward.Version, onward.JudgedAt, onward.Segment = "handover-scope-2", "handover-result/parcel-1/onward", later, "segment-2"
		if result, err := handler.Register(t.Context(), onward); err != nil || result.ParticipationEnd() != application.ParticipationEndedNow {
			t.Fatalf("下一次交接：%v %s", err, result.ParticipationEnd())
		}

		result, err := handler.Correct(t.Context(), confirmingAfter(t, enteringV1, enteringV2, judgedAt))
		assertRefused(t, fixture, result, err)
		if active := fixture.activeSegmentsOf(t, "parcel-1"); len(active) != 1 || active[0] != "segment-2" {
			t.Fatalf("parcel-1 在场的段 = %v, want 仍只在 segment-2", active)
		}
		previous, _ := fixture.tail(t, previousSegment, "parcel-1")
		if _, endBasis, endedAt, _ := previous.End(); endBasis.String() != handoverKeyBasisOf("parcel-1", "handover-scope-2", "handover-result/parcel-1/onward") || !endedAt.Equal(later) {
			t.Fatalf("先前段的终点被改写：%s %s", endBasis, endedAt)
		}
	})

	t.Run("已离场", func(t *testing.T) {
		fixture := newParticipationFixture(t)
		handler := fixture.handoverHandlerEndingParticipations()
		fixture.enterPreviousSegment(t, handler, "parcel-1", handoverJudgedTime)
		if _, err := handler.Register(t.Context(), firstJudgment(t, domain.HandoverPendingConfirmation, judgedAt)); err != nil {
			t.Fatalf("首登：%v", err)
		}
		termination := terminationCommand(t, "parcel-1")
		termination.Segment, termination.EndedAt = previousSegment, later
		if ended, err := fixture.handler.End(t.Context(), termination); err != nil || ended.Outcome() != application.ParticipationEndedNow {
			t.Fatalf("明确终止：%v %s", err, ended.Outcome())
		}

		result, err := handler.Correct(t.Context(), confirmingAfter(t, enteringV1, enteringV2, judgedAt))
		assertRefused(t, fixture, result, err)
		if active := fixture.activeSegmentsOf(t, "parcel-1"); len(active) != 0 {
			t.Fatalf("离场之后又在场于 %v", active)
		}
	})

	t.Run("同一刻的入场不算更晚", func(t *testing.T) {
		fixture := newParticipationFixture(t)
		handler := fixture.handoverHandlerEndingParticipations()
		fixture.enterPreviousSegment(t, handler, "parcel-1", judgedAt)
		if _, err := handler.Register(t.Context(), firstJudgment(t, domain.HandoverPendingConfirmation, judgedAt)); err != nil {
			t.Fatalf("首登：%v", err)
		}
		result, err := handler.Correct(t.Context(), confirmingAfter(t, enteringV1, enteringV2, judgedAt))
		if err != nil || result.ParticipationEnd() != application.ParticipationEndedNow || result.SegmentEntryRefusal() != application.SegmentEntryRefusalNone {
			t.Fatalf("更正：%v end=%s refusal=%s", err, result.ParticipationEnd(), result.SegmentEntryRefusal())
		}
	})
}

// Covers: AT-TF-329 往回更正那一半（ADR-0307 决定四；ADR-0112 决定四）——待确认 → 更正成`已交接`（进声明的段、结束先前段）
// → 再更正回待确认：声明的段里长出失效参与版本，先前段的参与不还回去（不回退到更早的控制版本），对象此刻不在任何段里 →
// 又更正成`已交接`：从失效版本长出替代版本、回到声明的段，不另立参与，也不再结束任何参与。来回走的是同一条参与链。
func TestAHandoverCorrectedBackAndForthWalksOneParticipationChain(t *testing.T) {
	fixture := newParticipationFixture(t)
	handler := fixture.handoverHandlerEndingParticipations()
	judgedAt := handoverJudgedTime.Add(3 * time.Hour)
	fixture.enterPreviousSegment(t, handler, "parcel-1", handoverJudgedTime)
	if _, err := handler.Register(t.Context(), firstJudgment(t, domain.HandoverPendingConfirmation, judgedAt)); err != nil {
		t.Fatalf("首登：%v", err)
	}
	if entered, err := handler.Correct(t.Context(), confirmingAfter(t, enteringV1, enteringV2, judgedAt)); err != nil ||
		entered.ParticipationEnd() != application.ParticipationEndedNow {
		t.Fatalf("更正成已交接：%v %s", err, entered.ParticipationEnd())
	}

	withdrawing := correctHandoverStillHandedOver(t, enteringV2, "handover-result/parcel-1/v3")
	withdrawing.Verdict = domain.HandoverPendingConfirmation
	withdrawing.ReleasingEvidence, withdrawing.ReceivingEvidence, withdrawing.Rule = "", "", ""
	withdrawing.Basis = "handover-basis/withdrawn"
	withdrawing.CorrectedAt = handoverJudgedTime.Add(4 * time.Hour)
	withdrawn, err := handler.Correct(t.Context(), withdrawing)
	if err != nil || withdrawn.ParticipationEnd() != application.ParticipationEndOutcomeInvalid ||
		withdrawn.SegmentEntryRefusal() != application.SegmentEntryRefusalNone || withdrawn.SegmentContinuationReference() != "" {
		t.Fatalf("更正回待确认：%v end=%s refusal=%s debt=%q", err, withdrawn.ParticipationEnd(), withdrawn.SegmentEntryRefusal(), withdrawn.SegmentContinuationReference())
	}
	voided, _ := fixture.tail(t, declaredSegment, "parcel-1")
	if !voided.Voided() || voided.EntryBasis().String() != handoverKeyBasisOf("parcel-1", enteringScope, "handover-result/parcel-1/v3") {
		t.Fatalf("声明的段里的链尾 = %+v, want v3 的失效版本", voided)
	}
	if previous, _ := fixture.tail(t, previousSegment, "parcel-1"); previous.Active() {
		t.Fatal("撤回控制把先前段的参与还回去了")
	}
	if active := fixture.activeSegmentsOf(t, "parcel-1"); len(active) != 0 {
		t.Fatalf("撤回控制之后 parcel-1 仍在场于 %v", active)
	}

	joins := fixture.segments.joins
	confirming := correctHandoverStillHandedOver(t, "handover-result/parcel-1/v3", "handover-result/parcel-1/v4")
	confirming.CorrectedAt = handoverJudgedTime.Add(5 * time.Hour)
	reconfirmed, err := handler.Correct(t.Context(), confirming)
	if err != nil || reconfirmed.ParticipationEnd() != application.ParticipationEndOutcomeInvalid ||
		reconfirmed.SegmentEntryRefusal() != application.SegmentEntryRefusalNone {
		t.Fatalf("又更正成已交接：%v end=%s refusal=%s", err, reconfirmed.ParticipationEnd(), reconfirmed.SegmentEntryRefusal())
	}
	current, _ := fixture.tail(t, declaredSegment, "parcel-1")
	supersedes, chained := current.Supersedes()
	if !current.Active() || current.EntryBasis().String() != handoverKeyBasisOf("parcel-1", enteringScope, "handover-result/parcel-1/v4") ||
		!chained || supersedes.String() != voided.EntryBasis().String() {
		t.Fatalf("链尾 = %+v, want v4 的替代版本、回指 v3 的失效版本", current)
	}
	if fixture.segments.joins != joins {
		t.Fatal("回到声明的段另立了一条参与，该是链上长一版")
	}
	if active := fixture.activeSegmentsOf(t, "parcel-1"); len(active) != 1 || active[0] != declaredSegment {
		t.Fatalf("parcel-1 在场的段 = %v, want 只在 %s", active, declaredSegment)
	}
}

// Covers: ADR-0307 决定一——改走进段那一路要结束先前段的参与，结束参与那一口没接线时与首登同格：error 带具名格、不形成答案。
// 仍为`已交接`的更正只在同段重派生、用不到它，没接线照答。
func TestACorrectionIntoTheSegmentWithoutAParticipationEnderFormsNoAnswer(t *testing.T) {
	fixture := newParticipationFixture(t)
	handler := application.NewRegisterTransportHandoverHandler(application.RegisterTransportHandoverDeps{
		Handovers:  fixture.handovers,
		Segments:   fixture.segments,
		Downstream: &handoverHandoffDouble{},
		Clock:      handoverClock{at: handoverRegisteredAt},
	})
	if _, err := handler.Register(t.Context(), firstJudgment(t, domain.HandoverPendingConfirmation, handoverJudgedTime)); err != nil {
		t.Fatalf("待确认首登不该要结束参与那一口：%v", err)
	}
	result, err := handler.Correct(t.Context(), correctHandoverStillHandedOver(t, enteringV1, enteringV2))
	if !errors.Is(err, application.ErrParticipationEndsNotWired) || result.Outcome() != application.HandoverRegistrationOutcomeInvalid {
		t.Fatalf("err = %v outcome = %s, want ErrParticipationEndsNotWired 且不形成答案", err, result.Outcome())
	}

	t.Run("仍为已交接的更正不用它", func(t *testing.T) {
		fixture := newHandoverSegmentFixture(t)
		seed := registerHandoverCommand(t)
		seed.Segment = "segment-1"
		if _, err := fixture.handler.Register(t.Context(), seed); err != nil {
			t.Fatalf("首登：%v", err)
		}
		bare := application.NewRegisterTransportHandoverHandler(application.RegisterTransportHandoverDeps{
			Handovers:  fixture.handovers,
			Segments:   fixture.segments,
			Downstream: &handoverHandoffDouble{},
			Clock:      handoverClock{at: handoverRegisteredAt},
		})
		result, err := bare.Correct(t.Context(), correctHandoverStillHandedOver(t, enteringV1, enteringV2))
		if err != nil || result.Outcome() != application.HandoverCorrected || fixture.segments.supersedes != 1 {
			t.Fatalf("同段替代：%v %s supersedes=%d", err, result.Outcome(), fixture.segments.supersedes)
		}
	})
}

// Covers: ADR-0307 决定二——首登把段声明三格原样随那一版留存（空白读作没有声明）；同键重放换了声明照答重放，在册的仍是先到
// 的那一份；更正照前版承继，命令里没有这三格。带 NUL 的声明存不下，答未受理、不落册。
func TestAHandoverKeepsItsSegmentDeclarationAndACorrectionInheritsIt(t *testing.T) {
	fixture := newHandoverSegmentFixture(t)
	pending := firstJudgment(t, domain.HandoverPendingConfirmation, handoverJudgedTime)
	pending.PlannedSegment = "   "
	if result, err := fixture.handler.Register(t.Context(), pending); err != nil || result.Outcome() != application.HandoverRegistered {
		t.Fatalf("首登：%v %s", err, result.Outcome())
	}
	want := ports.HandoverSegmentDeclaration{Segment: declaredSegment, ServiceAction: domain.SegmentServesLinehaul.String()}
	if got := handoverRecordAt(t, &handoverFixture{registry: fixture.handovers}, enteringV1).SegmentDeclaration; got != want {
		t.Fatalf("留存的段声明 = %+v, want %+v", got, want)
	}

	replay := pending
	replay.Segment = "segment-9"
	if result, err := fixture.handler.Register(t.Context(), replay); err != nil || result.Outcome() != application.HandoverExistingVersion {
		t.Fatalf("换了段声明的重放：%v %s, want EXISTING_VERSION", err, result.Outcome())
	}
	if got := handoverRecordAt(t, &handoverFixture{registry: fixture.handovers}, enteringV1).SegmentDeclaration; got != want {
		t.Fatalf("重放顶替了留存的段声明：%+v", got)
	}

	withdrawn := correctHandoverStillHandedOver(t, enteringV1, enteringV2)
	withdrawn.Verdict = domain.HandoverRefused
	withdrawn.ReleasingEvidence, withdrawn.ReceivingEvidence, withdrawn.Rule = "", "", ""
	withdrawn.Basis = "handover-basis/refused"
	if result, err := fixture.handler.Correct(t.Context(), withdrawn); err != nil || result.Outcome() != application.HandoverCorrected {
		t.Fatalf("更正：%v %s", err, result.Outcome())
	}
	if got := handoverRecordAt(t, &handoverFixture{registry: fixture.handovers}, enteringV2).SegmentDeclaration; got != want {
		t.Fatalf("更正那一版的段声明 = %+v, want 照前版承继 %+v", got, want)
	}

	for name, change := range map[string]func(*application.RegisterTransportHandoverCommand){
		"段":   func(command *application.RegisterTransportHandoverCommand) { command.Segment = "segment\x00-1" },
		"计划段": func(command *application.RegisterTransportHandoverCommand) { command.PlannedSegment = "planned\x00" },
		"段服务动作": func(command *application.RegisterTransportHandoverCommand) {
			command.SegmentServiceAction = "LINE\x00HAUL"
		},
	} {
		t.Run(name+"带 NUL", func(t *testing.T) {
			command := firstJudgment(t, domain.HandoverPendingConfirmation, handoverJudgedTime)
			command.Object, command.Version = "parcel-nul", "handover-result/parcel-nul/v1"
			change(&command)
			before := len(fixture.handovers.records)
			result, err := fixture.handler.Register(t.Context(), command)
			if err != nil || result.Outcome() != application.HandoverNotAccepted || len(fixture.handovers.records) != before {
				t.Fatalf("%v %s 落册 %d→%d, want SOURCE_NOT_ACCEPTED 且不落册", err, result.Outcome(), before, len(fixture.handovers.records))
			}
		})
	}
}

// 段登记册整体读不动时，改走进段的更正照旧答欠账（更正落下、重试可续办）——那是重派生那一口先读册失败给的，走不到晚到检查，也不写任何段；晚到检查本身读册失败
// 不作答（见 handOverIntoSegment），所以这里不会有「更正落下、段那一半悄悄放行」的一格。
func TestACorrectionIntoTheSegmentWithTheSegmentRegistryDownWritesNothing(t *testing.T) {
	judgedAt := handoverJudgedTime.Add(3 * time.Hour)
	fixture := newParticipationFixture(t)
	handler := fixture.handoverHandlerEndingParticipations()
	fixture.enterPreviousSegment(t, handler, "parcel-1", handoverJudgedTime)
	if _, err := handler.Register(t.Context(), firstJudgment(t, domain.HandoverPendingConfirmation, judgedAt)); err != nil {
		t.Fatalf("首登：%v", err)
	}
	fixture.segments.findErr = errors.New("segment registry down")
	result, err := handler.Correct(t.Context(), confirmingAfter(t, enteringV1, enteringV2, judgedAt))
	fixture.segments.findErr = nil
	if err != nil || result.Outcome() != application.HandoverCorrected || result.SegmentContinuationReference() == "" {
		t.Fatalf("更正：%v %s debt=%q, want 更正落下、答欠账", err, result.Outcome(), result.SegmentContinuationReference())
	}
	if result.ParticipationEnd() != application.ParticipationEndOutcomeInvalid {
		t.Fatalf("读不动册却结束了参与：%s", result.ParticipationEnd())
	}
	for _, row := range fixture.segments.rows {
		if row.key.Segment.String() == declaredSegment {
			t.Fatal("声明的段被立起来了")
		}
	}
	if active := fixture.activeSegmentsOf(t, "parcel-1"); len(active) != 1 || active[0] != previousSegment {
		t.Fatalf("在场的段 = %v, want 仍只在 %s", active, previousSegment)
	}
}
