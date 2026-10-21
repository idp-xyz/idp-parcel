package application_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/application"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
)

// 本文件钉初始路由计划的生效时刻（CONTEXT「路由计划适用性」）：从委托接受的时刻起生效，判断时刻另记处理时钟。路由交接
// 积压时 NR 晚于接受很久才判；生效若取处理时钟，此后业务时间落在接受与处理之间的控制触发让原计划离场时，离场早于生效，
// 被离场门拒，整封信封响亮失败。

var (
	backlogAcceptedAt  = routeJudgedAt
	backlogProcessedAt = backlogAcceptedAt.Add(10 * time.Hour)
	backlogTriggeredAt = backlogAcceptedAt.Add(5 * time.Hour)
)

// initialRouteUnderBacklog 让初始路由编排在接受十小时之后才处理交接，计划落进复核夹具的路由库，好让复核从这份真经编排形成
// 的计划判起。
func initialRouteUnderBacklog(t *testing.T, fixture *reassessFixture) domain.InitialRoutePlan {
	t.Helper()
	eligibility, err := domain.NewNetworkEligibility(domain.NetworkJudgmentRequired, domain.EligibilityBasisReference{})
	if err != nil {
		t.Fatalf("new eligibility: %v", err)
	}
	fixture.evidence.byParcel["parcel-1"] = evidenceBeforeFreeze(t)
	handler := application.NewCreateInitialRouteHandler(application.CreateInitialRouteDeps{
		Applicability: &applicabilityDouble{eligibility: eligibility},
		Evidence:      fixture.evidence,
		Costs:         passthroughCosts{},
		Store:         fixture.routes,
		Log:           newHandoffLog(),
		Downstream:    &routeDownstreamDouble{},
		// 号段与复核夹具的签发错开：两个替身都从 plan-1 签起，改善替代会撞上「新计划与原计划同版本」。
		Identities: &routeIdentityDouble{next: 5},
		Clock:      fixedClock{at: backlogProcessedAt},
	})
	command := routeCommand(t, "parcel-1")
	command.Handoff.AcceptedAt = backlogAcceptedAt
	result, err := handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("初始路由：%v", err)
	}
	parcels := result.Parcels()
	if len(parcels) != 1 || parcels[0].Outcome() != application.ParcelRouteFormed {
		t.Fatalf("初始路由 = %q / %+v, want 一件 ROUTE_FORMED", result.Outcome(), parcels)
	}
	plan, _ := parcels[0].Plan()
	return plan
}

// Covers: AT-NR-086（编排半边）——路由交接积压，NR 在接受十小时之后才判初始路由：计划从委托接受的时刻起生效，判断时刻是
// 处理时钟。此后业务时间落在两者之间的控制触发照常判：揽收按硬限制让原计划失效、可控节点上的改善替代，原计划都在触发的
// 业务时间离场，过得了离场门。阈值 0 是本测试交入的租户取值。
func TestUnderAHandoffBacklogTheInitialPlanTakesEffectAtAcceptance(t *testing.T) {
	t.Run("生效与判断", func(t *testing.T) {
		plan := initialRouteUnderBacklog(t, newReassessFixture(t))
		if !plan.EffectiveFrom().Equal(backlogAcceptedAt) || !plan.JudgedAt().Equal(backlogProcessedAt) {
			t.Fatalf("计划生效 %s、判断 %s, want 从接受 %s 起生效、判断于处理时刻 %s",
				plan.EffectiveFrom(), plan.JudgedAt(), backlogAcceptedAt, backlogProcessedAt)
		}
	})

	t.Run("失效", func(t *testing.T) {
		fixture := newReassessFixture(t)
		plan := initialRouteUnderBacklog(t, fixture)
		fixture.evidence.byParcel["parcel-1"] = restrictedSelectedCandidate(t, evidenceBeforeFreeze(t))
		command := pickupCommand(t)
		command.Trigger.OccurredAt = backlogTriggeredAt

		lapsed := handleOK(t, fixture, command)
		if lapsed.Outcome() != application.ReassessedPlanLapsed {
			t.Fatalf("揽收复核 = %q/%q, want PLAN_LAPSED", lapsed.Outcome(), lapsed.UndecidedReason())
		}
		applicability := fixture.applicability.byPlan[plan.Version()]
		if applicability.State() != domain.PlanLapsed || !applicability.TransitionedAt().Equal(backlogTriggeredAt) {
			t.Fatalf("原计划适用性 = %s @ %s, want LAPSED @ 触发业务时间 %s", applicability.State(), applicability.TransitionedAt(), backlogTriggeredAt)
		}
	})

	t.Run("改善替代", func(t *testing.T) {
		fixture := newReassessFixture(t)
		plan := initialRouteUnderBacklog(t, fixture)
		evidence := declareCostImprovement(cheaperAlternative(t, 100), 0)
		limit := 0
		evidence.FreezeForm = domain.RemainingSegmentCountFreeze
		evidence.FreezeRemainingSegmentLimit = &limit
		fixture.evidence.byParcel["parcel-1"] = evidence

		switched := handleOK(t, fixture, reassessCommandAt(t, "trigger-1", "node-origin", backlogTriggeredAt))
		if switched.Outcome() != application.ReassessedRerouted {
			t.Fatalf("节点收寄复核 = %q/%q, want REROUTED", switched.Outcome(), switched.UndecidedReason())
		}
		superseded := fixture.applicability.byPlan[plan.Version()]
		if superseded.State() != domain.PlanSuperseded || !superseded.TransitionedAt().Equal(backlogTriggeredAt) {
			t.Fatalf("原计划适用性 = %s @ %s, want SUPERSEDED @ 触发业务时间 %s", superseded.State(), superseded.TransitionedAt(), backlogTriggeredAt)
		}
	})
}
