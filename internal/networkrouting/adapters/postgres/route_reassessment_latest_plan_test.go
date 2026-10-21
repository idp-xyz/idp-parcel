package postgres_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// 本文件对真实 PostgreSQL 16 证复核判断起点的库面（票 docs-consistency/code/52）：复核册里最近一次形成的新计划按判断键、
// 按触发业务时间取，不带新计划的复核与别的判断键都不掺进来；读回的是整份计划。

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

// Covers: 复核判断起点（库面）——同一判断键有两次形成计划的复核与一次更晚、不带新计划的复核：取带新计划里业务时间最晚的
// 那一份；它晚于落库顺序在先的那一份，与落库先后无关。别的包裹更晚形成的计划不掺进来；没有新计划的键答没有。读回的
// 计划段链完整。
func TestTheLatestNewPlanIsReadPerJudgmentKeyByBusinessTime(t *testing.T) {
	_, reassessments, transactor, _ := newRouteStores(t)
	ctx := t.Context()
	key := routeKey(t, "tenant-1", "parcel-1")
	other := routeKey(t, "tenant-1", "parcel-2")
	early, late := notJudgedTriggerAt.Add(-2*time.Hour), notJudgedTriggerAt

	for _, record := range []ports.ReassessmentRecord{
		firstPlanFormedAt(t, key, "trigger-n-late", "RPV-LATE", late),
		firstPlanFormedAt(t, key, "trigger-n-early", "RPV-EARLY", early),
		stillApplicableAt(t, key, "trigger-n-latest", late.Add(time.Hour)),
		firstPlanFormedAt(t, other, "trigger-n-other", "RPV-OTHER", late.Add(24*time.Hour)),
	} {
		saveReassessment(t, transactor, ctx, reassessments, record.Correlation, record)
	}

	within(t, transactor, ctx, func(txCtx context.Context) error {
		plan, found, err := reassessments.LatestNewPlan(txCtx, key)
		if err != nil || !found {
			t.Fatalf("最近一份新计划：found %v err %v", found, err)
		}
		if plan.Version().String() != "RPV-LATE" || len(plan.Legs()) != 2 || plan.Key() != key {
			t.Fatalf("最近一份新计划 = %s（%d 段）, want RPV-LATE、两段、本判断键", plan.Version(), len(plan.Legs()))
		}
		if _, found, err := reassessments.LatestNewPlan(txCtx, routeKey(t, "tenant-1", "parcel-none")); err != nil || found {
			t.Fatalf("没有复核的键答出了新计划（err %v）", err)
		}
		if _, found, err := reassessments.LatestNewPlan(txCtx, routeKey(t, "tenant-2", "parcel-1")); err != nil || found {
			t.Fatalf("另一个租户读到了本租户的计划（err %v）", err)
		}
		return nil
	})
}
