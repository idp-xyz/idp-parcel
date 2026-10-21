package application_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/application"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// 本文件钉复核的业务时间下限（ADR-0279）：触发的业务时间早于这一判断键的委托接受时刻时，入账为「触发早于委托接受」、
// 不判。初始路由从接受起生效（CONTEXT「路由计划适用性」）；先收后受理、离线补登或来源时间写早时，触发的业务时间可能早于
// 接受，而信封在初始路由形成之后才处理。这时判出失效或替代，离场早于生效，被离场门拒；原先无当前有效路由的那条线上，又会
// 形成一份早于接受就生效的首个计划。

// beforeAcceptance 是一次业务时间早于委托接受两小时的触发，委托接受取积压夹具的接受时刻。
func beforeAcceptance(command application.ReassessRouteCommand) application.ReassessRouteCommand {
	command.Trigger.AcceptedAt = backlogAcceptedAt
	command.Trigger.OccurredAt = backlogAcceptedAt.Add(-2 * time.Hour)
	return command
}

// requireNotJudgedBeforeAcceptance 断言这次触发入账为「触发早于委托接受」：复核册、适用性册一笔不写，未判登记点名比较所用的
// 委托接受时刻，不带当前位置。
func requireNotJudgedBeforeAcceptance(
	t *testing.T, fixture *reassessFixture, command application.ReassessRouteCommand, result application.ReassessRouteResult,
) {
	t.Helper()
	if result.Outcome() != application.ReassessTriggerNotJudged ||
		result.NotJudgedReason() != ports.NotJudgedTriggerPrecedesShipmentAcceptance {
		t.Fatalf("复核 = %q/%q（未决 %q）, want TRIGGER_NOT_JUDGED/TRIGGER_PRECEDES_SHIPMENT_ACCEPTANCE",
			result.Outcome(), result.NotJudgedReason(), result.UndecidedReason())
	}
	if fixture.store.saved != 0 || len(fixture.applicability.saved) != 0 {
		t.Fatalf("复核 %d 条、适用性写 %d 次, want 0 与 0——早于接受的触发不判", fixture.store.saved, len(fixture.applicability.saved))
	}
	noted, ok := fixture.store.notJudged[command.Trigger.Correlation]
	if !ok || noted.Reason != ports.NotJudgedTriggerPrecedesShipmentAcceptance ||
		!noted.ShipmentAcceptedAt.Equal(command.Trigger.AcceptedAt) ||
		!noted.TriggerOccurredAt.Equal(command.Trigger.OccurredAt) || !noted.CurrentPositionAt.IsZero() {
		t.Fatalf("未判登记 = %+v（在 %v）, want 早于委托接受 %s、不带当前位置", noted, ok, command.Trigger.AcceptedAt)
	}
}

// Covers: AT-NR-087（编排半边）——初始路由从委托接受起生效，一次业务时间早于接受的控制触发在那之后才处理：入账为「触发早于
// 委托接受」、不判。原计划本会因硬限制失效、或被更便宜的候选替代，原先无当前有效路由时本会形成首个计划，这几格都不动；
// 另一封信封带来同一触发同答。阈值 0 是本测试交入的租户取值。
func TestATriggerEarlierThanTheShipmentAcceptanceIsNotJudged(t *testing.T) {
	t.Run("原计划本会失效", func(t *testing.T) {
		fixture := newReassessFixture(t)
		plan := initialRouteUnderBacklog(t, fixture)
		fixture.evidence.byParcel["parcel-1"] = restrictedSelectedCandidate(t, evidenceBeforeFreeze(t))
		command := beforeAcceptance(pickupCommand(t))

		requireNotJudgedBeforeAcceptance(t, fixture, command, handleOK(t, fixture, command))
		if again := handleOK(t, fixture, command); again.Outcome() != application.ReassessTriggerNotJudged ||
			again.NotJudgedReason() != ports.NotJudgedTriggerPrecedesShipmentAcceptance {
			t.Fatalf("同一触发再来 = %q/%q, want 同答", again.Outcome(), again.NotJudgedReason())
		}
		if _, touched := fixture.applicability.byPlan[plan.Version()]; touched {
			t.Fatalf("原计划适用性被写了，want 不动")
		}
	})

	t.Run("原计划本会被替代", func(t *testing.T) {
		fixture := newReassessFixture(t)
		initialRouteUnderBacklog(t, fixture)
		evidence := declareCostImprovement(cheaperAlternative(t, 100), 0)
		limit := 0
		evidence.FreezeForm = domain.RemainingSegmentCountFreeze
		evidence.FreezeRemainingSegmentLimit = &limit
		fixture.evidence.byParcel["parcel-1"] = evidence
		command := beforeAcceptance(reassessCommandAt(t, "trigger-early", "node-origin", reassessedAt))

		requireNotJudgedBeforeAcceptance(t, fixture, command, handleOK(t, fixture, command))
	})

	t.Run("原先无当前有效路由，不形成首个计划", func(t *testing.T) {
		fixture := newReassessFixture(t)
		noRoute, err := domain.FormNoCurrentRouteJudgment(domain.NoCurrentRouteJudgmentSpec{
			Key:          reassessKey(t),
			Candidates:   []domain.RouteCandidate{eliminated(t, "candidate-1", "PATH_NOT_EXECUTABLE/SCHED-V3")},
			Strategy:     value(t, domain.NewRouteStrategyReference, "strategy-1/v1"),
			ViewRevision: value(t, domain.NewNetworkViewRevision, "net-view-rev-1"),
			JudgedAt:     backlogProcessedAt,
		})
		if err != nil {
			t.Fatalf("form prior no-route: %v", err)
		}
		fixture.routes.records[reassessKey(t)] = ports.InitialRouteRecord{Key: reassessKey(t), NoRoute: noRoute, HasNoRoute: true}
		fixture.evidence.byParcel["parcel-1"] = routableEvidence(t)
		command := beforeAcceptance(reassessCommand(t, "node-origin"))

		requireNotJudgedBeforeAcceptance(t, fixture, command, handleOK(t, fixture, command))
	})

	t.Run("与委托接受同一时刻照判", func(t *testing.T) {
		fixture := newReassessFixture(t)
		plan := initialRouteUnderBacklog(t, fixture)
		fixture.evidence.byParcel["parcel-1"] = restrictedSelectedCandidate(t, evidenceBeforeFreeze(t))
		command := pickupCommand(t)
		command.Trigger.AcceptedAt = backlogAcceptedAt
		command.Trigger.OccurredAt = backlogAcceptedAt

		if lapsed := handleOK(t, fixture, command); lapsed.Outcome() != application.ReassessedPlanLapsed {
			t.Fatalf("复核 = %q/%q, want PLAN_LAPSED——同一时刻不算更早", lapsed.Outcome(), lapsed.NotJudgedReason())
		}
		if applicability := fixture.applicability.byPlan[plan.Version()]; applicability.State() != domain.PlanLapsed ||
			!applicability.TransitionedAt().Equal(backlogAcceptedAt) {
			t.Fatalf("原计划适用性 = %s @ %s, want LAPSED @ 委托接受 %s", applicability.State(), applicability.TransitionedAt(), backlogAcceptedAt)
		}
	})
}

// Covers: AT-NR-087（编排半边，先比接受）——包裹已在接受之后的位置判过，一次早于委托接受的触发后到：它也早于当前位置，原因
// 记「触发早于委托接受」，不记早于当前位置——接受是这一判断键自己的下限，先于路由历史比。
func TestATriggerEarlierThanTheAcceptanceIsComparedBeforeTheCurrentPosition(t *testing.T) {
	fixture := newReassessFixture(t)
	initialRouteUnderBacklog(t, fixture)
	fixture.evidence.byParcel["parcel-1"] = evidenceBeforeFreeze(t)
	judged := reassessCommandAt(t, "trigger-after", "node-origin", backlogTriggeredAt)
	judged.Trigger.AcceptedAt = backlogAcceptedAt
	if after := handleOK(t, fixture, judged); after.Outcome() != application.ReassessedStillApplicable {
		t.Fatalf("接受之后那次 = %q/%q, want STILL_APPLICABLE", after.Outcome(), after.NotJudgedReason())
	}

	command := beforeAcceptance(reassessCommandAt(t, "trigger-before", "node-origin", reassessedAt))
	result := handleOK(t, fixture, command)
	if result.Outcome() != application.ReassessTriggerNotJudged ||
		result.NotJudgedReason() != ports.NotJudgedTriggerPrecedesShipmentAcceptance {
		t.Fatalf("早于接受那次 = %q/%q, want TRIGGER_NOT_JUDGED/TRIGGER_PRECEDES_SHIPMENT_ACCEPTANCE",
			result.Outcome(), result.NotJudgedReason())
	}
	if noted := fixture.store.notJudged[command.Trigger.Correlation]; !noted.CurrentPositionAt.IsZero() ||
		!noted.ShipmentAcceptedAt.Equal(backlogAcceptedAt) {
		t.Fatalf("未判登记 = %+v, want 只带委托接受时刻", noted)
	}
}
