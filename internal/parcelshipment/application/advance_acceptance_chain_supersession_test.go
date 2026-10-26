package application_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// supersededChainFixture 让委托停在受控补充之后：当前版本是 version-2，version-1 被取代。命令指名的是当前版本，
// 链照常判它；被取代那一版占没占下资金由 controlByVersion 定。
func supersededChainFixture(t *testing.T, priorControl domain.FinancialControlOutcome) (*chainFixture, application.AdvanceAcceptanceChainCommand) {
	t.Helper()
	fixture := newChainFixture(t)
	superseded := supersededRequest(t)
	fixture.requests.saved, fixture.requests.sticky = &superseded, true
	fixture.judgments.controlByVersion = map[string]domain.FinancialControlOutcome{
		"version-1": priorControl,
		"version-2": domain.FinancialControlHeld,
	}
	command := fixture.command(t)
	command.SubmissionVersion = mustValue(t, domain.NewSubmissionVersionID, "version-2")
	return fixture, command
}

// Covers: AT-PS-035「接受提交未成立时按原关联释放已成立各项形成的占用」被取代那一格、PS CONTEXT 词条「控制项
// 结果」与生命周期「已提交期间形成新提交版本」— 资料不足停在`等待受控补充`时旧版本已占下冻结，客户补件形成新版本；
// 链判新版本之前先按原关联放掉旧版本那一笔，放的是旧版本、只放一次，新版本照常进入接受前财务控制。
//
// 先放后判是要点：新版本的控制若先于释放，客户的余额或额度被旧版本那一笔吃着，新版本可能因此受限、被按策略拒掉。
func TestTheChainReleasesASupersededVersionsOccupationBeforeJudgingTheNewOne(t *testing.T) {
	fixture, command := supersededChainFixture(t, domain.FinancialControlHeld)

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AcceptanceChainDecided || result.State() != domain.ShipmentRequestAccepted {
		t.Fatalf("outcome = %q state = %q reason = %q, want DECIDED / ACCEPTED",
			result.Outcome(), result.State(), result.PendingReason())
	}
	if fixture.release.calls != 1 {
		t.Fatalf("release calls = %d, want 1——被取代版本那一笔要放，接受的新版本那一笔不放", fixture.release.calls)
	}
	if got := fixture.release.versions[0].String(); got != "version-1" {
		t.Fatalf("释放指名的版本 = %q, want 被取代的 version-1", got)
	}
	if fixture.release.controlResultID != "SAC-version-1" {
		t.Fatalf("释放认领的控制结果 = %q, want 旧版本自己那一笔 SAC-version-1", fixture.release.controlResultID)
	}
	release := slices.Index(fixture.calls, "release-control")
	firstAssess := slices.Index(fixture.calls, "assess-reachability")
	firstControl := slices.Index(fixture.calls, "apply-financial-control")
	if release < 0 || firstAssess < release || firstControl < release {
		t.Fatalf("calls = %v；旧版本的释放必须先于新版本的判断与控制", fixture.calls)
	}
	if fixture.controller.calls != 1 {
		t.Fatalf("financial control calls = %d, want 1——新版本照常进入接受前财务控制", fixture.controller.calls)
	}
}

// Covers: PS CONTEXT「没有任何控制项成立时不发释放」— 被取代那一版从未形成控制、或唯一一项受限，都没有占用可放；
// 凭空发一次释放会让 settlement-accounting 去认领一笔不存在的占用。
func TestASupersededVersionWithoutOccupationIsNotReleased(t *testing.T) {
	for name, prior := range map[string]domain.FinancialControlOutcome{
		"从未形成控制": domain.FinancialControlOutcomeInvalid,
		"唯一一项受限": domain.FinancialControlRestricted,
		"明确无控制":  domain.FinancialControlNotApplicable,
	} {
		t.Run(name, func(t *testing.T) {
			fixture, command := supersededChainFixture(t, prior)

			result, err := fixture.handler.Handle(context.Background(), command)
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			if result.Outcome() != application.AcceptanceChainDecided {
				t.Fatalf("outcome = %q reason = %q, want DECIDED", result.Outcome(), result.PendingReason())
			}
			if fixture.release.calls != 0 {
				t.Fatalf("release calls = %d（版本 %v）, want 0", fixture.release.calls, fixture.release.versions)
			}
		})
	}
}

// Covers: AT-PS-035 被取代那一格的「释放失败按补偿续办」、ADR-0094 按恢复动作分格 — 旧版本那一笔放不掉时，链停在
// 释放那一步、恢复动作是内部重试，新版本一步都不判：消费门回滚重投，下一轮先放再判。
//
// 不放就往下判有害：新版本的控制会在旧占用还在时执行，受限即可能按策略拒单——拒绝不可覆盖，一次可续办的补偿就被
// 改写成了业务结果。
func TestAFailedSupersededReleaseStopsTheChainBeforeTheNewVersionIsJudged(t *testing.T) {
	fixture, command := supersededChainFixture(t, domain.FinancialControlHeld)
	fixture.release.outcome = ports.ControlReleaseNotFormed

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AcceptanceChainUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
	}
	if result.Stage() != application.AcceptanceChainSupersededReleaseStage {
		t.Fatalf("stage = %q, want SUPERSEDED_CONTROL_RELEASE", result.Stage())
	}
	if result.PendingReason() != application.ControlReleasePending {
		t.Fatalf("reason = %q, want CONTROL_RELEASE_PENDING", result.PendingReason())
	}
	if result.ResumePath() != domain.ResumeByInternalRetry {
		t.Fatalf("resume path = %q, want INTERNAL_RETRY——放不掉的那一笔只有本方推得动", result.ResumePath())
	}
	if result.ContinuationReference().String() == "" {
		t.Fatal("释放没收口却没有续办引用")
	}
	if result.State() != domain.ShipmentRequestSubmitted {
		t.Fatalf("state = %q, want SUBMITTED", result.State())
	}
	if fixture.reachability.calls != 0 || fixture.controller.calls != 0 {
		t.Fatalf("旧占用没放掉，链却判了 %d 个成员的可达性、发起了 %d 次资金占用",
			fixture.reachability.calls, fixture.controller.calls)
	}
}

// Covers: 被取代那一格「释放可重放」— 消费门重投时链再走一遍，释放按原关联再发一次：认领交给提供方的幂等账本，
// 本方不记「放过没有」第二份事实。
func TestARedrivenChainAsksForTheSameSupersededReleaseAgain(t *testing.T) {
	fixture, command := supersededChainFixture(t, domain.FinancialControlHeld)
	fixture.release.outcome = ports.ControlReleaseNotFormed
	if _, err := fixture.handler.Handle(context.Background(), command); err != nil {
		t.Fatalf("first round: %v", err)
	}
	fixture.release.outcome = ports.ControlReleaseSettled

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("second round: %v", err)
	}
	if result.Outcome() != application.AcceptanceChainDecided {
		t.Fatalf("outcome = %q reason = %q, want DECIDED", result.Outcome(), result.PendingReason())
	}
	if len(fixture.release.versions) != 2 ||
		fixture.release.versions[0] != fixture.release.versions[1] ||
		fixture.release.versions[1].String() != "version-1" {
		t.Fatalf("两轮释放指名 %v, want 两次都是 version-1", fixture.release.versions)
	}
}

// Covers: 被取代那一格的「读不回判断不发释放」— 不知道旧版本形成过什么就发，settlement-accounting 无从认领；
// 不放就判新版本又会让旧占用压着新控制。两头都不走，停在释放那一步等内部重试。
func TestAnUnreadableSupersededVersionStopsTheChain(t *testing.T) {
	fixture, command := supersededChainFixture(t, domain.FinancialControlHeld)
	fixture.judgments.err = errors.New("judgment store unavailable")

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.AcceptanceChainUndecided ||
		result.Stage() != application.AcceptanceChainSupersededReleaseStage ||
		result.PendingReason() != application.RecordedJudgmentsUnavailable {
		t.Fatalf("outcome = %q stage = %q reason = %q, want UNDECIDED / SUPERSEDED_CONTROL_RELEASE / RECORDED_JUDGMENTS_UNAVAILABLE",
			result.Outcome(), result.Stage(), result.PendingReason())
	}
	if fixture.release.calls != 0 || fixture.reachability.calls != 0 {
		t.Fatalf("读不回旧版本的判断，却发了 %d 次释放、判了 %d 个成员", fixture.release.calls, fixture.reachability.calls)
	}
}

// Covers: 没有被取代版本的委托不问释放 — 首版走链时只读本版的判断，释放端口一次都不碰。
func TestAFirstVersionChainNeverAsksForASupersededRelease(t *testing.T) {
	fixture := newChainFixture(t)

	if _, err := fixture.handler.Handle(context.Background(), fixture.command(t)); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if fixture.release.calls != 0 {
		t.Fatalf("release calls = %d, want 0", fixture.release.calls)
	}
	for _, version := range fixture.judgments.loadedVersions {
		if version.String() != "version-1" {
			t.Fatalf("读了版本 %q 的判断——首版没有被取代的版本可读", version)
		}
	}
}
