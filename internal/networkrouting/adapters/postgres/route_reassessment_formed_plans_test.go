package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// 本文件对真实 PostgreSQL 16 证复核判断起点的库面（票 docs-consistency/code/52）：复核册里形成过的新计划按判断键全部
// 读回，不带新计划的复核、别的判断键与别的租户都不掺进来；迁移 0022 之前没有业务时间的行也在其中；读回的是整份计划，
// 重建不出来的计划交 ErrStoredRouteRecordUnreadable。

func firstPlanFormedAt(
	t *testing.T, key domain.InitialRouteJudgmentKey, correlation, version string, at time.Time,
) ports.ReassessmentRecord {
	t.Helper()
	return ports.ReassessmentRecord{
		Correlation:       scalar(t, domain.NewRequestCorrelationID, correlation),
		Key:               key,
		Conclusion:        ports.ReassessmentFirstPlanFormed,
		CandidateState:    ports.CandidatesAvailable,
		NewPlan:           formedPlan(t, key, version),
		HasNewPlan:        true,
		TriggerOccurredAt: at,
		ReassessedAt:      reassessedAtTS,
	}
}

func formedVersions(t *testing.T, ctx context.Context, reassessments interface {
	FormedPlans(context.Context, domain.InitialRouteJudgmentKey) ([]domain.InitialRoutePlan, error)
}, key domain.InitialRouteJudgmentKey) []string {
	t.Helper()
	plans, err := reassessments.FormedPlans(ctx, key)
	if err != nil {
		t.Fatalf("读形成过的新计划：%v", err)
	}
	versions := make([]string, 0, len(plans))
	for _, plan := range plans {
		if plan.Key() != key || len(plan.Legs()) != 2 {
			t.Fatalf("计划 %s 的键或段链没读全：%+v", plan.Version(), plan.Key())
		}
		versions = append(versions, plan.Version().String())
	}
	return versions
}

// Covers: 复核判断起点（库面）——同一判断键两次形成计划的复核按业务时间读回，与落库先后无关；迁移 0022 之前没有业务时间
// 的那一行也读回、排在最前（NULLS FIRST）；不带新计划的复核、别的包裹、别的租户都不掺进来。
func TestFormedPlansAreReadPerJudgmentKey(t *testing.T) {
	_, reassessments, transactor, pool := newRouteStores(t)
	ctx := t.Context()
	key := routeKey(t, "tenant-1", "parcel-1")
	other := routeKey(t, "tenant-1", "parcel-2")
	early, late := notJudgedTriggerAt.Add(-2*time.Hour), notJudgedTriggerAt

	for _, record := range []ports.ReassessmentRecord{
		firstPlanFormedAt(t, key, "trigger-n-late", "RPV-LATE", late),
		firstPlanFormedAt(t, key, "trigger-n-early", "RPV-EARLY", early),
		firstPlanFormedAt(t, key, "trigger-n-legacy", "RPV-LEGACY", late.Add(time.Hour)),
		stillApplicableAt(t, key, "trigger-n-kept", late.Add(2*time.Hour)),
		firstPlanFormedAt(t, other, "trigger-n-other", "RPV-OTHER", late.Add(24*time.Hour)),
	} {
		saveReassessment(t, transactor, ctx, reassessments, record.Correlation, record)
	}
	// 演迁移 0022 之前落的一行：那时没有业务时间列可填。CHECK 只拦新行，这里在本用例独占的库上拆掉它再抹去业务时间。
	if _, err := pool.Exec(ctx, `ALTER TABLE network_routing.route_reassessment
		DROP CONSTRAINT route_reassessment_trigger_occurred_at_present`); err != nil {
		t.Fatalf("拆业务时间约束：%v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE network_routing.route_reassessment SET trigger_occurred_at = NULL
		WHERE tenant_id = 'tenant-1' AND correlation_id = 'trigger-n-legacy'`); err != nil {
		t.Fatalf("抹业务时间：%v", err)
	}

	within(t, transactor, ctx, func(txCtx context.Context) error {
		got := formedVersions(t, txCtx, reassessments, key)
		want := []string{"RPV-LEGACY", "RPV-EARLY", "RPV-LATE"}
		if len(got) != len(want) {
			t.Fatalf("形成过的新计划 = %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("形成过的新计划 = %v, want %v", got, want)
			}
		}
		if got := formedVersions(t, txCtx, reassessments, routeKey(t, "tenant-1", "parcel-none")); len(got) != 0 {
			t.Fatalf("没有复核的键答出了 %v", got)
		}
		if got := formedVersions(t, txCtx, reassessments, routeKey(t, "tenant-2", "parcel-1")); len(got) != 0 {
			t.Fatalf("另一个租户读到了 %v", got)
		}
		return nil
	})
}

// Covers: 在册计划重建不出来是坏数据，交 ErrStoredRouteRecordUnreadable，好让编排响亮报错，不折成「路由历史读不回」那一格
// 未决、回滚重投。
func TestAnUnrebuildableFormedPlanIsReportedAsSuch(t *testing.T) {
	_, reassessments, transactor, pool := newRouteStores(t)
	ctx := t.Context()
	key := routeKey(t, "tenant-1", "parcel-1")
	record := firstPlanFormedAt(t, key, "trigger-n-bad", "RPV-BAD", notJudgedTriggerAt)
	saveReassessment(t, transactor, ctx, reassessments, record.Correlation, record)
	if _, err := pool.Exec(ctx, `UPDATE network_routing.route_reassessment SET new_plan = '{}'::jsonb
		WHERE tenant_id = 'tenant-1' AND correlation_id = 'trigger-n-bad'`); err != nil {
		t.Fatalf("改坏计划：%v", err)
	}
	within(t, transactor, ctx, func(txCtx context.Context) error {
		if _, err := reassessments.FormedPlans(txCtx, key); !errors.Is(err, ports.ErrStoredRouteRecordUnreadable) {
			t.Fatalf("err = %v, want ErrStoredRouteRecordUnreadable", err)
		}
		return nil
	})
}

// Covers: 路由历史另两处的库面——初始路由那一行的计划重建不出来、计划适用性那一行重建不出来，也交
// ErrStoredRouteRecordUnreadable，与读不回分开。适用性那一行有 CHECK 护着封闭态与逐态形状，这里在本用例独占的库上拆掉两条再写坏。
func TestUnrebuildableInitialRouteAndApplicabilityRowsAreReportedAsSuch(t *testing.T) {
	routes, _, transactor, pool := newRouteStores(t)
	ctx := t.Context()
	key := routeKey(t, "tenant-1", "parcel-1")
	saveRoute(t, transactor, ctx, routes, ports.InitialRouteRecord{Key: key, Plan: formedPlan(t, key, "RPV-0001"), HasPlan: true})
	if _, err := pool.Exec(ctx, `UPDATE network_routing.initial_route SET plan = '{}'::jsonb
		WHERE tenant_id = 'tenant-1' AND declared_parcel_id = 'parcel-1'`); err != nil {
		t.Fatalf("改坏初始路由：%v", err)
	}
	if _, _, err := routes.FindByKey(ctx, key); !errors.Is(err, ports.ErrStoredRouteRecordUnreadable) {
		t.Fatalf("初始路由 err = %v, want ErrStoredRouteRecordUnreadable", err)
	}

	applicabilities, _, applicabilityPool := newApplicabilityStore(t)
	if _, err := applicabilityPool.Exec(ctx, `ALTER TABLE network_routing.plan_applicability
		DROP CONSTRAINT plan_applicability_state_closed,
		DROP CONSTRAINT plan_applicability_state_shaped`); err != nil {
		t.Fatalf("拆适用性约束：%v", err)
	}
	if _, err := applicabilityPool.Exec(ctx, `INSERT INTO network_routing.plan_applicability (plan_id, state, transitioned_at, basis)
		VALUES ('RPV-0001', 'WITHDRAWN', now(), 'basis-1')`); err != nil {
		t.Fatalf("写坏适用性：%v", err)
	}
	if _, _, err := applicabilities.FindByPlan(ctx, scalar(t, domain.NewRoutePlanVersionID, "RPV-0001")); !errors.Is(err, ports.ErrStoredRouteRecordUnreadable) {
		t.Fatalf("适用性 err = %v, want ErrStoredRouteRecordUnreadable", err)
	}
}
