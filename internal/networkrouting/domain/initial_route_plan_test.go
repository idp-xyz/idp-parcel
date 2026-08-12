package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
)

var (
	routeJudgedAt  = time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC)
	windowEarliest = time.Date(2026, 8, 9, 8, 0, 0, 0, time.UTC)
	windowLatest   = time.Date(2026, 8, 9, 20, 0, 0, 0, time.UTC)
)

func judgmentKeyFor(t *testing.T, parcel string) domain.InitialRouteJudgmentKey {
	t.Helper()
	return domain.InitialRouteJudgmentKey{
		TenantID:           mustValue(t, domain.NewTenantID, "tenant-1"),
		CustomerAccountID:  mustValue(t, domain.NewCustomerAccountID, "customer-1"),
		ShipmentRequestID:  mustValue(t, domain.NewShipmentRequestID, "request-1"),
		AcceptanceBaseline: mustValue(t, domain.NewAcceptanceBaselineReference, "baseline-1/v1"),
		DeclaredParcelID:   mustValue(t, domain.NewDeclaredParcelID, parcel),
		ServicePurpose:     mustValue(t, domain.NewServicePurpose, "NETWORK_SERVICE"),
	}
}

func plannedWindow(t *testing.T) domain.PlannedTimeWindow {
	t.Helper()
	window, err := domain.NewPlannedTimeWindow(windowEarliest, windowLatest,
		mustValue(t, domain.NewWindowBasisReference, "CALENDAR-V1/BUFFER-V1"))
	if err != nil {
		t.Fatalf("new planned time window: %v", err)
	}
	return window
}

func plannedLeg(t *testing.T, from, to string, opaque bool) domain.PlannedLeg {
	t.Helper()
	leg, err := domain.NewPlannedLeg(domain.PlannedLegSpec{
		From:        mustValue(t, domain.NewPlanNodeReference, from),
		To:          mustValue(t, domain.NewPlanNodeReference, to),
		Responsible: mustValue(t, domain.NewResponsiblePartyReference, "party-1"),
		Window:      plannedWindow(t),
		Opaque:      opaque,
	})
	if err != nil {
		t.Fatalf("new planned leg %s->%s: %v", from, to, err)
	}
	return leg
}

func qualifiedRouteCandidate(t *testing.T, id string) domain.RouteCandidate {
	t.Helper()
	built, err := domain.NewRouteCandidate(
		mustValue(t, domain.NewCandidateID, id),
		domain.CandidateQualified,
		domain.CandidateReason{},
	)
	if err != nil {
		t.Fatalf("new qualified candidate: %v", err)
	}
	return built
}

func eliminatedCandidate(t *testing.T, id, reason string) domain.RouteCandidate {
	t.Helper()
	candidate, err := domain.NewRouteCandidate(
		mustValue(t, domain.NewCandidateID, id),
		domain.CandidateEliminated,
		mustValue(t, domain.NewCandidateReason, reason),
	)
	if err != nil {
		t.Fatalf("new eliminated candidate: %v", err)
	}
	return candidate
}

func planSpec(t *testing.T) domain.InitialRoutePlanSpec {
	t.Helper()
	return domain.InitialRoutePlanSpec{
		Key:      judgmentKeyFor(t, "parcel-1"),
		Version:  mustValue(t, domain.NewRoutePlanVersionID, "plan-1/v1"),
		Selected: mustValue(t, domain.NewCandidateID, "cand-1"),
		Candidates: []domain.RouteCandidate{
			qualifiedRouteCandidate(t, "cand-1"),
			eliminatedCandidate(t, "cand-2", "HARD_CONSTRAINT_RESTRICTION/CUSTOMS-1"),
		},
		Legs: []domain.PlannedLeg{
			plannedLeg(t, "node-origin", "node-hub", false),
			plannedLeg(t, "node-hub", "node-destination", false),
		},
		Strategy:      mustValue(t, domain.NewRouteStrategyReference, "strategy-1/v1"),
		ViewRevision:  mustValue(t, domain.NewNetworkViewRevision, "net-view-rev-1"),
		JudgedAt:      routeJudgedAt,
		EffectiveFrom: routeJudgedAt,
	}
}

// Covers: `AT-NR-001`「只形成一个当前有效计划并保留所有候选依据」的对象半边与 CONTEXT
// 「路由计划……包含计划节点、计划履约段和计划时间窗口」——被选候选必须是在册合格成员，
// 节点序列由连续段链推导，落选候选连同淘汰依据原样在册。
func TestAnInitialRoutePlanCarriesItsFullSelectionBasis(t *testing.T) {
	plan, err := domain.FormInitialRoutePlan(planSpec(t))
	if err != nil {
		t.Fatalf("form initial route plan: %v", err)
	}

	if plan.SelectedCandidate().String() != "cand-1" {
		t.Fatalf("selected = %s", plan.SelectedCandidate())
	}
	nodes := plan.Nodes()
	if len(nodes) != 3 || nodes[0].String() != "node-origin" || nodes[2].String() != "node-destination" {
		t.Fatalf("nodes = %v; 节点序列必须由段链推导", nodes)
	}
	kept := plan.Candidates()
	if len(kept) != 2 || kept[1].Reason().String() != "HARD_CONSTRAINT_RESTRICTION/CUSTOMS-1" {
		t.Fatalf("candidates = %#v; 落选依据没有随计划保全", kept)
	}
	if plan.Legs()[0].Window().Basis().String() != "CALENDAR-V1/BUFFER-V1" {
		t.Fatal("时间窗口没带形成依据——日历与缓冲版本审计要它")
	}
}

// Covers: 选择依据链的构造期防线——选中不在册、选中已被淘汰、段链断裂、窗口倒挂，任一
// 都立不起一个计划；`AT-NR-014` 的外部不透明段用已知交接点表达，不需要内部节点。
func TestAPlanRefusesABrokenSelectionOrChain(t *testing.T) {
	t.Run("selected candidate not in the evaluated space", func(t *testing.T) {
		spec := planSpec(t)
		spec.Selected = mustValue(t, domain.NewCandidateID, "cand-9")
		if _, err := domain.FormInitialRoutePlan(spec); !errors.Is(err, domain.ErrSelectedCandidateNotFound) {
			t.Fatalf("err = %v, want ErrSelectedCandidateNotFound", err)
		}
	})

	t.Run("selected candidate was eliminated", func(t *testing.T) {
		spec := planSpec(t)
		spec.Selected = mustValue(t, domain.NewCandidateID, "cand-2")
		if _, err := domain.FormInitialRoutePlan(spec); !errors.Is(err, domain.ErrSelectedCandidateNotFound) {
			t.Fatalf("err = %v; 选中一个已被淘汰的候选，依据链当场断裂", err)
		}
	})

	t.Run("a broken leg chain", func(t *testing.T) {
		spec := planSpec(t)
		spec.Legs = []domain.PlannedLeg{
			plannedLeg(t, "node-origin", "node-hub", false),
			plannedLeg(t, "node-elsewhere", "node-destination", false),
		}
		if _, err := domain.FormInitialRoutePlan(spec); !errors.Is(err, domain.ErrInvalidRoutePlan) {
			t.Fatalf("err = %v; 断链的路径不是一条路径", err)
		}
	})

	t.Run("an inverted window is not a range", func(t *testing.T) {
		if _, err := domain.NewPlannedTimeWindow(windowLatest, windowEarliest,
			mustValue(t, domain.NewWindowBasisReference, "CALENDAR-V1")); !errors.Is(err, domain.ErrInvalidRoutePlan) {
			t.Fatalf("err = %v, want ErrInvalidRoutePlan", err)
		}
	})

	t.Run("an opaque leg expresses only known handover points", func(t *testing.T) {
		spec := planSpec(t)
		spec.Legs = []domain.PlannedLeg{
			plannedLeg(t, "node-origin", "node-partner-handover", false),
			plannedLeg(t, "node-partner-handover", "node-return-control", true),
		}
		plan, err := domain.FormInitialRoutePlan(spec)
		if err != nil {
			t.Fatalf("form plan with opaque leg: %v", err)
		}
		legs := plan.Legs()
		if !legs[1].Opaque() || legs[0].Opaque() {
			t.Fatal("不透明标记没有随段保全")
		}
	})
}

// Covers: `AT-NR-005`「权威输入完整，所有候选均被有效硬限制淘汰 → 形成无当前有效路由并
// 保存逐候选淘汰依据」与一致性硬句「必须证明不存在尚未评估、证据未知或版本失配的可能
// 候选」——带证据未知候选的空间立不起无路由，空候选空间同样拒绝。
func TestNoCurrentRouteDemandsAClosedFullyEliminatedSpace(t *testing.T) {
	spec := domain.NoCurrentRouteJudgmentSpec{
		Key: judgmentKeyFor(t, "parcel-1"),
		Candidates: []domain.RouteCandidate{
			eliminatedCandidate(t, "cand-1", "HARD_CONSTRAINT_RESTRICTION/CUSTOMS-1"),
			eliminatedCandidate(t, "cand-2", "PATH_NOT_EXECUTABLE/SCHED-V3"),
		},
		Strategy:     mustValue(t, domain.NewRouteStrategyReference, "strategy-1/v1"),
		ViewRevision: mustValue(t, domain.NewNetworkViewRevision, "net-view-rev-1"),
		JudgedAt:     routeJudgedAt,
	}
	judgment, err := domain.FormNoCurrentRouteJudgment(spec)
	if err != nil {
		t.Fatalf("form no-current-route judgment: %v", err)
	}
	if len(judgment.Candidates()) != 2 {
		t.Fatal("逐候选淘汰依据没有随判断保全")
	}

	undecided := spec
	undecided.Candidates = append(undecided.Candidates, unknownCandidate(t, "cand-3"))
	if _, err := domain.FormNoCurrentRouteJudgment(undecided); !errors.Is(err, domain.ErrCandidateSpaceUndecided) {
		t.Fatalf("err = %v; 证据未知的候选在场，结论只能是未决不是无路由", err)
	}

	empty := spec
	empty.Candidates = nil
	if _, err := domain.FormNoCurrentRouteJudgment(empty); !errors.Is(err, domain.ErrCandidateSpaceNotEstablished) {
		t.Fatalf("err = %v; 空候选空间分不清是排除还是装配失败", err)
	}
}

func unknownCandidate(t *testing.T, id string) domain.RouteCandidate {
	t.Helper()
	candidate, err := domain.NewRouteCandidate(
		mustValue(t, domain.NewCandidateID, id),
		domain.CandidateEvidenceUnknown,
		mustValue(t, domain.NewCandidateReason, "CUSTOMS_ELIGIBILITY_UNKNOWN"),
	)
	if err != nil {
		t.Fatalf("new unknown candidate: %v", err)
	}
	return candidate
}
