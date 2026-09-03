package application_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件钉「所有者就这一条显式给出有效时间」那一路（ADR-0102 决定三）：待判断的事实经判断换
// 新版本回指前版并交 VE；同一判断重放不再换版本；判不出无中生有的事实。

func newJudgeHandler(fixture *adoptionFixture) *application.JudgeEffectiveTimeHandler {
	return application.NewJudgeEffectiveTimeHandler(application.JudgeEffectiveTimeDeps{
		Facts:      fixture.registry,
		Identities: fixture.identity,
		Downstream: fixture.handoff,
		Clock:      trackingClock{at: trackingRecordedAt.Add(time.Minute)},
	})
}

func adoptPending(t *testing.T, fixture *adoptionFixture) domain.ExternalTrackingFactReference {
	t.Helper()
	result, err := fixture.handler.Adopt(t.Context(), trackingMaterial(t, "evt-1"))
	if err != nil || result.Outcome() != application.TrackingFactAdoptedPendingJudgment {
		t.Fatalf("前置：认领为待判断失败：%v / %s", err, result.Outcome())
	}
	record, _ := result.Record()
	return record.Key.Fact
}

func TestAnExplicitJudgmentTurnsAPendingFactIntoAJudgedVersionAndHandsItOver(t *testing.T) {
	fixture := newAdoptionFixture(knownCredentials())
	factRef := adoptPending(t, fixture)
	tenant, _ := domain.NewTenantID("tenant-1")

	result, err := newJudgeHandler(fixture).Judge(t.Context(), application.JudgeEffectiveTimeCommand{
		TenantID: tenant, Fact: factRef.String(), EffectiveAt: trackingReceivedAt,
	})
	if err != nil {
		t.Fatalf("判断：%v", err)
	}
	if result.Outcome() != application.EffectiveTimeJudged {
		t.Fatalf("应判断成功：%s", result.Outcome())
	}
	record, _ := result.Record()
	at, judged := record.Fact.EffectiveAt()
	if !judged || !at.Equal(trackingReceivedAt) || record.Fact.Effective().Basis() != domain.EffectiveTimeJudgedExplicitly {
		t.Fatalf("新版本应带显式判断：%s judged=%v basis=%s", at, judged, record.Fact.Effective().Basis())
	}
	if _, has := record.Fact.Supersedes(); !has {
		t.Fatal("判断版本应回指待判断的那一版")
	}
	if len(fixture.registry.records) != 2 {
		t.Fatalf("原版本应保留，事实库里应是两版：%d", len(fixture.registry.records))
	}
	if len(fixture.handoff.intents) != 1 || fixture.handoff.intents[0].Record.Key != record.Key {
		t.Fatalf("判断过的版本应交给 visibility-exception：%d", len(fixture.handoff.intents))
	}
}

func TestReplayingTheSameExplicitJudgmentDoesNotMintAnotherVersion(t *testing.T) {
	fixture := newAdoptionFixture(knownCredentials())
	factRef := adoptPending(t, fixture)
	tenant, _ := domain.NewTenantID("tenant-1")
	handler := newJudgeHandler(fixture)
	command := application.JudgeEffectiveTimeCommand{TenantID: tenant, Fact: factRef.String(), EffectiveAt: trackingReceivedAt}
	if _, err := handler.Judge(t.Context(), command); err != nil {
		t.Fatalf("首次判断：%v", err)
	}

	result, err := handler.Judge(t.Context(), command)
	if err != nil {
		t.Fatalf("重放：%v", err)
	}
	if result.Outcome() != application.EffectiveTimeAlreadyJudgedAsGiven {
		t.Fatalf("同一判断重放应交回原版本：%s", result.Outcome())
	}
	if len(fixture.registry.records) != 2 {
		t.Fatalf("重放不得再换版本：%d", len(fixture.registry.records))
	}
}

func TestJudgingAnUnknownFactOrWithoutATimeIsNotAccepted(t *testing.T) {
	fixture := newAdoptionFixture(knownCredentials())
	tenant, _ := domain.NewTenantID("tenant-1")
	handler := newJudgeHandler(fixture)

	unknown, _ := handler.Judge(t.Context(), application.JudgeEffectiveTimeCommand{TenantID: tenant, Fact: "EXTF-nope", EffectiveAt: trackingReceivedAt})
	if unknown.Outcome() != application.EffectiveTimeJudgmentNotAccepted {
		t.Fatalf("判不出无中生有的事实：%s", unknown.Outcome())
	}
	factRef := adoptPending(t, fixture)
	noTime, _ := handler.Judge(t.Context(), application.JudgeEffectiveTimeCommand{TenantID: tenant, Fact: factRef.String()})
	if noTime.Outcome() != application.EffectiveTimeJudgmentNotAccepted {
		t.Fatalf("没有时间不是判断：%s", noTime.Outcome())
	}
	if len(fixture.registry.records) != 1 {
		t.Fatal("未受理不得换版本")
	}
}

func TestEveryJudgmentOutcomeHasAName(t *testing.T) {
	for _, outcome := range []application.EffectiveTimeJudgmentOutcome{
		application.EffectiveTimeJudged, application.EffectiveTimeAlreadyJudgedAsGiven,
		application.EffectiveTimeJudgmentNotAccepted, application.EffectiveTimeJudgmentUndecided,
	} {
		if outcome.String() == "" {
			t.Fatalf("结果 %d 没有名字", outcome)
		}
	}
}
