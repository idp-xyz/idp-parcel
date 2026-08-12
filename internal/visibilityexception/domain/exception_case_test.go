package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

var firstHitAt = time.Date(2026, 8, 11, 8, 0, 0, 0, time.UTC)

func openEpisode(t *testing.T) *domain.SignalEpisode {
	t.Helper()
	episode, err := domain.OpenEpisode(
		mustValue(t, domain.NewEpisodeID, "episode-1"),
		mustValue(t, domain.NewExceptionSignalKindReference, "ETA_BREACH_RISK"),
		mustValue(t, domain.NewTrackedParcelReference, "parcel-1"),
		mustValue(t, domain.NewSignalRuleVersionReference, "signal-rules/v3"),
		mustValue(t, domain.NewConfidenceReference, "HIGH/route-deviation"),
		firstHitAt,
	)
	if err != nil {
		t.Fatalf("open episode: %v", err)
	}
	return episode
}

// Covers: VE CONTEXT「同一对象、类型、因果条件和连续影响期内的重复命中更新同一信号
// 发作期。条件明确解除后，该发作期结束；再次发生时建立关联的新发作期」——连续命中
// 计数更新不建新发作期；结束保存解除依据且不再吸收命中；重启换新标识指回前发作期；
// 活跃发作期重启不了。
func TestAnEpisodeAbsorbsHitsEndsAndReopensLinked(t *testing.T) {
	episode := openEpisode(t)

	if err := episode.RecordHit(firstHitAt.Add(time.Hour)); err != nil {
		t.Fatalf("record hit: %v", err)
	}
	if episode.Hits() != 2 {
		t.Fatalf("hits = %d, want 2（重复命中更新同一发作期）", episode.Hits())
	}

	if _, err := episode.ReopenAsLinked(
		mustValue(t, domain.NewEpisodeID, "episode-2"), firstHitAt.Add(2*time.Hour),
	); !errors.Is(err, domain.ErrEpisodeStillActive) {
		t.Fatalf("err = %v; 活跃发作期被重启了", err)
	}

	if err := episode.End("ROUTE_RESTORED/plan-2", firstHitAt.Add(3*time.Hour)); err != nil {
		t.Fatalf("end episode: %v", err)
	}
	if episode.Active() {
		t.Fatal("已结束的发作期还活跃")
	}
	if err := episode.RecordHit(firstHitAt.Add(4 * time.Hour)); !errors.Is(err, domain.ErrEpisodeEnded) {
		t.Fatalf("err = %v; 已结束的发作期吸收了命中", err)
	}

	linked, err := episode.ReopenAsLinked(
		mustValue(t, domain.NewEpisodeID, "episode-2"), firstHitAt.Add(5*time.Hour))
	if err != nil {
		t.Fatalf("reopen linked: %v", err)
	}
	prior, present := linked.PriorEpisode()
	if !present || prior.String() != "episode-1" {
		t.Fatalf("prior = %s present = %v; 新发作期必须指回前发作期", prior, present)
	}
	if linked.Hits() != 1 {
		t.Fatalf("hits = %d; 新发作期自己从头数", linked.Hits())
	}
}

// Covers: VE CONTEXT「高可信、高影响且命中版本化分诊规则的信号可以自动建立或关联
// 案件；低可信、资料不足、可能重复或关联不明确的信号必须先进入分诊」——分诊四走向
// 封闭且规则版本必备（自动建案只能来自版本化规则的命中）。
func TestTriageDemandsItsRuleVersion(t *testing.T) {
	conclusion, err := domain.ConcludeTriage(
		mustValue(t, domain.NewEpisodeID, "episode-1"),
		domain.AutoEstablishCase,
		mustValue(t, domain.NewSignalRuleVersionReference, "triage-rules/v2"),
		firstHitAt.Add(time.Minute),
	)
	if err != nil {
		t.Fatalf("conclude triage: %v", err)
	}
	if conclusion.Outcome() != domain.AutoEstablishCase || conclusion.Rule().String() != "triage-rules/v2" {
		t.Fatalf("conclusion = %#v", conclusion)
	}

	if _, err := domain.ConcludeTriage(
		mustValue(t, domain.NewEpisodeID, "episode-1"),
		domain.AutoEstablishCase,
		domain.SignalRuleVersionReference{},
		firstHitAt,
	); !errors.Is(err, domain.ErrInvalidTriage) {
		t.Fatalf("err = %v; 没有规则版本的自动建案被收下了", err)
	}
}

// Covers: VE CONTEXT「异常案件采用`待响应 → 处理中 → 已关闭`的精简主生命周期。等待
// 客户、监控中、已升级等作为当前工作条件……不扩展为互斥主状态」与「误建的重复案件
// 通过受控归并处理：指定主案件继续处置，其他案件以『已归并』结论关闭并关联主案件。
// 原编号……不得删除或重置」——三态封闭（等待/监控没有格）；接单形成首次响应时间；
// 归并关闭保留关联与原历史；关闭后一切操作拒；自己归并进自己不成立。
func TestACaseWalksItsLeanLifecycleAndMergesControlled(t *testing.T) {
	exceptionCase, err := domain.EstablishCase(
		mustValue(t, domain.NewCaseID, "case-1"),
		mustValue(t, domain.NewTrackedParcelReference, "parcel-1"),
		mustValue(t, domain.NewImpactScopeReference, "impact-scope/v1"),
		mustValue(t, domain.NewResponsibleTeamReference, "ops-team-1"),
		firstHitAt,
	)
	if err != nil {
		t.Fatalf("establish case: %v", err)
	}
	if exceptionCase.Phase() != domain.CaseAwaitingResponse {
		t.Fatalf("phase = %q", exceptionCase.Phase())
	}

	if err := exceptionCase.TakeUp(firstHitAt.Add(30 * time.Minute)); err != nil {
		t.Fatalf("take up: %v", err)
	}
	if exceptionCase.Phase() != domain.CaseInProgress || exceptionCase.FirstResponseAt().IsZero() {
		t.Fatal("接单没有形成首次响应时间")
	}
	if err := exceptionCase.TakeUp(firstHitAt.Add(time.Hour)); !errors.Is(err, domain.ErrInvalidCase) {
		t.Fatalf("err = %v; 接了两次单", err)
	}

	duplicate, err := domain.EstablishCase(
		mustValue(t, domain.NewCaseID, "case-2"),
		mustValue(t, domain.NewTrackedParcelReference, "parcel-1"),
		mustValue(t, domain.NewImpactScopeReference, "impact-scope/v1"),
		mustValue(t, domain.NewResponsibleTeamReference, "ops-team-1"),
		firstHitAt.Add(time.Minute),
	)
	if err != nil {
		t.Fatalf("establish duplicate: %v", err)
	}
	if err := duplicate.MergeInto(duplicate.ID(), firstHitAt.Add(time.Hour)); !errors.Is(err, domain.ErrInvalidCase) {
		t.Fatalf("err = %v; 自己归并进了自己", err)
	}
	if err := duplicate.MergeInto(exceptionCase.ID(), firstHitAt.Add(time.Hour)); err != nil {
		t.Fatalf("merge into: %v", err)
	}
	if duplicate.Phase() != domain.CaseClosed {
		t.Fatal("归并案件没有关闭")
	}
	main, merged := duplicate.MergedInto()
	if !merged || main.String() != "case-1" {
		t.Fatalf("merged into = %s present = %v", main, merged)
	}
	if duplicate.ID().String() != "case-2" {
		t.Fatal("原编号被重置了")
	}

	if err := duplicate.TakeUp(firstHitAt.Add(2 * time.Hour)); !errors.Is(err, domain.ErrCaseClosed) {
		t.Fatalf("err = %v; 已关闭案件被接单了", err)
	}
	if err := duplicate.Close("AGAIN", firstHitAt.Add(2*time.Hour)); !errors.Is(err, domain.ErrCaseClosed) {
		t.Fatalf("err = %v; 关闭关了两次", err)
	}
}
