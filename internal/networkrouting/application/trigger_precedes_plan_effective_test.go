package application_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/application"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// 本文件钉复核的第三道业务时间下限（ADR-0292）：当前有效的计划从某一时刻起才生效，授权角色的决定形成的计划从决定的时刻起
// 生效（ADR-0284 决定四）、初始路由并列决定形成的首个计划同理（ADR-0309）。一次业务时间落在原计划失效与这一决定之间的控制触发
// 在决定之后才处理，起点就是还没生效的新计划；判出失效或改善替代时，离场与新计划的生效都取触发的业务时间，早于这份计划的生效，
// 被离场门拒。这时入账为「触发早于计划生效」、不判；判出仍适用不撞。

var (
	// planEffectiveBetween 是计划生效的时刻（决定的时刻），晚于夹具里各触发的业务时间 reassessedAt。
	planEffectiveBetween = reassessedAt.Add(2 * time.Hour)
	// triggerBeforePlanEffective 是落在失效与决定之间的触发业务时间。
	triggerBeforePlanEffective = reassessedAt.Add(time.Hour)
)

// planEffectiveAtRecord 是一份从 at 起才生效的当前计划，进路由历史：与 currentPlanRecord 同形，只有生效与判断时刻不同。
func planEffectiveAtRecord(t *testing.T, at time.Time) ports.InitialRouteRecord {
	t.Helper()
	evidence := routableEvidence(t)
	plan, err := domain.FormInitialRoutePlan(domain.InitialRoutePlanSpec{
		Key:      reassessKey(t),
		Version:  value(t, domain.NewRoutePlanVersionID, "plan-1/v1"),
		Selected: value(t, domain.NewCandidateID, "candidate-1"),
		Candidates: []domain.RouteCandidate{
			mustQualified(t, "candidate-1"),
		},
		Legs:          evidence.Paths[0].Legs,
		Strategy:      evidence.Strategy,
		ViewRevision:  evidence.ViewRevision,
		JudgedAt:      at,
		EffectiveFrom: at,
	})
	if err != nil {
		t.Fatalf("form plan: %v", err)
	}
	return ports.InitialRouteRecord{Key: reassessKey(t), Plan: plan, HasPlan: true}
}

// planNotYetEffectiveFixture 让路由历史里只有一份还没生效的当前计划。
func planNotYetEffectiveFixture(t *testing.T) *reassessFixture {
	t.Helper()
	fixture := newReassessFixture(t)
	record := planEffectiveAtRecord(t, planEffectiveBetween)
	fixture.routes.records[record.Key] = record
	fixture.evidence.byParcel["parcel-1"] = evidenceBeforeFreeze(t)
	return fixture
}

// improvementOn 让原计划所选候选之外有一条够格的更便宜候选，阈值 0 是本测试交入的租户取值。
func improvementOn(t *testing.T, fixture *reassessFixture) {
	t.Helper()
	evidence := declareCostImprovement(cheaperAlternative(t, 100), 0)
	limit := 0
	evidence.FreezeForm = domain.RemainingSegmentCountFreeze
	evidence.FreezeRemainingSegmentLimit = &limit
	fixture.evidence.byParcel["parcel-1"] = evidence
}

// requireNotJudgedBeforePlanEffective 断言这次触发入账为「触发早于计划生效」：复核册、适用性册一笔不写，原计划的适用性不动，
// 未判登记点名比较所用的生效时刻，不带委托接受与当前位置。
func requireNotJudgedBeforePlanEffective(
	t *testing.T, fixture *reassessFixture, command application.ReassessRouteCommand, result application.ReassessRouteResult,
) {
	t.Helper()
	if result.Outcome() != application.ReassessTriggerNotJudged ||
		result.NotJudgedReason() != ports.NotJudgedTriggerPrecedesPlanEffectiveFrom {
		t.Fatalf("复核 = %q/%q（未决 %q）, want TRIGGER_NOT_JUDGED/TRIGGER_PRECEDES_PLAN_EFFECTIVE_FROM",
			result.Outcome(), result.NotJudgedReason(), result.UndecidedReason())
	}
	if fixture.store.saved != 0 || len(fixture.applicability.saved) != 0 {
		t.Fatalf("复核 %d 条、适用性写 %d 次, want 0 与 0——离场门不调用、新计划不形成", fixture.store.saved, len(fixture.applicability.saved))
	}
	noted, ok := fixture.store.notJudged[command.Trigger.Correlation]
	if !ok || noted.Reason != ports.NotJudgedTriggerPrecedesPlanEffectiveFrom ||
		!noted.PlanEffectiveAt.Equal(planEffectiveBetween) || !noted.TriggerOccurredAt.Equal(command.Trigger.OccurredAt) ||
		!noted.ShipmentAcceptedAt.IsZero() || !noted.CurrentPositionAt.IsZero() {
		t.Fatalf("未判登记 = %+v（在 %v）, want 早于计划生效 %s、不带接受与当前位置", noted, ok, planEffectiveBetween)
	}
}

// Covers: AT-NR-104（编排半边）——当前计划从决定的时刻起才生效，一次业务时间早于它的触发在那之后才处理：判出失效（交进的节点
// 不在计划上）、判出改善替代，都入账为「触发早于计划生效」、不判——原计划的适用性不动，不形成新计划；另一封信封带来同一触发
// 同答。若照判，离场与新计划的生效都取触发的业务时间，早于这份计划的生效，被离场门拒，整封信封响亮失败。阈值 0 是本测试
// 交入的租户取值。
func TestATriggerEarlierThanTheCurrentPlansEffectiveTimeIsNotJudged(t *testing.T) {
	t.Run("判出失效", func(t *testing.T) {
		fixture := planNotYetEffectiveFixture(t)
		fixture.evidence.byParcel["parcel-1"] = routableEvidence(t)
		command := reassessCommandAt(t, "trigger-between", "node-actual-elsewhere", triggerBeforePlanEffective)

		requireNotJudgedBeforePlanEffective(t, fixture, command, handleOK(t, fixture, command))
		if again := handleOK(t, fixture, command); again.Outcome() != application.ReassessTriggerNotJudged ||
			again.NotJudgedReason() != ports.NotJudgedTriggerPrecedesPlanEffectiveFrom {
			t.Fatalf("同一触发再来 = %q/%q, want 同答", again.Outcome(), again.NotJudgedReason())
		}
	})

	t.Run("判出改善替代", func(t *testing.T) {
		fixture := planNotYetEffectiveFixture(t)
		improvementOn(t, fixture)
		command := reassessCommandAt(t, "trigger-between", "node-origin", triggerBeforePlanEffective)

		requireNotJudgedBeforePlanEffective(t, fixture, command, handleOK(t, fixture, command))
	})

	t.Run("场外揽收判出失效", func(t *testing.T) {
		fixture := planNotYetEffectiveFixture(t)
		fixture.evidence.byParcel["parcel-1"] = restrictedSelectedCandidate(t, evidenceBeforeFreeze(t))
		command := pickupCommand(t)
		command.Trigger.Correlation = value(t, domain.NewRequestCorrelationID, "trigger-between")
		command.Trigger.OccurredAt = triggerBeforePlanEffective

		requireNotJudgedBeforePlanEffective(t, fixture, command, handleOK(t, fixture, command))
	})
}

// Covers: AT-NR-105（编排半边）——同样早于计划生效，判出仍适用不撞：交进的节点就在计划上、没有够格的改善，照常记「仍适用」、
// 引用这份计划，不入账为未判。改善够不上阈值（改善 50，阈值 1000）同属仍适用。「触发早于计划生效」只拦离场与新计划生效这两件
// 会撞门的事，不拦一次不动计划的复核。阈值是本测试交入的租户取值。
func TestATriggerEarlierThanThePlansEffectiveTimeThatKeepsThePlanIsStillJudged(t *testing.T) {
	t.Run("节点在计划上、无改善", func(t *testing.T) {
		fixture := planNotYetEffectiveFixture(t)
		command := reassessCommandAt(t, "trigger-between", "node-origin", triggerBeforePlanEffective)

		result := handleOK(t, fixture, command)
		if result.Outcome() != application.ReassessedStillApplicable {
			t.Fatalf("复核 = %q/%q, want STILL_APPLICABLE", result.Outcome(), result.NotJudgedReason())
		}
		if kept, _ := result.Record(); kept.ReviewedPlan.String() != "plan-1/v1" || !kept.TriggerOccurredAt.Equal(triggerBeforePlanEffective) {
			t.Fatalf("复核记录 = %+v, want 引用 plan-1/v1、触发业务时间 %s", kept, triggerBeforePlanEffective)
		}
		if len(fixture.store.notJudged) != 0 || fixture.store.saved != 1 {
			t.Fatalf("未判 %d 行、复核 %d 条, want 0 与 1", len(fixture.store.notJudged), fixture.store.saved)
		}
	})

	t.Run("改善够不上阈值", func(t *testing.T) {
		fixture := planNotYetEffectiveFixture(t)
		evidence := declareCostImprovement(cheaperAlternative(t, 950), 1000)
		limit := 0
		evidence.FreezeForm = domain.RemainingSegmentCountFreeze
		evidence.FreezeRemainingSegmentLimit = &limit
		fixture.evidence.byParcel["parcel-1"] = evidence
		command := reassessCommandAt(t, "trigger-between", "node-origin", triggerBeforePlanEffective)

		result := handleOK(t, fixture, command)
		if result.Outcome() != application.ReassessedStillApplicable {
			t.Fatalf("复核 = %q/%q, want STILL_APPLICABLE", result.Outcome(), result.NotJudgedReason())
		}
		if len(fixture.store.notJudged) != 0 {
			t.Fatalf("未判 %d 行, want 0", len(fixture.store.notJudged))
		}
	})
}

// Covers: AT-NR-105（编排半边，下限的边）——触发不早于计划生效照判：恰在生效的那一刻，离场与生效同一刻，离场门放行，失效落在
// 生效时刻；晚于生效更是。
func TestATriggerNotEarlierThanThePlansEffectiveTimeIsJudgedAsUsual(t *testing.T) {
	for name, at := range map[string]time.Time{
		"同一时刻": planEffectiveBetween,
		"晚于生效": planEffectiveBetween.Add(time.Minute),
	} {
		t.Run(name, func(t *testing.T) {
			fixture := planNotYetEffectiveFixture(t)
			fixture.evidence.byParcel["parcel-1"] = routableEvidence(t)
			command := reassessCommandAt(t, "trigger-at", "node-actual-elsewhere", at)

			result := handleOK(t, fixture, command)
			if result.Outcome() != application.ReassessedPlanLapsed {
				t.Fatalf("复核 = %q/%q, want PLAN_LAPSED", result.Outcome(), result.NotJudgedReason())
			}
			lapsed := fixture.applicability.byPlan[value(t, domain.NewRoutePlanVersionID, "plan-1/v1")]
			if lapsed.State() != domain.PlanLapsed || !lapsed.TransitionedAt().Equal(at) {
				t.Fatalf("原计划适用性 = %s @ %s, want LAPSED @ 触发业务时间 %s", lapsed.State(), lapsed.TransitionedAt(), at)
			}
			if len(fixture.store.notJudged) != 0 {
				t.Fatalf("未判 %d 行, want 0", len(fixture.store.notJudged))
			}
		})
	}
}

// Covers: AT-NR-104（编排半边，次序）——先比当前位置，再比计划生效：包裹已在计划生效之后的位置判过，一次更早的触发后到，它既
// 早于当前位置也早于计划生效，原因记「触发早于当前位置」，不记早于计划生效——当前位置是路由历史里的事实，先于计划比。
func TestATriggerEarlierThanTheCurrentPositionIsComparedBeforeThePlanEffectiveTime(t *testing.T) {
	fixture := planNotYetEffectiveFixture(t)
	later := reassessCommandAt(t, "trigger-later", "node-origin", planEffectiveBetween.Add(time.Hour))
	if kept := handleOK(t, fixture, later); kept.Outcome() != application.ReassessedStillApplicable {
		t.Fatalf("较晚那次 = %q/%q, want STILL_APPLICABLE", kept.Outcome(), kept.NotJudgedReason())
	}

	command := reassessCommandAt(t, "trigger-earlier", "node-actual-elsewhere", triggerBeforePlanEffective)
	result := handleOK(t, fixture, command)
	if result.Outcome() != application.ReassessTriggerNotJudged ||
		result.NotJudgedReason() != ports.NotJudgedTriggerPrecedesCurrentPosition {
		t.Fatalf("较早那次 = %q/%q, want TRIGGER_NOT_JUDGED/TRIGGER_PRECEDES_CURRENT_POSITION",
			result.Outcome(), result.NotJudgedReason())
	}
	if noted := fixture.store.notJudged[command.Trigger.Correlation]; !noted.PlanEffectiveAt.IsZero() {
		t.Fatalf("未判登记 = %+v, want 不带计划生效时刻", noted)
	}
}

// Covers: AT-NR-104（编排半边，决定形成的计划）——票面时序：原计划在 T1 失效、形成改路建议，授权角色在 T3 据它决定，新计划从 T3
// 起生效；业务时间 T2（T1 < T2 < T3）的触发在决定之后才处理，起点是从 T3 才生效的新计划，判出失效：入账为「触发早于计划
// 生效」，生效时刻点名决定的时刻；决定形成的计划仍当前有效，适用性不动。随后一次不早于 T3 的触发从决定形成的计划起照判。
func TestATriggerBetweenASuggestionAndItsDecisionIsNotJudgedAgainstTheDecidedPlan(t *testing.T) {
	fixture := newDecideFixture(t)
	decided := fixture.decideOK(t, fixture.command(t, "candidate-2"))
	recorded, _ := decided.Decision()
	savedBefore, appliedBefore := fixture.store.saved, len(fixture.applicability.saved)

	fixture.evidence.byParcel["parcel-1"] = routableEvidence(t)
	command := reassessCommandAt(t, "trigger-between", "node-actual-elsewhere", reassessedAt.Add(time.Hour))
	result := handleOK(t, fixture.reassessFixture, command)
	if result.Outcome() != application.ReassessTriggerNotJudged ||
		result.NotJudgedReason() != ports.NotJudgedTriggerPrecedesPlanEffectiveFrom {
		t.Fatalf("复核 = %q/%q（未决 %q）, want TRIGGER_NOT_JUDGED/TRIGGER_PRECEDES_PLAN_EFFECTIVE_FROM",
			result.Outcome(), result.NotJudgedReason(), result.UndecidedReason())
	}
	if noted := fixture.store.notJudged[command.Trigger.Correlation]; !noted.PlanEffectiveAt.Equal(decidedAt) ||
		!noted.PlanEffectiveAt.Equal(recorded.Decision.NewPlan().EffectiveFrom()) {
		t.Fatalf("未判登记 = %+v, want 生效时刻取决定的时刻 %s", noted, decidedAt)
	}
	if fixture.store.saved != savedBefore || len(fixture.applicability.saved) != appliedBefore {
		t.Fatalf("复核多 %d 条、适用性多写 %d 次, want 0 与 0", fixture.store.saved-savedBefore, len(fixture.applicability.saved)-appliedBefore)
	}

	next := reassessCommandAt(t, "trigger-after", "node-actual-elsewhere", decidedAt.Add(time.Hour))
	after := handleOK(t, fixture.reassessFixture, next)
	if after.Outcome() != application.ReassessedPlanLapsed {
		t.Fatalf("决定之后那次 = %q/%q, want PLAN_LAPSED——下一次控制触发从决定形成的计划的位置判", after.Outcome(), after.NotJudgedReason())
	}
	if lapsed, _ := after.Record(); lapsed.ReviewedPlan != recorded.Decision.NewPlan().Version() {
		t.Fatalf("复核的计划 = %s, want 决定形成的 %s", lapsed.ReviewedPlan, recorded.Decision.NewPlan().Version())
	}
}
