package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
)

var planEffectiveAt = time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)

func currentPlan(t *testing.T) domain.PlanApplicability {
	t.Helper()
	applicability, err := domain.EstablishPlanApplicability(
		mustValue(t, domain.NewRoutePlanVersionID, "plan-1/v1"), planEffectiveAt)
	if err != nil {
		t.Fatalf("establish applicability: %v", err)
	}
	return applicability
}

// Covers: CONTEXT 路由计划适用性生命周期——「当前有效 → 已被替代：新路由版本已经生效；
// 旧计划、已执行前缀和选择依据继续保留」与「当前有效 → 已失效：原计划已经不可执行」。
// 三条离场各带依据与时刻，`已被替代`还要指名接班版本且不得自代。
func TestAPlanLeavesCurrentWithItsBasisAndSuccessor(t *testing.T) {
	basis := mustValue(t, domain.NewApplicabilityBasisReference, "LINE-CLOSED/NET-ADJ-7")

	superseded, err := currentPlan(t).Supersede(
		mustValue(t, domain.NewRoutePlanVersionID, "plan-1/v2"), basis, planEffectiveAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("supersede: %v", err)
	}
	if superseded.State() != domain.PlanSuperseded {
		t.Fatalf("state = %q, want SUPERSEDED", superseded.State())
	}
	successor, present := superseded.Successor()
	if !present || successor.String() != "plan-1/v2" {
		t.Fatalf("successor = %s present = %v", successor, present)
	}

	lapsed, err := currentPlan(t).Lapse(basis, planEffectiveAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("lapse: %v", err)
	}
	if lapsed.State() != domain.PlanLapsed {
		t.Fatalf("state = %q, want LAPSED", lapsed.State())
	}
	kept, present := lapsed.Basis()
	if !present || kept != basis {
		t.Fatalf("basis = %v present = %v; 失效不带依据与拍脑袋下线分不开", kept, present)
	}

	if _, err := currentPlan(t).Supersede(
		mustValue(t, domain.NewRoutePlanVersionID, "plan-1/v1"), basis, planEffectiveAt.Add(time.Hour)); !errors.Is(err, domain.ErrInvalidPlanApplicability) {
		t.Fatalf("err = %v; 自代不是接班", err)
	}
}

// Covers: 生命周期硬句「已失效或已被替代的计划不得原地恢复为当前有效；原路径后来重新
// 可用时，也必须重新评估并形成新的计划版本」——离场后任何再转移都拒绝，第一次离场的
// 依据不被改写。
func TestALapsedOrSupersededPlanNeverComesBack(t *testing.T) {
	basis := mustValue(t, domain.NewApplicabilityBasisReference, "LINE-CLOSED/NET-ADJ-7")
	later := planEffectiveAt.Add(2 * time.Hour)

	lapsed, err := currentPlan(t).Lapse(basis, planEffectiveAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("lapse: %v", err)
	}
	if _, err := lapsed.Supersede(
		mustValue(t, domain.NewRoutePlanVersionID, "plan-1/v2"), basis, later); !errors.Is(err, domain.ErrPlanNoLongerCurrent) {
		t.Fatalf("err = %v; 已失效的计划被接了班——第一次离场的依据被改写", err)
	}
	if _, err := lapsed.Lapse(basis, later); !errors.Is(err, domain.ErrPlanNoLongerCurrent) {
		t.Fatalf("err = %v; 二次失效改写了第一次的依据", err)
	}
	if _, err := lapsed.Conclude(basis, later); !errors.Is(err, domain.ErrPlanNoLongerCurrent) {
		t.Fatalf("err = %v", err)
	}

	superseded, err := currentPlan(t).Supersede(
		mustValue(t, domain.NewRoutePlanVersionID, "plan-1/v2"), basis, planEffectiveAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("supersede: %v", err)
	}
	if _, err := superseded.Lapse(basis, later); !errors.Is(err, domain.ErrPlanNoLongerCurrent) {
		t.Fatalf("err = %v; 已被替代的计划又失效了一次", err)
	}
}

// Covers: 离场防线——依据缺失、时刻缺失或早于生效时刻都立不成转移；生效本身必须带时刻。
func TestApplicabilityTransitionsDemandBasisAndOrderedTime(t *testing.T) {
	basis := mustValue(t, domain.NewApplicabilityBasisReference, "LINE-CLOSED/NET-ADJ-7")

	if _, err := currentPlan(t).Lapse(domain.ApplicabilityBasisReference{}, planEffectiveAt.Add(time.Hour)); !errors.Is(err, domain.ErrInvalidPlanApplicability) {
		t.Fatalf("err = %v, want ErrInvalidPlanApplicability", err)
	}
	if _, err := currentPlan(t).Lapse(basis, planEffectiveAt.Add(-time.Hour)); !errors.Is(err, domain.ErrInvalidPlanApplicability) {
		t.Fatalf("err = %v; 早于生效的失效倒写了历史", err)
	}
	if _, err := domain.EstablishPlanApplicability(domain.RoutePlanVersionID{}, planEffectiveAt); !errors.Is(err, domain.ErrInvalidPlanApplicability) {
		t.Fatalf("err = %v", err)
	}
}
