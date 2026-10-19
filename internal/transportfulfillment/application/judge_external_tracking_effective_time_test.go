package application_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
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

// judgedOverCurrent 形成另一方的一份显式判断：接在此刻的当前版上、回指它。
func judgedOverCurrent(
	t *testing.T,
	double *trackingFactRegistryDouble,
	factRef domain.ExternalTrackingFactReference,
	at time.Time,
	version string,
) ports.ExternalTrackingFactRecord {
	t.Helper()
	tenant, _ := domain.NewTenantID("tenant-1")
	current, found, err := double.FindCurrent(context.Background(), tenant, factRef)
	if err != nil || !found {
		t.Fatalf("另一方读当前版：found=%v err=%v", found, err)
	}
	judgment, err := domain.JudgeEffectiveTimeExplicitly(at)
	if err != nil {
		t.Fatalf("另一方的判断：%v", err)
	}
	ref, err := domain.NewExternalTrackingFactVersion(version)
	if err != nil {
		t.Fatalf("另一方的版本：%v", err)
	}
	judged, err := current.Fact.JudgeEffectiveTime(judgment, ref)
	if err != nil {
		t.Fatalf("另一方换版本：%v", err)
	}
	return ports.ExternalTrackingFactRecord{
		Key:        ports.ExternalTrackingFactKey{TenantID: tenant, Fact: factRef, Version: ref},
		Fact:       judged,
		RecordedAt: trackingRecordedAt.Add(30 * time.Second),
	}
}

// landsFirst 摆出「本方读过当前版之后、落行之前，另一方的判断先提交了」：只在第一次 Save 前落一次。
func landsFirst(
	t *testing.T,
	factRef domain.ExternalTrackingFactReference,
	at time.Time,
	version string,
) func(*trackingFactRegistryDouble) {
	landed := false
	return func(double *trackingFactRegistryDouble) {
		if landed {
			return
		}
		landed = true
		double.records = append(double.records, judgedOverCurrent(t, double, factRef, at, version))
	}
}

// assertLinearChain 断言没有任何一版被回指两次——分叉的链在读面上就是两条当前版。
func assertLinearChain(t *testing.T, records []ports.ExternalTrackingFactRecord) {
	t.Helper()
	successors := map[ports.ExternalTrackingFactKey]int{}
	for _, record := range records {
		if prior, has := record.Fact.Supersedes(); has {
			key := record.Key
			key.Version = prior
			successors[key]++
			if successors[key] > 1 {
				t.Fatalf("版本 %s 被回指了不止一次，链分叉", prior)
			}
		}
	}
}

func TestALosingConcurrentJudgmentWithTheSameTimeEchoesTheWinner(t *testing.T) {
	fixture := newAdoptionFixture(knownCredentials())
	factRef := adoptPending(t, fixture)
	tenant, _ := domain.NewTenantID("tenant-1")
	fixture.registry.beforeSave = landsFirst(t, factRef, trackingReceivedAt, "SYN-EXTV-WINNER")

	result, err := newJudgeHandler(fixture).Judge(t.Context(), application.JudgeEffectiveTimeCommand{
		TenantID: tenant, Fact: factRef.String(), EffectiveAt: trackingReceivedAt,
	})
	if err != nil {
		t.Fatalf("判断：%v", err)
	}
	if result.Outcome() != application.EffectiveTimeAlreadyJudgedAsGiven {
		t.Fatalf("同值输家应答已按同值判过：%s reason=%s", result.Outcome(), result.UndecidedReason())
	}
	record, has := result.Record()
	if !has || record.Key.Version.String() != "SYN-EXTV-WINNER" {
		t.Fatalf("应回显赢家：%q has=%v", record.Key.Version, has)
	}
	if len(fixture.registry.records) != 2 {
		t.Fatalf("输家不得另落一版：%d", len(fixture.registry.records))
	}
	assertLinearChain(t, fixture.registry.records)
}

func TestALosingConcurrentJudgmentWithAnotherTimeIsJudgedOverTheWinner(t *testing.T) {
	fixture := newAdoptionFixture(knownCredentials())
	factRef := adoptPending(t, fixture)
	tenant, _ := domain.NewTenantID("tenant-1")
	fixture.registry.beforeSave = landsFirst(t, factRef, trackingReceivedAt, "SYN-EXTV-WINNER")
	mine := trackingReceivedAt.Add(5 * time.Minute)

	result, err := newJudgeHandler(fixture).Judge(t.Context(), application.JudgeEffectiveTimeCommand{
		TenantID: tenant, Fact: factRef.String(), EffectiveAt: mine,
	})
	if err != nil {
		t.Fatalf("判断：%v", err)
	}
	if result.Outcome() != application.EffectiveTimeJudged {
		t.Fatalf("异值输家应在赢家之上改判：%s reason=%s", result.Outcome(), result.UndecidedReason())
	}
	record, _ := result.Record()
	if prior, has := record.Fact.Supersedes(); !has || prior.String() != "SYN-EXTV-WINNER" {
		t.Fatalf("改判应回指赢家：%q has=%v", prior, has)
	}
	if at, judged := record.Fact.EffectiveAt(); !judged || !at.Equal(mine) {
		t.Fatalf("改判应带本方的时间：%s judged=%v", at, judged)
	}
	if len(fixture.registry.records) != 3 {
		t.Fatalf("待判断、赢家、改判各一版：%d", len(fixture.registry.records))
	}
	assertLinearChain(t, fixture.registry.records)
}

func TestAJudgmentWhoseReadbackFailsIsUndecided(t *testing.T) {
	fixture := newAdoptionFixture(knownCredentials())
	factRef := adoptPending(t, fixture)
	tenant, _ := domain.NewTenantID("tenant-1")
	winner := landsFirst(t, factRef, trackingReceivedAt, "SYN-EXTV-WINNER")
	fixture.registry.beforeSave = func(double *trackingFactRegistryDouble) {
		winner(double)
		double.findErr = trackingRegistryOff
	}

	result, err := newJudgeHandler(fixture).Judge(t.Context(), application.JudgeEffectiveTimeCommand{
		TenantID: tenant, Fact: factRef.String(), EffectiveAt: trackingReceivedAt.Add(5 * time.Minute),
	})
	if err != nil {
		t.Fatalf("判断：%v", err)
	}
	if result.Outcome() != application.EffectiveTimeJudgmentUndecided ||
		result.UndecidedReason() != application.TrackingFactRegistryUnavailable || result.ContinuationReference() == "" {
		t.Fatalf("读不回赢家应答未决并带续办：%s reason=%s cont=%q",
			result.Outcome(), result.UndecidedReason(), result.ContinuationReference())
	}
	if len(fixture.registry.records) != 2 {
		t.Fatalf("读不回时不得盲落一版：%d", len(fixture.registry.records))
	}
}

func TestAJudgmentThatKeepsLosingRetriesBoundedlyThenIsUndecided(t *testing.T) {
	fixture := newAdoptionFixture(knownCredentials())
	factRef := adoptPending(t, fixture)
	tenant, _ := domain.NewTenantID("tenant-1")
	rivals := 0
	fixture.registry.beforeSave = func(double *trackingFactRegistryDouble) {
		rivals++
		double.records = append(double.records, judgedOverCurrent(t, double, factRef,
			trackingReceivedAt.Add(time.Duration(rivals)*time.Minute), fmt.Sprintf("SYN-EXTV-RIVAL-%d", rivals)))
	}

	result, err := newJudgeHandler(fixture).Judge(t.Context(), application.JudgeEffectiveTimeCommand{
		TenantID: tenant, Fact: factRef.String(), EffectiveAt: trackingReceivedAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("判断：%v", err)
	}
	if rivals < 2 || rivals > 5 {
		t.Fatalf("撞键后应重读重试且有界：试了 %d 次", rivals)
	}
	if result.Outcome() != application.EffectiveTimeJudgmentUndecided || result.ContinuationReference() == "" {
		t.Fatalf("一直撞键应答未决并带续办：%s cont=%q", result.Outcome(), result.ContinuationReference())
	}
	assertLinearChain(t, fixture.registry.records)
}
