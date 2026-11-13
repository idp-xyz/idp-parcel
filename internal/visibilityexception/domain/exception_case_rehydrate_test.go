package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// Covers: 读回的案件同样要过不变量——合法的三态各自读得回、推进后再折出的快照与读回前一致；处理中没有首次响应、待响应
// 却带着首次响应、已关闭没有结论、未关闭带着结论、归并指向自己、归并却没关闭、时刻早于建立，都读不回来。
func TestACaseIsRehydratedOnlyInAShapeItsLifecycleCanReach(t *testing.T) {
	id, _ := domain.NewCaseID("case-1")
	other, _ := domain.NewCaseID("case-2")
	root, _ := domain.NewTrackedParcelReference("parcel-1")
	scope, _ := domain.NewImpactScopeReference("scope-1")
	team, _ := domain.NewResponsibleTeamReference("team-1")
	at := time.Date(2026, 11, 12, 6, 0, 0, 0, time.UTC)
	base := domain.ExceptionCaseSnapshot{ID: id, Root: root, Scope: scope, Team: team, Phase: domain.CaseAwaitingResponse, EstablishedAt: at}
	with := func(change func(*domain.ExceptionCaseSnapshot)) domain.ExceptionCaseSnapshot {
		snapshot := base
		change(&snapshot)
		return snapshot
	}

	for name, snapshot := range map[string]domain.ExceptionCaseSnapshot{
		"待响应": base,
		"处理中": with(func(s *domain.ExceptionCaseSnapshot) {
			s.Phase, s.FirstResponse = domain.CaseInProgress, at.Add(time.Hour)
		}),
		"直接关闭": with(func(s *domain.ExceptionCaseSnapshot) {
			s.Phase, s.ClosedAt, s.Conclusion = domain.CaseClosed, at.Add(time.Hour), "判定不成立"
		}),
		"已归并": with(func(s *domain.ExceptionCaseSnapshot) {
			s.Phase, s.ClosedAt, s.Conclusion, s.MergedInto = domain.CaseClosed, at.Add(time.Hour), domain.CaseConclusionMerged, other
		}),
	} {
		rebuilt, err := domain.RehydrateCase(snapshot)
		if err != nil {
			t.Fatalf("%s：读不回来：%v", name, err)
		}
		if got := rebuilt.Snapshot(); got.Phase != snapshot.Phase || got.Conclusion != snapshot.Conclusion || got.MergedInto != snapshot.MergedInto ||
			!got.FirstResponse.Equal(snapshot.FirstResponse) || !got.ClosedAt.Equal(snapshot.ClosedAt) {
			t.Fatalf("%s：折出的快照与读回前不一致：%+v", name, got)
		}
	}

	for name, snapshot := range map[string]domain.ExceptionCaseSnapshot{
		"处理中没有首次响应": with(func(s *domain.ExceptionCaseSnapshot) { s.Phase = domain.CaseInProgress }),
		"待响应带着首次响应": with(func(s *domain.ExceptionCaseSnapshot) { s.FirstResponse = at.Add(time.Hour) }),
		"已关闭没有结论":   with(func(s *domain.ExceptionCaseSnapshot) { s.Phase, s.ClosedAt = domain.CaseClosed, at.Add(time.Hour) }),
		"未关闭带着结论":   with(func(s *domain.ExceptionCaseSnapshot) { s.Conclusion = "已恢复" }),
		"归并指向自己": with(func(s *domain.ExceptionCaseSnapshot) {
			s.Phase, s.ClosedAt, s.Conclusion, s.MergedInto = domain.CaseClosed, at.Add(time.Hour), domain.CaseConclusionMerged, id
		}),
		"归并却没关闭": with(func(s *domain.ExceptionCaseSnapshot) { s.MergedInto = other }),
		"关闭早于建立": with(func(s *domain.ExceptionCaseSnapshot) {
			s.Phase, s.ClosedAt, s.Conclusion = domain.CaseClosed, at.Add(-time.Hour), "x"
		}),
		"主状态不在封闭集里": with(func(s *domain.ExceptionCaseSnapshot) { s.Phase = domain.CasePhaseInvalid }),
	} {
		if _, err := domain.RehydrateCase(snapshot); !errors.Is(err, domain.ErrInvalidCase) {
			t.Fatalf("%s：err = %v, want ErrInvalidCase", name, err)
		}
	}
}
