package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/application"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// 本文件钉实测变化重判：称重不带来新位置，在最近一次已判控制触发的位置与业务时间上按此刻仍有效的实测再判一次。

func measurementCommand(t *testing.T, source string, at time.Time) application.RejudgeOnMeasurementChangeCommand {
	t.Helper()
	key := reassessKey(t)
	correlation, err := domain.NewMeasurementChangeCorrelation(key.TenantID, key.DeclaredParcelID, source)
	if err != nil {
		t.Fatalf("关联：%v", err)
	}
	return application.RejudgeOnMeasurementChangeCommand{
		Correlation:   correlation,
		Key:           key,
		SourceVersion: value(t, domain.NewSourceFactVersionReference, source),
		OccurredAt:    at,
		AcceptedAt:    reassessAcceptedAt,
		Carried:       reassessCarried,
	}
}

func seedAnchor(
	t *testing.T,
	fixture *reassessFixture,
	correlation string,
	control domain.ControlEvidenceKind,
	location string,
	at time.Time,
) {
	t.Helper()
	record := ports.ReassessmentRecord{
		Correlation:       value(t, domain.NewRequestCorrelationID, correlation),
		Key:               reassessKey(t),
		Conclusion:        ports.ReassessmentStillApplicable,
		ReviewedPlan:      value(t, domain.NewRoutePlanVersionID, "plan-1/v1"),
		TriggerControl:    control,
		TriggerLocation:   value(t, domain.NewActualLocationReference, location),
		TriggerOccurredAt: at,
		ReassessedAt:      at,
	}
	if _, err := fixture.store.Save(context.Background(), record.Correlation, record); err != nil {
		t.Fatalf("落下锚点：%v", err)
	}
}

func planEffectiveAt(t *testing.T, at time.Time) ports.InitialRouteRecord {
	t.Helper()
	evidence := routableEvidence(t)
	plan, err := domain.FormInitialRoutePlan(domain.InitialRoutePlanSpec{
		Key:           reassessKey(t),
		Version:       value(t, domain.NewRoutePlanVersionID, "plan-1/v1"),
		Selected:      value(t, domain.NewCandidateID, "candidate-1"),
		Candidates:    []domain.RouteCandidate{mustQualified(t, "candidate-1")},
		Legs:          evidence.Paths[0].Legs,
		Strategy:      evidence.Strategy,
		ViewRevision:  evidence.ViewRevision,
		JudgedAt:      at.Add(-time.Hour),
		EffectiveFrom: at,
	})
	if err != nil {
		t.Fatalf("计划：%v", err)
	}
	return ports.InitialRouteRecord{Key: reassessKey(t), Plan: plan, HasPlan: true}
}

func handleMeasurement(t *testing.T, fixture *reassessFixture, command application.RejudgeOnMeasurementChangeCommand) application.ReassessRouteResult {
	t.Helper()
	result, err := fixture.handler.HandleMeasurementChange(context.Background(), command)
	if err != nil {
		t.Fatalf("实测变化：%v", err)
	}
	return result
}

// Covers: 有控制位置时重判落册，记录上的控制依据、位置、业务时间等于锚点；取成本的判断点是实测变化重判，
// 物理事实截至判断此刻而不是收寄时刻。之后一条业务时间夹在锚点与这次变化时刻之间的交接照常判，不被判成早于当前位置。
func TestAMeasurementChangeRejudgesAtTheAnchorWithoutMovingTheCurrentPosition(t *testing.T) {
	// 阈值高到改善不够格：仍适用，但取成本那一步会走到，判断点才查得到。
	fixture := improvementFixture(t, 10000)
	anchor := reassessedAt
	seedAnchor(t, fixture, "anchor-intake", domain.NodeIntakeControl, "node-origin", anchor)
	savedBefore := fixture.store.saved

	changedAt := anchor.Add(2 * time.Hour)
	command := measurementCommand(t, "measure-1", changedAt)
	result := handleMeasurement(t, fixture, command)
	if result.Outcome() != application.ReassessedStillApplicable {
		t.Fatalf("outcome = %q, want STILL_APPLICABLE", result.Outcome())
	}
	got, ok := result.Record()
	if !ok || got.TriggerControl != domain.NodeIntakeControl || got.TriggerLocation.String() != "node-origin" ||
		!got.TriggerOccurredAt.Equal(anchor) {
		t.Fatalf("记录的锚点 = %s %s %s，want 节点收寄 node-origin %s",
			got.TriggerControl, got.TriggerLocation, got.TriggerOccurredAt, anchor)
	}
	if len(fixture.costs.points) == 0 || fixture.costs.points[len(fixture.costs.points)-1].Kind() != domain.MeasurementChangeFactsPoint {
		t.Fatalf("取成本的判断点 = %v，want 实测变化重判", fixture.costs.points)
	}
	asOf, ok := fixture.costs.points[len(fixture.costs.points)-1].PhysicalFactsAsOf(changedAt)
	if !ok || !asOf.Equal(changedAt) {
		t.Fatalf("物理事实截至 %s（ok %v），want 判断此刻 %s，而不是锚点 %s", asOf, ok, changedAt, anchor)
	}
	latest, positioned, err := fixture.store.LatestTriggerAt(context.Background(), reassessKey(t))
	if err != nil || !positioned || !latest.Equal(anchor) {
		t.Fatalf("当前位置 = %s（found %v err %v），want 仍是锚点 %s", latest, positioned, err, anchor)
	}
	if fixture.store.saved != savedBefore+1 {
		t.Fatalf("复核落了 %d 条，want 在锚点之外再落 1 条", fixture.store.saved-savedBefore)
	}

	between := handleOK(t, fixture, reassessCommandAt(t, "handover-between", "node-origin", anchor.Add(30*time.Minute)))
	if between.Outcome() == application.ReassessTriggerNotJudged &&
		between.NotJudgedReason() == ports.NotJudgedTriggerPrecedesCurrentPosition {
		t.Fatal("夹在锚点与变化时刻之间的交接被判成早于当前位置")
	}
	if between.Outcome() == application.ReassessTriggerNotJudged {
		t.Fatalf("夹在中间的交接未判：%s", between.NotJudgedReason())
	}
}

// Covers: 没有控制位置，或最近一次时刻上定不下位置，入账未判、原因是控制位置未定，复核册不落这一关联的行。
func TestAMeasurementChangeWithoutAUsableControlPositionIsNotJudged(t *testing.T) {
	t.Run("没有", func(t *testing.T) {
		fixture := newReassessFixture(t)
		changedAt := reassessedAt.Add(time.Hour)
		command := measurementCommand(t, "measure-none", changedAt)
		result := handleMeasurement(t, fixture, command)
		if result.Outcome() != application.ReassessTriggerNotJudged ||
			result.NotJudgedReason() != ports.NotJudgedControlPositionUnavailable {
			t.Fatalf("outcome = %q/%s，want 未判/控制位置未定", result.Outcome(), result.NotJudgedReason())
		}
		if _, present := fixture.store.byCorrelation[command.Correlation]; present {
			t.Fatal("没有位置却落下了复核行")
		}
		recorded := fixture.store.notJudged[command.Correlation]
		if !recorded.TriggerOccurredAt.Equal(changedAt) {
			t.Fatalf("未判登记的业务时间 = %s，want 这次变化的 %s", recorded.TriggerOccurredAt, changedAt)
		}
	})
	t.Run("定不下", func(t *testing.T) {
		fixture := newReassessFixture(t)
		seedAnchor(t, fixture, "anchor-a", domain.NodeIntakeControl, "node-a", reassessedAt)
		seedAnchor(t, fixture, "anchor-b", domain.TransportHandoverControl, "node-b", reassessedAt)
		command := measurementCommand(t, "measure-split", reassessedAt.Add(time.Hour))
		result := handleMeasurement(t, fixture, command)
		if result.Outcome() != application.ReassessTriggerNotJudged ||
			result.NotJudgedReason() != ports.NotJudgedControlPositionUnavailable {
			t.Fatalf("outcome = %q/%s，want 未判/控制位置未定", result.Outcome(), result.NotJudgedReason())
		}
		if _, present := fixture.store.byCorrelation[command.Correlation]; present {
			t.Fatal("定不下位置却落下了复核行")
		}
	})
}

// Covers: 同关联再投答原结论；同关联换了来源版本答冲突；两次投递之间锚点变了不是另一份指纹。
func TestAMeasurementChangeReplayKeepsItsFingerprintWhenOnlyTheAnchorMoves(t *testing.T) {
	fixture := newReassessFixture(t)
	record := currentPlanRecord(t)
	fixture.routes.records[record.Key] = record
	fixture.evidence.byParcel["parcel-1"] = evidenceBeforeFreeze(t)
	seedAnchor(t, fixture, "anchor", domain.NodeIntakeControl, "node-origin", reassessedAt)
	command := measurementCommand(t, "measure-replay", reassessedAt.Add(time.Hour))

	first := handleMeasurement(t, fixture, command)
	if first.Outcome() != application.ReassessedStillApplicable {
		t.Fatalf("首次 = %q, want STILL_APPLICABLE", first.Outcome())
	}
	for id, stored := range fixture.store.byCorrelation {
		if stored.TriggerControl != domain.NodeIntakeControl {
			continue
		}
		stored.TriggerLocation = value(t, domain.NewActualLocationReference, "node-moved")
		fixture.store.byCorrelation[id] = stored
	}
	replay := handleMeasurement(t, fixture, command)
	if replay.Outcome() != application.ReassessedStillApplicable {
		t.Fatalf("锚点变了之后再投 = %q, want 原结论，不是冲突", replay.Outcome())
	}

	moved := command
	moved.SourceVersion = value(t, domain.NewSourceFactVersionReference, "measure-other")
	conflict := handleMeasurement(t, fixture, moved)
	if conflict.Outcome() != application.ReassessTriggerConflict {
		t.Fatalf("同关联异指纹 = %q, want TRIGGER_CONFLICT", conflict.Outcome())
	}
}

// Covers: 重判判出改善替代时，原计划在锚点时刻被替代，新计划从锚点时刻生效。起点计划恰在锚点时刻生效也能被替代。
func TestAnImprovementOnAMeasurementChangeSupersedesAtTheAnchor(t *testing.T) {
	anchor := reassessedAt
	fixture := improvementFixture(t, 500)
	fixture.routes.records[reassessKey(t)] = planEffectiveAt(t, anchor)
	seedAnchor(t, fixture, "anchor", domain.TransportHandoverControl, "node-origin", anchor)

	result := handleMeasurement(t, fixture, measurementCommand(t, "measure-improve", anchor.Add(3*time.Hour)))
	if result.Outcome() != application.ReassessedRerouted {
		t.Fatalf("outcome = %q, want REROUTED", result.Outcome())
	}
	record, _ := result.Record()
	if !record.HasNewPlan || !record.NewPlan.EffectiveFrom().Equal(anchor) || !record.TriggerOccurredAt.Equal(anchor) {
		t.Fatalf("新计划生效 %s、记录业务时间 %s，want 都是锚点 %s",
			record.NewPlan.EffectiveFrom(), record.TriggerOccurredAt, anchor)
	}
	var superseded bool
	for _, saved := range fixture.applicability.saved {
		if saved.State() == domain.PlanSuperseded && saved.TransitionedAt().Equal(anchor) {
			superseded = true
		}
	}
	if !superseded {
		t.Fatal("原计划没有在锚点时刻被替代")
	}
}

// Covers: 起点计划晚于锚点才生效，判出改善或失效时入账「触发早于计划生效」，不绕开这条。
func TestAMeasurementChangeBeforeThePlanBecomesEffectiveIsNotJudged(t *testing.T) {
	anchor := reassessedAt
	later := anchor.Add(2 * time.Hour)

	t.Run("改善", func(t *testing.T) {
		fixture := improvementFixture(t, 500)
		fixture.routes.records[reassessKey(t)] = planEffectiveAt(t, later)
		seedAnchor(t, fixture, "anchor", domain.NodeIntakeControl, "node-origin", anchor)
		result := handleMeasurement(t, fixture, measurementCommand(t, "measure-early-improve", anchor.Add(time.Hour)))
		if result.Outcome() != application.ReassessTriggerNotJudged ||
			result.NotJudgedReason() != ports.NotJudgedTriggerPrecedesPlanEffectiveFrom {
			t.Fatalf("outcome = %q/%s，want 未判/触发早于计划生效", result.Outcome(), result.NotJudgedReason())
		}
	})
	t.Run("失效", func(t *testing.T) {
		fixture := newReassessFixture(t)
		fixture.routes.records[reassessKey(t)] = planEffectiveAt(t, later)
		fixture.evidence.byParcel["parcel-1"] = evidenceBeforeFreeze(t)
		seedAnchor(t, fixture, "anchor", domain.NodeIntakeControl, "node-actual-elsewhere", anchor)
		result := handleMeasurement(t, fixture, measurementCommand(t, "measure-early-lapse", anchor.Add(time.Hour)))
		if result.Outcome() != application.ReassessTriggerNotJudged ||
			result.NotJudgedReason() != ports.NotJudgedTriggerPrecedesPlanEffectiveFrom {
			t.Fatalf("outcome = %q/%s，want 未判/触发早于计划生效", result.Outcome(), result.NotJudgedReason())
		}
	})
}

// Covers: 锚点是场外揽收时仍适用即保留，不试改善，也不向取数侧要成本。
func TestAnOffsiteAnchorKeepsThePlanWithoutTryingImprovement(t *testing.T) {
	fixture := improvementFixture(t, 500)
	seedAnchor(t, fixture, "anchor-offsite", domain.OffsitePickupControl, "customer-door", reassessedAt)
	result := handleMeasurement(t, fixture, measurementCommand(t, "measure-offsite", reassessedAt.Add(time.Hour)))
	if result.Outcome() != application.ReassessedStillApplicable {
		t.Fatalf("outcome = %q, want STILL_APPLICABLE", result.Outcome())
	}
	record, _ := result.Record()
	if record.TriggerControl != domain.OffsitePickupControl || record.HasNewPlan || len(fixture.costs.points) != 0 {
		t.Fatalf("控制依据 %s、新计划 %v、取成本 %d 次，want 场外揽收、保留原计划、不问成本",
			record.TriggerControl, record.HasNewPlan, len(fixture.costs.points))
	}
}

func awaitMeasurement(t *testing.T, fixture *reassessFixture, source string, at time.Time) domain.RequestCorrelationID {
	t.Helper()
	command := measurementCommand(t, source, at)
	if err := fixture.store.RecordNotJudged(context.Background(), ports.NotJudgedTrigger{
		Correlation:       command.Correlation,
		Key:               command.Key,
		Reason:            ports.NotJudgedControlPositionUnavailable,
		TriggerOccurredAt: at,
	}); err != nil {
		t.Fatal(err)
	}
	return command.Correlation
}

// Covers: 先有等位置的实测变化，再来控制触发：控制触发与每条补判各落一条；补判的锚点是这次控制触发，判断点是此刻。
// 控制触发自己未判或答已有结果时不补判。补判未决时整个 Handle 答同一格未决。
func TestAControlTriggerCatchesUpMeasurementChangesThatWereWaitingForAPosition(t *testing.T) {
	t.Run("两条都补上", func(t *testing.T) {
		fixture := improvementFixture(t, 10000)
		earlier := awaitMeasurement(t, fixture, "measure-early", reassessedAt.Add(-time.Minute))
		later := awaitMeasurement(t, fixture, "measure-late", reassessedAt.Add(time.Minute))

		control := handleOK(t, fixture, reassessCommandAt(t, "intake-now", "node-origin", reassessedAt))
		if control.Outcome() != application.ReassessedStillApplicable {
			t.Fatalf("控制触发 = %q, want STILL_APPLICABLE", control.Outcome())
		}
		for _, correlation := range []domain.RequestCorrelationID{earlier, later} {
			caught, present := fixture.store.byCorrelation[correlation]
			if !present || caught.TriggerControl != domain.NodeIntakeControl ||
				caught.TriggerLocation.String() != "node-origin" || !caught.TriggerOccurredAt.Equal(reassessedAt) {
				t.Fatalf("补判 %s = present %v、%s %s %s，want 这次控制触发的锚点",
					correlation, present, caught.TriggerControl, caught.TriggerLocation, caught.TriggerOccurredAt)
			}
		}
		var rejudgePoints int
		for _, point := range fixture.costs.points {
			if point.Kind() == domain.MeasurementChangeFactsPoint {
				rejudgePoints++
			}
		}
		if rejudgePoints != 2 {
			t.Fatalf("实测变化重判的取成本次数 = %d，want 2；全部判断点 = %v", rejudgePoints, fixture.costs.points)
		}
	})
	t.Run("控制触发未判不补", func(t *testing.T) {
		fixture := newReassessFixture(t)
		fixture.evidence.byParcel["parcel-1"] = evidenceBeforeFreeze(t)
		waiting := awaitMeasurement(t, fixture, "measure-wait", reassessedAt)
		result := handleOK(t, fixture, reassessCommand(t, "node-origin"))
		if result.Outcome() != application.ReassessTriggerNotJudged {
			t.Fatalf("控制触发 = %q, want 未判", result.Outcome())
		}
		if _, present := fixture.store.byCorrelation[waiting]; present {
			t.Fatal("控制触发自己未判却补判了")
		}
	})
	t.Run("已有结果不补", func(t *testing.T) {
		fixture := newReassessFixture(t)
		record := currentPlanRecord(t)
		fixture.routes.records[record.Key] = record
		fixture.evidence.byParcel["parcel-1"] = evidenceBeforeFreeze(t)
		command := reassessCommand(t, "node-origin")
		seed := ports.ReassessmentRecord{
			Correlation:       command.Trigger.Correlation,
			Key:               command.Trigger.Key,
			Conclusion:        ports.ReassessmentStillApplicable,
			TriggerControl:    domain.NodeIntakeControl,
			TriggerLocation:   command.Trigger.Location,
			TriggerOccurredAt: command.Trigger.OccurredAt,
			ReassessedAt:      reassessedAt,
		}
		if _, err := fixture.store.Save(context.Background(), seed.Correlation, seed); err != nil {
			t.Fatal(err)
		}
		waiting := awaitMeasurement(t, fixture, "measure-existing", reassessedAt.Add(time.Hour))
		saved := fixture.store.saved
		result := handleOK(t, fixture, command)
		if result.Outcome() != application.ReassessExistingResult || fixture.store.saved != saved {
			t.Fatalf("outcome = %q、新落 %d 条，want 已有结果且不补判", result.Outcome(), fixture.store.saved-saved)
		}
		if _, present := fixture.store.byCorrelation[waiting]; present {
			t.Fatal("已有结果仍补判了")
		}
	})
	t.Run("补判未决则整体未决", func(t *testing.T) {
		fixture := newReassessFixture(t)
		record := currentPlanRecord(t)
		fixture.routes.records[record.Key] = record
		fixture.evidence.byParcel["parcel-1"] = evidenceBeforeFreeze(t)
		awaitMeasurement(t, fixture, "measure-stall", reassessedAt.Add(time.Hour))
		fixture.store.saveErr = errors.New("store down")
		fixture.store.saveErrAfter = 1
		result := handleOK(t, fixture, reassessCommand(t, "node-origin"))
		if result.Outcome() != application.ReassessUndecided || result.UndecidedReason() != application.ReassessStoreUnavailable {
			t.Fatalf("outcome = %q/%s，want 未决/登记册不可用", result.Outcome(), result.UndecidedReason())
		}
	})
}
