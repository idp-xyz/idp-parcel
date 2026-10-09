package postgres_test

import (
	"slices"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// Covers: 初始路由计划连同全候选的关务出处落库（票 routing-first-cut/12，ADR-0148 决定一「与判断
// 一并留痕」）——判断标识与目录版本引用逐候选原样读回，含目录读不到时那种没有版本可引的出处。
// 写得进读不回的出处等于没有留痕，而计划里其余各格照样往返，单看计划头部看不出它丢了。
func TestAPlanKeepsItsCustomsCitationsThroughTheStore(t *testing.T) {
	routes, _, transactor, _ := newRouteStores(t)
	ctx := t.Context()
	key := routeKey(t, "tenant-1", "parcel-1")

	window, err := domain.NewPlannedTimeWindow(routeJudgedAt.Add(2*time.Hour), routeJudgedAt.Add(6*time.Hour),
		scalar(t, domain.NewWindowBasisReference, "calendar/v1"))
	if err != nil {
		t.Fatalf("构造窗口：%v", err)
	}
	leg, err := domain.NewPlannedLeg(domain.PlannedLegSpec{
		From:        scalar(t, domain.NewPlanNodeReference, "hub-a"),
		To:          scalar(t, domain.NewPlanNodeReference, "hub-b"),
		Responsible: scalar(t, domain.NewResponsiblePartyReference, "carrier-1"),
		Window:      window,
	})
	if err != nil {
		t.Fatalf("构造段：%v", err)
	}
	cite := func(candidate, judgment string, versions ...string) domain.CustomsApplicabilityCitation {
		citation, err := domain.NewCustomsApplicabilityCitation(domain.CustomsApplicabilityCitationSpec{
			Candidate: scalar(t, domain.NewCandidateID, candidate), Judgment: judgment, Versions: versions,
		})
		if err != nil {
			t.Fatalf("构造关务出处：%v", err)
		}
		return citation
	}
	want := []domain.CustomsApplicabilityCitation{
		cite("candidate-1", "SYN-JUDGMENT-1",
			"PATH:SYN-PATH-CN-EXPORT-01@2026-01-01T00:00:00Z", "PORT:SYN-PORT-SZX-01@2026-03-01T00:00:00Z"),
		cite("candidate-2", "SYN-JUDGMENT-2"),
	}
	plan, err := domain.FormInitialRoutePlan(domain.InitialRoutePlanSpec{
		Key:              key,
		Version:          scalar(t, domain.NewRoutePlanVersionID, "RPV-0001"),
		Selected:         scalar(t, domain.NewCandidateID, "candidate-1"),
		Candidates:       planCandidates(t),
		Legs:             []domain.PlannedLeg{leg},
		Strategy:         scalar(t, domain.NewRouteStrategyReference, "strategy/v1"),
		ViewRevision:     scalar(t, domain.NewNetworkViewRevision, "netview-42"),
		JudgedAt:         routeJudgedAt,
		EffectiveFrom:    routeJudgedAt,
		CustomsCitations: want,
	})
	if err != nil {
		t.Fatalf("形成计划：%v", err)
	}
	saveRoute(t, transactor, ctx, routes, ports.InitialRouteRecord{Key: key, Plan: plan, HasPlan: true})

	found, present, err := routes.FindByKey(ctx, key)
	if err != nil || !present || !found.HasPlan {
		t.Fatalf("按键读回：present=%v hasPlan=%v err=%v", present, found.HasPlan, err)
	}
	got := found.Plan.CustomsCitations()
	if len(got) != len(want) {
		t.Fatalf("关务出处读回 %d 条，写入 %d 条", len(got), len(want))
	}
	byCandidate := make(map[string]domain.CustomsApplicabilityCitation, len(got))
	for _, citation := range got {
		byCandidate[citation.Candidate().String()] = citation
	}
	for _, written := range want {
		read, ok := byCandidate[written.Candidate().String()]
		if !ok || read.Judgment() != written.Judgment() || !slices.Equal(read.Versions(), written.Versions()) {
			t.Errorf("候选 %s 的关务出处往返变形：写 %q %v，读 %q %v", written.Candidate(),
				written.Judgment(), written.Versions(), read.Judgment(), read.Versions())
		}
	}
}
